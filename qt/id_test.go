package qt

import (
	"encoding/json"
	"errors"
	"testing"
)

type spaceA struct{}

func (spaceA) IDSpace() string { return "public.users" }

type spaceB struct{}

func (spaceB) IDSpace() string { return "public.orders" }

func setTestKey(t testing.TB) {
	t.Helper()
	if err := SetIDKey([]byte("0123456789abcdef-test-key")); err != nil {
		t.Fatal(err)
	}
}

func TestIDRoundTrip(t *testing.T) {
	setTestKey(t)
	vals := []int64{0, 1, 2, 3, 41, 1000, 1 << 31, 1<<32 + 5, idMax - 1}
	for n := int64(0); n < 20000; n++ {
		vals = append(vals, n*7919)
	}
	for _, n := range vals {
		id := IDOf[spaceA](n)
		s := id.String()
		if len(s) != idTextLen {
			t.Fatalf("%d: len %d", n, len(s))
		}
		back, err := ParseID[spaceA](s)
		if err != nil || back != id {
			t.Fatalf("%d -> %q -> %v, %v", n, s, back, err)
		}
	}
}

func TestIDBijectionNoCollisions(t *testing.T) {
	setTestKey(t)
	seen := make(map[string]int64, 300000)
	for n := int64(0); n < 300000; n++ {
		s := IDOf[spaceA](n).String()
		if p, dup := seen[s]; dup {
			t.Fatalf("%d and %d both encode to %s", p, n, s)
		}
		seen[s] = n
	}
}

func TestIDLooksRandom(t *testing.T) {
	setTestKey(t)
	first := map[byte]bool{}
	for n := int64(1); n <= 1000; n++ {
		first[IDOf[spaceA](n).String()[0]] = true
	}
	if len(first) < 20 {
		t.Fatalf("consecutive ids share their leading characters: %d distinct", len(first))
	}
}

func TestIDSpacesDiffer(t *testing.T) {
	setTestKey(t)
	same := 0
	for n := int64(1); n <= 1000; n++ {
		if IDOf[spaceA](n).String() == IDOf[spaceB](n).String() {
			same++
		}
	}
	if same != 0 {
		t.Fatalf("%d ids encode identically in two tables", same)
	}
	// Another table's id is not accepted.
	s := IDOf[spaceA](42).String()
	if _, err := ParseID[spaceB](s); !errors.Is(err, ErrBadID) {
		t.Fatalf("cross-table id accepted: %v", err)
	}
}

func TestIDTamper(t *testing.T) {
	setTestKey(t)
	s := []byte(IDOf[spaceA](123456).String())
	accepted, tried := 0, 0
	for p := range s {
		orig := s[p]
		for i := 0; i < len(idAlpha); i++ {
			if idAlpha[i] == orig {
				continue
			}
			s[p] = idAlpha[i]
			tried++
			if _, err := ParseID[spaceA](string(s)); err == nil {
				accepted++
			}
		}
		s[p] = orig
	}
	// Each forged string passes the 16-bit tag with probability 2^-16.
	if accepted > 3 {
		t.Fatalf("%d of %d forged strings accepted", accepted, tried)
	}
}

func TestIDRejects(t *testing.T) {
	setTestKey(t)
	for _, bad := range []string{"", "short", "123456789AB1", "0OIl0OIl0OI", "zzzzzzzzzzz", "11111111111\n"} {
		if _, err := ParseID[spaceA](bad); !errors.Is(err, ErrBadID) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if _, err := (ID[spaceA]{n: -1}).MarshalText(); !errors.Is(err, ErrIDRange) {
		t.Errorf("negative: %v", err)
	}
	if _, err := (ID[spaceA]{n: idMax}).MarshalText(); !errors.Is(err, ErrIDRange) {
		t.Errorf("too large: %v", err)
	}
}

func TestIDKeyNotSet(t *testing.T) {
	old := idKey.Swap(nil)
	defer idKey.Store(old)
	if _, err := (ID[spaceA]{n: 1}).MarshalText(); !errors.Is(err, ErrIDKey) {
		t.Fatalf("got %v", err)
	}
	if _, err := ParseID[spaceA]("11111111111"); !errors.Is(err, ErrIDKey) {
		t.Fatalf("got %v", err)
	}
	if err := SetIDKey([]byte("short")); err == nil {
		t.Fatal("short key accepted")
	}
}

func TestIDKeyChangesText(t *testing.T) {
	setTestKey(t)
	a := IDOf[spaceA](5).String()
	if err := SetIDKey([]byte("another-secret-0123456789")); err != nil {
		t.Fatal(err)
	}
	b := IDOf[spaceA](5).String()
	setTestKey(t)
	if a == b {
		t.Fatal("two keys produce the same text")
	}
	if IDOf[spaceA](5).String() != a {
		t.Fatal("same key must reproduce the same text")
	}
}

func TestIDJSONAndNull(t *testing.T) {
	setTestKey(t)
	type row struct {
		ID     ID[spaceA]       `json:"id"`
		Parent Null[ID[spaceA]] `json:"parent"`
	}
	in := row{ID: IDOf[spaceA](77), Parent: Some(IDOf[spaceA](5))}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"` + in.ID.String() + `","parent":"` + in.Parent.V.String() + `"}`
	if string(b) != want {
		t.Fatalf("got %s want %s", b, want)
	}
	var out row
	if err := json.Unmarshal(b, &out); err != nil || out != in {
		t.Fatalf("%v %+v", err, out)
	}
	if err := json.Unmarshal([]byte(`{"id":"nope"}`), &out); err == nil {
		t.Fatal("bad id accepted by JSON")
	}
}

func TestIDScanValue(t *testing.T) {
	var id ID[spaceA]
	for _, src := range []any{int64(9), int32(9), int16(9), []byte("9"), "9"} {
		id = ID[spaceA]{}
		if err := id.Scan(src); err != nil || id.Int64() != 9 {
			t.Fatalf("%T: %v %v", src, err, id)
		}
	}
	if err := id.Scan(nil); err == nil {
		t.Fatal("NULL into ID")
	}
	var n Null[ID[spaceA]]
	if err := n.Scan(int64(4)); err != nil || !n.Valid || n.V.Int64() != 4 {
		t.Fatalf("%v %+v", err, n)
	}
	if v, _ := IDOf[spaceA](4).Value(); v != int64(4) {
		t.Fatalf("value %v", v)
	}
}

func TestIDNoAllocs(t *testing.T) {
	setTestKey(t)
	id := IDOf[spaceA](123456789)
	buf := make([]byte, 0, 64)
	if a := testing.AllocsPerRun(1000, func() { buf, _ = id.AppendText(buf[:0]) }); a != 0 {
		t.Errorf("AppendText allocates %v", a)
	}
	var out ID[spaceA]
	txt := []byte(id.String())
	if a := testing.AllocsPerRun(1000, func() { _ = out.UnmarshalText(txt) }); a != 0 {
		t.Errorf("UnmarshalText allocates %v", a)
	}
}

func BenchmarkIDEncode(b *testing.B) {
	setTestKey(b)
	id := IDOf[spaceA](123456789)
	buf := make([]byte, 0, 16)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf, _ = id.AppendText(buf[:0])
	}
}

func BenchmarkIDDecode(b *testing.B) {
	setTestKey(b)
	txt := []byte(IDOf[spaceA](123456789).String())
	var id ID[spaceA]
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = id.UnmarshalText(txt)
	}
}
