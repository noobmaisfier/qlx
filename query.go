package qlx

import (
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/noobmaisfier/qlx/qt"
)

// Query is a row source that can stand in parentheses: a SELECT or a set
// operation. Data-modifying statements are CTEBody but not Query, because
// PostgreSQL only allows them inside WITH.
type Query interface {
	Statement
	queryNode()
	outNames() []string
	needsParens() bool
}

// CTEBody is anything allowed as the body of a WITH item.
type CTEBody interface {
	Statement
	cteBody()
	outNames() []string
}

// NameRef is anything that carries an output column name.
type NameRef interface{ outName() string }

type typedName[T any] interface {
	outName() string
	typ() T
}

// ---------------------------------------------------------------------------
// Typed references into subqueries, CTEs and VALUES
// ---------------------------------------------------------------------------

type sourceCols interface{ colNames() []string }

type qualNode struct {
	key  string
	name string
	cols []string // known columns of the source, nil when unknown
}

func (n qualNode) render(b *builder) {
	if n.name == "" {
		b.fail(fmt.Errorf("%w: referenced column has no name; give it an alias with As", ErrShape))
		return
	}
	if !b.inScope(n.key) {
		b.fail(fmt.Errorf("%w: column %s.%s; add %q with From or Join", ErrScope, n.key, n.name, n.key))
		return
	}
	if n.cols != nil && !slices.Contains(n.cols, n.name) {
		b.fail(fmt.Errorf("%w: %q has no output column %q", ErrShape, n.key, n.name))
		return
	}
	b.ident(n.key)
	b.write(".")
	b.ident(n.name)
}

func refNode[T any](src FromItem, n typedName[T]) node {
	var cols []string
	if sc, ok := src.(sourceCols); ok {
		cols = sc.colNames()
		if cols == nil {
			cols = []string{}
		}
	}
	return qualNode{key: src.scopeKey(), name: n.outName(), cols: cols}
}

// Ref is the typed column of a subquery, CTE or VALUES table. The name comes
// from the typed handle (an aliased expression, a column, or a Field), so the
// Go type of the result is the type the column was declared with.
func Ref[T any](src FromItem, n typedName[T]) Expr[T] { return Expr[T]{refNode(src, n)} }

// RefNum is Ref for numeric columns, keeping comparison and arithmetic.
func RefNum[T Num](src FromItem, n typedName[T]) NumExpr[T] {
	return NumExpr[T]{Expr[T]{refNode(src, n)}}
}

// RefText is Ref for text columns.
func RefText[T Text](src FromItem, n typedName[T]) TextExpr[T] {
	return TextExpr[T]{Expr[T]{refNode(src, n)}}
}

// RefTime is Ref for timestamp columns.
func RefTime[T TimeT](src FromItem, n typedName[T]) TimeExpr[T] {
	return TimeExpr[T]{Expr[T]{refNode(src, n)}}
}

// RefBool is Ref for boolean columns.
func RefBool(src FromItem, n typedName[bool]) BoolExpr { return BoolExpr{Expr[bool]{refNode(src, n)}} }

// Field is a typed column name that exists only in a VALUES table.
type Field[T any] struct{ name string }

// NewField declares a typed column name. The name is validated and quoted
// when the statement is rendered.
func NewField[T any](name string) Field[T] { return Field[T]{name} }

func (f Field[T]) outName() string { return f.name }
func (Field[T]) typ() (z T)        { return }

// ---------------------------------------------------------------------------
// Derived tables
// ---------------------------------------------------------------------------

// Derived is a subquery in FROM or JOIN.
type Derived struct {
	q       Query
	alias   string
	lateral bool
}

// Subquery names a subquery so it can be used as a table. A plain subquery
// cannot see the outer query's tables, and rendering enforces that.
func Subquery(q Query, alias string) Derived { return Derived{q: q, alias: alias} }

// Lateral is LATERAL (subquery): it may refer to tables earlier in FROM.
func Lateral(q Query, alias string) Derived { return Derived{q: q, alias: alias, lateral: true} }

func (d Derived) scopeKey() string   { return d.alias }
func (d Derived) colNames() []string { return d.q.outNames() }
func (d Derived) renderFrom(b *builder) {
	if d.lateral {
		b.write("LATERAL ")
	} else {
		saved := b.scopes
		b.scopes = nil
		defer func() { b.scopes = saved }()
	}
	b.write("(")
	d.q.render(b)
	b.write(") AS ")
	b.ident(d.alias)
}

// ---------------------------------------------------------------------------
// Set operations
// ---------------------------------------------------------------------------

// SetQuery is UNION, INTERSECT or EXCEPT, with or without ALL.
type SetQuery struct {
	op     string
	l, r   Query
	order  []OrderTerm
	limit  *int64
	offset *int64
}

func (SetQuery) queryNode()                         {}
func (SetQuery) cteBody()                           {}
func (s SetQuery) outNames() []string               { return s.l.outNames() }
func (s SetQuery) needsParens() bool                { return true }
func (s SetQuery) OrderBy(ts ...OrderTerm) SetQuery { s.order = app(s.order, ts...); return s }
func (s SetQuery) Limit(n int64) SetQuery           { s.limit = &n; return s }
func (s SetQuery) Offset(n int64) SetQuery          { s.offset = &n; return s }

func setOp(op string, l, r Query) SetQuery { return SetQuery{op: op, l: l, r: r} }

func Union(l, r Query) SetQuery        { return setOp("UNION", l, r) }
func UnionAll(l, r Query) SetQuery     { return setOp("UNION ALL", l, r) }
func Intersect(l, r Query) SetQuery    { return setOp("INTERSECT", l, r) }
func IntersectAll(l, r Query) SetQuery { return setOp("INTERSECT ALL", l, r) }
func Except(l, r Query) SetQuery       { return setOp("EXCEPT", l, r) }
func ExceptAll(l, r Query) SetQuery    { return setOp("EXCEPT ALL", l, r) }

func (s SetQuery) Union(r Query) SetQuery        { return Union(s, r) }
func (s SetQuery) UnionAll(r Query) SetQuery     { return UnionAll(s, r) }
func (s SetQuery) Intersect(r Query) SetQuery    { return Intersect(s, r) }
func (s SetQuery) IntersectAll(r Query) SetQuery { return IntersectAll(s, r) }
func (s SetQuery) Except(r Query) SetQuery       { return Except(s, r) }
func (s SetQuery) ExceptAll(r Query) SetQuery    { return ExceptAll(s, r) }

func renderOperand(b *builder, q Query) {
	if q.needsParens() {
		b.write("(")
		q.render(b)
		b.write(")")
		return
	}
	q.render(b)
}

func (s SetQuery) render(b *builder) {
	if ln, rn := len(s.l.outNames()), len(s.r.outNames()); ln != rn {
		b.fail(fmt.Errorf("%w: %s operands have %d and %d columns", ErrShape, s.op, ln, rn))
		return
	}
	for _, c := range [...]struct {
		what string
		v    *int64
	}{{"Limit", s.limit}, {"Offset", s.offset}} {
		if c.v != nil && *c.v < 0 {
			b.fail(fmt.Errorf("%w: %s must not be negative (got %d)", ErrShape, c.what, *c.v))
			return
		}
	}
	renderOperand(b, s.l)
	b.write(" " + s.op + " ")
	renderOperand(b, s.r)
	if len(s.order) > 0 {
		b.write(" ORDER BY ")
		joinParts(b, len(s.order), func(i int) { s.order[i].render(b) })
	}
	if s.limit != nil {
		b.write(" LIMIT ")
		b.arg(*s.limit)
	}
	if s.offset != nil {
		b.write(" OFFSET ")
		b.arg(*s.offset)
	}
}

type outColNode struct {
	name string
	cols []string
}

func (n outColNode) render(b *builder) {
	if n.name == "" || !slices.Contains(n.cols, n.name) {
		b.fail(fmt.Errorf("%w: ORDER BY of a set operation must name one of its output columns", ErrShape))
		return
	}
	b.ident(n.name)
}

// ByName orders a set operation by one of its output columns. It exists
// because ORDER BY on a UNION may only name output columns, not expressions.
func (s SetQuery) ByName(n NameRef) OrderTerm {
	return OrderTerm{n: outColNode{name: n.outName(), cols: s.outNames()}}
}

// Union and friends as methods, for chaining after a SELECT.
func (q SelectQuery) Union(r Query) SetQuery        { return Union(q, r) }
func (q SelectQuery) UnionAll(r Query) SetQuery     { return UnionAll(q, r) }
func (q SelectQuery) Intersect(r Query) SetQuery    { return Intersect(q, r) }
func (q SelectQuery) IntersectAll(r Query) SetQuery { return IntersectAll(q, r) }
func (q SelectQuery) Except(r Query) SetQuery       { return Except(q, r) }
func (q SelectQuery) ExceptAll(r Query) SetQuery    { return ExceptAll(q, r) }

// ---------------------------------------------------------------------------
// WITH
// ---------------------------------------------------------------------------

type searchClause struct {
	breadth bool
	by      []string
	set     string
}

type cycleClause struct {
	by         []string
	mark, path string
}

// CTE is a WITH item. It is also a FromItem, so it can be selected from.
type CTE struct {
	name, alias string
	body        CTEBody
	// recursive form
	recursive bool
	anchor    Query
	step      Query
	unionAll  bool
	cols      []string

	mat    string // "", "MATERIALIZED", "NOT MATERIALIZED"
	search *searchClause
	cycle  *cycleClause
}

// NewCTE names a query for use in WITH. Data-modifying statements with
// RETURNING are allowed as bodies.
func NewCTE(name string, body CTEBody) CTE { return CTE{name: name, body: body} }

// Recursive builds WITH RECURSIVE name AS (anchor UNION ALL step). The step
// receives the CTE itself so it can select from it. Every column of the
// anchor needs a name (an alias, or a plain column).
func Recursive(name string, anchor Query, step func(self CTE) Query) CTE {
	self := CTE{name: name, recursive: true, anchor: anchor, unionAll: true, cols: anchor.outNames()}
	self.step = step(self)
	return self
}

// RecursiveDistinct is Recursive with UNION instead of UNION ALL.
func RecursiveDistinct(name string, anchor Query, step func(self CTE) Query) CTE {
	c := Recursive(name, anchor, step)
	c.unionAll = false
	return c
}

func (c CTE) Materialized() CTE    { c.mat = "MATERIALIZED"; return c }
func (c CTE) NotMaterialized() CTE { c.mat = "NOT MATERIALIZED"; return c }

// As gives the CTE an alias at its use site.
func (c CTE) As(alias string) CTE { c.alias = alias; return c }

func names(ns []NameRef) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.outName()
	}
	return out
}

// SearchDepthFirst adds SEARCH DEPTH FIRST BY cols SET set. The set column
// can be sorted on with SearchOrder.
func (c CTE) SearchDepthFirst(set string, by ...NameRef) CTE {
	c.search = &searchClause{by: names(by), set: set}
	return c
}

// SearchBreadthFirst adds SEARCH BREADTH FIRST BY cols SET set.
func (c CTE) SearchBreadthFirst(set string, by ...NameRef) CTE {
	c.search = &searchClause{breadth: true, by: names(by), set: set}
	return c
}

// Cycle adds CYCLE cols SET mark USING path. Rows that close a cycle get
// mark = true; CycleMark returns it as a boolean.
func (c CTE) Cycle(mark, path string, by ...NameRef) CTE {
	c.cycle = &cycleClause{by: names(by), mark: mark, path: path}
	return c
}

func (c CTE) key() string {
	if c.alias != "" {
		return c.alias
	}
	return c.name
}

// SearchOrder sorts by the SEARCH clause's ordering column.
func (c CTE) SearchOrder() OrderTerm {
	if c.search == nil {
		return OrderTerm{n: failNode{fmt.Errorf("%w: SearchOrder on a CTE without a SEARCH clause", ErrShape)}}
	}
	return OrderTerm{n: qualNode{key: c.key(), name: c.search.set}}
}

// CycleMark is the CYCLE clause's boolean marker column.
func (c CTE) CycleMark() BoolExpr {
	if c.cycle == nil {
		return BoolExpr{Expr[bool]{failNode{fmt.Errorf("%w: CycleMark on a CTE without a CYCLE clause", ErrShape)}}}
	}
	return BoolExpr{Expr[bool]{qualNode{key: c.key(), name: c.cycle.mark}}}
}

type failNode struct{ err error }

func (n failNode) render(b *builder) { b.fail(n.err) }

func (c CTE) scopeKey() string { return c.key() }
func (c CTE) colNames() []string {
	var cols []string
	switch {
	case c.recursive:
		cols = slices.Clone(c.cols)
	case c.body != nil:
		cols = slices.Clone(c.body.outNames())
	}
	if c.search != nil {
		cols = append(cols, c.search.set)
	}
	if c.cycle != nil {
		cols = append(cols, c.cycle.mark, c.cycle.path)
	}
	return cols
}

func (c CTE) renderFrom(b *builder) {
	b.ident(c.name)
	if c.alias != "" {
		b.write(" AS ")
		b.ident(c.alias)
	}
}

func (c CTE) renderDef(b *builder) {
	b.ident(c.name)
	if c.recursive {
		for _, n := range c.cols {
			if n == "" {
				b.fail(fmt.Errorf("%w: every column of the anchor of recursive CTE %q needs a name", ErrShape, c.name))
				return
			}
		}
		b.write(" (")
		joinParts(b, len(c.cols), func(i int) { b.ident(c.cols[i]) })
		b.write(")")
	}
	b.write(" AS ")
	if c.mat != "" {
		b.write(c.mat + " ")
	}
	b.write("(")
	if c.recursive {
		renderOperand(b, c.anchor)
		if c.unionAll {
			b.write(" UNION ALL ")
		} else {
			b.write(" UNION ")
		}
		renderOperand(b, c.step)
	} else {
		if c.body == nil {
			b.fail(fmt.Errorf("%w: CTE %q has no body", ErrShape, c.name))
			return
		}
		c.body.render(b)
	}
	b.write(")")
	if s := c.search; s != nil {
		if !c.recursive {
			b.fail(fmt.Errorf("%w: SEARCH needs a recursive CTE", ErrShape))
			return
		}
		if s.breadth {
			b.write(" SEARCH BREADTH FIRST BY ")
		} else {
			b.write(" SEARCH DEPTH FIRST BY ")
		}
		joinParts(b, len(s.by), func(i int) { b.ident(s.by[i]) })
		b.write(" SET ")
		b.ident(s.set)
	}
	if cy := c.cycle; cy != nil {
		if !c.recursive {
			b.fail(fmt.Errorf("%w: CYCLE needs a recursive CTE", ErrShape))
			return
		}
		b.write(" CYCLE ")
		joinParts(b, len(cy.by), func(i int) { b.ident(cy.by[i]) })
		b.write(" SET ")
		b.ident(cy.mark)
		b.write(" USING ")
		b.ident(cy.path)
	}
}

func renderWith(b *builder, ctes []CTE) {
	if len(ctes) == 0 {
		return
	}
	b.write("WITH ")
	for _, c := range ctes {
		if c.recursive {
			b.write("RECURSIVE ")
			break
		}
	}
	joinParts(b, len(ctes), func(i int) { ctes[i].renderDef(b) })
	b.write(" ")
}

// ---------------------------------------------------------------------------
// VALUES as a table
// ---------------------------------------------------------------------------

// ValuesTable is (VALUES (...), (...)) AS alias (cols).
type ValuesTable struct {
	alias string
	cols  []string
	rows  [][]any
}

func (v ValuesTable) scopeKey() string   { return v.alias }
func (v ValuesTable) colNames() []string { return v.cols }
func (v ValuesTable) renderFrom(b *builder) {
	if len(v.rows) == 0 {
		b.fail(fmt.Errorf("%w: VALUES needs at least one row", ErrShape))
		return
	}
	b.write("(VALUES ")
	for i, r := range v.rows {
		if i > 0 {
			b.write(", ")
		}
		b.write("(")
		for j, x := range r {
			if j > 0 {
				b.write(", ")
			}
			b.typedArg(x)
		}
		b.write(")")
	}
	b.write(") AS ")
	b.ident(v.alias)
	b.write(" (")
	joinParts(b, len(v.cols), func(i int) { b.ident(v.cols[i]) })
	b.write(")")
}

// Row2 and Row3 are typed VALUES rows.
type Row2[A, B any] struct {
	A A
	B B
}
type Row3[A, B, C any] struct {
	A A
	B B
	C C
}
type Row4[A, B, C, D any] struct {
	A A
	B B
	C C
	D D
}

func Values1[A any](alias string, a Field[A], rows ...A) ValuesTable {
	v := ValuesTable{alias: alias, cols: []string{a.name}}
	for _, r := range rows {
		v.rows = append(v.rows, []any{r})
	}
	return v
}

func Values2[A, B any](alias string, a Field[A], b Field[B], rows ...Row2[A, B]) ValuesTable {
	v := ValuesTable{alias: alias, cols: []string{a.name, b.name}}
	for _, r := range rows {
		v.rows = append(v.rows, []any{r.A, r.B})
	}
	return v
}

func Values3[A, B, C any](alias string, a Field[A], b Field[B], c Field[C], rows ...Row3[A, B, C]) ValuesTable {
	v := ValuesTable{alias: alias, cols: []string{a.name, b.name, c.name}}
	for _, r := range rows {
		v.rows = append(v.rows, []any{r.A, r.B, r.C})
	}
	return v
}

func Values4[A, B, C, D any](alias string, a Field[A], b Field[B], c Field[C], d Field[D], rows ...Row4[A, B, C, D]) ValuesTable {
	v := ValuesTable{alias: alias, cols: []string{a.name, b.name, c.name, d.name}}
	for _, r := range rows {
		v.rows = append(v.rows, []any{r.A, r.B, r.C, r.D})
	}
	return v
}

// pgTypeName maps a Go value to the PostgreSQL type used to cast its
// placeholder where the server cannot infer one (VALUES lists).
func pgTypeName(v any) (string, bool) {
	v = normalizeArg(v)
	switch v.(type) {
	case int16:
		return "int2", true
	case int32:
		return "int4", true
	case int64, int:
		return "int8", true
	case float32:
		return "float4", true
	case float64:
		return "float8", true
	case string:
		return "text", true
	case bool:
		return "bool", true
	case []byte:
		return "bytea", true
	case time.Time:
		return "timestamptz", true
	case qt.LocalTime:
		return "timestamp", true
	case qt.Date:
		return "date", true
	case qt.XML:
		return "xml", true
	case qt.Char:
		return `"char"`, true
	case qt.TSVector:
		return "tsvector", true
	case qt.TSQuery:
		return "tsquery", true
	case qt.Bit:
		return "varbit", true
	case qt.Point:
		return "point", true
	case qt.Line:
		return "line", true
	case qt.Lseg:
		return "lseg", true
	case qt.Box:
		return "box", true
	case qt.Path:
		return "path", true
	case qt.Polygon:
		return "polygon", true
	case qt.Circle:
		return "circle", true
	case qt.Money:
		return "money", true
	case qt.MacAddr:
		return "macaddr", true
	case qt.MacAddr8:
		return "macaddr8", true
	case qt.LSN:
		return "pg_lsn", true
	case qt.RegClass:
		return "regclass", true
	case qt.RegType:
		return "regtype", true
	case qt.RegProc:
		return "regproc", true
	case qt.RegProcedure:
		return "regprocedure", true
	case qt.RegOper:
		return "regoper", true
	case qt.RegOperator:
		return "regoperator", true
	case qt.RegNamespace:
		return "regnamespace", true
	case qt.RegRole:
		return "regrole", true
	case qt.RegConfig:
		return "regconfig", true
	case qt.RegDict:
		return "regdictionary", true
	case qt.Xid:
		return "xid", true
	case qt.Xid8:
		return "xid8", true
	case qt.Cid:
		return "cid", true
	case qt.Tid:
		return "tid", true
	case qt.JSONPath:
		return "jsonpath", true
	case qt.ACLItem:
		return "aclitem", true
	case qt.OIDVector:
		return "oidvector", true
	case qt.Int2Vector:
		return "int2vector", true
	case qt.Oid8:
		return "oid8", true
	case qt.Snapshot:
		return "pg_snapshot", true
	case qt.NodeTree:
		return "pg_node_tree", true
	case qt.RefCursor:
		return "refcursor", true
	case qt.RegCollation:
		return "regcollation", true
	case qt.RegDatabase:
		return "regdatabase", true
	case qt.NDistinct:
		return "pg_ndistinct", true
	case qt.Dependencies:
		return "pg_dependencies", true
	case qt.MCVList:
		return "pg_mcv_list", true
	case qt.TimeOfDay:
		return "time", true
	case qt.TimeTZ:
		return "timetz", true
	case qt.Numeric:
		return "numeric", true
	case nil:
		return "", false
	}
	rt := reflect.TypeOf(v)
	switch rt.Kind() {
	case reflect.Int16:
		return "int2", true
	case reflect.Int32:
		return "int4", true
	case reflect.Int64, reflect.Int:
		return "int8", true
	case reflect.String:
		return "text", true
	case reflect.Bool:
		return "bool", true
	case reflect.Array:
		if rt.Len() == 16 && rt.Elem().Kind() == reflect.Uint8 {
			return "uuid", true // [16]byte and the standard library's uuid.UUID
		}
	}
	return "", false
}

// typedArg binds v and casts the placeholder to its natural PostgreSQL type.
func (b *builder) typedArg(v any) {
	b.arg(v)
	if t, ok := pgTypeName(v); ok {
		b.write("::" + t)
	}
}

// ---------------------------------------------------------------------------
// TABLESAMPLE
// ---------------------------------------------------------------------------

// SampleMethod is BERNOULLI or SYSTEM with a percentage.
type SampleMethod struct {
	name string
	pct  float64
}

// Bernoulli samples rows.
func Bernoulli(percent float64) SampleMethod { return SampleMethod{"BERNOULLI", percent} }

// System samples blocks.
func System(percent float64) SampleMethod { return SampleMethod{"SYSTEM", percent} }

// SampledTable is a table with TABLESAMPLE.
type SampledTable struct {
	t    TableRef
	m    SampleMethod
	seed *int64
}

// TableSample samples a table: Users.TableSample(qlx.Bernoulli(10)).
func (t TableRef) TableSample(m SampleMethod) SampledTable { return SampledTable{t: t, m: m} }

// Repeatable fixes the sampling seed.
func (s SampledTable) Repeatable(seed int64) SampledTable { s.seed = &seed; return s }

func (s SampledTable) scopeKey() string { return s.t.key() }
func (s SampledTable) renderFrom(b *builder) {
	if !(s.m.pct >= 0 && s.m.pct <= 100) { // also refuses NaN
		b.fail(fmt.Errorf("%w: TABLESAMPLE percentage %v outside 0..100", ErrShape, s.m.pct))
		return
	}
	s.t.renderFrom(b)
	b.write(" TABLESAMPLE " + s.m.name + " (")
	b.arg(s.m.pct)
	b.write(")")
	if s.seed != nil {
		b.write(" REPEATABLE (")
		b.arg(*s.seed)
		b.write(")")
	}
}

// ---------------------------------------------------------------------------
// Scalar subqueries
// ---------------------------------------------------------------------------

type scalarNode struct {
	q    Query
	name string
}

func (n scalarNode) render(b *builder) {
	cols := n.q.outNames()
	if len(cols) != 1 || cols[0] != n.name {
		b.fail(fmt.Errorf("%w: scalar subquery must select exactly the column %q", ErrShape, n.name))
		return
	}
	b.write("(")
	n.q.render(b)
	b.write(")")
}

// Scalar is a subquery used as a value. It yields NULL when it returns no
// row, so the type is nullable; col names the single selected column.
func Scalar[T any](q Query, col typedName[T]) Expr[qt.Null[T]] {
	return Expr[qt.Null[T]]{scalarNode{q, col.outName()}}
}

// ScalarN is Scalar for a column that is already nullable.
func ScalarN[T any](q Query, col typedName[qt.Null[T]]) Expr[qt.Null[T]] {
	return Expr[qt.Null[T]]{scalarNode{q, col.outName()}}
}
