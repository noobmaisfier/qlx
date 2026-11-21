package pgxexec

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/noobmaisfier/qlx"
)

// The operations below put on Client (and on the Tx it hands to InTx) what the
// package-level functions already do, so a program holds one value and writes
//
//	err := client.InTx(ctx, pgx.TxOptions{}, pgxexec.Retry{}, func(ctx context.Context, tx *pgxexec.Tx) error {
//	    if _, err := tx.Exec(ctx, debit); err != nil {
//	        return err
//	    }
//	    _, err := tx.InsertMany(ctx, q.InsertInto(Ledger), rows, pgxexec.BulkOptions{})
//	    return err
//	})
//
// instead of passing client.Pool, client.Dialect and a hand-rendered statement
// to each package-level function. The package-level functions stay: they take
// any Querier, which is what a test or a custom executor passes. The methods
// that read rows into a type parameter (All, One, Maybe, Each,
// InsertManyReturning) are in client_tx_generic.go.

// Tx is a transaction started by Client.InTx. It runs statements with the
// client's dialect, shape cache and tracer, like Client does, and it is the
// same transaction for every call: commit and rollback belong to InTx.
type Tx struct {
	tx pgx.Tx
	c  *Client
}

// Pgx returns the underlying pgx transaction, for anything qlx does not wrap
// (LargeObjects, SendBatch, raw SQL). Do not commit or roll it back: InTx does.
func (t *Tx) Pgx() pgx.Tx { return t.tx }

// InTx runs fn in a transaction, retrying the whole function on a
// serialization failure or deadlock (see the package-level InTx). The
// statements fn runs through tx are traced when the client has a tracer.
func (c *Client) InTx(ctx context.Context, opts pgx.TxOptions, r Retry, fn func(ctx context.Context, tx *Tx) error) error {
	var starter TxStarter = c.Pool
	if s, ok := c.db.(TxStarter); ok {
		starter = s // keeps the tracer on the statements inside the transaction
	}
	return InTx(ctx, starter, opts, r, func(ctx context.Context, tx pgx.Tx) error {
		return fn(ctx, &Tx{tx: tx, c: c})
	})
}

// Exec runs a statement that returns no rows inside the transaction.
func (t *Tx) Exec(ctx context.Context, st qlx.Statement, binds ...qlx.Binding) (pgconn.CommandTag, error) {
	sql, args, err := t.c.Shapes.Resolve(t.c.Dialect, st, binds...)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	return t.tx.Exec(ctx, sql, args...)
}

// InsertMany inserts rows in the transaction, in as many statements as the
// parameter limit needs (see the package-level InsertMany).
func (t *Tx) InsertMany(ctx context.Context, base qlx.InsertQuery, rows []qlx.RowAssigner, opt BulkOptions) (int64, error) {
	return InsertMany(ctx, t.tx, t.c.Dialect, base, rows, opt)
}

// CopyIn copies rows into the table of st with binary COPY, inside the
// transaction.
func (t *Tx) CopyIn(ctx context.Context, st qlx.CopyFromStdin, rows pgx.CopyFromSource) (int64, error) {
	return CopyIn(ctx, t.tx, st, rows)
}

// Savepoint runs fn in a savepoint: if fn fails its work is rolled back and
// the error returned, and the transaction stays usable (see WithSavepoint).
func (t *Tx) Savepoint(ctx context.Context, name string, fn func(ctx context.Context) error) error {
	return WithSavepoint(ctx, t.tx, name, fn)
}

// InsertMany inserts rows, splitting them to respect the parameter limit and
// running the statements in one transaction when more than one is needed.
func (c *Client) InsertMany(ctx context.Context, base qlx.InsertQuery, rows []qlx.RowAssigner, opt BulkOptions) (int64, error) {
	return InsertMany(ctx, c.db, c.Dialect, base, rows, opt)
}

// CopyIn copies rows into the table of st with binary COPY, the fastest bulk
// path.
func (c *Client) CopyIn(ctx context.Context, st qlx.CopyFromStdin, rows pgx.CopyFromSource) (int64, error) {
	return CopyIn(ctx, c.Pool, st, rows)
}

// RunPipeline sends the pipeline's statements in one round trip (see Pipeline).
func (c *Client) RunPipeline(ctx context.Context, p *Pipeline) error {
	return p.Run(ctx, c.Pool)
}
