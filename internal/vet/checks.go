package vet

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
)

type checker struct {
	fset  *token.FileSet
	info  *types.Info
	pkg   *types.Package
	regs  map[string]token.Position // registered statement names -> first position
	diags []Diagnostic
}

func (c *checker) report(n ast.Node, code, msg string) {
	c.diags = append(c.diags, Diagnostic{Pos: c.fset.Position(n.Pos()), Code: code, Msg: msg})
}

// callee returns the qlx function or method a call resolves to, or nil.
func (c *checker) callee(call *ast.CallExpr) *types.Func {
	var id *ast.Ident
	switch f := ast.Unparen(call.Fun).(type) {
	case *ast.SelectorExpr:
		id = f.Sel
	case *ast.Ident:
		id = f
	case *ast.IndexExpr: // generic instantiation: Register[T](...)
		if s, ok := f.X.(*ast.SelectorExpr); ok {
			id = s.Sel
		} else if i, ok := f.X.(*ast.Ident); ok {
			id = i
		}
	case *ast.IndexListExpr:
		if s, ok := f.X.(*ast.SelectorExpr); ok {
			id = s.Sel
		}
	}
	if id == nil {
		return nil
	}
	fn, _ := c.info.Uses[id].(*types.Func)
	if fn == nil || fn.Pkg() == nil || fn.Pkg().Path() != QLXPath {
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

// isQLXValue reports whether t is a type declared by the qlx package.
func isQLXValue(t types.Type) bool {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	n, ok := t.(*types.Named)
	if !ok || n.Obj().Pkg() == nil {
		return false
	}
	return n.Obj().Pkg().Path() == QLXPath && n.Obj().Exported()
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
		case *ast.CallExpr:
			c.register(n, stack)
			c.unboundedDML(n, stack)
		}
		return true
	})
}

// V001: every QLX builder is immutable, so a call whose result is thrown
// away changes nothing. `q.Where(p)` on its own line is always a bug.
// V004 (same shape): SQL/Render results thrown away.
func (c *checker) discarded(s *ast.ExprStmt) {
	call, ok := ast.Unparen(s.X).(*ast.CallExpr)
	if !ok {
		return
	}
	fn := c.callee(call)
	if fn == nil {
		return
	}
	if isFunc(fn, "Register") {
		return // registering is the point
	}
	if isFunc(fn, "SQL") || isFunc(fn, "Render") {
		c.report(call, "V004", "result of "+fn.Name()+" is discarded: the rendered statement and its error are lost")
		return
	}
	if tv, ok := c.info.Types[call]; ok && isQLXValue(tv.Type) {
		c.report(call, "V001", "result of "+fn.Name()+" is discarded: QLX builders are immutable, so this call has no effect (assign the result)")
	}
}

// V004: `_` in the error position of SQL or Render.
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
	last := a.Lhs[len(a.Lhs)-1]
	if id, ok := last.(*ast.Ident); ok && id.Name == "_" {
		c.report(call, "V004", "the error from "+fn.Name()+" is ignored: an unbound parameter or a version-gated feature would go unnoticed")
	}
}

// V003: registration names must be constant and unique. V005: registration
// belongs at package level (or in init), because Register panics on a repeated
// name and a function body can run more than once.
func (c *checker) register(call *ast.CallExpr, stack []ast.Node) {
	if !isFunc(c.callee(call), "Register") || len(call.Args) == 0 {
		return
	}
	tv := c.info.Types[call.Args[0]]
	if tv.Value == nil || tv.Value.Kind() != constant.String {
		c.report(call, "V003", "Register needs a constant string name so snapshots and `qlx prepare` output are stable")
	} else {
		name := constant.StringVal(tv.Value)
		pos := c.fset.Position(call.Pos())
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

// V002: an UPDATE or DELETE chain that is handed straight to something (an
// argument, a return value) with neither Where nor All. Render rejects it,
// but only when that code path runs; this finds it at build time. Chains
// assigned to a variable are left alone because they may be completed later.
func (c *checker) unboundedDML(call *ast.CallExpr, stack []ast.Node) {
	// Only the outermost call of a chain.
	if len(stack) >= 2 {
		if sel, ok := stack[len(stack)-2].(*ast.SelectorExpr); ok && sel.X == ast.Expr(call) {
			return
		}
	}
	var methods []string
	root := ast.Expr(call)
	for {
		cl, ok := ast.Unparen(root).(*ast.CallExpr)
		if !ok {
			return
		}
		sel, ok := ast.Unparen(cl.Fun).(*ast.SelectorExpr)
		if !ok {
			break
		}
		if _, nested := ast.Unparen(sel.X).(*ast.CallExpr); !nested {
			break
		}
		methods = append(methods, sel.Sel.Name)
		root = sel.X
	}
	rc, ok := ast.Unparen(root).(*ast.CallExpr)
	if !ok {
		return
	}
	fn := c.callee(rc)
	kind := ""
	switch {
	case isFunc(fn, "Update"):
		kind = "UPDATE"
	case isFunc(fn, "DeleteFrom"):
		kind = "DELETE"
	default:
		return
	}
	for _, m := range methods {
		if m == "Where" || m == "All" {
			return
		}
	}
	if len(stack) < 2 {
		return
	}
	switch parent := stack[len(stack)-2].(type) {
	case *ast.CallExpr:
		isArg := false
		for _, a := range parent.Args {
			if a == ast.Expr(call) {
				isArg = true
			}
		}
		if !isArg {
			return
		}
	case *ast.ReturnStmt, *ast.CompositeLit:
	default:
		return
	}
	c.report(call, "V002", kind+" chain has neither Where nor All; it will not render (add Where, or All() to mean every row)")
}
