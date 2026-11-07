package qlx_test

import (
	"github.com/noobmaisfier/qlx/internal/gover"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The compile-fail corpus: every rule that the Go compiler is supposed to
// enforce has an example that must NOT build, with the reason it must give.
// A positive control proves the harness does not simply fail everything.
func TestCompileFailCorpus(t *testing.T) {
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		body string
		want string // substring of the compiler output; "" means must build
	}{
		{"control: a valid query builds", `_ = q.Select(fx.Users.ID).From(fx.Users).Where(fx.Users.ID.Eq(1))`, ""},
		{"SUM over int8 needs SumBig", `_ = q.Sum(fx.Users.Balance)`, "SmallInt"},
		{"SUM over text", `_ = q.Sum(fx.Users.Email)`, "SmallInt"},
		{"aggregate in WHERE", `_ = q.Select(fx.Users.ID).From(fx.Users).Where(q.CountAll().Gt(1))`, "Pred"},
		{"aggregate as an operand", `_ = q.Count(q.CountAll())`, "Operand"},
		{"OLD in WHERE", `_ = q.Select(fx.Users.ID).From(fx.Users).Where(fx.Accounts.Balance.Old().Eq(1))`, "Eq"},
		{"OLD in SELECT", `_ = q.Select(fx.Accounts.Balance.Old())`, "Selectable"},
		{"wrong literal type", `_ = fx.Users.ID.Eq("x")`, "string"},
		{"LIKE on a number", `_ = fx.Users.Balance.Like("x")`, "Like"},
		{"arithmetic on text", `_ = fx.Users.Email.Add("x")`, "Add"},
		{"NULL into a NOT NULL column", `_ = fx.Users.Email.To(qt.None[string]())`, "qt.Null"},
		{"comparing columns of different types", `_ = fx.Users.ID.EqX(fx.Users.Email)`, "Operand"},
		{"parameter of the wrong type", `p := q.NewParam[string](); _ = fx.Users.ID.EqX(p)`, "Operand"},
		{"SQL text as a select item", `_ = q.Select("id")`, "Selectable"},
		{"SQL text as a table", `_ = q.Select(fx.Users.ID).From("users")`, "FromItem"},
		{"SQL text as a predicate", `_ = q.Select(fx.Users.ID).From(fx.Users).Where("id = 1")`, "Pred"},
		{"comparison operator on a bool-less column family", `_ = fx.Users.Tier.Gt("a")`, "Gt"},
		{"SQL text as a join item", `_ = q.Select(fx.Users.ID).From(fx.Users).Join("orders")`, "FromItem"},
		{"LIMIT with a string", `_ = q.Select(fx.Users.ID).From(fx.Users).Limit("5")`, "string"},
		{"nullable value for a NOT NULL comparison", `_ = fx.Users.Email.Eq(qt.None[string]())`, "qt.Null"},
		{"IN with the wrong element type", `_ = fx.Users.ID.In("a")`, "string"},
		{"NotNull on a non-nullable operand", `_ = q.NotNull(fx.Users.ID)`, "Operand"},
		{"Coalesce fallback of the wrong type", `_ = q.Coalesce(fx.Users.Nickname, 5)`, "untyped int"},
		{"And with a non-predicate", `_ = fx.Users.ID.Eq(1).And(1)`, "Pred"},
		{"ORDER BY a bare column", `_ = q.Select(fx.Users.ID).From(fx.Users).OrderBy(fx.Users.ID)`, "OrderTerm"},
		{"arithmetic on a timestamp", `_ = fx.Users.CreatedAt.Add(1)`, "Add"},
		{"NOT LIKE on a number", `_ = fx.Users.Balance.NotLike("x")`, "NotLike"},
		{"BETWEEN SYMMETRIC on text", `_ = fx.Users.Email.BetweenSymmetric("a", "b")`, "BetweenSymmetric"},
		{"COLLATE with a non-string name", `_ = fx.Users.Email.Collate(1)`, "string"},
		{"function result of the wrong type in a typed slot", `var x q.Expr[string] = q.CountAll(); _ = x`, "cannot use"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			write := func(name, content string) {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			write("go.mod", "module cf\n\ngo "+gover.Directive()+"\n\nrequire github.com/noobmaisfier/qlx v0.0.0\n\nreplace github.com/noobmaisfier/qlx => "+root+"\n")
			write("main.go", `package main

import (
	q "github.com/noobmaisfier/qlx"
	fx "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qt"
)

var _ = q.Select
var _ = fx.Users
var _ = qt.None[string]

func main() {
	`+c.body+`
}
`)
			cmd := exec.Command("go", "build", "-o", os.DevNull, ".")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOTOOLCHAIN=local")
			out, err := cmd.CombinedOutput()
			if c.want == "" {
				if err != nil {
					t.Fatalf("control must build:\n%s", out)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected a compile error mentioning %q, but it built", c.want)
			}
			if !strings.Contains(string(out), c.want) {
				t.Fatalf("compile error does not mention %q:\n%s", c.want, out)
			}
			// Messages stay short: the compiler's complaint about one
			// mistake fits in three lines of this file.
			if n := strings.Count(string(out), "main.go:"); n > 3 {
				t.Errorf("error is %d lines long, budget 3:\n%s", n, out)
			}
			t.Logf("rejected as intended: %s", firstLine(string(out)))
		})
	}
}

func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, "main.go") {
			return l
		}
	}
	return s
}
