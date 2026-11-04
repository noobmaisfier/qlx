package qlx_test

import (
	"sync/atomic"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

func TestShapeCacheOnThrash(t *testing.T) {
	c := q.NewShapeCache(32) // one entry per shard
	var calls atomic.Int32
	var seen q.ShapeStats
	c.OnThrash(func(s q.ShapeStats) { calls.Add(1); seen = s })
	d := q.Dialect{Version: q.PG18}
	for i := 0; i < 400; i++ {
		// a select list that grows with the input: one shape per length
		items := make([]q.Selectable, 0, i+1)
		for j := 0; j <= i%60; j++ {
			items = append(items, Users.ID.As("c"+string(rune('a'+j%26))+string(rune('a'+j/26))))
		}
		if _, _, err := c.Resolve(d, q.Select(items...).From(Users)); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("OnThrash called %d times, want exactly once", calls.Load())
	}
	if seen.Evictions < 1 {
		t.Fatalf("stats at the call: %+v", seen)
	}
	// removing it stops the notifications
	c.OnThrash(nil)
}
