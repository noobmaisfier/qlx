package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/noobmaisfier/qlx/internal/pgtool"
)

func TestGenCheckCycle(t *testing.T) {
	if _, err := pgtool.FindBin(); err != nil {
		t.Skipf("no PostgreSQL server binaries: %v", err)
	}
	dir := t.TempDir()
	mig := filepath.Join(dir, "migrations")
	os.MkdirAll(mig, 0o755)
	schema, err := os.ReadFile("../../fixture/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(mig, "001.sql"), schema, 0o644)
	cfg := filepath.Join(dir, "qlx.json")
	os.WriteFile(cfg, []byte(`{"package":"db","out":"db","migrations":"migrations"}`), 0o644)

	var out, errb bytes.Buffer
	if code := run([]string{"gen", "-config", cfg}, &out, &errb); code != 0 {
		t.Fatalf("gen failed (%d): %s", code, errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := run([]string{"check", "-config", cfg}, &out, &errb); code != 0 {
		t.Fatalf("check after gen failed: %s", errb.String())
	}

	// Hand-editing a generated file must be caught.
	f := filepath.Join(dir, "db", "users.go")
	data, _ := os.ReadFile(f)
	os.WriteFile(f, append(data, []byte("\n// tampered\n")...), 0o644)
	errb.Reset()
	if code := run([]string{"check", "-config", cfg}, &out, &errb); code == 0 || !strings.Contains(errb.String(), "users.go") {
		t.Fatalf("tampering not detected: code=%d %s", code, errb.String())
	}

	// -from-lock repairs it with no database at all.
	os.WriteFile(cfg, []byte(`{"package":"db","out":"db"}`), 0o644)
	errb.Reset()
	if code := run([]string{"gen", "-config", cfg, "-from-lock"}, &out, &errb); code != 0 {
		t.Fatalf("gen -from-lock failed: %s", errb.String())
	}
	errb.Reset()
	if code := run([]string{"check", "-config", cfg}, &out, &errb); code != 0 {
		t.Fatalf("check after repair failed: %s", errb.String())
	}

	// A config typo is an error, not silence.
	os.WriteFile(cfg, []byte(`{"pakage":"db"}`), 0o644)
	errb.Reset()
	if code := run([]string{"check", "-config", cfg}, &out, &errb); code != 2 {
		t.Fatalf("unknown key accepted: %d", code)
	}
}

func TestSQL2GoSubcommand(t *testing.T) {
	if _, err := pgtool.FindBin(); err != nil {
		t.Skipf("no PostgreSQL server binaries: %v", err)
	}
	dir := t.TempDir()
	mig := filepath.Join(dir, "migrations")
	os.MkdirAll(mig, 0o755)
	schema, _ := os.ReadFile("../../fixture/schema.sql")
	os.WriteFile(filepath.Join(mig, "001.sql"), schema, 0o644)
	cfg := filepath.Join(dir, "qlx.json")
	os.WriteFile(cfg, []byte(`{"package":"db","out":"db","migrations":"migrations"}`), 0o644)
	var out, errb bytes.Buffer
	if code := run([]string{"gen", "-config", cfg}, &out, &errb); code != 0 {
		t.Fatalf("gen: %s", errb.String())
	}
	out.Reset()
	errb.Reset()
	for _, c := range []struct{ sql, want string }{
		{"SELECT id FROM users WHERE id = 1", "q.Select(Users.ID)"},
		{"INSERT INTO users (email) VALUES ('a')", `q.InsertInto(Users)`},
		{"UPDATE users SET email = 'b' WHERE id = 1", "q.Update(Users)"},
		{"DELETE FROM users WHERE id = 1", "q.DeleteFrom(Users)"},
	} {
		out.Reset()
		errb.Reset()
		if code := run([]string{"sql2go", "-config", cfg, "-e", c.sql}, &out, &errb); code != 0 {
			if strings.Contains(errb.String(), "built without SQL parsing") {
				return // default build: the stub explains how to enable parsing
			}
			t.Fatalf("sql2go %q: %s", c.sql, errb.String())
		}
		if !strings.Contains(out.String(), c.want) {
			t.Fatalf("%q: unexpected output:\n%s", c.sql, out.String())
		}
	}
}

func TestPrepareSubcommand(t *testing.T) {
	if _, err := pgtool.FindBin(); err != nil {
		t.Skipf("no PostgreSQL server binaries: %v", err)
	}
	dir := t.TempDir()
	mig := filepath.Join(dir, "migrations")
	os.MkdirAll(mig, 0o755)
	schema, _ := os.ReadFile("../../fixture/schema.sql")
	os.WriteFile(filepath.Join(mig, "001.sql"), schema, 0o644)
	cfg := filepath.Join(dir, "qlx.json")
	os.WriteFile(cfg, []byte(`{"package":"db","out":"db","migrations":"migrations"}`), 0o644)

	good := filepath.Join(dir, "good.json")
	os.WriteFile(good, []byte(`[{"name":"a","sql":"SELECT \"id\" FROM \"public\".\"users\" WHERE \"id\" = $1"},
		{"name":"b","sql":"SELECT 1"}]`), 0o644)
	var out, errb bytes.Buffer
	if code := run([]string{"prepare", "-config", cfg, "-queries", good}, &out, &errb); code != 0 {
		t.Fatalf("prepare failed: %s", errb.String())
	}
	if !strings.Contains(out.String(), "2 queries prepared") {
		t.Fatalf("output: %q", out.String())
	}

	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte(`[{"name":"ok","sql":"SELECT 1"},{"name":"typo","sql":"SELECT \"nope\" FROM \"public\".\"users\""}]`), 0o644)
	out.Reset()
	errb.Reset()
	if code := run([]string{"prepare", "-config", cfg, "-queries", bad}, &out, &errb); code == 0 {
		t.Fatal("a query on a missing column must fail")
	}
	if e := errb.String(); !strings.Contains(e, "1 of 2 queries rejected") || !strings.Contains(e, "typo:") || strings.Contains(e, "\n  ok:") {
		t.Fatalf("error report: %s", e)
	}
}

func TestInitScaffoldsAndGenWorks(t *testing.T) {
	if _, err := pgtool.FindBin(); err != nil {
		t.Skipf("no PostgreSQL server binaries: %v", err)
	}
	dir := t.TempDir()
	var out, errb bytes.Buffer
	if code := run([]string{"init", "-dir", dir}, &out, &errb); code != 0 {
		t.Fatalf("init: %s", errb.String())
	}
	if code := run([]string{"init", "-dir", dir}, &out, &errb); code == 0 {
		t.Fatal("a second init must refuse to overwrite")
	}
	cfg := filepath.Join(dir, "qlx.yaml")
	out.Reset()
	errb.Reset()
	if code := run([]string{"gen", "-config", cfg}, &out, &errb); code != 0 {
		t.Fatalf("gen on the scaffold failed: %s", errb.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "db", "users.go")); err != nil {
		t.Fatalf("scaffold did not generate users.go: %v", err)
	}
}

func TestVetPointsToTheAnalyzer(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"vet", "./..."}, &out, &errb)
	if code != 2 || !strings.Contains(errb.String(), "qlxvet") || !strings.Contains(errb.String(), "go install") {
		t.Fatalf("code %d out %q err %q", code, out.String(), errb.String())
	}
}

func TestWatchRegeneratesAndSurvivesErrors(t *testing.T) {
	if _, err := pgtool.FindBin(); err != nil {
		t.Skipf("no PostgreSQL server binaries: %v", err)
	}
	dir := t.TempDir()
	var out, errb bytes.Buffer
	if code := run([]string{"init", "-dir", dir}, &out, &errb); code != 0 {
		t.Fatal(errb.String())
	}
	cfgPath := filepath.Join(dir, "qlx.yaml")
	f, err := loadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	log := &syncBuf{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- watch(ctx, f, dir, cfgPath, 50*time.Millisecond, log, log) }()

	waitFor := func(what string, cond func() bool) {
		t.Helper()
		for i := 0; i < 400; i++ {
			if cond() {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %s; log:\n%s", what, log.String())
	}
	users := filepath.Join(dir, "db", "users.go")
	waitFor("the first generation", func() bool { _, err := os.Stat(users); return err == nil })

	mig := filepath.Join(dir, "migrations", "002_posts.sql")
	os.WriteFile(mig, []byte("CREATE TABLE posts (id bigint PRIMARY KEY, title text NOT NULL);\n"), 0o644)
	posts := filepath.Join(dir, "db", "posts.go")
	waitFor("posts.go after a new migration", func() bool { _, err := os.Stat(posts); return err == nil })

	os.WriteFile(mig, []byte("CREATE TABL oops;\n"), 0o644)
	waitFor("a reported failure", func() bool { return strings.Contains(log.String(), "gen failed") })

	os.WriteFile(mig, []byte("CREATE TABLE posts (id bigint PRIMARY KEY, title text NOT NULL, body text);\n"), 0o644)
	waitFor("recovery after the fix", func() bool {
		b, _ := os.ReadFile(posts)
		return strings.Contains(string(b), "Body")
	})
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }
