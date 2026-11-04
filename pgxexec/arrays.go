package pgxexec

import (
	"database/sql/driver"
	"fmt"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/noobmaisfier/qlx/internal/pgtext"
)

// ---- arrays ----

// Reading an array into a plain Go slice is only faithful for a one-dimensional
// array whose first index is 1. pgx flattens anything else without a word: the
// array {{1,2},{3,4}} arrives as [1 2 3 4], and [0:1]={5,6} loses its bounds.
// PostgreSQL does not put the number of dimensions in the column type, so the
// generator cannot see it; the executor can, on the value. This wrapper
// refuses such a value with an error that says what to use instead.
//
// It only applies to *[]T targets that pgx would otherwise hand to its own
// slice wrapper. A pgtype.Array / pgtype.FlatArray target, or a [][]T for a
// multi-dimensional column, is not touched.

type sliceArrayGuard struct{ rv reflect.Value } // rv is the slice, addressable

func (g sliceArrayGuard) SetDimensions(dims []pgtype.ArrayDimension) error {
	if dims == nil {
		g.rv.Set(reflect.Zero(g.rv.Type()))
		return nil
	}
	if len(dims) > 1 {
		return fmt.Errorf("pgxexec: a %d-dimensional array cannot be read into %s (it would be flattened and lose its shape): scan into pgtype.Array[T] or a [][]T", len(dims), g.rv.Type())
	}
	if len(dims) == 1 && dims[0].LowerBound != 1 {
		return fmt.Errorf("pgxexec: an array with lower bound %d cannot be read into %s (the bound would be lost): scan into pgtype.Array[T]", dims[0].LowerBound, g.rv.Type())
	}
	n := 0
	if len(dims) == 1 {
		n = int(dims[0].Length)
	}
	g.rv.Set(reflect.MakeSlice(g.rv.Type(), n, n))
	return nil
}

func (g sliceArrayGuard) ScanIndex(i int) any { return g.rv.Index(i).Addr().Interface() }
func (g sliceArrayGuard) ScanIndexType() any  { return reflect.New(g.rv.Type().Elem()).Interface() }

type arrayGuardPlan struct{ next pgtype.ScanPlan }

func (p *arrayGuardPlan) SetNext(n pgtype.ScanPlan) { p.next = n }
func (p *arrayGuardPlan) Scan(src []byte, target any) error {
	return p.next.Scan(src, sliceArrayGuard{reflect.ValueOf(target).Elem()})
}

var byteSliceType = reflect.TypeOf([]byte(nil))

func tryWrapArrayGuard(target any) (pgtype.WrappedScanPlanNextSetter, any, bool) {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return nil, nil, false
	}
	st := rv.Type().Elem()
	if st.Kind() != reflect.Slice || st == byteSliceType {
		return nil, nil, false
	}
	// A slice of slices (not bytea) is pgx's multi-dimensional form: leave it.
	if el := st.Elem(); el.Kind() == reflect.Slice && el != byteSliceType {
		return nil, nil, false
	}
	// pgx's own handling of types that scan themselves
	if _, ok := target.(pgtype.ArraySetter); ok {
		return nil, nil, false
	}
	return &arrayGuardPlan{}, sliceArrayGuard{rv.Elem()}, true
}

// arrayLiteral carries a slice as PostgreSQL's array text form. It is the last
// resort of the encode plans: used only when nothing else could plan the slice,
// which is what happens without a type OID (the simple protocol) for a slice of
// qt types, such as []qt.Null[int32] or []qt.Date. With an OID the element
// codecs do the work in binary and this is never reached.
type arrayLiteral struct{ rv reflect.Value }

func (a arrayLiteral) Value() (driver.Value, error) {
	if a.rv.Kind() == reflect.Slice && a.rv.IsNil() {
		return nil, nil
	}
	parts := make([]string, a.rv.Len())
	for i := range parts {
		s, err := pgtext.ArrayElem(a.rv.Index(i).Interface())
		if err != nil {
			return nil, fmt.Errorf("array element %d: %w", i, err)
		}
		parts[i] = s
	}
	return "{" + strings.Join(parts, ",") + "}", nil
}

type arrayLiteralPlan struct{ next pgtype.EncodePlan }

func (p *arrayLiteralPlan) SetNext(n pgtype.EncodePlan) { p.next = n }
func (p *arrayLiteralPlan) Encode(value any, buf []byte) ([]byte, error) {
	return p.next.Encode(arrayLiteral{reflect.ValueOf(value)}, buf)
}

func tryWrapArrayLiteral(value any) (pgtype.WrappedEncodePlanNextSetter, any, bool) {
	rv := reflect.ValueOf(value)
	if _, isValuer := value.(driver.Valuer); isValuer || rv.Kind() != reflect.Slice {
		return nil, nil, false // a slice type with its own text form (qt.Multirange, qt.Vector)
	}
	if et := rv.Type().Elem(); et.Kind() == reflect.Uint8 || et.Kind() == reflect.Slice || et.Kind() == reflect.Array && et.Elem().Kind() != reflect.Uint8 {
		return nil, nil, false // []byte, and multi-dimensional shapes, are not ours
	}
	return &arrayLiteralPlan{}, arrayLiteral{rv}, true
}
