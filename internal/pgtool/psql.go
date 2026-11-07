// Package pgtool runs psql and throwaway PostgreSQL servers. It exists so
// the generator needs no database driver: introspection is plain SQL whose
// JSON result psql prints.
package pgtool

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// Conn is a psql connection description.
type Conn struct {
	// Env carries PGHOST, PGDATABASE and friends.
	Env []string
	// URL, when set, is passed as the connection string instead of Env.
	URL string
}

// FromURL connects with a postgres:// URL.
func FromURL(url string) Conn { return Conn{URL: url} }

// ErrNoPsql is returned when the psql client is not installed. The generator
// reads the schema through psql so that the core module needs no database
// driver; install the PostgreSQL client package (postgresql-client, or the
// libpq tools) or run `qlx gen -from-lock` where no database is wanted.
var ErrNoPsql = fmt.Errorf("psql not found in PATH: install the PostgreSQL client tools, or use `qlx gen -from-lock` to regenerate from schema.lock without a database")

func (c Conn) command(args ...string) *exec.Cmd {
	base := []string{"-X", "-q", "-v", "ON_ERROR_STOP=1"}
	if c.URL != "" {
		base = append(base, "-d", c.URL)
	}
	cmd := exec.Command("psql", append(base, args...)...)
	cmd.Env = append(os.Environ(), c.Env...)
	return cmd
}

// Query runs SQL (read from stdin, so psql variables and long text are fine)
// and returns the unaligned, tuples-only output, trimmed.
func (c Conn) Query(sql string) ([]byte, error) {
	return c.QueryVars(sql, nil)
}

// QueryVars is Query with psql variables. Refer to a variable as :'name' in
// the SQL and psql replaces it with the value as a quoted literal, so a caller
// never builds a string literal from a value itself.
func (c Conn) QueryVars(sql string, vars map[string]string) ([]byte, error) {
	if _, err := exec.LookPath("psql"); err != nil {
		return nil, ErrNoPsql
	}
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names)
	args := []string{"-A", "-t", "-f", "-"}
	for _, name := range names {
		args = append(args, "-v", name+"="+vars[name])
	}
	cmd := c.command(args...)
	cmd.Stdin = strings.NewReader(sql)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("psql: %v: %s", err, strings.TrimSpace(errb.String()))
	}
	return bytes.TrimSpace(out.Bytes()), nil
}

// Exec runs SQL and discards the output.
func (c Conn) Exec(sql string) error {
	_, err := c.Query(sql)
	return err
}

// ExecFile runs a SQL file.
func (c Conn) ExecFile(path string) error {
	if _, err := exec.LookPath("psql"); err != nil {
		return ErrNoPsql
	}
	cmd := c.command("-f", path)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %v: %s", path, err, strings.TrimSpace(errb.String()))
	}
	return nil
}
