package qlx_test

import (
	"errors"
	"strings"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qt"
)

func TestNegativeLimitOffsetAreRefused(t *testing.T) {
	base := func() q.SelectQuery { return q.Select(Users.ID).From(Users) }
	bad := map[string]q.SelectQuery{
		"Limit":      base().Limit(-1),
		"Offset":     base().Offset(-5),
		"FetchFirst": base().FetchFirst(-2),
	}
	for name, st := range bad {
		if _, err := q.Render(q.Dialect{Version: q.PG18}, st); !errors.Is(err, q.ErrShape) || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: err=%v", name, err)
		}
	}
	for _, st := range []q.SelectQuery{base().Limit(0), base().Limit(10).Offset(0), base().FetchFirst(1)} {
		if _, err := q.Render(q.Dialect{Version: q.PG18}, st); err != nil {
			t.Errorf("good control refused: %v", err)
		}
	}
}

func TestEqNeOnNullValueRenderIsNull(t *testing.T) {
	N := Users.Nickname
	sel := func(p q.Pred) string {
		r, err := q.Render(q.Dialect{Version: q.PG18}, q.Select(Users.ID).From(Users).Where(p))
		if err != nil {
			t.Fatal(err)
		}
		return r.SQL
	}
	cases := []struct{ got, want string }{
		{sel(N.Eq(qt.None[string]())), `WHERE "users"."nickname" IS NULL`},
		{sel(N.Ne(qt.None[string]())), `WHERE "users"."nickname" IS NOT NULL`},
		{sel(N.EqIf(qt.Of(qt.None[string]()))), `WHERE "users"."nickname" IS NULL`},
		{sel(N.Eq(qt.Some("x"))), `WHERE "users"."nickname" = $1`},
		{sel(Users.Tier.Eq("pro")), `WHERE "users"."tier" = $1`},
	}
	for _, c := range cases {
		if !strings.HasSuffix(c.got, c.want) {
			t.Errorf("got %s, want suffix %s", c.got, c.want)
		}
	}
}
