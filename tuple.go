package qlx

// Tuples are result rows for queries that select a few typed expressions. The
// type arguments are the Go types of the selected items, in order, so a row is
// checked against the query by the compiler and scanned without reflection:
//
//	rows, err := pgxexec.AllScan[qlx.Tuple2[int64, string]](ctx, db, stmt)

// Tuple2 is a row of 2 values.
type Tuple2[A, B any] struct {
	V1 A
	V2 B
}

// ScanTargets returns pointers to the fields, in column order.
func (t *Tuple2[A, B]) ScanTargets() []any {
	return []any{&t.V1, &t.V2}
}

// Tuple3 is a row of 3 values.
type Tuple3[A, B, C any] struct {
	V1 A
	V2 B
	V3 C
}

// ScanTargets returns pointers to the fields, in column order.
func (t *Tuple3[A, B, C]) ScanTargets() []any {
	return []any{&t.V1, &t.V2, &t.V3}
}

// Tuple4 is a row of 4 values.
type Tuple4[A, B, C, D any] struct {
	V1 A
	V2 B
	V3 C
	V4 D
}

// ScanTargets returns pointers to the fields, in column order.
func (t *Tuple4[A, B, C, D]) ScanTargets() []any {
	return []any{&t.V1, &t.V2, &t.V3, &t.V4}
}

// Tuple5 is a row of 5 values.
type Tuple5[A, B, C, D, E any] struct {
	V1 A
	V2 B
	V3 C
	V4 D
	V5 E
}

// ScanTargets returns pointers to the fields, in column order.
func (t *Tuple5[A, B, C, D, E]) ScanTargets() []any {
	return []any{&t.V1, &t.V2, &t.V3, &t.V4, &t.V5}
}

// Tuple6 is a row of 6 values.
type Tuple6[A, B, C, D, E, F any] struct {
	V1 A
	V2 B
	V3 C
	V4 D
	V5 E
	V6 F
}

// ScanTargets returns pointers to the fields, in column order.
func (t *Tuple6[A, B, C, D, E, F]) ScanTargets() []any {
	return []any{&t.V1, &t.V2, &t.V3, &t.V4, &t.V5, &t.V6}
}

// Tuple7 is a row of 7 values.
type Tuple7[A, B, C, D, E, F, G any] struct {
	V1 A
	V2 B
	V3 C
	V4 D
	V5 E
	V6 F
	V7 G
}

// ScanTargets returns pointers to the fields, in column order.
func (t *Tuple7[A, B, C, D, E, F, G]) ScanTargets() []any {
	return []any{&t.V1, &t.V2, &t.V3, &t.V4, &t.V5, &t.V6, &t.V7}
}

// Tuple8 is a row of 8 values.
type Tuple8[A, B, C, D, E, F, G, H any] struct {
	V1 A
	V2 B
	V3 C
	V4 D
	V5 E
	V6 F
	V7 G
	V8 H
}

// ScanTargets returns pointers to the fields, in column order.
func (t *Tuple8[A, B, C, D, E, F, G, H]) ScanTargets() []any {
	return []any{&t.V1, &t.V2, &t.V3, &t.V4, &t.V5, &t.V6, &t.V7, &t.V8}
}

// Tuple1 is a one-column row; use it to read a single value.
type Tuple1[A any] struct{ V1 A }

// ScanTargets returns a pointer to the field.
func (t *Tuple1[A]) ScanTargets() []any { return []any{&t.V1} }
