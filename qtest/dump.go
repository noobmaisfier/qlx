package qtest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/noobmaisfier/qlx"
)

// Query is one rendered registered statement, as `qlx prepare` reads it.
type Query struct {
	Name string `json:"name"`
	SQL  string `json:"sql"`
}

// Dump writes every registered query, rendered for d, to a JSON file that
// `qlx prepare -queries <file>` can check against a server without
// importing your program. Run it from a test (or `go run`) in CI:
//
//	func TestDump(t *testing.T) { qtest.Dump(t, qlx.Dialect{Version: qlx.PG18}, "queries.json") }
func Dump(t testing.TB, d qlx.Dialect, path string) {
	t.Helper()
	got := Render(t, d)
	qs := make([]Query, 0, len(got))
	for n, s := range got {
		qs = append(qs, Query{Name: n, SQL: s})
	}
	sort.Slice(qs, func(i, j int) bool { return qs[i].Name < qs[j].Name })
	data, err := json.MarshalIndent(qs, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
