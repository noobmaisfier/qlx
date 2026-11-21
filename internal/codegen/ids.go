package codegen

import (
	"fmt"

	"github.com/noobmaisfier/qlx/internal/catalog"
)

// idInfo names the ID type of one table: the marker that identifies its id
// space and the alias applications use.
type idInfo struct {
	owner  string // "schema.table" whose primary key defines the space
	space  string // the string qt.IDSpace returns
	marker string // Go name of the marker type
	alias  string // Go name of the ID alias
}

func newMapper(s *catalog.Schema, cfg Config, warn func(string)) *mapper {
	m := &mapper{s: s, over: cfg.Overrides, uuidBytes: cfg.UUID == "bytes", packTypes: newPackTypes(cfg.Extensions),
		enumNames: map[uint32]string{}, warn: warn, ids: cfg.IDs == "string", dateStd: cfg.Date == "stdlib", tsQT: cfg.Timestamptz != "stdlib", arrayNull: cfg.ArrayNulls,
		exclude: map[string]bool{}, idInfos: map[string]*idInfo{}}
	for _, e := range cfg.Exclude {
		m.exclude[e] = true
	}
	m.notNull = viewNotNull(s, cfg.NotNull)
	return m
}

// viewNotNull resolves the not_null entries to "schema.view.column" keys of
// the view and materialized-view columns they name. Entries that match
// nothing are left out here; checkNotNull reports them.
func viewNotNull(s *catalog.Schema, entries []string) map[string]bool {
	want := map[string]bool{}
	for _, e := range entries {
		want[e] = true
	}
	out := map[string]bool{}
	for _, t := range s.Tables {
		if t.Kind != "v" && t.Kind != "m" {
			continue
		}
		for _, c := range t.Columns {
			if want[t.Schema+"."+t.Name+"."+c.Name] || want[t.Name+"."+c.Name] {
				out[t.Schema+"."+t.Name+"."+c.Name] = true
			}
		}
	}
	return out
}

// checkNotNull refuses a not_null entry that does not name a view column, so a
// typo or a renamed column cannot leave a column nullable without notice.
func checkNotNull(s *catalog.Schema, entries []string) error {
	for _, e := range entries {
		var viewCol, tableCol bool
		for _, t := range s.Tables {
			for _, c := range t.Columns {
				if e != t.Schema+"."+t.Name+"."+c.Name && e != t.Name+"."+c.Name {
					continue
				}
				if t.Kind == "v" || t.Kind == "m" {
					viewCol = true
				} else {
					tableCol = true
				}
			}
		}
		switch {
		case viewCol:
		case tableCol:
			return fmt.Errorf("config: not_null %q names a table column: its nullability comes from the schema (NOT NULL); not_null is for views and materialized views", e)
		default:
			return fmt.Errorf("config: not_null %q names no view or materialized-view column (use view.column or schema.view.column)", e)
		}
	}
	return nil
}

func (m *mapper) table(schema, name string) (catalog.Table, bool) {
	for _, t := range m.s.Tables {
		if t.Schema == schema && t.Name == name {
			return t, true
		}
	}
	return catalog.Table{}, false
}

func isIntType(name string) bool { return name == "int2" || name == "int4" || name == "int8" }

// idFor reports the ID type of column col of the table, or nil when the column
// is not an integer key. A primary key defines its table's id space; a
// foreign key (or a primary key that is itself a foreign key, as in a 1:1
// extension table) takes the space of the column it references.
func (m *mapper) idFor(nm *namer, t catalog.Table, col string, depth int) *idInfo {
	if !m.ids || depth > 8 || t.Kind != "r" && t.Kind != "p" {
		return nil
	}
	if m.exclude[t.Schema+"."+t.Name] || m.exclude[t.Name] {
		return nil
	}
	var c *catalog.Column
	for i := range t.Columns {
		if t.Columns[i].Name == col {
			c = &t.Columns[i]
		}
	}
	if c == nil {
		return nil
	}
	ty, ok := m.s.TypeByOID(c.TypeOID)
	if !ok || ty.Schema != "pg_catalog" || !isIntType(ty.Name) {
		return nil // domains over integers and overridden types stay as they are
	}
	if _, over := m.override(ty); over {
		return nil
	}
	for _, k := range t.Constraints {
		if k.Type == "f" && len(k.Columns) == 1 && k.Columns[0] == col && len(k.RefColumns) == 1 {
			if rt, ok := m.table(k.RefSchema, k.RefTable); ok {
				if info := m.idFor(nm, rt, k.RefColumns[0], depth+1); info != nil {
					return info
				}
			}
		}
	}
	if len(t.PrimaryKey) == 1 && t.PrimaryKey[0] == col {
		key := t.Schema + "." + t.Name
		if info, ok := m.idInfos[key]; ok {
			return info
		}
		info := &idInfo{owner: key, space: key}
		info.marker = nm.take(nm.local(t.Schema, t.Name)+"IDSpace", camel(t.Schema))
		info.alias = nm.take(nm.localSingular(t.Schema, t.Name)+"ID", camel(t.Schema))
		m.idInfos[key] = info
		return info
	}
	return nil
}

func idGoType(info *idInfo, pgName string) goType {
	e := "qt." + "ID[" + info.marker + "]"
	return goType{expr: e, fam: famCol, pg: `qlx.NewPgType[` + e + `]("pg_catalog", "` + pgName + `")`, nullOK: true}
}

func (m *mapper) intName(oid uint32) string {
	if t, ok := m.s.TypeByOID(oid); ok {
		return t.Name
	}
	return "int8"
}
