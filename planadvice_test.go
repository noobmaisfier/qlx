package qlx_test

import (
	"strings"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

func TestPlanAdvice(t *testing.T) {
	u, o := Users.As("u"), Orders.As("o")
	adv := q.Advice(
		q.JoinOrder(q.Rel(q.On(u)), q.Group(q.Rel(q.On(o)), q.Rel(q.OnAlias("x").Occurrence(2)))),
		q.HashJoin(q.On(o)),
		q.NestedLoopMemoize.Together(q.On(u), q.On(o)),
		q.IndexScan(q.On(o), "public.orders_pkey"),
		q.GatherTogether(q.On(u), q.On(o)),
	)
	s, err := adv.String()
	if err != nil {
		t.Fatal(err)
	}
	want := "JOIN_ORDER(u (o x#2)) HASH_JOIN(o) NESTED_LOOP_MEMOIZE((u o)) INDEX_SCAN(o public.orders_pkey) GATHER((u o))"
	if s != want {
		t.Fatalf("got  %s\nwant %s", s, want)
	}
	sql, args, err := q.SQL(q.Dialect{Version: q.PG19}, adv.Set(true))
	if err != nil || !strings.Contains(sql, "set_config('pg_plan_advice.advice', $1, $2)") || args[0] != want {
		t.Fatalf("%q %v %v", sql, args, err)
	}
	if _, _, err := q.SQL(q.Dialect{Version: q.PG18}, adv.Set(true)); err == nil {
		t.Fatal("PostgreSQL 18 must be refused")
	}
	for _, bad := range []q.AdviceItem{
		q.SeqScan(),
		q.SeqScan(q.OnAlias("a b")),
		q.JoinOrder(q.Rel(q.OnAlias("a"))),
		q.JoinOrder(q.Rel(q.OnAlias("a")), q.Swappable(q.Group(q.Rel(q.OnAlias("b"))))),
		q.IndexScan(q.OnAlias("a"), "bad name"),
	} {
		if _, err := q.Advice(bad).String(); err == nil {
			t.Errorf("invalid advice item accepted")
		}
	}
	if sql, _, err := q.SQL(q.Dialect{Version: q.PG19}, q.Explain(q.Select(Users.ID).From(Users), q.PlanAdvice)); err != nil || !strings.Contains(sql, "PLAN_ADVICE") {
		t.Fatalf("%q %v", sql, err)
	}
}
