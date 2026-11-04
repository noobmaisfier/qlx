package qlx_test

import (
	"errors"
	"strings"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qt"
)

func renderIf(t *testing.T, s q.Statement) (q.Stmt, []any) {
	t.Helper()
	st, err := q.Render(q.Dialect{Version: q.PG18}, s)
	if err != nil {
		t.Fatal(err)
	}
	args, err := st.Args()
	if err != nil {
		t.Fatal(err)
	}
	return st, args
}

func TestToIfInsert(t *testing.T) {
	ins := func(tier qt.Opt[string], nick qt.Opt[qt.Null[string]]) q.InsertQuery {
		return q.InsertInto(Users).Row(Users.Email.To("a@b.c"), Users.Nickname.ToIf(nick), Users.Tier.ToIf(tier))
	}
	st, args := renderIf(t, ins(qt.Unset[string](), qt.Unset[qt.Null[string]]()))
	if strings.Contains(st.SQL, `"tier"`) || strings.Contains(st.SQL, `"nickname"`) || len(args) != 1 {
		t.Errorf("unset options must leave the columns out: %s %v", st.SQL, args)
	}
	st, args = renderIf(t, ins(qt.Of("pro"), qt.Of(qt.None[string]())))
	if !strings.Contains(st.SQL, `"nickname"`) || !strings.Contains(st.SQL, `"tier"`) || len(args) != 3 || args[1] != nil || args[2] != "pro" {
		t.Errorf("set options are written, an explicit NULL is a NULL parameter: %s %v", st.SQL, args)
	}
	// Each combination is its own shape, and the shape cache keeps them apart.
	c := q.NewShapeCache(64)
	d := q.Dialect{Version: q.PG18}
	sqlA, _, _ := c.Resolve(d, ins(qt.Unset[string](), qt.Unset[qt.Null[string]]()))
	sqlB, argsB, _ := c.Resolve(d, ins(qt.Of("pro"), qt.Unset[qt.Null[string]]()))
	sqlA2, argsA2, _ := c.Resolve(d, ins(qt.Unset[string](), qt.Unset[qt.Null[string]]()))
	if sqlA == sqlB || sqlA != sqlA2 || len(argsB) != 2 || len(argsA2) != 1 {
		t.Errorf("shape cache: %q %q %q %v %v", sqlA, sqlB, sqlA2, argsB, argsA2)
	}
}

func TestToIfRequiredColumnLeftOut(t *testing.T) {
	//qlx:ignore deliberately invalid: the renderer must reject it
	_, err := q.Render(q.Dialect{Version: q.PG18}, q.InsertInto(Users).Row(Users.Email.ToIf(qt.Unset[string]()), Users.Tier.To("pro")))
	if !errors.Is(err, q.ErrShape) {
		t.Errorf("a required column left out by ToIf must not render: %v", err)
	}
}

func TestToIfUpdate(t *testing.T) {
	upd := func(email qt.Opt[string], nick qt.Opt[qt.Null[string]]) q.UpdateQuery {
		return q.Update(Users).Set(Users.Email.ToIf(email), Users.Nickname.ToIf(nick)).Where(Users.ID.Eq(7))
	}
	if _, err := q.Render(q.Dialect{Version: q.PG18}, upd(qt.Unset[string](), qt.Unset[qt.Null[string]]())); !errors.Is(err, q.ErrShape) {
		t.Errorf("an UPDATE whose assignments were all left out must not render: %v", err)
	}
	st, args := renderIf(t, upd(qt.Unset[string](), qt.Of(qt.None[string]())))
	if !strings.Contains(st.SQL, `SET "nickname" = $1`) || strings.Contains(st.SQL, `"email"`) || args[0] != nil {
		t.Errorf("null: %s %v", st.SQL, args)
	}
	st, args = renderIf(t, upd(qt.Of("x@y.z"), qt.Of(qt.Some("ada"))))
	if !strings.Contains(st.SQL, `"email" = $1`) || !strings.Contains(st.SQL, `"nickname" = $2`) || args[1] != "ada" {
		t.Errorf("both: %s %v", st.SQL, args)
	}
}

func TestToOrDefault(t *testing.T) {
	ins := func(tier qt.Opt[string]) q.InsertQuery {
		return q.InsertInto(Users).Row(Users.Email.To("a@b.c"), Users.Tier.ToOrDefault(tier))
	}
	st, args := renderIf(t, ins(qt.Unset[string]()))
	if !strings.Contains(st.SQL, `("email", "tier") VALUES ($1, DEFAULT)`) || len(args) != 1 {
		t.Errorf("unset becomes DEFAULT: %s %v", st.SQL, args)
	}
	st, args = renderIf(t, ins(qt.Of("pro")))
	if !strings.Contains(st.SQL, `VALUES ($1, $2)`) || args[1] != "pro" {
		t.Errorf("set is a parameter: %s %v", st.SQL, args)
	}
	// rows that differ in what is set keep the same columns
	multi := q.InsertInto(Users).
		Row(Users.Email.To("a@b.c"), Users.Tier.ToOrDefault(qt.Of("pro"))).
		Row(Users.Email.To("d@e.f"), Users.Tier.ToOrDefault(qt.Unset[string]()))
	st, _ = renderIf(t, multi)
	if !strings.Contains(st.SQL, `($1, $2), ($3, DEFAULT)`) {
		t.Errorf("multi-row: %s", st.SQL)
	}
	// ToIf per row would not: the columns differ
	//qlx:ignore deliberately invalid: the renderer must reject it
	bad := q.InsertInto(Users).
		Row(Users.Email.To("a@b.c"), Users.Tier.ToIf(qt.Of("pro"))).
		Row(Users.Email.To("d@e.f"), Users.Tier.ToIf(qt.Unset[string]()))
	if _, err := q.Render(q.Dialect{Version: q.PG18}, bad); !errors.Is(err, q.ErrShape) {
		t.Errorf("rows with different columns rendered: %v", err)
	}
}

func TestToIfOnConflictAndMerge(t *testing.T) {
	st, args := renderIf(t, q.InsertInto(Users).Row(Users.Email.To("a@b.c")).
		OnConflict(Users.Email).DoUpdate(Users.Tier.ToIf(qt.Unset[string]()), Users.Nickname.ToIf(qt.Of(qt.Some("n")))))
	if !strings.Contains(st.SQL, `DO UPDATE SET "nickname" = $2`) || strings.Contains(st.SQL, `"tier" =`) || args[1] != "n" {
		t.Errorf("DoUpdate: %s %v", st.SQL, args)
	}
}
