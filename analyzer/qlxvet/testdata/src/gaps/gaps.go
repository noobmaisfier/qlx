package gaps

import (
	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qt"
)

func use(...any) {}

// ---- GRANT / REVOKE ----

func grants() {
	r := q.NewRole("app")
	use(q.Grant(q.PrivSelect).OnTables(Users).To(r))                      // fine
	use(q.Revoke(q.PrivSelect).OnTables(Users).From(r).Cascade())         // fine
	use(q.Grant().OnTables(Users).To(r))                                  // want `V023 GRANT needs at least one privilege`
	use(q.Grant(q.PrivSelect).To(r))                                      // want `V023 GRANT needs OnTables`
	use(q.Grant(q.PrivSelect).OnTables(Users))                            // want `V023 GRANT needs To`
	use(q.Revoke(q.PrivSelect).OnTables(Users))                           // want `V023 REVOKE needs From`
	use(q.Grant(q.PrivSelect).OnTables(Users).From(r))                    // want `V023 GRANT names its roles with To, not From`
	use(q.Revoke(q.PrivSelect).OnTables(Users).To(r))                     // want `V023 REVOKE names its roles with From, not To`
	use(q.Revoke(q.PrivSelect).OnTables(Users).From(r).WithGrantOption()) // want `V023 WithGrantOption applies to GRANT only`
	use(q.Grant(q.PrivSelect).OnTables(Users).To(r).Cascade())            // want `V023 Cascade applies to REVOKE only`
	use(q.Grant(q.PrivSelect).OnTables().To(r))                           // want `V023 GRANT OnTables has no argument`
	g := q.Grant(q.PrivSelect).OnTables(Users)                            // assigned: may be completed later
	use(g.To(r))
}

// ---- REFRESH, SET CONSTRAINTS ----

func commands() {
	use(q.RefreshMaterializedView("public", "mv").Concurrently())              // fine
	use(q.RefreshMaterializedView("public", "mv").Concurrently().WithNoData()) // want `V024 REFRESH MATERIALIZED VIEW cannot combine`
	use(q.SetConstraints(true))                                                // want `V024 SetConstraints needs at least one constraint name`
	use(q.SetConstraints(true, "fk"))                                          // fine
	use(q.SetConstraintsAll(true))                                             // fine
}

// ---- table functions, JSON_TABLE, XMLTABLE ----

func tables(doc q.Operand[qt.RawJSON], x q.Operand[qt.XML], xp q.Operand[string]) {
	use(q.Select(Users.ID).From(q.Func("", "f")))                  // want `V021 Func needs As`
	use(q.Select(Users.ID).From(q.Func("", "f").As("t")))          // fine
	use(q.JSONTable(doc, q.Root(), "t"))                           // want `V021 JSONTable needs at least one column`
	use(q.XMLTable(xp, x, "t"))                                    // want `V021 XMLTable needs at least one column`
	use(q.JSONTable(doc, q.Root(), "", q.JSONColNested(q.Root()))) // want `V021 JSONTable needs a non-empty alias`
}

// ---- JSON ----

func jsonPaths(doc q.Operand[qt.RawJSON], n q.Operand[int64]) {
	use(q.Select(Users.ID).From(Users).Where(q.JSONExists(doc, q.Var("min").Key("a")).Pred()))                                    // want `V022 path variable \$min is not passed`
	use(q.Select(Users.ID).From(Users).Where(q.JSONExists(doc, q.Var("min")).Passing(q.Pass("min", n)).Pred()))                   // fine
	use(q.Select(Users.ID).From(Users).Where(q.JSONExists(doc, q.Var("min")).Passing(q.Pass("min", n), q.Pass("min", n)).Pred())) // want `V013 path variable \$min is passed twice`
	use(q.Select(Users.ID).From(Users).Where(q.JSONExists(doc, q.Var("a")).Passing(q.Pass("b", n)).Pred()))                       // want `V022 path variable \$a is not passed`
	use(q.JSONQuery(doc, q.Root()).WithWrapper().OmitQuotes())                                                                    // want `V022 KeepQuotes/OmitQuotes cannot be combined`
	use(q.JSONQuery(doc, q.Root()).OmitQuotes())                                                                                  // fine
	use(q.JSONObjectAgg(q.JSONPair{}))                                                                                            // want `V012 empty JSONPair`
	use(q.JSONObject(q.JSONKV(Users.Email, Users.ID)))                                                                            // fine
}

// A path or a PASSING item held in a variable assigned once is followed.
func jsonVars(doc q.Operand[qt.RawJSON], n q.Operand[int64], param q.JSONPath) {
	path := q.Var("lo").Key("a")
	pass := q.Pass("lo", n)
	use(q.Select(Users.ID).From(Users).Where(q.JSONExists(doc, path).Pred()))               // want `V022 path variable \$lo is not passed`
	use(q.Select(Users.ID).From(Users).Where(q.JSONExists(doc, path).Passing(pass).Pred())) // fine
	use(q.Select(Users.ID).From(Users).Where(q.JSONExists(doc, param).Pred()))              // fine: a parameter, steps unknown
	moved := q.Var("hi")
	moved = q.Root()
	use(q.Select(Users.ID).From(Users).Where(q.JSONExists(doc, moved).Pred())) // fine: reassigned, silent
}

// ---- CTE SEARCH / CYCLE ----

func ctes() {
	plain := q.NewCTE("p", q.Select(Users.ID).From(Users))
	use(q.Select(Users.ID).From(Users).OrderBy(plain.SearchOrder())) // want `V025 SearchOrder on a CTE built without`
	use(plain.CycleMark())                                           // want `V025 CycleMark on a CTE built without Cycle`
	use(q.NewCTE("p", q.Select(Users.ID).From(Users)).CycleMark())   // want `V025 CycleMark on a CTE built without Cycle`
	r := q.Recursive("r", q.Select(Users.ID).From(Users), func(self q.CTE) q.Query { return q.Select(Users.ID).From(Users) }).SearchDepthFirst("ord", Users.ID)
	use(q.Select(Users.ID).From(Users).OrderBy(r.SearchOrder())) // fine
}

// ---- windows ----

func windows() {
	w := q.NewWindow("w", q.Window().OrderBy(Users.ID.Asc()))
	other := q.NewWindow("o", q.Window().OrderBy(Users.ID.Asc()))
	use(q.Select(q.RowNumber().Over(w.Spec())).From(Users).Windows(w))                                                                 // fine
	use(q.Select(q.RowNumber().Over(w.Spec())).From(Users).Windows(other))                                                             // want `V020 window w is not defined`
	use(q.Select(q.RowNumber().Over(w.Spec())).From(Users))                                                                            // want `V020 window w is not defined`
	use(q.Select(Users.ID).From(Users).Where(q.RowNumber().Over(q.Window().OrderBy(Users.ID.Asc())).Eq(1)))                            // want `V019 window function in WHERE`
	use(q.Select(Users.ID).From(Users).Where(q.Exists(q.Select(q.RowNumber().Over(q.Window().OrderBy(Users.ID.Asc()))).From(Orders)))) // fine: its own query
}

// ---- foreign targets ----

func targets() {
	use(q.InsertInto(Users).Row(Users.Email.To("a")).OnConflict(Orders.ID).DoNothing())                    // want `V026 conflict column Orders.ID belongs to Orders`
	use(q.InsertInto(Users).Row(Users.Email.To("a")).OnConflict(Users.Email).DoUpdate(Orders.Total.To(1))) // want `V026 DoUpdate assigns Orders.Total`
	use(q.Update(Users).Set(Orders.Total.To(1)).Where(Users.ID.Eq(1)))                                     // want `V026 Set assigns Orders.Total` `V006`
	use(q.InsertInto(Users).Row(Orders.Status.To("x")))                                                    // want `V026 Row assigns Orders.Status`
	use(q.Update(Users).Set(Users.Balance.To(1)).Where(Users.ID.Eq(1)))                                    // fine
}

// ---- COPY option versions (targets default 18,19; see app2 for lower targets) ----

func copies() {
	use(q.WithOnError(q.OnErrorSetNull)) // want `V009 COPY option OnErrorSetNull requires PostgreSQL 19`
	use(q.WithOnError(q.OnErrorIgnore))  // fine: 17
	use(q.WithFormat(q.CopyJSON))        // want `V009 COPY option CopyJSON requires PostgreSQL 19`
	use(q.WithFormat(q.CopyCSV))         // fine
}

// ---- helpers that return a builder leave the rest of the chain to the caller ----

func helperTableFunc() q.TableFunc { return q.Func("", "f") } // quiet: the caller names it with As

func helperUpdate() q.UpdateQuery { return q.Update(Users).Set(Users.Tier.To("x")) } // quiet: the caller adds Where

func helperStatement() q.Statement {
	return q.Update(Users).Set(Users.Tier.To("x")) // want `V002 UPDATE chain has neither Where nor All`
}

func helperFuncLit() func() q.Statement {
	return func() q.Statement {
		return q.Select(Users.ID).From(q.Func("", "f")) // want `V021 Func needs As`
	}
}
