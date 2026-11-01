package main

import (
	"os"
	"testing"

	"github.com/noobmaisfier/qlx/internal/grammar"
)

// The committed mapping must be internally complete. Comparing it with the
// real gram.y needs network access; CI does that with
// `pgcat grammar -y <REL_18_STABLE url>,<master url>`.
func TestGrammarMapIsComplete(t *testing.T) {
	m, err := grammar.LoadMap("../../ledgers/grammar_map.json")
	if err != nil {
		t.Fatal(err)
	}
	if ps := grammar.Validate(m); len(ps) > 0 {
		t.Fatalf("%d problems, first: %v", len(ps), ps[0])
	}
}

func TestGrammarAgainstSources(t *testing.T) {
	a, b := os.Getenv("QLX_GRAM_18"), os.Getenv("QLX_GRAM_19")
	if a == "" || b == "" {
		t.Skip("set QLX_GRAM_18 and QLX_GRAM_19 to gram.y paths or URLs")
	}
	if err := grammarCmd(a+","+b, "../../ledgers/grammar_map.json", "", "", false, os.Stderr); err != nil {
		t.Fatal(err)
	}
}
