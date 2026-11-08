package qlx_test

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qt"
)

func TestCacheRendersEachShapeOnce(t *testing.T) {
	c := q.NewCache(2)
	d := q.Dialect{Version: q.PG19}
	builds := 0
	get := func(shape string) string {
		st, err := c.Render(d, shape, func() q.Statement {
			builds++
			return q.Select(Users.ID).From(Users).Where(Users.Tier.EqIf(qtOpt(shape == "tier")))
		})
		if err != nil {
			t.Fatal(err)
		}
		return st.SQL
	}
	a1, a2 := get("tier"), get("tier")
	if a1 != a2 || builds != 1 {
		t.Fatalf("shape rendered %d times", builds)
	}
	get("plain")
	get("third") // evicts the least recently used ("tier")
	s := c.Stats()
	if s.Size != 2 || s.Evictions != 1 || s.Hits != 1 || s.Misses != 3 {
		t.Fatalf("stats %+v", s)
	}
}

func TestTaggedQueryComment(t *testing.T) {
	sql, _, err := q.SQL(q.Dialect{Version: q.PG19}, q.Tagged("users.by_id", q.Select(Users.ID).From(Users)))
	if err != nil || !strings.HasPrefix(sql, "/* qlx: users.by_id */ SELECT") {
		t.Fatalf("%q %v", sql, err)
	}
	for _, bad := range []string{"", "a*/ DROP", "x y", "--"} {
		_, _, err := q.SQL(q.Dialect{Version: q.PG19}, q.Tagged(bad, q.Select(Users.ID).From(Users)))
		if !errors.Is(err, q.ErrInvalidIdent) {
			t.Errorf("name %q accepted", bad)
		}
	}
}

func TestPrettyOnlyChangesWhitespace(t *testing.T) {
	for _, c := range goldenCases() {
		st, err := q.Render(q.Dialect{Version: q.PG19}, c.stmt)
		if err != nil {
			t.Fatal(err)
		}
		p := st.Pretty()
		if strings.Join(strings.Fields(p), " ") != strings.Join(strings.Fields(st.SQL), " ") {
			t.Errorf("%s: tokens changed\n%s\n%s", c.name, st.SQL, p)
		}
	}
	st, _ := q.Render(q.Dialect{Version: q.PG19}, q.Select(Users.ID).From(Users).Where(Users.ID.Eq(1)).OrderBy(Users.ID.Asc()))
	want := "SELECT \"users\".\"id\"\nFROM \"public\".\"users\"\nWHERE \"users\".\"id\" = $1\nORDER BY \"users\".\"id\""
	if st.Pretty() != want {
		t.Fatalf("got:\n%s", st.Pretty())
	}
}

func qtOpt(set bool) qt.Opt[string] {
	if set {
		return qt.Of("pro")
	}
	return qt.Unset[string]()
}

func TestCacheRejectsNonComparableShape(t *testing.T) {
	c := q.NewCache(4)
	_, err := c.Render(q.Dialect{Version: q.PG19}, []string{"a"}, func() q.Statement { return q.Select(Users.ID).From(Users) })
	if !errors.Is(err, q.ErrShape) {
		t.Fatalf("err = %v, want ErrShape instead of a panic", err)
	}
}

func TestCacheErrorsAreNotCountedAsMisses(t *testing.T) {
	c := q.NewCache(4)
	d := q.Dialect{Version: q.PG19}
	bad := func() q.Statement { return q.Update(Orders).Set(Orders.Status.To("x")) } // no WHERE
	for i := 0; i < 2; i++ {
		if _, err := c.Render(d, "bad", bad); err == nil {
			t.Fatal("expected an error")
		}
	}
	if s := c.Stats(); s.Misses != 0 || s.Size != 0 {
		t.Fatalf("stats %+v", s)
	}
}

func TestCacheSharesConcurrentRenders(t *testing.T) {
	c := q.NewCache(4)
	d := q.Dialect{Version: q.PG19}
	var builds atomic.Int32
	gate := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Render(d, "one", func() q.Statement {
				builds.Add(1)
				<-gate // hold the first render until the others are waiting
				return q.Select(Users.ID).From(Users)
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(gate)
	wg.Wait()
	if n := builds.Load(); n != 1 {
		t.Fatalf("built %d times, want 1", n)
	}
}

func TestCachePanicInBuildReleasesWaiters(t *testing.T) {
	c := q.NewCache(4)
	d := q.Dialect{Version: q.PG19}
	func() {
		defer func() { _ = recover() }()
		_, _ = c.Render(d, "p", func() q.Statement { panic("boom") })
	}()
	// the slot was freed: a later call renders normally
	if _, err := c.Render(d, "p", func() q.Statement { return q.Select(Users.ID).From(Users) }); err != nil {
		t.Fatal(err)
	}
}
