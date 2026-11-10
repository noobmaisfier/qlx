package pgtext

import "testing"

type named struct{ n int }

func (named) String() string { return "named" }

func TestNilPointerWithValueStringerIsNull(t *testing.T) {
	var p *named
	if _, null, err := Elem(p); err != nil || !null {
		t.Fatalf("null=%v err=%v", null, err)
	}
	if s, null, err := Elem(&named{}); err != nil || null || s != "named" {
		t.Fatalf("%q %v %v", s, null, err)
	}
}
