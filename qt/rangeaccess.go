package qt

// Range and Multirange are generic, but a driver codec has to handle them
// without knowing T. These methods are that bridge: the value is read and
// filled through small non-generic interfaces, so a driver adapter (see
// pgxexec) needs no reflection and qt still imports no driver.
//
// The bound objects exchanged are *T (scan targets) and T (values to encode);
// the driver's own codec for the element type does the conversion.

// RangeReader is implemented by Range[T] values (encoding side).
type RangeReader interface {
	RangeIsNull() bool
	RangeIsEmpty() bool
	// RangeBounds returns the bounds as values; nil means unbounded.
	RangeBounds() (lower, upper any)
	RangeInclusive() (lower, upper bool)
}

// RangeWriter is implemented by *Range[T] (decoding side). The driver calls
// RangeScanTargets, scans into the targets when the bounds exist, then calls
// RangeSet with what the wire said.
type RangeWriter interface {
	RangeSetNull()
	RangeScanTargets() (lower, upper any)
	RangeSet(hasLower, hasUpper, lowerInc, upperInc, empty bool)
}

func (r Range[T]) RangeIsNull() bool  { return r.Null }
func (r Range[T]) RangeIsEmpty() bool { return r.Empty }
func (r Range[T]) RangeBounds() (lower, upper any) {
	if r.Lower != nil {
		lower = *r.Lower
	}
	if r.Upper != nil {
		upper = *r.Upper
	}
	return
}
func (r Range[T]) RangeInclusive() (lower, upper bool) { return r.LowerInc, r.UpperInc }

func (r *Range[T]) RangeSetNull() { *r = Range[T]{Null: true} }
func (r *Range[T]) RangeScanTargets() (lower, upper any) {
	*r = Range[T]{Lower: new(T), Upper: new(T)}
	return r.Lower, r.Upper
}
func (r *Range[T]) RangeSet(hasLower, hasUpper, lowerInc, upperInc, empty bool) {
	if empty {
		*r = Range[T]{Empty: true}
		return
	}
	if !hasLower {
		r.Lower, lowerInc = nil, false
	}
	if !hasUpper {
		r.Upper, upperInc = nil, false
	}
	r.LowerInc, r.UpperInc, r.Empty, r.Null = lowerInc, upperInc, false, false
}

// MultirangeReader is implemented by Multirange[T] values.
type MultirangeReader interface {
	MultirangeIsNull() bool
	MultirangeLen() int
	// MultirangeIndex returns element i as a Range[T] value.
	MultirangeIndex(i int) any
	// MultirangeIndexType returns a zero Range[T], for codec planning.
	MultirangeIndexType() any
}

// MultirangeWriter is implemented by *Multirange[T].
type MultirangeWriter interface {
	MultirangeSetNull()
	MultirangeSetLen(n int)
	// MultirangeScanIndex returns a *Range[T] pointing into element i.
	MultirangeScanIndex(i int) any
	// MultirangeScanIndexType returns a zero *Range[T], for codec planning.
	MultirangeScanIndexType() any
}

func (m Multirange[T]) MultirangeIsNull() bool         { return m == nil }
func (m Multirange[T]) MultirangeLen() int             { return len(m) }
func (m Multirange[T]) MultirangeIndex(i int) any      { return m[i] }
func (m Multirange[T]) MultirangeIndexType() any       { return Range[T]{} }
func (m *Multirange[T]) MultirangeSetNull()            { *m = nil }
func (m *Multirange[T]) MultirangeSetLen(n int)        { *m = make(Multirange[T], n) }
func (m *Multirange[T]) MultirangeScanIndex(i int) any { return &(*m)[i] }
func (m *Multirange[T]) MultirangeScanIndexType() any  { return &Range[T]{} }
