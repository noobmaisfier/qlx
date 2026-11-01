package qlx_test

import (
	"errors"
	"reflect"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qt"
)

type goldenCase struct {
	name      string
	stmt      q.Statement
	sql       string
	args      []any
	serverMin int // lowest PostgreSQL major that accepts it (for PREPARE checks)
}

func goldenCases() []goldenCase {
	u := Users.As("u")
	byID := q.NewParam[int64]()
	_ = byID
	return []goldenCase{
		{
			name: "grouped select with join, having, order, limit",
			stmt: q.Select(Users.ID, Users.Email, q.Count(Orders.ID).As("order_count")).
				From(Users).
				LeftJoin(Orders).On(Orders.UserID.EqX(Users.ID)).
				Where(Users.Tier.Eq("pro"), Users.Nickname.IsNotNull()).
				GroupBy(Users.ID).
				Having(q.Count(Orders.ID).Gt(5)).
				OrderBy(Users.CreatedAt.Desc().NullsLast()).
				Limit(50),
			sql:  `SELECT "users"."id", "users"."email", count("orders"."id") AS "order_count" FROM "public"."users" LEFT JOIN "public"."orders" ON "orders"."user_id" = "users"."id" WHERE "users"."tier" = $1 AND "users"."nickname" IS NOT NULL GROUP BY "users"."id" HAVING count("orders"."id") > $2 ORDER BY "users"."created_at" DESC NULLS LAST LIMIT $3`,
			args: []any{"pro", int64(5), int64(50)},
		},
		{
			name: "dynamic filters drop absent predicates",
			stmt: q.Select(Users.ID).From(Users).Where(
				Users.Tier.EqIf(qt.Unset[string]()),
				q.If(false, Users.Email.ContainsFold("x")),
				Users.Balance.Gte(100),
			),
			sql:  `SELECT "users"."id" FROM "public"."users" WHERE "users"."balance" >= $1`,
			args: []any{int64(100)},
		},
		{
			name: "dynamic filters keep present predicates",
			stmt: q.Select(Users.ID).From(Users).Where(
				Users.Tier.EqIf(qt.Of("pro")),
				q.If(true, Users.Email.ContainsFold("50%_off")),
			),
			sql:  `SELECT "users"."id" FROM "public"."users" WHERE "users"."tier" = $1 AND "users"."email" ILIKE $2`,
			args: []any{"pro", `%50\%\_off%`},
		},
		{
			name: "alias rebinds columns",
			stmt: q.Select(u.ID, u.Email).From(u).Where(u.ID.Eq(7)),
			sql:  `SELECT "u"."id", "u"."email" FROM "public"."users" AS "u" WHERE "u"."id" = $1`,
			args: []any{int64(7)},
		},
		{
			name: "IN renders one array parameter",
			stmt: q.Select(Users.ID).From(Users).Where(Users.ID.In(1, 2, 3)),
			sql:  `SELECT "users"."id" FROM "public"."users" WHERE "users"."id" = ANY($1)`,
			args: []any{[]int64{1, 2, 3}},
		},
		{
			name: "empty IN is FALSE",
			stmt: q.Select(Users.ID).From(Users).Where(Users.ID.In()),
			sql:  `SELECT "users"."id" FROM "public"."users" WHERE FALSE`,
			args: []any{},
		},
		{
			name: "AND inside OR is parenthesized",
			stmt: q.Select(Users.ID).From(Users).Where(
				q.Or(q.And(Users.Tier.Eq("a"), Users.Balance.Gt(1)), Users.Tier.Eq("b")),
				Users.ID.Gt(0),
			),
			sql:  `SELECT "users"."id" FROM "public"."users" WHERE (("users"."tier" = $1 AND "users"."balance" > $2) OR "users"."tier" = $3) AND "users"."id" > $4`,
			args: []any{"a", int64(1), "b", int64(0)},
		},
		{
			name: "arithmetic in comparison is parenthesized",
			stmt: q.Select(Users.ID).From(Users).Where(Users.Balance.Add(5).Gt(10)),
			sql:  `SELECT "users"."id" FROM "public"."users" WHERE ("users"."balance" + $1) > $2`,
			args: []any{int64(5), int64(10)},
		},
		{
			name: "coalesce of nullable gives non-null expression",
			stmt: q.Select(q.Coalesce(Users.Nickname, "anon").As("nick")).From(Users),
			sql:  `SELECT coalesce("users"."nickname", $1) AS "nick" FROM "public"."users"`,
			args: []any{"anon"},
		},
		{
			name: "sum and filter",
			stmt: q.Select(
				q.SumBig(Orders.Total).As("revenue"),
				q.CountAll().Filter(Orders.Status.Eq("paid")).As("paid"),
			).From(Orders),
			sql:  `SELECT sum("orders"."total") AS "revenue", count(*) FILTER (WHERE "orders"."status" = $1) AS "paid" FROM "public"."orders"`,
			args: []any{"paid"},
		},
		{
			name: "distinct on",
			stmt: q.Select(Users.ID, Users.Tier).From(Users).DistinctOn(Users.Tier).
				OrderBy(Users.Tier.Asc(), Users.CreatedAt.Desc()),
			sql: `SELECT DISTINCT ON ("users"."tier") "users"."id", "users"."tier" FROM "public"."users" ORDER BY "users"."tier", "users"."created_at" DESC`,
		},
		{
			name: "row locking",
			stmt: q.Select(Users.ID).From(Users).Where(Users.Tier.Eq("pro")).
				ForUpdate().Of(Users).SkipLocked(),
			sql:  `SELECT "users"."id" FROM "public"."users" WHERE "users"."tier" = $1 FOR UPDATE OF "users" SKIP LOCKED`,
			args: []any{"pro"},
		},
		{
			name: "correlated exists",
			stmt: q.Select(Users.ID).From(Users).Where(
				q.Exists(q.Select(Orders.ID).From(Orders).Where(Orders.UserID.EqX(Users.ID))),
			),
			sql: `SELECT "users"."id" FROM "public"."users" WHERE EXISTS (SELECT "orders"."id" FROM "public"."orders" WHERE "orders"."user_id" = "users"."id")`,
		},
		{
			name: "in subquery",
			stmt: q.Select(Users.ID).From(Users).Where(
				q.InSelect(Users.ID.Expr, q.Select(Orders.UserID).From(Orders).Where(Orders.Total.Gt(100))),
			),
			sql:  `SELECT "users"."id" FROM "public"."users" WHERE "users"."id" IN (SELECT "orders"."user_id" FROM "public"."orders" WHERE "orders"."total" > $1)`,
			args: []any{int64(100)},
		},
		{
			name: "limit and offset are parameters",
			stmt: q.Select(Users.ID).From(Users).OrderBy(Users.ID.Asc()).Limit(10).Offset(20),
			sql:  `SELECT "users"."id" FROM "public"."users" ORDER BY "users"."id" LIMIT $1 OFFSET $2`,
			args: []any{int64(10), int64(20)},
		},
		{
			name: "insert single row",
			stmt: q.InsertInto(Users).Row(Users.Email.To("a@x.io"), Users.Tier.To("pro")).Returning(Users.ID),
			sql:  `INSERT INTO "public"."users" ("email", "tier") VALUES ($1, $2) RETURNING "users"."id"`,
			args: []any{"a@x.io", "pro"},
		},
		{
			name: "insert multiple rows, columns reordered to the first row",
			stmt: q.InsertInto(Users).
				Row(Users.Email.To("a"), Users.Tier.To("t1")).
				Row(Users.Tier.To("t2"), Users.Email.To("b")),
			sql:  `INSERT INTO "public"."users" ("email", "tier") VALUES ($1, $2), ($3, $4)`,
			args: []any{"a", "t1", "b", "t2"},
		},
		{
			name: "insert nullable binds NULL",
			stmt: q.InsertInto(Users).Row(Users.Email.To("a"), Users.Nickname.To(qt.None[string]())),
			sql:  `INSERT INTO "public"."users" ("email", "nickname") VALUES ($1, $2)`,
			args: []any{"a", nil},
		},
		{
			name: "upsert with excluded",
			stmt: q.InsertInto(Users).Row(Users.Email.To("a"), Users.Tier.To("pro")).
				OnConflict(Users.Email).DoUpdate(Users.Tier.ToX(Users.Tier.Excluded())).
				Returning(Users.ID),
			sql:  `INSERT INTO "public"."users" ("email", "tier") VALUES ($1, $2) ON CONFLICT ("email") DO UPDATE SET "tier" = EXCLUDED."tier" RETURNING "users"."id"`,
			args: []any{"a", "pro"},
		},
		{
			name: "on conflict do nothing without target",
			stmt: q.InsertInto(Users).Row(Users.Email.To("a")).OnConflict().DoNothing(),
			sql:  `INSERT INTO "public"."users" ("email") VALUES ($1) ON CONFLICT DO NOTHING`,
			args: []any{"a"},
		},
		{
			name: "update with arithmetic on the current value",
			stmt: q.Update(Accounts).
				Set(Accounts.Balance.ToX(Accounts.Balance.Add(10))).
				Where(Accounts.ID.Eq(1)).
				Returning(Accounts.Balance),
			sql:  `UPDATE "public"."accounts" SET "balance" = "accounts"."balance" + $1 WHERE "accounts"."id" = $2 RETURNING "accounts"."balance"`,
			args: []any{int64(10), int64(1)},
		},
		{
			name: "update all is explicit",
			stmt: q.Update(Users).Set(Users.Tier.To("free")).All(),
			sql:  `UPDATE "public"."users" SET "tier" = $1`,
			args: []any{"free"},
		},
		{
			name: "delete with returning",
			stmt: q.DeleteFrom(Orders).Where(Orders.Status.Eq("cancelled")).Returning(Orders.ID),
			sql:  `DELETE FROM "public"."orders" WHERE "orders"."status" = $1 RETURNING "orders"."id"`,
			args: []any{"cancelled"},
		},
		{
			name: "returning old and new (PostgreSQL 18)",
			stmt: q.Update(Accounts).
				Set(Accounts.Balance.ToX(Accounts.Balance.Add(10))).
				Where(Accounts.ID.Eq(1)).
				Returning(Accounts.Balance.Old().As("before"), Accounts.Balance.New().As("after")),
			sql:       `UPDATE "public"."accounts" SET "balance" = "accounts"."balance" + $1 WHERE "accounts"."id" = $2 RETURNING old."balance" AS "before", new."balance" AS "after"`,
			args:      []any{int64(10), int64(1)},
			serverMin: 18,
		},
		{
			name: "on conflict do select (PostgreSQL 19)",
			stmt: q.InsertInto(Users).Row(Users.Email.To("a")).
				OnConflict(Users.Email).DoSelect().ForUpdate().
				Returning(Users.ID),
			sql:       `INSERT INTO "public"."users" ("email") VALUES ($1) ON CONFLICT ("email") DO SELECT FOR UPDATE RETURNING "users"."id"`,
			args:      []any{"a"},
			serverMin: 19,
		},
	}
}

func TestGolden(t *testing.T) {
	for _, c := range goldenCases() {
		t.Run(c.name, func(t *testing.T) {
			sql, args, err := q.SQL(q.Dialect{Version: q.PG19}, c.stmt)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if sql != c.sql {
				t.Errorf("sql mismatch\n got: %s\nwant: %s", sql, c.sql)
			}
			want := c.args
			if want == nil {
				want = []any{}
			}
			if !reflect.DeepEqual(args, want) {
				t.Errorf("args mismatch\n got: %#v\nwant: %#v", args, want)
			}
		})
	}
}

func TestStaticParams(t *testing.T) {
	id := q.NewParam[int64]()
	st, err := q.Render(q.Dialect{}, q.Select(Users.Email).From(Users).Where(Users.ID.EqX(id), Users.Balance.GtX(id)))
	if err != nil {
		t.Fatal(err)
	}
	if want := `SELECT "users"."email" FROM "public"."users" WHERE "users"."id" = $1 AND "users"."balance" > $1`; st.SQL != want {
		t.Fatalf("got %s", st.SQL)
	}
	args, err := st.Args(id.Bind(42))
	if err != nil || !reflect.DeepEqual(args, []any{int64(42)}) {
		t.Fatalf("args %v err %v", args, err)
	}
	if _, err := st.Args(); !errors.Is(err, q.ErrParams) {
		t.Fatalf("missing binding: got %v", err)
	}
	other := q.NewParam[int64]()
	if _, err := st.Args(id.Bind(1), other.Bind(2)); !errors.Is(err, q.ErrParams) {
		t.Fatalf("extra binding: got %v", err)
	}
	// Rendering twice must give identical SQL: statements are values.
	st2, _ := q.Render(q.Dialect{}, q.Select(Users.Email).From(Users).Where(Users.ID.EqX(id), Users.Balance.GtX(id)))
	if st2.SQL != st.SQL {
		t.Fatal("render is not deterministic")
	}
}

func TestQueriesAreImmutable(t *testing.T) {
	base := q.Select(Users.ID).From(Users)
	a := base.Where(Users.Tier.Eq("a"))
	b := base.Where(Users.Tier.Eq("b"))
	_, aa, _ := q.SQL(q.Dialect{}, a)
	_, ab, _ := q.SQL(q.Dialect{}, b)
	sbase, _, _ := q.SQL(q.Dialect{}, base)
	if !reflect.DeepEqual(aa, []any{"a"}) || !reflect.DeepEqual(ab, []any{"b"}) || sbase != `SELECT "users"."id" FROM "public"."users"` {
		t.Fatalf("builder steps leaked state: %v %v %q", aa, ab, sbase)
	}
}

func TestErrors(t *testing.T) {
	bad := Users.As("bad\x00alias")
	cases := []struct {
		name string
		d    q.Dialect
		stmt q.Statement
		want error
	}{
		{"column of a table that is not in FROM", q.Dialect{}, q.Select(Orders.ID).From(Users), q.ErrScope},
		{"column with no FROM at all", q.Dialect{}, q.Select(Users.ID), q.ErrScope},
		{"join without condition", q.Dialect{}, q.Select(Users.ID).From(Users).LeftJoin(Orders).On(q.Pred{}), q.ErrShape},
		{"insert without rows", q.Dialect{}, q.InsertInto(Users), q.ErrShape},
		{"insert missing a required column", q.Dialect{}, q.InsertInto(Orders).Row(Orders.Total.To(1)), q.ErrShape},
		{"insert rows with different columns", q.Dialect{}, q.InsertInto(Users).Row(Users.Email.To("a")).Row(Users.Tier.To("b")), q.ErrShape},
		{"insert assigns a column of another table", q.Dialect{}, q.InsertInto(Users).Row(Users.Email.To("a"), Orders.Total.To(1)), q.ErrScope},
		{"update without where", q.Dialect{}, q.Update(Users).Set(Users.Tier.To("x")), q.ErrShape},
		{"update without set", q.Dialect{}, q.Update(Users).Where(Users.ID.Eq(1)), q.ErrShape},
		{"delete without where", q.Dialect{}, q.DeleteFrom(Users), q.ErrShape},
		{"do select without returning", q.Dialect{}, q.InsertInto(Users).Row(Users.Email.To("a")).OnConflict(Users.Email).DoSelect(), q.ErrShape},
		{"do update without target", q.Dialect{}, q.InsertInto(Users).Row(Users.Email.To("a")).OnConflict().DoUpdate(Users.Tier.To("x")), q.ErrShape},
		{"do select on PostgreSQL 18", q.Dialect{Version: 18}, q.InsertInto(Users).Row(Users.Email.To("a")).OnConflict(Users.Email).DoSelect().Returning(Users.ID), q.ErrUnsupportedByServer},
		{"returning old on PostgreSQL 17", q.Dialect{Version: 17}, q.Update(Accounts).Set(Accounts.Balance.To(1)).All().Returning(Accounts.Balance.Old()), q.ErrUnsupportedByServer},
		{"invalid alias is rejected, not quoted around", q.Dialect{}, q.Select(bad.ID).From(bad), q.ErrInvalidIdent},
		{"invalid result alias", q.Dialect{}, q.Select(Users.ID.As("a\x00b")).From(Users), q.ErrInvalidIdent},
		{"select without items", q.Dialect{}, q.Select(), q.ErrShape},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := q.Render(c.d, c.stmt)
			if !errors.Is(err, c.want) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
		})
	}
}

func TestQuoteIdent(t *testing.T) {
	ok := map[string]string{
		"users":      `"users"`,
		`we"ird`:     `"we""ird"`,
		"select":     `"select"`,
		"ünïcode":    `"ünïcode"`,
		"with space": `"with space"`,
		`'; DROP --`: `"'; DROP --"`,
	}
	for in, want := range ok {
		got, err := q.QuoteIdent(in)
		if err != nil || got != want {
			t.Errorf("QuoteIdent(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	long := make([]byte, 64)
	for i := range long {
		long[i] = 'a'
	}
	for _, in := range []string{"", "a\x00b", string(long), "\xff\xfe"} {
		if _, err := q.QuoteIdent(in); !errors.Is(err, q.ErrInvalidIdent) {
			t.Errorf("QuoteIdent(%q) should fail, got %v", in, err)
		}
	}
}

// FuzzQuoteIdent checks the safety property: any accepted name renders as
// exactly one quoted identifier, and unquoting it returns the original.
func FuzzQuoteIdent(f *testing.F) {
	for _, s := range []string{"a", `"`, `""`, "a\"b", "'", ";--", "ü", "\n", "\\"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		out, err := q.QuoteIdent(s)
		if err != nil {
			return
		}
		if len(out) < 2 || out[0] != '"' || out[len(out)-1] != '"' {
			t.Fatalf("not quoted: %q", out)
		}
		inner := out[1 : len(out)-1]
		// Every quote inside must be doubled; walk and unescape.
		var un []byte
		for i := 0; i < len(inner); i++ {
			if inner[i] == '"' {
				if i+1 >= len(inner) || inner[i+1] != '"' {
					t.Fatalf("unescaped quote in %q", out)
				}
				i++
			}
			un = append(un, inner[i])
		}
		if string(un) != s {
			t.Fatalf("roundtrip: %q -> %q -> %q", s, out, un)
		}
	})
}
