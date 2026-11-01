package qt

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDate(t *testing.T) {
	d := DateOf(2026, time.October, 28)
	if d.String() != "2026-10-28" || d.Year() != 2026 || d.Month() != time.October || d.Day() != 28 {
		t.Fatalf("%v", d)
	}
	if d.AddDays(4).String() != "2026-11-01" || d.AddMonths(4).String() != "2027-02-28" {
		t.Fatalf("%v %v", d.AddDays(4), d.AddMonths(4))
	}
	// the date a zoned time shows, not its UTC date
	loc := time.FixedZone("x", 10*3600)
	if DateFromTime(time.Date(2026, 1, 1, 1, 0, 0, 0, loc)).String() != "2026-01-01" {
		t.Fatal("zone shifted the date")
	}
	if !d.Before(DateInfinity) || !DateNegInfinity.Before(d) || DateInfinity.String() != "infinity" {
		t.Fatal("infinity ordering")
	}
	var back Date
	if err := back.Scan(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)); err != nil || back != d {
		t.Fatal(back, err)
	}
	if err := back.Scan("infinity"); err != nil || back != DateInfinity {
		t.Fatal(back, err)
	}
	b, _ := json.Marshal(struct{ D Date }{d})
	if string(b) != `{"D":"2026-10-28"}` {
		t.Fatal(string(b))
	}
	var s struct{ D Date }
	if err := json.Unmarshal(b, &s); err != nil || s.D != d {
		t.Fatal(s, err)
	}
	if _, err := ParseDate("2026-13-40"); err == nil {
		t.Fatal("invalid date accepted")
	}
	if DateOf(1999, 12, 31).Sub(DateOf(1999, 1, 1)) != 364 {
		t.Fatal("Sub")
	}
}

func TestTimeOfDay(t *testing.T) {
	v := TimeOfDayOf(7, 5, 9, 120000)
	if v.String() != "07:05:09.12" || v.Hour() != 7 || v.Micro() != 120000 {
		t.Fatal(v.String())
	}
	for _, in := range []string{"00:00", "23:59:59.999999", "24:00:00", "07:05:09.12"} {
		p, err := ParseTimeOfDay(in)
		if err != nil {
			t.Fatal(in, err)
		}
		if in == "07:05:09.12" && p != v {
			t.Fatal(p)
		}
	}
	for _, in := range []string{"", "24:00:01", "12:60", "ab:cd", "12:00:00x"} {
		if _, err := ParseTimeOfDay(in); err == nil {
			t.Fatalf("%q accepted", in)
		}
	}
	if !(TimeOfDayOf(1, 0, 0, 0) < TimeOfDayOf(2, 0, 0, 0)) {
		t.Fatal("ordering")
	}
	on := TimeOfDayOf(9, 30, 0, 0).On(DateOf(2026, 10, 28), time.UTC)
	if on.Format(time.RFC3339) != "2026-10-28T09:30:00Z" {
		t.Fatal(on)
	}
}

func TestTimeTZ(t *testing.T) {
	for in, want := range map[string]string{
		"12:00:00+02":       "12:00:00+02:00",
		"12:00:00.5-05:30":  "12:00:00.5-05:30",
		"01:02:03Z":         "01:02:03+00:00",
		"01:02:03+05:30:15": "01:02:03+05:30:15",
	} {
		v, err := ParseTimeTZ(in)
		if err != nil || v.String() != want {
			t.Fatalf("%s: %v %v", in, v, err)
		}
	}
	if _, err := ParseTimeTZ("12:00:00"); err == nil {
		t.Fatal("offset required")
	}
}
