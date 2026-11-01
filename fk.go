package qlx

import "fmt"

// FKColumns pairs one column of the referencing table with the column it
// references. Build pairs with FKPair; generated code does it for you.
type FKColumns struct{ from, to colRef }

// FKPair pairs a referencing column with the referenced one.
func FKPair(from, to ColumnRef) FKColumns {
	return FKColumns{from: from.columnRef(), to: to.columnRef()}
}

// ForeignKey is a foreign-key relationship between two table references. The
// generated methods (Orders.FKCustomer(Customers)) return one bound to the
// exact references passed, so aliased and self joins work: the condition is
// built from the references you give, never from table names.
type ForeignKey struct {
	Name  string
	pairs []FKColumns
}

// NewForeignKey builds a ForeignKey from column pairs.
func NewForeignKey(name string, pairs ...FKColumns) ForeignKey {
	return ForeignKey{Name: name, pairs: pairs}
}

// Pred is the join condition: every pair equal, joined with AND. A key with
// no pairs yields an absent predicate, which On rejects.
func (fk ForeignKey) Pred() Pred {
	ps := make([]Pred, len(fk.pairs))
	for i, p := range fk.pairs {
		ps[i] = Pred{cmpNode{l: p.from, op: "=", r: p.to}}
	}
	return And(ps...)
}

// OnFK joins on a foreign key: ON referencing.col = referenced.col [AND ...].
// A key without columns fails at render time with ErrShape.
func (j JoinBuilder) OnFK(fk ForeignKey) SelectQuery {
	p := fk.Pred()
	if !p.valid() {
		p = Pred{failNode{fmt.Errorf("%w: foreign key %q has no columns", ErrShape, fk.Name)}}
	}
	return j.On(p)
}
