package qlx_test

import (
	"strings"
	"testing"
	"time"

	q "github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/qt"
)

func TestTimestamptzValues(t *testing.T) {
	now := time.Date(2026, 2, 28, 23, 59, 59, 999999000, time.FixedZone("x", 3600))
	a := qt.TimestamptzOf(now)
	if !a.Time().Equal(now) || a.Time().Location() != time.UTC || a.IsInfinite() || a.Sign() != 0 {
		t.Fatalf("finite value: %v", a)
	}
	var zero qt.Timestamptz
	if zero.IsInfinite() || !zero.Time().Equal(time.Unix(0, 0)) {
		t.Fatalf("zero value must be the epoch, not infinity: %v", zero)
	}
	if !qt.TimestamptzInfinity.After(a) || !qt.TimestamptzNegInfinity.Before(a) || !a.Before(qt.TimestamptzInfinity) ||
		!qt.TimestamptzInfinity.Equal(qt.TimestamptzInfinity) || qt.TimestamptzInfinity.Equal(qt.TimestamptzNegInfinity) ||
		!qt.TimestamptzNegInfinity.Before(qt.TimestamptzInfinity) || !a.Equal(qt.TimestamptzOf(now.UTC())) {
		t.Fatal("ordering of infinities")
	}
	for _, c := range []struct {
		in   any
		want qt.Timestamptz
	}{
		{"infinity", qt.TimestamptzInfinity}, {[]byte("-infinity"), qt.TimestamptzNegInfinity},
		{now, a}, {"2026-02-28 22:59:59.999999+00", a}, {"2026-02-28 23:59:59.999999+01", a}, {"2026-02-28T23:59:59.999999+01:00", a},
	} {
		var got qt.Timestamptz
		if err := got.Scan(c.in); err != nil || !got.Equal(c.want) {
			t.Errorf("Scan(%v) = %v, %v; want %v", c.in, got, err, c.want)
		}
	}
	var bad qt.Timestamptz
	if bad.Scan(nil) == nil || bad.Scan("yesterday") == nil || bad.Scan(3) == nil {
		t.Error("Scan accepted NULL, garbage or an int")
	}
	if v, _ := qt.TimestamptzInfinity.Value(); v != "infinity" {
		t.Errorf("Value(infinity) = %v", v)
	}
	if v, _ := a.Value(); !v.(time.Time).Equal(now) {
		t.Errorf("Value(finite) = %v", v)
	}
	b, _ := a.MarshalText()
	var back qt.Timestamptz
	if err := back.UnmarshalText(b); err != nil || !back.Equal(a) {
		t.Errorf("text round trip: %s %v", b, err)
	}
}

func TestTimestamptzInExpressions(t *testing.T) {
	tbl := q.TableRef{Schema: "public", Name: "licences", PK: []string{"id"}}
	until := q.NewTimeCol[qt.Timestamptz](tbl, "valid_until")
	st, err := q.Render(q.Dialect{Version: q.PG18}, q.Select(until).From(tbl).
		Where(until.Gt(qt.TimestamptzOf(time.Unix(0, 0))), until.LtX(q.NowTZ()), until.Lt(qt.TimestamptzInfinity)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.SQL, `< now()`) || !strings.Contains(st.SQL, "$2") || strings.Contains(st.SQL, "$3") {
		t.Errorf("SQL (want 2 parameters and a comparison with now()): %s", st.SQL)
	}
	// the time.Time column keeps comparing with Now()
	ts := q.NewTimeCol[time.Time](tbl, "created_at")
	if _, err := q.Render(q.Dialect{Version: q.PG18}, q.Select(ts).From(tbl).Where(ts.GtX(q.Now()))); err != nil {
		t.Fatal(err)
	}
}
