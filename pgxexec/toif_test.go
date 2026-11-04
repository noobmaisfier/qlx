package pgxexec_test

import (
	"context"

	"github.com/jackc/pgx/v5"
	"testing"

	q "github.com/noobmaisfier/qlx"
	fx "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/pgxexec"
	"github.com/noobmaisfier/qlx/qt"
)

type toifRow struct {
	ID   int64           `db:"id"`
	Tier string          `db:"tier"`
	Nick qt.Null[string] `db:"nickname"`
}

// ToIf and ToOrDefault against a real server: an absent option keeps the
// column's DEFAULT on insert and leaves the column alone on update; an
// explicit NULL is written.
func TestToIfAgainstServer(t *testing.T) {
	p := pool(t)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		U := fx.Users
		ins := func(email string, tier qt.Opt[string], nick qt.Opt[qt.Null[string]]) toifRow {
			t.Helper()
			st := render(t, q.InsertInto(U).Row(U.Email.To(email), U.Tier.ToIf(tier), U.Nickname.ToIf(nick)).Returning(U.ID, U.Tier, U.Nickname))
			r, err := pgxexec.One[toifRow](ctx, tx, st)
			if err != nil {
				t.Fatal(err)
			}
			return r
		}
		a := ins("toif-a@x.io", qt.Unset[string](), qt.Unset[qt.Null[string]]())
		if a.Tier != "free" || a.Nick.Valid {
			t.Errorf("absent options: DEFAULT tier and NULL nickname expected, got %+v", a)
		}
		b := ins("toif-b@x.io", qt.Of("pro"), qt.Of(qt.Some("bee")))
		if b.Tier != "pro" || b.Nick != qt.Some("bee") {
			t.Errorf("set options: %+v", b)
		}
		// ToOrDefault, multi-row: the rows keep the same columns
		st := render(t, q.InsertInto(U).
			Row(U.Email.To("toif-c@x.io"), U.Tier.ToOrDefault(qt.Of("gold"))).
			Row(U.Email.To("toif-d@x.io"), U.Tier.ToOrDefault(qt.Unset[string]())).
			Returning(U.ID, U.Tier, U.Nickname))
		rows, err := pgxexec.All[toifRow](ctx, tx, st)
		if err != nil || len(rows) != 2 || rows[0].Tier != "gold" || rows[1].Tier != "free" {
			t.Errorf("multi-row ToOrDefault: %+v %v", rows, err)
		}
		// update: absent leaves the column, explicit NULL clears it, a value sets it
		upd := func(id int64, tier qt.Opt[string], nick qt.Opt[qt.Null[string]]) toifRow {
			t.Helper()
			st := render(t, q.Update(U).Set(U.Tier.ToIf(tier), U.Nickname.ToIf(nick)).Where(U.ID.Eq(id)).Returning(U.ID, U.Tier, U.Nickname))
			r, err := pgxexec.One[toifRow](ctx, tx, st)
			if err != nil {
				t.Fatal(err)
			}
			return r
		}
		if r := upd(b.ID, qt.Unset[string](), qt.Of(qt.None[string]())); r.Tier != "pro" || r.Nick.Valid {
			t.Errorf("NULL written, tier untouched: %+v", r)
		}
		if r := upd(b.ID, qt.Of("max"), qt.Unset[qt.Null[string]]()); r.Tier != "max" || r.Nick.Valid {
			t.Errorf("tier set, nickname untouched: %+v", r)
		}
	})
}
