package pgxexec_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	q "github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/pgcat/ext/postgis"
	"github.com/noobmaisfier/qlx/pgxexec"
	"github.com/noobmaisfier/qlx/qt"
)

// The PostGIS pack against a real server. The extension is not part of core
// PostgreSQL, so the test creates it when the server allows it and skips
// otherwise (the four-major matrix does not install extension binaries).
func TestPostGISExtension(t *testing.T) {
	p := pool(t)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		if _, err := tx.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS postgis"); err != nil {
			t.Skipf("postgis not available: %v", err)
		}

		geomType := q.NewPgType[qt.Geometry]("public", "geometry")
		geom := func(g qt.Geometry) q.Expr[qt.Geometry] { return q.Cast(q.Val(g), geomType) }

		// ST_AsText(ST_GeomFromText('POINT(1 2)'))
		txt, err := pgxexec.One[string](ctx, tx, render(t,
			q.Select(postgis.StAstextGeometry(postgis.StGeomfromtextText(q.Val("POINT(1 2)"))))))
		if err != nil || txt != "POINT(1 2)" {
			t.Fatalf("st_astext(st_geomfromtext): %q %v", txt, err)
		}

		// ST_Distance between POINT(0 0) and POINT(3 4) should be 5
		p1 := postgis.StGeomfromtextText(q.Val("POINT(0 0)"))
		p2 := postgis.StGeomfromtextText(q.Val("POINT(3 4)"))
		d, err := pgxexec.One[float64](ctx, tx, render(t,
			q.Select(postgis.StDistanceGeometryGeometry(p1, p2))))
		if err != nil || d != 5 {
			t.Fatalf("st_distance: %v %v", d, err)
		}

		// Operator &&
		// Bounding box overlap: POINT(0 0) && POINT(3 4) is false
		overlaps, err := pgxexec.One[bool](ctx, tx, render(t,
			q.Select(postgis.OpAmpAmpGeometryGeometry(p1, p2))))
		if err != nil || overlaps {
			t.Fatalf("&& operator: %v %v", overlaps, err)
		}
		pSame := postgis.StGeomfromtextText(q.Val("POINT(0 0)"))
		overlapsSame, err := pgxexec.One[bool](ctx, tx, render(t,
			q.Select(postgis.OpAmpAmpGeometryGeometry(p1, pSame))))
		if err != nil || !overlapsSame {
			t.Fatalf("&& operator same: %v %v", overlapsSame, err)
		}

		// Scan of a geometry column into qt.Geometry (hex EWKB string)
		// e.g. select ST_GeomFromText('POINT(1 2)') into qt.Geometry
		gVal, err := pgxexec.One[qt.Geometry](ctx, tx, render(t,
			q.Select(postgis.StGeomfromtextText(q.Val("POINT(1 2)")))))
		if err != nil || !strings.HasPrefix(string(gVal), "0101000000") {
			t.Fatalf("geometry scan into qt.Geometry: %q %v", gVal, err)
		}

		// Round trip: scan literal cast into qt.Geometry
		back, err := pgxexec.One[qt.Geometry](ctx, tx, render(t,
			q.Select(geom(gVal))))
		if err != nil || back != gVal {
			t.Fatalf("geometry round trip: %q %v", back, err)
		}
	})
}
