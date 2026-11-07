package qlx

import (
	"fmt"
	"slices"
)

// Utility commands an application runs that are not DDL: session variables,
// roles, DISCARD variants, deferred constraints, maintenance. Names are
// identifiers (validated and quoted); values that PostgreSQL allows as bound
// parameters are bound.

// Reset is RESET name.
func Reset(name string) Command { return cmd(func(b *builder) { b.write("RESET "); b.ident(name) }) }

// Show is SHOW name. (CurrentSetting is the bindable form.)
func Show(name string) Command { return cmd(func(b *builder) { b.write("SHOW "); b.ident(name) }) }

// SetRole is SET [LOCAL] ROLE name.
func SetRole(name string, local bool) Command {
	return cmd(func(b *builder) {
		b.write("SET ")
		if local {
			b.write("LOCAL ")
		}
		b.write("ROLE ")
		b.ident(name)
	})
}

// ResetRole is RESET ROLE.
func ResetRole() Command { return cmd(func(b *builder) { b.write("RESET ROLE") }) }

// SetSessionAuthorization is SET [LOCAL] SESSION AUTHORIZATION name.
func SetSessionAuthorization(name string, local bool) Command {
	return cmd(func(b *builder) {
		b.write("SET ")
		if local {
			b.write("LOCAL ")
		}
		b.write("SESSION AUTHORIZATION ")
		b.ident(name)
	})
}

// DiscardPlans is DISCARD PLANS.
func DiscardPlans() Command { return cmd(func(b *builder) { b.write("DISCARD PLANS") }) }

// DiscardSequences is DISCARD SEQUENCES.
func DiscardSequences() Command { return cmd(func(b *builder) { b.write("DISCARD SEQUENCES") }) }

// DiscardTemp is DISCARD TEMP.
func DiscardTemp() Command { return cmd(func(b *builder) { b.write("DISCARD TEMP") }) }

// SetConstraintsAll is SET CONSTRAINTS ALL DEFERRED|IMMEDIATE.
func SetConstraintsAll(deferred bool) Command {
	return cmd(func(b *builder) { b.write("SET CONSTRAINTS ALL " + deferWord(deferred)) })
}

// SetConstraints is SET CONSTRAINTS name, ... DEFERRED|IMMEDIATE.
func SetConstraints(deferred bool, names ...string) Command {
	names = slices.Clone(names)
	return cmd(func(b *builder) {
		if len(names) == 0 {
			b.fail(fmt.Errorf("%w: SET CONSTRAINTS needs a name (use SetConstraintsAll)", ErrShape))
			return
		}
		b.write("SET CONSTRAINTS ")
		for i, n := range names {
			if i > 0 {
				b.write(", ")
			}
			b.ident(n)
		}
		b.write(" " + deferWord(deferred))
	})
}

func deferWord(d bool) string {
	if d {
		return "DEFERRED"
	}
	return "IMMEDIATE"
}

// SetTransaction is SET TRANSACTION with the modes of Begin.
func SetTransaction(m TxMode) Command {
	return cmd(func(b *builder) {
		b.write("SET TRANSACTION")
		if m.Isolation != "" {
			b.write(" ISOLATION LEVEL " + string(m.Isolation))
		}
		if m.ReadOnly {
			b.write(" READ ONLY")
		}
		if m.Deferrable {
			b.write(" DEFERRABLE")
		}
	})
}

// RefreshStmt is REFRESH MATERIALIZED VIEW.
type RefreshStmt struct {
	schema, name       string
	concurrently, none bool
}

// RefreshMaterializedView starts REFRESH MATERIALIZED VIEW schema.name.
func RefreshMaterializedView(schema, name string) RefreshStmt {
	return RefreshStmt{schema: schema, name: name}
}

// Concurrently adds CONCURRENTLY.
func (s RefreshStmt) Concurrently() RefreshStmt { s.concurrently = true; return s }

// WithNoData adds WITH NO DATA.
func (s RefreshStmt) WithNoData() RefreshStmt { s.none = true; return s }

func (s RefreshStmt) render(b *builder) {
	if s.concurrently && s.none {
		b.fail(fmt.Errorf("%w: REFRESH MATERIALIZED VIEW cannot combine CONCURRENTLY and WITH NO DATA", ErrShape))
		return
	}
	b.write("REFRESH MATERIALIZED VIEW ")
	if s.concurrently {
		b.write("CONCURRENTLY ")
	}
	if s.schema != "" {
		b.ident(s.schema)
		b.write(".")
	}
	b.ident(s.name)
	if s.none {
		b.write(" WITH NO DATA")
	}
}

// ReindexKind is what REINDEX targets.
type ReindexKind string

// The REINDEX targets.
const (
	ReindexIndex    ReindexKind = "INDEX"
	ReindexTable    ReindexKind = "TABLE"
	ReindexSchema   ReindexKind = "SCHEMA"
	ReindexDatabase ReindexKind = "DATABASE"
)

// ReindexStmt is REINDEX.
type ReindexStmt struct {
	kind         ReindexKind
	schema, name string
	concurrently bool
}

// Reindex starts REINDEX kind [schema.]name.
func Reindex(kind ReindexKind, schema, name string) ReindexStmt {
	return ReindexStmt{kind: kind, schema: schema, name: name}
}

// Concurrently adds CONCURRENTLY.
func (s ReindexStmt) Concurrently() ReindexStmt { s.concurrently = true; return s }

func (s ReindexStmt) render(b *builder) {
	switch s.kind {
	case ReindexIndex, ReindexTable, ReindexSchema, ReindexDatabase:
	default:
		b.fail(fmt.Errorf("%w: unknown REINDEX target %q", ErrShape, s.kind))
		return
	}
	b.write("REINDEX " + string(s.kind) + " ")
	if s.concurrently {
		b.write("CONCURRENTLY ")
	}
	if s.schema != "" && (s.kind == ReindexIndex || s.kind == ReindexTable) {
		b.ident(s.schema)
		b.write(".")
	}
	b.ident(s.name)
}

// ClusterStmt is CLUSTER table USING index.
type ClusterStmt struct {
	t     TableRef
	index string
}

// Cluster is CLUSTER t USING index.
func Cluster(t Table, index string) ClusterStmt { return ClusterStmt{t.tableRef(), index} }

func (s ClusterStmt) render(b *builder) {
	b.write("CLUSTER ")
	s.t.renderFrom(b)
	b.write(" USING ")
	b.ident(s.index)
}
