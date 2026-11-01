package qlx_test

import (
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qt"
)

// Nullable only retypes: the SQL is the column itself.
func TestNullableKeepsSQL(t *testing.T) {
	var _ q.Expr[qt.Null[int64]] = q.Nullable(Orders.Total)
	sql, _, err := q.SQL(q.Dialect{Version: q.PG18}, q.Select(q.Nullable(Orders.Total).As("t")).From(Orders))
	if err != nil {
		t.Fatal(err)
	}
	if want := `SELECT "orders"."total" AS "t" FROM "public"."orders"`; sql != want {
		t.Fatalf("got %s", sql)
	}
}
