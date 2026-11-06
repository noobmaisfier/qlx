package qlx

import "github.com/noobmaisfier/qlx/qt"

// Optional predicates: the filters of a list endpoint, where each query
// parameter may be absent. An unset option gives an absent predicate, which
// Where, And and Or skip (a Where whose predicates are all absent writes no
// WHERE at all), so the statement is built without an if per field:
//
//	sel.Where(
//	    Users.Tier.EqIf(f.Tier),
//	    Users.Balance.GteIf(f.MinBalance),
//	    Users.CreatedAt.BetweenIf(f.From, f.To),
//	    Users.Email.ContainsFoldIf(f.Search),
//	    Users.Tier.InIf(f.Tiers),
//	)
//
// Every combination of present predicates is a different statement shape, and
// the shape cache keeps each one: with n optional filters there can be 2^n
// shapes (watch ShapeCache.OnThrash). Four or five filters are nothing; thirty
// freely combined are not.

// When applies pred to the value of an option, and gives an absent predicate
// when the option is unset. It turns any predicate method into an optional one:
//
//	q.When(f.Name, Users.Email.StartsWith)
//	q.When(f.Domain, func(d string) q.Pred { return Users.Email.ILike("%@" + d) })
//
// The named methods (GtIf, ContainsFoldIf, ...) are When applied to the common
// predicates; use When for the rest.
func When[T any](o qt.Opt[T], pred func(T) Pred) Pred {
	if !o.Set {
		return Pred{}
	}
	return pred(o.V)
}

// whenText is When for a text search: an empty string is as good as absent (a
// search box that submits nothing filters nothing). Eq("") and EqIf(Some("")) keep
// meaning the empty string.
func whenText(o qt.Opt[string], pred func(string) Pred) Pred {
	if !o.Set || o.V == "" {
		return Pred{}
	}
	return pred(o.V)
}

// NeIf is Ne, absent when the option is unset.
func (e Expr[T]) NeIf(o qt.Opt[T]) Pred { return When(o, e.Ne) }

// InIf is Any, absent when the option is unset. A set but empty list matches
// nothing, as In does: "in none of these" is not "no filter".
func (e Expr[T]) InIf(o qt.Opt[[]T]) Pred { return When(o, e.Any) }

// IsNullIf filters on NULL: Some(true) is IS NULL, Some(false) is IS NOT NULL,
// unset is absent (for a ?archived=true|false parameter).
func (e Expr[T]) IsNullIf(o qt.Opt[bool]) Pred {
	switch {
	case !o.Set:
		return Pred{}
	case o.V:
		return e.IsNull()
	}
	return e.IsNotNull()
}

// GtIf, GteIf, LtIf and LteIf are Gt, Gte, Lt and Lte, absent when the option
// is unset. BetweenIf takes both bounds as options and keeps the ones that are
// set, so one-sided ranges (?from=...) need no special case.
func (e NumExpr[T]) GtIf(o qt.Opt[T]) Pred  { return When(o, e.Gt) }
func (e NumExpr[T]) GteIf(o qt.Opt[T]) Pred { return When(o, e.Gte) }
func (e NumExpr[T]) LtIf(o qt.Opt[T]) Pred  { return When(o, e.Lt) }
func (e NumExpr[T]) LteIf(o qt.Opt[T]) Pred { return When(o, e.Lte) }
func (e NumExpr[T]) BetweenIf(lo, hi qt.Opt[T]) Pred {
	return And(e.GteIf(lo), e.LteIf(hi))
}

// GtIf, GteIf, LtIf, LteIf and BetweenIf: see NumExpr.
func (e TimeExpr[T]) GtIf(o qt.Opt[T]) Pred  { return When(o, e.Gt) }
func (e TimeExpr[T]) GteIf(o qt.Opt[T]) Pred { return When(o, e.Gte) }
func (e TimeExpr[T]) LtIf(o qt.Opt[T]) Pred  { return When(o, e.Lt) }
func (e TimeExpr[T]) LteIf(o qt.Opt[T]) Pred { return When(o, e.Lte) }
func (e TimeExpr[T]) BetweenIf(lo, hi qt.Opt[T]) Pred {
	return And(e.GteIf(lo), e.LteIf(hi))
}

// The text searches below are absent when the option is unset or empty.
// ContainsIf and ContainsFoldIf escape %, _ and \ in the text, so what the user
// typed is matched literally (use them for a search box); LikeIf and ILikeIf take
// a pattern the caller wrote.
func (e TextExpr[T]) ContainsIf(o qt.Opt[string]) Pred     { return whenText(o, e.Contains) }
func (e TextExpr[T]) ContainsFoldIf(o qt.Opt[string]) Pred { return whenText(o, e.ContainsFold) }
func (e TextExpr[T]) HasPrefixIf(o qt.Opt[string]) Pred    { return whenText(o, e.HasPrefix) }
func (e TextExpr[T]) LikeIf(o qt.Opt[string]) Pred         { return whenText(o, e.Like) }
func (e TextExpr[T]) ILikeIf(o qt.Opt[string]) Pred        { return whenText(o, e.ILike) }
