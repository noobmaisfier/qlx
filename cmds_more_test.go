package qlx_test

import (
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

func TestMoreCommands(t *testing.T) {
	cases := []struct {
		s    q.Statement
		want string
	}{
		{q.Reset("work_mem"), `RESET "work_mem"`},
		{q.Show("search_path"), `SHOW "search_path"`},
		{q.SetRole("app", true), `SET LOCAL ROLE "app"`},
		{q.ResetRole(), `RESET ROLE`},
		{q.DiscardPlans(), `DISCARD PLANS`},
		{q.SetConstraintsAll(true), `SET CONSTRAINTS ALL DEFERRED`},
		{q.SetConstraints(false, "a_fk"), `SET CONSTRAINTS "a_fk" IMMEDIATE`},
		{q.SetTransaction(q.TxMode{Isolation: q.Serializable, ReadOnly: true}), `SET TRANSACTION ISOLATION LEVEL SERIALIZABLE READ ONLY`},
		{q.RefreshMaterializedView("public", "mv").Concurrently(), `REFRESH MATERIALIZED VIEW CONCURRENTLY "public"."mv"`},
		{q.Reindex(q.ReindexTable, "public", "users").Concurrently(), `REINDEX TABLE CONCURRENTLY "public"."users"`},
		{q.Cluster(Users, "users_pkey"), `USING "users_pkey"`},
	}
	for _, c := range cases {
		sql, _, err := q.SQL(q.Dialect{Version: q.PG18}, c.s)
		if err != nil {
			t.Errorf("%s: %v", c.want, err)
			continue
		}
		if !contains(sql, c.want) {
			t.Errorf("got %q, want substring %q", sql, c.want)
		}
	}
	if _, _, err := q.SQL(q.Dialect{Version: q.PG18}, q.RefreshMaterializedView("", "mv").Concurrently().WithNoData()); err == nil {
		t.Error("CONCURRENTLY + WITH NO DATA must fail")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
