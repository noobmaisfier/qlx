package qlx_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

func commandCases() []goldenCase {
	sel := q.Select(Users.ID).From(Users).Where(Users.ID.Eq(1))
	pid := q.NewParam[int64]()
	return []goldenCase{
		{name: "explain analyze json", stmt: sel.Explain(q.Analyze, q.Buffers, q.FormatJSON)},
		{name: "explain update", stmt: q.Update(Orders).Set(Orders.Status.To("x")).All().Explain(q.Verbose)},
		{name: "begin serializable read only", stmt: q.Begin(q.TxMode{Isolation: q.Serializable, ReadOnly: true})},
		{name: "savepoint", stmt: q.Savepoint("sp1")},
		{name: "rollback to savepoint", stmt: q.RollbackToSavepoint("sp1")},
		{name: "prepare typed", stmt: q.Prepare("by_id", q.Select(Users.Email).From(Users).Where(Users.ID.EqX(pid)), q.Declare(pid, q.PgInt8))},
		{name: "execute", stmt: q.Execute("by_id", int64(3))},
		{name: "declare cursor", stmt: q.DeclareCursor("c1", sel)},
		{name: "fetch forward", stmt: q.FetchFrom(q.DirForward(10), "c1")},
		{name: "listen", stmt: q.Listen("jobs")},
		{name: "notify", stmt: q.Notify("jobs", "payload with 'quote'")},
		{name: "set config local", stmt: q.SetConfig("statement_timeout", "5s", true)},
		{name: "copy in csv", stmt: q.CopyIn(Users, Users.Email, Users.Balance).With(q.WithFormat(q.CopyCSV), q.WithHeader(1), q.WithNull(""))},
		{name: "copy out query", stmt: q.CopyQueryOut(q.Select(Users.ID).From(Users)).With(q.WithFormat(q.CopyCSV), q.WithDelimiter(q.DelimPipe))},
		{name: "copy out table", stmt: q.CopyTableOut(Users, Users.ID)},
		{name: "vacuum analyze", stmt: q.Vacuum([]q.MaintOption{q.VacAnalyze, q.VacVerbose}, q.Target(Users, Users.ID))},
		{name: "analyze", stmt: q.AnalyzeTables(nil, q.Target(Orders))},
		{name: "truncate restart cascade", stmt: q.Truncate(Orders, Users).RestartIdentity().Cascade()},
		{name: "lock table", stmt: q.LockTable(q.LockShare, true, Users)},
		{name: "grant select", stmt: q.Grant(q.PrivSelect, q.PrivInsert).OnTables(Users).To(q.NewRole("app")).WithGrantOption()},
		{name: "revoke", stmt: q.Revoke(q.PrivAll).OnTables(Users).From(q.NewRole("app")).Cascade()},
	}
}

func renderCommands(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for _, c := range commandCases() {
		sql, args, err := q.SQL(q.Dialect{Version: q.PG19}, c.stmt)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		fmt.Fprintf(&b, "-- %s\n%s\n%#v\n\n", c.name, sql, args)
	}
	return b.String()
}

func TestCommandsGolden(t *testing.T) {
	got := renderCommands(t)
	path := filepath.Join("testdata", "commands.golden")
	if *update {
		os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (create it with -update)", err)
	}
	if string(want) != got {
		t.Errorf("rendered SQL changed:\n%s", got)
	}
}

// Syntax newer than the target server must be refused when rendering, not by
// the server at run time.
func TestVersionGates(t *testing.T) {
	win := q.Window().OrderBy(Orders.ID.Asc())
	cases := []struct {
		name string
		stmt q.Statement
		min  int
	}{
		{"EXPLAIN IO", q.Select(Users.ID).From(Users).Explain(q.Analyze, q.IO), q.PG19},
		{"COPY FORMAT json", q.CopyTableOut(Users).With(q.WithFormat(q.CopyJSON)), q.PG19},
		{"IGNORE NULLS", q.Select(q.Lag(Orders.Total).IgnoreNulls().Over(win)).From(Orders), q.PG19},
		{"casefold", q.Select(Users.Email.Casefold()).From(Users), q.PG18},
		{"RETURNING OLD", q.Update(Accounts).Set(Accounts.Balance.To(1)).All().Returning(Accounts.Balance.Old()), q.PG18},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, _, err := q.SQL(q.Dialect{Version: c.min}, c.stmt); err != nil {
				t.Fatalf("must render on %d: %v", c.min, err)
			}
			_, _, err := q.SQL(q.Dialect{Version: c.min - 1}, c.stmt)
			if !errors.Is(err, q.ErrUnsupportedByServer) {
				t.Fatalf("on %d want ErrUnsupportedByServer, got %v", c.min-1, err)
			}
		})
	}
}

func TestShapeAndScopeErrors(t *testing.T) {
	cases := []struct {
		name string
		stmt q.Statement
		want error
	}{
		{"column of a table that is not in FROM", q.Select(Orders.ID).From(Users), q.ErrScope},
		{"WITH TIES without ORDER BY", q.Select(Users.ID).From(Users).FetchFirst(1).WithTies(), q.ErrShape},
		{"LIMIT and FETCH together", q.Select(Users.ID).From(Users).Limit(1).FetchFirst(1), q.ErrShape},
		{"join without condition", q.Select(Users.ID).From(Users).Join(Orders).On(q.Pred{}), q.ErrShape},
		{"COPY query with a parameter", q.CopyQueryOut(q.Select(Users.ID).From(Users).Where(Users.ID.Eq(1))), q.ErrParams},
		{"insert missing a required column", q.InsertInto(Users).Row(Users.Balance.To(1)), q.ErrShape},
		{"copy out of a table used as COPY FROM", q.CopyTableOut(Users).With(q.WithHeader(1), q.WithFreeze), q.ErrShape},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := q.SQL(q.Dialect{Version: q.PG19}, c.stmt)
			if !errors.Is(err, c.want) {
				t.Fatalf("want %v, got %v", c.want, err)
			}
		})
	}
}

// The server refuses some EXPLAIN options without (or with) ANALYZE; say so
// at render time, naming the option.
func TestExplainOptionDependencies(t *testing.T) {
	sel := q.Select(Users.ID).From(Users)
	d := q.Dialect{Version: q.PG19}
	for name, c := range map[string]q.Command{
		"IO without ANALYZE":             sel.Explain(q.IO),
		"WAL without ANALYZE":            sel.Explain(q.Wal),
		"SERIALIZE without ANALYZE":      sel.Explain(q.SerializeText),
		"GENERIC_PLAN with ANALYZE":      sel.Explain(q.Analyze, q.GenericPlan),
		"GENERIC_PLAN, ANALYZE reversed": sel.Explain(q.GenericPlan, q.Analyze),
	} {
		if _, _, err := q.SQL(d, c); !errors.Is(err, q.ErrShape) {
			t.Errorf("%s: want ErrShape, got %v", name, err)
		}
	}
	for name, c := range map[string]q.Command{
		"ANALYZE, IO":        sel.Explain(q.Analyze, q.IO),
		"IO, ANALYZE":        sel.Explain(q.IO, q.Analyze),
		"ANALYZE, WAL":       sel.Explain(q.Analyze, q.Wal),
		"GENERIC_PLAN":       sel.Explain(q.GenericPlan),
		"BUFFERS alone":      sel.Explain(q.Buffers),
		"TIMING FALSE":       sel.Explain(q.NoTiming),
		"ANALYZE, SERIALIZE": sel.Explain(q.Analyze, q.SerializeText),
	} {
		if _, _, err := q.SQL(d, c); err != nil {
			t.Errorf("%s must render: %v", name, err)
		}
	}
}
