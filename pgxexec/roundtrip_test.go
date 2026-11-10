package pgxexec_test

// Type round-trip matrix. For every case, in the binary protocol and in the
// text protocol (pgx's simple protocol sends and receives text), on whatever
// server QLX_TEST_DSN names:
//
//	read   SELECT <literal>::T       scans into the Go type
//	write  SELECT $1::T::text IS NOT DISTINCT FROM (<literal>::T)::text
//	       the server received the value the Go side meant
//	trip   SELECT $1::T              scans back to the value the literal reads as
//
// The literal is the oracle, so a symmetric bug (encoder and decoder wrong
// the same way) cannot hide. Values are the awkward ones: NaN and infinities,
// negative zero, extreme dates, huge numerics, empty and unbounded ranges,
// NUL and multi-byte text, 256-value bytea, arrays with NULLs and several
// dimensions, JSON null versus SQL NULL.
//
// A case with fails set must be refused by the server with that SQLSTATE
// rather than stored or panicking.

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/noobmaisfier/qlx/pgxexec"
	"github.com/noobmaisfier/qlx/qt"
)

type rtCase struct {
	name  string
	typ   string // SQL type the parameter is cast to
	lit   string // SQL literal of that type (the oracle)
	val   any    // Go value written as $1; its type is also the scan target
	exact bool   // val must equal what the literal reads as
	fails string // expected SQLSTATE (alternatives with |); no round trip is attempted
	// noText says why the case cannot run in the simple protocol: pgx sends
	// no type OIDs there, so it cannot encode a Go type it has no built-in
	// text form for (ranges, arrays of qt types, [16]byte).
	noText string
	min    int // lowest server major that supports the case
}

func ptr[T any](v T) *T { return &v }

func sameValue(a, b reflect.Value) bool {
	if !a.IsValid() || !b.IsValid() {
		return a.IsValid() == b.IsValid()
	}
	if a.Type() != b.Type() {
		return false
	}
	if a.Type() == reflect.TypeOf(time.Time{}) {
		return a.Interface().(time.Time).Equal(b.Interface().(time.Time))
	}
	switch a.Kind() {
	case reflect.Float32, reflect.Float64:
		x, y := a.Float(), b.Float()
		return x == y && math.Signbit(x) == math.Signbit(y) || math.IsNaN(x) && math.IsNaN(y)
	case reflect.Pointer, reflect.Interface:
		if a.IsNil() || b.IsNil() {
			return a.IsNil() == b.IsNil()
		}
		return sameValue(a.Elem(), b.Elem())
	case reflect.Slice:
		if a.IsNil() != b.IsNil() || a.Len() != b.Len() {
			return false
		}
		fallthrough
	case reflect.Array:
		if a.Len() != b.Len() {
			return false
		}
		for i := 0; i < a.Len(); i++ {
			if !sameValue(a.Index(i), b.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Struct:
		if a.NumField() > 0 && a.Type().PkgPath() != "" && !a.Type().Field(0).IsExported() {
			return reflect.DeepEqual(a.Interface(), b.Interface())
		}
		for i := 0; i < a.NumField(); i++ {
			if !a.Type().Field(i).IsExported() {
				return reflect.DeepEqual(a.Interface(), b.Interface())
			}
			if !sameValue(a.Field(i), b.Field(i)) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(a.Interface(), b.Interface())
}

// arg is what the library hands to pgx for a value: the renderer unwraps
// qt.Null and friends through QLXArg, so the wrapper never reaches the driver.
func arg(v any) any {
	if a, ok := v.(interface{ QLXArg() any }); ok {
		return a.QLXArg()
	}
	return v
}

func same(a, b any) bool { return sameValue(reflect.ValueOf(a), reflect.ValueOf(b)) }

func i32(v ...int32) []int32 { return v }

func rtCases() []rtCase {
	big := strings.Repeat("9", 5000) + "." + strings.Repeat("1", 500)
	allBytes := make([]byte, 256)
	for i := range allBytes {
		allBytes[i] = byte(i)
	}
	mb := strings.Repeat("é日😀", 100000) // ~1 MB of multi-byte text
	deep := strings.Repeat("[", 200) + "1" + strings.Repeat("]", 200)
	t0 := time.Date(2024, 2, 29, 23, 59, 59, 999999000, time.UTC)
	paris := time.FixedZone("x", 2*3600+30*60)
	lo, hi := int32(-5), int32(7)
	var cs []rtCase
	add := func(c ...rtCase) { cs = append(cs, c...) }

	add(
		rtCase{name: "bool/true", typ: "boolean", lit: "true", val: true, exact: true},
		rtCase{name: "int2/min", typ: "int2", lit: "-32768", val: int16(math.MinInt16), exact: true},
		rtCase{name: "int2/max", typ: "int2", lit: "32767", val: int16(math.MaxInt16), exact: true},
		rtCase{name: "int4/min", typ: "int4", lit: "-2147483648", val: int32(math.MinInt32), exact: true},
		rtCase{name: "int8/min", typ: "int8", lit: "-9223372036854775808", val: int64(math.MinInt64), exact: true},
		rtCase{name: "int8/max", typ: "int8", lit: "9223372036854775807", val: int64(math.MaxInt64), exact: true},
		rtCase{name: "oid/max", typ: "oid", lit: "4294967295", val: uint32(math.MaxUint32), exact: true},

		rtCase{name: "float8/NaN", typ: "float8", lit: "'NaN'", val: math.NaN(), exact: true},
		rtCase{name: "float8/+Inf", typ: "float8", lit: "'Infinity'", val: math.Inf(1), exact: true},
		rtCase{name: "float8/-Inf", typ: "float8", lit: "'-Infinity'", val: math.Inf(-1), exact: true},
		rtCase{name: "float8/-0", typ: "float8", lit: "'-0'", val: math.Copysign(0, -1), exact: true},
		rtCase{name: "float8/max", typ: "float8", lit: "'1.7976931348623157e308'", val: math.MaxFloat64, exact: true},
		rtCase{name: "float8/denormal", typ: "float8", lit: "'5e-324'", val: math.SmallestNonzeroFloat64, exact: true},
		rtCase{name: "float8/0.1", typ: "float8", lit: "'0.1'", val: 0.1, exact: true},
		rtCase{name: "float4/NaN", typ: "float4", lit: "'NaN'", val: float32(math.NaN()), exact: true},
		rtCase{name: "float4/max", typ: "float4", lit: "'3.4028235e38'", val: float32(math.MaxFloat32), exact: true},
		rtCase{name: "float4/-0", typ: "float4", lit: "'-0'", val: float32(math.Copysign(0, -1)), exact: true},

		rtCase{name: "numeric/big", typ: "numeric", lit: "'" + big + "'", val: qt.Numeric(big), exact: true},
		rtCase{name: "numeric/tiny", typ: "numeric", lit: "'0.000000000000000000000000001'", val: qt.Numeric("0.000000000000000000000000001"), exact: true},
		rtCase{name: "numeric/negative scale", typ: "numeric", lit: "'-123.4500'", val: qt.Numeric("-123.4500"), exact: true},
		rtCase{name: "numeric/NaN", typ: "numeric", lit: "'NaN'", val: qt.Numeric("NaN"), exact: true},
		rtCase{name: "numeric/Infinity", typ: "numeric", lit: "'Infinity'", val: qt.Numeric("Infinity"), exact: true, min: 14},
		rtCase{name: "numeric/-Infinity", typ: "numeric", lit: "'-Infinity'", val: qt.Numeric("-Infinity"), exact: true, min: 14},
		rtCase{name: "numeric/1e100", typ: "numeric", lit: "'1e100'", val: qt.Numeric("1e100")},

		rtCase{name: "text/empty", typ: "text", lit: "''", val: "", exact: true},
		rtCase{name: "text/unicode", typ: "text", lit: "'héllo 日本語 😀 e'||U&'\\0301'", val: "héllo 日本語 😀 e\u0301", exact: true},
		rtCase{name: "text/quotes", typ: "text", lit: "E'\\'\"\\\\ \\n\\r\\t'", val: "'\"\\ \n\r\t", exact: true},
		rtCase{name: "text/NULL word", typ: "text", lit: "'NULL'", val: "NULL", exact: true},
		rtCase{name: "text/1MB multibyte", typ: "text", lit: "repeat('é日😀', 100000)", val: mb, exact: true},
		rtCase{name: "text/NUL byte", typ: "text", val: "a\x00b", fails: "22021|08P01"},
		rtCase{name: "text/invalid utf8", typ: "text", val: "a\xffb", fails: "22021|08P01"},
		rtCase{name: "varchar/max", typ: "varchar", lit: "repeat('x', 10485760)", val: strings.Repeat("x", 10485760), exact: true},
		rtCase{name: "name/63", typ: "name", lit: "repeat('n', 63)", val: strings.Repeat("n", 63), exact: true},

		rtCase{name: "bytea/empty", typ: "bytea", lit: "''::bytea", val: []byte{}, exact: true},
		rtCase{name: "bytea/all values", typ: "bytea", lit: "(SELECT string_agg(set_byte('\\x00'::bytea, 0, g), ''::bytea ORDER BY g) FROM generate_series(0,255) g)", val: allBytes, exact: true},
		rtCase{name: "bytea/1MB zeros", typ: "bytea", lit: "decode(repeat('00', 1048576), 'hex')", val: make([]byte, 1<<20), exact: true},

		rtCase{name: "uuid/zero", typ: "uuid", lit: "'00000000-0000-0000-0000-000000000000'", val: [16]byte{}, exact: true, noText: "a [16]byte has no text form"},
		rtCase{name: "uuid/ff", typ: "uuid", lit: "'ffffffff-ffff-ffff-ffff-ffffffffffff'", val: [16]byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}, exact: true, noText: "a [16]byte has no text form"},

		rtCase{name: "date/epoch", typ: "date", lit: "'1970-01-01'", val: qt.DateOf(1970, 1, 1), exact: true},
		rtCase{name: "date/leap", typ: "date", lit: "'2024-02-29'", val: qt.DateOf(2024, 2, 29), exact: true},
		rtCase{name: "date/year 1", typ: "date", lit: "'0001-01-01'", val: qt.DateOf(1, 1, 1), exact: true},
		rtCase{name: "date/9999", typ: "date", lit: "'9999-12-31'", val: qt.DateOf(9999, 12, 31), exact: true},
		rtCase{name: "date/infinity", typ: "date", lit: "'infinity'", val: qt.DateInfinity, exact: true},
		rtCase{name: "date/-infinity", typ: "date", lit: "'-infinity'", val: qt.DateNegInfinity, exact: true},
		rtCase{name: "date/max", typ: "date", lit: "'5874897-12-31'", val: qt.DateOf(5874897, 12, 31), exact: true},

		rtCase{name: "timestamptz/micros", typ: "timestamptz", lit: "'2024-02-29 23:59:59.999999+00'", val: t0, exact: true},
		rtCase{name: "timestamptz/offset zone", typ: "timestamptz", lit: "'2024-02-29 21:29:59.999999+00'", val: t0.In(paris).Add(-2*time.Hour - 30*time.Minute), exact: true},
		rtCase{name: "timestamptz/year 1", typ: "timestamptz", lit: "'0001-01-01 00:00:00+00'", val: time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC), exact: true},
		rtCase{name: "timestamptz/9999", typ: "timestamptz", lit: "'9999-12-31 23:59:59.999999+00'", val: time.Date(9999, 12, 31, 23, 59, 59, 999999000, time.UTC), exact: true},
		rtCase{name: "timestamptz/infinity", typ: "timestamptz", lit: "'infinity'", val: pgtype.Timestamptz{InfinityModifier: pgtype.Infinity, Valid: true}, exact: true},
		rtCase{name: "timestamptz/-infinity", typ: "timestamptz", lit: "'-infinity'", val: pgtype.Timestamptz{InfinityModifier: pgtype.NegativeInfinity, Valid: true}, exact: true},
		rtCase{name: "tstz qt/finite", typ: "timestamptz", lit: "'2024-02-29 23:59:59.999999+00'", val: qt.TimestamptzOf(t0), exact: true},
		rtCase{name: "tstz qt/offset zone", typ: "timestamptz", lit: "'2024-02-29 21:29:59.999999+00'", val: qt.TimestamptzOf(t0.In(paris).Add(-2*time.Hour - 30*time.Minute)), exact: true},
		rtCase{name: "tstz qt/infinity", typ: "timestamptz", lit: "'infinity'", val: qt.TimestamptzInfinity, exact: true},
		rtCase{name: "tstz qt/-infinity", typ: "timestamptz", lit: "'-infinity'", val: qt.TimestamptzNegInfinity, exact: true},
		rtCase{name: "tstz qt/year 1", typ: "timestamptz", lit: "'0001-01-01 00:00:00+00'", val: qt.TimestamptzOf(time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)), exact: true},
		rtCase{name: "tstz qt/BC", typ: "timestamptz", lit: "'0044-03-15 BC'", val: qt.TimestamptzOf(time.Date(-43, 3, 15, 0, 0, 0, 0, time.UTC)), exact: true},
		rtCase{name: "tstz qt/null value", typ: "timestamptz", lit: "'infinity'", val: qt.Some(qt.TimestamptzInfinity), exact: true},
		rtCase{name: "tstz qt/null NULL", typ: "timestamptz", lit: "NULL", val: qt.Null[qt.Timestamptz]{}, exact: true},
		rtCase{name: "array/tstz qt", typ: "timestamptz[]", lit: "ARRAY['infinity','-infinity','2024-02-29 23:59:59.999999+00']::timestamptz[]", val: []qt.Timestamptz{qt.TimestamptzInfinity, qt.TimestamptzNegInfinity, qt.TimestamptzOf(t0)}, exact: true},
		rtCase{name: "array/Null elements", typ: "int4[]", lit: "'{1,NULL,3}'", val: []qt.Null[int32]{qt.Some(int32(1)), {}, qt.Some(int32(3))}, exact: true},
		rtCase{name: "array/Null text elements", typ: "text[]", lit: `'{a,NULL,"NULL"}'`, val: []qt.Null[string]{qt.Some("a"), {}, qt.Some("NULL")}, exact: true},
		rtCase{name: "timestamp/local", typ: "timestamp", lit: "'2024-02-29 23:59:59.999999'", val: qt.LocalTime(time.Date(2024, 2, 29, 23, 59, 59, 999999000, time.UTC)), exact: true},
		rtCase{name: "timestamp/infinity", typ: "timestamp", lit: "'infinity'", val: qt.LocalTimeInfinity, exact: true},
		rtCase{name: "timestamp/-infinity", typ: "timestamp", lit: "'-infinity'", val: qt.LocalTimeNegInfinity, exact: true},
		rtCase{name: "timestamp/null infinity", typ: "timestamp", lit: "'infinity'", val: qt.Some(qt.LocalTimeInfinity), exact: true},
		rtCase{name: "timestamp/null NULL", typ: "timestamp", lit: "NULL", val: qt.Null[qt.LocalTime]{}, exact: true},
		rtCase{name: "array/timestamp infinity", typ: "timestamp[]", lit: "ARRAY['infinity','-infinity','2024-02-29 23:59:59.999999']::timestamp[]", val: []qt.LocalTime{qt.LocalTimeInfinity, qt.LocalTimeNegInfinity, qt.LocalTime(time.Date(2024, 2, 29, 23, 59, 59, 999999000, time.UTC))}, exact: true},
		rtCase{name: "time/midnight", typ: "time", lit: "'00:00:00'", val: qt.TimeOfDay(0), exact: true},
		rtCase{name: "time/24:00", typ: "time", lit: "'24:00:00'", val: qt.TimeOfDay(24 * 3600 * 1e6), exact: true},
		rtCase{name: "time/micros", typ: "time", lit: "'23:59:59.999999'", val: qt.TimeOfDay(86399999999), exact: true},

		rtCase{name: "interval/zero", typ: "interval", lit: "'0'", val: qt.Interval{}, exact: true},
		rtCase{name: "interval/mixed", typ: "interval", lit: "'1 mon 2 days 03:04:05.678901'", val: qt.Interval{Months: 1, Days: 2, Micros: 11045678901}, exact: true},
		rtCase{name: "interval/negative", typ: "interval", lit: "'-1 mon -2 days -03:04:05.678901'", val: qt.Interval{Months: -1, Days: -2, Micros: -11045678901}, exact: true},
		rtCase{name: "interval/extremes", typ: "interval", lit: "make_interval(years => 178956970, months => 7, days => 2147483647)", val: qt.Interval{Months: 2147483647, Days: 2147483647}, exact: true},
		rtCase{name: "interval/max micros", typ: "interval", lit: "'9223372036854775807 microseconds'", val: qt.Interval{Micros: math.MaxInt64}, exact: true},

		rtCase{name: "json/null vs SQL NULL", typ: "jsonb", lit: "'null'", val: qt.Raw([]byte("null")), exact: true},
		rtCase{name: "jsonb/object", typ: "jsonb", lit: `'{"a":[1,2,{"b":null}],"c":"é\u00e9"}'`, val: qt.Raw([]byte(`{"a":[1,2,{"b":null}],"c":"é\u00e9"}`))},
		rtCase{name: "jsonb/big number", typ: "jsonb", lit: "'1e400'", val: qt.Raw([]byte("1e400"))},
		rtCase{name: "jsonb/200 deep", typ: "jsonb", lit: "'" + deep + "'", val: qt.Raw([]byte(deep))},
		rtCase{name: "jsonb/duplicate keys", typ: "jsonb", lit: `'{"a":1,"a":2}'`, val: qt.Raw([]byte(`{"a":1,"a":2}`))},
		rtCase{name: "jsonb/NUL escape", typ: "jsonb", val: qt.Raw([]byte(`"\u0000"`)), fails: "22P05"},
		rtCase{name: "json/NUL escape", typ: "json", lit: `'"\u0000"'`, val: qt.Raw([]byte(`"\u0000"`)), exact: true},
		rtCase{name: "json/spacing kept", typ: "json", lit: `'{ "a" : 1 }'`, val: qt.Raw([]byte(`{ "a" : 1 }`)), exact: true},

		rtCase{name: "timetz/negative offset", typ: "timetz", lit: "'23:59:59.999999-12:30'", val: qt.TimeTZ{Time: 86399999999, Offset: -(12*3600 + 30*60)}, exact: true},
		rtCase{name: "null/int4 value", typ: "int4", lit: "7", val: qt.Some(int32(7)), exact: true},
		rtCase{name: "null/int4 NULL", typ: "int4", lit: "NULL", val: qt.Null[int32]{}, exact: true},
		rtCase{name: "null/text value", typ: "text", lit: "'x'", val: qt.Some("x"), exact: true},
		rtCase{name: "null/text NULL", typ: "text", lit: "NULL", val: qt.Null[string]{}, exact: true},
		rtCase{name: "null/interval", typ: "interval", lit: "'1 mon 2 days 03:04:05.678901'", val: qt.Some(qt.Interval{Months: 1, Days: 2, Micros: 11045678901}), exact: true},
		rtCase{name: "null/numeric", typ: "numeric", lit: "'-0.50'", val: qt.Some(qt.Numeric("-0.50")), exact: true},
		rtCase{name: "null/date", typ: "date", lit: "'infinity'", val: qt.Some(qt.DateInfinity), exact: true},
		rtCase{name: "null/jsonb null", typ: "jsonb", lit: "'null'", val: qt.Some(qt.Raw([]byte("null"))), exact: true},
		rtCase{name: "null/jsonb NULL", typ: "jsonb", lit: "NULL", val: qt.Null[qt.RawJSON]{}, exact: true},
		rtCase{name: "inet/v6", typ: "inet", lit: "'::ffff:1.2.3.4'", val: netip.MustParseAddr("::ffff:1.2.3.4"), exact: true},
		rtCase{name: "inet/prefix", typ: "inet", lit: "'10.1.2.3/8'", val: netip.MustParsePrefix("10.1.2.3/8"), exact: true},
		rtCase{name: "cidr/v6", typ: "cidr", lit: "'2001:db8::/32'", val: netip.MustParsePrefix("2001:db8::/32"), exact: true},

		rtCase{name: "money/min", typ: "money", lit: "'-92233720368547758.08'::money", val: qt.Money("-$92,233,720,368,547,758.08"), exact: true},
		rtCase{name: "bit/empty varbit", typ: "varbit", lit: "B''", val: qt.Bit(""), exact: true},
		rtCase{name: "bit/long", typ: "varbit", lit: "('1' || repeat('01', 1000))::varbit", val: qt.Bit("1" + strings.Repeat("01", 1000)), exact: true},
		rtCase{name: "point", typ: "point", lit: "'(1.5,-2)'", val: qt.Point("(1.5,-2)"), exact: true},
		rtCase{name: "macaddr8", typ: "macaddr8", lit: "'08:00:2b:01:02:03:04:05'", val: qt.MacAddr8("08:00:2b:01:02:03:04:05"), exact: true},
		// pgx's binary tsvector codec re-encodes an apostrophe as \' while the
		// server's text output doubles it ('it''s'). Both spell the same
		// tsvector, so the text is not compared byte for byte across protocols.
		rtCase{name: "tsvector/quotes", typ: "tsvector", lit: `$$'it''s':1 'a b':2,3$$`, val: qt.TSVector(`'a b':2,3 'it''s':1`)},
		rtCase{name: "xml/unicode", typ: "xml", lit: "xmlparse(document '<a>é日</a>')", val: qt.XML("<a>é日</a>"), exact: true},
		rtCase{name: "pg_lsn/max", typ: "pg_lsn", lit: "'FFFFFFFF/FFFFFFFF'", val: qt.LSN("FFFFFFFF/FFFFFFFF"), exact: true},
		rtCase{name: "char/high byte", typ: `"char"`, lit: "'\\377'::\"char\"", val: qt.Char(255), exact: true, noText: "pgx cannot read the text form of a high-bit char"},

		// ranges
		rtCase{name: "int4range/bounded", typ: "int4range", lit: "'[-5,7)'", val: qt.Range[int32]{Lower: &lo, Upper: &hi, LowerInc: true}, exact: true},
		rtCase{name: "int4range/empty", typ: "int4range", lit: "'empty'", val: qt.Range[int32]{Empty: true}, exact: true},
		rtCase{name: "int4range/unbounded", typ: "int4range", lit: "'(,)'", val: qt.Range[int32]{}, exact: true},
		rtCase{name: "int4range/lower only", typ: "int4range", lit: "'[-5,)'", val: qt.Range[int32]{Lower: &lo, LowerInc: true}, exact: true},
		rtCase{name: "int4range/overflow", typ: "int4range", val: qt.Range[int32]{Lower: ptr(int32(1)), Upper: ptr(int32(math.MaxInt32)), LowerInc: true, UpperInc: true}, fails: "22003"},
		rtCase{name: "tstzrange/infinite upper", typ: "tstzrange", lit: "'[2024-02-29 23:59:59.999999+00,)'", val: qt.Range[time.Time]{Lower: &t0, LowerInc: true}},
		rtCase{name: "multirange/two", typ: "int4multirange", lit: "'{[-5,7),[10,20)}'", val: qt.Multirange[int32]{{Lower: &lo, Upper: &hi, LowerInc: true}, {Lower: ptr(int32(10)), Upper: ptr(int32(20)), LowerInc: true}}, exact: true, min: 14},
		rtCase{name: "multirange/empty is not NULL", typ: "int4multirange", lit: "'{}'", val: qt.Multirange[int32]{}, exact: true, min: 14},

		// arrays
		rtCase{name: "array/empty", typ: "int4[]", lit: "'{}'", val: []int32{}, exact: true},
		rtCase{name: "array/with NULL", typ: "int4[]", lit: "'{1,NULL,3}'", val: []*int32{ptr(int32(1)), nil, ptr(int32(3))}, exact: true},
		rtCase{name: "array/text hard", typ: "text[]", lit: `ARRAY['a,b','"q"','\','NULL','', ' x ','{}','é日']`, val: []string{"a,b", `"q"`, `\`, "NULL", "", " x ", "{}", "é日"}, exact: true},
		rtCase{name: "array/text NULL elements", typ: "text[]", lit: `'{a,NULL,"NULL"}'`, val: []*string{ptr("a"), nil, ptr("NULL")}, exact: true},
		rtCase{name: "array/2-dim", typ: "int4[]", lit: "'{{1,2},{3,4},{5,6}}'", val: pgtype.Array[int32]{Elements: i32(1, 2, 3, 4, 5, 6), Dims: []pgtype.ArrayDimension{{Length: 3, LowerBound: 1}, {Length: 2, LowerBound: 1}}, Valid: true}, exact: true},
		rtCase{name: "array/3-dim with NULL", typ: "int4[]", lit: "'{{{1,NULL}},{{3,4}}}'", val: pgtype.Array[*int32]{Elements: []*int32{ptr(int32(1)), nil, ptr(int32(3)), ptr(int32(4))}, Dims: []pgtype.ArrayDimension{{Length: 2, LowerBound: 1}, {Length: 1, LowerBound: 1}, {Length: 2, LowerBound: 1}}, Valid: true}, exact: true},
		rtCase{name: "array/lower bound 0", typ: "int4[]", lit: "'[0:2]={1,2,3}'", val: pgtype.Array[int32]{Elements: i32(1, 2, 3), Dims: []pgtype.ArrayDimension{{Length: 3, LowerBound: 0}}, Valid: true}, exact: true},
		rtCase{name: "array/bytea", typ: "bytea[]", lit: "ARRAY['\\x'::bytea, '\\x00ff'::bytea]", val: [][]byte{{}, {0, 255}}, exact: true},
		rtCase{name: "array/jsonb", typ: "jsonb[]", lit: `ARRAY['null'::jsonb, '{"a":1}'::jsonb]`, val: []qt.RawJSON{qt.Raw([]byte("null")), qt.Raw([]byte(`{"a":1}`))}},
		rtCase{name: "array/float8 specials", typ: "float8[]", lit: "ARRAY['NaN','Infinity','-Infinity','-0']::float8[]", val: []float64{math.NaN(), math.Inf(1), math.Inf(-1), math.Copysign(0, -1)}, exact: true},
		rtCase{name: "array/numeric", typ: "numeric[]", lit: "ARRAY['1.50','NaN','-0.001']::numeric[]", val: []qt.Numeric{"1.50", "NaN", "-0.001"}, exact: true},
		rtCase{name: "array/date infinity", typ: "date[]", lit: "ARRAY['infinity','-infinity','2024-02-29']::date[]", val: []qt.Date{qt.DateInfinity, qt.DateNegInfinity, qt.DateOf(2024, 2, 29)}, exact: true},
		rtCase{name: "array/timestamptz", typ: "timestamptz[]", lit: "ARRAY['2024-02-29 23:59:59.999999+00']::timestamptz[]", val: []time.Time{t0}, exact: true},
		rtCase{name: "array/inet", typ: "inet[]", lit: "ARRAY['10.0.0.1','::1']::inet[]", val: []netip.Addr{netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("::1")}, exact: true},
	)
	return cs
}

func rtConn(t *testing.T, mode pgx.QueryExecMode) *pgx.Conn {
	t.Helper()
	dsn := os.Getenv("QLX_TEST_DSN")
	if dsn == "" {
		t.Skip("QLX_TEST_DSN not set")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DefaultQueryExecMode = mode
	c, err := pgx.ConnectConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	pgxexec.RegisterRangeTypes(c.TypeMap())
	t.Cleanup(func() { c.Close(context.Background()) })
	return c
}

func serverMajor(t *testing.T, c *pgx.Conn) int {
	t.Helper()
	var n int
	if err := c.QueryRow(context.Background(), "SELECT current_setting('server_version_num')::int / 10000").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestTypeRoundTripMatrix(t *testing.T) {
	for _, mode := range []struct {
		name string
		m    pgx.QueryExecMode
	}{{"binary", pgx.QueryExecModeCacheStatement}, {"describe", pgx.QueryExecModeCacheDescribe}, {"text", pgx.QueryExecModeSimpleProtocol}} {
		t.Run(mode.name, func(t *testing.T) {
			c := rtConn(t, mode.m)
			major := serverMajor(t, c)
			ctx := context.Background()
			for _, rc := range rtCases() {
				t.Run(rc.name, func(t *testing.T) {
					if rc.min > major {
						t.Skipf("needs PostgreSQL %d", rc.min)
					}
					if mode.name == "text" && rc.noText != "" {
						t.Skip(rc.noText)
					}
					typeOK := true
					if err := c.QueryRow(ctx, "SELECT NULL::"+rc.typ).Scan(new(any)); err != nil {
						typeOK = false
						t.Skipf("server has no type %s: %v", rc.typ, err)
					}
					_ = typeOK
					if rc.fails != "" {
						var dst = reflect.New(reflect.TypeOf(rc.val)).Interface()
						err := c.QueryRow(ctx, "SELECT $1::"+rc.typ, arg(rc.val)).Scan(dst)
						var pe *pgconn.PgError
						if !errors.As(err, &pe) {
							// A client-side refusal is acceptable only if it names the problem.
							if err == nil {
								t.Fatalf("accepted, want SQLSTATE %s", rc.fails)
							}
							t.Logf("refused client-side: %v", err)
							return
						}
						if !strings.Contains("|"+rc.fails+"|", "|"+pe.Code+"|") {
							t.Fatalf("SQLSTATE %s (%s), want %s", pe.Code, pe.Message, rc.fails)
						}
						return
					}
					// read
					want := reflect.New(reflect.TypeOf(rc.val))
					if err := c.QueryRow(ctx, "SELECT ("+rc.lit+")::"+rc.typ).Scan(want.Interface()); err != nil {
						t.Fatalf("read: %v", err)
					}
					if rc.exact && !same(rc.val, want.Elem().Interface()) {
						t.Errorf("read: literal reads as %#v, expected %#v", want.Elem().Interface(), rc.val)
					}
					// write
					var ok bool
					q := "SELECT $1::" + rc.typ + "::text IS NOT DISTINCT FROM (" + rc.lit + ")::" + rc.typ + "::text"
					if err := c.QueryRow(ctx, q, arg(rc.val)).Scan(&ok); err != nil {
						t.Fatalf("write: %v", err)
					}
					if !ok {
						var got, lit string
						_ = c.QueryRow(ctx, "SELECT $1::"+rc.typ+"::text, ("+rc.lit+")::"+rc.typ+"::text", arg(rc.val)).Scan(&got, &lit)
						t.Errorf("write: server received %.200q, literal is %.200q", got, lit)
					}
					// round trip
					got := reflect.New(reflect.TypeOf(rc.val))
					if err := c.QueryRow(ctx, "SELECT $1::"+rc.typ, arg(rc.val)).Scan(got.Interface()); err != nil {
						t.Fatalf("trip: %v", err)
					}
					if !same(got.Elem().Interface(), want.Elem().Interface()) {
						t.Errorf("trip: got %.300s, want %.300s", fmt.Sprintf("%#v", got.Elem().Interface()), fmt.Sprintf("%#v", want.Elem().Interface()))
					}
				})
			}
		})
	}
}

// SQL NULL and a JSON null are different values and must stay different.
func TestNullsStayNull(t *testing.T) {
	for _, mode := range []pgx.QueryExecMode{pgx.QueryExecModeCacheStatement, pgx.QueryExecModeSimpleProtocol} {
		c := rtConn(t, mode)
		ctx := context.Background()
		var j qt.Null[qt.RawJSON]
		if err := c.QueryRow(ctx, "SELECT NULL::jsonb").Scan(&j); err != nil || j.Valid {
			t.Errorf("mode %v: NULL jsonb: %+v %v", mode, j, err)
		}
		if err := c.QueryRow(ctx, "SELECT 'null'::jsonb").Scan(&j); err != nil || !j.Valid || string(j.V.V) != "null" {
			t.Errorf("mode %v: jsonb null: %+v %v", mode, j, err)
		}
		var isNull bool
		if err := c.QueryRow(ctx, "SELECT $1::jsonb IS NULL", qt.Null[qt.RawJSON]{}).Scan(&isNull); err != nil || !isNull {
			t.Errorf("mode %v: Null[RawJSON]{} was sent as a value: %v", mode, err)
		}

		var r qt.Range[int32]
		if err := c.QueryRow(ctx, "SELECT NULL::int4range").Scan(&r); err != nil || !r.Null {
			t.Errorf("mode %v: NULL range: %+v %v", mode, r, err)
		}
		var ds qt.Null[qt.Date]
		if err := c.QueryRow(ctx, "SELECT NULL::date").Scan(&ds); err != nil || ds.Valid {
			t.Errorf("mode %v: NULL date: %+v %v", mode, ds, err)
		}
		var empty []byte
		if err := c.QueryRow(ctx, "SELECT ''::bytea").Scan(&empty); err != nil || empty == nil {
			t.Errorf("mode %v: empty bytea read as nil: %v %v", mode, empty, err)
		}
	}
}

// A multi-dimensional array, or one that does not start at index 1, must not be
// flattened into a plain slice without a word.
func TestArraysAreNeverFlattenedSilently(t *testing.T) {
	for _, mode := range []pgx.QueryExecMode{pgx.QueryExecModeCacheStatement, pgx.QueryExecModeSimpleProtocol} {
		c := rtConn(t, mode)
		ctx := context.Background()
		var flat []int32
		err := c.QueryRow(ctx, "SELECT '{{1,2},{3,4}}'::int4[]").Scan(&flat)
		if err == nil || !strings.Contains(err.Error(), "2-dimensional") {
			t.Errorf("mode %v: 2-dimensional array into []int32: %v %v", mode, flat, err)
		}
		err = c.QueryRow(ctx, "SELECT '[0:1]={5,6}'::int4[]").Scan(&flat)
		if err == nil || !strings.Contains(err.Error(), "lower bound") {
			t.Errorf("mode %v: lower bound 0 into []int32: %v %v", mode, flat, err)
		}
		// what must keep working
		if err := c.QueryRow(ctx, "SELECT '{1,2,3}'::int4[]").Scan(&flat); err != nil || len(flat) != 3 || flat[2] != 3 {
			t.Errorf("mode %v: plain array: %v %v", mode, flat, err)
		}
		if err := c.QueryRow(ctx, "SELECT '{}'::int4[]").Scan(&flat); err != nil || flat == nil || len(flat) != 0 {
			t.Errorf("mode %v: empty array: %#v %v", mode, flat, err)
		}
		flat = []int32{9}
		if err := c.QueryRow(ctx, "SELECT NULL::int4[]").Scan(&flat); err != nil || flat != nil {
			t.Errorf("mode %v: NULL array: %#v %v", mode, flat, err)
		}
		var strs []string
		if err := c.QueryRow(ctx, `SELECT ARRAY['a,b','"q"','']`).Scan(&strs); err != nil || len(strs) != 3 || strs[1] != `"q"` {
			t.Errorf("mode %v: text array: %q %v", mode, strs, err)
		}
		var raw []byte
		if err := c.QueryRow(ctx, `SELECT '\x0001'::bytea`).Scan(&raw); err != nil || len(raw) != 2 {
			t.Errorf("mode %v: bytea into []byte: %v %v", mode, raw, err)
		}
		var bs [][]byte
		if err := c.QueryRow(ctx, `SELECT ARRAY['\x00ff'::bytea, ''::bytea]`).Scan(&bs); err != nil || len(bs) != 2 || len(bs[0]) != 2 {
			t.Errorf("mode %v: bytea array: %v %v", mode, bs, err)
		}
		var nulls []qt.Null[int32]
		if err := c.QueryRow(ctx, "SELECT '{1,NULL}'::int4[]").Scan(&nulls); err != nil || len(nulls) != 2 || nulls[1].Valid {
			t.Errorf("mode %v: array with NULL into []qt.Null: %v %v", mode, nulls, err)
		}
		var multi [][]int32
		if err := c.QueryRow(ctx, "SELECT '{{1,2},{3,4}}'::int4[]").Scan(&multi); err != nil || len(multi) != 2 || multi[1][0] != 3 {
			t.Errorf("mode %v: [][]int32 for a 2-dimensional array: %v %v", mode, multi, err)
		}
		var pa pgtype.Array[int32]
		if err := c.QueryRow(ctx, "SELECT '[0:1]={5,6}'::int4[]").Scan(&pa); err != nil || pa.Dims[0].LowerBound != 0 {
			t.Errorf("mode %v: pgtype.Array keeps its bounds: %+v %v", mode, pa, err)
		}
	}
}
