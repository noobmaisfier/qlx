package qlx

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

var (
	// ErrInvalidIdent is returned for names that cannot be quoted safely.
	ErrInvalidIdent = errors.New("qlx: invalid identifier")
	// ErrUnsupportedByServer is returned when a construct needs a newer
	// PostgreSQL than the dialect targets.
	ErrUnsupportedByServer = errors.New("qlx: construct not supported by target PostgreSQL version")
	// ErrScope is returned when a column belongs to a table that is not in
	// the statement's FROM, JOIN or target.
	ErrScope = errors.New("qlx: column not in scope")
	// ErrShape is returned for statements that are structurally invalid
	// (no rows, missing required column, UPDATE without WHERE, and so on).
	ErrShape = errors.New("qlx: invalid statement shape")
	// ErrSort is returned by Sorts.Parse for a sort string that is not
	// in the allowlist or is malformed.
	ErrSort = errors.New("qlx: invalid sort")
	// ErrParams is returned when bound parameters do not match the statement.
	ErrParams = errors.New("qlx: parameter binding mismatch")
)

// maxIdentBytes is PostgreSQL's NAMEDATALEN-1. Longer names are silently
// truncated by the server, which could make two names collide, so they are
// rejected instead.
const maxIdentBytes = 63

// quoteIdent validates name and returns it double-quoted. Every string that
// reaches the renderer as a name goes through here; nothing else ever does.
func quoteIdent(name string) (string, error) {
	if name == "" || len(name) > maxIdentBytes || !utf8.ValidString(name) || strings.IndexByte(name, 0) >= 0 {
		return "", fmt.Errorf("%w: %q", ErrInvalidIdent, name)
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`, nil
}

// QuoteIdent exposes identifier quoting for tooling and tests.
func QuoteIdent(name string) (string, error) { return quoteIdent(name) }
