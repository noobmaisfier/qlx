package sql2go

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/noobmaisfier/qlx/internal/codegen"
)

// chain joins builder calls one per line.
func chain(first string, calls []string) string {
	if len(calls) == 0 {
		return first
	}
	return first + ".\n" + strings.Join(calls, ".\n")
}

func (c *conv) fromTree(n any) (first string, joins []string, err error) {
	k, f := kindOf(n)
	switch k {
	case "RangeVar":
		b, err := c.bindRange(f)
		if err != nil {
			return "", nil, err
		}
		return b.code, nil, nil
	case "JoinExpr":
		if b, _ := f["isNatural"].(bool); b || len(list(f, "usingClause")) > 0 {
			return "", nil, c.unsupported(f, "JOIN ... USING / NATURAL JOIN", "write the ON condition")
		}
		first, joins, err = c.fromTree(f["larg"])
		if err != nil {
			return "", nil, err
		}
		rk, rf := kindOf(f["rarg"])
		if rk != "RangeVar" {
			return "", nil, c.unsupported(f, "joined item "+rk, "only tables and views can be joined")
		}
		rb, err := c.bindRange(rf)
		if err != nil {
			return "", nil, err
		}
		jt := str(f, "jointype")
		if f["quals"] == nil {
			if jt == "JOIN_INNER" || jt == "" {
				return first, append(joins, "CrossJoin("+rb.code+")"), nil
			}
			return "", nil, c.unsupported(f, "outer join without ON", "")
		}
		ps, err := c.predList(f["quals"])
		if err != nil {
			return "", nil, err
		}
		on := ps[0]
		if len(ps) > 1 {
			on = c.o.Q + ".And(" + strings.Join(ps, ", ") + ")"
		}
		m, ok := map[string]string{"JOIN_INNER": "Join", "JOIN_LEFT": "LeftJoin", "JOIN_RIGHT": "RightJoin", "JOIN_FULL": "FullJoin"}[jt]
		if !ok {
			return "", nil, c.unsupported(f, "join type "+jt, "")
		}
		return first, append(joins, fmt.Sprintf("%s(%s).On(%s)", m, rb.code, on)), nil
	}
	return "", nil, c.unsupported(f, "FROM item "+k, "only tables, views and joins are converted")
}

func (c *conv) star(f map[string]any) ([]val, bool, error) {
	fs := list(f, "fields")
	if len(fs) == 0 {
		return nil, false, nil
	}
	if k, _ := kindOf(fs[len(fs)-1]); k != "A_Star" {
		return nil, false, nil
	}
	var qual string
	if len(fs) == 2 {
		qual = names(fs[:1])[0]
	} else if len(fs) != 1 {
		return nil, true, c.unsupported(f, "qualified *", "")
	}
	var out []val
	for _, b := range c.sc.b {
		if qual != "" && b.qual != qual {
			continue
		}
		for _, col := range b.tbl.Columns {
			out = append(out, colVal(b, col))
		}
	}
	if len(out) == 0 {
		return nil, true, c.unsupported(f, "* with nothing to expand", "")
	}
	return out, true, nil
}

// targets renders a select or returning list.
func (c *conv) targets(l []any, record bool) ([]string, error) {
	existsBody := c.exists
	c.exists = false
	var out []string
	for _, t := range l {
		k, f := kindOf(t)
		if k != "ResTarget" {
			return nil, c.unsupported(f, "target "+k, "")
		}
		if ck, cf := kindOf(f["val"]); ck == "ColumnRef" {
			if vs, ok, err := c.star(cf); err != nil {
				return nil, err
			} else if ok {
				for _, v := range vs {
					out = append(out, v.code)
					if record {
						c.items = append(c.items, v)
					}
				}
				continue
			}
		}
		v, err := c.expr(f["val"])
		if err != nil {
			return nil, err
		}
		if v.lit != nil && existsBody && len(c.sc.b) > 0 && len(c.sc.b[0].tbl.Columns) > 0 {
			v = colVal(c.sc.b[0], c.sc.b[0].tbl.Columns[0])
		}
		if v.lit != nil {
			return nil, c.unsupported(f, "literal in a select list", "use qlx.Val(x)")
		}
		if v.pred {
			return nil, c.unsupported(f, "condition in a select list", "")
		}
		alias := str(f, "name")
		code := v.code
		if record {
			c.items = append(c.items, v)
			if alias != "" {
				c.aliases[alias] = v
			}
		}
		if alias != "" && (v.col == nil || v.col.SQL != alias) {
			code += fmt.Sprintf(".As(%q)", alias)
		}
		out = append(out, code)
	}
	return out, nil
}

func (c *conv) intLit(n any, what string) (string, error) {
	k, f := kindOf(n)
	if k == "A_Const" {
		l, err := c.constant(f)
		if err == nil && l.kind == "int" {
			return l.text, nil
		}
	}
	return "", c.unsupported(f, what+" that is not an integer literal", "bind it with qlx.NewParam")
}

func (c *conv) selectStmt(f map[string]any) (string, error) {
	switch {
	case f["withClause"] != nil:
		return "", c.unsupported(f, "WITH", "use qlx.NewCTE")
	case str(f, "op") != "" && str(f, "op") != "SETOP_NONE":
		return "", c.unsupported(f, "set operation "+str(f, "op"), "use qlx.Union / Intersect / Except")
	case len(list(f, "windowClause")) > 0:
		return "", c.unsupported(f, "WINDOW", "")
	case len(list(f, "valuesLists")) > 0:
		return "", c.unsupported(f, "VALUES as a query", "use qlx.Values1..N")
	case f["intoClause"] != nil:
		return "", c.unsupported(f, "SELECT INTO", "")
	}
	if c.sc == nil {
		c.sc = &scope{}
	}
	c.aliases, c.items = map[string]val{}, nil
	q := c.o.Q

	var first string
	var joins []string
	for i, n := range list(f, "fromClause") {
		fi, js, err := c.fromTree(n)
		if err != nil {
			return "", err
		}
		if i == 0 {
			first, joins = fi, js
			continue
		}
		if len(js) > 0 {
			return "", c.unsupported(f, "comma-separated joins", "")
		}
		joins = append(joins, "CrossJoin("+fi+")")
	}

	sel, err := c.targets(list(f, "targetList"), true)
	if err != nil {
		return "", err
	}
	var calls []string
	if d := list(f, "distinctClause"); len(d) > 0 {
		var ons []string
		for _, n := range d {
			if k, _ := kindOf(n); k == "" {
				continue
			}
			v, err := c.expr(n)
			if err != nil {
				return "", err
			}
			ons = append(ons, v.code)
		}
		if len(ons) == 0 {
			calls = append(calls, "Distinct()")
		} else {
			calls = append(calls, "DistinctOn("+strings.Join(ons, ", ")+")")
		}
	}
	if first != "" {
		calls = append(calls, "From("+first+")")
	}
	calls = append(calls, joins...)
	if f["whereClause"] != nil {
		ps, err := c.predList(f["whereClause"])
		if err != nil {
			return "", err
		}
		calls = append(calls, "Where("+strings.Join(ps, ", ")+")")
	}
	if g := list(f, "groupClause"); len(g) > 0 {
		var gs []string
		for _, n := range g {
			v, err := c.expr(n)
			if err != nil {
				return "", err
			}
			if v.code == "" {
				return "", c.unsupported(nil, "GROUP BY of a literal or ordinal", "")
			}
			gs = append(gs, v.code)
		}
		calls = append(calls, "GroupBy("+strings.Join(gs, ", ")+")")
	}
	if f["havingClause"] != nil {
		ps, err := c.predList(f["havingClause"])
		if err != nil {
			return "", err
		}
		calls = append(calls, "Having("+strings.Join(ps, ", ")+")")
	}
	if so := list(f, "sortClause"); len(so) > 0 {
		var ts []string
		for _, n := range so {
			t, err := c.sortTerm(n)
			if err != nil {
				return "", err
			}
			ts = append(ts, t)
		}
		calls = append(calls, "OrderBy("+strings.Join(ts, ", ")+")")
	}
	ties := str(f, "limitOption") == "LIMIT_OPTION_WITH_TIES"
	if f["limitCount"] != nil {
		if k, cf := kindOf(f["limitCount"]); k == "A_Const" {
			if b, _ := cf["isnull"].(bool); b {
				goto offset
			}
		}
		n, err := c.intLit(f["limitCount"], "LIMIT")
		if err != nil {
			return "", err
		}
		if ties {
			calls = append(calls, "FetchFirst("+n+")", "WithTies()")
		} else {
			calls = append(calls, "Limit("+n+")")
		}
	}
offset:
	if f["limitOffset"] != nil {
		n, err := c.intLit(f["limitOffset"], "OFFSET")
		if err != nil {
			return "", err
		}
		calls = append(calls, "Offset("+n+")")
	}
	if lc := list(f, "lockingClause"); len(lc) > 0 {
		_, lf := kindOf(lc[0])
		m, ok := map[string]string{"LCS_FORUPDATE": "ForUpdate", "LCS_FORNOKEYUPDATE": "ForNoKeyUpdate",
			"LCS_FORSHARE": "ForShare", "LCS_FORKEYSHARE": "ForKeyShare"}[str(lf, "strength")]
		if !ok || len(lc) > 1 {
			return "", c.unsupported(lf, "locking clause", "")
		}
		calls = append(calls, m+"()")
		var of []string
		for _, r := range list(lf, "lockedRels") {
			_, rf := kindOf(r)
			name := str(rf, "relname")
			found := false
			for _, b := range c.sc.b {
				if b.qual == name || b.tbl.SQL == name {
					of = append(of, b.code)
					found = true
					break
				}
			}
			if !found {
				return "", c.unsupported(rf, "FOR UPDATE OF "+name, "")
			}
		}
		if len(of) > 0 {
			calls = append(calls, "Of("+strings.Join(of, ", ")+")")
		}
		switch str(lf, "waitPolicy") {
		case "LockWaitSkip":
			calls = append(calls, "SkipLocked()")
		case "LockWaitError":
			calls = append(calls, "NoWait()")
		}
	}
	return chain(q+".Select("+strings.Join(sel, ", ")+")", calls), nil
}

func (c *conv) sortTerm(n any) (string, error) {
	k, f := kindOf(n)
	if k != "SortBy" {
		return "", c.unsupported(f, "ORDER BY item "+k, "")
	}
	var v val
	var err error
	nk, nf := kindOf(f["node"])
	switch {
	case nk == "ColumnRef" && len(list(nf, "fields")) == 1 && c.aliases[names(list(nf, "fields"))[0]].code != "":
		v = c.aliases[names(list(nf, "fields"))[0]]
	case nk == "A_Const":
		var l *litv
		if l, err = c.constant(nf); err != nil {
			return "", err
		}
		i, _ := strconv.Atoi(l.text)
		if l.kind != "int" || i < 1 || i > len(c.items) {
			return "", c.unsupported(nf, "ORDER BY position", "")
		}
		v = c.items[i-1]
	default:
		if v, err = c.expr(f["node"]); err != nil {
			return "", err
		}
	}
	if v.code == "" || v.pred {
		return "", c.unsupported(nf, "ORDER BY expression", "")
	}
	switch str(f, "sortby_dir") {
	case "SORTBY_USING":
		return "", c.unsupported(f, "ORDER BY ... USING", "")
	case "SORTBY_DESC":
		v.code += ".Desc()"
	default:
		v.code += ".Asc()"
	}
	switch str(f, "sortby_nulls") {
	case "SORTBY_NULLS_FIRST":
		v.code += ".NullsFirst()"
	case "SORTBY_NULLS_LAST":
		v.code += ".NullsLast()"
	}
	return v.code, nil
}

// ---- DML ----

func (c *conv) target(f map[string]any) (binding, error) {
	if c.sc == nil {
		c.sc = &scope{}
	}
	rf, ok := embedded(f["relation"], "RangeVar")
	if !ok {
		return binding{}, c.unsupported(f, "target relation", "")
	}
	return c.bindRange(rf)
}

func (c *conv) targetCol(b binding, name string, at map[string]any) (codegen.ColumnName, error) {
	col, ok := b.tbl.Column(name)
	if !ok {
		return col, c.unsupported(at, "unknown column "+name, "not in schema.lock")
	}
	return col, nil
}

// assign renders `Tbl.Col.To(...)` for one SET / VALUES entry.
func (c *conv) assign(b binding, col codegen.ColumnName, n any) (string, error) {
	target := b.code + "." + col.Field
	k, f := kindOf(n)
	if k == "SetToDefault" {
		return target + ".ToDefault()", nil
	}
	v, err := c.expr(n)
	if err != nil {
		return "", err
	}
	if v.lit != nil {
		if v.lit.kind == "null" {
			in, nullable := inner(col.GoType)
			if !nullable {
				return "", c.unsupported(f, "NULL for the NOT NULL column "+col.SQL, "")
			}
			c.useQt()
			return fmt.Sprintf("%s.To(qt.None[%s]())", target, in), nil
		}
		code, err := c.litCode(v.lit, col.GoType, f)
		if err != nil {
			return "", err
		}
		return target + ".To(" + code + ")", nil
	}
	if v.pred {
		return "", c.unsupported(f, "condition as a value", "")
	}
	return target + ".ToX(" + v.code + ")", nil
}

func (c *conv) setList(b binding, l []any) ([]string, error) {
	var out []string
	for _, t := range l {
		_, f := kindOf(t)
		col, err := c.targetCol(b, str(f, "name"), f)
		if err != nil {
			return nil, err
		}
		a, err := c.assign(b, col, f["val"])
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func (c *conv) whereOrAll(f map[string]any, kind string) ([]string, error) {
	if f["whereClause"] == nil {
		c.warn("%s without WHERE: emitted .All(), which is how QLX spells \"every row\"", kind)
		return []string{"All()"}, nil
	}
	ps, err := c.predList(f["whereClause"])
	if err != nil {
		return nil, err
	}
	return []string{"Where(" + strings.Join(ps, ", ") + ")"}, nil
}

func (c *conv) returning(f map[string]any) ([]string, error) {
	rl := list(f, "returningList")
	if len(rl) == 0 {
		return nil, nil
	}
	ts, err := c.targets(rl, false)
	if err != nil {
		return nil, err
	}
	return []string{"Returning(" + strings.Join(ts, ", ") + ")"}, nil
}

func (c *conv) insertStmt(f map[string]any) (string, error) {
	if f["withClause"] != nil {
		return "", c.unsupported(f, "WITH on INSERT", "")
	}
	if str(f, "override") != "" && str(f, "override") != "OVERRIDING_NOT_SET" {
		return "", c.unsupported(f, "OVERRIDING", "use OverridingSystemValue() / OverridingUserValue()")
	}
	b, err := c.target(f)
	if err != nil {
		return "", err
	}
	var cols []codegen.ColumnName
	for _, t := range list(f, "cols") {
		_, tf := kindOf(t)
		col, err := c.targetCol(b, str(tf, "name"), tf)
		if err != nil {
			return "", err
		}
		cols = append(cols, col)
	}
	var calls []string
	sk, sf := kindOf(f["selectStmt"])
	switch {
	case sk == "":
		calls = append(calls, "DefaultValues()")
	case sk != "SelectStmt":
		return "", c.unsupported(f, "INSERT source "+sk, "")
	case len(list(sf, "valuesLists")) > 0:
		if len(cols) == 0 {
			return "", c.unsupported(f, "INSERT without a column list", "list the columns: positional inserts break when a column is added")
		}
		for _, row := range list(sf, "valuesLists") {
			vs := items(row)
			if len(vs) != len(cols) {
				return "", c.unsupported(sf, "VALUES row with "+strconv.Itoa(len(vs))+" values for "+strconv.Itoa(len(cols))+" columns", "")
			}
			var as []string
			for i, v := range vs {
				a, err := c.assign(b, cols[i], v)
				if err != nil {
					return "", err
				}
				as = append(as, a)
			}
			calls = append(calls, "Row("+strings.Join(as, ", ")+")")
		}
	default:
		if len(cols) == 0 {
			return "", c.unsupported(f, "INSERT ... SELECT without a column list", "")
		}
		var cs []string
		for _, col := range cols {
			cs = append(cs, b.code+"."+col.Field)
		}
		outer := c.sc
		c.sc = nil
		sub, err := c.selectStmt(sf)
		c.sc = outer
		if err != nil {
			return "", err
		}
		calls = append(calls, "Columns("+strings.Join(cs, ", ")+")", "FromQuery("+sub+")")
	}
	if oc, ok := f["onConflictClause"].(map[string]any); ok {
		cc, err := c.onConflict(b, oc)
		if err != nil {
			return "", err
		}
		calls = append(calls, cc...)
	}
	r, err := c.returning(f)
	if err != nil {
		return "", err
	}
	calls = append(calls, r...)
	return chain(c.o.Q+".InsertInto("+b.code+")", calls), nil
}

func (c *conv) onConflict(b binding, oc map[string]any) ([]string, error) {
	var cols []string
	if inf, ok := oc["infer"].(map[string]any); ok {
		if str(inf, "conname") != "" {
			return nil, c.unsupported(oc, "ON CONFLICT ON CONSTRAINT", "use OnConflictConstraint with the constraint symbol")
		}
		if inf["whereClause"] != nil {
			return nil, c.unsupported(oc, "partial-index conflict target", "")
		}
		for _, e := range list(inf, "indexElems") {
			_, ef := kindOf(e)
			if str(ef, "name") == "" {
				return nil, c.unsupported(oc, "expression in conflict target", "")
			}
			col, err := c.targetCol(b, str(ef, "name"), oc)
			if err != nil {
				return nil, err
			}
			cols = append(cols, b.code+"."+col.Field)
		}
	}
	start := "OnConflict(" + strings.Join(cols, ", ") + ")"
	switch str(oc, "action") {
	case "ONCONFLICT_NOTHING":
		return []string{start, "DoNothing()"}, nil
	case "ONCONFLICT_UPDATE":
		// EXCLUDED is visible while the SET list and WHERE are converted.
		c.sc.b = append(c.sc.b, binding{qual: "excluded", code: b.code, tbl: b.tbl, excluded: true})
		defer func() { c.sc.b = c.sc.b[:len(c.sc.b)-1] }()
		sets, err := c.setList(b, list(oc, "targetList"))
		if err != nil {
			return nil, err
		}
		if oc["whereClause"] != nil {
			ps, err := c.predList(oc["whereClause"])
			if err != nil {
				return nil, err
			}
			p := ps[0]
			if len(ps) > 1 {
				p = c.o.Q + ".And(" + strings.Join(ps, ", ") + ")"
			}
			return []string{start, "DoUpdateWhere(" + p + ", " + strings.Join(sets, ", ") + ")"}, nil
		}
		return []string{start, "DoUpdate(" + strings.Join(sets, ", ") + ")"}, nil
	}
	return nil, c.unsupported(oc, "ON CONFLICT action "+str(oc, "action"), "")
}

func (c *conv) updateStmt(f map[string]any) (string, error) {
	if f["withClause"] != nil {
		return "", c.unsupported(f, "WITH on UPDATE", "")
	}
	b, err := c.target(f)
	if err != nil {
		return "", err
	}
	sets, err := c.setList(b, list(f, "targetList"))
	if err != nil {
		return "", err
	}
	calls := []string{"Set(" + strings.Join(sets, ", ") + ")"}
	if fc := list(f, "fromClause"); len(fc) > 0 {
		var fs []string
		for _, n := range fc {
			fi, js, err := c.fromTree(n)
			if err != nil || len(js) > 0 {
				if err == nil {
					err = c.unsupported(f, "joins in UPDATE ... FROM", "list the tables and put the condition in WHERE")
				}
				return "", err
			}
			fs = append(fs, fi)
		}
		calls = append(calls, "From("+strings.Join(fs, ", ")+")")
	}
	w, err := c.whereOrAll(f, "UPDATE")
	if err != nil {
		return "", err
	}
	r, err := c.returning(f)
	if err != nil {
		return "", err
	}
	calls = append(append(calls, w...), r...)
	return chain(c.o.Q+".Update("+b.code+")", calls), nil
}

func (c *conv) deleteStmt(f map[string]any) (string, error) {
	if f["withClause"] != nil {
		return "", c.unsupported(f, "WITH on DELETE", "")
	}
	b, err := c.target(f)
	if err != nil {
		return "", err
	}
	var calls []string
	if uc := list(f, "usingClause"); len(uc) > 0 {
		var us []string
		for _, n := range uc {
			fi, js, err := c.fromTree(n)
			if err != nil || len(js) > 0 {
				if err == nil {
					err = c.unsupported(f, "joins in DELETE ... USING", "list the tables and put the condition in WHERE")
				}
				return "", err
			}
			us = append(us, fi)
		}
		calls = append(calls, "Using("+strings.Join(us, ", ")+")")
	}
	w, err := c.whereOrAll(f, "DELETE")
	if err != nil {
		return "", err
	}
	r, err := c.returning(f)
	if err != nil {
		return "", err
	}
	calls = append(append(calls, w...), r...)
	return chain(c.o.Q+".DeleteFrom("+b.code+")", calls), nil
}
