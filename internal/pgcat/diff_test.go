package pgcat_test

import (
	"strings"
	"testing"

	"github.com/noobmaisfier/qlx/internal/pgcat"
)

func TestDiff(t *testing.T) {
	a := &pgcat.Ledger{
		Funcs: []pgcat.Fn{
			{Name: "gone", Args: []string{"int4"}, Ret: "int4", Volatility: "i"},
			{Name: "same", Args: []string{"int4"}, Ret: "int4", Volatility: "i"},
			{Name: "morph", Args: []string{"int4"}, Ret: "int4", Volatility: "i"},
		},
		Casts: []pgcat.Cast{{From: "a", To: "b", Context: "e"}},
	}
	b := &pgcat.Ledger{
		Funcs: []pgcat.Fn{
			{Name: "same", Args: []string{"int4"}, Ret: "int4", Volatility: "i"},
			{Name: "morph", Args: []string{"int4"}, Ret: "int8", Volatility: "s"},
			{Name: "fresh", Args: []string{"text"}, Ret: "text", Volatility: "i"},
		},
		Casts: []pgcat.Cast{{From: "a", To: "b", Context: "i"}},
	}
	got := pgcat.Diff(a, b)
	for _, want := range []string{"Functions added (1)", "`fresh(text) → text`", "Functions removed (1)", "`gone(int4) → int4`", "Functions changed (1)", "morph(int4)", "Cast context changed", "a->b: e → i"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "same(") {
		t.Errorf("unchanged entry listed:\n%s", got)
	}
	if pgcat.Diff(a, a) != "No differences.\n" {
		t.Error("identical ledgers should report no differences")
	}
}
