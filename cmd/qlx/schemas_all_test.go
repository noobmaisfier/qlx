package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/noobmaisfier/qlx/internal/pgtool"
)

const allSchemasMigration = `
CREATE TABLE public.users (id bigint PRIMARY KEY, email text NOT NULL);
CREATE SCHEMA billing;
CREATE TABLE billing.users (id bigint PRIMARY KEY, plan text NOT NULL);
CREATE SCHEMA audit;
CREATE TABLE audit.log (id bigint PRIMARY KEY, what text);
CREATE SCHEMA nothing_here;
`

// generatedSpecSchemas returns the schemas the generated package says it was built from.
func generatedSpecSchemas(t *testing.T, dir string) string {
	t.Helper()
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		data, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		if _, rest, ok := strings.Cut(string(data), "Schemas: []string{"); ok {
			line, _, _ := strings.Cut(rest, "}")
			return line
		}
	}
	t.Fatal("no Spec in the generated package")
	return ""
}

func TestSchemasStarMeansEveryNonEmptySchema(t *testing.T) {
	if _, err := pgtool.FindBin(); err != nil {
		t.Skipf("no PostgreSQL server binaries: %v", err)
	}
	dir := t.TempDir()
	mig := filepath.Join(dir, "migrations")
	os.MkdirAll(mig, 0o755)
	os.WriteFile(filepath.Join(mig, "001.sql"), []byte(allSchemasMigration), 0o644)
	cfg := filepath.Join(dir, "qlx.yaml")
	exec := func(args ...string) (int, string) {
		var out, errb bytes.Buffer
		code := run(args, &out, &errb)
		return code, errb.String()
	}

	// The default is still public only.
	os.WriteFile(cfg, []byte("package: db\nout: db\nmigrations: migrations\n"), 0o644)
	if code, msg := exec("gen", "-config", cfg); code != 0 {
		t.Fatalf("default gen: %s", msg)
	}
	if got := generatedSpecSchemas(t, filepath.Join(dir, "db")); got != `"public"` {
		t.Fatalf("default schemas = %s, want only public", got)
	}

	// "*": every schema that holds something, none that is empty.
	os.WriteFile(cfg, []byte("package: db\nout: db\nmigrations: migrations\nschemas: ['*']\n"), 0o644)
	if code, msg := exec("gen", "-config", cfg); code != 0 {
		t.Fatalf("gen with *: %s", msg)
	}
	if got := generatedSpecSchemas(t, filepath.Join(dir, "db")); got != `"public", "audit", "billing"` {
		t.Fatalf("schemas = %s", got)
	}
	// Both tables named users exist, told apart by their schema.
	all := ""
	ents, _ := os.ReadDir(filepath.Join(dir, "db"))
	for _, e := range ents {
		b, _ := os.ReadFile(filepath.Join(dir, "db", e.Name()))
		all += string(b)
	}
	for _, want := range []string{`Schema: "billing", Name: "users"`, `Schema: "public", Name: "users"`, `Schema: "audit", Name: "log"`} {
		if !strings.Contains(all, want) {
			t.Errorf("generated code lacks %s", want)
		}
	}
	if strings.Contains(all, "nothing_here") {
		t.Error("an empty schema was generated")
	}
	if code, msg := exec("check", "-config", cfg); code != 0 {
		t.Fatalf("check after gen: %s", msg)
	}

	// The principal schema (public) keeps plain names; the others are always prefixed.
	for _, want := range []string{
		"// UsersT is the table public.users.", "// BillingUsersT is the table billing.users.", "// AuditLogT is the table audit.log.",
		"type User struct", "type BillingUser struct", "type AuditLogRow struct",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("generated code lacks %q", want)
		}
	}

	// Adding audit.users later must not change what Users means.
	os.WriteFile(filepath.Join(mig, "002.sql"), []byte("CREATE TABLE audit.users (id bigint PRIMARY KEY, email text);\n"), 0o644)
	if code, msg := exec("gen", "-config", cfg); code != 0 {
		t.Fatalf("gen after adding audit.users: %s", msg)
	}
	all = ""
	ents, _ = os.ReadDir(filepath.Join(dir, "db"))
	for _, e := range ents {
		b, _ := os.ReadFile(filepath.Join(dir, "db", e.Name()))
		all += string(b)
	}
	for _, want := range []string{"// UsersT is the table public.users.", "// AuditUsersT is the table audit.users."} {
		if !strings.Contains(all, want) {
			t.Errorf("after adding audit.users, generated code lacks %q", want)
		}
	}
	os.Remove(filepath.Join(mig, "002.sql"))
	if code, msg := exec("gen", "-config", cfg); code != 0 {
		t.Fatalf("gen after removing it: %s", msg)
	}

	// -from-lock works with no source at all, and still knows the schemas.
	os.WriteFile(cfg, []byte("package: db\nout: db\nschemas: ['*']\n"), 0o644)
	if code, msg := exec("gen", "-config", cfg, "-from-lock"); code != 0 {
		t.Fatalf("gen -from-lock: %s", msg)
	}
	if code, msg := exec("check", "-config", cfg); code != 0 {
		t.Fatalf("check from lock: %s", msg)
	}

	// exclude_schemas removes one.
	os.WriteFile(cfg, []byte("package: db\nout: db\nmigrations: migrations\nschemas: ['*']\nexclude_schemas: [audit]\n"), 0o644)
	if code, msg := exec("gen", "-config", cfg); code != 0 {
		t.Fatalf("gen with exclude_schemas: %s", msg)
	}
	if got := generatedSpecSchemas(t, filepath.Join(dir, "db")); got != `"public", "billing"` {
		t.Fatalf("schemas after exclude = %s", got)
	}
}

func TestSchemasStarConfigErrors(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"star mixed with a name":           "schemas: ['*', public]\n",
		"exclude_schemas without star":     "schemas: [public]\nexclude_schemas: [audit]\n",
		"exclude_schemas with the default": "exclude_schemas: [audit]\n",
		"unknown key next to the new one":  "schemas: ['*']\nexclude_schema: [audit]\n",
	} {
		cfg := filepath.Join(dir, "qlx.yaml")
		os.WriteFile(cfg, []byte(body), 0o644)
		var out, errb bytes.Buffer
		if code := run([]string{"check", "-config", cfg}, &out, &errb); code != 2 {
			t.Errorf("%s: exit %d, want 2 (%s)", name, code, errb.String())
		}
	}
	// Valid forms load.
	cfg := filepath.Join(dir, "ok.yaml")
	os.WriteFile(cfg, []byte("schemas: ['*']\nexclude_schemas: [audit]\n"), 0o644)
	if f, err := loadConfig(cfg); err != nil || len(f.ExcludeSchemas) != 1 {
		t.Errorf("valid config refused: %v", err)
	}
}

// Two tables that end up with the same Go name are an error, not a silent winner.
func TestSchemaNameCollisionIsAnError(t *testing.T) {
	if _, err := pgtool.FindBin(); err != nil {
		t.Skipf("no PostgreSQL server binaries: %v", err)
	}
	dir := t.TempDir()
	mig := filepath.Join(dir, "migrations")
	os.MkdirAll(mig, 0o755)
	os.WriteFile(filepath.Join(mig, "001.sql"), []byte(`
CREATE TABLE public.billing_users (id bigint PRIMARY KEY);
CREATE SCHEMA billing;
CREATE TABLE billing.users (id bigint PRIMARY KEY);
`), 0o644)
	cfg := filepath.Join(dir, "qlx.yaml")
	os.WriteFile(cfg, []byte("package: db\nout: db\nmigrations: migrations\nschemas: [public, billing]\n"), 0o644)
	var out, errb bytes.Buffer
	code := run([]string{"gen", "-config", cfg}, &out, &errb)
	if code == 0 || !strings.Contains(errb.String(), "would both be called BillingUsers") {
		t.Fatalf("collision not reported: code=%d %s", code, errb.String())
	}
	// Leaving one out resolves it.
	os.WriteFile(cfg, []byte("package: db\nout: db\nmigrations: migrations\nschemas: [public, billing]\nexclude: [public.billing_users]\n"), 0o644)
	errb.Reset()
	if code := run([]string{"gen", "-config", cfg}, &out, &errb); code != 0 {
		t.Fatalf("exclude did not resolve it: %s", errb.String())
	}
}
