// Package pgtext renders Go values in PostgreSQL's text form, for the places
// where the driver has no type OID to pick a codec: ranges, multiranges and
// arrays of qt types sent over the simple protocol.
package pgtext

import (
	"database/sql/driver"
	"fmt"
	"math"
	"net/netip"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type argUnwrapper interface{ QLXArg() any }

// Elem returns the text form of v as it would appear as an element of an
// array or a bound of a range, unquoted. null reports SQL NULL.
func Elem(v any) (text string, null bool, err error) {
	for depth := 0; depth < 4; depth++ {
		switch x := v.(type) {
		case nil:
			return "", true, nil
		case argUnwrapper:
			v = x.QLXArg()
			continue
		case driver.Valuer:
			// A nil pointer inside a Valuer is NULL, as database/sql treats it.
			if rv := reflect.ValueOf(v); rv.Kind() == reflect.Pointer && rv.IsNil() {
				return "", true, nil
			}
			dv, derr := x.Value()
			if derr != nil {
				return "", false, derr
			}
			if _, again := dv.(driver.Valuer); again {
				return "", false, fmt.Errorf("pgtext: %T.Value returned a Valuer", v)
			}
			v = dv
			continue
		}
		break
	}
	switch x := v.(type) {
	case nil:
		return "", true, nil
	case string:
		return x, false, nil
	case []byte:
		return `\x` + fmt.Sprintf("%x", x), false, nil
	case bool:
		if x {
			return "t", false, nil
		}
		return "f", false, nil
	case int:
		return strconv.FormatInt(int64(x), 10), false, nil
	case int8, int16, int32, int64:
		return strconv.FormatInt(reflect.ValueOf(x).Int(), 10), false, nil
	case uint, uint8, uint16, uint32, uint64:
		return strconv.FormatUint(reflect.ValueOf(x).Uint(), 10), false, nil
	case float32:
		return float(float64(x), 32), false, nil
	case float64:
		return float(x, 64), false, nil
	case time.Time:
		return x.Format("2006-01-02 15:04:05.999999999Z07:00"), false, nil
	case [16]byte:
		h := fmt.Sprintf("%x", x[:])
		return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], false, nil
	case netip.Addr:
		return x.String(), false, nil
	case netip.Prefix:
		return x.String(), false, nil
	case fmt.Stringer:
		// A nil pointer whose type has a value-receiver String would panic.
		if rv := reflect.ValueOf(v); rv.Kind() == reflect.Pointer && rv.IsNil() {
			return "", true, nil
		}
		return x.String(), false, nil
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			return "", true, nil
		}
		return Elem(rv.Elem().Interface())
	case reflect.String:
		return rv.String(), false, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10), false, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(rv.Uint(), 10), false, nil
	case reflect.Float32:
		return float(rv.Float(), 32), false, nil
	case reflect.Float64:
		return float(rv.Float(), 64), false, nil
	case reflect.Bool:
		return Elem(rv.Bool())
	}
	return "", false, fmt.Errorf("pgtext: no text form for %T", v)
}

func float(f float64, bits int) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	}
	return strconv.FormatFloat(f, 'g', -1, bits)
}

// Quote wraps s in double quotes for an array element or a range bound,
// escaping backslash and double quote.
func Quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		if s[i] == '"' || s[i] == '\\' {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	b.WriteByte('"')
	return b.String()
}

// ArrayElem is Elem for an array element: NULL, or the quoted text.
func ArrayElem(v any) (string, error) {
	t, null, err := Elem(v)
	if err != nil {
		return "", err
	}
	if null {
		return "NULL", nil
	}
	return Quote(t), nil
}

// RangeLiteral renders a range from its parts. lower and upper are nil when
// unbounded.
func RangeLiteral(empty bool, lower, upper any, lowerInc, upperInc bool) (string, error) {
	if empty {
		return "empty", nil
	}
	bound := func(v any) (string, error) {
		if v == nil {
			return "", nil
		}
		t, null, err := Elem(v)
		if err != nil || null {
			return "", err
		}
		return Quote(t), nil
	}
	l, err := bound(lower)
	if err != nil {
		return "", err
	}
	u, err := bound(upper)
	if err != nil {
		return "", err
	}
	open, closeB := "(", ")"
	if lowerInc && lower != nil {
		open = "["
	}
	if upperInc && upper != nil {
		closeB = "]"
	}
	return open + l + "," + u + closeB, nil
}
