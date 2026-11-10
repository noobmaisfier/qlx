package pgxexec

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/noobmaisfier/qlx"
)

// ---------------------------------------------------------------------------
// Tracing and metrics
// ---------------------------------------------------------------------------

// QueryInfo describes a statement about to run. It carries the shape and the
// number of arguments, never the argument values.
type QueryInfo struct {
	Name        string // from qlx.Tagged, or ""
	Fingerprint string // shape hash, stable across calls of one shape
	SQL         string // rendered text with $n placeholders
	NumArgs     int
}

// QueryResult is what a Tracer learns when the statement finishes. For Query
// the duration runs until the rows are closed, so it covers reading them.
type QueryResult struct {
	Duration time.Duration
	Rows     int64 // rows read (Query) or affected (Exec)
	Err      error
}

// Tracer observes statements. TraceQuery runs before the statement and may
// return a derived context (for a span); the function it returns runs after.
// Core stays on pgx only: an OpenTelemetry adapter is the separate otelqlx
// module.
type Tracer interface {
	TraceQuery(ctx context.Context, info QueryInfo) (context.Context, func(QueryResult))
}

// Traced wraps a Querier so every Query and Exec goes through t.
func Traced(db Querier, t Tracer) Querier {
	d := traced{db, t}
	if s, ok := db.(TxStarter); ok {
		return tracedStarter{d, s}
	}
	return d
}

// tracedStarter keeps BeginTx visible through the wrapper, so code that looks
// for a TxStarter (InsertMany, InTx) still gets its transaction, and the
// statements inside it are traced too.
type tracedStarter struct {
	traced
	s TxStarter
}

func (d tracedStarter) BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
	tx, err := d.s.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return tracedTx{tx, traced{tx, d.t}}, nil
}

type tracedTx struct {
	pgx.Tx
	q traced
}

func (x tracedTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return x.q.Query(ctx, sql, args...)
}

func (x tracedTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return x.q.Exec(ctx, sql, args...)
}

type traced struct {
	Querier
	t Tracer
}

func info(sql string, nargs int) QueryInfo {
	return QueryInfo{Name: qlx.QueryName(sql), Fingerprint: qlx.Stmt{SQL: sql}.Fingerprint(), SQL: sql, NumArgs: nargs}
}

func (d traced) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	ctx, done := d.t.TraceQuery(ctx, info(sql, len(args)))
	start := time.Now()
	rows, err := d.Querier.Query(ctx, sql, args...)
	if err != nil {
		done(QueryResult{Duration: time.Since(start), Err: err})
		return nil, err
	}
	return &tracedRows{Rows: rows, done: done, start: start}, nil
}

func (d traced) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	ctx, done := d.t.TraceQuery(ctx, info(sql, len(args)))
	start := time.Now()
	tag, err := d.Querier.Exec(ctx, sql, args...)
	done(QueryResult{Duration: time.Since(start), Rows: tag.RowsAffected(), Err: err})
	return tag, err
}

type tracedRows struct {
	pgx.Rows
	done  func(QueryResult)
	start time.Time
	n     int64
	fin   bool
}

func (r *tracedRows) Next() bool {
	ok := r.Rows.Next()
	if ok {
		r.n++
	}
	return ok
}

func (r *tracedRows) Close() {
	r.Rows.Close()
	if !r.fin {
		r.fin = true
		r.done(QueryResult{Duration: time.Since(r.start), Rows: r.n, Err: r.Rows.Err()})
	}
}

// Stats are process-wide counters, cheap enough to leave on. The statement
// cache keeps its own (qlx.Cache.Stats).
var Stats counters

type counters struct {
	queries, errors, rows, retries atomic.Int64
	nanos                          atomic.Int64
}

// StatsSnapshot is a point-in-time copy of the counters.
type StatsSnapshot struct {
	Queries, Errors, RowsScanned, Retries int64
	TotalDuration                         time.Duration
}

// Snapshot copies the counters.
func (c *counters) Snapshot() StatsSnapshot {
	return StatsSnapshot{c.queries.Load(), c.errors.Load(), c.rows.Load(), c.retries.Load(), time.Duration(c.nanos.Load())}
}

// Reset zeroes the counters (for tests).
func (c *counters) Reset() {
	c.queries.Store(0)
	c.errors.Store(0)
	c.rows.Store(0)
	c.retries.Store(0)
	c.nanos.Store(0)
}

// CountRetry is a Retry.OnRetry that feeds Stats.
func CountRetry(int, error) { Stats.retries.Add(1) }

// StatsTracer is a Tracer that feeds Stats; combine it with another through
// Tracers.
type StatsTracer struct{}

// TraceQuery implements Tracer.
func (StatsTracer) TraceQuery(ctx context.Context, _ QueryInfo) (context.Context, func(QueryResult)) {
	return ctx, func(r QueryResult) {
		Stats.queries.Add(1)
		Stats.nanos.Add(int64(r.Duration))
		if r.Err != nil {
			Stats.errors.Add(1)
		}
	}
}

// Tracers fans one statement out to several tracers, in order.
type Tracers []Tracer

// TraceQuery implements Tracer.
func (ts Tracers) TraceQuery(ctx context.Context, i QueryInfo) (context.Context, func(QueryResult)) {
	dones := make([]func(QueryResult), 0, len(ts))
	for _, t := range ts {
		var d func(QueryResult)
		ctx, d = t.TraceQuery(ctx, i)
		dones = append(dones, d)
	}
	return ctx, func(r QueryResult) {
		for i := len(dones) - 1; i >= 0; i-- {
			dones[i](r)
		}
	}
}
