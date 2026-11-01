package qlx_test

import (
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

func jsonCtorCases() []goldenCase {
	return []goldenCase{
		{name: "json_object members", stmt: q.Select(q.JSONObject(
			q.JSONKV(q.Val("id"), Users.ID), q.JSONKV(q.Val("email"), Users.Email)).AbsentOnNull().Expr().As("o")).From(Users)},
		{name: "json_array of values", stmt: q.Select(q.JSONArray(Users.ID, Users.Email).NullOnNull().Expr().As("a")).From(Users)},
		{name: "json_array from subquery", stmt: q.Select(q.JSONArrayFrom(q.Select(Orders.ID).From(Orders)).Expr().As("a"))},
		{name: "json_arrayagg ordered with filter", stmt: q.Select(
			q.JSONArrayAgg(Orders.ID).OrderBy(Orders.ID.Desc()).Filter(Orders.Total.Gt(0)).Expr().As("ids")).From(Orders)},
		{name: "json_objectagg", stmt: q.Select(
			q.JSONObjectAgg(q.JSONKV(q.Val("k"), Orders.Total)).WithUniqueKeys().Expr().As("m")).From(Orders)},
		{name: "is json object", stmt: q.Select(Users.ID).From(Users).Where(q.IsJSON(Users.Email, q.JSONObjectKind))},
		{name: "json_scalar", serverMin: 17, stmt: q.Select(q.JSONScalar(Users.Balance).As("s")).From(Users)},
		{name: "json parse", serverMin: 17, stmt: q.Select(q.JSONParse(Users.Email).WithUniqueKeys().Expr().As("j")).From(Users)},
		{name: "json_serialize", serverMin: 17, stmt: q.Select(q.JSONSerialize(q.JSONParse(Users.Email).Expr()).As("t")).From(Users)},
		{name: "json_serialize bytea", serverMin: 17, stmt: q.Select(q.JSONSerializeBytea(q.JSONParse(Users.Email).Expr()).As("t")).From(Users)},
		{name: "current_time", stmt: q.Select(q.CurrentTime().As("a"), q.CurrentTimeP(3).As("b"), q.LocalTime().As("c"), q.LocalTimeP(2).As("d")).From(Users)},
	}
}

func TestJSONCtorGolden(t *testing.T) { checkGolden(t, "jsonctor", jsonCtorCases()) }
