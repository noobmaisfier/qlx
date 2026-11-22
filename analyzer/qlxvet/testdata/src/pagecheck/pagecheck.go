// Cursor pagination (V032, V033).
package pagecheck

import (
	"time"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qpage"
	"testdata/src/pgxexec"
)

type Row struct {
	ID        int64     `db:"id"`
	CreatedAt time.Time `db:"created_at"`
}

var ok = qpage.Sorts[Row](
	qpage.Sort("created", qpage.Desc(Users.CreatedAt)),
	qpage.Tiebreak(qpage.Desc(Users.ID)),
)

var noTiebreak = qpage.Sorts[Row](qpage.Sort("created", qpage.Desc(Users.CreatedAt))) // want `V033 qpage.Sorts without a Tiebreak`

//qlx:ignore the only sort ends with the primary key
var ignored = qpage.Sorts[Row](qpage.Sort("id", qpage.Asc(Users.ID)))

var opts = []qpage.Option{qpage.Sort("created", qpage.Desc(Users.CreatedAt)), qpage.Tiebreak(qpage.Desc(Users.ID))}

var spread = qpage.Sorts[Row](opts...) // the options are not readable here: no finding

func list(c *pgxexec.Client, tx *pgxexec.Tx, w qpage.Window) {
	c.Page(nil, q.Select(Users.ID, Users.CreatedAt).From(Users), ok, nil)
	c.Page(nil, q.Select(Users.ID, Users.CreatedAt).From(Users).Where(Users.Tier.Eq("pro")), ok, nil)

	c.Page(nil, q.Select(Users.ID, Users.CreatedAt).From(Users).OrderBy(Users.ID.Asc()), ok, nil) // want `V032 OrderBy on the SELECT given to Page`
	tx.Page(nil, q.Select(Users.ID, Users.CreatedAt).From(Users).Limit(10), ok, nil)              // want `V032 Limit on the SELECT given to Page`
	c.Page(nil, q.Select(Users.ID, Users.CreatedAt).From(Users).Offset(10), ok, nil)              // want `V032 Offset on the SELECT given to Page`

	_, _ = w.Apply(q.Select(Users.ID, Users.CreatedAt).From(Users))
	_, _ = w.Apply(q.Select(Users.ID, Users.CreatedAt).From(Users).OrderBy(Users.ID.Asc())) // want `V032 OrderBy on the SELECT given to Apply`

	//qlx:ignore a fixed order is wanted here
	c.Page(nil, q.Select(Users.ID, Users.CreatedAt).From(Users).OrderBy(Users.ID.Asc()), ok, nil)

	base := q.Select(Users.ID, Users.CreatedAt).From(Users).OrderBy(Users.ID.Asc())
	c.Page(nil, base, ok, nil) // a SELECT held in a variable is not followed
}
