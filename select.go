package qlx

import (
	"fmt"
	"slices"
)

func app[T any](s []T, v ...T) []T {
	// Full slice expression forces a copy, so every builder step returns a
	// new value and queries stay immutable and safe to share.
	return append(s[:len(s):len(s)], v...)
}

type joinClause struct {
	kind string // closed set: JOIN, LEFT JOIN, ...
	item FromItem
	on   Pred
}

type lockClause struct {
	strength string // "UPDATE", "NO KEY UPDATE", "SHARE", "KEY SHARE"
	of       []TableRef
	wait     string // "", "NOWAIT", "SKIP LOCKED"
}

// SelectQuery is an immutable SELECT. Every method returns a new value.
type SelectQuery struct {
	ctes       []CTE
	items      []Selectable
	distinct   bool
	distinctOn []Value
	from       FromItem
	joins      []joinClause
	where      []Pred
	group      []Value
	having     []HavingCond
	order      []OrderTerm
	limit      *int64
	offset     *int64
	fetch      *int64
	ties       bool
	lock       *lockClause
	windows    []NamedWindow
}

// Select starts a SELECT with the given items.
// Select starts a SELECT. The items are copied: a slice passed as xs... can be
// reused or changed afterwards without changing the query.
func Select(items ...Selectable) SelectQuery { return SelectQuery{items: slices.Clone(items)} }

func (SelectQuery) queryNode() {}
func (SelectQuery) cteBody()   {}

func (q SelectQuery) outNames() []string {
	out := make([]string, len(q.items))
	for i, it := range q.items {
		out[i] = it.selectNode().name()
	}
	return out
}

func (q SelectQuery) needsParens() bool {
	return len(q.order) > 0 || q.limit != nil || q.offset != nil || q.fetch != nil || q.lock != nil || len(q.ctes) > 0
}

// Windows defines named windows (the WINDOW clause) for use by Over.
func (q SelectQuery) Windows(ws ...NamedWindow) SelectQuery {
	q.windows = app(q.windows, ws...)
	return q
}

// With adds WITH items in front of the query.
func (q SelectQuery) With(ctes ...CTE) SelectQuery { q.ctes = app(q.ctes, ctes...); return q }

func (q SelectQuery) From(f FromItem) SelectQuery { q.from = f; return q }
func (q SelectQuery) Distinct() SelectQuery       { q.distinct = true; return q }
func (q SelectQuery) DistinctOn(vs ...Value) SelectQuery {
	q.distinctOn = app(q.distinctOn, vs...)
	return q
}
func (q SelectQuery) Where(ps ...Pred) SelectQuery { q.where = app(q.where, ps...); return q }
func (q SelectQuery) GroupBy(vs ...Value) SelectQuery {
	q.group = app(q.group, vs...)
	return q
}
func (q SelectQuery) Having(cs ...HavingCond) SelectQuery { q.having = app(q.having, cs...); return q }
func (q SelectQuery) OrderBy(ts ...OrderTerm) SelectQuery { q.order = app(q.order, ts...); return q }
func (q SelectQuery) Limit(n int64) SelectQuery           { q.limit = &n; return q }
func (q SelectQuery) Offset(n int64) SelectQuery          { q.offset = &n; return q }

// FetchFirst is FETCH FIRST n ROWS ONLY, the standard spelling of LIMIT.
// Add WithTies to include rows that tie with the last one.
func (q SelectQuery) FetchFirst(n int64) SelectQuery { q.fetch = &n; return q }

// WithTies turns FetchFirst into FETCH FIRST n ROWS WITH TIES (needs ORDER BY).
func (q SelectQuery) WithTies() SelectQuery { q.ties = true; return q }

// JoinBuilder finishes a join with its ON condition.
type JoinBuilder struct {
	q    SelectQuery
	kind string
	item FromItem
}

// On sets the join condition.
func (j JoinBuilder) On(p Pred) SelectQuery {
	j.q.joins = app(j.q.joins, joinClause{kind: j.kind, item: j.item, on: p})
	return j.q
}

func (q SelectQuery) Join(f FromItem) JoinBuilder      { return JoinBuilder{q, "JOIN", f} }
func (q SelectQuery) LeftJoin(f FromItem) JoinBuilder  { return JoinBuilder{q, "LEFT JOIN", f} }
func (q SelectQuery) RightJoin(f FromItem) JoinBuilder { return JoinBuilder{q, "RIGHT JOIN", f} }
func (q SelectQuery) FullJoin(f FromItem) JoinBuilder  { return JoinBuilder{q, "FULL JOIN", f} }

// CrossJoin joins without a condition.
func (q SelectQuery) CrossJoin(f FromItem) SelectQuery {
	q.joins = app(q.joins, joinClause{kind: "CROSS JOIN", item: f})
	return q
}

func (q SelectQuery) setLock(strength string) SelectQuery {
	q.lock = &lockClause{strength: strength}
	return q
}

// ForUpdate adds FOR UPDATE.
func (q SelectQuery) ForUpdate() SelectQuery { return q.setLock("UPDATE") }

// ForNoKeyUpdate adds FOR NO KEY UPDATE.
func (q SelectQuery) ForNoKeyUpdate() SelectQuery { return q.setLock("NO KEY UPDATE") }

// ForShare adds FOR SHARE.
func (q SelectQuery) ForShare() SelectQuery { return q.setLock("SHARE") }

// ForKeyShare adds FOR KEY SHARE.
func (q SelectQuery) ForKeyShare() SelectQuery { return q.setLock("KEY SHARE") }

func (q SelectQuery) withLock(f func(*lockClause)) SelectQuery {
	if q.lock == nil {
		return q
	}
	l := *q.lock
	f(&l)
	q.lock = &l
	return q
}

// Of restricts the lock to the given tables.
func (q SelectQuery) Of(ts ...Table) SelectQuery {
	return q.withLock(func(l *lockClause) {
		for _, t := range ts {
			l.of = app(l.of, t.tableRef())
		}
	})
}

// SkipLocked adds SKIP LOCKED.
func (q SelectQuery) SkipLocked() SelectQuery {
	return q.withLock(func(l *lockClause) { l.wait = "SKIP LOCKED" })
}

// NoWait adds NOWAIT.
func (q SelectQuery) NoWait() SelectQuery {
	return q.withLock(func(l *lockClause) { l.wait = "NOWAIT" })
}

func joinParts(b *builder, n int, f func(i int)) {
	for i := 0; i < n; i++ {
		if i > 0 {
			b.write(", ")
		}
		f(i)
	}
}

func (q SelectQuery) render(b *builder) {
	if len(q.items) == 0 {
		b.fail(fmt.Errorf("%w: SELECT needs at least one item", ErrShape))
		return
	}
	if q.from == nil && len(q.joins) > 0 {
		b.fail(fmt.Errorf("%w: JOIN without FROM", ErrShape))
		return
	}
	if q.limit != nil && q.fetch != nil {
		b.fail(fmt.Errorf("%w: Limit and FetchFirst cannot be combined", ErrShape))
		return
	}
	if q.ties && (q.fetch == nil || len(q.order) == 0) {
		b.fail(fmt.Errorf("%w: WithTies needs FetchFirst and OrderBy", ErrShape))
		return
	}
	for _, c := range [...]struct {
		what string
		v    *int64
	}{{"Limit", q.limit}, {"Offset", q.offset}, {"FetchFirst", q.fetch}} {
		if c.v != nil && *c.v < 0 {
			b.fail(fmt.Errorf("%w: %s must not be negative (got %d)", ErrShape, c.what, *c.v))
			return
		}
	}
	renderWith(b, q.ctes)

	keys := make([]string, 0, len(q.ctes)+len(q.joins)+1) // one allocation, not one per append
	for _, c := range q.ctes {
		keys = append(keys, c.key())
	}
	if q.from != nil {
		keys = append(keys, q.from.scopeKey())
	}
	for _, j := range q.joins {
		keys = append(keys, j.item.scopeKey())
	}
	b.pushScope(keys...)
	defer b.popScope()

	savedHook, savedDepth, savedAgg := b.colHook, b.aggDepth, b.sawAgg
	b.colHook, b.aggDepth, b.sawAgg = nil, 0, false // a nested SELECT is checked on its own
	defer func() { b.colHook, b.aggDepth, b.sawAgg = savedHook, savedDepth, savedAgg }()
	savedWin := b.windows
	b.windows = nil
	for _, w := range q.windows {
		b.windows = append(b.windows, w.name)
	}
	defer func() { b.windows = savedWin }()

	b.write("SELECT ")
	if q.distinct {
		b.write("DISTINCT ")
	}
	if len(q.distinctOn) > 0 {
		b.write("DISTINCT ON (")
		joinParts(b, len(q.distinctOn), func(i int) { q.distinctOn[i].valueNode().render(b) })
		b.write(") ")
	}
	joinParts(b, len(q.items), func(i int) { q.items[i].selectNode().render(b) })

	if q.from != nil {
		b.write(" FROM ")
		nc := len(q.ctes)
		b.renderFromItem(q.from, keys[:nc]) // a LATERAL first item sees nothing of this FROM
		for ji, j := range q.joins {
			b.write(" " + j.kind + " ")
			b.renderFromItem(j.item, keys[:nc+1+ji])
			if j.kind != "CROSS JOIN" {
				if !j.on.valid() {
					b.fail(fmt.Errorf("%w: %s needs a condition", ErrShape, j.kind))
					return
				}
				b.write(" ON ")
				b.noWindow++
				// an ON clause sees the items up to and including its own join
				b.withVisible(keys[:nc+2+ji], func() { j.on.n.render(b) })
				b.noWindow--
			}
		}
	}
	if renderAndBegin(q.where) {
		b.write(" WHERE ")
		b.noWindow++
		renderAnd(b, q.where)
		b.noWindow--
	}
	if len(q.group) > 0 {
		b.write(" GROUP BY ")
		b.noWindow++
		joinParts(b, len(q.group), func(i int) { q.group[i].valueNode().render(b) })
		b.noWindow--
	}
	var hvBuf [4]node
	hv := hvBuf[:0]
	for _, h := range q.having {
		if n := h.havingNode(); n != nil {
			hv = append(hv, n)
		}
	}
	if len(hv) > 0 {
		b.write(" HAVING ")
		b.noWindow++
		if len(hv) == 1 {
			hv[0].render(b)
		} else {
			renderLogic(b, "AND", hv)
		}
		b.noWindow--
	}
	if len(q.windows) > 0 {
		b.write(" WINDOW ")
		joinParts(b, len(q.windows), func(i int) {
			b.ident(q.windows[i].name)
			b.write(" AS ")
			q.windows[i].spec.renderBody(b)
		})
	}
	if len(q.order) > 0 {
		b.write(" ORDER BY ")
		joinParts(b, len(q.order), func(i int) { q.order[i].render(b) })
	}
	if q.limit != nil {
		b.write(" LIMIT ")
		b.arg(*q.limit)
	}
	if q.offset != nil {
		b.write(" OFFSET ")
		b.arg(*q.offset)
	}
	if q.fetch != nil {
		b.write(" FETCH FIRST ")
		b.arg(*q.fetch)
		if q.ties {
			b.write(" ROWS WITH TIES")
		} else {
			b.write(" ROWS ONLY")
		}
	}
	if q.lock != nil {
		b.write(" FOR " + q.lock.strength)
		if len(q.lock.of) > 0 {
			b.write(" OF ")
			joinParts(b, len(q.lock.of), func(i int) {
				t := q.lock.of[i]
				if !b.inScope(t.key()) {
					b.fail(fmt.Errorf("%w: FOR %s OF %s", ErrScope, q.lock.strength, t.key()))
					return
				}
				b.ident(t.key())
			})
		}
		if q.lock.wait != "" {
			b.write(" " + q.lock.wait)
		}
	}
	// Only grouped queries pay for the check: it re-renders the expressions.
	if b.err == nil && (len(q.group) > 0 || b.sawAgg) {
		q.checkGrouping(b)
	}
}

// probe renders a node into a scratch builder that reports the columns it
// uses outside aggregate calls, and whether it contains an aggregate.
func probe(b *builder, n node) (cols []colRef, sawAgg bool) {
	p := &builder{version: b.version, paramPos: map[int]int{}, scopes: b.scopes, windows: b.windows}
	p.colHook = func(c colRef) { cols = append(cols, c) }
	n.render(p)
	return cols, p.sawAgg
}

// checkGrouping enforces PostgreSQL's rule that, in a grouped query, every
// column outside an aggregate appears in GROUP BY or is functionally
// dependent on a grouped primary key. It is deliberately conservative: it
// only judges columns of tables whose primary key is known (generated code
// sets TableRef.PK), and it stands down when GROUP BY holds anything but plain
// columns (grouping sets, expressions), where a text comparison would guess.
func (q SelectQuery) checkGrouping(b *builder) {
	if b.hash {
		return // a shape that rendered once already passed this check
	}
	var exprs []node
	for _, it := range q.items {
		exprs = append(exprs, it.selectNode())
	}
	for _, h := range q.having {
		if n := h.havingNode(); n != nil {
			exprs = append(exprs, n)
		}
	}
	for _, o := range q.order {
		exprs = append(exprs, o.n)
	}
	hasAgg := false
	var uses []colRef
	for _, e := range exprs {
		cols, agg := probe(b, e)
		hasAgg = hasAgg || agg
		uses = append(uses, cols...)
	}
	if len(q.group) == 0 && !hasAgg {
		return
	}
	grouped := map[string]bool{}
	for _, g := range q.group {
		c, ok := g.valueNode().(colRef)
		if !ok {
			return // expression or grouping sets: do not guess
		}
		grouped[c.t.key()+"\x00"+c.name] = true
	}
	pkCovered := func(t TableRef) bool {
		for _, k := range t.PK {
			if !grouped[t.key()+"\x00"+k] {
				return false
			}
		}
		return true
	}
	for _, c := range uses {
		if grouped[c.t.key()+"\x00"+c.name] || len(c.t.PK) == 0 || pkCovered(c.t) {
			continue
		}
		b.fail(fmt.Errorf("%w: column %s.%s must appear in GROUP BY or be used inside an aggregate", ErrShape, c.t.key(), c.name))
		return
	}
}
