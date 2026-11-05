// Package repo holds helpers that return a Select chain, called from another
// package: the analyzer follows them through a SelectFact.
package repo

import (
	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

func ActiveUsers() q.SelectQuery { // want ActiveUsers:"select id,email"
	return q.Select(Users.ID, Users.Email).From(Users).Where(Users.Balance.Gt(0))
}

// LeftTotal selects a column of the optional side of an outer join.
func LeftTotal() q.SelectQuery { // want LeftTotal:"select email,total"
	return q.Select(Users.Email, Orders.Total).From(Users).LeftJoin(Orders).On(Orders.UserID.EqX(Users.ID))
}

// ByTier takes a parameter: the items do not depend on it.
func ByTier(tier string) q.SelectQuery { // want ByTier:"select id,email"
	return ActiveUsers().Where(Users.Tier.Eq(tier))
}

type Repo struct{}

func (Repo) Nicknames() q.SelectQuery { // want Nicknames:"select id,nickname"
	return q.Select(Users.ID, Users.Nickname).From(Users)
}

// Dynamic builds its query in several statements: no fact.
func Dynamic(cond bool) q.SelectQuery {
	s := q.Select(Users.ID).From(Users)
	if cond {
		s = s.Where(Users.Balance.Gt(0))
	}
	return s
}

// Open takes its columns from the caller: the list is not known.
func Open(items ...q.Selectable) q.SelectQuery {
	return q.Select(items...).From(Users)
}

// hidden is not exported: it cannot be called from another package.
func hidden() q.SelectQuery {
	return q.Select(Users.ID).From(Users)
}
