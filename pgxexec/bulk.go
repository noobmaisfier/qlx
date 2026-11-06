package pgxexec

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/noobmaisfier/qlx"
)

// BulkOptions tunes InsertMany. The zero value is right for most callers.
type BulkOptions struct {
	// MaxParams is the parameter budget of one statement; 0 means
	// qlx.MaxBindParams. Lower it for rows built with ToX over expressions
	// that carry arguments of their own.
	MaxParams int
}

// InsertMany inserts rows with as few statements as the parameter limit
// allows (see qlx.InsertQuery.Chunks) and returns the number of rows the
// server reported.
//
// When db can start a transaction (a pool or a connection) and more than one
// statement is needed, they run in one transaction, so a failure leaves
// nothing behind. When db is already a transaction, the statements join it.
// base carries the target, ON CONFLICT and no rows; RETURNING is not read
// here: use InsertManyReturning for that.
func InsertMany(ctx context.Context, db Querier, d qlx.Dialect, base qlx.InsertQuery, rows []qlx.RowAssigner, opt BulkOptions) (int64, error) {
	chunks, err := base.Chunks(rows, opt.MaxParams)
	if err != nil {
		return 0, err
	}
	var total int64
	run := func(ctx context.Context, q Querier) error {
		for _, c := range chunks {
			st, err := qlx.Render(d, c)
			if err != nil {
				return err
			}
			tag, err := Exec(ctx, q, st)
			if err != nil {
				return err
			}
			total += tag.RowsAffected()
		}
		return nil
	}
	if err := inOneTx(ctx, db, len(chunks), run); err != nil {
		return 0, err
	}
	return total, nil
}

// InsertManyReturning is InsertMany for a statement with RETURNING: the
// returned rows of every statement are collected, in order.
func InsertManyReturning[R any](ctx context.Context, db Querier, d qlx.Dialect, base qlx.InsertQuery, rows []qlx.RowAssigner, opt BulkOptions) ([]R, error) {
	chunks, err := base.Chunks(rows, opt.MaxParams)
	if err != nil {
		return nil, err
	}
	var out []R
	run := func(ctx context.Context, q Querier) error {
		for _, c := range chunks {
			st, err := qlx.Render(d, c)
			if err != nil {
				return err
			}
			got, err := All[R](ctx, q, st)
			if err != nil {
				return err
			}
			out = append(out, got...)
		}
		return nil
	}
	if err := inOneTx(ctx, db, len(chunks), run); err != nil {
		return nil, err
	}
	return out, nil
}

func inOneTx(ctx context.Context, db Querier, n int, run func(context.Context, Querier) error) error {
	if s, ok := db.(TxStarter); ok && n > 1 {
		return runTx(ctx, s, pgx.TxOptions{}, func(ctx context.Context, tx pgx.Tx) error { return run(ctx, tx) })
	}
	return run(ctx, db)
}
