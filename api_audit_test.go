package qlx_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoSQLTextInAPI is the API audit of the plan: strings appear only as
// names (aliases, CTEs, savepoints, channels) and as values that are always
// bound. No exported function or method may take SQL text, and none may be
// named like an escape hatch.
func TestNoSQLTextInAPI(t *testing.T) {
	banned := map[string]bool{"sql": true, "rawsql": true, "query": true, "raw": true, "fragment": true, "clause": true, "expression": true, "stmt": true, "statement": true}
	// Functions that read already-rendered SQL (for logs, tracing, tests); none
	// of them can put text into a statement.
	readOnly := map[string]bool{"QueryName": true, "Pretty": true, "DescribeSQL": true}
	bannedFn := []string{"Raw", "Unsafe", "Literal", "SQLText", "Fragment"}
	files, _ := filepath.Glob("*.go")
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range af.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || !fd.Name.IsExported() {
				continue
			}
			for _, b := range bannedFn {
				if strings.HasPrefix(fd.Name.Name, b) {
					t.Errorf("%s: %s looks like an escape hatch", f, fd.Name.Name)
				}
			}
			if readOnly[fd.Name.Name] {
				continue
			}
			for _, p := range fd.Type.Params.List {
				id, ok := p.Type.(*ast.Ident)
				if !ok || id.Name != "string" {
					continue
				}
				for _, n := range p.Names {
					if banned[strings.ToLower(n.Name)] {
						t.Errorf("%s: %s takes a string parameter named %q: the API must not accept SQL text", f, fd.Name.Name, n.Name)
					}
				}
			}
		}
	}
}
