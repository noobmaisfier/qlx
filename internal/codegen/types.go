package codegen

import (
	"fmt"
	"strings"

	"github.com/noobmaisfier/qlx/internal/packs"

	"github.com/noobmaisfier/qlx/internal/catalog"
)

// family selects the column constructor and so the operators a column gets.
type family int

const (
	famCol family = iota
	famNum
	famText
	famTime
	famBool
	famArray
	famJSON
)

// goType is a resolved Go type for a PostgreSQL type.
type goType struct {
	expr    string   // base Go type, without nullability: "int64", "qt.Numeric", "Tier"
	fam     family   //
	elem    string   // element type for arrays
	pg      string   // qlx.PgType expression, "" when none
	imports []string // import paths the expression needs
	nullOK  bool     // can be wrapped in qt.Null (false for slices and raw JSON)
}

// Overrides maps "schema.type" or a bare built-in type name to a Go type.
type Overrides map[string]Override

// Override replaces the default Go type of a PostgreSQL type.
type Override struct {
	Type   string `json:"type"`             // Go type expression, e.g. "github.com/google/uuid.UUID"
	Family string `json:"family,omitempty"` // col (default), num, text, time
}

type mapper struct {
	s         *catalog.Schema
	over      Overrides
	enumNames map[uint32]string
	warn      func(string)
	ids       bool // IDs mode "string": integer keys become qt.ID[Space]
	dateStd   bool // Date mode "stdlib": date is time.Time
	tsQT      bool // Timestamptz mode "qt": timestamptz is qt.Timestamptz
	arrayNull bool // array_nulls: array elements are qt.Null[T]
	exclude   map[string]bool
	idInfos   map[string]*idInfo
	uuidBytes bool                  // UUID mode "bytes": [16]byte instead of the standard library type
	packTypes map[string]packs.Type // extension types of the enabled packs, by typname
}

// newPackTypes collects the types of the named packs.
func newPackTypes(names []string) map[string]packs.Type {
	out := map[string]packs.Type{}
	for _, n := range names {
		if p, ok := packs.Lookup(n); ok {
			for _, t := range p.Types {
				out[t.Name] = t
			}
		}
	}
	return out
}

// KnownExtension reports whether name is an extension pack.
func KnownExtension(name string) bool { _, ok := packs.Lookup(name); return ok }

// ExtensionNames lists the extension packs.
func ExtensionNames() []string { return packs.Names() }

var builtin = map[string]goType{
	"bool":        {expr: "bool", fam: famBool, pg: "qlx.PgBool", nullOK: true},
	"int2":        {expr: "int16", fam: famNum, pg: "qlx.PgInt2", nullOK: true},
	"int4":        {expr: "int32", fam: famNum, pg: "qlx.PgInt4", nullOK: true},
	"int8":        {expr: "int64", fam: famNum, pg: "qlx.PgInt8", nullOK: true},
	"float4":      {expr: "float32", fam: famNum, pg: "qlx.PgFloat4", nullOK: true},
	"float8":      {expr: "float64", fam: famNum, pg: "qlx.PgFloat8", nullOK: true},
	"numeric":     {expr: "qt.Numeric", fam: famCol, pg: "qlx.PgNumeric", nullOK: true},
	"money":       {expr: "qt.Money", fam: famCol, nullOK: true},
	"text":        {expr: "string", fam: famText, pg: "qlx.PgText", nullOK: true},
	"varchar":     {expr: "string", fam: famText, pg: "qlx.PgText", nullOK: true},
	"bpchar":      {expr: "string", fam: famText, nullOK: true},
	"name":        {expr: "string", fam: famText, nullOK: true},
	"citext":      {expr: "string", fam: famText, nullOK: true},
	"char":        {expr: "string", fam: famText, nullOK: true},
	"bytea":       {expr: "[]byte", fam: famCol, pg: "qlx.PgBytea"},
	"uuid":        {expr: "uuid.UUID", fam: famCol, pg: "qlx.PgUUID", imports: []string{"uuid"}, nullOK: true},
	"date":        {expr: "qt.Date", fam: famTime, pg: "qlx.PgDate", nullOK: true},
	"timestamp":   {expr: "qt.LocalTime", fam: famTime, pg: "qlx.PgTimestamp", nullOK: true},
	"timestamptz": {expr: "time.Time", fam: famTime, pg: "qlx.PgTimestamptz", imports: []string{"time"}, nullOK: true},
	"time":        {expr: "qt.TimeOfDay", fam: famCol, nullOK: true},
	"timetz":      {expr: "qt.TimeTZ", fam: famCol, nullOK: true},
	"interval":    {expr: "qt.Interval", fam: famCol, pg: "qlx.PgInterval", nullOK: true},
	"json":        {expr: "qt.JSONB[json.RawMessage]", fam: famJSON, pg: "qlx.PgJSON", imports: []string{"encoding/json"}, nullOK: true},
	"jsonb":       {expr: "qt.JSONB[json.RawMessage]", fam: famJSON, pg: "qlx.PgJSONB", imports: []string{"encoding/json"}, nullOK: true},
	"inet":        {expr: "netip.Addr", fam: famCol, pg: "qlx.PgInet", imports: []string{"net/netip"}, nullOK: true},
	"cidr":        {expr: "netip.Prefix", fam: famCol, imports: []string{"net/netip"}, nullOK: true},
	"bit":         {expr: "qt.Bit", fam: famCol, nullOK: true},
	"varbit":      {expr: "qt.Bit", fam: famCol, nullOK: true},
	"point":       {expr: "qt.Point", fam: famCol, nullOK: true},
	"box":         {expr: "qt.Box", fam: famCol, nullOK: true},
	"circle":      {expr: "qt.Circle", fam: famCol, nullOK: true},
	"polygon":     {expr: "qt.Polygon", fam: famCol, nullOK: true},
	"path":        {expr: "qt.Path", fam: famCol, nullOK: true},
	"line":        {expr: "qt.Line", fam: famCol, nullOK: true},
	"lseg":        {expr: "qt.Lseg", fam: famCol, nullOK: true},
	"macaddr":     {expr: "qt.MacAddr", fam: famCol, nullOK: true},
	"macaddr8":    {expr: "qt.MacAddr8", fam: famCol, nullOK: true},
	"pg_lsn":      {expr: "qt.LSN", fam: famCol, nullOK: true},
	"oid":         {expr: "uint32", fam: famCol, nullOK: true},
	"xml":         {expr: "qt.XML", fam: famCol, nullOK: true},
	"tsvector":    {expr: "qt.TSVector", fam: famCol, nullOK: true},
	"tsquery":     {expr: "qt.TSQuery", fam: famCol, nullOK: true},
}

// resolve maps a type OID to a Go type. Unknown types become string and a
// warning is recorded, so a schema never fails to generate; the user can add
// an override.
func (m *mapper) resolve(oid uint32) goType {
	t, ok := m.s.TypeByOID(oid)
	if !ok {
		m.warn(fmt.Sprintf("type oid %d not in catalog; using string", oid))
		return goType{expr: "string", fam: famCol, nullOK: true}
	}
	if ov, ok := m.override(t); ok {
		return ov
	}
	switch {
	case t.Category == "A" && t.Elem != 0:
		e := m.resolve(t.Elem)
		if m.arrayNull && e.nullOK && e.fam != famArray {
			el := "qt.Null[" + e.expr + "]"
			return goType{expr: "[]" + el, fam: famArray, elem: el, imports: e.imports}
		}
		out := goType{expr: "[]" + e.expr, fam: famArray, elem: e.expr, imports: e.imports}
		if e.pg != "" {
			out.pg = "qlx.ArrayOf(" + e.pg + ")"
		}
		return out
	case t.Kind == "d":
		return m.resolve(t.Base)
	case t.Kind == "e":
		n := m.enumNames[t.OID]
		return goType{expr: n, fam: famCol, pg: n + "Type", nullOK: true}
	case t.Kind == "r":
		e := m.resolve(t.RangeSubtype)
		return goType{expr: "qt.Range[" + e.expr + "]", fam: famCol, imports: e.imports}
	case t.Kind == "m":
		r := m.resolve(t.MultiOf)
		return goType{expr: "qt.Multirange[" + elemOfRange(r.expr) + "]", fam: famCol, imports: r.imports}
	}
	if g, ok := builtin[t.Name]; ok {
		if t.Name == "date" && m.dateStd {
			return goType{expr: "time.Time", fam: famTime, pg: "qlx.NewPgType[time.Time](\"pg_catalog\", \"date\")", imports: []string{"time"}, nullOK: true}
		}
		if t.Name == "timestamptz" && m.tsQT {
			return goType{expr: "qt.Timestamptz", fam: famTime, pg: "qlx.NewPgType[qt.Timestamptz](\"pg_catalog\", \"timestamptz\")", nullOK: true}
		}
		if t.Name == "uuid" && m.uuidBytes {
			return goType{expr: "[16]byte", fam: famCol, pg: `qlx.NewPgType[[16]byte]("pg_catalog", "uuid")`, nullOK: true}
		}
		return g
	}
	if pt, ok := m.packTypes[t.Name]; ok {
		g := goType{expr: pt.Expr, fam: famCol, nullOK: pt.NullOK, imports: pt.Imports}
		if pt.Family == "text" {
			g.fam = famText
		}
		return g
	}
	m.warn(fmt.Sprintf("type %s.%s has no Go mapping; using string (add an override or enable its extension pack)", t.Schema, t.Name))
	return goType{expr: "string", fam: famCol, nullOK: true}
}

func elemOfRange(r string) string {
	const p = "qt.Range["
	if len(r) > len(p) && r[:len(p)] == p {
		return r[len(p) : len(r)-1]
	}
	return r
}

func (m *mapper) override(t catalog.Type) (goType, bool) {
	for _, k := range []string{t.Schema + "." + t.Name, t.Name} {
		o, ok := m.over[k]
		if !ok {
			continue
		}
		path, name := splitImport(o.Type)
		// A slice or map is already nil-able: like the built-in arrays it
		// stays unwrapped, and a nullable column holds nil for NULL.
		sliceLike := strings.HasPrefix(o.Type, "[]") || strings.HasPrefix(o.Type, "map[")
		g := goType{expr: name, fam: famCol, nullOK: !sliceLike}
		if path != "" {
			g.imports = []string{path}
		}
		switch o.Family {
		case "num":
			g.fam = famNum
		case "text":
			g.fam = famText
		case "time":
			g.fam = famTime
		}
		return g, true
	}
	return goType{}, false
}

// splitImport turns "github.com/google/uuid.UUID" into the import path and
// the qualified Go expression "uuid.UUID".
func splitImport(s string) (path, expr string) {
	slash := -1
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			slash = i
			break
		}
	}
	dot := -1
	for i := slash + 1; i < len(s); i++ {
		if s[i] == '.' {
			dot = i
			break
		}
	}
	if dot < 0 {
		return "", s
	}
	path = s[:dot]
	pkg := path
	if slash >= 0 {
		pkg = path[slash+1:]
	}
	return path, pkg + s[dot:]
}
