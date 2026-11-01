package pgxexec

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jackc/pgx/v5"
	"github.com/noobmaisfier/qlx"
)

// Pipeline sends several statements in one round trip and hands each result
// back through a typed Future, so the call site reads like sequential code
// without the positional bookkeeping a raw Batch needs:
//
//	var p pgxexec.Pipeline
//	user := pgxexec.PipeOne[User](&p, getUser, qlx.Bind(id, 1))
//	posts := pgxexec.PipeAll[Post](&p, listPosts, qlx.Bind(id, 1))
//	if err := p.Run(ctx, pool); err != nil { ... }
//	u, _ := user.Get()
//
// Statements run in queue order inside one implicit transaction unless the
// sender is already a transaction (pgx batches are atomic by default).
type Pipeline struct {
	b     Batch
	reads []func(pgx.BatchResults) error
	ran   bool
}

// Future is the typed result of one pipelined statement. Get is valid only
// after Pipeline.Run has returned.
type Future[T any] struct {
	v    T
	err  error
	done bool
}

// Get returns the result. Before Run it reports ErrNotRun.
func (f *Future[T]) Get() (T, error) {
	if !f.done {
		var z T
		return z, ErrNotRun
	}
	return f.v, f.err
}

// ErrNotRun is returned by Future.Get when the pipeline has not run.
var ErrNotRun = errors.New("pgxexec: pipeline has not run")

func (p *Pipeline) add(st qlx.Stmt, binds []qlx.Binding, read func(pgx.BatchResults) error) {
	p.b.Queue(st, binds...)
	p.reads = append(p.reads, read)
}

// Len is the number of queued statements.
func (p *Pipeline) Len() int { return len(p.reads) }

// PipeAll queues a statement whose rows are read as a list of T (by position,
// through ScanTargets, like BatchAllScan).
func PipeAll[T any, PT Scannable[T]](p *Pipeline, st qlx.Stmt, binds ...qlx.Binding) *Future[[]T] {
	f := &Future[[]T]{}
	p.add(st, binds, func(res pgx.BatchResults) error {
		f.v, f.err = BatchAllScan[T, PT](res)
		f.done = true
		return f.err
	})
	return f
}

// PipeOne queues a statement that must return exactly one row.
func PipeOne[T any, PT Scannable[T]](p *Pipeline, st qlx.Stmt, binds ...qlx.Binding) *Future[T] {
	f := &Future[T]{}
	p.add(st, binds, func(res pgx.BatchResults) error {
		f.v, f.err = BatchOneScan[T, PT](res)
		f.done = true
		return f.err
	})
	return f
}

// PipeExec queues a statement and keeps its command tag (INSERT/UPDATE/DELETE
// without RETURNING).
func PipeExec(p *Pipeline, st qlx.Stmt, binds ...qlx.Binding) *Future[pgconn.CommandTag] {
	f := &Future[pgconn.CommandTag]{}
	p.add(st, binds, func(res pgx.BatchResults) error {
		f.v, f.err = res.Exec()
		f.done = true
		return f.err
	})
	return f
}

// Run sends the batch and reads every result in order. A statement that fails
// aborts the rest on the server (the batch is one transaction); the futures
// before it keep their values and the failing one holds its error, those
// after it hold the batch error. The first error is returned. A Pipeline runs
// once.
func (p *Pipeline) Run(ctx context.Context, db BatchSender) error {
	if p.ran {
		return errors.New("pgxexec: pipeline already ran")
	}
	p.ran = true
	res, err := p.b.Send(ctx, db)
	if err != nil {
		return err
	}
	var first error
	for _, read := range p.reads {
		if first != nil {
			// The server stopped at the first failure; draining further
			// results would only repeat that error.
			break
		}
		first = read(res)
	}
	if cerr := res.Close(); first == nil {
		first = cerr
	}
	return first
}
