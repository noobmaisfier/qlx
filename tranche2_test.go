package qlx_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qt"
)

func tranche2Cases() []goldenCase {
	n := q.NewField[int64]("n")
	uid := q.NewField[int64]("user_id")
	big := q.NewCTE("big", q.Select(Orders.UserID.As("user_id"), q.Count(Orders.ID).As("n")).
		From(Orders).GroupBy(Orders.UserID))
	tree := q.Recursive("t", q.Select(Accounts.ID.As("id")).From(Accounts), func(self q.CTE) q.Query {
		return q.Select(Accounts.ID.As("id")).From(Accounts)
	})
	_ = tree
	win := q.Window().PartitionBy(Orders.UserID).OrderBy(Orders.ID.Asc())
	return []goldenCase{
		{name: "cte referenced from main query",
			stmt: q.Select(Users.Email, q.RefNum(big, n).As("n")).With(big).From(Users).
				Join(big).On(q.RefNum(big, uid).EqX(Users.ID)).Where(q.RefNum(big, n).Gt(3))},
		{name: "materialized cte", stmt: q.Select(q.RefNum(big, n)).With(big.Materialized()).From(big)},
		{name: "union all with order and limit",
			stmt: q.UnionAll(q.Select(Users.ID).From(Users), q.Select(Orders.ID).From(Orders)).Limit(5)},
		{name: "except", stmt: q.Select(Users.ID).From(Users).Except(q.Select(Orders.UserID).From(Orders))},
		{name: "window functions", stmt: q.Select(
			q.RowNumber().Over(win).As("rn"),
			q.SumBig(Orders.Total).Over(win.Rows(q.Preceding(2), q.Following(0))).As("run"),
			q.Lag(Orders.Total).Over(win).As("prev")).From(Orders)},
		{name: "aggregate filter and distinct", stmt: q.Select(
			q.CountDistinct(Orders.UserID).Filter(Orders.Total.Gt(10)).As("c")).From(Orders)},
		{name: "grouping sets", stmt: q.Select(Orders.UserID, Orders.Status, q.CountAll().As("c")).From(Orders).
			GroupBy(q.Rollup(Orders.UserID, Orders.Status))},
		{name: "fetch first with ties", stmt: q.Select(Orders.ID).From(Orders).
			OrderBy(Orders.Total.Desc()).FetchFirst(3).WithTies()},
		{name: "for update skip locked", stmt: q.Select(Orders.ID).From(Orders).ForUpdate().Of(Orders).SkipLocked()},
		{name: "insert returning with on conflict update",
			stmt: q.InsertInto(Users).Row(Users.Email.To("a@b.c")).
				OnConflict(Users.Email).DoUpdate(Users.Nickname.To(qt.Some("x"))).Returning(Users.ID)},
		{name: "update from", stmt: q.Update(Orders).Set(Orders.Status.To("paid")).
			From(Users).Where(Orders.UserID.EqX(Users.ID), Users.Tier.Eq("pro"))},
		{name: "delete using", stmt: q.DeleteFrom(Orders).Using(Users).Where(Orders.UserID.EqX(Users.ID), Users.Balance.Lt(0))},
		{name: "merge", stmt: q.Merge(Accounts).Using(Users).On(Accounts.ID.EqX(Users.ID)).
			WhenMatched().ThenUpdate(Accounts.Balance.ToX(Users.Balance)).
			WhenNotMatched().ThenInsert(Accounts.ID.ToX(Users.ID), Accounts.Balance.To(0))},
		{name: "case expression", stmt: q.Select(q.Case[string]().When(Users.Balance.Gt(100), "rich").Else("poor").As("k")).From(Users)},
		{name: "cast", stmt: q.Select(q.Cast(Users.ID, q.PgText).As("s")).From(Users)},
		{name: "values table", stmt: q.Select(q.RefNum(q.Values1("v", q.NewField[int64]("x"), 1, 2), q.NewField[int64]("x"))).
			From(q.Values1("v", q.NewField[int64]("x"), 1, 2))},
		{name: "lateral", stmt: q.Select(Users.ID).From(Users).CrossJoin(
			q.Lateral(q.Select(Orders.ID).From(Orders).Where(Orders.UserID.EqX(Users.ID)).Limit(1), "o"))},
		{name: "tablesample", stmt: q.Select(Users.ID).From(Users.TableSample(q.Bernoulli(10)).Repeatable(7))},
	}
}

var update = flag.Bool("update", false, "rewrite testdata/tranche2.golden")

func renderTranche2(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for _, c := range tranche2Cases() {
		sql, args, err := q.SQL(q.Dialect{Version: q.PG19}, c.stmt)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		fmt.Fprintf(&b, "-- %s\n%s\n%#v\n\n", c.name, sql, args)
	}
	return b.String()
}

// TestTranche2Golden compares every case with testdata/tranche2.golden.
// Review a diff by eye, then refresh with: go test -run Tranche2Golden -update
func TestTranche2Golden(t *testing.T) {
	got := renderTranche2(t)
	path := filepath.Join("testdata", "tranche2.golden")
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
		t.Errorf("rendered SQL changed; diff testdata/tranche2.golden against:\n%s", got)
	}
}
