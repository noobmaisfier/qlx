package qlx

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode/utf8"
)

// ABI1 is referenced by generated code. A generator that emits a newer ABI
// references a symbol this runtime does not have, so a mismatched generator
// and runtime fail to compile instead of misbehaving.
const ABI1 = 1

// Supported dialect versions.
const (
	PG16 = 16
	PG17 = 17
	PG18 = 18
	PG19 = 19
)

// Dialect selects the PostgreSQL major version the SQL is rendered for.
// The zero value targets the newest supported version.
type Dialect struct{ Version int }

func (d Dialect) version() int {
	if d.Version == 0 {
		return PG19
	}
	return d.Version
}

// node is the internal AST. It is sealed: callers cannot create nodes, so
// no code path can inject SQL text.
type node interface{ render(b *builder) }

type slot struct {
	value any
	param int // > 0: bound later through Binding
}

type builder struct {
	sb       strings.Builder
	slots    []slot
	paramPos map[int]int
	version  int
	err      error
	scopes   [][]string
	// noWindow > 0 while rendering a clause that forbids window functions.
	colHook  func(colRef) // set on probe builders (grouping check)
	aggDepth int          // > 0 inside a grouping aggregate call
	sawAgg   bool
	winCall  bool     // the next aggNode is a window function call
	windows  []string // named windows of the SELECT being rendered
	noWindow int
	// hash mode: instead of building SQL text the builder hashes it (the
	// bytes that would have been written) and collects the argument slots.
	// This is how a statement finds its cached rendering without rendering.
	hash   bool
	h1, h2 uint64
	// inline > 0: arguments are written into the text as literals (COPY).
	inline int
}

func newBuilder(d Dialect) *builder {
	return &builder{version: d.version(), paramPos: map[int]int{}}
}

func (b *builder) fail(err error) {
	if b.err == nil {
		b.err = err
	}
}

func (b *builder) write(s string) {
	if b.hash {
		b.mix(s)
		return
	}
	b.sb.WriteString(s)
}

func (b *builder) ident(name string) {
	if b.hash {
		if name == "" || len(name) > maxIdentBytes || !utf8.ValidString(name) || strings.IndexByte(name, 0) >= 0 {
			b.fail(fmt.Errorf("%w: %q", ErrInvalidIdent, name))
			return
		}
		b.mixByte('"')
		b.mix(name) // unescaped: injective, and cheaper than quoting
		b.mixByte('"')
		return
	}
	q, err := quoteIdent(name)
	if err != nil {
		b.fail(err)
		return
	}
	b.sb.WriteString(q)
}

type argValuer interface{ QLXArg() any }

func normalizeArg(v any) any {
	// A wrapper (qt.Null[qt.ID[T]]) hands back another argValuer; two hops
	// reach every value type this library defines.
	for i := 0; i < 3; i++ {
		a, ok := v.(argValuer)
		if !ok {
			break
		}
		v = a.QLXArg()
	}
	return v
}

func (b *builder) arg(v any) {
	if b.inline > 0 {
		lit, err := inlineLiteral(v)
		if err != nil {
			b.fail(err)
			return
		}
		b.write(lit)
		return
	}
	b.slots = append(b.slots, slot{value: normalizeArg(v)})
	if b.hash {
		b.mixByte('$')
		b.mixInt(len(b.slots))
		return
	}
	b.sb.WriteByte('$')
	b.sb.WriteString(strconv.Itoa(len(b.slots)))
}

func (b *builder) param(id int) {
	if b.inline > 0 {
		b.fail(fmt.Errorf("%w: a parameter bound later cannot be written as a literal", ErrParams))
		return
	}
	if b.hash {
		pos := 0
		for i, sl := range b.slots {
			if sl.param == id {
				pos = i + 1
				break
			}
		}
		if pos == 0 {
			b.slots = append(b.slots, slot{param: id})
			pos = len(b.slots)
		}
		b.mixByte('$')
		b.mixInt(pos)
		return
	}
	pos, ok := b.paramPos[id]
	if !ok {
		b.slots = append(b.slots, slot{param: id})
		pos = len(b.slots)
		b.paramPos[id] = pos
	}
	b.sb.WriteByte('$')
	b.sb.WriteString(strconv.Itoa(pos))
}

func (b *builder) need(version int, what string) {
	if b.version < version {
		b.fail(fmt.Errorf("%w: %s requires PostgreSQL %d, target is %d", ErrUnsupportedByServer, what, version, b.version))
	}
}

func (b *builder) pushScope(keys ...string) { b.scopes = append(b.scopes, keys) }
func (b *builder) popScope()                { b.scopes = b.scopes[:len(b.scopes)-1] }

func (b *builder) inScope(key string) bool {
	for i := len(b.scopes) - 1; i >= 0; i-- {
		for _, k := range b.scopes[i] {
			if k == key {
				return true
			}
		}
	}
	return false
}

// Stmt is a rendered statement: SQL text plus the argument layout. It is
// immutable and safe to share between goroutines, so static queries are
// rendered once at startup and reused.
type Stmt struct {
	SQL   string
	slots []slot
}

// NumParams reports how many bound parameters Args expects.
func (s Stmt) NumParams() int {
	seen := map[int]struct{}{}
	for _, sl := range s.slots {
		if sl.param > 0 {
			seen[sl.param] = struct{}{}
		}
	}
	return len(seen)
}

// Args resolves the argument list, filling typed parameters from binds.
func (s Stmt) Args(binds ...Binding) ([]any, error) {
	// Statements have a handful of parameters, so a linear scan beats a map
	// and keeps the hot path at a single allocation (the result slice).
	for i := range binds {
		for j := 0; j < i; j++ {
			if binds[j].id == binds[i].id {
				return nil, fmt.Errorf("%w: parameter bound twice", ErrParams)
			}
		}
	}
	out := make([]any, len(s.slots))
	for i, sl := range s.slots {
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
	if want := s.NumParams(); len(binds) != want {
		return nil, fmt.Errorf("%w: %d bindings for %d parameters", ErrParams, len(binds), want)
	}
	return out, nil
}

// Binding is a value for a Param.
type Binding struct {
	id    int
	value any
}

var paramSeq atomic.Int64

// Param is a typed placeholder for static queries: declare it once, use it in
// the query, bind a value on every call. Types are checked by the compiler.
type Param[T any] struct{ id int }

// NewParam allocates a new parameter.
func NewParam[T any]() Param[T] { return Param[T]{id: int(paramSeq.Add(1))} }

// Bind pairs the parameter with a value.
func (p Param[T]) Bind(v T) Binding { return Binding{id: p.id, value: v} }

func (p Param[T]) exprNode() node { return paramNode{p.id} }
func (Param[T]) typ() (z T)       { return }

type paramNode struct{ id int }

func (n paramNode) render(b *builder) { b.param(n.id) }

// Statement is anything that renders to SQL.
type Statement interface {
	render(b *builder)
}

// Render renders a statement once for the dialect.
func Render(d Dialect, s Statement) (Stmt, error) {
	if s == nil {
		return Stmt{}, fmt.Errorf("%w: nil statement", ErrShape)
	}
	b := newBuilder(d)
	s.render(b)
	if b.err != nil {
		return Stmt{}, b.err
	}
	return Stmt{SQL: b.sb.String(), slots: b.slots}, nil
}

// SQL renders a statement that has no typed parameters and returns the SQL
// and its arguments, ready for any pgx call.
func SQL(d Dialect, s Statement) (string, []any, error) {
	st, err := Render(d, s)
	if err != nil {
		return "", nil, err
	}
	args, err := st.Args()
	if err != nil {
		return "", nil, err
	}
	return st.SQL, args, nil
}

func (b *builder) hasWindow(name string) bool {
	for _, w := range b.windows {
		if w == name {
			return true
		}
	}
	return false
}
