package qt

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// errIntervalRange reports an interval field that does not fit its Go type.
// The parsers reject it instead of wrapping around.
var errIntervalRange = errors.New("qt: interval out of range")

// addInt64 adds two counts, failing instead of overflowing.
func addInt64(a, b int64) (int64, error) {
	if b > 0 && a > math.MaxInt64-b || b < 0 && a < math.MinInt64-b {
		return 0, errIntervalRange
	}
	return a + b, nil
}

// scaleInt64 multiplies a count by a positive factor, failing instead of
// overflowing.
func scaleInt64(n, factor int64) (int64, error) {
	if n > math.MaxInt64/factor || n < math.MinInt64/factor {
		return 0, errIntervalRange
	}
	return n * factor, nil
}

// intervalInt32 narrows a count to the int32 fields of Interval, failing
// instead of truncating.
func intervalInt32(n int64) (int32, error) {
	if n < math.MinInt32 || n > math.MaxInt32 {
		return 0, errIntervalRange
	}
	return int32(n), nil
}

// intervalFloat converts a fractional count to int64, rounding when asked,
// and fails instead of producing an implementation-defined value.
func intervalFloat(f, scale float64, round bool) (int64, error) {
	v := f * scale
	if round {
		v = math.Round(v)
	}
	if math.IsNaN(v) || v < float64(math.MinInt64) || v >= float64(math.MaxInt64) {
		return 0, errIntervalRange
	}
	return int64(v), nil
}

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
	var months, days int64
	fields := strings.Fields(s)
	for k := 0; k < len(fields); k++ {
		f := fields[k]
		if strings.Contains(f, ":") { // [-+]HH:MM:SS[.ffffff]
			us, err := parseClock(f)
			if err != nil {
				return Interval{}, err
			}
			out.Micros, err = addInt64(out.Micros, us)
			if err != nil {
				return Interval{}, fmt.Errorf("%w in %q", errIntervalRange, s)
			}
			continue
		}
		n, err := strconv.ParseInt(f, 10, 64)
		if err != nil || k+1 >= len(fields) {
			return Interval{}, fmt.Errorf("qt: bad interval %q", s)
		}
		k++
		var add int64
		switch strings.TrimSuffix(fields[k], "s") {
		case "year":
			add, err = scaleInt64(n, 12)
			if err == nil {
				months, err = addInt64(months, add)
			}
		case "mon":
			months, err = addInt64(months, n)
		case "day":
			days, err = addInt64(days, n)
		default:
			return Interval{}, fmt.Errorf("qt: unknown interval unit %q in %q", fields[k], s)
		}
		if err != nil {
			return Interval{}, fmt.Errorf("%w in %q", errIntervalRange, s)
		}
	}
	var err error
	if out.Months, err = intervalInt32(months); err != nil {
		return Interval{}, fmt.Errorf("%w in %q", errIntervalRange, s)
	}
	if out.Days, err = intervalInt32(days); err != nil {
		return Interval{}, fmt.Errorf("%w in %q", errIntervalRange, s)
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
	var months, days int64
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
			var add int64
			switch {
			case r == 'Y' && !inTime:
				add, err = intervalFloat(f, 12, false)
				if err == nil {
					months, err = addInt64(months, add)
				}
			case r == 'M' && !inTime:
				add, err = intervalFloat(f, 1, false)
				if err == nil {
					months, err = addInt64(months, add)
				}
			case r == 'W' && !inTime:
				add, err = intervalFloat(f, 7, false)
				if err == nil {
					days, err = addInt64(days, add)
				}
			case r == 'D' && !inTime:
				add, err = intervalFloat(f, 1, false)
				if err == nil {
					days, err = addInt64(days, add)
				}
			case r == 'H' && inTime:
				add, err = intervalFloat(f, 3600e6, false)
				if err == nil {
					out.Micros, err = addInt64(out.Micros, add)
				}
			case r == 'M' && inTime:
				add, err = intervalFloat(f, 60e6, false)
				if err == nil {
					out.Micros, err = addInt64(out.Micros, add)
				}
			case r == 'S' && inTime:
				add, err = intervalFloat(f, 1e6, true)
				if err == nil {
					out.Micros, err = addInt64(out.Micros, add)
				}
			default:
				return Interval{}, fmt.Errorf("qt: bad ISO interval %q", s)
			}
			if err != nil {
				return Interval{}, fmt.Errorf("%w in %q", errIntervalRange, s)
			}
		}
	}
	var err error
	if out.Months, err = intervalInt32(months); err != nil {
		return Interval{}, fmt.Errorf("%w in %q", errIntervalRange, s)
	}
	if out.Days, err = intervalInt32(days); err != nil {
		return Interval{}, fmt.Errorf("%w in %q", errIntervalRange, s)
	}
	return out, nil
}
