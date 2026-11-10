package pgxexec

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/noobmaisfier/qlx/qt"
)

// RegisterRangeTypes teaches a pgx type map to encode and decode qt.Range[T]
// and qt.Multirange[T] for every range and multirange type the map knows
// (int4range, tstzrange, daterange, ... and custom ones once registered with
// pgtype). It rides on pgx's own RangeCodec: the element bounds are scanned
// and encoded by the element type's codec, so T can be anything pgx handles
// (int32, int64, time.Time, qt.Numeric through its codec, ...).
//
// Use it when you build connections:
//
//	cfg.AfterConnect = pgxexec.AfterConnect
//
// Written from pgx v5's documented RangeScanner/RangeValuer and
// MultirangeSetter/Getter interfaces; not compiled yet (see package doc).
func RegisterRangeTypes(m *pgtype.Map) {
	registerCatalogTypes(m)
	m.TryWrapEncodePlanFuncs = append([]pgtype.TryWrapEncodePlanFunc{tryWrapRangeEncode, tryWrapMultirangeEncode, tryWrapIntervalEncode}, m.TryWrapEncodePlanFuncs...)
	m.TryWrapEncodePlanFuncs = append(m.TryWrapEncodePlanFuncs, tryWrapArrayLiteral) // last resort: text form, for the simple protocol
	m.TryWrapScanPlanFuncs = append([]pgtype.TryWrapScanPlanFunc{tryWrapRangeScan, tryWrapMultirangeScan, tryWrapArrayGuard}, m.TryWrapScanPlanFuncs...)
}

// AfterConnect is a pgxpool.Config.AfterConnect hook. On each new connection it
// registers the range types (RegisterRangeTypes) and loads the database's
// enums, domains and extension types with their arrays (LoadTypes), so arrays
// of those can be scanned and written.
func AfterConnect(ctx context.Context, c *pgx.Conn) error {
	RegisterRangeTypes(c.TypeMap())
	return LoadTypes(ctx, c)
}

// ---- intervals ----

// intervalValuer adapts a qt.Interval for encoding (pgtype.IntervalValuer).
// Without it pgx treats the struct as something else in the binary protocol
// and writes only the Days field: months and microseconds would be lost
// without an error.
type intervalValuer struct{ v qt.Interval }

func (a intervalValuer) IntervalValue() (pgtype.Interval, error) {
	return pgtype.Interval{Months: a.v.Months, Days: a.v.Days, Microseconds: a.v.Micros, Valid: true}, nil
}

type intervalEncodePlan struct{ next pgtype.EncodePlan }

func (p *intervalEncodePlan) SetNext(n pgtype.EncodePlan) { p.next = n }
func (p *intervalEncodePlan) Encode(value any, buf []byte) ([]byte, error) {
	return p.next.Encode(intervalValuer{value.(qt.Interval)}, buf)
}

func tryWrapIntervalEncode(value any) (pgtype.WrappedEncodePlanNextSetter, any, bool) {
	if v, ok := value.(qt.Interval); ok {
		return &intervalEncodePlan{}, intervalValuer{v}, true
	}
	return nil, nil, false
}

// ---- ranges ----

func boundType(has, inc bool) pgtype.BoundType {
	switch {
	case !has:
		return pgtype.Unbounded
	case inc:
		return pgtype.Inclusive
	}
	return pgtype.Exclusive
}

// rangeValuer adapts a qt range for encoding (pgtype.RangeValuer).
type rangeValuer struct{ r qt.RangeReader }

func (a rangeValuer) IsNull() bool { return a.r.RangeIsNull() }
func (a rangeValuer) BoundTypes() (lower, upper pgtype.BoundType) {
	if a.r.RangeIsEmpty() {
		return pgtype.Empty, pgtype.Empty
	}
	lo, up := a.r.RangeBounds()
	li, ui := a.r.RangeInclusive()
	return boundType(lo != nil, li), boundType(up != nil, ui)
}
func (a rangeValuer) Bounds() (lower, upper any) { return a.r.RangeBounds() }

// rangeScanner adapts a *qt.Range for decoding (pgtype.RangeScanner).
type rangeScanner struct{ w qt.RangeWriter }

func (a rangeScanner) ScanNull() error                { a.w.RangeSetNull(); return nil }
func (a rangeScanner) ScanBounds() (lower, upper any) { return a.w.RangeScanTargets() }
func (a rangeScanner) SetBoundTypes(lower, upper pgtype.BoundType) error {
	if lower == pgtype.Empty || upper == pgtype.Empty {
		a.w.RangeSet(false, false, false, false, true)
		return nil
	}
	a.w.RangeSet(lower != pgtype.Unbounded, upper != pgtype.Unbounded,
		lower == pgtype.Inclusive, upper == pgtype.Inclusive, false)
	return nil
}

type rangeEncodePlan struct{ next pgtype.EncodePlan }

func (p *rangeEncodePlan) SetNext(n pgtype.EncodePlan) { p.next = n }
func (p *rangeEncodePlan) Encode(value any, buf []byte) ([]byte, error) {
	return p.next.Encode(rangeValuer{value.(qt.RangeReader)}, buf)
}

func tryWrapRangeEncode(value any) (pgtype.WrappedEncodePlanNextSetter, any, bool) {
	if r, ok := value.(qt.RangeReader); ok {
		return &rangeEncodePlan{}, rangeValuer{r}, true
	}
	return nil, nil, false
}

type rangeScanPlan struct{ next pgtype.ScanPlan }

func (p *rangeScanPlan) SetNext(n pgtype.ScanPlan) { p.next = n }
func (p *rangeScanPlan) Scan(src []byte, target any) error {
	return p.next.Scan(src, rangeScanner{target.(qt.RangeWriter)})
}

func tryWrapRangeScan(target any) (pgtype.WrappedScanPlanNextSetter, any, bool) {
	if w, ok := target.(qt.RangeWriter); ok {
		return &rangeScanPlan{}, rangeScanner{w}, true
	}
	return nil, nil, false
}

// ---- multiranges ----

// multirangeGetter adapts a qt multirange for encoding (pgtype.MultirangeGetter).
type multirangeGetter struct{ m qt.MultirangeReader }

func (a multirangeGetter) IsNull() bool    { return a.m.MultirangeIsNull() }
func (a multirangeGetter) Len() int        { return a.m.MultirangeLen() }
func (a multirangeGetter) Index(i int) any { return a.m.MultirangeIndex(i) }
func (a multirangeGetter) IndexType() any  { return a.m.MultirangeIndexType() }

// multirangeSetter adapts a *qt.Multirange for decoding (pgtype.MultirangeSetter).
type multirangeSetter struct{ m qt.MultirangeWriter }

func (a multirangeSetter) ScanNull() error     { a.m.MultirangeSetNull(); return nil }
func (a multirangeSetter) SetLen(n int) error  { a.m.MultirangeSetLen(n); return nil }
func (a multirangeSetter) ScanIndex(i int) any { return a.m.MultirangeScanIndex(i) }
func (a multirangeSetter) ScanIndexType() any  { return a.m.MultirangeScanIndexType() }

type multirangeEncodePlan struct{ next pgtype.EncodePlan }

func (p *multirangeEncodePlan) SetNext(n pgtype.EncodePlan) { p.next = n }
func (p *multirangeEncodePlan) Encode(value any, buf []byte) ([]byte, error) {
	return p.next.Encode(multirangeGetter{value.(qt.MultirangeReader)}, buf)
}

func tryWrapMultirangeEncode(value any) (pgtype.WrappedEncodePlanNextSetter, any, bool) {
	if m, ok := value.(qt.MultirangeReader); ok {
		return &multirangeEncodePlan{}, multirangeGetter{m}, true
	}
	return nil, nil, false
}

type multirangeScanPlan struct{ next pgtype.ScanPlan }

func (p *multirangeScanPlan) SetNext(n pgtype.ScanPlan) { p.next = n }
func (p *multirangeScanPlan) Scan(src []byte, target any) error {
	return p.next.Scan(src, multirangeSetter{target.(qt.MultirangeWriter)})
}

func tryWrapMultirangeScan(target any) (pgtype.WrappedScanPlanNextSetter, any, bool) {
	if w, ok := target.(qt.MultirangeWriter); ok {
		return &multirangeScanPlan{}, multirangeSetter{w}, true
	}
	return nil, nil, false
}
