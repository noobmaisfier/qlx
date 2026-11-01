package qlx

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/noobmaisfier/qlx/qt"
)

// FuncCallSince is FuncCall for a function that only exists from a given
// PostgreSQL major version; rendering for an older target fails.
func FuncCallSince[T any](min int, schema, name string, args ...Value) Expr[T] {
	return Expr[T]{versioned{min, name + "()", qualFnNode{schema, name, nodesOf(args)}}}
}

// FuncCallRange is FuncCall for a function that exists from major min (0:
// always) and was removed in major until (0: never). Rendering for a target
// outside that range fails with ErrUnsupportedByServer.
func FuncCallRange[T any](min, until int, schema, name string, args ...Value) Expr[T] {
	return Expr[T]{versionedRange{min, until, name + "()", qualFnNode{schema, name, nodesOf(args)}}}
}

// BinaryOpRange is BinaryOp gated to the majors min <= version < until.
func BinaryOpRange[T any](min, until int, schema, sym string, l, r Value) Expr[T] {
	return Expr[T]{versionedRange{min, until, "operator " + sym, opNode{schema, sym, l.valueNode(), r.valueNode()}}}
}

// PrefixOpRange is PrefixOp gated to the majors min <= version < until.
func PrefixOpRange[T any](min, until int, schema, sym string, r Value) Expr[T] {
	return Expr[T]{versionedRange{min, until, "operator " + sym, opNode{schema, sym, nil, r.valueNode()}}}
}

// opNode is "(l OPERATOR(schema.sym) r)", or a prefix operator when l is nil.
// The symbol is not free text: it must consist only of PostgreSQL operator
// characters and contain no comment opener, so it cannot carry other SQL.
type opNode struct {
	schema string
	sym    string
	l, r   node
}

const opChars = "+-*/<>=~!@#%^&|`?"

func validOpSymbol(s string) bool {
	if s == "" || len(s) > 63 {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune(opChars, c) {
			return false
		}
	}
	return !strings.Contains(s, "--") && !strings.Contains(s, "/*")
}

func (n opNode) render(b *builder) {
	if !validOpSymbol(n.sym) {
		b.fail(fmt.Errorf("%w: %q is not an operator symbol", ErrInvalidIdent, n.sym))
		return
	}
	b.write("(")
	if n.l != nil {
		n.l.render(b)
		b.write(" ")
	}
	b.write("OPERATOR(")
	if n.schema != "" {
		b.ident(n.schema)
		b.write(".")
	}
	b.write(n.sym + ")")
	b.write(" ")
	n.r.render(b)
	b.write(")")
}

// BinaryOp applies a catalog operator to two operands. Generated code is the
// intended caller; the symbol is validated at render time.
func BinaryOp[T any](schema, sym string, l, r Value) Expr[T] {
	return Expr[T]{opNode{schema, sym, l.valueNode(), r.valueNode()}}
}

// PrefixOp applies a prefix operator such as @ (absolute value) or ~ (bit not).
func PrefixOp[T any](schema, sym string, r Value) Expr[T] {
	return Expr[T]{opNode{schema, sym, nil, r.valueNode()}}
}

// BinaryOpSince is BinaryOp gated on a PostgreSQL major version.
func BinaryOpSince[T any](min int, schema, sym string, l, r Value) Expr[T] {
	return Expr[T]{versioned{min, "operator " + sym, opNode{schema, sym, l.valueNode(), r.valueNode()}}}
}

// PrefixOpSince is PrefixOp gated on a PostgreSQL major version.
func PrefixOpSince[T any](min int, schema, sym string, r Value) Expr[T] {
	return Expr[T]{versioned{min, "operator " + sym, opNode{schema, sym, nil, r.valueNode()}}}
}

// placeholderTyped casts a bare placeholder to the declared parameter type of
// the function it is passed to. Without it an overloaded built-in such as
// abs($1) is ambiguous to the server; with it the call resolves to exactly the
// overload the Go signature names. Anything that is not a bare placeholder
// already has a type and is rendered unchanged.
type placeholderTyped struct {
	n node
	t typeRef
}

func (p placeholderTyped) render(b *builder) {
	switch p.n.(type) {
	case argNode, paramNode:
		castNode{p.n, p.t}.render(b)
	default:
		p.n.render(b)
	}
}

// ArgT adapts an operand for a function call and pins the placeholder type;
// see placeholderTyped. Generated wrappers use it for every argument.
func ArgT[T, X any](o Operand[T], t PgType[X]) Value {
	return nodeValue{placeholderTyped{o.exprNode(), t}}
}

// ArgsT adapts the extra operands of a variadic wrapper, pinning each to t.
func ArgsT[T, X any](os []Operand[T], t PgType[X]) []Value {
	out := make([]Value, len(os))
	for i, o := range os {
		out[i] = ArgT(o, t)
	}
	return out
}

// ArgP adapts an operand of a generic wrapper: the placeholder is pinned to
// the PostgreSQL type of T when T is a plain built-in Go type, so polymorphic
// built-ins such as array_append($1, $2) can resolve their element type.
// Types it does not know (enums, composites) are left for the server to infer.
func ArgP[T any](o Operand[T]) Value {
	if name, ok := pgNameOfType(reflect.TypeFor[T]()); ok {
		return nodeValue{placeholderTyped{o.exprNode(), rawType(name)}}
	}
	return nodeValue{o.exprNode()}
}

// rawType is a type name taken from the closed set returned by pgNameOfType.
type rawType string

func (t rawType) renderType(b *builder) { b.write(string(t)) }

func pgNameOfType(t reflect.Type) (string, bool) {
	// Null[T] wrappers pin to the type of their value.
	if t.Kind() == reflect.Struct && t.NumField() == 2 && t.Field(0).Name == "V" && t.Field(1).Name == "Valid" {
		return pgNameOfType(t.Field(0).Type)
	}
	switch t {
	case reflect.TypeFor[int16]():
		return "int2", true
	case reflect.TypeFor[int32]():
		return "int4", true
	case reflect.TypeFor[int64](), reflect.TypeFor[int]():
		return "int8", true
	case reflect.TypeFor[float32]():
		return "float4", true
	case reflect.TypeFor[float64]():
		return "float8", true
	case reflect.TypeFor[string]():
		return "text", true
	case reflect.TypeFor[bool]():
		return "bool", true
	case reflect.TypeFor[[]byte]():
		return "bytea", true
	case reflect.TypeFor[time.Time]():
		return "timestamptz", true
	case reflect.TypeFor[qt.LocalTime]():
		return "timestamp", true
	case reflect.TypeFor[qt.Date]():
		return "date", true
	case reflect.TypeFor[qt.XML]():
		return "xml", true
	case reflect.TypeFor[qt.Char]():
		return `"char"`, true
	case reflect.TypeFor[qt.TSVector]():
		return "tsvector", true
	case reflect.TypeFor[qt.TSQuery]():
		return "tsquery", true
	case reflect.TypeFor[qt.Bit]():
		return "varbit", true
	case reflect.TypeFor[qt.Point]():
		return "point", true
	case reflect.TypeFor[qt.Line]():
		return "line", true
	case reflect.TypeFor[qt.Lseg]():
		return "lseg", true
	case reflect.TypeFor[qt.Box]():
		return "box", true
	case reflect.TypeFor[qt.Path]():
		return "path", true
	case reflect.TypeFor[qt.Polygon]():
		return "polygon", true
	case reflect.TypeFor[qt.Circle]():
		return "circle", true
	case reflect.TypeFor[qt.Money]():
		return "money", true
	case reflect.TypeFor[qt.MacAddr]():
		return "macaddr", true
	case reflect.TypeFor[qt.MacAddr8]():
		return "macaddr8", true
	case reflect.TypeFor[qt.LSN]():
		return "pg_lsn", true
	case reflect.TypeFor[qt.RegClass]():
		return "regclass", true
	case reflect.TypeFor[qt.RegType]():
		return "regtype", true
	case reflect.TypeFor[qt.RegProc]():
		return "regproc", true
	case reflect.TypeFor[qt.RegProcedure]():
		return "regprocedure", true
	case reflect.TypeFor[qt.RegOper]():
		return "regoper", true
	case reflect.TypeFor[qt.RegOperator]():
		return "regoperator", true
	case reflect.TypeFor[qt.RegNamespace]():
		return "regnamespace", true
	case reflect.TypeFor[qt.RegRole]():
		return "regrole", true
	case reflect.TypeFor[qt.RegConfig]():
		return "regconfig", true
	case reflect.TypeFor[qt.RegDict]():
		return "regdictionary", true
	case reflect.TypeFor[qt.Xid]():
		return "xid", true
	case reflect.TypeFor[qt.Xid8]():
		return "xid8", true
	case reflect.TypeFor[qt.Cid]():
		return "cid", true
	case reflect.TypeFor[qt.Tid]():
		return "tid", true
	case reflect.TypeFor[qt.JSONPath]():
		return "jsonpath", true
	case reflect.TypeFor[qt.ACLItem]():
		return "aclitem", true
	case reflect.TypeFor[qt.OIDVector]():
		return "oidvector", true
	case reflect.TypeFor[qt.Int2Vector]():
		return "int2vector", true
	case reflect.TypeFor[qt.Oid8]():
		return "oid8", true
	case reflect.TypeFor[qt.Snapshot]():
		return "pg_snapshot", true
	case reflect.TypeFor[qt.NodeTree]():
		return "pg_node_tree", true
	case reflect.TypeFor[qt.RefCursor]():
		return "refcursor", true
	case reflect.TypeFor[qt.RegCollation]():
		return "regcollation", true
	case reflect.TypeFor[qt.RegDatabase]():
		return "regdatabase", true
	case reflect.TypeFor[qt.NDistinct]():
		return "pg_ndistinct", true
	case reflect.TypeFor[qt.Dependencies]():
		return "pg_dependencies", true
	case reflect.TypeFor[qt.MCVList]():
		return "pg_mcv_list", true
	case reflect.TypeFor[qt.TimeOfDay]():
		return "time", true
	case reflect.TypeFor[qt.TimeTZ]():
		return "timetz", true
	case reflect.TypeFor[qt.Numeric]():
		return "numeric", true
	}
	if t.PkgPath() == "github.com/noobmaisfier/qlx/qt" {
		multi := strings.HasPrefix(t.Name(), "Multirange[")
		if multi || strings.HasPrefix(t.Name(), "Range[") {
			rt := t
			if multi {
				rt = t.Elem()
			}
			if rt.Kind() == reflect.Struct && rt.NumField() > 0 && rt.Field(0).Name == "Lower" {
				var name string
				switch rt.Field(0).Type.Elem() {
				case reflect.TypeFor[int32]():
					name = "int4"
				case reflect.TypeFor[int64]():
					name = "int8"
				case reflect.TypeFor[qt.Numeric]():
					name = "num"
				case reflect.TypeFor[qt.Date]():
					name = "date"
				case reflect.TypeFor[qt.LocalTime]():
					name = "ts"
				case reflect.TypeFor[time.Time]():
					name = "tstz"
				default:
					return "", false
				}
				if multi {
					return name + "multirange", true
				}
				return name + "range", true
			}
		}
	}
	if t.Kind() == reflect.Array && t.Len() == 16 && t.Elem().Kind() == reflect.Uint8 {
		return "uuid", true // [16]byte and the standard library's uuid.UUID
	}
	if t.Kind() == reflect.Slice {
		if e, ok := pgNameOfType(t.Elem()); ok {
			return e + "[]", true
		}
	}
	return "", false
}
