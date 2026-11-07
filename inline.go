package qlx

import (
	"encoding/hex"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Inline arguments.
//
// PostgreSQL accepts $n placeholders in few utility statements, and COPY is
// not one of them. For COPY (query) and COPY ... FROM ... WHERE, InlineArgs
// writes each argument into the SQL text instead. That is only safe because
// the text of a literal is made from the typed value alone, and because the
// accepted set is small enough to check:
//
//   - integers, bools and finite or NaN/Infinity floats: formatted from the
//     number, nothing else can appear;
//   - time.Time: formatted in UTC from its fields;
//   - [16]byte (uuid) and []byte: formatted as hex;
//   - strings: only printable ASCII without a quote or a backslash, as for the
//     other literals the renderer writes (the string is refused, not escaped);
//   - NULL (nil, an unset qt.Null);
//   - slices of the above, as ARRAY[...] (a slice with no element is refused).
//
// Anything else, and an argument bound later through a Binding, is an
// ErrParams error that names the type.

// inlineLiteral returns the SQL text of v, or an error.
func inlineLiteral(v any) (string, error) {
	v = normalizeArg(v)
	if v == nil {
		return "NULL", nil
	}
	if x, ok := v.(time.Time); ok {
		if x.Year() < 1 { // Go counts a year 0 and negative years; PostgreSQL writes BC dates differently
			return "", fmt.Errorf("%w: a time before year 1 cannot be written as a literal", ErrParams)
		}
		return "'" + x.UTC().Format("2006-01-02T15:04:05.999999999Z") + "'::timestamptz", nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Array && rv.Len() == 16 && rv.Type().Elem().Kind() == reflect.Uint8 {
		var b [16]byte
		reflect.Copy(reflect.ValueOf(b[:]), rv)
		h := hex.EncodeToString(b[:])
		return "'" + h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:] + "'::uuid", nil
	}
	if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() == reflect.Uint8 {
		// decode(..., 'hex') has no backslash, which a string with
		// standard_conforming_strings off would read as an escape
		return "decode('" + hex.EncodeToString(rv.Bytes()) + "', 'hex')", nil
	}
	switch rv.Kind() {
	case reflect.Bool:
		if rv.Bool() {
			return "TRUE", nil
		}
		return "FALSE", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return parenNeg(strconv.FormatInt(rv.Int(), 10)), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(rv.Uint(), 10), nil
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		switch {
		case math.IsNaN(f):
			return "'NaN'::float8", nil
		case math.IsInf(f, 1):
			return "'Infinity'::float8", nil
		case math.IsInf(f, -1):
			return "'-Infinity'::float8", nil
		}
		if f == 0 && math.Signbit(f) { // (-0)::float8 would be an integer 0, which has no sign
			return "'-0'::float8", nil
		}
		return parenNeg(strconv.FormatFloat(f, 'g', -1, 64)) + "::float8", nil
	case reflect.String:
		s := rv.String()
		if !safeLit.MatchString(s) || strings.ContainsAny(s, `'\`) {
			return "", fmt.Errorf("%w: the string %q cannot be written as a literal (printable ASCII without quote or backslash only)", ErrParams, s)
		}
		return "'" + s + "'", nil
	case reflect.Slice, reflect.Array:
		if rv.Len() == 0 {
			return "", fmt.Errorf("%w: an empty array cannot be written as a literal", ErrParams)
		}
		parts := make([]string, rv.Len())
		for i := range parts {
			p, err := inlineLiteral(rv.Index(i).Interface())
			if err != nil {
				return "", err
			}
			parts[i] = p
		}
		return "ARRAY[" + strings.Join(parts, ", ") + "]", nil
	}
	return "", fmt.Errorf("%w: a value of type %T cannot be written as a literal", ErrParams, v)
}

func parenNeg(s string) string {
	if strings.HasPrefix(s, "-") {
		return "(" + s + ")"
	}
	return s
}
