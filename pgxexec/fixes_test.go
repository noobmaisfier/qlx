package pgxexec_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	q "github.com/noobmaisfier/qlx"
	fx "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/pgxexec"
)

type rowAssign []q.Assignment

func (r rowAssign) Assignments() []q.Assignment { return r }

type countTracer struct{ n int }

func (c *countTracer) TraceQuery(ctx context.Context, _ pgxexec.QueryInfo) (context.Context, func(pgxexec.QueryResult)) {
	c.n++
	return ctx, func(pgxexec.QueryResult) {}
}

// InsertMany promises one transaction. Wrapping the pool for tracing or
// observation must not turn that into one statement per chunk.
func TestInsertManyStaysAtomicThroughWrappers(t *testing.T) {
	p := pool(t)
	ctx := context.Background()
	clean := func() { _, _ = p.Exec(ctx, "DELETE FROM users WHERE email LIKE 'zzatomic-%'") }
	clean()
	defer clean()
	rows := []q.RowAssigner{
		rowAssign{fx.Users.Email.To("zzatomic-1")},
		rowAssign{fx.Users.Email.To("zzatomic-2")},
		rowAssign{fx.Users.Email.To("zzatomic-1")}, // duplicate: the last chunk fails
	}
	tr := &countTracer{}
	var observed int
	wrapped := map[string]pgxexec.Querier{
		"pool":     p,
		"traced":   pgxexec.Traced(p, tr),
		"observed": pgxexec.Observed(p, func(context.Context, string, time.Duration, error) { observed++ }),
	}
	for name, db := range wrapped {
		clean()
		_, err := pgxexec.InsertMany(ctx, db, q.Dialect{Version: q.PG18}, q.InsertInto(fx.Users), rows, pgxexec.BulkOptions{MaxParams: 1})
		if err == nil {
			t.Fatalf("%s: expected the duplicate to fail", name)
		}
		var n int
		if err := p.QueryRow(ctx, "SELECT count(*) FROM users WHERE email LIKE 'zzatomic-%'").Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%s: %d rows left behind by a failed InsertMany", name, n)
		}
	}
	if tr.n == 0 || observed == 0 {
		t.Errorf("statements inside the transaction were not traced (%d) or observed (%d)", tr.n, observed)
	}
}

func TestPipelineFuturesAfterFailure(t *testing.T) {
	p := pool(t)
	ctx := context.Background()

	// A server error: the later futures hold the batch error, not ErrNotRun.
	var pl pgxexec.Pipeline
	a := pgxexec.PipeOne[q.Tuple1[int32]](&pl, q.Stmt{SQL: "SELECT 1"})
	b := pgxexec.PipeOne[q.Tuple1[int32]](&pl, q.Stmt{SQL: "SELECT 1/0"})
	c := pgxexec.PipeOne[q.Tuple1[int32]](&pl, q.Stmt{SQL: "SELECT 3"})
	if err := pl.Run(ctx, p); err == nil {
		t.Fatal("expected the division error")
	}
	if _, err := a.Get(); err != nil {
		t.Errorf("a: %v", err)
	}
	if _, err := b.Get(); err == nil {
		t.Error("b should hold its error")
	}
	if _, err := c.Get(); err == nil || errors.Is(err, pgxexec.ErrNotRun) {
		t.Errorf("c: %v, want the batch error, not ErrNotRun", err)
	}

	// A client-side error (two rows for PipeOne): the server ran the rest and
	// their results are good.
	var p2 pgxexec.Pipeline
	x := pgxexec.PipeOne[q.Tuple1[int32]](&p2, q.Stmt{SQL: "SELECT 1 UNION ALL SELECT 2"})
	y := pgxexec.PipeOne[q.Tuple1[int32]](&p2, q.Stmt{SQL: "SELECT 7"})
	if err := p2.Run(ctx, p); err == nil {
		t.Fatal("expected too many rows")
	}
	if _, err := x.Get(); err == nil {
		t.Error("x should hold too many rows")
	}
	if v, err := y.Get(); err != nil || v.V1 != 7 {
		t.Errorf("y = %+v %v, want the real result 7", v, err)
	}
}

type fakeTx struct{ begins int }

func (f *fakeTx) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	f.begins++
	return nil, &pgconn.PgError{Code: "40001"}
}

// Out of attempts, InTx returns at once instead of sleeping first.
func TestInTxDoesNotSleepAfterLastAttempt(t *testing.T) {
	f := &fakeTx{}
	start := time.Now()
	err := pgxexec.InTx(context.Background(), f, pgx.TxOptions{}, pgxexec.Retry{Attempts: 2, Base: 300 * time.Millisecond, Max: 300 * time.Millisecond},
		func(context.Context, pgx.Tx) error { return nil })
	if !pgxexec.IsSerialization(err) {
		t.Fatalf("err = %v", err)
	}
	// one sleep (300-450ms) between the two attempts, none after the second
	if d := time.Since(start); d > 700*time.Millisecond {
		t.Errorf("took %v: slept after the last attempt", d)
	}
	if f.begins != 2 {
		t.Errorf("attempts = %d", f.begins)
	}
}

// When the context ends during the backoff, the error that caused the retry is
// not lost.
func TestInTxKeepsServerErrorWhenContextEnds(t *testing.T) {
	f := &fakeTx{}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := pgxexec.InTx(ctx, f, pgx.TxOptions{}, pgxexec.Retry{Attempts: 5, Base: time.Second},
		func(context.Context, pgx.Tx) error { return nil })
	if !errors.Is(err, context.DeadlineExceeded) || !pgxexec.IsSerialization(err) {
		t.Fatalf("err = %v: want both the deadline and the serialization failure", err)
	}
}
