package qlx

import (
	"fmt"

	"github.com/noobmaisfier/qlx/qt"
)

// ---------------------------------------------------------------------------
// Aggregate call node (also the body of window function calls)
// ---------------------------------------------------------------------------

type aggNode struct {
	name      string // constant function name from this package
	args      []node
	star      bool
	distinct  bool
	order     []OrderTerm // ORDER BY inside the call: string_agg(x, ',' ORDER BY y)
	within    []OrderTerm // WITHIN GROUP (ORDER BY ...)
	filter    node
	nullsMode string // "", "IGNORE NULLS", "RESPECT NULLS" (PostgreSQL 19)
	// Set by generated catalog wrappers (AggCall): the owning schema, which
	// is then quoted, and the majors the aggregate exists in.
	schema     string
	min, until int
}

func (n aggNode) render(b *builder) {
	if b.winCall {
		b.winCall = false // a window function call: its arguments are not exempt from grouping
	} else {
		b.sawAgg = true
		b.aggDepth++
		defer func() { b.aggDepth-- }()
	}
	if n.min > 0 {
		b.need(n.min, n.name+"()")
	}
	if n.until > 0 && b.version >= n.until {
		b.fail(fmt.Errorf("%w: %s was removed in PostgreSQL %d, target is %d", ErrUnsupportedByServer, n.name, n.until, b.version))
		return
	}
	if n.schema != "" {
		b.ident(n.schema)
		b.write(".")
		b.ident(n.name)
	} else {
		b.write(n.name)
	}
	b.write("(")
	switch {
	case n.star:
		b.write("*")
	default:
		if n.distinct {
			b.write("DISTINCT ")
		}
		for i, a := range n.args {
			if i > 0 {
				b.write(", ")
			}
			a.render(b)
		}
		if len(n.order) > 0 {
			b.write(" ORDER BY ")
			joinParts(b, len(n.order), func(i int) { n.order[i].render(b) })
		}
	}
	b.write(")")
	if len(n.within) > 0 {
		b.write(" WITHIN GROUP (ORDER BY ")
		joinParts(b, len(n.within), func(i int) { n.within[i].render(b) })
		b.write(")")
	}
	if n.filter != nil {
		b.write(" FILTER (WHERE ")
		n.filter.render(b)
		b.write(")")
	}
	if n.nullsMode != "" {
		b.need(PG19, n.nullsMode)
		b.write(" " + n.nullsMode)
	}
}

// AggExpr is an aggregate expression. It is accepted by SELECT, HAVING and
// ORDER BY, and is deliberately not an Operand, so it cannot be placed in
// WHERE, a JOIN condition or the argument of another aggregate.
type AggExpr[T any] struct{ n aggNode }

func (a AggExpr[T]) selectNode() selectItem   { return selectItem{n: a.n} }
func (a AggExpr[T]) As(alias string) Named[T] { return Named[T]{selectItem{a.n, alias}} }
func (a AggExpr[T]) Asc() OrderTerm           { return OrderTerm{n: a.n} }
func (a AggExpr[T]) Desc() OrderTerm          { return OrderTerm{n: a.n, desc: true} }

// Filter adds FILTER (WHERE p); an absent predicate leaves the aggregate as is.
func (a AggExpr[T]) Filter(p Pred) AggExpr[T] {
	if p.valid() {
		a.n.filter = p.n
	}
	return a
}

// Distinct makes the aggregate DISTINCT.
func (a AggExpr[T]) Distinct() AggExpr[T] { a.n.distinct = true; return a }

// OrderBy adds an ORDER BY inside the call (string_agg, array_agg ...).
func (a AggExpr[T]) OrderBy(ts ...OrderTerm) AggExpr[T] {
	a.n.order = app(a.n.order, ts...)
	return a
}

// Over turns the aggregate into a window function call.
func (a AggExpr[T]) Over(w WindowSpec) WinExpr[T] {
	return WinExpr[T]{Expr[T]{winNode{call: a.n, spec: w}}}
}

func (a AggExpr[T]) agg(op string, v T) AggPred {
	return AggPred{cmpNode{l: a.n, op: op, r: argNode{v}}}
}
func (a AggExpr[T]) Eq(v T) AggPred  { return a.agg("=", v) }
func (a AggExpr[T]) Ne(v T) AggPred  { return a.agg("<>", v) }
func (a AggExpr[T]) Gt(v T) AggPred  { return a.agg(">", v) }
func (a AggExpr[T]) Gte(v T) AggPred { return a.agg(">=", v) }
func (a AggExpr[T]) Lt(v T) AggPred  { return a.agg("<", v) }
func (a AggExpr[T]) Lte(v T) AggPred { return a.agg("<=", v) }

func agg1[R any](name string, o interface{ exprNode() node }) AggExpr[R] {
	return AggExpr[R]{aggNode{name: name, args: []node{o.exprNode()}}}
}

// CountAll is count(*). COUNT never returns NULL, so the type is not nullable.
func CountAll() AggExpr[int64] { return AggExpr[int64]{aggNode{name: "count", star: true}} }

// Count is count(expr).
func Count[T any](o Operand[T]) AggExpr[int64] { return agg1[int64]("count", o) }

// CountDistinct is count(DISTINCT expr).
func CountDistinct[T any](o Operand[T]) AggExpr[int64] {
	a := agg1[int64]("count", o)
	a.n.distinct = true
	return a
}

// Integer families by sum/avg result type (PostgreSQL: sum(int2|int4) is
// bigint, sum(int8) and avg(any integer) are numeric).
type (
	// SmallInt is int2 or int4, nullable or not.
	SmallInt interface {
		~int16 | ~int32 | qt.Null[int16] | qt.Null[int32]
	}
	// BigInt is int8, nullable or not.
	BigInt interface{ ~int64 | qt.Null[int64] }
	// AnyInt is any integer type, nullable or not.
	AnyInt interface {
		~int16 | ~int32 | ~int64 | qt.Null[int16] | qt.Null[int32] | qt.Null[int64]
	}
	// FloatT is float4 or float8, nullable or not.
	FloatT interface {
		~float32 | ~float64 | qt.Null[float32] | qt.Null[float64]
	}
)

// Sum of int2 or int4 is bigint; sum of no rows is NULL.
func Sum[T SmallInt](o Operand[T]) AggExpr[qt.Null[int64]] { return agg1[qt.Null[int64]]("sum", o) }

// SumBig is sum over int8, which PostgreSQL returns as numeric.
func SumBig[T BigInt](o Operand[T]) AggExpr[qt.Null[qt.Numeric]] {
	return agg1[qt.Null[qt.Numeric]]("sum", o)
}

// SumFloat is sum over float8.
func SumFloat[T FloatT](o Operand[T]) AggExpr[qt.Null[float64]] {
	return agg1[qt.Null[float64]]("sum", o)
}

// Avg of any integer is numeric.
func Avg[T AnyInt](o Operand[T]) AggExpr[qt.Null[qt.Numeric]] {
	return agg1[qt.Null[qt.Numeric]]("avg", o)
}

// AvgFloat is avg over float8.
func AvgFloat[T FloatT](o Operand[T]) AggExpr[qt.Null[float64]] {
	return agg1[qt.Null[float64]]("avg", o)
}

// Max is max(expr); the result is nullable (no rows).
func Max[T any](o Operand[T]) AggExpr[qt.Null[T]] { return agg1[qt.Null[T]]("max", o) }

// Min is min(expr); the result is nullable (no rows).
func Min[T any](o Operand[T]) AggExpr[qt.Null[T]] { return agg1[qt.Null[T]]("min", o) }

// BoolAnd is bool_and(expr).
func BoolAnd(o Operand[bool]) AggExpr[qt.Null[bool]] { return agg1[qt.Null[bool]]("bool_and", o) }

// BoolOr is bool_or(expr).
func BoolOr(o Operand[bool]) AggExpr[qt.Null[bool]] { return agg1[qt.Null[bool]]("bool_or", o) }

// ArrayAgg is array_agg(expr); an empty group gives NULL.
func ArrayAgg[T any](o Operand[T]) AggExpr[qt.Null[[]T]] {
	return agg1[qt.Null[[]T]]("array_agg", o)
}

// StringAgg is string_agg(expr, sep). The separator is a bound parameter.
func StringAgg[T Text](o Operand[T], sep string) AggExpr[qt.Null[string]] {
	return AggExpr[qt.Null[string]]{aggNode{name: "string_agg", args: []node{o.exprNode(), argNode{sep}}}}
}

// GroupingFn is grouping(a, b ...), valid only with GROUP BY.
func GroupingFn(vs ...Value) AggExpr[int32] {
	a := aggNode{name: "grouping"}
	for _, v := range vs {
		a.args = append(a.args, v.valueNode())
	}
	return AggExpr[int32]{a}
}

// OrderedAgg is an ordered-set aggregate waiting for WITHIN GROUP.
type OrderedAgg[T any] struct{ n aggNode }

// WithinGroup supplies the sort key.
func (a OrderedAgg[T]) WithinGroup(o OrderTerm) AggExpr[T] {
	a.n.within = []OrderTerm{o}
	return AggExpr[T]{a.n}
}

// PercentileCont is percentile_cont(fraction) over float8.
func PercentileCont(fraction float64) OrderedAgg[qt.Null[float64]] {
	return OrderedAgg[qt.Null[float64]]{aggNode{name: "percentile_cont", args: []node{argNode{fraction}}}}
}

// PercentileDisc is percentile_disc(fraction); T is the type of the sort key.
func PercentileDisc[T any](fraction float64) OrderedAgg[qt.Null[T]] {
	return OrderedAgg[qt.Null[T]]{aggNode{name: "percentile_disc", args: []node{argNode{fraction}}}}
}

// Mode is mode() WITHIN GROUP (ORDER BY ...); T is the type of the sort key.
func Mode[T any]() OrderedAgg[qt.Null[T]] {
	return OrderedAgg[qt.Null[T]]{aggNode{name: "mode"}}
}

// ---------------------------------------------------------------------------
// GROUP BY: ROLLUP, CUBE, GROUPING SETS
// ---------------------------------------------------------------------------

type groupFnNode struct {
	kind  string // ROLLUP | CUBE
	items []node
}

func (n groupFnNode) render(b *builder) {
	b.write(n.kind + " (")
	joinParts(b, len(n.items), func(i int) { n.items[i].render(b) })
	b.write(")")
}

type groupSetsNode struct{ sets [][]node }

func (n groupSetsNode) render(b *builder) {
	b.write("GROUPING SETS (")
	for i, s := range n.sets {
		if i > 0 {
			b.write(", ")
		}
		b.write("(")
		joinParts(b, len(s), func(j int) { s[j].render(b) })
		b.write(")")
	}
	b.write(")")
}

// GroupItem is a GROUP BY item that is not a plain expression.
type GroupItem struct{ n node }

func (g GroupItem) valueNode() node { return g.n }

func nodesOf(vs []Value) []node {
	out := make([]node, len(vs))
	for i, v := range vs {
		out[i] = v.valueNode()
	}
	return out
}

// Rollup is ROLLUP (a, b, ...).
func Rollup(vs ...Value) GroupItem { return GroupItem{groupFnNode{"ROLLUP", nodesOf(vs)}} }

// Cube is CUBE (a, b, ...).
func Cube(vs ...Value) GroupItem { return GroupItem{groupFnNode{"CUBE", nodesOf(vs)}} }

// GroupSet is one set inside GROUPING SETS; the empty set is the grand total.
type GroupSet struct{ items []node }

// SetOf builds one grouping set.
func SetOf(vs ...Value) GroupSet { return GroupSet{nodesOf(vs)} }

// GroupingSets is GROUPING SETS ((a, b), (a), ()).
func GroupingSets(sets ...GroupSet) GroupItem {
	n := groupSetsNode{}
	for _, s := range sets {
		n.sets = append(n.sets, s.items)
	}
	return GroupItem{n}
}

// ---------------------------------------------------------------------------
// Windows
// ---------------------------------------------------------------------------

// FrameBound is one end of a window frame.
type FrameBound struct {
	sql string
	n   *int64
}

// Frame bound constants and constructors.
var (
	UnboundedPreceding = FrameBound{sql: "UNBOUNDED PRECEDING"}
	CurrentRow         = FrameBound{sql: "CURRENT ROW"}
	UnboundedFollowing = FrameBound{sql: "UNBOUNDED FOLLOWING"}
)

// Preceding is "n PRECEDING"; n is a bound parameter.
func Preceding(n int64) FrameBound { return FrameBound{sql: "PRECEDING", n: &n} }

// Following is "n FOLLOWING".
func Following(n int64) FrameBound { return FrameBound{sql: "FOLLOWING", n: &n} }

func (f FrameBound) render(b *builder) {
	if f.n != nil {
		b.arg(*f.n)
		b.write(" ")
	}
	b.write(f.sql)
}

// WindowSpec is an immutable window definition.
type WindowSpec struct {
	partition []Value
	order     []OrderTerm
	mode      string // ROWS | RANGE | GROUPS
	start     *FrameBound
	end       *FrameBound
	exclude   string
	ref       string // OVER name: use a window defined in the WINDOW clause
	base      string // OVER (name ...): extend a named window
}

// NamedWindow is a window defined once in the WINDOW clause and used by name.
type NamedWindow struct {
	name string
	spec WindowSpec
}

// NewWindow names a window definition. Add it to a query with
// SelectQuery.Windows, then pass Spec() or Extend() to Over.
func NewWindow(name string, spec WindowSpec) NamedWindow { return NamedWindow{name, spec} }

// Spec refers to the window as written: OVER "name".
func (w NamedWindow) Spec() WindowSpec { return WindowSpec{ref: w.name} }

// Extend starts a window that builds on this one: OVER ("name" ORDER BY ...).
// PostgreSQL restricts what may be added (no PARTITION BY, no second frame).
func (w NamedWindow) Extend() WindowSpec { return WindowSpec{base: w.name} }

// Window starts an empty window definition: OVER ().
func Window() WindowSpec { return WindowSpec{} }

func (w WindowSpec) PartitionBy(vs ...Value) WindowSpec {
	w.partition = app(w.partition, vs...)
	return w
}
func (w WindowSpec) OrderBy(ts ...OrderTerm) WindowSpec { w.order = app(w.order, ts...); return w }

func (w WindowSpec) frame(mode string, start, end FrameBound) WindowSpec {
	w.mode, w.start, w.end = mode, &start, &end
	return w
}

// Rows sets ROWS BETWEEN start AND end.
func (w WindowSpec) Rows(start, end FrameBound) WindowSpec { return w.frame("ROWS", start, end) }

// Range sets RANGE BETWEEN start AND end.
func (w WindowSpec) Range(start, end FrameBound) WindowSpec { return w.frame("RANGE", start, end) }

// Groups sets GROUPS BETWEEN start AND end.
func (w WindowSpec) Groups(start, end FrameBound) WindowSpec { return w.frame("GROUPS", start, end) }

// ExcludeCurrentRow adds EXCLUDE CURRENT ROW.
func (w WindowSpec) ExcludeCurrentRow() WindowSpec { w.exclude = "CURRENT ROW"; return w }

// ExcludeGroup adds EXCLUDE GROUP.
func (w WindowSpec) ExcludeGroup() WindowSpec { w.exclude = "GROUP"; return w }

// ExcludeTies adds EXCLUDE TIES.
func (w WindowSpec) ExcludeTies() WindowSpec { w.exclude = "TIES"; return w }

func (w WindowSpec) render(b *builder) {
	if w.ref != "" {
		if !b.hasWindow(w.ref) {
			b.fail(fmt.Errorf("%w: window %q is not defined by this query's WINDOW clause", ErrScope, w.ref))
			return
		}
		b.ident(w.ref)
		return
	}
	w.renderBody(b)
}

func (w WindowSpec) renderBody(b *builder) {
	b.write("(")
	first := true
	if w.base != "" {
		if !b.hasWindow(w.base) {
			b.fail(fmt.Errorf("%w: window %q is not defined by this query's WINDOW clause", ErrScope, w.base))
			return
		}
		b.ident(w.base)
		first = false
	}
	sep := func() {
		if !first {
			b.write(" ")
		}
		first = false
	}
	if len(w.partition) > 0 {
		sep()
		b.write("PARTITION BY ")
		joinParts(b, len(w.partition), func(i int) { w.partition[i].valueNode().render(b) })
	}
	if len(w.order) > 0 {
		sep()
		b.write("ORDER BY ")
		joinParts(b, len(w.order), func(i int) { w.order[i].render(b) })
	}
	if w.mode != "" {
		sep()
		b.write(w.mode + " BETWEEN ")
		w.start.render(b)
		b.write(" AND ")
		w.end.render(b)
		if w.exclude != "" {
			b.write(" EXCLUDE " + w.exclude)
		}
	}
	b.write(")")
}

type winNode struct {
	call aggNode
	spec WindowSpec
}

func (n winNode) render(b *builder) {
	if b.noWindow > 0 {
		b.fail(fmt.Errorf("%w: window function %s not allowed in WHERE, GROUP BY, HAVING or JOIN conditions", ErrShape, n.call.name))
		return
	}
	b.winCall = true
	n.call.render(b)
	b.winCall = false
	b.write(" OVER ")
	n.spec.render(b)
}

// WinExpr is a window function result. It is an Operand so it can sit inside
// ordinary expressions in SELECT and ORDER BY; using one in WHERE, GROUP BY,
// HAVING or a JOIN condition is reported when the statement is rendered.
type WinExpr[T any] struct{ Expr[T] }

// WinFn is a window function waiting for OVER.
type WinFn[T any] struct{ n aggNode }

// Over completes the call.
func (f WinFn[T]) Over(w WindowSpec) WinExpr[T] {
	return WinExpr[T]{Expr[T]{winNode{call: f.n, spec: w}}}
}

// NullsWinFn is a window function that accepts IGNORE NULLS / RESPECT NULLS
// (PostgreSQL 19): lag, lead, first_value, last_value and nth_value, and
// nothing else. The method exists only on this type, so other window
// functions cannot be given it.
type NullsWinFn[T any] struct{ WinFn[T] }

// IgnoreNulls adds IGNORE NULLS (PostgreSQL 19).
//
// Requires PostgreSQL 19.
func (f NullsWinFn[T]) IgnoreNulls() NullsWinFn[T] { f.n.nullsMode = "IGNORE NULLS"; return f }

// RespectNulls adds RESPECT NULLS (PostgreSQL 19).
//
// Requires PostgreSQL 19.
func (f NullsWinFn[T]) RespectNulls() NullsWinFn[T] { f.n.nullsMode = "RESPECT NULLS"; return f }

func winCall[T any](name string, args ...node) WinFn[T] {
	return WinFn[T]{aggNode{name: name, args: args}}
}

func nullsCall[T any](name string, args ...node) NullsWinFn[T] {
	return NullsWinFn[T]{winCall[T](name, args...)}
}

func RowNumber() WinFn[int64]          { return winCall[int64]("row_number") }
func Rank() WinFn[int64]               { return winCall[int64]("rank") }
func DenseRank() WinFn[int64]          { return winCall[int64]("dense_rank") }
func PercentRank() WinFn[float64]      { return winCall[float64]("percent_rank") }
func CumeDist() WinFn[float64]         { return winCall[float64]("cume_dist") }
func Ntile(buckets int32) WinFn[int32] { return winCall[int32]("ntile", argNode{buckets}) }

// Lag over a non-null operand: NULL when there is no earlier row.
func Lag[T any](o Operand[T]) NullsWinFn[qt.Null[T]] {
	return nullsCall[qt.Null[T]]("lag", o.exprNode())
}

// LagN is Lag over an operand that is already nullable.
func LagN[T any](o Operand[qt.Null[T]]) NullsWinFn[qt.Null[T]] {
	return nullsCall[qt.Null[T]]("lag", o.exprNode())
}

// LagBy looks n rows back.
func LagBy[T any](o Operand[T], n int64) NullsWinFn[qt.Null[T]] {
	return nullsCall[qt.Null[T]]("lag", o.exprNode(), argNode{n})
}

// LagDefault looks n rows back and substitutes def when there is none.
func LagDefault[T any](o Operand[T], n int64, def T) NullsWinFn[T] {
	return nullsCall[T]("lag", o.exprNode(), argNode{n}, argNode{def})
}

// Lead is the forward-looking counterpart of Lag.
func Lead[T any](o Operand[T]) NullsWinFn[qt.Null[T]] {
	return nullsCall[qt.Null[T]]("lead", o.exprNode())
}

// LeadN is Lead over an operand that is already nullable.
func LeadN[T any](o Operand[qt.Null[T]]) NullsWinFn[qt.Null[T]] {
	return nullsCall[qt.Null[T]]("lead", o.exprNode())
}

// LeadBy looks n rows ahead.
func LeadBy[T any](o Operand[T], n int64) NullsWinFn[qt.Null[T]] {
	return nullsCall[qt.Null[T]]("lead", o.exprNode(), argNode{n})
}

// LeadDefault looks n rows ahead and substitutes def when there is none.
func LeadDefault[T any](o Operand[T], n int64, def T) NullsWinFn[T] {
	return nullsCall[T]("lead", o.exprNode(), argNode{n}, argNode{def})
}

// FirstValue is first_value(expr).
func FirstValue[T any](o Operand[T]) NullsWinFn[qt.Null[T]] {
	return nullsCall[qt.Null[T]]("first_value", o.exprNode())
}

// FirstValueN is FirstValue over a nullable operand.
func FirstValueN[T any](o Operand[qt.Null[T]]) NullsWinFn[qt.Null[T]] {
	return nullsCall[qt.Null[T]]("first_value", o.exprNode())
}

// LastValue is last_value(expr).
func LastValue[T any](o Operand[T]) NullsWinFn[qt.Null[T]] {
	return nullsCall[qt.Null[T]]("last_value", o.exprNode())
}

// LastValueN is LastValue over a nullable operand.
func LastValueN[T any](o Operand[qt.Null[T]]) NullsWinFn[qt.Null[T]] {
	return nullsCall[qt.Null[T]]("last_value", o.exprNode())
}

// NthValue is nth_value(expr, n).
func NthValue[T any](o Operand[T], n int32) NullsWinFn[qt.Null[T]] {
	return nullsCall[qt.Null[T]]("nth_value", o.exprNode(), argNode{n})
}

// NthValueN is NthValue over a nullable operand.
func NthValueN[T any](o Operand[qt.Null[T]], n int32) NullsWinFn[qt.Null[T]] {
	return nullsCall[qt.Null[T]]("nth_value", o.exprNode(), argNode{n})
}

// AggCall is a generated catalog aggregate: schema.name(args...), usable with
// Filter, Distinct, OrderBy and Over like the hand-written ones. min and
// until bound the PostgreSQL majors it exists in (0: no bound).
func AggCall[T any](min, until int, schema, name string, args ...Value) AggExpr[T] {
	return AggExpr[T]{aggNode{name: name, schema: schema, args: nodesOf(args), min: min, until: until}}
}
