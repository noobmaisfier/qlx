package qlx

import (
	"fmt"
	"github.com/noobmaisfier/qlx/qt"
)

// SQL/JSON constructors: JSON_OBJECT, JSON_ARRAY, JSON_OBJECTAGG,
// JSON_ARRAYAGG, JSON_SCALAR and the IS JSON predicates.

// JSONPair is one `key VALUE value` member.
type JSONPair struct{ k, v node }

// JSONKV makes a member. The key is a text operand; the value any operand.
func JSONKV[V any](key Operand[string], value Operand[V]) JSONPair {
	return JSONPair{ArgT(key, PgText).valueNode(), ArgP(value).valueNode()}
}

type jsonCtor struct {
	fn        string // JSON_OBJECT | JSON_ARRAY | JSON_OBJECTAGG | JSON_ARRAYAGG
	pairs     []JSONPair
	items     []node
	query     Query
	absent    bool // ABSENT ON NULL (otherwise NULL ON NULL)
	absentSet bool
	unique    bool
	order     []OrderTerm
	filter    node
	ret       string // "", "json", "jsonb"
}

func (n jsonCtor) render(b *builder) {
	if n.filter != nil && (n.fn != "JSON_OBJECTAGG" && n.fn != "JSON_ARRAYAGG") {
		b.fail(fmt.Errorf("%w: FILTER on %s", ErrShape, n.fn))
		return
	}
	if n.fn == "JSON_OBJECTAGG" || n.fn == "JSON_ARRAYAGG" {
		b.sawAgg = true
		b.aggDepth++
		defer func() { b.aggDepth-- }()
	}
	b.write(n.fn + "(")
	switch n.fn {
	case "JSON_OBJECT", "JSON_OBJECTAGG":
		if len(n.pairs) == 0 && n.fn == "JSON_OBJECTAGG" {
			b.fail(fmt.Errorf("%w: JSON_OBJECTAGG needs a key and value", ErrShape))
			return
		}
		joinParts(b, len(n.pairs), func(i int) {
			b.write("(") // the grammar wants a c_expr here; parentheses make any expression one
			n.pairs[i].k.render(b)
			b.write(") VALUE ")
			n.pairs[i].v.render(b)
		})
	case "JSON_ARRAY", "JSON_ARRAYAGG":
		if n.query != nil {
			n.query.render(b)
		} else {
			joinParts(b, len(n.items), func(i int) { n.items[i].render(b) })
		}
		if len(n.order) > 0 {
			b.write(" ORDER BY ")
			joinParts(b, len(n.order), func(i int) { n.order[i].render(b) })
		}
	}
	if n.absentSet && n.query == nil {
		if n.absent {
			b.write(" ABSENT ON NULL")
		} else {
			b.write(" NULL ON NULL")
		}
	}
	if n.unique {
		b.write(" WITH UNIQUE KEYS")
	}
	if n.ret != "" {
		b.write(" RETURNING " + n.ret)
	}
	b.write(")")
	if n.filter != nil {
		b.write(" FILTER (WHERE ")
		n.filter.render(b)
		b.write(")")
	}
}

// JSONObjectBuilder is JSON_OBJECT(...) or JSON_OBJECTAGG(...).
type JSONObjectBuilder struct{ n jsonCtor }

// JSONObject is JSON_OBJECT(k VALUE v, ...), returning jsonb.
func JSONObject(pairs ...JSONPair) JSONObjectBuilder {
	return JSONObjectBuilder{jsonCtor{fn: "JSON_OBJECT", pairs: pairs, ret: "jsonb"}}
}

// JSONObjectAgg is the aggregate form: one object from many rows.
func JSONObjectAgg(pair JSONPair) JSONObjectBuilder {
	return JSONObjectBuilder{jsonCtor{fn: "JSON_OBJECTAGG", pairs: []JSONPair{pair}, ret: "jsonb"}}
}

func (j JSONObjectBuilder) NullOnNull() JSONObjectBuilder {
	j.n.absent, j.n.absentSet = false, true
	return j
}
func (j JSONObjectBuilder) AbsentOnNull() JSONObjectBuilder {
	j.n.absent, j.n.absentSet = true, true
	return j
}
func (j JSONObjectBuilder) WithUniqueKeys() JSONObjectBuilder { j.n.unique = true; return j }
func (j JSONObjectBuilder) AsJSON() JSONObjectBuilder         { j.n.ret = "json"; return j }

// Filter applies to the aggregate form.
func (j JSONObjectBuilder) Filter(p Pred) JSONObjectBuilder { j.n.filter = p.n; return j }

// Expr is the jsonb (or json) value.
func (j JSONObjectBuilder) Expr() Expr[qt.RawJSON] { return Expr[qt.RawJSON]{j.n} }

// JSONArrayBuilder is JSON_ARRAY(...) or JSON_ARRAYAGG(...).
type JSONArrayBuilder struct{ n jsonCtor }

// JSONArray is JSON_ARRAY(a, b, ...), returning jsonb.
func JSONArray(items ...Value) JSONArrayBuilder {
	return JSONArrayBuilder{jsonCtor{fn: "JSON_ARRAY", items: nodesOf(items), ret: "jsonb"}}
}

// JSONArrayFrom is JSON_ARRAY(subquery): one array of the single column's rows.
func JSONArrayFrom(q Query) JSONArrayBuilder {
	return JSONArrayBuilder{jsonCtor{fn: "JSON_ARRAY", query: q, ret: "jsonb"}}
}

// JSONArrayAgg is the aggregate form.
func JSONArrayAgg[T any](v Operand[T]) JSONArrayBuilder {
	return JSONArrayBuilder{jsonCtor{fn: "JSON_ARRAYAGG", items: []node{ArgP(v).valueNode()}, ret: "jsonb"}}
}

func (j JSONArrayBuilder) NullOnNull() JSONArrayBuilder {
	j.n.absent, j.n.absentSet = false, true
	return j
}
func (j JSONArrayBuilder) AbsentOnNull() JSONArrayBuilder {
	j.n.absent, j.n.absentSet = true, true
	return j
}
func (j JSONArrayBuilder) AsJSON() JSONArrayBuilder { j.n.ret = "json"; return j }

// OrderBy orders the elements of JSON_ARRAYAGG.
func (j JSONArrayBuilder) OrderBy(ts ...OrderTerm) JSONArrayBuilder {
	j.n.order = app(j.n.order, ts...)
	return j
}

// Filter applies to the aggregate form.
func (j JSONArrayBuilder) Filter(p Pred) JSONArrayBuilder { j.n.filter = p.n; return j }

// Expr is the jsonb (or json) value.
func (j JSONArrayBuilder) Expr() Expr[qt.RawJSON] { return Expr[qt.RawJSON]{j.n} }

// JSONScalar is JSON_SCALAR(v): a SQL value as a JSON scalar (PostgreSQL 17).
//
// Requires PostgreSQL 17.
func JSONScalar[T any](v Operand[T]) Expr[qt.RawJSON] {
	return Expr[qt.RawJSON]{versioned{PG17, "JSON_SCALAR", fnNode{"JSON_SCALAR", []node{ArgP(v).valueNode()}}}}
}

// JSONKind selects what IS JSON tests for.
type JSONKind string

const (
	JSONAny        JSONKind = ""
	JSONObjectKind JSONKind = " OBJECT"
	JSONArrayKind  JSONKind = " ARRAY"
	JSONScalarKind JSONKind = " SCALAR"
)

type isJSONNode struct {
	n      node
	kind   JSONKind
	unique bool
	not    bool
}

func (n isJSONNode) render(b *builder) {
	b.write("(")
	n.n.render(b)
	b.write(" IS ")
	if n.not {
		b.write("NOT ")
	}
	if !n.kind.valid() {
		badEnum(b, "JSON kind", string(n.kind))
		return
	}
	b.write("JSON" + string(n.kind))
	if n.unique {
		b.write(" WITH UNIQUE KEYS")
	}
	b.write(")")
}

// IsJSON is `text IS JSON [OBJECT|ARRAY|SCALAR]`.
func IsJSON(text Operand[string], kind JSONKind) Pred {
	return Pred{isJSONNode{n: ArgT(text, PgText).valueNode(), kind: kind}}
}

// IsNotJSON is the negation, spelled with IS NOT JSON.
func IsNotJSON(text Operand[string], kind JSONKind) Pred {
	return Pred{isJSONNode{n: ArgT(text, PgText).valueNode(), kind: kind, not: true}}
}

// JSONParseBuilder is JSON(text) (PostgreSQL 17): text parsed as a JSON value.
type JSONParseBuilder struct {
	arg    node
	unique bool
}

type jsonParseNode struct {
	arg    node
	unique bool
}

func (n jsonParseNode) render(b *builder) {
	b.need(PG17, "JSON()")
	b.write("JSON(")
	n.arg.render(b)
	if n.unique {
		b.write(" WITH UNIQUE KEYS")
	}
	b.write(")")
}

// JSONParse is JSON(text): the text must be valid JSON; the result is json.
//
// Requires PostgreSQL 17.
func JSONParse(text Operand[string]) JSONParseBuilder {
	return JSONParseBuilder{arg: ArgT(text, PgText).valueNode()}
}

// WithUniqueKeys rejects objects with duplicate keys.
func (j JSONParseBuilder) WithUniqueKeys() JSONParseBuilder { j.unique = true; return j }

// Expr is the parsed value.
func (j JSONParseBuilder) Expr() Expr[qt.RawJSON] {
	return Expr[qt.RawJSON]{jsonParseNode{j.arg, j.unique}}
}

type jsonSerializeNode struct {
	arg node
	ret string
}

func (n jsonSerializeNode) render(b *builder) {
	b.need(PG17, "JSON_SERIALIZE")
	b.write("JSON_SERIALIZE(")
	n.arg.render(b)
	if n.ret != "" {
		b.write(" RETURNING " + n.ret)
	}
	b.write(")")
}

// JSONSerialize is JSON_SERIALIZE(v) (PostgreSQL 17): a JSON value as text.
//
// Requires PostgreSQL 17.
func JSONSerialize(v Operand[qt.RawJSON]) Expr[string] {
	return Expr[string]{jsonSerializeNode{arg: ArgP(v).valueNode()}}
}

// JSONSerializeBytea is JSON_SERIALIZE(v RETURNING bytea).
func JSONSerializeBytea(v Operand[qt.RawJSON]) Expr[[]byte] {
	return Expr[[]byte]{jsonSerializeNode{arg: ArgP(v).valueNode(), ret: "bytea"}}
}
