package miniyaml

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestParseConfigShapedDocument(t *testing.T) {
	src := `
# generated package
package: db        # trailing comment
out: internal/db
schemas: [public, billing]
targets:
  - 18
  - 19
migrations: "db/migrations"
overrides:
  numeric:
    type: github.com/shopspring/decimal.Decimal
    family: num
json_types:
  users.settings: example.com/app/model.Settings
extensions:
  - pgvector
  - name: postgis
    schema: public
strict: true
ratio: 1.5
empty:
`
	got, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"package": "db", "out": "internal/db",
		"schemas":    []any{"public", "billing"},
		"targets":    []any{int64(18), int64(19)},
		"migrations": "db/migrations",
		"overrides":  map[string]any{"numeric": map[string]any{"type": "github.com/shopspring/decimal.Decimal", "family": "num"}},
		"json_types": map[string]any{"users.settings": "example.com/app/model.Settings"},
		"extensions": []any{"pgvector", map[string]any{"name": "postgis", "schema": "public"}},
		"strict":     true, "ratio": 1.5, "empty": nil,
	}
	if !reflect.DeepEqual(got, want) {
		gj, _ := json.MarshalIndent(got, "", " ")
		wj, _ := json.MarshalIndent(want, "", " ")
		t.Fatalf("got\n%s\nwant\n%s", gj, wj)
	}
}

func TestQuotedHashAndColonStayInStrings(t *testing.T) {
	got, err := Parse([]byte("a: \"x # y: z\"\nb: 'it''s'\n"))
	if err != nil {
		t.Fatal(err)
	}
	m := got.(map[string]any)
	if m["a"] != "x # y: z" || m["b"] != "it's" {
		t.Fatalf("%v", m)
	}
}

func TestRejectsWhatItDoesNotUnderstand(t *testing.T) {
	for name, src := range map[string]string{
		"block scalar": "a: |\n  text\n",
		"anchor":       "a: &x 1\n",
		"flow map":     "a: {b: 1}\n",
		"tab indent":   "a:\n\tb: 1\n",
		"dup key":      "a: 1\na: 2\n",
		"bad indent":   "a: 1\n   b: 2\n",
		"two docs":     "a: 1\n---\nb: 2\n",
		"open list":    "a: [1, 2\n",
	} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
