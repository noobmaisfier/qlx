package qlx

import (
	"strings"
	"testing"
)

// textStmt renders fixed text with one argument; the text is the shape.
type textStmt struct{ sql string }

func (s textStmt) render(b *builder) {
	b.write(s.sql)
	b.write(" ")
	b.arg(1)
}

func lookup(c *ShapeCache, s Statement, d Dialect) (*shapeEntry, shapeKey) {
	hb := &builder{hash: true, version: d.version(), h1: hashSeed1, h2: hashSeed2}
	s.render(hb)
	key := shapeKey{hb.version, hb.h1, hb.h2}
	sh := &c.shards[key.h1%shapeShards]
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	return sh.m[key], key
}

// An entry that does not match what the statement renders (a hash collision,
// or anything else) is replaced by the fresh render rather than served or
// left to force a full render forever.
func TestShapeCacheRepairsMismatchedEntry(t *testing.T) {
	c := NewShapeCache(64)
	d := Dialect{Version: PG18}
	st := textStmt{"SELECT x"}
	if _, _, err := c.Resolve(d, st); err != nil {
		t.Fatal(err)
	}
	e, key := lookup(c, st, d)
	if e == nil {
		t.Fatal("not cached")
	}
	sh := &c.shards[key.h1%shapeShards]
	sh.mu.Lock()
	sh.m[key] = &shapeEntry{sql: "SELECT forged", nslots: 7}
	sh.mu.Unlock()

	for i := 0; i < 2; i++ {
		sql, args, err := c.Resolve(d, st)
		if err != nil || !strings.HasPrefix(sql, "SELECT x") || len(args) != 1 {
			t.Fatalf("pass %d: %q %v %v", i, sql, args, err)
		}
	}
	if e, _ := lookup(c, st, d); e == nil || strings.Contains(e.sql, "forged") {
		t.Fatalf("entry not repaired: %+v", e)
	}
	if h := c.Stats().Hits; h != 1 {
		t.Fatalf("hits = %d: the second pass should hit the repaired entry", h)
	}
}

// Second chance: an entry that is hit survives the pressure that evicts
// entries nobody asks for.
func TestShapeCacheKeepsHotEntries(t *testing.T) {
	c := NewShapeCache(shapeShards * 4) // four entries per shard
	d := Dialect{Version: PG18}
	hot := textStmt{"SELECT hot"}
	_, _, _ = c.Resolve(d, hot)
	for i := 0; i < 2000; i++ {
		_, _, _ = c.Resolve(d, hot) // keeps its bit set
		_, _, _ = c.Resolve(d, textStmt{"SELECT cold" + strings.Repeat("x", i%500) + string(rune('a'+i%26))})
	}
	if c.Stats().Evictions == 0 {
		t.Fatal("expected evictions")
	}
	if e, _ := lookup(c, hot, d); e == nil {
		t.Fatal("the hot entry was evicted")
	}
}

func TestShapeCacheSkipsOversizeStatements(t *testing.T) {
	c := NewShapeCache(64)
	d := Dialect{Version: PG18}
	big := textStmt{"SELECT " + strings.Repeat("a", maxCachedSQL)}
	for i := 0; i < 3; i++ {
		sql, _, err := c.Resolve(d, big)
		if err != nil || len(sql) <= maxCachedSQL {
			t.Fatalf("%d %v", len(sql), err)
		}
	}
	s := c.Stats()
	if s.Size != 0 || s.Oversize != 3 {
		t.Fatalf("stats %+v", s)
	}
}

func TestShapeCacheReset(t *testing.T) {
	c := NewShapeCache(64)
	d := Dialect{Version: PG18}
	for _, s := range []string{"SELECT a", "SELECT b"} {
		_, _, _ = c.Resolve(d, textStmt{s})
		_, _, _ = c.Resolve(d, textStmt{s})
	}
	if s := c.Stats(); s.Size != 2 || s.Hits != 2 {
		t.Fatalf("before: %+v", s)
	}
	c.Reset()
	if s := c.Stats(); s != (ShapeStats{}) {
		t.Fatalf("after Reset: %+v", s)
	}
	if _, _, err := c.Resolve(d, textStmt{"SELECT a"}); err != nil || c.Stats().Misses != 1 {
		t.Fatalf("a reset cache must work: %v %+v", err, c.Stats())
	}
}

// Two processes must not hash alike; within one, the seeds are not the
// published constants.
func TestHashSeedsArePerProcess(t *testing.T) {
	if hashSeed1 == 14695981039346656037 || hashSeed2 == 0x9E3779B97F4A7C15 {
		t.Fatal("seeds were not randomized")
	}
}
