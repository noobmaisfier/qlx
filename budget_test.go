package qlx

import (
	"testing"
	"unsafe"
)

// Performance budgets from the plan, enforced as tests so a regression fails
// the build instead of showing up in a benchmark nobody reads.

// An AST node stays within 80 bytes: nodes are copied by value into every
// builder, so size is paid on every chained call.
func TestNodeSizeBudget(t *testing.T) {
	const max = 80
	sizes := map[string]uintptr{
		"cmpNode":      unsafe.Sizeof(cmpNode{}),
		"argNode":      unsafe.Sizeof(argNode{}),
		"fnNode":       unsafe.Sizeof(fnNode{}),
		"likeNode":     unsafe.Sizeof(likeNode{}),
		"patNode":      unsafe.Sizeof(patNode{}),
		"betweenNode":  unsafe.Sizeof(betweenNode{}),
		"castNode":     unsafe.Sizeof(castNode{}),
		"isNullNode":   unsafe.Sizeof(isNullNode{}),
		"existsNode":   unsafe.Sizeof(existsNode{}),
		"inSelectNode": unsafe.Sizeof(inSelectNode{}),
		"notAllNode":   unsafe.Sizeof(notAllNode{}),
		"collateNode":  unsafe.Sizeof(collateNode{}),
		"atZoneNode":   unsafe.Sizeof(atZoneNode{}),
		"quantNode":    unsafe.Sizeof(quantNode{}),
		"boolTestNode": unsafe.Sizeof(boolTestNode{}),
		"kwNode":       unsafe.Sizeof(kwNode("")),
	}
	for name, sz := range sizes {
		if sz > max {
			t.Errorf("%s is %d bytes, budget %d", name, sz, max)
		}
	}
}
