// Command qlx generates typed query-builder symbols from a PostgreSQL
// schema and keeps them honest.
//
//	qlx init     write qlx.yaml and a migrations directory
//	qlx gen      read the schema, write schema.lock and the Go package
//	qlx check    fail if the Go package or schema.lock is out of date
//	qlx verify   fail if a live database differs from schema.lock
//	qlx doctor   report tools, warnings and unsupported objects
//	qlx watch    regenerate on every change to the migrations or qlx.json
//	qlx prepare  make a server PREPARE every query in a qtest.Dump file
//	qlx sql2go   turn SQL text into QLX code (build with -tags sql2go)
//	qlx regress  translate PostgreSQL's regression statements and track every outcome (-tags sql2go)
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/noobmaisfier/qlx/internal/catalog"
	"github.com/noobmaisfier/qlx/internal/codegen"
	"github.com/noobmaisfier/qlx/internal/miniyaml"
	"github.com/noobmaisfier/qlx/internal/pgtool"
)

// File is qlx.yaml (or the same keys as qlx.json).
type File struct {
	codegen.Config
	Lock        string `json:"lock"`                   // default schema.lock
	DatabaseURL string `json:"database_url,omitempty"` // read the schema from a live server
	Migrations  string `json:"migrations,omitempty"`   // or build it from *.sql files on a throwaway server
	PGBin       string `json:"pg_bin,omitempty"`       // directory with postgres, initdb, pg_ctl
	// ExcludeSchemas removes schemas from schemas: ["*"]. It has no meaning with a list of names.
	ExcludeSchemas []string `json:"exclude_schemas,omitempty"`
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, out, errw io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errw, "usage: qlx rows|init|gen|watch|check|verify|doctor|prepare|sql2go|regress [-config qlx.json] [-from-lock]")
		return 2
	}
	cmd := args[0]
	if cmd == "vet" { // removed: the analyzer is the only static checker
		fmt.Fprintln(errw, "qlx: `qlx vet` was removed; use the analyzer, which reports every rule:\n"+
			"  go install github.com/noobmaisfier/qlx/analyzer/cmd/qlxvet@latest\n"+
			"  go vet -vettool=$(which qlxvet) ./...")
		return 2 // not 0: a CI step that still calls `qlx vet` must not pass silently
	}
	if cmd == "rows" { // needs no config either
		if err := cmdRows(args[1:], out); err != nil {
			fmt.Fprintln(errw, "qlx:", err)
			return 1
		}
		return 0
	}
	if cmd == "init" { // runs before a config exists
		if err := cmdInit(args[1:], out); err != nil {
			fmt.Fprintln(errw, "qlx:", err)
			return 1
		}
		return 0
	}
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(errw)
	cfgPath := fs.String("config", "", "config file (default qlx.yaml, qlx.yml or qlx.json)")
	fromLock := fs.Bool("from-lock", false, "use schema.lock instead of reading a database")
	queries := fs.String("queries", "", "prepare: JSON file written by qtest.Dump")
	regDir := fs.String("dir", "", "regress: directory of PostgreSQL regression .sql files")
	regLedger := fs.String("ledger", "", "regress: write the ledger here")
	regCases := fs.String("cases", "", "regress: write a Go file of cases for qtest.RunRegress")
	regBase := fs.String("baseline", "", "regress: fail if a statement this ledger translated no longer does")
	inline := fs.String("e", "", "sql2go: SQL text to convert")
	qName := fs.String("q", "q", "sql2go: name the qlx package is imported as")
	pkgName := fs.String("pkg", "", "sql2go: qualifier of the generated package (empty when dot-imported)")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *cfgPath == "" {
		*cfgPath = findConfig(".")
	}
	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		fmt.Fprintln(errw, "qlx:", err)
		return 2
	}
	base := filepath.Dir(*cfgPath)
	var runErr error
	switch cmd {
	case "gen":
		runErr = cmdGen(cfg, base, *fromLock, out)
	case "check":
		runErr = cmdCheck(cfg, base, out)
	case "verify":
		runErr = cmdVerify(cfg, base, out)
	case "doctor":
		runErr = cmdDoctor(cfg, base, out)
	case "watch":
		runErr = cmdWatch(cfg, base, *cfgPath, out, errw)
	case "prepare":
		runErr = cmdPrepare(cfg, base, *queries, out)
	case "regress":
		runErr = cmdRegress(cfg, base, *regDir, *regLedger, *regCases, *regBase, *pkgName, out)
	case "sql2go":
		runErr = cmdSQL2Go(cfg, base, *inline, *qName, *pkgName, fs.Args(), os.Stdin, out)
	default:
		fmt.Fprintf(errw, "qlx: unknown command %q\n", cmd)
		return 2
	}
	if runErr != nil {
		fmt.Fprintln(errw, "qlx:", runErr)
		return 1
	}
	return 0
}

// findConfig picks the first of qlx.yaml, qlx.yml and qlx.json that
// exists in dir; with none it names qlx.yaml so the error says what to create.
func findConfig(dir string) string {
	for _, n := range []string{"qlx.yaml", "qlx.yml", "qlx.json"} {
		p := filepath.Join(dir, n)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(dir, "qlx.yaml")
}

func loadConfig(path string) (File, error) {
	var f File
	data, err := os.ReadFile(path)
	if err != nil {
		return f, fmt.Errorf("reading %s: %w", path, err)
	}
	if ext := strings.ToLower(filepath.Ext(path)); ext == ".yaml" || ext == ".yml" {
		if data, err = miniyaml.ToJSON(data); err != nil {
			return f, fmt.Errorf("%s: %w", path, err)
		}
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields() // a typo in a key must not be silently ignored
	if err := dec.Decode(&f); err != nil {
		return f, fmt.Errorf("%s: %w", path, err)
	}
	if f.Lock == "" {
		f.Lock = "schema.lock"
	}
	switch f.UUID {
	case "", "std", "bytes":
	default:
		return f, fmt.Errorf(`%s: uuid must be "std" or "bytes", not %q`, path, f.UUID)
	}
	switch f.IDs {
	case "", "int", "string":
	default:
		return f, fmt.Errorf(`%s: ids must be "int" or "string", not %q`, path, f.IDs)
	}
	switch f.Date {
	case "", "qt", "stdlib":
	default:
		return f, fmt.Errorf(`%s: date must be "qt" or "stdlib", not %q`, path, f.Date)
	}
	switch f.Timestamptz {
	case "", "qt", "stdlib":
	default:
		return f, fmt.Errorf(`%s: timestamptz must be "qt" or "stdlib", not %q`, path, f.Timestamptz)
	}
	if f.Nullable != "" && f.Nullable != "null" {
		return f, fmt.Errorf(`%s: nullable must be "null" (qt.Null[T]); pointers cannot tell NULL from not-loaded`, path)
	}
	for _, t := range f.Targets {
		if t < 14 || t > 99 {
			return f, fmt.Errorf("%s: targets lists PostgreSQL majors such as 18 and 19, not %d", path, t)
		}
	}
	for _, e := range f.Extensions {
		if !codegen.KnownExtension(e) {
			return f, fmt.Errorf("%s: unknown extension pack %q (known: %s)", path, e, strings.Join(codegen.ExtensionNames(), ", "))
		}
	}
	if len(f.Schemas) == 0 {
		f.Schemas = []string{"public"}
	}
	if wantsAll(f.Schemas) && len(f.Schemas) != 1 {
		return f, fmt.Errorf(`%s: schemas: "*" means every schema and cannot be combined with other names`, path)
	}
	if len(f.ExcludeSchemas) > 0 && !(len(f.Schemas) == 1 && f.Schemas[0] == catalog.AllSchemas) {
		return f, fmt.Errorf(`%s: exclude_schemas only applies to schemas: ["*"]`, path)
	}
	if f.Package == "" {
		f.Package = "db"
	}
	if f.Out == "" {
		f.Out = f.Package
	}
	return f, nil
}

// wantsAll reports whether the schemas setting asks for every schema.
func wantsAll(schemas []string) bool { return slices.Contains(schemas, catalog.AllSchemas) }

// schemasFor resolves the schemas setting against a server (see
// catalog.ResolveSchemas). A list of names comes back unchanged.
func schemasFor(f File, c pgtool.Conn) ([]string, error) {
	return catalog.ResolveSchemas(c, f.Schemas, f.ExcludeSchemas)
}

// withLockSchemas gives the generator the schemas the lock was built from,
// when the config says "*": the generated package must list names, and the
// lock is what says which ones there were.
func withLockSchemas(f File, s *catalog.Schema) File {
	if wantsAll(f.Schemas) {
		f.Schemas = catalog.PublicFirst(s.Schemas)
	}
	return f
}

func rel(base, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

// readSource reads the schema from its configured source.
func readSource(f File, base string) (*catalog.Schema, error) {
	switch {
	case f.DatabaseURL != "":
		conn := pgtool.FromURL(f.DatabaseURL)
		schemas, err := schemasFor(f, conn)
		if err != nil {
			return nil, err
		}
		return catalog.Load(conn, schemas)
	case f.Migrations != "":
		srv, err := pgtool.StartEphemeral(f.PGBin)
		if err != nil {
			return nil, fmt.Errorf("starting a throwaway server for the migrations: %w", err)
		}
		defer srv.Close()
		if err := srv.Conn().ApplySQLDir(rel(base, f.Migrations)); err != nil {
			return nil, fmt.Errorf("applying migrations: %w", err)
		}
		schemas, err := schemasFor(f, srv.Conn())
		if err != nil {
			return nil, err
		}
		return catalog.Load(srv.Conn(), schemas)
	}
	return nil, errors.New(`no schema source: set "database_url" or "migrations" in qlx.json, or use -from-lock`)
}

func readLock(f File, base string) (*catalog.Schema, []byte, error) {
	data, err := os.ReadFile(rel(base, f.Lock))
	if err != nil {
		return nil, nil, err
	}
	s, err := catalog.ParseLock(data)
	return s, data, err
}

func cmdGen(f File, base string, fromLock bool, out io.Writer) error {
	var s *catalog.Schema
	if fromLock {
		var err error
		if s, _, err = readLock(f, base); err != nil {
			return err
		}
	} else {
		raw, err := readSource(f, base)
		if err != nil {
			return err
		}
		lock, sealed, err := catalog.Seal(raw)
		if err != nil {
			return err
		}
		if err := writeIfChanged(rel(base, f.Lock), lock); err != nil {
			return err
		}
		s = sealed
	}
	res, err := codegen.Generate(s, withLockSchemas(f, s).Config)
	if err != nil {
		return err
	}
	dir := rel(base, f.Out)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, src := range res.Files {
		if err := writeIfChanged(filepath.Join(dir, name), src); err != nil {
			return err
		}
	}
	removed, err := removeStale(dir, res.Files)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "qlx: %d files in %s (schema %s)\n", len(res.Files), dir, s.Hash[:12])
	for _, r := range removed {
		fmt.Fprintf(out, "  removed %s\n", r)
	}
	report(out, res)
	return nil
}

func report(out io.Writer, res *codegen.Result) {
	for _, w := range res.Warnings {
		fmt.Fprintln(out, "  warning:", w)
	}
	for _, s := range res.Skipped {
		fmt.Fprintln(out, "  no wrapper:", s)
	}
}

const genMarker = "// Code generated by qlx; DO NOT EDIT."

// removeStale deletes generated files whose table or type no longer exists.
// Files without the marker are never touched.
func removeStale(dir string, keep map[string][]byte) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var gone []string
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		if _, ok := keep[e.Name()]; ok {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return gone, err
		}
		if bytes.HasPrefix(data, []byte(genMarker)) {
			if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
				return gone, err
			}
			gone = append(gone, e.Name())
		}
	}
	sort.Strings(gone)
	return gone, nil
}

func writeIfChanged(path string, data []byte) error {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path) // atomic: a crash never leaves half a file
}

// cmdCheck regenerates from schema.lock in memory and compares bytes with the
// files on disk. When a schema source is configured it also checks that the
// lock still matches it.
func cmdCheck(f File, base string, out io.Writer) error {
	s, lockBytes, err := readLock(f, base)
	if err != nil {
		return err
	}
	res, err := codegen.Generate(s, withLockSchemas(f, s).Config)
	if err != nil {
		return err
	}
	var drift []string
	dir := rel(base, f.Out)
	for name, want := range res.Files {
		got, err := os.ReadFile(filepath.Join(dir, name))
		switch {
		case err != nil:
			drift = append(drift, name+": missing")
		case !bytes.Equal(got, want):
			drift = append(drift, name+": differs from what the lock generates")
		}
	}
	if ents, err := os.ReadDir(dir); err == nil {
		for _, e := range ents {
			if _, ok := res.Files[e.Name()]; ok || e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
				continue
			}
			if data, err := os.ReadFile(filepath.Join(dir, e.Name())); err == nil && bytes.HasPrefix(data, []byte(genMarker)) {
				drift = append(drift, e.Name()+": stale generated file")
			}
		}
	}
	if f.DatabaseURL != "" || f.Migrations != "" {
		raw, err := readSource(f, base)
		if err != nil {
			return err
		}
		lock, _, err := catalog.Seal(raw)
		if err != nil {
			return err
		}
		if !bytes.Equal(lock, lockBytes) {
			drift = append(drift, f.Lock+": schema source has changed")
		}
	}
	sort.Strings(drift)
	if len(drift) > 0 {
		return fmt.Errorf("generated code is out of date; run `qlx gen`:\n  %s", strings.Join(drift, "\n  "))
	}
	fmt.Fprintln(out, "qlx: up to date")
	return nil
}

// cmdVerify compares a live database to schema.lock using the same
// fingerprint query the generated package embeds.
func cmdVerify(f File, base string, out io.Writer) error {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = f.DatabaseURL
	}
	if url == "" {
		return errors.New("verify needs DATABASE_URL or database_url in qlx.json")
	}
	s, _, err := readLock(f, base)
	if err != nil {
		return err
	}
	conn := pgtool.FromURL(url)
	schemas, err := schemasFor(f, conn)
	if err != nil {
		return err
	}
	live, err := catalog.Load(conn, schemas)
	if err != nil {
		return err
	}
	have := map[string]string{}
	for _, t := range live.Tables {
		have[t.Schema+"."+t.Name] = t.Fingerprint
	}
	var bad []string
	for _, t := range s.Tables {
		k := t.Schema + "." + t.Name
		switch got, ok := have[k]; {
		case !ok:
			bad = append(bad, k+": missing from the database")
		case got != t.Fingerprint:
			bad = append(bad, k+": changed")
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		return fmt.Errorf("database differs from %s:\n  %s", f.Lock, strings.Join(bad, "\n  "))
	}
	fmt.Fprintf(out, "qlx: database matches %s (%d tables)\n", f.Lock, len(s.Tables))
	return nil
}

func cmdDoctor(f File, base string, out io.Writer) error {
	ok := true
	say := func(good bool, format string, a ...any) {
		mark := "ok  "
		if !good {
			mark, ok = "FAIL", false
		}
		fmt.Fprintf(out, "%s %s\n", mark, fmt.Sprintf(format, a...))
	}
	if _, err := os.Stat(rel(base, f.Lock)); err != nil {
		say(false, "%s not found (run `qlx gen`)", f.Lock)
	} else if s, _, err := readLock(f, base); err != nil {
		say(false, "%v", err)
	} else {
		say(true, "%s valid: PostgreSQL %d, %d tables, %d enums, %d functions", f.Lock, s.Major(), len(s.Tables), len(s.Enums), len(s.Funcs))
		if res, err := codegen.Generate(s, withLockSchemas(f, s).Config); err != nil {
			say(false, "generation: %v", err)
		} else {
			report(out, res)
		}
	}
	switch {
	case f.DatabaseURL != "":
		if _, err := pgtool.FromURL(f.DatabaseURL).Query("SELECT 1"); err != nil {
			say(false, "database_url: %v", err)
		} else {
			say(true, "database_url reachable")
		}
	case f.Migrations != "":
		if bin, err := pgtool.FindBin(); err != nil {
			say(false, "migrations need PostgreSQL server binaries: %v", err)
		} else {
			say(true, "server binaries in %s", bin)
		}
	default:
		say(true, "no schema source configured; only -from-lock works")
	}
	if !ok {
		return errors.New("doctor found problems")
	}
	return nil
}
