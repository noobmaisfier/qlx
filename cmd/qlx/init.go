package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const initConfig = `# QLX configuration. Unknown keys are errors.
package: %s            # Go package of the generated code
out: %s                # directory it is written to
schemas: [public]      # schemas to generate for
migrations: migrations # *.sql applied in name order to a throwaway server
targets: [18, 19]      # PostgreSQL majors the code must run on
# database_url: postgres://localhost/app   # read a live database instead
# uuid: std            # "std" is the standard library uuid.UUID, "bytes" is [16]byte
# date: qt            # "qt" is qt.Date (a calendar date), "stdlib" is time.Time
# timestamptz: stdlib  # opt in to time.Time (default is qt.Timestamptz, which can hold infinity)
# array_nulls: false   # true types array elements as qt.Null[T] so NULL elements can be read
# ids: int             # "string" types integer primary and foreign keys as qt.ID (opaque strings in JSON; call qt.SetIDKey)
# json_types:          # document type of json/jsonb columns (default json.RawMessage)
#   users.settings: example.com/app/model.Settings
# overrides:           # Go type of a PostgreSQL type
#   numeric:
#     type: github.com/shopspring/decimal.Decimal
#     family: num
# extensions: [pgvector, postgis, pg_trgm, hstore, ltree, citext, pgcrypto, uuid-ossp, unaccent, fuzzystrmatch, btree_gist]
`

const initMigration = `-- Your schema lives here as plain SQL. ` + "`qlx gen`" + ` applies every *.sql file in
-- this directory, in name order, to a throwaway PostgreSQL server, reads the
-- catalogs back, and writes schema.lock and the typed Go package.
--
-- Replace this example with your own tables.
CREATE TABLE users (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email      text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);
`

// cmdInit scaffolds qlx.json and a migrations directory. It never
// overwrites: an existing file is an error, because the config is the one
// thing in a project that is edited by hand.
func cmdInit(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	pkg := fs.String("package", "db", "Go package name of the generated code")
	dir := fs.String("out", "", "output directory (default: the package name)")
	root := fs.String("dir", ".", "project directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" {
		*dir = *pkg
	}
	cfgPath := filepath.Join(*root, "qlx.yaml")
	for _, name := range []string{"qlx.yaml", "qlx.yml", "qlx.json"} {
		existing := filepath.Join(*root, name)
		if _, err := os.Stat(existing); err == nil {
			return fmt.Errorf("%s already exists; init does not overwrite it", existing)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	cfg := fmt.Sprintf(initConfig, *pkg, *dir)
	mig := filepath.Join(*root, "migrations")
	if err := os.MkdirAll(mig, 0o755); err != nil {
		return err
	}
	first := filepath.Join(mig, "001_init.sql")
	if _, err := os.Stat(first); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(first, []byte(initMigration), 0o644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "wrote %s and %s\nnext: edit the migration, then run `qlx gen`\n", cfgPath, first)
	return nil
}
