package qlxvet_test

import (
	"bytes"
	"encoding/gob"
	"reflect"
	"testing"

	"github.com/noobmaisfier/qlx/analyzer/qlxvet"
)

func TestTableFactGobIsDeterministic(t *testing.T) {
	fact := &qlxvet.TableFact{
		Name:     "users",
		Required: []string{"email"},
		PK:       []string{"id"},
		Cols: map[string]string{
			"ID": "id", "Email": "email", "Name": "name", "CreatedAt": "created_at",
		},
	}

	encode := func() []byte {
		t.Helper()
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(fact); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}

	first := encode()
	for i := 0; i < 20; i++ {
		if got := encode(); !bytes.Equal(first, got) {
			t.Fatal("TableFact gob encoding is not deterministic")
		}
	}

	var decoded qlxvet.TableFact
	if err := gob.NewDecoder(bytes.NewReader(first)).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fact, &decoded) {
		t.Fatalf("round trip mismatch: got %#v, want %#v", &decoded, fact)
	}
}
