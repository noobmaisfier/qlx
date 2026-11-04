package pgtext

import "testing"

func TestElemAndLiterals(t *testing.T) {
	for _, c := range []struct {
		in   any
		want string
		null bool
	}{
		{int32(-7), "-7", false}, {"a\"b", "a\"b", false}, {true, "t", false},
		{[]byte{1, 2}, `\x0102`, false}, {nil, "", true}, {(*int)(nil), "", true},
		{[16]byte{0, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}, "00112233-4455-6677-8899-aabbccddeeff", false},
	} {
		got, null, err := Elem(c.in)
		if err != nil || got != c.want || null != c.null {
			t.Errorf("Elem(%#v) = %q %v %v, want %q %v", c.in, got, null, err, c.want, c.null)
		}
	}
	if s, _ := ArrayElem("a\\b\"c"); s != `"a\\b\"c"` {
		t.Errorf("ArrayElem quoting: %s", s)
	}
	if s, _ := RangeLiteral(false, 1, nil, true, true); s != `["1",)` {
		t.Errorf("unbounded upper keeps ')': %s", s)
	}
	if s, _ := RangeLiteral(true, nil, nil, false, false); s != "empty" {
		t.Errorf("empty: %s", s)
	}
}
