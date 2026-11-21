package pgxexec_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	q "github.com/noobmaisfier/qlx"
	fx "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/pgxexec"
)

// client connects with the test DSN and removes the rows these tests insert.
func client(t *testing.T, opt pgxexec.ConnectOptions) *pgxexec.Client {
	t.Helper()
	dsn := os.Getenv("QLX_TEST_DSN")
	if dsn == "" {
		t.Skip("QLX_TEST_DSN not set")
	}
	ctx := context.Background()
	c, err := pgxexec.Connect(ctx, dsn, opt)
	if err != nil {
		t.Fatal(err)
	}
	clean := func() { _, _ = c.Pool.Exec(ctx, "DELETE FROM users WHERE email LIKE 'zzctx-%'") }
	clean()
	t.Cleanup(func() { clean(); c.Close() })
	return c
}

func countCtx(t *testing.T, c *pgxexec.Client) int {
	t.Helper()
	var n int
	if err := c.Pool.QueryRow(context.Background(), "SELECT count(*) FROM users WHERE email LIKE 'zzctx-%'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestClientInTxCommitsAndRollsBack(t *testing.T) {
	c := client(t, pgxexec.ConnectOptions{})
	ctx := context.Background()
	rows := []q.RowAssigner{
		rowAssign{fx.Users.Email.To("zzctx-1")},
		rowAssign{fx.Users.Email.To("zzctx-2")},
	}

	err := c.InTx(ctx, pgx.TxOptions{}, pgxexec.Retry{}, func(ctx context.Context, tx *pgxexec.Tx) error {
		n, err := tx.InsertMany(ctx, q.InsertInto(fx.Users), rows, pgxexec.BulkOptions{MaxParams: 1})
		if err != nil || n != 2 {
			t.Errorf("InsertMany in tx: %d %v", n, err)
		}
		_, err = tx.Exec(ctx, q.Update(fx.Users).Set(fx.Users.Balance.To(7)).Where(fx.Users.Email.HasPrefix("zzctx-")))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := countCtx(t, c); n != 2 {
		t.Fatalf("commit: %d rows, want 2", n)
	}

	boom := errors.New("boom")
	err = c.InTx(ctx, pgx.TxOptions{}, pgxexec.Retry{}, func(ctx context.Context, tx *pgxexec.Tx) error {
		if _, err := tx.Exec(ctx, q.DeleteFrom(fx.Users).Where(fx.Users.Email.HasPrefix("zzctx-"))); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	if n := countCtx(t, c); n != 2 {
		t.Fatalf("rollback: %d rows, want 2 (the delete must be undone)", n)
	}
}

func TestClientTxSavepointKeepsTheTransactionUsable(t *testing.T) {
	c := client(t, pgxexec.ConnectOptions{})
	ctx := context.Background()
	err := c.InTx(ctx, pgx.TxOptions{}, pgxexec.Retry{}, func(ctx context.Context, tx *pgxexec.Tx) error {
		one := []q.RowAssigner{rowAssign{fx.Users.Email.To("zzctx-sp1")}}
		if _, err := tx.InsertMany(ctx, q.InsertInto(fx.Users), one, pgxexec.BulkOptions{}); err != nil {
			return err
		}
		bad := tx.Savepoint(ctx, "inner", func(ctx context.Context) error {
			dup := []q.RowAssigner{rowAssign{fx.Users.Email.To("zzctx-sp1")}}
			_, err := tx.InsertMany(ctx, q.InsertInto(fx.Users), dup, pgxexec.BulkOptions{})
			return err
		})
		if !pgxexec.IsUnique(bad, "") && bad == nil {
			t.Errorf("the duplicate inside the savepoint must fail: %v", bad)
		}
		// the transaction is still alive and holds the first row
		_, err := tx.Exec(ctx, q.Update(fx.Users).Set(fx.Users.Balance.To(1)).Where(fx.Users.Email.Eq("zzctx-sp1")))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := countCtx(t, c); n != 1 {
		t.Fatalf("%d rows, want 1", n)
	}
}

func TestClientInTxIsTraced(t *testing.T) {
	tr := &countTracer{}
	c := client(t, pgxexec.ConnectOptions{Tracer: tr})
	ctx := context.Background()
	err := c.InTx(ctx, pgx.TxOptions{}, pgxexec.Retry{}, func(ctx context.Context, tx *pgxexec.Tx) error {
		_, err := tx.Exec(ctx, q.Update(fx.Users).Set(fx.Users.Balance.To(1)).Where(fx.Users.Email.Eq("zzctx-none")))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if tr.n == 0 {
		t.Error("a statement run through Tx.Exec was not traced")
	}
}

func TestClientInsertManyIsAtomic(t *testing.T) {
	c := client(t, pgxexec.ConnectOptions{Tracer: &countTracer{}})
	rows := []q.RowAssigner{
		rowAssign{fx.Users.Email.To("zzctx-a")},
		rowAssign{fx.Users.Email.To("zzctx-b")},
		rowAssign{fx.Users.Email.To("zzctx-a")}, // duplicate: the last chunk fails
	}
	if _, err := c.InsertMany(context.Background(), q.InsertInto(fx.Users), rows, pgxexec.BulkOptions{MaxParams: 1}); err == nil {
		t.Fatal("the duplicate must fail")
	}
	if n := countCtx(t, c); n != 0 {
		t.Fatalf("%d rows left behind by a failed Client.InsertMany", n)
	}
	ok := rows[:2]
	if n, err := c.InsertMany(context.Background(), q.InsertInto(fx.Users), ok, pgxexec.BulkOptions{MaxParams: 1}); err != nil || n != 2 {
		t.Fatalf("InsertMany: %d %v", n, err)
	}
}

func TestClientRunPipeline(t *testing.T) {
	c := client(t, pgxexec.ConnectOptions{})
	var p pgxexec.Pipeline
	a := pgxexec.PipeOne[q.Tuple1[int32]](&p, q.Stmt{SQL: "SELECT 1"})
	b := pgxexec.PipeOne[q.Tuple1[int32]](&p, q.Stmt{SQL: "SELECT 2"})
	if err := c.RunPipeline(context.Background(), &p); err != nil {
		t.Fatal(err)
	}
	x, err1 := a.Get()
	y, err2 := b.Get()
	if err1 != nil || err2 != nil || x.V1 != 1 || y.V1 != 2 {
		t.Errorf("%v %v %v %v", x, y, err1, err2)
	}
}
