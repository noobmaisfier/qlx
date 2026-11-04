package qlx_test

import (
	"errors"
	"strings"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qt"
)

// What the generator writes for an extension window function (PostGIS
// ST_ClusterDBSCAN): a WinFn that needs Over, in the extension's schema.
func fakeClusterDBSCAN(v q.Operand[int64], eps q.Operand[float64], minPoints q.Operand[int32]) q.WinFn[qt.Null[int32]] {
	return q.WinCall[qt.Null[int32]](0, 0, "public", "st_clusterdbscan", q.ArgT(v, q.PgInt8), q.ArgT(eps, q.PgFloat8), q.ArgT(minPoints, q.PgInt4))
}

func TestWinCallRendersWithOver(t *testing.T) {
	st, err := q.Render(q.Dialect{Version: q.PG18}, q.Select(Users.ID, fakeClusterDBSCAN(Users.Balance, q.NewParam[float64](), q.NewParam[int32]()).Over(q.Window().OrderBy(Users.ID.Asc())).As("cluster")).From(Users))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.SQL, `"public"."st_clusterdbscan"(`) || !strings.Contains(st.SQL, ") OVER (ORDER BY") {
		t.Errorf("SQL: %s", st.SQL)
	}
}

func TestWinCallOutsideWindowContextIsAnError(t *testing.T) {
	over := fakeClusterDBSCAN(Users.Balance, q.NewParam[float64](), q.NewParam[int32]()).Over(q.Window())
	_, err := q.Render(q.Dialect{Version: q.PG18}, q.Select(Users.ID).From(Users).Where(over.IsNotNull()))
	if !errors.Is(err, q.ErrShape) {
		t.Errorf("a window function in WHERE rendered: %v", err)
	}
}
