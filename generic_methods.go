package qlx

import "github.com/noobmaisfier/qlx/qt"

// Go 1.27 generic methods: the target type is a method type parameter, so a
// conversion reads left to right instead of wrapping the operand.
//
//	users.Balance.Cast[float64](qlx.PgFloat8)   // CAST(balance AS float8)
//
// The package-level Cast and CastN remain for operands that are not an Expr.

// Cast converts the expression: CAST(e AS type).
func (e Expr[T]) Cast[To any](t PgType[To]) Expr[To] {
	return Expr[To]{castNode{e.n, t}}
}

// CastN is Cast for a result that may be NULL.
func (e Expr[T]) CastN[To any](t PgType[To]) Expr[qt.Null[To]] {
	return Expr[qt.Null[To]]{castNode{e.n, t}}
}

// AtTimeZone is x AT TIME ZONE zone. A timestamptz becomes a timestamp and the
// reverse, so the result type is a method type parameter (Go 1.27):
//
//	e.AtTimeZone[qt.LocalTime]("UTC")
func (e TimeExpr[T]) AtTimeZone[To TimeT](zone string) TimeExpr[To] {
	return TimeExpr[To]{Expr[To]{atZoneNode{e.n, argNode{zone}}}}
}

// AtTimeZoneX is AtTimeZone with the zone taken from another expression.
func (e TimeExpr[T]) AtTimeZoneX[To TimeT](zone Operand[string]) TimeExpr[To] {
	return TimeExpr[To]{Expr[To]{atZoneNode{e.n, zone.exprNode()}}}
}

// AtLocal is x AT LOCAL: convert to the session time zone.
func (e TimeExpr[T]) AtLocal[To TimeT]() TimeExpr[To] {
	return TimeExpr[To]{Expr[To]{atZoneNode{e.n, nil}}}
}
