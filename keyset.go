package qlx

// Keyset pagination.
//
// A keyset page is "the rows after the last one you saw", in a total order
// given by one or more key columns. Unlike OFFSET the cost does not grow with
// the depth of the page, and a row inserted meanwhile cannot shift the pages.
//
//	byDate, byID := qlx.KeyDesc(Orders.CreatedAt), qlx.KeyAsc(Orders.ID)
//	var after qlx.Pred // absent on the first page
//	if last != nil {   // the last row of the previous page
//		after = qlx.After(byDate.After(last.CreatedAt), byID.After(last.ID))
//	}
//	sel := qlx.Select(Orders.ID, Orders.Total).From(Orders).
//		Where(after).
//		OrderBy(qlx.KeyOrder(byDate, byID)...).
//		Limit(50)
//
// The keys are declared once, without a value: that is what the order needs.
// The value, typed like the column, is added with After when there is a
// cursor. After() with no key is an absent predicate.
//
// The key columns must be NOT NULL and, together, unique (end with the
// primary key): a NULL never compares true, and two rows with the same key
// would be skipped or repeated at a page boundary.

// KeyCol is a key column and its direction, declared once and without a
// value: it gives the ORDER BY of every page. Its After method adds the value
// of the last row seen, typed like the column, and gives the Key that After
// needs.
type KeyCol[T any] struct {
	n    node
	desc bool
}

// KeyAsc declares a key ordered ascending.
func KeyAsc[T any](e Operand[T]) KeyCol[T] { return KeyCol[T]{n: e.exprNode()} }

// KeyDesc declares a key ordered descending.
func KeyDesc[T any](e Operand[T]) KeyCol[T] { return KeyCol[T]{n: e.exprNode(), desc: true} }

// After pairs the key with its value in the last row of the previous page.
func (k KeyCol[T]) After(v T) Key { return Key{n: k.n, v: v, desc: k.desc} }

func (k KeyCol[T]) keyDef() (node, bool) { return k.n, k.desc }

// Key is a key column with the value of the last row seen.
type Key struct {
	n    node
	v    any
	desc bool
}

func (k Key) keyDef() (node, bool) { return k.n, k.desc }

// KeyDef is a KeyCol or a Key: what KeyOrder needs is the column and the
// direction, which both carry.
type KeyDef interface{ keyDef() (node, bool) }

// KeyOrder returns the ORDER BY terms of the keys, in order.
func KeyOrder(keys ...KeyDef) []OrderTerm {
	out := make([]OrderTerm, len(keys))
	for i, k := range keys {
		n, desc := k.keyDef()
		out[i] = OrderTerm{n: n, desc: desc}
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
