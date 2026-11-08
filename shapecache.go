package qlx

import (
	"encoding/binary"
	"fmt"
	"hash/maphash"
	"sync"
	"sync/atomic"
)

// The automatic static path.
//
// A statement's *shape* is the SQL text it renders to, with every value
// replaced by its placeholder. Writing a query naturally, with values in
// line,
//
//	q.Select(Users.Email).From(Users).Where(Users.ID.Eq(id))
//
// builds a new AST on every call, but the shape is the same every time. The
// shape cache therefore does not render: it *hashes* what rendering would have
// written (builder hash mode: same traversal and same checks of what is
// written, no string building) while collecting the argument values. The hash
// finds the SQL rendered earlier for that shape. Rendering, with all its
// validation, happens once per shape per process.
//
// Because the hash is a function of the exact SQL text, two statements share
// an entry only when they would render identical SQL; literals that the
// renderer writes inline are part of the text, hence of the hash.

// There is nothing to invalidate. The rendered SQL is a function of the
// statement and of Dialect.Version alone, and both are part of the key; the
// database schema never enters it. A migration therefore cannot make an entry
// stale. Entries leave only by eviction or Reset.
//
// The hash is seeded per process, so which statements collide cannot be
// predicted or crafted from outside.

// ShapeCache is a bounded process-wide cache of rendered shapes.
type ShapeCache struct {
	shards   [shapeShards]shapeShard
	hits     atomic.Int64
	misses   atomic.Int64
	evicts   atomic.Int64
	oversize atomic.Int64

	thrash atomic.Pointer[func(ShapeStats)]
	warned atomic.Bool
}

// OnThrash registers fn to be called once, the first time the cache has to
// evict a shape: the sign that the program builds more distinct shapes than
// fit (a select list, join set or IN list taken from input). It is called
// from the goroutine that rendered the statement, outside any lock, with the
// counters at that moment; log from it and return. Pass nil to remove it.
// The static analyzer cannot tell a data-dependent shape from a legitimate
// one, so this is the check that sees the real program.
func (c *ShapeCache) OnThrash(fn func(ShapeStats)) {
	if fn == nil {
		c.thrash.Store(nil)
		return
	}
	c.warned.Store(false)
	c.thrash.Store(&fn)
}

const shapeShards = 32

type shapeShard struct {
	mu  sync.RWMutex
	m   map[shapeKey]*shapeEntry
	max int
}

type shapeKey struct {
	version int
	h1, h2  uint64
}

type shapeEntry struct {
	sql    string
	nslots int
	ref    atomic.Bool // second-chance bit: set by a hit, cleared by the evictor
}

// maxCachedSQL is the longest statement text the cache keeps. A longer one
// (an IN list of thousands of placeholders) is rendered every time, so a few
// huge shapes cannot hold the memory of thousands of ordinary ones; it is
// counted in ShapeStats.Oversize.
const maxCachedSQL = 16 << 10

// evictScan bounds how many entries an eviction inspects.
const evictScan = 16

// DefaultShapes is the cache used by Resolve and by the pgxexec client. Its
// size is per process; see NewShapeCache for a private one. The ceiling is
// not preallocated: a program with a few hundred shapes pays for those only.
var DefaultShapes = NewShapeCache(32768)

// NewShapeCache makes a cache holding about max shapes: max is a ceiling, not
// an allocation, and each of the 32 shards holds max/32 entries. A stored
// shape costs its SQL text plus about 200 bytes of key and map overhead.
func NewShapeCache(max int) *ShapeCache {
	if max < shapeShards {
		max = shapeShards
	}
	c := &ShapeCache{}
	for i := range c.shards {
		c.shards[i] = shapeShard{m: map[shapeKey]*shapeEntry{}, max: max / shapeShards}
	}
	return c
}

// ShapeStats are the counters; a steadily growing Evictions means the program
// builds an unbounded number of shapes, which values alone cannot do: the
// structure has to vary (a select list, join set, ORDER BY or predicate set
// chosen at run time). Lists are one shape whatever their length: In and Any
// both render "= ANY($1)". Oversize counts statements too long to cache (see
// maxCachedSQL); a growing value has the same cause.
type ShapeStats struct {
	Hits, Misses, Evictions int64
	Size                    int
	Oversize                int64
}

// Reset empties the cache and zeroes its counters. It is for tests and for a
// long-lived process that wants a clean start after a burst of shapes. The
// OnThrash callback stays registered and may fire again.
func (c *ShapeCache) Reset() {
	for i := range c.shards {
		sh := &c.shards[i]
		sh.mu.Lock()
		clear(sh.m)
		sh.mu.Unlock()
	}
	c.hits.Store(0)
	c.misses.Store(0)
	c.evicts.Store(0)
	c.oversize.Store(0)
	c.warned.Store(false)
}

// Stats reports the counters.
func (c *ShapeCache) Stats() ShapeStats {
	n := 0
	for i := range c.shards {
		sh := &c.shards[i]
		sh.mu.RLock()
		n += len(sh.m)
		sh.mu.RUnlock()
	}
	return ShapeStats{c.hits.Load(), c.misses.Load(), c.evicts.Load(), n, c.oversize.Load()}
}

var hashBuilders = sync.Pool{New: func() any { return &builder{hash: true, slots: make([]slot, 0, 16)} }}

// Resolve returns the SQL and arguments for s on dialect d, rendering the
// shape only the first time it is seen. binds supply the values of typed
// parameters. A Stmt that was rendered by hand is passed through unchanged.
func (c *ShapeCache) Resolve(d Dialect, s Statement, binds ...Binding) (string, []any, error) {
	if st, ok := s.(Stmt); ok {
		args, err := st.Args(binds...)
		return st.SQL, args, err
	}
	b := hashBuilders.Get().(*builder)
	b.hash, b.err, b.version = true, nil, d.version()
	b.slots = b.slots[:0]
	b.h1, b.h2 = hashSeed1, hashSeed2
	b.scopes, b.windows, b.noWindow, b.colHook, b.aggDepth, b.sawAgg, b.winCall, b.inline = b.scopes[:0], nil, 0, nil, 0, false, false, 0
	s.render(b)
	key := shapeKey{b.version, b.h1, b.h2}
	nslots := len(b.slots)
	if b.err != nil || nslots > 65535 {
		releaseHashBuilder(b)
		return c.miss(d, s, binds) // report the real error from a full render
	}
	sh := &c.shards[key.h1%shapeShards]
	sh.mu.RLock()
	e, ok := sh.m[key]
	sh.mu.RUnlock()
	if !ok || e.nslots != nslots {
		releaseHashBuilder(b)
		return c.miss(d, s, binds)
	}
	c.hits.Add(1)
	if !e.ref.Load() { // read first: a write per hit would bounce the cache line
		e.ref.Store(true)
	}
	args, err := argsFrom(b.slots, binds)
	releaseHashBuilder(b)
	return e.sql, args, err
}

// releaseHashBuilder returns a builder to the pool without the argument
// values of the statement it just hashed: the pool must not keep user data
// alive (a long-lived builder would pin the last statement's values).
func releaseHashBuilder(b *builder) {
	clear(b.slots)
	hashBuilders.Put(b)
}

// miss renders in full, stores the shape and returns its SQL and arguments.
func (c *ShapeCache) miss(d Dialect, s Statement, binds []Binding) (string, []any, error) {
	c.misses.Add(1)
	hb := &builder{hash: true, version: d.version(), h1: hashSeed1, h2: hashSeed2}
	s.render(hb)
	st, err := Render(d, s)
	if err != nil {
		return "", nil, err
	}
	switch {
	case hb.err != nil || len(hb.slots) != len(st.slots):
		// not cacheable: the shape cannot be told apart from the render
	case len(st.SQL) > maxCachedSQL:
		c.oversize.Add(1)
		c.notifyThrash()
	default:
		key := shapeKey{hb.version, hb.h1, hb.h2}
		sh := &c.shards[key.h1%shapeShards]
		sh.mu.Lock()
		old, exists := sh.m[key]
		if exists && old.nslots == len(st.slots) && old.sql == st.SQL {
			// another goroutine stored the same shape first
		} else {
			// A different entry under this key means a hash collision, or an
			// entry that no longer matches: the fresh render wins.
			if !exists && len(sh.m) >= sh.max {
				if sh.evict() {
					c.evicts.Add(1)
				}
			}
			sh.m[key] = &shapeEntry{sql: st.SQL, nslots: len(st.slots)}
		}
		sh.mu.Unlock()
		if c.evicts.Load() > 0 {
			c.notifyThrash()
		}
	}
	args, err := st.Args(binds...)
	return st.SQL, args, err
}

// evict removes one entry, preferring one that no hit has touched since the
// last pass (second chance): the walk clears the bit of each recently used
// entry it skips. After evictScan inspections it takes the last one seen, so
// the cost stays bounded. The caller holds the shard lock.
func (sh *shapeShard) evict() bool {
	var last shapeKey
	seen := false
	n := 0
	for k, e := range sh.m {
		last, seen = k, true
		if e.ref.Load() {
			e.ref.Store(false)
			if n++; n < evictScan {
				continue
			}
			break
		}
		delete(sh.m, k)
		return true
	}
	if seen {
		delete(sh.m, last)
	}
	return seen
}

// notifyThrash calls the OnThrash callback once per registration.
func (c *ShapeCache) notifyThrash() {
	if fn := c.thrash.Load(); fn != nil && c.warned.CompareAndSwap(false, true) {
		(*fn)(c.Stats())
	}
}

func argsFrom(slots []slot, binds []Binding) ([]any, error) {
	out := make([]any, len(slots))
	used := 0
	for i, sl := range slots {
		if sl.param == 0 {
			out[i] = sl.value
			continue
		}
		found := false
		for k := range binds {
			if binds[k].id == sl.param {
				out[i] = normalizeArg(binds[k].value)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("%w: parameter %d is not bound", ErrParams, sl.param)
		}
	}
	// every binding must belong to the statement, and none may repeat
	for k := range binds {
		for j := 0; j < k; j++ {
			if binds[j].id == binds[k].id {
				return nil, fmt.Errorf("%w: parameter bound twice", ErrParams)
			}
		}
		for _, sl := range slots {
			if sl.param == binds[k].id {
				used++
				break
			}
		}
	}
	if used != len(binds) {
		return nil, fmt.Errorf("%w: %d bindings do not all belong to this statement", ErrParams, len(binds))
	}
	return out, nil
}

// Resolve is ShapeCache.Resolve on DefaultShapes.
func Resolve(d Dialect, s Statement, binds ...Binding) (string, []any, error) {
	return DefaultShapes.Resolve(d, s, binds...)
}

// Stmt is itself a Statement so that hand-rendered statements and builders go
// through the same entry points. A Stmt can only be the whole statement: its
// placeholders are already numbered.
func (s Stmt) render(b *builder) {
	if len(b.slots) != 0 || (!b.hash && b.sb.Len() != 0) {
		b.fail(fmt.Errorf("%w: a rendered Stmt cannot be nested inside another statement", ErrShape))
		return
	}
	b.write(s.SQL)
	b.slots = append(b.slots, s.slots...)
}

// ---- hashing --------------------------------------------------------------

const (
	prime1 = 1099511628211
	prime2 = 0xFF51AFD7ED558CCD
)

// The seeds start from the FNV-1a offset and a golden-ratio constant and are
// xored with per-process random values, so no two processes hash alike and a
// collision cannot be built ahead of time. Entries never leave the process.
var hashSeed1, hashSeed2 = func() (uint64, uint64) {
	var s maphash.Seed = maphash.MakeSeed()
	return 14695981039346656037 ^ maphash.String(s, "qlx/1"), 0x9E3779B97F4A7C15 ^ maphash.String(s, "qlx/2")
}()

func (b *builder) mixByte(c byte) {
	b.h1 = (b.h1 ^ uint64(c)) * prime1
	b.h2 = (b.h2 + uint64(c) + 1) * prime2
	b.h2 ^= b.h2 >> 29
}

func (b *builder) mix(s string) {
	h1, h2 := b.h1, b.h2
	// Eight bytes per step: the byte-at-a-time chain is latency bound. A
	// short tail is packed with its length so that "ab" and "ab\x00" differ.
	for len(s) >= 8 {
		w := binary.LittleEndian.Uint64([]byte(s[:8]))
		h1 = (h1 ^ w) * prime1
		h1 ^= h1 >> 32
		h2 = (h2 + w + 1) * prime2
		h2 ^= h2 >> 29
		s = s[8:]
	}
	if len(s) > 0 {
		w := uint64(len(s)) << 56
		for i := 0; i < len(s); i++ {
			w |= uint64(s[i]) << (8 * uint(i))
		}
		h1 = (h1 ^ w) * prime1
		h1 ^= h1 >> 32
		h2 = (h2 + w + 1) * prime2
		h2 ^= h2 >> 29
	}
	b.h1, b.h2 = h1, h2
}

func (b *builder) mixInt(n int) {
	var buf [20]byte
	i := len(buf)
	if n == 0 {
		i--
		buf[i] = '0'
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	for ; i < len(buf); i++ {
		b.mixByte(buf[i])
	}
}
