package qlx_test

import (
	"errors"
	"github.com/noobmaisfier/qlx/qt"
	"math"
	"strings"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

func jsonCases() []goldenCase {
	// All of these need PostgreSQL 17, so serverMin is 17 on each.
	doc := q.NewParam[qt.RawJSON]()
	minPrice := q.NewParam[int64]()
	items := q.Root().Key("items").AllElements()
	name := q.NewField[string]("name")
	price := q.NewField[int64]("price")
	n := q.NewField[int64]("n")
	tbl := q.JSONTable(doc, items, "jt",
		q.JSONColOrdinality(n),
		q.JSONColValue(name, q.PgText, q.At().Key("name")),
		q.JSONColValue(price, q.PgInt8, q.At().Key("price")))
	cs := []goldenCase{
		{name: "json_exists with passed variable", stmt: q.Select(Users.ID).From(Users).Where(
			q.JSONExists(doc, items.Filter(q.At().Key("price").Gt(q.Var("min")))).Passing(q.Pass("min", minPrice)).FalseOnError().Pred())},
		{name: "json_value returning int", stmt: q.Select(
			q.JSONValue(doc, q.Root().Key("a").Key("b"), q.PgInt4).NullOnError().DefaultOnEmpty(0).Expr().As("v"))},
		{name: "json_query with wrapper", stmt: q.Select(
			q.JSONQuery(doc, items.Key("sku")).WithWrapper().Expr().As("skus"))},
		{name: "json_query without wrapper omit quotes", stmt: q.Select(
			q.JSONQuery(doc, items.Key("sku")).WithoutWrapper().OmitQuotes().Expr().As("skus"))},
		{name: "json_value default is a literal", stmt: q.Select(
			q.JSONValue(doc, q.Root().Key("a"), q.PgText).DefaultOnEmpty("it's a \\ test").DefaultOnError("x").Expr().As("v"))},
		{name: "json_table", stmt: q.Select(q.RefText(tbl, name), q.RefNum(tbl, price)).From(tbl)},
		{name: "filters combine", stmt: q.Select(q.JSONValue(doc, items.Filter(q.PathAnd(
			q.At().Key("price").Gte(10), q.PathNot(q.At().Key("tag").Eq("old")))).Key("sku"), q.PgText).Expr().As("s"))},
	}
	for i := range cs {
		cs[i].serverMin = 17
	}
	return cs
}

func TestJSONGolden(t *testing.T) { checkGolden(t, "json", jsonCases()) }

func TestJSONNeedsPG17(t *testing.T) {
	for _, c := range jsonCases() {
		_, err := q.Render(q.Dialect{Version: q.PG16}, c.stmt)
		if !errors.Is(err, q.ErrUnsupportedByServer) {
			t.Errorf("%s: want ErrUnsupportedByServer on PG16, got %v", c.name, err)
		}
	}
}

func TestJSONPathCannotCarryQuotes(t *testing.T) {
	doc := q.NewParam[qt.RawJSON]()
	evil := `x'); DROP TABLE users; --"\`
	st, err := q.Render(q.Dialect{Version: q.PG19}, q.Select(q.JSONValue(doc, q.Root().Key(evil), q.PgText).Expr().As("v")))
	if err != nil {
		t.Fatal(err)
	}
	// The whole key must stay inside one E'' literal with quotes doubled.
	want := `SELECT JSON_VALUE(CAST($1 AS jsonb), E'$."x''); DROP TABLE users; --\\"\\\\"' RETURNING text) AS "v"`
	if st.SQL != want {
		t.Fatalf("got  %s\nwant %s", st.SQL, want)
	}
}

func TestUnpassedJSONVariableIsRefused(t *testing.T) {
	doc := q.NewParam[qt.RawJSON]()
	p := q.JSONExists(doc, q.Root().Key("a").Filter(q.At().Gt(q.Var("lim")))).Pred()
	_, err := q.Render(q.Dialect{Version: q.PG19}, q.Select(Users.ID).From(Users).Where(p))
	if !errors.Is(err, q.ErrShape) {
		t.Fatalf("want ErrShape, got %v", err)
	}
}

// PostgreSQL refuses KEEP/OMIT QUOTES together with a wrapper; saying so at
// render time beats a server error at run time.
func TestJSONQueryQuotesNeedNoWrapper(t *testing.T) {
	doc := q.NewParam[qt.RawJSON]()
	items := q.Root().Key("items")
	for _, b := range []q.JSONQueryBuilder{
		q.JSONQuery(doc, items).WithWrapper().OmitQuotes(),
		q.JSONQuery(doc, items).WithConditionalWrapper().KeepQuotes(),
	} {
		_, err := q.Render(q.Dialect{Version: q.PG19}, q.Select(b.Expr().As("x")))
		if !errors.Is(err, q.ErrShape) {
			t.Errorf("want ErrShape, got %v", err)
		}
	}
}

// JSON_VALUE ... DEFAULT takes a constant, never a bound parameter.
func TestJSONDefaultIsLiteralNotParameter(t *testing.T) {
	doc := q.NewParam[qt.RawJSON]()
	path := q.Root().Key("a")
	render := func(e q.Statement) (q.Stmt, error) { return q.Render(q.Dialect{Version: q.PG19}, e) }

	st, err := render(q.Select(q.JSONValue(doc, path, q.PgInt4).DefaultOnEmpty(-7).Expr().As("v")))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.SQL, "DEFAULT -7 ON EMPTY") || strings.Contains(st.SQL, "$2") || st.NumParams() != 1 {
		t.Errorf("int default: %s (%d params)", st.SQL, st.NumParams())
	}

	st, err = render(q.Select(q.JSONValue(doc, path, q.PgText).DefaultOnEmpty("it's \\ \"q\"").Expr().As("v")))
	if err != nil || !strings.Contains(st.SQL, `DEFAULT E'it''s \\ "q"' ON EMPTY`) {
		t.Errorf("string default: %v\n%s", err, st.SQL)
	}

	if _, err := render(q.Select(q.JSONValue(doc, path, q.PgInt4).DefaultOnError(0).Expr().As("v"))); err != nil {
		t.Errorf("zero default: %v", err)
	}
	_, err = render(q.Select(q.JSONValue(doc, path, q.PgFloat8).DefaultOnEmpty(math.NaN()).Expr().As("v")))
	if !errors.Is(err, q.ErrParams) {
		t.Errorf("NaN default must be refused, got %v", err)
	}
}
