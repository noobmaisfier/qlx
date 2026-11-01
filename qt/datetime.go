package qt

import (
	"database/sql/driver"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Date
// ---------------------------------------------------------------------------

// Date is a calendar date: PostgreSQL's date. It has no time of day and no
// time zone, which is why it is not a time.Time (a time.Time invents a
// midnight and a zone, and a date round-tripped through the wrong zone moves by
// a day). It is stored as the number of days since 1970-01-01, so it is
// comparable with == and < and costs four bytes.
type Date struct{ days int32 }

const (
	dateInf    = math.MaxInt32
	dateNegInf = math.MinInt32
)

// Infinity and NegInfinity are PostgreSQL's 'infinity' and '-infinity' dates.
var (
	DateInfinity    = Date{dateInf}
	DateNegInfinity = Date{dateNegInf}
)

// DateOf builds a date; out-of-range months and days normalise like time.Date.
func DateOf(year int, month time.Month, day int) Date {
	return DateFromTime(time.Date(year, month, day, 0, 0, 0, 0, time.UTC))
}

// DateFromTime takes the calendar date t shows in its own location.
func DateFromTime(t time.Time) Date {
	y, m, d := t.Date()
	u := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return Date{int32(u.Unix() / 86400)}
}

// Today is the current date in loc.
func Today(loc *time.Location) Date { return DateFromTime(time.Now().In(loc)) }

// ParseDate parses YYYY-MM-DD (and "infinity" / "-infinity").
func ParseDate(s string) (Date, error) {
	switch s {
	case "infinity":
		return DateInfinity, nil
	case "-infinity":
		return DateNegInfinity, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return Date{}, fmt.Errorf("qt: invalid date %q", s)
	}
	return DateFromTime(t), nil
}

// IsInfinite reports whether d is infinity or -infinity.
func (d Date) IsInfinite() bool { return d.days == dateInf || d.days == dateNegInf }

// Time is midnight UTC on d. Infinite dates have no Time; the zero Time is returned.
func (d Date) Time() time.Time {
	if d.IsInfinite() {
		return time.Time{}
	}
	return time.Unix(int64(d.days)*86400, 0).UTC()
}

// In is midnight on d in loc, the instant a person in loc means by "that day".
func (d Date) In(loc *time.Location) time.Time {
	t := d.Time()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

// Year, Month, Day and Weekday read the calendar fields.
func (d Date) Year() int             { return d.Time().Year() }
func (d Date) Month() time.Month     { return d.Time().Month() }
func (d Date) Day() int              { return d.Time().Day() }
func (d Date) Weekday() time.Weekday { return d.Time().Weekday() }

// AddDays moves by n days; infinite dates stay infinite.
func (d Date) AddDays(n int) Date {
	if d.IsInfinite() {
		return d
	}
	return Date{d.days + int32(n)}
}

// AddMonths moves by n calendar months with time.AddDate's normalisation.
func (d Date) AddMonths(n int) Date {
	if d.IsInfinite() {
		return d
	}
	return DateFromTime(d.Time().AddDate(0, n, 0))
}

// Sub is the number of days from o to d.
func (d Date) Sub(o Date) int { return int(d.days) - int(o.days) }

// Before, After, Equal and Compare order dates; infinity sorts last, -infinity first.
func (d Date) Before(o Date) bool { return d.days < o.days }
func (d Date) After(o Date) bool  { return d.days > o.days }
func (d Date) Equal(o Date) bool  { return d.days == o.days }
func (d Date) Compare(o Date) int {
	switch {
	case d.days < o.days:
		return -1
	case d.days > o.days:
		return 1
	}
	return 0
}

// IsZero is the Unix epoch day, 1970-01-01. Use Null[Date] for "no date".
func (d Date) IsZero() bool { return d.days == 0 }

// String is the ISO form PostgreSQL accepts.
func (d Date) String() string {
	switch d.days {
	case dateInf:
		return "infinity"
	case dateNegInf:
		return "-infinity"
	}
	return d.Time().Format("2006-01-02")
}

// MarshalText and UnmarshalText make Date a JSON string "2006-01-02".
func (d Date) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// UnmarshalText parses the ISO form.
func (d *Date) UnmarshalText(b []byte) error {
	v, err := ParseDate(string(b))
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// Value implements driver.Valuer: a time.Time at midnight UTC, which the
// date codec of pgx and database/sql both accept; infinity as its text.
func (d Date) Value() (driver.Value, error) {
	if d.IsInfinite() {
		return d.String(), nil
	}
	return d.Time(), nil
}

// QLXArg is the argument form pgx sees.
func (d Date) QLXArg() any {
	v, _ := d.Value()
	return v
}

// Scan implements sql.Scanner for time.Time, string and []byte sources.
func (d *Date) Scan(src any) error {
	switch v := src.(type) {
	case time.Time:
		*d = DateFromTime(v)
	case string:
		x, err := ParseDate(v)
		if err != nil {
			return err
		}
		*d = x
	case []byte:
		x, err := ParseDate(string(v))
		if err != nil {
			return err
		}
		*d = x
	case nil:
		return fmt.Errorf("qt: NULL into Date (use Null[Date])")
	default:
		return fmt.Errorf("qt: cannot scan %T into Date", src)
	}
	return nil
}

// ---------------------------------------------------------------------------
// TimeOfDay
// ---------------------------------------------------------------------------

// TimeOfDay is PostgreSQL's time (without time zone): microseconds since
// midnight, 0 up to and including 24:00:00. Comparable and ordered with the
// built-in operators.
type TimeOfDay int64

const microsPerDay = 86400 * 1_000_000

// TimeOfDayOf builds a time of day from its parts.
func TimeOfDayOf(hour, min, sec, micro int) TimeOfDay {
	return TimeOfDay(int64(hour)*3600_000_000 + int64(min)*60_000_000 + int64(sec)*1_000_000 + int64(micro))
}

// TimeOfDayFromTime takes the clock reading t shows in its own location.
func TimeOfDayFromTime(t time.Time) TimeOfDay {
	return TimeOfDayOf(t.Hour(), t.Minute(), t.Second(), t.Nanosecond()/1000)
}

// Hour, Minute, Second and Micro read the parts.
func (t TimeOfDay) Hour() int   { return int(int64(t) / 3600_000_000) }
func (t TimeOfDay) Minute() int { return int(int64(t) / 60_000_000 % 60) }
func (t TimeOfDay) Second() int { return int(int64(t) / 1_000_000 % 60) }
func (t TimeOfDay) Micro() int  { return int(int64(t) % 1_000_000) }

// Duration is the time since midnight.
func (t TimeOfDay) Duration() time.Duration { return time.Duration(t) * time.Microsecond }

// On combines the time with a date in loc.
func (t TimeOfDay) On(d Date, loc *time.Location) time.Time {
	b := d.Time()
	return time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, loc).Add(t.Duration())
}

// String is HH:MM:SS with fractional seconds only when present.
func (t TimeOfDay) String() string {
	s := fmt.Sprintf("%02d:%02d:%02d", t.Hour(), t.Minute(), t.Second())
	if us := t.Micro(); us != 0 {
		s += strings.TrimRight(fmt.Sprintf(".%06d", us), "0")
	}
	return s
}

// ParseTimeOfDay parses HH:MM[:SS[.ffffff]].
func ParseTimeOfDay(s string) (TimeOfDay, error) {
	v, rest, err := parseClockOfDay(s)
	if err != nil || rest != "" {
		return 0, fmt.Errorf("qt: invalid time %q", s)
	}
	return v, nil
}

func parseClockOfDay(s string) (TimeOfDay, string, error) {
	bad := fmt.Errorf("qt: invalid time %q", s)
	if len(s) < 5 || s[2] != ':' {
		return 0, "", bad
	}
	h, err1 := strconv.Atoi(s[0:2])
	m, err2 := strconv.Atoi(s[3:5])
	if err1 != nil || err2 != nil || m > 59 {
		return 0, "", bad
	}
	rest := s[5:]
	sec, us := 0, 0
	if strings.HasPrefix(rest, ":") {
		if len(rest) < 3 {
			return 0, "", bad
		}
		var err error
		if sec, err = strconv.Atoi(rest[1:3]); err != nil || sec > 60 {
			return 0, "", bad
		}
		rest = rest[3:]
		if strings.HasPrefix(rest, ".") {
			i := 1
			for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
				i++
			}
			frac := rest[1:i]
			if frac == "" || len(frac) > 6 {
				frac = (frac + "000000")[:6]
			}
			frac = (frac + "000000")[:6]
			if us, err = strconv.Atoi(frac); err != nil {
				return 0, "", bad
			}
			rest = rest[i:]
		}
	}
	v := TimeOfDayOf(h, m, sec, us)
	if v < 0 || v > microsPerDay {
		return 0, "", bad
	}
	return v, rest, nil
}

// MarshalText and UnmarshalText make TimeOfDay a JSON string.
func (t TimeOfDay) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

// UnmarshalText parses HH:MM:SS[.ffffff].
func (t *TimeOfDay) UnmarshalText(b []byte) error {
	v, err := ParseTimeOfDay(string(b))
	if err != nil {
		return err
	}
	*t = v
	return nil
}

// Value implements driver.Valuer as PostgreSQL's text form.
func (t TimeOfDay) Value() (driver.Value, error) { return t.String(), nil }

// QLXArg is the argument form pgx sees.
func (t TimeOfDay) QLXArg() any { return t.String() }

// Scan implements sql.Scanner for string, []byte and time.Time sources.
func (t *TimeOfDay) Scan(src any) error {
	switch v := src.(type) {
	case string:
		x, err := ParseTimeOfDay(v)
		if err != nil {
			return err
		}
		*t = x
	case []byte:
		x, err := ParseTimeOfDay(string(v))
		if err != nil {
			return err
		}
		*t = x
	case time.Time:
		*t = TimeOfDayFromTime(v)
	case int64:
		*t = TimeOfDay(v)
	default:
		return fmt.Errorf("qt: cannot scan %T into TimeOfDay", src)
	}
	return nil
}

// ---------------------------------------------------------------------------
// TimeTZ
// ---------------------------------------------------------------------------

// TimeTZ is PostgreSQL's timetz: a time of day with a UTC offset. PostgreSQL
// itself discourages the type (offsets without a date are ambiguous); it is
// carried faithfully for schemas that have it.
type TimeTZ struct {
	Time TimeOfDay
	// Offset is seconds east of UTC.
	Offset int32
}

// String is HH:MM:SS[.ffffff]±HH:MM.
func (t TimeTZ) String() string {
	off := t.Offset
	sign := '+'
	if off < 0 {
		sign, off = '-', -off
	}
	s := fmt.Sprintf("%s%c%02d:%02d", t.Time, sign, off/3600, off%3600/60)
	if off%60 != 0 {
		s += fmt.Sprintf(":%02d", off%60)
	}
	return s
}

// ParseTimeTZ parses HH:MM:SS[.ffffff] followed by ±HH[:MM[:SS]] or Z.
func ParseTimeTZ(s string) (TimeTZ, error) {
	bad := fmt.Errorf("qt: invalid timetz %q", s)
	clock, rest, err := parseClockOfDay(s)
	if err != nil {
		return TimeTZ{}, bad
	}
	if rest == "Z" || rest == "z" {
		return TimeTZ{Time: clock}, nil
	}
	if len(rest) < 3 || (rest[0] != '+' && rest[0] != '-') {
		return TimeTZ{}, bad
	}
	sign := int32(1)
	if rest[0] == '-' {
		sign = -1
	}
	parts := strings.Split(rest[1:], ":")
	if len(parts) > 3 {
		return TimeTZ{}, bad
	}
	var secs int32
	mult := []int32{3600, 60, 1}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 59 && i > 0 {
			return TimeTZ{}, bad
		}
		secs += int32(n) * mult[i]
	}
	return TimeTZ{Time: clock, Offset: sign * secs}, nil
}

// MarshalText and UnmarshalText make TimeTZ a JSON string.
func (t TimeTZ) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

// UnmarshalText parses the text form.
func (t *TimeTZ) UnmarshalText(b []byte) error {
	v, err := ParseTimeTZ(string(b))
	if err != nil {
		return err
	}
	*t = v
	return nil
}

// Value implements driver.Valuer as PostgreSQL's text form.
func (t TimeTZ) Value() (driver.Value, error) { return t.String(), nil }

// QLXArg is the argument form pgx sees.
func (t TimeTZ) QLXArg() any { return t.String() }

// Scan implements sql.Scanner.
func (t *TimeTZ) Scan(src any) error {
	var s string
	switch v := src.(type) {
	case string:
		s = v
	case []byte:
		s = string(v)
	default:
		return fmt.Errorf("qt: cannot scan %T into TimeTZ", src)
	}
	x, err := ParseTimeTZ(s)
	if err != nil {
		return err
	}
	*t = x
	return nil
}
