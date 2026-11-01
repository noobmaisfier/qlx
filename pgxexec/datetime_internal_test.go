package pgxexec

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/noobmaisfier/qlx/qt"
)

// qt.Date, qt.TimeOfDay, qt.TimeTZ and qt.ID against a real server, in both
// directions and as nullable values, through the arguments the renderer
// hands to pgx (the QLXArg form).
func TestDateTimeAndIDRoundTrip(t *testing.T) {
	dsn := os.Getenv("QLX_TEST_DSN")
	if dsn == "" {
		t.Skip("QLX_TEST_DSN not set")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AfterConnect = AfterConnect
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `DROP TABLE IF EXISTS qt_dt;
CREATE TABLE qt_dt (id bigint PRIMARY KEY, d date, d2 date, t time, tz timetz, span daterange)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DROP TABLE IF EXISTS qt_dt`) })
	if err := qt.SetIDKey([]byte("integration-test-key-0123456789")); err != nil {
		t.Fatal(err)
	}

	id := qt.IDOf[testSpace](12345)
	d := qt.DateOf(2026, time.October, 28)
	tod := qt.TimeOfDayOf(13, 45, 6, 250000)
	tz, err := qt.ParseTimeTZ("13:45:06.25+02")
	if err != nil {
		t.Fatal(err)
	}

	arg := func(v interface{ QLXArg() any }) any { return v.QLXArg() }
	if _, err := pool.Exec(ctx, `INSERT INTO qt_dt VALUES ($1, $2, $3, $4, $5)`,
		arg(id), arg(d), arg(qt.Null[qt.Date]{}), arg(tod), arg(tz)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO qt_dt (id, d) VALUES ($1, $2), ($3, $4)`,
		int64(2), arg(qt.DateInfinity), int64(3), arg(qt.DateNegInfinity)); err != nil {
		t.Fatal(err)
	}

	var (
		gid  qt.ID[testSpace]
		gd   qt.Date
		gd2  qt.Null[qt.Date]
		gt   qt.TimeOfDay
		gtz  qt.TimeTZ
		gnid qt.Null[qt.ID[testSpace]]
	)
	if err := pool.QueryRow(ctx, `SELECT id, d, d2, t, tz, NULL::bigint FROM qt_dt WHERE id = $1`, arg(id)).
		Scan(&gid, &gd, &gd2, &gt, &gtz, &gnid); err != nil {
		t.Fatal(err)
	}
	if gid != id || gd != d || gd2.Valid || gt != tod || gtz != tz || gnid.Valid {
		t.Fatalf("round trip: %v %v %v %v %v %v", gid, gd, gd2, gt, gtz, gnid)
	}
	var inf, ninf qt.Date
	if err := pool.QueryRow(ctx, `SELECT (SELECT d FROM qt_dt WHERE id=2), (SELECT d FROM qt_dt WHERE id=3)`).Scan(&inf, &ninf); err != nil {
		t.Fatal(err)
	}
	if inf != qt.DateInfinity || ninf != qt.DateNegInfinity {
		t.Fatalf("infinity: %v %v", inf, ninf)
	}
	// Comparison happens in the database, so dates compare as dates.
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM qt_dt WHERE d > $1`, arg(d)).Scan(&n); err != nil || n != 1 {
		t.Fatalf("d > date: %d %v", n, err)
	}
	// Ranges over qt.Date.
	var got qt.Range[qt.Date]
	if err := pool.QueryRow(ctx, `SELECT daterange('2026-10-01', '2026-11-01')`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got.Null || got.Lower == nil || *got.Lower != qt.DateOf(2026, time.October, 1) || got.Upper == nil || *got.Upper != qt.DateOf(2026, time.November, 1) {
		t.Fatalf("daterange: %+v", got)
	}
}

type testSpace struct{}

func (testSpace) IDSpace() string { return "public.qt_dt" }
