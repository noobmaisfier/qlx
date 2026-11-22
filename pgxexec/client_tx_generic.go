package pgxexec

import (
	"context"
	"iter"

	"github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/qpage"
	"github.com/noobmaisfier/qlx/qt"
)

// Go 1.27 generic methods of Tx and the bulk insert that reads RETURNING rows.
// They mirror the ones of Client (client_generic.go): the row type is a method
// type parameter, and every one delegates to the same helper as its Client
// twin, so a row is scanned the same way inside and outside a transaction.

// All runs st in the transaction and scans every row into R.
func (t *Tx) All[R any](ctx context.Context, st qlx.Statement, binds ...qlx.Binding) ([]R, error) {
	return allR[R](ctx, t.tx, t.c.resolver(st), binds)
}

// Page is Client.Page inside the transaction.
func (t *Tx) Page[R any](ctx context.Context, base qlx.SelectQuery, s *qpage.SortSet[R], req qpage.Request, binds ...qlx.Binding) (qpage.Page[R], error) {
	return pageR[R](ctx, t.tx, t.c.resolver, base, s, req, binds)
}

// One scans exactly one row; pgx.ErrNoRows / pgx.ErrTooManyRows otherwise.
func (t *Tx) One[R any](ctx context.Context, st qlx.Statement, binds ...qlx.Binding) (R, error) {
	return oneR[R](ctx, t.tx, t.c.resolver(st), binds)
}

// Maybe scans zero or one row.
func (t *Tx) Maybe[R any](ctx context.Context, st qlx.Statement, binds ...qlx.Binding) (qt.Opt[R], error) {
	return maybeR[R](ctx, t.tx, t.c.resolver(st), binds)
}

// Each streams rows without materialising the slice. The transaction's
// connection is busy until the loop ends: do not run another statement on tx
// from inside the loop.
func (t *Tx) Each[R any](ctx context.Context, st qlx.Statement, binds ...qlx.Binding) iter.Seq2[R, error] {
	return eachR[R](ctx, t.tx, t.c.resolver(st), binds)
}

// InsertManyReturning inserts rows and returns the RETURNING rows of every
// statement, in order (see the package-level InsertManyReturning).
func (t *Tx) InsertManyReturning[R any](ctx context.Context, base qlx.InsertQuery, rows []qlx.RowAssigner, opt BulkOptions) ([]R, error) {
	return InsertManyReturning[R](ctx, t.tx, t.c.Dialect, base, rows, opt)
}

// InsertManyReturning inserts rows and returns the RETURNING rows of every
// statement, in order; several statements run in one transaction.
func (c *Client) InsertManyReturning[R any](ctx context.Context, base qlx.InsertQuery, rows []qlx.RowAssigner, opt BulkOptions) ([]R, error) {
	return InsertManyReturning[R](ctx, c.db, c.Dialect, base, rows, opt)
}
