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

// A view gets an Insert and a Patch type only for the events it accepts, and a
// Patch field only for a column it lets you write.
func TestUpdatableViewsGetWriteTypes(t *testing.T) {
	s := viewSchema()
	for i := range s.Tables {
		switch s.Tables[i].Name {
		case "rich": // simple view: INSERT, UPDATE, DELETE
			s.Tables[i].Updatable = 28
			s.Tables[i].Columns[0].Updatable = true
			s.Tables[i].Columns[1].Updatable = true
			s.Tables[i].Columns[1].NotNull = true // email: required for an insert
			s.Tables[i].Columns[2].Updatable = false
		case "mrich":
			s.Tables[i].Updatable = 0
		}
	}
	s.Tables = append(s.Tables,
		catalog.Table{Schema: "app", Name: "ins_only", Kind: "v", Updatable: 8, Columns: []catalog.Column{
			{Num: 1, Name: "email", TypeOID: 25, NotNull: true, Updatable: true}}},
		catalog.Table{Schema: "app", Name: "upd_only", Kind: "v", Updatable: 4, Columns: []catalog.Column{
			{Num: 1, Name: "email", TypeOID: 25, Updatable: true}}},
		catalog.Table{Schema: "app", Name: "deleted_only", Kind: "v", Updatable: 16, Columns: []catalog.Column{
			{Num: 1, Name: "email", TypeOID: 25}}})
	r, err := codegen.Generate(s, codegen.Config{Package: "db", Schemas: []string{"app"}, Out: "db"})
	if err != nil {
		t.Fatal(err)
	}
	f := map[string]string{}
	for n, b := range r.Files {
		f[n] = strings.Join(strings.Fields(string(b)), " ")
	}
	has := func(file, sub string) bool { return strings.Contains(f[file], sub) }

	if !has("rich.go", "type RichRowInsert struct") || !has("rich.go", "type RichRowPatch struct") {
		t.Errorf("a simple view gets Insert and Patch:\n%s", f["rich.go"])
	}
	if !has("rich.go", "func NewRichRowInsert(email string)") {
		t.Errorf("email is required for an insert:\n%s", f["rich.go"])
	}
	if has("rich.go", "Note qt.Opt") {
		t.Error("note is not a writable column of the view: no Patch field")
	}
	if !has("rich.go", "Email qt.Opt[string]") {
		t.Error("email is writable: it is a Patch field")
	}
	if has("mrich.go", "Insert struct") || has("mrich.go", "Patch struct") {
		t.Error("a materialized view stays read-only")
	}
	if !has("ins_only.go", "Insert struct") || has("ins_only.go", "Patch struct") {
		t.Error("an insert-only view gets an Insert type and no Patch")
	}
	if has("upd_only.go", "Insert struct") || !has("upd_only.go", "Patch struct") {
		t.Error("an update-only view gets a Patch type and no Insert")
	}
	if has("deleted_only.go", "Insert struct") || has("deleted_only.go", "Patch struct") {
		t.Error("a view that only accepts DELETE has no write types")
	}
}
