package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/noobmaisfier/qlx/internal/codegen"
	"github.com/noobmaisfier/qlx/internal/regress"
	"github.com/noobmaisfier/qlx/internal/sql2go"
)

// cmdRegress is the regression-suite ledger tool:
//
//	qlx regress -dir src/test/regress/sql -ledger ledgers/regress.json [-cases regress_cases.go] [-baseline old.json]
//
// It splits every .sql file, skips DDL, translates the query-side statements
// with sql2go (names resolved through schema.lock, like `qlx sql2go`),
// writes the ledger, optionally a Go file of cases for qtest.RunRegress, and
// fails when a statement the baseline translated no longer does.
func cmdRegress(f File, base, dir, ledgerPath, casesPath, baseline, pkg string, out io.Writer) error {
	if dir == "" {
		return fmt.Errorf("regress: -dir with the regression .sql files is required")
	}
	s, _, err := readLock(f, base)
	if err != nil {
		return err
	}
	names := codegen.Names(s, withLockSchemas(f, s).Config)
	paths, _ := filepath.Glob(filepath.Join(dir, "*.sql"))
	if len(paths) == 0 {
		return fmt.Errorf("regress: no .sql files in %s", dir)
	}
	files := map[string]string{}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files[strings.TrimSuffix(filepath.Base(p), ".sql")] = string(b)
	}
	imports := map[string]bool{}
	tr := func(sql string) (string, []string, error) {
		r, err := sql2go.FromSQL(sql, names, sql2go.Options{Q: "qlx"})
		if err != nil {
			return "", nil, err
		}
		for _, im := range r.Imports {
			imports[im] = true
		}
		return r.Code, r.Warnings, nil
	}
	l := regress.Build(dir, files, tr)
	counts := l.Counts()
	var keys []string
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(out, "%-14s %d\n", k, counts[k])
	}
	if ledgerPath != "" {
		if err := l.Save(ledgerPath); err != nil {
			return err
		}
	}
	if casesPath != "" {
		var ims []string
		for im := range imports {
			ims = append(ims, im)
		}
		sort.Strings(ims)
		if pkg == "" {
			pkg = "regress"
		}
		if err := os.WriteFile(casesPath, []byte(regress.EmitCases(pkg, "", l, ims)), 0o644); err != nil {
			return err
		}
	}
	if baseline != "" {
		old, err := regress.Load(baseline)
		if err != nil {
			return err
		}
		if rs := regress.Regressions(old, l); len(rs) > 0 {
			for _, r := range rs {
				fmt.Fprintln(out, "regression:", r)
			}
			return fmt.Errorf("regress: %d statement(s) regressed", len(rs))
		}
	}
	return nil
}
