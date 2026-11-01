//go:build !sql2go

package sql2go

import "errors"

// ParseToJSON is the stub used when the binary is built without the sql2go
// tag (and therefore without libpg_query).
func ParseToJSON(string) (string, error) {
	return "", errors.New("this qlx binary was built without SQL parsing: rebuild with -tags sql2go (needs cgo and github.com/pganalyze/pg_query_go/v6)")
}

// NormalizedFingerprint needs the sql2go build tag.
func NormalizedFingerprint(string) (string, error) {
	return "", errors.New("built without the sql2go tag")
}
