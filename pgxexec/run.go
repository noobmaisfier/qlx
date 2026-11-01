package pgxexec

import (
	"context"
	"errors"
	"iter"
	"reflect"
	"strings"
	"sync"
	"unsafe"

	"github.com/jackc/pgx/v5"

	"github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/qt"
)

// ---------------------------------------------------------------------------
// Result scanning: three paths, chosen once per result type
// ---------------------------------------------------------------------------
//
//  1. Positional: *R implements ScanTargets (generated rows, qlx.Tuple1..8,
//     structs marked //qlx:row). One pgx Scan call, no reflection.
//  2. Scalar: R is not a struct and the query has one column.
//  3. Plan: any other struct. Column-to-field offsets are resolved once with
//     reflection per (type, column list) and cached; per row there is only
//     pointer arithmetic, no reflection. A column with no field, or a field
//     with no column, is an error on the first row of that shape.

// targeter is what Scannable requires, without the type parameters.
type targeter interface{ ScanTargets() []any }

type scanMode uint8

const (
	modePositional scanMode = iota
	modeScalar
	modePlan
)

type rowScanner[R any] struct {
	mode scanMode
	plan *scanPlan
	buf  []any
}

func newRowScanner[R any](rows pgx.Rows) (*rowScanner[R], error) {
	var zero R
	if _, ok := any(&zero).(targeter); ok {
		return &rowScanner[R]{mode: modePositional}, nil
	}
	rt := reflect.TypeFor[R]()
	fds := rows.FieldDescriptions()
	if rt.Kind() != reflect.Struct {
		if len(fds) != 1 {
			return nil, errors.New("pgxexec: result type " + rt.String() + " is not a struct, so the query must select exactly one column")
		}
		return &rowScanner[R]{mode: modeScalar}, nil
	}
	cols := make([]string, len(fds))
	for i, fd := range fds {
		cols[i] = fd.Name
	}
	plan, err := planFor(rt, cols)
	if err != nil {
		if len(cols) == 1 {
			// A struct that is really one value (time.Time, qt.Null[T],
			// netip.Addr): scan the single column into it directly.
			return &rowScanner[R]{mode: modeScalar}, nil
		}
		return nil, err
	}
	return &rowScanner[R]{mode: modePlan, plan: plan, buf: make([]any, len(cols))}, nil
}

func (s *rowScanner[R]) scan(rows pgx.Rows, dst *R) error {
	switch s.mode {
	case modePositional:
		return rows.Scan(any(dst).(targeter).ScanTargets()...)
	case modeScalar:
		return rows.Scan(dst)
	}
	return rows.Scan(s.plan.targets(unsafe.Pointer(dst), s.buf)...)
}

// scanPlan maps result columns to struct fields.
type scanPlan struct {
	offs []uintptr
	typs []unsafe.Pointer // the *T type word for each field's T
}

type planKey struct {
	rt   reflect.Type
	cols string
}

var planCache sync.Map // planKey -> *scanPlan

func planFor(rt reflect.Type, cols []string) (*scanPlan, error) {
	key := planKey{rt, strings.Join(cols, "\x00")}
	if p, ok := planCache.Load(key); ok {
		return p.(*scanPlan), nil
	}
	p, err := buildPlan(rt, cols)
	if err != nil {
		return nil, err
	}
	planCache.Store(key, p)
	return p, nil
}

type fieldInfo struct {
	name string // lower-cased match key
	off  uintptr
	typ  reflect.Type
}

func collectFields(rt reflect.Type, base uintptr, out *[]fieldInfo) {
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct && f.Tag.Get("db") == "" {
			collectFields(f.Type, base+f.Offset, out)
			continue
		}
		if !f.IsExported() {
			continue
		}
		name := f.Name
		if tag, ok := f.Tag.Lookup("db"); ok {
			if tag == "-" {
				continue
			}
			name = tag
		}
		*out = append(*out, fieldInfo{strings.ToLower(name), base + f.Offset, f.Type})
	}
}

func buildPlan(rt reflect.Type, cols []string) (*scanPlan, error) {
	var fields []fieldInfo
	collectFields(rt, 0, &fields)
	byName := make(map[string]fieldInfo, len(fields))
	for _, f := range fields {
		byName[f.name] = f
	}
	p := &scanPlan{offs: make([]uintptr, len(cols)), typs: make([]unsafe.Pointer, len(cols))}
	used := map[string]bool{}
	for i, c := range cols {
		f, ok := byName[strings.ToLower(c)]
		if !ok {
			return nil, errors.New("pgxexec: " + rt.String() + " has no field for column " + c + " (tag a field with `db:\"" + c + "\"`)")
		}
		used[f.name] = true
		p.offs[i] = f.off
		p.typs[i] = typeWord(reflect.PointerTo(f.typ))
	}
	for _, f := range fields {
		if !used[f.name] {
			return nil, errors.New("pgxexec: field " + rt.String() + "." + f.name + " has no column in the result (tag it `db:\"-\"` to skip it)")
		}
	}
	return p, nil
}

type eface struct {
	typ, data unsafe.Pointer
}

// typeWord returns the runtime type word of a pointer type, by asking
// reflection for a value of that type once.
func typeWord(pt reflect.Type) unsafe.Pointer {
	v := reflect.New(pt.Elem()).Interface()
	return (*eface)(unsafe.Pointer(&v)).typ
}

// targets fills buf with one pointer per column, aimed into the struct at
// base, and returns it. No allocation: the interface words are written
// directly.
func (p *scanPlan) targets(base unsafe.Pointer, buf []any) []any {
	for i := range p.offs {
		e := (*eface)(unsafe.Pointer(&buf[i]))
		e.typ = p.typs[i]
		e.data = unsafe.Add(base, p.offs[i])
	}
	return buf
}

// ---------------------------------------------------------------------------
// Executors
// ---------------------------------------------------------------------------

// resolver yields the SQL and arguments to run: from a statement rendered by
// hand (stmtResolver) or from a builder looked up by shape (Client).
type resolver func(binds []qlx.Binding) (string, []any, error)

func stmtResolver(st qlx.Stmt) resolver {
	return func(binds []qlx.Binding) (string, []any, error) {
		args, err := st.Args(binds...)
		return st.SQL, args, err
	}
}

func query(ctx context.Context, db Querier, r resolver, binds []qlx.Binding) (pgx.Rows, error) {
	sql, args, err := r(binds)
	if err != nil {
		return nil, err
	}
	return db.Query(ctx, sql, args...)
}

// All runs a statement and returns every row as an R. R picks its own scan
// path (see the top of this file).
func All[R any](ctx context.Context, db Querier, st qlx.Stmt, binds ...qlx.Binding) ([]R, error) {
	return allR[R](ctx, db, stmtResolver(st), binds)
}

func allR[R any](ctx context.Context, db Querier, r resolver, binds []qlx.Binding) ([]R, error) {
	rows, err := query(ctx, db, r, binds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sc, err := newRowScanner[R](rows)
	if err != nil {
		return nil, err
	}
	var out []R
	for rows.Next() {
		var r R
		if err := sc.scan(rows, &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	Stats.rows.Add(int64(len(out)))
	return out, rows.Err()
}

// first reads at most two rows: one is the answer, a second means the query
// matched more than it should.
func first[R any](ctx context.Context, db Querier, rs resolver, binds []qlx.Binding) (r R, n int, err error) {
	rows, err := query(ctx, db, rs, binds)
	if err != nil {
		return r, 0, err
	}
	defer rows.Close()
	sc, err := newRowScanner[R](rows)
	if err != nil {
		return r, 0, err
	}
	for n < 2 && rows.Next() {
		if n == 0 {
			if err := sc.scan(rows, &r); err != nil {
				return r, 0, err
			}
		}
		n++
	}
	if err := rows.Err(); err != nil {
		return r, 0, err
	}
	Stats.rows.Add(int64(min(n, 1)))
	return r, n, nil
}

// One runs a statement that must return exactly one row. It reads at most two
// rows and reports pgx.ErrNoRows for none and pgx.ErrTooManyRows for more.
func One[R any](ctx context.Context, db Querier, st qlx.Stmt, binds ...qlx.Binding) (R, error) {
	return oneR[R](ctx, db, stmtResolver(st), binds)
}

func oneR[R any](ctx context.Context, db Querier, rs resolver, binds []qlx.Binding) (R, error) {
	r, n, err := first[R](ctx, db, rs, binds)
	switch {
	case err != nil:
		var z R
		return z, err
	case n == 0:
		var z R
		return z, pgx.ErrNoRows
	case n > 1:
		var z R
		return z, pgx.ErrTooManyRows
	}
	return r, nil
}

// Maybe runs a statement that returns zero or one row: an unset Opt for none,
// a set one for one, pgx.ErrTooManyRows for more.
func Maybe[R any](ctx context.Context, db Querier, st qlx.Stmt, binds ...qlx.Binding) (qt.Opt[R], error) {
	return maybeR[R](ctx, db, stmtResolver(st), binds)
}

func maybeR[R any](ctx context.Context, db Querier, rs resolver, binds []qlx.Binding) (qt.Opt[R], error) {
	r, n, err := first[R](ctx, db, rs, binds)
	switch {
	case err != nil:
		return qt.Opt[R]{}, err
	case n == 0:
		return qt.Opt[R]{}, nil
	case n > 1:
		return qt.Opt[R]{}, pgx.ErrTooManyRows
	}
	return qt.Of(r), nil
}

// Each streams rows through a range-over-func loop without holding them all
// in memory. The connection is released when the loop ends or breaks.
func Each[R any](ctx context.Context, db Querier, st qlx.Stmt, binds ...qlx.Binding) iter.Seq2[R, error] {
	return eachR[R](ctx, db, stmtResolver(st), binds)
}

func eachR[R any](ctx context.Context, db Querier, rs resolver, binds []qlx.Binding) iter.Seq2[R, error] {
	return func(yield func(R, error) bool) {
		var zero R
		rows, err := query(ctx, db, rs, binds)
		if err != nil {
			yield(zero, err)
			return
		}
		defer rows.Close()
		sc, err := newRowScanner[R](rows)
		if err != nil {
			yield(zero, err)
			return
		}
		for rows.Next() {
			var r R
			if err := sc.scan(rows, &r); err != nil {
				yield(zero, err)
				return
			}
			Stats.rows.Add(1)
			if !yield(r, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(zero, err)
		}
	}
}

// AllScan, OneScan and EachScan are the positional-only forms: the type
// constraint makes the compiler check that R really has ScanTargets, where
// All, One and Each would fall back to a field plan.
func AllScan[T any, PT Scannable[T]](ctx context.Context, db Querier, st qlx.Stmt, binds ...qlx.Binding) ([]T, error) {
	return All[T](ctx, db, st, binds...)
}

// OneScan is One for a type with ScanTargets.
func OneScan[T any, PT Scannable[T]](ctx context.Context, db Querier, st qlx.Stmt, binds ...qlx.Binding) (T, error) {
	return One[T](ctx, db, st, binds...)
}

// EachScan is Each for a type with ScanTargets.
func EachScan[T any, PT Scannable[T]](ctx context.Context, db Querier, st qlx.Stmt, binds ...qlx.Binding) iter.Seq2[T, error] {
	return Each[T](ctx, db, st, binds...)
}
