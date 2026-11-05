package mapping

import (
	"net/netip"
	"time"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qt"
	"testdata/src/pgxexec"
)

// Positional rows, written the way `qlx rows` generates them.
type UserRow struct {
	ID    int64
	Email string
}

func (r *UserRow) ScanTargets() []any { return []any{&r.ID, &r.Email} } // want ScanTargets:"row ID,Email"

type TwoInts struct {
	UserID int64
	ID     int64
}

func (r *TwoInts) ScanTargets() []any { return []any{&r.UserID, &r.ID} } // want ScanTargets:"row UserID,ID"

// By name.
type ByName struct {
	ID    int64
	Email string
}

type ByNameTagged struct {
	ID   int64  `db:"id"`
	Mail string `db:"email"`
}

type Nullable struct {
	Nickname string
}

type NullableOK struct {
	Nickname qt.Null[string]
}

func positional() {
	pgxexec.One[UserRow](nil, nil, q.Select(Users.ID, Users.Email).From(Users))                             // fine
	pgxexec.One[UserRow](nil, nil, q.Select(Users.ID).From(Users))                                          // want `V018 UserRow scans 2 columns \(its ScanTargets\) but the query selects 1`
	pgxexec.One[UserRow](nil, nil, q.Select(Users.ID, Users.Email, Users.Tier).From(Users))                 // want `V018 UserRow scans 2 columns`
	pgxexec.One[UserRow](nil, nil, q.Select(Users.Email, Users.ID).From(Users))                             // want `V018 column 1 \(Users.Email\) into UserRow.ID` `V018 column 2 \(Users.ID\) into UserRow.Email: an integer column is read into a string field` `V018 column email \(position 1\) matches field UserRow.Email`
	pgxexec.One[TwoInts](nil, nil, q.Select(Orders.ID, Orders.UserID).From(Orders))                         // want `V018 column id \(position 1\) matches field TwoInts.ID \(position 2\)`
	pgxexec.One[TwoInts](nil, nil, q.Select(Orders.UserID, Orders.ID).From(Orders))                         // fine
	pgxexec.All[q.Tuple2[int64, string]](nil, nil, q.Select(Users.ID, Users.Email).From(Users))             // fine
	pgxexec.All[q.Tuple2[int64, bool]](nil, nil, q.Select(Users.ID, Users.Balance).From(Users))             // want `V018 column 2 \(Users.Balance\) into qlx.Tuple2\[int64, bool\].V2: int column`
	pgxexec.All[q.Tuple3[int64, string, int64]](nil, nil, q.Select(Users.ID, Users.Email).From(Users))      // want `V018 qlx.Tuple3\[int64, string, int64\] scans 3 columns`
	pgxexec.One[q.Tuple2[int64, string]](nil, nil, q.Select(Users.ID, Users.Nickname).From(Users))          // want `V018 column 2 \(Users.Nickname\) into qlx.Tuple2\[int64, string\].V2: the column can be NULL`
	pgxexec.One[q.Tuple2[int64, qt.Null[string]]](nil, nil, q.Select(Users.ID, Users.Nickname).From(Users)) // fine
	pgxexec.One[q.Tuple2[int64, *string]](nil, nil, q.Select(Users.ID, Users.Nickname).From(Users))         // fine: a pointer holds NULL
	pgxexec.One[q.Tuple2[int64, string]](nil, nil, q.Select(Users.ID, Users.Email.As("mail")).From(Users))  // fine
	pgxexec.One[q.Tuple2[int64, time.Time]](nil, nil, q.Select(Users.ID, Users.CreatedAt).From(Users))      // fine
	pgxexec.One[q.Tuple2[int64, int64]](nil, nil, q.Select(Users.ID, Users.CreatedAt).From(Users))          // want `V018 column 2 \(Users.CreatedAt\) into qlx.Tuple2\[int64, int64\].V2: time column`
}

func byName() {
	pgxexec.One[ByName](nil, nil, q.Select(Users.ID, Users.Email).From(Users))             // fine
	pgxexec.One[ByName](nil, nil, q.Select(Users.ID, Users.Tier).From(Users))              // want `V018 ByName has no field for column "tier"` `V018 field ByName.Email has no column`
	pgxexec.One[ByName](nil, nil, q.Select(Users.ID, Users.Email, Users.Tier).From(Users)) // want `V018 ByName has no field for column "tier"`
	pgxexec.One[ByName](nil, nil, q.Select(Users.ID, Users.Email.As("mail")).From(Users))  // want `V018 ByName has no field for column "mail"` `V018 field ByName.Email has no column`
	pgxexec.One[ByName](nil, nil, q.Select(Users.Email.As("id"), Users.Email).From(Users)) // want `V018 column id into ByName.ID: string column`
	pgxexec.One[ByNameTagged](nil, nil, q.Select(Users.ID, Users.Email).From(Users))       // fine
	pgxexec.One[ByName](nil, nil, q.Select(Users.ID, q.CountAll()).From(Users))            // want `V007`
	pgxexec.One[Nullable](nil, nil, q.Select(Users.Nickname).From(Users))                  // fine: one column may be a scalar
	pgxexec.One[struct {                                                                   // want `V018 column nickname into struct\{ID int64; Nickname string\}.Nickname: the column can be NULL`
		ID       int64
		Nickname string
	}](nil, nil, q.Select(Users.ID, Users.Nickname).From(Users))
	pgxexec.One[struct {
		ID       int64
		Nickname qt.Null[string]
	}](nil, nil, q.Select(Users.ID, Users.Nickname).From(Users)) // fine
}

func scalars() {
	pgxexec.One[int64](nil, nil, q.Select(Users.ID).From(Users))                 // fine
	pgxexec.One[int64](nil, nil, q.Select(Users.ID, Users.Email).From(Users))    // want `V018 int64 is not a struct, so the query must select exactly one column, but it selects 2`
	pgxexec.One[string](nil, nil, q.Select(Users.ID).From(Users))                // want `V018 column Users.ID: an integer column is read into a string field`
	pgxexec.One[int64](nil, nil, q.Select(Users.Email).From(Users))              // want `V018 column Users.Email: string column`
	pgxexec.One[int64](nil, nil, q.Select(Users.Nickname).From(Users))           // want `V018 column Users.Nickname: the column can be NULL`
	pgxexec.One[qt.Null[string]](nil, nil, q.Select(Users.Nickname).From(Users)) // fine
	pgxexec.One[int64](nil, nil, q.Select(q.CountAll()).From(Users))             // fine
	pgxexec.All[[]string](nil, nil, q.Select(Users.Email).From(Users))           // fine: unknown shape is not guessed
}

// Statements held by package-level variables, with Register and Tagged.
var byID = q.Register("user_by_id", q.Tagged("user_by_id", q.Select(Users.ID, Users.Email).From(Users)))

func viaVariable() {
	pgxexec.One[UserRow](nil, nil, byID) // fine
	pgxexec.One[TwoInts](nil, nil, byID) // want `V018 column 2 \(Users.Email\) into TwoInts.ID: string column` `V018 column id \(position 1\)`
}

// Silent when the statement or the result type is not known.
func silent(extra q.Statement, items ...q.Selectable) {
	pgxexec.One[UserRow](nil, nil, extra)
	pgxexec.One[UserRow](nil, nil, q.Select(items...).From(Users))
}

type userID struct{}

func (userID) IDSpace() string { return "users" }

type orderID struct{}

func (orderID) IDSpace() string { return "orders" }

var uid = q.NewCol[qt.ID[userID]](q.TableRef{Schema: "public", Name: "users"}, "id")

// A Select held in a local variable is followed, and may be extended with
// builder methods: none of them changes the selected columns.
func locals(cond bool, other q.SelectQuery) {
	one := q.Select(Users.ID).From(Users)
	pgxexec.One[UserRow](nil, nil, one) // want `V018 UserRow scans 2 columns \(its ScanTargets\) but the query selects 1`

	ok := q.Select(Users.ID, Users.Email).From(Users)
	pgxexec.One[UserRow](nil, nil, ok) // fine

	ext := q.Select(Users.ID, Users.Email).From(Users)
	if cond {
		ext = ext.Where(Users.Balance.Gt(0))
	}
	ext = ext.OrderBy(Users.ID.Asc()).Limit(10)
	pgxexec.One[TwoInts](nil, nil, ext) // want `V018 column 2 \(Users.Email\) into TwoInts.ID: string column` `V018 column id \(position 1\)`

	// reassigned to something else: unknown, silent
	swapped := q.Select(Users.ID).From(Users)
	swapped = other
	pgxexec.One[UserRow](nil, nil, swapped)

	// its address is taken: unknown, silent
	addr := q.Select(Users.ID).From(Users)
	grow(&addr)
	pgxexec.One[UserRow](nil, nil, addr)

	// a set operation changes the type of the variable's value: not followed
	u := q.Select(Users.ID).From(Users)
	pgxexec.One[UserRow](nil, nil, u.Union(q.Select(Users.ID).From(Users)))
}

func grow(*q.SelectQuery) {}

// Two columns with the same name are read into one field by name.
func duplicates() {
	pgxexec.One[ByName](nil, nil, q.Select(Users.ID, Users.Email, Orders.ID).From(Users).Join(Orders).On(Orders.UserID.EqX(Users.ID)))                // want `V018 two columns of the query are called "id" and ByName is read by name`
	pgxexec.One[ByName](nil, nil, q.Select(Users.ID, Users.Email, Orders.ID.As("order_id")).From(Users).Join(Orders).On(Orders.UserID.EqX(Users.ID))) // want `V018 ByName has no field for column "order_id"`
}

// An id column read into a string field gives its decimal text; another
// table's id type is a different space.
func ids() {
	pgxexec.One[string](nil, nil, q.Select(uid).From(Users))         // want `V018 column uid: an integer column is read into a string field`
	pgxexec.One[qt.ID[userID]](nil, nil, q.Select(uid).From(Users))  // fine
	pgxexec.One[qt.ID[orderID]](nil, nil, q.Select(uid).From(Users)) // want `V018 column uid: an id of qt.ID\[mapping.userID\] is read into qt.ID\[mapping.orderID\], the id of another table`
	pgxexec.One[int64](nil, nil, q.Select(uid).From(Users))          // fine: the integer behind the id
}

func generic[R any]() {
	pgxexec.One[R](nil, nil, q.Select(Users.ID).From(Users))
}

// Columns from the optional side of an outer join are NOT NULL in the schema
// but NULL when no row matches.
func outerJoins() {
	on := Orders.UserID.EqX(Users.ID)
	pgxexec.One[q.Tuple2[string, int64]](nil, nil, q.Select(Users.Email, Orders.Total).From(Users).LeftJoin(Orders).On(on))                      // want `V018 column 2 \(Orders.Total\) into qlx.Tuple2\[string, int64\].V2: the column comes from the optional side of an outer join`
	pgxexec.One[q.Tuple2[string, qt.Null[int64]]](nil, nil, q.Select(Users.Email, Orders.Total).From(Users).LeftJoin(Orders).On(on))             // fine
	pgxexec.One[q.Tuple2[string, *int64]](nil, nil, q.Select(Users.Email, Orders.Total).From(Users).LeftJoin(Orders).On(on))                     // fine
	pgxexec.One[q.Tuple2[string, qt.Null[int64]]](nil, nil, q.Select(Users.Email, q.Nullable(Orders.Total)).From(Users).LeftJoin(Orders).On(on)) // fine
	pgxexec.One[q.Tuple2[string, int64]](nil, nil, q.Select(Users.Email, q.Nullable(Orders.Total)).From(Users).LeftJoin(Orders).On(on))          // want `V018 column 2 .* the column can be NULL`
	pgxexec.One[q.Tuple2[int64, string]](nil, nil, q.Select(Users.ID, Users.Email).From(Users).LeftJoin(Orders).On(on))                          // fine: the left side keeps its rows
	pgxexec.One[q.Tuple2[string, int64]](nil, nil, q.Select(Users.Email, Orders.Total).From(Users).Join(Orders).On(on))                          // fine: an inner join
	pgxexec.One[q.Tuple2[string, int64]](nil, nil, q.Select(Users.Email, Orders.Total).From(Users).RightJoin(Orders).On(on))                     // want `V018 column 1 \(Users.Email\)`
	pgxexec.One[q.Tuple2[string, int64]](nil, nil, q.Select(Users.Email, Orders.Total.As("t")).From(Users).FullJoin(Orders).On(on))              // want `V018 column 1 \(Users.Email\)` `V018 column 2 \(Orders.Total.As\("t"\)\)`
}

// What pgx can plan between number classes, and what it cannot.
func classes() {
	pgxexec.One[float64](nil, nil, q.Select(Users.Balance).From(Users))  // fine: an integer into a float
	pgxexec.One[int64](nil, nil, q.Select(Users.Balance).From(Users))    // fine
	pgxexec.One[string](nil, nil, q.Select(Users.Balance).From(Users))   // want `V018 column Users.Balance: an integer column is read into a string field`
	pgxexec.One[string](nil, nil, q.Select(Users.CreatedAt).From(Users)) // want `V018 column Users.CreatedAt: time column`
	pgxexec.One[bool](nil, nil, q.Select(Users.Balance).From(Users))     // want `V018 column Users.Balance: int column`
	pgxexec.One[int64](nil, nil, q.Select(Users.Email).From(Users))      // want `V018 column Users.Email: string column`
}

// Types that carry their own wire format (V018 knows them since the type
// table was extended): the same kind reads, an unrelated one is reported, and
// text-like fields stay silent because the mapping of such a column may be
// text.
var (
	dateCol  q.Expr[qt.Date]
	ivCol    q.Expr[qt.Interval]
	rangeCol q.Expr[qt.Range[int32]]
	inetCol  q.Expr[netip.Addr]
	uuidCol  q.Expr[[16]byte]
	jsonCol  q.Expr[qt.JSONB[map[string]any]]
	tstzCol  q.Expr[qt.Timestamptz]
)

func richTypes() {
	pgxexec.One[qt.Date](nil, nil, q.Select(dateCol))          // fine
	pgxexec.One[time.Time](nil, nil, q.Select(dateCol))        // fine: a date reads into time.Time
	pgxexec.One[string](nil, nil, q.Select(dateCol))           // fine: text is never decided by the type
	pgxexec.One[int64](nil, nil, q.Select(dateCol))            // want `V018 column dateCol: date column`
	pgxexec.One[qt.Interval](nil, nil, q.Select(ivCol))        // fine
	pgxexec.One[time.Time](nil, nil, q.Select(ivCol))          // want `V018 column ivCol: interval column`
	pgxexec.One[int32](nil, nil, q.Select(rangeCol))           // want `V018 column rangeCol: range column`
	pgxexec.One[qt.Range[int32]](nil, nil, q.Select(rangeCol)) // fine
	pgxexec.One[netip.Addr](nil, nil, q.Select(inetCol))       // fine
	pgxexec.One[bool](nil, nil, q.Select(inetCol))             // want `V018 column inetCol: inet column`
	pgxexec.One[[16]byte](nil, nil, q.Select(uuidCol))         // fine
	pgxexec.One[int64](nil, nil, q.Select(uuidCol))            // want `V018 column uuidCol: uuid column`
	pgxexec.One[string](nil, nil, q.Select(uuidCol))           // fine
	pgxexec.One[map[string]any](nil, nil, q.Select(jsonCol))   // fine: JSON decodes into anything
	pgxexec.One[time.Time](nil, nil, q.Select(tstzCol))        // fine
	pgxexec.One[qt.Timestamptz](nil, nil, q.Select(tstzCol))   // fine
	pgxexec.One[qt.Date](nil, nil, q.Select(tstzCol))          // fine: a timestamp reads into a date
	pgxexec.One[qt.Interval](nil, nil, q.Select(tstzCol))      // want `V018 column tstzCol: time column`
}

// Helpers of the same package that are one `return <Select chain>` are
// followed: the call selects what the helper selects, whatever the call site
// adds after it.
func twoColumns() q.SelectQuery { return q.Select(Users.ID, Users.Email).From(Users) }

func withBranch(on bool) q.SelectQuery {
	if on {
		return q.Select(Users.ID).From(Users)
	}
	return q.Select(Users.ID, Users.Email).From(Users)
}

func oneColumn() q.SelectQuery {
	return q.Select(Users.Nickname).From(Users)
}

func helpers() {
	pgxexec.One[UserRow](nil, nil, twoColumns())                                       // fine
	pgxexec.One[UserRow](nil, nil, twoColumns().Where(Users.ID.Eq(1)).Limit(1))        // fine: the call site adds no column
	pgxexec.One[TwoInts](nil, nil, twoColumns())                                       // want `V018 column 2 \(Users.Email\) into TwoInts.ID: string column` `V018 column id \(position 1\) matches field TwoInts.ID`
	pgxexec.One[UserRow](nil, nil, oneColumn())                                        // want `V018 UserRow scans 2 columns \(its ScanTargets\) but the query selects 1`
	pgxexec.One[q.Tuple2[int64, string]](nil, nil, twoColumns().Where(Users.ID.Eq(2))) // fine
	pgxexec.One[UserRow](nil, nil, withBranch(true))                                   // two returns: not followed, silent
}
