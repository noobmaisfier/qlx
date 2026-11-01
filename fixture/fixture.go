// Package fixture is hand-written in the shape the generator will emit for
// schema.sql. It exists so the core library can be tested end to end before
// the generator exists, and it documents the generated-code contract.
package fixture

import (
	_ "embed"
	"time"

	"github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/qt"
)

// SchemaSQL is the DDL the fixture tables mirror.
//
//go:embed schema.sql
var SchemaSQL string

// UsersT is public.users.
type UsersT struct {
	qlx.TableRef
	ID        qlx.NumCol[int64]
	Email     qlx.TextCol[string]
	Nickname  qlx.TextCol[qt.Null[string]]
	Tier      qlx.Col[string]
	Balance   qlx.NumCol[int64]
	CreatedAt qlx.TimeCol[time.Time]
}

func newUsers(alias string) UsersT {
	t := qlx.TableRef{Schema: "public", Name: "users", Alias: alias, Required: []string{"email"}, PK: []string{"id"}}
	return UsersT{
		TableRef:  t,
		ID:        qlx.NewNumCol[int64](t, "id"),
		Email:     qlx.NewTextCol[string](t, "email"),
		Nickname:  qlx.NewTextCol[qt.Null[string]](t, "nickname"),
		Tier:      qlx.NewCol[string](t, "tier"),
		Balance:   qlx.NewNumCol[int64](t, "balance"),
		CreatedAt: qlx.NewTimeCol[time.Time](t, "created_at"),
	}
}

// Users is the table symbol; Users.As("u") returns an aliased copy.
var Users = newUsers("")

// As returns the table under an alias, with every column rebound to it.
func (UsersT) As(alias string) UsersT { return newUsers(alias) }

// OrdersT is public.orders.
type OrdersT struct {
	qlx.TableRef
	ID     qlx.NumCol[int64]
	UserID qlx.NumCol[int64]
	Total  qlx.NumCol[int64]
	Status qlx.Col[string]
}

func newOrders(alias string) OrdersT {
	t := qlx.TableRef{Schema: "public", Name: "orders", Alias: alias, Required: []string{"user_id", "total", "status"}, PK: []string{"id"}}
	return OrdersT{
		TableRef: t,
		ID:       qlx.NewNumCol[int64](t, "id"),
		UserID:   qlx.NewNumCol[int64](t, "user_id"),
		Total:    qlx.NewNumCol[int64](t, "total"),
		Status:   qlx.NewCol[string](t, "status"),
	}
}

// Orders is the table symbol.
var Orders = newOrders("")

// As returns the table under an alias.
func (OrdersT) As(alias string) OrdersT { return newOrders(alias) }

// AccountsT is public.accounts.
type AccountsT struct {
	qlx.TableRef
	ID      qlx.NumCol[int64]
	Balance qlx.NumCol[int64]
	Owner   qlx.TextCol[qt.Null[string]]
}

func newAccounts(alias string) AccountsT {
	t := qlx.TableRef{Schema: "public", Name: "accounts", Alias: alias, Required: []string{"id", "balance"}, PK: []string{"id"}}
	return AccountsT{
		TableRef: t,
		ID:       qlx.NewNumCol[int64](t, "id"),
		Balance:  qlx.NewNumCol[int64](t, "balance"),
		Owner:    qlx.NewTextCol[qt.Null[string]](t, "owner"),
	}
}

// Accounts is the table symbol.
var Accounts = newAccounts("")

// As returns the table under an alias.
func (AccountsT) As(alias string) AccountsT { return newAccounts(alias) }

// FKOrdersUserIDFkey is orders.user_id -> users.id (generated code emits this
// for every foreign key whose target table is generated too).
func (t OrdersT) FKOrdersUserIDFkey(to UsersT) qlx.ForeignKey {
	return qlx.NewForeignKey("orders_user_id_fkey", qlx.FKPair(t.UserID, to.ID))
}
