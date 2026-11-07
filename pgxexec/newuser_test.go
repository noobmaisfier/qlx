package pgxexec_test

// New-user end-to-end test: everything a first-time user does, in order, in a
// scratch project outside the repository:
//
//	qlx init -> write a migration -> qlx gen -> qlx check -> a real program
//	(insert, update, join, delete, VerifyRow) -> go vet
//
// It needs: the Go toolchain, PostgreSQL server binaries (qlx gen starts a
// throwaway server from the migrations), QLX_TEST_DSN naming a database the
// program may create the schema "newuser" in, and the Go module cache (the
// scratch module requires pgx). It skips itself when any of them is missing.

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/noobmaisfier/qlx/internal/gover"
	"github.com/noobmaisfier/qlx/internal/pgtool"
)

const newUserMigration = `
CREATE SCHEMA newuser;
CREATE TYPE newuser.status AS ENUM ('new', 'active', 'closed');
CREATE TABLE newuser.users (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email      text NOT NULL UNIQUE,
    nickname   text,
    tags       text[] NOT NULL DEFAULT '{}',
    settings   jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE newuser.orders (
    id      bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES newuser.users (id),
    total   numeric(12,2) NOT NULL,
    status  newuser.status NOT NULL DEFAULT 'new'
);
`

const newUserMain = `package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"

	qlx "github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/pgxexec"
	"github.com/noobmaisfier/qlx/qt"

	"example.com/app/db"
)

type userOrder struct {
	Email string
	Total qt.Null[qt.Numeric]
}

func (r *userOrder) ScanTargets() []any { return []any{&r.Email, &r.Total} }

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "FAIL:", err)
		os.Exit(1)
	}
}

func main() {
	ctx := context.Background()
	c, err := pgxexec.Connect(ctx, os.Getenv("DATABASE_URL"), pgxexec.ConnectOptions{})
	must(err)
	defer c.Close()

	// insert ... returning
	st, err := c.Render(qlx.InsertInto(db.Users).
		Values(db.NewUserInsert("ada@example.com").Nickname(qt.Some("ada")).Tags([]string{"a", "b"})).
		Returning(db.Users.ID))
	must(err)
	id, err := pgxexec.One[qlx.Tuple1[int64]](ctx, c.DB(), st)
	must(err)

	// partial update
	st, err = c.Render(qlx.Update(db.Users).
		SetPatch(db.UserPatch{Nickname: qt.Opt[qt.Null[string]]{V: qt.Some("countess"), Set: true}}).
		Where(db.Users.ID.Eq(id.V1)))
	must(err)
	_, err = pgxexec.Exec(ctx, c.DB(), st)
	must(err)

	// a user without orders must still appear in a LEFT JOIN, with a NULL total
	st, err = c.Render(qlx.InsertInto(db.Users).Values(db.NewUserInsert("bob@example.com")))
	must(err)
	_, err = pgxexec.Exec(ctx, c.DB(), st)
	must(err)
	st, err = c.Render(qlx.InsertInto(db.Orders).Row(
		db.Orders.UserID.To(id.V1), db.Orders.Total.To(qt.Numeric("12.50")), db.Orders.Status.To(db.StatusActive)))
	must(err)
	_, err = pgxexec.Exec(ctx, c.DB(), st)
	must(err)

	join := qlx.Select(db.Users.Email, db.Orders.Total).From(db.Users).
		LeftJoin(db.Orders).On(db.Orders.UserID.EqX(db.Users.ID)).
		OrderBy(db.Users.Email.Asc())
	st, err = c.Render(join)
	must(err)
	rows, err := pgxexec.AllScan[userOrder](ctx, c.DB(), st)
	must(err)
	if len(rows) != 2 || rows[0].Email != "ada@example.com" || !rows[0].Total.Valid || rows[0].Total.V != "12.50" || rows[1].Total.Valid {
		must(fmt.Errorf("unexpected join rows: %+v", rows))
	}

	// the generated row type matches its table, on a real server
	conn, err := c.Pool.Acquire(ctx)
	must(err)
	defer conn.Release()
	st, err = c.Render(qlx.Select(db.Users.All()...).From(db.Users))
	must(err)
	must(pgxexec.VerifyRow[db.User](ctx, conn.Conn(), st))
	users, err := pgxexec.AllScan[db.User](ctx, c.DB(), st)
	must(err)
	if len(users) != 2 {
		must(fmt.Errorf("want 2 users, got %d", len(users)))
	}

	// the foreign key and the unique constraint are reported as such
	st, err = c.Render(qlx.InsertInto(db.Users).Values(db.NewUserInsert("ada@example.com")))
	must(err)
	if _, err = pgxexec.Exec(ctx, c.DB(), st); !pgxexec.IsUnique(err, "users_email_key") {
		must(fmt.Errorf("want unique violation, got %v", err))
	}
	st, err = c.Render(qlx.DeleteFrom(db.Users).Where(db.Users.ID.Eq(id.V1)))
	must(err)
	if _, err = pgxexec.Exec(ctx, c.DB(), st); !pgxexec.IsForeignKey(err, "orders_user_id_fkey") {
		must(fmt.Errorf("want foreign key violation, got %v", err))
	}
	_ = pgx.ErrNoRows
	fmt.Println("newuser e2e ok")
}
`

func TestNewUserEndToEnd(t *testing.T) {
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
	tmp := t.TempDir()
	run := func(dir string, env []string, name string, args ...string) string {
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

	bin := filepath.Join(tmp, "qlx")
	run(root, nil, "go", "build", "-o", bin, "github.com/noobmaisfier/qlx/cmd/qlx")

	app := filepath.Join(tmp, "app")
	run(tmp, nil, bin, "init", "-dir", app)
	write := func(rel, s string) {
		t.Helper()
		p := filepath.Join(app, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("migrations/001_init.sql", newUserMigration)
	cfgPath := filepath.Join(app, "qlx.yaml")
	cfg, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, l := range strings.Split(string(cfg), "\n") {
		if strings.HasPrefix(l, "schemas:") || strings.HasPrefix(l, "targets:") {
			continue
		}
		lines = append(lines, l)
	}
	lines = append(lines, "schemas: [newuser]", "targets: [16, 17, 18, 19]", "")
	if err := os.WriteFile(cfgPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	run(app, nil, bin, "gen")
	run(app, nil, bin, "check")

	// stale detection: a hand edit of generated code must make check fail
	users := filepath.Join(app, "db", "users.go")
	orig, err := os.ReadFile(users)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(users, append(orig, []byte("\n// edited\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "check")
	cmd.Dir = app
	if err := cmd.Run(); err == nil {
		t.Fatal("qlx check passed on a hand-edited generated file")
	}
	if err := os.WriteFile(users, orig, 0o644); err != nil {
		t.Fatal(err)
	}
	run(app, nil, bin, "check")

	// the module: pgx requirements copied from pgxexec so the module cache serves them
	pgxMod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	var reqs, repls []string
	inReq := false
	for _, l := range strings.Split(string(pgxMod), "\n") {
		switch {
		case strings.HasPrefix(l, "require ("):
			inReq = true
		case l == ")":
			inReq = false
		case inReq && !strings.Contains(l, "noobmaisfier/qlx"):
			reqs = append(reqs, l)
		case strings.HasPrefix(l, "replace ") && !strings.Contains(l, "noobmaisfier/qlx"):
			repls = append(repls, l) // third-party replaces (offline builds) carry over
		}
	}
	sum, err := os.ReadFile("go.sum")
	if err != nil {
		t.Fatal(err)
	}
	write("go.mod", "module example.com/app\n\ngo "+gover.Directive()+"\n\nrequire (\n"+
		"\tgithub.com/noobmaisfier/qlx v0.0.0\n\tgithub.com/noobmaisfier/qlx/pgxexec v0.0.0\n"+
		strings.Join(reqs, "\n")+"\n)\n\nreplace (\n"+
		"\tgithub.com/noobmaisfier/qlx => "+root+"\n"+
		"\tgithub.com/noobmaisfier/qlx/pgxexec => "+filepath.Join(root, "pgxexec")+"\n)\n"+
		strings.Join(repls, "\n")+"\n")
	write("go.sum", string(sum))
	write("main.go", newUserMain)

	env := []string{"GOWORK=off", "GOFLAGS=-mod=mod"}
	build := func(name string, args ...string) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = app
		cmd.Env = append(os.Environ(), env...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			if strings.Contains(string(out), "dial tcp") || strings.Contains(string(out), "proxy") ||
				strings.Contains(string(out), "lookup ") {
				t.Skipf("modules not reachable: %s", out)
			}
			t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
		}
	}
	build("go", "build", "-o", filepath.Join(tmp, "app.bin"), ".")
	build("go", "vet", "./...")

	// the program owns the schema "newuser" in the test database
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(ctx) }) // runs after the reset below
	reset := func() {
		if _, err := conn.Exec(ctx, "DROP SCHEMA IF EXISTS newuser CASCADE"); err != nil {
			t.Fatal(err)
		}
	}
	reset()
	t.Cleanup(reset)
	if _, err := conn.Exec(ctx, newUserMigration); err != nil {
		t.Fatal(err)
	}
	out := run(app, []string{"DATABASE_URL=" + dsn}, filepath.Join(tmp, "app.bin"))
	if !strings.Contains(out, "newuser e2e ok") {
		t.Fatalf("program output: %s", out)
	}
	// qlx verify: the live database matches schema.lock
	run(app, []string{"DATABASE_URL=" + dsn}, bin, "verify")
}
