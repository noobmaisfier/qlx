// Result-type mapping (V018) through Go 1.27 generic methods: the same checks
// as testdata/src/mapping, written client.One[R](ctx, stmt).
package use

import (
	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qt"
	"testdata/src/gm/pgxexec"
)

type UserRow struct {
	ID    int64
	Email string
}

func (r *UserRow) ScanTargets() []any { return []any{&r.ID, &r.Email} } // want ScanTargets:"row ID,Email"

type ByName struct {
	ID    int64
	Email string
}

func methods(c *pgxexec.Client) {
	c.One[UserRow](nil, q.Select(Users.ID, Users.Email).From(Users))                             // fine
	c.One[UserRow](nil, q.Select(Users.ID).From(Users))                                          // want `V018 UserRow scans 2 columns \(its ScanTargets\) but the query selects 1`
	c.One[UserRow](nil, q.Select(Users.Email, Users.ID).From(Users))                             // want `V018 column 1 \(Users.Email\) into UserRow.ID` `V018 column 2 \(Users.ID\) into UserRow.Email: an integer column is read into a string field` `V018 column email \(position 1\) matches field UserRow.Email`
	c.All[UserRow](nil, q.Select(Users.ID).From(Users))                                          // want `V018 UserRow scans 2 columns`
	c.Maybe[ByName](nil, q.Select(Users.ID, Users.Tier).From(Users))                             // want `V018 ByName has no field for column "tier"` `V018 field ByName.Email has no column`
	c.Each[ByName](nil, q.Select(Users.ID, Users.Tier).From(Users))                              // want `V018 ByName has no field for column "tier"` `V018 field ByName.Email has no column`
	c.One[q.Tuple2[int64, string]](nil, q.Select(Users.ID, Users.Nickname).From(Users))          // want `V018 column 2 \(Users.Nickname\) into qlx.Tuple2\[int64, string\].V2: the column can be NULL`
	c.One[q.Tuple2[int64, qt.Null[string]]](nil, q.Select(Users.ID, Users.Nickname).From(Users)) // fine
	c.One[ByName](nil, q.Select(Users.ID, Users.Email).From(Users))                              // fine
}

// A generic method of the query builder (Cast) must not confuse the other
// checks, and its result type is the method's type argument.
func casts(c *pgxexec.Client) {
	c.One[q.Tuple2[int64, float64]](nil, q.Select(Users.ID, Users.Balance.Cast[float64](q.PgFloat8)).From(Users)) // fine
	c.One[q.Tuple2[int64, string]](nil, q.Select(Users.ID, Users.Balance.Cast[float64](q.PgFloat8)).From(Users))  // fine: numbers scan into text
	c.One[q.Tuple2[int64, bool]](nil, q.Select(Users.ID, Users.Balance.Cast[float64](q.PgFloat8)).From(Users))    // want `V018 column 2 .* into qlx.Tuple2\[int64, bool\].V2`
}

// Not an entry point: Exec has no row type.
func exec(c *pgxexec.Client) {
	_ = c.Exec(nil, q.Select(Users.ID).From(Users))
}

// A type parameter is not known here, so nothing is reported.
func generic[R any](c *pgxexec.Client) {
	c.One[R](nil, q.Select(Users.ID).From(Users))
}
