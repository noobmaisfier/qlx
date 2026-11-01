// Package vet is `qlx vet`: static checks for mistakes the type system and
// the render-time checks cannot see because the program is still valid Go and
// still renders (a discarded immutable builder) or fails only when run (a
// duplicate registration, an UPDATE chain with no WHERE). It uses go/types
// fed by the compiler's export data from `go list -export`, the same inputs as
// `go vet`, so it needs nothing beyond the standard library.
package vet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// QLXPath is the import path of the runtime package.
const QLXPath = "github.com/noobmaisfier/qlx"

// Diagnostic is one finding.
type Diagnostic struct {
	Pos  token.Position
	Code string // V001...
	Msg  string
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s:%d:%d: %s %s", d.Pos.Filename, d.Pos.Line, d.Pos.Column, d.Code, d.Msg)
}

type listPkg struct {
	ImportPath string
	Dir        string
	GoFiles    []string
	CgoFiles   []string
	Export     string
	ImportMap  map[string]string
	DepOnly    bool
	Error      *struct{ Err string }
}

// Run type-checks the packages matching patterns (relative to dir) and
// returns every finding, sorted by position.
func Run(dir string, patterns []string) ([]Diagnostic, error) {
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	args := append([]string{"list", "-e", "-export", "-deps", "-json=ImportPath,Dir,GoFiles,CgoFiles,Export,ImportMap,DepOnly,Error"}, patterns...)
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("go list: %v: %s", err, strings.TrimSpace(errb.String()))
	}
	var pkgs []listPkg
	exports := map[string]string{}
	for dec := json.NewDecoder(&out); ; {
		var p listPkg
		if err := dec.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		if p.Export != "" {
			exports[p.ImportPath] = p.Export
		}
		pkgs = append(pkgs, p)
	}
	var diags []Diagnostic
	for _, p := range pkgs {
		if p.DepOnly {
			continue
		}
		if p.Error != nil {
			return nil, fmt.Errorf("%s: %s", p.ImportPath, p.Error.Err)
		}
		ds, err := checkPackage(p, exports)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p.ImportPath, err)
		}
		diags = append(diags, ds...)
	}
	sort.Slice(diags, func(i, j int) bool {
		a, b := diags[i].Pos, diags[j].Pos
		if a.Filename != b.Filename {
			return a.Filename < b.Filename
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Column < b.Column
	})
	return diags, nil
}

func checkPackage(p listPkg, exports map[string]string) ([]Diagnostic, error) {
	fset := token.NewFileSet()
	var files []*ast.File
	for _, name := range p.GoFiles {
		f, err := parser.ParseFile(fset, filepath.Join(p.Dir, name), nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	if len(p.CgoFiles) > 0 {
		return nil, fmt.Errorf("cgo files are not supported by qlx vet")
	}
	lookup := func(path string) (io.ReadCloser, error) {
		if m, ok := p.ImportMap[path]; ok {
			path = m
		}
		file, ok := exports[path]
		if !ok {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(file)
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Uses:  map[*ast.Ident]types.Object{},
		Defs:  map[*ast.Ident]types.Object{},
	}
	conf := types.Config{Importer: importer.ForCompiler(fset, "gc", lookup), Error: func(error) {}}
	pkg, _ := conf.Check(p.ImportPath, fset, files, info)
	// Type errors are the compiler's to report; analyse whatever type-checked.
	c := &checker{fset: fset, info: info, pkg: pkg, regs: map[string]token.Position{}}
	for _, f := range files {
		c.file(f)
	}
	return c.diags, nil
}
