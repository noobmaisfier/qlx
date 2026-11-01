package qlx_test

import (
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

func tableFuncCases() []goldenCase {
	x := q.NewField[int64]("x")
	n := q.NewField[int64]("n")
	arr := q.NewParam[[]int64]()
	w := q.NewWindow("w", q.Window().PartitionBy(Orders.UserID).OrderBy(Orders.ID.Asc()))
	series := q.GenerateSeries(q.Val[int64](1), q.Val[int64](5)).As("s", x)
	un := q.Unnest(arr).As("u", x).WithOrdinality(n)
	return []goldenCase{
		{name: "generate_series in FROM", stmt: q.Select(q.RefNum(series, x)).From(series)},
		{name: "unnest with ordinality", stmt: q.Select(q.RefNum(un, x), q.RefNum(un, n)).From(un)},
		{name: "unnest joined laterally to a table", stmt: q.Select(Users.ID, q.RefNum(un, x)).From(Users).CrossJoin(un)},
		{name: "named window reused", stmt: q.Select(
			q.RowNumber().Over(w.Spec()).As("rn"),
			q.SumBig(Orders.Total).Over(w.Extend().Rows(q.UnboundedPreceding, q.CurrentRow)).As("run")).
			From(Orders).Windows(w)},
	}
}

func TestTableFuncGolden(t *testing.T) { checkGolden(t, "tablefunc", tableFuncCases()) }

func TestUndefinedNamedWindowIsRefused(t *testing.T) {
	w := q.NewWindow("w", q.Window().OrderBy(Orders.ID.Asc()))
	_, _, err := q.SQL(q.Dialect{Version: q.PG19}, q.Select(q.RowNumber().Over(w.Spec())).From(Orders))
	if err == nil {
		t.Fatal("window used without being defined must fail")
	}
}
