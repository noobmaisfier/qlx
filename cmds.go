package qlx

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Command is a non-query statement (utility and session commands). It is a
// Statement like any query, so Render and SQL work on it.
type Command struct{ f func(b *builder) }

func (c Command) render(b *builder) { c.f(b) }

func cmd(f func(b *builder)) Command { return Command{f} }

var safeLit = regexp.MustCompile(`^[\x20-\x7E]*$`)

// safeLiteral writes a quoted literal for the few places where PostgreSQL's
// grammar does not accept a parameter (COPY options, transaction ids). Only
// printable ASCII without quote or backslash is accepted; anything else is
// refused instead of escaped.
func (b *builder) safeLiteral(s string) {
	if !safeLit.MatchString(s) || strings.ContainsAny(s, `'\`) {
		b.fail(fmt.Errorf("%w: %q cannot be written as a literal here", ErrInvalidIdent, s))
		return
	}
	b.write("'" + s + "'")
}

func (b *builder) int(n int64) { b.write(strconv.FormatInt(n, 10)) }

func (b *builder) identList(names []string) {
	joinParts(b, len(names), func(i int) { b.ident(names[i]) })
}

func colNamesOf(t TableRef, cols []ColumnRef, b *builder) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		r := c.columnRef()
		if !sameTable(r.t, t) {
			b.fail(fmt.Errorf("%w: column %s.%s is not a column of %s", ErrScope, r.t.key(), r.name, t.key()))
		}
		out[i] = r.name
	}
	return out
}

// ---------------------------------------------------------------------------
// EXPLAIN
// ---------------------------------------------------------------------------

// ExplainOption is one EXPLAIN option.
type ExplainOption struct {
	kw           string
	min          int
	needsAnalyze bool // the server refuses it without ANALYZE
	noAnalyze    bool // the server refuses it together with ANALYZE
}

// EXPLAIN options. Buffers is on by default with Analyze since PostgreSQL 18;
// IO is new in PostgreSQL 19; Memory needs 17; GenericPlan needs 16.
var (
	Analyze         = ExplainOption{kw: "ANALYZE"}
	Verbose         = ExplainOption{kw: "VERBOSE"}
	Buffers         = ExplainOption{kw: "BUFFERS"}
	NoBuffers       = ExplainOption{kw: "BUFFERS FALSE"}
	Wal             = ExplainOption{kw: "WAL", needsAnalyze: true}
	Settings        = ExplainOption{kw: "SETTINGS"}
	GenericPlan     = ExplainOption{kw: "GENERIC_PLAN", min: 16, noAnalyze: true}
	Memory          = ExplainOption{kw: "MEMORY", min: 17}
	IO              = ExplainOption{kw: "IO", min: 19, needsAnalyze: true}
	NoCosts         = ExplainOption{kw: "COSTS FALSE"}
	NoTiming        = ExplainOption{kw: "TIMING FALSE"}
	NoSummary       = ExplainOption{kw: "SUMMARY FALSE"}
	SerializeText   = ExplainOption{kw: "SERIALIZE TEXT", min: 17, needsAnalyze: true}
	SerializeBinary = ExplainOption{kw: "SERIALIZE BINARY", min: 17, needsAnalyze: true}
	FormatText      = ExplainOption{kw: "FORMAT TEXT"}
	FormatJSON      = ExplainOption{kw: "FORMAT JSON"}
	FormatYAML      = ExplainOption{kw: "FORMAT YAML"}
	FormatXML       = ExplainOption{kw: "FORMAT XML"}
)

// Explain wraps any statement in EXPLAIN.
func Explain(s Statement, opts ...ExplainOption) Command {
	return cmd(func(b *builder) {
		analyze := false
		for _, o := range opts {
			analyze = analyze || o.kw == "ANALYZE"
		}
		for _, o := range opts {
			switch {
			case o.needsAnalyze && !analyze:
				b.fail(fmt.Errorf("%w: EXPLAIN option %s requires ANALYZE", ErrShape, o.kw))
				return
			case o.noAnalyze && analyze:
				b.fail(fmt.Errorf("%w: EXPLAIN options ANALYZE and %s cannot be used together", ErrShape, o.kw))
				return
			}
		}
		b.write("EXPLAIN")
		if len(opts) > 0 {
			b.write(" (")
			for i, o := range opts {
				if i > 0 {
					b.write(", ")
				}
				if o.min > 0 {
					b.need(o.min, "EXPLAIN option "+o.kw)
				}
				b.write(o.kw)
			}
			b.write(")")
		}
		b.write(" ")
		s.render(b)
	})
}

func (q SelectQuery) Explain(opts ...ExplainOption) Command { return Explain(q, opts...) }
func (q SetQuery) Explain(opts ...ExplainOption) Command    { return Explain(q, opts...) }
func (q InsertQuery) Explain(opts ...ExplainOption) Command { return Explain(q, opts...) }
func (q UpdateQuery) Explain(opts ...ExplainOption) Command { return Explain(q, opts...) }
func (q DeleteQuery) Explain(opts ...ExplainOption) Command { return Explain(q, opts...) }
func (q MergeQuery) Explain(opts ...ExplainOption) Command  { return Explain(q, opts...) }

// ---------------------------------------------------------------------------
// PREPARE / EXECUTE / DEALLOCATE
// ---------------------------------------------------------------------------

// ParamDecl declares the PostgreSQL type of a typed Param for PREPARE.
type ParamDecl struct {
	id int
	t  typeRef
}

// Declare pairs a Param with its PostgreSQL type.
func Declare[T any](p Param[T], t PgType[T]) ParamDecl { return ParamDecl{p.id, t} }

// Prepare renders PREPARE name (types) AS statement. The statement may only
// use typed Params (no constants), because the server owns the placeholders;
// the declared types are ordered by first use of each Param.
func Prepare(name string, s Statement, decls ...ParamDecl) Command {
	decls = slices.Clone(decls)
	return cmd(func(b *builder) {
		nb := newBuilder(Dialect{Version: b.version})
		s.render(nb)
		if nb.err != nil {
			b.fail(nb.err)
			return
		}
		types := make([]typeRef, len(nb.slots))
		for i, sl := range nb.slots {
			if sl.param == 0 {
				b.fail(fmt.Errorf("%w: PREPARE statements may only use typed Params, found a constant at $%d", ErrShape, i+1))
				return
			}
			j := slices.IndexFunc(decls, func(d ParamDecl) bool { return d.id == sl.param })
			if j < 0 {
				b.fail(fmt.Errorf("%w: parameter $%d has no Declare", ErrParams, i+1))
				return
			}
			types[i] = decls[j].t
		}
		b.write("PREPARE ")
		b.ident(name)
		if len(types) > 0 {
			b.write(" (")
			for i, t := range types {
				if i > 0 {
					b.write(", ")
				}
				t.renderType(b)
			}
			b.write(")")
		}
		b.write(" AS " + nb.sb.String())
	})
}

// Execute renders EXECUTE name (args). The arguments are bound parameters.
func Execute(name string, args ...any) Command {
	args = slices.Clone(args)
	return cmd(func(b *builder) {
		b.write("EXECUTE ")
		b.ident(name)
		if len(args) > 0 {
			b.write(" (")
			joinParts(b, len(args), func(i int) { b.typedArg(args[i]) })
			b.write(")")
		}
	})
}

// Deallocate renders DEALLOCATE name.
func Deallocate(name string) Command {
	return cmd(func(b *builder) { b.write("DEALLOCATE "); b.ident(name) })
}

// DeallocateAll renders DEALLOCATE ALL.
func DeallocateAll() Command { return cmd(func(b *builder) { b.write("DEALLOCATE ALL") }) }

// ---------------------------------------------------------------------------
// Transactions
// ---------------------------------------------------------------------------

// Isolation is a transaction isolation level.
type Isolation string

const (
	ReadCommitted  Isolation = "READ COMMITTED"
	RepeatableRead Isolation = "REPEATABLE READ"
	Serializable   Isolation = "SERIALIZABLE"
)

// TxMode are the options of BEGIN.
type TxMode struct {
	Isolation  Isolation
	ReadOnly   bool
	Deferrable bool
}

// Begin renders BEGIN with its modes.
func Begin(m TxMode) Command {
	return cmd(func(b *builder) {
		b.write("BEGIN")
		if m.Isolation != "" {
			b.write(" ISOLATION LEVEL " + string(m.Isolation))
		}
		if m.ReadOnly {
			b.write(" READ ONLY")
		}
		if m.Deferrable {
			if m.Isolation != Serializable || !m.ReadOnly {
				b.fail(fmt.Errorf("%w: DEFERRABLE only applies to SERIALIZABLE READ ONLY transactions", ErrShape))
				return
			}
			b.write(" DEFERRABLE")
		}
	})
}

func Commit() Command   { return cmd(func(b *builder) { b.write("COMMIT") }) }
func Rollback() Command { return cmd(func(b *builder) { b.write("ROLLBACK") }) }

func Savepoint(name string) Command {
	return cmd(func(b *builder) { b.write("SAVEPOINT "); b.ident(name) })
}
func ReleaseSavepoint(name string) Command {
	return cmd(func(b *builder) { b.write("RELEASE SAVEPOINT "); b.ident(name) })
}
func RollbackToSavepoint(name string) Command {
	return cmd(func(b *builder) { b.write("ROLLBACK TO SAVEPOINT "); b.ident(name) })
}

// PrepareTransaction is PREPARE TRANSACTION 'gid' (two-phase commit).
func PrepareTransaction(gid string) Command {
	return cmd(func(b *builder) { b.write("PREPARE TRANSACTION "); b.safeLiteral(gid) })
}
func CommitPrepared(gid string) Command {
	return cmd(func(b *builder) { b.write("COMMIT PREPARED "); b.safeLiteral(gid) })
}
func RollbackPrepared(gid string) Command {
	return cmd(func(b *builder) { b.write("ROLLBACK PREPARED "); b.safeLiteral(gid) })
}

// ---------------------------------------------------------------------------
// Cursors
// ---------------------------------------------------------------------------

// DeclareStmt is DECLARE name CURSOR FOR query.
type DeclareStmt struct {
	name                   string
	q                      Query
	scroll                 string
	hold, binary, insensit bool
}

// DeclareCursor starts a cursor over a query.
func DeclareCursor(name string, q Query) DeclareStmt { return DeclareStmt{name: name, q: q} }

func (d DeclareStmt) Scroll() DeclareStmt   { d.scroll = "SCROLL"; return d }
func (d DeclareStmt) NoScroll() DeclareStmt { d.scroll = "NO SCROLL"; return d }
func (d DeclareStmt) WithHold() DeclareStmt { d.hold = true; return d }
func (d DeclareStmt) Binary() DeclareStmt   { d.binary = true; return d }

func (d DeclareStmt) render(b *builder) {
	b.write("DECLARE ")
	b.ident(d.name)
	if d.binary {
		b.write(" BINARY")
	}
	if d.scroll != "" {
		b.write(" " + d.scroll)
	}
	b.write(" CURSOR")
	if d.hold {
		b.write(" WITH HOLD")
	}
	b.write(" FOR ")
	d.q.render(b)
}

// Direction is a FETCH or MOVE direction.
type Direction struct {
	kw string
	n  *int64
}

func dirN(kw string, n int64) Direction { return Direction{kw, &n} }

var (
	DirNext        = Direction{kw: "NEXT"}
	DirPrior       = Direction{kw: "PRIOR"}
	DirFirst       = Direction{kw: "FIRST"}
	DirLast        = Direction{kw: "LAST"}
	DirAll         = Direction{kw: "ALL"}
	DirForwardAll  = Direction{kw: "FORWARD ALL"}
	DirBackwardAll = Direction{kw: "BACKWARD ALL"}
)

func DirAbsolute(n int64) Direction { return dirN("ABSOLUTE", n) }
func DirRelative(n int64) Direction { return dirN("RELATIVE", n) }
func DirForward(n int64) Direction  { return dirN("FORWARD", n) }
func DirBackward(n int64) Direction { return dirN("BACKWARD", n) }
func DirCount(n int64) Direction    { return dirN("", n) }

func (d Direction) render(b *builder) {
	b.write(d.kw)
	if d.n != nil {
		if d.kw != "" {
			b.write(" ")
		}
		b.int(*d.n)
	}
}

// FetchFrom is FETCH direction FROM cursor.
func FetchFrom(d Direction, cursor string) Command {
	return cmd(func(b *builder) {
		b.write("FETCH ")
		d.render(b)
		b.write(" FROM ")
		b.ident(cursor)
	})
}

// MoveIn is MOVE direction IN cursor.
func MoveIn(d Direction, cursor string) Command {
	return cmd(func(b *builder) {
		b.write("MOVE ")
		d.render(b)
		b.write(" IN ")
		b.ident(cursor)
	})
}

// CloseCursor is CLOSE name.
func CloseCursor(name string) Command {
	return cmd(func(b *builder) { b.write("CLOSE "); b.ident(name) })
}

// ---------------------------------------------------------------------------
// LISTEN / NOTIFY
// ---------------------------------------------------------------------------

func Listen(channel string) Command {
	return cmd(func(b *builder) { b.write("LISTEN "); b.ident(channel) })
}
func Unlisten(channel string) Command {
	return cmd(func(b *builder) { b.write("UNLISTEN "); b.ident(channel) })
}
func UnlistenAll() Command { return cmd(func(b *builder) { b.write("UNLISTEN *") }) }

// Notify is SELECT pg_notify($1, $2), so the payload is a bound parameter
// (the NOTIFY command itself only takes a literal).
func Notify(channel, payload string) Command {
	return cmd(func(b *builder) {
		b.write("SELECT pg_notify(")
		b.arg(channel)
		b.write(", ")
		b.arg(payload)
		b.write(")")
	})
}

// SetConfig is SELECT set_config($1, $2, local): the way to set a session or
// transaction variable (for example row-level-security tenant ids) with bound
// values.
func SetConfig(name, value string, local bool) Command {
	return cmd(func(b *builder) {
		b.write("SELECT set_config(")
		b.arg(name)
		b.write(", ")
		b.arg(value)
		b.write(", ")
		b.arg(local)
		b.write(")")
	})
}

// CurrentSetting is SELECT current_setting($1).
func CurrentSetting(name string) Command {
	return cmd(func(b *builder) { b.write("SELECT current_setting("); b.arg(name); b.write(")") })
}

func ResetAll() Command   { return cmd(func(b *builder) { b.write("RESET ALL") }) }
func DiscardAll() Command { return cmd(func(b *builder) { b.write("DISCARD ALL") }) }

// ---------------------------------------------------------------------------
// CALL
// ---------------------------------------------------------------------------

// Procedure names a stored procedure; generated code declares one per procedure.
type Procedure struct{ Schema, Name string }

// Call is CALL schema.name(args).
func Call(p Procedure, args ...Value) Command {
	args = slices.Clone(args)
	return cmd(func(b *builder) {
		b.write("CALL ")
		if p.Schema != "" {
			b.ident(p.Schema)
			b.write(".")
		}
		b.ident(p.Name)
		b.write("(")
		joinParts(b, len(args), func(i int) { args[i].valueNode().render(b) })
		b.write(")")
	})
}
