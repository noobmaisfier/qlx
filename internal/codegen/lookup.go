package codegen

import (
	"github.com/noobmaisfier/qlx/internal/catalog"
)

// ColumnName ties a SQL column to the Go field that represents it.
type ColumnName struct {
	SQL      string
	Field    string
	Family   string // "num", "text", "time", "bool", "array", "json", "col"
	Nullable bool
	GoType   string
}

// TableName ties a SQL relation to its generated symbol.
type TableName struct {
	Schema  string
	SQL     string
	Symbol  string // "Users": the value users write in From(...)
	Columns []ColumnName
}

// Column finds a column by its SQL name.
func (t TableName) Column(name string) (ColumnName, bool) {
	for _, c := range t.Columns {
		if c.SQL == name {
			return c, true
		}
	}
	return ColumnName{}, false
}

var familyNames = map[family]string{
	famCol: "col", famNum: "num", famText: "text", famTime: "time",
	famBool: "bool", famArray: "array", famJSON: "json",
}

// Names replays the naming decisions of Generate (enums first, then tables in
// lock order) without producing source, so that tools which emit code against
// the generated package (sql2go) agree with it symbol for symbol.
func Names(s *catalog.Schema, cfg Config) []TableName {
	m := newMapper(s, cfg, func(string) {})
	nm := newNamer()
	exclude := m.exclude
	for _, t := range s.Types {
		if t.Kind == "e" {
			m.enumNames[t.OID] = nm.take(camel(t.Name), camel(t.Schema))
		}
	}
	// emitEnum reserves one constant name per label; replay that too, or a
	// table named like a constant would get a different symbol than in gen.
	for _, t := range s.Types {
		if t.Kind != "e" {
			continue
		}
		en, _ := s.EnumByOID(t.OID)
		goName := m.enumNames[t.OID]
		seen := map[string]bool{}
		for _, l := range en.Labels {
			c := goName + camel(l)
			for i := 2; seen[c] || nm.used[c] && !seen[c]; i++ {
				c = goName + camel(l) + itoa(i)
			}
			seen[c] = true
			nm.used[c] = true
		}
	}
	multi := len(cfg.Schemas) > 1
	var out []TableName
	for _, t := range s.Tables {
		if exclude[t.Schema+"."+t.Name] || exclude[t.Name] {
			continue
		}
		g := newTableGen(m, nm, t, multi, cfg.JSONTypes)
		tn := TableName{Schema: t.Schema, SQL: t.Name, Symbol: g.sym}
		for _, c := range g.cols {
			tn.Columns = append(tn.Columns, ColumnName{
				SQL: c.col.Name, Field: c.field, Family: familyNames[c.typ.fam],
				Nullable: c.nullable, GoType: c.goT,
			})
		}
		out = append(out, tn)
	}
	return out
}
