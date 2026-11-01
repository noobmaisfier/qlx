package pgcat_test

import (
	"strings"
	"testing"

	"github.com/noobmaisfier/qlx/internal/pgcat"
)

func TestDocsShowVersionRanges(t *testing.T) {
	l := pgcat.Merge(
		&pgcat.Ledger{Versions: []int{16}, Funcs: []pgcat.Fn{
			{Name: "gone", Args: []string{"text"}, Ret: "text", Volatility: "i"},
			{Name: "stay", Args: []string{"text"}, Ret: "text", Volatility: "i"},
		}},
		&pgcat.Ledger{Versions: []int{17}, Funcs: []pgcat.Fn{
			{Name: "stay", Args: []string{"text"}, Ret: "text", Volatility: "i"},
			{Name: "fresh", Args: []string{"text"}, Ret: "text", Volatility: "s"},
		}},
	)
	res, err := pgcat.Generate(l, "pg")
	if err != nil {
		t.Fatal(err)
	}
	files := pgcat.Docs(l, res, "pg")
	all := string(files["functions-g.md"]) + string(files["functions-s.md"]) + string(files["functions-f.md"])
	for _, want := range []string{"`gone(text)` | `text` | up to 16", "`stay(text)` | `text` | all", "`fresh(text)` | `text` | 17+ | stable"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in:\n%s", want, all)
		}
	}
	if !strings.Contains(string(files["README.md"]), "PostgreSQL 16, 17") {
		t.Errorf("index lacks the merged versions:\n%s", files["README.md"])
	}
	// Deterministic.
	again := pgcat.Docs(l, res, "pg")
	for n, b := range files {
		if string(again[n]) != string(b) {
			t.Errorf("%s is not deterministic", n)
		}
	}
}
