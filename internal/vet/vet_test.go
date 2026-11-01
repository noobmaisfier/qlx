package vet

import (
	"github.com/noobmaisfier/qlx/internal/gover"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func qlxRoot(t *testing.T) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

func project(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	gomod := "module example.com/app\n\ngo " + gover.Directive() + "\n\nrequire github.com/noobmaisfier/qlx v0.0.0\n\nreplace github.com/noobmaisfier/qlx => " + qlxRoot(t) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func codes(ds []Diagnostic) string {
	var out []string
	for _, d := range ds {
		out = append(out, d.Code)
	}
	return strings.Join(out, ",")
}

const header = `package app

import (
	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

var _ = q.PG17
`

func TestFindings(t *testing.T) {
	src := header + `
var A = q.Register("a", q.Select(Users.ID).From(Users))
var B = q.Register("a", q.Select(Orders.ID).From(Orders)) // V003 duplicate

func discard() {
	s := q.Select(Users.ID).From(Users)
	s.Where(Users.ID.Eq(1)) // V001
	_ = s
}

func render(s q.Statement) {
	q.Render(q.Dialect{}, s)                 // V004 discarded
	st, _ := q.Render(q.Dialect{}, s)        // V004 error ignored
	_, _, _ = q.SQL(q.Dialect{}, s)          // V004 error ignored
	_ = st
}

func unbounded() q.Statement {
	return q.Update(Users).Set(Users.Balance.To(0)) // V002
}

func bounded() q.Statement {
	return q.Update(Users).Set(Users.Balance.To(0)).Where(Users.ID.Eq(1))
}

func everyRow() q.Statement { return q.DeleteFrom(Orders).All() }

func deleteAll() q.Statement { return q.DeleteFrom(Orders) } // V002

func later() {
	u := q.Update(Users).Set(Users.Balance.To(0)) // completed below: no finding
	u = u.Where(Users.ID.Eq(2))
	_ = u
}

func inFunc() {
	q.Register("late", q.Select(Users.ID).From(Users)) // V005
}

func dynamic(n string) {
	_ = q.Register(n, q.Select(Users.ID).From(Users)) // V003 + V005
}

func init() {
	q.Register("in_init", q.Select(Users.ID).From(Users)) // fine
}
`
	ds, err := Run(project(t, src), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := codes(ds)
	want := "V003,V001,V004,V004,V004,V002,V002,V005,V003,V005"
	if got != want {
		for _, d := range ds {
			t.Log(d)
		}
		t.Fatalf("codes %s, want %s", got, want)
	}
	if !strings.Contains(ds[0].Msg, "main.go:") {
		t.Errorf("duplicate message should point at the first registration: %s", ds[0].Msg)
	}
}

func TestCleanProgramHasNoFindings(t *testing.T) {
	src := header + `
var List = q.Register("list", q.Select(Users.ID, Users.Email).From(Users).Where(Users.Tier.Eq("pro")).Limit(10))

func build(min int64) (string, []any, error) {
	st := q.Select(Users.ID).From(Users).Where(Users.Balance.Gte(min))
	return q.SQL(q.Dialect{}, st)
}
`
	ds, err := Run(project(t, src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 0 {
		t.Fatalf("false positives: %v", ds)
	}
}

// vet needs code that compiles (it reads the compiler's export data, like
// go vet); a broken package must give the compiler's message, not a panic.
func TestBrokenPackageReportsCompilerError(t *testing.T) {
	_, err := Run(project(t, header+"\nfunc broken() { undefinedThing() }\n"), nil)
	if err == nil || !strings.Contains(err.Error(), "undefinedThing") {
		t.Fatalf("want the compiler's error, got %v", err)
	}
}
