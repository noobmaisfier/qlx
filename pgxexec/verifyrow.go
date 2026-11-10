package pgxexec

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unsafe"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	qlx "github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/qt"
)

// Describer is satisfied by *pgx.Conn and pgx.Tx.
type Describer interface {
	Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error)
}

// VerifyRow checks, against a real server and without running the query, that
// the statement's result columns can be read into R: the same count and names
// the run-time mapping needs, and for every column a pgx scan plan for the
// destination Go type. It catches what the compiler cannot (R is not tied to
// the query) and what the analyzer cannot (a statement built across functions,
// a type the analyzer does not classify).
//
// It also checks NULLability when db can run a query (a *pgx.Conn or pgx.Tx
// can): a result column that is a plain column of a table, which the server
// reports as table OID plus attribute number, and that the table declares
// nullable, must be read into a qt.Null, a pointer, a slice, or a type that
// implements sql.Scanner; a plain int64 or string is reported. The server
// does not describe computed columns (coalesce, casts, aggregates) or the
// nullability an outer join adds, so those are not checked. A query that
// filters NULLs out itself (WHERE col IS NOT NULL) is a legitimate reason to
// read a nullable column into a plain field: pass AllowNullable to skip the
// check.
//
//	func TestUserRows(t *testing.T) {
//		conn := ... // a *pgx.Conn
//		st, _ := qlx.Render(dialect, userByID)
//		if err := pgxexec.VerifyRow[UserRow](ctx, conn, st); err != nil { t.Fatal(err) }
//	}
func VerifyRow[R any](ctx context.Context, db Describer, st qlx.Stmt, opts ...VerifyOption) error {
	var o verifyOptions
	for _, f := range opts {
		f(&o)
	}
	sd, err := db.Prepare(ctx, "", st.SQL)
	if err != nil {
		return err
	}
	n := len(sd.Fields)
	var dst R
	var targets []any
	switch rt := reflect.TypeFor[R](); {
	case implementsTargeter(&dst):
		targets = any(&dst).(targeter).ScanTargets()
		if len(targets) != n {
			return fmt.Errorf("pgxexec: %s scans %d columns (its ScanTargets) but the query returns %d", rt, len(targets), n)
		}
	case rt.Kind() != reflect.Struct:
		if n != 1 {
			return fmt.Errorf("pgxexec: %s is not a struct, so the query must return exactly one column, not %d", rt, n)
		}
		targets = []any{&dst}
	default:
		cols := make([]string, n)
		for i, f := range sd.Fields {
			cols[i] = f.Name
		}
		plan, perr := planFor(rt, cols)
		switch {
		case perr == nil:
			targets = plan.targets(unsafe.Pointer(&dst), make([]any, n))
		case n == 1:
			targets = []any{&dst} // a struct that is one value, like time.Time
		default:
			return perr
		}
	}
	m := pgtype.NewMap()
	RegisterRangeTypes(m)
	var errs []error
	for i, f := range sd.Fields {
		// The format pgx asks the server for is the one the type prefers.
		// PlanScan never returns nil: an impossible scan is a plan that fails
		// when used, and its type is unexported.
		if plan := m.PlanScan(f.DataTypeOID, m.FormatCodeForOID(f.DataTypeOID), targets[i]); plan == nil || reflect.TypeOf(plan).String() == "*pgtype.scanPlanFail" {
			errs = append(errs, fmt.Errorf("column %q (type OID %d) cannot be scanned into %T", f.Name, f.DataTypeOID, targets[i]))
		}
	}
	if !o.allowNullable {
		errs = append(errs, checkNullability(ctx, db, sd, targets)...)
	}
	return errors.Join(errs...)
}

// VerifyOption adjusts VerifyRow.
type VerifyOption func(*verifyOptions)

type verifyOptions struct{ allowNullable bool }

// AllowNullable turns off the NULLability check: nullable table columns may be
// read into plain Go types, for a query that excludes NULLs itself.
func AllowNullable() VerifyOption { return func(o *verifyOptions) { o.allowNullable = true } }

type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

var scannerType = reflect.TypeFor[sql.Scanner]()

// acceptsNull reports whether scanning SQL NULL into target works.
func acceptsNull(target any) bool {
	t := reflect.TypeOf(target)
	if t == nil || t.Kind() != reflect.Pointer {
		return true // not ours to judge
	}
	if t.Implements(scannerType) {
		return true
	}
	// qt.Range keeps NULL in a field, and the codec sets it (RangeSetNull)
	if _, ok := target.(qt.RangeWriter); ok {
		return true
	}
	switch t.Elem().Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Interface, reflect.Map:
		return true
	}
	return false
}

func checkNullability(ctx context.Context, db Describer, sd *pgconn.StatementDescription, targets []any) []error {
	qr, ok := db.(queryer)
	if !ok {
		return nil
	}
	var oids []string
	seen := map[uint32]bool{}
	for i, f := range sd.Fields {
		if f.TableOID == 0 || f.TableAttributeNumber == 0 || i >= len(targets) || acceptsNull(targets[i]) {
			continue
		}
		if !seen[f.TableOID] {
			seen[f.TableOID] = true
			oids = append(oids, strconv.FormatUint(uint64(f.TableOID), 10))
		}
	}
	if len(oids) == 0 {
		return nil
	}
	// The numbers are server-reported unsigned integers, inlined so the check
	// also works on a connection in simple protocol, which has no oid arrays.
	rows, err := qr.Query(ctx, "SELECT attrelid::int8, attnum::int4, attnotnull, attrelid::regclass::text FROM pg_attribute WHERE attnum > 0 AND NOT attisdropped AND attrelid IN ("+strings.Join(oids, ",")+")", pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		return nil // no catalog access: the check is best effort
	}
	type key struct {
		rel uint32
		att uint16
	}
	notNull := map[key]bool{}
	names := map[uint32]string{}
	for rows.Next() {
		var rel int64
		var att int32
		var nn bool
		var name string
		if rows.Scan(&rel, &att, &nn, &name) != nil {
			rows.Close()
			return nil
		}
		notNull[key{uint32(rel), uint16(att)}] = nn
		names[uint32(rel)] = name
	}
	if rows.Err() != nil {
		return nil
	}
	var errs []error
	for i, f := range sd.Fields {
		if f.TableOID == 0 || f.TableAttributeNumber == 0 || i >= len(targets) || acceptsNull(targets[i]) {
			continue
		}
		if nn, known := notNull[key{f.TableOID, f.TableAttributeNumber}]; known && !nn {
			errs = append(errs, fmt.Errorf("column %q is nullable (%s) but is read into %T, which cannot hold NULL: use qt.Null or a pointer, or pass pgxexec.AllowNullable() if the query excludes NULLs", f.Name, names[f.TableOID], targets[i]))
		}
	}
	return errs
}

func implementsTargeter(p any) bool { _, ok := p.(targeter); return ok }
