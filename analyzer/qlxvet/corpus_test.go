package qlxvet_test

// Valid-code corpus: the analyzer must stay silent on code that is fine. It
// runs the real tool, as a user would (go vet -vettool=qlxvet), over the
// repository's own modules and fails on any finding outside a short, explicit
// list of files that exist to build invalid statements on purpose.
//
// What it proves:
//   - production code of every module, the guide's compiled examples
//     (guide_examples_test.go) and the generated fixture package produce no
//     finding, so a new rule cannot ship with a false positive on code the
//     project itself writes;
//   - the tool really runs (a known-bad package must still be reported), so a
//     silent pass cannot come from a broken harness.
//
// Needs the Go toolchain and the modules of x/tools; it skips itself when the
// tool cannot be built (offline).

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// negativeTests build invalid or version-gated statements on purpose, to check
// that the renderer rejects them. The analyzer is expected to report them; they
// are not part of the valid corpus.
var negativeTests = map[string]bool{
	"cache_test.go": true, "cmds_more_test.go": true, "commands_test.go": true, "fk_test.go": true,
	"fuzz_pg_test.go": true, "golden_test.go": true, "grouping_test.go": true, "jsonctor_test.go": true,
	"name_test.go": true, "planadvice_test.go": true, "shapecache_test.go": true, "sqljson_test.go": true,
	"tablefunc_test.go": true, "xml_test.go": true,
	"dump_test.go": true, "qtest_test.go": true, // package qtest: Register inside tests, duplicate names
	"prepare_test.go": true, // pgcat: calls every wrapper, whatever the targets, to PREPARE it
}

var finding = regexp.MustCompile(`^(?:\./)?(\S+\.go):(\d+):(\d+): (V\d{3}) (.*)$`)

func buildTool(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "qlxvet")
	cmd := exec.Command("go", "build", "-o", bin, "../cmd/qlxvet")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Skipf("cannot build the tool (offline?): %v\n%s", err, out.String())
	}
	return bin
}

type vetResult struct {
	findings []string // "file.go:line: Vxxx text"
	files    []string // base names, parallel to findings
	raw      string
	failed   bool // the command failed for a reason other than findings
}

func vet(t *testing.T, tool, dir string, patterns ...string) vetResult {
	t.Helper()
	cmd := exec.Command("go", append([]string{"vet", "-vettool=" + tool}, patterns...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	r := vetResult{raw: out.String()}
	for _, l := range strings.Split(out.String(), "\n") {
		if m := finding.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
			r.findings = append(r.findings, m[1]+":"+m[2]+": "+m[4]+" "+m[5])
			r.files = append(r.files, filepath.Base(m[1]))
		}
	}
	if err != nil && len(r.findings) == 0 {
		r.failed = true
	}
	return r
}

func TestAnalyzerSilentOnValidCode(t *testing.T) {
	tool := buildTool(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("the tool reports known-bad code", func(t *testing.T) {
		r := vet(t, tool, "testdata", "./src/app")
		if len(r.findings) == 0 {
			t.Fatalf("no finding on testdata/src/app: the harness is not running the analyzer\n%s", r.raw)
		}
	})

	for _, mod := range []struct {
		name   string
		dir    string
		strict bool // test files too; otherwise production files only
	}{
		{"root", root, true},
		{"pgxexec", filepath.Join(root, "pgxexec"), false},
		{"otelqlx", filepath.Join(root, "otelqlx"), false},
		{"bench", filepath.Join(root, "bench"), false},
	} {
		t.Run(mod.name, func(t *testing.T) {
			if _, err := os.Stat(filepath.Join(mod.dir, "go.mod")); err != nil {
				t.Skip("module not present")
			}
			r := vet(t, tool, mod.dir, "./...")
			if r.failed {
				t.Fatalf("go vet failed before analysing:\n%s", r.raw)
			}
			negative := 0
			for i, f := range r.findings {
				base := r.files[i]
				switch {
				case negativeTests[base] && mod.name == "root":
					negative++
				case !mod.strict && strings.HasSuffix(base, "_test.go"):
					t.Logf("not enforced (test file of %s): %s", mod.name, f)
				default:
					t.Errorf("false positive on valid code: %s", f)
				}
			}
			if mod.name == "root" && negative == 0 {
				t.Error("the negative test files produced no finding: the analyzer no longer sees what the renderer rejects")
			}
			t.Logf("%s: %d findings in negative test files, as expected", mod.name, negative)
		})
	}
}
