// Package ltree has helpers for ltree columns (materialized paths such as
// "top.science.physics"): ancestors, descendants and pattern matches.
package ltree

import (
	"github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/pgcat/ext/ltree"
	"github.com/noobmaisfier/qlx/qt"
)

func pred(e qlx.Expr[bool]) qlx.Pred { return qlx.BoolExpr{Expr: e}.Pred() }

var lqueryType = qlx.NewPgType[qt.LQuery]("", "lquery")

// IsDescendant is path <@ of: path is of itself or lies below it.
func IsDescendant(path, of qlx.Operand[qt.LTree]) qlx.Pred {
	return pred(ltree.OpLtAtLtreeLtree2(path, of))
}

// IsAncestor is path @> of: path is of itself or lies above it.
func IsAncestor(path, of qlx.Operand[qt.LTree]) qlx.Pred {
	return pred(ltree.OpAtGtLtreeLtree2(path, of))
}

// Matches is path ~ pattern, an lquery such as "*.physics.*".
func Matches(path qlx.Operand[qt.LTree], pattern qt.LQuery) qlx.Pred {
	return pred(ltree.OpTildeLtreeLquery(path, qlx.Cast(qlx.Val(pattern), lqueryType)))
}

// Depth is the number of labels in path.
func Depth(path qlx.Operand[qt.LTree]) qlx.Expr[int32] { return ltree.Nlevel(path) }
