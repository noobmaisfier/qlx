package shape

import (
	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

func use(...any) {}

// ---- UPDATE ----

func updates() {
	use(q.Update(Users).Where(Users.ID.Eq(1)))                                                   // want `V012 UPDATE has no assignment`
	use(q.Update(Users).Set().Where(Users.ID.Eq(1)))                                             // want `V012 UPDATE has no assignment`
	use(q.Update(Users).Set(Users.Balance.To(1), Users.Balance.To(2)).Where(Users.ID.Eq(1)))     // want `V013 column Users.Balance is assigned twice in this UPDATE`
	use(q.Update(Users).Set(Users.Balance.To(1)).Set(Users.Balance.To(2)).Where(Users.ID.Eq(1))) // want `V013 column Users.Balance is assigned twice`
	use(q.Update(Users).Set(Users.Balance.To(1), Users.Tier.To("x")).Where(Users.ID.Eq(1)))      // fine
	u := q.Update(Users).Where(Users.ID.Eq(1))                                                   // assigned: may be completed later
	use(u.Set(Users.Balance.To(1)))
}

// ---- INSERT ----

func inserts() {
	use(q.InsertInto(Users))                                                                                                               // want `V012 INSERT has no row`
	use(q.InsertInto(Users).Row())                                                                                                         // want `V012 INSERT has no row` `V008`
	use(q.InsertInto(Users).Row(Users.Email.To("a")).DefaultValues())                                                                      // want `V013 DefaultValues cannot be combined with rows`
	use(q.InsertInto(Users).Row(Users.Email.To("a")).FromQuery(q.Select(Users.Email).From(Users)).Columns(Users.Email))                    // want `V013 INSERT takes either rows or FromQuery`
	use(q.InsertInto(Users).FromQuery(q.Select(Users.Email).From(Users)))                                                                  // want `V012 INSERT ... SELECT needs Columns`
	use(q.InsertInto(Users).Columns(Users.Email, Users.Tier).FromQuery(q.Select(Users.Email).From(Users)))                                 // want `V013 INSERT names 2 columns but the query produces 1`
	use(q.InsertInto(Users).Columns(Users.Email).FromQuery(q.Select(Users.Email).From(Users)))                                             // fine
	use(q.InsertInto(Users).Columns(Users.Email).FromQuery(q.Union(q.Select(Users.Email).From(Users), q.Select(Users.Email).From(Users)))) // fine
	use(q.InsertInto(Users).Row(Users.Email.To("a"), Users.Email.To("b")))                                                                 // want `V013 column Users.Email is assigned twice in this row`
	use(q.InsertInto(Users).Row(Users.Email.To("a")).Row(Users.Tier.To("x")))                                                              // want `V013 rows of one INSERT must assign the same columns` `V008`
	use(q.InsertInto(Users).Row(Users.Email.To("a"), Users.Tier.To("x")).Row(Users.Tier.To("y"), Users.Email.To("b")))                     // fine: same set, other order
	use(q.InsertInto(Users).DefaultValues())                                                                                               // fine for the shape checks (V008 may complain elsewhere)
}

func conflicts() {
	use(q.InsertInto(Users).Row(Users.Email.To("a")).OnConflict().DoUpdate(Users.Tier.To("x")))                                // want `V012 ON CONFLICT DoUpdate needs conflict columns`
	use(q.InsertInto(Users).Row(Users.Email.To("a")).OnConflict(Users.Email).DoUpdate())                                       // want `V012 DoUpdate needs at least one assignment`
	use(q.InsertInto(Users).Row(Users.Email.To("a")).OnConflict(Users.Email).DoUpdate(Users.Tier.To("x"), Users.Tier.To("y"))) // want `V013 column Users.Tier is assigned twice in DoUpdate`
	use(q.InsertInto(Users).Row(Users.Email.To("a")).OnConflict(Users.Email).DoUpdateWhere(Users.ID.Eq(1)))                    // want `V012 DoUpdateWhere needs at least one assignment`
	use(q.InsertInto(Users).Row(Users.Email.To("a")).OnConflict(Users.Email).DoSelect())                                       // want `V012 DoSelect needs a Returning list` `V009 DoSelect requires PostgreSQL 19`
	use(q.InsertInto(Users).Row(Users.Email.To("a")).OnConflict().DoNothing())                                                 // fine: DO NOTHING needs no target
	use(q.InsertInto(Users).Row(Users.Email.To("a")).OnConflict(Users.Email).DoUpdate(Users.Tier.To("x")))                     // fine
}

// ---- MERGE ----

func merges() {
	use(q.Merge(Users).On(Users.ID.EqX(Orders.UserID)).WhenMatched().ThenDelete())                                                     // want `V012 MERGE needs Using`
	use(q.Merge(Users).Using(Orders).WhenMatched().ThenDelete())                                                                       // want `V012 MERGE needs On`
	use(q.Merge(Users).Using(Orders).On(Users.ID.EqX(Orders.UserID)))                                                                  // want `V012 MERGE needs at least one When clause`
	use(q.Merge(Users).Using(Orders).On(Users.ID.EqX(Orders.UserID)).WhenMatched().ThenUpdate())                                       // want `V012 ThenUpdate needs at least one assignment`
	use(q.Merge(Users).Using(Orders).On(Users.ID.EqX(Orders.UserID)).WhenMatched().ThenUpdate(Users.Tier.To("a"), Users.Tier.To("b"))) // want `V013 column Users.Tier is assigned twice in ThenUpdate`
	use(q.Merge(Users).Using(Orders).On(Users.ID.EqX(Orders.UserID)).WhenMatched().ThenDelete())                                       // fine
}

// ---- SELECT ----

func selects() {
	use(q.Select())                                                                                         // want `V012 Select with no item`
	use(q.Select(Users.ID).From(Users).Limit(1).FetchFirst(1))                                              // want `V013 Limit and FetchFirst cannot be combined`
	use(q.Select(Users.ID).From(Users).FetchFirst(1).WithTies())                                            // want `V013 WithTies needs FetchFirst and OrderBy`
	use(q.Select(Users.ID).From(Users).OrderBy(Users.ID.Asc()).FetchFirst(1).WithTies())                    // fine
	use(q.Select(Users.ID).CrossJoin(Orders))                                                               // want `V012 CrossJoin without From` `V006`
	use(q.Select(Users.ID).From(Users).CrossJoin(Orders))                                                   // fine
	use(q.Select(Users.ID).From(Users).ForUpdate().Of(Orders))                                              // want `V016 Of\(Orders\)`
	use(q.Select(Users.ID).From(Users).ForUpdate().Of(Users))                                               // fine
	use(q.Select(Users.ID).From(Users).Join(Orders).On(Orders.UserID.EqX(Users.ID)).ForUpdate().Of(Orders)) // fine
	use(q.Select(Users.Email, q.CountAll()).From(Users).GroupBy(Orders.Status))                             // want `V014 column Users.Email` `V006`
	use(q.Select(Users.Email).From(Users).GroupBy(Users.ID))                                                // fine: primary key grouped
	use(q.Select(Users.Email).From(Users).GroupBy(Users.Email))                                             // fine
}

func setops() {
	use(q.Union(q.Select(Users.ID).From(Users), q.Select(Orders.ID, Orders.Total).From(Orders))) // want `V013 Union operands have 1 and 2 columns`
	use(q.Except(q.Select(Users.ID).From(Users), q.Select(Orders.ID).From(Orders)))              // fine
}

// ---- misc ----

func misc() {
	use(q.Case[string]().Else("x"))                                          // want `V012 CASE needs at least one When`
	use(q.Case[string]().When(Users.ID.Eq(1), "a").Else("x"))                // fine
	use(q.Truncate())                                                        // want `V012 Truncate needs at least one table`
	use(q.Truncate(Users))                                                   // fine
	use(q.NewCTE("c", q.Select(Users.ID).From(Users)).SearchDepthFirst("s")) // want `V017 SearchDepthFirst needs a recursive CTE`
	use(q.XMLConcat())                                                       // want `V012 XMLConcat needs at least one argument`
}

func explain() {
	use(q.Explain(q.Select(Users.ID).From(Users), q.IO))                        // want `V015 EXPLAIN option IO requires Analyze` `V009 EXPLAIN option IO requires PostgreSQL 19`
	use(q.Explain(q.Select(Users.ID).From(Users), q.Analyze, q.IO))             // want `V009 EXPLAIN option IO requires PostgreSQL 19`
	use(q.Explain(q.Select(Users.ID).From(Users), q.Analyze, q.GenericPlan))    // want `V015 EXPLAIN options Analyze and GenericPlan`
	use(q.Select(Users.ID).From(Users).Explain(q.Wal))                          // want `V015 EXPLAIN option Wal requires Analyze`
	use(q.Explain(q.Select(Users.ID).From(Users), q.Analyze, q.Wal, q.Buffers)) // fine
	use(q.Explain(q.Select(Users.ID).From(Users), q.Memory))                    // fine: 17 is below the targets
}

// ---- version gates on methods ----

func gates() {
	use(q.Update(Accounts).Set(Accounts.Balance.To(1)).Where(Accounts.ID.Eq(1)).Returning(Accounts.Balance.Old().As("b"))) // fine at 18
	use(q.Select(q.Uuidv7[string]()).From(Users))                                                                          // fine at 18
}

// ---- correlated subqueries use the outer tables (no V006) ----

var correlated = q.Select(Users.Email).From(Users).Where(
	q.Exists(q.Select(Orders.ID).From(Orders).Where(Orders.UserID.EqX(Users.ID))),
)

var lateral = q.Select(Users.Email).From(Users).LeftJoin(q.Lateral(
	q.Select(Orders.ID).From(Orders).Where(Orders.UserID.EqX(Users.ID)).Limit(1), "last")).On(q.True)

// A LATERAL subquery built before the query that joins it.
func lateralVar() {
	last := q.Lateral(q.Select(Orders.ID).From(Orders).Where(Orders.UserID.EqX(Users.ID)).Limit(1), "last")
	use(last)
}
