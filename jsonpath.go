package qlx

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// JSONPath is a typed SQL/JSON path expression. It is built from steps, never
// parsed from text, so a path can neither be malformed nor carry anything but
// path syntax. Values that vary at run time travel as PASSING variables, not
// inside the path.
//
//	qlx.Root().Key("items").AllElements().Filter(qlx.At().Key("price").Gt(10)).Key("sku")
//	=> $.items[*] ? (@.price > 10).sku
type JSONPath struct {
	root  string // "$" or "@"
	steps []string
	mode  string // "", "lax", "strict"
}

// Root is the context item: $.
func Root() JSONPath { return JSONPath{root: "$"} }

// At is the current item inside a filter: @.
func At() JSONPath { return JSONPath{root: "@"} }

// Var is a PASSING variable: $name. The name must be declared with Passing.
func Var(name string) JSONPath { return JSONPath{root: "$" + name} }

func (p JSONPath) add(s string) JSONPath {
	p.steps = append(p.steps[:len(p.steps):len(p.steps)], s)
	return p
}

// Lax and Strict set the path mode (the default is lax).
func (p JSONPath) Lax() JSONPath    { p.mode = "lax"; return p }
func (p JSONPath) Strict() JSONPath { p.mode = "strict"; return p }

// Key selects an object member: .key. The name is JSON-quoted.
func (p JSONPath) Key(name string) JSONPath { return p.add("." + jsonQuote(name)) }

// Index selects an array element: [n].
func (p JSONPath) Index(n int) JSONPath { return p.add("[" + strconv.Itoa(n) + "]") }

// Slice selects elements from through to inclusive: [from to to].
func (p JSONPath) Slice(from, to int) JSONPath {
	return p.add("[" + strconv.Itoa(from) + " to " + strconv.Itoa(to) + "]")
}

// Last selects the last element: [last].
func (p JSONPath) Last() JSONPath { return p.add("[last]") }

// AllElements is [*].
func (p JSONPath) AllElements() JSONPath { return p.add("[*]") }

// AllMembers is .*.
func (p JSONPath) AllMembers() JSONPath { return p.add(".*") }

// Descendants is .** (all levels).
func (p JSONPath) Descendants() JSONPath { return p.add(".**") }

// Item methods.
func (p JSONPath) Size() JSONPath     { return p.add(".size()") }
func (p JSONPath) Type() JSONPath     { return p.add(".type()") }
func (p JSONPath) Abs() JSONPath      { return p.add(".abs()") }
func (p JSONPath) Floor() JSONPath    { return p.add(".floor()") }
func (p JSONPath) Ceiling() JSONPath  { return p.add(".ceiling()") }
func (p JSONPath) Double() JSONPath   { return p.add(".double()") }
func (p JSONPath) Number() JSONPath   { return p.add(".number()") }
func (p JSONPath) String() JSONPath   { return p.add(".string()") }
func (p JSONPath) Boolean() JSONPath  { return p.add(".boolean()") }
func (p JSONPath) Integer() JSONPath  { return p.add(".integer()") }
func (p JSONPath) Datetime() JSONPath { return p.add(".datetime()") }

// Filter keeps the items for which the condition holds: ? (cond).
func (p JSONPath) Filter(c PathCond) JSONPath { return p.add(" ? (" + c.s + ")") }

func (p JSONPath) text() string {
	var b strings.Builder
	if p.mode != "" {
		b.WriteString(p.mode + " ")
	}
	b.WriteString(p.root)
	for _, s := range p.steps {
		b.WriteString(s)
	}
	return b.String()
}

// PathCond is a filter condition.
type PathCond struct{ s string }

func pathLit(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(x)
	case string:
		return jsonQuote(x)
	case int:
		return strconv.Itoa(x)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case JSONPath:
		return x.text()
	}
	panic(fmt.Sprintf("qlx: unsupported JSON path literal %T", v))
}

func jsonQuote(s string) string {
	b, _ := json.Marshal(s) // never fails for a string
	return string(b)
}

func (p JSONPath) cmp(op string, v any) PathCond {
	return PathCond{"(" + p.text() + " " + op + " " + pathLit(v) + ")"}
}

// Comparisons against a literal (number, string, bool, nil) or another path.
func (p JSONPath) Eq(v any) PathCond  { return p.cmp("==", v) }
func (p JSONPath) Ne(v any) PathCond  { return p.cmp("!=", v) }
func (p JSONPath) Gt(v any) PathCond  { return p.cmp(">", v) }
func (p JSONPath) Gte(v any) PathCond { return p.cmp(">=", v) }
func (p JSONPath) Lt(v any) PathCond  { return p.cmp("<", v) }
func (p JSONPath) Lte(v any) PathCond { return p.cmp("<=", v) }

// Exists is exists(path).
func (p JSONPath) Exists() PathCond { return PathCond{"exists(" + p.text() + ")"} }

// StartsWith is path starts with "prefix".
func (p JSONPath) StartsWith(prefix string) PathCond {
	return PathCond{"(" + p.text() + " starts with " + jsonQuote(prefix) + ")"}
}

// PathAnd, PathOr and PathNot combine conditions.
func PathAnd(a, b PathCond) PathCond { return PathCond{"(" + a.s + " && " + b.s + ")"} }
func PathOr(a, b PathCond) PathCond  { return PathCond{"(" + a.s + " || " + b.s + ")"} }
func PathNot(a PathCond) PathCond    { return PathCond{"!(" + a.s + ")"} }

// pathNode renders the path as a string literal. E” form is used so the
// result does not depend on standard_conforming_strings: backslashes are
// doubled and quotes doubled. The text can only contain what the builder
// emits, but the escaping is done anyway.
type pathNode struct{ p JSONPath }

func (n pathNode) render(b *builder) {
	s := n.p.text()
	if strings.ContainsRune(s, 0) {
		b.fail(fmt.Errorf("%w: NUL in JSON path", ErrShape))
		return
	}
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `''`)
	b.write("E'" + s + "'")
}
