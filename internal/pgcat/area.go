package pgcat

import (
	"bytes"
	"fmt"
	"github.com/noobmaisfier/qlx/internal/packs"
	"go/format"
	"sort"
	"strings"
	"sync"
)

// rootImport is the import path of the core package; area packages live under
// rootImport + "/pgcat/<area>" and the facade at rootImport + "/pg".
const rootImport = "github.com/noobmaisfier/qlx"

// The areas. Package names avoid the standard library's (no "time", "math",
// "json"), so an application can import an area next to the std package.
var areaDescription = map[string]string{
	"text":     "text, varchar, bpchar and name: string functions, pattern matching, collation",
	"num":      "integers, floats, numeric and money: arithmetic, rounding, trigonometry, statistics helpers",
	"datetime": "date, time, timestamp and interval: construction, extraction, arithmetic, time zones",
	"jsonb":    "json, jsonb and jsonpath: access, construction, path queries",
	"array":    "arrays and polymorphic element functions",
	"ranges":   "range and multirange types: bounds, containment, set operations",
	"fts":      "full-text search: tsvector, tsquery and text search configurations",
	"network":  "inet, cidr and MAC addresses",
	"geo":      "geometric types: point, line, box, path, polygon, circle",
	"binary":   "bytea, bit and bit varying",
	"xml":      "the xml type",
	"sys":      "system information, object identifiers, LSNs, administration and statistics functions",
	"misc":     "everything else (uuid, booleans, enums and other types without a family of their own)",
}

// areaOf picks the sub-package of a wrapper from the types it touches; the
// first matching rule in priority order wins, so a function over jsonb and
// text lands in jsonb, not text.
func areaOf(w *wrapper) string {
	var types []string
	name := ""
	if w.fn != nil {
		types = append(types, w.fn.Args...)
		types = append(types, w.fn.Ret)
		name = w.fn.Name
	} else {
		types = append(types, w.op.Left, w.op.Right, w.op.Ret)
	}
	has := func(f func(string) bool) bool {
		for _, t := range types {
			if t != "" && f(strings.TrimPrefix(t, "_")) {
				return true
			}
		}
		return false
	}
	isArray := false
	for _, t := range types {
		if strings.HasPrefix(t, "_") || t == "anyarray" || t == "anycompatiblearray" {
			isArray = true
		}
	}
	in := func(set ...string) func(string) bool {
		return func(t string) bool {
			for _, x := range set {
				if t == x {
					return true
				}
			}
			return false
		}
	}
	switch {
	case has(in("json", "jsonb", "jsonpath")):
		return "jsonb"
	case has(func(t string) bool {
		return strings.HasSuffix(t, "range") || strings.HasSuffix(t, "multirange") || t == "anyrange" || t == "anymultirange" || t == "anycompatiblerange" || t == "anycompatiblemultirange"
	}):
		return "ranges"
	case has(in("tsvector", "tsquery", "regconfig", "regdictionary", "gtsvector")):
		return "fts"
	case has(in("inet", "cidr", "macaddr", "macaddr8")):
		return "network"
	case has(in("point", "line", "lseg", "box", "path", "polygon", "circle")):
		return "geo"
	case has(in("xml")):
		return "xml"
	case strings.HasPrefix(name, "pg_") || has(func(t string) bool {
		return strings.HasPrefix(t, "reg") || strings.HasPrefix(t, "oid") || t == "pg_lsn" || t == "tid" || t == "xid" || t == "xid8" || t == "cid" ||
			t == "txid_snapshot" || t == "pg_snapshot" || strings.HasPrefix(t, "pg_") || t == "aclitem" || t == "internal" || t == "name"
	}):
		return "sys"
	case isArray || has(in("anyarray", "anyelement", "anynonarray", "anycompatible", "anycompatiblearray", "anycompatiblenonarray")):
		return "array"
	case has(in("date", "time", "timetz", "timestamp", "timestamptz", "interval", "abstime", "reltime", "tinterval")):
		return "datetime"
	case has(in("text", "varchar", "bpchar", "char")):
		return "text"
	case has(in("bytea", "bit", "varbit")):
		return "binary"
	case has(in("int2", "int4", "int8", "float4", "float8", "numeric", "money")):
		return "num"
	}
	return "misc"
}

// preferred names the argument type that gives a facade function its short
// name when a PostgreSQL function is overloaded: pg.Lower is lower(text).
var facadeOrder = []string{"text", "int8", "numeric", "float8", "timestamptz", "jsonb", "bool", "int4"}

// facadeNames are the most-used built-ins, re-exported by the pg package.
var facadeNames = []string{
	"lower", "upper", "length", "char_length", "trim", "ltrim", "rtrim", "btrim", "substr", "substring", "replace", "concat", "concat_ws",
	"left", "right", "lpad", "rpad", "repeat", "reverse", "split_part", "strpos", "position", "starts_with", "initcap", "md5", "format",
	"regexp_replace", "regexp_match", "to_char", "to_number", "to_date", "to_timestamp",
	"abs", "ceil", "ceiling", "floor", "round", "trunc", "sign", "sqrt", "power", "exp", "ln", "log", "mod", "greatest", "least", "random", "div",
	"now", "clock_timestamp", "statement_timestamp", "transaction_timestamp", "current_date", "date_trunc", "date_part", "extract", "age", "make_date", "make_interval", "make_timestamp",
	"array_length", "array_append", "array_prepend", "array_cat", "array_remove", "array_position", "array_positions", "array_to_string", "string_to_array", "cardinality", "array_dims",
	"jsonb_build_object", "jsonb_build_array", "jsonb_typeof", "jsonb_set", "jsonb_insert", "jsonb_path_exists", "jsonb_path_query_first", "jsonb_strip_nulls", "jsonb_pretty", "to_jsonb", "to_json", "json_typeof",
	"lower_inf", "upper_inf", "isempty", "lower", "upper", "range_merge",
	"to_tsvector", "to_tsquery", "plainto_tsquery", "websearch_to_tsquery", "ts_rank", "ts_headline", "setweight",
	"gen_random_uuid", "uuidv7", "uuidv4", "uuid_extract_timestamp", "encode", "decode", "digest", "casefold", "crc32", "crc32c",
	"pg_typeof", "pg_sleep", "pg_advisory_lock", "pg_advisory_xact_lock", "pg_try_advisory_lock", "pg_try_advisory_xact_lock", "pg_advisory_unlock", "pg_notify", "set_config", "current_setting", "txid_current", "pg_backend_pid",
	"nextval", "currval", "setval", "lastval", "error_on_null", "host", "family", "masklen", "network", "broadcast",
}

// facade writes package pg: the few hundred most-used built-ins re-exported
// from the area packages under their short PostgreSQL names. Only wrappers
// that are not generic can be re-exported as values; an overloaded name
// resolves to the first overload in facadeOrder.
func facade(infos []WrapperInfo, fns, ops []*wrapper) ([]byte, error) {
	byName := map[string][]*wrapper{}
	for _, w := range fns {
		if len(w.tparam) > 0 {
			continue
		}
		byName[w.fn.Name] = append(byName[w.fn.Name], w)
	}
	rank := func(w *wrapper) int {
		if len(w.fn.Args) == 0 {
			return 0
		}
		for i, t := range facadeOrder {
			if w.fn.Args[0] == t {
				return i
			}
		}
		return len(facadeOrder)
	}
	type entry struct {
		short, area, long string
		nulls             bool
	}
	var entries []entry
	seen := map[string]bool{}
	for _, n := range facadeNames {
		cands := byName[n]
		if len(cands) == 0 {
			continue
		}
		sort.SliceStable(cands, func(i, j int) bool { return rank(cands[i]) < rank(cands[j]) })
		w := cands[0]
		short := camel(n)
		if seen[short] {
			continue
		}
		seen[short] = true
		entries = append(entries, entry{short, areaOf(w), w.name, w.nulls})
	}
	areas := map[string]bool{}
	for _, e := range entries {
		areas[e.area] = true
	}
	var as []string
	for a := range areas {
		as = append(as, a)
	}
	sort.Strings(as)
	var b bytes.Buffer
	b.WriteString("// Code generated by pgcat; DO NOT EDIT.\n\n// Package pg re-exports the most-used PostgreSQL built-ins under their own\n// names (pg.Lower, pg.Now, pg.DateTrunc). Where a function is overloaded the\n// short name is the text, bigint, numeric or timestamptz form; the other\n// overloads, the nullable (N) variants and everything else are in the area\n// packages under pgcat/.\npackage pg\n\nimport (\n")
	for _, a := range as {
		fmt.Fprintf(&b, "\t%q\n", rootImport+"/pgcat/"+a)
	}
	b.WriteString(")\n\nvar (\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "\t%s = %s.%s\n", e.short, e.area, e.long)
		if e.nulls {
			fmt.Fprintf(&b, "\t%sN = %s.%sN\n", e.short, e.area, e.long)
		}
	}
	b.WriteString(")\n")
	return format.Source(b.Bytes())
}

var _ = strings.TrimSpace

var (
	packMu       sync.Mutex
	extTypeNames = map[string]bool{} // scalar entries added by the pack being generated
)

// injectPack registers the Go types of an extension's pack in the scalar map
// for the duration of one Generate call, so mapType resolves them like
// built-ins. The returned function removes them again.
func injectPack(extension string) (restore func(), err error) {
	var pack *packs.Pack
	for _, n := range packs.Names() {
		p, _ := packs.Lookup(n)
		if p.Extension == extension {
			pack = &p
			break
		}
	}
	if pack == nil {
		return nil, fmt.Errorf("pgcat: no extension pack for %q (known: %s)", extension, strings.Join(packs.Names(), ", "))
	}
	packMu.Lock()
	var added []string
	for _, t := range pack.Types {
		if _, exists := scalar[t.Name]; exists {
			continue
		}
		scalar[t.Name] = goType{expr: t.Expr, imports: t.Imports, nullOK: t.NullOK}
		extTypeNames[t.Name] = true
		added = append(added, t.Name)
	}
	return func() {
		for _, n := range added {
			delete(scalar, n)
			delete(extTypeNames, n)
		}
		packMu.Unlock()
	}, nil
}
