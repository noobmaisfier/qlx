package qlx

import (
	"time"

	"github.com/noobmaisfier/qlx/qt"
)

// Expression syntax that PostgreSQL's grammar spells with keywords rather than
// function calls. Each form below is the DSL entry point for one gram.y
// production; ledgers/grammar_map.json records the correspondence and
// `pgcat grammar` fails when a production has none.

// ---- pattern matching -----------------------------------------------------

type patNode struct {
	l       node
	op      string // "NOT LIKE", "NOT ILIKE", "SIMILAR TO", "NOT SIMILAR TO", "LIKE", "ILIKE"
	pattern string
	escape  string
	hasEsc  bool
}

func (n patNode) render(b *builder) {
	if !PatternKind(n.op).valid() {
		badEnum(b, "pattern operator", n.op)
		return
	}
	wrapIfCompound(b, n.l)
	b.write(" " + n.op + " ")
	b.arg(n.pattern)
	if n.hasEsc {
		b.write(" ESCAPE ")
		b.arg(n.escape)
	}
}

func (e TextExpr[T]) pat(op, p string) Pred { return Pred{patNode{l: e.n, op: op, pattern: p}} }

// NotLike is x NOT LIKE pattern.
func (e TextExpr[T]) NotLike(pattern string) Pred { return e.pat("NOT LIKE", pattern) }

// NotILike is x NOT ILIKE pattern.
func (e TextExpr[T]) NotILike(pattern string) Pred { return e.pat("NOT ILIKE", pattern) }

// SimilarTo is x SIMILAR TO pattern.
func (e TextExpr[T]) SimilarTo(pattern string) Pred { return e.pat("SIMILAR TO", pattern) }

// NotSimilarTo is x NOT SIMILAR TO pattern.
func (e TextExpr[T]) NotSimilarTo(pattern string) Pred { return e.pat("NOT SIMILAR TO", pattern) }

// LikeEscape is x LIKE pattern ESCAPE esc; kind picks LIKE, NOT LIKE, ILIKE,
// NOT ILIKE, SIMILAR TO or NOT SIMILAR TO.
func (e TextExpr[T]) LikeEscape(kind PatternKind, pattern, esc string) Pred {
	return Pred{patNode{l: e.n, op: string(kind), pattern: pattern, escape: esc, hasEsc: true}}
}

// PatternKind names the operator of LikeEscape.
type PatternKind string

// The pattern operators.
const (
	PatLike       PatternKind = "LIKE"
	PatNotLike    PatternKind = "NOT LIKE"
	PatILike      PatternKind = "ILIKE"
	PatNotILike   PatternKind = "NOT ILIKE"
	PatSimilar    PatternKind = "SIMILAR TO"
	PatNotSimilar PatternKind = "NOT SIMILAR TO"
)

// ---- BETWEEN --------------------------------------------------------------

type betweenNode struct {
	l, lo, hi node
	not, sym  bool
}

func (n betweenNode) render(b *builder) {
	wrapIfCompound(b, n.l)
	if n.not {
		b.write(" NOT")
	}
	b.write(" BETWEEN ")
	if n.sym {
		b.write("SYMMETRIC ")
	}
	n.lo.render(b)
	b.write(" AND ")
	n.hi.render(b)
}

// NotBetween is x NOT BETWEEN lo AND hi.
func (e NumExpr[T]) NotBetween(lo, hi T) Pred {
	return Pred{betweenNode{l: e.n, lo: argNode{lo}, hi: argNode{hi}, not: true}}
}

// BetweenSymmetric is x BETWEEN SYMMETRIC lo AND hi: the bounds may be given
// in either order.
func (e NumExpr[T]) BetweenSymmetric(lo, hi T) Pred {
	return Pred{betweenNode{l: e.n, lo: argNode{lo}, hi: argNode{hi}, sym: true}}
}

// NotBetweenSymmetric is x NOT BETWEEN SYMMETRIC lo AND hi.
func (e NumExpr[T]) NotBetweenSymmetric(lo, hi T) Pred {
	return Pred{betweenNode{l: e.n, lo: argNode{lo}, hi: argNode{hi}, not: true, sym: true}}
}

// NotBetween is x NOT BETWEEN lo AND hi.
func (e TimeExpr[T]) NotBetween(lo, hi T) Pred {
	return Pred{betweenNode{l: e.n, lo: argNode{lo}, hi: argNode{hi}, not: true}}
}

// BetweenSymmetric is x BETWEEN SYMMETRIC lo AND hi.
func (e TimeExpr[T]) BetweenSymmetric(lo, hi T) Pred {
	return Pred{betweenNode{l: e.n, lo: argNode{lo}, hi: argNode{hi}, sym: true}}
}

// NotBetweenSymmetric is x NOT BETWEEN SYMMETRIC lo AND hi.
func (e TimeExpr[T]) NotBetweenSymmetric(lo, hi T) Pred {
	return Pred{betweenNode{l: e.n, lo: argNode{lo}, hi: argNode{hi}, not: true, sym: true}}
}

// ---- COLLATE, AT TIME ZONE, AT LOCAL -------------------------------------

type collateNode struct {
	n    node
	name string
}

func (n collateNode) render(b *builder) {
	wrapIfCompound(b, n.n)
	b.write(" COLLATE ")
	b.ident(n.name)
}

// Collate is x COLLATE "name". The name is an identifier, quoted on output.
func (e Expr[T]) Collate(name string) Expr[T] { return Expr[T]{collateNode{e.n, name}} }

type atZoneNode struct {
	n    node
	zone node // nil: AT LOCAL
}

func (n atZoneNode) render(b *builder) {
	b.write("(")
	n.n.render(b)
	if n.zone == nil {
		b.write(" AT LOCAL)")
		return
	}
	b.write(" AT TIME ZONE ")
	n.zone.render(b)
	b.write(")")
}

// ---- niladic functions the grammar treats as keywords --------------------

type kwNode string

func (k kwNode) render(b *builder) { b.write(string(k)) }

type kwPrecNode struct {
	kw   string
	prec int32
}

func (k kwPrecNode) render(b *builder) {
	b.write(k.kw)
	b.write("(")
	b.write(itoa(int64(k.prec)))
	b.write(")")
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// CurrentUser is CURRENT_USER.
func CurrentUser() TextExpr[string] { return TextExpr[string]{Expr[string]{kwNode("CURRENT_USER")}} }

// SessionUser is SESSION_USER.
func SessionUser() TextExpr[string] { return TextExpr[string]{Expr[string]{kwNode("SESSION_USER")}} }

// CurrentRole is CURRENT_ROLE.
func CurrentRole() TextExpr[string] { return TextExpr[string]{Expr[string]{kwNode("CURRENT_ROLE")}} }

// SystemUser is SYSTEM_USER (NULL when the session was not authenticated by
// an external method).
func SystemUser() TextExpr[qt.Null[string]] {
	return TextExpr[qt.Null[string]]{Expr[qt.Null[string]]{kwNode("SYSTEM_USER")}}
}

// CurrentSchema is CURRENT_SCHEMA.
func CurrentSchema() TextExpr[qt.Null[string]] {
	return TextExpr[qt.Null[string]]{Expr[qt.Null[string]]{kwNode("CURRENT_SCHEMA")}}
}

// CurrentCatalog is CURRENT_CATALOG, the current database name.
func CurrentCatalog() TextExpr[string] {
	return TextExpr[string]{Expr[string]{kwNode("CURRENT_CATALOG")}}
}

// CurrentDate is CURRENT_DATE, delivered as a qt.Date.
func CurrentDate() TimeExpr[qt.Date] {
	return TimeExpr[qt.Date]{Expr[qt.Date]{kwNode("CURRENT_DATE")}}
}

// CurrentTime is CURRENT_TIME, the time of day with its zone offset.
func CurrentTime() TimeExpr[qt.TimeTZ] {
	return TimeExpr[qt.TimeTZ]{Expr[qt.TimeTZ]{kwNode("CURRENT_TIME")}}
}

// CurrentTimeP is CURRENT_TIME(precision).
func CurrentTimeP(precision int32) TimeExpr[qt.TimeTZ] {
	return TimeExpr[qt.TimeTZ]{Expr[qt.TimeTZ]{kwPrecNode{"CURRENT_TIME", precision}}}
}

// LocalTime is LOCALTIME, the time of day without a zone.
func LocalTime() TimeExpr[qt.TimeOfDay] {
	return TimeExpr[qt.TimeOfDay]{Expr[qt.TimeOfDay]{kwNode("LOCALTIME")}}
}

// LocalTimeP is LOCALTIME(precision).
func LocalTimeP(precision int32) TimeExpr[qt.TimeOfDay] {
	return TimeExpr[qt.TimeOfDay]{Expr[qt.TimeOfDay]{kwPrecNode{"LOCALTIME", precision}}}
}

// CurrentTimestamp is CURRENT_TIMESTAMP.
func CurrentTimestamp() TimeExpr[time.Time] {
	return TimeExpr[time.Time]{Expr[time.Time]{kwNode("CURRENT_TIMESTAMP")}}
}

// CurrentTimestampP is CURRENT_TIMESTAMP(precision).
func CurrentTimestampP(precision int32) TimeExpr[time.Time] {
	return TimeExpr[time.Time]{Expr[time.Time]{kwPrecNode{"CURRENT_TIMESTAMP", precision}}}
}

// LocalTimestamp is LOCALTIMESTAMP (timestamp without time zone).
func LocalTimestamp() TimeExpr[time.Time] {
	return TimeExpr[time.Time]{Expr[time.Time]{kwNode("LOCALTIMESTAMP")}}
}

// LocalTimestampP is LOCALTIMESTAMP(precision).
func LocalTimestampP(precision int32) TimeExpr[time.Time] {
	return TimeExpr[time.Time]{Expr[time.Time]{kwPrecNode{"LOCALTIMESTAMP", precision}}}
}

// ---- subquery forms -------------------------------------------------------

type arraySubNode struct{ q Query }

func (n arraySubNode) render(b *builder) {
	b.write("ARRAY(")
	n.q.render(b)
	b.write(")")
}

// ArrayFromQuery is ARRAY(SELECT ...): the single output column as an array.
func ArrayFromQuery[T any](q Query, col typedName[T]) Expr[[]T] {
	_ = col
	return Expr[[]T]{arraySubNode{q}}
}

type quantNode struct {
	l   node
	op  string
	q   string // ANY, SOME, ALL
	sub Query
}

func (n quantNode) render(b *builder) {
	if !CmpOp(n.op).valid() {
		badEnum(b, "comparison operator", n.op)
		return
	}
	wrapIfCompound(b, n.l)
	b.write(" " + n.op + " " + n.q + " (")
	n.sub.render(b)
	b.write(")")
}

// NotInSelect is x NOT IN (SELECT ...).
func NotInSelect[T any](e Operand[T], q Query) Pred {
	return Not(Pred{inSelectNode{l: e.exprNode(), q: q}})
}

// AnySelect is x op ANY (SELECT ...), for op one of = <> < <= > >=.
func AnySelect[T any](e Operand[T], op CmpOp, q Query) Pred {
	return Pred{quantNode{l: e.exprNode(), op: string(op), q: "ANY", sub: q}}
}

// AllSelect is x op ALL (SELECT ...).
func AllSelect[T any](e Operand[T], op CmpOp, q Query) Pred {
	return Pred{quantNode{l: e.exprNode(), op: string(op), q: "ALL", sub: q}}
}

// CmpOp is a comparison operator usable with AnySelect and AllSelect.
type CmpOp string

// The comparison operators.
const (
	OpEq  CmpOp = "="
	OpNe  CmpOp = "<>"
	OpLt  CmpOp = "<"
	OpLte CmpOp = "<="
	OpGt  CmpOp = ">"
	OpGte CmpOp = ">="
)

// ---- boolean tests on predicates -----------------------------------------

// IsNotTrue is p IS NOT TRUE.
func (p Pred) IsNotTrue() Pred { return Pred{boolTestNode{p.n, "IS NOT TRUE"}} }

// IsNotFalse is p IS NOT FALSE.
func (p Pred) IsNotFalse() Pred { return Pred{boolTestNode{p.n, "IS NOT FALSE"}} }

// IsNotUnknown is p IS NOT UNKNOWN.
func (p Pred) IsNotUnknown() Pred { return Pred{boolTestNode{p.n, "IS NOT UNKNOWN"}} }

// IsUnknown is p IS UNKNOWN.
func (p Pred) IsUnknown() Pred { return Pred{boolTestNode{p.n, "IS UNKNOWN"}} }

// IsTrue is p IS TRUE.
func (p Pred) IsTrue() Pred { return Pred{boolTestNode{p.n, "IS TRUE"}} }

// IsFalse is p IS FALSE.
func (p Pred) IsFalse() Pred { return Pred{boolTestNode{p.n, "IS FALSE"}} }
