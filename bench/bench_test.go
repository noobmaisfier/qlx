// Package bench compares QLX with the Go query builders people use today,
// on the same three statements: a filtered join with ordering and a limit, an
// insert, and an update. Everything is measured without a database so the
// numbers are the library's own cost.
//
//	go test -bench . -benchmem -count 10 | benchstat
//
// Budgets (enforced by TestBudgets) are about allocations, not time: the
// static path (render once, bind per call) allocates no more than one more
// than the hand-written SQL string plus argument slice, and the dynamic path
// (build and render each call) allocates at most 1.5x what squirrel does for
// the same statement. In time the static path is about twice the hand-written
// one (it also builds the argument slice from typed bindings); time is not
// budgeted because it is too machine-dependent for CI.
package bench

import (
	"database/sql"
	"testing"

	sq "github.com/Masterminds/squirrel"
	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	jet "github.com/go-jet/jet/v2/postgres"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

const handSelect = `SELECT u.id, u.email, o.total FROM users u JOIN orders o ON o.user_id = u.id WHERE u.tier = $1 AND o.total > $2 ORDER BY o.total DESC LIMIT 50`

var sink any

// ---- SELECT ---------------------------------------------------------------

func BenchmarkSelect_Handwritten(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		args := []any{"pro", int64(i)}
		sink = handSelect
		sink = args
	}
}

func BenchmarkSelect_QLXStatic(b *testing.B) {
	tier, min := q.NewParam[string](), q.NewParam[int64]()
	u, o := Users.As("u"), Orders.As("o")
	st, err := q.Render(q.Dialect{Version: q.PG18}, q.Select(u.ID, u.Email, o.Total).From(u).
		Join(o).On(o.UserID.EqX(u.ID)).Where(u.Tier.EqX(tier), o.Total.GtX(min)).
		OrderBy(o.Total.Desc()).Limit(50))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		args, err := st.Args(tier.Bind("pro"), min.Bind(int64(i)))
		if err != nil {
			b.Fatal(err)
		}
		sink = args
	}
}

func BenchmarkSelect_QLXDynamic(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		u, o := Users.As("u"), Orders.As("o")
		sql, args, err := q.SQL(q.Dialect{Version: q.PG18}, q.Select(u.ID, u.Email, o.Total).From(u).
			Join(o).On(o.UserID.EqX(u.ID)).Where(u.Tier.Eq("pro"), o.Total.Gt(int64(i))).
			OrderBy(o.Total.Desc()).Limit(50))
		if err != nil {
			b.Fatal(err)
		}
		sink, sink = sql, args
	}
}

// The automatic path: the query is written naturally with its values, and the
// shape cache finds the rendered SQL by hashing; nothing is declared up front.
func BenchmarkSelect_QLXAuto(b *testing.B) {
	c := q.NewShapeCache(1024)
	d := q.Dialect{Version: q.PG18}
	u, o := Users.As("u"), Orders.As("o")
	mk := func(i int) q.Statement {
		return q.Select(u.ID, u.Email, o.Total).From(u).
			Join(o).On(o.UserID.EqX(u.ID)).Where(u.Tier.Eq("pro"), o.Total.Gt(int64(i))).
			OrderBy(o.Total.Desc()).Limit(50)
	}
	if _, _, err := c.Resolve(d, mk(0)); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sql, args, err := c.Resolve(d, mk(i))
		if err != nil {
			b.Fatal(err)
		}
		sink, sink = sql, args
	}
}

func BenchmarkSelect_Squirrel(b *testing.B) {
	b.ReportAllocs()
	ps := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	for i := 0; i < b.N; i++ {
		s, args, err := ps.Select("u.id", "u.email", "o.total").From("users u").
			Join("orders o ON o.user_id = u.id").Where(sq.Eq{"u.tier": "pro"}).Where(sq.Gt{"o.total": i}).
			OrderBy("o.total DESC").Limit(50).ToSql()
		if err != nil {
			b.Fatal(err)
		}
		sink, sink = s, args
	}
}

func BenchmarkSelect_Goqu(b *testing.B) {
	b.ReportAllocs()
	d := goqu.Dialect("postgres")
	for i := 0; i < b.N; i++ {
		s, args, err := d.From(goqu.T("users").As("u")).
			Select("u.id", "u.email", "o.total").
			Join(goqu.T("orders").As("o"), goqu.On(goqu.I("o.user_id").Eq(goqu.I("u.id")))).
			Where(goqu.I("u.tier").Eq("pro"), goqu.I("o.total").Gt(i)).
			Order(goqu.I("o.total").Desc()).Limit(50).Prepared(true).ToSQL()
		if err != nil {
			b.Fatal(err)
		}
		sink, sink = s, args
	}
}

func BenchmarkSelect_Bun(b *testing.B) {
	b.ReportAllocs()
	db := bun.NewDB(&sql.DB{}, pgdialect.New())
	for i := 0; i < b.N; i++ {
		s := db.NewSelect().TableExpr("users AS u").ColumnExpr("u.id, u.email, o.total").
			Join("JOIN orders AS o ON o.user_id = u.id").Where("u.tier = ?", "pro").Where("o.total > ?", i).
			OrderExpr("o.total DESC").Limit(50).String()
		sink = s
	}
}

func BenchmarkSelect_Jet(b *testing.B) {
	b.ReportAllocs()
	users := jet.NewTable("public", "users", "u", jet.IntegerColumn("id"), jet.StringColumn("email"), jet.StringColumn("tier"))
	orders := jet.NewTable("public", "orders", "o", jet.IntegerColumn("user_id"), jet.IntegerColumn("total"))
	uid, email, tier := jet.IntegerColumn("id"), jet.StringColumn("email"), jet.StringColumn("tier")
	ouid, total := jet.IntegerColumn("user_id"), jet.IntegerColumn("total")
	for i := 0; i < b.N; i++ {
		stmt := jet.SELECT(uid, email, total).FROM(users.INNER_JOIN(orders, ouid.EQ(uid))).
			WHERE(tier.EQ(jet.String("pro")).AND(total.GT(jet.Int(int64(i))))).ORDER_BY(total.DESC()).LIMIT(50)
		s, args := stmt.Sql()
		sink, sink = s, args
	}
}

// ---- INSERT ---------------------------------------------------------------

func BenchmarkInsert_QLXDynamic(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s, args, err := q.SQL(q.Dialect{Version: q.PG18}, q.InsertInto(Orders).
			Row(Orders.UserID.To(int64(i)), Orders.Total.To(10), Orders.Status.To("new")).
			Returning(Orders.ID))
		if err != nil {
			b.Fatal(err)
		}
		sink, sink = s, args
	}
}

func BenchmarkInsert_Squirrel(b *testing.B) {
	b.ReportAllocs()
	ps := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	for i := 0; i < b.N; i++ {
		s, args, err := ps.Insert("orders").Columns("user_id", "total", "status").Values(i, 10, "new").Suffix("RETURNING id").ToSql()
		if err != nil {
			b.Fatal(err)
		}
		sink, sink = s, args
	}
}

func BenchmarkInsert_Goqu(b *testing.B) {
	b.ReportAllocs()
	d := goqu.Dialect("postgres")
	for i := 0; i < b.N; i++ {
		s, args, err := d.Insert("orders").Rows(goqu.Record{"user_id": i, "total": 10, "status": "new"}).Returning("id").Prepared(true).ToSQL()
		if err != nil {
			b.Fatal(err)
		}
		sink, sink = s, args
	}
}

// ---- UPDATE ---------------------------------------------------------------

func BenchmarkUpdate_QLXDynamic(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s, args, err := q.SQL(q.Dialect{Version: q.PG18}, q.Update(Orders).Set(Orders.Status.To("paid")).Where(Orders.ID.Eq(int64(i))))
		if err != nil {
			b.Fatal(err)
		}
		sink, sink = s, args
	}
}

func BenchmarkUpdate_Squirrel(b *testing.B) {
	b.ReportAllocs()
	ps := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	for i := 0; i < b.N; i++ {
		s, args, err := ps.Update("orders").Set("status", "paid").Where(sq.Eq{"id": i}).ToSql()
		if err != nil {
			b.Fatal(err)
		}
		sink, sink = s, args
	}
}

// ---- budgets --------------------------------------------------------------

func allocs(f func(b *testing.B)) float64 {
	r := testing.Benchmark(f)
	return float64(r.AllocsPerOp())
}

// TestBudgets turns the plan's performance claims into a failing test.
func TestBudgets(t *testing.T) {
	if testing.Short() {
		t.Skip("benchmarks skipped in -short mode")
	}
	static := allocs(BenchmarkSelect_QLXStatic)
	hand := allocs(BenchmarkSelect_Handwritten)
	if static > hand+1 {
		t.Errorf("static path allocates %.0f/op, hand-written %.0f/op (budget: +1)", static, hand)
	}
	dyn := allocs(BenchmarkSelect_QLXDynamic)
	squirrel := allocs(BenchmarkSelect_Squirrel)
	if dyn > squirrel*3/2 {
		t.Errorf("dynamic path allocates %.0f/op, squirrel %.0f/op (budget: at most 1.5x)", dyn, squirrel)
	}
	auto := allocs(BenchmarkSelect_QLXAuto)
	if auto > squirrel {
		t.Errorf("automatic path allocates %.0f/op, squirrel %.0f/op (budget: not more)", auto, squirrel)
	}
	t.Logf("automatic path: %.0f allocs/op", auto)
	t.Logf("allocs/op: static %.0f, handwritten %.0f, dynamic %.0f, squirrel %.0f", static, hand, dyn, squirrel)
}
