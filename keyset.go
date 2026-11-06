package qlx

// Keyset pagination.
//
// A keyset page is "the rows after the last one you saw", in a total order
// given by one or more key columns. Unlike OFFSET the cost does not grow with
// the depth of the page, and a row inserted meanwhile cannot shift the pages.
//
//	last := cursor // the key values of the last row of the previous page
//	keys := []qlx.Key{qlx.KeyDesc(Orders.CreatedAt, last.CreatedAt), qlx.KeyAsc(Orders.ID, last.ID)}
//	sel := qlx.Select(Orders.ID, Orders.Total).From(Orders).
//		Where(qlx.After(keys...)).
//		OrderBy(qlx.KeyOrder(keys...)...).
//		Limit(50)
//
// For the first page, build the predicate with If(hasCursor, After(...)), or
// pass no keys: After() with no key is an absent predicate.
//
// The key columns must be NOT NULL and, together, unique (end with the
// primary key): a NULL never compares true, and two rows with the same key
// would be skipped or repeated at a page boundary.

// Key is one column of a keyset order with the value of the last row seen.
type Key struct {
	n    node
	v    any
	desc bool
}

// KeyAsc is a key ordered ascending; after is its value in the last row of
// the previous page.
func KeyAsc[T any](e Operand[T], after T) Key { return Key{n: e.exprNode(), v: after} }

// KeyDesc is a key ordered descending.
func KeyDesc[T any](e Operand[T], after T) Key { return Key{n: e.exprNode(), v: after, desc: true} }

// KeyOrder returns the ORDER BY terms of the keys, in order.
func KeyOrder(keys ...Key) []OrderTerm {
	out := make([]OrderTerm, len(keys))
	for i, k := range keys {
		out[i] = OrderTerm{n: k.n, desc: k.desc}
	}
	return out
}

// After is the predicate "strictly after the given key values" in the order
// of KeyOrder. When every key has the same direction it renders one row
// comparison, ("a", "b") > ($1, $2), which PostgreSQL can answer with a
// single index range scan. With mixed directions it renders the equivalent
// expansion (a > $1) OR (a = $1 AND b < $2). With no key it is absent.
func After(keys ...Key) Pred {
	switch len(keys) {
	case 0:
		return Pred{}
	case 1:
		return Pred{cmpNode{l: keys[0].n, op: keyOp(keys[0]), r: argNode{keys[0].v}}}
	}
	same := true
	for _, k := range keys[1:] {
		if k.desc != keys[0].desc {
			same = false
			break
		}
	}
	if same {
		n := rowCmpNode{op: keyOp(keys[0])}
		for _, k := range keys {
			n.l = append(n.l, k.n)
			n.r = append(n.r, argNode{k.v})
		}
		return Pred{n}
	}
	alts := make([]Pred, len(keys))
	for i := range keys {
		parts := make([]Pred, 0, i+1)
		for _, e := range keys[:i] {
			parts = append(parts, Pred{cmpNode{l: e.n, op: "=", r: argNode{e.v}}})
		}
		parts = append(parts, Pred{cmpNode{l: keys[i].n, op: keyOp(keys[i]), r: argNode{keys[i].v}}})
		alts[i] = And(parts...)
	}
	return Or(alts...)
}

func keyOp(k Key) string {
	if k.desc {
		return "<"
	}
	return ">"
}

// rowCmpNode is (l1, l2, ...) op (r1, r2, ...).
type rowCmpNode struct {
	l, r []node
	op   string
}

func (n rowCmpNode) render(b *builder) {
	list := func(ns []node) {
		b.write("(")
		for i, x := range ns {
			if i > 0 {
				b.write(", ")
			}
			x.render(b)
		}
		b.write(")")
	}
	list(n.l)
	b.write(" ")
	b.write(n.op)
	b.write(" ")
	list(n.r)
}
