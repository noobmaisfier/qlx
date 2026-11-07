package qlx_test

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
	"time"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

// Every typed-string enum is written into the SQL, and Go lets any string be
// converted to it. A value outside the closed set must be an error, never text.
func TestStringEnumValuesAreValidated(t *testing.T) {
	const evil = "x); DROP TABLE users; --"
	sel := func(p q.Pred) q.Statement { return q.Select(Users.ID).From(Users).Where(p) }
	bad := map[string]q.Statement{
		"CopyFormat":    q.CopyTableOut(Users).With(q.WithFormat(q.CopyFormat(evil))),
		"OnErrorMode":   q.CopyIn(Users).With(q.WithOnError(q.OnErrorMode(evil))),
		"LogVerbosity":  q.CopyIn(Users).With(q.WithLogVerbosity(q.LogVerbosity(evil))),
		"LockMode":      q.LockTable(q.LockMode(evil), false, Users),
		"Isolation":     q.Begin(q.TxMode{Isolation: q.Isolation(evil)}),
		"Isolation set": q.SetTransaction(q.TxMode{Isolation: q.Isolation(evil)}),
		"PatternKind":   sel(Users.Email.LikeEscape(q.PatternKind(evil), "a", `\`)),
		"CmpOp any":     sel(q.AnySelect(Users.ID, q.CmpOp(evil), q.Select(Users.ID).From(Users))),
		"CmpOp all":     sel(q.AllSelect(Users.ID, q.CmpOp(evil), q.Select(Users.ID).From(Users))),
		"JSONKind":      sel(q.IsJSON(Users.Email, q.JSONKind(evil))),
		"JSONKind not":  sel(q.IsNotJSON(Users.Email, q.JSONKind(evil))),
		"Privilege":     q.Grant(q.Privilege(evil)).OnTables(Users).To(q.NewRole("bob")),
		"Revoke":        q.Revoke(q.PrivSelect, q.Privilege(evil)).OnTables(Users).From(q.NewRole("bob")),
		"ExtractField":  q.Select(q.Extract[time.Time](q.ExtractField(evil), Users.CreatedAt.TimeExpr)).From(Users),
		"XMLParse":      q.Select(q.XMLParse(q.XMLKind(evil), Users.Email).Expr()).From(Users),
		"zero Delim":    q.CopyIn(Users).With(q.WithDelimiter(q.Delim{})),
		"zero quote":    q.CopyIn(Users).With(q.WithQuote(q.Delim{})),
	}
	for name, st := range bad {
		out, err := q.Render(q.Dialect{Version: q.PG19}, st)
		if !errors.Is(err, q.ErrShape) {
			t.Errorf("%s: want ErrShape, got %v (sql %q)", name, err, out.SQL)
		}
		if strings.Contains(out.SQL, "DROP") {
			t.Errorf("%s: the value reached the SQL: %s", name, out.SQL)
		}
	}
	// the provided constants still render
	good := map[string]q.Statement{
		"format":    q.CopyTableOut(Users).With(q.WithFormat(q.CopyCSV)),
		"lock":      q.LockTable(q.LockShareRowExclusive, true, Users),
		"begin":     q.Begin(q.TxMode{Isolation: q.Serializable, ReadOnly: true, Deferrable: true}),
		"set":       q.SetTransaction(q.TxMode{Isolation: q.RepeatableRead}),
		"like":      sel(Users.Email.LikeEscape(q.PatNotILike, "a%", `\`)),
		"any":       sel(q.AnySelect(Users.ID, q.OpGte, q.Select(Users.ID).From(Users))),
		"isjson":    sel(q.IsJSON(Users.Email, q.JSONObjectKind)),
		"isjson-an": sel(q.IsJSON(Users.Email, q.JSONAny)),
		"grant":     q.Grant(q.PrivSelect, q.PrivUpdate).OnTables(Users).To(q.NewRole("bob")),
		"extract":   q.Select(q.Extract[time.Time](q.FieldEpoch, Users.CreatedAt.TimeExpr)).From(Users),
		"delimiter": q.CopyIn(Users).With(q.WithDelimiter(q.DelimTab)),
	}
	for name, st := range good {
		if _, err := q.Render(q.Dialect{Version: q.PG19}, st); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// SET TRANSACTION shares the rules of BEGIN: PostgreSQL accepts DEFERRABLE
// anywhere and silently ignores it unless SERIALIZABLE READ ONLY.
func TestSetTransactionValidatesDeferrable(t *testing.T) {
	d := q.Dialect{Version: q.PG18}
	for _, m := range []q.TxMode{{Deferrable: true}, {Isolation: q.Serializable, Deferrable: true}, {ReadOnly: true, Deferrable: true}} {
		if _, err := q.Render(d, q.SetTransaction(m)); !errors.Is(err, q.ErrShape) {
			t.Errorf("%+v: want ErrShape, got %v", m, err)
		}
	}
	if _, err := q.Render(d, q.SetTransaction(q.TxMode{Isolation: q.Serializable, ReadOnly: true, Deferrable: true})); err != nil {
		t.Error(err)
	}
}

// A new exported `type X string` is written into SQL unless proven otherwise:
// it needs a valid() method checked at render time, or an entry below that
// says why it cannot reach the SQL text.
func TestEveryStringTypeIsValidatedOrBound(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	notText := map[string]string{
		"TruncUnit":   "bound as a parameter: date_trunc($1, ...)",
		"JoinMethod":  "goes into the plan advice, itself a bound parameter",
		"ReindexKind": "validated by a switch in its render",
		"kwNode":      "unexported, built from constants",
		"rawType":     "unexported, built from constants",
	}
	declared := map[string]bool{}
	hasValid := map[string]bool{}
	for _, p := range pkgs {
		for _, f := range p.Files {
			for _, d := range f.Decls {
				switch d := d.(type) {
				case *ast.GenDecl:
					for _, sp := range d.Specs {
						if ts, ok := sp.(*ast.TypeSpec); ok {
							if id, ok := ts.Type.(*ast.Ident); ok && id.Name == "string" {
								declared[ts.Name.Name] = true
							}
						}
					}
				case *ast.FuncDecl:
					if d.Recv != nil && d.Name.Name == "valid" && len(d.Recv.List) == 1 {
						if id, ok := d.Recv.List[0].Type.(*ast.Ident); ok {
							hasValid[id.Name] = true
						}
					}
				}
			}
		}
	}
	for name := range declared {
		if _, ok := notText[name]; !ok && !hasValid[name] {
			t.Errorf("type %s string has no valid() method and is not listed as bound or validated: it may be written into SQL unchecked", name)
		}
	}
	for name := range notText {
		if !declared[name] {
			t.Errorf("notText lists %s, which is not a string type any more", name)
		}
	}
}
