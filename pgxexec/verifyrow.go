package pgxexec

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"unsafe"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	qlx "github.com/noobmaisfier/qlx"
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
// a type the analyzer does not classify). It cannot see NULLs: a nullable
// column read into a plain field fails only on the row that holds NULL.
//
//	func TestUserRows(t *testing.T) {
//		conn := ... // a *pgx.Conn
//		st, _ := qlx.Render(dialect, userByID)
//		if err := pgxexec.VerifyRow[UserRow](ctx, conn, st); err != nil { t.Fatal(err) }
//	}
func VerifyRow[R any](ctx context.Context, db Describer, st qlx.Stmt) error {
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
	return errors.Join(errs...)
}

func implementsTargeter(p any) bool { _, ok := p.(targeter); return ok }
