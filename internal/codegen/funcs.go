package codegen

import (
	"fmt"
	"strings"

	"github.com/noobmaisfier/qlx/internal/catalog"
)

// emitFuncs writes a typed wrapper for each plain function and procedure.
// Anything it cannot type safely (set-returning, OUT parameters, variadic,
// default arguments) is skipped and reported, never guessed.
func emitFuncs(f *file, m *mapper, nm *namer, s *catalog.Schema, res *Result) {
	for _, fn := range s.Funcs {
		qual := fn.Schema + "." + fn.Name
		switch {
		case fn.ReturnsSet:
			res.Skipped = append(res.Skipped, qual+": returns a set (use a derived table)")
			continue
		case fn.HasVariadic:
			res.Skipped = append(res.Skipped, qual+": variadic")
			continue
		case fn.HasDefaults:
			res.Skipped = append(res.Skipped, qual+": has default arguments")
			continue
		}
		outMode := false
		for _, md := range fn.ArgModes {
			if md != "i" {
				outMode = true
			}
		}
		if outMode {
			res.Skipped = append(res.Skipped, qual+": OUT or INOUT parameters")
			continue
		}
		f.use("github.com/noobmaisfier/qlx")
		var params, args []string
		seen := map[string]bool{}
		for i, at := range fn.ArgTypes {
			g := m.resolve(at)
			f.use(g.imports...)
			name := fmt.Sprintf("a%d", i+1)
			if i < len(fn.ArgNames) && fn.ArgNames[i] != "" {
				name = lowerCamel(fn.ArgNames[i])
			}
			for seen[name] {
				name += "_"
			}
			seen[name] = true
			t := g.expr
			if strings.Contains(t, "qt.") {
				f.use("github.com/noobmaisfier/qlx/qt")
			}
			params = append(params, fmt.Sprintf("%s qlx.Operand[%s]", name, t))
			args = append(args, "qlx.Arg("+name+")")
		}
		suffix := ""
		for _, at := range fn.ArgTypes {
			if tt, ok := s.TypeByOID(at); ok {
				suffix += camel(tt.Name)
			}
		}
		goName := nm.take(camel(fn.Name), suffix)
		if fn.Kind == "p" {
			f.p("// %s is the procedure %s.\nfunc %s(%s) qlx.Command {\n\treturn qlx.Call(qlx.Procedure{Schema: %q, Name: %q}%s)\n}\n\n",
				goName, qual, goName, strings.Join(params, ", "), fn.Schema, fn.Name, joinArgs(args))
			continue
		}
		rt := m.resolve(fn.ReturnType)
		f.use(rt.imports...)
		ret := rt.expr
		if !fn.Strict && rt.nullOK && rt.fam != famArray {
			ret = "qt.Null[" + rt.expr + "]"
		}
		if strings.Contains(ret, "qt.") {
			f.use("github.com/noobmaisfier/qlx/qt")
		}
		f.p("// %s calls %s.", goName, qual)
		if !fn.Strict {
			f.p(" It is not STRICT, so the result may be NULL.")
		}
		f.p("\nfunc %s(%s) qlx.Expr[%s] {\n\treturn qlx.FuncCall[%s](%q, %q%s)\n}\n\n",
			goName, strings.Join(params, ", "), ret, ret, fn.Schema, fn.Name, joinArgs(args))
	}
}

func joinArgs(a []string) string {
	if len(a) == 0 {
		return ""
	}
	return ", " + strings.Join(a, ", ")
}
