package packs

// extractionOnly lists the packs whose types are mapped and whose API can be
// extracted (`pgcat extract -ext NAME`), but for which no ledger and no
// generated package are committed yet. A ledger must come from a real server
// with the extension installed; nobody writes one by hand.
//
// When ledgers/NAME.json lands, remove the entry: TestExtractionOnlyHasNoLedger
// fails while both exist.
var extractionOnly = map[string]bool{
	// Empty: every pack with a committed mapping has a ledger and a generated
	// package. PostGIS moved out when ledgers/postgis.json landed.
}

// ExtractionOnly reports whether the pack has no committed ledger yet.
func ExtractionOnly(name string) bool { return extractionOnly[name] }
