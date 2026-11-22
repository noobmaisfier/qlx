// Package textval parses one text value (a URL query parameter, a path
// segment) into a Go value. It is shared by qjson.DecodeQuery and by
// qt.Opt.UnmarshalText so both accept exactly the same text.
package textval

import (
	"encoding"
	"errors"
	"reflect"
	"strconv"
	"time"
)

// Error is a refused value. Want names what was expected ("an integer");
// it is empty when the value's own UnmarshalText refused it, and Err then
// holds that error.
type Error struct {
	Want string
	Err  error
}

func (e *Error) Error() string {
	if e.Want != "" {
		return "must be " + e.Want
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "invalid value"
}

// Unwrap returns the underlying error.
func (e *Error) Unwrap() error { return e.Err }

var (
	textType     = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	durationType = reflect.TypeOf(time.Duration(0))
)

// Supported reports whether Set can fill a value of type t.
func Supported(t reflect.Type) bool {
	if reflect.PointerTo(t).Implements(textType) {
		return true
	}
	if t == durationType {
		return true
	}
	switch t.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	case reflect.Pointer:
		return Supported(t.Elem())
	}
	return false
}

// Set parses raw into v, which must be settable. A pointer is allocated.
// A type with its own UnmarshalText uses it; time.Duration is read with
// time.ParseDuration ("90s", "1h30m").
func Set(v reflect.Value, raw string) error {
	if v.Kind() == reflect.Pointer {
		elem := reflect.New(v.Type().Elem())
		if err := Set(elem.Elem(), raw); err != nil {
			return err
		}
		v.Set(elem)
		return nil
	}
	if v.CanAddr() {
		if u, ok := v.Addr().Interface().(encoding.TextUnmarshaler); ok {
			if err := u.UnmarshalText([]byte(raw)); err != nil {
				return &Error{Err: err}
			}
			return nil
		}
	}
	if v.Type() == durationType {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return &Error{Want: "a duration such as 90s or 1h30m", Err: err}
		}
		v.SetInt(int64(d))
		return nil
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return &Error{Want: "a boolean", Err: err}
		}
		v.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, v.Type().Bits())
		if err != nil {
			return &Error{Want: "an integer", Err: unwrapNum(err)}
		}
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, v.Type().Bits())
		if err != nil {
			return &Error{Want: "a non-negative integer", Err: unwrapNum(err)}
		}
		v.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(raw, v.Type().Bits())
		if err != nil {
			return &Error{Want: "a number", Err: unwrapNum(err)}
		}
		v.SetFloat(f)
	default:
		return &Error{Err: errors.New("unsupported type " + v.Type().String())}
	}
	return nil
}

func unwrapNum(err error) error {
	var ne *strconv.NumError
	if errors.As(err, &ne) {
		return ne.Err
	}
	return err
}
