package qlxvet

import (
	"go/ast"
	"go/types"
	"reflect"
	"strconv"
	"strings"
)

// optOmitZero is V031: a struct field of type qt.Opt[qt.Null[T]] whose json tag
// lacks omitzero. An unset Opt has no JSON form of its own: without omitzero
// the field would marshal as null, which reads back as an explicit NULL, so a
// "leave it alone" would turn into "set it to NULL". qt.Opt refuses to do
// that: marshaling an unset Opt[Null[T]] returns qt.ErrUnsetOptNull. The
// analyzer says so before the first request does.
//
// Only Opt[Null[T]] is reported. A plain Opt[T] marshals null when unset, and
// null decodes into it as "not set", so it round-trips; omitzero is still the
// better tag there (the key is left out), but it is not a defect.
func (c *checker) optOmitZero(st *ast.StructType) {
	if st.Fields == nil {
		return
	}
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 || !f.Names[0].IsExported() {
			continue // embedded or unexported: not a JSON member of its own
		}
		tv, ok := c.pass.TypesInfo.Types[f.Type]
		if !ok || !isOptOfNull(tv.Type) {
			continue
		}
		tag := ""
		if f.Tag != nil {
			if s, err := strconv.Unquote(f.Tag.Value); err == nil {
				tag = s
			}
		}
		name, opts, _ := strings.Cut(reflect.StructTag(tag).Get("json"), ",")
		if name == "-" && opts == "" {
			continue // json:"-": not marshaled
		}
		if hasOption(opts, "omitzero") {
			continue
		}
		c.report(f, "V031", f.Names[0].Name+" is a qt.Opt[qt.Null[T]] without omitzero in its json tag: unset, it marshals as null, which reads back as an explicit NULL, so qt refuses to marshal it (qt.ErrUnsetOptNull); tag it `json:\""+jsonKey(name, f.Names[0].Name)+",omitzero\"`")
	}
}

func jsonKey(name, goName string) string {
	if name == "" {
		return goName
	}
	return name
}

func hasOption(opts, want string) bool {
	for _, o := range strings.Split(opts, ",") {
		if o == want {
			return true
		}
	}
	return false
}

// isOptOfNull reports whether t is qt.Opt[qt.Null[T]].
func isOptOfNull(t types.Type) bool {
	n, ok := types.Unalias(t).(*types.Named)
	if !ok || n.Obj().Pkg() == nil || n.Obj().Pkg().Path() != qtPath || n.Obj().Name() != "Opt" {
		return false
	}
	ta := n.TypeArgs()
	if ta == nil || ta.Len() != 1 {
		return false
	}
	_, null := isNullType(types.Unalias(ta.At(0)))
	return null
}
