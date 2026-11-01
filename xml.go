package qlx

import (
	"fmt"

	"github.com/noobmaisfier/qlx/qt"
)

// SQL/XML: the keyword forms of XMLCONCAT, XMLELEMENT, XMLFOREST, XMLPARSE,
// XMLPI, XMLROOT, XMLSERIALIZE, XMLEXISTS, IS DOCUMENT and XMLTABLE. The
// server must be built with libxml (the Debian and official packages are).
// XML functions that are ordinary calls (xpath, xml_is_well_formed, ...) are
// generated under pgcat/xml.

func xmlOperand(o Operand[qt.XML]) node { return ArgT(o, PgXML).valueNode() }

// parens renders n inside parentheses, which turns any expression into the
// c_expr some of the XML grammar rules require.
type parenNode struct{ n node }

func (p parenNode) render(b *builder) {
	b.write("(")
	p.n.render(b)
	b.write(")")
}

// XMLKind selects DOCUMENT or CONTENT in XMLPARSE and XMLSERIALIZE.
type XMLKind string

// The two XML value kinds.
const (
	XMLDocument XMLKind = "DOCUMENT"
	XMLContent  XMLKind = "CONTENT"
)

// XMLAttr is `value AS name`, the member of XMLATTRIBUTES and XMLFOREST.
type XMLAttr struct {
	name string
	v    node
}

// XMLAttribute pairs a value with the attribute or element name it becomes.
func XMLAttribute[T any](name string, v Operand[T]) XMLAttr {
	return XMLAttr{name: name, v: ArgP(v).valueNode()}
}

func (a XMLAttr) render(b *builder) {
	a.v.render(b)
	b.write(" AS ")
	b.ident(a.name)
}

type xmlConcatNode struct{ items []node }

func (n xmlConcatNode) render(b *builder) {
	if len(n.items) == 0 {
		b.fail(fmt.Errorf("%w: XMLCONCAT needs at least one argument", ErrShape))
		return
	}
	b.write("XMLCONCAT(")
	joinParts(b, len(n.items), func(i int) { n.items[i].render(b) })
	b.write(")")
}

// XMLConcat is XMLCONCAT(a, b, ...): the XML values joined.
func XMLConcat(items ...Operand[qt.XML]) Expr[qt.XML] {
	var ns []node
	for _, it := range items {
		ns = append(ns, xmlOperand(it))
	}
	return Expr[qt.XML]{xmlConcatNode{ns}}
}

// XMLElementBuilder is XMLELEMENT(NAME n [, XMLATTRIBUTES(...)] [, content...]).
type XMLElementBuilder struct {
	name    string
	attrs   []XMLAttr
	content []node
}

// XMLElement starts an element with the given tag name.
func XMLElement(name string) XMLElementBuilder { return XMLElementBuilder{name: name} }

// Attrs adds XMLATTRIBUTES(...).
func (e XMLElementBuilder) Attrs(attrs ...XMLAttr) XMLElementBuilder {
	e.attrs = append(e.attrs[:len(e.attrs):len(e.attrs)], attrs...)
	return e
}

// Content adds the element's content values, in order.
func (e XMLElementBuilder) Content(vals ...Value) XMLElementBuilder {
	for _, v := range vals {
		e.content = append(e.content[:len(e.content):len(e.content)], v.valueNode())
	}
	return e
}

type xmlElementNode struct{ e XMLElementBuilder }

func (n xmlElementNode) render(b *builder) {
	b.write("XMLELEMENT(NAME ")
	b.ident(n.e.name)
	if len(n.e.attrs) > 0 {
		b.write(", XMLATTRIBUTES(")
		joinParts(b, len(n.e.attrs), func(i int) { n.e.attrs[i].render(b) })
		b.write(")")
	}
	for _, c := range n.e.content {
		b.write(", ")
		c.render(b)
	}
	b.write(")")
}

// Expr is the element.
func (e XMLElementBuilder) Expr() Expr[qt.XML] { return Expr[qt.XML]{xmlElementNode{e}} }

type xmlForestNode struct{ attrs []XMLAttr }

func (n xmlForestNode) render(b *builder) {
	if len(n.attrs) == 0 {
		b.fail(fmt.Errorf("%w: XMLFOREST needs at least one element", ErrShape))
		return
	}
	b.write("XMLFOREST(")
	joinParts(b, len(n.attrs), func(i int) { n.attrs[i].render(b) })
	b.write(")")
}

// XMLForest is XMLFOREST(v AS name, ...): one element per member.
func XMLForest(members ...XMLAttr) Expr[qt.XML] { return Expr[qt.XML]{xmlForestNode{members}} }

// XMLParseBuilder is XMLPARSE(DOCUMENT|CONTENT text [whitespace option]).
type XMLParseBuilder struct {
	kind XMLKind
	text node
	ws   string
}

// XMLParse parses text as an XML document or content fragment.
func XMLParse(kind XMLKind, text Operand[string]) XMLParseBuilder {
	return XMLParseBuilder{kind: kind, text: ArgT(text, PgText).valueNode()}
}

// PreserveWhitespace keeps insignificant whitespace.
func (x XMLParseBuilder) PreserveWhitespace() XMLParseBuilder {
	x.ws = " PRESERVE WHITESPACE"
	return x
}

// StripWhitespace drops insignificant whitespace (the default).
func (x XMLParseBuilder) StripWhitespace() XMLParseBuilder { x.ws = " STRIP WHITESPACE"; return x }

type xmlParseNode struct{ x XMLParseBuilder }

func (n xmlParseNode) render(b *builder) {
	b.write("XMLPARSE(" + string(n.x.kind) + " ")
	n.x.text.render(b)
	b.write(n.x.ws + ")")
}

// Expr is the parsed value.
func (x XMLParseBuilder) Expr() Expr[qt.XML] { return Expr[qt.XML]{xmlParseNode{x}} }

type xmlPINode struct {
	name    string
	content node
}

func (n xmlPINode) render(b *builder) {
	b.write("XMLPI(NAME ")
	b.ident(n.name)
	if n.content != nil {
		b.write(", ")
		n.content.render(b)
	}
	b.write(")")
}

// XMLPI is XMLPI(NAME target): a processing instruction.
func XMLPI(target string) Expr[qt.XML] { return Expr[qt.XML]{xmlPINode{name: target}} }

// XMLPIContent is XMLPI(NAME target, content).
func XMLPIContent(target string, content Operand[string]) Expr[qt.XML] {
	return Expr[qt.XML]{xmlPINode{name: target, content: ArgT(content, PgText).valueNode()}}
}

// XMLRootBuilder is XMLROOT(x, VERSION ... [, STANDALONE ...]).
type XMLRootBuilder struct {
	x          node
	version    node // nil: VERSION NO VALUE
	standalone string
}

// XMLRoot starts XMLROOT on x with VERSION NO VALUE; set a version with Version.
func XMLRoot(x Operand[qt.XML]) XMLRootBuilder { return XMLRootBuilder{x: xmlOperand(x)} }

// Version sets VERSION v.
func (r XMLRootBuilder) Version(v Operand[string]) XMLRootBuilder {
	r.version = ArgT(v, PgText).valueNode()
	return r
}

// StandaloneYes sets STANDALONE YES.
func (r XMLRootBuilder) StandaloneYes() XMLRootBuilder { r.standalone = ", STANDALONE YES"; return r }

// StandaloneNo sets STANDALONE NO.
func (r XMLRootBuilder) StandaloneNo() XMLRootBuilder { r.standalone = ", STANDALONE NO"; return r }

// StandaloneNoValue sets STANDALONE NO VALUE.
func (r XMLRootBuilder) StandaloneNoValue() XMLRootBuilder {
	r.standalone = ", STANDALONE NO VALUE"
	return r
}

type xmlRootNode struct{ r XMLRootBuilder }

func (n xmlRootNode) render(b *builder) {
	b.write("XMLROOT(")
	n.r.x.render(b)
	b.write(", VERSION ")
	if n.r.version == nil {
		b.write("NO VALUE")
	} else {
		n.r.version.render(b)
	}
	b.write(n.r.standalone + ")")
}

// Expr is the value with its XML declaration properties set.
func (r XMLRootBuilder) Expr() Expr[qt.XML] { return Expr[qt.XML]{xmlRootNode{r}} }

// XMLSerializeBuilder is XMLSERIALIZE(DOCUMENT|CONTENT x AS type [INDENT]).
type XMLSerializeBuilder struct {
	kind   XMLKind
	x      node
	indent string
}

// XMLSerialize starts XMLSERIALIZE; finish with Text or Varchar (the server accepts character types only).
func XMLSerialize(kind XMLKind, x Operand[qt.XML]) XMLSerializeBuilder {
	return XMLSerializeBuilder{kind: kind, x: xmlOperand(x)}
}

// Indent asks for pretty-printed output (PostgreSQL 16).
func (s XMLSerializeBuilder) Indent() XMLSerializeBuilder { s.indent = " INDENT"; return s }

// NoIndent is the explicit NO INDENT.
func (s XMLSerializeBuilder) NoIndent() XMLSerializeBuilder { s.indent = " NO INDENT"; return s }

type xmlSerializeNode struct {
	s  XMLSerializeBuilder
	as string
}

func (n xmlSerializeNode) render(b *builder) {
	b.write("XMLSERIALIZE(" + string(n.s.kind) + " ")
	n.s.x.render(b)
	b.write(" AS " + n.as + n.s.indent + ")")
}

// Text is the serialization as text.
func (s XMLSerializeBuilder) Text() Expr[string] { return Expr[string]{xmlSerializeNode{s, "text"}} }

// Varchar is the serialization as varchar.
func (s XMLSerializeBuilder) Varchar() Expr[string] {
	return Expr[string]{xmlSerializeNode{s, "varchar"}}
}

type xmlExistsNode struct{ path, doc node }

func (n xmlExistsNode) render(b *builder) {
	b.write("XMLEXISTS(")
	parenNode{n.path}.render(b)
	b.write(" PASSING BY VALUE ")
	parenNode{n.doc}.render(b)
	b.write(")")
}

// XMLExists is XMLEXISTS(xpath PASSING BY VALUE doc).
func XMLExists(xpath Operand[string], doc Operand[qt.XML]) Pred {
	return Pred{xmlExistsNode{ArgT(xpath, PgText).valueNode(), xmlOperand(doc)}}
}

type isDocumentNode struct {
	x   node
	not bool
}

func (n isDocumentNode) render(b *builder) {
	b.write("(")
	n.x.render(b)
	if n.not {
		b.write(" IS NOT DOCUMENT)")
		return
	}
	b.write(" IS DOCUMENT)")
}

// IsDocument is `x IS DOCUMENT`: a well-formed document with a single root.
func IsDocument(x Operand[qt.XML]) Pred { return Pred{isDocumentNode{x: xmlOperand(x)}} }

// IsNotDocument is `x IS NOT DOCUMENT`.
func IsNotDocument(x Operand[qt.XML]) Pred { return Pred{isDocumentNode{x: xmlOperand(x), not: true}} }

// ---------------------------------------------------------------------------
// XMLTABLE
// ---------------------------------------------------------------------------

// XMLColumn is one column of XMLTABLE.
type XMLColumn struct {
	name    string
	ordinal bool
	typ     typeRef
	path    node
	def     node
	nullity string // "", " NOT NULL", " NULL"
}

// XMLColOrdinality is `name FOR ORDINALITY`.
func XMLColOrdinality(f Field[int64]) XMLColumn { return XMLColumn{name: f.name, ordinal: true} }

// XMLColValue is `name type [PATH xpath]`; without a path the column is the
// child element of the same name.
func XMLColValue[T any](f Field[T], as PgType[T]) XMLColumn {
	return XMLColumn{name: f.name, typ: as}
}

// Path sets PATH xpath for the column.
func (c XMLColumn) Path(xpath Operand[string]) XMLColumn {
	c.path = ArgT(xpath, PgText).valueNode()
	return c
}

// Default sets DEFAULT v for a missing element.
func (c XMLColumn) Default(v Value) XMLColumn { c.def = v.valueNode(); return c }

// NotNull adds NOT NULL.
func (c XMLColumn) NotNull() XMLColumn { c.nullity = " NOT NULL"; return c }

// Nullable adds an explicit NULL.
func (c XMLColumn) Nullable() XMLColumn { c.nullity = " NULL"; return c }

func (c XMLColumn) render(b *builder) {
	b.ident(c.name)
	if c.ordinal {
		b.write(" FOR ORDINALITY")
		return
	}
	b.write(" ")
	c.typ.renderType(b)
	if c.path != nil {
		b.write(" PATH ")
		parenNode{c.path}.render(b)
	}
	if c.def != nil {
		b.write(" DEFAULT ")
		parenNode{c.def}.render(b)
	}
	b.write(c.nullity)
}

// XMLNS is one namespace of XMLNAMESPACES.
type XMLNS struct {
	uri, prefix string
}

// XMLNamespace is `'uri' AS prefix`. (DEFAULT 'uri' is in the grammar but the
// server rejects it: "DEFAULT namespace is not supported".)
func XMLNamespace(uri, prefix string) XMLNS { return XMLNS{uri: uri, prefix: prefix} }

// XMLTableFrom is XMLTABLE(...) used as a row source.
type XMLTableFrom struct {
	row   node
	doc   node
	cols  []XMLColumn
	ns    []XMLNS
	alias string
}

// XMLTable is XMLTABLE(rowXPath PASSING doc COLUMNS ...) AS alias.
func XMLTable(rowXPath Operand[string], doc Operand[qt.XML], alias string, cols ...XMLColumn) XMLTableFrom {
	return XMLTableFrom{row: ArgT(rowXPath, PgText).valueNode(), doc: xmlOperand(doc), cols: cols, alias: alias}
}

// Namespaces adds XMLNAMESPACES(...).
func (t XMLTableFrom) Namespaces(ns ...XMLNS) XMLTableFrom {
	t.ns = append(t.ns[:len(t.ns):len(t.ns)], ns...)
	return t
}

func (t XMLTableFrom) scopeKey() string { return t.alias }
func (t XMLTableFrom) colNames() []string {
	var out []string
	for _, c := range t.cols {
		out = append(out, c.name)
	}
	return out
}

func (t XMLTableFrom) renderFrom(b *builder) {
	if t.alias == "" || len(t.cols) == 0 {
		b.fail(fmt.Errorf("%w: XMLTABLE needs an alias and at least one column", ErrShape))
		return
	}
	b.write("XMLTABLE(")
	if len(t.ns) > 0 {
		b.write("XMLNAMESPACES(")
		joinParts(b, len(t.ns), func(i int) {
			n := t.ns[i]
			lit, err := sqlLiteral(n.uri)
			if err != nil {
				b.fail(err)
				return
			}
			b.write(lit + " AS ")
			b.ident(n.prefix)
		})
		b.write("), ")
	}
	parenNode{t.row}.render(b)
	b.write(" PASSING ")
	parenNode{t.doc}.render(b)
	b.write(" COLUMNS ")
	joinParts(b, len(t.cols), func(i int) { t.cols[i].render(b) })
	b.write(") AS ")
	b.ident(t.alias)
}
