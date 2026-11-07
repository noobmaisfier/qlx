package pgxexec_test

// Fault injection: what the executor does when the world misbehaves. Each
// test makes a real fault happen on a real server (a lock that never frees, a
// backend killed with pg_terminate_backend, a genuine serialization conflict,
// a pool with no connection left) and checks two things: the caller gets an
// error that says what happened, not a hang or a panic; and the pool is still
// healthy afterwards. Run it with -race: the last test hammers the shape cache
// from many goroutines.
//
// Needs QLX_TEST_DSN with fixture/schema.sql loaded and a role that may call
// pg_terminate_backend on its own sessions (the same role, so any role).

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	q "github.com/noobmaisfier/qlx"
	fx "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/pgxexec"
)

func faultPool(t *testing.T, params string) *pgxpool.Pool {
	t.Helper()
	dsn := dsnOrSkip(t)
	if params != "" {
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		dsn += sep + params
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AfterConnect = pgxexec.AfterConnect
	p, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

func dsnOrSkip(t *testing.T) string {
	t.Helper()
	p := pool(t) // skips without QLX_TEST_DSN
	return p.Config().ConnString()
}

func selectUsers(t *testing.T) q.Stmt {
	t.Helper()
	return render(t, q.Select(fx.Users.ID, fx.Users.Email).From(fx.Users))
}

type idEmail struct {
	ID    int64
	Email string
}

func (r *idEmail) ScanTargets() []any { return []any{&r.ID, &r.Email} }

func healthy(t *testing.T, p *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := pgxexec.AllScan[idEmail](ctx, p, selectUsers(t)); err != nil {
		t.Fatalf("pool unhealthy after the fault: %v", err)
	}
	// a cancelled connection is closed in the background: allow it a moment
	deadline := time.Now().Add(3 * time.Second)
	for p.Stat().AcquiredConns() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("connections still held after the fault: %d", p.Stat().AcquiredConns())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A statement stuck behind a lock must come back when its context ends, with
// the context's error, and must not leave the connection checked out.
func TestCancelWhileBlockedOnALock(t *testing.T) {
	p := faultPool(t, "pool_max_conns=4")
	ctx := context.Background()
	holder, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback(ctx)
	if _, err := holder.Exec(ctx, "LOCK TABLE users IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		cctx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
		start := time.Now()
		_, err := pgxexec.AllScan[idEmail](cctx, p, selectUsers(t))
		cancel()
		if err == nil {
			t.Fatal("a blocked query returned without error")
		}
		var pe *pgconn.PgError
		if !errors.Is(err, context.DeadlineExceeded) && !(errors.As(err, &pe) && pe.Code == "57014") {
			t.Fatalf("error does not say it was cancelled: %v", err)
		}
		if d := time.Since(start); d > 3*time.Second {
			t.Fatalf("cancellation took %v", d)
		}
	}
	// the same cancel on a write, and on an explicit cancel() rather than a deadline
	wctx, cancel := context.WithCancel(ctx)
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()
	_, err = pgxexec.Exec(wctx, p, render(t, q.Update(fx.Users).Set(fx.Users.Tier.To("gold")).All()))
	if err == nil {
		t.Fatal("blocked UPDATE returned without error")
	}
	holder.Rollback(ctx)
	healthy(t, p)
}

// A backend killed in the middle of work: the caller sees an error, the pool
// discards the dead connection, and the next statements succeed.
func TestBackendTerminatedUnderneath(t *testing.T) {
	p := faultPool(t, "pool_max_conns=2")
	ctx := context.Background()
	admin := pool(t)

	kill := func(pid int32) {
		t.Helper()
		var ok bool
		if err := admin.QueryRow(ctx, "SELECT pg_terminate_backend($1)", pid).Scan(&ok); err != nil || !ok {
			t.Fatalf("terminate: %v %v", ok, err)
		}
	}

	t.Run("idle connection", func(t *testing.T) {
		var pid int32
		if err := p.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
			t.Fatal(err)
		}
		kill(pid)
		time.Sleep(100 * time.Millisecond)
		// at most one statement may see the dead connection; the next must work
		var lastErr error
		for i := 0; i < 3; i++ {
			if _, lastErr = pgxexec.AllScan[idEmail](ctx, p, selectUsers(t)); lastErr == nil {
				break
			}
		}
		if lastErr != nil {
			t.Fatalf("pool did not recover: %v", lastErr)
		}
		healthy(t, p)
	})

	t.Run("inside InTx", func(t *testing.T) {
		calls := 0
		err := pgxexec.InTx(ctx, p, pgx.TxOptions{}, pgxexec.Retry{Attempts: 4, Base: time.Millisecond}, func(ctx context.Context, tx pgx.Tx) error {
			calls++
			var pid int32
			if err := tx.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
				return err
			}
			kill(pid)
			_, err := tx.Exec(ctx, "SELECT 1")
			return err
		})
		if err == nil {
			t.Fatal("InTx reported success after its backend was killed")
		}
		if calls != 1 {
			t.Fatalf("a lost connection was retried %d times; only 40001 and 40P01 are retried", calls)
		}
		healthy(t, p)
	})

	t.Run("mid-query", func(t *testing.T) {
		var pid int32
		conn, err := p.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
			t.Fatal(err)
		}
		go func() { time.Sleep(150 * time.Millisecond); kill(pid) }()
		_, err = pgxexec.Exec(ctx, conn, render(t, q.Select(fx.Users.ID).From(fx.Users)))
		_, err2 := conn.Exec(ctx, "SELECT pg_sleep(5)")
		conn.Release()
		if err2 == nil {
			t.Fatal("pg_sleep survived pg_terminate_backend")
		}
		_ = err
		healthy(t, p)
	})
}

// Two transactions that really conflict at SERIALIZABLE: every increment must
// be applied exactly once, and at least one retry must have happened.
func TestSerializableConflictsAreRetriedToCompletion(t *testing.T) {
	p := faultPool(t, "pool_max_conns=16")
	ctx := context.Background()
	if _, err := p.Exec(ctx, "DROP TABLE IF EXISTS qlx_fault_ctr; CREATE TABLE qlx_fault_ctr (id int PRIMARY KEY, n int NOT NULL); INSERT INTO qlx_fault_ctr VALUES (1, 0), (2, 0)"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Exec(context.Background(), "DROP TABLE IF EXISTS qlx_fault_ctr") })

	const workers, each = 8, 6
	var retries atomic.Int64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < each; i++ {
				err := pgxexec.InTx(ctx, p, pgx.TxOptions{IsoLevel: pgx.Serializable}, pgxexec.Retry{
					Attempts: 50, Base: time.Millisecond,
					OnRetry: func(int, error) { retries.Add(1) },
				}, func(ctx context.Context, tx pgx.Tx) error {
					// write skew: read the other row, then write ours
					var other int
					if err := tx.QueryRow(ctx, "SELECT n FROM qlx_fault_ctr WHERE id = 2").Scan(&other); err != nil {
						return err
					}
					time.Sleep(2 * time.Millisecond)
					_, err := tx.Exec(ctx, "UPDATE qlx_fault_ctr SET n = n + 1 + 0 * $1 WHERE id = 1", other)
					return err
				})
				if err != nil {
					errs <- err
					return
				}
			}
		}()
		wg.Add(1)
		go func() { // the other half of the skew: read row 1, write row 2
			defer wg.Done()
			for i := 0; i < each; i++ {
				err := pgxexec.InTx(ctx, p, pgx.TxOptions{IsoLevel: pgx.Serializable}, pgxexec.Retry{
					Attempts: 50, Base: time.Millisecond,
					OnRetry: func(int, error) { retries.Add(1) },
				}, func(ctx context.Context, tx pgx.Tx) error {
					var other int
					if err := tx.QueryRow(ctx, "SELECT n FROM qlx_fault_ctr WHERE id = 1").Scan(&other); err != nil {
						return err
					}
					time.Sleep(2 * time.Millisecond)
					_, err := tx.Exec(ctx, "UPDATE qlx_fault_ctr SET n = n + 1 + 0 * $1 WHERE id = 2", other)
					return err
				})
				if err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("worker failed: %v", err)
	}
	var a, b int
	if err := p.QueryRow(ctx, "SELECT (SELECT n FROM qlx_fault_ctr WHERE id=1), (SELECT n FROM qlx_fault_ctr WHERE id=2)").Scan(&a, &b); err != nil {
		t.Fatal(err)
	}
	if a != workers*each || b != workers*each {
		t.Errorf("lost or doubled updates: %d and %d, want %d each", a, b, workers*each)
	}
	if retries.Load() == 0 {
		t.Log("no serialization conflict happened on this run; the retry path was not exercised")
	} else {
		t.Logf("%d retries", retries.Load())
	}
	healthy(t, p)
}

func TestRetryPolicy(t *testing.T) {
	p := faultPool(t, "")
	ctx := context.Background()
	fail := func(code string) func(context.Context, pgx.Tx) error {
		return func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "DO $$ BEGIN RAISE EXCEPTION 'x' USING ERRCODE = '"+code+"'; END $$")
			return err
		}
	}
	t.Run("exhausted", func(t *testing.T) {
		calls, onRetry := 0, 0
		f := fail("40P01")
		err := pgxexec.InTx(ctx, p, pgx.TxOptions{}, pgxexec.Retry{Attempts: 4, Base: time.Millisecond, OnRetry: func(int, error) { onRetry++ }},
			func(ctx context.Context, tx pgx.Tx) error { calls++; return f(ctx, tx) })
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || pe.Code != "40P01" || calls != 4 || onRetry != 3 {
			t.Fatalf("calls=%d onRetry=%d err=%v", calls, onRetry, err)
		}
		if !pgxexec.IsSerialization(err) {
			t.Fatal("IsSerialization false for a deadlock")
		}
	})
	t.Run("not retried", func(t *testing.T) {
		for _, code := range []string{"23505", "42P01", "22012", "57014", "XX000"} {
			calls := 0
			f := fail(code)
			err := pgxexec.InTx(ctx, p, pgx.TxOptions{}, pgxexec.Retry{Attempts: 5, Base: time.Millisecond},
				func(ctx context.Context, tx pgx.Tx) error { calls++; return f(ctx, tx) })
			if err == nil || calls != 1 {
				t.Errorf("code %s: calls=%d err=%v", code, calls, err)
			}
		}
	})
	t.Run("backoff is capped and bad values fall back", func(t *testing.T) {
		// Attempts large enough for base<<i to overflow; Max keeps each wait short.
		calls := 0
		f := fail("40001")
		start := time.Now()
		err := pgxexec.InTx(ctx, p, pgx.TxOptions{}, pgxexec.Retry{Attempts: 70, Base: time.Millisecond, Max: 2 * time.Millisecond},
			func(ctx context.Context, tx pgx.Tx) error { calls++; return f(ctx, tx) })
		if err == nil || calls != 70 || time.Since(start) > 20*time.Second {
			t.Fatalf("calls=%d err=%v after %v", calls, err, time.Since(start))
		}
		// A negative Attempts used to skip fn and return nil.
		calls = 0
		_ = pgxexec.InTx(ctx, p, pgx.TxOptions{}, pgxexec.Retry{Attempts: -1, Base: -1},
			func(ctx context.Context, tx pgx.Tx) error { calls++; return nil })
		if calls != 1 {
			t.Fatalf("negative Attempts: fn ran %d times", calls)
		}
	})
	t.Run("context ends during backoff", func(t *testing.T) {
		cctx, cancel := context.WithTimeout(ctx, 80*time.Millisecond)
		defer cancel()
		start := time.Now()
		f := fail("40001")
		err := pgxexec.InTx(cctx, p, pgx.TxOptions{}, pgxexec.Retry{Attempts: 20, Base: 10 * time.Second},
			func(ctx context.Context, tx pgx.Tx) error { return f(ctx, tx) })
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 3*time.Second {
			t.Fatalf("err=%v after %v", err, time.Since(start))
		}
	})
	t.Run("panic in fn rolls back", func(t *testing.T) {
		func() {
			defer func() { _ = recover() }()
			_ = pgxexec.InTx(ctx, p, pgx.TxOptions{}, pgxexec.Retry{}, func(ctx context.Context, tx pgx.Tx) error {
				tx.Exec(ctx, "UPDATE users SET tier = 'panic' WHERE false")
				panic("boom")
			})
		}()
		// the panicking transaction must not leave its connection checked out and open
		time.Sleep(50 * time.Millisecond)
		if n := p.Stat().AcquiredConns(); n != 0 {
			t.Errorf("a panic in the transaction function leaked %d connection(s)", n)
		}
	})
}

// The listener's connection is killed: it must subscribe again and keep
// delivering, and tell the application through OnReconnect so it can resync.
func TestListenResubscribesAfterTheBackendDies(t *testing.T) {
	p := faultPool(t, "pool_max_conns=4")
	admin := pool(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := pgxexec.NewChannel("qlx_fault_chan")
	if err != nil {
		t.Fatal(err)
	}
	var reconnects atomic.Int32
	attempts := make(chan int, 16)
	var onErr atomic.Int32
	notes := pgxexec.ListenChan(ctx, p, pgxexec.ListenOptions{
		BackoffBase: 20 * time.Millisecond, BackoffMax: 200 * time.Millisecond,
		OnReconnect: func(a int) { reconnects.Add(1); attempts <- a },
		OnError:     func(error) { onErr.Add(1) },
	}, ch)
	wait := func(what string) int {
		t.Helper()
		select {
		case a := <-attempts:
			return a
		case <-time.After(10 * time.Second):
			t.Fatalf("no OnReconnect: %s", what)
			return -1
		}
	}
	if a := wait("first subscription"); a != 0 {
		t.Fatalf("first attempt number %d, want 0", a)
	}
	for round := 1; round <= 3; round++ {
		var killed int
		if err := admin.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity a, LATERAL pg_terminate_backend(a.pid) t
			WHERE a.query ILIKE 'LISTEN%qlx_fault_chan%' AND a.pid <> pg_backend_pid() AND t`).Scan(&killed); err != nil || killed == 0 {
			t.Fatalf("round %d: killed %d listeners: %v", round, killed, err)
		}
		if a := wait(fmt.Sprintf("round %d", round)); a < 1 {
			t.Fatalf("round %d: reconnect attempt %d", round, a)
		}
		if _, err := admin.Exec(ctx, "SELECT pg_notify('qlx_fault_chan', $1)", fmt.Sprint("r", round)); err != nil {
			t.Fatal(err)
		}
		select {
		case n := <-notes:
			if n.Payload != fmt.Sprint("r", round) {
				t.Fatalf("round %d: got %+v", round, n)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("round %d: notification not delivered after reconnect", round)
		}
	}
	if onErr.Load() == 0 {
		t.Error("OnError never reported the lost connection")
	}
	cancel()
	select {
	case _, ok := <-notes:
		if ok {
			t.Error("unexpected notification after cancel")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("channel not closed")
	}
	// the listener must not leak a subscription into the pool
	time.Sleep(100 * time.Millisecond)
	var listening int
	admin.QueryRow(context.Background(), "SELECT count(*) FROM pg_listening_channels() WHERE pg_listening_channels = 'qlx_fault_chan'").Scan(&listening)
	healthy(t, p)
}

// No connection left: callers wait for the pool, give up with their own
// context error, and everything works again once a connection is returned.
func TestPoolExhaustion(t *testing.T) {
	p := faultPool(t, "pool_max_conns=2")
	ctx := context.Background()
	a, err := p.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
			defer cancel()
			_, err := pgxexec.AllScan[idEmail](cctx, p, selectUsers(t))
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("waiting for a connection: %v", err)
			}
		}()
	}
	wg.Wait()
	cctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	err = pgxexec.InTx(cctx, p, pgx.TxOptions{}, pgxexec.Retry{}, func(context.Context, pgx.Tx) error { return nil })
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("InTx with no connection: %v", err)
	}
	a.Release()
	b.Release()
	healthy(t, p)
}

// Many goroutines resolve many shapes through one small cache while it evicts.
// Run with -race. Every result must be the SQL its own statement renders to.
func TestShapeCacheUnderConcurrentEviction(t *testing.T) {
	cache := q.NewShapeCache(32)
	d := q.Dialect{Version: q.PG18}
	var wg sync.WaitGroup
	var calls atomic.Int64
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 400; i++ {
				mask := (g*7+i)%63 + 1 // 63 different column subsets: 63 shapes through a cache of ~32
				var cols []q.Selectable
				for k, c := range []q.Selectable{fx.Users.ID, fx.Users.Email, fx.Users.Nickname, fx.Users.Tier, fx.Users.Balance, fx.Users.CreatedAt} {
					if mask>>k&1 == 1 {
						cols = append(cols, c)
					}
				}
				st := q.Select(cols...).From(fx.Users).Where(fx.Users.Balance.Gt(int64(i)))
				sql, args, err := cache.Resolve(d, st)
				calls.Add(1)
				if err != nil {
					t.Errorf("resolve: %v", err)
					return
				}
				// the SQL is the one this statement renders to, whoever cached it
				want, _, err := q.SQL(d, st)
				if err != nil || sql != want {
					t.Errorf("mask %d: cached SQL differs from a fresh render:\n%s\n%s", mask, sql, want)
					return
				}
				if len(args) != 1 || args[0] != int64(i) {
					t.Errorf("mask %d: arguments %v, want [%d]", mask, args, i)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	s := cache.Stats()
	if s.Hits+s.Misses != calls.Load() {
		t.Errorf("hits %d + misses %d != %d calls", s.Hits, s.Misses, calls.Load())
	}
	if s.Size > 32+16 {
		t.Errorf("cache grew to %d entries, limit about 32", s.Size)
	}
	if s.Evictions == 0 {
		t.Error("the test never evicted; shrink the cache")
	}
}
