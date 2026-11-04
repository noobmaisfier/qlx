package pgxexec_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	q "github.com/noobmaisfier/qlx"
	fx "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/pgxexec"
	"github.com/noobmaisfier/qlx/qt"
)

// The cases of docs/mapping.md, against a real server. Each subtest names the
// section it backs.

type mapUser struct {
	ID    int64           `db:"id"`
	Email string          `db:"email"`
	Nick  qt.Null[string] `db:"nickname"`
}

type mapUserWithOrder struct {
	Email string `db:"email"`
	Total int64  `db:"total"` // plain: breaks on a user without orders
}

type mapUserMaybeOrder struct {
	Email string          `db:"email"`
	Total qt.Null[int64]  `db:"total"`
	Note  qt.Null[string] `db:"note"`
}

type mapBase struct {
	ID    int64  `db:"id"`
	Email string `db:"email"`
}

type mapEmbedded struct {
	mapBase
	Tier string `db:"tier"`
}

type mapStringID struct {
	ID    string `db:"id"`
	Email string `db:"email"`
}

type mapFirst struct {
	A int64 `db:"balance"`
	B int64 `db:"id"`
}

func (r *mapFirst) ScanTargets() []any { return []any{&r.A, &r.B} }

func TestMappingCases(t *testing.T) {
	p := pool(t)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		U, O := fx.Users, fx.Orders
		ins := func(email string, balance int64) int64 {
			t.Helper()
			st := render(t, q.InsertInto(U).Row(U.Email.To(email), U.Balance.To(balance)).Returning(U.ID))
			id, err := pgxexec.One[int64](ctx, tx, st)
			if err != nil {
				t.Fatal(err)
			}
			return id
		}
		a := ins("map-a@x.io", 10)
		b := ins("map-b@x.io", 20)
		st := render(t, q.InsertInto(O).Row(O.UserID.To(a), O.Total.To(5), O.Status.To("paid")))
		if _, err := pgxexec.Exec(ctx, tx, st); err != nil {
			t.Fatal(err)
		}
		byEmail := func(emails ...string) q.Stmt {
			return render(t, q.Select(U.ID, U.Email, U.Nickname).From(U).Where(U.Email.In(emails...)).OrderBy(U.ID.Asc()))
		}

		t.Run("by name", func(t *testing.T) {
			rows, err := pgxexec.All[mapUser](ctx, tx, byEmail("map-a@x.io", "map-b@x.io"))
			if err != nil || len(rows) != 2 || rows[0].Email != "map-a@x.io" || rows[0].Nick.Valid {
				t.Fatalf("%+v %v", rows, err)
			}
		})
		t.Run("column without field", func(t *testing.T) {
			type noNick struct {
				ID    int64  `db:"id"`
				Email string `db:"email"`
			}
			_, err := pgxexec.All[noNick](ctx, tx, byEmail("map-a@x.io"))
			if err == nil || !strings.Contains(err.Error(), "nickname") {
				t.Fatalf("want an error naming the column, got %v", err)
			}
			t.Logf("column without field: %v", err)
		})
		t.Run("field without column", func(t *testing.T) {
			type extra struct {
				ID    int64           `db:"id"`
				Email string          `db:"email"`
				Nick  qt.Null[string] `db:"nickname"`
				Posts int64           `db:"posts"`
			}
			_, err := pgxexec.All[extra](ctx, tx, byEmail("map-a@x.io"))
			if err == nil || !strings.Contains(err.Error(), "posts") {
				t.Fatalf("want an error naming the field, got %v", err)
			}
			t.Logf("field without column: %v", err)
		})
		t.Run("go field name is not snake_case", func(t *testing.T) {
			type bad struct {
				ID       int64
				Email    string
				Nickname qt.Null[string]
			}
			rows, err := pgxexec.All[bad](ctx, tx, byEmail("map-a@x.io"))
			if err != nil || len(rows) != 1 || rows[0].Email != "map-a@x.io" {
				t.Fatalf("single-word names match lowercased: %+v %v", rows, err)
			}
			type bad2 struct {
				UserID int64
			}
			_, err = pgxexec.All[bad2](ctx, tx, render(t, q.Select(O.UserID).From(O)))
			if err == nil {
				t.Fatal("UserID must not match user_id")
			}
			t.Logf("UserID versus user_id: %v", err)
		})
		t.Run("embedded struct is flattened", func(t *testing.T) {
			sel := render(t, q.Select(U.ID, U.Email, U.Tier).From(U).Where(U.ID.Eq(a)))
			r, err := pgxexec.One[mapEmbedded](ctx, tx, sel)
			if err != nil || r.ID != a || r.Tier != "free" {
				t.Fatalf("%+v %v", r, err)
			}
		})
		t.Run("tuple and scalar", func(t *testing.T) {
			tu, err := pgxexec.One[q.Tuple2[int64, string]](ctx, tx, render(t, q.Select(U.ID, U.Email).From(U).Where(U.ID.Eq(a))))
			if err != nil || tu.V1 != a || tu.V2 != "map-a@x.io" {
				t.Fatalf("%+v %v", tu, err)
			}
			n, err := pgxexec.One[int64](ctx, tx, render(t, q.Select(q.CountAll()).From(U).Where(U.ID.In(a, b))))
			if err != nil || n != 2 {
				t.Fatalf("%d %v", n, err)
			}
			_, err = pgxexec.One[q.Tuple2[int64, string]](ctx, tx, render(t, q.Select(U.ID).From(U).Where(U.ID.Eq(a))))
			if err == nil {
				t.Fatal("two targets, one column must fail")
			}
			t.Logf("tuple count mismatch: %v", err)
			_, err = pgxexec.One[int64](ctx, tx, render(t, q.Select(U.ID, U.Email).From(U).Where(U.ID.Eq(a))))
			if err == nil {
				t.Fatal("scalar over two columns must fail")
			}
			t.Logf("scalar over two columns: %v", err)
		})
		t.Run("position: same types swapped, no error", func(t *testing.T) {
			// ScanTargets says (balance, id); the select says (id, balance).
			r, err := pgxexec.One[mapFirst](ctx, tx, render(t, q.Select(U.ID, U.Balance).From(U).Where(U.ID.Eq(a))))
			if err != nil || r.A != a || r.B != 10 {
				t.Fatalf("expected swapped values, got %+v %v", r, err)
			}
		})
		t.Run("zero and many rows", func(t *testing.T) {
			_, err := pgxexec.One[mapUser](ctx, tx, byEmail("nobody@x.io"))
			if err == nil {
				t.Fatal("no row")
			}
			t.Logf("One, no row: %v", err)
			_, err = pgxexec.One[mapUser](ctx, tx, byEmail("map-a@x.io", "map-b@x.io"))
			if err == nil {
				t.Fatal("two rows")
			}
			t.Logf("One, two rows: %v", err)
			o, err := pgxexec.Maybe[mapUser](ctx, tx, byEmail("nobody@x.io"))
			if err != nil || o.IsSet() {
				t.Fatalf("%+v %v", o, err)
			}
		})
		t.Run("left join", func(t *testing.T) {
			join := func(item q.Selectable) q.Stmt {
				return render(t, q.Select(U.Email, item).From(U).LeftJoin(O).On(O.UserID.EqX(U.ID)).
					Where(U.ID.In(a, b)).OrderBy(U.ID.Asc()))
			}
			// the declared-NOT-NULL column from the optional side, into a plain field
			_, err := pgxexec.All[mapUserWithOrder](ctx, tx, join(O.Total.As("total")))
			if err == nil {
				t.Fatal("NULL into int64 must fail on the row without an order")
			}
			t.Logf("outer join, plain field: %v", err)
			// q.Nullable changes the Go type of the expression, not the SQL
			rows, err := pgxexec.All[mapUserMaybeOrder](ctx, tx, render(t,
				q.Select(U.Email, q.Nullable(O.Total).As("total"), q.Nullable(O.Status).As("note")).From(U).
					LeftJoin(O).On(O.UserID.EqX(U.ID)).Where(U.ID.In(a, b)).OrderBy(U.ID.Asc())))
			if err != nil || len(rows) != 2 || rows[0].Total != qt.Some(int64(5)) || rows[1].Total.Valid || rows[1].Note.Valid {
				t.Fatalf("%+v %v", rows, err)
			}
			// VerifyRow cannot see the NULL an outer join adds
			conn := tx.Conn()
			if err := pgxexec.VerifyRow[mapUserWithOrder](ctx, conn, join(O.Total.As("total"))); err != nil {
				t.Fatalf("VerifyRow is expected to be silent here: %v", err)
			}
		})
		t.Run("VerifyRow sees a nullable table column", func(t *testing.T) {
			type plainNick struct {
				ID    int64  `db:"id"`
				Email string `db:"email"`
				Nick  string `db:"nickname"`
			}
			conn := tx.Conn()
			err := pgxexec.VerifyRow[plainNick](ctx, conn, byEmail("map-a@x.io"))
			if err == nil {
				t.Fatal("nickname is nullable")
			}
			t.Logf("VerifyRow, nullable into plain: %v", err)
			if err := pgxexec.VerifyRow[plainNick](ctx, conn, byEmail("map-a@x.io"), pgxexec.AllowNullable()); err != nil {
				t.Fatalf("AllowNullable: %v", err)
			}
			if err := pgxexec.VerifyRow[mapUser](ctx, conn, byEmail("map-a@x.io")); err != nil {
				t.Fatalf("correct type: %v", err)
			}
			// a computed column: invisible to the server's description
			type sumRow struct {
				S int64 `db:"s"`
			}
			if err := pgxexec.VerifyRow[sumRow](ctx, conn, render(t, q.Select(q.Max(U.Balance).As("s")).From(U).Where(U.ID.Eq(-1)))); err != nil {
				t.Fatalf("VerifyRow is expected to be silent on a computed column: %v", err)
			}
		})
		t.Run("one to many as one JSON column", func(t *testing.T) {
			type orderItem struct {
				ID    int64 `json:"id"`
				Total int64 `json:"total"`
			}
			type userOrders struct {
				Email  string                         `db:"email"`
				Orders qt.Null[qt.JSONB[[]orderItem]] `db:"orders"`
			}
			item := q.JSONObject(q.JSONKV(q.Val("id"), O.ID), q.JSONKV(q.Val("total"), O.Total)).Expr()
			sel := render(t, q.Select(U.Email, q.JSONArrayAgg(item).Expr().As("orders")).From(U).
				LeftJoin(O).On(O.UserID.EqX(U.ID)).Where(U.ID.In(a, b)).GroupBy(U.ID).OrderBy(U.ID.Asc()))
			t.Logf("SQL: %s", sel.SQL)
			rows, err := pgxexec.All[userOrders](ctx, tx, sel)
			if err != nil || len(rows) != 2 {
				t.Fatalf("%+v %v", rows, err)
			}
			if !rows[0].Orders.Valid || len(rows[0].Orders.V.V) != 1 || rows[0].Orders.V.V[0].Total != 5 {
				t.Errorf("user a: %+v", rows[0])
			}
			// the LEFT JOIN row of a user without orders is aggregated too: one phantom item
			if !rows[1].Orders.Valid || len(rows[1].Orders.V.V) != 1 || rows[1].Orders.V.V[0] != (orderItem{}) {
				t.Errorf("expected one phantom zero item, got %+v", rows[1].Orders)
			}
			// FILTER removes it, and the aggregate is NULL on zero rows
			fsel := render(t, q.Select(U.Email, q.JSONArrayAgg(item).Filter(O.ID.IsNotNull()).Expr().As("orders")).From(U).
				LeftJoin(O).On(O.UserID.EqX(U.ID)).Where(U.ID.In(a, b)).GroupBy(U.ID).OrderBy(U.ID.Asc()))
			frows, err := pgxexec.All[userOrders](ctx, tx, fsel)
			if err != nil || len(frows) != 2 || !frows[0].Orders.Valid || frows[1].Orders.Valid {
				t.Fatalf("with FILTER: %+v %v", frows, err)
			}
		})
		t.Run("two columns with the same name", func(t *testing.T) {
			type dup struct {
				ID    int64  `db:"id"`
				Email string `db:"email"`
			}
			sel := render(t, q.Select(U.ID, U.Email, O.ID).From(U).Join(O).On(O.UserID.EqX(U.ID)).Where(U.ID.Eq(a)))
			_, err := pgxexec.One[dup](ctx, tx, sel)
			if err == nil || !strings.Contains(err.Error(), "two result columns are called id") {
				t.Fatalf("want an error about the repeated column, got %v", err)
			}
			t.Logf("duplicate column name: %v", err)
			// aliased, it reads
			type ok struct {
				ID      int64  `db:"id"`
				Email   string `db:"email"`
				OrderID int64  `db:"order_id"`
			}
			sel = render(t, q.Select(U.ID, U.Email, O.ID.As("order_id")).From(U).Join(O).On(O.UserID.EqX(U.ID)).Where(U.ID.Eq(a)))
			r, err := pgxexec.One[ok](ctx, tx, sel)
			if err != nil || r.ID != a || r.OrderID <= 0 {
				t.Fatalf("%+v %v", r, err)
			}
		})
		t.Run("integer id read into a string field", func(t *testing.T) {
			r, err := pgxexec.One[mapStringID](ctx, tx, render(t, q.Select(U.ID, U.Email).From(U).Where(U.ID.Eq(a))))
			if err != nil {
				t.Fatal(err)
			}
			if r.ID == "" || strings.ContainsAny(r.ID, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
				t.Fatalf("expected the decimal text of the integer, got %q", r.ID)
			}
			t.Logf("integer into string field gives %q: the raw number, not a hashid", r.ID)
		})
	})
}
