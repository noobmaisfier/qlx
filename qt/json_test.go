package qt_test

import (
	"encoding/json"
	"net/netip"
	"testing"
	"time"

	"github.com/noobmaisfier/qlx/qt"
)

// The JSON forms quoted in docs/dtos.md. These are a public contract: a
// change here is a breaking change for every API that returns a qt value.

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("%T: %v", v, err)
	}
	return string(b)
}

func TestJSONScalarForms(t *testing.T) {
	ptr := func(v int) *int { return &v }
	cases := []struct {
		name string
		v    any
		want string
	}{
		{"null value", qt.Null[string]{}, `null`},
		{"null set", qt.Some("x"), `"x"`},
		{"date", qt.DateOf(2024, time.February, 29), `"2024-02-29"`},
		{"date infinity", qt.DateInfinity, `"infinity"`},
		{"timestamptz", qt.TimestamptzOf(time.Date(2024, 2, 29, 1, 2, 3, 0, time.UTC)), `"2024-02-29T01:02:03Z"`},
		{"timestamptz infinity", qt.TimestamptzInfinity, `"infinity"`},
		{"numeric", qt.Numeric("1.50"), `"1.50"`},
		{"time of day", qt.TimeOfDayOf(1, 0, 0, 0), `"01:00:00"`},
		{"interval", qt.Interval{Months: 14, Days: 3, Micros: 3600e6}, `"P1Y2M3DT1H"`},
		{"range", qt.Range[int]{Lower: ptr(1), Upper: ptr(5), LowerInc: true}, `{"lower":1,"upper":5,"lower_inc":true,"upper_inc":false}`},
		{"empty range", qt.Range[int]{Empty: true}, `{"empty":true}`},
		{"null range", qt.Range[int]{Null: true}, `null`},
		{"multirange", qt.Multirange[int]{{Lower: ptr(1), Upper: ptr(2), LowerInc: true}}, `[{"lower":1,"upper":2,"lower_inc":true,"upper_inc":false}]`},
		{"empty multirange", qt.Multirange[int]{}, `[]`},
		{"null multirange", qt.Multirange[int](nil), `null`},
		{"jsonb", qt.RawJSON{V: json.RawMessage(`{"a":1}`)}, `{"a":1}`},
		{"addr", netip.MustParseAddr("10.0.0.1"), `"10.0.0.1"`},
		{"vector", qt.Vector{1, 2}, `[1,2]`},
		{"char", qt.Char(97), `97`},
	}
	for _, c := range cases {
		if got := mustJSON(t, c.v); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestJSONScalarFormsBack(t *testing.T) {
	var d qt.Date
	if err := json.Unmarshal([]byte(`"2024-02-29"`), &d); err != nil || d != qt.DateOf(2024, 2, 29) {
		t.Errorf("date: %v %v", d, err)
	}
	var ts qt.Timestamptz
	if err := json.Unmarshal([]byte(`"2024-02-29T01:02:03Z"`), &ts); err != nil || !ts.Time().Equal(time.Date(2024, 2, 29, 1, 2, 3, 0, time.UTC)) {
		t.Errorf("timestamptz: %v %v", ts, err)
	}
	if err := json.Unmarshal([]byte(`"infinity"`), &ts); err != nil || !ts.IsInfinite() {
		t.Errorf("timestamptz infinity: %v %v", ts, err)
	}
	var n qt.Null[netip.Addr]
	if err := json.Unmarshal([]byte(`null`), &n); err != nil || n.Valid {
		t.Errorf("null addr: %v %v", n, err)
	}
	if err := json.Unmarshal([]byte(`"10.0.0.1"`), &n); err != nil || !n.Valid || n.V != netip.MustParseAddr("10.0.0.1") {
		t.Errorf("addr: %v %v", n, err)
	}
}

func TestJSONIntervalBack(t *testing.T) {
	for in, want := range map[string]qt.Interval{
		`"P1Y2M3DT1H"`:                    {Months: 14, Days: 3, Micros: 3600e6},
		`"1 year 2 mons 3 days 01:00:00"`: {Months: 14, Days: 3, Micros: 3600e6},
		`"PT0S"`:                          {},
	} {
		var got qt.Interval
		if err := json.Unmarshal([]byte(in), &got); err != nil || got != want {
			t.Errorf("%s: %+v %v, want %+v", in, got, err, want)
		}
	}
	// A round trip must preserve the components, which are not
	// interconvertible.
	for _, v := range []qt.Interval{
		{Months: -14, Days: 3, Micros: -3600e6},
		{Months: 0, Days: 0, Micros: 1_500_000},
	} {
		var back qt.Interval
		if err := json.Unmarshal([]byte(mustJSON(t, v)), &back); err != nil || back != v {
			t.Errorf("round trip %+v: %+v %v", v, back, err)
		}
	}
}

func TestJSONRangeBack(t *testing.T) {
	var r qt.Range[int]
	if err := json.Unmarshal([]byte(`{"lower":1,"upper":5,"lower_inc":true,"upper_inc":false}`), &r); err != nil {
		t.Fatal(err)
	}
	if r.Lower == nil || *r.Lower != 1 || r.Upper == nil || *r.Upper != 5 || !r.LowerInc || r.UpperInc || r.Empty || r.Null {
		t.Errorf("range: %+v", r)
	}
	if err := json.Unmarshal([]byte(`{"empty":true}`), &r); err != nil || !r.Empty {
		t.Errorf("empty: %+v %v", r, err)
	}
	if err := json.Unmarshal([]byte(`null`), &r); err != nil || !r.Null {
		t.Errorf("null: %+v %v", r, err)
	}
	for _, bad := range []string{
		`{"lower_inc":true}`,
		`{"upper_inc":true}`,
	} {
		if err := json.Unmarshal([]byte(bad), &r); err == nil {
			t.Errorf("%s: inclusive bound on an unbounded side accepted", bad)
		}
	}
	var m qt.Multirange[int]
	if err := json.Unmarshal([]byte(`[{"lower":1,"upper":2,"lower_inc":true}]`), &m); err != nil || len(m) != 1 || *m[0].Lower != 1 {
		t.Errorf("multirange: %+v %v", m, err)
	}
	if err := json.Unmarshal([]byte(`null`), &m); err != nil || m != nil {
		t.Errorf("null multirange: %+v %v", m, err)
	}
	if err := json.Unmarshal([]byte(`[]`), &m); err != nil || m == nil || len(m) != 0 {
		t.Errorf("empty multirange: %+v %v", m, err)
	}
}

func TestJSONOpt(t *testing.T) {
	type body struct {
		Text qt.Opt[string]          `json:"text,omitzero"`
		Nick qt.Opt[qt.Null[string]] `json:"nick,omitzero"`
		Keep qt.Opt[int]             `json:"keep,omitzero"`
	}
	var b body
	if err := json.Unmarshal([]byte(`{"text":"x","nick":null,"keep":null}`), &b); err != nil {
		t.Fatal(err)
	}
	if !b.Text.Set || b.Text.V != "x" {
		t.Errorf("text: %+v", b.Text)
	}
	if !b.Nick.Set || b.Nick.V.Valid {
		t.Errorf("an explicit null must set the option: %+v", b.Nick)
	}
	if b.Keep.Set {
		t.Errorf("null on a plain int must leave the option unset: %+v", b.Keep)
	}
	if got, want := mustJSON(t, b), `{"text":"x","nick":null}`; got != want {
		t.Errorf("omitzero: %s, want %s", got, want)
	}
	if !qt.Of(1).IsSet() || qt.Of(1).IsZero() {
		t.Error("a set option is set")
	}
	if qt.Unset[int]().IsSet() || !qt.Unset[int]().IsZero() {
		t.Error("an unset option is not set")
	}
	if got := mustJSON(t, qt.Unset[int]()); got != `null` {
		t.Errorf("unset without omitzero: %s", got)
	}
}
