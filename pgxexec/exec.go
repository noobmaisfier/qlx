// Package pgxexec runs QLX statements on pgx v5.
//
// Entry points:
//
//   - Connect returns a Client: a pool plus the server's Dialect, with an
//     optional Tracer and a version check against the generated Targets.
//     Go 1.27 generic methods give client.All[User](ctx, st) and friends.
//   - All, One, Maybe, Each scan rows into structs, scalars or tuples; the
//     scan plan is cached per (type, column list) and uses no per-row
//     reflection. Exec runs statements without rows.
//   - NewStatic0..4 are typed prepared handles that render once.
//   - Pipeline and Batch send several statements in one round trip; InTx
//     retries serialization failures; CopyFrom uses the binary protocol.
//   - ListenChan is a LISTEN loop that reconnects with backoff.
//   - Traced, Stats and StatsTracer observe every statement by its Tagged
//     name or fingerprint, never by SQL text or argument values.
//
// The package is verified by the integration tests in this directory against
// PostgreSQL 18 and 19 (scripts/with_pg.sh).
package pgxexec

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/noobmaisfier/qlx"
)

// Querier is satisfied by *pgxpool.Pool, pgx.Tx and *pgx.Conn. It is not
// generic, so any of them can be passed without adapters.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// Exec runs a statement that returns no rows.
func Exec(ctx context.Context, db Querier, st qlx.Stmt, binds ...qlx.Binding) (pgconn.CommandTag, error) {
	args, err := st.Args(binds...)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	return db.Exec(ctx, st.SQL, args...)
}

// TxStarter is satisfied by *pgxpool.Pool and *pgx.Conn.
type TxStarter interface {
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
}

// Retry controls InTx retries on serialization failure (40001) and
// deadlock (40P01).
type Retry struct {
	Attempts int           // total tries; 0 means 5
	Base     time.Duration // first backoff; 0 means 10ms
	// OnRetry, when set, is called before each retry with the attempt that
	// failed (1-based) and its error; use it to count retries as a metric.
	OnRetry func(attempt int, err error)
}

// InTx runs fn in a transaction, retrying the whole function when the server
// reports a serialization failure or deadlock. fn must be safe to run again.
func InTx(ctx context.Context, db TxStarter, opts pgx.TxOptions, r Retry, fn func(ctx context.Context, tx pgx.Tx) error) error {
	attempts, base := r.Attempts, r.Base
	if attempts == 0 {
		attempts = 5
	}
	if base == 0 {
		base = 10 * time.Millisecond
	}
	var err error
	for i := 0; i < attempts; i++ {
		err = runTx(ctx, db, opts, fn)
		if err == nil || !retryable(err) {
			return err
		}
		if r.OnRetry != nil && i < attempts-1 {
			r.OnRetry(i+1, err)
		}
		sleep := base << i
		sleep += time.Duration(rand.Int64N(int64(sleep)/2 + 1))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleep):
		}
	}
	return err
}

func runTx(ctx context.Context, db TxStarter, opts pgx.TxOptions, fn func(ctx context.Context, tx pgx.Tx) error) error {
	tx, err := db.BeginTx(ctx, opts)
	if err != nil {
		return err
	}
	// Whatever way fn leaves (error, panic, runtime.Goexit), the transaction
	// ends and its connection goes back to the pool. The rollback does not
	// depend on ctx: a cancelled context must not leave the transaction open.
	finished := false
	defer func() {
		if !finished {
			rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = tx.Rollback(rctx)
		}
	}()
	if err := fn(ctx, tx); err != nil {
		return err
	}
	finished = true
	return tx.Commit(ctx)
}

func retryable(err error) bool {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code == "40001" || pg.Code == "40P01"
	}
	return false
}

// IsUnique reports a unique-violation (23505) on the named constraint.
func IsUnique(err error, constraint string) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505" && pg.ConstraintName == constraint
}

// ---------------------------------------------------------------------------
// Error classification
// ---------------------------------------------------------------------------

func pgErr(err error) *pgconn.PgError {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg
	}
	return nil
}

// IsForeignKey reports a foreign-key violation (23503), optionally on one constraint.
func IsForeignKey(err error, constraint string) bool {
	pg := pgErr(err)
	return pg != nil && pg.Code == "23503" && (constraint == "" || pg.ConstraintName == constraint)
}

// IsNotNull reports a not-null violation (23502) on a column.
func IsNotNull(err error, column string) bool {
	pg := pgErr(err)
	return pg != nil && pg.Code == "23502" && (column == "" || pg.ColumnName == column)
}

// IsCheck reports a check-constraint violation (23514).
func IsCheck(err error, constraint string) bool {
	pg := pgErr(err)
	return pg != nil && pg.Code == "23514" && (constraint == "" || pg.ConstraintName == constraint)
}

// IsSerialization reports a serialization failure (40001) or deadlock (40P01).
func IsSerialization(err error) bool { return retryable(err) }

// IsNoRows reports pgx.ErrNoRows, which One returns for an empty result.
func IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// ---------------------------------------------------------------------------
// Batch: several statements in one round trip
// ---------------------------------------------------------------------------

// Batch queues rendered statements and sends them together. Results come back
// in queue order through the typed readers below.
type Batch struct {
	b    pgx.Batch
	errs []error
}

// Queue adds a statement. A binding error is kept and returned by Send, so
// building a batch never needs error handling at every step.
func (b *Batch) Queue(st qlx.Stmt, binds ...qlx.Binding) *Batch {
	args, err := st.Args(binds...)
	if err != nil {
		b.errs = append(b.errs, err)
		return b
	}
	b.b.Queue(st.SQL, args...)
	return b
}

// Len is the number of queued statements.
func (b *Batch) Len() int { return b.b.Len() }

// BatchSender is satisfied by *pgxpool.Pool, pgx.Tx and *pgx.Conn.
type BatchSender interface {
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// Send sends the batch. The caller must Close the results; Close reports the
// first error not already returned by a reader.
func (b *Batch) Send(ctx context.Context, db BatchSender) (pgx.BatchResults, error) {
	if len(b.errs) > 0 {
		return nil, errors.Join(b.errs...)
	}
	return db.SendBatch(ctx, &b.b), nil
}

// BatchAllScan reads the next result of a batch as a list of T, scanning each
// row through ScanTargets (generated row types and qlx.Tuple1..8). Columns
// are matched by position, so a projection such as count(*) works with a
// tuple; BatchAll matches by column name and suits structs whose fields are
// named like the columns.
func BatchAllScan[T any, PT Scannable[T]](res pgx.BatchResults) ([]T, error) {
	rows, err := res.Query()
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (T, error) {
		var t T
		err := row.Scan(PT(&t).ScanTargets()...)
		return t, err
	})
}

// BatchOneScan reads the next result of a batch as exactly one T, by position.
func BatchOneScan[T any, PT Scannable[T]](res pgx.BatchResults) (T, error) {
	var zero T
	rows, err := res.Query()
	if err != nil {
		return zero, err
	}
	return pgx.CollectExactlyOneRow(rows, func(row pgx.CollectableRow) (T, error) {
		var t T
		err := row.Scan(PT(&t).ScanTargets()...)
		return t, err
	})
}

// BatchAll reads the next result of a batch as a list of R, matching struct
// fields to columns by name (pgx.RowToStructByName). For tuples and other
// positional projections use BatchAllScan.
func BatchAll[R any](res pgx.BatchResults) ([]R, error) {
	rows, err := res.Query()
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[R])
}

// BatchOne reads the next result of a batch as exactly one R, by column name.
// For tuples and other positional projections use BatchOneScan.
func BatchOne[R any](res pgx.BatchResults) (R, error) {
	var zero R
	rows, err := res.Query()
	if err != nil {
		return zero, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[R])
}

// ---------------------------------------------------------------------------
// COPY FROM through the binary protocol
// ---------------------------------------------------------------------------

// CopyFromer is satisfied by *pgx.Conn and pgx.Tx.
type CopyFromer interface {
	CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)
}

// CopyIn copies rows into the table named by a CopyIn statement using pgx's
// binary COPY, the fastest bulk path. Options in the statement (format,
// header, ...) do not apply: the binary protocol has its own framing. rows
// yields one []any per row in the order of the statement's columns.
func CopyIn(ctx context.Context, db CopyFromer, st qlx.CopyFromStdin, rows pgx.CopyFromSource) (int64, error) {
	schema, table, cols, err := st.Target()
	if err != nil {
		return 0, err
	}
	if len(cols) == 0 {
		return 0, errors.New("pgxexec.CopyIn: name the columns in qlx.CopyIn(table, cols...)")
	}
	ident := pgx.Identifier{table}
	if schema != "" {
		ident = pgx.Identifier{schema, table}
	}
	return db.CopyFrom(ctx, ident, cols, rows)
}

// CopyRows adapts a slice of rows to pgx.CopyFromSource.
func CopyRows(rows [][]any) pgx.CopyFromSource { return pgx.CopyFromRows(rows) }

// ---------------------------------------------------------------------------
// LISTEN / NOTIFY
// ---------------------------------------------------------------------------

// Notification is one NOTIFY received.
type Notification struct {
	Channel string
	Payload string
	PID     uint32
}

// Listen subscribes to channels on a dedicated connection and calls handle
// for each notification until ctx ends or handle returns an error. The
// connection must not be shared: use pool.Acquire and pass conn.Conn().
// Channel names are quoted by the renderer, not concatenated here.
func Listen(ctx context.Context, conn *pgx.Conn, handle func(Notification) error, channels ...string) error {
	for _, ch := range channels {
		sql, _, err := qlx.SQL(qlx.Dialect{Version: qlx.PG18}, qlx.Listen(ch))
		if err != nil {
			return err
		}
		if _, err := conn.Exec(ctx, sql); err != nil {
			return err
		}
	}
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if err := handle(Notification{n.Channel, n.Payload, n.PID}); err != nil {
			return err
		}
	}
}

// ---------------------------------------------------------------------------
// Schema verification and qtest
// ---------------------------------------------------------------------------

// VerifySchema compares the live database with the fingerprints embedded in
// generated code. Name tables to check only those the program uses.
//
//	err := pgxexec.VerifySchema(ctx, pool, db.Spec, "public.users", "public.orders")
func VerifySchema(ctx context.Context, db Querier, spec qlx.SchemaSpec, tables ...string) error {
	rows, err := db.Query(ctx, spec.FingerprintSQL, spec.Schemas)
	if err != nil {
		return err
	}
	live := map[string]string{}
	for rows.Next() {
		var tbl, fp string
		if err := rows.Scan(&tbl, &fp); err != nil {
			rows.Close()
			return err
		}
		live[tbl] = fp
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return spec.Check(live, tables...)
}

// Preparer adapts a connection to qtest.VerifyPrepare: it prepares each
// statement on the server under one reused name (pgx replaces it each time).
// Pass a *pgx.Conn or pgx.Tx.
func Preparer(db interface {
	Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error)
}) func(ctx context.Context, name, sql string) error {
	return func(ctx context.Context, name, sql string) error {
		_, err := db.Prepare(ctx, "qlx_check", sql)
		return err
	}
}

// ---------------------------------------------------------------------------
// Reflection-free scanning
// ---------------------------------------------------------------------------

// Scannable is implemented (on the pointer) by generated row types and by
// qlx.Tuple1..Tuple8: ScanTargets lists a pointer for each column, in the
// order the query selects them. Scanning through it uses no reflection, so a
// row costs one pgx Scan call and no per-row type lookups.
type Scannable[T any] interface {
	*T
	ScanTargets() []any
}

// ---------------------------------------------------------------------------
// Observability
// ---------------------------------------------------------------------------

// Observer is called after every statement run through an Observed Querier.
// sql is the statement text with $n placeholders: arguments are never passed,
// so nothing sensitive reaches logs or traces by accident.
type Observer func(ctx context.Context, sql string, took time.Duration, err error)

type observed struct {
	Querier
	obs Observer
}

// Observed wraps a Querier so each Query and Exec reports its text, duration
// and error to obs. For Query the duration covers sending the statement and
// receiving the first response, not reading the rows.
func Observed(db Querier, obs Observer) Querier { return observed{db, obs} }

func (o observed) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	start := time.Now()
	rows, err := o.Querier.Query(ctx, sql, args...)
	o.obs(ctx, sql, time.Since(start), err)
	return rows, err
}

func (o observed) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	start := time.Now()
	tag, err := o.Querier.Exec(ctx, sql, args...)
	o.obs(ctx, sql, time.Since(start), err)
	return tag, err
}

// ---------------------------------------------------------------------------
// Server version, pool modes, savepoints
// ---------------------------------------------------------------------------

// DetectDialect asks the server for its version and returns the matching
// Dialect, so constructs newer than the server fail at render time with
// ErrUnsupportedByServer instead of at run time with a syntax error. Call it
// once at startup and reuse the result.
func DetectDialect(ctx context.Context, db Querier) (qlx.Dialect, error) {
	rows, err := db.Query(ctx, "SHOW server_version_num")
	if err != nil {
		return qlx.Dialect{}, err
	}
	defer rows.Close()
	var s string
	if rows.Next() {
		if err := rows.Scan(&s); err != nil {
			return qlx.Dialect{}, err
		}
	}
	if err := rows.Err(); err != nil {
		return qlx.Dialect{}, err
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return qlx.Dialect{}, fmt.Errorf("pgxexec: unexpected server_version_num %q", s)
	}
	return qlx.Dialect{Version: n / 10000}, nil
}

// PoolMode chooses how statements reach the server.
type PoolMode int

const (
	// CacheStatements prepares each statement once per connection (pgx's default).
	CacheStatements PoolMode = iota
	// CacheDescribe uses unnamed statements with a cached description: safe
	// behind PgBouncer in transaction mode.
	CacheDescribe
	// Simple uses the simple protocol: parameters are interpolated client-side
	// by pgx. Needed only by poolers that support nothing else.
	Simple
)

// Configure sets the query execution mode on a connection config.
func (m PoolMode) Configure(cfg *pgx.ConnConfig) {
	switch m {
	case CacheDescribe:
		cfg.DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe
	case Simple:
		cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	default:
		cfg.DefaultQueryExecMode = pgx.QueryExecModeCacheStatement
	}
}

// SavepointRunner is satisfied by pgx.Tx.
type SavepointRunner interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// WithSavepoint runs fn inside a savepoint: if fn fails the work since the
// savepoint is rolled back and the error returned, and the enclosing
// transaction stays usable. The name is validated and quoted by the renderer.
func WithSavepoint(ctx context.Context, tx SavepointRunner, name string, fn func(ctx context.Context) error) error {
	run := func(c qlx.Command) error {
		sql, _, err := qlx.SQL(qlx.Dialect{Version: qlx.PG18}, c)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, sql)
		return err
	}
	if err := run(qlx.Savepoint(name)); err != nil {
		return err
	}
	if err := fn(ctx); err != nil {
		if rbErr := run(qlx.RollbackToSavepoint(name)); rbErr != nil {
			return errors.Join(err, rbErr)
		}
		return err
	}
	return run(qlx.ReleaseSavepoint(name))
}
