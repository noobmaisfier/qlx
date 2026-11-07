// Builders are immutable: a slice passed as xs... must not stay shared with the query.
package qlx_test

import (
	"errors"
	"strings"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

func TestBuildersDoNotAliasCallerSlices(t *testing.T) {
	as := []q.Assignment{Users.Email.To("a@x"), Users.Balance.To(1)}
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
