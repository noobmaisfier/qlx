package qlx_test

import (
	"errors"
	"strings"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qt"
)

func TestOnFK(t *testing.T) {
	u := Users.As("u")
	o := Orders.As("o")
	st, err := q.Render(q.Dialect{Version: q.PG18}, q.Select(u.Email.As("email"), o.Total.As("total")).From(u).
		Join(o).OnFK(o.FKOrdersUserIDFkey(u)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.SQL, `ON "o"."user_id" = "u"."id"`) {
		t.Fatalf("unexpected join condition:\n%s", st.SQL)
	}
}

func TestOnFKEmptyKeyFails(t *testing.T) {
	_, err := q.Render(q.Dialect{Version: q.PG18}, q.Select(Users.Email.As("email")).From(Users).
		Join(Orders).OnFK(q.NewForeignKey("empty")))
	if !errors.Is(err, q.ErrShape) {
		t.Fatalf("want ErrShape, got %v", err)
	}
}

func TestAnyAndNotAll(t *testing.T) {
	st, err := q.Render(q.Dialect{Version: q.PG18}, q.Select(Users.ID.As("id")).From(Users).
		Where(q.And(Users.ID.Any([]int64{1, 2}), Users.ID.NotAll([]int64{3}))))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.SQL, "= ANY($1)") || !strings.Contains(st.SQL, "<> ALL($2)") {
		t.Fatalf("unexpected SQL:\n%s", st.SQL)
	}
	st, _ = q.Render(q.Dialect{Version: q.PG18}, q.Select(Users.ID.As("id")).From(Users).
		Where(Users.ID.Any(nil)))
	if !strings.Contains(st.SQL, "FALSE") {
		t.Fatalf("empty Any should be FALSE:\n%s", st.SQL)
	}
	st, _ = q.Render(q.Dialect{Version: q.PG18}, q.Select(Users.ID.As("id")).From(Users).
		Where(Users.ID.NotAll(nil)))
	if !strings.Contains(st.SQL, "TRUE") {
		t.Fatalf("empty NotAll should be TRUE:\n%s", st.SQL)
	}
}

func TestFromPtr(t *testing.T) {
	if qt.FromPtr[int](nil).Valid {
		t.Fatal("nil pointer must be NULL")
	}
	v := 7
	if n := qt.FromPtr(&v); !n.Valid || n.V != 7 {
		t.Fatalf("got %+v", n)
	}
}

func TestNotNullRendersByVersion(t *testing.T) {
	sel := q.Select(q.NotNull(Users.Nickname).As("nick")).From(Users)
	st18, err := q.Render(q.Dialect{Version: q.PG18}, sel)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(st18.SQL, "error_on_null") {
		t.Fatalf("PG18 must not use error_on_null:\n%s", st18.SQL)
	}
	st19, err := q.Render(q.Dialect{Version: q.PG19}, sel)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st19.SQL, "error_on_null(") {
		t.Fatalf("PG19 should use error_on_null:\n%s", st19.SQL)
	}
}
