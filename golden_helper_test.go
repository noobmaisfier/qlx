package qlx_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	q "github.com/noobmaisfier/qlx"
)

// checkGolden renders cases for PG19 and compares them with
// testdata/<name>.golden. Refresh after reviewing a diff with
// go test -run Golden -update
func checkGolden(t *testing.T, name string, cases []goldenCase) {
	t.Helper()
	var b strings.Builder
	for _, c := range cases {
		st, err := q.Render(q.Dialect{Version: q.PG19}, c.stmt)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		fmt.Fprintf(&b, "-- %s\n%s\n\n", c.name, st.SQL)
	}
	path := filepath.Join("testdata", name+".golden")
	if *update {
		os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (create it with -update)", err)
	}
	if string(want) != b.String() {
		t.Errorf("rendered SQL changed:\n%s", b.String())
	}
}
