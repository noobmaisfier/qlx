package pgxexec_test

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	q "github.com/noobmaisfier/qlx"
	fx "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/pgxexec"
	"github.com/noobmaisfier/qlx/qt"
)

// A list endpoint's filters against a real server: each parameter may be
// absent, and every combination is a valid statement.
type userFilter struct {
	Tier    qt.Opt[string]
	Tiers   qt.Opt[[]string]
	MinBal  qt.Opt[int64]
	MaxBal  qt.Opt[int64]
	Search  qt.Opt[string]
	HasNick qt.Opt[bool]
}

func (f userFilter) query() q.SelectQuery {
	U := fx.Users
	return q.Select(U.Email).From(U).Where(
		U.Tier.EqIf(f.Tier),
		U.Tier.InIf(f.Tiers),
		U.Balance.BetweenIf(f.MinBal, f.MaxBal),
		U.Email.ContainsFoldIf(f.Search),
		U.Nickname.IsNullIf(f.HasNick),
	).OrderBy(U.Email.Asc())
}

func TestOptionalFiltersAgainstServer(t *testing.T) {
	p := pool(t)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		U := fx.Users
		add := func(email, tier string, balance int64, nick qt.Null[string]) {
			t.Helper()
			st := render(t, q.InsertInto(U).Row(U.Email.To(email), U.Tier.To(tier), U.Balance.To(balance), U.Nickname.To(nick)))
			if _, err := pgxexec.Exec(ctx, tx, st); err != nil {
				t.Fatal(err)
			}
		}
		add("opt-ada@x.io", "pro", 100, qt.Some("ada"))
		add("opt-bob@x.io", "free", 10, qt.None[string]())
		add("opt-cyd@x.io", "team", 50, qt.Some("cyd"))
		add("opt-50%_off@x.io", "free", 70, qt.None[string]())

		run := func(f userFilter) []string {
			t.Helper()
			// only the rows of this test: the others of the table stay out of the way
			st := render(t, f.query().Where(U.Email.HasPrefix("opt-")))
			emails, err := pgxexec.All[string](ctx, tx, st)
			if err != nil {
				t.Fatalf("%+v: %v", f, err)
			}
			return emails
		}
		all := []string{"opt-50%_off@x.io", "opt-ada@x.io", "opt-bob@x.io", "opt-cyd@x.io"}
		cases := []struct {
			name string
			f    userFilter
			want []string
		}{
			{"no filter", userFilter{}, all},
			{"tier", userFilter{Tier: qt.Of("free")}, []string{"opt-50%_off@x.io", "opt-bob@x.io"}},
			{"tiers", userFilter{Tiers: qt.Of([]string{"pro", "team"})}, []string{"opt-ada@x.io", "opt-cyd@x.io"}},
			{"tiers, set but empty", userFilter{Tiers: qt.Of([]string{})}, nil},
			{"lower bound only", userFilter{MinBal: qt.Of[int64](60)}, []string{"opt-50%_off@x.io", "opt-ada@x.io"}},
			{"upper bound only", userFilter{MaxBal: qt.Of[int64](50)}, []string{"opt-bob@x.io", "opt-cyd@x.io"}},
			{"both bounds", userFilter{MinBal: qt.Of[int64](20), MaxBal: qt.Of[int64](80)}, []string{"opt-50%_off@x.io", "opt-cyd@x.io"}},
			{"search ignores case", userFilter{Search: qt.Of("ADA")}, []string{"opt-ada@x.io"}},
			{"search is literal: % and _ are not wildcards", userFilter{Search: qt.Of("50%_off")}, []string{"opt-50%_off@x.io"}},
			{"search that would match everything as a pattern", userFilter{Search: qt.Of("%")}, []string{"opt-50%_off@x.io"}},
			{"empty search filters nothing", userFilter{Search: qt.Of("")}, all},
			{"has no nickname", userFilter{HasNick: qt.Of(true)}, []string{"opt-50%_off@x.io", "opt-bob@x.io"}},
			{"has a nickname", userFilter{HasNick: qt.Of(false)}, []string{"opt-ada@x.io", "opt-cyd@x.io"}},
			{"everything at once", userFilter{Tier: qt.Of("free"), MinBal: qt.Of[int64](20), Search: qt.Of("off"), HasNick: qt.Of(true)}, []string{"opt-50%_off@x.io"}},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				if got := run(c.f); !slices.Equal(got, c.want) {
					t.Fatalf("got %v, want %v", got, c.want)
				}
			})
		}
	})
}
