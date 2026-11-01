package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSrc(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRowsGeneratesScanTargets(t *testing.T) {
	dir := writeSrc(t, `package p

//qlx:row
type Summary struct {
	ID, N int64
	Email string
	Skip  int `+"`qlx:\"-\"`"+`
}

type Plain struct{ X int }
`)
	var out bytes.Buffer
	if err := cmdRows([]string{"-dir", dir}, &out); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "qlx_rows.go"))
	if !strings.Contains(string(got), "return []any{&r.ID, &r.N, &r.Email}") {
		t.Fatalf("unexpected output:\n%s", got)
	}
	if strings.Contains(string(got), "Plain") {
		t.Fatal("unmarked struct was generated")
	}
	// Idempotent: a second run (which now also parses nothing new) is identical.
	if err := cmdRows([]string{"-dir", dir}, &out); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(filepath.Join(dir, "qlx_rows.go"))
	if !bytes.Equal(got, again) {
		t.Fatal("not idempotent")
	}
}

func TestRowsRejectsAmbiguousShapes(t *testing.T) {
	for name, src := range map[string]string{
		"embedded":  "package p\n\ntype B struct{ X int }\n\n//qlx:row\ntype A struct {\n\tB\n\tY int\n}\n",
		"generic":   "package p\n\n//qlx:row\ntype A[T any] struct{ V T }\n",
		"notstruct": "package p\n\n//qlx:row\ntype A int\n",
	} {
		dir := writeSrc(t, src)
		if err := cmdRows([]string{"-dir", dir}, &bytes.Buffer{}); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestRowsRemovesStaleFile(t *testing.T) {
	dir := writeSrc(t, "package p\n\ntype A struct{ X int }\n")
	stale := filepath.Join(dir, "qlx_rows.go")
	os.WriteFile(stale, []byte("package p\n"), 0o644)
	if err := cmdRows([]string{"-dir", dir}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Fatal("stale file kept")
	}
}
