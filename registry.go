package qlx

import (
	"fmt"
	"sort"
	"sync"
)

// Registered is a named statement known to the registry.
type Registered struct {
	Name string
	Stmt Statement
}

var (
	regMu sync.Mutex
	reg   = map[string]Statement{}
)

// Register records a statement under a unique name and returns it unchanged,
// so a query can be declared and registered in one expression:
//
//	var ActiveUsers = qlx.Register("active_users", qlx.Select(...)...)
//
// The registry exists so tests can render, snapshot and PREPARE every query
// the program contains (see package qtest). Registering the same name twice
// panics: it is a programming error that would make snapshots ambiguous.
func Register[S Statement](name string, s S) S {
	regMu.Lock()
	defer regMu.Unlock()
	if name == "" {
		panic("qlx: Register with an empty name")
	}
	if _, dup := reg[name]; dup {
		panic(fmt.Sprintf("qlx: statement %q registered twice", name))
	}
	reg[name] = s
	return s
}

// RegisteredStatements lists every registered statement sorted by name.
func RegisteredStatements() []Registered {
	regMu.Lock()
	defer regMu.Unlock()
	out := make([]Registered, 0, len(reg))
	for n, s := range reg {
		out = append(out, Registered{n, s})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
