package pgcat_test

import (
	"testing"

	"github.com/noobmaisfier/qlx/internal/pgcat"
	"github.com/noobmaisfier/qlx/internal/pgtool"
)

func extract(t *testing.T) *pgcat.Ledger {
	t.Helper()
	srv, err := pgtool.StartEphemeral("")
	if err != nil {
		t.Skipf("no PostgreSQL server binaries: %v", err)
	}
	t.Cleanup(srv.Close)
	l, err := pgcat.Extract(srv.Conn())
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestExtract(t *testing.T) {
	l := extract(t)
	if len(l.Funcs) < 2000 || len(l.Ops) < 500 || len(l.Casts) < 100 {
		t.Fatalf("catalog looks truncated: %d funcs, %d ops, %d casts", len(l.Funcs), len(l.Ops), len(l.Casts))
	}
	find := func(sig string) *pgcat.Fn {
		for i := range l.Funcs {
			if l.Funcs[i].Sig() == sig {
				return &l.Funcs[i]
			}
		}
		return nil
	}
	if f := find("lower(text)"); f == nil || f.Ret != "text" || !f.Strict || f.Volatility != "i" {
		t.Fatalf("lower(text) wrong: %+v", f)
	}
	if f := find("count(any)"); f == nil || f.Kind != "a" {
		t.Fatalf("count(any) wrong: %+v", f)
	}
	if f := find("row_number()"); f == nil || f.Kind != "w" {
		t.Fatalf("row_number() wrong: %+v", f)
	}
}

func TestMergeSince(t *testing.T) {
	a := &pgcat.Ledger{Versions: []int{16}, Funcs: []pgcat.Fn{{Name: "old", Args: []string{}}, {Name: "both", Args: []string{}}}}
	b := &pgcat.Ledger{Versions: []int{18}, Funcs: []pgcat.Fn{{Name: "new", Args: []string{}}, {Name: "both", Args: []string{}}}}
	m := pgcat.Merge(a, b)
	got := map[string]int{}
	for _, f := range m.Funcs {
		got[f.Name] = f.Since
	}
	if got["both"] != 0 || got["old"] != 0 || got["new"] != 18 {
		t.Fatalf("since markers wrong: %v", got)
	}
	// Merging again with a newer ledger must not change older markers.
	c := &pgcat.Ledger{Versions: []int{19}, Funcs: []pgcat.Fn{{Name: "new", Args: []string{}}, {Name: "newer", Args: []string{}}}}
	m2 := pgcat.Merge(m, c)
	got = map[string]int{}
	for _, f := range m2.Funcs {
		got[f.Name] = f.Since
	}
	if got["new"] != 18 || got["newer"] != 19 || got["both"] != 0 {
		t.Fatalf("incremental merge wrong: %v", got)
	}
}
