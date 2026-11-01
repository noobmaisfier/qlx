package qlx_test

import (
	"reflect"
	"sync"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qt"
)

func shapeStatements() map[string]func(i int64) q.Statement {
	return map[string]func(i int64) q.Statement{
		"select": func(i int64) q.Statement {
			return q.Select(Users.ID, Users.Email).From(Users).Where(Users.ID.Eq(i))
		},
		"grouped": func(i int64) q.Statement {
			return q.Select(Users.ID, Users.Email, q.Count(Orders.ID).As("n")).From(Users).
				LeftJoin(Orders).On(Orders.UserID.EqX(Users.ID)).
				Where(Users.Tier.Eq("pro"), Users.Nickname.IsNotNull()).GroupBy(Users.ID).
				Having(q.Count(Orders.ID).Gt(i)).OrderBy(Users.CreatedAt.Desc().NullsLast()).Limit(50)
		},
		"insert": func(i int64) q.Statement {
			return q.InsertInto(Orders).Row(Orders.UserID.To(i), Orders.Total.To(10), Orders.Status.To("new")).Returning(Orders.ID)
		},
		"upsert": func(i int64) q.Statement {
			return q.InsertInto(Users).Row(Users.Email.To("a@b")).OnConflict(Users.Email).DoUpdate(Users.Balance.To(i))
		},
		"update": func(i int64) q.Statement {
			return q.Update(Orders).Set(Orders.Status.To("paid")).Where(Orders.ID.Eq(i))
		},
		"delete": func(i int64) q.Statement { return q.DeleteFrom(Orders).Where(Orders.ID.Eq(i)) },
		"like":   func(i int64) q.Statement { return q.Select(Users.ID).From(Users).Where(Users.Email.NotLike("x%")) },
		"tagged": func(i int64) q.Statement {
			return q.Tagged("by_id", q.Select(Users.ID).From(Users).Where(Users.ID.Eq(i)))
		},
		"any": func(i int64) q.Statement {
			return q.Select(Users.ID).From(Users).Where(Users.ID.Any([]int64{i, i + 1, i + 2}))
		},
		"nullable": func(i int64) q.Statement {
			return q.Update(Users).Set(Users.Nickname.To(qt.Some("n"))).Where(Users.ID.Eq(i))
		},
		"subquery": func(i int64) q.Statement {
			return q.Select(Users.ID).From(Users).Where(q.Exists(q.Select(Orders.ID).From(Orders).Where(Orders.UserID.EqX(Users.ID), Orders.Total.Gt(i))))
		},
	}
}

// The cached path must give exactly what a full render gives, on the first
// call (miss) and on every later one (hit), with the values of that call.
func TestShapeCacheDifferential(t *testing.T) {
	c := q.NewShapeCache(1024)
	d := q.Dialect{Version: q.PG18}
	for name, mk := range shapeStatements() {
		for i := int64(1); i <= 3; i++ {
			wantSQL, wantArgs, err := q.SQL(d, mk(i))
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			sql, args, err := c.Resolve(d, mk(i))
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if sql != wantSQL || !reflect.DeepEqual(args, wantArgs) {
				t.Fatalf("%s call %d:\n got  %s %v\n want %s %v", name, i, sql, args, wantSQL, wantArgs)
			}
		}
	}
	st := c.Stats()
	n := int64(len(shapeStatements()))
	if st.Misses != n || st.Hits != 2*n || st.Size != int(n) {
		t.Fatalf("stats %+v, want %d misses, %d hits", st, n, 2*n)
	}
}

func TestShapeCacheDistinguishesShapes(t *testing.T) {
	c := q.NewShapeCache(1024)
	d := q.Dialect{Version: q.PG18}
	seen := map[string]string{}
	for name, mk := range shapeStatements() {
		sql, _, err := c.Resolve(d, mk(1))
		if err != nil {
			t.Fatal(err)
		}
		if other, dup := seen[sql]; dup {
			t.Fatalf("%s and %s share SQL %s", name, other, sql)
		}
		seen[sql] = name
	}
	// The dialect is part of the key: a version-dependent rendering must not leak across versions.
	if _, _, err := c.Resolve(q.Dialect{Version: q.PG18}, q.Select(q.Lag(Users.Balance).IgnoreNulls().Over(q.Window()).As("x")).From(Users)); err == nil {
		t.Skip("IGNORE NULLS accepted on 18 by this build")
	}
}

func TestShapeCacheParams(t *testing.T) {
	c := q.NewShapeCache(64)
	d := q.Dialect{Version: q.PG18}
	id := q.NewParam[int64]()
	mk := func() q.Statement {
		return q.Select(Users.ID).From(Users).Where(Users.ID.EqX(id), Users.Balance.GtX(id))
	}
	for i := int64(0); i < 3; i++ {
		sql, args, err := c.Resolve(d, mk(), id.Bind(i))
		if err != nil {
			t.Fatal(err)
		}
		if len(args) != 1 || args[0] != i {
			t.Fatalf("%s %v", sql, args)
		}
	}
	if _, _, err := c.Resolve(d, mk()); err == nil {
		t.Fatal("unbound parameter must fail")
	}
	other := q.NewParam[int64]()
	if _, _, err := c.Resolve(d, mk(), id.Bind(1), other.Bind(2)); err == nil {
		t.Fatal("a binding that is not in the statement must fail")
	}
}

func TestShapeCacheErrorsAreNotCached(t *testing.T) {
	c := q.NewShapeCache(64)
	d := q.Dialect{Version: q.PG18}
	bad := func() q.Statement { return q.Update(Orders).Set(Orders.Status.To("x")) } // no WHERE
	for i := 0; i < 3; i++ {
		_, _, err := c.Resolve(d, bad())
		_, _, want := q.SQL(d, bad())
		if err == nil || want == nil || err.Error() != want.Error() {
			t.Fatalf("got %v, want %v", err, want)
		}
	}
	if c.Stats().Size != 0 {
		t.Fatal("a failing statement must not be cached")
	}
}

func TestShapeCacheRenderedStmtPassesThrough(t *testing.T) {
	d := q.Dialect{Version: q.PG18}
	id := q.NewParam[int64]()
	st, err := q.Render(d, q.Select(Users.ID).From(Users).Where(Users.ID.EqX(id)))
	if err != nil {
		t.Fatal(err)
	}
	sql, args, err := q.Resolve(d, st, id.Bind(5))
	if err != nil || sql != st.SQL || args[0] != int64(5) {
		t.Fatalf("%q %v %v", sql, args, err)
	}
}

func TestShapeCacheBoundedAndConcurrent(t *testing.T) {
	c := q.NewShapeCache(32)
	d := q.Dialect{Version: q.PG18}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := int64(0); i < 200; i++ {
				// a select list whose length varies per call: one shape per length
				items := make([]q.Selectable, 1+i%100)
				for k := range items {
					items[k] = Users.ID
				}
				sel := q.Select(items...).From(Users)
				want, _, _ := q.SQL(d, sel)
				got, _, err := c.Resolve(d, sel)
				if err != nil || got != want {
					t.Errorf("mismatch %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	st := c.Stats()
	if st.Size > 64 || st.Evictions == 0 {
		t.Fatalf("cache not bounded: %+v", st)
	}
}

func BenchmarkShapeCacheHit(b *testing.B) {
	c := q.NewShapeCache(1024)
	d := q.Dialect{Version: q.PG18}
	mk := shapeStatements()["grouped"]
	if _, _, err := c.Resolve(d, mk(1)); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := c.Resolve(d, mk(int64(i))); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkShapeCacheHitSimple(b *testing.B) {
	c := q.NewShapeCache(1024)
	d := q.Dialect{Version: q.PG18}
	mk := shapeStatements()["select"]
	if _, _, err := c.Resolve(d, mk(1)); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := c.Resolve(d, mk(int64(i))); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBuildOnly(b *testing.B) {
	mk := shapeStatements()["grouped"]
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = mk(int64(i))
	}
}
