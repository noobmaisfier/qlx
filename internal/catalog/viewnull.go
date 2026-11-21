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
 'views', COALESCE((SELECT json_agg(json_build_object('oid', c.oid::int, 'schema', n.nspname, 'name', c.relname, 'tree', r.ev_action::text))
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    JOIN pg_rewrite r ON r.ev_class = c.oid AND r.rulename = '_RETURN'
    WHERE c.relkind IN ('v', 'm') AND n.nspname NOT IN ('pg_catalog', 'information_schema') AND n.nspname NOT LIKE 'pg\_toast%'), '[]'::json),
 'attrs', COALESCE((SELECT json_agg(json_build_array(a.attrelid::int, a.attnum))
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
}

type viewFacts struct {
	Views   []viewDef  `json:"views"`
	Attrs   [][2]int64 `json:"attrs"`
	Domains []uint32   `json:"domains"`
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
	byName := map[string]uint32{}
	for _, v := range f.Views {
		byName[v.Schema+"."+v.Name] = v.OID
	}
	for i := range s.Tables {
		t := &s.Tables[i]
		if t.Kind != "v" && t.Kind != "m" {
			continue
		}
		cols := proven[byName[t.Schema+"."+t.Name]]
		for j := range t.Columns {
			col := &t.Columns[j]
			if name, ok := cols[col.Num]; ok && name == col.Name {
				col.NotNull = true
			}
		}
	}
}

// inferViewNotNull returns, per view oid, the columns (attnum -> name) its
// stored query proves NOT NULL.
func inferViewNotNull(f viewFacts) map[uint32]map[int]string {
	in := &inferer{
		views:   map[uint32]viewDef{},
		attrs:   map[[2]int64]bool{},
		domains: map[uint32]bool{},
		memo:    map[uint32]map[int]string{},
		busy:    map[uint32]bool{},
	}
	for _, v := range f.Views {
		in.views[v.OID] = v
	}
	for _, a := range f.Attrs {
		in.attrs[a] = true
	}
	for _, d := range f.Domains {
		in.domains[d] = true
	}
	out := map[uint32]map[int]string{}
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
	memo    map[uint32]map[int]string
	busy    map[uint32]bool
}

func (in *inferer) analyze(oid uint32) map[int]string {
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

func (in *inferer) analyzeTree(tree string) map[int]string {
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
	out := map[int]string{}
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
		if in.nonNull(t.f["expr"], rtable, 0) {
			out[resno] = atom(t.f["resname"])
		}
	}
	return out
}

// Counting aggregates never return NULL: count(*) and count("any").
var countOIDs = map[string]bool{"2803": true, "2147": true}

func (in *inferer) nonNull(e any, rtable []any, depth int) bool {
	n, ok := e.(*treeNode)
	if !ok || depth > 32 {
		return false
	}
	switch n.name {
	case "VAR":
		return in.nonNullVar(n, rtable, true, depth)
	case "CONST":
		return atom(n.f["constisnull"]) == "false"
	case "COALESCEEXPR":
		args, _ := n.f["args"].([]any)
		for _, a := range args {
			if in.nonNull(a, rtable, depth+1) {
				return true
			}
		}
		return false
	case "COERCETODOMAIN":
		// A cast to a NOT NULL domain makes PostgreSQL raise an error on NULL.
		if id, err := strconv.ParseUint(atom(n.f["resulttype"]), 10, 32); err == nil && in.domains[uint32(id)] {
			return true
		}
		return in.nonNull(n.f["arg"], rtable, depth+1)
	case "RELABELTYPE":
		return in.nonNull(n.f["arg"], rtable, depth+1)
	case "AGGREF":
		return countOIDs[atom(n.f["aggfnoid"])]
	case "WINDOWFUNC":
		return countOIDs[atom(n.f["winfnoid"])]
	}
	return false
}

// emptyNullingRels reports whether a Var is not nulled by an outer join:
// PostgreSQL stores varnullingrels as the bitmapset (b) when it is empty.
func emptyNullingRels(v any) bool {
	l, ok := v.([]any)
	return ok && len(l) == 1 && l[0] == "b"
}

func (in *inferer) nonNullVar(v *treeNode, rtable []any, top bool, depth int) bool {
	if depth > 32 || atom(v.f["varlevelsup"]) != "0" || !emptyNullingRels(v.f["varnullingrels"]) {
		return false
	}
	varno, err1 := strconv.Atoi(atom(v.f["varno"]))
	attno, err2 := strconv.Atoi(atom(v.f["varattno"]))
	if err1 != nil || err2 != nil || varno < 1 || varno > len(rtable) || attno < 1 {
		return false
	}
	rte, ok := rtable[varno-1].(*treeNode)
	if !ok || rte.name != "RANGETBLENTRY" {
		return false
	}
	switch atom(rte.f["rtekind"]) {
	case "0": // a relation: table, view, materialized view, foreign table
		id, err := strconv.ParseUint(atom(rte.f["relid"]), 10, 32)
		if err != nil {
			return false
		}
		if in.attrs[[2]int64{int64(id), int64(attno)}] {
			return true
		}
		if _, isView := in.views[uint32(id)]; isView {
			_, proven := in.analyze(uint32(id))[attno]
			return proven
		}
		return false
	case "2": // a join: its output column is an alias for a Var of a base relation
		aliases, _ := rte.f["joinaliasvars"].([]any)
		if attno > len(aliases) {
			return false
		}
		inner, ok := aliases[attno-1].(*treeNode)
		return ok && inner.name == "VAR" && in.nonNullVar(inner, rtable, false, depth+1)
	case "9": // RTE_GROUP (PostgreSQL 17+): a grouped column points into groupexprs
		exprs, _ := rte.f["groupexprs"].([]any)
		if attno < 1 || attno > len(exprs) {
			return false
		}
		return in.nonNull(exprs[attno-1], rtable, depth+1)
	}
	return false
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
