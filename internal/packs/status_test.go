package packs

import (
	"os"
	"path/filepath"
	"testing"
)

// An extraction-only pack must not have a ledger file, and the entry must
// name a real pack. Delivering the ledger means removing the entry.
func TestExtractionOnlyHasNoLedger(t *testing.T) {
	for name := range extractionOnly {
		if _, ok := Lookup(name); !ok {
			t.Errorf("%q is marked extraction-only but is not a pack", name)
		}
		if _, err := os.Stat(filepath.Join("..", "..", "ledgers", name+".json")); err == nil {
			t.Errorf("ledgers/%s.json exists: remove %q from extractionOnly in status.go", name, name)
		}
		if _, err := os.Stat(filepath.Join("..", "..", "pgcat", "ext", name)); err == nil {
			t.Errorf("pgcat/ext/%s exists but the pack is marked extraction-only", name)
		}
	}
	if ExtractionOnly("pgvector") || ExtractionOnly("postgis") {
		t.Error("pgvector and postgis both have committed ledgers and packages")
	}
}

// The PostGIS types stay mapped now that the wrappers are generated.
func TestPostGISTypesMapped(t *testing.T) {
	p, ok := Lookup("postgis")
	if !ok || len(p.Types) != 9 || p.Types[0].Expr != "qt.Geometry" || p.Types[1].Expr != "qt.Geography" {
		t.Fatalf("postgis pack changed: %+v", p)
	}
}
