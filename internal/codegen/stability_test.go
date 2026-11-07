package codegen_test

// What users see of the generator is the code it writes: the names of the
// tables, rows and columns, and the Go type of every field. Two tests keep
// that from changing by surprise.
//
//   - TestGeneratedSurfaceGolden snapshots the exported surface of the code
//     generated for the fixture and for every hand-written hostile schema.
//     Any change to a name or a type shows up as a diff of
//     testdata/surface.golden, which is reviewed like an API change. After
//     reviewing it:  QLX_UPDATE_GOLDEN=1 go test -run SurfaceGolden ./internal/codegen
//   - TestAddingThingsNeverChangesExistingNames adds tables, columns, enums,
//     functions, constraints and whole schemas to a database and requires
//     every exported name and type generated before to still be there,
//     unchanged. A name must depend on the object and the config, not on what
//     else happens to exist.

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/noobmaisfier/qlx/internal/catalog"
	"github.com/noobmaisfier/qlx/internal/codegen"
	"github.com/noobmaisfier/qlx/internal/pgtool"
)

// surface lists the exported declarations of generated Go source, one per
// line: functions and methods with their signatures, and every exported field
// of every struct with its type. Bodies, comments and literals are left out.
func surface(t *testing.T, name string, src []byte) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, 0)
	if err != nil {
		t.Fatalf("%s does not parse: %v", name, err)
	}
	expr := func(e ast.Node) string {
		var b bytes.Buffer
		printer.Fprint(&b, fset, e)
		return strings.Join(strings.Fields(b.String()), " ")
	}
	var out []string
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if !d.Name.IsExported() {
				continue
			}
			recv := ""
			if d.Recv != nil && len(d.Recv.List) == 1 {
				rt := expr(d.Recv.List[0].Type)
				if !ast.IsExported(strings.TrimLeft(strings.SplitN(rt, "[", 2)[0], "*")) {
					continue
				}
				recv = "(" + rt + ") "
			}
			out = append(out, "func "+recv+d.Name.Name+strings.TrimPrefix(expr(d.Type), "func"))
		case *ast.GenDecl:
			for _, sp := range d.Specs {
				switch sp := sp.(type) {
				case *ast.TypeSpec:
					if !sp.Name.IsExported() {
						continue
					}
					st, ok := sp.Type.(*ast.StructType)
					if !ok {
						out = append(out, "type "+sp.Name.Name+" "+expr(sp.Type))
						continue
					}
					out = append(out, "type "+sp.Name.Name+" struct")
					for _, fl := range st.Fields.List {
						for _, n := range fl.Names {
							if n.IsExported() {
								out = append(out, "field "+sp.Name.Name+"."+n.Name+" "+expr(fl.Type))
							}
						}
					}
				case *ast.ValueSpec:
					for _, n := range sp.Names {
						if n.IsExported() {
							out = append(out, d.Tok.String()+" "+n.Name)
						}
					}
				}
			}
		}
	}
	return out
}

// surfaceOf is the sorted union of the surface of every file of a result.
func surfaceOf(t *testing.T, res *codegen.Result, schemas ...string) []string {
	t.Helper()
	var all []string
	for fn, src := range res.Files {
		all = append(all, surface(t, fn, src)...)
	}
	sort.Strings(all)
	for i, l := range all {
		for _, s := range schemas {
			l = strings.ReplaceAll(l, s, "SCHEMA")
		}
		all[i] = l
	}
	return all
}

var schemaSeq atomic.Int32

// build creates schemas on srv from the statements (each gets its schema name
// as %[1]s) and generates code for them, the first being the principal one.
func build(t *testing.T, srv *pgtool.Server, cfg codegen.Config, stmts ...string) (*codegen.Result, []string) {
	t.Helper()
	var names []string
	for _, sql := range stmts {
		n := fmt.Sprintf("st%d", schemaSeq.Add(1))
		if err := srv.Conn().Exec("CREATE SCHEMA " + n + ";\n" + fmt.Sprintf(sql, n)); err != nil {
			t.Fatalf("schema %s: %v\n%s", n, err, sql)
		}
		names = append(names, n)
	}
	raw, err := catalog.Load(srv.Conn(), names)
	if err != nil {
		t.Fatal(err)
	}
	_, s, err := catalog.Seal(raw)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Package, cfg.Schemas = "p", names
	res, err := codegen.Generate(s, cfg)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return res, names
}

func TestGeneratedSurfaceGolden(t *testing.T) {
	srv, err := pgtool.StartEphemeral("")
	if err != nil {
		t.Skipf("no PostgreSQL server binaries: %v", err)
	}
	t.Cleanup(srv.Close)

	variants := []struct {
		name string
		cfg  codegen.Config
	}{
		{"def", codegen.Config{}},
		{"alt", codegen.Config{IDs: "string", Date: "stdlib", UUID: "bytes", Timestamptz: "qt", ArrayNulls: true}},
	}
	var b strings.Builder
	for _, c := range oddCases() {
		for _, v := range variants {
			if v.name == "alt" && c.name != "every_type" && c.name != "keys" {
				continue // the options change types; two schemas show it
			}
			schema := fmt.Sprintf("gold%d", schemaSeq.Add(1))
			if err := srv.Conn().Exec("CREATE SCHEMA " + schema + ";\n" + c.sql(schema)); err != nil {
				continue // the server refuses the schema; not a generator matter
			}
			raw, err := catalog.Load(srv.Conn(), []string{schema})
			if err != nil {
				t.Fatal(err)
			}
			_, s, err := catalog.Seal(raw)
			if err != nil {
				continue
			}
			cfg := v.cfg
			cfg.Package, cfg.Schemas = "p", []string{schema}
			res, err := codegen.Generate(s, cfg)
			if err != nil {
				fmt.Fprintf(&b, "== %s/%s: refused: %s\n\n", c.name, v.name, strings.ReplaceAll(firstLine(err.Error()), schema, "SCHEMA"))
				continue
			}
			fmt.Fprintf(&b, "== %s/%s\n", c.name, v.name)
			for _, l := range surfaceOf(t, res, schema) {
				b.WriteString(l + "\n")
			}
			b.WriteString("\n")
		}
	}
	got := b.String()
	const path = "testdata/surface.golden"
	if os.Getenv("QLX_UPDATE_GOLDEN") == "1" {
		os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (create it with QLX_UPDATE_GOLDEN=1)", err)
	}
	if string(want) != got {
		wl, gl := strings.Split(string(want), "\n"), strings.Split(got, "\n")
		seen := map[string]bool{}
		for _, l := range wl {
			seen[l] = true
		}
		var added, removed []string
		for _, l := range gl {
			if !seen[l] {
				added = append(added, l)
			}
		}
		gs := map[string]bool{}
		for _, l := range gl {
			gs[l] = true
		}
		for _, l := range wl {
			if !gs[l] {
				removed = append(removed, l)
			}
		}
		show := func(ls []string) string {
			if len(ls) > 25 {
				ls = append(ls[:25:25], fmt.Sprintf("... and %d more", len(ls)-25))
			}
			return strings.Join(ls, "\n  ")
		}
		t.Errorf("the generated surface changed. Names and types are what users write; review, then QLX_UPDATE_GOLDEN=1.\nremoved (%d):\n  %s\nadded (%d):\n  %s",
			len(removed), show(removed), len(added), show(added))
	}
}

const stableBase = `
CREATE TYPE %[1]s.status AS ENUM ('new', 'done');
CREATE TABLE %[1]s.orgs (id bigint PRIMARY KEY, name text NOT NULL);
CREATE TABLE %[1]s.users (id bigint PRIMARY KEY, email text NOT NULL, org_id bigint REFERENCES %[1]s.orgs (id), st %[1]s.status, tags text[]);
CREATE TABLE %[1]s.status_archived (id bigint PRIMARY KEY);
CREATE FUNCTION %[1]s.double(x int) RETURNS int LANGUAGE sql IMMUTABLE AS 'SELECT x * 2';
`

func TestAddingThingsNeverChangesExistingNames(t *testing.T) {
	srv, err := pgtool.StartEphemeral("")
	if err != nil {
		t.Skipf("no PostgreSQL server binaries: %v", err)
	}
	t.Cleanup(srv.Close)

	mutations := []struct {
		name  string
		extra string   // appended to the base schema
		other []string // further schemas, listed after the base one
		// warns: this mutation cannot keep every name (a Go identifier cannot
		// be two things), so it must at least say so.
		warns string
	}{
		{"unrelated table", `CREATE TABLE %[1]s.audit_log (id bigint PRIMARY KEY, who text);`, nil, ""},
		{"table referencing users", `CREATE TABLE %[1]s.posts (id bigint PRIMARY KEY, author bigint REFERENCES %[1]s.users (id));`, nil, ""},
		{"column on a referenced table", `ALTER TABLE %[1]s.orgs ADD COLUMN plan text;`, nil, ""},
		{"column on the table itself", `ALTER TABLE %[1]s.users ADD COLUMN age int;`, nil, ""},
		{"unique constraint and index", `ALTER TABLE %[1]s.users ADD CONSTRAINT users_email_key UNIQUE (email); CREATE INDEX ON %[1]s.users (org_id);`, nil, ""},
		{"new enum", `CREATE TYPE %[1]s.level AS ENUM ('low', 'high');`, nil, ""},
		{"new enum label", `ALTER TYPE %[1]s.status ADD VALUE 'archived';`, nil,
			// the value becomes the constant StatusArchived, the name the table status_archived had
			"status_archived"},
		{"new function", `CREATE FUNCTION %[1]s.triple(x int) RETURNS int LANGUAGE sql IMMUTABLE AS 'SELECT x * 3';`, nil, ""},
		{"function overload", `CREATE FUNCTION %[1]s.double(x text) RETURNS text LANGUAGE sql IMMUTABLE AS 'SELECT x || x';`, nil, ""},
		{"view", `CREATE VIEW %[1]s.active_users AS SELECT id, email FROM %[1]s.users;`, nil, ""},
		{"another schema with the same names", ``, []string{`
CREATE TYPE %[1]s.status AS ENUM ('x', 'y');
CREATE TABLE %[1]s.orgs (id bigint PRIMARY KEY, other text);
CREATE TABLE %[1]s.users (id bigint PRIMARY KEY, email int);
CREATE TABLE %[1]s.status_archived (id bigint PRIMARY KEY);
CREATE FUNCTION %[1]s.double(x int) RETURNS int LANGUAGE sql IMMUTABLE AS 'SELECT x';`}, ""},
	}
	variants := []struct {
		name string
		cfg  codegen.Config
	}{
		{"def", codegen.Config{}},
		{"alt", codegen.Config{IDs: "string", Date: "stdlib", UUID: "bytes", Timestamptz: "qt", ArrayNulls: true}},
	}
	for _, v := range variants {
		for _, m := range mutations {
			t.Run(v.name+"/"+m.name, func(t *testing.T) {
				baseRes, baseNames := build(t, srv, v.cfg, stableBase)
				mutRes, mutNames := build(t, srv, v.cfg, append([]string{stableBase + m.extra}, m.other...)...)
				want := surfaceOf(t, baseRes, baseNames...)
				have := map[string]bool{}
				for _, l := range surfaceOf(t, mutRes, mutNames...) {
					have[l] = true
				}
				var lost []string
				for _, l := range want {
					if !have[l] {
						lost = append(lost, l)
					}
				}
				if m.warns != "" {
					if len(lost) == 0 {
						t.Errorf("expected %q to be unavoidable; names stayed the same, so update the test", m.name)
					}
					if !strings.Contains(strings.Join(mutRes.Warnings, "\n"), m.warns) {
						t.Errorf("names changed with no warning naming %s: %v", m.warns, mutRes.Warnings)
					}
					return
				}
				if len(lost) > 0 {
					if len(lost) > 20 {
						lost = append(lost[:20], "...")
					}
					t.Errorf("adding %q changed names or types that existed before:\n  %s", m.name, strings.Join(lost, "\n  "))
				}
			})
		}
	}
}
