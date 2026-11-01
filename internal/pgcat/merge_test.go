package pgcat_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/noobmaisfier/qlx/internal/pgcat"
)

func fns(vs []int, names ...string) *pgcat.Ledger {
	l := &pgcat.Ledger{Versions: vs}
	for _, n := range names {
		l.Funcs = append(l.Funcs, pgcat.Fn{Name: n, Args: []string{}})
	}
	return l
}

func marks(l *pgcat.Ledger) map[string][2]int {
	out := map[string][2]int{}
	for _, f := range l.Funcs {
		out[f.Name] = [2]int{f.Since, f.Until}
	}
	return out
}

func TestMergeUntil(t *testing.T) {
	// 16 has old+both; 17 has both; 18 has both+new; 19 has both+new+newer.
	m := pgcat.Merge(
		fns([]int{16}, "old", "both"),
		fns([]int{17}, "both"),
		fns([]int{18}, "both", "new"),
		fns([]int{19}, "both", "new", "newer"),
	)
	want := map[string][2]int{
		"both":  {0, 0},
		"old":   {0, 17}, // gone from 17
		"new":   {18, 0},
		"newer": {19, 0},
	}
	got := marks(m)
	for n, w := range want {
		if got[n] != w {
			t.Errorf("%s: since/until %v, want %v", n, got[n], w)
		}
	}
}

func TestMergeUntilIsOnlyAsPreciseAsTheVersionsMerged(t *testing.T) {
	// Without a 17 ledger the removal can only be placed at the next version
	// that is known to lack the function.
	m := pgcat.Merge(fns([]int{16}, "old", "both"), fns([]int{18}, "both"))
	if got := marks(m)["old"]; got != [2]int{0, 18} {
		t.Fatalf("old: %v, want {0 18}", got)
	}
}

func TestMergeIsIncremental(t *testing.T) {
	ab := pgcat.Merge(fns([]int{16}, "old", "both"), fns([]int{18}, "both", "new"))
	abc := pgcat.Merge(ab, fns([]int{19}, "both", "new"))
	d := pgcat.Merge(pgcat.Merge(fns([]int{16}, "old", "both"), fns([]int{18}, "both", "new")), fns([]int{19}, "both", "new"))
	for n, w := range map[string][2]int{"old": {0, 18}, "both": {0, 0}, "new": {18, 0}} {
		if got := marks(abc)[n]; got != w {
			t.Errorf("incremental %s: %v, want %v", n, got, w)
		}
		if got := marks(d)[n]; got != w {
			t.Errorf("nested %s: %v, want %v", n, got, w)
		}
	}
	// A function added in 18 and dropped in 19 is bounded on both sides.
	mid := pgcat.Merge(fns([]int{16}, "keep"), fns([]int{18}, "keep", "blip"), fns([]int{19}, "keep"))
	if got := marks(mid)["blip"]; got != [2]int{18, 19} {
		t.Errorf("blip: %v, want {18 19}", got)
	}
}

func TestMergeOperatorsAndCasts(t *testing.T) {
	a := &pgcat.Ledger{Versions: []int{16},
		Ops:   []pgcat.Op{{Name: "@@@", Left: "a", Right: "b"}},
		Casts: []pgcat.Cast{{From: "a", To: "b", Context: "e"}}}
	b := &pgcat.Ledger{Versions: []int{19}}
	m := pgcat.Merge(a, b)
	if m.Ops[0].Until != 19 || m.Casts[0].Until != 19 || m.Ops[0].Since != 0 {
		t.Fatalf("op %+v cast %+v", m.Ops[0], m.Casts[0])
	}
}

func TestUntilSurvivesJSONRoundTrip(t *testing.T) {
	m := pgcat.Merge(fns([]int{16}, "old"), fns([]int{18}))
	data, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"until": 18`) {
		t.Fatalf("until missing from ledger JSON:\n%s", data)
	}
	back, err := pgcat.Parse(data)
	if err != nil || marks(back)["old"] != [2]int{0, 18} {
		t.Fatalf("round trip: %v %v", err, marks(back))
	}
}

func TestGenerateEmitsRangeGates(t *testing.T) {
	l := &pgcat.Ledger{Versions: []int{16, 18, 19}, Funcs: []pgcat.Fn{
		{Name: "gone_in_18", Args: []string{"int4"}, Ret: "int4", Strict: true, Volatility: "i", Until: 18},
		{Name: "new_in_18", Args: []string{"int4"}, Ret: "int4", Strict: true, Volatility: "i", Since: 18},
		{Name: "blip", Args: []string{"int4"}, Ret: "int4", Strict: true, Volatility: "i", Since: 18, Until: 19},
		{Name: "always", Args: []string{"int4"}, Ret: "int4", Strict: true, Volatility: "i"},
	}}
	res, err := pgcat.Generate(l, "pgfn")
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, f := range res.Files {
		all.Write(f)
	}
	src := all.String()
	for _, want := range []string{
		`FuncCallRange[int32](0, 18, "pg_catalog", "gone_in_18"`,
		`FuncCallRange[int32](18, 19, "pg_catalog", "blip"`,
		`FuncCallSince[int32](18, "pg_catalog", "new_in_18"`,
		`FuncCall[int32]("pg_catalog", "always"`,
		"Removed in PostgreSQL 18.",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated code lacks %s", want)
		}
	}
}

func recFn(v int, cols ...string) *pgcat.Ledger {
	f := pgcat.Fn{Name: "pg_stat_get_wal", Kind: "f", Args: []string{}, Ret: "record", RetSet: true, OutModes: true, Volatility: "s"}
	for _, c := range cols {
		f.Outs = append(f.Outs, pgcat.Out{Name: c, Type: "int8"})
	}
	return &pgcat.Ledger{Versions: []int{v}, Funcs: []pgcat.Fn{f}}
}

// A function that keeps its name and arguments but changes its record between
// majors is one entry per shape, each with its own version range.
func TestMergeSplitsRecordShapes(t *testing.T) {
	wide := []string{"a", "b", "c", "d"}
	m := pgcat.Merge(recFn(16, wide...), recFn(17, wide...), recFn(18, "a", "b"), recFn(19, "a", "b", "z"))
	if len(m.Funcs) != 3 {
		t.Fatalf("want 3 shapes, got %d: %+v", len(m.Funcs), m.Funcs)
	}
	got := map[string][2]int{}
	for _, f := range m.Funcs {
		got[fmt.Sprint(len(f.Outs))] = [2]int{f.Since, f.Until}
	}
	want := map[string][2]int{"4": {0, 18}, "2": {18, 19}, "3": {19, 0}}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("shape with %s columns: range %v, want %v", k, got[k], w)
		}
	}
	// A ledger read before columns were recorded adopts a known shape.
	old := recFn(20)
	old.Funcs[0].Outs = nil
	if m2 := pgcat.Merge(m, old); len(m2.Funcs) != 3 {
		t.Fatalf("a ledger without OUT columns must not add a shape: %d", len(m2.Funcs))
	}
}

// Shapes of one signature are told apart by the versions they exist in.
func TestGenerateNamesShapeVariantsByVersion(t *testing.T) {
	mk := func(since, until int, cols ...string) pgcat.Fn {
		f := pgcat.Fn{Name: "pg_stat_get_wal", Kind: "f", Args: []string{}, Ret: "record", RetSet: true,
			OutModes: true, Volatility: "s", Since: since, Until: until}
		for _, c := range cols {
			f.Outs = append(f.Outs, pgcat.Out{Name: c, Type: "int8"})
		}
		return f
	}
	l := &pgcat.Ledger{Versions: []int{16, 17, 18, 19}, Funcs: []pgcat.Fn{
		mk(0, 18, "a", "b", "c"), mk(18, 19, "a", "b"), mk(19, 0, "a", "b", "z"), mk(17, 19, "q"), mk(16, 19, "w", "x"),
	}}
	res, err := pgcat.Generate(l, "pgfn")
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, f := range res.Files {
		all.Write(f)
	}
	for _, want := range []string{"func PgStatGetWalUpTo17()", "func PgStatGetWalV18()", "func PgStatGetWalFrom19()", "func PgStatGetWalV17To18()", "func PgStatGetWalV16To18()"} {
		if !strings.Contains(all.String(), want) {
			t.Errorf("generated code lacks %s", want)
		}
	}
}

// An extension ledger merged from several majors bounds its wrappers like the
// catalog does: a function that appears in 18 carries Since, one that is gone
// in 19 carries Until, and both keep the extension's schema.
func TestGenerateExtensionRespectsVersionRange(t *testing.T) {
	ext := func(v int, names ...string) *pgcat.Ledger {
		l := &pgcat.Ledger{Extension: "pg_trgm", Schema: "public", Versions: []int{v}}
		for _, n := range names {
			l.Funcs = append(l.Funcs, pgcat.Fn{Name: n, Kind: "f", Args: []string{"text", "text"}, Ret: "float4", Volatility: "i", Strict: true, Schema: "public"})
		}
		return l
	}
	m := pgcat.Merge(
		ext(16, "similarity", "old_fn"),
		ext(17, "similarity", "old_fn"),
		ext(18, "similarity", "old_fn", "new_fn"),
		ext(19, "similarity", "new_fn"),
	)
	byName := map[string][2]int{}
	for _, f := range m.Funcs {
		byName[f.Name] = [2]int{f.Since, f.Until}
	}
	want := map[string][2]int{"similarity": {0, 0}, "old_fn": {0, 19}, "new_fn": {18, 0}}
	for n, w := range want {
		if byName[n] != w {
			t.Errorf("merged range of %s = %v, want %v", n, byName[n], w)
		}
	}
	if m.Extension != "pg_trgm" || m.Schema != "public" || len(m.Versions) != 4 {
		t.Fatalf("merge lost the extension identity: %q %q %v", m.Extension, m.Schema, m.Versions)
	}
	res, err := pgcat.Generate(m, "pg")
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for name, f := range res.Files {
		if strings.Contains(name, "ext/pgtrgm/") {
			all.Write(f)
		}
	}
	src := all.String()
	if src == "" {
		t.Fatalf("no file generated under ext/pgtrgm: %v", keys(res.Files))
	}
	for _, want := range []string{
		`FuncCallSince[float32](18, "public", "new_fn"`,
		`FuncCallRange[float32](0, 19, "public", "old_fn"`,
		"Requires PostgreSQL 18.",
		"Removed in PostgreSQL 19.",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated extension code lacks %q", want)
		}
	}
	// The server test skips wrappers outside their range, so it must know it.
	for _, want := range []string{`{"NewFn", 18, 0,`, `{"OldFn", 0, 19,`, `{"Similarity", 0, 0,`} {
		if !strings.Contains(src, want) {
			t.Errorf("generated server test lacks the range entry %q", want)
		}
	}
	// An unbounded function carries no gate at all.
	if i := strings.Index(src, `"similarity"`); i < 0 || strings.Contains(src[i-80:i], "Since") || strings.Contains(src[i-80:i], "Range") {
		t.Errorf("similarity must not be version-gated")
	}
}

func keys(m map[string][]byte) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
