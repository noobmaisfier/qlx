package qlx

import (
	"fmt"
	"strings"
	"time"

	"github.com/noobmaisfier/qlx/qt"
)

// ---------------------------------------------------------------------------
// Tables and columns
// ---------------------------------------------------------------------------

// TableRef names a table. Generated code embeds it in each table symbol.
// Required lists columns an INSERT must set (NOT NULL without default).
type TableRef struct {
	Schema, Name, Alias string
	Required            []string
	PK                  []string // primary key columns, when known; enables the GROUP BY check
}

func (t TableRef) key() string {
	if t.Alias != "" {
		return t.Alias
	}
	return t.Name
}

func (t TableRef) scopeKey() string { return t.key() }

// Table is implemented by every generated table symbol (it embeds TableRef).
type Table interface{ tableRef() TableRef }

func (t TableRef) tableRef() TableRef { return t }

func (t TableRef) renderFrom(b *builder) {
	if t.Schema != "" {
		b.ident(t.Schema)
		b.write(".")
	}
	b.ident(t.Name)
	if t.Alias != "" {
		b.write(" AS ")
		b.ident(t.Alias)
	}
}

// FromItem is a table (or later: view, subquery) usable after FROM or JOIN.
type FromItem interface {
	renderFrom(b *builder)
	scopeKey() string
}

type colRef struct {
	t    TableRef
	name string
}

func (c colRef) render(b *builder) {
	if !b.inScope(c.t.key()) {
		b.fail(fmt.Errorf("%w: column %s.%s; add table %q with From or Join", ErrScope, c.t.key(), c.name, c.t.key()))
		return
	}
	if b.colHook != nil && b.aggDepth == 0 {
		b.colHook(c)
	}
	b.ident(c.t.key())
	b.write(".")
	b.ident(c.name)
}

// ColumnRef is implemented by every generated column type.
type ColumnRef interface{ columnRef() colRef }

// ---------------------------------------------------------------------------
// Closed-set AST nodes
// ---------------------------------------------------------------------------

type argNode struct{ v any }

func (n argNode) render(b *builder) { b.arg(n.v) }

type defaultNode struct{}

func (defaultNode) render(b *builder) { b.write("DEFAULT") }

type nullNode struct{}

func (nullNode) render(b *builder) { b.write("NULL") }

type falseNode struct{}

func (falseNode) render(b *builder) { b.write("FALSE") }

type excludedNode struct{ name string }

func (n excludedNode) render(b *builder) {
	b.write("EXCLUDED.")
	b.ident(n.name)
}

type oldNewNode struct{ prefix, name string }

func (n oldNewNode) render(b *builder) {
	b.need(PG18, "RETURNING OLD/NEW")
	b.write(n.prefix)
	b.write(".")
	b.ident(n.name)
}

// cmpNode covers comparison and arithmetic. op always comes from a constant
// in this package, never from a caller.
type cmpNode struct {
	l, r  node
	op    string
	arith bool
}

func wrapIfCompound(b *builder, n node) {
	switch c := n.(type) {
	case cmpNode:
		if c.arith {
			b.write("(")
			c.render(b)
			b.write(")")
			return
		}
	case logicNode:
		b.write("(")
		c.render(b)
		b.write(")")
		return
	}
	n.render(b)
}

func (n cmpNode) render(b *builder) {
	wrapIfCompound(b, n.l)
	b.write(" " + n.op + " ")
	wrapIfCompound(b, n.r)
}

type isNullNode struct {
	n   node
	not bool
}

func (n isNullNode) render(b *builder) {
	wrapIfCompound(b, n.n)
	if n.not {
		b.write(" IS NOT NULL")
	} else {
		b.write(" IS NULL")
	}
}

type logicNode struct {
	op    string // "AND" or "OR"
	parts []node
}

func (n logicNode) render(b *builder) { renderLogic(b, n.op, n.parts) }

// renderLogic writes parts joined by op; a nested node of the other operator
// is parenthesised.
func renderLogic(b *builder, op string, parts []node) {
	for i, p := range parts {
		if i > 0 {
			b.write(" ")
			b.write(op)
			b.write(" ")
		}
		renderLogicPart(b, op, p)
	}
}

func renderLogicPart(b *builder, op string, p node) {
	if l, ok := p.(logicNode); ok && l.op != op {
		b.write("(")
		l.render(b)
		b.write(")")
	} else {
		p.render(b)
	}
}

// renderAndBegin reports whether And(ps...) would be a present predicate.
func renderAndBegin(ps []Pred) bool {
	for _, p := range ps {
		if p.valid() {
			return true
		}
	}
	return false
}

// renderAnd writes And(ps...) without building the combined node: the same
// text, no allocation. A single present predicate is written as it is.
func renderAnd(b *builder, ps []Pred) {
	n := 0
	var only node
	for _, p := range ps {
		if p.valid() {
			n++
			only = p.n
		}
	}
	if n == 1 {
		only.render(b)
		return
	}
	first := true
	for _, p := range ps {
		if !p.valid() {
			continue
		}
		if !first {
			b.write(" AND ")
		}
		first = false
		renderLogicPart(b, "AND", p.n)
	}
}

type notNode struct{ n node }

func (n notNode) render(b *builder) {
	b.write("NOT (")
	n.n.render(b)
	b.write(")")
}

type anyNode struct {
	l   node
	arr any
}

func (n anyNode) render(b *builder) {
	wrapIfCompound(b, n.l)
	b.write(" = ANY(")
	b.arg(n.arr)
	b.write(")")
}

type notAllNode struct {
	l   node
	arr any
}

func (n notAllNode) render(b *builder) {
	wrapIfCompound(b, n.l)
	b.write(" <> ALL(")
	b.arg(n.arr)
	b.write(")")
}

type likeNode struct {
	l       node
	op      string // "LIKE" or "ILIKE"
	pattern string
}

func (n likeNode) render(b *builder) {
	wrapIfCompound(b, n.l)
	b.write(" " + n.op + " ")
	b.arg(n.pattern)
}

type fnNode struct {
	name string // constant function name from this package
	args []node
}

func (n fnNode) render(b *builder) {
	b.write(n.name)
	b.write("(")
	for i, a := range n.args {
		if i > 0 {
			b.write(", ")
		}
		a.render(b)
	}
	b.write(")")
}

type existsNode struct{ q Query }

func (n existsNode) render(b *builder) {
	b.write("EXISTS (")
	n.q.render(b)
	b.write(")")
}

type inSelectNode struct {
	l node
	q Query
}

func (n inSelectNode) render(b *builder) {
	wrapIfCompound(b, n.l)
	b.write(" IN (")
	n.q.render(b)
	b.write(")")
}

// ---------------------------------------------------------------------------
// Operands, expressions, predicates
// ---------------------------------------------------------------------------

// Operand is a typed value expression: a column, an expression or a Param.
// Aggregates are deliberately not Operands, so they cannot be used where
// PostgreSQL forbids them (WHERE, JOIN conditions, arguments of aggregates).
type Operand[T any] interface {
	exprNode() node
	typ() T
}

// Value is any expression, whatever its type; used by GROUP BY and DISTINCT ON.
type Value interface{ valueNode() node }

// Selectable can appear in a SELECT list.
type Selectable interface{ selectNode() selectItem }

// ReturningItem can appear in a RETURNING list. It is a superset of
// Selectable: OLD and NEW references are accepted here and nowhere else.
type ReturningItem interface{ returningNode() selectItem }

type selectItem struct {
	n     node
	alias string
}

// name is the output column name: the alias, or the column's own name.
func (s selectItem) name() string {
	if s.alias != "" {
		return s.alias
	}
	if c, ok := s.n.(colRef); ok {
		return c.name
	}
	return ""
}

func (s selectItem) render(b *builder) {
	s.n.render(b)
	if s.alias != "" {
		b.write(" AS ")
		b.ident(s.alias)
	}
}

// Expr is a typed scalar expression. T is the Go type of the value; a
// nullable expression has T = qt.Null[X]. Derived expressions follow the
// "safe by default" rule: they are typed nullable unless they cannot be NULL.
type Expr[T any] struct{ n node }

func (e Expr[T]) exprNode() node            { return e.n }
func (Expr[T]) typ() (z T)                  { return }
func (e Expr[T]) valueNode() node           { return e.n }
func (e Expr[T]) selectNode() selectItem    { return selectItem{n: e.n} }
func (e Expr[T]) returningNode() selectItem { return selectItem{n: e.n} }
func (e Expr[T]) As(alias string) Named[T]  { return Named[T]{selectItem{e.n, alias}} }
func (e Expr[T]) outName() string {
	if c, ok := e.n.(colRef); ok {
		return c.name
	}
	return ""
}
func (e Expr[T]) Eq(v T) Pred           { return Pred{cmpNode{l: e.n, op: "=", r: argNode{v}}} }
func (e Expr[T]) EqX(o Operand[T]) Pred { return Pred{cmpNode{l: e.n, op: "=", r: o.exprNode()}} }
func (e Expr[T]) Ne(v T) Pred           { return Pred{cmpNode{l: e.n, op: "<>", r: argNode{v}}} }
func (e Expr[T]) NeX(o Operand[T]) Pred { return Pred{cmpNode{l: e.n, op: "<>", r: o.exprNode()}} }
func (e Expr[T]) IsNull() Pred          { return Pred{isNullNode{n: e.n}} }
func (e Expr[T]) IsNotNull() Pred       { return Pred{isNullNode{n: e.n, not: true}} }
func (e Expr[T]) Asc() OrderTerm        { return OrderTerm{n: e.n} }
func (e Expr[T]) Desc() OrderTerm       { return OrderTerm{n: e.n, desc: true} }

// In renders "= ANY($1)" with one array parameter, so the statement shape
// does not change with the number of values.
//
// For a nullable element type (qt.Null[T]) a None element means NULL and is
// matched with IS NULL: In(Some("a"), None) is (col = ANY($1) OR col IS NULL).
// SQL's own "= NULL" never matches, so passing the NULL inside the array would
// silently find nothing.
func (e Expr[T]) In(vs ...T) Pred {
	if len(vs) == 0 {
		return Pred{falseNode{}}
	}
	if keep, ok := splitNullElems(vs); ok {
		isNull := Pred{isNullNode{n: e.n}}
		if len(keep) == 0 {
			return isNull
		}
		return Or(Pred{anyNode{l: e.n, arr: keep}}, isNull)
	}
	return Pred{anyNode{l: e.n, arr: vs}}
}

// splitNullElems separates the NULL elements of a slice of nullable values
// (qt.Null[T], or any type that binds as an argument and can bind as nil).
// ok is false when there is nothing to separate: the elements are not of a
// nullable type, or none is NULL.
func splitNullElems[T any](vs []T) (keep []T, ok bool) {
	if _, nullable := any(vs[0]).(argValuer); !nullable {
		return nil, false
	}
	nulls := 0
	for _, v := range vs {
		if normalizeArg(v) == nil {
			nulls++
		} else {
			keep = append(keep, v)
		}
	}
	return keep, nulls > 0
}

// Any is In for a slice you already hold: = ANY($1) with the slice as the one
// array parameter. An empty slice is constant false, as with In.
func (e Expr[T]) Any(vs []T) Pred { return e.In(vs...) }

// NotAll renders "<> ALL($1)": true when the value differs from every element.
// An empty slice is TRUE, not an absent predicate: "differs from every one of
// none" holds, and the statement shape stays explicit.
//
// For a nullable element type a None element means NULL: the value must also
// not be NULL. NotAll([]qt.Null[string]{Some("a"), None}) is
// (col <> ALL($1) AND col IS NOT NULL); with SQL's own rules a NULL inside the
// list would make the predicate never true.
func (e Expr[T]) NotAll(vs []T) Pred {
	if len(vs) == 0 {
		return Pred{trueNode{}}
	}
	if keep, ok := splitNullElems(vs); ok {
		notNull := Pred{isNullNode{n: e.n, not: true}}
		if len(keep) == 0 {
			return notNull
		}
		return And(Pred{notAllNode{l: e.n, arr: keep}}, notNull)
	}
	return Pred{notAllNode{l: e.n, arr: vs}}
}

// EqIf drops the predicate when the option is unset.
func (e Expr[T]) EqIf(o qt.Opt[T]) Pred {
	if !o.Set {
		return Pred{}
	}
	return e.Eq(o.V)
}

// Named is a typed expression with a result name. Because it keeps its type,
// it can later be referenced from a subquery or CTE with Ref, which is how
// derived tables get typed columns without any string typing.
type Named[T any] struct{ it selectItem }

func (a Named[T]) selectNode() selectItem    { return a.it }
func (a Named[T]) returningNode() selectItem { return a.it }
func (a Named[T]) outName() string           { return a.it.alias }
func (Named[T]) typ() (z T)                  { return }

// ReturningExpr is an OLD or NEW reference (PostgreSQL 18). Only RETURNING
// accepts it, so using one in WHERE or SELECT does not compile.
type ReturningExpr[T any] struct{ n node }

func (r ReturningExpr[T]) returningNode() selectItem { return selectItem{n: r.n} }
func (r ReturningExpr[T]) As(alias string) NamedReturning[T] {
	return NamedReturning[T]{selectItem{r.n, alias}}
}

// NamedReturning is a named OLD or NEW reference.
type NamedReturning[T any] struct{ it selectItem }

func (a NamedReturning[T]) returningNode() selectItem { return a.it }

// Pred is a boolean condition usable in WHERE, ON and HAVING. The zero Pred
// is "absent" and is dropped, which is how dynamic filters work.
type Pred struct{ n node }

func (p Pred) valid() bool      { return p.n != nil }
func (p Pred) havingNode() node { return p.n }
func (p Pred) And(q Pred) Pred  { return And(p, q) }
func (p Pred) Or(q Pred) Pred   { return Or(p, q) }

// HavingCond is accepted by HAVING: ordinary predicates and aggregate ones.
type HavingCond interface{ havingNode() node }

// AggPred is a condition over an aggregate. It is accepted by HAVING only.
type AggPred struct{ n node }

func (p AggPred) havingNode() node { return p.n }

func combine(op string, ps []Pred) Pred {
	var parts []node
	for _, p := range ps {
		if p.valid() {
			parts = append(parts, p.n)
		}
	}
	switch len(parts) {
	case 0:
		return Pred{}
	case 1:
		return Pred{parts[0]}
	}
	return Pred{logicNode{op: op, parts: parts}}
}

// And joins predicates; absent ones are skipped.
func And(ps ...Pred) Pred { return combine("AND", ps) }

// Or joins predicates; absent ones are skipped.
func Or(ps ...Pred) Pred { return combine("OR", ps) }

// Not negates a predicate. Not of an absent predicate is absent.
func Not(p Pred) Pred {
	if !p.valid() {
		return Pred{}
	}
	return Pred{notNode{p.n}}
}

// If keeps p only when cond is true.
func If(cond bool, p Pred) Pred {
	if cond {
		return p
	}
	return Pred{}
}

// Exists renders EXISTS (subquery). The subquery may refer to outer tables.
func Exists(q Query) Pred { return Pred{existsNode{q}} }

// InSelect renders "expr IN (subquery)".
func InSelect[T any](e Expr[T], q Query) Pred { return Pred{inSelectNode{l: e.n, q: q}} }

// ---------------------------------------------------------------------------
// Ordering
// ---------------------------------------------------------------------------

// OrderTerm is one ORDER BY item.
type OrderTerm struct {
	n     node
	desc  bool
	nulls string
}

// Asc and Desc set the direction explicitly (ByName returns an OrderTerm).
func (o OrderTerm) Asc() OrderTerm  { o.desc = false; return o }
func (o OrderTerm) Desc() OrderTerm { o.desc = true; return o }

// NullsFirst places NULLs first.
func (o OrderTerm) NullsFirst() OrderTerm { o.nulls = " NULLS FIRST"; return o }

// NullsLast places NULLs last.
func (o OrderTerm) NullsLast() OrderTerm { o.nulls = " NULLS LAST"; return o }

func (o OrderTerm) render(b *builder) {
	o.n.render(b)
	if o.desc {
		b.write(" DESC")
	}
	b.write(o.nulls)
}

// ---------------------------------------------------------------------------
// Column families
// ---------------------------------------------------------------------------

// Assignment sets a column in INSERT, UPDATE or ON CONFLICT DO UPDATE.
type Assignment struct {
	col  colRef
	val  node
	skip bool // from ToIf with an unset option: left out when the statement is built
}

type colOps[T any] struct{ ref colRef }

func (c colOps[T]) columnRef() colRef           { return c.ref }
func (c colOps[T]) To(v T) Assignment           { return Assignment{c.ref, argNode{v}, false} }
func (c colOps[T]) ToX(o Operand[T]) Assignment { return Assignment{c.ref, o.exprNode(), false} }
func (c colOps[T]) ToDefault() Assignment       { return Assignment{c.ref, defaultNode{}, false} }

// ToNull assigns NULL: `SET nickname = NULL`, or a NULL in an INSERT row. It
// is the short form of To(qt.None[T]()) and also covers the nullable columns
// that are not a qt.Null: an array, a bytea, a range. Go cannot reserve a
// method for some type arguments, so ToNull compiles on a NOT NULL column too;
// the server then answers 23502 at run time, and the analyzer reports it
// beforehand (V030).
func (c colOps[T]) ToNull() Assignment { return Assignment{c.ref, nullNode{}, false} }

// ToIf assigns the value of an option and leaves the assignment out when the
// option is unset: no column in an INSERT (the column takes its DEFAULT), no
// SET in an UPDATE (the column is not touched). It is the inline form of a
// generated patch, for a body whose fields may be absent:
//
//	q.InsertInto(Users).Row(Users.Email.To(r.Email), Users.Tier.ToIf(r.Tier))
//	q.Update(Users).Set(Users.Email.ToIf(p.Email), Users.Nickname.ToIf(p.Nickname))
//
// With a nullable column, the option holds a qt.Null: Opt[Null[string]] is
// "absent", "NULL" or "a value". Every combination of present columns is a
// different statement shape, and the shape cache stores each one: with n
// optional columns there can be 2^n shapes (watch ShapeCache.OnThrash). A
// required column (NOT NULL without default) assigned with ToIf can disappear;
// the renderer reports it as it does for a missing column, and the analyzer
// (V008) reports it. If every assignment of an UPDATE is left out, the
// statement has no SET and does not render. In a multi-row INSERT every row
// must keep the same columns, so rows that differ in which options are set
// do not render: use ToOrDefault.
func (c colOps[T]) ToIf(o qt.Opt[T]) Assignment {
	return Assignment{c.ref, argNode{o.V}, !o.Set}
}

// ToOrDefault assigns the value of an option, or DEFAULT when it is unset. It
// is for INSERT: the column is always in the list, so the rows of a multi-row
// insert keep the same columns whatever each row's options hold (the SQL text
// still differs between a value and DEFAULT, so the shape count is the same as
// with ToIf). In an UPDATE it would reset the column to its default, which is
// almost never what an absent field means; use ToIf there (the analyzer
// reports ToOrDefault in a SET, V027).
func (c colOps[T]) ToOrDefault(o qt.Opt[T]) Assignment {
	if !o.Set {
		return Assignment{c.ref, defaultNode{}, false}
	}
	return Assignment{c.ref, argNode{o.V}, false}
}

// live drops the assignments that ToIf left out. The slice is returned as it
// is when nothing was dropped.
func live(as []Assignment) []Assignment {
	n := 0
	for _, a := range as {
		if a.skip {
			n++
		}
	}
	if n == 0 {
		return as
	}
	out := make([]Assignment, 0, len(as)-n)
	for _, a := range as {
		if !a.skip {
			out = append(out, a)
		}
	}
	return out
}

func (c colOps[T]) Excluded() Expr[T] { return Expr[T]{excludedNode{c.ref.name}} }

// Old: Requires PostgreSQL 18.
func (c colOps[T]) Old() ReturningExpr[T] { return ReturningExpr[T]{oldNewNode{"old", c.ref.name}} }

// New: Requires PostgreSQL 18.
func (c colOps[T]) New() ReturningExpr[T] { return ReturningExpr[T]{oldNewNode{"new", c.ref.name}} }

// Col is a column of any type with equality, null tests, ordering and
// assignment.
type Col[T any] struct {
	Expr[T]
	colOps[T]
}

// NewCol is used by generated code.
func NewCol[T any](t TableRef, name string) Col[T] {
	r := colRef{t, name}
	return Col[T]{Expr[T]{r}, colOps[T]{r}}
}

// Num is the set of numeric Go types, nullable or not.
type Num interface {
	~int16 | ~int32 | ~int64 | ~float32 | ~float64 |
		qt.Null[int16] | qt.Null[int32] | qt.Null[int64] | qt.Null[float32] | qt.Null[float64]
}

// NumBase is the non-null numeric types.
type NumBase interface {
	~int16 | ~int32 | ~int64 | ~float32 | ~float64
}

// Text is the set of text Go types, nullable or not.
type Text interface{ ~string | qt.Null[string] }

// TimeT is the set of timestamp Go types, nullable or not.
type TimeT interface {
	time.Time | qt.LocalTime | qt.Timestamptz | qt.Date | qt.TimeOfDay | qt.TimeTZ |
		qt.Null[time.Time] | qt.Null[qt.LocalTime] | qt.Null[qt.Timestamptz] | qt.Null[qt.Date] | qt.Null[qt.TimeOfDay] | qt.Null[qt.TimeTZ]
}

func ordCmp(l node, op string, r node) Pred { return Pred{cmpNode{l: l, op: op, r: r}} }

// NumExpr is a numeric expression.
type NumExpr[T Num] struct{ Expr[T] }

func (e NumExpr[T]) Gt(v T) Pred           { return ordCmp(e.n, ">", argNode{v}) }
func (e NumExpr[T]) GtX(o Operand[T]) Pred { return ordCmp(e.n, ">", o.exprNode()) }
func (e NumExpr[T]) Gte(v T) Pred          { return ordCmp(e.n, ">=", argNode{v}) }
func (e NumExpr[T]) Lt(v T) Pred           { return ordCmp(e.n, "<", argNode{v}) }
func (e NumExpr[T]) LtX(o Operand[T]) Pred { return ordCmp(e.n, "<", o.exprNode()) }
func (e NumExpr[T]) Lte(v T) Pred          { return ordCmp(e.n, "<=", argNode{v}) }
func (e NumExpr[T]) Between(lo, hi T) Pred {
	return And(e.Gte(lo), e.Lte(hi))
}
func (e NumExpr[T]) arith(op string, r node) NumExpr[T] {
	return NumExpr[T]{Expr[T]{cmpNode{l: e.n, op: op, r: r, arith: true}}}
}
func (e NumExpr[T]) Add(v T) NumExpr[T]           { return e.arith("+", argNode{v}) }
func (e NumExpr[T]) AddX(o Operand[T]) NumExpr[T] { return e.arith("+", o.exprNode()) }
func (e NumExpr[T]) Sub(v T) NumExpr[T]           { return e.arith("-", argNode{v}) }
func (e NumExpr[T]) SubX(o Operand[T]) NumExpr[T] { return e.arith("-", o.exprNode()) }
func (e NumExpr[T]) Mul(v T) NumExpr[T]           { return e.arith("*", argNode{v}) }
func (e NumExpr[T]) Div(v T) NumExpr[T]           { return e.arith("/", argNode{v}) }

// NumCol is a numeric column.
type NumCol[T Num] struct {
	NumExpr[T]
	colOps[T]
}

// NewNumCol is used by generated code.
func NewNumCol[T Num](t TableRef, name string) NumCol[T] {
	r := colRef{t, name}
	return NumCol[T]{NumExpr[T]{Expr[T]{r}}, colOps[T]{r}}
}

// TextExpr is a text expression.
type TextExpr[T Text] struct{ Expr[T] }

func (e TextExpr[T]) Gt(v T) Pred  { return ordCmp(e.n, ">", argNode{v}) }
func (e TextExpr[T]) Gte(v T) Pred { return ordCmp(e.n, ">=", argNode{v}) }
func (e TextExpr[T]) Lt(v T) Pred  { return ordCmp(e.n, "<", argNode{v}) }
func (e TextExpr[T]) Lte(v T) Pred { return ordCmp(e.n, "<=", argNode{v}) }

// Like matches a pattern the caller wrote on purpose, metacharacters included.
func (e TextExpr[T]) Like(pattern string) Pred  { return Pred{likeNode{e.n, "LIKE", pattern}} }
func (e TextExpr[T]) ILike(pattern string) Pred { return Pred{likeNode{e.n, "ILIKE", pattern}} }

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// Contains matches s as a literal substring: %, _ and \ in s are escaped.
func (e TextExpr[T]) Contains(s string) Pred {
	return Pred{likeNode{e.n, "LIKE", "%" + likeEscaper.Replace(s) + "%"}}
}

// ContainsFold is Contains, ignoring case.
func (e TextExpr[T]) ContainsFold(s string) Pred {
	return Pred{likeNode{e.n, "ILIKE", "%" + likeEscaper.Replace(s) + "%"}}
}

// HasPrefix matches s as a literal prefix.
func (e TextExpr[T]) HasPrefix(s string) Pred {
	return Pred{likeNode{e.n, "LIKE", likeEscaper.Replace(s) + "%"}}
}

func (e TextExpr[T]) Lower() TextExpr[T] {
	return TextExpr[T]{Expr[T]{fnNode{"lower", []node{e.n}}}}
}
func (e TextExpr[T]) Upper() TextExpr[T] {
	return TextExpr[T]{Expr[T]{fnNode{"upper", []node{e.n}}}}
}

// TextCol is a text column.
type TextCol[T Text] struct {
	TextExpr[T]
	colOps[T]
}

// NewTextCol is used by generated code.
func NewTextCol[T Text](t TableRef, name string) TextCol[T] {
	r := colRef{t, name}
	return TextCol[T]{TextExpr[T]{Expr[T]{r}}, colOps[T]{r}}
}

// TimeExpr is a timestamp expression.
type TimeExpr[T TimeT] struct{ Expr[T] }

func (e TimeExpr[T]) Gt(v T) Pred  { return ordCmp(e.n, ">", argNode{v}) }
func (e TimeExpr[T]) Gte(v T) Pred { return ordCmp(e.n, ">=", argNode{v}) }
func (e TimeExpr[T]) Lt(v T) Pred  { return ordCmp(e.n, "<", argNode{v}) }
func (e TimeExpr[T]) Lte(v T) Pred { return ordCmp(e.n, "<=", argNode{v}) }

// GtX, GteX, LtX and LteX compare with another expression of the same type,
// such as a column or Now().
func (e TimeExpr[T]) GtX(o Operand[T]) Pred  { return ordCmp(e.n, ">", o.exprNode()) }
func (e TimeExpr[T]) GteX(o Operand[T]) Pred { return ordCmp(e.n, ">=", o.exprNode()) }
func (e TimeExpr[T]) LtX(o Operand[T]) Pred  { return ordCmp(e.n, "<", o.exprNode()) }
func (e TimeExpr[T]) LteX(o Operand[T]) Pred { return ordCmp(e.n, "<=", o.exprNode()) }
func (e TimeExpr[T]) Between(lo, hi T) Pred {
	return And(e.Gte(lo), e.Lte(hi))
}

// TimeCol is a timestamp column.
type TimeCol[T TimeT] struct {
	TimeExpr[T]
	colOps[T]
}

// NewTimeCol is used by generated code.
func NewTimeCol[T TimeT](t TableRef, name string) TimeCol[T] {
	r := colRef{t, name}
	return TimeCol[T]{TimeExpr[T]{Expr[T]{r}}, colOps[T]{r}}
}

// NowTZ is now() typed as qt.Timestamptz, for columns generated with
// `timestamptz: qt`: valid_until > NowTZ() compares like for like.
func NowTZ() TimeExpr[qt.Timestamptz] {
	return TimeExpr[qt.Timestamptz]{Expr[qt.Timestamptz]{fnNode{"now", nil}}}
}

// Now is now().
func Now() TimeExpr[time.Time] { return TimeExpr[time.Time]{Expr[time.Time]{fnNode{"now", nil}}} }
