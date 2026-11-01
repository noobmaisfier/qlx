package qlx_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCommandLedger is the command ledger of the plan: every non-DDL command
// an application runs is listed in ledgers/commands.json with the Go symbol
// that builds it, and the symbol must exist. A command with no symbol cannot
// be listed, so a gap is visible in review instead of discovered in use.
func TestCommandLedger(t *testing.T) {
	raw, err := os.ReadFile("ledgers/commands.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Command string `json:"command"`
		Symbol  string `json:"symbol"`
		Since   int    `json:"since"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
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
			if fd.Recv == nil {
				have[fd.Name.Name] = true
				continue
			}
			have[recvName(fd.Recv.List[0].Type)+"."+fd.Name.Name] = true
			have["*."+fd.Name.Name] = true
		}
		for _, d := range af.Decls { // exported vars (kw-built options)
			if gd, ok := d.(*ast.GenDecl); ok {
				for _, s := range gd.Specs {
					if vs, ok := s.(*ast.ValueSpec); ok {
						for _, n := range vs.Names {
							have[n.Name] = true
						}
					}
				}
			}
		}
	}
	for _, r := range rows {
		sym := r.Symbol
		if strings.Contains(sym, ".") {
			// Type.Method: accept any receiver with that method name.
			sym = "*." + sym[strings.Index(sym, ".")+1:]
		}
		if !have[sym] {
			t.Errorf("command %q: symbol %s does not exist", r.Command, r.Symbol)
		}
	}
	if len(rows) < 50 {
		t.Errorf("command ledger shrank to %d rows", len(rows))
	}
}

func recvName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.StarExpr:
		return recvName(x.X)
	case *ast.IndexExpr:
		return recvName(x.X)
	case *ast.IndexListExpr:
		return recvName(x.X)
	case *ast.Ident:
		return x.Name
	}
	return ""
}
