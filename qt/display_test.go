package qt_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/noobmaisfier/qlx/qt"
)

func TestPrintingShowsValuesNotStructs(t *testing.T) {
	one, five := 1, 5
	for _, c := range []struct {
		got  any
		want string
	}{
		{qt.Some("hi"), "hi"},
		{qt.Null[string]{}, "NULL"},
		{qt.Some(42), "42"},
		{qt.Of("x"), "x"},
		{qt.Opt[string]{}, "unset"},
		{qt.Of(qt.Null[int]{}), "NULL"},
		{qt.Of(qt.Some(7)), "7"},
		{qt.JSONB[map[string]int]{V: map[string]int{"a": 1}}, `{"a":1}`},
		{qt.JSONB[any]{}, "null"},
		{qt.Range[int]{Lower: &one, Upper: &five, LowerInc: true}, "[1,5)"},
		{qt.Range[int]{Upper: &five, UpperInc: true}, "(,5]"},
		{qt.Range[int]{Empty: true}, "empty"},
		{qt.Range[int]{Null: true}, "NULL"},
		{qt.Multirange[int](nil), "NULL"},
		{qt.Multirange[int]{}, "{}"},
		{qt.Multirange[int]{{Lower: &one, Upper: &five, LowerInc: true}, {Empty: true}}, "{[1,5),empty}"},
	} {
		if s := fmt.Sprintf("%v", c.got); s != c.want {
			t.Errorf("%%v of %#v = %q, want %q", c.got, s, c.want)
		}
	}
	// a nested Stringer is used, so a Null of a date prints the date
	if s := fmt.Sprint(qt.Some(qt.DateOf(2026, time.November, 21))); s != "2026-11-21" {
		t.Errorf("Null[Date] = %q", s)
	}
}
