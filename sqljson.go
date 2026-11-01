package qlx

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/noobmaisfier/qlx/qt"
)

// SQL/JSON query functions and JSON_TABLE (PostgreSQL 17 and later).

type passItem struct {
	name string
	val  node
}

// PassArg is one PASSING variable.
type PassArg struct{ item passItem }

// Pass binds a path variable to a typed operand. The placeholder is pinned to
// T's PostgreSQL type, since the server cannot infer one from a path.
func Pass[T any](name string, v Operand[T]) PassArg {
	return PassArg{passItem{name, ArgP(v).valueNode()}}
}

func renderPassing(b *builder, ps []passItem, declared map[string]bool) {
	if len(ps) == 0 {
		return
	}
	b.write(" PASSING ")
	joinParts(b, len(ps), func(i int) {
		ps[i].val.render(b)
		b.write(" AS ")
		b.ident(ps[i].name)
	})
}

// checkVars verifies every $variable used in the path is passed.
func checkVars(b *builder, p JSONPath, ps []passItem) {
	have := map[string]bool{}
	for _, x := range ps {
		have[x.name] = true
	}
	for _, s := range append([]string{p.root}, p.steps...) {
		for i := 0; i < len(s); i++ {
			if s[i] != '$' || i+1 >= len(s) {
				continue
			}
			j := i + 1
			for j < len(s) && (s[j] == '_' || s[j] >= 'a' && s[j] <= 'z' || s[j] >= 'A' && s[j] <= 'Z' || s[j] >= '0' && s[j] <= '9') {
				j++
			}
			// A "$" inside a JSON-quoted key is not a variable: only
			// check names outside quotes (builder keys are quoted).
			if j > i+1 && !insideQuotes(s, i) && !have[s[i+1:j]] {
				b.fail(fmt.Errorf("%w: JSON path variable $%s is not passed; add Passing(Pass(%q, value))", ErrShape, s[i+1:j], s[i+1:j]))
				return
			}
			i = j - 1
		}
	}
}

func insideQuotes(s string, pos int) bool {
	in := false
	for i := 0; i < pos; i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == '"' {
			in = !in
		}
	}
	return in
}

// ---------------------------------------------------------------------------
// JSON_EXISTS
// ---------------------------------------------------------------------------

type jsonExistsNode struct {
	doc     node
	path    JSONPath
	passing []passItem
	onErr   string
}

func (n jsonExistsNode) render(b *builder) {
	b.need(PG17, "JSON_EXISTS")
	checkVars(b, n.path, n.passing)
	b.write("JSON_EXISTS(")
	n.doc.render(b)
	b.write(", ")
	pathNode{n.path}.render(b)
	renderPassing(b, n.passing, nil)
	if n.onErr != "" {
		b.write(" " + n.onErr + " ON ERROR")
	}
	b.write(")")
}

// JSONExistsBuilder is JSON_EXISTS under construction; it is a predicate.
type JSONExistsBuilder struct{ n jsonExistsNode }

// JSONExists is JSON_EXISTS(doc, path): true when the path matches anything.
//
// Requires PostgreSQL 17.
func JSONExists(doc Operand[qt.RawJSON], path JSONPath) JSONExistsBuilder {
	return JSONExistsBuilder{jsonExistsNode{doc: ArgT(doc, PgJSONB).valueNode(), path: path}}
}

// Passing binds a path variable to a value.
func (j JSONExistsBuilder) Passing(vars ...PassArg) JSONExistsBuilder {
	for _, v := range vars {
		j.n.passing = app(j.n.passing, v.item)
	}
	return j
}

// ErrorOnError, TrueOnError, FalseOnError and UnknownOnError choose the ON ERROR behavior.
func (j JSONExistsBuilder) ErrorOnError() JSONExistsBuilder   { j.n.onErr = "ERROR"; return j }
func (j JSONExistsBuilder) TrueOnError() JSONExistsBuilder    { j.n.onErr = "TRUE"; return j }
func (j JSONExistsBuilder) FalseOnError() JSONExistsBuilder   { j.n.onErr = "FALSE"; return j }
func (j JSONExistsBuilder) UnknownOnError() JSONExistsBuilder { j.n.onErr = "UNKNOWN"; return j }

// Pred makes it a WHERE condition.
func (j JSONExistsBuilder) Pred() Pred { return Pred{j.n} }

// ---------------------------------------------------------------------------
// JSON_VALUE and JSON_QUERY
// ---------------------------------------------------------------------------

type jsonValueNode struct {
	query    bool
	doc      node
	path     JSONPath
	passing  []passItem
	ret      typeRef
	wrapper  string // JSON_QUERY: "", "WITHOUT WRAPPER", "WITH WRAPPER", "WITH CONDITIONAL WRAPPER"
	quotes   string // JSON_QUERY: "", "KEEP QUOTES", "OMIT QUOTES"
	onEmpty  string // "", "ERROR", "NULL", "DEFAULT"
	emptyDef node
	onErr    string
	errDef   node
}

func (n jsonValueNode) render(b *builder) {
	name := "JSON_VALUE"
	if n.query {
		name = "JSON_QUERY"
	}
	b.need(PG17, name)
	checkVars(b, n.path, n.passing)
	b.write(name + "(")
	n.doc.render(b)
	b.write(", ")
	pathNode{n.path}.render(b)
	renderPassing(b, n.passing, nil)
	if n.ret != nil {
		b.write(" RETURNING ")
		n.ret.renderType(b)
	}
	if n.wrapper != "" && n.wrapper != "WITHOUT WRAPPER" && n.quotes != "" {
		b.fail(fmt.Errorf("%w: JSON_QUERY: %s cannot be combined with a QUOTES clause (PostgreSQL allows it only with WITHOUT WRAPPER)", ErrShape, n.wrapper))
		return
	}
	if n.wrapper != "" {
		b.write(" " + n.wrapper)
	}
	if n.quotes != "" {
		b.write(" " + n.quotes + " ON SCALAR STRING")
	}
	clause := func(kind, act string, def node) {
		if act == "" {
			return
		}
		if act == "DEFAULT" {
			b.write(" DEFAULT ")
			def.render(b)
		} else {
			b.write(" " + act)
		}
		b.write(" ON " + kind)
	}
	clause("EMPTY", n.onEmpty, n.emptyDef)
	clause("ERROR", n.onErr, n.errDef)
	b.write(")")
}

// JSONValueBuilder is JSON_VALUE: extract one scalar as a typed value.
// The result is nullable because the path may match nothing.
type JSONValueBuilder[T any] struct{ n jsonValueNode }

// JSONValue is JSON_VALUE(doc, path RETURNING type).
//
// Requires PostgreSQL 17.
func JSONValue[T any](doc Operand[qt.RawJSON], path JSONPath, as PgType[T]) JSONValueBuilder[T] {
	return JSONValueBuilder[T]{jsonValueNode{doc: ArgT(doc, PgJSONB).valueNode(), path: path, ret: as}}
}

func (j JSONValueBuilder[T]) Passing(vars ...PassArg) JSONValueBuilder[T] {
	for _, v := range vars {
		j.n.passing = app(j.n.passing, v.item)
	}
	return j
}
func (j JSONValueBuilder[T]) ErrorOnEmpty() JSONValueBuilder[T] { j.n.onEmpty = "ERROR"; return j }
func (j JSONValueBuilder[T]) NullOnEmpty() JSONValueBuilder[T]  { j.n.onEmpty = "NULL"; return j }
func (j JSONValueBuilder[T]) ErrorOnError() JSONValueBuilder[T] { j.n.onErr = "ERROR"; return j }
func (j JSONValueBuilder[T]) NullOnError() JSONValueBuilder[T]  { j.n.onErr = "NULL"; return j }

// DefaultOnEmpty returns v when the path matches nothing. The result is then
// never NULL for that reason, but the type stays nullable.
func (j JSONValueBuilder[T]) DefaultOnEmpty(v T) JSONValueBuilder[T] {
	j.n.onEmpty, j.n.emptyDef = "DEFAULT", literalNode{v}
	return j
}

// DefaultOnError returns v when evaluation fails.
func (j JSONValueBuilder[T]) DefaultOnError(v T) JSONValueBuilder[T] {
	j.n.onErr, j.n.errDef = "DEFAULT", literalNode{v}
	return j
}

// Expr is the nullable typed expression.
func (j JSONValueBuilder[T]) Expr() Expr[qt.Null[T]] { return Expr[qt.Null[T]]{j.n} }

// JSONQueryBuilder is JSON_QUERY: extract a JSON fragment.
type JSONQueryBuilder struct{ n jsonValueNode }

// JSONQuery is JSON_QUERY(doc, path RETURNING jsonb).
//
// Requires PostgreSQL 17.
func JSONQuery(doc Operand[qt.RawJSON], path JSONPath) JSONQueryBuilder {
	return JSONQueryBuilder{jsonValueNode{query: true, doc: ArgT(doc, PgJSONB).valueNode(), path: path, ret: PgJSONB}}
}

func (j JSONQueryBuilder) Passing(vars ...PassArg) JSONQueryBuilder {
	for _, v := range vars {
		j.n.passing = app(j.n.passing, v.item)
	}
	return j
}
func (j JSONQueryBuilder) WithWrapper() JSONQueryBuilder { j.n.wrapper = "WITH WRAPPER"; return j }
func (j JSONQueryBuilder) WithConditionalWrapper() JSONQueryBuilder {
	j.n.wrapper = "WITH CONDITIONAL WRAPPER"
	return j
}
func (j JSONQueryBuilder) WithoutWrapper() JSONQueryBuilder {
	j.n.wrapper = "WITHOUT WRAPPER"
	return j
}
func (j JSONQueryBuilder) KeepQuotes() JSONQueryBuilder   { j.n.quotes = "KEEP QUOTES"; return j }
func (j JSONQueryBuilder) OmitQuotes() JSONQueryBuilder   { j.n.quotes = "OMIT QUOTES"; return j }
func (j JSONQueryBuilder) ErrorOnEmpty() JSONQueryBuilder { j.n.onEmpty = "ERROR"; return j }
func (j JSONQueryBuilder) NullOnEmpty() JSONQueryBuilder  { j.n.onEmpty = "NULL"; return j }
func (j JSONQueryBuilder) ErrorOnError() JSONQueryBuilder { j.n.onErr = "ERROR"; return j }
func (j JSONQueryBuilder) NullOnError() JSONQueryBuilder  { j.n.onErr = "NULL"; return j }

// Expr is the nullable fragment.
func (j JSONQueryBuilder) Expr() Expr[qt.RawJSON] { return Expr[qt.RawJSON]{j.n} }

// ---------------------------------------------------------------------------
// JSON_TABLE
// ---------------------------------------------------------------------------

// JSONColumn is one column of JSON_TABLE.
type JSONColumn struct {
	render func(b *builder)
	names  []string
}

func colType(t typeRef) func(b *builder) { return func(b *builder) { t.renderType(b) } }

// JSONColOrdinality is `name FOR ORDINALITY`.
func JSONColOrdinality(f Field[int64]) JSONColumn {
	return JSONColumn{func(b *builder) { b.ident(f.name); b.write(" FOR ORDINALITY") }, []string{f.name}}
}

// JSONColValue is `name type PATH path`, a scalar extracted as type T.
func JSONColValue[T any](f Field[T], as PgType[T], path JSONPath) JSONColumn {
	return JSONColumn{func(b *builder) {
		b.ident(f.name)
		b.write(" ")
		as.renderType(b)
		b.write(" PATH ")
		pathNode{colPath(path)}.render(b)
	}, []string{f.name}}
}

// JSONColExists is `name boolean EXISTS PATH path`.
func JSONColExists(f Field[bool], path JSONPath) JSONColumn {
	return JSONColumn{func(b *builder) {
		b.ident(f.name)
		b.write(" boolean EXISTS PATH ")
		pathNode{colPath(path)}.render(b)
	}, []string{f.name}}
}

// JSONColQuery is `name jsonb FORMAT JSON PATH path`, a fragment.
func JSONColQuery(f Field[qt.RawJSON], path JSONPath) JSONColumn {
	return JSONColumn{func(b *builder) {
		b.ident(f.name)
		b.write(" jsonb FORMAT JSON PATH ")
		pathNode{colPath(path)}.render(b)
	}, []string{f.name}}
}

// JSONColNested is NESTED PATH path COLUMNS (...): rows of a nested array.
func JSONColNested(path JSONPath, cols ...JSONColumn) JSONColumn {
	var names []string
	for _, c := range cols {
		names = append(names, c.names...)
	}
	return JSONColumn{func(b *builder) {
		b.write("NESTED PATH ")
		pathNode{colPath(path)}.render(b)
		b.write(" COLUMNS (")
		joinParts(b, len(cols), func(i int) { cols[i].render(b) })
		b.write(")")
	}, names}
}

// JSONTableFrom is JSON_TABLE(...) used as a row source.
type JSONTableFrom struct {
	doc     node
	path    JSONPath
	passing []passItem
	cols    []JSONColumn
	alias   string
}

// JSONTable is JSON_TABLE(doc, path COLUMNS (...)) AS alias (PostgreSQL 17).
// The path must be a constant, which a JSONPath always is.
//
// Requires PostgreSQL 17.
func JSONTable(doc Operand[qt.RawJSON], path JSONPath, alias string, cols ...JSONColumn) JSONTableFrom {
	return JSONTableFrom{doc: ArgT(doc, PgJSONB).valueNode(), path: path, alias: alias, cols: cols}
}

// Passing binds a path variable.
func (t JSONTableFrom) Passing(vars ...PassArg) JSONTableFrom {
	for _, v := range vars {
		t.passing = app(t.passing, v.item)
	}
	return t
}

func (t JSONTableFrom) scopeKey() string { return t.alias }
func (t JSONTableFrom) colNames() []string {
	var out []string
	for _, c := range t.cols {
		out = append(out, c.names...)
	}
	return out
}

func (t JSONTableFrom) renderFrom(b *builder) {
	b.need(PG17, "JSON_TABLE")
	if t.alias == "" || len(t.cols) == 0 {
		b.fail(fmt.Errorf("%w: JSON_TABLE needs an alias and at least one column", ErrShape))
		return
	}
	checkVars(b, t.path, t.passing)
	b.write("JSON_TABLE(")
	t.doc.render(b)
	b.write(", ")
	pathNode{t.path}.render(b)
	renderPassing(b, t.passing, nil)
	b.write(" COLUMNS (")
	joinParts(b, len(t.cols), func(i int) { t.cols[i].render(b) })
	b.write(")) AS ")
	b.ident(t.alias)
}

// colPath makes a path written from At() relative to the row item, which
// JSON_TABLE column paths spell as $.
func colPath(p JSONPath) JSONPath {
	if p.root == "@" {
		p.root = "$"
	}
	return p
}

// literalNode renders a Go value as an SQL literal. PostgreSQL's grammar
// takes only a constant expression for JSON_VALUE ... DEFAULT (a bound
// parameter is refused: "can only specify a constant, non-aggregate function,
// or operator expression"), so these few places cannot use $n. Only scalar
// kinds with an unambiguous literal form are accepted; everything else is an
// error, never a guess.
type literalNode struct{ v any }

func (n literalNode) render(b *builder) {
	lit, err := sqlLiteral(n.v)
	if err != nil {
		b.fail(err)
		return
	}
	b.write(lit)
}

func sqlLiteral(v any) (string, error) {
	if v == nil {
		return "NULL", nil
	}
	if a, ok := v.(interface{ QLXArg() any }); ok { // qt.Null[T]
		return sqlLiteral(a.QLXArg())
	}
	if t, ok := v.(time.Time); ok {
		return "'" + t.Format(time.RFC3339Nano) + "'", nil
	}
	if r, ok := v.(json.RawMessage); ok {
		return sqlStringLiteral(string(r))
	}
	if r, ok := v.(qt.RawJSON); ok {
		return sqlStringLiteral(string(r.V))
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Bool:
		if rv.Bool() {
			return "TRUE", nil
		}
		return "FALSE", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(rv.Uint(), 10), nil
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return "", fmt.Errorf("%w: %v has no literal form here", ErrParams, f)
		}
		return strconv.FormatFloat(f, 'g', -1, 64), nil
	case reflect.String:
		return sqlStringLiteral(rv.String())
	}
	return "", fmt.Errorf("%w: a value of type %T has no literal form where PostgreSQL requires a constant", ErrParams, v)
}

// sqlStringLiteral writes E'...' with the quote and backslash doubled, which
// reads the same under any standard_conforming_strings setting.
func sqlStringLiteral(s string) (string, error) {
	if strings.ContainsRune(s, 0) {
		return "", fmt.Errorf("%w: a NUL byte cannot be written in a literal", ErrInvalidIdent)
	}
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `''`)
	return "E'" + s + "'", nil
}
