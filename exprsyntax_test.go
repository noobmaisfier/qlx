package qlx_test

import (
	"strings"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

func TestExprSyntax(t *testing.T) {
	cases := []struct {
		name string
		sel  q.Statement
		want string
	}{
		{"not like", q.Select(Users.ID).From(Users).Where(Users.Email.NotLike("a%")), `"email" NOT LIKE $1`},
		{"similar", q.Select(Users.ID).From(Users).Where(Users.Email.SimilarTo("(a|b)%")), `"email" SIMILAR TO $1`},
		{"like escape", q.Select(Users.ID).From(Users).Where(Users.Email.LikeEscape(q.PatLike, "a!%", "!")), `LIKE $1 ESCAPE $2`},
		{"not between", q.Select(Users.ID).From(Users).Where(Users.Balance.NotBetween(1, 5)), `NOT BETWEEN $1 AND $2`},
		{"between symmetric", q.Select(Users.ID).From(Users).Where(Users.Balance.BetweenSymmetric(5, 1)), `BETWEEN SYMMETRIC $1 AND $2`},
		{"is not true", q.Select(Users.ID).From(Users).Where(Users.Balance.Eq(1).IsNotTrue()), `IS NOT TRUE`},
		{"current user", q.Select(q.CurrentUser().As("u")).From(Users), `CURRENT_USER`},
		{"current ts prec", q.Select(q.CurrentTimestampP(3).As("n")).From(Users), `CURRENT_TIMESTAMP(3)`},
		{"collate", q.Select(Users.ID).From(Users).OrderBy(Users.Email.Collate("C").Asc()), `COLLATE "C"`},
		{"any select", q.Select(Users.ID).From(Users).Where(q.AnySelect(Users.ID, q.OpEq, q.Select(Orders.UserID).From(Orders))), `= ANY (SELECT`},
	}
	for _, c := range cases {
		sql, _, err := q.SQL(q.Dialect{Version: q.PG18}, c.sel)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !strings.Contains(sql, c.want) {
			t.Errorf("%s: %s does not contain %s", c.name, sql, c.want)
		}
	}
}
