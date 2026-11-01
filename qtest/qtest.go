// Package qtest checks every registered query: that it renders for the target
// dialect, that its SQL has not changed unreviewed, and that a real server
// accepts it. It depends on no driver; the server check takes a function.
package qtest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/noobmaisfier/qlx"
)

// Render renders every registered statement. A statement that fails to render
// is reported through t and left out. Parameters need not be bound: the text
// keeps its $n placeholders, which is exactly what is snapshotted and
// PREPAREd (a query built with qlx.NewParam is the common case).
func Render(t testing.TB, d qlx.Dialect) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, r := range qlx.RegisteredStatements() {
		st, err := qlx.Render(d, r.Stmt)
		sql := st.SQL
		if err != nil {
			t.Errorf("query %q does not render: %v", r.Name, err)
			continue
		}
		out[r.Name] = sql
	}
	return out
}

// Snapshot compares the SQL of every registered query with a file. Set update
// to rewrite the file; review the diff like any other code change. A query
// that appears or disappears is a difference too.
func Snapshot(t testing.TB, d qlx.Dialect, path string, update bool) {
	t.Helper()
	got := Render(t, d)
	names := make([]string, 0, len(got))
	for n := range got {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintf(&b, "-- %s\n%s\n\n", n, got[n])
	}
	if update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (create it by running with update=true)", err)
	}
	if string(want) != b.String() {
		t.Errorf("query SQL differs from %s; if intended, regenerate it and review the diff.\n--- now ---\n%s", path, b.String())
	}
}

// Preparer asks a server to parse and plan one statement without running it
// (PREPARE, or the extended protocol's Parse and Describe). It returns the
// server's error, if any.
type Preparer func(ctx context.Context, name, sql string) error

// VerifyPrepare makes the server accept every registered query. This is the
// check that no amount of type safety replaces: it proves the SQL is valid for
// the schema and server version the tests run against.
func VerifyPrepare(t testing.TB, d qlx.Dialect, p Preparer) {
	t.Helper()
	ctx := context.Background()
	for name, sql := range Render(t, d) {
		if err := p(ctx, name, sql); err != nil {
			t.Errorf("server rejected %q: %v\n  %s", name, err, sql)
		}
	}
}
