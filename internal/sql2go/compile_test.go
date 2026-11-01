// The snippets below are what emit_test.go pins sql2go to produce, pasted
// verbatim: compiling and rendering them proves the emitted spellings are
// real API calls with the right types, not just plausible-looking text.
package sql2go_test

import (
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qt"
)

func TestEmittedCodeCompilesAndRenders(t *testing.T) {
	u := Users.As("u")
	o := Orders.As("o")
	stmt := q.Select(u.ID, u.Email, q.Count(o.ID).As("order_count")).
		From(u).
		LeftJoin(o).On(o.UserID.EqX(u.ID)).
		Where(u.Tier.Eq("pro"), u.Nickname.IsNotNull()).
		GroupBy(u.ID).
		Having(q.Count(o.ID).Gt(5)).
		OrderBy(u.CreatedAt.Desc().NullsLast()).
		Limit(50)
	ins := q.InsertInto(Users).
		Row(Users.Email.To("a"), Users.Tier.To("pro")).
		OnConflict(Users.Email).
		DoUpdate(Users.Tier.ToX(Users.Tier.Excluded())).
		Returning(Users.ID)
	upd := q.Update(Users).
		Set(Users.Balance.ToX(Users.Balance.Add(5))).
		Where(Users.ID.Eq(7))
	del := q.DeleteFrom(Orders).All()
	nul := q.Update(Users).Set(Users.Nickname.To(qt.None[string]())).Where(Users.ID.Eq(1))
	nn := q.Select(Users.ID).From(Users).Where(Users.Nickname.Eq(qt.Some[string]("x")), Users.Balance.Add(1).Gt(10), Users.Balance.Between(1, 9), Users.Email.Lower().Eq("a"), q.Not(Users.Email.Like("%x")), Users.ID.In(1, 2, 3), Users.Balance.Gt(-5))
	ex := q.Select(u.ID, u.Email).From(u).Where(q.Exists(q.Select(o.ID).From(o).Where(o.UserID.EqX(u.ID))))
	for _, s := range []q.Statement{stmt, ins, upd, del, nul, nn, ex} {
		if _, err := q.Render(q.Dialect{Version: q.PG17}, s); err != nil {
			t.Error(err)
		}
	}
}
