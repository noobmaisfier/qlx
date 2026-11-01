// Package app2 uses the generated package through a qualified import, the
// usual way; the rules must fire exactly as with a dot import.
package app2

import (
	q "github.com/noobmaisfier/qlx"
	fx "github.com/noobmaisfier/qlx/fixture"
)

var scopeBad = q.Select(fx.Users.ID, fx.Orders.ID).From(fx.Users) // want `V006 column fx.Orders.ID`

var scopeOK = q.Select(fx.Users.ID, fx.Orders.ID).From(fx.Users).Join(fx.Orders).On(fx.Orders.UserID.EqX(fx.Users.ID))

var groupBad = q.Select(fx.Users.Tier, q.CountAll()).From(fx.Users) // want `V007`

var insBad = q.InsertInto(fx.Users).Row(fx.Users.Balance.To(1)) // want `V008 INSERT INTO users does not set required column\(s\) email`
