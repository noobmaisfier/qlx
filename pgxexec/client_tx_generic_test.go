package pgxexec_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	q "github.com/noobmaisfier/qlx"
	fx "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/pgxexec"
)

type ctxEmail struct {
	Email string `db:"email"`
}

// The generic methods of Tx and the RETURNING bulk insert read their rows the
// way Client's do, and see the transaction's own uncommitted writes.
func TestTxGenericMethodsSeeTheTransaction(t *testing.T) {
	c := client(t, pgxexec.ConnectOptions{})
	ctx := context.Background()
	rows := []q.RowAssigner{
		rowAssign{fx.Users.Email.To("zzctx-g1")},
		rowAssign{fx.Users.Email.To("zzctx-g2")},
		rowAssign{fx.Users.Email.To("zzctx-g3")},
	}
	err := c.InTx(ctx, pgx.TxOptions{}, pgxexec.Retry{}, func(ctx context.Context, tx *pgxexec.Tx) error {
		got, err := tx.InsertManyReturning[ctxEmail](ctx,
			q.InsertInto(fx.Users).Returning(fx.Users.Email), rows, pgxexec.BulkOptions{MaxParams: 1})
		if err != nil || len(got) != 3 || got[0].Email != "zzctx-g1" || got[2].Email != "zzctx-g3" {
			t.Errorf("InsertManyReturning: %v %v", got, err)
		}
		sel := q.Select(fx.Users.Email).From(fx.Users).Where(fx.Users.Email.HasPrefix("zzctx-g")).OrderBy(fx.Users.Email.Asc())
		all, err := tx.All[ctxEmail](ctx, sel)
		if err != nil || len(all) != 3 {
			t.Errorf("All sees %d rows, want 3: %v", len(all), err)
		}
		one, err := tx.One[ctxEmail](ctx, q.Select(fx.Users.Email).From(fx.Users).Where(fx.Users.Email.Eq("zzctx-g2")))
		if err != nil || one.Email != "zzctx-g2" {
			t.Errorf("One: %v %v", one, err)
		}
		none, err := tx.Maybe[ctxEmail](ctx, q.Select(fx.Users.Email).From(fx.Users).Where(fx.Users.Email.Eq("zzctx-none")))
		if err != nil || none.IsSet() {
			t.Errorf("Maybe: %v %v", none, err)
		}
		n := 0
		for r, err := range tx.Each[ctxEmail](ctx, sel) {
			if err != nil || r.Email == "" {
				t.Errorf("Each: %v %v", r, err)
			}
			n++
		}
		if n != 3 {
			t.Errorf("Each yielded %d rows, want 3", n)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClientInsertManyReturning(t *testing.T) {
	c := client(t, pgxexec.ConnectOptions{})
	rows := []q.RowAssigner{rowAssign{fx.Users.Email.To("zzctx-r1")}, rowAssign{fx.Users.Email.To("zzctx-r2")}}
	got, err := c.InsertManyReturning[ctxEmail](context.Background(),
		q.InsertInto(fx.Users).Returning(fx.Users.Email), rows, pgxexec.BulkOptions{MaxParams: 1})
	if err != nil || len(got) != 2 || got[1].Email != "zzctx-r2" {
		t.Fatalf("%v %v", got, err)
	}
}
