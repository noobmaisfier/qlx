package qlx

import "testing"

type secretStmt struct{ fail bool }

func (s secretStmt) render(b *builder) {
	b.write("SELECT ")
	b.arg("a-secret-value")
	if s.fail {
		b.fail(ErrShape)
	}
}

// The pool of hashing builders must not keep the arguments of the last
// statement alive, whichever way Resolve leaves (hit, miss, error).
func TestShapeCachePoolKeepsNoArguments(t *testing.T) {
	c := NewShapeCache(8)
	d := Dialect{Version: PG18}
	for _, st := range []secretStmt{{}, {}, {fail: true}} { // miss, hit, error
		_, _, _ = c.Resolve(d, st)
		b := hashBuilders.Get().(*builder)
		for i, sl := range b.slots[:cap(b.slots)] {
			if sl.value != nil {
				t.Errorf("fail=%v: slot %d of a pooled builder still holds %v", st.fail, i, sl.value)
			}
		}
		hashBuilders.Put(b)
	}
}
