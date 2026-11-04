package qlx_test

import (
	"reflect"
	"testing"

	q "github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/qt"
)

// Autocomplete budget: a column family must not become a wall of methods.
// Raising a limit is a deliberate act that shows up in review.
func TestColumnFamilySurface(t *testing.T) {
	limits := map[string]int{
		"Col":      30,
		"NumCol":   46,
		"TextCol":  52,
		"TimeCol":  40, // 38 + ToIf, ToOrDefault (every column type has them)
		"BoolCol":  35,
		"ArrayCol": 37,
		"JSONBCol": 37,
	}
	types := map[string]reflect.Type{
		"Col":      reflect.TypeFor[q.Col[int64]](),
		"NumCol":   reflect.TypeFor[q.NumCol[int64]](),
		"TextCol":  reflect.TypeFor[q.TextCol[string]](),
		"TimeCol":  reflect.TypeFor[q.TimeCol[qt.LocalTime]](),
		"BoolCol":  reflect.TypeFor[q.BoolCol](),
		"ArrayCol": reflect.TypeFor[q.ArrayCol[string]](),
		"JSONBCol": reflect.TypeFor[q.JSONBCol[[]byte]](),
	}
	for name, ty := range types {
		n := 0
		for i := 0; i < ty.NumMethod(); i++ {
			if ty.Method(i).IsExported() {
				n++
			}
		}
		t.Logf("%s: %d exported methods", name, n)
		if n > limits[name] {
			t.Errorf("%s has %d exported methods, budget %d", name, n, limits[name])
		}
	}
}
