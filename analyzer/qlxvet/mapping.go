package qlxvet

// V018: the result type R of client.One[R](ctx, stmt) is not tied to the
// columns of stmt by the compiler. When the statement is a Select written out
// in the source (directly, through Tagged/Register, or as a package-level
// variable), the analyzer compares R with the selected items the way pgxexec
// will at run time:
//
//   - R with a ScanTargets method (generated rows, qlx rows, TupleN) is read
//     by position: same count, same Go types, same order;
//   - any other struct is read by name (field name or `db` tag against the
//     column name or alias): every column needs a field and every field a column;
//   - any other type is a scalar: the query must select exactly one column.
//
// It stays silent when something is not known: spread arguments, a statement
// held in a local variable, a name the analyzer cannot derive (an aggregate
// without alias), a type it cannot classify.

import (
	"go/ast"
	"go/constant"
	"go/types"
	"reflect"
	"strconv"
	"strings"
)

const qtPath = "github.com/noobmaisfier/qlx/qt"

// rowFuncs are the pgxexec entry points whose first type argument is the
// result type; the statement is the first argument that resolves to a Select.
var rowFuncs = map[string]bool{
	"All": true, "One": true, "Maybe": true, "Each": true,
	"AllScan": true, "OneScan": true, "EachScan": true,
	"PipeAll": true, "PipeOne": true,
	"NewStatic0": true, "NewStatic1": true, "NewStatic2": true, "NewStatic3": true, "NewStatic4": true,
}

func isPgxexec(path string) bool {
	return path == qlxPath+"/pgxexec" || strings.HasSuffix(path, "/pgxexec")
}

type item struct {
	typ  types.Type // Go type of the value, nil when unknown
	name string     // lower-cased output column name, "" when unknown
	text string     // source text, for messages
	// outer: a plain column of the optional side of an outer join, which
	// the type says is NOT NULL but a row without a match makes NULL.
	outer bool
}

func (c *checker) mappingCheck(call *ast.CallExpr) {
	var id *ast.Ident
	var typeArg ast.Expr // the explicit R of client.One[R](...) or pgxexec.One[R](...)
	switch f := ast.Unparen(call.Fun).(type) {
	case *ast.SelectorExpr:
		id = f.Sel
	case *ast.Ident:
		id = f
	case *ast.IndexExpr:
		id, typeArg = identOf(f.X), f.Index
	case *ast.IndexListExpr:
		id = identOf(f.X)
		if len(f.Indices) > 0 {
			typeArg = f.Indices[0]
		}
	}
	if id == nil {
		return
	}
	fn := c.funcOf(id, call)
	if fn == nil || fn.Pkg() == nil || !isPgxexec(fn.Pkg().Path()) || !rowFuncs[fn.Name()] {
		return
	}
	// R is read from the type argument as written: that works for a function
	// (pgxexec.One[R]) and for a Go 1.27 generic method (client.One[R]) alike,
	// whatever go/types records in Info.Instances for the latter.
	var r types.Type
	if typeArg != nil {
		r = c.pass.TypesInfo.TypeOf(typeArg)
	}
	if r == nil {
		if inst, ok := c.pass.TypesInfo.Instances[id]; ok && inst.TypeArgs != nil && inst.TypeArgs.Len() > 0 {
			r = inst.TypeArgs.At(0)
		}
	}
	if r == nil || hasTypeParam(r) {
		return
	}
	var root *ast.CallExpr
	var links []link
	for _, a := range call.Args {
		if rt, ls, ok := c.selectRoot(a, 0); ok {
			root, links = rt, ls
			break
		}
	}
	if root == nil || spread(root) || len(root.Args) == 0 {
		return
	}
	outer := c.outerTables(links)
	items := make([]item, len(root.Args))
	for i, a := range root.Args {
		items[i] = c.itemOf(a)
		if t := c.columnTable(a); t != nil && outer[t] {
			items[i].outer = true
		}
	}
	c.mapResult(call, r, items)
}

// columnTable returns the table variable of a plain column item, looking
// through .As("alias").
func (c *checker) columnTable(a ast.Expr) types.Object {
	a = ast.Unparen(a)
	if call, ok := a.(*ast.CallExpr); ok {
		if sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr); ok && sel.Sel.Name == "As" && len(call.Args) == 1 {
			a = ast.Unparen(sel.X)
		}
	}
	if sel, ok := a.(*ast.SelectorExpr); ok && c.isColumn(sel) {
		return c.tableVar(sel.X)
	}
	return nil
}

// outerTables returns the table variables on the optional side of an outer
// join in the chain: the joined table of a LEFT JOIN, every table seen so far
// for a RIGHT JOIN, both sides of a FULL JOIN. Tables it cannot name (aliases,
// subqueries) are not listed, so they are never reported.
func (c *checker) outerTables(links []link) map[types.Object]bool {
	nullable := map[types.Object]bool{}
	var seen []types.Object
	for _, l := range links {
		if l.name != "From" && !joinMethods[l.name] {
			continue
		}
		var added []types.Object
		for _, a := range l.call.Args {
			if v := c.tableVar(a); v != nil {
				added = append(added, v)
			}
		}
		switch l.name {
		case "LeftJoin":
			for _, v := range added {
				nullable[v] = true
			}
		case "RightJoin":
			for _, v := range seen {
				nullable[v] = true
			}
		case "FullJoin":
			for _, v := range seen {
				nullable[v] = true
			}
			for _, v := range added {
				nullable[v] = true
			}
		}
		seen = append(seen, added...)
	}
	return nullable
}

func identOf(e ast.Expr) *ast.Ident {
	switch x := ast.Unparen(e).(type) {
	case *ast.Ident:
		return x
	case *ast.SelectorExpr:
		return x.Sel
	}
	return nil
}

func hasTypeParam(t types.Type) bool {
	switch x := t.(type) {
	case *types.TypeParam:
		return true
	case *types.Pointer:
		return hasTypeParam(x.Elem())
	case *types.Slice:
		return hasTypeParam(x.Elem())
	case *types.Array:
		return hasTypeParam(x.Elem())
	case *types.Map:
		return hasTypeParam(x.Key()) || hasTypeParam(x.Elem())
	case *types.Named:
		if ta := x.TypeArgs(); ta != nil {
			for i := 0; i < ta.Len(); i++ {
				if hasTypeParam(ta.At(i)) {
					return true
				}
			}
		}
	}
	return false
}

// selectRoot resolves e to the root call of a Select chain: written out, or
// wrapped in Tagged/Register, or held by a package-level variable.
func (c *checker) selectRoot(e ast.Expr, depth int) (*ast.CallExpr, []link, bool) {
	if depth > 4 {
		return nil, nil, false
	}
	switch x := ast.Unparen(e).(type) {
	case *ast.CallExpr:
		root, links := chain(x)
		fn := c.callee(root)
		switch {
		case isFunc(fn, "Select"):
			return root, links, true
		case isFunc(fn, "Tagged") && len(root.Args) == 2, isFunc(fn, "Register") && len(root.Args) == 2:
			return c.selectRoot(root.Args[1], depth+1)
		}
	case *ast.Ident, *ast.SelectorExpr:
		if id := identOf(x); id != nil {
			if init := c.pkgVarInit(c.pass.TypesInfo.Uses[id]); init != nil {
				return c.selectRoot(init, depth+1)
			}
		}
	}
	return nil, nil, false
}

// pkgVarInit returns the initializer of a package-level variable declared with
// a single value in this package.
func (c *checker) pkgVarInit(obj types.Object) ast.Expr {
	v, ok := obj.(*types.Var)
	if !ok || v.Pkg() != c.pass.Pkg || v.Parent() != c.pass.Pkg.Scope() {
		return nil
	}
	if c.varInits == nil {
		c.varInits = map[types.Object]ast.Expr{}
		for _, f := range c.pass.Files {
			for _, d := range f.Decls {
				gd, ok := d.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, sp := range gd.Specs {
					vs, ok := sp.(*ast.ValueSpec)
					if !ok || len(vs.Names) != len(vs.Values) {
						continue
					}
					for i, n := range vs.Names {
						if o := c.pass.TypesInfo.Defs[n]; o != nil {
							c.varInits[o] = vs.Values[i]
						}
					}
				}
			}
		}
	}
	return c.varInits[obj]
}

// valueType is the Go type of the value an expression type produces:
// Expr[T], Named[T], AggExpr[T], and every column or expression type that
// embeds one of them.
func valueType(t types.Type) types.Type {
	n, ok := t.(*types.Named)
	if !ok || n.Obj().Pkg() == nil || n.Obj().Pkg().Path() != qlxPath {
		return nil
	}
	switch n.Obj().Name() {
	case "Expr", "Named", "AggExpr":
		if ta := n.TypeArgs(); ta != nil && ta.Len() > 0 {
			return ta.At(0)
		}
		return nil
	}
	st, ok := n.Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	for i := 0; i < st.NumFields(); i++ {
		if f := st.Field(i); f.Embedded() {
			if v := valueType(f.Type()); v != nil {
				return v
			}
		}
	}
	return nil
}

func (c *checker) itemOf(a ast.Expr) item {
	a = ast.Unparen(a)
	it := item{text: types.ExprString(a)}
	if tv, ok := c.pass.TypesInfo.Types[a]; ok {
		it.typ = valueType(tv.Type)
	}
	switch x := a.(type) {
	case *ast.CallExpr:
		if sel, ok := ast.Unparen(x.Fun).(*ast.SelectorExpr); ok && sel.Sel.Name == "As" && len(x.Args) == 1 {
			if tv := c.pass.TypesInfo.Types[x.Args[0]]; tv.Value != nil && tv.Value.Kind() == constant.String {
				it.name = strings.ToLower(constant.StringVal(tv.Value))
			}
		}
	case *ast.SelectorExpr:
		if v := c.tableVar(x.X); v != nil {
			if n, ok := v.Type().(*types.Named); ok {
				var tf TableFact
				if c.pass.ImportObjectFact(n.Obj(), &tf) {
					if col := tf.Cols[x.Sel.Name]; col != "" {
						it.name = strings.ToLower(col)
					}
				}
			}
		}
	}
	return it
}

type field struct {
	goName string // Go field name
	name   string // lower-cased match key
	typ    types.Type
}

// collectFields mirrors pgxexec: exported fields, `db` tags, "-" to skip,
// anonymous struct fields without a tag flattened.
func collectFields(st *types.Struct, out *[]field) {
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		tag, hasTag := reflect.StructTag(st.Tag(i)).Lookup("db")
		if f.Embedded() {
			if es, ok := f.Type().Underlying().(*types.Struct); ok && tag == "" {
				collectFields(es, out)
				continue
			}
		}
		if !f.Exported() {
			continue
		}
		name := f.Name()
		if hasTag {
			if tag == "-" {
				continue
			}
			name = tag
		}
		*out = append(*out, field{f.Name(), strings.ToLower(name), f.Type()})
	}
}

func (c *checker) mapResult(call *ast.CallExpr, r types.Type, items []item) {
	name := types.TypeString(r, func(p *types.Package) string {
		if p == c.pass.Pkg {
			return ""
		}
		return p.Name()
	})

	// Positional: a ScanTargets method whose body we understood.
	if ms := types.NewMethodSet(types.NewPointer(r)); ms.Len() > 0 {
		if sel := ms.Lookup(nil, "ScanTargets"); sel != nil {
			c.mapPositional(call, r, name, sel, items)
			return
		}
	}

	st, isStruct := r.Underlying().(*types.Struct)
	if !isStruct {
		switch r.Underlying().(type) {
		case *types.Interface:
			return
		}
		if len(items) != 1 {
			c.report(call, "V018", name+" is not a struct, so the query must select exactly one column, but it selects "+strconv.Itoa(len(items)))
			return
		}
		if msg := mismatch(items[0], r); msg != "" {
			c.report(call, "V018", "column "+items[0].text+": "+msg+" ("+name+")")
		}
		return
	}
	if len(items) == 1 {
		// A struct that is one value (time.Time, qt.Null[T], ...). When the
		// type is one the analyzer classifies, the column is compared with it.
		if classOf(r) != "" {
			if msg := mismatch(items[0], r); msg != "" {
				c.report(call, "V018", "column "+items[0].text+": "+msg+" ("+name+")")
			}
		}
		return
	}
	var fields []field
	collectFields(st, &fields)
	byName := map[string]field{}
	for _, f := range fields {
		byName[f.name] = f
	}
	used := map[string]bool{}
	allNamed := true
	for _, it := range items {
		if it.name == "" {
			allNamed = false
			continue
		}
		f, ok := byName[it.name]
		if !ok {
			c.report(call, "V018", name+" has no field for column "+strconv.Quote(it.name)+" (tag a field `db:\""+it.name+"\"`)")
			continue
		}
		used[f.name] = true
		if msg := mismatch(it, f.typ); msg != "" {
			c.report(call, "V018", "column "+it.name+" into "+name+"."+f.goName+": "+msg)
		}
	}
	if allNamed {
		for _, f := range fields {
			if !used[f.name] {
				c.report(call, "V018", "field "+name+"."+f.goName+" has no column in the query (tag it `db:\"-\"` to skip it)")
			}
		}
	}
}

func (c *checker) mapPositional(call *ast.CallExpr, r types.Type, name string, sel *types.Selection, items []item) {
	fn, ok := sel.Obj().(*types.Func)
	if !ok {
		return
	}
	var rf RowFact
	if !c.pass.ImportObjectFact(fn.Origin(), &rf) {
		return
	}
	if len(rf.Fields) != len(items) {
		c.report(call, "V018", name+" scans "+strconv.Itoa(len(rf.Fields))+" columns (its ScanTargets) but the query selects "+strconv.Itoa(len(items)))
		return
	}
	st, ok := r.Underlying().(*types.Struct)
	if !ok {
		if p, isPtr := r.Underlying().(*types.Pointer); isPtr {
			st, _ = p.Elem().Underlying().(*types.Struct)
		}
		if st == nil {
			return
		}
	}
	ftypes := map[string]types.Type{}
	for i := 0; i < st.NumFields(); i++ {
		ftypes[st.Field(i).Name()] = st.Field(i).Type()
	}
	for i, it := range items {
		if ft := ftypes[rf.Fields[i]]; ft != nil {
			if msg := mismatch(it, ft); msg != "" {
				c.report(call, "V018", "column "+strconv.Itoa(i+1)+" ("+it.text+") into "+name+"."+rf.Fields[i]+": "+msg)
			}
		}
	}
	// Same types in a different order cannot be seen by type; names can.
	norm := func(s string) string { return strings.ReplaceAll(strings.ToLower(s), "_", "") }
	for i, it := range items {
		if it.name == "" || norm(it.name) == norm(rf.Fields[i]) {
			continue
		}
		for j, f := range rf.Fields {
			if j != i && norm(f) == norm(it.name) {
				c.report(call, "V018", "column "+it.name+" (position "+strconv.Itoa(i+1)+") matches field "+name+"."+f+" (position "+strconv.Itoa(j+1)+"): ScanTargets reads by position, so the field order must follow the select order")
				return
			}
		}
	}
}

// ---- Go type compatibility ----

func isNullType(t types.Type) (types.Type, bool) {
	n, ok := t.(*types.Named)
	if !ok || n.Obj().Pkg() == nil || n.Obj().Pkg().Path() != qtPath || n.Obj().Name() != "Null" {
		return t, false
	}
	if ta := n.TypeArgs(); ta != nil && ta.Len() == 1 {
		return ta.At(0), true
	}
	return t, false
}

// classOf buckets a type for the scan rules that matter; "" means unknown,
// and unknown never produces a finding.
func classOf(t types.Type) string {
	if n, ok := t.(*types.Named); ok && n.Obj().Pkg() != nil {
		switch pkg, name := n.Obj().Pkg().Path(), n.Obj().Name(); {
		case pkg == "time" && name == "Time", pkg == qtPath && name == "Timestamptz":
			return "time"
		case pkg == qtPath && name == "Date":
			return "date"
		case pkg == qtPath && name == "Interval":
			return "interval"
		case pkg == qtPath && name == "TimeTZ":
			return "timetz"
		case pkg == qtPath && (name == "Range" || name == "Multirange"):
			return "range"
		case pkg == qtPath && name == "JSONB":
			return "json"
		case pkg == "net/netip" && (name == "Addr" || name == "Prefix"):
			return "inet"
		}
	}
	if a, ok := t.Underlying().(*types.Array); ok {
		if b, ok := a.Elem().Underlying().(*types.Basic); ok && b.Kind() == types.Uint8 && a.Len() == 16 {
			return "uuid"
		}
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch {
		case u.Info()&types.IsBoolean != 0:
			return "bool"
		case u.Info()&types.IsInteger != 0:
			return "int"
		case u.Info()&types.IsFloat != 0:
			return "float"
		case u.Info()&types.IsString != 0:
			return "string"
		}
	case *types.Slice:
		if b, ok := u.Elem().Underlying().(*types.Basic); ok && b.Kind() == types.Uint8 {
			return "bytes"
		}
		return "slice"
	}
	return ""
}

// mismatch explains why a column of Go type x cannot be read into y, or "".
func mismatch(it item, y types.Type) string {
	x := it.typ
	if x == nil || y == nil {
		return ""
	}
	xc, xNull := isNullType(x)
	if it.outer && !xNull {
		xNull = true
	}
	if types.Identical(x, y) && !it.outer {
		return ""
	}
	yt, yNullable := y, false
	if p, ok := yt.(*types.Pointer); ok {
		yt, yNullable = p.Elem(), true
	}
	if yc, ok := isNullType(yt); ok {
		yt, yNullable = yc, true
	}
	if xNull && !yNullable {
		if _, basic := yt.Underlying().(*types.Basic); basic {
			if it.outer {
				return "the column comes from the optional side of an outer join, so it can be NULL, but the field cannot hold it; wrap it with q.Nullable and read it into qt.Null[" + types.TypeString(xc, nil) + "] or a pointer"
			}
			return "the column can be NULL but the field cannot hold it; use qt.Null[" + types.TypeString(xc, nil) + "] or a pointer"
		}
	}
	a, b := classOf(xc), classOf(yt)
	if a == "" || b == "" || a == b {
		return ""
	}
	num := func(c string) bool { return c == "int" || c == "float" }
	// Types that carry their own wire format (dates, intervals, ranges, inet,
	// uuid, time with zone): a column of one reads into a field of the same
	// kind, or into text, bytes or a JSON-ish value, which pgx can render. The
	// text-like side is never decided by the Go type alone (a uuid column may
	// be mapped to string, a range to a string alias), so it stays silent.
	rich := func(c string) bool {
		switch c {
		case "date", "interval", "timetz", "range", "inet", "uuid":
			return true
		}
		return false
	}
	vague := func(c string) bool { return c == "string" || c == "bytes" || c == "slice" || c == "json" }
	if rich(a) || rich(b) {
		switch {
		case vague(a) || vague(b):
			return ""
		case a == "date" && b == "time", a == "time" && b == "date":
			return "" // a date reads into time.Time and back
		}
	}
	if (a == "json" || b == "json") && (num(a) || num(b) || a == "bool" || b == "bool") {
		return "" // a JSON value may be a scalar
	}
	switch {
	case num(a) && num(b):
		return "" // pgx plans int <-> float (a value that does not fit fails on that row)
	case num(a) && b == "string":
		return "" // pgx renders numbers as text
	case b == "string" && (a == "bool" || a == "bytes" || a == "slice"):
		return "" // not decided by the type
	case b == "bytes" || b == "slice" && (a == "string" || a == "bytes"):
		return "" // text and bytea scan into byte slices and arrays alike
	}
	return a + " column (" + types.TypeString(xc, nil) + ") cannot be read into a " + b + " field (" + types.TypeString(yt, nil) + ")"
}

// funcOf resolves the function or method an identifier of a call names.
func (c *checker) funcOf(id *ast.Ident, call *ast.CallExpr) *types.Func {
	if fn, ok := c.pass.TypesInfo.Uses[id].(*types.Func); ok {
		return fn
	}
	// A method selected on a value: look at the selection as well.
	var sel *ast.SelectorExpr
	switch f := ast.Unparen(call.Fun).(type) {
	case *ast.SelectorExpr:
		sel = f
	case *ast.IndexExpr:
		sel, _ = ast.Unparen(f.X).(*ast.SelectorExpr)
	case *ast.IndexListExpr:
		sel, _ = ast.Unparen(f.X).(*ast.SelectorExpr)
	}
	if sel != nil {
		if s := c.pass.TypesInfo.Selections[sel]; s != nil {
			if fn, ok := s.Obj().(*types.Func); ok {
				return fn
			}
		}
	}
	return nil
}
