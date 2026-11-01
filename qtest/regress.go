package qtest

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/noobmaisfier/qlx"
)

// RegressCase is one statement of PostgreSQL's regression suite, translated
// by sql2go: the original text and the builder that must reproduce it.
type RegressCase struct {
	ID       string
	Original string
	Build    func() qlx.Statement
}

// Normalizer reduces SQL text to a form where equivalent statements are equal
// (libpg_query's normalize + fingerprint, in practice).
type Normalizer func(sql string) (string, error)

// RegressOptions configure RunRegress.
type RegressOptions struct {
	Dialect qlx.Dialect
	// Normalize compares the original with the rendered SQL. When nil, or
	// when it fails on the original (the parser predates the syntax), the
	// live server is the oracle: Prepare must accept the rendered SQL.
	Normalize Normalizer
	Prepare   Preparer
	// Known maps case IDs to the reason they are expected to fail. A known
	// failure does not fail the test; a known failure that now passes does,
	// so the ledger only shrinks.
	Known map[string]string
}

// LoadKnown reads a {"id": "reason"} file; a missing file is an empty set.
func LoadKnown(path string) map[string]string {
	b, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	m := map[string]string{}
	_ = json.Unmarshal(b, &m)
	return m
}

// RunRegress renders every case and compares it with the original. Each case
// is its own subtest, so a failure is tracked individually.
func RunRegress(t *testing.T, cases []RegressCase, o RegressOptions) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			reason, known := o.Known[c.ID]
			err := runOne(c, o)
			switch {
			case err != nil && !known:
				t.Errorf("%v\n  original: %s", err, c.Original)
			case err == nil && known:
				t.Errorf("now passes but is listed as a known failure (%s): remove it from the known-failures file", reason)
			}
		})
	}
}

func runOne(c RegressCase, o RegressOptions) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &caseError{"builder panicked", r}
		}
	}()
	sql, _, rerr := qlx.SQL(o.Dialect, c.Build())
	if rerr != nil {
		return &caseError{"render failed", rerr}
	}
	if o.Normalize != nil {
		a, aerr := o.Normalize(c.Original)
		if aerr == nil {
			b, berr := o.Normalize(sql)
			if berr != nil {
				return &caseError{"rendered SQL does not parse: " + sql, berr}
			}
			if a != b {
				return &caseError{"rendered SQL differs after normalisation: " + sql, nil}
			}
			return nil
		}
	}
	if o.Prepare == nil {
		return &caseError{"no normalizer accepted the statement and no server to ask", nil}
	}
	if perr := o.Prepare(context.Background(), "regress", sql); perr != nil {
		return &caseError{"server rejected the rendered SQL: " + sql, perr}
	}
	return nil
}

type caseError struct {
	msg string
	err any
}

func (e *caseError) Error() string {
	if e.err == nil {
		return e.msg
	}
	return e.msg + ": " + toString(e.err)
}

func toString(v any) string {
	switch x := v.(type) {
	case error:
		return x.Error()
	case string:
		return x
	}
	return "panic"
}
