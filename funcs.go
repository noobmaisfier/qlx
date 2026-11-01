package qlx

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"time"
	"uuid"

	"github.com/noobmaisfier/qlx/qt"
)

// ---------------------------------------------------------------------------
// Small nodes
// ---------------------------------------------------------------------------

type trueNode struct{}

func (trueNode) render(b *builder) { b.write("TRUE") }

// True and False are constant predicates (for example the ON condition of a
// LATERAL join).
var (
	True  = Pred{trueNode{}}
	False = Pred{falseNode{}}
)

func (a Named[T]) valueNode() node { return a.it.n }

type typedArgNode struct{ v any }

func (n typedArgNode) render(b *builder) { b.typedArg(n.v) }

// Val binds a Go value as a typed expression, casting the placeholder to the
// value's PostgreSQL type so the server never has to guess it.
func Val[T any](v T) Expr[T] { return Expr[T]{typedArgNode{v}} }

type castedArgNode struct {
	v   any
	typ string // constant type name from this package
}

func (n castedArgNode) render(b *builder) {
	b.arg(n.v)
	b.write("::" + n.typ)
}

// ---------------------------------------------------------------------------
// Boolean expressions
// ---------------------------------------------------------------------------

// BoolExpr is a boolean value expression.
type BoolExpr struct{ Expr[bool] }

// Test turns a predicate into a boolean value, for SELECT lists.
func Test(p Pred) BoolExpr {
	if !p.valid() {
		return BoolExpr{Expr[bool]{failNode{fmt.Errorf("%w: absent predicate used as a value", ErrShape)}}}
	}
	return BoolExpr{Expr[bool]{p.n}}
}

// Pred uses the boolean as a condition.
func (e BoolExpr) Pred() Pred      { return Pred{e.n} }
func (e BoolExpr) IsTrue() Pred    { return Pred{boolTestNode{e.n, "IS TRUE"}} }
func (e BoolExpr) IsFalse() Pred   { return Pred{boolTestNode{e.n, "IS FALSE"}} }
func (e BoolExpr) IsUnknown() Pred { return Pred{boolTestNode{e.n, "IS UNKNOWN"}} }
func (e BoolExpr) Not() BoolExpr   { return BoolExpr{Expr[bool]{notNode{e.n}}} }

type boolTestNode struct {
	n  node
	op string
}

func (n boolTestNode) render(b *builder) {
	wrapIfCompound(b, n.n)
	b.write(" " + n.op)
}

// BoolCol is a boolean column.
type BoolCol struct {
	BoolExpr
	colOps[bool]
}

// NewBoolCol is used by generated code.
func NewBoolCol(t TableRef, name string) BoolCol {
	r := colRef{t, name}
	return BoolCol{BoolExpr{Expr[bool]{r}}, colOps[bool]{r}}
}

// ---------------------------------------------------------------------------
// IS DISTINCT FROM, NOT IN
// ---------------------------------------------------------------------------

// IsDistinctFrom is a NULL-safe inequality.
func (e Expr[T]) IsDistinctFrom(v T) Pred {
	return Pred{cmpNode{l: e.n, op: "IS DISTINCT FROM", r: argNode{v}}}
}

// IsNotDistinctFrom is a NULL-safe equality.
func (e Expr[T]) IsNotDistinctFrom(v T) Pred {
	return Pred{cmpNode{l: e.n, op: "IS NOT DISTINCT FROM", r: argNode{v}}}
}

type notInNode struct {
	l   node
	arr any
}

func (n notInNode) render(b *builder) {
	wrapIfCompound(b, n.l)
	b.write(" <> ALL(")
	b.arg(n.arr)
	b.write(")")
}

// NotIn renders "<> ALL($1)" with one array parameter. An empty list is TRUE.
func (e Expr[T]) NotIn(vs ...T) Pred {
	if len(vs) == 0 {
		return True
	}
	return Pred{notInNode{l: e.n, arr: vs}}
}

// ---------------------------------------------------------------------------
// CASE
// ---------------------------------------------------------------------------

type caseWhen struct{ cond, then node }

type caseNode struct {
	whens []caseWhen
	els   node
}

func (n caseNode) render(b *builder) {
	if len(n.whens) == 0 {
		b.fail(fmt.Errorf("%w: CASE needs at least one When", ErrShape))
		return
	}
	b.write("CASE")
	for _, w := range n.whens {
		if w.cond == nil {
			b.fail(fmt.Errorf("%w: CASE branch has an absent condition", ErrShape))
			return
		}
		b.write(" WHEN ")
		w.cond.render(b)
		b.write(" THEN ")
		w.then.render(b)
	}
	if n.els != nil {
		b.write(" ELSE ")
		n.els.render(b)
	}
	b.write(" END")
}

// CaseBuilder builds CASE WHEN ... THEN ... END with a typed result.
type CaseBuilder[T any] struct{ n caseNode }

// Case starts a CASE expression whose branches all have type T.
func Case[T any]() CaseBuilder[T] { return CaseBuilder[T]{} }

func (c CaseBuilder[T]) add(p Pred, then node) CaseBuilder[T] {
	c.n.whens = app(c.n.whens, caseWhen{cond: p.n, then: then})
	return c
}

// When adds a branch with a constant result.
func (c CaseBuilder[T]) When(p Pred, v T) CaseBuilder[T] { return c.add(p, typedArgNode{v}) }

// WhenX adds a branch with an expression result.
func (c CaseBuilder[T]) WhenX(p Pred, o Operand[T]) CaseBuilder[T] { return c.add(p, o.exprNode()) }

// Else ends the CASE with a default value.
func (c CaseBuilder[T]) Else(v T) Expr[T] { c.n.els = typedArgNode{v}; return Expr[T]{c.n} }

// ElseX ends the CASE with a default expression.
func (c CaseBuilder[T]) ElseX(o Operand[T]) Expr[T] { c.n.els = o.exprNode(); return Expr[T]{c.n} }

// End ends the CASE without ELSE; the result is NULL when no branch matches.
func (c CaseBuilder[T]) End() Expr[qt.Null[T]] { return Expr[qt.Null[T]]{c.n} }

// ---------------------------------------------------------------------------
// Casts and type symbols
// ---------------------------------------------------------------------------

// PgType is a PostgreSQL type tied to the Go type it maps to, so a cast
// changes the Go type of the expression in step with the SQL type.
type PgType[T any] struct {
	builtin      string
	schema, name string
	array        bool
}

// Built-in types.
var (
	PgInt2        = PgType[int16]{builtin: "int2"}
	PgInt4        = PgType[int32]{builtin: "int4"}
	PgInt8        = PgType[int64]{builtin: "int8"}
	PgFloat4      = PgType[float32]{builtin: "float4"}
	PgFloat8      = PgType[float64]{builtin: "float8"}
	PgNumeric     = PgType[qt.Numeric]{builtin: "numeric"}
	PgText        = PgType[string]{builtin: "text"}
	PgBool        = PgType[bool]{builtin: "bool"}
	PgBytea       = PgType[[]byte]{builtin: "bytea"}
	PgUUID        = PgType[uuid.UUID]{builtin: "uuid"}
	PgTimestamptz = PgType[time.Time]{builtin: "timestamptz"}
	PgTimestamp   = PgType[qt.LocalTime]{builtin: "timestamp"}
	PgDate        = PgType[qt.Date]{builtin: "date"}
	PgXML         = PgType[qt.XML]{builtin: "xml"}
	PgInterval    = PgType[qt.Interval]{builtin: "interval"}
	PgJSONB       = PgType[qt.RawJSON]{builtin: "jsonb"}
	PgJSON        = PgType[qt.RawJSON]{builtin: "json"}
	PgInet        = PgType[netip.Addr]{builtin: "inet"}
)

// NewPgType names a user-defined type (enum, composite, domain). Generated
// code uses it; the names are validated and quoted when rendered.
func NewPgType[T any](schema, name string) PgType[T] { return PgType[T]{schema: schema, name: name} }

// ArrayOf is the array type of t. (A function, because a method returning
// PgType[[]T] would instantiate itself forever.)
func ArrayOf[T any](t PgType[T]) PgType[[]T] {
	return PgType[[]T]{builtin: t.builtin, schema: t.schema, name: t.name, array: true}
}

type typeRef interface{ renderType(b *builder) }

func (t PgType[T]) renderType(b *builder) {
	if t.builtin != "" {
		b.write(t.builtin)
	} else {
		if t.schema != "" {
			b.ident(t.schema)
			b.write(".")
		}
		b.ident(t.name)
	}
	if t.array {
		b.write("[]")
	}
}

type castNode struct {
	n node
	t typeRef
}

func (n castNode) render(b *builder) {
	b.write("CAST(")
	n.n.render(b)
	b.write(" AS ")
	n.t.renderType(b)
	b.write(")")
}

// Cast converts a non-null operand: CAST(x AS type).
func Cast[From, To any](o Operand[From], t PgType[To]) Expr[To] {
	return Expr[To]{castNode{o.exprNode(), t}}
}

// CastN converts a nullable operand; the result is nullable too.
func CastN[From, To any](o Operand[From], t PgType[To]) Expr[qt.Null[To]] {
	return Expr[qt.Null[To]]{castNode{o.exprNode(), t}}
}

// ---------------------------------------------------------------------------
// Function calls
// ---------------------------------------------------------------------------

type qualFnNode struct {
	schema, name string
	args         []node
}

func (n qualFnNode) render(b *builder) {
	if n.schema != "" {
		b.ident(n.schema)
		b.write(".")
	}
	b.ident(n.name)
	b.write("(")
	joinParts(b, len(n.args), func(i int) { n.args[i].render(b) })
	b.write(")")
}

// FuncCall calls a user-defined function. Generated code uses it for every
// function found in the catalog; the schema and name are validated and
// quoted, never spliced. T is the Go type of the result.
func FuncCall[T any](schema, name string, args ...Value) Expr[T] {
	return Expr[T]{qualFnNode{schema, name, nodesOf(args)}}
}

type notNullNode struct{ n node }

func (n notNullNode) render(b *builder) {
	if b.version >= PG19 {
		b.write("error_on_null(")
		n.n.render(b)
		b.write(")")
		return
	}
	n.n.render(b)
}

// NotNull asserts that a nullable expression is not NULL. On PostgreSQL 19 it
// renders error_on_null(expr), so a violated assumption fails in SQL with a
// clear message; on 18 the assertion is enforced when the row is scanned.
func NotNull[T any](o Operand[qt.Null[T]]) Expr[T] { return Expr[T]{notNullNode{o.exprNode()}} }

// Nullable types a value as possibly NULL without changing the SQL. Use it
// for a column from the optional side of a LEFT, RIGHT or FULL JOIN: the
// column is declared NOT NULL, but a row without a match yields NULL, and
// reading that into a plain field fails. The result is an Expr[qt.Null[T]], so
// the Go type says what the row can hold:
//
//	q.Select(Users.Email, q.Nullable(Orders.Total).As("total")).
//		From(Users).LeftJoin(Orders).On(Orders.UserID.EqX(Users.ID))
func Nullable[T any](o Operand[T]) Expr[qt.Null[T]] { return Expr[qt.Null[T]]{o.exprNode()} }

// Coalesce returns the first non-NULL of a nullable operand and a fallback.
func Coalesce[T any](a Operand[qt.Null[T]], fallback T) Expr[T] {
	return Expr[T]{fnNode{"coalesce", []node{a.exprNode(), argNode{fallback}}}}
}

// CoalesceOp is coalesce(a, b) over two nullable operands.
func CoalesceOp[T any](a, b Operand[qt.Null[T]]) Expr[qt.Null[T]] {
	return Expr[qt.Null[T]]{fnNode{"coalesce", []node{a.exprNode(), b.exprNode()}}}
}

// CoalesceOpDefault is coalesce(a, b) where b is never NULL.
func CoalesceOpDefault[T any](a Operand[qt.Null[T]], b Operand[T]) Expr[T] {
	return Expr[T]{fnNode{"coalesce", []node{a.exprNode(), b.exprNode()}}}
}

// NullIf is nullif(a, b).
func NullIf[T any](a Operand[T], b T) Expr[qt.Null[T]] {
	return Expr[qt.Null[T]]{fnNode{"nullif", []node{a.exprNode(), argNode{b}}}}
}

// Greatest and Least pick the extreme of non-null operands.
func Greatest[T any](a Operand[T], rest ...Operand[T]) Expr[T] {
	return Expr[T]{fnNode{"greatest", opNodes(a, rest)}}
}
func Least[T any](a Operand[T], rest ...Operand[T]) Expr[T] {
	return Expr[T]{fnNode{"least", opNodes(a, rest)}}
}

func opNodes[T any](a Operand[T], rest []Operand[T]) []node {
	out := []node{a.exprNode()}
	for _, r := range rest {
		out = append(out, r.exprNode())
	}
	return out
}

// ---------------------------------------------------------------------------
// Text functions
// ---------------------------------------------------------------------------

func fn1[T any](name string, e Expr[T]) Expr[T] { return Expr[T]{fnNode{name, []node{e.n}}} }

// Concat appends with the || operator.
func (e TextExpr[T]) Concat(s string) TextExpr[T] {
	return TextExpr[T]{Expr[T]{cmpNode{l: e.n, op: "||", r: argNode{s}, arith: true}}}
}

// ConcatX appends another text expression.
func (e TextExpr[T]) ConcatX(o Operand[T]) TextExpr[T] {
	return TextExpr[T]{Expr[T]{cmpNode{l: e.n, op: "||", r: o.exprNode(), arith: true}}}
}

func (e TextExpr[T]) Trim() TextExpr[T]    { return TextExpr[T]{fn1("btrim", e.Expr)} }
func (e TextExpr[T]) Reverse() TextExpr[T] { return TextExpr[T]{fn1("reverse", e.Expr)} }

// Casefold is casefold(text), new in PostgreSQL 18.
//
// Requires PostgreSQL 18.
func (e TextExpr[T]) Casefold() TextExpr[T] {
	return TextExpr[T]{Expr[T]{versioned{PG18, "casefold", fnNode{"casefold", []node{e.n}}}}}
}

// Replace is replace(text, from, to); from and to are bound parameters.
func (e TextExpr[T]) Replace(from, to string) TextExpr[T] {
	return TextExpr[T]{Expr[T]{fnNode{"replace", []node{e.n, argNode{from}, argNode{to}}}}}
}

// Length is length(text). The result is typed nullable (strict function).
func (e TextExpr[T]) Length() NumExpr[qt.Null[int32]] {
	return NumExpr[qt.Null[int32]]{Expr[qt.Null[int32]]{fnNode{"length", []node{e.n}}}}
}

// StartsWith is the ^@ operator.
func (e TextExpr[T]) StartsWith(prefix string) Pred {
	return Pred{cmpNode{l: e.n, op: "^@", r: argNode{prefix}}}
}

// Regexp is the ~ operator; Regexpi ignores case.
func (e TextExpr[T]) Regexp(pattern string) Pred {
	return Pred{cmpNode{l: e.n, op: "~", r: argNode{pattern}}}
}
func (e TextExpr[T]) Regexpi(pattern string) Pred {
	return Pred{cmpNode{l: e.n, op: "~*", r: argNode{pattern}}}
}

// versioned renders inner only on servers at or above a version.
type versioned struct {
	min   int
	what  string
	inner node
}

func (n versioned) render(b *builder) {
	b.need(n.min, n.what)
	n.inner.render(b)
}

// versionedRange gates a construct to the majors min <= version < until
// (min 0: from the first supported major). Built-ins that a later major
// removed or renamed (pg_read_file_old, interval_avg ...) need the upper bound.
type versionedRange struct {
	min, until int
	what       string
	inner      node
}

func (n versionedRange) render(b *builder) {
	if n.min > 0 {
		b.need(n.min, n.what)
	}
	if n.until > 0 && b.version >= n.until {
		b.fail(fmt.Errorf("%w: %s was removed in PostgreSQL %d, target is %d", ErrUnsupportedByServer, n.what, n.until, b.version))
		return
	}
	n.inner.render(b)
}

// ---------------------------------------------------------------------------
// Numeric functions
// ---------------------------------------------------------------------------

func (e NumExpr[T]) Abs() NumExpr[T] { return NumExpr[T]{fn1("abs", e.Expr)} }
func (e NumExpr[T]) Mod(v T) NumExpr[T] {
	return e.arith("%", argNode{v})
}
func (e NumExpr[T]) Neg() NumExpr[T] { return NumExpr[T]{Expr[T]{unaryNode{"-", e.n}}} }

type unaryNode struct {
	op string
	n  node
}

func (n unaryNode) render(b *builder) {
	b.write(n.op)
	wrapIfCompound(b, n.n)
}

// Gamma is gamma(float8), new in PostgreSQL 18.
//
// Requires PostgreSQL 18.
func Gamma(o Operand[float64]) NumExpr[qt.Null[float64]] {
	return NumExpr[qt.Null[float64]]{Expr[qt.Null[float64]]{versioned{PG18, "gamma()", fnNode{"gamma", []node{o.exprNode()}}}}}
}

// Lgamma is lgamma(float8), new in PostgreSQL 18.
//
// Requires PostgreSQL 18.
func Lgamma(o Operand[float64]) NumExpr[qt.Null[float64]] {
	return NumExpr[qt.Null[float64]]{Expr[qt.Null[float64]]{versioned{PG18, "lgamma()", fnNode{"lgamma", []node{o.exprNode()}}}}}
}

// Random is random(), a float8 in [0, 1). It is volatile.
func Random() NumExpr[float64] { return NumExpr[float64]{Expr[float64]{fnNode{"random", nil}}} }

// RandomInt is random(min, max) for integers (PostgreSQL 17).
//
// Requires PostgreSQL 17.
func RandomInt(min, max int64) NumExpr[int64] {
	return NumExpr[int64]{Expr[int64]{versioned{17, "random(min, max)", fnNode{"random", []node{typedArgNode{min}, typedArgNode{max}}}}}}
}

// RandomTime is random(min, max) for timestamps (PostgreSQL 19).
//
// Requires PostgreSQL 19.
func RandomTime(min, max time.Time) TimeExpr[time.Time] {
	return TimeExpr[time.Time]{Expr[time.Time]{versioned{PG19, "random(min, max) over timestamps", fnNode{"random", []node{typedArgNode{min}, typedArgNode{max}}}}}}
}

// Uuidv7 is uuidv7(), new in PostgreSQL 18; T is the Go UUID type in use.
//
// Requires PostgreSQL 18.
func Uuidv7[T any]() Expr[T] { return Expr[T]{versioned{PG18, "uuidv7()", fnNode{"uuidv7", nil}}} }

// Uuidv4 is uuidv4(), new in PostgreSQL 18.
//
// Requires PostgreSQL 18.
func Uuidv4[T any]() Expr[T] { return Expr[T]{versioned{PG18, "uuidv4()", fnNode{"uuidv4", nil}}} }

// ---------------------------------------------------------------------------
// Time functions
// ---------------------------------------------------------------------------

// TruncUnit is a date_trunc precision.
type TruncUnit string

// Units accepted by DateTrunc.
const (
	Microseconds TruncUnit = "microseconds"
	Milliseconds TruncUnit = "milliseconds"
	Second       TruncUnit = "second"
	Minute       TruncUnit = "minute"
	Hour         TruncUnit = "hour"
	Day          TruncUnit = "day"
	Week         TruncUnit = "week"
	Month        TruncUnit = "month"
	Quarter      TruncUnit = "quarter"
	Year         TruncUnit = "year"
)

// DateTrunc is date_trunc(unit, ts); the unit is a typed constant bound as a
// parameter.
func DateTrunc[T TimeT](u TruncUnit, e TimeExpr[T]) TimeExpr[T] {
	return TimeExpr[T]{Expr[T]{fnNode{"date_trunc", []node{castedArgNode{string(u), "text"}, e.n}}}}
}

// ExtractField is an EXTRACT field.
type ExtractField string

// Fields accepted by Extract (Week is the ISO week; PostgreSQL 18 extends
// EXTRACT with WEEK for intervals).
const (
	FieldYear    ExtractField = "YEAR"
	FieldMonth   ExtractField = "MONTH"
	FieldDay     ExtractField = "DAY"
	FieldHour    ExtractField = "HOUR"
	FieldMinute  ExtractField = "MINUTE"
	FieldSecond  ExtractField = "SECOND"
	FieldDOW     ExtractField = "DOW"
	FieldDOY     ExtractField = "DOY"
	FieldWeek    ExtractField = "WEEK"
	FieldQuarter ExtractField = "QUARTER"
	FieldEpoch   ExtractField = "EPOCH"
)

type extractNode struct {
	field ExtractField
	n     node
}

func (n extractNode) render(b *builder) {
	b.write("EXTRACT(" + string(n.field) + " FROM ")
	n.n.render(b)
	b.write(")")
}

// Extract is EXTRACT(field FROM ts); the result is numeric.
func Extract[T TimeT](f ExtractField, e TimeExpr[T]) Expr[qt.Null[qt.Numeric]] {
	return Expr[qt.Null[qt.Numeric]]{extractNode{f, e.n}}
}

type intervalNode struct{ v qt.Interval }

func (n intervalNode) render(b *builder) {
	b.arg(n.v)
	b.write("::interval")
}

func (e TimeExpr[T]) shift(op string, i qt.Interval) TimeExpr[T] {
	return TimeExpr[T]{Expr[T]{cmpNode{l: e.n, op: op, r: intervalNode{i}, arith: true}}}
}

// AddInterval is ts + interval.
func (e TimeExpr[T]) AddInterval(i qt.Interval) TimeExpr[T] { return e.shift("+", i) }

// SubInterval is ts - interval.
func (e TimeExpr[T]) SubInterval(i qt.Interval) TimeExpr[T] { return e.shift("-", i) }

// Diff is ts - ts, an interval.
func (e TimeExpr[T]) Diff(o Operand[T]) Expr[qt.Null[qt.Interval]] {
	return Expr[qt.Null[qt.Interval]]{cmpNode{l: e.n, op: "-", r: o.exprNode(), arith: true}}
}

// ---------------------------------------------------------------------------
// Arrays
// ---------------------------------------------------------------------------

// ArrayExpr is an array expression with element type E.
type ArrayExpr[E any] struct{ Expr[[]E] }

func arrOp[E any](e ArrayExpr[E], op string, vs []E) Pred {
	return Pred{cmpNode{l: e.n, op: op, r: argNode{vs}}}
}

// Contains is arr @> values.
func (e ArrayExpr[E]) Contains(vs ...E) Pred { return arrOp(e, "@>", vs) }

// ContainedBy is arr <@ values.
func (e ArrayExpr[E]) ContainedBy(vs ...E) Pred { return arrOp(e, "<@", vs) }

// Overlaps is arr && values.
func (e ArrayExpr[E]) Overlaps(vs ...E) Pred { return arrOp(e, "&&", vs) }

// Has is "value = ANY(arr)".
func (e ArrayExpr[E]) Has(v E) Pred { return Pred{arrayHasNode{v, e.n}} }

type arrayHasNode struct {
	v   any
	arr node
}

func (n arrayHasNode) render(b *builder) {
	b.arg(n.v)
	b.write(" = ANY(")
	n.arr.render(b)
	b.write(")")
}

// Cardinality is cardinality(arr).
func (e ArrayExpr[E]) Cardinality() NumExpr[qt.Null[int32]] {
	return NumExpr[qt.Null[int32]]{Expr[qt.Null[int32]]{fnNode{"cardinality", []node{e.n}}}}
}

// Sort is array_sort(arr), new in PostgreSQL 18.
//
// Requires PostgreSQL 18.
func (e ArrayExpr[E]) Sort() ArrayExpr[E] {
	return ArrayExpr[E]{Expr[[]E]{versioned{PG18, "array_sort()", fnNode{"array_sort", []node{e.n}}}}}
}

// Reverse is array_reverse(arr), new in PostgreSQL 18.
//
// Requires PostgreSQL 18.
func (e ArrayExpr[E]) Reverse() ArrayExpr[E] {
	return ArrayExpr[E]{Expr[[]E]{versioned{PG18, "array_reverse()", fnNode{"array_reverse", []node{e.n}}}}}
}

// ArrayCol is an array column.
type ArrayCol[E any] struct {
	ArrayExpr[E]
	colOps[[]E]
}

// NewArrayCol is used by generated code.
func NewArrayCol[E any](t TableRef, name string) ArrayCol[E] {
	r := colRef{t, name}
	return ArrayCol[E]{ArrayExpr[E]{Expr[[]E]{r}}, colOps[[]E]{r}}
}

// ---------------------------------------------------------------------------
// JSON
// ---------------------------------------------------------------------------

// JSONExpr is a json or jsonb expression. T is the Go type it decodes to.
type JSONExpr[T any] struct{ Expr[T] }

type jsonOptNode struct {
	l   node
	op  string
	arg any
	typ string
}

func (n jsonOptNode) render(b *builder) {
	wrapIfCompound(b, n.l)
	b.write(" " + n.op + " ")
	b.arg(n.arg)
	b.write("::" + n.typ)
}

// Get is expr -> key (an object field).
func (e JSONExpr[T]) Get(key string) JSONExpr[qt.Null[qt.RawJSON]] {
	return JSONExpr[qt.Null[qt.RawJSON]]{Expr[qt.Null[qt.RawJSON]]{jsonOptNode{e.n, "->", key, "text"}}}
}

// GetIdx is expr -> index (an array element).
func (e JSONExpr[T]) GetIdx(i int32) JSONExpr[qt.Null[qt.RawJSON]] {
	return JSONExpr[qt.Null[qt.RawJSON]]{Expr[qt.Null[qt.RawJSON]]{jsonOptNode{e.n, "->", i, "int4"}}}
}

// GetText is expr ->> key, as text.
func (e JSONExpr[T]) GetText(key string) TextExpr[qt.Null[string]] {
	return TextExpr[qt.Null[string]]{Expr[qt.Null[string]]{jsonOptNode{e.n, "->>", key, "text"}}}
}

// Path is expr #> path.
func (e JSONExpr[T]) Path(keys ...string) JSONExpr[qt.Null[qt.RawJSON]] {
	return JSONExpr[qt.Null[qt.RawJSON]]{Expr[qt.Null[qt.RawJSON]]{jsonOptNode{e.n, "#>", keys, "text[]"}}}
}

// PathText is expr #>> path, as text.
func (e JSONExpr[T]) PathText(keys ...string) TextExpr[qt.Null[string]] {
	return TextExpr[qt.Null[string]]{Expr[qt.Null[string]]{jsonOptNode{e.n, "#>>", keys, "text[]"}}}
}

// Contains is jsonb @> doc.
func (e JSONExpr[T]) Contains(doc json.RawMessage) Pred {
	return Pred{jsonOptNode{e.n, "@>", string(doc), "jsonb"}}
}

// HasKey is jsonb ? key.
func (e JSONExpr[T]) HasKey(key string) Pred {
	return Pred{jsonOptNode{e.n, "?", key, "text"}}
}

// JSONBCol is a json or jsonb column.
type JSONBCol[T any] struct {
	JSONExpr[T]
	colOps[T]
}

// NewJSONBCol is used by generated code.
func NewJSONBCol[T any](t TableRef, name string) JSONBCol[T] {
	r := colRef{t, name}
	return JSONBCol[T]{JSONExpr[T]{Expr[T]{r}}, colOps[T]{r}}
}

type nodeValue struct{ n node }

func (v nodeValue) valueNode() node { return v.n }

// Arg adapts a typed operand to the untyped Value that FuncCall and Call take.
// Generated function wrappers use it so their parameters stay typed.
func Arg[T any](o Operand[T]) Value { return nodeValue{o.exprNode()} }
