package qlx

import (
	"fmt"
)

type mergeWhen struct {
	kind     string // "MATCHED" | "NOT MATCHED" | "NOT MATCHED BY SOURCE"
	cond     Pred
	action   string // "update" | "delete" | "nothing" | "insert" | "insert-default"
	sets     []Assignment
	scopeSrc bool
	scopeTgt bool
}

// MergeQuery is an immutable MERGE.
type MergeQuery struct {
	ctes      []CTE
	target    TableRef
	source    FromItem
	on        Pred
	whens     []mergeWhen
	returning []ReturningItem
}

// Merge starts MERGE INTO t.
func Merge(t Table) MergeQuery { return MergeQuery{target: t.tableRef()} }

func (MergeQuery) cteBody()             {}
func (q MergeQuery) outNames() []string { return returningNames(q.returning) }

func (q MergeQuery) With(ctes ...CTE) MergeQuery { q.ctes = app(q.ctes, ctes...); return q }

// Using sets the data source: a table, subquery, CTE or VALUES table.
func (q MergeQuery) Using(src FromItem) MergeQuery { q.source = src; return q }

// On sets the join condition between target and source.
func (q MergeQuery) On(p Pred) MergeQuery { q.on = p; return q }

// Returning adds RETURNING. merge_action(), OLD and NEW are available.
func (q MergeQuery) Returning(items ...ReturningItem) MergeQuery {
	q.returning = app(q.returning, items...)
	return q
}

// MergeAction is merge_action(): 'INSERT', 'UPDATE' or 'DELETE'. It is a
// RETURNING-only expression, like OLD and NEW.
//
// Requires PostgreSQL 17.
func MergeAction() ReturningExpr[string] {
	return ReturningExpr[string]{versioned{17, "merge_action()", fnNode{"merge_action", nil}}}
}

func (q MergeQuery) addWhen(w mergeWhen) MergeQuery {
	q.whens = app(q.whens, w)
	return q
}

// MatchedBuilder completes WHEN MATCHED.
type MatchedBuilder struct {
	q    MergeQuery
	cond Pred
}

// WhenMatched starts WHEN MATCHED [AND cond].
func (q MergeQuery) WhenMatched(cond ...Pred) MatchedBuilder {
	return MatchedBuilder{q, And(cond...)}
}

func (m MatchedBuilder) w(action string, sets []Assignment) MergeQuery {
	return m.q.addWhen(mergeWhen{kind: "MATCHED", cond: m.cond, action: action, sets: live(sets), scopeSrc: true, scopeTgt: true})
}
func (m MatchedBuilder) ThenUpdate(sets ...Assignment) MergeQuery { return m.w("update", sets) }
func (m MatchedBuilder) ThenDelete() MergeQuery                   { return m.w("delete", nil) }
func (m MatchedBuilder) ThenDoNothing() MergeQuery                { return m.w("nothing", nil) }

// NotMatchedBuilder completes WHEN NOT MATCHED (BY TARGET).
type NotMatchedBuilder struct {
	q    MergeQuery
	cond Pred
}

// WhenNotMatched starts WHEN NOT MATCHED [AND cond]: a source row without a
// target row. Only source columns are visible in its condition and values.
func (q MergeQuery) WhenNotMatched(cond ...Pred) NotMatchedBuilder {
	return NotMatchedBuilder{q, And(cond...)}
}

func (m NotMatchedBuilder) w(action string, sets []Assignment) MergeQuery {
	return m.q.addWhen(mergeWhen{kind: "NOT MATCHED", cond: m.cond, action: action, sets: live(sets), scopeSrc: true})
}
func (m NotMatchedBuilder) ThenInsert(as ...RowValue) MergeQuery {
	return m.w("insert", rowAssignments(as))
}
func (m NotMatchedBuilder) ThenInsertDefaults() MergeQuery { return m.w("insert-default", nil) }
func (m NotMatchedBuilder) ThenDoNothing() MergeQuery      { return m.w("nothing", nil) }

// BySourceBuilder completes WHEN NOT MATCHED BY SOURCE.
type BySourceBuilder struct {
	q    MergeQuery
	cond Pred
}

// WhenNotMatchedBySource starts WHEN NOT MATCHED BY SOURCE [AND cond]: a
// target row without a source row. Only target columns are visible.
func (q MergeQuery) WhenNotMatchedBySource(cond ...Pred) BySourceBuilder {
	return BySourceBuilder{q, And(cond...)}
}

func (m BySourceBuilder) w(action string, sets []Assignment) MergeQuery {
	return m.q.addWhen(mergeWhen{kind: "NOT MATCHED BY SOURCE", cond: m.cond, action: action, sets: live(sets), scopeTgt: true})
}
func (m BySourceBuilder) ThenUpdate(sets ...Assignment) MergeQuery { return m.w("update", sets) }
func (m BySourceBuilder) ThenDelete() MergeQuery                   { return m.w("delete", nil) }
func (m BySourceBuilder) ThenDoNothing() MergeQuery                { return m.w("nothing", nil) }

func (q MergeQuery) render(b *builder) {
	if q.source == nil {
		b.fail(fmt.Errorf("%w: MERGE needs Using", ErrShape))
		return
	}
	if !q.on.valid() {
		b.fail(fmt.Errorf("%w: MERGE needs On", ErrShape))
		return
	}
	if len(q.whens) == 0 {
		b.fail(fmt.Errorf("%w: MERGE needs at least one When clause", ErrShape))
		return
	}
	var ctes []string
	for _, c := range q.ctes {
		ctes = append(ctes, c.key())
	}
	tgt := append([]string{q.target.key()}, ctes...)
	src := append([]string{q.source.scopeKey()}, ctes...)
	both := append(append([]string{}, tgt...), q.source.scopeKey())

	renderWith(b, q.ctes)
	b.write("MERGE INTO ")
	q.target.renderFrom(b)
	b.write(" USING ")
	b.pushScope(ctes...)
	q.source.renderFrom(b)
	b.popScope()
	b.write(" ON ")
	b.pushScope(both...)
	b.noWindow++
	q.on.n.render(b)
	b.noWindow--
	b.popScope()

	for _, w := range q.whens {
		var keys []string
		switch {
		case w.scopeSrc && w.scopeTgt:
			keys = both
		case w.scopeSrc:
			keys = src
		default:
			keys = tgt
		}
		b.pushScope(keys...)
		q.renderWhen(b, w)
		b.popScope()
		if b.err != nil {
			return
		}
	}
	b.pushScope(both...)
	renderReturning(b, q.returning)
	b.popScope()
}

func (q MergeQuery) renderWhen(b *builder, w mergeWhen) {
	b.write(" WHEN " + w.kind)
	if w.cond.valid() {
		b.write(" AND ")
		w.cond.n.render(b)
	}
	b.write(" THEN ")
	switch w.action {
	case "nothing":
		b.write("DO NOTHING")
	case "delete":
		b.write("DELETE")
	case "update":
		if len(w.sets) == 0 {
			b.fail(fmt.Errorf("%w: MERGE ... UPDATE needs at least one assignment", ErrShape))
			return
		}
		b.write("UPDATE SET ")
		seen := map[string]bool{}
		joinParts(b, len(w.sets), func(i int) {
			a := w.sets[i]
			if !checkAssignTable(b, q.target, a) {
				return
			}
			if seen[a.col.name] {
				b.fail(fmt.Errorf("%w: column %s assigned twice", ErrShape, a.col.name))
				return
			}
			seen[a.col.name] = true
			b.ident(a.col.name)
			b.write(" = ")
			a.val.render(b)
		})
	case "insert-default":
		if len(q.target.Required) > 0 {
			b.fail(fmt.Errorf("%w: INSERT DEFAULT VALUES into %s misses required columns %v", ErrShape, q.target.Name, q.target.Required))
			return
		}
		b.write("INSERT DEFAULT VALUES")
	case "insert":
		if len(w.sets) == 0 {
			b.fail(fmt.Errorf("%w: MERGE ... INSERT needs at least one assignment", ErrShape))
			return
		}
		have := map[string]bool{}
		for _, a := range w.sets {
			if !checkAssignTable(b, q.target, a) {
				return
			}
			if have[a.col.name] {
				b.fail(fmt.Errorf("%w: column %s assigned twice", ErrShape, a.col.name))
				return
			}
			have[a.col.name] = true
		}
		for _, req := range q.target.Required {
			if !have[req] {
				b.fail(fmt.Errorf("%w: MERGE insert into %s is missing required column %q", ErrShape, q.target.Name, req))
				return
			}
		}
		b.write("INSERT (")
		joinParts(b, len(w.sets), func(i int) { b.ident(w.sets[i].col.name) })
		b.write(") VALUES (")
		joinParts(b, len(w.sets), func(i int) { w.sets[i].val.render(b) })
		b.write(")")
	}
}
