package qt

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Scan and Value for the value types whose PostgreSQL form is not a plain Go
// scalar. They implement sql.Scanner and driver.Valuer, which pgx honors, so
// these types work with pgx without registering codecs.

// Scan implements sql.Scanner. Numeric accepts text, bytes and Go numbers.
func (n *Numeric) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		return fmt.Errorf("qt: NULL scanned into Numeric; use qt.Null[qt.Numeric]")
	case string:
		*n = Numeric(v)
	case []byte:
		*n = Numeric(v)
	case int64:
		*n = Numeric(strconv.FormatInt(v, 10))
	case float64:
		*n = Numeric(strconv.FormatFloat(v, 'f', -1, 64))
	default:
		return fmt.Errorf("qt: cannot scan %T into Numeric", src)
	}
	return nil
}

// Value implements driver.Valuer; the decimal travels as its text.
func (n Numeric) Value() (driver.Value, error) { return string(n), nil }

// Scan implements sql.Scanner for timestamp without time zone.
func (t *LocalTime) Scan(src any) error {
	switch v := src.(type) {
	case time.Time:
		*t = LocalTime(v)
	case string:
		p, err := parseTimestamp(v)
		if err != nil {
			return err
		}
		*t = LocalTime(p)
	case []byte:
		p, err := parseTimestamp(string(v))
		if err != nil {
			return err
		}
		*t = LocalTime(p)
	default:
		return fmt.Errorf("qt: cannot scan %T into LocalTime", src)
	}
	return nil
}

func parseTimestamp(s string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02 15:04:05.999999", "2006-01-02T15:04:05.999999", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("qt: bad timestamp %q", s)
}

// Value implements driver.Valuer. The wall-clock time is sent without a zone.
func (t LocalTime) Value() (driver.Value, error) {
	return time.Time(t).Format("2006-01-02 15:04:05.999999"), nil
}

// Value implements driver.Valuer using units PostgreSQL always accepts.
func (i Interval) Value() (driver.Value, error) {
	return fmt.Sprintf("%d months %d days %d microseconds", i.Months, i.Days, i.Micros), nil
}

// Scan implements sql.Scanner for the "postgres" and "iso_8601" interval
// output styles (the default is "postgres").
func (i *Interval) Scan(src any) error {
	var s string
	switch v := src.(type) {
	case string:
		s = v
	case []byte:
		s = string(v)
	default:
		return fmt.Errorf("qt: cannot scan %T into Interval", src)
	}
	p, err := ParseInterval(s)
	if err != nil {
		return err
	}
	*i = p
	return nil
}

// ParseInterval parses PostgreSQL's text output of an interval in the
// "postgres" style (1 year 2 mons 3 days -04:05:06.5) and ISO 8601 (P1Y2M3DT4H).
func ParseInterval(s string) (Interval, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "P") {
		return parseISOInterval(s)
	}
	var out Interval
	fields := strings.Fields(s)
	for k := 0; k < len(fields); k++ {
		f := fields[k]
		if strings.Contains(f, ":") { // [-+]HH:MM:SS[.ffffff]
			us, err := parseClock(f)
			if err != nil {
				return Interval{}, err
			}
			out.Micros += us
			continue
		}
		n, err := strconv.ParseInt(f, 10, 64)
		if err != nil || k+1 >= len(fields) {
			return Interval{}, fmt.Errorf("qt: bad interval %q", s)
		}
		k++
		switch strings.TrimSuffix(fields[k], "s") {
		case "year":
			out.Months += int32(n * 12)
		case "mon":
			out.Months += int32(n)
		case "day":
			out.Days += int32(n)
		default:
			return Interval{}, fmt.Errorf("qt: unknown interval unit %q in %q", fields[k], s)
		}
	}
	return out, nil
}

func parseClock(f string) (int64, error) {
	sign := int64(1)
	switch f[0] {
	case '-':
		sign, f = -1, f[1:]
	case '+':
		f = f[1:]
	}
	parts := strings.Split(f, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("qt: bad interval time %q", f)
	}
	h, err1 := strconv.ParseInt(parts[0], 10, 64)
	m, err2 := strconv.ParseInt(parts[1], 10, 64)
	sec, err3 := strconv.ParseFloat(parts[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, fmt.Errorf("qt: bad interval time %q", f)
	}
	us := h*3600_000_000 + m*60_000_000 + int64(sec*1e6+0.5)
	return sign * us, nil
}

func parseISOInterval(s string) (Interval, error) {
	var out Interval
	inTime := false
	num := ""
	for _, r := range s[1:] {
		switch {
		case r == 'T':
			inTime = true
		case r >= '0' && r <= '9' || r == '.' || r == '-':
			num += string(r)
		default:
			f, err := strconv.ParseFloat(num, 64)
			if err != nil {
				return Interval{}, fmt.Errorf("qt: bad ISO interval %q", s)
			}
			num = ""
			switch {
			case r == 'Y' && !inTime:
				out.Months += int32(f * 12)
			case r == 'M' && !inTime:
				out.Months += int32(f)
			case r == 'W' && !inTime:
				out.Days += int32(f * 7)
			case r == 'D' && !inTime:
				out.Days += int32(f)
			case r == 'H' && inTime:
				out.Micros += int64(f * 3600e6)
			case r == 'M' && inTime:
				out.Micros += int64(f * 60e6)
			case r == 'S' && inTime:
				out.Micros += int64(f*1e6 + 0.5)
			default:
				return Interval{}, fmt.Errorf("qt: bad ISO interval %q", s)
			}
		}
	}
	return out, nil
}
