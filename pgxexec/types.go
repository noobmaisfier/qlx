package pgxexec

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// pgx knows the types of the standard catalog it has a codec for. For every
// other type it falls back to text, which works for a single value but not for
// an array of them: scanning {a,b} needs the element type registered, and an
// unknown OID inside an array fails with "cannot scan unknown type (OID n) in
// text format". Three groups of types are affected, and each has a fix here:
//
//   - catalog types pgx has no codec for (money, timetz, tsquery, int2vector,
//     ...) and the array types of multiranges: registered statically by
//     RegisterRangeTypes, since their OIDs never change;
//   - enums, domains and extension types (hstore, ltree, citext, vector,
//     geometry, ...): their OIDs are assigned by the database, so LoadTypes
//     reads them from pg_type, and AfterConnect calls it on each connection.
//
// A type is registered with a text-only codec: values travel and arrive in text
// form and are handed to the Go type's Scan and Value methods (every qt type
// has them), or to a string. It must be text only. pgx's own TextCodec claims
// the binary format too, because for text the two are the same; for money,
// timetz, ltree or hstore they are not, and an array would be requested in
// binary and arrive as bytes the Go type cannot read.
type textOnlyCodec struct{ pgtype.TextCodec }

func (textOnlyCodec) FormatSupported(format int16) bool { return format == pgtype.TextFormatCode }
func (textOnlyCodec) PreferredFormat() int16            { return pgtype.TextFormatCode }

type textType struct {
	name       string
	oid, array uint32
}

// builtinTextTypes are the catalog types without a pgx codec. The OIDs are
// fixed in PostgreSQL's catalog (pg_type.dat). macaddr8 is known to pgx, only
// its array type is not.
var builtinTextTypes = []textType{
	{"int2vector", 22, 1006},
	{"regproc", 24, 1008},
	{"oidvector", 30, 1013},
	{"money", 790, 791},
	{"timetz", 1266, 1270},
	{"refcursor", 1790, 2201},
	{"regprocedure", 2202, 2207},
	{"regoper", 2203, 2208},
	{"regoperator", 2204, 2209},
	{"regclass", 2205, 2210},
	{"regtype", 2206, 2211},
	{"pg_lsn", 3220, 3221},
	{"tsvector", 3614, 3643},
	{"tsquery", 3615, 3645},
	{"regconfig", 3734, 3735},
	{"regdictionary", 3769, 3770},
	{"regnamespace", 4089, 4090},
	{"regrole", 4096, 4097},
	{"regcollation", 4191, 4192},
	{"macaddr8", 774, 775},
}

// multirangeArrays are the array types of the multiranges pgx registers
// without their arrays: name, multirange OID, array OID.
var multirangeArrays = []textType{
	{"int4multirange", 4451, 6150},
	{"nummultirange", 4532, 6151},
	{"tsmultirange", 4533, 6152},
	{"tstzmultirange", 4534, 6153},
	{"datemultirange", 4535, 6155},
	{"int8multirange", 4536, 6157},
}

// registerCatalogTypes adds what pgx lacks for the standard catalog.
func registerCatalogTypes(m *pgtype.Map) {
	for _, t := range builtinTextTypes {
		registerTextWithArray(m, t.name, t.oid, t.array)
	}
	for _, t := range multirangeArrays {
		if el, ok := m.TypeForOID(t.oid); ok {
			registerArray(m, t.name, el, t.array)
		}
	}
}

func registerArray(m *pgtype.Map, elemName string, el *pgtype.Type, arrayOID uint32) {
	if _, ok := m.TypeForOID(arrayOID); ok {
		return
	}
	m.RegisterType(&pgtype.Type{Name: "_" + elemName, OID: arrayOID, Codec: &pgtype.ArrayCodec{ElementType: el}})
}

// registerTextWithArray registers a text-codec type and its array type, unless
// pgx already knows them.
func registerTextWithArray(m *pgtype.Map, name string, oid, arrayOID uint32) {
	el, ok := m.TypeForOID(oid)
	if !ok {
		el = &pgtype.Type{Name: name, OID: oid, Codec: textOnlyCodec{}}
		m.RegisterType(el)
	}
	if arrayOID != 0 {
		registerArray(m, name, el, arrayOID)
	}
}

// userTypesQuery lists the enums, domains and base types (extension types
// among them) that have an array type, outside the system schemas. Types the
// catalog list above covers are skipped by the registration, which does not
// replace a type pgx already knows.
const userTypesQuery = `
SELECT t.oid::int8, t.typname::text, t.typtype::text, t.typbasetype::int8, t.typarray::int8
FROM pg_catalog.pg_type t
WHERE t.typtype IN ('e', 'd', 'b')
  AND t.typarray <> 0
  AND t.typnamespace NOT IN ('pg_catalog'::regnamespace, 'information_schema'::regnamespace, 'pg_toast'::regnamespace)
ORDER BY t.oid`

// LoadTypes registers, on this connection, the enums, domains and extension
// types of the database together with their array types, so that arrays of
// them can be read and written (see the note above). AfterConnect does it for
// every pooled connection; call it yourself on a connection you opened another
// way, or after a migration created types the connection has not seen.
//
// It costs one query. A domain whose base type pgx cannot resolve is skipped
// (its scalar values still work through text).
func LoadTypes(ctx context.Context, c *pgx.Conn) error {
	m := c.TypeMap()
	registerCatalogTypes(m)
	rows, err := c.Query(ctx, userTypesQuery, pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		return fmt.Errorf("pgxexec: loading types: %w", err)
	}
	type row struct {
		oid        uint32
		name, kind string
		base       uint32
		array      uint32
	}
	var all []row
	for rows.Next() {
		var oid, base, arr int64
		var r row
		if err := rows.Scan(&oid, &r.name, &r.kind, &base, &arr); err != nil {
			rows.Close()
			return fmt.Errorf("pgxexec: loading types: %w", err)
		}
		r.oid, r.base, r.array = uint32(oid), uint32(base), uint32(arr)
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("pgxexec: loading types: %w", err)
	}
	// enums and base types first: a domain may be over one of them
	for _, r := range all {
		switch r.kind {
		case "e":
			if _, ok := m.TypeForOID(r.oid); !ok {
				m.RegisterType(&pgtype.Type{Name: r.name, OID: r.oid, Codec: &pgtype.EnumCodec{}})
			}
			if el, ok := m.TypeForOID(r.oid); ok {
				registerArray(m, r.name, el, r.array)
			}
		case "b":
			registerTextWithArray(m, r.name, r.oid, r.array)
		}
	}
	// domains, repeated until nothing more resolves (a domain over a domain)
	for pending := all; len(pending) > 0; {
		var next []row
		for _, r := range pending {
			if r.kind != "d" {
				continue
			}
			base, ok := m.TypeForOID(r.base)
			if !ok {
				next = append(next, r)
				continue
			}
			if _, known := m.TypeForOID(r.oid); !known {
				m.RegisterType(&pgtype.Type{Name: r.name, OID: r.oid, Codec: base.Codec})
			}
			if el, ok := m.TypeForOID(r.oid); ok {
				registerArray(m, r.name, el, r.array)
			}
		}
		if len(next) == len(pending) {
			break
		}
		pending = next
	}
	return nil
}
