package pgxexec_test

// Integration tests for the pgx executor. NOT RUN YET: pgx could not be
// downloaded where this was written. They need a database that already holds
// fixture/schema.sql:
//
//	createdb qlx_test && psql qlx_test -f ../fixture/schema.sql
//	QLX_TEST_DSN=postgres:///qlx_test go test ./...

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	q "github.com/noobmaisfier/qlx"
	fx "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/internal/catalog"
	"github.com/noobmaisfier/qlx/pgxexec"
	"github.com/noobmaisfier/qlx/qt"
)

func pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("QLX_TEST_DSN")
	if dsn == "" {
		t.Skip("QLX_TEST_DSN not set")
	}
	p, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

// clean runs in a rolled-back transaction so tests never leave rows behind.
func inRollbackTx(t *testing.T, p *pgxpool.Pool, f func(ctx context.Context, tx pgx.Tx)) {
	t.Helper()
	ctx := context.Background()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	f(ctx, tx)
}

func render(t *testing.T, s q.Statement) q.Stmt {
	t.Helper()
	st, err := q.Render(q.Dialect{Version: q.PG18}, s)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

type userRow struct {
	ID    int64           `db:"id"`
	Email string          `db:"email"`
	Nick  qt.Null[string] `db:"nickname"`
}

func TestInsertSelectRoundTrip(t *testing.T) {
	p := pool(t)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		ins := render(t, q.InsertInto(fx.Users).Row(fx.Users.Email.To("a@example.com"), fx.Users.Nickname.To(qt.None[string]())))
		if _, err := pgxexec.Exec(ctx, tx, ins); err != nil {
			t.Fatal(err)
		}
		sel := render(t, q.Select(fx.Users.ID.As("id"), fx.Users.Email.As("email"), fx.Users.Nickname.As("nickname")).
			From(fx.Users).Where(fx.Users.Email.Eq("a@example.com")))
		rows, err := pgxexec.All[userRow](ctx, tx, sel)
		if err != nil || len(rows) != 1 || rows[0].Nick.Valid {
			t.Fatalf("by name: %+v %v", rows, err)
		}
		tup, err := pgxexec.AllScan[q.Tuple2[int64, string]](ctx, tx, render(t,
			q.Select(fx.Users.ID, fx.Users.Email).From(fx.Users).Where(fx.Users.Email.Eq("a@example.com"))))
		if err != nil || len(tup) != 1 || tup[0].V2 != "a@example.com" {
			t.Fatalf("tuple: %+v %v", tup, err)
		}
	})
}

func TestUniqueViolationIsClassified(t *testing.T) {
	p := pool(t)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		ins := render(t, q.InsertInto(fx.Users).Row(fx.Users.Email.To("dup@example.com")))
		if _, err := pgxexec.Exec(ctx, tx, ins); err != nil {
			t.Fatal(err)
		}
		_, err := pgxexec.Exec(ctx, tx, ins)
		if !pgxexec.IsUnique(err, "users_email_key") {
			t.Fatalf("not classified as unique violation: %v", err)
		}
	})
}

func TestBatchAndCopy(t *testing.T) {
	p := pool(t)
	ctx := context.Background()
	conn, err := p.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	n, err := pgxexec.CopyIn(ctx, tx, q.CopyIn(fx.Users, fx.Users.Email), pgxexec.CopyRows([][]any{{"c1@example.com"}, {"c2@example.com"}}))
	if err != nil || n != 2 {
		t.Fatalf("copy: %d %v", n, err)
	}
	var b pgxexec.Batch
	b.Queue(render(t, q.Select(q.CountAll()).From(fx.Users).Where(fx.Users.Email.Like("c_@example.com"))))
	b.Queue(render(t, q.Select(fx.Users.Email).From(fx.Users).Where(fx.Users.Email.Like("c_@example.com")).OrderBy(fx.Users.Email.Asc())))
	res, err := b.Send(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Close()
	got, err := pgxexec.BatchOneScan[q.Tuple1[int64]](res)
	if err != nil || got.V1 < 2 {
		t.Fatalf("batch count: %+v %v", got, err)
	}
	emails, err := pgxexec.BatchAllScan[q.Tuple1[string]](res)
	if err != nil || len(emails) < 2 || emails[0].V1 != "c1@example.com" {
		t.Fatalf("batch rows: %+v %v", emails, err)
	}
}

func TestVerifySchema(t *testing.T) {
	p := pool(t)
	ctx := context.Background()
	rows, err := p.Query(ctx, catalog.FingerprintSQL, []string{"public"})
	if err != nil {
		t.Fatal(err)
	}
	live := map[string]string{}
	for rows.Next() {
		var tbl, fp string
		if err := rows.Scan(&tbl, &fp); err != nil {
			t.Fatal(err)
		}
		live[tbl] = fp
	}
	spec := q.SchemaSpec{Schemas: []string{"public"}, Fingerprints: live, FingerprintSQL: catalog.FingerprintSQL}
	if err := pgxexec.VerifySchema(ctx, p, spec, "public.users"); err != nil {
		t.Fatalf("matching schema rejected: %v", err)
	}
	spec.Fingerprints = map[string]string{"public.users": "stale"}
	if err := pgxexec.VerifySchema(ctx, p, spec, "public.users"); err == nil {
		t.Fatal("changed schema accepted")
	}
}

func TestInTxRetriesSerializationFailure(t *testing.T) {
	p := pool(t)
	attempts := 0
	err := pgxexec.InTx(context.Background(), p, pgx.TxOptions{}, pgxexec.Retry{Attempts: 3}, func(ctx context.Context, tx pgx.Tx) error {
		attempts++
		if attempts < 3 {
			_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(1); DO $$ BEGIN RAISE EXCEPTION 'x' USING ERRCODE = '40001'; END $$")
			return err
		}
		return nil
	})
	if err != nil || attempts != 3 {
		t.Fatalf("attempts=%d err=%v", attempts, err)
	}
}

// Ranges go through pgx's RangeCodec via RegisterRangeTypes. Needs only a
// server: the table is a temp table.
func TestRangeAndMultirangeRoundTrip(t *testing.T) {
	dsn := os.Getenv("QLX_TEST_DSN")
	if dsn == "" {
		t.Skip("QLX_TEST_DSN not set")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AfterConnect = pgxexec.AfterConnect
	cfg.MaxConns = 1 // one connection, so the temp table is visible
	p, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	ctx := context.Background()
	if _, err := p.Exec(ctx, `CREATE TEMP TABLE r (id int, a int4range, b tstzrange, m int4multirange)`); err != nil {
		t.Fatal(err)
	}
	lo, hi, lo2, hi2 := int32(3), int32(9), int32(20), int32(30)
	t0 := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	in := qt.Range[int32]{Lower: &lo, Upper: &hi, LowerInc: true}
	tr := qt.Range[time.Time]{Lower: &t0, LowerInc: true} // [t0,)
	// Non-adjacent on purpose: PostgreSQL merges [3,9) and [9,) into one range.
	multi := qt.Multirange[int32]{in, {Lower: &lo2, Upper: &hi2, LowerInc: true}}
	if _, err := p.Exec(ctx, `INSERT INTO r VALUES (1, $1, $2, $3), (2, $4, NULL, NULL), (3, $5, NULL, NULL)`,
		in, tr, multi, qt.Range[int32]{Null: true}, qt.Range[int32]{Empty: true}); err != nil {
		t.Fatal(err)
	}
	var a1, a2, a3 qt.Range[int32]
	var b1 qt.Range[time.Time]
	var m1 qt.Multirange[int32]
	if err := p.QueryRow(ctx, `SELECT a, b, m FROM r WHERE id = 1`).Scan(&a1, &b1, &m1); err != nil {
		t.Fatal(err)
	}
	if a1.Lower == nil || *a1.Lower != 3 || *a1.Upper != 9 || !a1.LowerInc || a1.UpperInc {
		t.Errorf("int4range: %+v", a1)
	}
	if b1.Lower == nil || !b1.Lower.Equal(t0) || b1.Upper != nil {
		t.Errorf("tstzrange: %+v", b1)
	}
	if len(m1) != 2 || m1[0].Lower == nil || *m1[0].Lower != 3 || m1[1].Upper == nil || *m1[1].Upper != 30 {
		t.Errorf("multirange: %+v", m1)
	}
	if err := p.QueryRow(ctx, `SELECT a FROM r WHERE id = 2`).Scan(&a2); err != nil || !a2.Null {
		t.Errorf("NULL range: %+v %v", a2, err)
	}
	if err := p.QueryRow(ctx, `SELECT a FROM r WHERE id = 3`).Scan(&a3); err != nil || !a3.Empty {
		t.Errorf("empty range: %+v %v", a3, err)
	}
}

func TestPipelineTypedFutures(t *testing.T) {
	p := pool(t)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		var pl pgxexec.Pipeline
		ins := pgxexec.PipeExec(&pl, render(t, q.InsertInto(fx.Users).Row(fx.Users.Email.To("pipe@example.com"))))
		cnt := pgxexec.PipeOne[q.Tuple1[int64]](&pl, render(t, q.Select(q.CountAll()).From(fx.Users).Where(fx.Users.Email.Eq("pipe@example.com"))))
		all := pgxexec.PipeAll[q.Tuple1[string]](&pl, render(t, q.Select(fx.Users.Email).From(fx.Users).Where(fx.Users.Email.Eq("pipe@example.com"))))
		if _, err := cnt.Get(); err != pgxexec.ErrNotRun {
			t.Fatalf("Get before Run: %v", err)
		}
		if err := pl.Run(ctx, tx); err != nil {
			t.Fatal(err)
		}
		if tag, err := ins.Get(); err != nil || tag.RowsAffected() != 1 {
			t.Fatalf("insert: %v %v", tag, err)
		}
		if c, err := cnt.Get(); err != nil || c.V1 != 1 {
			t.Fatalf("count: %+v %v", c, err)
		}
		if rows, err := all.Get(); err != nil || len(rows) != 1 || rows[0].V1 != "pipe@example.com" {
			t.Fatalf("rows: %+v %v", rows, err)
		}
		if err := pl.Run(ctx, tx); err == nil {
			t.Fatal("second Run should fail")
		}
	})
}

type planRow struct {
	ID    int64
	Email string
	Nick  qt.Null[string] `db:"nickname"`
}

func TestStructWithoutScanTargetsUsesAPlan(t *testing.T) {
	p := pool(t)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		if _, err := pgxexec.Exec(ctx, tx, render(t, q.InsertInto(fx.Users).Row(fx.Users.Email.To("plan@example.com"), fx.Users.Nickname.To(qt.Some("pl"))))); err != nil {
			t.Fatal(err)
		}
		sel := render(t, q.Select(fx.Users.ID, fx.Users.Email, fx.Users.Nickname).From(fx.Users).Where(fx.Users.Email.Eq("plan@example.com")))
		got, err := pgxexec.One[planRow](ctx, tx, sel)
		if err != nil || got.Email != "plan@example.com" || !got.Nick.Valid || got.Nick.V != "pl" || got.ID == 0 {
			t.Fatalf("got %+v, %v", got, err)
		}
		// A column with no field is a clear error, not a silent zero.
		bad := render(t, q.Select(fx.Users.ID, fx.Users.Email, fx.Users.Tier).From(fx.Users).Where(fx.Users.Email.Eq("plan@example.com")))
		if _, err := pgxexec.One[planRow](ctx, tx, bad); err == nil {
			t.Fatal("want an error for a column without a field")
		}
	})
}

func TestScalarMaybeAndTooManyRows(t *testing.T) {
	p := pool(t)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		for _, e := range []string{"m1@example.com", "m2@example.com"} {
			if _, err := pgxexec.Exec(ctx, tx, render(t, q.InsertInto(fx.Users).Row(fx.Users.Email.To(e)))); err != nil {
				t.Fatal(err)
			}
		}
		one := render(t, q.Select(fx.Users.Email).From(fx.Users).Where(fx.Users.Email.Eq("m1@example.com")))
		if s, err := pgxexec.One[string](ctx, tx, one); err != nil || s != "m1@example.com" {
			t.Fatalf("scalar: %q %v", s, err)
		}
		if o, err := pgxexec.Maybe[string](ctx, tx, one); err != nil || !o.Set || o.V != "m1@example.com" {
			t.Fatalf("maybe: %+v %v", o, err)
		}
		none := render(t, q.Select(fx.Users.Email).From(fx.Users).Where(fx.Users.Email.Eq("nobody@example.com")))
		if o, err := pgxexec.Maybe[string](ctx, tx, none); err != nil || o.Set {
			t.Fatalf("maybe none: %+v %v", o, err)
		}
		if _, err := pgxexec.One[string](ctx, tx, none); !pgxexec.IsNoRows(err) {
			t.Fatalf("one none: %v", err)
		}
		many := render(t, q.Select(fx.Users.Email).From(fx.Users).Where(fx.Users.Email.Like("m_@example.com")))
		if _, err := pgxexec.One[string](ctx, tx, many); !errors.Is(err, pgx.ErrTooManyRows) {
			t.Fatalf("one many: %v", err)
		}
		var seen int
		for e, err := range pgxexec.Each[string](ctx, tx, many) {
			if err != nil || e == "" {
				t.Fatal(err)
			}
			seen++
		}
		if seen != 2 {
			t.Fatalf("each saw %d", seen)
		}
		// time.Time is a struct: a single column scans straight into it.
		ts := render(t, q.Select(fx.Users.CreatedAt).From(fx.Users).Where(fx.Users.Email.Eq("m1@example.com")))
		if tm, err := pgxexec.One[time.Time](ctx, tx, ts); err != nil || tm.IsZero() {
			t.Fatalf("time: %v %v", tm, err)
		}
	})
}

func TestTracerSeesNameFingerprintAndRows(t *testing.T) {
	p := pool(t)
	var got []pgxexec.QueryInfo
	var res []pgxexec.QueryResult
	tr := tracerFunc(func(ctx context.Context, i pgxexec.QueryInfo) (context.Context, func(pgxexec.QueryResult)) {
		got = append(got, i)
		return ctx, func(r pgxexec.QueryResult) { res = append(res, r) }
	})
	db := pgxexec.Traced(p, tr)
	st := render(t, q.Tagged("users.count", q.Select(q.CountAll()).From(fx.Users)))
	if _, err := pgxexec.One[int64](context.Background(), db, st); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "users.count" || got[0].Fingerprint != st.Fingerprint() || got[0].NumArgs != 0 {
		t.Fatalf("info: %+v", got)
	}
	if len(res) != 1 || res[0].Rows != 1 || res[0].Err != nil {
		t.Fatalf("result: %+v", res)
	}
}

type tracerFunc func(context.Context, pgxexec.QueryInfo) (context.Context, func(pgxexec.QueryResult))

func (f tracerFunc) TraceQuery(ctx context.Context, i pgxexec.QueryInfo) (context.Context, func(pgxexec.QueryResult)) {
	return f(ctx, i)
}

func TestStaticHandleRendersOnceAndBinds(t *testing.T) {
	p := pool(t)
	id := q.NewParam[int64]()
	h := pgxexec.NewStatic1[q.Tuple1[string], int64](q.Dialect{Version: q.PG18},
		q.Select(fx.Users.Email).From(fx.Users).Where(fx.Users.ID.EqX(id)), id)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		ins, err := pgxexec.One[int64](ctx, tx, render(t, q.InsertInto(fx.Users).Row(fx.Users.Email.To("static@example.com")).Returning(fx.Users.ID)))
		if err != nil {
			t.Fatal(err)
		}
		got, err := h.One(ctx, tx, ins)
		if err != nil || got.V1 != "static@example.com" {
			t.Fatalf("got %+v %v", got, err)
		}
		if o, err := h.Maybe(ctx, tx, -1); err != nil || o.Set {
			t.Fatalf("maybe: %+v %v", o, err)
		}
	})
}

func TestListenChanDeliversAndStopsWithContext(t *testing.T) {
	p := pool(t)
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := pgxexec.NewChannel("qlx_test_chan")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pgxexec.NewChannel(""); err == nil {
		t.Fatal("empty channel name must be rejected")
	}
	ready := make(chan struct{}, 1)
	notes := pgxexec.ListenChan(ctx, p, pgxexec.ListenOptions{OnReconnect: func(int) { ready <- struct{}{} }}, ch)
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("listener did not subscribe")
	}
	if _, err := p.Exec(ctx, `SELECT pg_notify('qlx_test_chan', 'hello')`); err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-notes:
		if n.Payload != "hello" || n.Channel != "qlx_test_chan" {
			t.Fatalf("got %+v", n)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no notification")
	}
	cancel()
	select {
	case _, ok := <-notes:
		if ok {
			t.Fatal("expected the channel to close")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("channel not closed after cancel")
	}
}

type verifyRow struct {
	ID    int64
	Email string
}

func (r *verifyRow) ScanTargets() []any { return []any{&r.ID, &r.Email} }

type verifyByName struct {
	ID    int64
	Email string
}

func TestVerifyRow(t *testing.T) {
	p := pool(t)
	ctx := context.Background()
	conn, err := p.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	two := render(t, q.Select(fx.Users.ID, fx.Users.Email).From(fx.Users))
	one := render(t, q.Select(fx.Users.ID).From(fx.Users))
	nick := render(t, q.Select(fx.Users.ID, fx.Users.Nickname).From(fx.Users))
	c := conn.Conn()
	ok := func(name string, err error) {
		t.Helper()
		if err != nil {
			t.Errorf("%s: unexpected %v", name, err)
		}
	}
	bad := func(name string, err error, want string) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want an error containing %q", name, err, want)
		}
	}
	ok("positional", pgxexec.VerifyRow[verifyRow](ctx, c, two))
	ok("by name", pgxexec.VerifyRow[verifyByName](ctx, c, two))
	ok("tuple", pgxexec.VerifyRow[q.Tuple2[int64, string]](ctx, c, two))
	ok("scalar", pgxexec.VerifyRow[int64](ctx, c, one))
	ok("nullable column into Null", pgxexec.VerifyRow[q.Tuple2[int64, qt.Null[string]]](ctx, c, nick))
	bad("count", pgxexec.VerifyRow[verifyRow](ctx, c, one), "scans 2 columns")
	bad("tuple type", pgxexec.VerifyRow[q.Tuple2[int64, bool]](ctx, c, two), "cannot be scanned")
	bad("scalar width", pgxexec.VerifyRow[int64](ctx, c, two), "exactly one column")
	bad("missing field", pgxexec.VerifyRow[verifyByName](ctx, c, nick), "no field for column")
}
