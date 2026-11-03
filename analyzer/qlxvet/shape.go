package qlxvet

// Shape checks: mistakes that render rejects (ErrShape, ErrScope) but that are
// visible in the source, so they can be reported at build time. Every check
// follows two rules.
//
//  1. A finding about something that is *present* (a column assigned twice,
//     Limit together with FetchFirst) holds for any chain, however it is built.
//  2. A finding about something that is *absent* (an UPDATE with no Set) holds
//     only when the chain is handed straight to something (an argument, a
//     composite literal, a return from a function whose result is the
//     Statement interface). A chain assigned to a variable, or returned as a
//     builder, may be completed later, so it is left alone, exactly like V002.
//
// Anything the analyzer cannot resolve (spread arguments, helper functions that
// build assignments, aliased tables) makes a check stay silent, never guess.

import (
	"go/ast"
	"go/constant"
	"go/types"
	"strconv"
)

// V012 missing part, V013 contradictory or duplicated parts, V014 group by
// coverage, V015 EXPLAIN option rules, V016 lock scope, V017 CTE SEARCH/CYCLE.

func spread(call *ast.CallExpr) bool { return call.Ellipsis.IsValid() }

// noArgs reports a call that certainly passes nothing to its variadic tail.
func noArgs(call *ast.CallExpr) bool { return len(call.Args) == 0 && !spread(call) }

// handedOn reports whether call, the outermost call of a chain, is passed on
// as an argument, returned, or stored in a composite literal: the situations
// where nothing can be added to it afterwards.
func handedOn(call *ast.CallExpr, stack []ast.Node) bool {
	if len(stack) < 2 {
		return false
	}
	switch parent := stack[len(stack)-2].(type) {
	case *ast.CallExpr:
		for _, a := range parent.Args {
			if ast.Unparen(a) == ast.Expr(call) {
				return true
			}
		}
		return false
	case *ast.ReturnStmt:
		return returnsStatement(call, parent, stack)
	case *ast.CompositeLit, *ast.KeyValueExpr:
		return true
	}
	return false
}

// returnsStatement reports whether a return hands the chain over for good:
// the function's result is the Statement interface, which nothing can be added
// to. A helper that returns the builder itself (SelectQuery, UpdateQuery,
// TableFunc, ...) leaves the rest of the chain to its caller, so it is not
// judged here. When the result type cannot be read from the source, the answer
// is no: the check stays silent.
func returnsStatement(call *ast.CallExpr, ret *ast.ReturnStmt, stack []ast.Node) bool {
	idx := -1
	for i, r := range ret.Results {
		if ast.Unparen(r) == ast.Expr(call) {
			idx = i
		}
	}
	if idx < 0 {
		return false
	}
	var results *ast.FieldList
	for i := len(stack) - 1; i >= 0 && results == nil; i-- {
		switch f := stack[i].(type) {
		case *ast.FuncDecl:
			results = f.Type.Results
			i = -1
		case *ast.FuncLit:
			results = f.Type.Results
			i = -1
		}
		if i == -1 {
			break
		}
	}
	if results == nil {
		return false
	}
	n := 0
	for _, f := range results.List {
		w := len(f.Names)
		if w == 0 {
			w = 1
		}
		if idx < n+w {
			switch t := f.Type.(type) {
			case *ast.SelectorExpr:
				return t.Sel.Name == "Statement"
			case *ast.Ident:
				return t.Name == "Statement"
			}
			return false
		}
		n += w
	}
	return false
}

// insideQLXCall reports whether an enclosing call belongs to QLX, so the
// query may be a subquery whose scope includes the outer query's tables.
func (c *checker) insideQLXCall(stack []ast.Node) bool {
	for i := len(stack) - 2; i >= 0; i-- {
		if ce, ok := stack[i].(*ast.CallExpr); ok && c.callee(ce) != nil {
			return true
		}
	}
	return false
}

func findLinks(ls []link, name string) []link {
	var out []link
	for _, l := range ls {
		if l.name == name {
			out = append(out, l)
		}
	}
	return out
}

// assignKeys returns a key per recognisable assignment (Col.To, ToX,
// ToDefault) in args, and whether every argument was recognised.
func (c *checker) assignKeys(args []ast.Expr) (keys []string, all bool) {
	all = true
	for _, a := range args {
		call, ok := ast.Unparen(a).(*ast.CallExpr)
		if !ok {
			all = false
			continue
		}
		sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
		if !ok || c.callee(call) == nil {
			all = false
			continue
		}
		switch sel.Sel.Name {
		case "To", "ToX", "ToDefault":
			keys = append(keys, types.ExprString(ast.Unparen(sel.X)))
		default:
			all = false
		}
	}
	return keys, all
}

func firstDup(keys []string) string {
	seen := map[string]bool{}
	for _, k := range keys {
		if seen[k] {
			return k
		}
		seen[k] = true
	}
	return ""
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]int{}
	for _, k := range a {
		m[k]++
	}
	for _, k := range b {
		m[k]--
	}
	for _, n := range m {
		if n != 0 {
			return false
		}
	}
	return true
}

// outWidth is the number of columns a query expression produces, when it is
// written as Select(a, b, ...) or a set operation over such selects.
func (c *checker) outWidth(e ast.Expr) (int, bool) {
	call, ok := ast.Unparen(e).(*ast.CallExpr)
	if !ok {
		return 0, false
	}
	root, _ := chain(call)
	fn := c.callee(root)
	switch {
	case isFunc(fn, "Select"):
		if spread(root) {
			return 0, false
		}
		return len(root.Args), true
	case isFunc(fn, "Union"), isFunc(fn, "Intersect"), isFunc(fn, "Except"):
		if len(root.Args) == 2 && !spread(root) {
			return c.outWidth(root.Args[0])
		}
	}
	return 0, false
}

var joinMethods = map[string]bool{"Join": true, "LeftJoin": true, "RightJoin": true, "FullJoin": true, "CrossJoin": true}

// shapeChain checks one complete method chain.
func (c *checker) shapeChain(call *ast.CallExpr, stack []ast.Node) {
	root, links := chain(call)
	fn := c.callee(root)
	if fn == nil {
		return
	}
	complete := handedOn(call, stack)
	switch {
	case isFunc(fn, "Select"):
		c.shapeSelect(root, links, complete, stack)
	case isFunc(fn, "Update"):
		c.shapeUpdate(root, links, complete)
	case isFunc(fn, "InsertInto"):
		c.shapeInsert(root, links, complete)
	case isFunc(fn, "Merge"):
		c.shapeMerge(root, links, complete)
	case isFunc(fn, "Case"):
		c.shapeCase(root, links)
	case isFunc(fn, "NewCTE"):
		for _, l := range links {
			switch l.name {
			case "SearchDepthFirst", "SearchBreadthFirst", "Cycle":
				c.report(l.call, "V017", l.name+" needs a recursive CTE: build it with Recursive or RecursiveDistinct, NewCTE makes an ordinary one")
			}
		}
	}
}

func (c *checker) shapeSelect(root *ast.CallExpr, links []link, complete bool, stack []ast.Node) {
	if noArgs(root) {
		c.report(root, "V012", "Select with no item: SELECT needs at least one column or expression")
	}
	if lim, fetch := findLinks(links, "Limit"), findLinks(links, "FetchFirst"); len(lim) > 0 && len(fetch) > 0 {
		c.report(fetch[0].call, "V013", "Limit and FetchFirst cannot be combined: use one of them")
	}
	if complete && hasLink(links, "WithTies") && !(hasLink(links, "FetchFirst") && hasLink(links, "OrderBy")) {
		c.report(findLinks(links, "WithTies")[0].call, "V013", "WithTies needs FetchFirst and OrderBy in the same query")
	}
	if complete && !hasLink(links, "From") {
		for _, l := range links {
			if joinMethods[l.name] {
				c.report(l.call, "V012", l.name+" without From: a join needs a first table")
				break
			}
		}
	}
	c.lockScope(links, stack)
	c.groupCoverage(root, links)
}

// V016: FOR UPDATE OF t where t is not one of the query's tables.
func (c *checker) lockScope(links []link, stack []ast.Node) {
	ofs := findLinks(links, "Of")
	if len(ofs) == 0 || c.insideQLXCall(stack) || !hasLink(links, "From") {
		return
	}
	tables := map[types.Object]bool{}
	for _, l := range links {
		if l.name != "From" && !joinMethods[l.name] {
			continue
		}
		for _, a := range l.call.Args {
			v := c.tableVar(a)
			if v == nil {
				return // alias, subquery, function table: scope not known
			}
			tables[v] = true
		}
	}
	for _, l := range ofs {
		for _, a := range l.call.Args {
			if v := c.tableVar(a); v != nil && !tables[v] {
				c.report(a, "V016", "Of("+types.ExprString(a)+"): the table is not in this query's FROM/JOIN")
			}
		}
	}
}

// V014: a column is selected next to a GroupBy that never mentions its table.
// Render accepts a column of a table whose primary key is grouped, so only the
// case where no column of that table is grouped at all is reported.
func (c *checker) groupCoverage(root *ast.CallExpr, links []link) {
	gbs := findLinks(links, "GroupBy")
	if len(gbs) == 0 {
		return
	}
	grouped := map[types.Object]bool{}
	for _, l := range gbs {
		if spread(l.call) {
			return
		}
		for _, a := range l.call.Args {
			sel, ok := ast.Unparen(a).(*ast.SelectorExpr)
			if !ok || !c.isColumn(sel) {
				return // expression, rollup or grouping sets: do not guess
			}
			grouped[c.tableVar(sel.X)] = true
		}
	}
	for _, a := range root.Args {
		sel, ok := ast.Unparen(a).(*ast.SelectorExpr)
		if !ok || !c.isColumn(sel) {
			continue
		}
		if v := c.tableVar(sel.X); !grouped[v] && c.hasPK(v) {
			c.report(sel, "V014", "column "+types.ExprString(sel)+" is selected but GroupBy lists no column of "+v.Name()+": group by it or use an aggregate")
		}
	}
}

// hasPK reports whether the generated table type of v declares a primary key.
// Render only rejects the ungrouped column when it has one, and an unknown
// table (no fact) is never reported.
func (c *checker) hasPK(v types.Object) bool {
	n, ok := v.Type().(*types.Named)
	if !ok {
		return false
	}
	var tf TableFact
	return c.pass.ImportObjectFact(n.Obj(), &tf) && len(tf.PK) > 0
}

func (c *checker) shapeUpdate(root *ast.CallExpr, links []link, complete bool) {
	has := false
	var keys []string
	for _, l := range links {
		switch l.name {
		case "SetPatch":
			has = true
		case "Set":
			if !noArgs(l.call) {
				has = true
			}
			k, _ := c.assignKeys(l.call.Args)
			keys = append(keys, k...)
		}
	}
	if complete && !has {
		c.report(root, "V012", "UPDATE has no assignment: call Set or SetPatch with at least one field")
	}
	if d := firstDup(keys); d != "" {
		c.report(root, "V013", "column "+d+" is assigned twice in this UPDATE")
	}
}

func (c *checker) shapeInsert(root *ast.CallExpr, links []link, complete bool) {
	rows := false
	var rowKeys [][]string
	rowKeysKnown := true
	for _, l := range links {
		switch l.name {
		case "Row":
			if !noArgs(l.call) {
				rows = true
			}
			k, all := c.assignKeys(l.call.Args)
			if d := firstDup(k); d != "" {
				c.report(l.call, "V013", "column "+d+" is assigned twice in this row")
			}
			if !all || spread(l.call) {
				rowKeysKnown = false
			}
			rowKeys = append(rowKeys, k)
		case "Values":
			if !noArgs(l.call) {
				rows = true
			}
			rowKeysKnown = false
		}
	}
	defaults := hasLink(links, "DefaultValues")
	fq := findLinks(links, "FromQuery")
	switch {
	case complete && !rows && !defaults && len(fq) == 0:
		c.report(root, "V012", "INSERT has no row: call Row, Values, DefaultValues or FromQuery")
	case len(fq) > 0 && (rows || defaults):
		c.report(fq[0].call, "V013", "INSERT takes either rows or FromQuery, not both")
	case defaults && rows:
		c.report(findLinks(links, "DefaultValues")[0].call, "V013", "DefaultValues cannot be combined with rows")
	}
	if rowKeysKnown && len(rowKeys) > 1 {
		for _, k := range rowKeys[1:] {
			if !sameSet(rowKeys[0], k) {
				c.report(root, "V013", "rows of one INSERT must assign the same columns")
				break
			}
		}
	}
	if len(fq) > 0 {
		cols := findLinks(links, "Columns")
		if complete && len(cols) == 0 {
			c.report(fq[0].call, "V012", "INSERT ... SELECT needs Columns naming the target columns")
		}
		if len(cols) == 1 && !spread(cols[0].call) && len(fq[0].call.Args) == 1 {
			if n, ok := c.outWidth(fq[0].call.Args[0]); ok && n != len(cols[0].call.Args) {
				c.report(fq[0].call, "V013", "INSERT names "+strconv.Itoa(len(cols[0].call.Args))+" columns but the query produces "+strconv.Itoa(n))
			}
		}
	}
	for i, l := range links {
		switch l.name {
		case "OnConflict":
			if noArgs(l.call) && i+1 < len(links) {
				switch links[i+1].name {
				case "DoUpdate", "DoUpdateWhere", "DoSelect":
					c.report(l.call, "V012", "ON CONFLICT "+links[i+1].name+" needs conflict columns or OnConflictConstraint")
				}
			}
		case "DoUpdate":
			if noArgs(l.call) {
				c.report(l.call, "V012", "DoUpdate needs at least one assignment")
			}
			k, _ := c.assignKeys(l.call.Args)
			if d := firstDup(k); d != "" {
				c.report(l.call, "V013", "column "+d+" is assigned twice in DoUpdate")
			}
		case "DoUpdateWhere":
			if len(l.call.Args) <= 1 && !spread(l.call) {
				c.report(l.call, "V012", "DoUpdateWhere needs at least one assignment after the condition")
			} else if len(l.call.Args) > 1 {
				k, _ := c.assignKeys(l.call.Args[1:])
				if d := firstDup(k); d != "" {
					c.report(l.call, "V013", "column "+d+" is assigned twice in DoUpdateWhere")
				}
			}
		case "DoSelect":
			if complete && !hasLink(links, "Returning") {
				c.report(l.call, "V012", "DoSelect needs a Returning list")
			}
		}
	}
}

func (c *checker) shapeMerge(root *ast.CallExpr, links []link, complete bool) {
	if complete {
		if !hasLink(links, "Using") {
			c.report(root, "V012", "MERGE needs Using (the source)")
		}
		if !hasLink(links, "On") {
			c.report(root, "V012", "MERGE needs On (the join condition)")
		}
		if !hasLink(links, "ThenUpdate", "ThenInsert", "ThenInsertDefaults", "ThenDelete", "ThenDoNothing") {
			c.report(root, "V012", "MERGE needs at least one When clause finished by a Then action")
		}
	}
	for _, l := range links {
		if l.name != "ThenUpdate" && l.name != "ThenInsert" {
			continue
		}
		if noArgs(l.call) {
			c.report(l.call, "V012", l.name+" needs at least one assignment")
		}
		k, _ := c.assignKeys(l.call.Args)
		if d := firstDup(k); d != "" {
			c.report(l.call, "V013", "column "+d+" is assigned twice in "+l.name)
		}
	}
}

func (c *checker) shapeCase(root *ast.CallExpr, links []link) {
	for _, l := range links {
		switch l.name {
		case "When", "WhenX":
			return
		case "Else", "ElseX", "End":
			c.report(l.call, "V012", "CASE needs at least one When before "+l.name)
			return
		}
	}
}

// explainNeedsAnalyze lists EXPLAIN options the server refuses without ANALYZE;
// explainSince gives the major that introduced an option.
var (
	explainNeedsAnalyze = map[string]bool{"Wal": true, "IO": true, "SerializeText": true, "SerializeBinary": true}
	explainSince        = map[string]int{"GenericPlan": 16, "Memory": 17, "SerializeText": 17, "SerializeBinary": 17, "IO": 19, "PlanAdvice": 19}
)

// versionAtLeast reports against -targets (V009) a construct that needs
// PostgreSQL since or later.
func (c *checker) needsVersion(n ast.Node, what string, since int) {
	if len(c.targets) == 0 || since <= 0 {
		return
	}
	if lo := c.targets[0]; lo < since {
		c.report(n, "V009", what+" requires PostgreSQL "+strconv.Itoa(since)+" but the targets include "+strconv.Itoa(lo))
	}
}

// shapeCall checks a single call, whatever chain it belongs to.
func (c *checker) shapeCall(call *ast.CallExpr, stack []ast.Node) {
	fn := c.callee(call)
	if fn == nil {
		return
	}
	sig, _ := fn.Type().(*types.Signature)
	isPlain := sig != nil && sig.Recv() == nil
	switch {
	case isFunc(fn, "Union"), isFunc(fn, "Intersect"), isFunc(fn, "Except"):
		if len(call.Args) == 2 && !spread(call) {
			l, lok := c.outWidth(call.Args[0])
			r, rok := c.outWidth(call.Args[1])
			if lok && rok && l != r {
				c.report(call, "V013", fn.Name()+" operands have "+strconv.Itoa(l)+" and "+strconv.Itoa(r)+" columns")
			}
		}
	case isFunc(fn, "Truncate"):
		if noArgs(call) {
			c.report(call, "V012", "Truncate needs at least one table")
		}
	case isFunc(fn, "LockTable"):
		if len(call.Args) == 2 && !spread(call) {
			c.report(call, "V012", "LockTable needs at least one table")
		}
	case isFunc(fn, "Values1"), isFunc(fn, "Values2"), isFunc(fn, "Values3"), isFunc(fn, "Values4"):
		fixed := int(fn.Name()[len(fn.Name())-1]-'0') + 1
		if len(call.Args) == fixed && !spread(call) {
			c.report(call, "V012", fn.Name()+" needs at least one row")
		}
	case isFunc(fn, "XMLConcat"), isFunc(fn, "XMLForest"):
		if noArgs(call) {
			c.report(call, "V012", fn.Name()+" needs at least one argument")
		}
	case isFunc(fn, "Checkpoint"):
		if !noArgs(call) {
			c.needsVersion(call, "Checkpoint options", 19)
		}
	case isFunc(fn, "WithHeader"):
		if len(call.Args) == 1 {
			if tv := c.pass.TypesInfo.Types[call.Args[0]]; tv.Value != nil && tv.Value.Kind() == constant.Int {
				if n, ok := constant.Int64Val(tv.Value); ok && n > 1 {
					c.needsVersion(call, "WithHeader(n) with n > 1", 19)
				}
			}
		}
	case fn.Name() == "Explain":
		args := call.Args
		if isPlain && len(args) > 0 {
			args = args[1:] // Explain(stmt, opts...)
		}
		c.explainOptions(call, args)
	}
}

// explainOptions checks the options given to an EXPLAIN: those that need or
// exclude ANALYZE (V015) and those newer than the targets (V009).
func (c *checker) explainOptions(call *ast.CallExpr, args []ast.Expr) {
	names := map[string]ast.Expr{}
	unknown := spread(call)
	for _, a := range args {
		var id *ast.Ident
		switch x := ast.Unparen(a).(type) {
		case *ast.Ident:
			id = x
		case *ast.SelectorExpr:
			id = x.Sel
		}
		var v *types.Var
		if id != nil {
			v, _ = c.pass.TypesInfo.Uses[id].(*types.Var)
		}
		if v == nil || v.Pkg() == nil || v.Pkg().Path() != qlxPath {
			unknown = true
			continue
		}
		names[v.Name()] = a
	}
	_, analyze := names["Analyze"]
	for name, a := range names {
		if explainNeedsAnalyze[name] && !analyze && !unknown {
			c.report(a, "V015", "EXPLAIN option "+name+" requires Analyze")
		}
		if name == "GenericPlan" && analyze {
			c.report(a, "V015", "EXPLAIN options Analyze and GenericPlan cannot be used together")
		}
		if since := explainSince[name]; since > 0 {
			c.needsVersion(a, "EXPLAIN option "+name, since)
		}
	}
}
