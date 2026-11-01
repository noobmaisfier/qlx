package qlx_test

import (
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

func groupedQuery() q.SelectQuery {
	return q.Select(Users.ID, Users.Email, q.Count(Orders.ID).As("order_count")).
		From(Users).
		LeftJoin(Orders).On(Orders.UserID.EqX(Users.ID)).
		Where(Users.Tier.Eq("pro"), Users.Nickname.IsNotNull()).
		GroupBy(Users.ID).
		Having(q.Count(Orders.ID).Gt(5)).
		OrderBy(Users.CreatedAt.Desc().NullsLast()).
		Limit(50)
}

// Build + render, as paid once per dynamic shape.
func BenchmarkBuildAndRender(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := q.Render(q.Dialect{}, groupedQuery()); err != nil {
			b.Fatal(err)
		}
	}
}

// Hot path of a static query: the statement is rendered once, each call only
// resolves arguments.
func BenchmarkStaticArgs(b *testing.B) {
	id := q.NewParam[int64]()
	st, err := q.Render(q.Dialect{}, q.Select(Users.Email).From(Users).Where(Users.ID.EqX(id)))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := st.Args(id.Bind(int64(i))); err != nil {
			b.Fatal(err)
		}
	}
}
