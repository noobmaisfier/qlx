package qlx_test

// The statements of docs/dtos.md, built the way the guide builds them.

import (
	"strings"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qt"
)

// A create body: nickname is nullable and has no default, so qt.Null is enough.
type dtoCreateUser struct {
	Email string          `json:"email"`
	Nick  qt.Null[string] `json:"nickname"`
	Tier  qt.Opt[string]  `json:"tier"` // column with a DEFAULT: absent must keep it
}

func (r dtoCreateUser) insert() q.InsertQuery {
	return q.InsertInto(Users).Row(
		Users.Email.To(r.Email),
		Users.Nickname.To(r.Nick),
		Users.Tier.ToIf(r.Tier), // absent: the column is left out and takes its DEFAULT
	).Returning(Users.ID, Users.Email, Users.Nickname)
}

type dtoPatchUser struct {
	Email    qt.Opt[string]          `json:"email,omitzero"`
	Nickname qt.Opt[qt.Null[string]] `json:"nickname,omitzero"`
}

func (p dtoPatchUser) assignments() []q.Assignment {
	return []q.Assignment{
		Users.Email.ToIf(p.Email),
		Users.Nickname.ToIf(p.Nickname),
	}
}

type dtoListQuery struct {
	Tier   *string
	Search *string
	Limit  int
	Offset int
}

const dtoMaxLimit = 100

// page applies the defaults and the cap; a client number never reaches Limit as is.
func (f dtoListQuery) page() (limit, offset int64) {
	limit = int64(f.Limit)
	if limit <= 0 {
		limit = 20
	}
	if limit > dtoMaxLimit {
		limit = dtoMaxLimit
	}
	if f.Offset > 0 {
		offset = int64(f.Offset)
	}
	return
}

func (f dtoListQuery) statement() q.SelectQuery {
	var preds []q.Pred
	if f.Tier != nil {
		preds = append(preds, Users.Tier.Eq(*f.Tier))
	}
	if f.Search != nil {
		preds = append(preds, Users.Email.ILike("%"+*f.Search+"%"))
	}
	limit, offset := f.page()
	sel := q.Select(Users.ID, Users.Email, Users.Nickname).From(Users)
	if len(preds) > 0 {
		sel = sel.Where(preds...)
	}
	return sel.OrderBy(Users.CreatedAt.Desc(), Users.ID.Desc()).Limit(limit).Offset(offset)
}

func renderDTO(t *testing.T, s q.Statement) q.Stmt {
	t.Helper()
	st, err := q.Render(q.Dialect{Version: q.PG18}, s)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestDTOCreateStatement(t *testing.T) {
	st := renderDTO(t, dtoCreateUser{Email: "a@b.c"}.insert())
	if strings.Contains(st.SQL, `"tier"`) {
		t.Errorf("an absent tier must keep the DEFAULT: %s", st.SQL)
	}
	if !strings.Contains(st.SQL, `"nickname"`) {
		t.Errorf("nickname is always written (NULL when not sent): %s", st.SQL)
	}
	args, err := st.Args()
	if err != nil || len(args) != 2 || args[1] != nil {
		t.Errorf("args %v %v: an absent nickname is a NULL parameter", args, err)
	}
	st = renderDTO(t, dtoCreateUser{Email: "a@b.c", Nick: qt.Some("ada"), Tier: qt.Of("pro")}.insert())
	if !strings.Contains(st.SQL, `"tier"`) {
		t.Errorf("a sent tier is written: %s", st.SQL)
	}
}

func TestDTOPatchStatement(t *testing.T) {
	// absent: nothing to set, and Render says so instead of running an empty UPDATE
	if _, err := q.Render(q.Dialect{Version: q.PG18}, q.Update(Users).Set(dtoPatchUser{}.assignments()...).Where(Users.ID.Eq(7))); err == nil {
		t.Error("an empty patch rendered an UPDATE")
	}
	// null: the column is set to NULL
	st := renderDTO(t, q.Update(Users).Set(dtoPatchUser{Nickname: qt.Of(qt.None[string]())}.assignments()...).Where(Users.ID.Eq(7)))
	args, _ := st.Args()
	if !strings.Contains(st.SQL, `"nickname" = $1`) || args[0] != nil {
		t.Errorf("null patch: %s %v", st.SQL, args)
	}
	// both
	st = renderDTO(t, q.Update(Users).Set(dtoPatchUser{Email: qt.Of("x@y.z"), Nickname: qt.Of(qt.Some("ada"))}.assignments()...).Where(Users.ID.Eq(7)))
	if !strings.Contains(st.SQL, `"email" = $1`) || !strings.Contains(st.SQL, `"nickname" = $2`) {
		t.Errorf("patch: %s", st.SQL)
	}
}

func TestDTOListStatement(t *testing.T) {
	st := renderDTO(t, dtoListQuery{}.statement())
	if strings.Contains(st.SQL, "WHERE") {
		t.Errorf("no filter, no WHERE: %s", st.SQL)
	}
	args, _ := st.Args()
	if args[0] != int64(20) {
		t.Errorf("default limit: %v", args)
	}
	tier, s := "pro", "ada"
	st = renderDTO(t, dtoListQuery{Tier: &tier, Search: &s, Limit: 1_000_000, Offset: -5}.statement())
	args, _ = st.Args()
	if !strings.Contains(st.SQL, `"users"."tier" = $1`) || !strings.Contains(st.SQL, "ILIKE $2") {
		t.Errorf("filters: %s", st.SQL)
	}
	if args[2] != int64(dtoMaxLimit) || args[3] != int64(0) {
		t.Errorf("a client limit must be capped and a negative offset dropped: %v", args)
	}
	if args[1] != "%ada%" {
		t.Errorf("search is a parameter, never SQL text: %v", args)
	}
}

// A relation: a user with their orders, read with one join and grouped in Go.
func TestDTORelationStatement(t *testing.T) {
	st := renderDTO(t, q.Select(Users.ID, Users.Email, Orders.ID.As("order_id"), Orders.Total).
		From(Users).
		LeftJoin(Orders).On(Orders.UserID.EqX(Users.ID)).
		Where(Users.ID.Eq(7)).
		OrderBy(Orders.ID.Asc()))
	if !strings.Contains(st.SQL, `LEFT JOIN "public"."orders"`) {
		t.Errorf("join: %s", st.SQL)
	}
}
