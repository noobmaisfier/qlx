package qt_test

import (
	"encoding/json"
	"testing"

	"github.com/noobmaisfier/qlx/qt"
)

func TestNullScanValueJSON(t *testing.T) {
	var n qt.Null[int64]
	if err := n.Scan(nil); err != nil || n.Valid {
		t.Fatalf("nil must scan to NULL: %v %+v", err, n)
	}
	if err := n.Scan(int64(7)); err != nil || !n.Valid || n.V != 7 {
		t.Fatalf("value scan: %v %+v", err, n)
	}
	if v, _ := n.Value(); v != int64(7) {
		t.Fatalf("Value() = %v", v)
	}
	if v, _ := (qt.Null[string]{}).Value(); v != nil {
		t.Fatalf("NULL Value() = %v", v)
	}
	b, _ := json.Marshal(struct {
		A qt.Null[string]
		B qt.Null[string]
	}{B: qt.Some("x")})
	if string(b) != `{"A":null,"B":"x"}` {
		t.Fatalf("json: %s", b)
	}
	var back struct{ A, B qt.Null[string] }
	if err := json.Unmarshal(b, &back); err != nil || back.A.Valid || back.B.V != "x" || !back.B.Valid {
		t.Fatalf("round trip: %v %+v", err, back)
	}
}

func TestIntervalParse(t *testing.T) {
	cases := map[string]qt.Interval{
		"1 year 2 mons 3 days 04:05:06.5": {Months: 14, Days: 3, Micros: 4*3600e6 + 5*60e6 + 6_500_000},
		"-1 days +02:03:04":               {Days: -1, Micros: 2*3600e6 + 3*60e6 + 4e6},
		"00:00:00":                        {},
		"P1Y2M3DT4H5M6S":                  {Months: 14, Days: 3, Micros: 4*3600e6 + 5*60e6 + 6e6},
	}
	for in, want := range cases {
		got, err := qt.ParseInterval(in)
		if err != nil || got != want {
			t.Errorf("%q: got %+v, %v; want %+v", in, got, err, want)
		}
	}
	if _, err := qt.ParseInterval("3 fortnights"); err == nil {
		t.Error("unknown unit accepted")
	}
}
