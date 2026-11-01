package pgcat_test

import (
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/pg"
	"github.com/noobmaisfier/qlx/pgcat/array"
	"github.com/noobmaisfier/qlx/pgcat/num"
	"github.com/noobmaisfier/qlx/pgcat/text"
	"github.com/noobmaisfier/qlx/qt"
)

func render(t *testing.T, s q.Statement) (string, []any) {
	t.Helper()
	sql, args, err := q.SQL(q.Dialect{Version: q.PG19}, s)
	if err != nil {
		t.Fatal(err)
	}
	return sql, args
}

func TestFunctionWrappersRenderQualifiedCalls(t *testing.T) {
	sql, args := render(t, q.Select(num.AbsInt8(Users.Balance)).From(Users))
	want := `SELECT "pg_catalog"."abs"("users"."balance") FROM "public"."users"`
	if sql != want || len(args) != 0 {
		t.Fatalf("got %q %v", sql, args)
	}
}

func TestNullableVariantTakesNullableColumns(t *testing.T) {
	sql, _ := render(t, q.Select(text.UpperN(Users.Nickname)).From(Users))
	want := `SELECT "pg_catalog"."upper"("users"."nickname") FROM "public"."users"`
	if sql != want {
		t.Fatalf("got %q", sql)
	}
}

func TestOperatorWrapper(t *testing.T) {
	sql, _ := render(t, q.Select(text.OpPipePipeTextText(Users.Email, Users.Email)).From(Users))
	want := `SELECT ("users"."email" OPERATOR("pg_catalog".||) "users"."email") FROM "public"."users"`
	if sql != want {
		t.Fatalf("got %q", sql)
	}
}

func TestParametersArePinnedToTheOverload(t *testing.T) {
	p := q.NewParam[int32]()
	st, err := q.Render(q.Dialect{Version: q.PG19}, q.Select(num.AbsInt4(p)))
	if err != nil {
		t.Fatal(err)
	}
	if st.SQL != `SELECT "pg_catalog"."abs"(CAST($1 AS int4))` {
		t.Fatalf("got %q", st.SQL)
	}
}

func TestPolymorphicWrapperPinsElementType(t *testing.T) {
	a := q.NewParam[[]int32]()
	e := q.NewParam[int32]()
	st, err := q.Render(q.Dialect{Version: q.PG19}, q.Select(array.ArrayAppend[int32](a, e)))
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT "pg_catalog"."array_append"(CAST($1 AS int4[]), CAST($2 AS int4))`
	if st.SQL != want {
		t.Fatalf("got %q", st.SQL)
	}
}

var _ = qt.Some[int]

func TestFacadeReexportsShortNames(t *testing.T) {
	sql, _ := render(t, q.Select(pg.Upper(Users.Email)).From(Users))
	want := `SELECT "pg_catalog"."upper"("users"."email") FROM "public"."users"`
	if sql != want {
		t.Fatalf("got %q", sql)
	}
}
