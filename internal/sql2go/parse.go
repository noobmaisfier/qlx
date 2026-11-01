//go:build sql2go

package sql2go

import pg_query "github.com/pganalyze/pg_query_go/v6"

// ParseToJSON parses SQL with the PostgreSQL parser itself (libpg_query) and
// returns its JSON parse tree. Needs cgo and `go get github.com/pganalyze/pg_query_go/v6`;
// build with -tags sql2go. The tag exists only so the rest of the module
// builds where that dependency has not been fetched yet.
func ParseToJSON(sql string) (string, error) { return pg_query.ParseToJSON(sql) }

// NormalizedFingerprint is libpg_query's fingerprint of the normalised
// statement: literals become parameters first, so a statement written with
// constants and its rendering with $n parameters compare equal.
func NormalizedFingerprint(sql string) (string, error) {
	n, err := pg_query.Normalize(sql)
	if err != nil {
		return "", err
	}
	return pg_query.Fingerprint(n)
}
