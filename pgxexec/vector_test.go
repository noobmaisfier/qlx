package pgxexec_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	q "github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/pgcat/ext/vector"
	"github.com/noobmaisfier/qlx/pgxexec"
	"github.com/noobmaisfier/qlx/qt"
)

// The pgvector pack against a real server. The extension is not part of core
// PostgreSQL, so the test creates it when the server allows it and skips
// otherwise (the four-major matrix does not install extension binaries).
func TestPGVectorExtension(t *testing.T) {
	p := pool(t)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		if _, err := tx.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector"); err != nil {
			t.Skipf("pgvector not available: %v", err)
		}

		vt := q.NewPgType[qt.Vector]("public", "vector")
		vec := func(v qt.Vector) q.Expr[qt.Vector] { return q.Cast(q.Val(v), vt) }

		l2, err := pgxexec.One[float64](ctx, tx, render(t,
			q.Select(vector.L2DistanceVectorVector(vec(qt.Vector{1, 2}), vec(qt.Vector{4, 6})))))
		if err != nil || l2 != 5 {
			t.Fatalf("l2_distance: %v %v", l2, err)
		}

		op, err := pgxexec.One[qt.Null[float64]](ctx, tx, render(t,
			q.Select(vector.OpLtMinusGtVectorVector(vec(qt.Vector{1, 2}), vec(qt.Vector{4, 6})))))
		if err != nil || !op.Valid || op.V != 5 {
			t.Fatalf("<->: %+v %v", op, err)
		}

		s, err := pgxexec.One[string](ctx, tx, render(t,
			q.Select(q.Cast(vec(qt.Vector{1, 2, 3}), q.PgText))))
		if err != nil || strings.TrimSpace(s) != "[1,2,3]" {
			t.Fatalf("vector round trip: %q %v", s, err)
		}

		back, err := pgxexec.One[qt.Vector](ctx, tx, render(t,
			q.Select(q.Cast(q.Val("[1,2,3]"), vt))))
		if err != nil || len(back) != 3 || back[0] != 1 || back[2] != 3 {
			t.Fatalf("vector scan: %v %v", back, err)
		}
	})
}
