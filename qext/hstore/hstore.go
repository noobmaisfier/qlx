// Package hstore has helpers for hstore columns (flat key/value maps).
package hstore

import (
	"github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/pgcat/ext/hstore"
	"github.com/noobmaisfier/qlx/qt"
)

// Get is col -> key: the value, or NULL when the key is missing.
func Get(col qlx.Operand[qt.Hstore], key string) qlx.Expr[qt.Null[string]] {
	return hstore.OpMinusGtHstoreText2(col, qlx.Val(key))
}

// HasKey is exist(col, key).
func HasKey(col qlx.Operand[qt.Hstore], key string) qlx.Pred {
	return qlx.BoolExpr{Expr: hstore.Exist(col, qlx.Val(key))}.Pred()
}

// Contains is col @> other: every pair of other is in col.
func Contains(col qlx.Operand[qt.Hstore], other qt.Hstore) qlx.Pred {
	return qlx.BoolExpr{Expr: hstore.OpAtGtHstoreHstore(col, qlx.Val(other))}.Pred()
}

// Merge is a || b; on a key in both, b wins.
func Merge(a qlx.Operand[qt.Hstore], b qt.Hstore) qlx.Expr[qt.Hstore] {
	return hstore.OpPipePipeHstoreHstore(a, qlx.Val(b))
}
