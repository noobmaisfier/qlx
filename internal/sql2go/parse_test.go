//go:build sql2go

package sql2go

import (
	"strings"
	"testing"
)

// These tests go through the real PostgreSQL parser (libpg_query), so they
// catch every mismatch between the parse-tree shapes the emitter expects and
// the ones the parser produces: the hand-built trees of emit_test.go cannot.
// Run: go test -tags sql2go ./internal/sql2go/

func fromSQL(t *testing.T, sql string) (*Result, error) {
	t.Helper()
	return FromSQL(sql, schema(), Options{})
}

func TestRealParserConversions(t *testing.T) {
	cases := []struct{ name, sql, want string }{
		{"select", `SELECT id FROM users WHERE id = 1`,
			`stmt := q.Select(Users.ID). From(Users). Where(Users.ID.Eq(1))`},
		{"insert returning", `INSERT INTO users (email, tier) VALUES ('a', 'pro') RETURNING id`,
			`stmt := q.InsertInto(Users). Row(Users.Email.To("a"), Users.Tier.To("pro")). Returning(Users.ID)`},
		{"multi-row insert", `INSERT INTO users (email, tier) VALUES ('a', 'x'), ('b', 'y')`,
			`stmt := q.InsertInto(Users). Row(Users.Email.To("a"), Users.Tier.To("x")). Row(Users.Email.To("b"), Users.Tier.To("y"))`},
		{"update", `UPDATE users SET balance = balance + 5 WHERE id = 7`,
			`stmt := q.Update(Users). Set(Users.Balance.ToX(Users.Balance.Add(5))). Where(Users.ID.Eq(7))`},
		{"update to null", `UPDATE users SET nickname = NULL WHERE id = 1`,
			`stmt := q.Update(Users). Set(Users.Nickname.To(qt.None[string]())). Where(Users.ID.Eq(1))`},
		{"delete", `DELETE FROM orders WHERE id = 1`,
			`stmt := q.DeleteFrom(Orders). Where(Orders.ID.Eq(1))`},
		{"delete with alias", `DELETE FROM orders o WHERE o.id = 1`,
			`o := Orders.As("o") stmt := q.DeleteFrom(o). Where(o.ID.Eq(1))`},
		{"upsert", `INSERT INTO users (email, tier) VALUES ('a', 'pro') ON CONFLICT (email) DO UPDATE SET tier = excluded.tier RETURNING id`,
			`stmt := q.InsertInto(Users). Row(Users.Email.To("a"), Users.Tier.To("pro")). OnConflict(Users.Email). DoUpdate(Users.Tier.ToX(Users.Tier.Excluded())). Returning(Users.ID)`},
		{"do nothing", `INSERT INTO users (email) VALUES ('a') ON CONFLICT DO NOTHING`,
			`stmt := q.InsertInto(Users). Row(Users.Email.To("a")). OnConflict(). DoNothing()`},
		{"join group having order limit", `SELECT u.id, u.email, count(o.id) AS order_count
			FROM users u LEFT JOIN orders o ON o.user_id = u.id
			WHERE u.tier = 'pro' AND u.nickname IS NOT NULL
			GROUP BY u.id HAVING count(o.id) > 5
			ORDER BY u.created_at DESC NULLS LAST LIMIT 50`,
			`u := Users.As("u") o := Orders.As("o") stmt := q.Select(u.ID, u.Email, q.Count(o.ID).As("order_count")). From(u). LeftJoin(o).On(o.UserID.EqX(u.ID)). Where(u.Tier.Eq("pro"), u.Nickname.IsNotNull()). GroupBy(u.ID). Having(q.Count(o.ID).Gt(5)). OrderBy(u.CreatedAt.Desc().NullsLast()). Limit(50)`},
		{"predicates", `SELECT id FROM users WHERE id IN (1, 2, 3) AND email NOT LIKE '%@x.io' AND balance BETWEEN 1 AND 9 AND NOT (tier = 'a' OR tier = 'b')`,
			`stmt := q.Select(Users.ID). From(Users). Where(Users.ID.In(1, 2, 3), q.Not(Users.Email.Like("%@x.io")), Users.Balance.Between(1, 9), q.Not(q.Or(Users.Tier.Eq("a"), Users.Tier.Eq("b"))))`},
		{"negative literal and zero", `SELECT id FROM users WHERE balance > -5 AND id <> 0`,
			`stmt := q.Select(Users.ID). From(Users). Where(Users.Balance.Gt(-5), Users.ID.Ne(0))`},
		{"exists", `SELECT u.id FROM users u WHERE EXISTS (SELECT 1 FROM orders o WHERE o.user_id = u.id)`,
			`u := Users.As("u") o := Orders.As("o") stmt := q.Select(u.ID). From(u). Where(q.Exists(q.Select(o.ID). From(o). Where(o.UserID.EqX(u.ID))))`},
		{"star", `SELECT * FROM orders`,
			`stmt := q.Select(Orders.ID, Orders.UserID, Orders.Total, Orders.Status). From(Orders)`},
		{"distinct limit offset", `SELECT DISTINCT tier FROM users ORDER BY tier LIMIT 10 OFFSET 20`,
			`stmt := q.Select(Users.Tier). Distinct(). From(Users). OrderBy(Users.Tier.Asc()). Limit(10). Offset(20)`},
		{"for update skip locked", `SELECT id FROM users WHERE id = 1 FOR UPDATE SKIP LOCKED`,
			`stmt := q.Select(Users.ID). From(Users). Where(Users.ID.Eq(1)). ForUpdate(). SkipLocked()`},
	}
	for _, c := range cases {
		r, err := fromSQL(t, c.sql)
		if err != nil {
			js, _ := ParseToJSON(c.sql)
			t.Errorf("%s: %v\ntree: %s", c.name, err, js)
			continue
		}
		if got := norm(r.Code); got != c.want {
			js, _ := ParseToJSON(c.sql)
			t.Errorf("%s:\n got  %s\n want %s\ntree: %s", c.name, got, c.want, js)
		}
	}
}

func TestRealParserDeleteWithoutWhereWarns(t *testing.T) {
	r, err := fromSQL(t, `DELETE FROM orders`)
	if err != nil || norm(r.Code) != `stmt := q.DeleteFrom(Orders). All()` || len(r.Warnings) != 1 {
		t.Fatalf("%v %v %v", err, r, r.Warnings)
	}
}

func TestRealParserErrorsCarryPositions(t *testing.T) {
	_, err := fromSQL(t, "SELECT id\nFROM users\nWHERE zzz = 1")
	if err == nil || !strings.HasPrefix(err.Error(), "3:7:") {
		t.Fatalf("want a 3:7 position, got %v", err)
	}
	if _, err := fromSQL(t, "SELECT FROM WHERE"); err == nil {
		t.Fatal("a syntax error must be reported by the parser")
	}
}
