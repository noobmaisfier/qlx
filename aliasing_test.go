// Builders are immutable: a slice passed as xs... must not stay shared with the query.
package qlx_test

import (
	"errors"
	"strings"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qt"
)

func TestBuildersDoNotAliasCallerSlices(t *testing.T) {
	as := []q.RowValue{Users.Email.To("a@x"), Users.Balance.To(1)}
	ins := q.InsertInto(Users).Row(as...)
	as[0] = Users.Email.To("MUTATED")
	_, args := renderIf(t, ins)
	if args[0] != "a@x" {
		t.Errorf("INSERT aliases the caller's slice: %v", args)
	}
	sets := []q.Assignment{Users.Balance.To(5)}
	up := q.Update(Users).Set(sets...).Where(Users.ID.Eq(1))
	sets[0] = Users.Balance.To(999)
	_, args = renderIf(t, up)
	if args[0] != int64(5) && args[0] != 5 {
		t.Errorf("UPDATE aliases the caller's slice: %v", args)
	}
	ps := []q.Pred{Users.ID.Eq(1)}
	sel := q.Select(Users.ID).From(Users).Where(ps...)
	ps[0] = Users.ID.Eq(2)
	_, args = renderIf(t, sel)
	if args[0] != int64(1) && args[0] != 1 {
		t.Errorf("WHERE aliases: %v", args)
	}
	// Values from a RowAssigner that returns a shared slice
	shared := []q.Assignment{Users.Email.To("v@x")}
	vi := q.InsertInto(Users).Values(sliceRow{shared})
	shared[0] = Users.Email.To("MUTATED")
	if _, args := renderIf(t, vi); args[0] != "v@x" {
		t.Errorf("Values aliases the RowAssigner's slice: %v", args)
	}
	cols := []q.Selectable{Users.ID}
	s2 := q.Select(cols...).From(Users)
	cols[0] = Users.Email
	st, _ := renderIf(t, s2)
	if !strings.Contains(st.SQL, `"id"`) {
		t.Errorf("SELECT aliases: %s", st.SQL)
	}
	ords := []q.OrderTerm{Users.ID.Asc()}
	s3 := q.Select(Users.ID).From(Users).OrderBy(ords...)
	ords[0] = Users.Email.Desc()
	st, _ = renderIf(t, s3)
	if !strings.Contains(st.SQL, `ORDER BY "users"."id"`) {
		t.Errorf("ORDER BY aliases: %s", st.SQL)
	}
}

func TestMergeDoesNotAliasCallerSlices(t *testing.T) {
	updates := []q.Assignment{Users.Balance.To(5)}
	mg := q.Merge(Users).Using(Orders).
		On(Users.ID.EqX(Orders.UserID)).
		WhenMatched().ThenUpdate(updates...).
		WhenNotMatchedBySource().ThenUpdate(updates...)
	updates[0] = Users.Balance.To(999)
	st, err := q.Render(q.Dialect{Version: q.PG18}, mg)
	if err != nil {
		t.Fatal(err)
	}
	args, err := st.Args()
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 || args[0] != int64(5) || args[1] != int64(5) {
		t.Errorf("MERGE ThenUpdate aliases the caller's slice: %v", args)
	}

	inserts := []q.RowValue{Users.Email.To("a@x"), Users.Balance.To(1)}
	mi := q.Merge(Users).Using(Orders).
		On(Users.ID.EqX(Orders.UserID)).
		WhenNotMatched().ThenInsert(inserts...)
	inserts[0] = Users.Email.To("MUTATED")
	st, err = q.Render(q.Dialect{Version: q.PG18}, mi)
	if err != nil {
		t.Fatal(err)
	}
	args, err = st.Args()
	if err != nil {
		t.Fatal(err)
	}
	if len(args) == 0 || args[0] != "a@x" {
		t.Errorf("MERGE ThenInsert aliases the caller's slice: %v", args)
	}
}

func TestConflictUpdateDoesNotAliasCallerSlices(t *testing.T) {
	for _, c := range []struct {
		name string
		stmt func([]q.Assignment) q.Statement
	}{
		{"DoUpdate", func(sets []q.Assignment) q.Statement {
			return q.InsertInto(Users).Row(Users.Email.To("a")).
				OnConflict(Users.Email).DoUpdate(sets...)
		}},
		{"DoUpdateWhere", func(sets []q.Assignment) q.Statement {
			return q.InsertInto(Users).Row(Users.Email.To("a")).
				OnConflict(Users.Email).DoUpdateWhere(Users.ID.Gt(0), sets...)
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			sets := []q.Assignment{Users.Balance.To(5)}
			stmt := c.stmt(sets)
			sets[0] = Users.Balance.To(999)
			st, err := q.Render(q.Dialect{Version: q.PG18}, stmt)
			if err != nil {
				t.Fatal(err)
			}
			args, err := st.Args()
			if err != nil {
				t.Fatal(err)
			}
			if len(args) < 2 || args[1] != int64(5) {
				t.Errorf("%s aliases the caller's slice: %v", c.name, args)
			}
		})
	}
}

func TestTableSourcesDoNotAliasColumnSlices(t *testing.T) {
	doc := q.NewParam[qt.RawJSON]()
	items := q.Root().Key("items").AllElements()
	name := q.NewField[string]("name")

	jsonCols := []q.JSONColumn{q.JSONColValue(name, q.PgText, q.At().Key("name"))}
	jt := q.JSONTable(doc, items, "jt", jsonCols...)
	jsonCols[0] = q.JSONColValue(name, q.PgText, q.At().Key("MUTATED"))
	st, err := q.Render(q.Dialect{Version: q.PG18}, q.Select(q.RefText(jt, name)).From(jt))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(st.SQL, "MUTATED") {
		t.Errorf("JSONTable aliases the caller's column slice: %s", st.SQL)
	}

	nested := []q.JSONColumn{q.JSONColValue(name, q.PgText, q.At().Key("name"))}
	col := q.JSONColNested(items, nested...)
	nested[0] = q.JSONColValue(name, q.PgText, q.At().Key("MUTATED"))
	jt = q.JSONTable(doc, items, "jt", col)
	st, err = q.Render(q.Dialect{Version: q.PG18}, q.Select(q.RefText(jt, name)).From(jt))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(st.SQL, "MUTATED") {
		t.Errorf("JSONColNested aliases the caller's column slice: %s", st.SQL)
	}

	xmlDoc := q.NewParam[qt.XML]()
	id := q.NewField[int64]("id")
	xmlCols := []q.XMLColumn{q.XMLColValue(id, q.PgInt8)}
	xt := q.XMLTable(q.Val("/rows/row"), xmlDoc, "x", xmlCols...)
	xmlCols[0] = q.XMLColValue(id, q.PgInt8).Path(q.Val("@mutated"))
	st, err = q.Render(q.Dialect{Version: q.PG18}, q.Select(q.RefNum(xt, id)).From(xt))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(st.SQL, "mutated") {
		t.Errorf("XMLTable aliases the caller's column slice: %s", st.SQL)
	}
}

func TestNilRowValueIsAnEmptyRow(t *testing.T) {
	ins := q.InsertInto(Users).Row(nil, Users.Email.To("a@x"), Users.Balance.To(1))
	_, args := renderIf(t, ins)
	if len(args) != 2 || args[0] != "a@x" {
		t.Errorf("nil row value: %v", args)
	}

	if _, err := q.Render(q.Dialect{Version: q.PG18}, q.InsertInto(Users).Row(nil)); !errors.Is(err, q.ErrShape) {
		t.Errorf("INSERT with only a nil row: want ErrShape, got %v", err)
	}

	mg := q.Merge(Users).Using(Orders).
		On(Users.ID.EqX(Orders.UserID)).
		WhenNotMatched().ThenInsert(nil)
	if _, err := q.Render(q.Dialect{Version: q.PG18}, mg); !errors.Is(err, q.ErrShape) {
		t.Errorf("MERGE with only a nil row: want ErrShape, got %v", err)
	}
}

type sliceRow struct{ as []q.Assignment }

func (r sliceRow) Assignments() []q.Assignment { return r.as }

// A later row that assigns one column twice (and so misses another) is an
// error, not a nil dereference at render time.
func TestInsertRowRepeatingAColumnIsAnError(t *testing.T) {
	ins := q.InsertInto(Users).
		Row(Users.Email.To("a"), Users.Balance.To(1)).
		Row(Users.Email.To("b"), Users.Email.To("c"))
	if _, err := q.Render(q.Dialect{Version: q.PG18}, ins); !errors.Is(err, q.ErrShape) {
		t.Fatalf("want ErrShape, got %v", err)
	}
}

func TestRenderNilStatementIsAnError(t *testing.T) {
	if _, err := q.Render(q.Dialect{Version: q.PG18}, nil); !errors.Is(err, q.ErrShape) {
		t.Fatalf("want ErrShape, got %v", err)
	}
}
