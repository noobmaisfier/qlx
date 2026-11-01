// Package packs holds the curated Go mappings for extension types: the one
// hand-written part of extension support. Everything else (the functions and
// operators an extension adds) is generated from the server's own catalog by
// `pgcat extract -ext NAME` and `pgcat gen`, so a pack never lists an API.
package packs

import "sort"

// Type maps one extension type to a Go type.
type Type struct {
	Name    string   // pg_type.typname
	Expr    string   // Go type expression, "qt.Vector"
	Imports []string // import paths the expression needs
	NullOK  bool     // may be wrapped in qt.Null; false for slices and maps, where nil is NULL
	Family  string   // column family: "" (generic Col), "text", "num"
}

// Pack describes one extension.
type Pack struct {
	Name      string // the pack's name in qlx.yaml
	Extension string // the name in pg_extension
	Types     []Type
	// Verified says whether the pack was exercised against a real server in
	// this repository's tests (all shipped packs are: the contrib extensions,
	// pgvector and PostGIS).
	Verified bool
	Notes    string
}

var all = []Pack{
	{Name: "pgvector", Extension: "vector", Verified: true, Types: []Type{
		{Name: "vector", Expr: "qt.Vector"},
		{Name: "halfvec", Expr: "qt.HalfVector"},
		{Name: "sparsevec", Expr: "qt.SparseVector", NullOK: true},
	}, Notes: "vectors travel in text form; nil is NULL; a literal operand must be cast to the extension type before it reaches an overloaded function"},
	{Name: "postgis", Extension: "postgis", Verified: true, Types: []Type{
		{Name: "geometry", Expr: "qt.Geometry", NullOK: true},
		{Name: "geography", Expr: "qt.Geography", NullOK: true},
	}, Notes: "PostGIS 3.7.0; values are extended well-known binary in hex; decode them with the geometry library of your choice"},
	{Name: "pg_trgm", Extension: "pg_trgm", Notes: "functions and operators only (similarity, %, <->)"},
	{Name: "hstore", Extension: "hstore", Verified: true, Types: []Type{
		{Name: "hstore", Expr: "qt.Hstore"},
	}, Notes: "nil is NULL"},
	{Name: "ltree", Extension: "ltree", Verified: true, Types: []Type{
		{Name: "ltree", Expr: "qt.LTree", NullOK: true},
		{Name: "lquery", Expr: "qt.LQuery", NullOK: true},
		{Name: "ltxtquery", Expr: "qt.LTxtQuery", NullOK: true},
	}},
	{Name: "citext", Extension: "citext", Verified: true, Types: []Type{
		{Name: "citext", Expr: "string", NullOK: true, Family: "text"},
	}},
}

// Lookup finds a pack by its qlx.yaml name.
func Lookup(name string) (Pack, bool) {
	for _, p := range all {
		if p.Name == name {
			return p, true
		}
	}
	return Pack{}, false
}

// Names lists the pack names, sorted.
func Names() []string {
	var out []string
	for _, p := range all {
		out = append(out, p.Name)
	}
	sort.Strings(out)
	return out
}
