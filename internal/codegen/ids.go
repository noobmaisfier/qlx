package codegen

import (
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
		enumNames: map[uint32]string{}, warn: warn, ids: cfg.IDs == "string", dateStd: cfg.Date == "stdlib",
		exclude: map[string]bool{}, idInfos: map[string]*idInfo{}}
	for _, e := range cfg.Exclude {
		m.exclude[e] = true
	}
	return m
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
		info.marker = nm.take(camel(t.Name)+"IDSpace", camel(t.Schema))
		info.alias = nm.take(singular(camel(t.Name))+"ID", camel(t.Schema))
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
