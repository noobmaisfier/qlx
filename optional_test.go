package qlx_test

import (
	"strings"
	"testing"
	"time"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qt"
)

func TestOptionalPredicates(t *testing.T) {
	sel := func(p ...q.Pred) q.Statement { return q.Select(Users.ID).From(Users).Where(p...) }
	none := qt.Unset[int64]()
	day := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		st   q.Statement
		want string // substring of the SQL; "" means no WHERE at all
		args int
	}{
		{"every filter unset", sel(Users.Balance.GtIf(none), Users.Balance.LtIf(none), Users.Tier.NeIf(qt.Unset[string]()),
			Users.Email.ContainsFoldIf(qt.Unset[string]()), Users.Tier.InIf(qt.Unset[[]string]()), Users.Nickname.IsNullIf(qt.Unset[bool]())), "", 0},
		{"gt", sel(Users.Balance.GtIf(qt.Of[int64](10))), `"users"."balance" > $1`, 1},
		{"gte and lt", sel(Users.Balance.GteIf(qt.Of[int64](1)), Users.Balance.LtIf(qt.Of[int64](9))), `"users"."balance" >= $1 AND "users"."balance" < $2`, 2},
		{"lte", sel(Users.Balance.LteIf(qt.Of[int64](5))), `"users"."balance" <= $1`, 1},
		{"ne", sel(Users.Tier.NeIf(qt.Of("free"))), `"users"."tier" <> $1`, 1},
		{"between, both bounds", sel(Users.Balance.BetweenIf(qt.Of[int64](1), qt.Of[int64](9))), `"users"."balance" >= $1 AND "users"."balance" <= $2`, 2},
		{"between, lower bound only", sel(Users.Balance.BetweenIf(qt.Of[int64](1), none)), `"users"."balance" >= $1`, 1},
		{"between, upper bound only", sel(Users.Balance.BetweenIf(none, qt.Of[int64](9))), `"users"."balance" <= $1`, 1},
		{"between, no bound", sel(Users.Balance.BetweenIf(none, none)), "", 0},
		{"time range", sel(Users.CreatedAt.BetweenIf(qt.Of(day), qt.Unset[time.Time]())), `"users"."created_at" >= $1`, 1},
		{"in", sel(Users.Tier.InIf(qt.Of([]string{"pro", "team"}))), `"users"."tier" = ANY($1)`, 1},
		{"in, set but empty: matches nothing", sel(Users.Tier.InIf(qt.Of([]string{}))), `FALSE`, 0},
		{"contains escapes", sel(Users.Email.ContainsIf(qt.Of("50%_off"))), `"users"."email" LIKE $1`, 1},
		{"contains fold", sel(Users.Email.ContainsFoldIf(qt.Of("ada"))), `"users"."email" ILIKE $1`, 1},
		{"has prefix", sel(Users.Email.HasPrefixIf(qt.Of("ada"))), `"users"."email" LIKE $1`, 1},
		{"like and ilike", sel(Users.Email.LikeIf(qt.Of("a%")), Users.Nickname.ILikeIf(qt.Of("P%"))), `LIKE $1 AND "users"."nickname" ILIKE $2`, 2},
		{"empty search is absent", sel(Users.Email.ContainsFoldIf(qt.Of("")), Users.Email.HasPrefixIf(qt.Of("")), Users.Email.LikeIf(qt.Of(""))), "", 0},
		{"is null", sel(Users.Nickname.IsNullIf(qt.Of(true))), `"users"."nickname" IS NULL`, 0},
		{"is not null", sel(Users.Nickname.IsNullIf(qt.Of(false))), `"users"."nickname" IS NOT NULL`, 0},
		{"When with any predicate", sel(q.When(qt.Of("ada"), Users.Email.StartsWith), q.When(qt.Unset[string](), Users.Email.Regexp)), `"users"."email" ^@ $1`, 1},
		{"in Or, absent ones are skipped", sel(q.Or(Users.Tier.EqIf(qt.Of("pro")), Users.Balance.GtIf(none))), `"users"."tier" = $1`, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, args := renderIf(t, c.st)
			if c.want == "" {
				if strings.Contains(st.SQL, "WHERE") {
					t.Fatalf("want no WHERE, got %s", st.SQL)
				}
			} else if !strings.Contains(st.SQL, c.want) {
				t.Fatalf("got  %s\nwant …%s…", st.SQL, c.want)
			}
			if len(args) != c.args {
				t.Fatalf("args = %d, want %d: %v (%s)", len(args), c.args, args, st.SQL)
			}
		})
	}
	// the text is escaped, not interpreted
	_, args := renderIf(t, sel(Users.Email.ContainsIf(qt.Of(`50%_off\`))))
	if args[0] != `%50\%\_off\\%` {
		t.Fatalf("escaped pattern: %v", args[0])
	}
}
