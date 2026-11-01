package qlx

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// pg_plan_advice (PostgreSQL 19 contrib) steers the planner with an advice
// string set in the pg_plan_advice.advice parameter. This file builds that
// string from typed pieces, so a target is a table of the query rather than a
// name typed by hand, and applies it with set_config (a bound parameter).
//
//	adv := qlx.Advice(
//	    qlx.JoinOrder(qlx.On(u), qlx.On(o)),
//	    qlx.HashJoin(qlx.On(o)),
//	    qlx.IndexScan(qlx.On(o), "orders_user_id_idx"),
//	)
//	cmds := []qlx.Statement{qlx.LoadPlanAdvice(), adv.Set(true)}
//
// Grammar source: https://www.postgresql.org/docs/19/pgplanadvice.html

// PlanAdvice is the EXPLAIN option that prints the advice for a plan.
var PlanAdvice = ExplainOption{kw: "PLAN_ADVICE", min: 19}

// AdviceTarget is alias_name#occurrence/partition_schema.partition_name@plan_name.
type AdviceTarget struct {
	alias      string
	occurrence int
	partSchema string
	partName   string
	plan       string
}

// On targets a table of the query by its alias (or its name when unaliased).
func On(t Table) AdviceTarget {
	r := t.tableRef()
	a := r.Alias
	if a == "" {
		a = r.Name
	}
	return AdviceTarget{alias: a}
}

// OnAlias targets a relation by alias for cases with no table symbol (a
// subquery or CTE alias).
func OnAlias(alias string) AdviceTarget { return AdviceTarget{alias: alias} }

// Occurrence selects the nth appearance of the relation within its subquery.
func (a AdviceTarget) Occurrence(n int) AdviceTarget { a.occurrence = n; return a }

// Partition selects a child of a partitioned table.
func (a AdviceTarget) Partition(schema, name string) AdviceTarget {
	a.partSchema, a.partName = schema, name
	return a
}

// InPlan places the target inside the named subplan.
func (a AdviceTarget) InPlan(name string) AdviceTarget { a.plan = name; return a }

// advice identifiers: letters, digits, underscore, dollar; anything else is
// double-quoted by the server's tokenizer rules, which we refuse to guess at.
var adviceIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$]*$`)

func (a AdviceTarget) text() (string, error) {
	if !adviceIdent.MatchString(a.alias) {
		return "", fmt.Errorf("%w: advice target %q is not a plain identifier", ErrInvalidIdent, a.alias)
	}
	s := a.alias
	if a.occurrence > 0 {
		s += "#" + strconv.Itoa(a.occurrence)
	}
	if a.partName != "" {
		if !adviceIdent.MatchString(a.partName) || (a.partSchema != "" && !adviceIdent.MatchString(a.partSchema)) {
			return "", fmt.Errorf("%w: partition %q.%q is not a plain identifier", ErrInvalidIdent, a.partSchema, a.partName)
		}
		s += "/"
		if a.partSchema != "" {
			s += a.partSchema + "."
		}
		s += a.partName
	}
	if a.plan != "" {
		if !adviceIdent.MatchString(a.plan) {
			return "", fmt.Errorf("%w: plan name %q is not a plain identifier", ErrInvalidIdent, a.plan)
		}
		s += "@" + a.plan
	}
	return s, nil
}

// AdviceItem is one tag of an advice string.
type AdviceItem struct {
	text string
	err  error
}

func targetsText(ts []AdviceTarget) (string, error) {
	parts := make([]string, len(ts))
	for i, t := range ts {
		s, err := t.text()
		if err != nil {
			return "", err
		}
		parts[i] = s
	}
	return strings.Join(parts, " "), nil
}

func simpleTag(tag string, ts []AdviceTarget) AdviceItem {
	if len(ts) == 0 {
		return AdviceItem{err: fmt.Errorf("%w: %s needs at least one target", ErrShape, tag)}
	}
	s, err := targetsText(ts)
	return AdviceItem{text: tag + "(" + s + ")", err: err}
}

// SeqScan is SEQ_SCAN(targets...).
func SeqScan(ts ...AdviceTarget) AdviceItem { return simpleTag("SEQ_SCAN", ts) }

// TIDScan is TID_SCAN(targets...).
func TIDScan(ts ...AdviceTarget) AdviceItem { return simpleTag("TID_SCAN", ts) }

// BitmapHeapScan is BITMAP_HEAP_SCAN(targets...).
func BitmapHeapScan(ts ...AdviceTarget) AdviceItem { return simpleTag("BITMAP_HEAP_SCAN", ts) }

// DoNotScan is DO_NOT_SCAN(targets...).
func DoNotScan(ts ...AdviceTarget) AdviceItem { return simpleTag("DO_NOT_SCAN", ts) }

// NoGather is NO_GATHER(targets...).
func NoGather(ts ...AdviceTarget) AdviceItem { return simpleTag("NO_GATHER", ts) }

func indexTag(tag string, t AdviceTarget, index string) AdviceItem {
	ts, err := t.text()
	if err != nil {
		return AdviceItem{err: err}
	}
	parts := strings.Split(index, ".")
	for _, p := range parts {
		if !adviceIdent.MatchString(p) {
			return AdviceItem{err: fmt.Errorf("%w: index name %q is not a plain identifier", ErrInvalidIdent, index)}
		}
	}
	return AdviceItem{text: tag + "(" + ts + " " + index + ")"}
}

// IndexScan is INDEX_SCAN(target index); the index may be schema-qualified.
func IndexScan(t AdviceTarget, index string) AdviceItem { return indexTag("INDEX_SCAN", t, index) }

// IndexOnlyScan is INDEX_ONLY_SCAN(target index).
func IndexOnlyScan(t AdviceTarget, index string) AdviceItem {
	return indexTag("INDEX_ONLY_SCAN", t, index)
}

// ForeignJoin is FOREIGN_JOIN((a b) (c d)): each group is pushed down as one
// foreign scan.
func ForeignJoin(groups ...[]AdviceTarget) AdviceItem {
	var parts []string
	for _, g := range groups {
		if len(g) < 2 {
			return AdviceItem{err: fmt.Errorf("%w: FOREIGN_JOIN groups need two or more foreign tables", ErrShape)}
		}
		s, err := targetsText(g)
		if err != nil {
			return AdviceItem{err: err}
		}
		parts = append(parts, "("+s+")")
	}
	if len(parts) == 0 {
		return AdviceItem{err: fmt.Errorf("%w: FOREIGN_JOIN needs a group", ErrShape)}
	}
	return AdviceItem{text: "FOREIGN_JOIN(" + strings.Join(parts, " ") + ")"}
}

// JoinOrderItem is a target, a (nested) ordered group, or a swappable {set}.
type JoinOrderItem struct {
	text string
	err  error
}

// Rel is a single relation in a JOIN_ORDER.
func Rel(t AdviceTarget) JoinOrderItem {
	s, err := t.text()
	return JoinOrderItem{text: s, err: err}
}

// Group is ( ... ): the members form a join product that must be on the
// inner side at that point.
func Group(items ...JoinOrderItem) JoinOrderItem { return joinGroup("(", ")", items) }

// Swappable is { ... }: like Group, but members may be reordered. It cannot
// contain groups.
func Swappable(items ...JoinOrderItem) JoinOrderItem {
	for _, it := range items {
		if strings.ContainsAny(it.text, "({") {
			return JoinOrderItem{err: fmt.Errorf("%w: a {swappable} list cannot contain sublists", ErrShape)}
		}
	}
	return joinGroup("{", "}", items)
}

func joinGroup(open, close string, items []JoinOrderItem) JoinOrderItem {
	parts := make([]string, len(items))
	for i, it := range items {
		if it.err != nil {
			return it
		}
		parts[i] = it.text
	}
	return JoinOrderItem{text: open + strings.Join(parts, " ") + close}
}

// JoinOrder is JOIN_ORDER(items...): the first item drives, then the next is
// joined to it, and so on.
func JoinOrder(items ...JoinOrderItem) AdviceItem {
	if len(items) < 2 {
		return AdviceItem{err: fmt.Errorf("%w: JOIN_ORDER needs two or more items", ErrShape)}
	}
	parts := make([]string, len(items))
	for i, it := range items {
		if it.err != nil {
			return AdviceItem{err: it.err}
		}
		parts[i] = it.text
	}
	return AdviceItem{text: "JOIN_ORDER(" + strings.Join(parts, " ") + ")"}
}

// JoinMethod is one of the join method tags.
type JoinMethod string

// The join methods.
const (
	MergeJoinMaterialize  JoinMethod = "MERGE_JOIN_MATERIALIZE"
	MergeJoinPlain        JoinMethod = "MERGE_JOIN_PLAIN"
	NestedLoopMaterialize JoinMethod = "NESTED_LOOP_MATERIALIZE"
	NestedLoopMemoize     JoinMethod = "NESTED_LOOP_MEMOIZE"
	NestedLoopPlain       JoinMethod = "NESTED_LOOP_PLAIN"
	HashJoinMethod        JoinMethod = "HASH_JOIN"
)

// Using is METHOD(a b): each target is on the inner side of its own join of
// that method.
func (m JoinMethod) Using(ts ...AdviceTarget) AdviceItem { return simpleTag(string(m), ts) }

// Together is METHOD((a b)): the targets together form the inner side of one
// join of that method.
func (m JoinMethod) Together(ts ...AdviceTarget) AdviceItem { return groupedTag(string(m), ts) }

// HashJoin is HASH_JOIN(targets...).
func HashJoin(ts ...AdviceTarget) AdviceItem { return HashJoinMethod.Using(ts...) }

func groupedTag(tag string, ts []AdviceTarget) AdviceItem {
	if len(ts) < 2 {
		return AdviceItem{err: fmt.Errorf("%w: %s((...)) needs two or more targets", ErrShape, tag)}
	}
	s, err := targetsText(ts)
	return AdviceItem{text: tag + "((" + s + "))", err: err}
}

// Gather is GATHER(targets...): one Gather node per target.
func Gather(ts ...AdviceTarget) AdviceItem { return simpleTag("GATHER", ts) }

// GatherTogether is GATHER((a b c)): a single Gather over the join.
func GatherTogether(ts ...AdviceTarget) AdviceItem { return groupedTag("GATHER", ts) }

// GatherMerge is GATHER_MERGE(targets...).
func GatherMerge(ts ...AdviceTarget) AdviceItem { return simpleTag("GATHER_MERGE", ts) }

// Partitionwise is PARTITIONWISE(targets...): each target is excluded from
// partitionwise joins.
func Partitionwise(ts ...AdviceTarget) AdviceItem { return simpleTag("PARTITIONWISE", ts) }

// PartitionwiseTogether is PARTITIONWISE((a b)): exactly that set is joined
// partitionwise.
func PartitionwiseTogether(ts ...AdviceTarget) AdviceItem { return groupedTag("PARTITIONWISE", ts) }

// SemijoinUnique is SEMIJOIN_UNIQUE(targets...).
func SemijoinUnique(ts ...AdviceTarget) AdviceItem { return simpleTag("SEMIJOIN_UNIQUE", ts) }

// SemijoinNonUnique is SEMIJOIN_NON_UNIQUE(targets...).
func SemijoinNonUnique(ts ...AdviceTarget) AdviceItem { return simpleTag("SEMIJOIN_NON_UNIQUE", ts) }

// AdviceSet is a whole advice string.
type AdviceSet struct{ items []AdviceItem }

// Advice collects tags.
func Advice(items ...AdviceItem) AdviceSet { return AdviceSet{items} }

// String renders the advice string; the first invalid item is the error.
func (a AdviceSet) String() (string, error) {
	parts := make([]string, 0, len(a.items))
	for _, it := range a.items {
		if it.err != nil {
			return "", it.err
		}
		parts = append(parts, it.text)
	}
	return strings.Join(parts, " "), nil
}

// Set is SELECT set_config('pg_plan_advice.advice', advice, local): the
// advice is a bound parameter. PostgreSQL 19 and the pg_plan_advice module
// (see LoadPlanAdvice) are required.
//
// Requires PostgreSQL 19.
func (a AdviceSet) Set(local bool) Command {
	return cmd(func(b *builder) {
		b.need(PG19, "pg_plan_advice")
		s, err := a.String()
		if err != nil {
			b.fail(err)
			return
		}
		b.write("SELECT set_config('pg_plan_advice.advice', ")
		b.arg(s)
		b.write(", ")
		b.arg(local)
		b.write(")")
	})
}

// ClearPlanAdvice is RESET pg_plan_advice.advice.
//
// Requires PostgreSQL 19.
func ClearPlanAdvice() Command {
	return cmd(func(b *builder) {
		b.need(PG19, "pg_plan_advice")
		b.write("RESET pg_plan_advice.advice")
	})
}

// LoadPlanAdvice is LOAD 'pg_plan_advice' (session scope; superuser or a
// granted privilege is needed unless the module is in
// session_preload_libraries).
//
// Requires PostgreSQL 19.
func LoadPlanAdvice() Command {
	return cmd(func(b *builder) {
		b.need(PG19, "pg_plan_advice")
		b.write("LOAD 'pg_plan_advice'")
	})
}
