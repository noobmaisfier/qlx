// Command pgcat maintains the ledger of PostgreSQL built-ins and generates
// the pgcat/<area> packages and the pg facade from it.
//
//	pgcat extract -ext hstore -o hstore.json  read one extension pack (needs the extension installed)
//	pgcat extract -o pg18.json            read the built-ins of the server at $DATABASE_URL (or an ephemeral one)
//	pgcat enrich -ledger pgcat.json -o pgcat.json   add the OUT columns of record-returning functions, read from a server
//	pgcat versions -ledger pgcat.json -min 2   print the majors a ledger covers; fail below -min
//	pgcat merge -o pgcat.json a.json b.json   merge ledgers; entries get "since" markers
//	pgcat gen -ledger pgcat.json -o pgcat  generate wrappers and print the coverage report
//	pgcat coverage -ledger pgcat.json     print the report only
//	pgcat grammar -y gram.y -map ledgers/grammar_map.json  check every tracked production is mapped (-fill adds new ones, -o writes the ledger)
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/noobmaisfier/qlx/internal/packs"
	"github.com/noobmaisfier/qlx/internal/pgcat"
	"github.com/noobmaisfier/qlx/internal/pgtool"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, out, errw io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errw, "usage: pgcat extract|enrich|merge|versions|gen|docs|diff|coverage|grammar [flags]")
		return 2
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(errw)
	outPath := fs.String("o", "", "output file (extract, merge) or directory (gen)")
	ledgerPath := fs.String("ledger", "pgcat.json", "ledger file (gen, coverage)")
	pkg := fs.String("pkg", "pg", "package name of the facade (gen, docs)")
	ext := fs.String("ext", "", "extract: read an extension pack's API (pgvector, postgis, pg_trgm, hstore, ltree, citext)")
	gramY := fs.String("y", "", "grammar: path to gram.y (or an https URL)")
	mapPath := fs.String("map", "ledgers/grammar_map.json", "grammar: mapping file")
	cheat := fs.String("cheatsheet", "", "grammar: write the generated SQL-to-Go cheat sheet to this file")
	minVersions := fs.Int("min", 0, "versions: fail when the ledger lists fewer majors")
	report := fs.Bool("report", false, "diff: print the differences but exit 0 (review mode)")
	like := fs.String("like", "", "versions: the ledger must list exactly the majors of this one (an extension ledger against pgcat.json)")
	fill := fs.Bool("fill", false, "grammar: add `unmapped` entries for new syntax to the mapping file")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	var err error
	switch args[0] {
	case "extract":
		err = extract(*outPath, *ext)
	case "merge":
		err = merge(*outPath, fs.Args())
	case "versions":
		err = versions(*ledgerPath, *minVersions, *like, out)
	case "enrich":
		err = enrich(*ledgerPath, *outPath, out)
	case "gen":
		err = gen(*ledgerPath, *outPath, *pkg, out)
	case "diff":
		err = diff(fs.Args(), *report, out)
	case "grammar":
		err = grammarCmd(*gramY, *mapPath, *outPath, *cheat, *fill, out)
	case "docs":
		err = docs(*ledgerPath, *outPath, *pkg, out)
	case "coverage":
		err = coverage(*ledgerPath, out)
	default:
		fmt.Fprintf(errw, "pgcat: unknown command %q\n", args[0])
		return 2
	}
	if err != nil {
		fmt.Fprintln(errw, "pgcat:", err)
		return 1
	}
	return 0
}

func extract(path, pack string) error {
	if path == "" {
		return fmt.Errorf("extract needs -o")
	}
	var conn pgtool.Conn
	if url := os.Getenv("DATABASE_URL"); url != "" {
		conn = pgtool.FromURL(url)
	} else {
		srv, err := pgtool.StartEphemeral(os.Getenv("PGBIN"))
		if err != nil {
			return fmt.Errorf("no DATABASE_URL and no local server binaries: %w", err)
		}
		defer srv.Close()
		conn = srv.Conn()
	}
	var l *pgcat.Ledger
	var err error
	if pack == "" {
		l, err = pgcat.Extract(conn)
	} else {
		p, ok := packs.Lookup(pack)
		if !ok {
			return fmt.Errorf("unknown extension pack %q (known: %s)", pack, strings.Join(packs.Names(), ", "))
		}
		// A throwaway server has nothing installed; a live one may. Either
		// way CREATE EXTENSION IF NOT EXISTS is harmless.
		if err := conn.Exec(`CREATE EXTENSION IF NOT EXISTS "` + p.Extension + `"`); err != nil {
			return fmt.Errorf("installing extension %s: %w", p.Extension, err)
		}
		l, err = pgcat.ExtractExtension(conn, p.Extension)
	}
	if err != nil {
		return err
	}
	b, err := l.Marshal()
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func merge(path string, files []string) error {
	if path == "" || len(files) == 0 {
		return fmt.Errorf("usage: pgcat merge -o out.json a.json b.json ...")
	}
	var ls []*pgcat.Ledger
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		l, err := pgcat.Parse(data)
		if err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		ls = append(ls, l)
	}
	b, err := pgcat.Merge(ls...).Marshal()
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func load(path string) (*pgcat.Ledger, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return pgcat.Parse(data)
}

func gen(ledger, dir, pkg string, out io.Writer) error {
	if dir == "" {
		return fmt.Errorf("gen needs -o <directory>")
	}
	l, err := load(ledger)
	if err != nil {
		return err
	}
	res, err := pgcat.Generate(l, pkg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, src := range res.Files {
		dst := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, src, 0o644); err != nil {
			return err
		}
	}
	printReport(out, res.Report)
	return nil
}

func coverage(ledger string, out io.Writer) error {
	l, err := load(ledger)
	if err != nil {
		return err
	}
	res, err := pgcat.Generate(l, "pg")
	if err != nil {
		return err
	}
	printReport(out, res.Report)
	return nil
}

func printReport(out io.Writer, r pgcat.Report) {
	fmt.Fprintf(out, "functions: %d of %d wrapped\noperators: %d of %d wrapped\nnot wrapped:\n", r.FuncsWrapped, r.Funcs, r.OpsWrapped, r.Ops)
	type kv struct {
		k string
		v int
	}
	var list []kv
	for k, v := range r.Skipped {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v || list[i].v == list[j].v && list[i].k < list[j].k })
	for _, e := range list {
		fmt.Fprintf(out, "  %5d  %s\n", e.v, e.k)
	}
}

// docs writes the generated reference pages (Markdown) for the pgcat packages.
func docs(ledger, dir, pkg string, out io.Writer) error {
	if dir == "" {
		return fmt.Errorf("docs needs -o <directory>")
	}
	l, err := load(ledger)
	if err != nil {
		return err
	}
	res, err := pgcat.Generate(l, pkg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	files := pgcat.Docs(l, res, pkg)
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), src, 0o644); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "wrote %d pages to %s\n", len(files), dir)
	return nil
}

// errLedgersDiffer is what diff returns when the two ledgers are not the
// same, so a script or CI step can rely on the exit status alone.
var errLedgersDiffer = errors.New("the ledgers differ")

// diff prints what changed between two ledgers: pgcat diff old.json new.json.
// It fails when they differ; with report set it only prints (a review of what
// a new server version changed, where differences are the point).
func diff(files []string, report bool, out io.Writer) error {
	if len(files) != 2 {
		return fmt.Errorf("usage: pgcat diff old.json new.json")
	}
	a, err := load(files[0])
	if err != nil {
		return err
	}
	b, err := load(files[1])
	if err != nil {
		return err
	}
	text := pgcat.Diff(a, b)
	fmt.Fprint(out, text)
	if text != pgcat.NoDifferences && !report {
		return errLedgersDiffer
	}
	return nil
}

// enrich reads the connected server's catalog and copies the OUT columns of
// its functions into the ledger.
func enrich(ledger, path string, out io.Writer) error {
	if path == "" {
		path = ledger
	}
	l, err := load(ledger)
	if err != nil {
		return err
	}
	var conn pgtool.Conn
	if url := os.Getenv("DATABASE_URL"); url != "" {
		conn = pgtool.FromURL(url)
	} else {
		srv, err := pgtool.StartEphemeral(os.Getenv("PGBIN"))
		if err != nil {
			return fmt.Errorf("no DATABASE_URL and no local server binaries: %w", err)
		}
		defer srv.Close()
		conn = srv.Conn()
	}
	fresh, err := pgcat.Extract(conn)
	if err != nil {
		return err
	}
	n := pgcat.Enrich(l, fresh)
	b, err := l.Marshal()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "enriched %d functions with OUT columns\n", n)
	return os.WriteFile(path, b, 0o644)
}

func versions(ledger string, min int, like string, out io.Writer) error {
	l, err := load(ledger)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "ledger versions:", l.Versions)
	if len(l.Versions) < min {
		return fmt.Errorf("%s lists %d PostgreSQL majors, need at least %d: wrappers would be unbounded on the others", ledger, len(l.Versions), min)
	}
	if like != "" {
		ref, err := load(like)
		if err != nil {
			return err
		}
		if !slices.Equal(l.Versions, ref.Versions) {
			return fmt.Errorf("%s lists majors %v but %s lists %v: re-extract it on every major and merge, otherwise its wrappers are unbounded on the missing ones", ledger, l.Versions, like, ref.Versions)
		}
	}
	return nil
}
