package qlxvet

// Checks for mistakes that Render rejects (ErrShape) or the server rejects at
// PREPARE, and that are visible in the source:
//
//	GRANT/REVOKE missing or misdirected parts, REFRESH CONCURRENTLY with NO DATA
//	table functions without As, JSON_TABLE/XMLTable without alias or columns
//	JSON path variables that are not passed, QUOTES with a WITH WRAPPER
//	empty JSONPair, SET CONSTRAINTS without a name
//	SearchOrder/CycleMark on a CTE without SEARCH/CYCLE
//	window functions in WHERE/HAVING/ON/GROUP BY, named windows never defined
//	conflict target or assignment naming a column of another table
//	COPY option values newer than -targets
//
// The same two rules as shape.go apply: present things are reported for any
// chain, absent things only when the chain is handed on, and whatever cannot
// be resolved keeps the check silent.

import (
	"go/ast"
	"go/constant"
	"go/types"
	"strings"
)

// V019 window function where none is allowed, V020 named window never
// defined, V021 table function or JSON/XML table without its naming, V022
// JSON path variable not passed or QUOTES with WRAPPER, V023 GRANT/REVOKE
// shape, V024 REFRESH and SET CONSTRAINTS shape, V025 SearchOrder/CycleMark
// without SEARCH/CYCLE, V026 column of another table as target.

// recvName is the name of the type a qlx method is declared on.
func recvName(fn *types.Func) string {
	sig, _ := fn.Type().(*types.Signature)
	if sig == nil || sig.Recv() == nil {
		return ""
	}
	t := sig.Recv().Type()
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	if n, ok := t.(*types.Named); ok {
		return n.Obj().Name()
	}
	return ""
}

func (c *checker) qlxConst(e ast.Expr) string {
	var obj types.Object
	switch x := ast.Unparen(e).(type) {
	case *ast.Ident:
		obj = c.pass.TypesInfo.Uses[x]
	case *ast.SelectorExpr:
		obj = c.pass.TypesInfo.Uses[x.Sel]
	}
	if k, ok := obj.(*types.Const); ok && k.Pkg() != nil && k.Pkg().Path() == qlxPath {
		return k.Name()
	}
	return ""
}

// constString is the value of a constant string expression.
func (c *checker) constString(e ast.Expr) (string, bool) {
	tv := c.pass.TypesInfo.Types[e]
	if tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}

// isQueryExpr reports whether e produces a query (a nested statement), whose
// own windows and clauses are judged separately.
func (c *checker) isQueryExpr(e ast.Expr) bool {
	t := c.pass.TypesInfo.TypeOf(e)
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	n, ok := t.(*types.Named)
	if !ok || n.Obj().Pkg() == nil || n.Obj().Pkg().Path() != qlxPath {
		return false
	}
	return strings.HasSuffix(n.Obj().Name(), "Query") || n.Obj().Name() == "CTE"
}

// walkOuter visits n without entering nested queries or function literals.
func (c *checker) walkOuter(n ast.Node, f func(ast.Node)) {
	ast.Inspect(n, func(x ast.Node) bool {
		switch x := x.(type) {
		case *ast.FuncLit:
			return false
		case *ast.CallExpr:
			if c.isQueryExpr(x) {
				return false
			}
		}
		f(x)
		return true
	})
}

// ---------------------------------------------------------------------------
// single-assignment variables, to follow `cte := NewCTE(...).Cycle(...)`
// ---------------------------------------------------------------------------

func (c *checker) singleInit(obj types.Object) ast.Expr {
	if c.assigns == nil {
		c.assigns = map[types.Object]ast.Expr{}
		c.reassigned = map[types.Object]bool{}
		for _, f := range c.pass.Files {
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.ValueSpec:
					if len(n.Names) == len(n.Values) {
						for i, id := range n.Names {
							if o := c.pass.TypesInfo.Defs[id]; o != nil {
								c.assigns[o] = n.Values[i]
							}
						}
					}
				case *ast.AssignStmt:
					for i, l := range n.Lhs {
						id, ok := l.(*ast.Ident)
						if !ok {
							continue
						}
						if n.Tok.String() == ":=" {
							if o := c.pass.TypesInfo.Defs[id]; o != nil {
								if len(n.Lhs) == len(n.Rhs) {
									c.assigns[o] = n.Rhs[i]
								}
								continue
							}
						}
						if o := c.pass.TypesInfo.Uses[id]; o != nil {
							c.reassigned[o] = true
						}
					}
				case *ast.UnaryExpr:
					if n.Op.String() == "&" {
						if id, ok := ast.Unparen(n.X).(*ast.Ident); ok {
							if o := c.pass.TypesInfo.Uses[id]; o != nil {
								c.reassigned[o] = true
							}
						}
					}
				}
				return true
			})
		}
	}
	if c.reassigned[obj] {
		return nil
	}
	return c.assigns[obj]
}

// ---------------------------------------------------------------------------
// literals
// ---------------------------------------------------------------------------

func (c *checker) gapLit(lit *ast.CompositeLit) {
	n, ok := c.pass.TypesInfo.TypeOf(lit).(*types.Named)
	if !ok || n.Obj().Pkg() == nil || n.Obj().Pkg().Path() != qlxPath {
		return
	}
	if n.Obj().Name() == "JSONPair" && len(lit.Elts) == 0 {
		c.report(lit, "V012", "empty JSONPair has no key and no value: build it with JSONKV(key, value)")
	}
}

// ---------------------------------------------------------------------------
// single calls
// ---------------------------------------------------------------------------

var copyOptionSince = map[string]int{
	"OnErrorIgnore": 17, "OnErrorSetNull": 19, "LogVerbose": 17, "LogSilent": 18, "CopyJSON": 19,
}

func (c *checker) gapCall(call *ast.CallExpr, stack []ast.Node) {
	fn := c.callee(call)
	if fn == nil {
		return
	}
	switch {
	case isFunc(fn, "SetConstraints"):
		if len(call.Args) == 1 && !spread(call) {
			c.report(call, "V024", "SetConstraints needs at least one constraint name (SetConstraintsAll covers every constraint)")
		}
	case isFunc(fn, "JSONTable"), isFunc(fn, "XMLTable"):
		if !spread(call) && len(call.Args) == 3 {
			c.report(call, "V021", fn.Name()+" needs at least one column")
		}
		if len(call.Args) >= 3 {
			if s, ok := c.constString(call.Args[2]); ok && s == "" {
				c.report(call.Args[2], "V021", fn.Name()+" needs a non-empty alias")
			}
		}
	case isFunc(fn, "WithOnError"), isFunc(fn, "WithLogVerbosity"), isFunc(fn, "WithFormat"):
		if len(call.Args) == 1 {
			if since := copyOptionSince[c.qlxConst(call.Args[0])]; since > 0 {
				c.needsVersion(call, "COPY option "+c.qlxConst(call.Args[0]), since)
			}
		}
	case fn.Name() == "SearchOrder" && recvName(fn) == "CTE", fn.Name() == "CycleMark" && recvName(fn) == "CTE":
		c.cteClause(call, fn.Name())
	case recvName(fn) != "" && (fn.Name() == "Where" || fn.Name() == "Having" || fn.Name() == "On" || fn.Name() == "GroupBy"):
		for _, a := range call.Args {
			c.walkOuter(a, func(n ast.Node) {
				ce, ok := n.(*ast.CallExpr)
				if !ok {
					return
				}
				if f := c.callee(ce); f != nil && f.Name() == "Over" && recvName(f) != "" {
					c.report(ce, "V019", "window function in "+strings.ToUpper(fn.Name())+": PostgreSQL does not allow it there; compute it in a subquery or CTE and filter on the result")
				}
			})
		}
	}
}

// cteClause: SearchOrder/CycleMark on a CTE built without SEARCH/CYCLE.
func (c *checker) cteClause(call *ast.CallExpr, name string) {
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return
	}
	x := ast.Unparen(sel.X)
	if id, ok := x.(*ast.Ident); ok {
		obj := c.pass.TypesInfo.Uses[id]
		if obj == nil {
			return
		}
		x = nil
		if init := c.singleInit(obj); init != nil {
			x = ast.Unparen(init)
		}
	}
	ce, ok := x.(*ast.CallExpr)
	if !ok {
		return
	}
	root, links := chain(ce)
	rf := c.callee(root)
	if spread(root) || !(isFunc(rf, "NewCTE") || isFunc(rf, "Recursive") || isFunc(rf, "RecursiveDistinct")) {
		return
	}
	for _, l := range links {
		if spread(l.call) {
			return
		}
	}
	if name == "SearchOrder" && !hasLink(links, "SearchDepthFirst", "SearchBreadthFirst") {
		c.report(call, "V025", "SearchOrder on a CTE built without SearchDepthFirst or SearchBreadthFirst")
	}
	if name == "CycleMark" && !hasLink(links, "Cycle") {
		c.report(call, "V025", "CycleMark on a CTE built without Cycle")
	}
}

// ---------------------------------------------------------------------------
// chains
// ---------------------------------------------------------------------------

func allEmpty(ls []link) bool {
	for _, l := range ls {
		if !noArgs(l.call) {
			return false
		}
	}
	return len(ls) > 0
}

func (c *checker) gapChain(call *ast.CallExpr, stack []ast.Node) {
	root, links := chain(call)
	fn := c.callee(root)
	if fn == nil {
		return
	}
	complete := handedOn(call, stack)
	switch {
	case isFunc(fn, "Grant"), isFunc(fn, "Revoke"):
		c.grantShape(fn.Name() == "Revoke", root, links, complete)
	case isFunc(fn, "RefreshMaterializedView"):
		if hasLink(links, "Concurrently") && hasLink(links, "WithNoData") {
			c.report(findLinks(links, "WithNoData")[0].call, "V024", "REFRESH MATERIALIZED VIEW cannot combine Concurrently and WithNoData")
		}
	case isFunc(fn, "Func"), isFunc(fn, "FuncSince"), isFunc(fn, "FuncRange"):
		if complete && !hasLink(links, "As") {
			c.report(root, "V021", fn.Name()+" needs As(alias, columns...): the columns of a table function are named by As")
		}
	case isFunc(fn, "JSONExists"), isFunc(fn, "JSONValue"), isFunc(fn, "JSONQuery"), isFunc(fn, "JSONTable"):
		c.jsonPassing(fn, root, links, complete)
		if isFunc(fn, "JSONQuery") && hasLink(links, "KeepQuotes", "OmitQuotes") && hasLink(links, "WithWrapper", "WithConditionalWrapper") {
			l := findLinks(links, "KeepQuotes")
			if len(l) == 0 {
				l = findLinks(links, "OmitQuotes")
			}
			c.report(l[0].call, "V022", "KeepQuotes/OmitQuotes cannot be combined with a WITH WRAPPER clause: the server rejects it")
		}
	case isFunc(fn, "Select"):
		c.windowScope(root, links, complete)
	case isFunc(fn, "InsertInto"):
		c.foreignTargets(root, links)
	case isFunc(fn, "Update"):
		c.foreignTargets(root, links)
	}
}

func (c *checker) grantShape(revoke bool, root *ast.CallExpr, links []link, complete bool) {
	verb, dir, wrong := "GRANT", "To", "From"
	if revoke {
		verb, dir, wrong = "REVOKE", "From", "To"
	}
	if noArgs(root) {
		c.report(root, "V023", verb+" needs at least one privilege")
	}
	if l := findLinks(links, wrong); len(l) > 0 {
		c.report(l[0].call, "V023", verb+" names its roles with "+dir+", not "+wrong)
	}
	if revoke && hasLink(links, "WithGrantOption") {
		c.report(findLinks(links, "WithGrantOption")[0].call, "V023", "WithGrantOption applies to GRANT only")
	}
	if !revoke && hasLink(links, "Cascade") {
		c.report(findLinks(links, "Cascade")[0].call, "V023", "Cascade applies to REVOKE only")
	}
	for _, name := range []string{"OnTables", dir} {
		ls := findLinks(links, name)
		if allEmpty(ls) {
			c.report(ls[0].call, "V023", verb+" "+name+" has no argument")
		}
	}
	if complete {
		if !hasLink(links, "OnTables") {
			c.report(root, "V023", verb+" needs OnTables")
		}
		if !hasLink(links, dir) && !hasLink(links, wrong) {
			c.report(root, "V023", verb+" needs "+dir+" (the roles)")
		}
	}
}

// jsonPassing: every $variable of the path must be bound by Passing.
func (c *checker) jsonPassing(fn *types.Func, root *ast.CallExpr, links []link, complete bool) {
	if !complete || spread(root) || len(root.Args) < 2 {
		return
	}
	pathArgs := []ast.Expr{root.Args[1]}
	if fn.Name() == "JSONTable" && len(root.Args) > 3 {
		pathArgs = append(pathArgs, root.Args[3:]...)
	}
	used := map[string]ast.Expr{}
	unknown := false
	// scan reads the Var("x") calls of a path expression, following a path
	// held in a variable that is assigned exactly once.
	var scan func(root ast.Expr, e ast.Expr, depth int)
	scan = func(at ast.Expr, e ast.Expr, depth int) {
		ast.Inspect(e, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				if f := c.callee(n); isFunc(f, "Var") && len(n.Args) == 1 {
					if s, ok := c.constString(n.Args[0]); ok {
						used[s] = at
					} else {
						unknown = true
					}
				}
			case *ast.Ident:
				if !isNamed(c.pass.TypesInfo.TypeOf(n), "JSONPath") {
					return true
				}
				v, isVar := c.pass.TypesInfo.Uses[n].(*types.Var)
				if !isVar {
					return true
				}
				if init := c.singleInit(v); init != nil && depth < 4 {
					scan(at, init, depth+1)
				} else {
					unknown = true // a parameter, a field or a reassigned variable: its steps are not visible
				}
			}
			return true
		})
	}
	for _, a := range pathArgs {
		scan(a, a, 0)
	}
	if unknown || len(used) == 0 {
		return
	}
	passed := map[string]bool{}
	var names []string
	for _, l := range links {
		if l.name != "Passing" {
			continue
		}
		if spread(l.call) {
			return
		}
		for _, a := range l.call.Args {
			if id, isID := ast.Unparen(a).(*ast.Ident); isID {
				if v, isVar := c.pass.TypesInfo.Uses[id].(*types.Var); isVar {
					if init := c.singleInit(v); init != nil {
						a = init // pass := q.Pass("x", n)
					}
				}
			}
			pc, ok := ast.Unparen(a).(*ast.CallExpr)
			if !ok || !isFunc(c.callee(pc), "Pass") || len(pc.Args) < 1 {
				return
			}
			s, ok := c.constString(pc.Args[0])
			if !ok {
				return
			}
			passed[s] = true
			names = append(names, s)
		}
	}
	if d := firstDup(names); d != "" {
		c.report(root, "V013", "path variable $"+d+" is passed twice")
	}
	for name, at := range used {
		if !passed[name] {
			c.report(at, "V022", "path variable $"+name+" is not passed: add Passing(Pass(\""+name+"\", value))")
		}
	}
}

func isNamed(t types.Type, name string) bool {
	n, ok := t.(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == qlxPath && n.Obj().Name() == name
}

// windowScope: Over(w.Spec()) or Over(w.Extend()) where w is not in this
// query's Windows(...).
func (c *checker) windowScope(root *ast.CallExpr, links []link, complete bool) {
	if !complete || spread(root) {
		return
	}
	defined := map[types.Object]bool{}
	for _, l := range links {
		if l.name != "Windows" {
			continue
		}
		if spread(l.call) {
			return
		}
		for _, a := range l.call.Args {
			o := c.varObj(a)
			if o == nil {
				return // inline NewWindow or a helper: unknown
			}
			defined[o] = true
		}
	}
	check := func(a ast.Expr) {
		c.walkOuter(a, func(n ast.Node) {
			ce, ok := n.(*ast.CallExpr)
			if !ok {
				return
			}
			f := c.callee(ce)
			if f == nil || recvName(f) != "NamedWindow" || (f.Name() != "Spec" && f.Name() != "Extend") {
				return
			}
			sel, ok := ast.Unparen(ce.Fun).(*ast.SelectorExpr)
			if !ok {
				return
			}
			if o := c.varObj(sel.X); o != nil && !defined[o] {
				c.report(ce, "V020", "window "+types.ExprString(sel.X)+" is not defined in this query: add it with Windows("+types.ExprString(sel.X)+")")
			}
		})
	}
	for _, a := range root.Args {
		check(a)
	}
	for _, l := range links {
		if l.name == "Windows" {
			continue
		}
		for _, a := range l.call.Args {
			check(a)
		}
	}
}

func (c *checker) varObj(e ast.Expr) types.Object {
	switch x := ast.Unparen(e).(type) {
	case *ast.Ident:
		if v, ok := c.pass.TypesInfo.Uses[x].(*types.Var); ok {
			return v
		}
	case *ast.SelectorExpr:
		if v, ok := c.pass.TypesInfo.Uses[x.Sel].(*types.Var); ok {
			return v
		}
	}
	return nil
}

// comparable: two table symbols can be told apart by identity. When the write
// target is an alias, a symbol that is not a package-level table or another
// alias (a parameter, a field) may be that same alias under another name.
func (c *checker) comparable(target, other types.Object) bool {
	if c.isAliasObj(target) {
		return c.isKnownTable(other)
	}
	return true
}

// foreignTargets: a conflict column or an assignment that names a column of
// a table other than the one written to.
func (c *checker) foreignTargets(root *ast.CallExpr, links []link) {
	if len(root.Args) != 1 || spread(root) {
		return
	}
	tbl := c.tableVar(root.Args[0])
	if tbl == nil {
		return // alias or subquery: not resolvable
	}
	colTable := func(col ast.Expr) types.Object {
		if s, ok := ast.Unparen(col).(*ast.SelectorExpr); ok {
			return c.tableVar(s.X)
		}
		return nil
	}
	assigns := func(what string, args []ast.Expr) {
		for _, a := range args {
			ac, ok := ast.Unparen(a).(*ast.CallExpr)
			if !ok {
				continue
			}
			sel, ok := ast.Unparen(ac.Fun).(*ast.SelectorExpr)
			if !ok || c.callee(ac) == nil {
				continue
			}
			switch sel.Sel.Name {
			case "To", "ToX", "ToDefault":
				if t := colTable(sel.X); t != nil && t != tbl && c.comparable(tbl, t) {
					c.report(a, "V026", what+" assigns "+types.ExprString(sel.X)+", a column of "+t.Name()+" while the statement writes to "+tbl.Name())
				}
			}
		}
	}
	for _, l := range links {
		switch l.name {
		case "Set", "Row", "DoUpdate":
			assigns(l.name, l.call.Args)
		case "DoUpdateWhere":
			if len(l.call.Args) > 1 {
				assigns(l.name, l.call.Args[1:])
			}
		case "OnConflict":
			for _, a := range l.call.Args {
				if t := colTable(a); t != nil && t != tbl && c.comparable(tbl, t) {
					c.report(a, "V026", "conflict column "+types.ExprString(a)+" belongs to "+t.Name()+", not to "+tbl.Name())
				}
			}
		}
	}
}
