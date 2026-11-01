package qlx_test

import (
	"strings"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qt"
)

// FuzzRenderNeverPanics feeds arbitrary strings and numbers to every builder
// entry that takes a user value or a name. Rendering must never panic, and a
// user value must never appear in the SQL text: values are bound parameters.
func FuzzRenderNeverPanics(f *testing.F) {
	for _, s := range []string{"", "a", "x'; DROP TABLE users; --", `"`, "\x00", "é", "%_", "$1", "a b", strings.Repeat("z", 300)} {
		f.Add(s, int64(1), true)
	}
	d := q.Dialect{Version: q.PG18}
	f.Fuzz(func(t *testing.T, s string, n int64, flag bool) {
		stmts := []q.Statement{
			q.Select(Users.ID).From(Users).Where(Users.Email.Eq(s)),
			q.Select(Users.ID).From(Users).Where(Users.Email.Like(s)),
			q.Select(Users.ID).From(Users).Where(Users.Email.NotLike(s)),
			q.Select(Users.ID).From(Users).Where(Users.Email.LikeEscape(q.PatLike, s, s)),
			q.Select(Users.ID).From(Users).Where(Users.Balance.BetweenSymmetric(n, n+1)),
			q.Select(Users.ID).From(Users).Limit(n).Offset(n),
			q.InsertInto(Users).Row(Users.Email.To(s)),
			q.Update(Users).Set(Users.Nickname.ToX(q.Val(qt.Some(s)))).Where(Users.ID.Eq(n)),
			q.Select(Users.ID.As(s)).From(Users),
			q.Select(Users.ID).From(Users.As(s)),
			q.Select(Users.Email.Collate(s)).From(Users),
			q.Tagged(s, q.Select(Users.ID).From(Users)),
			q.SetConfig(s, s, flag),
			q.Notify(s, s),
			q.Listen(s),
			q.SetRole(s, flag),
			q.Savepoint(s),
		}
		for i, st := range stmts {
			sql, _, err := q.SQL(d, st)
			if err != nil {
				continue
			}
			// Cases 0-3, 6, 7, 12, 13 carry s as a value.
			valueCase := i <= 3 || i == 6 || i == 7 || i == 12 || i == 13
			if valueCase && len(s) > 3 && strings.ContainsAny(s, "';\"-") && strings.Contains(sql, s) {
				t.Fatalf("case %d: value %q appears in SQL text: %s", i, s, sql)
			}
		}
	})
}
