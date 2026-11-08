package qlx_test

// The examples of docs/GUIDE.md. Each `exName` function is shown in the guide
// as written here, with the SQL this test renders for it (docs/guide/gen.py
// assembles them). Because they compile and render, none of them can be wrong.

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/pg"
	"github.com/noobmaisfier/qlx/qext/textsearch"
	"github.com/noobmaisfier/qlx/qt"
)

// ex: select-basic
func exSelectBasic() q.Statement {
	return q.Select(Users.ID, Users.Email).
		From(Users).
		Where(Users.Tier.Eq("pro")).
		OrderBy(Users.CreatedAt.Desc()).
		Limit(20).Offset(40)
}

// ex: where-operators
func exWhereOperators() q.Statement {
	return q.Select(Users.ID).From(Users).Where(
		Users.Balance.Between(100, 1000),
		Users.Email.Like("%@example.com"),
		Users.Nickname.IsNull(),
		Users.Tier.In("pro", "enterprise"),
	)
}

// ex: where-or-not
func exWhereOrNot() q.Statement {
	return q.Select(Users.ID).From(Users).Where(
		q.Or(Users.Tier.Eq("enterprise"), q.And(Users.Tier.Eq("pro"), Users.Balance.Gt(1000))),
		q.Not(Users.Email.ILike("%@spam.test")),
	)
}

// ex: where-any-slice
func exWhereAnySlice() q.Statement {
	ids := []int64{4, 8, 15, 16, 23, 42}
	return q.Select(Users.Email).From(Users).Where(Users.ID.Any(ids))
}

// ex: where-dynamic
func exWhereDynamic() q.Statement {
	tier := "pro"
	createdOn := qt.Of(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) // present: becomes a predicate
	minBalance := qt.Unset[int64]()                                 // absent: no predicate at all

	sel := q.Select(Users.ID).From(Users)
	if tier != "" {
		sel = sel.Where(Users.Tier.Eq(tier))
	}
	return sel.Where(Users.CreatedAt.EqIf(createdOn), Users.Balance.EqIf(minBalance))
}

// ex: compare-expressions
func exCompareExpressions() q.Statement {
	return q.Select(Orders.ID).From(Orders).Join(Users).On(Orders.UserID.EqX(Users.ID)).
		Where(Orders.Total.GtX(Users.Balance))
}

// ex: join-kinds
func exJoinKinds() q.Statement {
	return q.Select(Users.Email, Orders.Total).
		From(Users).
		LeftJoin(Orders).On(Orders.UserID.EqX(Users.ID))
}

// ex: join-fk
func exJoinFK() q.Statement {
	u, o := Users.As("u"), Orders.As("o")
	return q.Select(u.Email, o.Total).From(u).Join(o).OnFK(o.FKOrdersUserIDFkey(u))
}

// ex: join-self
func exJoinSelf() q.Statement {
	a, b := Users.As("a"), Users.As("b")
	return q.Select(a.Email, b.Email).From(a).Join(b).On(q.And(a.Balance.EqX(b.Balance), a.ID.LtX(b.ID)))
}

// ex: aggregates
func exAggregates() q.Statement {
	return q.Select(
		Orders.Status,
		q.CountAll().As("orders"),
		q.SumBig(Orders.Total).As("revenue"),
		q.Avg(Orders.Total).As("average"),
		q.Max(Orders.Total).As("biggest"),
	).From(Orders).GroupBy(Orders.Status).Having(q.CountAll().Gt(10))
}

// ex: aggregate-modifiers
func exAggregateModifiers() q.Statement {
	return q.Select(
		q.CountDistinct(Orders.UserID).As("buyers"),
		q.CountAll().Filter(Orders.Status.Eq("paid")).As("paid"),
		q.StringAgg(Users.Email, ", ").OrderBy(Users.Email.Asc()).As("emails"),
	).From(Orders).Join(Users).On(Orders.UserID.EqX(Users.ID))
}

// ex: rollup
func exRollup() q.Statement {
	return q.Select(Orders.UserID, Orders.Status, q.CountAll().As("n")).
		From(Orders).
		GroupBy(q.Rollup(Orders.UserID, Orders.Status))
}

// ex: window
func exWindow() q.Statement {
	byUser := q.Window().PartitionBy(Orders.UserID).OrderBy(Orders.ID.Asc())
	return q.Select(
		Orders.ID,
		q.RowNumber().Over(byUser).As("nth"),
		q.Lag(Orders.Total).Over(byUser).As("previous"),
		q.SumBig(Orders.Total).Over(byUser.Rows(q.UnboundedPreceding, q.CurrentRow)).As("running"),
	).From(Orders)
}

// ex: cte
func exCTE() q.Statement {
	userID, n := q.NewField[int64]("user_id"), q.NewField[int64]("n")
	counts := q.NewCTE("counts",
		q.Select(Orders.UserID.As("user_id"), q.Count(Orders.ID).As("n")).
			From(Orders).GroupBy(Orders.UserID))
	return q.Select(Users.Email, q.RefNum(counts, n).As("n")).
		With(counts).
		From(Users).
		Join(counts).On(q.RefNum(counts, userID).EqX(Users.ID)).
		Where(q.RefNum(counts, n).Gt(3))
}

// ex: subquery-exists
func exSubqueryExists() q.Statement {
	return q.Select(Users.Email).From(Users).Where(
		q.Exists(q.Select(Orders.ID).From(Orders).Where(Orders.UserID.EqX(Users.ID), Orders.Total.Gt(500))),
	)
}

// ex: subquery-in
func exSubqueryIn() q.Statement {
	return q.Select(Users.Email).From(Users).Where(
		q.InSelect(Users.ID.Expr, q.Select(Orders.UserID).From(Orders).Where(Orders.Status.Eq("refunded"))),
	)
}

// ex: subquery-lateral
func exSubqueryLateral() q.Statement {
	last := q.Lateral(
		q.Select(Orders.ID).From(Orders).Where(Orders.UserID.EqX(Users.ID)).OrderBy(Orders.ID.Desc()).Limit(1),
		"last")
	return q.Select(Users.Email).From(Users).CrossJoin(last)
}

// ex: set-operations
func exSetOperations() q.Statement {
	return q.Select(Users.ID).From(Users).
		Except(q.Select(Orders.UserID).From(Orders)).
		Limit(10)
}

// ex: case-coalesce
func exCaseCoalesce() q.Statement {
	return q.Select(
		q.Coalesce(Users.Nickname, "anonymous").As("name"),
		q.Case[string]().
			When(Users.Balance.Gt(10_000), "gold").
			When(Users.Balance.Gt(1_000), "silver").
			Else("standard").As("level"),
	).From(Users)
}

// ex: arithmetic-functions
func exArithmeticFunctions() q.Statement {
	return q.Select(
		Users.Balance.Add(100).As("plus"),
		Users.Balance.Mul(2).As("double"),
		Users.Email.Upper().As("shout"),
		pg.Length(Users.Email).As("len"),
		q.Greatest(Users.Balance.Neg(), Users.Balance).As("abs_balance"),
	).From(Users)
}

// ex: cast
func exCast() q.Statement {
	return q.Select(q.Cast(Users.ID, q.PgText).As("id_text")).From(Users)
}

// ex: locking
func exLocking() q.Statement {
	return q.Select(Orders.ID).From(Orders).
		Where(Orders.Status.Eq("queued")).
		OrderBy(Orders.ID.Asc()).Limit(10).
		ForUpdate().Of(Orders).SkipLocked()
}

// ex: generate-series
func exGenerateSeries() q.Statement {
	x := q.NewField[int64]("x")
	s := q.GenerateSeries(q.Val[int64](1), q.Val[int64](5)).As("s", x)
	return q.Select(q.RefNum(s, x)).From(s)
}

// ex: insert-row
func exInsertRow() q.Statement {
	return q.InsertInto(Users).
		Row(Users.Email.To("ada@example.com"), Users.Tier.To("pro")).
		Returning(Users.ID, Users.CreatedAt)
}

// ex: insert-rows
func exInsertRows() q.Statement {
	return q.InsertInto(Orders).
		Row(Orders.UserID.To(1), Orders.Total.To(100), Orders.Status.To("new")).
		Row(Orders.UserID.To(2), Orders.Total.To(250), Orders.Status.To("new"))
}

// ex: insert-select
func exInsertSelect() q.Statement {
	return q.InsertInto(Accounts).
		Columns(Accounts.ID, Accounts.Balance).
		FromQuery(q.Select(Users.ID, Users.Balance).From(Users))
}

// ex: upsert
func exUpsert() q.Statement {
	return q.InsertInto(Users).
		Row(Users.Email.To("ada@example.com"), Users.Balance.To(10)).
		OnConflict(Users.Email).
		DoUpdate(Users.Balance.ToX(Users.Balance.Add(10))).
		Returning(Users.ID)
}

// ex: insert-do-nothing
func exInsertDoNothing() q.Statement {
	return q.InsertInto(Users).Row(Users.Email.To("ada@example.com")).OnConflict(Users.Email).DoNothing()
}

// ex: update
func exUpdate() q.Statement {
	return q.Update(Users).
		Set(Users.Balance.ToX(Users.Balance.Sub(5)), Users.Nickname.ToNull()).
		Where(Users.ID.Eq(7)).
		Returning(Users.Balance)
}

// ex: returning-old-new
func exReturningOldNew() q.Statement {
	return q.Update(Users).Set(Users.Balance.ToX(Users.Balance.Add(10))).
		Where(Users.ID.Eq(7)).
		Returning(Users.Balance.Old().As("before"), Users.Balance.New().As("after"))
}

// ex: param-typing
func exParamTyping() q.Statement {
	return q.Select(Users.ID, q.Val(int64(5)).As("five"), q.Val("x").As("label")).
		From(Users).Where(Users.Balance.Gt(100))
}

// ex: left-join-nullable
func exLeftJoinNullable() q.Statement {
	return q.Select(Users.Email, q.Nullable(Orders.Total).As("total")).
		From(Users).LeftJoin(Orders).On(Orders.UserID.EqX(Users.ID))
}

// ex: update-from
func exUpdateFrom() q.Statement {
	return q.Update(Orders).Set(Orders.Status.To("vip")).
		From(Users).
		Where(Orders.UserID.EqX(Users.ID), Users.Tier.Eq("enterprise"))
}

// ex: delete
func exDelete() q.Statement {
	return q.DeleteFrom(Orders).Where(Orders.Status.Eq("cancelled")).Returning(Orders.ID)
}

// ex: delete-using
func exDeleteUsing() q.Statement {
	return q.DeleteFrom(Orders).Using(Users).Where(Orders.UserID.EqX(Users.ID), Users.Balance.Lt(0))
}

// ex: merge
func exMerge() q.Statement {
	return q.Merge(Accounts).
		Using(Users).On(Accounts.ID.EqX(Users.ID)).
		WhenMatched().ThenUpdate(Accounts.Balance.ToX(Users.Balance)).
		WhenNotMatched().ThenInsert(Accounts.ID.ToX(Users.ID), Accounts.Balance.To(0))
}

// ex: explain
func exExplain() q.Statement {
	return q.Select(Users.ID).From(Users).Where(Users.Tier.Eq("pro")).Explain(q.Analyze, q.Buffers)
}

// ex: tagged
func exTagged() q.Statement {
	return q.Tagged("users.by_tier", q.Select(Users.ID).From(Users).Where(Users.Tier.Eq("pro")))
}

// ex: qext-textsearch
func exQextTextsearch() q.Statement {
	doc := textsearch.Document("english",
		textsearch.Weighted(textsearch.A, Users.Email),
		textsearch.Weighted(textsearch.B, Users.Nickname))
	query := textsearch.Web("english", q.Val("ada"))
	return q.Select(Users.ID, textsearch.Rank(doc, query).As("rank")).
		From(Users).
		Where(textsearch.Search(doc, "english", q.Val("ada"))).
		OrderBy(textsearch.Rank(doc, query).Desc()).
		Limit(20)
}

// ex: set-config
func exSetConfig() q.Statement {
	return q.SetConfig("app.tenant", "42", true)
}

var guideExamples = map[string]func() q.Statement{
	"select-basic": exSelectBasic, "where-operators": exWhereOperators, "where-or-not": exWhereOrNot,
	"where-any-slice": exWhereAnySlice, "where-dynamic": exWhereDynamic, "compare-expressions": exCompareExpressions,
	"join-kinds": exJoinKinds, "join-fk": exJoinFK, "join-self": exJoinSelf,
	"aggregates": exAggregates, "aggregate-modifiers": exAggregateModifiers, "rollup": exRollup,
	"window": exWindow, "cte": exCTE,
	"subquery-exists": exSubqueryExists, "subquery-in": exSubqueryIn, "subquery-lateral": exSubqueryLateral,
	"set-operations": exSetOperations, "case-coalesce": exCaseCoalesce, "arithmetic-functions": exArithmeticFunctions,
	"cast": exCast, "locking": exLocking, "generate-series": exGenerateSeries,
	"insert-row": exInsertRow, "insert-rows": exInsertRows, "insert-select": exInsertSelect, "upsert": exUpsert,
	"insert-do-nothing": exInsertDoNothing, "update": exUpdate, "update-from": exUpdateFrom, "param-typing": exParamTyping, "left-join-nullable": exLeftJoinNullable, "returning-old-new": exReturningOldNew,
	"delete": exDelete, "delete-using": exDeleteUsing, "merge": exMerge, "explain": exExplain,
	"tagged": exTagged, "set-config": exSetConfig, "qext-textsearch": exQextTextsearch,
}

// TestGuideExamples renders every example on PostgreSQL 18; with
// QLX_GUIDE_OUT=file it also writes the SQL for docs/guide/gen.py.
func TestGuideExamples(t *testing.T) {
	var out strings.Builder
	for name, f := range guideExamples {
		sql, args, err := q.SQL(q.Dialect{Version: q.PG18}, f())
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		fmt.Fprintf(&out, "=== %s\n%s\n--- %v\n", name, sql, args)
	}
	if p := os.Getenv("QLX_GUIDE_OUT"); p != "" {
		if err := os.WriteFile(p, []byte(out.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
