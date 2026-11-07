package qlx_test

import (
	"errors"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

// FROM items are visible from left to right: a LATERAL subquery and an ON
// clause may refer only to what comes before them. PostgreSQL refuses the
// others ("invalid reference to FROM-clause entry"); qlx says so at render time.
func TestFromItemsSeeOnlyThePrecedingOnes(t *testing.T) {
	d := q.Dialect{Version: q.PG18}
	oid := Orders.ID.As("oid")
	lat := q.Lateral(q.Select(oid).From(Orders).Where(Orders.UserID.EqX(Users.ID)), "l")
	latOnOrders := q.Lateral(q.Select(Users.ID.As("uid")).From(Users).Where(Users.ID.EqX(Orders.UserID)), "m")

	bad := map[string]q.Statement{
		"lateral first, refers to a later join": q.Select(q.Ref(lat, oid)).From(lat).Join(Users).On(Users.ID.Eq(1)),
		"update: lateral sees the target":       q.Update(Users).Set(Users.Balance.To(1)).From(lat).Where(Users.ID.Eq(1)),
		"delete: lateral sees the target":       q.DeleteFrom(Users).Using(lat).Where(Users.ID.Eq(1)),
	}
	for name, st := range bad {
		if _, err := q.Render(d, st); !errors.Is(err, q.ErrScope) {
			t.Errorf("%s: want ErrScope, got %v", name, err)
		}
	}
	// ON naming a table joined later
	later := q.Select(Users.ID).From(Users).Join(Orders).On(Orders.UserID.EqX(Users.ID)).Join(Accounts).On(Accounts.ID.EqX(Orders.ID))
	if _, err := q.Render(d, later); err != nil {
		t.Errorf("ON naming earlier tables must work: %v", err)
	}
	bad2 := q.Select(Users.ID).From(Users).Join(Orders).On(Accounts.ID.EqX(Orders.ID)).Join(Accounts).On(Accounts.ID.EqX(Orders.ID))
	if _, err := q.Render(d, bad2); !errors.Is(err, q.ErrScope) {
		t.Errorf("ON naming a table joined later: want ErrScope, got %v", err)
	}

	good := map[string]q.Statement{
		"lateral after the table":                 q.Select(Users.ID).From(Users).Join(lat).On(Users.ID.Eq(1)),
		"lateral cross join":                      q.Select(Users.ID).From(Users).CrossJoin(lat),
		"lateral after a second join":             q.Select(Users.ID).From(Users).Join(Orders).On(Orders.UserID.EqX(Users.ID)).CrossJoin(latOnOrders),
		"update: lateral on an earlier from item": q.Update(Users).Set(Users.Balance.To(1)).From(Orders, latOnOrders).Where(Users.ID.Eq(1)),
	}
	for name, st := range good {
		if _, err := q.Render(d, st); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
