// Package postgis has helpers for the PostGIS queries most applications write:
// points, "within N meters", distances and GeoJSON output. They work on
// geography, so distances are in meters on the spheroid, and WithinMeters can
// use a GIST index on the column.
//
// For anything else, use the generated wrappers in pgcat/ext/postgis.
package postgis

import (
	"github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/pgcat/ext/postgis"
	"github.com/noobmaisfier/qlx/qt"
)

// WGS84 is the SRID of GPS coordinates.
const WGS84 int32 = 4326

var geographyType = qlx.NewPgType[qt.Geography]("", "geography")

// Point is the WGS84 point at lon, lat (in that order, as PostGIS wants them)
// as a geography.
func Point(lon, lat qlx.Operand[float64]) qlx.Expr[qt.Geography] {
	geom := postgis.StSetsridGeometryInt4(postgis.StMakepointFloat8Float8(lon, lat), qlx.Val(WGS84))
	return qlx.Cast(geom, geographyType)
}

// WithinMeters is true when col is at most meters from p.
func WithinMeters(col, p qlx.Operand[qt.Geography], meters float64) qlx.Pred {
	return qlx.BoolExpr{Expr: postgis.StDwithinGeographyGeographyFloat8Bool2(col, p, qlx.Val(meters), qlx.Val(true))}.Pred()
}

// DistanceMeters is the distance from a to b in meters.
func DistanceMeters(a, b qlx.Operand[qt.Geography]) qlx.Expr[float64] {
	return postgis.StDistanceGeographyGeographyBool(a, b, qlx.Val(true))
}

// GeoJSON renders col as a GeoJSON geometry, with at most digits decimals.
func GeoJSON(col qlx.Operand[qt.Geography], digits int32) qlx.Expr[string] {
	return postgis.StAsgeojsonGeographyInt4Int4(col, qlx.Val(digits), qlx.Val(int32(0)))
}
