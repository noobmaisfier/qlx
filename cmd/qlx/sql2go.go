package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/noobmaisfier/qlx/internal/codegen"
	"github.com/noobmaisfier/qlx/internal/sql2go"
)

// cmdSQL2Go converts SQL text into QLX code, resolving names through
// schema.lock so the symbols are the ones `qlx gen` produced.
//
//	qlx sql2go -e 'SELECT id FROM users WHERE id = 1'
//	qlx sql2go query.sql
//	echo 'DELETE FROM orders WHERE id = 1' | qlx sql2go
func cmdSQL2Go(f File, base string, inline, q, pkg string, files []string, in io.Reader, out io.Writer) error {
	s, _, err := readLock(f, base)
	if err != nil {
		return err
	}
	var input string
	switch {
	case inline != "":
		input = inline
	case len(files) > 0:
		var b strings.Builder
		for _, p := range files {
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			b.Write(data)
			b.WriteString("\n")
		}
		input = b.String()
	default:
		data, err := io.ReadAll(in)
		if err != nil {
			return err
		}
		input = string(data)
	}
	if strings.TrimSpace(input) == "" {
		return fmt.Errorf("sql2go: no SQL given (use -e, a file, or stdin)")
	}
	res, err := sql2go.FromSQL(input, codegen.Names(s, f.Config), sql2go.Options{Q: q, Pkg: pkg})
	if err != nil {
		return err
	}
	fmt.Fprintln(out, res.Code)
	if len(res.Imports) > 0 {
		fmt.Fprintf(out, "\n// imports: %s\n", strings.Join(res.Imports, ", "))
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(out, "// warning: %s\n", w)
	}
	return nil
}
