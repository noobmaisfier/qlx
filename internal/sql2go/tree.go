// Package sql2go turns the parse tree of a SQL statement into the equivalent
// QLX code. The tree comes from libpg_query (the PostgreSQL parser itself,
// via pg_query_go), so the grammar is never approximated here: whatever the
// server accepts parses, and what QLX has no typed form for is reported as
// an error that names the construct and its position instead of being guessed.
package sql2go

import (
	"encoding/json"
	"fmt"
	"strings"
)

// node is one element of the JSON parse tree: {"Kind": {fields...}}.
type node = map[string]any

// Unsupported is returned for SQL that is valid but has no emission rule.
type Unsupported struct {
	What string
	Hint string
	Line int // 1-based, 0 when unknown
	Col  int
}

func (e *Unsupported) Error() string {
	var b strings.Builder
	if e.Line > 0 {
		fmt.Fprintf(&b, "%d:%d: ", e.Line, e.Col)
	}
	b.WriteString("unsupported: " + e.What)
	if e.Hint != "" {
		b.WriteString(" (" + e.Hint + ")")
	}
	return b.String()
}

// kindOf splits {"Kind": {...}} into its parts. libpg_query writes a Node as a
// one-key object whose key is the node's CamelCase kind, but writes embedded
// structs (the relation of an INSERT, an alias, an ON CONFLICT clause) as flat
// objects of lower-case fields. Only the first shape is a wrapper; a flat
// object yields "", nil so callers can tell the two apart and read its fields
// directly. A nil or empty node also yields "".
func kindOf(n any) (string, map[string]any) {
	m, ok := n.(map[string]any)
	if !ok || len(m) != 1 {
		return "", nil
	}
	for k, v := range m {
		if k == "" || k[0] < 'A' || k[0] > 'Z' {
			return "", nil
		}
		f, _ := v.(map[string]any)
		return k, f
	}
	return "", nil
}

// embedded reads a field that may be either a wrapped Node of the given kind
// or a flat struct (what libpg_query emits for RangeVar fields such as
// "relation"). It returns the struct's fields.
func embedded(v any, kind string) (map[string]any, bool) {
	if k, f := kindOf(v); k == kind && f != nil {
		return f, true
	}
	if m, ok := v.(map[string]any); ok && len(m) > 0 {
		if k, _ := kindOf(m); k == "" {
			return m, true
		}
	}
	return nil, false
}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

func list(m map[string]any, k string) []any {
	l, _ := m[k].([]any)
	return l
}

// items unwraps a List node into its elements.
func items(n any) []any {
	k, f := kindOf(n)
	if k != "List" {
		return nil
	}
	return list(f, "items")
}

// names reads a qualified-name list: [{"String":{"sval":"a"}}, ...].
func names(l []any) []string {
	var out []string
	for _, n := range l {
		if k, f := kindOf(n); k == "String" {
			out = append(out, str(f, "sval"))
		} else if k == "A_Star" {
			out = append(out, "*")
		}
	}
	return out
}

func parseTree(js string) ([]node, error) {
	var root struct {
		Stmts []struct {
			Stmt node `json:"stmt"`
		} `json:"stmts"`
	}
	if err := json.Unmarshal([]byte(js), &root); err != nil {
		return nil, fmt.Errorf("parse tree: %w", err)
	}
	var out []node
	for _, s := range root.Stmts {
		out = append(out, s.Stmt)
	}
	return out, nil
}
