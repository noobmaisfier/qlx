package pgxexec_test

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	q "github.com/noobmaisfier/qlx"
	fx "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/pgxexec"
	"github.com/noobmaisfier/qlx/qt"
)

// A None in the list of In means NULL and finds the NULL rows, which SQL's own
// "= ANY" with a NULL element never does.
func TestInWithNullElementsAgainstServer(t *testing.T) {
	p := pool(t)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		U := fx.Users
		for i, nick := range []qt.Null[string]{qt.Some("a"), qt.Some("b"), qt.None[string]()} {
			st := render(t, q.InsertInto(U).Row(U.Email.To("ni-"+string(rune('a'+i))), U.Nickname.To(nick)))
			if _, err := pgxexec.Exec(ctx, tx, st); err != nil {
				t.Fatal(err)
			}
		}
		emails := func(p q.Pred) []string {
			t.Helper()
			got, err := pgxexec.All[string](ctx, tx, render(t, q.Select(U.Email).From(U).Where(U.Email.HasPrefix("ni-"), p).OrderBy(U.Email.Asc())))
			if err != nil {
				t.Fatal(err)
			}
			return got
		}
		some, none := qt.Some[string], qt.None[string]
		cases := []struct {
			name string
			p    q.Pred
			want []string
		}{
			{"some and none", U.Nickname.In(some("a"), none()), []string{"ni-a", "ni-c"}},
			{"only none", U.Nickname.In(none()), []string{"ni-c"}},
			{"no none", U.Nickname.In(some("a"), some("b")), []string{"ni-a", "ni-b"}},
			{"not in, with none", U.Nickname.NotAll([]qt.Null[string]{some("a"), none()}), []string{"ni-b"}},
			{"not in, only none", U.Nickname.NotAll([]qt.Null[string]{none()}), []string{"ni-a", "ni-b"}},
			{"not in, no none: NULL rows stay out (SQL rule)", U.Nickname.NotAll([]qt.Null[string]{some("a")}), []string{"ni-b"}},
			{"under Not", q.Not(U.Nickname.In(some("a"), none())), []string{"ni-b"}},
			{"inside an Or", q.Or(U.Nickname.In(some("b"), none()), U.Email.Eq("ni-a")), []string{"ni-a", "ni-b", "ni-c"}},
		}
		for _, c := range cases {
			if got := emails(c.p); !slices.Equal(got, c.want) {
				t.Errorf("%s: got %v, want %v", c.name, got, c.want)
			}
		}
	})
}

// COPY cannot take parameters; InlineArgs writes literals, and the server
// accepts the statements.
func TestCopyInlineArgsAgainstServer(t *testing.T) {
	p := pool(t)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		U := fx.Users
		conn := tx.Conn().PgConn()

		// COPY ... FROM STDIN WHERE: only the rows that satisfy the condition are loaded
		from := render(t, q.CopyIn(U, U.Email).Where(U.Email.HasPrefix("ci-keep")).InlineArgs())
		if _, err := conn.CopyFrom(ctx, strings.NewReader("ci-keep-1\nci-drop-1\nci-keep-2\n"), from.SQL); err != nil {
			t.Fatalf("%s: %v", from.SQL, err)
		}
		got, err := pgxexec.All[string](ctx, tx, render(t, q.Select(U.Email).From(U).Where(U.Email.HasPrefix("ci-")).OrderBy(U.Email.Asc())))
		if err != nil || !slices.Equal(got, []string{"ci-keep-1", "ci-keep-2"}) {
			t.Fatalf("copy from where: %v %v", got, err)
		}

		// COPY (query) TO STDOUT with a string, an int list and a limit
		out := render(t, q.CopyQueryOut(q.Select(U.Email).From(U).
			Where(U.Email.HasPrefix("ci-"), U.Tier.Ne("none"), U.Balance.Gte(-1)).
			OrderBy(U.Email.Asc()).Limit(1)).InlineArgs())
		var buf bytes.Buffer
		if _, err := conn.CopyTo(ctx, &buf, out.SQL); err != nil {
			t.Fatalf("%s: %v", out.SQL, err)
		}
		if buf.String() != "ci-keep-1\n" {
			t.Fatalf("copy to: %q (%s)", buf.String(), out.SQL)
		}
	})
}
