package pgxexec

import (
	"context"
	"iter"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/qt"
)

// Static handles are typed, pre-rendered queries for the hot path. The shape
// is rendered once when the handle is built (a package-level var or init-time
// call), so a call is: bind arguments, send, scan. There is nothing to hash
// or render per call, and the argument types are checked by the compiler:
//
//	var getUser = pgxexec.NewStatic1[User](dialect, query, id) // id is a qlx.Param[int64]
//
//	u, err := getUser.One(ctx, db, 42)
//
// The number after Static is the number of parameters. R is the row type and
// scans the way All[R] does.

func render(d qlx.Dialect, s qlx.Statement) qlx.Stmt {
	st, err := qlx.Render(d, s)
	if err != nil {
		// A static query that does not render is a programming error found
		// at init, with the file and line of the Static call in the trace.
		panic("pgxexec: static query does not render: " + err.Error())
	}
	return st
}

// Static0 is a handle for a query without parameters.
type Static0[R any] struct{ st qlx.Stmt }

// NewStatic0 renders s once.
func NewStatic0[R any](d qlx.Dialect, s qlx.Statement) Static0[R] {
	return Static0[R]{render(d, s)}
}

// Stmt returns the rendered statement (for tracing, qtest and verify).
func (h Static0[R]) Stmt() qlx.Stmt { return h.st }

// All returns every row.
func (h Static0[R]) All(ctx context.Context, db Querier) ([]R, error) { return All[R](ctx, db, h.st) }

// One returns exactly one row.
func (h Static0[R]) One(ctx context.Context, db Querier) (R, error) { return One[R](ctx, db, h.st) }

// Maybe returns zero or one row.
func (h Static0[R]) Maybe(ctx context.Context, db Querier) (qt.Opt[R], error) {
	return Maybe[R](ctx, db, h.st)
}

// Each streams the rows.
func (h Static0[R]) Each(ctx context.Context, db Querier) iter.Seq2[R, error] {
	return Each[R](ctx, db, h.st)
}

// Exec runs the statement without reading rows.
func (h Static0[R]) Exec(ctx context.Context, db Querier) (pgconn.CommandTag, error) {
	return Exec(ctx, db, h.st)
}

// Static1 is a handle for a query with one parameter.
type Static1[R, A any] struct {
	st qlx.Stmt
	a  qlx.Param[A]
}

// NewStatic1 renders s once; a must be the parameter used in s.
func NewStatic1[R, A any](d qlx.Dialect, s qlx.Statement, a qlx.Param[A]) Static1[R, A] {
	return Static1[R, A]{render(d, s), a}
}

// Stmt returns the rendered statement.
func (h Static1[R, A]) Stmt() qlx.Stmt { return h.st }

// All returns every row.
func (h Static1[R, A]) All(ctx context.Context, db Querier, a A) ([]R, error) {
	return All[R](ctx, db, h.st, h.a.Bind(a))
}

// One returns exactly one row.
func (h Static1[R, A]) One(ctx context.Context, db Querier, a A) (R, error) {
	return One[R](ctx, db, h.st, h.a.Bind(a))
}

// Maybe returns zero or one row.
func (h Static1[R, A]) Maybe(ctx context.Context, db Querier, a A) (qt.Opt[R], error) {
	return Maybe[R](ctx, db, h.st, h.a.Bind(a))
}

// Each streams the rows.
func (h Static1[R, A]) Each(ctx context.Context, db Querier, a A) iter.Seq2[R, error] {
	return Each[R](ctx, db, h.st, h.a.Bind(a))
}

// Exec runs the statement without reading rows.
func (h Static1[R, A]) Exec(ctx context.Context, db Querier, a A) (pgconn.CommandTag, error) {
	return Exec(ctx, db, h.st, h.a.Bind(a))
}

// Static2 is a handle for a query with two parameters.
type Static2[R, A, B any] struct {
	st qlx.Stmt
	a  qlx.Param[A]
	b  qlx.Param[B]
}

// NewStatic2 renders s once.
func NewStatic2[R, A, B any](d qlx.Dialect, s qlx.Statement, a qlx.Param[A], b qlx.Param[B]) Static2[R, A, B] {
	return Static2[R, A, B]{render(d, s), a, b}
}

// Stmt returns the rendered statement.
func (h Static2[R, A, B]) Stmt() qlx.Stmt { return h.st }

// All returns every row.
func (h Static2[R, A, B]) All(ctx context.Context, db Querier, a A, b B) ([]R, error) {
	return All[R](ctx, db, h.st, h.a.Bind(a), h.b.Bind(b))
}

// One returns exactly one row.
func (h Static2[R, A, B]) One(ctx context.Context, db Querier, a A, b B) (R, error) {
	return One[R](ctx, db, h.st, h.a.Bind(a), h.b.Bind(b))
}

// Maybe returns zero or one row.
func (h Static2[R, A, B]) Maybe(ctx context.Context, db Querier, a A, b B) (qt.Opt[R], error) {
	return Maybe[R](ctx, db, h.st, h.a.Bind(a), h.b.Bind(b))
}

// Each streams the rows.
func (h Static2[R, A, B]) Each(ctx context.Context, db Querier, a A, b B) iter.Seq2[R, error] {
	return Each[R](ctx, db, h.st, h.a.Bind(a), h.b.Bind(b))
}

// Exec runs the statement without reading rows.
func (h Static2[R, A, B]) Exec(ctx context.Context, db Querier, a A, b B) (pgconn.CommandTag, error) {
	return Exec(ctx, db, h.st, h.a.Bind(a), h.b.Bind(b))
}

// Static3 is a handle for a query with three parameters.
type Static3[R, A, B, C any] struct {
	st qlx.Stmt
	a  qlx.Param[A]
	b  qlx.Param[B]
	c  qlx.Param[C]
}

// NewStatic3 renders s once.
func NewStatic3[R, A, B, C any](d qlx.Dialect, s qlx.Statement, a qlx.Param[A], b qlx.Param[B], c qlx.Param[C]) Static3[R, A, B, C] {
	return Static3[R, A, B, C]{render(d, s), a, b, c}
}

// Stmt returns the rendered statement.
func (h Static3[R, A, B, C]) Stmt() qlx.Stmt { return h.st }

// All returns every row.
func (h Static3[R, A, B, C]) All(ctx context.Context, db Querier, a A, b B, c C) ([]R, error) {
	return All[R](ctx, db, h.st, h.a.Bind(a), h.b.Bind(b), h.c.Bind(c))
}

// One returns exactly one row.
func (h Static3[R, A, B, C]) One(ctx context.Context, db Querier, a A, b B, c C) (R, error) {
	return One[R](ctx, db, h.st, h.a.Bind(a), h.b.Bind(b), h.c.Bind(c))
}

// Maybe returns zero or one row.
func (h Static3[R, A, B, C]) Maybe(ctx context.Context, db Querier, a A, b B, c C) (qt.Opt[R], error) {
	return Maybe[R](ctx, db, h.st, h.a.Bind(a), h.b.Bind(b), h.c.Bind(c))
}

// Each streams the rows.
func (h Static3[R, A, B, C]) Each(ctx context.Context, db Querier, a A, b B, c C) iter.Seq2[R, error] {
	return Each[R](ctx, db, h.st, h.a.Bind(a), h.b.Bind(b), h.c.Bind(c))
}

// Exec runs the statement without reading rows.
func (h Static3[R, A, B, C]) Exec(ctx context.Context, db Querier, a A, b B, c C) (pgconn.CommandTag, error) {
	return Exec(ctx, db, h.st, h.a.Bind(a), h.b.Bind(b), h.c.Bind(c))
}

// Static4 is a handle for a query with four parameters.
type Static4[R, A, B, C, D any] struct {
	st qlx.Stmt
	a  qlx.Param[A]
	b  qlx.Param[B]
	c  qlx.Param[C]
	d  qlx.Param[D]
}

// NewStatic4 renders s once.
func NewStatic4[R, A, B, C, D any](dl qlx.Dialect, s qlx.Statement, a qlx.Param[A], b qlx.Param[B], c qlx.Param[C], d qlx.Param[D]) Static4[R, A, B, C, D] {
	return Static4[R, A, B, C, D]{render(dl, s), a, b, c, d}
}

// Stmt returns the rendered statement.
func (h Static4[R, A, B, C, D]) Stmt() qlx.Stmt { return h.st }

// All returns every row.
func (h Static4[R, A, B, C, D]) All(ctx context.Context, db Querier, a A, b B, c C, d D) ([]R, error) {
	return All[R](ctx, db, h.st, h.a.Bind(a), h.b.Bind(b), h.c.Bind(c), h.d.Bind(d))
}

// One returns exactly one row.
func (h Static4[R, A, B, C, D]) One(ctx context.Context, db Querier, a A, b B, c C, d D) (R, error) {
	return One[R](ctx, db, h.st, h.a.Bind(a), h.b.Bind(b), h.c.Bind(c), h.d.Bind(d))
}

// Maybe returns zero or one row.
func (h Static4[R, A, B, C, D]) Maybe(ctx context.Context, db Querier, a A, b B, c C, d D) (qt.Opt[R], error) {
	return Maybe[R](ctx, db, h.st, h.a.Bind(a), h.b.Bind(b), h.c.Bind(c), h.d.Bind(d))
}

// Each streams the rows.
func (h Static4[R, A, B, C, D]) Each(ctx context.Context, db Querier, a A, b B, c C, d D) iter.Seq2[R, error] {
	return Each[R](ctx, db, h.st, h.a.Bind(a), h.b.Bind(b), h.c.Bind(c), h.d.Bind(d))
}

// Exec runs the statement without reading rows.
func (h Static4[R, A, B, C, D]) Exec(ctx context.Context, db Querier, a A, b B, c C, d D) (pgconn.CommandTag, error) {
	return Exec(ctx, db, h.st, h.a.Bind(a), h.b.Bind(b), h.c.Bind(c), h.d.Bind(d))
}
