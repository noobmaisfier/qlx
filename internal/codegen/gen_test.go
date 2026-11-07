package codegen_test

import (
	"github.com/noobmaisfier/qlx/internal/gover"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/noobmaisfier/qlx/internal/catalog"
	"github.com/noobmaisfier/qlx/internal/codegen"
	"github.com/noobmaisfier/qlx/internal/pgtool"
)

func generate(t *testing.T) (*codegen.Result, *catalog.Schema) {
	t.Helper()
	srv, err := pgtool.StartEphemeral("")
	if err != nil {
		t.Skipf("no PostgreSQL server binaries: %v", err)
	}
	t.Cleanup(srv.Close)
	if err := srv.Conn().ExecFile("../../fixture/schema.sql"); err != nil {
		t.Fatal(err)
	}
	raw, err := catalog.Load(srv.Conn(), []string{"public"})
	if err != nil {
		t.Fatal(err)
	}
	_, s, err := catalog.Seal(raw)
	if err != nil {
		t.Fatal(err)
	}
	res, err := codegen.Generate(s, codegen.Config{Package: "gen", Schemas: []string{"public"}})
	if err != nil {
		t.Fatal(err)
	}
	return res, s
}

func TestGeneratedCodeBuilds(t *testing.T) {
	res, _ := generate(t)
	root, _ := filepath.Abs("../..")
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module gentest\n\ngo "+gover.Directive()+"\n\nrequire github.com/noobmaisfier/qlx v0.0.0\n\nreplace github.com/noobmaisfier/qlx => "+root+"\n"), 0o644)
	pkg := filepath.Join(dir, "gen")
	os.MkdirAll(pkg, 0o755)
	for n, src := range res.Files {
		os.WriteFile(filepath.Join(pkg, n), src, 0o644)
	}
	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod", "GOPROXY=off", "GOTOOLCHAIN=local")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated code does not build:\n%s", out)
	}
}

func TestGenerateShape(t *testing.T) {
	res, _ := generate(t)
	for _, want := range []string{"users.go", "orders.go", "members.go", "enums.go", "functions.go", "register.go"} {
		if _, ok := res.Files[want]; !ok {
			t.Errorf("missing %s; have %v", want, keys(res.Files))
		}
	}
	m := string(res.Files["members.go"])
	for _, want := range []string{
		"qlx.Col[Tier]",        // enum column
		"qlx.ArrayCol[string]", // text[]
		"qlx.JSONBCol[qt.Null[qt.JSONB[json.RawMessage]]]",
		"func NewMemberInsert(",
		"Required: []string{",
	} {
		if !strings.Contains(m, want) {
			t.Errorf("members.go lacks %q", want)
		}
	}
	o := string(res.Files["orders.go"])
	if !strings.Contains(o, "func (t OrdersT) FKOrdersUserIDFkey(to UsersT) qlx.ForeignKey") ||
		!strings.Contains(o, "qlx.FKPair(t.UserID, to.ID)") {
		t.Errorf("orders.go lacks the foreign-key method:\n%s", o)
	}
	// The identity ALWAYS and stored generated columns must not be settable.
	if strings.Contains(m, "func (i MemberInsert) Seq(") {
		t.Error("identity ALWAYS column is settable")
	}
	if len(res.Warnings) > 0 {
		t.Logf("warnings: %v", res.Warnings)
	}
}

func TestGenerateDeterministic(t *testing.T) {
	a, _ := generate(t)
	b, _ := generate(t)
	for n, src := range a.Files {
		if string(b.Files[n]) != string(src) {
			t.Errorf("%s differs between two runs", n)
		}
	}
}

func keys(m map[string][]byte) []string {
	var k []string
	for n := range m {
		k = append(k, n)
	}
	return k
}

// Names must replay Generate's naming exactly, or sql2go would emit symbols
// that do not exist in the generated package.
func TestNamesMatchGeneratedSymbols(t *testing.T) {
	res, s := generate(t)
	var all strings.Builder
	for _, src := range res.Files {
		all.Write(src)
	}
	text := all.String()
	names := codegen.Names(s, codegen.Config{Package: "gen", Schemas: []string{"public"}})
	if len(names) == 0 {
		t.Fatal("no tables named")
	}
	for _, tn := range names {
		if !regexp.MustCompile(`\nvar ` + tn.Symbol + `\s`).MatchString(text) {
			t.Errorf("symbol %s for %s is not declared in the generated package", tn.Symbol, tn.SQL)
		}
		if !strings.Contains(text, "type "+tn.Symbol+"T struct") {
			t.Errorf("type %sT missing", tn.Symbol)
		}
		for _, c := range tn.Columns {
			if !regexp.MustCompile(`\n\t` + c.Field + `\s+qlx\.`).MatchString(text) {
				t.Errorf("%s.%s: field %s is not in the generated struct", tn.SQL, c.SQL, c.Field)
			}
		}
	}
}
