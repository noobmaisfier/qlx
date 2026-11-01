package pgcat

import (
	"bytes"
	"fmt"
	"go/format"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// goType describes how a PostgreSQL type appears in generated wrappers.
type goType struct {
	expr    string
	imports []string
	nullOK  bool // may be wrapped in qt.Null
	generic bool // mentions a type parameter
}

// poly collects the type parameters of one polymorphic wrapper.
type poly struct {
	params []string // in order of first use: E, C, A1, A2...
	anyN   int
}

func (p *poly) use(name string) string {
	for _, n := range p.params {
		if n == name {
			return name
		}
	}
	p.params = append(p.params, name)
	return name
}

// mapPoly maps the polymorphic pseudo-types to Go type parameters. anyelement,
// anyarray and anynonarray share E (PostgreSQL requires them to agree);
// anycompatible* share C; every "any" is independent.
func mapPoly(name string, p *poly) (goType, bool) {
	switch name {
	case "anyelement", "anynonarray":
		return goType{expr: p.use("E"), nullOK: true, generic: true}, true
	case "anyarray":
		return goType{expr: "[]" + p.use("E"), generic: true}, true
	case "anycompatible", "anycompatiblenonarray":
		return goType{expr: p.use("C"), nullOK: true, generic: true}, true
	case "anycompatiblearray":
		return goType{expr: "[]" + p.use("C"), generic: true}, true
	case "anyrange":
		return goType{expr: "qt.Range[" + p.use("E") + "]", generic: true}, true
	case "anymultirange":
		return goType{expr: "qt.Multirange[" + p.use("E") + "]", generic: true}, true
	case "anycompatiblerange":
		return goType{expr: "qt.Range[" + p.use("C") + "]", generic: true}, true
	case "anycompatiblemultirange":
		return goType{expr: "qt.Multirange[" + p.use("C") + "]", generic: true}, true
	case "anyenum":
		return goType{expr: p.use("N"), nullOK: true, generic: true}, true
	case "any":
		p.anyN++
		return goType{expr: p.use(fmt.Sprintf("A%d", p.anyN)), nullOK: true, generic: true}, true
	}
	return goType{}, false
}

// orderedSetAgg lists the aggregates that take WITHIN GROUP (ORDER BY ...).
// They are written by hand (PercentileCont, PercentileDisc, Mode) or are
// hypothetical-set aggregates whose arguments must match the sort key.
var orderedSetAgg = map[string]bool{
	"mode": true, "percentile_cont": true, "percentile_disc": true,
	"rank": true, "dense_rank": true, "percent_rank": true, "cume_dist": true,
}

// neverNullAgg lists aggregates that return 0 rather than NULL on no rows.
var neverNullAgg = map[string]bool{"count": true, "regr_count": true}

// polyBase is the type parameter a generic Go type is built on:
// []E, qt.Range[E] and qt.Multirange[E] are all based on E.
func polyBase(expr string) string {
	expr = strings.TrimPrefix(expr, "[]")
	for _, p := range []string{"qt.Range[", "qt.Multirange["} {
		if strings.HasPrefix(expr, p) {
			return strings.TrimSuffix(strings.TrimPrefix(expr, p), "]")
		}
	}
	return expr
}

// strictButNullable lists built-ins that are STRICT (NULL in, NULL out) yet
// also return NULL for non-NULL input: a missing JSON key, an empty array's
// bounds, an unbounded range, a name that resolves to nothing. The catalog's
// proisstrict cannot say so, and typing such a result as non-null would make
// the first absent key a scan error. Wrong in this direction is safe: a Null
// result is a superset.
var strictButNullable = map[string]bool{
	"array_dims": true, "array_length": true, "array_lower": true, "array_ndims": true, "array_upper": true,
	"col_description": true, "obj_description": true, "shobj_description": true,
	"json_array_element": true, "json_array_element_text": true, "json_extract_path": true, "json_extract_path_text": true,
	"json_object_field": true, "json_object_field_text": true,
	"jsonb_array_element": true, "jsonb_array_element_text": true, "jsonb_extract_path": true, "jsonb_extract_path_text": true,
	"jsonb_object_field": true, "jsonb_object_field_text": true,
	"jsonb_path_exists": true, "jsonb_path_exists_opr": true, "jsonb_path_exists_tz": true,
	"jsonb_path_match": true, "jsonb_path_match_opr": true, "jsonb_path_match_tz": true,
	"jsonb_path_query_first": true, "jsonb_path_query_first_tz": true,
	"pg_get_serial_sequence": true, "pg_get_expr": true, "pg_get_constraintdef": true, "pg_get_functiondef": true,
	"pg_get_indexdef": true, "pg_get_triggerdef": true, "pg_get_viewdef": true,
	"pg_tablespace_location": true, "pg_relation_filepath": true,
	"regexp_match": true, "regexp_substr": true, // no match
	"fetchval": true, // hstore -> key
}

// strictButNullablePrefix matches families: the statistics functions return
// NULL for an object with no statistics, to_reg* for a name that resolves to
// nothing, pg_last_* outside recovery.
var strictButNullablePrefix = []string{"pg_stat_get_", "to_reg", "pg_last_"}

func mayReturnNull(f *Fn) bool {
	name := f.Name
	switch name {
	case "lower", "upper": // text lower/upper never return NULL; range bounds do
		return len(f.Args) == 1 && (f.Args[0] == "anyrange" || f.Args[0] == "anymultirange")
	case "substring": // only the regular-expression forms can fail to match
		return len(f.Args) >= 2 && f.Args[1] == "text"
	}
	if strictButNullable[name] {
		return true
	}
	for _, p := range strictButNullablePrefix {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

var scalar = map[string]goType{
	"bool":            {"bool", nil, true, false},
	"int2":            {"int16", nil, true, false},
	"int4":            {"int32", nil, true, false},
	"int8":            {"int64", nil, true, false},
	"float4":          {"float32", nil, true, false},
	"float8":          {"float64", nil, true, false},
	"numeric":         {"qt.Numeric", nil, true, false},
	"text":            {"string", nil, true, false},
	"varchar":         {"string", nil, true, false},
	"bpchar":          {"string", nil, true, false},
	"name":            {"string", nil, true, false},
	"bytea":           {"[]byte", nil, false, false},
	"uuid":            {"uuid.UUID", []string{"uuid"}, true, false},
	"date":            {"qt.Date", nil, true, false},
	"timestamp":       {"qt.LocalTime", nil, true, false},
	"timestamptz":     {"time.Time", []string{"time"}, true, false},
	"interval":        {"qt.Interval", nil, true, false},
	"json":            {"qt.JSONB[json.RawMessage]", []string{"encoding/json"}, true, false},
	"jsonb":           {"qt.JSONB[json.RawMessage]", []string{"encoding/json"}, true, false},
	"inet":            {"netip.Addr", []string{"net/netip"}, true, false},
	"oid":             {"uint32", nil, true, false},
	"cidr":            {"netip.Prefix", []string{"net/netip"}, true, false},
	"time":            {"qt.TimeOfDay", nil, true, false},
	"timetz":          {"qt.TimeTZ", nil, true, false},
	"tsvector":        {"qt.TSVector", nil, true, false},
	"tsquery":         {"qt.TSQuery", nil, true, false},
	"bit":             {"qt.Bit", nil, true, false},
	"varbit":          {"qt.Bit", nil, true, false},
	"point":           {"qt.Point", nil, true, false},
	"line":            {"qt.Line", nil, true, false},
	"lseg":            {"qt.Lseg", nil, true, false},
	"box":             {"qt.Box", nil, true, false},
	"path":            {"qt.Path", nil, true, false},
	"polygon":         {"qt.Polygon", nil, true, false},
	"circle":          {"qt.Circle", nil, true, false},
	"money":           {"qt.Money", nil, true, false},
	"macaddr":         {"qt.MacAddr", nil, true, false},
	"macaddr8":        {"qt.MacAddr8", nil, true, false},
	"pg_lsn":          {"qt.LSN", nil, true, false},
	"xml":             {"qt.XML", nil, true, false},
	"regclass":        {"qt.RegClass", nil, true, false},
	"regtype":         {"qt.RegType", nil, true, false},
	"regproc":         {"qt.RegProc", nil, true, false},
	"regprocedure":    {"qt.RegProcedure", nil, true, false},
	"regoper":         {"qt.RegOper", nil, true, false},
	"regoperator":     {"qt.RegOperator", nil, true, false},
	"regnamespace":    {"qt.RegNamespace", nil, true, false},
	"regrole":         {"qt.RegRole", nil, true, false},
	"regconfig":       {"qt.RegConfig", nil, true, false},
	"regdictionary":   {"qt.RegDict", nil, true, false},
	"xid":             {"qt.Xid", nil, true, false},
	"xid8":            {"qt.Xid8", nil, true, false},
	"cid":             {"qt.Cid", nil, true, false},
	"tid":             {"qt.Tid", nil, true, false},
	"char":            {"qt.Char", nil, true, false},
	"jsonpath":        {"qt.JSONPath", nil, true, false},
	"aclitem":         {"qt.ACLItem", nil, true, false},
	"oidvector":       {"qt.OIDVector", nil, true, false},
	"int2vector":      {"qt.Int2Vector", nil, true, false},
	"oid8":            {"qt.Oid8", nil, true, false},
	"pg_snapshot":     {"qt.Snapshot", nil, true, false},
	"txid_snapshot":   {"qt.Snapshot", nil, true, false},
	"pg_node_tree":    {"qt.NodeTree", nil, true, false},
	"refcursor":       {"qt.RefCursor", nil, true, false},
	"regcollation":    {"qt.RegCollation", nil, true, false},
	"regdatabase":     {"qt.RegDatabase", nil, true, false},
	"pg_ndistinct":    {"qt.NDistinct", nil, true, false},
	"pg_dependencies": {"qt.Dependencies", nil, true, false},
	"pg_mcv_list":     {"qt.MCVList", nil, true, false},
	"int4range":       {"qt.Range[int32]", nil, true, false},
	"int8range":       {"qt.Range[int64]", nil, true, false},
	"numrange":        {"qt.Range[qt.Numeric]", nil, true, false},
	"daterange":       {"qt.Range[qt.Date]", nil, true, false},
	"tsrange":         {"qt.Range[qt.LocalTime]", nil, true, false},
	"tstzrange":       {"qt.Range[time.Time]", []string{"time"}, true, false},
	"int4multirange":  {"qt.Multirange[int32]", nil, true, false},
	"int8multirange":  {"qt.Multirange[int64]", nil, true, false},
	"nummultirange":   {"qt.Multirange[qt.Numeric]", nil, true, false},
	"datemultirange":  {"qt.Multirange[qt.Date]", nil, true, false},
	"tsmultirange":    {"qt.Multirange[qt.LocalTime]", nil, true, false},
	"tstzmultirange":  {"qt.Multirange[time.Time]", []string{"time"}, true, false},
}

// mapType resolves a pg typname; reason explains a failure.
func mapType(name string) (t goType, reason string) {
	if strings.HasPrefix(name, "_") {
		e, why := mapType(name[1:])
		if why != "" {
			return goType{}, why
		}
		return goType{"[]" + e.expr, e.imports, false, false}, ""
	}
	if g, ok := scalar[name]; ok {
		return g, ""
	}
	switch name {
	case "any", "anyelement", "anyarray", "anynonarray", "anyenum", "anyrange", "anymultirange",
		"anycompatible", "anycompatiblearray", "anycompatiblenonarray", "anycompatiblerange", "anycompatiblemultirange":
		return goType{}, "polymorphic"
	case "record", "internal", "cstring", "trigger", "event_trigger", "language_handler", "void", "unknown":
		return goType{}, "pseudo-type " + name
	}
	return goType{}, "type " + name + " has no Go mapping yet"
}

// Report is the coverage ledger of one generation: how many catalog entries
// got a wrapper and why the others did not.
type Report struct {
	Funcs, FuncsWrapped int
	Ops, OpsWrapped     int
	Skipped             map[string]int // reason -> count
}

// Result is the output of Generate.
type Result struct {
	Files  map[string][]byte
	Report Report
	// Wrappers lists every generated wrapper with its Go name and PostgreSQL
	// identity; Docs turns it into the reference pages.
	Wrappers []WrapperInfo
	Areas    []string // sub-package names, sorted
}

// WrapperInfo describes one generated wrapper.
type WrapperInfo struct {
	Go       string // Go identifier in the generated package
	IsOp     bool
	Sig      string // PostgreSQL signature, "lower(text)" or "text || text"
	Ret      string // PostgreSQL result type
	Since    int
	Until    int
	Volatile string // "immutable", "stable" or "volatile" (functions only)
	Area     string // sub-package the wrapper lives in
	Nullable bool   // result may be NULL
}

var pgVarRe = regexp.MustCompile(`\bpg[A-Z][A-Za-z0-9]*\b`)

var tparamRe = regexp.MustCompile(`\b(E|C|N|A[0-9]+)\b`)

var ioSuffix = regexp.MustCompile(`(in|out|recv|send|typmodin|typmodout)$`)

// internalName reports whether an operator's implementing function is named
// after a type (int4pl, texteq, float8_div): those are plumbing behind the
// operator, not API. Documented functions that also back an operator
// (array_append, array_cat) carry no type prefix and are kept.
func internalName(name string, types map[string]bool) bool {
	for t := range types {
		t = strings.TrimPrefix(t, "_")
		if len(t) >= 3 && strings.HasPrefix(name, t) {
			return true
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func camel(s string) string {
	var b strings.Builder
	up := true
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			up = true
			continue
		}
		if up {
			b.WriteRune(unicode.ToUpper(r))
			up = false
		} else {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" || unicode.IsDigit([]rune(out)[0]) {
		out = "X" + out
	}
	return out
}

var opWord = map[rune]string{
	'+': "Plus", '-': "Minus", '*': "Star", '/': "Slash", '<': "Lt", '>': "Gt", '=': "Eq",
	'~': "Tilde", '!': "Bang", '@': "At", '#': "Hash", '%': "Pct", '^': "Caret", '&': "Amp",
	'|': "Pipe", '`': "Tick", '?': "Q",
}

func opName(sym string) string {
	var b strings.Builder
	for _, r := range sym {
		b.WriteString(opWord[r])
	}
	return b.String()
}

type wrapper struct {
	fn     *Fn
	op     *Op
	params []goType
	ret    goType
	name   string
	nulls  bool // generate the N variant
	// variadic is "" for a fixed signature, "value" for VARIADIC "any" (the
	// caller passes any qlx.Value, each placed in the call as written) and
	// "operand" for VARIADIC T[] (every extra argument is an Operand[T]).
	variadic string
	varType  goType
	retNul   bool     // result may be NULL
	outs     []outCol // output columns of a function with OUT arguments
	srf      bool     // scalar set-returning function: emitted as a qlx.TableFunc
	tparam   []string
}

// outCol is one output column of a row-source function.
type outCol struct {
	name string
	typ  goType
}

// Generate emits the pgcat area packages and the pg facade. Every entry in the ledger is wrapped, and
// its version range travels with it: an entry with Since renders only on
// servers at or above it, one with Until only below it, so a function that a
// later release removed stays usable for the older servers you still target
// and fails at render time (not at run time) for the newer ones.
func Generate(l *Ledger, pkg string) (*Result, error) {
	res := &Result{Files: map[string][]byte{}, Report: Report{Skipped: map[string]int{}}}
	skip := func(why string) { res.Report.Skipped[why]++ }

	extPkg := ""
	if l.Extension != "" {
		restore, err := injectPack(l.Extension)
		if err != nil {
			return nil, err
		}
		defer restore()
		extPkg = strings.ReplaceAll(l.Extension, "_", "")
	}
	areaFor := func(w *wrapper) string {
		if extPkg != "" {
			return extPkg
		}
		return areaOf(w)
	}
	dirOf := func(area string) string {
		if extPkg != "" {
			return "ext/" + area
		}
		return area
	}

	typeNames := map[string]bool{}
	for _, f := range l.Funcs {
		for _, a := range f.Args {
			typeNames[a] = true
		}
		typeNames[f.Ret] = true
	}
	opProcs := map[string]bool{}
	for _, o := range l.Ops {
		opProcs[o.Proc] = true
	}
	castFns := map[string]bool{} // cast functions are named after the target type
	for _, c := range l.Casts {
		castFns[c.To] = true
	}
	byName := map[string]*Fn{}
	for i := range l.Funcs {
		byName[l.Funcs[i].Name] = &l.Funcs[i]
	}

	// ---- functions
	var ws []*wrapper
	for i := range l.Funcs {
		f := &l.Funcs[i]
		res.Report.Funcs++
		switch {
		case f.Kind == "a" && len(f.Args) == 0:
			skip("count(*) form (hand-written Count/CountStar)")
			continue
		case f.Kind == "a" && orderedSetAgg[f.Name]:
			skip("ordered-set aggregate (hand-written in the core package)")
			continue
		case f.Kind == "w":
			skip("window function (hand-written in the core package)")
			continue
		case f.Ret == "trigger" || f.Ret == "internal" || f.Ret == "event_trigger":
			skip("result: " + f.Ret)
			continue
		case f.Variadic && (len(f.Args) == 0 || f.Args[len(f.Args)-1] == "internal"):
			skip("variadic")
			continue
		case opProcs[f.Name] && internalName(f.Name, typeNames):
			skip("operator implementation (use the operator wrapper)")
			continue
		case castFns[f.Name]:
			skip("cast function (use Cast)")
			continue
		}
		if m := ioSuffix.FindString(f.Name); m != "" && typeNames[strings.TrimSuffix(f.Name, m)] {
			skip("type input/output function")
			continue
		}
		w := &wrapper{fn: f}
		pc := &poly{}
		ok := true
		for ai, a := range f.Args {
			if f.Variadic && ai == len(f.Args)-1 {
				if a == "any" {
					w.variadic = "value"
					continue
				}
				if strings.HasPrefix(a, "_") {
					et, why := mapType(a[1:])
					if why != "" {
						skip(why)
						ok = false
						break
					}
					w.variadic, w.varType = "operand", et
					continue
				}
				skip("variadic")
				ok = false
				break
			}
			t, isPoly := mapPoly(a, pc)
			why := ""
			if !isPoly {
				t, why = mapType(a)
			}
			if why != "" {
				skip(why)
				ok = false
				break
			}
			w.params = append(w.params, t)
		}
		if !ok {
			continue
		}
		if f.OutModes || f.Ret == "record" {
			// A row source whose columns come from the ledger (pgcat enrich).
			if len(f.Outs) == 0 {
				skip("OUT columns not in the ledger (run pgcat enrich)")
				continue
			}
			okc := true
			for _, o := range f.Outs {
				ct, why := mapType(o.Type)
				if why != "" || o.Name == "" {
					skip("OUT column " + o.Name + ": " + why)
					okc = false
					break
				}
				w.outs = append(w.outs, outCol{name: o.Name, typ: ct})
			}
			if !okc {
				continue
			}
			w.srf = true
			w.tparam = pc.params
			ws = append(ws, w)
			res.Report.FuncsWrapped++
			continue
		}
		rt, isPoly := mapPoly(f.Ret, pc)
		why := ""
		if f.RetSet {
			// A scalar set-returning function is a row source: the caller names
			// the one column, so the result type only has to be a base type.
			if !isPoly {
				if _, why = mapType(f.Ret); why != "" {
					skip("result: " + why)
					continue
				}
			}
			w.srf = true
			w.tparam = pc.params
			ws = append(ws, w)
			res.Report.FuncsWrapped++
			continue
		}
		if isPoly {
			// A result type parameter must be fixed by an argument.
			if f.Ret == "any" || !contains(pc.params, polyBase(rt.expr)) {
				skip("result: polymorphic with no argument to infer it from")
				continue
			}
		} else if f.Ret == "void" {
			rt = goType{expr: "qt.Void"} // SELECT pg_advisory_lock(k) runs it; the result carries nothing
		} else {
			rt, why = mapType(f.Ret)
		}
		if why != "" {
			skip("result: " + why)
			continue
		}
		w.ret = rt
		w.tparam = pc.params
		w.retNul = (!f.Strict || mayReturnNull(f)) && rt.nullOK
		if f.Kind == "a" {
			w.retNul = rt.nullOK && !neverNullAgg[f.Name] // an aggregate over no rows is NULL
		}
		allNullable := f.Kind != "a" && len(w.params) > 0 && f.Strict && rt.nullOK && len(pc.params) == 0 && w.variadic == ""
		for _, p := range w.params {
			if !p.nullOK {
				allNullable = false
			}
		}
		w.nulls = allNullable
		ws = append(ws, w)
		res.Report.FuncsWrapped++
	}
	// A function whose record changes shape between majors has one entry, and
	// one wrapper, per shape: tell them apart by the versions they exist in
	// (PgStatGetWalUpTo17, PgStatGetWalV18, PgStatGetWalFrom19).
	shapes := map[string]int{}
	for _, w := range ws {
		shapes[w.fn.Sig()]++
	}
	assignNames(ws, areaFor, func(w *wrapper) (string, string) {
		var parts []string
		for _, a := range w.fn.Args {
			parts = append(parts, camel(strings.TrimPrefix(a, "_")+map[bool]string{true: "Array", false: ""}[strings.HasPrefix(a, "_")]))
		}
		if shapes[w.fn.Sig()] > 1 {
			parts = append(parts, versionTag(w.fn.Since, w.fn.Until))
		}
		return camel(w.fn.Name), strings.Join(parts, "")
	})

	// ---- operators
	var os []*wrapper
	for i := range l.Ops {
		o := &l.Ops[i]
		res.Report.Ops++
		w := &wrapper{op: o}
		pc := &poly{}
		bad := ""
		mapOne := func(name string) goType {
			if t, ok := mapPoly(name, pc); ok {
				return t
			}
			t, why := mapType(name)
			if why != "" && bad == "" {
				bad = why
			}
			return t
		}
		if o.Left != "" {
			w.params = append(w.params, mapOne(o.Left))
		}
		w.params = append(w.params, mapOne(o.Right))
		w.ret = mapOne(o.Ret)
		if bad == "" && w.ret.generic && !contains(pc.params, polyBase(w.ret.expr)) {
			bad = "polymorphic result with no argument to infer it from"
		}
		w.tparam = pc.params
		if bad != "" {
			skip("operator: " + bad)
			continue
		}
		if p := byName[o.Proc]; p != nil {
			w.retNul = (!p.Strict || mayReturnNull(p)) && w.ret.nullOK
		} else {
			w.retNul = w.ret.nullOK
		}
		os = append(os, w)
		res.Report.OpsWrapped++
	}
	assignNames(os, areaFor, func(w *wrapper) (string, string) {
		o := w.op
		suffix := camel(strings.TrimPrefix(o.Left, "_")) + camel(strings.TrimPrefix(o.Right, "_"))
		if strings.HasPrefix(o.Left, "_") {
			suffix = strings.Replace(suffix, camel(strings.TrimPrefix(o.Left, "_")), camel(strings.TrimPrefix(o.Left, "_"))+"Array", 1)
		}
		return "Op" + opName(o.Name), suffix
	}, true)

	// ---- emit: one sub-package per area; inside it functions by first
	// letter and operators in one file, plus the type variables it uses and a
	// server-side PREPARE test of its own wrappers.
	for _, w := range append(append([]*wrapper{}, ws...), os...) {
		since, until := w.versions()
		wi := WrapperInfo{Go: w.name, Since: since, Until: until, Nullable: w.retNul, Area: areaFor(w)}
		if w.fn != nil {
			wi.Sig, wi.Ret = w.fn.Sig(), w.fn.Ret
			wi.Volatile = map[string]string{"i": "immutable", "s": "stable", "v": "volatile"}[w.fn.Volatility]
		} else {
			wi.IsOp, wi.Sig, wi.Ret = true, strings.TrimSpace(w.op.Sig()), w.op.Ret
		}
		res.Wrappers = append(res.Wrappers, wi)
	}
	sort.Slice(res.Wrappers, func(i, j int) bool {
		a, b := res.Wrappers[i], res.Wrappers[j]
		if a.IsOp != b.IsOp {
			return !a.IsOp
		}
		return a.Sig < b.Sig
	})
	byArea := map[string][]*wrapper{}
	for _, w := range append(append([]*wrapper{}, ws...), os...) {
		a := areaFor(w)
		byArea[a] = append(byArea[a], w)
	}
	var areas []string
	for a := range byArea {
		areas = append(areas, a)
	}
	sort.Strings(areas)
	res.Areas = areas
	for _, area := range areas {
		list := byArea[area]
		var fnList, opList []*wrapper
		for _, w := range list {
			if w.op != nil {
				opList = append(opList, w)
			} else {
				fnList = append(fnList, w)
			}
		}
		byLetter := map[string][]*wrapper{}
		for _, w := range fnList {
			k := strings.ToLower(w.name[:1])
			if k < "a" || k > "z" {
				k = "x"
			}
			byLetter[k] = append(byLetter[k], w)
		}
		var body bytes.Buffer
		for k, grp := range byLetter {
			src, err := emit(l, area, grp)
			if err != nil {
				return nil, err
			}
			res.Files[dirOf(area)+"/funcs_"+k+".go"] = src
			body.Write(src)
		}
		if len(opList) > 0 {
			src, err := emit(l, area, opList)
			if err != nil {
				return nil, err
			}
			res.Files[dirOf(area)+"/ops.go"] = src
			body.Write(src)
		}
		tf, err := typesFile(l, area, body.String(), l.Schema)
		if err != nil {
			return nil, err
		}
		if tf != nil {
			res.Files[dirOf(area)+"/types.go"] = tf
		}
		desc := fmt.Sprintf("wraps the PostgreSQL built-ins of the %q area: %s.\n// Import only the areas you use; unused ones are never linked", area, areaDescription[area])
		if extPkg != "" {
			desc = fmt.Sprintf("wraps the functions and operators of the PostgreSQL extension %s (schema %s).\n// Generated from a server where the extension is installed", l.Extension, l.Schema)
		}
		res.Files[dirOf(area)+"/doc.go"] = []byte(fmt.Sprintf("// Code generated by pgcat; DO NOT EDIT.\n\n// Package %s %s.\npackage %s\n", area, desc, area))
		pt, err := prepareTest(area, list, l.Extension)
		if err != nil {
			return nil, err
		}
		res.Files[dirOf(area)+"/prepare_test.go"] = pt
	}
	if extPkg == "" {
		fc, err := facade(res.Wrappers, ws, os)
		if err != nil {
			return nil, err
		}
		res.Files["../pg/pg.go"] = fc
	}
	return res, nil
}

// assignNames gives every wrapper a unique Go name. A name used once stays
// bare; overloads get their argument types appended. forceSuffix suffixes
// everything (operators).
func assignNames(ws []*wrapper, areaFor func(*wrapper) string, parts func(*wrapper) (base, suffix string), force ...bool) {
	// Names only have to be unique inside their package, so overloads are
	// told apart per area: text.Lower and ranges.Lower both keep their bare name.
	byArea := map[string][]*wrapper{}
	for _, w := range ws {
		a := areaFor(w)
		byArea[a] = append(byArea[a], w)
	}
	var areas []string
	for a := range byArea {
		areas = append(areas, a)
	}
	sort.Strings(areas)
	for _, a := range areas {
		assignNamesIn(byArea[a], parts, force...)
	}
}

// versionTag names the majors an entry exists in, for the suffix of a wrapper
// whose shape differs between versions.
func versionTag(since, until int) string {
	switch {
	case since == 0 && until > 0:
		return fmt.Sprintf("UpTo%d", until-1)
	case since > 0 && until == 0:
		return fmt.Sprintf("From%d", since)
	case since > 0 && until == since+1:
		return fmt.Sprintf("V%d", since)
	case since > 0:
		return fmt.Sprintf("V%dTo%d", since, until-1)
	}
	return ""
}

func assignNamesIn(ws []*wrapper, parts func(*wrapper) (base, suffix string), force ...bool) {
	groups := map[string][]*wrapper{}
	for _, w := range ws {
		b, _ := parts(w)
		groups[b] = append(groups[b], w)
	}
	used := map[string]bool{}
	var keys []string
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		g := groups[k]
		for _, w := range g {
			b, s := parts(w)
			name := b
			if len(g) > 1 || (len(force) > 0 && force[0]) {
				name = b + s
			}
			for i := 2; used[name] || used[name+"N"]; i++ {
				name = fmt.Sprintf("%s%s%d", b, s, i)
			}
			used[name] = true
			if w.nulls {
				used[name+"N"] = true
			}
			w.name = name
		}
	}
}

func emit(l *Ledger, pkg string, ws []*wrapper) ([]byte, error) {
	sort.Slice(ws, func(i, j int) bool { return ws[i].name < ws[j].name })
	imports := map[string]bool{"github.com/noobmaisfier/qlx": true}
	var body bytes.Buffer
	for _, w := range ws {
		for _, p := range w.params {
			for _, im := range p.imports {
				imports[im] = true
			}
		}
		for _, im := range w.ret.imports {
			imports[im] = true
		}
		emitOne(&body, w, false, imports)
		if w.nulls {
			emitOne(&body, w, true, imports)
		}
	}
	var out bytes.Buffer
	source := "PostgreSQL built-in catalog"
	if l.Extension != "" {
		source = "PostgreSQL extension " + l.Extension
	}
	fmt.Fprintf(&out, "// Code generated by pgcat; DO NOT EDIT.\n// Source: %s, versions %v.\n\npackage %s\n\nimport (\n", source, l.Versions, pkg)
	var paths []string
	for p := range imports {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		fmt.Fprintf(&out, "\t%q\n", p)
	}
	out.WriteString(")\n\n")
	out.Write(body.Bytes())
	src, err := format.Source(out.Bytes())
	if err != nil {
		return out.Bytes(), fmt.Errorf("generated pgcat code does not parse: %w", err)
	}
	return src, nil
}

func emitOne(out *bytes.Buffer, w *wrapper, nullable bool, imports map[string]bool) {
	var b bytes.Buffer
	emitOneTo(&b, w, nullable, imports)
	schema := "pg_catalog"
	if w.fn != nil && w.fn.Schema != "" {
		schema = w.fn.Schema
	} else if w.op != nil && w.op.Schema != "" {
		schema = w.op.Schema
	}
	text := b.String()
	if schema != "pg_catalog" {
		text = strings.ReplaceAll(text, `"pg_catalog"`, strconv.Quote(schema))
		text = strings.ReplaceAll(text, "calls pg_catalog.", "calls "+schema+".")
	}
	out.WriteString(text)
}

func emitOneTo(b *bytes.Buffer, w *wrapper, nullable bool, imports map[string]bool) {
	name := w.name
	if nullable {
		name += "N"
	}
	var params, args []string
	taken := map[string]bool{}
	pname := func(i int) string {
		n := fmt.Sprintf("a%d", i+1)
		if w.fn != nil && i < len(w.fn.ArgNames) && w.fn.ArgNames[i] != "" && isIdent(w.fn.ArgNames[i]) {
			n = paramName(w.fn.ArgNames[i], i)
		}
		for taken[n] {
			n += "_"
		}
		taken[n] = true
		return n
	}
	for i, p := range w.params {
		t := p.expr
		if nullable {
			t = "qt.Null[" + t + "]"
			imports["github.com/noobmaisfier/qlx/qt"] = true
		}
		if strings.Contains(t, "qt.") {
			imports["github.com/noobmaisfier/qlx/qt"] = true
		}
		n := pname(i)
		params = append(params, fmt.Sprintf("%s qlx.Operand[%s]", n, t))
		if p.generic {
			args = append(args, "qlx.ArgP("+n+")")
		} else {
			args = append(args, "qlx.ArgT("+n+", "+typeVar(w, i)+")")
		}
	}
	callArgs := joinArgs(args)
	switch w.variadic {
	case "value":
		params = append(params, "rest ...qlx.Value")
		callArgs = ", " + "append([]qlx.Value{" + strings.Join(args, ", ") + "}, rest...)..."
		if len(args) == 0 {
			callArgs = ", rest..."
		}
	case "operand":
		for _, im := range w.varType.imports {
			imports[im] = true
		}
		if strings.Contains(w.varType.expr, "qt.") {
			imports["github.com/noobmaisfier/qlx/qt"] = true
		}
		params = append(params, "rest ..."+"qlx.Operand["+w.varType.expr+"]")
		callArgs = ", " + "append([]qlx.Value{" + strings.Join(args, ", ") + "}, qlx.ArgsT(rest, " + variadicTypeVar(w) + ")...)..."
	}
	ret := w.ret.expr
	if nullable || w.retNul {
		ret = "qt.Null[" + ret + "]"
		if !w.ret.nullOK {
			ret = w.ret.expr
		}
	}
	if strings.Contains(ret, "qt.") {
		imports["github.com/noobmaisfier/qlx/qt"] = true
	}

	since, until := w.versions()
	switch {
	case w.fn != nil:
		fmt.Fprintf(b, "// %s calls pg_catalog.%s(%s).", name, w.fn.Name, strings.Join(w.fn.Args, ", "))
	default:
		o := w.op
		fmt.Fprintf(b, "// %s applies the operator %s", name, o.Name)
		if o.Left != "" {
			fmt.Fprintf(b, " to (%s, %s).", o.Left, o.Right)
		} else {
			fmt.Fprintf(b, " to (%s).", o.Right)
		}
	}
	if since > 0 {
		fmt.Fprintf(b, " Requires PostgreSQL %d.", since)
	}
	if until > 0 {
		fmt.Fprintf(b, " Removed in PostgreSQL %d.", until)
	}
	if w.fn != nil && w.fn.Volatility == "v" {
		b.WriteString(" VOLATILE.")
	}
	if nullable {
		b.WriteString(" Arguments may be NULL; the result is NULL if any is.")
	}
	b.WriteString("\n")
	tp := ""
	if len(w.tparam) > 0 {
		tp = "[" + strings.Join(w.tparam, ", ") + " any]"
	}
	if w.srf {
		fmt.Fprintf(b, "func %s%s(%s) qlx.TableFunc {\n", name, tp, strings.Join(params, ", "))
		fmt.Fprintf(b, "\treturn qlx.FuncRange(%d, %d, \"pg_catalog\", %q%s)\n}\n\n", since, until, w.fn.Name, callArgs)
		if len(w.outs) > 0 {
			emitOutCols(b, w, name, imports)
		}
		return
	}
	if w.fn != nil && w.fn.Kind == "a" {
		fmt.Fprintf(b, "func %s%s(%s) qlx.AggExpr[%s] {\n", name, tp, strings.Join(params, ", "), ret)
		fmt.Fprintf(b, "\treturn qlx.AggCall[%s](%d, %d, \"pg_catalog\", %q%s)\n}\n\n", ret, since, until, w.fn.Name, callArgs)
		return
	}
	fmt.Fprintf(b, "func %s%s(%s) qlx.Expr[%s] {\n", name, tp, strings.Join(params, ", "), ret)
	switch {
	case w.fn != nil && until > 0:
		fmt.Fprintf(b, "\treturn qlx.FuncCallRange[%s](%d, %d, \"pg_catalog\", %q%s)\n", ret, since, until, w.fn.Name, callArgs)
	case w.fn == nil && until > 0 && w.op.Left == "":
		fmt.Fprintf(b, "\treturn qlx.PrefixOpRange[%s](%d, %d, \"pg_catalog\", %q, %s)\n", ret, since, until, w.op.Name, args[0])
	case w.fn == nil && until > 0:
		fmt.Fprintf(b, "\treturn qlx.BinaryOpRange[%s](%d, %d, \"pg_catalog\", %q, %s, %s)\n", ret, since, until, w.op.Name, args[0], args[1])
	case w.fn != nil && since > 0:
		fmt.Fprintf(b, "\treturn qlx.FuncCallSince[%s](%d, \"pg_catalog\", %q%s)\n", ret, since, w.fn.Name, callArgs)
	case w.fn != nil:
		fmt.Fprintf(b, "\treturn qlx.FuncCall[%s](\"pg_catalog\", %q%s)\n", ret, w.fn.Name, callArgs)
	case w.op.Left == "" && since > 0:
		fmt.Fprintf(b, "\treturn qlx.PrefixOpSince[%s](%d, \"pg_catalog\", %q, %s)\n", ret, since, w.op.Name, args[0])
	case w.op.Left == "":
		fmt.Fprintf(b, "\treturn qlx.PrefixOp[%s](\"pg_catalog\", %q, %s)\n", ret, w.op.Name, args[0])
	case since > 0:
		fmt.Fprintf(b, "\treturn qlx.BinaryOpSince[%s](%d, \"pg_catalog\", %q, %s, %s)\n", ret, since, w.op.Name, args[0], args[1])
	default:
		fmt.Fprintf(b, "\treturn qlx.BinaryOp[%s](\"pg_catalog\", %q, %s, %s)\n", ret, w.op.Name, args[0], args[1])
	}
	b.WriteString("}\n\n")
}

// emitOutCols writes the typed columns of a function with OUT arguments:
//
//	fn := text.PgGetKeywords().As("k", text.PgGetKeywordsCols.Fields()...)
//	q.Ref(fn, text.PgGetKeywordsCols.Word)
func emitOutCols(b *bytes.Buffer, w *wrapper, name string, imports map[string]bool) {
	type col struct{ field, typ, pg string }
	var cols []col
	seen := map[string]bool{"Fields": true}
	for _, o := range w.outs {
		t := o.typ.expr
		if o.typ.nullOK {
			t = "qt.Null[" + t + "]"
			imports["github.com/noobmaisfier/qlx/qt"] = true
		}
		if strings.Contains(t, "qt.") {
			imports["github.com/noobmaisfier/qlx/qt"] = true
		}
		for _, im := range o.typ.imports {
			imports[im] = true
		}
		f := camel(o.name)
		if f == "" {
			f = "Column"
		}
		for seen[f] {
			f += "_"
		}
		seen[f] = true
		cols = append(cols, col{f, t, o.name})
	}
	fmt.Fprintf(b, "// %sColumns are the output columns of %s, in order. Every column may be NULL.\n", name, name)
	fmt.Fprintf(b, "type %sColumns struct {\n", name)
	for _, c := range cols {
		fmt.Fprintf(b, "\t%s qlx.Field[%s]\n", c.field, c.typ)
	}
	b.WriteString("}\n\n")
	fmt.Fprintf(b, "// Fields lists the columns for As.\nfunc (c %sColumns) Fields() []qlx.NameRef {\n\treturn []qlx.NameRef{", name)
	for i, c := range cols {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(b, "c.%s", c.field)
	}
	b.WriteString("}\n}\n\n")
	fmt.Fprintf(b, "// %sCols names the columns of %s for use with As and qlx.Ref.\nvar %sCols = %sColumns{\n", name, name, name, name)
	for _, c := range cols {
		fmt.Fprintf(b, "\t%s: qlx.NewField[%s](%q),\n", c.field, c.typ, c.pg)
	}
	b.WriteString("}\n\n")
}

// variadicTypeVar names the PgType of the element of a VARIADIC T[] parameter.
func variadicTypeVar(w *wrapper) string {
	return "pg" + camel(strings.TrimPrefix(w.fn.Args[len(w.fn.Args)-1], "_"))
}

// typeVar names the package-level PgType for argument i (see typesFile).
func typeVar(w *wrapper, i int) string {
	var name string
	switch {
	case w.fn != nil:
		name = w.fn.Args[i]
	case w.op.Left == "":
		name = w.op.Right
	case i == 0:
		name = w.op.Left
	default:
		name = w.op.Right
	}
	if strings.HasPrefix(name, "_") {
		return "pg" + camel(name[1:]) + "Arr"
	}
	return "pg" + camel(name)
}

// typesFile declares one PgType variable per mapped type, and one per array.
func typesFile(l *Ledger, pkg string, body string, extSchema string) ([]byte, error) {
	usedVars := map[string]bool{}
	for _, m := range pgVarRe.FindAllString(body, -1) {
		usedVars[m] = true
	}
	known := map[string]string{
		"bool": "PgBool", "int2": "PgInt2", "int4": "PgInt4", "int8": "PgInt8", "float4": "PgFloat4",
		"float8": "PgFloat8", "numeric": "PgNumeric", "text": "PgText", "bytea": "PgBytea",
		"date": "PgDate", "timestamp": "PgTimestamp", "timestamptz": "PgTimestamptz", "interval": "PgInterval",
		"json": "PgJSON", "jsonb": "PgJSONB", "inet": "PgInet",
	}
	var names []string
	for n := range scalar {
		names = append(names, n)
	}
	sort.Strings(names)
	imps := map[string]bool{"github.com/noobmaisfier/qlx": true}
	var keep []string
	for _, n := range names {
		if usedVars["pg"+camel(n)] || usedVars["pg"+camel(n)+"Arr"] {
			keep = append(keep, n)
		}
	}
	names = keep
	if len(names) == 0 {
		return nil, nil
	}
	for _, n := range names {
		g := scalar[n]
		if _, ok := known[n]; ok {
			continue
		}
		for _, im := range g.imports {
			imps[im] = true
		}
		if strings.Contains(g.expr, "qt.") {
			imps["github.com/noobmaisfier/qlx/qt"] = true
		}
	}
	var ipaths []string
	for p := range imps {
		ipaths = append(ipaths, p)
	}
	sort.Strings(ipaths)
	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by pgcat; DO NOT EDIT.\n\npackage %s\n\nimport (\n", pkg)
	for _, p := range ipaths {
		fmt.Fprintf(&b, "\t%q\n", p)
	}
	b.WriteString(")\n\n")
	b.WriteString("// The type each wrapper pins its placeholders to, so overloaded built-ins resolve\n// to the overload named by the Go signature.\nvar (\n")
	for _, n := range names {
		g := scalar[n]
		if sym, ok := known[n]; ok {
			fmt.Fprintf(&b, "\tpg%s = qlx.%s\n\tpg%sArr = qlx.ArrayOf(pg%s)\n", camel(n), sym, camel(n), camel(n))
		} else {
			schema := "pg_catalog"
			if extTypeNames[n] && extSchema != "" {
				schema = extSchema
			}
			fmt.Fprintf(&b, "\tpg%s = qlx.NewPgType[%s](%q, %q)\n\tpg%sArr = qlx.ArrayOf(pg%s)\n", camel(n), g.expr, schema, n, camel(n), camel(n))
		}
	}
	b.WriteString(")\n")
	return format.Source(b.Bytes())
}

func joinArgs(a []string) string {
	if len(a) == 0 {
		return ""
	}
	return ", " + strings.Join(a, ", ")
}

var goKeyword = map[string]bool{"break": true, "default": true, "func": true, "interface": true, "select": true,
	"case": true, "defer": true, "go": true, "map": true, "struct": true, "chan": true, "else": true, "goto": true,
	"package": true, "switch": true, "const": true, "fallthrough": true, "if": true, "range": true, "type": true,
	"continue": true, "for": true, "import": true, "return": true, "var": true,
	"qlx": true, "qt": true, "time": true, "json": true, "netip": true}

func isIdent(s string) bool {
	for i, r := range s {
		if !(r == '_' || unicode.IsLetter(r) || (i > 0 && unicode.IsDigit(r))) {
			return false
		}
	}
	return s != ""
}

func paramName(s string, i int) string {
	if goKeyword[s] {
		return s + "_"
	}
	return s
}

// prepareTest emits a test that asks a real server to PREPARE every wrapper,
// built with placeholders so the pinned parameter types are what is checked.
// It proves each wrapper names an existing overload with the right types.
func prepareTest(pkg string, ws []*wrapper, extension string) ([]byte, error) {
	sort.Slice(ws, func(i, j int) bool { return ws[i].name < ws[j].name })
	imports := map[string]bool{"github.com/noobmaisfier/qlx": true}
	var body bytes.Buffer
	call := func(w *wrapper, nullable bool) {
		name := w.name
		if nullable {
			name += "N"
		}
		since, until := w.versions()
		var args []string
		inst := ""
		if len(w.tparam) > 0 {
			ts := make([]string, len(w.tparam))
			for i := range ts {
				ts[i] = "int32"
			}
			inst = "[" + strings.Join(ts, ", ") + "]"
		}
		for _, p := range w.params {
			t := tparamRe.ReplaceAllString(p.expr, "int32")
			if nullable {
				t = "qt.Null[" + t + "]"
			}
			if strings.Contains(t, "qt.") {
				imports["github.com/noobmaisfier/qlx/qt"] = true
			}
			for _, im := range p.imports {
				imports[im] = true
			}
			args = append(args, "qlx.NewParam["+t+"]()")
		}
		switch w.variadic {
		case "value":
			args = append(args, "qlx.ArgP(qlx.NewParam[int32]())", "qlx.ArgP(qlx.NewParam[string]())")
		case "operand":
			et := tparamRe.ReplaceAllString(w.varType.expr, "int32")
			if strings.Contains(et, "qt.") {
				imports["github.com/noobmaisfier/qlx/qt"] = true
			}
			for _, im := range w.varType.imports {
				imports[im] = true
			}
			args = append(args, "qlx.NewParam["+et+"]()", "qlx.NewParam["+et+"]()")
		}
		if w.srf {
			srfCols := `qlx.NewField[int32]("c")`
			if len(w.outs) > 0 {
				srfCols = name + "Cols.Fields()..."
			}
			fmt.Fprintf(&body, "\t{%q, %d, %d, func() qlx.Statement {\n\t\treturn qlx.Select(qlx.CountAll()).From(%s%s(%s).As(\"t\", %s))\n\t}},\n", name, since, until, name, inst, strings.Join(args, ", "), srfCols)
			return
		}
		fmt.Fprintf(&body, "\t{%q, %d, %d, func() qlx.Statement { return qlx.Select(%s%s(%s)) }},\n", name, since, until, name, inst, strings.Join(args, ", "))
	}
	for _, w := range ws {
		if contains(w.tparam, "N") {
			continue // anyenum needs a user-defined enum type; nothing built in to PREPARE it with
		}
		call(w, false)
		if w.nulls {
			call(w, true)
		}
	}
	var out bytes.Buffer
	fmt.Fprintf(&out, "// Code generated by pgcat; DO NOT EDIT.\n\npackage %s\n\nimport (\n\t\"os\"\n\t\"os/exec\"\n\t\"strconv\"\n\t\"strings\"\n\t\"testing\"\n", pkg)
	var paths []string
	for p := range imports {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		fmt.Fprintf(&out, "\t%q\n", p)
	}
	out.WriteString(")\n\n")
	out.WriteString(`type wrapperCase struct {
	name  string
	since int // first major with the function (0: all)
	until int // first major without it (0: still there)
	build func() qlx.Statement
}

var wrapperCases = []wrapperCase{
`)
	out.Write(body.Bytes())
	fmt.Fprintf(&out, "}\n\n")
	out.WriteString(strings.Replace(testBody, "__EXT__", extension, 1))
	src, err := format.Source(out.Bytes())
	if err != nil {
		return out.Bytes(), fmt.Errorf("generated pgcat test does not parse: %w", err)
	}
	return src, nil
}

const testBody = `// TestServerAcceptsEveryWrapper PREPAREs each wrapper on a real server. It is
// skipped unless QLX_PG_HOST is set (scripts/with_pg.sh sets it).
func TestServerAcceptsEveryWrapper(t *testing.T) {
	host := os.Getenv("QLX_PG_HOST")
	if host == "" {
		t.Skip("QLX_PG_HOST not set; run scripts/with_pg.sh go test ./...")
	}
	version, _ := strconv.Atoi(os.Getenv("QLX_PG_VERSION"))
	env := append(os.Environ(), "PGHOST="+host, "PGDATABASE="+os.Getenv("QLX_PG_DB"))
	if u := os.Getenv("QLX_PG_USER"); u != "" {
		env = append(env, "PGUSER="+u)
	}
	if ext := "__EXT__"; ext != "" {
		create := exec.Command("psql", "-X", "-q", "-v", "ON_ERROR_STOP=1", "-c", "CREATE EXTENSION IF NOT EXISTS "+ext)
		create.Env = env
		if out, err := create.CombinedOutput(); err != nil {
			t.Skipf("extension %s is not available on this server: %s", ext, strings.TrimSpace(string(out)))
		}
	}
	var script strings.Builder
	var names []string
	for _, c := range wrapperCases {
		if c.since > version || (c.until > 0 && version >= c.until) {
			continue
		}
		st, err := qlx.Render(qlx.Dialect{Version: version}, c.build())
		sql := st.SQL
		if err != nil {
			t.Errorf("%s does not render: %v", c.name, err)
			continue
		}
		script.WriteString("\\echo @@" + c.name + "\n")
		// DEALLOCATE ALL cannot fail, so a rejected PREPARE yields exactly one
		// ERROR, attributed to the right wrapper.
		script.WriteString("PREPARE s AS " + sql + ";\nDEALLOCATE ALL;\n")
		names = append(names, c.name)
	}
	cmd := exec.Command("psql", "-X", "-q", "-v", "ON_ERROR_STOP=0")
	cmd.Env = env
	cmd.Stdin = strings.NewReader(script.String())
	out, _ := cmd.CombinedOutput()
	// With ON_ERROR_STOP off, every failure prints "ERROR:" after the \echo of
	// its case; collect them.
	var current string
	failed := 0
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.Contains(line, "ERROR:"):
			failed++
			if failed <= 40 {
				t.Errorf("%s: %s", current, strings.TrimSpace(line))
			}
		case strings.HasPrefix(line, "@@"):
			current = strings.TrimPrefix(line, "@@")
		}
	}
	if failed > 40 {
		t.Errorf("... and %d more", failed-40)
	}
	t.Logf("%d wrappers checked against PostgreSQL %d", len(names), version)
}
`

// versions is the version range a wrapper's function or operator lives in.
func (w *wrapper) versions() (since, until int) {
	if w.fn != nil {
		return w.fn.Since, w.fn.Until
	}
	return w.op.Since, w.op.Until
}
