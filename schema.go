package qlx

import (
	"fmt"
	"sort"
	"strings"
)

// SchemaSpec is what generated code expects of the live database. The
// generator embeds it; pgxexec.VerifySchema runs FingerprintSQL and hands the
// result to Check.
type SchemaSpec struct {
	Schemas        []string
	Targets        []int             // PostgreSQL majors the code was generated for
	Hash           string            // hash of the whole schema.lock
	Fingerprints   map[string]string // "schema.table" -> fingerprint
	FingerprintSQL string            // $1 is a text[] of schemas
}

// Check compares the fingerprints read from a live database with the ones the
// code was generated from. With no tables named it checks all of them; naming
// tables checks only those, which is how an application verifies just what
// it uses. Every difference is reported at once.
func (s SchemaSpec) Check(live map[string]string, tables ...string) error {
	names := tables
	if len(names) == 0 {
		for k := range s.Fingerprints {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	var bad []string
	for _, n := range names {
		want, ok := s.Fingerprints[n]
		if !ok {
			bad = append(bad, fmt.Sprintf("%s: not known to the generated code", n))
			continue
		}
		got, ok := live[n]
		switch {
		case !ok:
			bad = append(bad, fmt.Sprintf("%s: missing from the database", n))
		case got != want:
			bad = append(bad, fmt.Sprintf("%s: changed since generation (expected %s, found %s)", n, want, got))
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("qlx: database schema does not match generated code; run `qlx gen`:\n  %s", strings.Join(bad, "\n  "))
	}
	return nil
}

// CheckServer verifies that a server's major version is one the code was
// generated for. A server older than every target fails; with targets [18, 19]
// a 17 server is rejected before any query runs, and a newer major is allowed
// (the ledger is pinned, the server is not) but reported through the second
// result so an application can log that it runs ahead of its generated code.
func (s SchemaSpec) CheckServer(major int) (ahead bool, err error) {
	if len(s.Targets) == 0 {
		return false, nil
	}
	lo, hi := s.Targets[0], s.Targets[0]
	for _, t := range s.Targets {
		lo, hi = min(lo, t), max(hi, t)
	}
	if major < lo {
		return false, fmt.Errorf("qlx: server is PostgreSQL %d but the generated code targets %v; set targets in qlx.yaml and run `qlx gen`", major, s.Targets)
	}
	return major > hi, nil
}
