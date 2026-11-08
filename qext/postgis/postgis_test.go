package postgis_test

import (
	"strings"
	"testing"

	"github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/qext/postgis"
)

func TestShapes(t *testing.T) {
	p := postgis.Point(qlx.Val(2.35), qlx.Val(48.85))
	st, err := qlx.Render(qlx.Dialect{Version: qlx.PG18}, qlx.Select(
		qlx.Test(postgis.WithinMeters(p, p, 500)),
		postgis.DistanceMeters(p, p),
		postgis.GeoJSON(p, 5)))
	if err != nil {
		t.Fatal(err)
	}
	// st_dwithin, not the internal _st_dwithin, which cannot use an index.
	for _, w := range []string{`"public"."st_dwithin"(`, `"st_setsrid"(`, `"st_makepoint"(`, `AS "geography"`, `"st_distance"(`, `"st_asgeojson"(`} {
		if !strings.Contains(st.SQL, w) {
			t.Errorf("%q lacks %q", st.SQL, w)
		}
	}
	if strings.Contains(st.SQL, `"_st_`) {
		t.Errorf("internal function in %q", st.SQL)
	}
}
