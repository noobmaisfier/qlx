package qlx

import "fmt"

// Typed-string enums.
//
// CopyFormat, LockMode, Privilege and the other string types below are
// written into the SQL text. Go lets any string be converted to them
// (LockMode("...")), so each value is checked against its closed set when the
// statement is rendered; anything else is an ErrShape error and nothing is
// written. This keeps the promise that no API takes SQL text.

func badEnum(b *builder, what, v string) {
	b.fail(fmt.Errorf("%w: %q is not a valid %s", ErrShape, v, what))
}

func (f CopyFormat) valid() bool {
	switch f {
	case CopyText, CopyCSV, CopyBinary, CopyJSON:
		return true
	}
	return false
}

func (m OnErrorMode) valid() bool {
	switch m {
	case OnErrorStop, OnErrorIgnore, OnErrorSetNull:
		return true
	}
	return false
}

func (v LogVerbosity) valid() bool {
	switch v {
	case LogDefault, LogVerbose, LogSilent:
		return true
	}
	return false
}

func (m LockMode) valid() bool {
	switch m {
	case LockAccessShare, LockRowShare, LockRowExclusive, LockShareUpdateExclusive,
		LockShare, LockShareRowExclusive, LockExclusive, LockAccessExclusive:
		return true
	}
	return false
}

func (p Privilege) valid() bool {
	switch p {
	case PrivSelect, PrivInsert, PrivUpdate, PrivDelete, PrivTruncate, PrivReferences, PrivTrigger, PrivAll:
		return true
	}
	return false
}

func (i Isolation) valid() bool {
	switch i {
	case "", ReadCommitted, RepeatableRead, Serializable:
		return true
	}
	return false
}

func (k PatternKind) valid() bool {
	switch k {
	case PatLike, PatNotLike, PatILike, PatNotILike, PatSimilar, PatNotSimilar:
		return true
	}
	return false
}

func (o CmpOp) valid() bool {
	switch o {
	case OpEq, OpNe, OpLt, OpLte, OpGt, OpGte:
		return true
	}
	return false
}

func (f ExtractField) valid() bool {
	switch f {
	case FieldYear, FieldMonth, FieldDay, FieldHour, FieldMinute, FieldSecond,
		FieldDOW, FieldDOY, FieldWeek, FieldQuarter, FieldEpoch:
		return true
	}
	return false
}

func (k JSONKind) valid() bool {
	switch k {
	case JSONAny, JSONObjectKind, JSONArrayKind, JSONScalarKind:
		return true
	}
	return false
}

func (k XMLKind) valid() bool {
	switch k {
	case XMLDocument, XMLContent:
		return true
	}
	return false
}

// badCopyOption is a COPY option that fails when the statement is rendered.
func badCopyOption(what, v string) CopyOption {
	return CopyOption{what: what, f: func(b *builder) { badEnum(b, what, v) }}
}

// renderTxMode writes the modes of BEGIN or SET TRANSACTION.
func renderTxMode(b *builder, m TxMode) {
	if !m.Isolation.valid() {
		badEnum(b, "isolation level", string(m.Isolation))
		return
	}
	if m.Isolation != "" {
		b.write(" ISOLATION LEVEL " + string(m.Isolation))
	}
	if m.ReadOnly {
		b.write(" READ ONLY")
	}
	if m.Deferrable {
		// PostgreSQL accepts DEFERRABLE anywhere and silently ignores it
		// unless the transaction is SERIALIZABLE READ ONLY: refused here.
		if m.Isolation != Serializable || !m.ReadOnly {
			b.fail(fmt.Errorf("%w: DEFERRABLE only applies to SERIALIZABLE READ ONLY transactions", ErrShape))
			return
		}
		b.write(" DEFERRABLE")
	}
}
