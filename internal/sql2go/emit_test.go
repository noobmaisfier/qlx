package sql2go

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/noobmaisfier/qlx/internal/codegen"
)

// The trees below follow libpg_query's JSON output (PostgreSQL 16/17 shape).
// They are built with small helpers so the tests read like the SQL they stand
// for; the real-parser round trip lives in parse_test.go (build tag sql2go).

type m = map[string]any

func s(v string) m { return m{"String": m{"sval": v}} }
func col(p ...string) m {
	var fs []any
	for _, x := range p {
		if x == "*" {
			fs = append(fs, m{"A_Star": m{}})
		} else {
			fs = append(fs, s(x))
		}
	}
	return m{"ColumnRef": m{"fields": fs, "location": 7}}
}
func ival(n int) m    { return m{"A_Const": m{"ival": m{"ival": n}}} }
func sval(v string) m { return m{"A_Const": m{"sval": m{"sval": v}}} }
func op(o string, l, r any) m {
	return m{"A_Expr": m{"kind": "AEXPR_OP", "name": []any{s(o)}, "lexpr": l, "rexpr": r}}
}
func and(a ...any) m { return m{"BoolExpr": m{"boolop": "AND_EXPR", "args": a}} }
func or(a ...any) m  { return m{"BoolExpr": m{"boolop": "OR_EXPR", "args": a}} }
func target(v any, alias string) m {
	t := m{"val": v}
	if alias != "" {
		t["name"] = alias
	}
	return m{"ResTarget": t}
}
func rv(name, alias string) m {
	r := m{"schemaname": "public", "relname": name, "inh": true}
	if alias != "" {
		r["alias"] = m{"aliasname": alias}
	}
	return m{"RangeVar": r}
}

// flat is how libpg_query writes the RangeVar of INSERT/UPDATE/DELETE: the
// fields directly, without the {"RangeVar": ...} wrapper a FROM item has.
func flat(n m) m { return n["RangeVar"].(m) }

func fn(name string, args ...any) m {
	return m{"FuncCall": m{"funcname": []any{s(name)}, "args": args}}
}
func tree(stmt m) string {
	b, _ := json.Marshal(m{"stmts": []any{m{"stmt": stmt}}})
	return string(b)
}

func schema() []codegen.TableName {
	c := func(sql, field, fam, goT string, null bool) codegen.ColumnName {
		return codegen.ColumnName{SQL: sql, Field: field, Family: fam, GoType: goT, Nullable: null}
	}
	return []codegen.TableName{
		{Schema: "public", SQL: "users", Symbol: "Users", Columns: []codegen.ColumnName{
			c("id", "ID", "num", "int64", false), c("email", "Email", "text", "string", false),
			c("nickname", "Nickname", "text", "qt.Null[string]", true), c("tier", "Tier", "text", "string", false),
			c("balance", "Balance", "num", "int64", false), c("created_at", "CreatedAt", "time", "time.Time", false)}},
		{Schema: "public", SQL: "orders", Symbol: "Orders", Columns: []codegen.ColumnName{
			c("id", "ID", "num", "int64", false), c("user_id", "UserID", "num", "int64", false),
			c("total", "Total", "num", "int64", false), c("status", "Status", "text", "string", false)}},
	}
}

func conv1(t *testing.T, stmt m) (*Result, error) {
	t.Helper()
	return Convert(tree(stmt), schema(), Options{})
}

func must(t *testing.T, stmt m) *Result {
	t.Helper()
	r, err := conv1(t, stmt)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func norm(s string) string { return strings.Join(strings.Fields(s), " ") }

func TestGroupedSelect(t *testing.T) {
	// SELECT u.id, u.email, count(o.id) AS order_count
	// FROM users u LEFT JOIN orders o ON o.user_id = u.id
	// WHERE u.tier = 'pro' AND u.nickname IS NOT NULL
	// GROUP BY u.id HAVING count(o.id) > 5 ORDER BY u.created_at DESC NULLS LAST LIMIT 50
	r := must(t, m{"SelectStmt": m{
		"targetList": []any{target(col("u", "id"), ""), target(col("u", "email"), ""),
			target(fn("count", col("o", "id")), "order_count")},
		"fromClause": []any{m{"JoinExpr": m{"jointype": "JOIN_LEFT", "larg": rv("users", "u"), "rarg": rv("orders", "o"),
			"quals": op("=", col("o", "user_id"), col("u", "id"))}}},
		"whereClause": and(op("=", col("u", "tier"), sval("pro")),
			m{"NullTest": m{"arg": col("u", "nickname"), "nulltesttype": "IS_NOT_NULL"}}),
		"groupClause":  []any{col("u", "id")},
		"havingClause": op(">", fn("count", col("o", "id")), ival(5)),
		"sortClause": []any{m{"SortBy": m{"node": col("u", "created_at"), "sortby_dir": "SORTBY_DESC",
			"sortby_nulls": "SORTBY_NULLS_LAST"}}},
		"limitCount": ival(50),
	}})
	want := `u := Users.As("u") o := Orders.As("o") stmt := q.Select(u.ID, u.Email, q.Count(o.ID).As("order_count")). From(u). LeftJoin(o).On(o.UserID.EqX(u.ID)). Where(u.Tier.Eq("pro"), u.Nickname.IsNotNull()). GroupBy(u.ID). Having(q.Count(o.ID).Gt(5)). OrderBy(u.CreatedAt.Desc().NullsLast()). Limit(50)`
	if norm(r.Code) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", r.Code, want)
	}
}

func TestPredicates(t *testing.T) {
	cases := []struct {
		name string
		pred m
		want string
	}{
		{"flipped literal", op("<", ival(100), col("balance")), `Where(Users.Balance.Gt(100))`},
		{"or inside and", and(or(op("=", col("tier"), sval("a")), op("=", col("tier"), sval("b"))), op(">", col("id"), ival(0))),
			`Where(q.Or(Users.Tier.Eq("a"), Users.Tier.Eq("b")), Users.ID.Gt(0))`},
		{"nullable literal", op("=", col("nickname"), sval("x")), `Where(Users.Nickname.Eq(qt.Some[string]("x")))`},
		{"negative literal", op(">", col("balance"), op("-", nil, ival(5))), `Where(Users.Balance.Gt(-5))`},
		{"arith", op(">", op("+", col("balance"), ival(1)), ival(10)), `Where(Users.Balance.Add(1).Gt(10))`},
		{"in", m{"A_Expr": m{"kind": "AEXPR_IN", "name": []any{s("=")}, "lexpr": col("id"),
			"rexpr": m{"List": m{"items": []any{ival(1), ival(2), ival(3)}}}}}, `Where(Users.ID.In(1, 2, 3))`},
		{"not like", m{"A_Expr": m{"kind": "AEXPR_LIKE", "name": []any{s("!~~")}, "lexpr": col("email"), "rexpr": sval("%@x.io")}},
			`Where(q.Not(Users.Email.Like("%@x.io")))`},
		{"between", m{"A_Expr": m{"kind": "AEXPR_BETWEEN", "name": []any{s("BETWEEN")}, "lexpr": col("balance"),
			"rexpr": m{"List": m{"items": []any{ival(1), ival(9)}}}}}, `Where(Users.Balance.Between(1, 9))`},
		{"lower", op("=", fn("lower", col("email")), sval("a@x.io")), `Where(Users.Email.Lower().Eq("a@x.io"))`},
	}
	for _, c := range cases {
		r := must(t, m{"SelectStmt": m{"targetList": []any{target(col("id"), "")},
			"fromClause": []any{rv("users", "")}, "whereClause": c.pred}})
		if !strings.Contains(norm(r.Code), c.want) {
			t.Errorf("%s:\n%s\nlacks %s", c.name, r.Code, c.want)
		}
	}
}

func TestInsertUpdateDelete(t *testing.T) {
	// INSERT INTO users (email, tier) VALUES ('a', 'pro')
	// ON CONFLICT (email) DO UPDATE SET tier = excluded.tier RETURNING id
	ins := must(t, m{"InsertStmt": m{
		"relation":   flat(rv("users", "")),
		"cols":       []any{m{"ResTarget": m{"name": "email"}}, m{"ResTarget": m{"name": "tier"}}},
		"selectStmt": m{"SelectStmt": m{"valuesLists": []any{m{"List": m{"items": []any{sval("a"), sval("pro")}}}}}},
		"onConflictClause": m{"action": "ONCONFLICT_UPDATE",
			"infer":      m{"indexElems": []any{m{"IndexElem": m{"name": "email"}}}},
			"targetList": []any{m{"ResTarget": m{"name": "tier", "val": col("excluded", "tier")}}}},
		"returningList": []any{target(col("id"), "")},
	}})
	want := `stmt := q.InsertInto(Users). Row(Users.Email.To("a"), Users.Tier.To("pro")). OnConflict(Users.Email). DoUpdate(Users.Tier.ToX(Users.Tier.Excluded())). Returning(Users.ID)`
	if norm(ins.Code) != want {
		t.Fatalf("insert:\n%s\nwant\n%s", ins.Code, want)
	}

	upd := must(t, m{"UpdateStmt": m{"relation": flat(rv("users", "")),
		"targetList":  []any{m{"ResTarget": m{"name": "balance", "val": op("+", col("balance"), ival(5))}}},
		"whereClause": op("=", col("id"), ival(7))}})
	if w := `stmt := q.Update(Users). Set(Users.Balance.ToX(Users.Balance.Add(5))). Where(Users.ID.Eq(7))`; norm(upd.Code) != w {
		t.Fatalf("update:\n%s", upd.Code)
	}

	del := must(t, m{"DeleteStmt": m{"relation": flat(rv("orders", ""))}})
	if norm(del.Code) != `stmt := q.DeleteFrom(Orders). All()` || len(del.Warnings) != 1 {
		t.Fatalf("delete:\n%s\n%v", del.Code, del.Warnings)
	}

	nul := must(t, m{"UpdateStmt": m{"relation": flat(rv("users", "")),
		"targetList":  []any{m{"ResTarget": m{"name": "nickname", "val": m{"A_Const": m{"isnull": true}}}}},
		"whereClause": op("=", col("id"), ival(1))}})
	if !strings.Contains(nul.Code, "Users.Nickname.To(qt.None[string]())") {
		t.Fatalf("null assign:\n%s", nul.Code)
	}
}

func TestExistsAndStar(t *testing.T) {
	// SELECT * FROM users u WHERE EXISTS (SELECT o.id FROM orders o WHERE o.user_id = u.id)
	sub := m{"SelectStmt": m{"targetList": []any{target(col("o", "id"), "")},
		"fromClause": []any{rv("orders", "o")}, "whereClause": op("=", col("o", "user_id"), col("u", "id"))}}
	r := must(t, m{"SelectStmt": m{"targetList": []any{target(col("*"), "")}, "fromClause": []any{rv("users", "u")},
		"whereClause": m{"SubLink": m{"subLinkType": "EXISTS_SUBLINK", "subselect": sub}}}})
	n := norm(r.Code)
	for _, w := range []string{"q.Select(u.ID, u.Email, u.Nickname, u.Tier, u.Balance, u.CreatedAt)", "Where(q.Exists(q.Select(o.ID). From(o). Where(o.UserID.EqX(u.ID))))"} {
		if !strings.Contains(n, w) {
			t.Errorf("lacks %q in:\n%s", w, r.Code)
		}
	}
}

func TestUnsupportedIsReported(t *testing.T) {
	cases := []struct {
		name string
		stmt m
		sub  string
	}{
		{"unknown table", m{"SelectStmt": m{"targetList": []any{target(col("id"), "")}, "fromClause": []any{rv("nope", "")}}}, "relation public.nope"},
		{"unknown column", m{"SelectStmt": m{"targetList": []any{target(col("zzz"), "")}, "fromClause": []any{rv("users", "")}}}, "unknown column zzz"},
		{"ambiguous", m{"SelectStmt": m{"targetList": []any{target(col("id"), "")},
			"fromClause": []any{m{"JoinExpr": m{"jointype": "JOIN_INNER", "larg": rv("users", ""), "rarg": rv("orders", ""),
				"quals": op("=", col("users", "id"), col("orders", "user_id"))}}}}}, "ambiguous column id"},
		{"null compare", m{"SelectStmt": m{"targetList": []any{target(col("id"), "")}, "fromClause": []any{rv("users", "")},
			"whereClause": op("=", col("nickname"), m{"A_Const": m{"isnull": true}})}}, "comparison with NULL"},
		{"time literal", m{"SelectStmt": m{"targetList": []any{target(col("id"), "")}, "fromClause": []any{rv("users", "")},
			"whereClause": op(">", col("created_at"), sval("2024-01-01"))}}, "string literal for a time.Time"},
		{"union", m{"SelectStmt": m{"op": "SETOP_UNION"}}, "set operation"},
		{"unknown func", m{"SelectStmt": m{"targetList": []any{target(fn("md5", col("email")), "")},
			"fromClause": []any{rv("users", "")}}}, "pgcat"},
		{"param", m{"SelectStmt": m{"targetList": []any{target(col("id"), "")}, "fromClause": []any{rv("users", "")},
			"whereClause": op("=", col("id"), m{"ParamRef": m{"number": 1}})}}, "NewParam"},
	}
	for _, c := range cases {
		_, err := conv1(t, c.stmt)
		var u *Unsupported
		if !errors.As(err, &u) || !strings.Contains(err.Error(), c.sub) {
			t.Errorf("%s: got %v, want an Unsupported containing %q", c.name, err, c.sub)
		}
	}
}

func TestPositionInError(t *testing.T) {
	sql := "SELECT id\nFROM users\nWHERE zzz = 1"
	stmt := m{"SelectStmt": m{"targetList": []any{target(col("id"), "")}, "fromClause": []any{rv("users", "")},
		"whereClause": op("=", m{"ColumnRef": m{"fields": []any{s("zzz")}, "location": 27}}, ival(1))}}
	_, err := Convert(tree(stmt), schema(), Options{SQL: sql})
	if err == nil || !strings.HasPrefix(err.Error(), "3:7:") {
		t.Fatalf("want a 3:7 position, got %v", err)
	}
}

func TestPkgQualifier(t *testing.T) {
	r, err := Convert(tree(m{"DeleteStmt": m{"relation": flat(rv("orders", "o")), "whereClause": op("=", col("o", "id"), ival(1))}}),
		schema(), Options{Pkg: "db", Q: "qlx"})
	if err != nil {
		t.Fatal(err)
	}
	if norm(r.Code) != `o := db.Orders.As("o") stmt := qlx.DeleteFrom(o). Where(o.ID.Eq(1))` {
		t.Fatal(r.Code)
	}
}

// A wrapped relation is still accepted (older libpg_query shapes, hand-built
// trees), and a flat struct is never mistaken for a node wrapper.
func TestRelationShapes(t *testing.T) {
	for name, rel := range map[string]any{"flat": flat(rv("orders", "")), "wrapped": rv("orders", "")} {
		r, err := conv1(t, m{"DeleteStmt": m{"relation": rel, "whereClause": op("=", col("id"), ival(1))}})
		if err != nil || norm(r.Code) != `stmt := q.DeleteFrom(Orders). Where(Orders.ID.Eq(1))` {
			t.Errorf("%s: %v\n%v", name, err, r)
		}
	}
	for _, v := range []any{
		m{"relname": "x"},                  // flat fields
		m{"aliasname": "u"},                // flat, single lower-case key
		m{"RangeVar": m{}, "relname": "x"}, // two keys: not a wrapper
		nil,
	} {
		if k, f := kindOf(v); k != "" || f != nil {
			t.Errorf("kindOf(%v) = %q, want not a wrapper", v, k)
		}
	}
	if k, _ := kindOf(m{"A_Star": m{}}); k != "A_Star" {
		t.Error("an empty node body is still a wrapper")
	}
}

// EXISTS (SELECT 1 ...) is the usual spelling: the literal select item is
// replaced by a column, since the list does not affect the result.
func TestExistsWithLiteralSelectList(t *testing.T) {
	sub := m{"SelectStmt": m{"targetList": []any{target(ival(1), "")},
		"fromClause": []any{rv("orders", "o")}, "whereClause": op("=", col("o", "user_id"), col("u", "id"))}}
	r := must(t, m{"SelectStmt": m{"targetList": []any{target(col("u", "id"), "")}, "fromClause": []any{rv("users", "u")},
		"whereClause": m{"SubLink": m{"subLinkType": "EXISTS_SUBLINK", "subselect": sub}}}})
	want := `u := Users.As("u") o := Orders.As("o") stmt := q.Select(u.ID). From(u). Where(q.Exists(q.Select(o.ID). From(o). Where(o.UserID.EqX(u.ID))))`
	if norm(r.Code) != want {
		t.Fatalf("got\n%s", r.Code)
	}
	// Outside EXISTS a literal select item is still refused.
	_, err := conv1(t, m{"SelectStmt": m{"targetList": []any{target(ival(1), "")}, "fromClause": []any{rv("users", "")}}})
	if err == nil {
		t.Fatal("literal select item outside EXISTS must be refused")
	}
}

// pg_query gives -1 as the location of nodes it invents, and offsets past the
// end for nodes it rewrites. Neither may panic: the regression-suite ledger
// hit this on real input.
func TestLineColClampsOutOfRangeOffsets(t *testing.T) {
	cases := []struct {
		off       int
		line, col int
	}{
		{-1, 1, 1},
		{-1000, 1, 1},
		{0, 1, 1},
		{3, 1, 4},
		{1000, 1, 5},
	}
	for _, c := range cases {
		line, col := lineCol("abcd", c.off)
		if line != c.line || col != c.col {
			t.Errorf("lineCol(%d) = (%d,%d), want (%d,%d)", c.off, line, col, c.line, c.col)
		}
	}
	if line, col := lineCol("a\nbc", 3); line != 2 || col != 2 {
		t.Errorf("lineCol across a newline = (%d,%d), want (2,2)", line, col)
	}
}
