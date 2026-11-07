package qlx

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestInlineLiteralEdges(t *testing.T) {
	ok := map[string]any{
		`'-0'::float8`:                         math.Copysign(0, -1),
		`0::float8`:                            0.0,
		`(-1.5)::float8`:                       -1.5,
		`1e+21::float8`:                        1e21,
		`'NaN'::float8`:                        math.NaN(),
		`'-Infinity'::float8`:                  math.Inf(-1),
		`(-9223372036854775808)`:               int64(math.MinInt64),
		`18446744073709551615`:                 uint64(math.MaxUint64),
		`'0001-01-01T00:00:00Z'::timestamptz`:  time.Time{},
		`'12000-01-01T00:00:00Z'::timestamptz`: time.Date(12000, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	for want, v := range ok {
		if got, err := inlineLiteral(v); err != nil || got != want {
			t.Errorf("%v: got %q, %v; want %q", v, got, err, want)
		}
	}
	if _, err := inlineLiteral(time.Date(-5, 1, 1, 0, 0, 0, 0, time.UTC)); !errors.Is(err, ErrParams) {
		t.Errorf("BC time: %v", err)
	}
}
