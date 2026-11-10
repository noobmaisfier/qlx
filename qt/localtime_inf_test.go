package qt_test

import (
	"testing"
	"time"

	"github.com/noobmaisfier/qlx/qt"
)

func TestLocalTimeInfinity(t *testing.T) {
	if qt.LocalTimeInfinity.Sign() != 1 || qt.LocalTimeNegInfinity.Sign() != -1 {
		t.Fatal("sentinel signs")
	}
	// the edges of PostgreSQL's range are ordinary instants
	for _, d := range []time.Time{time.Date(294276, 12, 31, 23, 59, 59, 0, time.UTC), time.Date(-4712, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), {}} {
		if qt.LocalTime(d).IsInfinite() {
			t.Errorf("%v is not infinite", d)
		}
	}
	if !qt.LocalTimeNegInfinity.Time().Before(qt.LocalTime(time.Date(-4712, 1, 1, 0, 0, 0, 0, time.UTC)).Time()) {
		t.Error("-infinity sorts below every timestamp")
	}
	if !qt.LocalTimeInfinity.Time().After(time.Date(294276, 12, 31, 0, 0, 0, 0, time.UTC)) {
		t.Error("infinity sorts above every timestamp")
	}
	for _, c := range []struct {
		in   string
		want qt.LocalTime
	}{{"infinity", qt.LocalTimeInfinity}, {"-infinity", qt.LocalTimeNegInfinity}} {
		var got qt.LocalTime
		if err := got.Scan(c.in); err != nil || got != c.want {
			t.Errorf("Scan(%q) = %v %v", c.in, got, err)
		}
		if err := got.Scan([]byte(c.in)); err != nil || got != c.want {
			t.Errorf("Scan([]byte(%q)) = %v %v", c.in, got, err)
		}
		v, err := c.want.Value()
		if err != nil || v != c.in {
			t.Errorf("Value() = %v %v, want %q", v, err, c.in)
		}
	}
	// ordinary values are unchanged
	d := qt.LocalTime(time.Date(2024, 2, 29, 23, 59, 59, 999999000, time.UTC))
	if v, _ := d.Value(); v != "2024-02-29 23:59:59.999999" {
		t.Errorf("Value() = %v", v)
	}
}
