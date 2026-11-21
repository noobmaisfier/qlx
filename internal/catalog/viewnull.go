package catalog

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/noobmaisfier/qlx/internal/pgtool"
)

// PostgreSQL never marks a view column NOT NULL (pg_attribute.attnotnull is
// false for every view column), so a view built on NOT NULL columns would be
// typed as nullable everywhere. This file recovers what the server knows but
// does not record: it reads the view's stored query tree (pg_rewrite), and a
// column is NOT NULL only when the tree proves it:
//
//   - a plain reference to a NOT NULL column of a table, not on the nullable
//     side of an outer join (the Var's varnullingrels is empty);
//   - a reference to a column of another view or materialized view that is
//     itself proven NOT NULL;
//   - a non-NULL constant, COALESCE with a proven argument, count(*) and
//     count(x) (aggregate or window), a binary-compatible relabel of a proven
//     expression;
//   - a cast to a domain declared NOT NULL: PostgreSQL itself raises an error
//     when the value is NULL, so the guarantee is enforced, not assumed.
//
// Everything else stays nullable: UNION and other set operations, grouping
// sets, sub-selects, CTEs, functions, CASE, arithmetic. When anything in the
// tree is not understood the whole view stays nullable, so a surprise from a
// newer PostgreSQL can only cost precision, never claim a column is NOT NULL
// that is not.

const viewNullSQL = `
SELECT json_build_object(
 'views', COALESCE((SELECT json_agg(json_build_object('oid', c.oid::int, 'schema', n.nspname, 'name', c.relname, 'tree', r.ev_action::text,
      'updatable', CASE WHEN c.relkind = 'v' THEN pg_relation_is_updatable(c.oid, true) ELSE 0 END,
      'writable', CASE WHEN c.relkind = 'v' THEN (SELECT COALESCE(json_agg(a.attnum), '[]'::json) FROM pg_attribute a
                    WHERE a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped AND pg_column_is_updatable(c.oid, a.attnum, true)) ELSE '[]'::json END))
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    JOIN pg_rewrite r ON r.ev_class = c.oid AND r.rulename = '_RETURN'
    WHERE c.relkind IN ('v', 'm') AND n.nspname NOT IN ('pg_catalog', 'information_schema') AND n.nspname NOT LIKE 'pg\_toast%'), '[]'::json),
 'attrs', COALESCE((SELECT json_agg(json_build_array(a.attrelid::int, a.attnum, a.atthasdef, a.attidentity::text, a.attgenerated::text))
    FROM pg_attribute a
    JOIN pg_class c ON c.oid = a.attrelid
    JOIN pg_namespace n ON n.oid = c.relnamespace
    LEFT JOIN pg_constraint nn ON nn.conrelid = a.attrelid AND nn.contype = 'n' AND nn.conkey = ARRAY[a.attnum]
    WHERE a.attnotnull AND COALESCE(nn.convalidated, true) AND a.attnum > 0 AND NOT a.attisdropped
      AND c.relkind IN ('r', 'p', 'f') AND n.nspname NOT IN ('pg_catalog', 'information_schema') AND n.nspname NOT LIKE 'pg\_toast%'), '[]'::json),
 'domains', COALESCE((SELECT json_agg(t.oid::int) FROM pg_type t WHERE t.typtype = 'd' AND t.typnotnull), '[]'::json)
)::text;
`

type viewDef struct {
	OID    uint32 `json:"oid"`
	Schema string `json:"schema"`
	Name   string `json:"name"`
	Tree   string `json:"tree"`
	// Updatable is pg_relation_is_updatable(view, true): 4 UPDATE, 8 INSERT,
	// 16 DELETE, counting INSTEAD OF triggers; Writable lists the attnums for
	// which pg_column_is_updatable(view, attnum, true) holds.
	Updatable int   `json:"updatable"`
	Writable  []int `json:"writable"`
}

// attrFact is a NOT NULL column of a table: its relation oid and attnum, and
// what an INSERT through a view needs to know about it.
type attrFact struct {
	Rel      int64
	Num      int
	HasDef   bool
	Identity string
	Gen      string
}

func (a *attrFact) UnmarshalJSON(b []byte) error {
	var v []any
	if err := json.Unmarshal(b, &v); err != nil || len(v) != 5 {
		return errors.New("catalog: bad attribute fact")
	}
	rel, ok1 := v[0].(float64)
	num, ok2 := v[1].(float64)
	def, ok3 := v[2].(bool)
	ident, ok4 := v[3].(string)
	gen, ok5 := v[4].(string)
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
		return errors.New("catalog: bad attribute fact")
	}
	*a = attrFact{int64(rel), int(num), def, ident, gen}
	return nil
}

type viewFacts struct {
	Views   []viewDef  `json:"views"`
	Attrs   []attrFact `json:"attrs"`
	Domains []uint32   `json:"domains"`
}

// viewCol is a view column the stored query proves NOT NULL: its name, and,
// when it is a plain reference to a column of a table, that column.
type viewCol struct {
	Name string
	Src  [2]int64 // relation oid and attnum of the base column; zero when not a plain reference
}

// applyViewNullability marks the columns of s's views that the server's query
// trees prove NOT NULL. A failure leaves every view column nullable.
func applyViewNullability(c pgtool.Conn, s *Schema) {
	hasView := false
	for _, t := range s.Tables {
		if t.Kind == "v" || t.Kind == "m" {
			hasView = true
			break
		}
	}
	if !hasView {
		return
	}
	out, err := c.Query(viewNullSQL)
	if err != nil {
		return
	}
	var f viewFacts
	if json.Unmarshal(out, &f) != nil {
		return
	}
	proven := inferViewNotNull(f)
	byName := map[string]viewDef{}
	for _, v := range f.Views {
		byName[v.Schema+"."+v.Name] = v
	}
	base := map[[2]int64]attrFact{}
	for _, a := range f.Attrs {
		base[[2]int64{a.Rel, int64(a.Num)}] = a
	}
	for i := range s.Tables {
		t := &s.Tables[i]
		if t.Kind != "v" && t.Kind != "m" {
			continue
		}
		v := byName[t.Schema+"."+t.Name]
		cols := proven[v.OID]
		writable := map[int]bool{}
		for _, n := range v.Writable {
			writable[n] = true
		}
		t.Updatable = v.Updatable
		for j := range t.Columns {
			col := &t.Columns[j]
			col.Updatable = writable[col.Num]
			vc, ok := cols[col.Num]
			if !ok || vc.Name != col.Name {
				continue
			}
			col.NotNull = true
			// An INSERT through a simple view takes the view's own default, else
			// the base column's: copy what the base says, so a required column
			// is not asked for when the table fills it.
			if b, ok := base[vc.Src]; ok {
				col.HasDefault = col.HasDefault || b.HasDef
				if col.Identity == "" {
					col.Identity = b.Identity
				}
				if col.Generated == "" {
					col.Generated = b.Gen
				}
			}
		}
	}
}

// inferViewNotNull returns, per view oid, the columns (attnum -> name) its
// stored query proves NOT NULL.
func inferViewNotNull(f viewFacts) map[uint32]map[int]viewCol {
	in := &inferer{
		views:   map[uint32]viewDef{},
		attrs:   map[[2]int64]bool{},
		domains: map[uint32]bool{},
		memo:    map[uint32]map[int]viewCol{},
		busy:    map[uint32]bool{},
	}
	for _, v := range f.Views {
		in.views[v.OID] = v
	}
	for _, a := range f.Attrs {
		in.attrs[[2]int64{a.Rel, int64(a.Num)}] = true
	}
	for _, d := range f.Domains {
		in.domains[d] = true
	}
	out := map[uint32]map[int]viewCol{}
	for _, v := range f.Views {
		if cols := in.analyze(v.OID); len(cols) > 0 {
			out[v.OID] = cols
		}
	}
	return out
}

type inferer struct {
	views   map[uint32]viewDef
	attrs   map[[2]int64]bool
	domains map[uint32]bool
	memo    map[uint32]map[int]viewCol
	busy    map[uint32]bool
}

func (in *inferer) analyze(oid uint32) map[int]viewCol {
	if m, ok := in.memo[oid]; ok {
		return m
	}
	if in.busy[oid] {
		return nil
	}
	in.busy[oid] = true
	defer delete(in.busy, oid)
	v, ok := in.views[oid]
	if !ok {
		return nil
	}
	cols := in.analyzeTree(v.Tree)
	in.memo[oid] = cols
	return cols
}

func (in *inferer) analyzeTree(tree string) map[int]viewCol {
	root, err := parseNodeTree(tree)
	if err != nil {
		return nil
	}
	list, ok := root.([]any)
	if !ok || len(list) != 1 {
		return nil
	}
	q, ok := list[0].(*treeNode)
	if !ok || q.name != "QUERY" || atom(q.f["commandType"]) != "1" {
		return nil
	}
	if q.f["setOperations"] != nil || q.f["groupingSets"] != nil || atom(q.f["hasTargetSRFs"]) == "true" {
		return nil
	}
	rtable, _ := q.f["rtable"].([]any)
	out := map[int]viewCol{}
	targets, _ := q.f["targetList"].([]any)
	for _, te := range targets {
		t, ok := te.(*treeNode)
		if !ok || t.name != "TARGETENTRY" || atom(t.f["resjunk"]) != "false" {
			return nil
		}
		resno, err := strconv.Atoi(atom(t.f["resno"]))
		if err != nil {
			return nil
		}
		if ok, src := in.nonNull(t.f["expr"], rtable, 0); ok {
			out[resno] = viewCol{Name: atom(t.f["resname"]), Src: src}
		}
	}
	return out
}

// Counting aggregates never return NULL: count(*) and count("any").
var countOIDs = map[string]bool{"2803": true, "2147": true}

// nonNull reports whether the expression is proven never NULL and, when it is
// a plain reference to a column of a table, which column.
func (in *inferer) nonNull(e any, rtable []any, depth int) (bool, [2]int64) {
	n, ok := e.(*treeNode)
	if !ok || depth > 32 {
		return false, [2]int64{}
	}
	switch n.name {
	case "VAR":
		return in.nonNullVar(n, rtable, true, depth)
	case "CONST":
		return atom(n.f["constisnull"]) == "false", [2]int64{}
	case "COALESCEEXPR":
		args, _ := n.f["args"].([]any)
		for _, a := range args {
			if ok, _ := in.nonNull(a, rtable, depth+1); ok {
				return true, [2]int64{}
			}
		}
		return false, [2]int64{}
	case "COERCETODOMAIN":
		// A cast to a NOT NULL domain makes PostgreSQL raise an error on NULL.
		if id, err := strconv.ParseUint(atom(n.f["resulttype"]), 10, 32); err == nil && in.domains[uint32(id)] {
			return true, [2]int64{}
		}
		ok, _ := in.nonNull(n.f["arg"], rtable, depth+1)
		return ok, [2]int64{}
	case "RELABELTYPE":
		return in.nonNull(n.f["arg"], rtable, depth+1)
	case "AGGREF":
		return countOIDs[atom(n.f["aggfnoid"])], [2]int64{}
	case "WINDOWFUNC":
		return countOIDs[atom(n.f["winfnoid"])], [2]int64{}
	}
	return false, [2]int64{}
}

// emptyNullingRels reports whether a Var is not nulled by an outer join:
// PostgreSQL stores varnullingrels as the bitmapset (b) when it is empty.
func emptyNullingRels(v any) bool {
	l, ok := v.([]any)
	return ok && len(l) == 1 && l[0] == "b"
}

func (in *inferer) nonNullVar(v *treeNode, rtable []any, top bool, depth int) (bool, [2]int64) {
	no := [2]int64{}
	if depth > 32 || atom(v.f["varlevelsup"]) != "0" || !emptyNullingRels(v.f["varnullingrels"]) {
		return false, no
	}
	varno, err1 := strconv.Atoi(atom(v.f["varno"]))
	attno, err2 := strconv.Atoi(atom(v.f["varattno"]))
	if err1 != nil || err2 != nil || varno < 1 || varno > len(rtable) || attno < 1 {
		return false, no
	}
	rte, ok := rtable[varno-1].(*treeNode)
	if !ok || rte.name != "RANGETBLENTRY" {
		return false, no
	}
	switch atom(rte.f["rtekind"]) {
	case "0": // a relation: table, view, materialized view, foreign table
		id, err := strconv.ParseUint(atom(rte.f["relid"]), 10, 32)
		if err != nil {
			return false, no
		}
		key := [2]int64{int64(id), int64(attno)}
		if in.attrs[key] {
			return true, key
		}
		if _, isView := in.views[uint32(id)]; isView {
			vc, proven := in.analyze(uint32(id))[attno]
			return proven, vc.Src
		}
		return false, no
	case "2": // a join: its output column is an alias for a Var of a base relation
		aliases, _ := rte.f["joinaliasvars"].([]any)
		if attno > len(aliases) {
			return false, no
		}
		inner, ok := aliases[attno-1].(*treeNode)
		if !ok || inner.name != "VAR" {
			return false, no
		}
		return in.nonNullVar(inner, rtable, false, depth+1)
	case "9": // RTE_GROUP (PostgreSQL 17+): a grouped column points into groupexprs
		exprs, _ := rte.f["groupexprs"].([]any)
		if attno < 1 || attno > len(exprs) {
			return false, no
		}
		return in.nonNull(exprs[attno-1], rtable, depth+1)
	}
	return false, no
}

// ---- parser for PostgreSQL's nodeToString text -----------------------------

// treeNode is {NAME :field value ...}. A value is a *treeNode, a []any (a
// parenthesised list), a string (an atom) or nil (<>).
type treeNode struct {
	name string
	f    map[string]any
}

func atom(v any) string {
	s, _ := v.(string)
	return s
}

var errTree = errors.New("catalog: unreadable query tree")

type treeParser struct {
	s     string
	i     int
	depth int
}

func parseNodeTree(s string) (any, error) {
	p := &treeParser{s: s}
	v, err := p.value()
	if err != nil {
		return nil, err
	}
	p.skip()
	if p.i != len(p.s) {
		return nil, errTree
	}
	return v, nil
}

func (p *treeParser) skip() {
	for p.i < len(p.s) && (p.s[p.i] == ' ' || p.s[p.i] == '\n' || p.s[p.i] == '\t' || p.s[p.i] == '\r') {
		p.i++
	}
}

func (p *treeParser) value() (any, error) {
	p.skip()
	if p.i >= len(p.s) {
		return nil, errTree
	}
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > 256 {
		return nil, errTree
	}
	switch p.s[p.i] {
	case '{':
		return p.node()
	case '(':
		p.i++
		list := []any{}
		for {
			p.skip()
			if p.i >= len(p.s) {
				return nil, errTree
			}
			if p.s[p.i] == ')' {
				p.i++
				return list, nil
			}
			v, err := p.value()
			if err != nil {
				return nil, err
			}
			list = append(list, v)
		}
	case '}', ')':
		return nil, errTree
	}
	a := p.atom()
	if a == "<>" {
		return nil, nil
	}
	return a, nil
}

// atom reads a bare token: it ends at a space or a bracket; a backslash
// escapes the next character (nodeToString escapes spaces and brackets).
func (p *treeParser) atom() string {
	var b strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == '\\' && p.i+1 < len(p.s) {
			b.WriteByte(p.s[p.i+1])
			p.i += 2
			continue
		}
		if c == ' ' || c == '\n' || c == '\t' || c == '\r' || c == '{' || c == '}' || c == '(' || c == ')' {
			break
		}
		b.WriteByte(c)
		p.i++
	}
	return b.String()
}

func (p *treeParser) node() (any, error) {
	p.i++ // {
	p.skip()
	n := &treeNode{name: p.atom(), f: map[string]any{}}
	if n.name == "" {
		return nil, errTree
	}
	for {
		p.skip()
		if p.i >= len(p.s) {
			return nil, errTree
		}
		if p.s[p.i] == '}' {
			p.i++
			return n, nil
		}
		field := p.atom()
		if !strings.HasPrefix(field, ":") || len(field) < 2 {
			return nil, errTree
		}
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		n.f[field[1:]] = v
		// Some fields hold several bare tokens (a datum: "4 [ 1 0 0 0 ]"): the
		// first is the value, the rest are skipped up to the next field.
		for {
			p.skip()
			if p.i >= len(p.s) || p.s[p.i] == ':' || p.s[p.i] == '}' {
				break
			}
			if _, err := p.value(); err != nil {
				return nil, err
			}
		}
	}
}
