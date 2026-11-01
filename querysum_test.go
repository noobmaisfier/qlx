package qlx_test

import (
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

func TestDescribeSQL(t *testing.T) {
	cases := []struct {
		sql, name, op, label string
	}{
		{`SELECT 1`, "", "SELECT", "SELECT"},
		{"  \n insert into t values (1)", "", "INSERT", "INSERT"},
		{`/* qlx: users.by_id */ SELECT "id" FROM "users"`, "users.by_id", "SELECT", "users.by_id"},
		{`/* qlx: x */   UPDATE "t" SET "a" = $1`, "x", "UPDATE", "x"},
		{`WITH a AS (SELECT 1), b AS (DELETE FROM t RETURNING id) SELECT * FROM a`, "", "SELECT", "SELECT"},
		{`WITH RECURSIVE r(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM r) DELETE FROM t USING r`, "", "DELETE", "DELETE"},
		{`WITH "select" AS (SELECT ')' AS "x(") UPDATE t SET a = 1`, "", "UPDATE", "UPDATE"},
		{`WITH moved AS (DELETE FROM a RETURNING *) INSERT INTO b SELECT * FROM moved`, "", "INSERT", "INSERT"},
		{`COPY t FROM STDIN`, "", "COPY", "COPY"},
		{``, "", "", ""},
	}
	for _, c := range cases {
		got := q.DescribeSQL(c.sql)
		if got.Name != c.name || got.Operation != c.op || got.Label() != c.label {
			t.Errorf("%q: got %+v label %q, want name %q op %q label %q", c.sql, got, got.Label(), c.name, c.op, c.label)
		}
	}
}

func TestDescribeSQLOfRenderedTaggedStatement(t *testing.T) {
	sql, _, err := q.SQL(q.Dialect{}, q.Tagged("orders.list", q.Select(Orders.ID).From(Orders)))
	if err != nil {
		t.Fatal(err)
	}
	if got := q.DescribeSQL(sql); got.Name != "orders.list" || got.Operation != "SELECT" {
		t.Fatalf("%q -> %+v", sql, got)
	}
}
