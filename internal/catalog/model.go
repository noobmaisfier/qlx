// Package catalog reads the PostgreSQL system catalogs into a schema model
// and serializes it as schema.lock. The server is the only source of truth:
// there is no DDL parser here, so nothing can fall behind the grammar.
package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// Column is one table column.
type Column struct {
	Num        int    `json:"num"`
	Name       string `json:"name"`
	TypeOID    uint32 `json:"type_oid"`
	TypeName   string `json:"type_name"`
	NotNull    bool   `json:"not_null"` // NOT VALID not-null constraints count as nullable
	HasDefault bool   `json:"has_default"`
	Identity   string `json:"identity"`  // "", "a" (ALWAYS), "d" (BY DEFAULT)
	Generated  string `json:"generated"` // "", "s" (stored), "v" (virtual, PostgreSQL 18)
}

// Constraint is a primary key, unique, foreign key or exclusion constraint.
type Constraint struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"` // p, u, f, x
	Columns    []string `json:"columns"`
	RefSchema  string   `json:"ref_schema,omitempty"`
	RefTable   string   `json:"ref_table,omitempty"`
	RefColumns []string `json:"ref_columns,omitempty"`
}

// Table is a table, partitioned table, view, materialized view or foreign table.
type Table struct {
	Schema      string       `json:"schema"`
	Name        string       `json:"name"`
	Kind        string       `json:"kind"` // r, p, v, m, f
	Columns     []Column     `json:"columns"`
	PrimaryKey  []string     `json:"primary_key"`
	Constraints []Constraint `json:"constraints"`
	Fingerprint string       `json:"fingerprint"`
}

// ReadOnly reports whether the relation cannot be written to.
func (t Table) ReadOnly() bool { return t.Kind == "v" || t.Kind == "m" }

// Type is a pg_type row reachable from the included tables and functions.
type Type struct {
	OID          uint32 `json:"oid"`
	Schema       string `json:"schema"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`     // typtype: b base, d domain, e enum, r range, m multirange, c composite
	Category     string `json:"category"` // typcategory: A array, ...
	Elem         uint32 `json:"elem"`
	Base         uint32 `json:"base"`          // domain base type
	RangeSubtype uint32 `json:"range_subtype"` // for ranges
	MultiOf      uint32 `json:"multi_of"`      // for multiranges: the range type
	Extension    string `json:"extension,omitempty"`
}

// Enum lists the labels of an enum type in sort order.
type Enum struct {
	TypeOID uint32   `json:"type_oid"`
	Labels  []string `json:"labels"`
}

// Func is a user-defined function or procedure.
type Func struct {
	Schema      string   `json:"schema"`
	Name        string   `json:"name"`
	Kind        string   `json:"kind"` // f function, p procedure
	ArgTypes    []uint32 `json:"arg_types"`
	ArgNames    []string `json:"arg_names"`
	ArgModes    []string `json:"arg_modes"` // empty when all IN
	ReturnType  uint32   `json:"return_type"`
	ReturnsSet  bool     `json:"returns_set"`
	Strict      bool     `json:"strict"`
	Volatility  string   `json:"volatility"` // i, s, v
	HasVariadic bool     `json:"has_variadic"`
	HasDefaults bool     `json:"has_defaults"`
}

// Schema is the whole model; it is what schema.lock stores.
type Schema struct {
	Version int      `json:"server_version_num"`
	Schemas []string `json:"schemas"`
	Tables  []Table  `json:"tables"`
	Types   []Type   `json:"types"`
	Enums   []Enum   `json:"enums"`
	Funcs   []Func   `json:"funcs"`
	Hash    string   `json:"hash"`
}

// Major is the PostgreSQL major version the schema was read from.
func (s *Schema) Major() int { return s.Version / 10000 }

// TypeByOID looks a type up.
func (s *Schema) TypeByOID(oid uint32) (Type, bool) {
	for _, t := range s.Types {
		if t.OID == oid {
			return t, true
		}
	}
	return Type{}, false
}

// EnumByOID returns the labels of an enum type.
func (s *Schema) EnumByOID(oid uint32) (Enum, bool) {
	for _, e := range s.Enums {
		if e.TypeOID == oid {
			return e, true
		}
	}
	return Enum{}, false
}

// Normalize sorts everything into a canonical order so output is stable.
func (s *Schema) Normalize() {
	sort.Strings(s.Schemas)
	sort.Slice(s.Tables, func(i, j int) bool {
		a, b := s.Tables[i], s.Tables[j]
		return a.Schema+"."+a.Name < b.Schema+"."+b.Name
	})
	for i := range s.Tables {
		t := &s.Tables[i]
		sort.Slice(t.Columns, func(a, b int) bool { return t.Columns[a].Num < t.Columns[b].Num })
		sort.Slice(t.Constraints, func(a, b int) bool { return t.Constraints[a].Name < t.Constraints[b].Name })
	}
	sort.Slice(s.Types, func(i, j int) bool { return s.Types[i].OID < s.Types[j].OID })
	sort.Slice(s.Enums, func(i, j int) bool { return s.Enums[i].TypeOID < s.Enums[j].TypeOID })
	sort.Slice(s.Funcs, func(i, j int) bool {
		a, b := s.Funcs[i], s.Funcs[j]
		ka := fmt.Sprintf("%s.%s.%v", a.Schema, a.Name, a.ArgTypes)
		kb := fmt.Sprintf("%s.%s.%v", b.Schema, b.Name, b.ArgTypes)
		return ka < kb
	})
}

// Lock renders the canonical schema.lock: sorted, indented JSON that ends in a
// hash of everything before it. Equal schemas give byte-equal files.
func (s *Schema) Lock() ([]byte, error) {
	cp := *s
	cp.Normalize()
	cp.Hash = ""
	body, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(body)
	cp.Hash = hex.EncodeToString(sum[:])
	out, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// ParseLock reads a schema.lock and verifies its hash.
func ParseLock(data []byte) (*Schema, error) {
	var s Schema
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("schema.lock: %w", err)
	}
	want := s.Hash
	canon, err := s.Lock()
	if err != nil {
		return nil, err
	}
	var back Schema
	_ = json.Unmarshal(canon, &back)
	if back.Hash != want {
		return nil, fmt.Errorf("schema.lock: hash mismatch (edited by hand?): file says %s, content hashes to %s", want, back.Hash)
	}
	return &s, nil
}

// Seal computes the schema's hash and returns the canonical lock file together
// with the schema as ParseLock would read it back (Hash filled in).
func Seal(s *Schema) ([]byte, *Schema, error) {
	lock, err := s.Lock()
	if err != nil {
		return nil, nil, err
	}
	back, err := ParseLock(lock)
	if err != nil {
		return nil, nil, err
	}
	return lock, back, nil
}
