package qlx

import (
	"fmt"
)

func sameTable(a, b TableRef) bool {
	return a.Schema == b.Schema && a.Name == b.Name && a.Alias == b.Alias
}

func checkAssignTable(b *builder, t TableRef, a Assignment) bool {
	if !sameTable(a.col.t, t) {
		b.fail(fmt.Errorf("%w: column %s.%s assigned in a statement on %s", ErrScope, a.col.t.key(), a.col.name, t.key()))
		return false
	}
	return true
}

// RowAssigner is implemented by the generated insert and patch types: it
// yields the assignments for one row.
type RowAssigner interface{ Assignments() []Assignment }

// Constraint names a constraint; generated code declares one per constraint.
type Constraint struct{ Name string }

func returningNames(items []ReturningItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.returningNode().name()
	}
	return out
}

func renderReturning(b *builder, items []ReturningItem) {
	if len(items) == 0 {
		return
	}
	b.write(" RETURNING ")
	joinParts(b, len(items), func(i int) { items[i].returningNode().render(b) })
}

func renderWhere(b *builder, ps []Pred) bool {
	w := And(ps...)
	if !w.valid() {
		return false
	}
	b.write(" WHERE ")
	b.noWindow++
	w.n.render(b)
	b.noWindow--
	return true
}

func scopeKeysOf(ctes []CTE, t TableRef, items []FromItem) []string {
	keys := []string{t.key()}
	for _, c := range ctes {
		keys = append(keys, c.key())
	}
	for _, it := range items {
		keys = append(keys, it.scopeKey())
	}
	return keys
}

// ---------------------------------------------------------------------------
// INSERT
// ---------------------------------------------------------------------------

type conflictClause struct {
	targets    []colRef
	constraint string
	action     string // "nothing", "update", "select"
	sets       []Assignment
	where      Pred
	lock       string // "", "UPDATE", "SHARE" (DO SELECT only)
}

// InsertQuery is an immutable INSERT.
type InsertQuery struct {
	ctes       []CTE
	table      TableRef
	rows       [][]Assignment
	cols       []colRef
	source     Query
	defaults   bool
	overriding string // "", "SYSTEM VALUE", "USER VALUE"
	conflict   *conflictClause
	returning  []ReturningItem
}

// InsertInto starts an INSERT on t.
func InsertInto(t Table) InsertQuery { return InsertQuery{table: t.tableRef()} }

func (InsertQuery) cteBody()             {}
func (q InsertQuery) outNames() []string { return returningNames(q.returning) }

func (q InsertQuery) With(ctes ...CTE) InsertQuery { q.ctes = app(q.ctes, ctes...); return q }

// Row adds one row of assignments. Every row must assign the same columns.
func (q InsertQuery) Row(as ...Assignment) InsertQuery { q.rows = app(q.rows, live(as)); return q }

// Values adds rows from generated insert types.
func (q InsertQuery) Values(rows ...RowAssigner) InsertQuery {
	for _, r := range rows {
		q.rows = app(q.rows, live(r.Assignments()))
	}
	return q
}

// DefaultValues renders INSERT ... DEFAULT VALUES.
func (q InsertQuery) DefaultValues() InsertQuery { q.defaults = true; return q }

// Columns names the target columns for InsertQuery.FromQuery.
func (q InsertQuery) Columns(cols ...ColumnRef) InsertQuery {
	for _, c := range cols {
		q.cols = app(q.cols, c.columnRef())
	}
	return q
}

// FromQuery inserts the rows of a query (INSERT ... SELECT). Name the target
// columns with Columns first; the query must produce them in that order.
func (q InsertQuery) FromQuery(src Query) InsertQuery { q.source = src; return q }

// OverridingSystemValue is OVERRIDING SYSTEM VALUE, needed to set a
// GENERATED ALWAYS identity column.
func (q InsertQuery) OverridingSystemValue() InsertQuery { q.overriding = "SYSTEM VALUE"; return q }

// OverridingUserValue is OVERRIDING USER VALUE.
func (q InsertQuery) OverridingUserValue() InsertQuery { q.overriding = "USER VALUE"; return q }

// Returning adds a RETURNING list. OLD and NEW references need PostgreSQL 18.
func (q InsertQuery) Returning(items ...ReturningItem) InsertQuery {
	q.returning = app(q.returning, items...)
	return q
}

// ConflictBuilder chooses the ON CONFLICT action.
type ConflictBuilder struct {
	q          InsertQuery
	targets    []colRef
	constraint string
}

// OnConflict starts ON CONFLICT (cols). The columns must match a unique
// index or constraint; PREPARE on the server verifies that.
func (q InsertQuery) OnConflict(cols ...ColumnRef) ConflictBuilder {
	t := make([]colRef, len(cols))
	for i, c := range cols {
		t[i] = c.columnRef()
	}
	return ConflictBuilder{q: q, targets: t}
}

// OnConflictConstraint starts ON CONFLICT ON CONSTRAINT name.
func (q InsertQuery) OnConflictConstraint(c Constraint) ConflictBuilder {
	return ConflictBuilder{q: q, constraint: c.Name}
}

func (c ConflictBuilder) with(cc conflictClause) InsertQuery {
	cc.targets, cc.constraint = c.targets, c.constraint
	c.q.conflict = &cc
	return c.q
}

// DoNothing renders DO NOTHING.
func (c ConflictBuilder) DoNothing() InsertQuery { return c.with(conflictClause{action: "nothing"}) }

// DoUpdate renders DO UPDATE SET ...; use Col.Excluded() for the proposed row.
func (c ConflictBuilder) DoUpdate(sets ...Assignment) InsertQuery {
	return c.with(conflictClause{action: "update", sets: live(sets)})
}

// DoUpdateWhere is DoUpdate with a WHERE condition.
func (c ConflictBuilder) DoUpdateWhere(p Pred, sets ...Assignment) InsertQuery {
	return c.with(conflictClause{action: "update", sets: live(sets), where: p})
}

// DoSelect renders ON CONFLICT DO SELECT (PostgreSQL 19), which returns the
// existing row through RETURNING. It requires a RETURNING list.
//
// Requires PostgreSQL 19.
func (c ConflictBuilder) DoSelect() InsertQuery { return c.with(conflictClause{action: "select"}) }

func (q InsertQuery) lockExisting(strength string) InsertQuery {
	if q.conflict == nil {
		return q
	}
	cc := *q.conflict
	cc.lock = strength
	q.conflict = &cc
	return q
}

// ForUpdate locks the existing row selected by DoSelect.
func (q InsertQuery) ForUpdate() InsertQuery { return q.lockExisting("UPDATE") }

// ForShare locks the existing row selected by DoSelect with FOR SHARE.
func (q InsertQuery) ForShare() InsertQuery { return q.lockExisting("SHARE") }

func (q InsertQuery) checkRequired(b *builder, have map[string]bool) bool {
	for _, req := range q.table.Required {
		if !have[req] {
			b.fail(fmt.Errorf("%w: INSERT into %s is missing required column %q", ErrShape, q.table.Name, req))
			return false
		}
	}
	return true
}

func (q InsertQuery) render(b *builder) {
	renderWith(b, q.ctes)
	keys := scopeKeysOf(q.ctes, q.table, nil)
	b.pushScope(keys...)
	defer b.popScope()

	switch {
	case q.source != nil:
		if len(q.rows) > 0 || q.defaults {
			b.fail(fmt.Errorf("%w: INSERT takes either rows or FromQuery", ErrShape))
			return
		}
		if len(q.cols) == 0 {
			b.fail(fmt.Errorf("%w: INSERT ... SELECT needs Columns", ErrShape))
			return
		}
		if n := len(q.source.outNames()); n != len(q.cols) {
			b.fail(fmt.Errorf("%w: INSERT names %d columns but the query produces %d", ErrShape, len(q.cols), n))
			return
		}
		have := map[string]bool{}
		for _, c := range q.cols {
			if !sameTable(c.t, q.table) {
				b.fail(fmt.Errorf("%w: column %s.%s is not a column of %s", ErrScope, c.t.key(), c.name, q.table.key()))
				return
			}
			if have[c.name] {
				b.fail(fmt.Errorf("%w: column %s listed twice", ErrShape, c.name))
				return
			}
			have[c.name] = true
		}
		if !q.checkRequired(b, have) {
			return
		}
		b.write("INSERT INTO ")
		q.table.renderFrom(b)
		b.write(" (")
		joinParts(b, len(q.cols), func(i int) { b.ident(q.cols[i].name) })
		b.write(")")
		q.renderOverriding(b)
		b.write(" ")
		saved := b.scopes
		b.scopes = nil
		q.source.render(b)
		b.scopes = saved
	case q.defaults:
		if len(q.rows) > 0 {
			b.fail(fmt.Errorf("%w: DefaultValues cannot be combined with rows", ErrShape))
			return
		}
		if !q.checkRequired(b, map[string]bool{}) {
			return
		}
		b.write("INSERT INTO ")
		q.table.renderFrom(b)
		b.write(" DEFAULT VALUES")
	default:
		if !q.renderRows(b) {
			return
		}
	}
	if !q.renderConflict(b) {
		return
	}
	renderReturning(b, q.returning)
}

func (q InsertQuery) renderOverriding(b *builder) {
	if q.overriding != "" {
		b.write(" OVERRIDING " + q.overriding)
	}
}

func (q InsertQuery) renderRows(b *builder) bool {
	if len(q.rows) == 0 || len(q.rows[0]) == 0 {
		b.fail(fmt.Errorf("%w: INSERT into %s has no row; call Row or Values", ErrShape, q.table.Name))
		return false
	}
	first := q.rows[0]
	pos := make(map[string]int, len(first))
	for i, a := range first {
		if !checkAssignTable(b, q.table, a) {
			return false
		}
		if _, dup := pos[a.col.name]; dup {
			b.fail(fmt.Errorf("%w: column %s assigned twice", ErrShape, a.col.name))
			return false
		}
		pos[a.col.name] = i
	}
	for _, r := range q.rows[1:] {
		if len(r) != len(first) {
			b.fail(fmt.Errorf("%w: rows of one INSERT must assign the same columns", ErrShape))
			return false
		}
		for _, a := range r {
			if !checkAssignTable(b, q.table, a) {
				return false
			}
			if _, ok := pos[a.col.name]; !ok {
				b.fail(fmt.Errorf("%w: rows of one INSERT must assign the same columns (%s)", ErrShape, a.col.name))
				return false
			}
		}
	}
	have := make(map[string]bool, len(pos))
	for k := range pos {
		have[k] = true
	}
	if !q.checkRequired(b, have) {
		return false
	}

	b.write("INSERT INTO ")
	q.table.renderFrom(b)
	b.write(" (")
	joinParts(b, len(first), func(i int) { b.ident(first[i].col.name) })
	b.write(")")
	q.renderOverriding(b)
	b.write(" VALUES ")
	for ri, r := range q.rows {
		if ri > 0 {
			b.write(", ")
		}
		b.write("(")
		ordered := make([]Assignment, len(first))
		for _, a := range r {
			ordered[pos[a.col.name]] = a
		}
		joinParts(b, len(ordered), func(i int) { ordered[i].val.render(b) })
		b.write(")")
	}
	return true
}

func (q InsertQuery) renderConflict(b *builder) bool {
	c := q.conflict
	if c == nil {
		return true
	}
	for _, t := range c.targets {
		if !sameTable(t.t, q.table) {
			b.fail(fmt.Errorf("%w: conflict target %s.%s is not a column of %s", ErrScope, t.t.key(), t.name, q.table.key()))
			return false
		}
	}
	if c.action != "nothing" && len(c.targets) == 0 && c.constraint == "" {
		b.fail(fmt.Errorf("%w: ON CONFLICT DO %s needs conflict columns or a constraint", ErrShape, c.action))
		return false
	}
	b.write(" ON CONFLICT")
	switch {
	case c.constraint != "":
		b.write(" ON CONSTRAINT ")
		b.ident(c.constraint)
	case len(c.targets) > 0:
		b.write(" (")
		joinParts(b, len(c.targets), func(i int) { b.ident(c.targets[i].name) })
		b.write(")")
	}
	switch c.action {
	case "nothing":
		b.write(" DO NOTHING")
	case "update":
		if len(c.sets) == 0 {
			b.fail(fmt.Errorf("%w: DO UPDATE needs at least one assignment", ErrShape))
			return false
		}
		b.write(" DO UPDATE SET ")
		joinParts(b, len(c.sets), func(i int) {
			if !checkAssignTable(b, q.table, c.sets[i]) {
				return
			}
			b.ident(c.sets[i].col.name)
			b.write(" = ")
			c.sets[i].val.render(b)
		})
		if c.where.valid() {
			b.write(" WHERE ")
			c.where.n.render(b)
		}
	case "select":
		b.need(PG19, "ON CONFLICT DO SELECT")
		if len(q.returning) == 0 {
			b.fail(fmt.Errorf("%w: DO SELECT needs a RETURNING list", ErrShape))
			return false
		}
		b.write(" DO SELECT")
		if c.lock != "" {
			b.write(" FOR " + c.lock)
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// UPDATE
// ---------------------------------------------------------------------------

// UpdateQuery is an immutable UPDATE.
type UpdateQuery struct {
	ctes      []CTE
	table     TableRef
	sets      []Assignment
	from      []FromItem
	where     []Pred
	all       bool
	returning []ReturningItem
}

// Update starts an UPDATE on t.
func Update(t Table) UpdateQuery { return UpdateQuery{table: t.tableRef()} }

func (UpdateQuery) cteBody()             {}
func (q UpdateQuery) outNames() []string { return returningNames(q.returning) }

func (q UpdateQuery) With(ctes ...CTE) UpdateQuery     { q.ctes = app(q.ctes, ctes...); return q }
func (q UpdateQuery) Set(as ...Assignment) UpdateQuery { q.sets = app(q.sets, live(as)...); return q }

// SetPatch applies a generated patch: only the fields that are set become
// assignments, so an empty patch is reported instead of updating nothing.
func (q UpdateQuery) SetPatch(p RowAssigner) UpdateQuery { return q.Set(p.Assignments()...) }

// From adds UPDATE ... FROM items; join them to the target in Where.
func (q UpdateQuery) From(items ...FromItem) UpdateQuery { q.from = app(q.from, items...); return q }

func (q UpdateQuery) Where(ps ...Pred) UpdateQuery { q.where = app(q.where, ps...); return q }

// All confirms that updating every row is intended.
func (q UpdateQuery) All() UpdateQuery { q.all = true; return q }

func (q UpdateQuery) Returning(items ...ReturningItem) UpdateQuery {
	q.returning = app(q.returning, items...)
	return q
}

func (q UpdateQuery) render(b *builder) {
	if len(q.sets) == 0 {
		b.fail(fmt.Errorf("%w: UPDATE of %s has no assignment; call Set or SetPatch with at least one field", ErrShape, q.table.Name))
		return
	}
	renderWith(b, q.ctes)
	b.pushScope(scopeKeysOf(q.ctes, q.table, q.from)...)
	defer b.popScope()
	b.write("UPDATE ")
	q.table.renderFrom(b)
	b.write(" SET ")
	seen := map[string]bool{}
	joinParts(b, len(q.sets), func(i int) {
		a := q.sets[i]
		if !checkAssignTable(b, q.table, a) {
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
	if len(q.from) > 0 {
		b.write(" FROM ")
		joinParts(b, len(q.from), func(i int) { q.from[i].renderFrom(b) })
	}
	if !renderWhere(b, q.where) && !q.all {
		b.fail(fmt.Errorf("%w: UPDATE of %s without WHERE; add Where or call All()", ErrShape, q.table.Name))
		return
	}
	renderReturning(b, q.returning)
}

// ---------------------------------------------------------------------------
// DELETE
// ---------------------------------------------------------------------------

// DeleteQuery is an immutable DELETE.
type DeleteQuery struct {
	ctes      []CTE
	table     TableRef
	using     []FromItem
	where     []Pred
	all       bool
	returning []ReturningItem
}

// DeleteFrom starts a DELETE on t.
func DeleteFrom(t Table) DeleteQuery { return DeleteQuery{table: t.tableRef()} }

func (DeleteQuery) cteBody()             {}
func (q DeleteQuery) outNames() []string { return returningNames(q.returning) }

func (q DeleteQuery) With(ctes ...CTE) DeleteQuery { q.ctes = app(q.ctes, ctes...); return q }

// Using adds DELETE ... USING items; join them to the target in Where.
func (q DeleteQuery) Using(items ...FromItem) DeleteQuery { q.using = app(q.using, items...); return q }

func (q DeleteQuery) Where(ps ...Pred) DeleteQuery { q.where = app(q.where, ps...); return q }

// All confirms that deleting every row is intended.
func (q DeleteQuery) All() DeleteQuery { q.all = true; return q }

func (q DeleteQuery) Returning(items ...ReturningItem) DeleteQuery {
	q.returning = app(q.returning, items...)
	return q
}

func (q DeleteQuery) render(b *builder) {
	renderWith(b, q.ctes)
	b.pushScope(scopeKeysOf(q.ctes, q.table, q.using)...)
	defer b.popScope()
	b.write("DELETE FROM ")
	q.table.renderFrom(b)
	if len(q.using) > 0 {
		b.write(" USING ")
		joinParts(b, len(q.using), func(i int) { q.using[i].renderFrom(b) })
	}
	if !renderWhere(b, q.where) && !q.all {
		b.fail(fmt.Errorf("%w: DELETE from %s without WHERE; add Where or call All()", ErrShape, q.table.Name))
		return
	}
	renderReturning(b, q.returning)
}
