package optfield

import "github.com/noobmaisfier/qlx/qt"

// V031: an Opt[Null[T]] field needs omitzero, or an unset one cannot be
// marshaled (qt.ErrUnsetOptNull).
type PatchUser struct {
	Nick  qt.Opt[qt.Null[string]]  `json:"nick"` // want `V031 Nick is a qt.Opt\[qt.Null\[T\]\] without omitzero`
	Bio   qt.Opt[qt.Null[string]]  ``            // want `V031 Bio is a qt.Opt`
	Age   qt.Opt[qt.Null[int32]]   // want `V031 Age is a qt.Opt`
	Score qt.Opt[qt.Null[float64]] `json:"score,omitempty"`        // want `V031 Score is a qt.Opt.*json:"score,omitzero"`
	Ok    qt.Opt[qt.Null[string]]  `json:"ok,omitzero"`            // fine
	Ok2   qt.Opt[qt.Null[string]]  `json:"ok2,omitempty,omitzero"` // fine
	Skip  qt.Opt[qt.Null[string]]  `json:"-"`                      // fine: not marshaled
	skip  qt.Opt[qt.Null[string]]  `json:"skip"`                   // fine: unexported
	Plain qt.Opt[string]           `json:"plain"`                  // fine: an unset Opt[T] marshals null and reads back unset
	Null  qt.Null[string]          `json:"null"`                   // fine: not an Opt
	Ign   qt.Opt[qt.Null[string]]  `json:"ign"`                    //qlx:ignore decode-only body, never marshaled
}

type Alias = qt.Opt[qt.Null[string]]

type ViaAlias struct {
	A Alias `json:"a"` // want `V031 A is a qt.Opt`
}

func anonymous() {
	_ = struct {
		N qt.Opt[qt.Null[bool]] `json:"n"` // want `V031 N is a qt.Opt`
	}{}
}

type Both struct {
	A, B qt.Opt[qt.Null[string]] `json:"x"` // want `V031 A is a qt.Opt`
}
