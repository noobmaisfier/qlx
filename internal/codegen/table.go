package codegen

import (
	"strconv"
	"strings"

	"github.com/noobmaisfier/qlx/internal/catalog"
)

type colGen struct {
	col      catalog.Column
	field    string
	typ      goType
	nullable bool
	goT      string // Go type of values: "int64", "qt.Null[string]"
	settable bool   // may appear in INSERT/UPDATE
	required bool   // INSERT must set it
}

type tableGen struct {
	t       catalog.Table
	sym     string // table symbol, "Users"
	tname   string // type name, "UsersT"
	row     string // row type, "User"
	cols    []colGen
	usedFld map[string]bool
	m       *mapper
	all     map[string]*tableGen // every generated table, for foreign keys
}

// Names that a column field must not take: the generated types have these
// methods, and a field of the same name would not compile.
var reservedField = map[string]bool{"As": true, "Assignments": true, "All": true}

func newTableGen(m *mapper, nm *namer, t catalog.Table, multiSchema bool, jsonTypes map[string]string) *tableGen {
	g := &tableGen{t: t, m: m, usedFld: map[string]bool{}}
	prefix := ""
	if multiSchema {
		prefix = camel(t.Schema)
	}
	g.sym = nm.take(camel(t.Name), camel(t.Schema))
	g.tname = nm.take(g.sym+"T", "")
	row := singular(camel(t.Name))
	g.row = nm.take(row, "Row")
	_ = prefix
	for _, c := range t.Columns {
		cg := colGen{col: c}
		cg.typ = m.resolve(c.TypeOID)
		if info := m.idFor(nm, t, c.Name, 0); info != nil {
			cg.typ = idGoType(info, m.intName(c.TypeOID))
		}
		cg.nullable = !c.NotNull
		if cg.typ.fam == famJSON {
			for _, k := range []string{t.Schema + "." + t.Name + "." + c.Name, t.Name + "." + c.Name} {
				if ty, ok := jsonTypes[k]; ok {
					path, expr := splitImport(ty)
					imps := append([]string(nil), cg.typ.imports...)
					if path != "" {
						imps = append(imps, path)
					}
					cg.typ.expr, cg.typ.imports = "qt.JSONB["+expr+"]", imps
					break
				}
			}
		}
		cg.goT = cg.typ.expr
		if cg.nullable && cg.typ.nullOK && cg.typ.fam != famArray {
			cg.goT = "qt.Null[" + cg.typ.expr + "]"
		}
		f := camel(c.Name)
		if reservedField[f] {
			f += "Col"
		}
		base := f
		for i := 2; g.usedFld[f]; i++ {
			f = base + itoa(i)
		}
		g.usedFld[f] = true
		cg.field = f
		cg.settable = c.Generated == "" && !t.ReadOnly() && c.Identity != "a"
		cg.required = cg.settable && c.NotNull && !c.HasDefault && c.Identity == ""
		g.cols = append(g.cols, cg)
	}
	return g
}

// dbTag is the struct tag db:"name". A name with a backquote cannot sit in a
// raw string, so the tag is then written as an interpreted string.
func dbTag(name string) string {
	tag := "db:" + strconv.Quote(name)
	if strings.Contains(tag, "`") {
		return strconv.Quote(tag)
	}
	return "`" + tag + "`"
}

// colType is the column symbol type: the family decides the operators.
func (c colGen) colType() (typ string, ctor string) {
	t := c.typ
	switch t.fam {
	case famNum:
		return "qlx.NumCol[" + c.goT + "]", "qlx.NewNumCol[" + c.goT + "]"
	case famText:
		return "qlx.TextCol[" + c.goT + "]", "qlx.NewTextCol[" + c.goT + "]"
	case famTime:
		return "qlx.TimeCol[" + c.goT + "]", "qlx.NewTimeCol[" + c.goT + "]"
	case famBool:
		if !c.nullable {
			return "qlx.BoolCol", "qlx.NewBoolCol"
		}
	case famArray:
		return "qlx.ArrayCol[" + t.elem + "]", "qlx.NewArrayCol[" + t.elem + "]"
	case famJSON:
		return "qlx.JSONBCol[" + c.goT + "]", "qlx.NewJSONBCol[" + c.goT + "]"
	}
	return "qlx.Col[" + c.goT + "]", "qlx.NewCol[" + c.goT + "]"
}

// assignT is the Go type that To() accepts for the column.
func (c colGen) assignT() string {
	if c.typ.fam == famArray {
		return "[]" + c.typ.elem
	}
	return c.goT
}

func (g *tableGen) emit(f *file) {
	t := g.t
	f.use("github.com/noobmaisfier/qlx", "github.com/noobmaisfier/qlx/qt")
	for _, c := range g.cols {
		f.use(c.typ.imports...)
	}
	kind := map[string]string{"r": "table", "p": "partitioned table", "v": "view", "m": "materialized view", "f": "foreign table"}[t.Kind]
	if kind == "" {
		kind = "relation"
	}

	// ID space, when this table owns one.
	if info := g.m.idInfos[t.Schema+"."+t.Name]; info != nil && info.owner == t.Schema+"."+t.Name {
		f.p("// %s identifies the id space of %s.%s: the key that derives its text form.\ntype %s struct{}\n\n", info.marker, t.Schema, t.Name, info.marker)
		f.p("// IDSpace implements qt.IDSpace.\nfunc (%s) IDSpace() string { return %q }\n\n", info.marker, info.space)
		f.p("// %s is the id of a %s.%s row: an integer in the database, an opaque\n// string in JSON, URLs and logs (see qt.ID; set the key with qt.SetIDKey).\ntype %s = qt.ID[%s]\n\n", info.alias, t.Schema, t.Name, info.alias, info.marker)
	}

	// Table symbol.
	f.p("// %s is the %s %s.%s. %s.As(\"x\") returns an aliased copy.\n", g.tname, kind, t.Schema, t.Name, g.sym)
	f.p("type %s struct {\n\tqlx.TableRef\n", g.tname)
	for _, c := range g.cols {
		typ, _ := c.colType()
		f.p("\t%s %s\n", c.field, typ)
	}
	f.p("}\n\n")

	var req []string
	for _, c := range g.cols {
		if c.required {
			req = append(req, "%q")
			_ = c
		}
	}
	f.p("func new%s(alias string) %s {\n", g.sym, g.tname)
	f.p("\tt := qlx.TableRef{Schema: %q, Name: %q, Alias: alias", t.Schema, t.Name)
	if len(req) > 0 {
		f.p(", Required: []string{")
		first := true
		for _, c := range g.cols {
			if c.required {
				if !first {
					f.p(", ")
				}
				f.p("%q", c.col.Name)
				first = false
			}
		}
		f.p("}")
	}
	if len(t.PrimaryKey) > 0 {
		f.p(", PK: []string{")
		for i, k := range t.PrimaryKey {
			if i > 0 {
				f.p(", ")
			}
			f.p("%q", k)
		}
		f.p("}")
	}
	f.p("}\n\treturn %s{\n\t\tTableRef: t,\n", g.tname)
	for _, c := range g.cols {
		_, ctor := c.colType()
		f.p("\t\t%s: %s(t, %q),\n", c.field, ctor, c.col.Name)
	}
	f.p("\t}\n}\n\n")
	f.p("// %s is the table symbol.\nvar %s = new%s(\"\")\n\n", g.sym, g.sym, g.sym)
	f.p("// As returns the %s under an alias, with every column rebound to it.\nfunc (%s) As(alias string) %s { return new%s(alias) }\n\n", kind, g.tname, g.tname, g.sym)

	// Row.
	f.p("// %s is one row of %s.%s, for scanning by column name.\ntype %s struct {\n", g.row, t.Schema, t.Name, g.row)
	for _, c := range g.cols {
		f.p("\t%s %s %s\n", c.field, c.goT, dbTag(c.col.Name))
	}
	f.p("}\n\n")

	// Reflection-free scanning for SELECT of the whole row.
	f.p("// All lists every column in table order, for SELECT; scan the rows into %s.\nfunc (t %s) All() []qlx.Selectable {\n\treturn []qlx.Selectable{", g.row, g.tname)
	for i, c := range g.cols {
		if i > 0 {
			f.p(", ")
		}
		f.p("t.%s", c.field)
	}
	f.p("}\n}\n\n")
	f.p("// ScanTargets implements pgxexec.Scannable: pointers to the fields in the\n// order of %s.All(), so rows scan without reflection.\nfunc (r *%s) ScanTargets() []any {\n\treturn []any{", g.tname, g.row)
	for i, c := range g.cols {
		if i > 0 {
			f.p(", ")
		}
		f.p("&r.%s", c.field)
	}
	f.p("}\n}\n\n")

	// Constraint symbols.
	for _, k := range t.Constraints {
		if k.Type != "p" && k.Type != "u" {
			continue
		}
		f.p("// %s%s is the %s constraint %s, for ON CONFLICT ON CONSTRAINT.\nvar %s%s = qlx.Constraint{Name: %q}\n\n",
			g.sym, camel(k.Name), map[string]string{"p": "primary key", "u": "unique"}[k.Type], k.Name, g.sym, camel(k.Name), k.Name)
	}

	g.emitForeignKeys(f)

	if t.ReadOnly() {
		return
	}
	g.emitInsert(f)
	g.emitPatch(f)
}

func (g *tableGen) emitInsert(f *file) {
	ins := g.row + "Insert"
	f.p("// %s is a row to insert. Build it with New%s, which takes the columns\n// that have no default, then chain setters for the rest.\ntype %s struct{ as []qlx.Assignment }\n\n", ins, ins, ins)
	var params []string
	var sets []string
	taken := map[string]bool{}
	for _, c := range g.cols {
		if c.required {
			p := lowerCamel(c.col.Name)
			for base, i := p, 2; taken[p]; i++ { // two columns can collapse to one name
				p = base + itoa(i)
			}
			taken[p] = true
			params = append(params, p+" "+c.assignT())
			sets = append(sets, g.sym+"."+c.field+".To("+p+")")
		}
	}
	f.p("// New%s starts an insert with its required columns.\nfunc New%s(%s) %s {\n\treturn %s{as: []qlx.Assignment{%s}}\n}\n\n",
		ins, ins, strings.Join(params, ", "), ins, ins, strings.Join(sets, ", "))
	for _, c := range g.cols {
		if !c.settable || c.required {
			continue
		}
		f.p("// %s sets %s.\nfunc (i %s) %s(v %s) %s {\n\ti.as = append(i.as[:len(i.as):len(i.as)], %s.%s.To(v))\n\treturn i\n}\n\n",
			c.field, c.col.Name, ins, c.field, c.assignT(), ins, g.sym, c.field)
	}
	f.p("// Assignments implements qlx.RowAssigner.\nfunc (i %s) Assignments() []qlx.Assignment { return i.as }\n\n", ins)
}

func (g *tableGen) emitPatch(f *file) {
	pt := g.row + "Patch"
	f.p("// %s is a partial update: only the fields that are set are written.\ntype %s struct {\n", pt, pt)
	for _, c := range g.cols {
		if c.settable {
			f.p("\t%s qt.Opt[%s]\n", c.field, c.assignT())
		}
	}
	f.p("}\n\n// Assignments implements qlx.RowAssigner.\nfunc (p %s) Assignments() []qlx.Assignment {\n\tvar as []qlx.Assignment\n", pt)
	for _, c := range g.cols {
		if c.settable {
			f.p("\tif p.%s.Set {\n\t\tas = append(as, %s.%s.To(p.%s.V))\n\t}\n", c.field, g.sym, c.field, c.field)
		}
	}
	f.p("\treturn as\n}\n\n")
}

// emitForeignKeys writes one method per foreign key whose target table was
// generated too: Orders.FKCustomer(Customers) returns a qlx.ForeignKey
// bound to the references passed in, so aliased and self joins work. Keys
// whose target is excluded or in another package are skipped.
func (g *tableGen) emitForeignKeys(f *file) {
	fieldOf := func(tg *tableGen, col string) (string, bool) {
		for _, c := range tg.cols {
			if c.col.Name == col {
				return c.field, true
			}
		}
		return "", false
	}
	for _, k := range g.t.Constraints {
		if k.Type != "f" || len(k.Columns) == 0 || len(k.Columns) != len(k.RefColumns) {
			continue
		}
		ref := g.all[k.RefSchema+"."+k.RefTable]
		if ref == nil {
			continue
		}
		method := "FK" + camel(k.Name)
		if g.usedFld[method] {
			continue
		}
		var pairs []string
		ok := true
		for i, c := range k.Columns {
			a, ok1 := fieldOf(g, c)
			b, ok2 := fieldOf(ref, k.RefColumns[i])
			if !ok1 || !ok2 {
				ok = false
				break
			}
			pairs = append(pairs, "qlx.FKPair(t."+a+", to."+b+")")
		}
		if !ok {
			continue
		}
		f.p("// %s is the foreign key %s to %s.%s: join with .OnFK(a.%s(b)).\nfunc (t %s) %s(to %s) qlx.ForeignKey {\n\treturn qlx.NewForeignKey(%q, %s)\n}\n\n",
			method, k.Name, k.RefSchema, k.RefTable, method, g.tname, method, ref.tname, k.Name, strings.Join(pairs, ", "))
	}
}
