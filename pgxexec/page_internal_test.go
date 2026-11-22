package pgxexec

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	q "github.com/noobmaisfier/qlx"
	fx "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qpage"
)

type pageUser struct {
	ID      int64  `db:"id"`
	Email   string `db:"email"`
	Balance int64  `db:"balance"`
}

var pageSorts = qpage.Sorts[pageUser](
	qpage.Sort("balance", qpage.Desc(fx.Users.Balance)), // many ties: the tiebreak decides at boundaries
	qpage.Sort("email", qpage.Asc(fx.Users.Email)),
	qpage.Sort("mixed", qpage.Desc(fx.Users.Balance), qpage.Asc(fx.Users.Email)), // mixed directions: the OR expansion
	qpage.Tiebreak(qpage.Asc(fx.Users.ID)),
)

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("QLX_TEST_DSN")
	if dsn == "" {
		t.Skip("QLX_TEST_DSN not set")
	}
	if err := qpage.SetSecret([]byte("0123456789abcdef-test-secret")); err != nil {
		t.Fatal(err)
	}
	return dsn
}

// pagePos has no tag: it is scanned by position, like a //qlx:row struct.
type pagePos struct {
	Balance int64
	Mail    string
	Num     int64
}

func (r *pagePos) ScanTargets() []any { return []any{&r.Num, &r.Mail, &r.Balance} }

var posSorts = qpage.Sorts[pagePos](
	qpage.Sort("balance", qpage.Desc(fx.Users.Balance)),
	qpage.Sort("mixed", qpage.Desc(fx.Users.Balance), qpage.Asc(fx.Users.Email)),
	qpage.Tiebreak(qpage.Asc(fx.Users.ID)),
)

func pageRes(st q.Statement) resolver {
	stmt, err := q.Render(q.Dialect{Version: q.PG18}, st)
	return func([]q.Binding) (string, []any, error) {
		if err != nil {
			return "", nil, err
		}
		args, err := stmt.Args()
		return stmt.SQL, args, err
	}
}

// Walking every page of every sort returns exactly the rows of the same query
// without pagination, whatever the page size.
func TestPageWalkAgainstServer(t *testing.T) {
	dsn := testDSN(t)
	p, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	ctx := context.Background()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	U := fx.Users
	d := q.Dialect{Version: q.PG18}
	rows := make([]q.RowAssigner, 53)
	for i := range rows {
		rows[i] = pageRow{email: "pg-" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + "@x.io", balance: int64(i % 5)}
	}
	if n, err := InsertMany(ctx, tx, d, q.InsertInto(U), rows, BulkOptions{}); err != nil || n != 53 {
		t.Fatalf("insert: %d %v", n, err)
	}
	base := q.Select(U.ID, U.Email, U.Balance).From(U).Where(U.Email.HasPrefix("pg-"))
	orders := map[string][]q.OrderTerm{
		"balance": {U.Balance.Desc(), U.ID.Asc()},
		"email":   {U.Email.Asc(), U.ID.Asc()},
		"mixed":   {U.Balance.Desc(), U.Email.Asc(), U.ID.Asc()},
	}
	for sort, order := range orders {
		full, err := All[pageUser](ctx, tx, mustRender(t, base.OrderBy(order...)))
		if err != nil || len(full) != 53 {
			t.Fatalf("%s: full %d %v", sort, len(full), err)
		}
		for _, size := range []int{1, 7, 10, 53, 100} {
			var got []pageUser
			cursor := ""
			for pages := 0; ; pages++ {
				if pages > 100 {
					t.Fatalf("%s/%d: never ended", sort, size)
				}
				page, err := pageR[pageUser](ctx, tx, pageRes, base, pageSorts, qpage.Request{Sort: sort, Cursor: cursor, Size: size}, nil)
				if err != nil {
					t.Fatalf("%s/%d: %v", sort, size, err)
				}
				got = append(got, page.Items...)
				if page.Next == "" {
					break
				}
				cursor = page.Next
			}
			if !slices.Equal(got, full) {
				t.Fatalf("%s/%d: the walk differs from the full query (%d vs %d rows)", sort, size, len(got), len(full))
			}
		}
	}

	// the same walk with a row scanned by position: the SELECT lists id, email, balance
	// and the row type scans Num, Mail, Balance in that order
	posBase := q.Select(U.ID, U.Email, U.Balance).From(U).Where(U.Email.HasPrefix("pg-"))
	for sort, order := range map[string][]q.OrderTerm{
		"balance": orders["balance"], "mixed": orders["mixed"],
	} {
		full, err := All[pagePos](ctx, tx, mustRender(t, posBase.OrderBy(order...)))
		if err != nil || len(full) != 53 {
			t.Fatalf("pos %s: full %d %v", sort, len(full), err)
		}
		for _, size := range []int{1, 8, 53} {
			var got []pagePos
			cursor := ""
			for pages := 0; ; pages++ {
				if pages > 100 {
					t.Fatalf("pos %s/%d: never ended", sort, size)
				}
				page, err := pageR[pagePos](ctx, tx, pageRes, posBase, posSorts, qpage.Request{Sort: sort, Cursor: cursor, Size: size}, nil)
				if err != nil {
					t.Fatalf("pos %s/%d: %v", sort, size, err)
				}
				got = append(got, page.Items...)
				if page.Next == "" {
					break
				}
				cursor = page.Next
			}
			if !slices.Equal(got, full) {
				t.Fatalf("pos %s/%d: the walk differs from the full query (%d vs %d rows)", sort, size, len(got), len(full))
			}
		}
	}
	// a select list out of the order of the fields is caught when the types differ...
	if _, err := pageR[pagePos](ctx, tx, pageRes, q.Select(U.Email, U.ID, U.Balance).From(U), posSorts, qpage.Request{Sort: "balance"}, nil); err == nil {
		t.Error("id at the position of a string must be refused")
	}
	// ...and not when two columns have the same type: the documented limit of scanning by position
	if _, err := pageR[pagePos](ctx, tx, pageRes, q.Select(U.Balance, U.Email, U.ID).From(U), posSorts, qpage.Request{Sort: "balance"}, nil); err != nil {
		t.Errorf("same-typed swap is not detectable: %v", err)
	}

	// errors reach the caller unchanged
	if _, err := pageR[pageUser](ctx, tx, pageRes, base, pageSorts, qpage.Request{Sort: "nope"}, nil); !errors.Is(err, qpage.ErrSort) {
		t.Errorf("unknown sort: %v", err)
	}
	if _, err := pageR[pageUser](ctx, tx, pageRes, base, pageSorts, qpage.Request{Cursor: "junk"}, nil); !errors.Is(err, qpage.ErrCursor) {
		t.Errorf("bad cursor: %v", err)
	}
	if _, err := pageR[pageUser](ctx, tx, pageRes, q.Select(U.ID).From(U), pageSorts, qpage.Request{}, nil); err == nil {
		t.Error("a SELECT without the key columns must be refused")
	}
}

type pageRow struct {
	email   string
	balance int64
}

func (r pageRow) Assignments() []q.Assignment {
	return []q.Assignment{fx.Users.Email.To(r.email), fx.Users.Balance.To(r.balance)}
}

func mustRender(t *testing.T, s q.Statement) q.Stmt {
	t.Helper()
	st, err := q.Render(q.Dialect{Version: q.PG18}, s)
	if err != nil {
		t.Fatal(err)
	}
	return st
}
