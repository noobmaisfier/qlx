// Package qlxvet is the QLX static analyzer, written against
// golang.org/x/tools/go/analysis so it runs under `go vet -vettool`, gopls and
// golangci-lint, and is tested with analysistest.
//
// Findings (stable codes):
//
//	V001 discarded builder result (builders are immutable)
//	V002 UPDATE/DELETE chain handed on with neither Where nor All
//	V003 Register name not constant, or duplicated
//	V004 SQL/Render result or error discarded
//	V005 Register inside a function body
//	V006 column of a table that is not in the query's FROM/JOIN/USING scope
//	V007 aggregate mixed with bare columns and no GroupBy
//	V008 INSERT row that does not set every required column
//	V009 function, method or EXPLAIN option whose PostgreSQL version range does not cover -targets
//	V010 //qlx:ignore without a reason
//	V012 statement or clause missing a required part (UPDATE without Set, INSERT without row, MERGE without Using/On/When, ...)
//	V013 contradictory, duplicated or mismatched parts (column assigned twice, Limit with FetchFirst, set operands of different width, ...)
//	V014 bare column next to a GroupBy that does not list its table
//	V015 EXPLAIN options that need ANALYZE or cannot be combined with it
//	V016 FOR ... OF a table that is not in the query's FROM/JOIN
//	V017 CTE SEARCH/CYCLE on a non-recursive CTE
//	V019 window function in WHERE/HAVING/ON/GROUP BY
//	V020 named window used but not defined in the query
//	V021 table function without As, JSON_TABLE/XMLTable without alias or columns
//	V022 JSON path variable not passed, QUOTES combined with WITH WRAPPER
//	V023 GRANT/REVOKE missing or misdirected parts
//	V024 REFRESH CONCURRENTLY with NO DATA, SET CONSTRAINTS without a name
//	V025 SearchOrder/CycleMark on a CTE built without SEARCH/CYCLE
//	V026 conflict column or assignment naming a column of another table
//	V027 retired: ToOrDefault in a SET list no longer compiles (the code is not reused)
//	V028 FOR UPDATE/SHARE together with DISTINCT, GROUP BY, HAVING, aggregates or window functions
//	V029 SELECT DISTINCT ordered by a column that is not selected; DISTINCT ON not matching ORDER BY
//	V030 ToNull() on a column whose Go type cannot hold NULL
//	V018 result type that does not match the selected columns (count, names, Go types, nullability, order)
//
// A finding is suppressed by a comment on the same or the previous line:
//
//	//qlx:ignore <reason>
//
// With -audit every suppression is itself reported (V011) with its reason, so
// the count and the reasons are reviewable in CI output.
package qlxvet

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
)

const qlxPath = "github.com/noobmaisfier/qlx"

var (
	targetsFlag string
	auditFlag   bool
)

// Analyzer is the entry point.
var Analyzer = &analysis.Analyzer{
	Name:      "qlxvet",
	Doc:       "reports QLX misuse the type system cannot see (see package documentation)",
	Run:       run,
	FactTypes: []analysis.Fact{(*TableFact)(nil), (*VersionFact)(nil), (*RowFact)(nil), (*SelectFact)(nil)},
}

func init() {
	Analyzer.Flags.StringVar(&targetsFlag, "targets", "18,19", "comma-separated PostgreSQL majors the program must run on (V009)")
	Analyzer.Flags.BoolVar(&auditFlag, "audit", false, "report every //qlx:ignore with its reason (V011)")
}

type ignore struct {
	reason string
	pos    token.Pos
	used   bool
}

type checker struct {
	pass       *analysis.Pass
	regs       map[string]token.Position
	ignores    map[string]map[int]*ignore // file -> line -> ignore
	targets    []int
	reported   map[string]bool
	varInits   map[types.Object]ast.Expr // package-level variable initializers (V018)
	assigns    map[types.Object]ast.Expr // single-assignment variable initializers (V025)
	selInit    map[types.Object]ast.Expr // variable initializers, for the Select a variable holds (V018)
	selBad     map[types.Object]bool     // variables reassigned to something else, or whose address is taken
	reassigned map[types.Object]bool
	aliases    map[aliasKey]types.Object // one object per (table variable, alias literal)
	funcDecls  map[types.Object]*ast.FuncDecl
}

func run(pass *analysis.Pass) (any, error) {
	exportFacts(pass)
	c := &checker{pass: pass, regs: map[string]token.Position{}, ignores: map[string]map[int]*ignore{}}
	for _, s := range strings.Split(targetsFlag, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
			c.targets = append(c.targets, n)
		}
	}
	sort.Ints(c.targets)
	for _, f := range pass.Files {
		c.collectIgnores(f)
	}
	c.exportSelectFacts()
	for _, f := range pass.Files {
		c.file(f)
	}
	if auditFlag {
		for _, byLine := range c.ignores {
			for _, ig := range byLine {
				if ig.reason != "" {
					pass.Reportf(ig.pos, "V011 suppressed finding: %s", ig.reason)
				}
			}
		}
	}
	return nil, nil
}

func (c *checker) collectIgnores(f *ast.File) {
	for _, cg := range f.Comments {
		for _, cm := range cg.List {
			txt := strings.TrimSpace(strings.TrimPrefix(cm.Text, "//"))
			if !strings.HasPrefix(txt, "qlx:ignore") {
				continue
			}
			reason := strings.TrimSpace(strings.TrimPrefix(txt, "qlx:ignore"))
			p := c.pass.Fset.Position(cm.Pos())
			if reason == "" {
				c.pass.Reportf(cm.Pos(), "V010 //qlx:ignore needs a reason: say why this finding is acceptable")
				continue
			}
			m := c.ignores[p.Filename]
			if m == nil {
				m = map[int]*ignore{}
				c.ignores[p.Filename] = m
			}
			ig := &ignore{reason: reason, pos: cm.Pos()}
			m[p.Line] = ig
			m[p.Line+1] = ig
		}
	}
}

func (c *checker) report(n ast.Node, code, msg string) {
	p := c.pass.Fset.Position(n.Pos())
	// A call can be reached through more than one walk (the file walk and the
	// descent into nested queries); report each finding once.
	key := p.String() + code + msg
	if c.reported[key] {
		return
	}
	if c.reported == nil {
		c.reported = map[string]bool{}
	}
	c.reported[key] = true
	if ig := c.ignores[p.Filename][p.Line]; ig != nil {
		ig.used = true
		return
	}
	c.pass.Reportf(n.Pos(), "%s %s", code, msg)
}

// callee returns the qlx function or method a call resolves to, or nil.
func (c *checker) callee(call *ast.CallExpr) *types.Func {
	var id *ast.Ident
	switch f := ast.Unparen(call.Fun).(type) {
	case *ast.SelectorExpr:
		id = f.Sel
	case *ast.Ident:
		id = f
	case *ast.IndexExpr:
		switch x := f.X.(type) {
		case *ast.SelectorExpr:
			id = x.Sel
		case *ast.Ident:
			id = x
		}
	case *ast.IndexListExpr:
		if s, ok := f.X.(*ast.SelectorExpr); ok {
			id = s.Sel
		}
	}
	if id == nil {
		return nil
	}
	fn, _ := c.pass.TypesInfo.Uses[id].(*types.Func)
	if fn == nil || fn.Pkg() == nil || fn.Pkg().Path() != qlxPath {
		return nil
	}
	return fn
}

func isFunc(fn *types.Func, name string) bool {
	if fn == nil || fn.Name() != name {
		return false
	}
	sig, _ := fn.Type().(*types.Signature)
	return sig != nil && sig.Recv() == nil
}

func isQLXValue(t types.Type) bool {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	n, ok := t.(*types.Named)
	if !ok || n.Obj().Pkg() == nil {
		return false
	}
	return n.Obj().Pkg().Path() == qlxPath && n.Obj().Exported()
}

func (c *checker) file(f *ast.File) {
	var stack []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		switch n := n.(type) {
		case *ast.ExprStmt:
			c.discarded(n)
		case *ast.AssignStmt:
			c.ignoredError(n)
		case *ast.CompositeLit:
			c.gapLit(n)
		case *ast.CallExpr:
			c.register(n, stack)
			c.unboundedDML(n, stack)
			c.versionGate(n)
			c.shapeCall(n, stack)
			c.mappingCheck(n)
			c.nullAssign(n)
			c.gapCall(n, stack)
			if outermost(n, stack) {
				// A chain inside another QLX call is reached by that
				// call's own descent, with the outer query's tables in scope
				// (EXISTS and LATERAL subqueries use them); checking it again
				// here with an empty scope would report valid correlated
				// subqueries.
				if !c.insideQLXCall(stack) {
					c.chainChecks(n, nil)
				}
				c.shapeChain(n, stack)
				c.gapChain(n, stack)
			}
		}
		return true
	})
}

// outermost reports whether call is the last call of its method chain.
func outermost(call *ast.CallExpr, stack []ast.Node) bool {
	if len(stack) >= 2 {
		if sel, ok := stack[len(stack)-2].(*ast.SelectorExpr); ok && sel.X == ast.Expr(call) {
			return false
		}
	}
	return true
}

func (c *checker) discarded(s *ast.ExprStmt) {
	call, ok := ast.Unparen(s.X).(*ast.CallExpr)
	if !ok {
		return
	}
	fn := c.callee(call)
	if fn == nil || isFunc(fn, "Register") {
		return
	}
	if isFunc(fn, "SQL") || isFunc(fn, "Render") {
		c.report(call, "V004", "result of "+fn.Name()+" is discarded: the rendered statement and its error are lost")
		return
	}
	if tv, ok := c.pass.TypesInfo.Types[call]; ok && isQLXValue(tv.Type) {
		c.report(call, "V001", "result of "+fn.Name()+" is discarded: QLX builders are immutable, so this call has no effect (assign the result)")
	}
}

func (c *checker) ignoredError(a *ast.AssignStmt) {
	if len(a.Rhs) != 1 {
		return
	}
	call, ok := ast.Unparen(a.Rhs[0]).(*ast.CallExpr)
	if !ok {
		return
	}
	fn := c.callee(call)
	if !isFunc(fn, "SQL") && !isFunc(fn, "Render") {
		return
	}
	if id, ok := a.Lhs[len(a.Lhs)-1].(*ast.Ident); ok && id.Name == "_" {
		c.report(call, "V004", "the error from "+fn.Name()+" is ignored: an unbound parameter or a version-gated feature would go unnoticed")
	}
}

func (c *checker) register(call *ast.CallExpr, stack []ast.Node) {
	if !isFunc(c.callee(call), "Register") || len(call.Args) == 0 {
		return
	}
	tv := c.pass.TypesInfo.Types[call.Args[0]]
	if tv.Value == nil || tv.Value.Kind() != constant.String {
		c.report(call, "V003", "Register needs a constant string name so snapshots and `qlx prepare` output are stable")
	} else {
		name := constant.StringVal(tv.Value)
		pos := c.pass.Fset.Position(call.Pos())
		if first, dup := c.regs[name]; dup {
			c.report(call, "V003", "statement name \""+name+"\" is already registered at "+first.String()+": Register panics on duplicates")
		} else {
			c.regs[name] = pos
		}
	}
	for i := len(stack) - 2; i >= 0; i-- {
		switch fn := stack[i].(type) {
		case *ast.FuncLit:
			c.report(call, "V005", "Register inside a function literal: it runs every time the function does and panics the second time")
			return
		case *ast.FuncDecl:
			if fn.Name.Name != "init" || fn.Recv != nil {
				c.report(call, "V005", "Register inside function "+fn.Name.Name+": declare registered statements at package level (or in init) so the name is registered exactly once")
			}
			return
		}
	}
}

// link is one method call of a chain.
type link struct {
	name string
	call *ast.CallExpr
}

// chain unwinds X.A(..).B(..) into its root call and the links in call order.
func chain(call *ast.CallExpr) (root *ast.CallExpr, links []link) {
	cur := call
	for {
		sel, ok := ast.Unparen(cur.Fun).(*ast.SelectorExpr)
		if !ok {
			break
		}
		inner, ok := ast.Unparen(sel.X).(*ast.CallExpr)
		if !ok {
			break
		}
		links = append(links, link{sel.Sel.Name, cur})
		cur = inner
	}
	for i, j := 0, len(links)-1; i < j; i, j = i+1, j-1 {
		links[i], links[j] = links[j], links[i]
	}
	return cur, links
}

func hasLink(ls []link, names ...string) bool {
	for _, l := range ls {
		for _, n := range names {
			if l.name == n {
				return true
			}
		}
	}
	return false
}

func (c *checker) unboundedDML(call *ast.CallExpr, stack []ast.Node) {
	if !outermost(call, stack) {
		return
	}
	root, links := chain(call)
	fn := c.callee(root)
	kind := ""
	switch {
	case isFunc(fn, "Update"):
		kind = "UPDATE"
	case isFunc(fn, "DeleteFrom"):
		kind = "DELETE"
	default:
		return
	}
	if hasLink(links, "Where", "All") || !handedOn(call, stack) {
		return
	}
	c.report(call, "V002", kind+" chain has neither Where nor All; it will not render (add Where, or All() to mean every row)")
}

// ---- chain-level checks ----

type scope struct {
	tables  map[types.Object]bool
	unknown bool // some FROM item is not a plain table variable
	aliased bool // some FROM item is an alias written as T.As("name")
}

func (s *scope) clone() *scope {
	n := &scope{tables: map[types.Object]bool{}, unknown: s != nil && s.unknown, aliased: s != nil && s.aliased}
	if s != nil {
		for k := range s.tables {
			n.tables[k] = true
		}
	}
	return n
}

// tableVar names the table an expression stands for: a table variable, or an
// alias of one. `Users.As("u")` and a variable assigned once from it are the
// same table, so they resolve to one object; two different spellings of the
// same alias do not produce a false "not in scope". An alias whose name is not
// a constant, or a variable assigned more than once, is not resolved further
// than the variable itself.
func (c *checker) tableVar(e ast.Expr) types.Object {
	return c.tableVarDepth(e, 0)
}

type aliasKey struct {
	base  types.Object
	alias string
}

func (c *checker) tableVarDepth(e ast.Expr, depth int) types.Object {
	var obj types.Object
	switch x := ast.Unparen(e).(type) {
	case *ast.Ident: // dot import or same package
		obj = c.pass.TypesInfo.Uses[x]
	case *ast.SelectorExpr: // db.Users, the usual qualified import
		obj = c.pass.TypesInfo.Uses[x.Sel]
	case *ast.CallExpr:
		return c.aliasOf(x, depth)
	default:
		return nil
	}
	v, ok := obj.(*types.Var)
	if !ok {
		return nil
	}
	n, ok := v.Type().(*types.Named)
	if !ok || !embedsTableRef(n) {
		return nil
	}
	// u := Users.As("u"): the variable is that alias.
	if depth < 3 && !v.IsField() {
		if init := c.singleInit(v); init != nil {
			if call, ok := ast.Unparen(init).(*ast.CallExpr); ok {
				if a := c.aliasOf(call, depth+1); a != nil {
					return a
				}
			}
		} else if init := c.pkgVarInit(v); init != nil {
			if call, ok := ast.Unparen(init).(*ast.CallExpr); ok {
				if a := c.aliasOf(call, depth+1); a != nil {
					return a
				}
			}
		}
	}
	return v
}

// aliasOf resolves `T.As("name")` with a constant name on a table variable.
func (c *checker) aliasOf(call *ast.CallExpr, depth int) types.Object {
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "As" || len(call.Args) != 1 || depth > 3 {
		return nil
	}
	tv := c.pass.TypesInfo.Types[call.Args[0]]
	if tv.Value == nil || tv.Value.Kind() != constant.String {
		return nil
	}
	base := c.tableVarDepth(sel.X, depth+1)
	if base == nil {
		return nil
	}
	if c.isAliasObj(base) {
		return nil // an alias of an alias is not a table qlx lets you write
	}
	key := aliasKey{base, constant.StringVal(tv.Value)}
	if c.aliases == nil {
		c.aliases = map[aliasKey]types.Object{}
	}
	if o, ok := c.aliases[key]; ok {
		return o
	}
	o := types.NewVar(token.NoPos, c.pass.Pkg, base.Name()+".As("+strconv.Quote(key.alias)+")", base.Type())
	c.aliases[key] = o
	return o
}

var scopeAdders = map[string]bool{"From": true, "Join": true, "LeftJoin": true, "RightJoin": true, "FullJoin": true, "CrossJoin": true, "Using": true}

func (c *checker) chainChecks(call *ast.CallExpr, outer *scope) {
	root, links := chain(call)
	fn := c.callee(root)
	switch {
	case isFunc(fn, "Select"), isFunc(fn, "Update"), isFunc(fn, "DeleteFrom"):
		c.scopeCheck(root, links, fn, outer)
	case isFunc(fn, "InsertInto"):
		c.requiredCheck(root, links)
		c.nested(root, links, outer)
		return
	default:
		if isFunc(fn, "Lateral") {
			// A LATERAL subquery may be built before the query that joins it;
			// the tables it correlates with are not known here.
			outer = &scope{tables: map[types.Object]bool{}, unknown: true}
		}
		c.nested(root, links, outer)
		return
	}
	if isFunc(fn, "Select") {
		c.groupByCheck(root, links)
	}
}

// nested recurses into chains found in the arguments of a chain.
func (c *checker) nested(root *ast.CallExpr, links []link, outer *scope) {
	visit := func(args []ast.Expr) {
		for _, a := range args {
			c.descend(a, outer)
		}
	}
	visit(root.Args)
	for _, l := range links {
		visit(l.call.Args)
	}
}

func (c *checker) descend(e ast.Expr, outer *scope) {
	ast.Inspect(e, func(n ast.Node) bool {
		cl, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		r, _ := chain(cl)
		fn := c.callee(r)
		if isFunc(fn, "Select") || isFunc(fn, "Update") || isFunc(fn, "DeleteFrom") || isFunc(fn, "InsertInto") {
			c.chainChecks(cl, outer)
			return false
		}
		return true
	})
}

func (c *checker) scopeCheck(root *ast.CallExpr, links []link, fn *types.Func, outer *scope) {
	sc := outer.clone()
	if !isFunc(fn, "Select") && len(root.Args) > 0 {
		if v := c.tableVar(root.Args[0]); v != nil {
			sc.tables[v] = true
		} else {
			sc.unknown = true
		}
	}
	for _, l := range links {
		if !scopeAdders[l.name] {
			continue
		}
		for _, a := range l.call.Args {
			if v := c.tableVar(a); v != nil {
				sc.tables[v] = true
				if c.isAliasObj(v) {
					sc.aliased = true
				}
			} else if a != nil {
				// subquery, CTE, function table or aliased expression
				sc.unknown = true
			}
		}
	}
	check := func(args []ast.Expr) {
		for _, a := range args {
			ast.Inspect(a, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CallExpr:
					r, _ := chain(n)
					rf := c.callee(r)
					if isFunc(rf, "Select") || isFunc(rf, "Update") || isFunc(rf, "DeleteFrom") || isFunc(rf, "InsertInto") {
						c.chainChecks(n, sc)
						return false
					}
				case *ast.SelectorExpr:
					if sc.unknown {
						return true
					}
					if !c.isFieldSel(n) {
						return true // a method value such as Users.As, not a column
					}
					if v := c.tableVar(n.X); v != nil && !sc.tables[v] && !c.declaredInside(v, root) {
						if sc.aliased && !c.isKnownTable(v) {
							return true // an alias is in scope and v may be spelled the same: not sure
						}
						c.report(n, "V006", "column "+types.ExprString(n)+" belongs to table variable "+v.Name()+", which is not in this query's FROM/JOIN scope")
					}
				}
				return true
			})
		}
	}
	check(root.Args)
	for _, l := range links {
		if scopeAdders[l.name] {
			// join items may be nested queries; still descend for those
			for _, a := range l.call.Args {
				c.descend(a, sc)
			}
			if l.name == "Join" || strings.HasSuffix(l.name, "Join") {
				continue
			}
			continue
		}
		check(l.call.Args)
	}
}

// declaredInside is true for variables declared within the chain itself
// (function literals in arguments), which are not outer tables.
func (c *checker) declaredInside(v types.Object, root *ast.CallExpr) bool {
	return v.Pos() >= root.Pos() && v.Pos() <= root.End()
}

var aggregates = map[string]bool{
	"CountAll": true, "Count": true, "CountDistinct": true, "Sum": true, "SumBig": true, "SumFloat": true,
	"Avg": true, "AvgFloat": true, "Max": true, "Min": true, "BoolAnd": true, "BoolOr": true,
	"ArrayAgg": true, "StringAgg": true,
}

// V007: SELECT agg(...), t.col with no GROUP BY. Window functions are not
// aggregates here: they are built with .Over(...), which is not a bare call.
func (c *checker) groupByCheck(root *ast.CallExpr, links []link) {
	if hasLink(links, "GroupBy", "GroupByAll", "Windows") {
		return
	}
	hasAgg, hasBare := false, false
	var bare ast.Node
	for _, a := range root.Args {
		a = ast.Unparen(a)
		switch x := a.(type) {
		case *ast.CallExpr:
			if sel, ok := ast.Unparen(x.Fun).(*ast.SelectorExpr); ok {
				if _, nested := ast.Unparen(sel.X).(*ast.CallExpr); nested {
					// method on a call (e.g. Sum(...).As("x") or .Over(...))
					r, ls := chain(x)
					if aggregates[funcName(c, r)] && !hasLink(ls, "Over", "Filter") {
						hasAgg = true
					}
					if aggregates[funcName(c, r)] && hasLink(ls, "Over") {
						continue
					}
					continue
				}
			}
			if aggregates[funcName(c, x)] {
				hasAgg = true
			}
		case *ast.SelectorExpr:
			if c.isColumn(x) {
				hasBare = true
				bare = x
			}
		}
	}
	if hasAgg && hasBare {
		c.report(bare, "V007", "column "+types.ExprString(bare.(ast.Expr))+" is selected next to an aggregate but the query has no GroupBy")
	}
}

func funcName(c *checker, call *ast.CallExpr) string {
	if fn := c.callee(call); fn != nil {
		return fn.Name()
	}
	return ""
}

func (c *checker) isColumn(s *ast.SelectorExpr) bool {
	return c.isFieldSel(s) && c.tableVar(s.X) != nil
}

// isFieldSel reports whether a selector names a struct field (a column), not a
// method (Users.As).
func (c *checker) isFieldSel(s *ast.SelectorExpr) bool {
	_, ok := c.pass.TypesInfo.Uses[s.Sel].(*types.Var)
	return ok
}

func (c *checker) isAliasObj(o types.Object) bool {
	for _, a := range c.aliases {
		if a == o {
			return true
		}
	}
	return false
}

// isKnownTable: a table symbol the analyzer can compare by identity, which is
// a package-level variable (the generated Users, db.Orders) or an alias
// resolved from T.As("name"). A parameter, a struct field or a variable
// assigned more than once may be the same table under another name.
func (c *checker) isKnownTable(o types.Object) bool {
	if c.isAliasObj(o) {
		return true
	}
	return o.Parent() != nil && o.Parent().Parent() == types.Universe
}

// V008: INSERT rows must set every column the schema says is required.
func (c *checker) requiredCheck(root *ast.CallExpr, links []link) {
	if hasLink(links, "DefaultValues", "FromQuery", "Values", "Columns") || len(root.Args) == 0 {
		return
	}
	tv, ok := c.pass.TypesInfo.Types[root.Args[0]]
	if !ok {
		return
	}
	named, ok := tv.Type.(*types.Named)
	if !ok {
		return
	}
	var tf TableFact
	if !c.pass.ImportObjectFact(named.Obj(), &tf) || len(tf.Required) == 0 {
		return
	}
	for _, l := range links {
		if l.name != "Row" {
			continue
		}
		have := map[string]bool{}
		maybe := map[string]string{} // required column -> ToIf / ToOrDefault that may leave it out
		complete := true
		for _, a := range l.call.Args {
			ac, ok := ast.Unparen(a).(*ast.CallExpr)
			if !ok {
				complete = false
				break
			}
			sel, ok := ast.Unparen(ac.Fun).(*ast.SelectorExpr)
			if !ok {
				complete = false
				break
			}
			fld, ok := ast.Unparen(sel.X).(*ast.SelectorExpr)
			if !ok {
				complete = false
				break
			}
			col, ok := tf.Cols[fld.Sel.Name]
			if !ok {
				complete = false
				break
			}
			have[col] = true
			if m := sel.Sel.Name; m == "ToIf" || m == "ToOrDefault" {
				maybe[col] = m
			}
		}
		if !complete {
			continue
		}
		var missing []string
		for _, r := range tf.Required {
			if !have[r] {
				missing = append(missing, r)
			}
		}
		if len(missing) > 0 {
			c.report(l.call, "V008", "INSERT INTO "+tf.Name+" does not set required column(s) "+strings.Join(missing, ", ")+" (NOT NULL without default)")
		}
		for _, r := range tf.Required {
			if m, ok := maybe[r]; ok {
				c.report(l.call, "V008", "INSERT INTO "+tf.Name+" sets required column "+r+" with "+m+", which leaves it out or writes DEFAULT when the option is unset (NOT NULL without default): use To")
			}
		}
	}
}

// V009: a function with a version range must cover every target.
func (c *checker) versionGate(call *ast.CallExpr) {
	if len(c.targets) == 0 {
		return
	}
	var id *ast.Ident
	switch f := ast.Unparen(call.Fun).(type) {
	case *ast.SelectorExpr:
		id = f.Sel
	case *ast.Ident:
		id = f
	case *ast.IndexExpr: // explicit instantiation: Uuidv7[uuid.UUID]()
		switch x := f.X.(type) {
		case *ast.SelectorExpr:
			id = x.Sel
		case *ast.Ident:
			id = x
		}
	case *ast.IndexListExpr:
		if s, ok := f.X.(*ast.SelectorExpr); ok {
			id = s.Sel
		}
	}
	if id == nil {
		return
	}
	obj, ok := c.pass.TypesInfo.Uses[id].(*types.Func)
	if !ok {
		return
	}
	obj = obj.Origin() // methods of instantiated generic types carry the fact on their origin
	var vf VersionFact
	if !c.pass.ImportObjectFact(obj, &vf) {
		return
	}
	lo, hi := c.targets[0], c.targets[len(c.targets)-1]
	if vf.Since > 0 && lo < vf.Since {
		c.report(call, "V009", obj.Name()+" requires PostgreSQL "+strconv.Itoa(vf.Since)+" but the targets include "+strconv.Itoa(lo))
	}
	if vf.Until > 0 && hi >= vf.Until {
		c.report(call, "V009", obj.Name()+" was removed in PostgreSQL "+strconv.Itoa(vf.Until)+" but the targets include "+strconv.Itoa(hi))
	}
}
