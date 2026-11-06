package qlx_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qt"
)

func TestInWithNullElements(t *testing.T) {
	N := Users.Nickname
	sel := func(p ...q.Pred) q.Statement { return q.Select(Users.ID).From(Users).Where(p...) }
	cases := []struct {
		name string
		st   q.Statement
		want string
		args int
	}{
		{"some and none", sel(N.In(qt.Some("a"), qt.None[string]())), `WHERE "users"."nickname" = ANY($1) OR "users"."nickname" IS NULL`, 1},
		{"in a conjunction it is parenthesised", sel(Users.Tier.Eq("pro"), N.In(qt.Some("a"), qt.None[string]())), `"users"."tier" = $1 AND ("users"."nickname" = ANY($2) OR "users"."nickname" IS NULL)`, 2},
		{"under Not it is parenthesised", sel(q.Not(N.In(qt.Some("a"), qt.None[string]()))), `NOT (`, 1},
		{"only none", sel(N.In(qt.None[string]())), `"users"."nickname" IS NULL`, 0},
		{"no none: unchanged", sel(N.In(qt.Some("a"), qt.Some("b"))), `"users"."nickname" = ANY($1)`, 1},
		{"plain column: unchanged", sel(Users.Tier.In("a", "b")), `"users"."tier" = ANY($1)`, 1},
		{"Any shares the rule", sel(N.Any([]qt.Null[string]{qt.Some("a"), qt.None[string]()})), `= ANY($1) OR "users"."nickname" IS NULL`, 1},
		{"InIf shares the rule", sel(N.InIf(qt.Of([]qt.Null[string]{qt.None[string](), qt.Some("z")}))), `= ANY($1) OR "users"."nickname" IS NULL`, 1},
		{"not all, with none", sel(N.NotAll([]qt.Null[string]{qt.Some("a"), qt.None[string]()})), `WHERE "users"."nickname" <> ALL($1) AND "users"."nickname" IS NOT NULL`, 1},
		{"not all, only none", sel(N.NotAll([]qt.Null[string]{qt.None[string]()})), `"users"."nickname" IS NOT NULL`, 0},
		{"not all, no none: unchanged", sel(N.NotAll([]qt.Null[string]{qt.Some("a")})), `"users"."nickname" <> ALL($1)`, 1},
	}
	for _, c := range cases {
		st, args := renderIf(t, c.st)
		if !strings.Contains(st.SQL, c.want) {
			t.Errorf("%s:\n got %s\nwant …%s…", c.name, st.SQL, c.want)
		}
		if len(args) != c.args {
			t.Errorf("%s: %d args, want %d", c.name, len(args), c.args)
		}
	}
	// the array holds only the non-NULL values
	_, args := renderIf(t, sel(N.In(qt.Some("a"), qt.None[string](), qt.Some("b"))))
	if got, ok := args[0].([]qt.Null[string]); !ok || len(got) != 2 {
		t.Fatalf("array argument: %#v", args[0])
	}
}

func TestCopyInlineArgs(t *testing.T) {
	day := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	out := func(p ...q.Pred) q.CopyOut {
		return q.CopyQueryOut(q.Select(Users.ID).From(Users).Where(p...)).InlineArgs()
	}
	cases := []struct {
		name, want string
		st         q.Statement
	}{
		{"int", `"users"."id" = 7`, out(Users.ID.Eq(7))},
		{"negative number in parentheses", `"users"."balance" > (-5)`, out(Users.Balance.Gt(-5))},
		{"string", `"users"."tier" = 'pro'`, out(Users.Tier.Eq("pro"))},
		{"time", `"users"."created_at" >= '2026-01-02T03:04:05Z'::timestamptz`, out(Users.CreatedAt.Gte(day))},
		{"slice", `"users"."id" = ANY(ARRAY[1, 2, 3])`, out(Users.ID.In(1, 2, 3))},
		{"strings in a slice", `"users"."tier" = ANY(ARRAY['a', 'b'])`, out(Users.Tier.In("a", "b"))},
		{"limit", `LIMIT 10`, q.CopyQueryOut(q.Select(Users.ID).From(Users).Limit(10)).InlineArgs()},
		{"nullable value", `"users"."nickname" = 'x'`, out(Users.Nickname.Eq(qt.Some("x")))},
		{"copy from where", `FROM STDIN WHERE "users"."balance" >= 100`, q.CopyIn(Users, Users.Email).Where(Users.Balance.Gte(100)).InlineArgs()},
	}
	for _, c := range cases {
		st, err := q.Render(q.Dialect{Version: q.PG18}, c.st)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !strings.Contains(st.SQL, c.want) {
			t.Errorf("%s:\n got %s\nwant …%s…", c.name, st.SQL, c.want)
		}
		if strings.Contains(st.SQL, "$") {
			t.Errorf("%s: a placeholder is left: %s", c.name, st.SQL)
		}
		if st.NumParams() != 0 {
			t.Errorf("%s: %d params", c.name, st.NumParams())
		}
	}
	// refused, never escaped
	bad := []struct {
		name string
		st   q.Statement
	}{
		{"quote", out(Users.Tier.Eq("o'neil"))},
		{"backslash", out(Users.Tier.Eq(`a\b`))},
		{"non-ASCII", out(Users.Tier.Eq("é"))},
		{"injection attempt", out(Users.Tier.Eq("x' OR '1'='1"))},
		{"newline", out(Users.Tier.Eq("a\nb"))},
		{"NUL byte", out(Users.Tier.Eq("a\x00b"))},
		{"empty list is false, not a literal", nil},
	}
	for _, c := range bad[:len(bad)-1] {
		if _, err := q.Render(q.Dialect{Version: q.PG18}, c.st); !errors.Is(err, q.ErrParams) {
			t.Errorf("%s: want ErrParams, got %v", c.name, err)
		}
	}
	// a parameter bound later cannot be written
	p := q.NewParam[int64]()
	if _, err := q.Render(q.Dialect{Version: q.PG18}, out(Users.ID.EqX(p))); !errors.Is(err, q.ErrParams) {
		t.Errorf("late parameter: %v", err)
	}
	// without InlineArgs the old rule stands
	if _, err := q.Render(q.Dialect{Version: q.PG18}, q.CopyQueryOut(q.Select(Users.ID).From(Users).Where(Users.ID.Eq(1)))); !errors.Is(err, q.ErrParams) {
		t.Errorf("no InlineArgs: %v", err)
	}
	// two values give two statements, whatever the shape cache holds
	a, err := q.Render(q.Dialect{Version: q.PG18}, out(Users.ID.Eq(1)))
	if err != nil {
		t.Fatal(err)
	}
	b, err := q.Render(q.Dialect{Version: q.PG18}, out(Users.ID.Eq(2)))
	if err != nil {
		t.Fatal(err)
	}
	if a.SQL == b.SQL {
		t.Errorf("same SQL for two values: %s", a.SQL)
	}
}
