package qt

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/noobmaisfier/qlx/internal/textval"
)

// The JSON forms of the types that carry optionality or a PostgreSQL-only
// value. Their job is to be the contract of an HTTP body (see docs/dtos.md):
// a NULL, an absent key and an empty value must stay distinguishable.

// IsSet reports whether the option carries a value, including an explicit
// NULL (an Opt[Null[T]] whose null was present in the body).
func (o Opt[T]) IsSet() bool { return o.Set }

// IsZero reports whether the option is unset. encoding/json uses it for
// `omitzero`, so an unset field leaves no key behind.
func (o Opt[T]) IsZero() bool { return !o.Set }

// ErrUnsetOptNull is returned when an unset Opt[Null[T]] is marshaled. Its JSON
// form would be null, which reads back as an explicit NULL, so a patch that
// meant "leave it alone" would become "set it to NULL". Tag the field
// `json:"name,omitzero"`: the key is then left out.
var ErrUnsetOptNull = errors.New("qt: unset Opt[Null[T]] marshaled without omitzero: add ,omitzero to its json tag")

// MarshalJSON writes null for an unset option and the value otherwise. An
// unset Opt[Null[T]] is an error (ErrUnsetOptNull): null would be read back as
// a set NULL.
func (o Opt[T]) MarshalJSON() ([]byte, error) {
	if !o.Set {
		var v T
		if _, ok := any(&v).(interface{ holdsNull() }); ok {
			return nil, ErrUnsetOptNull
		}
		return []byte("null"), nil
	}
	return json.Marshal(o.V)
}

// UnmarshalJSON sets the option when the key is present. A JSON null sets it
// only when T can hold NULL (a qt.Null[T]): for a plain value it leaves the
// option unset, as encoding/json ignores null for a string or an int.
func (o *Opt[T]) UnmarshalJSON(b []byte) error {
	if isJSONNull(b) {
		var v T
		if _, ok := any(&v).(interface{ holdsNull() }); ok {
			if err := json.Unmarshal(b, &v); err != nil {
				return err
			}
			*o = Opt[T]{V: v, Set: true}
			return nil
		}
		*o = Opt[T]{}
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*o = Opt[T]{V: v, Set: true}
	return nil
}

// UnmarshalText sets the option from a text value, so a URL query parameter
// (or any other text source) fills an Opt without a special case: the option
// is set with the parsed value. It reads what qjson.DecodeQuery reads: a
// string, a bool, an integer, a float, a time.Duration, or any T whose pointer
// implements encoding.TextUnmarshaler (qt.ID, qt.Date, time.Time...). Empty
// text is a value like any other: an absent parameter is the unset option, and
// qjson.DecodeQuery never calls UnmarshalText for it. An Opt[Null[T]] is not
// supported: text has no NULL.
func (o *Opt[T]) UnmarshalText(b []byte) error {
	var v T
	if _, ok := any(&v).(interface{ holdsNull() }); ok {
		return errors.New("qt: Opt[Null[T]] cannot be read from text")
	}
	if err := textval.Set(reflect.ValueOf(&v).Elem(), string(b)); err != nil {
		return err
	}
	*o = Opt[T]{V: v, Set: true}
	return nil
}

// String is the ISO 8601 duration form, e.g. P1Y2M3DT1H. Months are split
// into years and months (12 months = 1 year); they are not converted to days.
func (i Interval) String() string {
	if i == (Interval{}) {
		return "PT0S"
	}
	var b strings.Builder
	b.WriteByte('P')
	writeUnit(&b, int64(i.Months/12), 'Y')
	writeUnit(&b, int64(i.Months%12), 'M')
	writeUnit(&b, int64(i.Days), 'D')
	if i.Micros != 0 {
		b.WriteByte('T')
		us := i.Micros
		sign := ""
		if us < 0 {
			sign, us = "-", -us
		}
		h := us / 3_600_000_000
		us %= 3_600_000_000
		m := us / 60_000_000
		us %= 60_000_000
		if h != 0 {
			fmt.Fprintf(&b, "%s%dH", sign, h)
		}
		if m != 0 {
			fmt.Fprintf(&b, "%s%dM", sign, m)
		}
		if us != 0 {
			b.WriteString(sign)
			b.WriteString(strconv.FormatFloat(float64(us)/1e6, 'f', -1, 64))
			b.WriteByte('S')
		}
	}
	return b.String()
}

func writeUnit(b *strings.Builder, n int64, unit byte) {
	if n != 0 {
		fmt.Fprintf(b, "%d%c", n, unit)
	}
}

// MarshalText makes an interval a JSON string in ISO 8601 form.
func (i Interval) MarshalText() ([]byte, error) { return []byte(i.String()), nil }

// UnmarshalText accepts ISO 8601 and PostgreSQL's own text form.
func (i *Interval) UnmarshalText(b []byte) error {
	v, err := ParseInterval(string(b))
	if err != nil {
		return err
	}
	*i = v
	return nil
}

// MarshalJSON writes {"empty":true}, null for an SQL NULL, or the bounds. The
// field names are part of the public contract (snake_case, unlike the Go
// fields of Range).
func (r Range[T]) MarshalJSON() ([]byte, error) {
	switch {
	case r.Null:
		return []byte("null"), nil
	case r.Empty:
		return []byte(`{"empty":true}`), nil
	}
	return json.Marshal(struct {
		Lower    *T   `json:"lower,omitempty"`
		Upper    *T   `json:"upper,omitempty"`
		LowerInc bool `json:"lower_inc"`
		UpperInc bool `json:"upper_inc"`
	}{r.Lower, r.Upper, r.LowerInc, r.UpperInc})
}

type rangeJSON[T any] struct {
	Lower    *T   `json:"lower"`
	Upper    *T   `json:"upper"`
	LowerInc bool `json:"lower_inc"`
	UpperInc bool `json:"upper_inc"`
	Empty    bool `json:"empty"`
}

// UnmarshalJSON reads the form MarshalJSON writes. An inclusive bound on an
// unbounded side is rejected: it cannot mean anything, and PostgreSQL prints
// it as exclusive.
func (r *Range[T]) UnmarshalJSON(b []byte) error {
	if isJSONNull(b) {
		*r = Range[T]{Null: true}
		return nil
	}
	var v rangeJSON[T]
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v.Empty {
		*r = Range[T]{Empty: true}
		return nil
	}
	if v.Lower == nil && v.LowerInc {
		return fmt.Errorf("qt: range lower bound is inclusive but unbounded")
	}
	if v.Upper == nil && v.UpperInc {
		return fmt.Errorf("qt: range upper bound is inclusive but unbounded")
	}
	*r = Range[T]{Lower: v.Lower, Upper: v.Upper, LowerInc: v.LowerInc, UpperInc: v.UpperInc}
	return nil
}

// MarshalJSON writes an array of ranges, or null for the SQL NULL multirange.
func (m Multirange[T]) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return json.Marshal([]Range[T](m))
}

// UnmarshalJSON reads an array of ranges; null is the SQL NULL multirange.
func (m *Multirange[T]) UnmarshalJSON(b []byte) error {
	if isJSONNull(b) {
		*m = nil
		return nil
	}
	var rs []Range[T]
	if err := json.Unmarshal(b, &rs); err != nil {
		return err
	}
	*m = rs
	return nil
}

func isJSONNull(b []byte) bool {
	return bytes.Equal(bytes.TrimSpace(b), []byte("null"))
}

// JSONInner, JSONKeys: shape hints for qjson's strict decoder. encoding/json
// calls UnmarshalJSON with a fresh decoder, so DisallowUnknownFields does not
// reach the value inside a wrapper; qjson reads these hints to check the
// nested keys itself. They are duck-typed, so qt does not import qjson.

// JSONInner is the type of the JSON value an Opt holds.
func (Opt[T]) JSONInner() reflect.Type { return reflect.TypeFor[T]() }

// JSONInner is the type of the JSON value a Null holds.
func (Null[T]) JSONInner() reflect.Type { return reflect.TypeFor[T]() }

// JSONInner is the type of the JSON document a JSONB holds.
func (JSONB[T]) JSONInner() reflect.Type { return reflect.TypeFor[T]() }

// JSONKeys lists the keys of the object form of a Range.
func (Range[T]) JSONKeys() []string {
	return []string{"lower", "upper", "lower_inc", "upper_inc", "empty"}
}

// JSONInner is the object shape of a Range so strict decoding also checks its
// lower and upper bounds.
func (Range[T]) JSONInner() reflect.Type { return reflect.TypeFor[rangeJSON[T]]() }

// JSONInner is the type of the JSON value a Multirange holds: an array of
// ranges.
func (Multirange[T]) JSONInner() reflect.Type { return reflect.TypeFor[[]Range[T]]() }
