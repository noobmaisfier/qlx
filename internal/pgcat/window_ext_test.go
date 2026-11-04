package pgcat_test

import (
	"strings"
	"testing"

	"github.com/noobmaisfier/qlx/internal/pgcat"
)

// A window function of an extension (PostGIS ST_ClusterDBSCAN) is generated as
// a qlx.WinFn: it needs Over, and its schema is the extension's. The same
// function in pg_catalog stays hand-written in the core package.
func TestGenerateExtensionWindowFunction(t *testing.T) {
	win := pgcat.Fn{Name: "st_clusterdbscan", Kind: "w", Args: []string{"geometry", "float8", "int4"}, Ret: "int4", Volatility: "i", Schema: "public"}
	ext := &pgcat.Ledger{Extension: "postgis", Schema: "public", Versions: []int{18}, Funcs: []pgcat.Fn{win}}
	res, err := pgcat.Generate(ext, "pg")
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for name, f := range res.Files {
		if strings.Contains(name, "ext/postgis/") {
			all.Write(f)
		}
	}
	src := all.String()
	for _, want := range []string{
		`) qlx.WinFn[qt.Null[int32]] {`,
		`qlx.WinCall[qt.Null[int32]](0, 0, "public", "st_clusterdbscan"`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated source lacks %q:\n%s", want, src)
		}
	}
	if res.Report.Skipped["window function (hand-written in the core package)"] != 0 {
		t.Errorf("an extension window function was skipped: %v", res.Report.Skipped)
	}

	core := &pgcat.Ledger{Schema: "pg_catalog", Versions: []int{18}, Funcs: []pgcat.Fn{{Name: "row_number", Kind: "w", Ret: "int8", Volatility: "i"}}}
	res, err = pgcat.Generate(core, "pg")
	if err != nil {
		t.Fatal(err)
	}
	if res.Report.Skipped["window function (hand-written in the core package)"] != 1 {
		t.Errorf("a core window function must stay skipped: %v", res.Report.Skipped)
	}
}
