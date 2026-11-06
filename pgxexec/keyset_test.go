package pgxexec_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	q "github.com/noobmaisfier/qlx"
	fx "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/pgxexec"
)

type bulkRow struct {
	email   string
	balance int64
}

func (r bulkRow) Assignments() []q.Assignment {
	return []q.Assignment{fx.Users.Email.To(r.email), fx.Users.Balance.To(r.balance)}
}

type idBal struct {
	ID      int64 `db:"id"`
	Balance int64 `db:"balance"`
}

func bulkRows(prefix string, n int) []q.RowAssigner {
	rows := make([]q.RowAssigner, n)
	for i := range rows {
		// few distinct balances: many ties, so the id decides at boundaries
		rows[i] = bulkRow{fmt.Sprintf("%s%04d@x.io", prefix, i), int64(i % 5)}
	}
	return rows
}

// Walking every page of a keyset query returns exactly the rows of the
// unpaginated query, in the same order, whatever the page size and the mix of
// directions.
func TestKeysetWalkAgainstServer(t *testing.T) {
	p := pool(t)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		U := fx.Users
		d := q.Dialect{Version: q.PG18}
		if n, err := pgxexec.InsertMany(ctx, tx, d, q.InsertInto(U), bulkRows("ks-", 53), pgxexec.BulkOptions{}); err != nil || n != 53 {
			t.Fatalf("insert: %d %v", n, err)
		}
		mine := U.Email.HasPrefix("ks-")
		full, err := pgxexec.All[idBal](ctx, tx, render(t, q.Select(U.ID, U.Balance).From(U).Where(mine).OrderBy(U.Balance.Desc(), U.ID.Asc())))
		if err != nil || len(full) != 53 {
			t.Fatalf("full: %d %v", len(full), err)
		}
		for _, size := range []int64{1, 7, 10, 53, 100} {
			var got []idBal
			var keys []q.Key
			byBal, byID := q.KeyDesc(U.Balance), q.KeyAsc(U.ID)
			for pages := 0; pages < 200; pages++ {
				st := render(t, q.Select(U.ID, U.Balance).From(U).Where(mine, q.After(keys...)).OrderBy(q.KeyOrder(byBal, byID)...).Limit(size))
				rows, err := pgxexec.All[idBal](ctx, tx, st)
				if err != nil {
					t.Fatal(err)
				}
				if len(rows) == 0 {
					break
				}
				got = append(got, rows...)
				last := rows[len(rows)-1]
				keys = []q.Key{byBal.After(last.Balance), byID.After(last.ID)}
			}
			if !slices.Equal(got, full) {
				t.Fatalf("size %d: walk differs from the full query (%d vs %d rows)", size, len(got), len(full))
			}
		}
		// same direction: the row comparison form
		var got []int64
		var keys []q.Key
		upBal, upID := q.KeyAsc(U.Balance), q.KeyAsc(U.ID)
		for range 100 {
			rows, err := pgxexec.All[idBal](ctx, tx, render(t, q.Select(U.ID, U.Balance).From(U).Where(mine, q.After(keys...)).
				OrderBy(q.KeyOrder(upBal, upID)...).Limit(8)))
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) == 0 {
				break
			}
			for _, r := range rows {
				got = append(got, r.ID)
			}
			last := rows[len(rows)-1]
			keys = []q.Key{upBal.After(last.Balance), upID.After(last.ID)}
		}
		want, _ := pgxexec.All[int64](ctx, tx, render(t, q.Select(U.ID).From(U).Where(mine).OrderBy(U.Balance.Asc(), U.ID.Asc())))
		if !slices.Equal(got, want) {
			t.Fatalf("row comparison walk differs: %d vs %d", len(got), len(want))
		}
	})
}

func TestSortsAgainstServer(t *testing.T) {
	p := pool(t)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		U := fx.Users
		d := q.Dialect{Version: q.PG18}
		if _, err := pgxexec.InsertMany(ctx, tx, d, q.InsertInto(U), bulkRows("so-", 12), pgxexec.BulkOptions{}); err != nil {
			t.Fatal(err)
		}
		s := q.NewSorts(map[string]q.Orderable{"balance": U.Balance, "email": U.Email}).Tiebreak(U.ID.Asc())
		for _, spec := range []string{"", "balance", "-balance", "-balance,email", "email,-balance"} {
			ord, err := s.Parse(spec)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pgxexec.All[int64](ctx, tx, render(t, q.Select(U.ID).From(U).Where(U.Email.HasPrefix("so-")).OrderBy(ord...))); err != nil {
				t.Fatalf("%q: %v", spec, err)
			}
		}
		ord, _ := s.Parse("-balance")
		ids, _ := pgxexec.All[idBal](ctx, tx, render(t, q.Select(U.ID, U.Balance).From(U).Where(U.Email.HasPrefix("so-")).OrderBy(ord...)))
		if !slices.IsSortedFunc(ids, func(a, b idBal) int { return int(b.Balance - a.Balance) }) {
			t.Fatalf("not sorted descending: %v", ids)
		}
	})
}

func TestInsertManyAgainstServer(t *testing.T) {
	p := pool(t)
	ctx := context.Background()
	d := q.Dialect{Version: q.PG18}
	U := fx.Users
	count := func(prefix string) int64 {
		t.Helper()
		n, err := pgxexec.One[int64](ctx, p, render(t, q.Select(q.CountAll()).From(U).Where(U.Email.HasPrefix(prefix))))
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	clean := func(prefix string) {
		_, _ = pgxexec.Exec(ctx, p, render(t, q.DeleteFrom(U).Where(U.Email.HasPrefix(prefix))))
	}
	// many statements: a 10-parameter budget gives 5 rows per statement
	clean("bm-")
	t.Cleanup(func() { clean("bm-") })
	n, err := pgxexec.InsertMany(ctx, p, d, q.InsertInto(U), bulkRows("bm-", 23), pgxexec.BulkOptions{MaxParams: 10})
	if err != nil || n != 23 || count("bm-") != 23 {
		t.Fatalf("pool: %d %v count %d", n, err, count("bm-"))
	}
	// all or nothing on a pool: a duplicate in the last statement undoes the first ones
	clean("bf-")
	t.Cleanup(func() { clean("bf-") })
	rows := bulkRows("bf-", 12)
	rows = append(rows, rows[0])
	if _, err := pgxexec.InsertMany(ctx, p, d, q.InsertInto(U), rows, pgxexec.BulkOptions{MaxParams: 10}); !pgxexec.IsUnique(err, "users_email_key") {
		t.Fatalf("want a unique violation, got %v", err)
	}
	if c := count("bf-"); c != 0 {
		t.Fatalf("%d rows left behind", c)
	}
	// RETURNING collects every statement's rows, in order
	clean("br-")
	t.Cleanup(func() { clean("br-") })
	ids, err := pgxexec.InsertManyReturning[int64](ctx, p, d, q.InsertInto(U).Returning(U.ID), bulkRows("br-", 11), pgxexec.BulkOptions{MaxParams: 6})
	if err != nil || len(ids) != 11 || !slices.IsSorted(ids) {
		t.Fatalf("returning: %v %v", ids, err)
	}
	// over the protocol limit without chunking would fail: 40000 rows x 2 columns
	clean("bb-")
	t.Cleanup(func() { clean("bb-") })
	n, err = pgxexec.InsertMany(ctx, p, d, q.InsertInto(U), bulkRows("bb-", 40000), pgxexec.BulkOptions{})
	if err != nil || n != 40000 {
		t.Fatalf("big: %d %v", n, err)
	}
	// ON CONFLICT DO NOTHING in base is kept on every statement
	n, err = pgxexec.InsertMany(ctx, p, d, q.InsertInto(U).OnConflict(U.Email).DoNothing(), bulkRows("bm-", 23), pgxexec.BulkOptions{MaxParams: 10})
	if err != nil || n != 0 {
		t.Fatalf("on conflict: %d %v", n, err)
	}
}
