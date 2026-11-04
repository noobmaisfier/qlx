package packs

// extractionOnly lists the packs whose types are mapped and whose API can be
// extracted (`pgcat extract -ext NAME`), but for which no ledger and no
// generated package are committed yet. A ledger must come from a real server
// with the extension installed; nobody writes one by hand.
//
// When ledgers/NAME.json lands, remove the entry: TestExtractionOnlyHasNoLedger
// fails while both exist.
var extractionOnly = map[string]bool{
	// The contrib packs: definitions only, until the ledgers are extracted on
	// PostgreSQL 16 to 19 (docs/HANDOFF.md, "Adding a contrib pack").
	"pgcrypto":      true,
	"uuid-ossp":     true,
	"unaccent":      true,
	"fuzzystrmatch": true,
	"btree_gist":    true,
}

// ExtractionOnly reports whether the pack has no committed ledger yet.
func ExtractionOnly(name string) bool { return extractionOnly[name] }
