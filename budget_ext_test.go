package qlx_test

import (
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

// The static path (render once, bind per call) allocates only the argument
// slice.
func TestStaticArgsAllocBudget(t *testing.T) {
	id := q.NewParam[int64]()
	st, err := q.Render(q.Dialect{}, q.Select(Users.Email).From(Users).Where(Users.ID.EqX(id)))
	if err != nil {
		t.Fatal(err)
	}
	bd := id.Bind(7)
	allocs := testing.AllocsPerRun(200, func() {
		if _, err := st.Args(bd); err != nil {
			t.Fatal(err)
		}
	})
	if allocs > 1 {
		t.Fatalf("Stmt.Args allocates %.0f times per call, budget 1", allocs)
	}
}

// Building and rendering a typical dynamic query stays within a fixed
// allocation budget; raise it only with a reason in the commit message.
func TestBuildRenderAllocBudget(t *testing.T) {
	allocs := testing.AllocsPerRun(100, func() {
		if _, err := q.Render(q.Dialect{}, groupedQuery()); err != nil {
			t.Fatal(err)
		}
	})
	t.Logf("build+render allocs/op: %.0f", allocs)
	if allocs > 150 {
		t.Fatalf("build+render allocates %.0f times, budget 150 (measured 132 on go1.24)", allocs)
	}
}

// A natural, in-line-value statement through the shape cache: building the
// AST is most of the cost; the lookup itself adds the hash walk and the
// argument slice. Measured 13 allocations including the builder's own.
func TestShapeHitAllocBudget(t *testing.T) {
	c := q.NewShapeCache(64)
	d := q.Dialect{Version: q.PG18}
	run := func(i int64) {
		if _, _, err := c.Resolve(d, q.Select(Users.ID, Users.Email).From(Users).Where(Users.ID.Eq(i))); err != nil {
			t.Fatal(err)
		}
	}
	run(0)
	var i int64
	allocs := testing.AllocsPerRun(200, func() { i++; run(i) })
	if allocs > 18 {
		t.Fatalf("shape-cache hit allocates %.0f times, budget 18", allocs)
	}
}
