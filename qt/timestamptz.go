package qt

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// Timestamptz is PostgreSQL's timestamptz with its two special values.
// time.Time cannot hold 'infinity' or '-infinity' (reading such a row into a
// time.Time fails), which is why columns that use them as "never expires" or
// "since forever" need this type: it is the default for `timestamptz` columns (opt out with `timestamptz: stdlib`).
//
// A finite value is an instant (the zone it was built in is irrelevant, as in
// the database). The zero Timestamptz is the Unix epoch, not infinity. Compare
// with Equal, not ==: the zero value and TimestamptzOf(epoch) are Equal but not
// identical.
type Timestamptz struct {
	t   time.Time
	set bool // t is meaningful; false for the zero value, which reads as the epoch
	inf int8 // 1 = infinity, -1 = -infinity
}

// Infinity and NegInfinity are PostgreSQL's 'infinity' and '-infinity'.
var (
	TimestamptzInfinity    = Timestamptz{inf: 1}
	TimestamptzNegInfinity = Timestamptz{inf: -1}
)

// TimestamptzOf wraps an instant.
func TimestamptzOf(t time.Time) Timestamptz { return Timestamptz{t: t.UTC(), set: true} }

// Time is the instant, in UTC. For an infinite value it is the zero time.Time;
// check IsInfinite first.
func (x Timestamptz) Time() time.Time {
	if x.inf != 0 {
		return time.Time{}
	}
	if !x.set {
		return time.Unix(0, 0).UTC()
	}
	return x.t
}

// IsInfinite reports whether x is 'infinity' or '-infinity'.
func (x Timestamptz) IsInfinite() bool { return x.inf != 0 }

// Sign is 1 for infinity, -1 for -infinity and 0 for a finite value.
func (x Timestamptz) Sign() int { return int(x.inf) }

// Before reports whether x is earlier than y; -infinity is before everything
// else and infinity after.
func (x Timestamptz) Before(y Timestamptz) bool { return x.cmp(y) < 0 }

// After reports whether x is later than y.
func (x Timestamptz) After(y Timestamptz) bool { return x.cmp(y) > 0 }

// Equal reports whether x and y are the same instant or the same infinity.
func (x Timestamptz) Equal(y Timestamptz) bool { return x.cmp(y) == 0 }

func (x Timestamptz) cmp(y Timestamptz) int {
	if x.inf != 0 || y.inf != 0 {
		switch {
		case x.inf == y.inf:
			return 0
		case x.inf < y.inf:
			return -1
		}
		return 1
	}
	switch a, b := x.Time(), y.Time(); {
	case a.Before(b):
		return -1
	case a.After(b):
		return 1
	}
	return 0
}

// String is the PostgreSQL text form: infinity, -infinity or RFC 3339.
func (x Timestamptz) String() string {
	switch x.inf {
	case 1:
		return "infinity"
	case -1:
		return "-infinity"
	}
	return x.Time().Format(time.RFC3339Nano)
}

// Value implements driver.Valuer: a time.Time, or the text of an infinity.
func (x Timestamptz) Value() (driver.Value, error) {
	if x.inf != 0 {
		return x.String(), nil
	}
	return x.Time(), nil
}

// QLXArg is the argument form pgx sees.
func (x Timestamptz) QLXArg() any {
	v, _ := x.Value()
	return v
}

// Scan implements sql.Scanner for time.Time, string and []byte sources.
func (x *Timestamptz) Scan(src any) error {
	switch v := src.(type) {
	case time.Time:
		*x = TimestamptzOf(v)
	case string:
		return x.parse(v)
	case []byte:
		return x.parse(string(v))
	case nil:
		return fmt.Errorf("qt: NULL into Timestamptz (use Null[Timestamptz])")
	default:
		return fmt.Errorf("qt: cannot scan %T into Timestamptz", src)
	}
	return nil
}

func (x *Timestamptz) parse(s string) error {
	switch s {
	case "infinity":
		*x = TimestamptzInfinity
		return nil
	case "-infinity":
		*x = TimestamptzNegInfinity
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999Z07:00", "2006-01-02 15:04:05.999999999Z07"} {
		if t, err := time.Parse(layout, s); err == nil {
			*x = TimestamptzOf(t)
			return nil
		}
	}
	return fmt.Errorf("qt: invalid timestamptz %q", s)
}

// MarshalText encodes infinity as "infinity" and an instant as RFC 3339.
func (x Timestamptz) MarshalText() ([]byte, error) { return []byte(x.String()), nil }

// UnmarshalText is the inverse of MarshalText.
func (x *Timestamptz) UnmarshalText(b []byte) error { return x.parse(string(b)) }
