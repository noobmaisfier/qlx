package qlxvet

import (
	"go/ast"
	"go/types"
)

const qpagePath = qlxPath + "/qpage"

// pageCall checks the two things about cursor pagination (package qpage) that
// the type system cannot see.
//
// V032: the SELECT given to Client.Page, Tx.Page or Window.Apply has its own
// ORDER BY, LIMIT, OFFSET or FETCH FIRST. qpage appends its own ORDER BY after
// yours, so the cursor predicate would no longer follow the order the rows come
// in (rows skipped or repeated, silently), and its LIMIT replaces yours.
// Only a chain written in the call is read: a SELECT built in a variable is
// not followed.
//
// V033: qpage.Sorts without a Tiebreak. Without a last key that is unique (the
// primary key), two rows with the same sort value can be skipped or repeated
// at a page boundary. If a Sort already ends with a unique key, say so with
// //qlx:ignore and the reason.
func (c *checker) pageCall(call *ast.CallExpr) {
	fn := c.anyCallee(call)
	if fn == nil || fn.Pkg() == nil {
		return
	}
	sig, _ := fn.Type().(*types.Signature)
	if sig == nil {
		return
	}
	path := fn.Pkg().Path()
	switch {
	case path == qpagePath && fn.Name() == "Sorts" && sig.Recv() == nil:
		c.sortsTiebreak(call)
	case path == qpagePath && fn.Name() == "Apply" && recvName(fn) == "Window" && len(call.Args) == 1:
		c.pageBase(call.Args[0], fn.Name())
	case isPgxexec(path) && fn.Name() == "Page" && (recvName(fn) == "Client" || recvName(fn) == "Tx") && len(call.Args) >= 2:
		c.pageBase(call.Args[1], "Page")
	}
}

func (c *checker) sortsTiebreak(call *ast.CallExpr) {
	if call.Ellipsis.IsValid() {
		return // the options come from a slice: not readable here
	}
	for _, a := range call.Args {
		ce, ok := ast.Unparen(a).(*ast.CallExpr)
		if !ok {
			return // an option held in a variable: it may be the tiebreak
		}
		if fn := c.anyCallee(ce); fn != nil && fn.Pkg() != nil && fn.Pkg().Path() == qpagePath && fn.Name() == "Tiebreak" {
			return
		}
	}
	c.report(call, "V033", "qpage.Sorts without a Tiebreak: if the sort keys are not unique, rows that share a value can be skipped or repeated at a page boundary; end every sort with the primary key (qpage.Tiebreak), or //qlx:ignore with the reason if a Sort already ends with a unique key")
}

func (c *checker) pageBase(arg ast.Expr, entry string) {
	call, ok := ast.Unparen(arg).(*ast.CallExpr)
	if !ok {
		return
	}
	root, links := chain(call)
	if !isFunc(c.callee(root), "Select") {
		return
	}
	for _, l := range links {
		switch l.name {
		case "OrderBy", "Limit", "Offset", "FetchFirst":
			c.report(l.call, "V032", l.name+" on the SELECT given to "+entry+": qpage adds its own ORDER BY and LIMIT after yours, so the cursor no longer follows the order of the rows (rows skipped or repeated); remove it and declare the order with qpage.Sort")
		}
	}
}

// anyCallee is callee without the restriction to package qlx.
func (c *checker) anyCallee(call *ast.CallExpr) *types.Func {
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
	return fn
}
