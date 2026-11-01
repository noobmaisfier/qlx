package sql2go

import (
	"fmt"
	"go/format"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/noobmaisfier/qlx/internal/codegen"
)

// Options controls the emitted code.
type Options struct {
	Q   string // identifier the qlx package is imported as (default "q")
	Pkg string // qualifier of the generated package; "" when it is dot-imported
	SQL string // original text, used to turn parse-tree offsets into line:col
}

// Result is the emitted code for one input.
type Result struct {
	Code     string   // Go statements: alias declarations then `stmt := ...`
	Imports  []string // import paths the code needs besides the generated package
	Warnings []string
}

// Convert emits QLX code for every statement of a parse tree. names comes
// from codegen.Names so symbols match the generated package.
func Convert(treeJSON string, names []codegen.TableName, o Options) (*Result, error) {
	stmts, err := parseTree(treeJSON)
	if err != nil {
		return nil, err
	}
	if o.Q == "" {
		o.Q = "q"
	}
	res := &Result{}
	imports := map[string]bool{"github.com/noobmaisfier/qlx": true}
	var parts []string
	for i, st := range stmts {
		c := &conv{o: o, names: names, imports: imports, used: map[string]bool{o.Q: true}}
		name := "stmt"
		if len(stmts) > 1 {
			name = "stmt" + strconv.Itoa(i+1)
		}
		code, err := c.statement(st)
		if err != nil {
			if len(stmts) > 1 {
				return nil, fmt.Errorf("statement %d: %w", i+1, err)
			}
			return nil, err
		}
		var b strings.Builder
		for _, d := range c.decls {
			b.WriteString(d + "\n")
		}
		b.WriteString(name + " := " + code)
		parts = append(parts, b.String())
		res.Warnings = append(res.Warnings, c.warns...)
	}
	res.Code = tidy(strings.Join(parts, "\n\n"))
	for p := range imports {
		res.Imports = append(res.Imports, p)
	}
	sort.Strings(res.Imports)
	return res, nil
}

// tidy runs gofmt over the snippet by wrapping it in a function body.
func tidy(code string) string {
	src := "package p\nfunc _() {\n" + code + "\n}\n"
	out, err := format.Source([]byte(src))
	if err != nil {
		return code
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) < 4 {
		return code
	}
	var b []string
	for _, l := range lines[3 : len(lines)-1] {
		b = append(b, strings.TrimPrefix(l, "\t"))
	}
	return strings.Join(b, "\n")
}

// binding is a relation visible in a scope.
type binding struct {
	qual     string // alias, or the relation name
	code     string // Go expression for the table value: "Users" or "u"
	tbl      codegen.TableName
	excluded bool // the EXCLUDED pseudo-table of ON CONFLICT
}

type scope struct {
	b      []binding
	parent *scope
}

type litv struct {
	kind string // int, float, str, bool, null
	text string
}

// val is an emitted expression with what is known about its Go type.
type val struct {
	code string
	col  *codegen.ColumnName
	lit  *litv
	goT  string // Go type of the value, "" when unknown
	fam  string // num, text, time, bool, ... or ""
	pred bool
	agg  bool
}

type conv struct {
	o       Options
	names   []codegen.TableName
	imports map[string]bool
	used    map[string]bool
	decls   []string
	warns   []string
	sc      *scope
	exists  bool           // the SELECT being converted is the body of EXISTS(...)
	aliases map[string]val // select-list aliases, for ORDER BY
	items   []val          // select-list values in order, for ordinals
}

func (c *conv) warn(f string, a ...any) { c.warns = append(c.warns, fmt.Sprintf(f, a...)) }

func (c *conv) unsupported(n map[string]any, what, hint string) error {
	e := &Unsupported{What: what, Hint: hint}
	if n != nil {
		if loc, ok := n["location"].(float64); ok && c.o.SQL != "" {
			e.Line, e.Col = lineCol(c.o.SQL, int(loc))
		}
	}
	return e
}

func lineCol(sql string, off int) (int, int) {
	// pg_query reports -1 for a node with no position, and offsets past the
	// end for nodes it synthesised.
	if off < 0 {
		off = 0
	}
	if off > len(sql) {
		off = len(sql)
	}
	line, col := 1, 1
	for _, r := range sql[:off] {
		if r == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

func (c *conv) qual(sym string) string {
	if c.o.Pkg == "" {
		return sym
	}
	return c.o.Pkg + "." + sym
}

func (c *conv) need(path string) { c.imports[path] = true }

var goKeywords = map[string]bool{"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true, "func": true, "go": true,
	"goto": true, "if": true, "import": true, "interface": true, "map": true, "package": true, "range": true,
	"return": true, "select": true, "struct": true, "switch": true, "type": true, "var": true, "stmt": true,
	"nil": true, "true": true, "false": true, "qt": true}

func (c *conv) varName(alias string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(alias) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	n := b.String()
	if n == "" || unicode.IsDigit(rune(n[0])) {
		n = "t" + n
	}
	for goKeywords[n] || c.used[n] {
		n += "_"
	}
	c.used[n] = true
	return n
}

func (c *conv) table(schema, name string, n map[string]any) (codegen.TableName, error) {
	var hit []codegen.TableName
	for _, t := range c.names {
		if t.SQL == name && (schema == "" || t.Schema == schema) {
			hit = append(hit, t)
		}
	}
	switch len(hit) {
	case 0:
		return codegen.TableName{}, c.unsupported(n, "relation "+strings.Trim(schema+"."+name, "."), "not in schema.lock (or excluded in qlx.json)")
	case 1:
		return hit[0], nil
	}
	return codegen.TableName{}, c.unsupported(n, "relation "+name+" exists in several schemas", "qualify it with a schema")
}

// bindRange adds a RangeVar to the scope and returns its Go expression.
func (c *conv) bindRange(f map[string]any) (binding, error) {
	t, err := c.table(str(f, "schemaname"), str(f, "relname"), f)
	if err != nil {
		return binding{}, err
	}
	b := binding{qual: t.SQL, code: c.qual(t.Symbol), tbl: t}
	if af, ok := f["alias"].(map[string]any); ok { // flat struct in libpg_query's JSON
		alias := str(af, "aliasname")
		if alias != "" {
			if len(list(af, "colnames")) > 0 {
				return binding{}, c.unsupported(f, "column aliases in FROM", "")
			}
			v := c.varName(alias)
			c.decls = append(c.decls, fmt.Sprintf("%s := %s.As(%q)", v, b.code, alias))
			b.qual, b.code = alias, v
		}
	}
	if c.sc == nil {
		c.sc = &scope{}
	}
	c.sc.b = append(c.sc.b, b)
	return b, nil
}

func (c *conv) statement(st node) (string, error) {
	k, f := kindOf(st)
	switch k {
	case "SelectStmt":
		return c.selectStmt(f)
	case "InsertStmt":
		return c.insertStmt(f)
	case "UpdateStmt":
		return c.updateStmt(f)
	case "DeleteStmt":
		return c.deleteStmt(f)
	}
	return "", c.unsupported(nil, k, "sql2go converts SELECT, INSERT, UPDATE and DELETE")
}

// ---- column and literal rendering ----

func inner(goT string) (string, bool) {
	if strings.HasPrefix(goT, "qt.Null[") && strings.HasSuffix(goT, "]") {
		return goT[len("qt.Null[") : len(goT)-1], true
	}
	return goT, false
}

func colVal(b binding, col codegen.ColumnName) val {
	code := b.code + "." + col.Field
	if b.excluded {
		code += ".Excluded()"
	}
	cc := col
	return val{code: code, col: &cc, goT: col.GoType, fam: col.Family}
}

func (c *conv) resolve(sc *scope, fields []string, n map[string]any) (val, error) {
	var qual, col string
	switch len(fields) {
	case 1:
		col = fields[0]
	case 2:
		qual, col = fields[0], fields[1]
	case 3:
		qual, col = fields[1], fields[2]
	default:
		return val{}, c.unsupported(n, "column reference "+strings.Join(fields, "."), "")
	}
	for s := sc; s != nil; s = s.parent {
		var found []val
		for _, b := range s.b {
			if qual != "" && b.qual != qual {
				continue
			}
			if cc, ok := b.tbl.Column(col); ok {
				found = append(found, colVal(b, cc))
			}
		}
		if len(found) == 1 {
			return found[0], nil
		}
		if len(found) > 1 {
			return val{}, c.unsupported(n, "ambiguous column "+col, "PostgreSQL rejects it too")
		}
	}
	return val{}, c.unsupported(n, "unknown column "+strings.Join(fields, "."), "not in schema.lock")
}

func (c *conv) constant(f map[string]any) (*litv, error) {
	if b, _ := f["isnull"].(bool); b {
		return &litv{kind: "null"}, nil
	}
	// PostgreSQL 16+ trees: {"ival":{"ival":5}}; older: {"val":{"Integer":{...}}}.
	if v, ok := f["ival"].(map[string]any); ok {
		x, _ := v["ival"].(float64)
		return &litv{kind: "int", text: strconv.FormatInt(int64(x), 10)}, nil
	}
	if v, ok := f["fval"].(map[string]any); ok {
		return &litv{kind: "float", text: str(v, "fval")}, nil
	}
	if v, ok := f["sval"].(map[string]any); ok {
		return &litv{kind: "str", text: str(v, "sval")}, nil
	}
	if v, ok := f["boolval"].(map[string]any); ok {
		b, _ := v["boolval"].(bool)
		return &litv{kind: "bool", text: strconv.FormatBool(b)}, nil
	}
	if k, v := kindOf(f["val"]); k != "" {
		switch k {
		case "Integer":
			x, _ := v["ival"].(float64)
			return &litv{kind: "int", text: strconv.FormatInt(int64(x), 10)}, nil
		case "Float":
			return &litv{kind: "float", text: str(v, "fval")}, nil
		case "String":
			return &litv{kind: "str", text: str(v, "sval")}, nil
		case "Null":
			return &litv{kind: "null"}, nil
		}
	}
	return nil, c.unsupported(f, "constant", "")
}

var nonStringGo = func(t string) bool {
	return strings.HasPrefix(t, "time.") || strings.HasPrefix(t, "qt.") || strings.HasPrefix(t, "[]") ||
		strings.HasPrefix(t, "int") || strings.HasPrefix(t, "float") || t == "bool" || strings.HasPrefix(t, "uuid.")
}

// litCode renders a literal for a slot of Go type goT ("" = unknown). A
// nullable slot takes qt.Some[T](lit) because the generated column types are
// qt.Null[T]; that is the honest spelling of "this value is present".
func (c *conv) litCode(l *litv, goT string, at map[string]any) (string, error) {
	in, nullable := inner(goT)
	var code string
	switch l.kind {
	case "int", "float":
		if in == "string" || in == "bool" || strings.HasPrefix(in, "time.") {
			return "", c.unsupported(at, "number compared with a "+in+" column", "")
		}
		if in == "qt.Numeric" {
			return "", c.unsupported(at, "literal for a qt.Numeric slot", "build it with qt.ParseNumeric")
		}
		code = l.text
		if l.kind == "float" && strings.HasPrefix(in, "int") {
			return "", c.unsupported(at, "fractional literal for an "+in+" column", "")
		}
	case "str":
		if goT != "" && nonStringGo(in) {
			return "", c.unsupported(at, "string literal for a "+in+" column", "write the typed value by hand (time.Date, uuid.MustParse, ...)")
		}
		code = strconv.Quote(l.text)
	case "bool":
		if in != "" && in != "bool" {
			return "", c.unsupported(at, "boolean for a "+in+" column", "")
		}
		code = l.text
	case "null":
		return "", c.unsupported(at, "NULL as a value", "use IS NULL, or qt.None[T]() in an assignment")
	}
	if nullable {
		c.useQt()
		return fmt.Sprintf("qt.Some[%s](%s)", in, code), nil
	}
	return code, nil
}

func (c *conv) useQt() { c.need("github.com/noobmaisfier/qlx/qt") }

// FromSQL parses SQL text and emits QLX code for it.
func FromSQL(sql string, names []codegen.TableName, o Options) (*Result, error) {
	js, err := ParseToJSON(sql)
	if err != nil {
		return nil, err
	}
	o.SQL = sql
	return Convert(js, names, o)
}
