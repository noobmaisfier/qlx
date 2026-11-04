package qt_test

import (
	"net/netip"
	"testing"

	"github.com/noobmaisfier/qlx/qt"
)

func TestNullScanTextSources(t *testing.T) {
	var a qt.Null[netip.Addr]
	if err := a.Scan("10.0.0.1"); err != nil || !a.Valid || a.V != netip.MustParseAddr("10.0.0.1") {
		t.Fatalf("addr: %v %+v", err, a)
	}
	if err := a.Scan("10.0.0.1/32"); err != nil || a.V != netip.MustParseAddr("10.0.0.1") {
		t.Fatalf("host mask: %v %+v", err, a)
	}
	if err := a.Scan("10.0.0.0/8"); err == nil {
		t.Fatal("network accepted as address")
	}
	var p qt.Null[netip.Prefix]
	if err := p.Scan([]byte("10.0.0.0/8")); err != nil || !p.Valid || p.V != netip.MustParsePrefix("10.0.0.0/8") {
		t.Fatalf("prefix: %v %+v", err, p)
	}
	var u qt.Null[[16]byte]
	if err := u.Scan("00112233-4455-6677-8899-aabbccddeeff"); err != nil || !u.Valid || u.V[0] != 0 || u.V[15] != 0xff {
		t.Fatalf("uuid: %v %+v", err, u)
	}
	if err := a.Scan(nil); err != nil || a.Valid {
		t.Fatalf("nil: %v %+v", err, a)
	}
	if err := a.Scan("not an ip"); err == nil {
		t.Fatal("bad address accepted")
	}
	var s qt.Null[string]
	if err := s.Scan("x"); err != nil || s.V != "x" {
		t.Fatal(err)
	}
}
