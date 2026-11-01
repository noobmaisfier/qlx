package qt

import (
	"reflect"
	"testing"
)

func TestVectorRoundTrip(t *testing.T) {
	v := Vector{1, 2.5, -3}
	val, err := v.Value()
	if err != nil || val != "[1,2.5,-3]" {
		t.Fatalf("%v %v", val, err)
	}
	var back Vector
	if err := back.Scan("[1, 2.5, -3]"); err != nil || !reflect.DeepEqual(back, v) {
		t.Fatalf("%v %v", back, err)
	}
	var null Vector
	if err := null.Scan(nil); err != nil || null != nil {
		t.Fatalf("null: %v %v", null, err)
	}
	if err := back.Scan("1,2"); err == nil {
		t.Fatal("want an error for a malformed vector")
	}
}

func TestHstoreRoundTrip(t *testing.T) {
	h := Hstore{"a": Some("1"), "q\"k": Some(`x\y`), "n": None[string]()}
	val, err := h.Value()
	if err != nil {
		t.Fatal(err)
	}
	var back Hstore
	if err := back.Scan(val); err != nil {
		t.Fatalf("scan %q: %v", val, err)
	}
	if !reflect.DeepEqual(back, h) {
		t.Fatalf("%q -> %v", val, back)
	}
	// What PostgreSQL itself prints.
	if err := back.Scan(`"a"=>"1", "b"=>NULL`); err != nil || len(back) != 2 || back["b"].Valid || back["a"].V != "1" {
		t.Fatalf("%v %v", back, err)
	}
	for _, bad := range []string{`a=>1`, `"a"="1"`, `"a"=>"1" "b"=>"2"`, `"a`} {
		if err := back.Scan(bad); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
}
