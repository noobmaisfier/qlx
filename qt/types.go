package qt

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// LocalTime is a timestamp without time zone. It is a distinct type so a
// zone-less value can never be mistaken for an instant (timestamptz maps to
// time.Time). pgx scans it through its time.Time underlying type.
type LocalTime time.Time

// Time converts back to time.Time (the zone is meaningless, UTC by convention).
func (t LocalTime) Time() time.Time { return time.Time(t) }

// PostgreSQL's timestamp runs from 4713 BC to 294276 AD, and has two more
// values, 'infinity' and '-infinity', that are not instants. LocalTime keeps
// them as instants outside that range, so it stays a plain time.Time and
// compares with Before and After: every value later than 294276 AD is +infinity
// and every one earlier than 4713 BC is -infinity. Use the constants below to
// write them and IsInfinite or Sign to test for them.
var (
	// LocalTimeInfinity is 'infinity'.
	LocalTimeInfinity = LocalTime(time.Date(294277, 1, 1, 0, 0, 0, 0, time.UTC))
	// LocalTimeNegInfinity is '-infinity'.
	LocalTimeNegInfinity = LocalTime(time.Date(-4800, 1, 1, 0, 0, 0, 0, time.UTC))
)

// Sign is 1 for infinity, -1 for -infinity and 0 for an ordinary timestamp.
func (t LocalTime) Sign() int {
	switch y := time.Time(t).Year(); {
	case y > 294276:
		return 1
	case y < -4712: // 4713 BC
		return -1
	}
	return 0
}

// IsInfinite reports whether t is infinity or -infinity.
func (t LocalTime) IsInfinite() bool { return t.Sign() != 0 }

// String is the JSON and text form: "infinity", "-infinity", or an ISO 8601
// date-time without a zone ("2024-02-29T01:02:03.123456").
func (t LocalTime) String() string {
	switch t.Sign() {
	case 1:
		return "infinity"
	case -1:
		return "-infinity"
	}
	return time.Time(t).Format("2006-01-02T15:04:05.999999999")
}

// MarshalText makes LocalTime a JSON string.
func (t LocalTime) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

// UnmarshalText is the inverse of MarshalText. It also accepts PostgreSQL's
// space-separated form and a date alone, as Scan does.
func (t *LocalTime) UnmarshalText(b []byte) error { return t.parse(string(b)) }

// Numeric is an arbitrary-precision decimal kept as its canonical text, so no
// precision is lost on the way through Go. Swap it for a decimal library with
// a type override in qlx.yaml.
type Numeric string

// Interval is a PostgreSQL interval. Months, days and microseconds are kept
// separate because they are not interconvertible (a day is not always 24 h).
type Interval struct {
	Months int32
	Days   int32
	Micros int64
}

// Days, Hours and friends build common intervals.
func Days(n int32) Interval             { return Interval{Days: n} }
func Months(n int32) Interval           { return Interval{Months: n} }
func Duration(d time.Duration) Interval { return Interval{Micros: d.Microseconds()} }

// JSONB is a json or jsonb column value. T is the decoded shape of the
// document; json.RawMessage (see RawJSON) keeps it opaque. Encoding goes
// through encoding/json, which is the v2 engine since Go 1.27, so a column
// typed JSONB[Settings] reads and writes a Settings with no glue code.
type JSONB[T any] struct {
	V T
}

// RawJSON is a JSON document kept as text: the default type of json and
// jsonb columns.
type RawJSON = JSONB[json.RawMessage]

// JSON is the old name of JSONB.
//
// Deprecated: use JSONB.
type JSON[T any] = JSONB[T]

// Raw wraps JSON text as a RawJSON.
func Raw(b []byte) RawJSON { return RawJSON{V: json.RawMessage(b)} }

// JSONOf wraps a Go value as a JSONB.
func JSONOf[T any](v T) JSONB[T] { return JSONB[T]{V: v} }

// MarshalJSON encodes the document itself, not the wrapper.
func (j JSONB[T]) MarshalJSON() ([]byte, error) { return json.Marshal(j.V) }

// UnmarshalJSON decodes into the document.
func (j *JSONB[T]) UnmarshalJSON(b []byte) error { return json.Unmarshal(b, &j.V) }

// Value implements driver.Valuer: the document as JSON text.
func (j JSONB[T]) Value() (driver.Value, error) {
	// Text, not []byte: in the simple protocol pgx would send bytes as bytea.
	if raw, ok := any(j.V).(json.RawMessage); ok {
		if raw == nil {
			return "null", nil
		}
		return string(raw), nil
	}
	b, err := json.Marshal(j.V)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan implements sql.Scanner for text and bytes; SQL NULL is the zero value
// (use qt.Null[JSONB[T]] for a nullable column to tell them apart).
func (j *JSONB[T]) Scan(src any) error {
	var b []byte
	switch v := src.(type) {
	case nil:
		var zero T
		j.V = zero
		return nil
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("qt: cannot scan %T into JSONB", src)
	}
	if raw, ok := any(&j.V).(*json.RawMessage); ok {
		*raw = append((*raw)[:0], b...)
		return nil
	}
	return json.Unmarshal(b, &j.V)
}

// Range is a PostgreSQL range. A nil bound is unbounded. Null marks SQL NULL:
// a nullable range column is a Range with Null set rather than a Null[Range],
// so the pgx codec can fill it directly (see RangeReader / RangeWriter).
type Range[T any] struct {
	Lower, Upper       *T
	LowerInc, UpperInc bool
	Empty              bool
	Null               bool
}

// Multirange is a PostgreSQL multirange. A nil slice is SQL NULL; an empty,
// non-nil slice is the empty multirange.
type Multirange[T any] []Range[T]

// Text-form types. PostgreSQL types that have no natural Go counterpart are
// carried as named strings in PostgreSQL's own text format, so a value of one
// type cannot be passed where another is expected, and nothing is lost in
// translation. Convert with a library of your choice at the edge.
type (
	TSVector     string // tsvector
	TSQuery      string // tsquery
	Bit          string // bit, varbit
	Point        string
	Line         string
	Lseg         string
	Box          string
	Path         string
	Polygon      string
	Circle       string
	Money        string
	MacAddr      string // macaddr
	MacAddr8     string // macaddr8
	LSN          string // pg_lsn
	XML          string
	RegClass     string
	RegType      string
	RegProc      string
	RegProcedure string
	RegOper      string
	RegOperator  string
	RegNamespace string
	RegRole      string
	RegConfig    string
	RegDict      string
	Xid          string
	Xid8         string
	Cid          string
	Tid          string
	JSONPath     string // jsonpath
	ACLItem      string // aclitem, e.g. "alice=r*w/bob"
	OIDVector    string // oidvector
	Int2Vector   string // int2vector
	Oid8         string // oid8 (PostgreSQL 19): 64-bit object id, in text form
	Snapshot     string // pg_snapshot and txid_snapshot, "xmin:xmax:xip,..."
	NodeTree     string // pg_node_tree
	RefCursor    string // refcursor
	RegCollation string // regcollation
	RegDatabase  string // regdatabase (PostgreSQL 19)
	NDistinct    string // pg_ndistinct
	Dependencies string // pg_dependencies
	MCVList      string // pg_mcv_list
)

// Char is PostgreSQL's single-byte internal type "char" (not char(n), which
// is bpchar): a catalog flag such as pg_class.relkind. pgx reads and writes it
// as a byte.
type Char byte

// Void is the result of a function declared RETURNS void (pg_advisory_lock,
// pg_sleep, ...). It carries nothing; select the function to run it.
type Void struct{}

// Scan implements sql.Scanner: a void result is an empty string, which is
// accepted and ignored.
func (*Void) Scan(any) error { return nil }

// Scan and Value for the text-form types whose pgx codec decodes to a struct
// (tsvector, tsquery, bit strings, geometric types): without them pgx cannot
// scan a result into the named string. pgx hands Scan the text form its codec
// decodes to, and sends Value as the text parameter.
func scanText(dst *string, src any, what string) error {
	switch v := src.(type) {
	case string:
		*dst = v
	case []byte:
		*dst = string(v)
	case nil:
		return fmt.Errorf("qt: NULL into %s (use Null[%s])", what, what)
	default:
		return fmt.Errorf("qt: cannot scan %T into %s", src, what)
	}
	return nil
}

// Scan implements sql.Scanner. Without it pgx decodes xml by unmarshalling it
// into the string, which keeps only the character data and drops the markup.
func (v *XML) Scan(src any) error { return scanText((*string)(v), src, "qt.XML") }

// Value implements driver.Valuer: the document text.
func (v XML) Value() (driver.Value, error) { return string(v), nil }

// Scan implements sql.Scanner.
func (v *TSVector) Scan(src any) error { return scanText((*string)(v), src, "qt.TSVector") }

// Value implements driver.Valuer: the text form.
func (v TSVector) Value() (driver.Value, error) { return string(v), nil }

// Scan implements sql.Scanner.
func (v *TSQuery) Scan(src any) error { return scanText((*string)(v), src, "qt.TSQuery") }

// Value implements driver.Valuer: the text form.
func (v TSQuery) Value() (driver.Value, error) { return string(v), nil }

// Scan implements sql.Scanner.
func (v *Bit) Scan(src any) error { return scanText((*string)(v), src, "qt.Bit") }

// Value implements driver.Valuer: the text form.
func (v Bit) Value() (driver.Value, error) { return string(v), nil }

// Scan implements sql.Scanner.
func (v *Point) Scan(src any) error { return scanText((*string)(v), src, "qt.Point") }

// Value implements driver.Valuer: the text form.
func (v Point) Value() (driver.Value, error) { return string(v), nil }

// Scan implements sql.Scanner.
func (v *Line) Scan(src any) error { return scanText((*string)(v), src, "qt.Line") }

// Value implements driver.Valuer: the text form.
func (v Line) Value() (driver.Value, error) { return string(v), nil }

// Scan implements sql.Scanner.
func (v *Lseg) Scan(src any) error { return scanText((*string)(v), src, "qt.Lseg") }

// Value implements driver.Valuer: the text form.
func (v Lseg) Value() (driver.Value, error) { return string(v), nil }

// Scan implements sql.Scanner.
func (v *Box) Scan(src any) error { return scanText((*string)(v), src, "qt.Box") }

// Value implements driver.Valuer: the text form.
func (v Box) Value() (driver.Value, error) { return string(v), nil }

// Scan implements sql.Scanner.
func (v *Path) Scan(src any) error { return scanText((*string)(v), src, "qt.Path") }

// Value implements driver.Valuer: the text form.
func (v Path) Value() (driver.Value, error) { return string(v), nil }

// Scan implements sql.Scanner.
func (v *Polygon) Scan(src any) error { return scanText((*string)(v), src, "qt.Polygon") }

// Value implements driver.Valuer: the text form.
func (v Polygon) Value() (driver.Value, error) { return string(v), nil }

// Scan implements sql.Scanner.
func (v *Circle) Scan(src any) error { return scanText((*string)(v), src, "qt.Circle") }

// Value implements driver.Valuer: the text form.
func (v Circle) Value() (driver.Value, error) { return string(v), nil }
