package qlx_test

import (
	"errors"
	"strings"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

func TestOperatorSymbolsCannotCarrySQL(t *testing.T) {
	bad := []string{"", "||; DROP TABLE users", "a", "--", "+/*", "|| 'x'", "+\n"}
	for _, sym := range bad {
		e := q.BinaryOp[string]("pg_catalog", sym, q.Arg(Users.Email), q.Arg(Users.Email))
		_, _, err := q.SQL(q.Dialect{Version: q.PG19}, q.Select(e).From(Users))
		if !errors.Is(err, q.ErrInvalidIdent) {
			t.Errorf("symbol %q was accepted: %v", sym, err)
		}
	}
	e := q.BinaryOp[string]("pg_catalog", "||", q.Arg(Users.Email), q.Arg(Users.Email))
	sql, _, err := q.SQL(q.Dialect{Version: q.PG19}, q.Select(e).From(Users))
	want := `SELECT ("users"."email" OPERATOR("pg_catalog".||) "users"."email") FROM "public"."users"`
	if err != nil || sql != want {
		t.Fatalf("got %q, %v", sql, err)
	}
}

// Built-ins that a later major removed carry an upper bound: rendering for a
// target at or past it fails the same way a too-new construct does.
func TestRangeGatedBuiltins(t *testing.T) {
	arg := q.Val(int32(1))
	cases := []struct {
		name string
		e    q.Statement
	}{
		{"function", q.Select(q.FuncCallRange[int32](0, 18, "pg_catalog", "gone", arg))},
		{"binary operator", q.Select(q.BinaryOpRange[int32](0, 18, "pg_catalog", "@@@", arg, arg))},
		{"prefix operator", q.Select(q.PrefixOpRange[int32](0, 18, "pg_catalog", "@@@", arg))},
	}
	for _, c := range cases {
		if _, err := q.Render(q.Dialect{Version: q.PG17}, c.e); err != nil {
			t.Errorf("%s must render on 17: %v", c.name, err)
		}
		for _, v := range []int{q.PG18, q.PG19} {
			_, err := q.Render(q.Dialect{Version: v}, c.e)
			if !errors.Is(err, q.ErrUnsupportedByServer) || !strings.Contains(err.Error(), "removed in PostgreSQL 18") {
				t.Errorf("%s on %d: want a removal error, got %v", c.name, v, err)
			}
		}
	}
	// Both bounds at once: [18, 19).
	e := q.Select(q.FuncCallRange[int32](18, 19, "pg_catalog", "blip", arg))
	for v, ok := range map[int]bool{q.PG17: false, q.PG18: true, q.PG19: false} {
		_, err := q.Render(q.Dialect{Version: v}, e)
		if (err == nil) != ok {
			t.Errorf("[18,19) on %d: err=%v want ok=%v", v, err, ok)
		}
	}
}
