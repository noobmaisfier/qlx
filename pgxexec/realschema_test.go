package pgxexec_test

// Real-world schemas. Each file in testdata/schemas (and, when QLX_SCHEMA_DIR
// is set, every *.sql in that directory: Pagila, Chinook, Northwind or your
// own) goes through everything a user does with it, in a database of its own:
//
//	qlx init -> migration -> qlx gen -> qlx check -> go vet
//	-> a program that, for every generated table, renders SELECT of all its
//	   columns, asks the server to PREPARE it, checks the row type against the
//	   server's description (VerifyRow) and runs it
//
// The first line of a file may say which schemas to generate, as
// "-- schemas: public" or "-- schemas: app, billing" (default: public). Further
// "-- qlx: key: value" lines at the top are added to qlx.yaml (for example
// "-- qlx: timestamptz: qt").
//
// The bundled schemas are written for this repository, in the style of the
// classic sample databases, and carry no third-party licence. Needs the same
// things as TestNewUserEndToEnd, and a role that may CREATE DATABASE.

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"text/template"

	"github.com/jackc/pgx/v5"

	"github.com/noobmaisfier/qlx/internal/gover"
	"github.com/noobmaisfier/qlx/internal/pgtool"
)

const rsMain = `package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"

	qlx "github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/pgxexec"

	"example.com/app/db"
)

var _ = pgx.ErrNoRows

func main() {
	ctx := context.Background()
	c, err := pgxexec.Connect(ctx, os.Getenv("DATABASE_URL"), pgxexec.ConnectOptions{})
	if err != nil {
		fmt.Println("connect:", err)
		os.Exit(1)
	}
	defer c.Close()
	conn, err := c.Pool.Acquire(ctx)
	if err != nil {
		fmt.Println("acquire:", err)
		os.Exit(1)
	}
	defer conn.Release()
	bad := 0
	check := func(name string, err error) {
		if err != nil {
			fmt.Printf("FAIL %s: %v\n", name, err)
			bad++
		}
	}
{{range .}}
	{
		st, err := c.Render(qlx.Select(db.{{.Sym}}.All()...).From(db.{{.Sym}}))
		if err == nil {
			err = pgxexec.VerifyRow[db.{{.Row}}](ctx, conn.Conn(), st)
		}
		if err == nil {
			_, err = pgxexec.AllScan[db.{{.Row}}](ctx, c.DB(), st)
		}
		check("{{.Sym}}", err)
	}
{{end}}
	if bad > 0 {
		os.Exit(1)
	}
	fmt.Println("tables ok:", {{len .}})
}
`

type rsTable struct{ Sym, Row string }

var (
	rsSym      = regexp.MustCompile(`(?m)^var (\w+) = new\w+\(""\)$`)
	rsRow      = regexp.MustCompile(`(?m)^func \(r \*(\w+)\) ScanTargets\(\) \[\]any`)
	rsEmptyAll = regexp.MustCompile(`return \[\]qlx\.Selectable\{\}`)
)

// rsTables lists the (symbol, row type) pair of every generated table that has
// at least one column.
func rsTables(t *testing.T, dbDir string) []rsTable {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(dbDir, "*.go"))
	sort.Strings(files)
	var out []rsTable
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		s, r := rsSym.FindSubmatch(b), rsRow.FindSubmatch(b)
		if s == nil || r == nil || rsEmptyAll.Match(b) {
			continue
		}
		out = append(out, rsTable{string(s[1]), string(r[1])})
	}
	return out
}

// rsConfigLines returns the "-- qlx: key: value" lines at the top of a file.
func rsConfigLines(sql string) []string {
	var out []string
	for _, l := range strings.Split(sql, "\n") {
		if !strings.HasPrefix(l, "--") {
			break
		}
		if rest, ok := strings.CutPrefix(l, "-- qlx:"); ok {
			out = append(out, strings.TrimSpace(rest))
		}
	}
	return out
}

func rsSchemas(sql string) []string {
	first, _, _ := strings.Cut(sql, "\n")
	if rest, ok := strings.CutPrefix(first, "-- schemas:"); ok {
		var out []string
		for _, s := range strings.Split(rest, ",") {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return []string{"public"}
}

// rsDSN points a DSN at another database.
func rsDSN(dsn, db string) string {
	if strings.Contains(dsn, "://") {
		u, err := url.Parse(dsn)
		if err == nil {
			u.Path = "/" + db
			return u.String()
		}
	}
	return dsn + " dbname=" + db
}

func TestRealWorldSchemas(t *testing.T) {
	dsn := os.Getenv("QLX_TEST_DSN")
	if dsn == "" {
		t.Skip("QLX_TEST_DSN not set")
	}
	if _, err := pgtool.FindBin(); err != nil {
		t.Skip(err)
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go command")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob("testdata/schemas/*.sql")
	if dir := os.Getenv("QLX_SCHEMA_DIR"); dir != "" {
		extra, _ := filepath.Glob(filepath.Join(dir, "*.sql"))
		files = append(files, extra...)
	}
	if len(files) == 0 {
		t.Fatal("no schema files")
	}

	// tools shared by every case
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "qlx")
	rsRun(t, root, nil, "go", "build", "-o", bin, "github.com/noobmaisfier/qlx/cmd/qlx")
	gomod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	gosum, err := os.ReadFile("go.sum")
	if err != nil {
		t.Fatal(err)
	}
	var reqs, repls []string
	inReq := false
	for _, l := range strings.Split(string(gomod), "\n") {
		switch {
		case strings.HasPrefix(l, "require ("):
			inReq = true
		case l == ")":
			inReq = false
		case inReq && !strings.Contains(l, "noobmaisfier/qlx"):
			reqs = append(reqs, l)
		case strings.HasPrefix(l, "replace ") && !strings.Contains(l, "noobmaisfier/qlx"):
			repls = append(repls, l)
		}
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close(ctx) })
	tmpl := template.Must(template.New("main").Parse(rsMain))

	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".sql")
		t.Run(name, func(t *testing.T) {
			sqlBytes, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			sql := string(sqlBytes)
			app := filepath.Join(tmp, "app_"+name)
			run := func(env []string, prog string, args ...string) string {
				t.Helper()
				return rsRun(t, app, env, prog, args...)
			}
			rsRun(t, tmp, nil, bin, "init", "-dir", app)
			old, _ := filepath.Glob(filepath.Join(app, "migrations", "*.sql"))
			for _, f := range old { // init writes a sample migration; the schema under test replaces it
				os.Remove(f)
			}
			rsWrite(t, filepath.Join(app, "migrations", "001_schema.sql"), sql)
			cfgPath := filepath.Join(app, "qlx.yaml")
			cfg, _ := os.ReadFile(cfgPath)
			var lines []string
			for _, l := range strings.Split(string(cfg), "\n") {
				if !strings.HasPrefix(l, "schemas:") && !strings.HasPrefix(l, "targets:") {
					lines = append(lines, l)
				}
			}
			lines = append(lines, "schemas: ["+strings.Join(rsSchemas(sql), ", ")+"]", "targets: [16, 17, 18, 19]")
			lines = append(lines, rsConfigLines(sql)...)
			lines = append(lines, "")
			rsWrite(t, cfgPath, strings.Join(lines, "\n"))

			run(nil, bin, "gen")
			run(nil, bin, "check") // generation is deterministic and up to date

			tables := rsTables(t, filepath.Join(app, "db"))
			if len(tables) < 3 {
				t.Fatalf("only %d tables generated from %s", len(tables), file)
			}
			var prog bytes.Buffer
			if err := tmpl.Execute(&prog, tables); err != nil {
				t.Fatal(err)
			}
			rsWrite(t, filepath.Join(app, "go.mod"), "module example.com/app\n\ngo "+gover.Directive()+"\n\nrequire (\n"+
				"\tgithub.com/noobmaisfier/qlx v0.0.0\n\tgithub.com/noobmaisfier/qlx/pgxexec v0.0.0\n"+strings.Join(reqs, "\n")+"\n)\n\nreplace (\n"+
				"\tgithub.com/noobmaisfier/qlx => "+root+"\n\tgithub.com/noobmaisfier/qlx/pgxexec => "+filepath.Join(root, "pgxexec")+"\n)\n"+
				strings.Join(repls, "\n")+"\n")
			rsWrite(t, filepath.Join(app, "go.sum"), string(gosum))
			rsWrite(t, filepath.Join(app, "main.go"), prog.String())

			env := []string{"GOFLAGS=-mod=mod"}
			exe := filepath.Join(tmp, "app_"+name+".bin")
			rsGo := func(args ...string) {
				t.Helper()
				cmd := exec.Command("go", args...)
				cmd.Dir = app
				cmd.Env = append(os.Environ(), env...)
				out, err := cmd.CombinedOutput()
				if err != nil {
					if rsOffline(string(out)) {
						t.Skipf("modules not reachable: %s", out)
					}
					t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
				}
			}
			rsGo("build", "-o", exe, ".")
			rsGo("vet", "./...")
			if tool := os.Getenv("QLX_VETTOOL"); tool != "" { // the go/analysis analyzer, when built
				rsGo("vet", "-vettool="+tool, "./...")
			}

			// a database of its own for the schema
			dbName := "qlx_rs_" + strings.NewReplacer("-", "_", ".", "_").Replace(name)
			quoted := pgx.Identifier{dbName}.Sanitize()
			if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+quoted+" WITH (FORCE)"); err != nil {
				t.Fatal(err)
			}
			if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
				t.Fatalf("CREATE DATABASE (the role needs the privilege): %v", err)
			}
			t.Cleanup(func() { admin.Exec(ctx, "DROP DATABASE IF EXISTS "+quoted+" WITH (FORCE)") })
			target := rsDSN(dsn, dbName)
			tc, err := pgx.Connect(ctx, target)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tc.Exec(ctx, sql); err != nil {
				tc.Close(ctx)
				t.Fatalf("the schema file does not load on this server: %v", err)
			}
			tc.Close(ctx)
			out := run([]string{"DATABASE_URL=" + target}, exe)
			if !strings.Contains(out, fmt.Sprintf("tables ok: %d", len(tables))) {
				t.Fatalf("program output:\n%s", out)
			}
			run([]string{"DATABASE_URL=" + target}, bin, "verify")
			t.Logf("%s: %d tables generated, compiled, prepared and read", name, len(tables))
		})
	}
}

func rsOffline(out string) bool {
	return strings.Contains(out, "dial tcp") || strings.Contains(out, "proxy") || strings.Contains(out, "lookup ")
}

func rsWrite(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func rsRun(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out.String())
	}
	return out.String()
}
