package pgxexec

import (
	"context"
	"iter"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/qt"
)

// Go 1.27 generic methods on Client. The row type is a method type parameter,
// so the call site reads client.All[User](ctx, query) and the tracer and the
// shape cache configured at Connect apply without passing anything around.
//
// The statement may be a builder written in line, with its values:
//
//	users, err := client.All[UserRow](ctx,
//	    qlx.Select(Users.ID, Users.Email).From(Users).Where(Users.Tier.Eq(tier)))
//
// The first call with a given shape renders it; every later call finds the
// SQL by hashing what rendering would write (see qlx.ShapeCache) and only
// collects the values. Nothing has to be declared, registered or prepared up
// front. A statement rendered earlier (a qlx.Stmt, or a NewStatic handle's
// Stmt) is accepted too and skips the lookup.

// All runs st and scans every row into R.
func (c *Client) All[R any](ctx context.Context, st qlx.Statement, binds ...qlx.Binding) ([]R, error) {
	return allR[R](ctx, c.db, c.resolver(st), binds)
}

// One scans exactly one row; pgx.ErrNoRows / pgx.ErrTooManyRows otherwise.
func (c *Client) One[R any](ctx context.Context, st qlx.Statement, binds ...qlx.Binding) (R, error) {
	return oneR[R](ctx, c.db, c.resolver(st), binds)
}

// Maybe scans zero or one row.
func (c *Client) Maybe[R any](ctx context.Context, st qlx.Statement, binds ...qlx.Binding) (qt.Opt[R], error) {
	return maybeR[R](ctx, c.db, c.resolver(st), binds)
}

// Each streams rows without materialising the slice.
func (c *Client) Each[R any](ctx context.Context, st qlx.Statement, binds ...qlx.Binding) iter.Seq2[R, error] {
	return eachR[R](ctx, c.db, c.resolver(st), binds)
}

// Exec runs a statement that returns no rows.
func (c *Client) Exec(ctx context.Context, st qlx.Statement, binds ...qlx.Binding) (pgconn.CommandTag, error) {
	sql, args, err := c.Shapes.Resolve(c.Dialect, st, binds...)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	return c.db.Exec(ctx, sql, args...)
}
