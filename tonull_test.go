package qlx_test

import (
	"strings"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qt"
)

func TestToNullRenders(t *testing.T) {
	cases := []struct {
		name string
		st   q.Statement
		want string
		args int
	}{
		{"update", q.Update(Users).Set(Users.Nickname.ToNull()).Where(Users.ID.Eq(7)),
			`UPDATE "public"."users" SET "nickname" = NULL WHERE "users"."id" = $1`, 1},
		{"update with a value", q.Update(Users).Set(Users.Balance.ToX(Users.Balance.Sub(5)), Users.Nickname.ToNull()).Where(Users.ID.Eq(7)),
			`UPDATE "public"."users" SET "balance" = "users"."balance" - $1, "nickname" = NULL WHERE "users"."id" = $2`, 2},
		{"insert", q.InsertInto(Users).Row(Users.Email.To("a@b.c"), Users.Nickname.ToNull()),
			`INSERT INTO "public"."users" ("email", "nickname") VALUES ($1, NULL)`, 1},
		{"multi-row insert", q.InsertInto(Users).
			Row(Users.Email.To("a@b.c"), Users.Nickname.ToNull()).
			Row(Users.Email.To("d@e.f"), Users.Nickname.To(qt.Some("d"))),
			`INSERT INTO "public"."users" ("email", "nickname") VALUES ($1, NULL), ($2, $3)`, 3},
		{"upsert", q.InsertInto(Users).Row(Users.Email.To("a@b.c")).
			OnConflict(Users.Email).DoUpdate(Users.Nickname.ToNull()),
			`ON CONFLICT ("email") DO UPDATE SET "nickname" = NULL`, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, args := renderIf(t, c.st)
			if !strings.Contains(st.SQL, c.want) {
				t.Fatalf("got  %s\nwant %s", st.SQL, c.want)
			}
			if len(args) != c.args {
				t.Fatalf("args = %d, want %d: %v", len(args), c.args, args)
			}
		})
	}
}

// NULL and a value are different SQL, so they are different shapes; the cache
// must not return one for the other.
func TestToNullIsADistinctShape(t *testing.T) {
	a, _ := renderIf(t, q.Update(Users).Set(Users.Nickname.ToNull()).Where(Users.ID.Eq(1)))
	b, _ := renderIf(t, q.Update(Users).Set(Users.Nickname.To(qt.None[string]())).Where(Users.ID.Eq(1)))
	if a.SQL == b.SQL {
		t.Fatalf("ToNull and To(None) render the same SQL: %s", a.SQL)
	}
	if !strings.Contains(a.SQL, "= NULL") || !strings.Contains(b.SQL, "= $1") {
		t.Fatalf("%s | %s", a.SQL, b.SQL)
	}
	sc := q.NewShapeCache(8)
	d := q.Dialect{Version: q.PG18}
	s1, a1, err := sc.Resolve(d, q.Update(Users).Set(Users.Nickname.ToNull()).Where(Users.ID.Eq(1)))
	if err != nil {
		t.Fatal(err)
	}
	s2, a2, err := sc.Resolve(d, q.Update(Users).Set(Users.Nickname.To(qt.None[string]())).Where(Users.ID.Eq(1)))
	if err != nil {
		t.Fatal(err)
	}
	if s1 == s2 || len(a1) != 1 || len(a2) != 2 {
		t.Fatalf("shape cache confused them: %q %v / %q %v", s1, a1, s2, a2)
	}
}

// Required column: ToNull names the column, so it is not "missing".
func TestToNullCountsAsAssigned(t *testing.T) {
	//qlx:ignore deliberately a NOT NULL column: the renderer cannot know, the server refuses
	if _, err := q.Render(q.Dialect{Version: q.PG18}, q.InsertInto(Users).Row(Users.Email.ToNull())); err != nil {
		t.Fatalf("the renderer does not know the column is NOT NULL, the server does: %v", err)
	}
}
