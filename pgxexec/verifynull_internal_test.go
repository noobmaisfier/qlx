package pgxexec

import (
	"testing"

	"github.com/noobmaisfier/qlx/qt"
)

// qt.Range holds NULL (its Null field), so VerifyRow must not call a nullable
// range column unreadable; a plain int32 still cannot hold it.
func TestAcceptsNullForRange(t *testing.T) {
	var r qt.Range[int32]
	var n int32
	var m qt.Multirange[int32]
	if !acceptsNull(&r) || !acceptsNull(&m) {
		t.Error("a range or multirange can hold NULL")
	}
	if acceptsNull(&n) {
		t.Error("an int32 cannot hold NULL")
	}
}
