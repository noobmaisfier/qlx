package qlx_test

import (
	"errors"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

func TestGroupingRule(t *testing.T) {
	d := q.Dialect{Version: q.PG19}
	bad := map[string]q.Statement{
		"ungrouped column next to an aggregate": q.Select(Users.Email, q.CountAll()).From(Users),
		"column not in GROUP BY":                q.Select(Orders.Status, q.CountAll()).From(Orders).GroupBy(Orders.UserID),
		"ungrouped column in ORDER BY":          q.Select(Orders.UserID, q.CountAll()).From(Orders).GroupBy(Orders.UserID).OrderBy(Orders.Total.Asc()),
		"ungrouped column in a window argument": q.Select(q.CountAll(), q.SumBig(Orders.Total).Over(q.Window())).From(Orders),
	}
	for name, s := range bad {
		if _, _, err := q.SQL(d, s); !errors.Is(err, q.ErrShape) {
			t.Errorf("%s: want ErrShape, got %v", name, err)
		}
	}
	good := map[string]q.Statement{
		"plain":                      q.Select(Users.ID, Users.Email).From(Users),
		"grouped column":             q.Select(Orders.UserID, q.CountAll()).From(Orders).GroupBy(Orders.UserID),
		"functional dependency":      q.Select(Users.ID, Users.Email, q.Count(Orders.ID)).From(Users).LeftJoin(Orders).On(Orders.UserID.EqX(Users.ID)).GroupBy(Users.ID),
		"column inside aggregate":    q.Select(q.SumBig(Orders.Total)).From(Orders),
		"window beside plain column": q.Select(Orders.ID, q.RowNumber().Over(q.Window().OrderBy(Orders.ID.Asc()))).From(Orders),
		"aggregate in a subquery does not group the outer query": q.Select(Users.Email,
			q.Scalar(q.Select(q.Count(Orders.ID).As("n")).From(Orders).Where(Orders.UserID.EqX(Users.ID)), q.NewField[int64]("n")).As("c")).From(Users),
		"grouping sets are not judged": q.Select(Orders.UserID, Orders.Status, q.CountAll()).From(Orders).GroupBy(q.Rollup(Orders.UserID, Orders.Status)),
	}
	for name, s := range good {
		if _, _, err := q.SQL(d, s); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
