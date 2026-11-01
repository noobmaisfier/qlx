package qtest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/fixture"
)

func TestDumpWritesSortedJSON(t *testing.T) {
	qlx.Register("zz_dump_b", qlx.Select(fixture.Users.ID).From(fixture.Users))
	qlx.Register("zz_dump_a", qlx.Select(fixture.Orders.ID).From(fixture.Orders))
	path := filepath.Join(t.TempDir(), "sub", "q.json")
	Dump(t, qlx.Dialect{}, path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var qs []Query
	if err := json.Unmarshal(data, &qs); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, q := range qs {
		names = append(names, q.Name)
		if q.SQL == "" {
			t.Errorf("%s has no SQL", q.Name)
		}
	}
	ia, ib := -1, -1
	for i, n := range names {
		if n == "zz_dump_a" {
			ia = i
		}
		if n == "zz_dump_b" {
			ib = i
		}
	}
	if ia < 0 || ib < 0 || ia > ib {
		t.Fatalf("names %v: want zz_dump_a before zz_dump_b", names)
	}
}

// A registered query built with qlx.NewParam has unbound parameters; it
// must still be snapshotted, dumped and PREPAREd with its $n placeholders.
func TestRegisteredQueriesMayHaveUnboundParams(t *testing.T) {
	id := qlx.NewParam[int64]()
	qlx.Register("zz_param_query", qlx.Select(fixture.Users.Email).From(fixture.Users).Where(fixture.Users.ID.EqX(id)))
	got := Render(t, qlx.Dialect{})
	sql, ok := got["zz_param_query"]
	if !ok || !strings.Contains(sql, "$1") {
		t.Fatalf("param query missing or lost its placeholder: %q (%v)", sql, ok)
	}
	path := filepath.Join(t.TempDir(), "q.json")
	Dump(t, qlx.Dialect{}, path)
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "zz_param_query") {
		t.Fatal("Dump dropped the parameterized query")
	}
}
