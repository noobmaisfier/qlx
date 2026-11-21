package codegen_test

import (
	"strings"
	"testing"

	"github.com/noobmaisfier/qlx/internal/catalog"
	"github.com/noobmaisfier/qlx/internal/codegen"
)

// PostgreSQL never marks a view column NOT NULL, so without not_null every
// view column is qt.Null[T]. not_null types the listed ones like NOT NULL
// table columns.
func viewSchema() *catalog.Schema {
	return &catalog.Schema{
		Version: 160000,
		Schemas: []string{"app"},
		Types: []catalog.Type{
			{OID: 20, Schema: "pg_catalog", Name: "int8", Kind: "b", Category: "N"},
			{OID: 25, Schema: "pg_catalog", Name: "text", Kind: "b", Category: "S"},
		},
		Tables: []catalog.Table{
			{Schema: "app", Name: "accounts", Kind: "r", PrimaryKey: []string{"id"}, Columns: []catalog.Column{
				{Num: 1, Name: "id", TypeOID: 20, NotNull: true},
				{Num: 2, Name: "email", TypeOID: 25, NotNull: true},
				{Num: 3, Name: "note", TypeOID: 25},
			}},
			{Schema: "app", Name: "rich", Kind: "v", Columns: []catalog.Column{
				{Num: 1, Name: "id", TypeOID: 20},
				{Num: 2, Name: "email", TypeOID: 25},
				{Num: 3, Name: "note", TypeOID: 25},
			}},
			{Schema: "app", Name: "mrich", Kind: "m", Columns: []catalog.Column{
				{Num: 1, Name: "id", TypeOID: 20},
				{Num: 2, Name: "email", TypeOID: 25},
			}},
		},
	}
}

func genViews(t *testing.T, notNull ...string) (map[string]string, error) {
	t.Helper()
	r, err := codegen.Generate(viewSchema(), codegen.Config{Package: "db", Schemas: []string{"app"}, Out: "db", NotNull: notNull})
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for n, b := range r.Files {
		out[n] = strings.Join(strings.Fields(string(b)), " ") // gofmt aligns columns: compare without it
	}
	return out, nil
}

func TestViewColumnsAreNullableByDefault(t *testing.T) {
	f, err := genViews(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ID qt.Null[int64]", "Email qt.Null[string]"} {
		if !strings.Contains(f["rich.go"], want) {
			t.Errorf("rich.go should hold %q:\n%s", want, f["rich.go"])
		}
	}
	if strings.Contains(f["accounts.go"], "qt.Null[int64]") {
		t.Error("a NOT NULL table column must stay plain")
	}
}

func TestNotNullTypesListedViewColumns(t *testing.T) {
	f, err := genViews(t, "rich.id", "app.rich.email", "mrich.id")
	if err != nil {
		t.Fatal(err)
	}
	rich := f["rich.go"]
	if !strings.Contains(rich, "ID int64 `db:") || !strings.Contains(rich, "Email string `db:") {
		t.Errorf("rich.id and rich.email should be plain:\n%s", rich)
	}
	if !strings.Contains(rich, "Note qt.Null[string]") {
		t.Errorf("rich.note was not listed and stays qt.Null:\n%s", rich)
	}
	if m := f["mrich.go"]; !strings.Contains(m, "ID int64 `db:") || !strings.Contains(m, "Email qt.Null[string] `db:") {
		t.Errorf("a materialized view takes not_null too, only for the listed column:\n%s", m)
	}
}

func TestNotNullRefusesWhatItCannotApply(t *testing.T) {
	cases := map[string]string{
		"rich.nope":       "names no view or materialized-view column",
		"ghost.id":        "names no view or materialized-view column",
		"accounts.id":     "names a table column",
		"app.accounts.id": "names a table column",
		"rich":            "names no view or materialized-view column",
	}
	for entry, want := range cases {
		if _, err := genViews(t, entry); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("not_null %q: want an error containing %q, got %v", entry, want, err)
		}
	}
}
