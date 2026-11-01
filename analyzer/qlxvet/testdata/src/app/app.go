package app

import (
	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

var A = q.Register("a", q.Select(Users.ID).From(Users))
var B = q.Register("a", q.Select(Orders.ID).From(Orders)) // want `V003 statement name "a" is already registered`

func discard() {
	s := q.Select(Users.ID).From(Users)
	s.Where(Users.ID.Eq(1)) // want `V001`
	_ = s
}

func render(s q.Statement) {
	q.Render(q.Dialect{}, s)        // want `V004`
	_, _ = q.Render(q.Dialect{}, s) // want `V004`
	q.Render(q.Dialect{}, s)        //qlx:ignore fixture exercises suppression
	/* want `V010` */        //qlx:ignore
	q.Render(q.Dialect{}, s) // want `V004`
}

var scopeBad = q.Select(Users.ID, Orders.ID).From(Users) // want `V006 column Orders.ID`

var scopeOK = q.Select(Users.ID, Orders.ID).From(Users).Join(Orders).On(Orders.UserID.EqX(Users.ID))

var groupBad = q.Select(Users.Tier, q.CountAll()).From(Users) // want `V007`

var groupOK = q.Select(Users.Tier, q.CountAll()).From(Users).GroupBy(Users.Tier)

var insBad = q.InsertInto(Users).Row(Users.Balance.To(1)) // want `V008 INSERT INTO users does not set required column\(s\) email`
