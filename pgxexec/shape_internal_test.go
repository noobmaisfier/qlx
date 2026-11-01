package pgxexec

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	q "github.com/noobmaisfier/qlx"
	fx "github.com/noobmaisfier/qlx/fixture"
)

type shapeUser struct {
	ID    int64
	Email string
}

// The automatic path end to end: natural builders with in-line values, run
// through the shape cache against a real server. (Client.All[R] and friends
// are thin generic-method wrappers over the functions used here.)
func TestShapeCacheThroughExecutor(t *testing.T) {
	dsn := os.Getenv("QLX_TEST_DSN")
	if dsn == "" {
		t.Skip("QLX_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	d, err := DetectDialect(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{Pool: pool, Dialect: d, Shapes: q.NewShapeCache(64), db: pool}
	if _, err := pool.Exec(ctx, `DELETE FROM orders; DELETE FROM users`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM orders; DELETE FROM users`) })

	for _, e := range []string{"a@x", "b@x", "c@x"} {
		ins := q.InsertInto(fx.Users).Row(fx.Users.Email.To(e))
		sql, args, err := c.Shapes.Resolve(c.Dialect, ins)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []string{"a@x", "c@x", "zzz"} {
		rows, err := allR[shapeUser](ctx, c.db, c.resolver(
			q.Select(fx.Users.ID, fx.Users.Email).From(fx.Users).Where(fx.Users.Email.Eq(e))), nil)
		if err != nil {
			t.Fatal(err)
		}
		if e == "zzz" && len(rows) != 0 || e != "zzz" && (len(rows) != 1 || rows[0].Email != e) {
			t.Fatalf("%s: %v", e, rows)
		}
	}
	one, err := oneR[shapeUser](ctx, c.db, c.resolver(q.Select(fx.Users.ID, fx.Users.Email).From(fx.Users).Where(fx.Users.Email.Eq("b@x"))), nil)
	if err != nil || one.Email != "b@x" {
		t.Fatalf("%v %v", one, err)
	}
	st := c.Shapes.Stats()
	// two shapes (insert, select by email); the select ran 4 times, the insert 3
	if st.Size != 2 || st.Misses != 2 || st.Hits != 5 {
		t.Fatalf("stats %+v", st)
	}
}
