package qlx

import (
	"container/list"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// Cache keeps rendered statements by shape so a hot path renders each shape
// once. The key is chosen by the caller and names the shape: a constant for a
// static query, or the set of switches (which filters are present) for a
// dynamic one. The cache is bounded and counts evictions, so an accidentally
// unbounded key space shows up in Stats instead of in memory use.
type Cache struct {
	mu        sync.Mutex
	max       int
	ll        *list.List
	m         map[cacheKey]*list.Element
	hits      atomic.Int64
	misses    atomic.Int64
	evictions atomic.Int64
}

type cacheKey struct {
	version int
	shape   any
}

type cacheEntry struct {
	key cacheKey
	st  Stmt
}

// CacheStats are the cache counters.
type CacheStats struct {
	Hits, Misses, Evictions int64
	Size                    int
}

// NewCache makes a cache holding at most max shapes (at least 1).
func NewCache(max int) *Cache {
	if max < 1 {
		max = 1
	}
	return &Cache{max: max, ll: list.New(), m: map[cacheKey]*list.Element{}}
}

// Render returns the cached statement for the shape, building and rendering
// it with build on a miss. Errors are not cached. shape must be comparable.
func (c *Cache) Render(d Dialect, shape any, build func() Statement) (Stmt, error) {
	key := cacheKey{d.version(), shape}
	c.mu.Lock()
	if el, ok := c.m[key]; ok {
		c.ll.MoveToFront(el)
		st := el.Value.(*cacheEntry).st
		c.mu.Unlock()
		c.hits.Add(1)
		return st, nil
	}
	c.mu.Unlock()
	c.misses.Add(1)
	st, err := Render(d, build())
	if err != nil {
		return Stmt{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.m[key]; ok { // another goroutine got there first
		return el.Value.(*cacheEntry).st, nil
	}
	c.m[key] = c.ll.PushFront(&cacheEntry{key, st})
	for c.ll.Len() > c.max {
		last := c.ll.Back()
		c.ll.Remove(last)
		delete(c.m, last.Value.(*cacheEntry).key)
		c.evictions.Add(1)
	}
	return st, nil
}

// Stats reports hits, misses, evictions and the current size.
func (c *Cache) Stats() CacheStats {
	c.mu.Lock()
	n := c.ll.Len()
	c.mu.Unlock()
	return CacheStats{c.hits.Load(), c.misses.Load(), c.evictions.Load(), n}
}

// Tagged tags a statement with a leading comment, /* qlx: name */, so a DBA
// can tell which query a row of pg_stat_activity or the server log belongs to.
// The name is restricted to letters, digits and . _ : / - (no `*`), so it
// cannot end the comment early.
func Tagged(name string, s Statement) Statement { return namedStmt{name, s} }

type namedStmt struct {
	name string
	s    Statement
}

func validQueryName(n string) bool {
	if n == "" || len(n) > 64 {
		return false
	}
	for _, r := range n {
		if !(r == '.' || r == '_' || r == ':' || r == '/' || r == '-' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return !strings.Contains(n, "--")
}

func (n namedStmt) render(b *builder) {
	if !validQueryName(n.name) {
		b.fail(fmt.Errorf("%w: query name %q", ErrInvalidIdent, n.name))
		return
	}
	b.write("/* qlx: " + n.name + " */ ")
	n.s.render(b)
}
