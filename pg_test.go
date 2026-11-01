package qlx_test

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	q "github.com/noobmaisfier/qlx"
)

// These tests ask a real PostgreSQL to PREPARE every rendered statement.
// They are skipped unless scripts/with_pg.sh (or QLX_PG_*) provides a
// server, so the plain `go test ./...` stays hermetic.
func pgEnv(t *testing.T) (env []string, version int) {
	t.Helper()
	host := os.Getenv("QLX_PG_HOST")
	if host == "" {
		t.Skip("QLX_PG_HOST not set; run scripts/with_pg.sh go test ./...")
	}
	if _, err := exec.LookPath("psql"); err != nil {
		t.Skip("psql not found")
	}
	version, _ = strconv.Atoi(os.Getenv("QLX_PG_VERSION"))
	env = append(os.Environ(), "PGHOST="+host, "PGDATABASE="+os.Getenv("QLX_PG_DB"))
	if u := os.Getenv("QLX_PG_USER"); u != "" {
		env = append(env, "PGUSER="+u)
	}
	return env, version
}

func prepare(env []string, sql string) error {
	cmd := exec.Command("psql", "-X", "-q", "-v", "ON_ERROR_STOP=1", "-c", "PREPARE s AS "+sql)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &prepareError{strings.TrimSpace(string(out))}
	}
	return nil
}

type prepareError struct{ msg string }

func (e *prepareError) Error() string { return e.msg }

func TestServerAcceptsRenderedSQL(t *testing.T) {
	env, version := pgEnv(t)
	for _, c := range append(append(append(append(append(goldenCases(), tranche2Cases()...), tableFuncCases()...), jsonCtorCases()...), jsonCases()...), xmlCases()...) {
		t.Run(c.name, func(t *testing.T) {
			st, err := q.Render(q.Dialect{Version: q.PG19}, c.stmt)
			if err != nil {
				t.Fatal(err)
			}
			sql := st.SQL
			err = prepare(env, sql)
			switch {
			case c.serverMin <= version:
				if err != nil {
					t.Fatalf("PostgreSQL %d rejected:\n%s\n%v", version, sql, err)
				}
			default:
				// Version rule: syntax newer than the server must be rejected.
				// If it is accepted, the serverMin tag (and the gate) is wrong.
				if err == nil {
					t.Fatalf("PostgreSQL %d accepted syntax tagged as needing %d:\n%s", version, c.serverMin, sql)
				}
				t.Logf("correctly rejected by PostgreSQL %d (needs %d): %v", version, c.serverMin, err)
			}
		})
	}
}
