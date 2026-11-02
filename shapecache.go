package qlx

import (
	"encoding/binary"
	"fmt"
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

// ShapeCache is a bounded process-wide cache of rendered shapes.
type ShapeCache struct {
	shards [shapeShards]shapeShard
	hits   atomic.Int64
	misses atomic.Int64
	evicts atomic.Int64
}

const shapeShards = 32

type shapeShard struct {
	mu  sync.RWMutex
	m   map[shapeKey]shapeEntry
	max int
}

type shapeKey struct {
	version int
	h1, h2  uint64
}

type shapeEntry struct {
	sql    string
	nslots int
}

// DefaultShapes is the cache used by Resolve and by the pgxexec client. Its
// size is per process; see NewShapeCache for a private one.
var DefaultShapes = NewShapeCache(4096)

// NewShapeCache makes a cache holding about max shapes.
func NewShapeCache(max int) *ShapeCache {
	if max < shapeShards {
		max = shapeShards
	}
	c := &ShapeCache{}
	for i := range c.shards {
		c.shards[i] = shapeShard{m: map[shapeKey]shapeEntry{}, max: max / shapeShards}
	}
	return c
}

// ShapeStats are the counters; a steadily growing Evictions means the program
// builds an unbounded number of shapes, which values alone cannot do: the
// structure has to vary (a select list, join set, ORDER BY or predicate set
// chosen at run time). Lists are one shape whatever their length: In and Any
// both render "= ANY($1)".
type ShapeStats struct {
	Hits, Misses, Evictions int64
	Size                    int
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
	return ShapeStats{c.hits.Load(), c.misses.Load(), c.evicts.Load(), n}
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
	b.scopes, b.windows, b.noWindow, b.colHook, b.aggDepth, b.sawAgg, b.winCall = b.scopes[:0], nil, 0, nil, 0, false, false
	s.render(b)
	key := shapeKey{b.version, b.h1, b.h2}
	nslots := len(b.slots)
	if b.err != nil || nslots > 65535 {
		hashBuilders.Put(b)
		return c.miss(d, s, binds) // report the real error from a full render
	}
	sh := &c.shards[key.h1%shapeShards]
	sh.mu.RLock()
	e, ok := sh.m[key]
	sh.mu.RUnlock()
	if !ok || e.nslots != nslots {
		hashBuilders.Put(b)
		return c.miss(d, s, binds)
	}
	c.hits.Add(1)
	args, err := argsFrom(b.slots, binds)
	for i := range b.slots {
		b.slots[i] = slot{} // do not keep user values alive in the pool
	}
	hashBuilders.Put(b)
	return e.sql, args, err
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
	if hb.err == nil && len(hb.slots) == len(st.slots) {
		key := shapeKey{hb.version, hb.h1, hb.h2}
		sh := &c.shards[key.h1%shapeShards]
		sh.mu.Lock()
		if _, exists := sh.m[key]; !exists {
			if len(sh.m) >= sh.max {
				for k := range sh.m { // evict an arbitrary entry
					delete(sh.m, k)
					c.evicts.Add(1)
					break
				}
			}
			sh.m[key] = shapeEntry{sql: st.SQL, nslots: len(st.slots)}
		}
		sh.mu.Unlock()
	}
	args, err := st.Args(binds...)
	return st.SQL, args, err
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
	hashSeed1 = 14695981039346656037 // FNV-1a offset
	hashSeed2 = 0x9E3779B97F4A7C15
	prime1    = 1099511628211
	prime2    = 0xFF51AFD7ED558CCD
)

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
