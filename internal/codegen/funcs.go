package codegen

import (
	"fmt"
	"strings"

	"github.com/noobmaisfier/qlx/internal/catalog"
)

// emitFuncs writes a typed wrapper for each function and procedure it can type
// safely, and reports (never guesses) the ones it cannot:
//
//   - a plain function becomes qlx.Expr[R], a procedure qlx.Command;
//   - a VARIADIC function takes the variadic arguments as a Go variadic
//     parameter, typed by the element type (VARIADIC "any" is skipped);
//   - a function with default arguments gets two wrappers: Name takes the
//     arguments that have no default, NameAll takes every argument (PostgreSQL
//     fills the trailing defaults of a positional call);
//   - a set-returning function with OUT/TABLE columns, or returning a set of a
//     scalar type, becomes a qlx.TableFunc with a NameColumns struct naming its
//     columns (pass NameCols.Fields() to As and use qlx.Ref for a column);
//   - INOUT parameters, OUT parameters of a non-set function, and a set of a
//     composite or record type without OUT columns are skipped.
func emitFuncs(f *file, m *mapper, nm *namer, s *catalog.Schema, res *Result) {
	for _, fn := range s.Funcs {
		qual := fn.Schema + "." + fn.Name
		ins, outs, ok := funcArgs(fn)
		if !ok {
			res.Skipped = append(res.Skipped, qual+": INOUT parameters")
			continue
		}
		variadic := len(ins) > 0 && ins[len(ins)-1].mode == "v"
		if variadic && fn.VariadicType == anyOID {
			res.Skipped = append(res.Skipped, qual+": VARIADIC \"any\"")
			continue
		}
		if variadic && fn.Defaults > 0 {
			res.Skipped = append(res.Skipped, qual+": variadic with default arguments")
			continue
		}
		if fn.Defaults > len(ins) {
			res.Skipped = append(res.Skipped, qual+": default arguments do not match the parameters")
			continue
		}

		var cols []fnArg // output columns of a set-returning function
		switch {
		case fn.Kind == "p":
			if len(outs) > 0 {
				res.Skipped = append(res.Skipped, qual+": OUT parameters")
				continue
			}
		case fn.ReturnsSet:
			switch {
			case len(outs) > 0:
				cols = outs
			case scalarType(s, fn.ReturnType):
				cols = []fnArg{{name: fn.Name, oid: fn.ReturnType}}
			default:
				res.Skipped = append(res.Skipped, qual+": returns a set of a composite or record type (declare it with OUT or TABLE columns)")
				continue
			}
		case len(outs) > 0:
			res.Skipped = append(res.Skipped, qual+": OUT parameters")
			continue
		}

		f.use("github.com/noobmaisfier/qlx")
		suffix := ""
		for _, at := range fn.ArgTypes {
			if tt, ok := s.TypeByOID(at); ok {
				suffix += camel(tt.Name)
			}
		}
		goName := nm.take(nm.local(fn.Schema, fn.Name), suffix)

		w := &fnWriter{f: f, m: m, s: s, fn: fn, qual: qual, ins: ins, variadic: variadic}
		if len(cols) > 0 {
			w.colsType = nm.take(goName+"Columns", "")
			w.colsVar = nm.take(goName+"Cols", "")
			w.cols = cols
		}
		required := len(ins) - fn.Defaults
		w.emit(goName, required, false)
		if fn.Defaults > 0 {
			w.emit(nm.take(goName+"All", ""), len(ins), true)
		}
		if len(cols) > 0 {
			w.emitColumns(goName)
		}
	}
}

// anyOID is the pseudo-type "any", whose VARIADIC form cannot be typed.
const anyOID = 2276

// fnArg is one argument of a function: the name may be empty.
type fnArg struct {
	name string
	oid  uint32
	mode string // i, v for inputs; o, t for outputs
}

// funcArgs splits a function's arguments into inputs and outputs. ok is false
// when an INOUT argument is present.
func funcArgs(fn catalog.Func) (ins, outs []fnArg, ok bool) {
	types, modes := fn.AllArgTypes, fn.ArgModes
	if len(types) == 0 {
		types = fn.ArgTypes // only IN and VARIADIC arguments: proargtypes is the whole list
	}
	for i, t := range types {
		a := fnArg{oid: t, mode: "i"}
		if i < len(modes) && modes[i] != "" {
			a.mode = modes[i]
		}
		if i < len(fn.ArgNames) {
			a.name = fn.ArgNames[i]
		}
		switch a.mode {
		case "i", "v":
			ins = append(ins, a)
		case "o", "t":
			outs = append(outs, a)
		default: // b: INOUT
			return nil, nil, false
		}
	}
	return ins, outs, true
}

// scalarType reports whether a set of this type is a set of single values.
func scalarType(s *catalog.Schema, oid uint32) bool {
	t, ok := s.TypeByOID(oid)
	return ok && t.Kind != "c" && t.Kind != "p"
}

type fnWriter struct {
	f        *file
	m        *mapper
	s        *catalog.Schema
	fn       catalog.Func
	qual     string
	ins      []fnArg
	variadic bool
	cols     []fnArg
	colsType string
	colsVar  string
}

// emit writes the wrapper taking the first n inputs. all marks the NameAll
// form of a function with defaults.
func (w *fnWriter) emit(goName string, n int, all bool) {
	f, fn := w.f, w.fn
	var params, args []string
	seen := map[string]bool{}
	for i := 0; i < n; i++ {
		a := w.ins[i]
		var g goType
		if a.mode == "v" {
			g = w.m.resolve(fn.VariadicType)
		} else {
			g = w.m.resolve(a.oid)
		}
		f.use(g.imports...)
		name := fmt.Sprintf("a%d", i+1)
		if a.name != "" {
			name = lowerCamel(a.name)
		}
		for seen[name] || reservedParam[name] {
			name += "_"
		}
		seen[name] = true
		t := g.expr
		if strings.Contains(t, "qt.") {
			f.use("github.com/noobmaisfier/qlx/qt")
		}
		if a.mode == "v" {
			params = append(params, fmt.Sprintf("%s ...qlx.Operand[%s]", name, t))
			args = append(args, name+"...")
			continue
		}
		params = append(params, fmt.Sprintf("%s qlx.Operand[%s]", name, t))
		args = append(args, "qlx.Arg("+name+")")
	}
	call := joinArgs(variadicArgs(args))
	prelude := variadicPrelude(args)

	note := ""
	if all {
		note = " It takes every argument, defaults included: " + strings.TrimSuffix(goName, "All") + " leaves the trailing ones to PostgreSQL."
	} else if fn.Defaults > 0 {
		note = " The arguments that have a default are left out (see " + goName + "All)."
	}

	switch {
	case fn.Kind == "p":
		f.p("// %s is the procedure %s.%s\nfunc %s(%s) qlx.Command {\n%s\treturn qlx.Call(qlx.Procedure{Schema: %q, Name: %q}%s)\n}\n\n",
			goName, w.qual, note, goName, strings.Join(params, ", "), prelude, fn.Schema, fn.Name, call)
	case len(w.cols) > 0:
		f.p("// %s is the set-returning function %s. Name its columns with As: %s.As(\"t\", %s.Fields()...).%s\nfunc %s(%s) qlx.TableFunc {\n%s\treturn qlx.Func(%q, %q%s)\n}\n\n",
			goName, w.qual, goName+"(...)", w.colsVar, note, goName, strings.Join(params, ", "), prelude, fn.Schema, fn.Name, call)
	default:
		rt := w.m.resolve(fn.ReturnType)
		f.use(rt.imports...)
		ret := rt.expr
		if !fn.Strict && rt.nullOK && rt.fam != famArray {
			ret = "qt.Null[" + rt.expr + "]"
		}
		if strings.Contains(ret, "qt.") {
			f.use("github.com/noobmaisfier/qlx/qt")
		}
		f.p("// %s calls %s.", goName, w.qual)
		if !fn.Strict {
			f.p(" It is not STRICT, so the result may be NULL.")
		}
		f.p("%s\nfunc %s(%s) qlx.Expr[%s] {\n%s\treturn qlx.FuncCall[%s](%q, %q%s)\n}\n\n",
			note, goName, strings.Join(params, ", "), ret, prelude, ret, fn.Schema, fn.Name, call)
	}
}

// reservedParam are names a parameter cannot take without shadowing something
// the generated body uses.
var reservedParam = map[string]bool{"qlx": true, "qt": true, "args": true}

// variadicArgs and variadicPrelude build the argument list of a call whose last
// argument is a Go variadic: the operands are converted one by one.
func variadicArgs(args []string) []string {
	if len(args) > 0 && strings.HasSuffix(args[len(args)-1], "...") {
		return []string{"args..."} // the prelude has put the leading arguments in args
	}
	return args
}

func variadicPrelude(args []string) string {
	if len(args) == 0 || !strings.HasSuffix(args[len(args)-1], "...") {
		return ""
	}
	last := strings.TrimSuffix(args[len(args)-1], "...")
	head := strings.Join(args[:len(args)-1], ", ")
	if head != "" {
		head += ", "
	}
	return fmt.Sprintf("\targs := []qlx.Value{%s}\n\tfor _, v := range %s {\n\t\targs = append(args, qlx.Arg(v))\n\t}\n", head, last)
}

// emitColumns writes the typed columns of a set-returning function: every
// column may be NULL, and they are listed in the order the function returns
// them.
func (w *fnWriter) emitColumns(goName string) {
	f := w.f
	f.use("github.com/noobmaisfier/qlx")
	type col struct{ field, typ, pg string }
	var cols []col
	seen := map[string]bool{"Fields": true}
	for i, o := range w.cols {
		g := w.m.resolve(o.oid)
		f.use(g.imports...)
		t := g.expr
		if g.nullOK && g.fam != famArray {
			t = "qt.Null[" + t + "]"
		}
		if strings.Contains(t, "qt.") {
			f.use("github.com/noobmaisfier/qlx/qt")
		}
		name := o.name
		if name == "" {
			name = fmt.Sprintf("column%d", i+1) // PostgreSQL names an unnamed OUT column column1, column2, ...
		}
		fld := camel(name)
		if fld == "" {
			fld = "Column"
		}
		for seen[fld] {
			fld += "_"
		}
		seen[fld] = true
		cols = append(cols, col{fld, t, name})
	}
	f.p("// %s are the output columns of %s, in order. Every column may be NULL.\ntype %s struct {\n", w.colsType, w.qual, w.colsType)
	for _, c := range cols {
		f.p("\t%s qlx.Field[%s]\n", c.field, c.typ)
	}
	f.p("}\n\n// Fields lists the columns for As.\nfunc (c %s) Fields() []qlx.NameRef {\n\treturn []qlx.NameRef{", w.colsType)
	for i, c := range cols {
		if i > 0 {
			f.p(", ")
		}
		f.p("c.%s", c.field)
	}
	f.p("}\n}\n\n// %s names the columns of %s for use with As and qlx.Ref.\nvar %s = %s{\n", w.colsVar, goName, w.colsVar, w.colsType)
	for _, c := range cols {
		f.p("\t%s: qlx.NewField[%s](%q),\n", c.field, c.typ, c.pg)
	}
	f.p("}\n\n")
}

func joinArgs(a []string) string {
	if len(a) == 0 {
		return ""
	}
	return ", " + strings.Join(a, ", ")
}
