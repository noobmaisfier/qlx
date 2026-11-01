package qlxvet

import (
	"go/ast"
	"go/token"
	"go/types"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// TableFact is exported for every generated table type. It carries what the
// generated constructor says: the columns an INSERT must set and the mapping
// from Go field to SQL column, so a package that imports the schema can be
// checked without running the program.
type TableFact struct {
	Name     string
	Required []string
	PK       []string          // primary key columns (GroupBy coverage, V014)
	Cols     map[string]string // Go field -> SQL column
}

func (*TableFact) AFact()           {}
func (f *TableFact) String() string { return "table " + f.Name }

// VersionFact is exported for every function or method whose doc comment carries the
// "Requires PostgreSQL N." / "Removed in PostgreSQL N." sentences the
// generator writes.
type VersionFact struct{ Since, Until int }

func (*VersionFact) AFact() {}

// RowFact is exported for every ScanTargets method whose body is the plain
// `return []any{&r.A, &r.B}` that generated rows and qlx.TupleN use: the
// struct fields it scans, in column order (V018).
type RowFact struct{ Fields []string }

func (*RowFact) AFact()           {}
func (f *RowFact) String() string { return "row " + strings.Join(f.Fields, ",") }
func (f *VersionFact) String() string {
	return "versions " + strconv.Itoa(f.Since) + "-" + strconv.Itoa(f.Until)
}

var (
	reSince = regexp.MustCompile(`Requires PostgreSQL (\d+)`)
	reUntil = regexp.MustCompile(`Removed in PostgreSQL (\d+)`)
)

func exportFacts(pass *analysis.Pass) {
	for _, f := range pass.Files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if fd.Doc != nil {
				txt := fd.Doc.Text()
				var vf VersionFact
				if m := reSince.FindStringSubmatch(txt); m != nil {
					vf.Since, _ = strconv.Atoi(m[1])
				}
				if m := reUntil.FindStringSubmatch(txt); m != nil {
					vf.Until, _ = strconv.Atoi(m[1])
				}
				if vf.Since > 0 || vf.Until > 0 {
					if obj, ok := pass.TypesInfo.Defs[fd.Name].(*types.Func); ok {
						pass.ExportObjectFact(obj, &vf)
					}
				}
			}
			if fd.Body != nil {
				tableFact(pass, fd)
				rowFact(pass, fd)
			}
		}
	}
}

// tableFact recognises the generated constructor shape:
//
//	func newUsers(alias string) UsersT {
//		t := qlx.TableRef{Schema: "public", Name: "users", Alias: alias, Required: []string{"email"}}
//		return UsersT{TableRef: t, ID: qlx.NewNumCol[int64](t, "id"), ...}
//	}
func tableFact(pass *analysis.Pass, fd *ast.FuncDecl) {
	var ref *ast.CompositeLit
	var ret *ast.CompositeLit
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		tv, ok := pass.TypesInfo.Types[cl]
		if !ok {
			return true
		}
		named, ok := tv.Type.(*types.Named)
		if !ok {
			return true
		}
		if named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == qlxPath && named.Obj().Name() == "TableRef" {
			ref = cl
		} else if embedsTableRef(named) {
			ret = cl
		}
		return true
	})
	if ref == nil || ret == nil {
		return
	}
	tv := pass.TypesInfo.Types[ret]
	named := tv.Type.(*types.Named)
	tf := &TableFact{Cols: map[string]string{}}
	for _, e := range ref.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		switch keyName(kv.Key) {
		case "Name":
			tf.Name = strLit(kv.Value)
		case "Required":
			if l, ok := kv.Value.(*ast.CompositeLit); ok {
				for _, x := range l.Elts {
					tf.Required = append(tf.Required, strLit(x))
				}
			}
		case "PK":
			if l, ok := kv.Value.(*ast.CompositeLit); ok {
				for _, x := range l.Elts {
					tf.PK = append(tf.PK, strLit(x))
				}
			}
		}
	}
	for _, e := range ret.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		call, ok := kv.Value.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 {
			continue
		}
		if col := strLit(call.Args[len(call.Args)-1]); col != "" {
			tf.Cols[keyName(kv.Key)] = col
		}
	}
	pass.ExportObjectFact(named.Obj(), tf)
}

func keyName(e ast.Expr) string {
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

func strLit(e ast.Expr) string {
	bl, ok := e.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return ""
	}
	s, err := strconv.Unquote(bl.Value)
	if err != nil {
		return ""
	}
	return s
}

func embedsTableRef(n *types.Named) bool {
	st, ok := n.Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if !f.Embedded() {
			continue
		}
		if nt, ok := f.Type().(*types.Named); ok && nt.Obj().Pkg() != nil &&
			nt.Obj().Pkg().Path() == qlxPath && nt.Obj().Name() == "TableRef" {
			return true
		}
	}
	return false
}

// rowFact recognises func (r *T) ScanTargets() []any { return []any{&r.A, ...} }.
func rowFact(pass *analysis.Pass, fd *ast.FuncDecl) {
	if fd.Name.Name != "ScanTargets" || fd.Recv == nil || len(fd.Recv.List) != 1 ||
		len(fd.Recv.List[0].Names) != 1 || len(fd.Body.List) != 1 {
		return
	}
	recv := fd.Recv.List[0].Names[0].Name
	ret, ok := fd.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return
	}
	lit, ok := ret.Results[0].(*ast.CompositeLit)
	if !ok {
		return
	}
	rf := &RowFact{}
	for _, e := range lit.Elts {
		u, ok := e.(*ast.UnaryExpr)
		if !ok || u.Op != token.AND {
			return
		}
		sel, ok := u.X.(*ast.SelectorExpr)
		if !ok {
			return
		}
		if id, ok := sel.X.(*ast.Ident); !ok || id.Name != recv {
			return
		}
		rf.Fields = append(rf.Fields, sel.Sel.Name)
	}
	if obj, ok := pass.TypesInfo.Defs[fd.Name].(*types.Func); ok {
		pass.ExportObjectFact(obj, rf)
	}
}
