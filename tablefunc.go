package qlx

import (
	"fmt"
	"time"

	"github.com/noobmaisfier/qlx/qt"
)

// TableFunc is a set-returning function used as a row source:
//
//	FROM unnest($1) WITH ORDINALITY AS "u" ("x", "n")
//
// Like LATERAL, its arguments may refer to tables earlier in FROM. Build one
// with a typed constructor (Unnest, GenerateSeries, ...) or, for a function in
// the generated catalog, with Func and As.
type TableFunc struct {
	schema, name string
	args         []node
	alias        string
	cols         []string
	ord          string
	min, until   int
}

// Func starts a table function call. The columns it produces are named by As.
func Func(schema, name string, args ...Value) TableFunc {
	return TableFunc{schema: schema, name: name, args: nodesOf(args)}
}

// FuncSince is Func for a function that needs a PostgreSQL version.
func FuncSince(min int, schema, name string, args ...Value) TableFunc {
	t := Func(schema, name, args...)
	t.min = min
	return t
}

// FuncRange is Func for a function that exists only in majors min <= v < until
// (0: no bound on that side).
func FuncRange(min, until int, schema, name string, args ...Value) TableFunc {
	t := Func(schema, name, args...)
	t.min, t.until = min, until
	return t
}

// As names the row source and its columns. The Fields give the column names
// and their Go types, in the order the function returns them.
func (t TableFunc) As(alias string, cols ...NameRef) TableFunc {
	t.alias = alias
	t.cols = nil
	for _, c := range cols {
		t.cols = append(t.cols, c.outName())
	}
	return t
}

// WithOrdinality adds WITH ORDINALITY; the given column receives the 1-based
// row number (a bigint).
func (t TableFunc) WithOrdinality(col Field[int64]) TableFunc {
	t.ord = col.name
	return t
}

func (t TableFunc) scopeKey() string { return t.alias }
func (t TableFunc) colNames() []string {
	out := append([]string{}, t.cols...)
	if t.ord != "" {
		out = append(out, t.ord)
	}
	return out
}

func (t TableFunc) renderFrom(b *builder) {
	if t.alias == "" || len(t.cols) == 0 {
		b.fail(fmt.Errorf("%w: table function %s needs As(alias, columns...)", ErrShape, t.name))
		return
	}
	if t.min > 0 {
		b.need(t.min, t.name+"()")
	}
	if t.until > 0 && b.version >= t.until {
		b.fail(fmt.Errorf("%w: %s was removed in PostgreSQL %d, target is %d", ErrUnsupportedByServer, t.name, t.until, b.version))
		return
	}
	if t.schema != "" {
		b.ident(t.schema)
		b.write(".")
	}
	b.ident(t.name)
	b.write("(")
	joinParts(b, len(t.args), func(i int) { t.args[i].render(b) })
	b.write(")")
	if t.ord != "" {
		b.write(" WITH ORDINALITY")
	}
	b.write(" AS ")
	b.ident(t.alias)
	b.write(" (")
	names := t.colNames()
	joinParts(b, len(names), func(i int) { b.ident(names[i]) })
	b.write(")")
}

// Unnest expands an array into rows: unnest(arr) AS alias (col).
func Unnest[E any](arr Operand[[]E]) TableFunc {
	return Func("pg_catalog", "unnest", ArgP(arr))
}

// Unnest2 expands two arrays side by side (padded with NULLs to equal length).
func Unnest2[A, B any](a Operand[[]A], b Operand[[]B]) TableFunc {
	return Func("pg_catalog", "unnest", ArgP(a), ArgP(b))
}

// Integer is the integer types generate_series accepts.
type Integer interface{ ~int32 | ~int64 }

// GenerateSeries is generate_series(start, stop).
func GenerateSeries[T Integer](start, stop Operand[T]) TableFunc {
	return Func("pg_catalog", "generate_series", ArgP(start), ArgP(stop))
}

// GenerateSeriesStep is generate_series(start, stop, step).
func GenerateSeriesStep[T Integer](start, stop, step Operand[T]) TableFunc {
	return Func("pg_catalog", "generate_series", ArgP(start), ArgP(stop), ArgP(step))
}

// GenerateSeriesTime is generate_series over timestamps with an interval step.
func GenerateSeriesTime(start, stop Operand[time.Time], step Operand[qt.Interval]) TableFunc {
	return Func("pg_catalog", "generate_series", ArgP(start), ArgP(stop), ArgP(step))
}

// JSONBArrayElements is jsonb_array_elements(doc): one row per array element.
func JSONBArrayElements(doc Operand[qt.RawJSON]) TableFunc {
	return Func("pg_catalog", "jsonb_array_elements", ArgT(doc, PgJSONB))
}

// JSONBEach is jsonb_each(doc): one (key, value) row per top-level member.
func JSONBEach(doc Operand[qt.RawJSON]) TableFunc {
	return Func("pg_catalog", "jsonb_each", ArgT(doc, PgJSONB))
}

// StringToTable is string_to_table(text, delimiter): one row per piece.
func StringToTable(s, delim Operand[string]) TableFunc {
	return Func("pg_catalog", "string_to_table", ArgT(s, PgText), ArgT(delim, PgText))
}
