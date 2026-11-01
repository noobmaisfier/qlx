package qt

import "testing"

// fill plays the part of a driver: the sequence of calls pgx's codec makes
// when decoding a range value.
func fill(w RangeWriter, null, empty, hasLo, hasUp, loInc, upInc bool, lo, up any) {
	if null {
		w.RangeSetNull()
		return
	}
	l, u := w.RangeScanTargets()
	if hasLo {
		*(l.(*int32)) = lo.(int32)
	}
	if hasUp {
		*(u.(*int32)) = up.(int32)
	}
	w.RangeSet(hasLo, hasUp, loInc, upInc, empty)
}

func TestRangeDecodeSequences(t *testing.T) {
	var r Range[int32]
	fill(&r, false, false, true, true, true, false, int32(1), int32(10))
	if r.Lower == nil || *r.Lower != 1 || *r.Upper != 10 || !r.LowerInc || r.UpperInc || r.Empty || r.Null {
		t.Fatalf("[1,10): %+v", r)
	}
	fill(&r, false, false, true, false, true, true, int32(5), nil)
	if r.Upper != nil || r.UpperInc || r.Lower == nil {
		t.Fatalf("[5,): upper must be unbounded and exclusive: %+v", r)
	}
	fill(&r, false, true, false, false, false, false, nil, nil)
	if !r.Empty || r.Lower != nil || r.Upper != nil {
		t.Fatalf("empty: %+v", r)
	}
	fill(&r, true, false, false, false, false, false, nil, nil)
	if !r.Null || r.Empty {
		t.Fatalf("null: %+v", r)
	}
	fill(&r, false, false, false, false, false, false, nil, nil)
	if r.Null || r.Lower != nil || r.Upper != nil {
		t.Fatalf("(,): a decoded unbounded range is not NULL: %+v", r)
	}
}

func TestRangeEncodeView(t *testing.T) {
	lo, up := int32(3), int32(8)
	var rd RangeReader = Range[int32]{Lower: &lo, Upper: &up, LowerInc: true}
	l, u := rd.RangeBounds()
	if l.(int32) != 3 || u.(int32) != 8 {
		t.Fatalf("bounds %v %v", l, u)
	}
	if li, ui := rd.RangeInclusive(); !li || ui {
		t.Fatal("inclusivity")
	}
	var open RangeReader = Range[int32]{Upper: &up}
	if l, _ := open.RangeBounds(); l != nil {
		t.Fatal("an unbounded side must come out as nil")
	}
	if !(Range[int32]{Null: true}).RangeIsNull() || !(Range[int32]{Empty: true}).RangeIsEmpty() {
		t.Fatal("null/empty flags")
	}
}

func TestMultirangeDecode(t *testing.T) {
	var m Multirange[int32]
	var w MultirangeWriter = &m
	w.MultirangeSetLen(2)
	for i, v := range []int32{1, 7} {
		r := w.MultirangeScanIndex(i).(*Range[int32])
		l, u := r.RangeScanTargets()
		*(l.(*int32)), *(u.(*int32)) = v, v+2
		r.RangeSet(true, true, true, false, false)
	}
	if len(m) != 2 || *m[0].Lower != 1 || *m[1].Upper != 9 {
		t.Fatalf("%+v", m)
	}
	var rd MultirangeReader = m
	if rd.MultirangeLen() != 2 || rd.MultirangeIndex(1).(Range[int32]).Upper == nil {
		t.Fatal("reader view")
	}
	w.MultirangeSetLen(0)
	if m == nil || len(m) != 0 {
		t.Fatal("an empty multirange must be non-nil (nil is NULL)")
	}
	w.MultirangeSetNull()
	if !m.MultirangeIsNull() {
		t.Fatal("null")
	}
	if _, ok := rd.MultirangeIndexType().(Range[int32]); !ok {
		t.Fatal("index type must be a Range value")
	}
	if _, ok := w.MultirangeScanIndexType().(*Range[int32]); !ok {
		t.Fatal("scan index type must be a *Range")
	}
}
