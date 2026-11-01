package qlx_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The analyzer reads "Requires PostgreSQL N." from a doc comment to report
// version-gated calls against -targets (V009). This test keeps the hand-written
// API honest: an exported function or method whose body gates on a PostgreSQL
// version must say so in its doc comment.
func TestVersionGatedAPIIsDocumented(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
		if err != nil {
			continue // Go 1.27-only files do not parse on older toolchains
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil || !fd.Name.IsExported() {
				continue
			}
			// WithHeader and Checkpoint are gated only for some arguments; the
			// analyzer checks those call shapes itself (shape.go).
			if strings.HasPrefix(fd.Name.Name, "Render") || fd.Name.Name == "String" ||
				fd.Name.Name == "WithHeader" || fd.Name.Name == "Checkpoint" {
				continue
			}
			body := string(src[fset.Position(fd.Body.Pos()).Offset:fset.Position(fd.Body.End()).Offset])
			gated := strings.Contains(body, "versioned{PG") || strings.Contains(body, "versioned{1") ||
				strings.Contains(body, "need(PG") || strings.Contains(body, "min: PG") || strings.Contains(body, "min: 1")
			if !gated {
				continue
			}
			// Gating through a parameter (FuncCall(min, ...)) is the caller's.
			if strings.Contains(body, "versioned{min") || strings.Contains(body, "versionedRange{min") {
				continue
			}
			doc := ""
			if fd.Doc != nil {
				doc = fd.Doc.Text()
			}
			if !strings.Contains(doc, "Requires PostgreSQL") && !strings.Contains(doc, "Removed in PostgreSQL") {
				t.Errorf("%s: %s is version-gated but its doc comment has no \"Requires PostgreSQL N.\" sentence (the analyzer's V009 reads it)", name, fd.Name.Name)
			}
		}
	}
}
