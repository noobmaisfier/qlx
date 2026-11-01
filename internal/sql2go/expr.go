package sql2go

import (
	"fmt"
	"strings"
)

var cmpMethod = map[string]string{"=": "Eq", "<>": "Ne", "!=": "Ne", "<": "Lt", "<=": "Lte", ">": "Gt", ">=": "Gte"}
var flipOp = map[string]string{"Eq": "Eq", "Ne": "Ne", "Lt": "Gt", "Gt": "Lt", "Lte": "Gte", "Gte": "Lte"}
var arithMethod = map[string]string{"+": "Add", "-": "Sub", "*": "Mul", "/": "Div", "%": "Mod"}

func ordered(fam string) bool { return fam == "num" || fam == "text" || fam == "time" }

func (c *conv) expr(n any) (val, error) {
	k, f := kindOf(n)
	switch k {
	case "ColumnRef":
		fs := names(list(f, "fields"))
		for _, x := range fs {
			if x == "*" {
				return val{}, c.unsupported(f, "* in an expression", "")
			}
		}
		return c.resolve(c.sc, fs, f)
	case "A_Const":
		l, err := c.constant(f)
		if err != nil {
			return val{}, err
		}
		return val{lit: l}, nil
	case "A_Expr":
		return c.aExpr(f)
	case "BoolExpr":
		return c.boolExpr(f)
	case "NullTest":
		arg, err := c.expr(f["arg"])
		if err != nil {
			return val{}, err
		}
		if arg.lit != nil {
			return val{}, c.unsupported(f, "NULL test of a literal", "")
		}
		m := "IsNull"
		if str(f, "nulltesttype") == "IS_NOT_NULL" {
			m = "IsNotNull"
		}
		return val{code: arg.code + "." + m + "()", pred: true}, nil
	case "BooleanTest":
		arg, err := c.expr(f["arg"])
		if err != nil {
			return val{}, err
		}
		if str(f, "booltesttype") == "IS_TRUE" && arg.fam == "bool" && arg.col != nil && !arg.col.Nullable {
			return val{code: arg.code + ".IsTrue()", pred: true}, nil
		}
		return val{}, c.unsupported(f, "boolean test "+str(f, "booltesttype"), "only IS TRUE on a NOT NULL boolean column")
	case "FuncCall":
		return c.funcCall(f)
	case "CoalesceExpr":
		return c.coalesce(f)
	case "SubLink":
		if str(f, "subLinkType") != "EXISTS_SUBLINK" {
			return val{}, c.unsupported(f, "subquery expression "+str(f, "subLinkType"), "only EXISTS is converted; use qlx.Exists / InSelect by hand")
		}
		sk, sf := kindOf(f["subselect"])
		if sk != "SelectStmt" {
			return val{}, c.unsupported(f, "subquery", "")
		}
		code, err := c.existsBody(sf)
		if err != nil {
			return val{}, err
		}
		return val{code: c.o.Q + ".Exists(" + code + ")", pred: true}, nil
	case "SQLValueFunction":
		if str(f, "op") == "SVFOP_CURRENT_TIMESTAMP" {
			return val{code: c.o.Q + ".Now()", goT: "time.Time", fam: "time"}, nil
		}
		return val{}, c.unsupported(f, "SQL value function "+str(f, "op"), "")
	case "ParamRef":
		return val{}, c.unsupported(f, "parameter $n", "declare it with qlx.NewParam[T]() and use the X variants")
	case "TypeCast":
		return val{}, c.unsupported(f, "type cast", "use qlx.Cast(x, <type symbol>)")
	case "CaseExpr":
		return val{}, c.unsupported(f, "CASE", "use qlx.Case[T]()")
	}
	return val{}, c.unsupported(f, "expression "+k, "")
}

func (c *conv) nested(sf map[string]any) (string, error) {
	outer, aliases, items := c.sc, c.aliases, c.items
	c.sc = &scope{parent: outer}
	defer func() { c.sc, c.aliases, c.items = outer, aliases, items }()
	return c.selectStmt(sf)
}

// existsBody converts the SELECT inside EXISTS. Its select list is irrelevant
// to the result (EXISTS (SELECT 1 ...) is the usual spelling), but QLX
// still needs a selectable item, so a literal is replaced by the first column
// of the first table.
func (c *conv) existsBody(sf map[string]any) (string, error) {
	c.exists = true
	return c.nested(sf)
}

func (c *conv) asPred(v val, at map[string]any) (string, error) {
	if v.pred {
		return v.code, nil
	}
	if v.fam == "bool" && v.col != nil {
		if !v.col.Nullable {
			return v.code + ".IsTrue()", nil
		}
		c.useQt()
		return v.code + ".Eq(qt.Some[bool](true))", nil
	}
	return "", c.unsupported(at, "expression used as a condition", "it is not a boolean column or comparison")
}

// predList flattens a top-level AND into separate Where/Having arguments.
func (c *conv) predList(n any) ([]string, error) {
	k, f := kindOf(n)
	if k == "BoolExpr" && str(f, "boolop") == "AND_EXPR" {
		var out []string
		for _, a := range list(f, "args") {
			p, err := c.predList(a)
			if err != nil {
				return nil, err
			}
			out = append(out, p...)
		}
		return out, nil
	}
	v, err := c.expr(n)
	if err != nil {
		return nil, err
	}
	p, err := c.asPred(v, f)
	if err != nil {
		return nil, err
	}
	return []string{p}, nil
}

func (c *conv) boolExpr(f map[string]any) (val, error) {
	var ps []string
	for _, a := range list(f, "args") {
		v, err := c.expr(a)
		if err != nil {
			return val{}, err
		}
		_, af := kindOf(a)
		p, err := c.asPred(v, af)
		if err != nil {
			return val{}, err
		}
		ps = append(ps, p)
	}
	q := c.o.Q
	switch str(f, "boolop") {
	case "AND_EXPR":
		return val{code: q + ".And(" + strings.Join(ps, ", ") + ")", pred: true}, nil
	case "OR_EXPR":
		return val{code: q + ".Or(" + strings.Join(ps, ", ") + ")", pred: true}, nil
	case "NOT_EXPR":
		return val{code: q + ".Not(" + ps[0] + ")", pred: true}, nil
	}
	return val{}, c.unsupported(f, "boolean operator "+str(f, "boolop"), "")
}

func (c *conv) aExpr(f map[string]any) (val, error) {
	op := ""
	if ns := names(list(f, "name")); len(ns) > 0 {
		op = ns[len(ns)-1]
	}
	q := c.o.Q
	switch str(f, "kind") {
	case "AEXPR_OP":
		if f["lexpr"] == nil {
			return c.unary(op, f)
		}
		l, err := c.expr(f["lexpr"])
		if err != nil {
			return val{}, err
		}
		r, err := c.expr(f["rexpr"])
		if err != nil {
			return val{}, err
		}
		if m, ok := cmpMethod[op]; ok {
			return c.compare(m, l, r, f)
		}
		if op == "||" {
			return c.concat(l, r, f)
		}
		if m, ok := arithMethod[op]; ok {
			return c.arith(m, op, l, r, f)
		}
		return val{}, c.unsupported(f, "operator "+op, "use qlx.BinaryOp / the pgcat operator wrappers")
	case "AEXPR_IN":
		l, err := c.expr(f["lexpr"])
		if err != nil {
			return val{}, err
		}
		if l.col == nil && !l.agg {
			return val{}, c.unsupported(f, "IN on a computed expression", "")
		}
		var vs []string
		for _, it := range items(f["rexpr"]) {
			v, err := c.expr(it)
			if err != nil {
				return val{}, err
			}
			if v.lit == nil {
				return val{}, c.unsupported(f, "IN with non-literal elements", "")
			}
			code, err := c.litCode(v.lit, l.goT, f)
			if err != nil {
				return val{}, err
			}
			vs = append(vs, code)
		}
		code := l.code + ".In(" + strings.Join(vs, ", ") + ")"
		if op == "<>" {
			code = q + ".Not(" + code + ")"
		}
		return val{code: code, pred: true}, nil
	case "AEXPR_LIKE", "AEXPR_ILIKE":
		l, err := c.expr(f["lexpr"])
		if err != nil {
			return val{}, err
		}
		r, err := c.expr(f["rexpr"])
		if err != nil {
			return val{}, err
		}
		if l.fam != "text" || r.lit == nil || r.lit.kind != "str" {
			return val{}, c.unsupported(f, "LIKE needs a text column and a string literal", "")
		}
		m := "Like"
		if str(f, "kind") == "AEXPR_ILIKE" {
			m = "ILike"
		}
		code := fmt.Sprintf("%s.%s(%q)", l.code, m, r.lit.text)
		if strings.HasPrefix(op, "!") {
			code = q + ".Not(" + code + ")"
		}
		return val{code: code, pred: true}, nil
	case "AEXPR_BETWEEN", "AEXPR_NOT_BETWEEN":
		l, err := c.expr(f["lexpr"])
		if err != nil {
			return val{}, err
		}
		it := items(f["rexpr"])
		if len(it) != 2 || !ordered(l.fam) {
			return val{}, c.unsupported(f, "BETWEEN", "needs a numeric, text or time column and two literals")
		}
		var b [2]string
		for i := range b {
			v, err := c.expr(it[i])
			if err != nil {
				return val{}, err
			}
			if v.lit == nil {
				return val{}, c.unsupported(f, "BETWEEN with non-literal bounds", "")
			}
			if b[i], err = c.litCode(v.lit, l.goT, f); err != nil {
				return val{}, err
			}
		}
		var code string
		if l.fam == "text" {
			code = fmt.Sprintf("%s.And(%s.Gte(%s), %s.Lte(%s))", q, l.code, b[0], l.code, b[1])
		} else {
			code = fmt.Sprintf("%s.Between(%s, %s)", l.code, b[0], b[1])
		}
		if str(f, "kind") == "AEXPR_NOT_BETWEEN" {
			code = q + ".Not(" + code + ")"
		}
		return val{code: code, pred: true}, nil
	}
	return val{}, c.unsupported(f, "operator form "+str(f, "kind"), "")
}

func (c *conv) unary(op string, f map[string]any) (val, error) {
	r, err := c.expr(f["rexpr"])
	if err != nil {
		return val{}, err
	}
	if op != "-" {
		return val{}, c.unsupported(f, "prefix operator "+op, "")
	}
	if r.lit != nil && (r.lit.kind == "int" || r.lit.kind == "float") {
		return val{lit: &litv{kind: r.lit.kind, text: "-" + r.lit.text}}, nil
	}
	if r.fam == "num" {
		r.code += ".Neg()"
		return r, nil
	}
	return val{}, c.unsupported(f, "unary minus", "")
}

func (c *conv) compare(m string, l, r val, at map[string]any) (val, error) {
	if l.lit != nil && r.lit != nil {
		return val{}, c.unsupported(at, "comparison of two literals", "")
	}
	if l.lit != nil {
		l, r, m = r, l, flipOp[m]
	}
	if l.lit == nil && l.code == "" {
		return val{}, c.unsupported(at, "comparison operand", "")
	}
	if r.lit != nil {
		if r.lit.kind == "null" {
			return val{}, c.unsupported(at, "comparison with NULL", "it is never true; use IS NULL / IS NOT NULL")
		}
		if m != "Eq" && m != "Ne" && !ordered(l.fam) {
			return val{}, c.unsupported(at, "ordering comparison on a column of type "+l.goT, "")
		}
		code, err := c.litCode(r.lit, l.goT, at)
		if err != nil {
			return val{}, err
		}
		return val{code: l.code + "." + m + "(" + code + ")", pred: true}, nil
	}
	switch m {
	case "Eq", "Ne":
		return val{code: l.code + "." + m + "X(" + r.code + ")", pred: true}, nil
	case "Gt", "Lt":
		if l.fam == "num" {
			return val{code: l.code + "." + m + "X(" + r.code + ")", pred: true}, nil
		}
	case "Gte", "Lte":
		// a >= b is NOT (a < b) in three-valued logic as well; there is no
		// GteX/LteX on the typed expressions, so say it that way.
		if l.fam == "num" {
			opp := map[string]string{"Gte": "Lt", "Lte": "Gt"}[m]
			return val{code: c.o.Q + ".Not(" + l.code + "." + opp + "X(" + r.code + "))", pred: true}, nil
		}
	}
	return val{}, c.unsupported(at, "column-to-column ordering on a non-numeric column", "use qlx.BinaryOp")
}

func (c *conv) arith(m, op string, l, r val, at map[string]any) (val, error) {
	if l.lit != nil && r.lit == nil && (op == "+" || op == "*") {
		l, r = r, l
	}
	if l.lit != nil || l.fam != "num" {
		return val{}, c.unsupported(at, "arithmetic needs a numeric column on the left", "")
	}
	out := val{goT: l.goT, fam: "num"}
	if r.lit != nil {
		code, err := c.litCode(r.lit, l.goT, at)
		if err != nil {
			return val{}, err
		}
		out.code = l.code + "." + m + "(" + code + ")"
		return out, nil
	}
	if op != "+" && op != "-" {
		return val{}, c.unsupported(at, "column "+op+" column", "only + and - have typed column-to-column forms; use qlx.BinaryOp")
	}
	out.code = l.code + "." + m + "X(" + r.code + ")"
	return out, nil
}

func (c *conv) concat(l, r val, at map[string]any) (val, error) {
	if l.fam != "text" || l.lit != nil {
		return val{}, c.unsupported(at, "|| needs a text column on the left", "")
	}
	out := val{goT: l.goT, fam: "text"}
	if r.lit != nil && r.lit.kind == "str" {
		out.code = fmt.Sprintf("%s.Concat(%q)", l.code, r.lit.text)
	} else if r.lit == nil && r.fam == "text" {
		out.code = l.code + ".ConcatX(" + r.code + ")"
	} else {
		return val{}, c.unsupported(at, "|| operand", "")
	}
	return out, nil
}

func (c *conv) coalesce(f map[string]any) (val, error) {
	args := list(f, "args")
	if len(args) != 2 {
		return val{}, c.unsupported(f, "COALESCE with "+fmt.Sprint(len(args))+" arguments", "only two are converted")
	}
	a, err := c.expr(args[0])
	if err != nil {
		return val{}, err
	}
	b, err := c.expr(args[1])
	if err != nil {
		return val{}, err
	}
	in, nullable := inner(a.goT)
	if a.col == nil || !nullable {
		return val{}, c.unsupported(f, "COALESCE of a non-nullable or computed value", "")
	}
	q := c.o.Q
	out := val{goT: in, fam: a.fam}
	switch {
	case b.lit != nil:
		code, err := c.litCode(b.lit, in, f)
		if err != nil {
			return val{}, err
		}
		out.code = fmt.Sprintf("%s.Coalesce(%s, %s)", q, a.code, code)
	case b.col != nil && b.col.Nullable:
		out.code = fmt.Sprintf("%s.CoalesceOp(%s, %s)", q, a.code, b.code)
		out.goT = a.goT
	case b.col != nil:
		out.code = fmt.Sprintf("%s.CoalesceOpDefault(%s, %s)", q, a.code, b.code)
	default:
		return val{}, c.unsupported(f, "COALESCE fallback", "")
	}
	return out, nil
}

var pgfnHint = "use the typed wrapper from the pg facade or a pgcat area package (names carry the argument type, e.g. num.AbsInt8)"

func (c *conv) funcCall(f map[string]any) (val, error) {
	fn := names(list(f, "funcname"))
	if len(fn) == 0 {
		return val{}, c.unsupported(f, "function call", "")
	}
	name := strings.ToLower(fn[len(fn)-1])
	if f["over"] != nil {
		return val{}, c.unsupported(f, "window function "+name, "use qlx.Over / NewWindow")
	}
	if f["agg_filter"] != nil || len(list(f, "agg_order")) > 0 {
		return val{}, c.unsupported(f, "aggregate FILTER / ORDER BY", "")
	}
	q := c.o.Q
	if star, _ := f["agg_star"].(bool); star {
		if name != "count" {
			return val{}, c.unsupported(f, name+"(*)", "")
		}
		return val{code: q + ".CountAll()", goT: "int64", fam: "num", agg: true}, nil
	}
	args := list(f, "args")
	if name == "now" && len(args) == 0 {
		return val{code: q + ".Now()", goT: "time.Time", fam: "time"}, nil
	}
	if len(args) != 1 {
		return val{}, c.unsupported(f, "function "+name, pgfnHint)
	}
	a, err := c.expr(args[0])
	if err != nil {
		return val{}, err
	}
	if a.lit != nil || a.code == "" {
		return val{}, c.unsupported(f, name+" of a literal", "")
	}
	distinct, _ := f["agg_distinct"].(bool)
	in, _ := inner(a.goT)
	agg := func(fn, goT, fam string) (val, error) {
		if distinct {
			return val{}, c.unsupported(f, name+"(DISTINCT ...)", "")
		}
		return val{code: q + "." + fn + "(" + a.code + ")", goT: goT, fam: fam, agg: true}, nil
	}
	switch name {
	case "count":
		if distinct {
			return val{code: q + ".CountDistinct(" + a.code + ")", goT: "int64", fam: "num", agg: true}, nil
		}
		return agg("Count", "int64", "num")
	case "sum", "avg":
		switch in {
		case "int16", "int32":
			if name == "sum" {
				return agg("Sum", "qt.Null[int64]", "num")
			}
			return agg("Avg", "qt.Null[qt.Numeric]", "num")
		case "int64":
			if name == "sum" {
				return agg("SumBig", "qt.Null[qt.Numeric]", "num")
			}
			return agg("Avg", "qt.Null[qt.Numeric]", "num")
		case "float32", "float64":
			if name == "sum" {
				return agg("SumFloat", "qt.Null[float64]", "num")
			}
			return agg("AvgFloat", "qt.Null[float64]", "num")
		}
		return val{}, c.unsupported(f, name+" of a "+a.goT+" column", "use qlx.Sum* / Avg* by hand")
	case "min", "max":
		fnm := map[string]string{"min": "Min", "max": "Max"}[name]
		return agg(fnm, "qt.Null["+in+"]", a.fam)
	case "lower", "upper":
		if a.fam != "text" {
			return val{}, c.unsupported(f, name+" of a non-text value", "")
		}
		m := map[string]string{"lower": "Lower", "upper": "Upper"}[name]
		return val{code: a.code + "." + m + "()", goT: a.goT, fam: "text"}, nil
	case "length":
		if a.fam == "text" {
			return val{code: a.code + ".Length()", goT: "qt.Null[int32]", fam: "num"}, nil
		}
	}
	return val{}, c.unsupported(f, "function "+name, pgfnHint)
}
