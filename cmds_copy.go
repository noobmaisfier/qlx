package qlx

import (
	"fmt"
	"regexp"
	"slices"
)

var lsnRe = regexp.MustCompile(`^[0-9A-Fa-f]{1,8}/[0-9A-Fa-f]{1,8}$`)

// ---------------------------------------------------------------------------
// COPY
// ---------------------------------------------------------------------------

type copyDir uint8

const (
	dirBoth copyDir = iota
	dirFromOnly
	dirToOnly
)

// CopyOption is one COPY option. Options that only make sense in one
// direction are refused in the other when the statement is rendered.
type CopyOption struct {
	dir  copyDir
	min  int
	what string
	f    func(b *builder)
}

func kw(dir copyDir, min int, text string) CopyOption {
	return CopyOption{dir: dir, min: min, what: text, f: func(b *builder) { b.write(text) }}
}

// CopyFormat is the COPY format.
type CopyFormat string

const (
	CopyText   CopyFormat = "text"
	CopyCSV    CopyFormat = "csv"
	CopyBinary CopyFormat = "binary"
	// CopyJSON is COPY ... TO ... (FORMAT json), new in PostgreSQL 19.
	CopyJSON CopyFormat = "json"
)

// WithFormat sets FORMAT. JSON is output-only and needs PostgreSQL 19.
func WithFormat(f CopyFormat) CopyOption {
	o := kw(dirBoth, 0, "FORMAT "+string(f))
	if f == CopyJSON {
		o.dir, o.min = dirToOnly, PG19
	}
	return o
}

// Delim is a COPY delimiter or quote character from a closed set.
type Delim struct{ lit string }

var (
	DelimComma     = Delim{"','"}
	DelimTab       = Delim{`E'\t'`}
	DelimPipe      = Delim{"'|'"}
	DelimSemicolon = Delim{"';'"}
	DelimColon     = Delim{"':'"}
	QuoteDouble    = Delim{"'\"'"}
	QuoteSingle    = Delim{"''''"}
)

func WithDelimiter(d Delim) CopyOption { return kw(dirBoth, 0, "DELIMITER "+d.lit) }
func WithQuote(d Delim) CopyOption     { return kw(dirBoth, 0, "QUOTE "+d.lit) }

// WithNull sets the NULL marker string (printable ASCII only).
func WithNull(s string) CopyOption {
	return CopyOption{what: "NULL", f: func(b *builder) { b.write("NULL "); b.safeLiteral(s) }}
}

// WithHeader sets HEADER; n > 1 (skipping several header lines) needs PostgreSQL 19.
func WithHeader(n int) CopyOption {
	switch {
	case n <= 0:
		return kw(dirBoth, 0, "HEADER false")
	case n == 1:
		return kw(dirBoth, 0, "HEADER true")
	}
	return CopyOption{dir: dirFromOnly, min: PG19, what: "HEADER n", f: func(b *builder) { b.write("HEADER "); b.int(int64(n)) }}
}

// WithFreeze is FREEZE (COPY FROM only).
var WithFreeze = kw(dirFromOnly, 0, "FREEZE")

// WithForceArray is FORCE_ARRAY for JSON output (PostgreSQL 19).
var WithForceArray = kw(dirToOnly, PG19, "FORCE_ARRAY true")

// OnErrorMode is the ON_ERROR behaviour.
type OnErrorMode string

const (
	OnErrorStop    OnErrorMode = "stop"
	OnErrorIgnore  OnErrorMode = "ignore"   // PostgreSQL 17
	OnErrorSetNull OnErrorMode = "set_null" // PostgreSQL 19
)

// WithOnError sets ON_ERROR (COPY FROM only).
func WithOnError(m OnErrorMode) CopyOption {
	min := 0
	switch m {
	case OnErrorIgnore:
		min = 17
	case OnErrorSetNull:
		min = PG19
	}
	return kw(dirFromOnly, min, "ON_ERROR "+string(m))
}

// WithRejectLimit is REJECT_LIMIT n (PostgreSQL 18, with ON_ERROR ignore).
//
// Requires PostgreSQL 18.
func WithRejectLimit(n int64) CopyOption {
	return CopyOption{dir: dirFromOnly, min: PG18, what: "REJECT_LIMIT", f: func(b *builder) { b.write("REJECT_LIMIT "); b.int(n) }}
}

// LogVerbosity is the LOG_VERBOSITY level.
type LogVerbosity string

const (
	LogDefault LogVerbosity = "default"
	LogVerbose LogVerbosity = "verbose" // PostgreSQL 17
	LogSilent  LogVerbosity = "silent"  // PostgreSQL 18
)

// WithLogVerbosity sets LOG_VERBOSITY (COPY FROM only).
func WithLogVerbosity(v LogVerbosity) CopyOption {
	min := 0
	switch v {
	case LogVerbose:
		min = 17
	case LogSilent:
		min = PG18
	}
	return kw(dirFromOnly, min, "LOG_VERBOSITY "+string(v))
}

// CopyColumns lists columns for FORCE_QUOTE, FORCE_NOT_NULL, FORCE_NULL.
func WithForceQuote(t Table, cols ...ColumnRef) CopyOption {
	return colListOption(dirToOnly, "FORCE_QUOTE", t, cols)
}
func WithForceNotNull(t Table, cols ...ColumnRef) CopyOption {
	return colListOption(dirFromOnly, "FORCE_NOT_NULL", t, cols)
}
func WithForceNull(t Table, cols ...ColumnRef) CopyOption {
	return colListOption(dirFromOnly, "FORCE_NULL", t, cols)
}

func colListOption(dir copyDir, name string, t Table, cols []ColumnRef) CopyOption {
	cols = slices.Clone(cols)
	return CopyOption{dir: dir, what: name, f: func(b *builder) {
		b.write(name + " (")
		b.identList(colNamesOf(t.tableRef(), cols, b))
		b.write(")")
	}}
}

func renderCopyOptions(b *builder, opts []CopyOption, from bool) {
	if len(opts) == 0 {
		return
	}
	b.write(" (")
	for i, o := range opts {
		if (o.dir == dirFromOnly && !from) || (o.dir == dirToOnly && from) {
			dir := "COPY ... TO"
			if from {
				dir = "COPY ... FROM"
			}
			b.fail(fmt.Errorf("%w: option %q is not valid for %s", ErrShape, o.what, dir))
			return
		}
		if o.min > 0 {
			b.need(o.min, "COPY option "+o.what)
		}
		if i > 0 {
			b.write(", ")
		}
		o.f(b)
	}
	b.write(")")
}

// CopyFromStdin is COPY table (cols) FROM STDIN (options) [WHERE ...]. Run it
// with pgx's PgConn().CopyFrom; for typed binary row copying use pgxexec.
type CopyFromStdin struct {
	t     TableRef
	cols  []ColumnRef
	opts  []CopyOption
	where Pred
	// inline writes the arguments of the WHERE as literals (see InlineArgs).
	inline bool
}

func CopyIn(t Table, cols ...ColumnRef) CopyFromStdin {
	return CopyFromStdin{t: t.tableRef(), cols: slices.Clone(cols)}
}
func (c CopyFromStdin) With(opts ...CopyOption) CopyFromStdin {
	c.opts = app(c.opts, opts...)
	return c
}

// Where filters incoming rows (system columns are not allowed since PostgreSQL 19).
func (c CopyFromStdin) Where(p Pred) CopyFromStdin { c.where = p; return c }

// InlineArgs writes the arguments of Where into the SQL as literals, since
// COPY cannot take parameters. Only integers, bools, floats, times, uuids,
// bytes, NULL, slices of those and plain ASCII strings are accepted; any
// other value is an ErrParams error. See inline.go for the exact rules.
func (c CopyFromStdin) InlineArgs() CopyFromStdin { c.inline = true; return c }

func (c CopyFromStdin) render(b *builder) {
	b.pushScope(c.t.key())
	defer b.popScope()
	b.write("COPY ")
	c.t.renderFrom(b)
	if len(c.cols) > 0 {
		b.write(" (")
		b.identList(colNamesOf(c.t, c.cols, b))
		b.write(")")
	}
	b.write(" FROM STDIN")
	renderCopyOptions(b, c.opts, true)
	if c.where.valid() {
		b.write(" WHERE ")
		b.noParams(c.inline, "COPY FROM ... WHERE", func() { c.where.n.render(b) })
	}
}

// CopyOut is COPY (query | table) TO STDOUT (options).
type CopyOut struct {
	q    Query
	t    *TableRef
	cols []ColumnRef
	opts []CopyOption
	// inline writes the arguments of the query as literals (see InlineArgs).
	inline bool
}

// CopyQueryOut copies the result of a query out.
func CopyQueryOut(q Query) CopyOut { return CopyOut{q: q} }

// CopyTableOut copies a table (or some of its columns) out. Tables can be
// partitioned (PostgreSQL 19) or materialized views (PostgreSQL 18).
func CopyTableOut(t Table, cols ...ColumnRef) CopyOut {
	tr := t.tableRef()
	return CopyOut{t: &tr, cols: slices.Clone(cols)}
}

func (c CopyOut) With(opts ...CopyOption) CopyOut { c.opts = app(c.opts, opts...); return c }

// InlineArgs writes the arguments of the query into the SQL as literals,
// since COPY cannot take parameters. Only integers, bools, floats, times,
// uuids, bytes, NULL, slices of those and plain ASCII strings are accepted;
// any other value is an ErrParams error. See inline.go for the exact rules.
func (c CopyOut) InlineArgs() CopyOut { c.inline = true; return c }

func (c CopyOut) render(b *builder) {
	b.write("COPY ")
	if c.q != nil {
		b.write("(")
		b.noParams(c.inline, "COPY (query)", func() { c.q.render(b) })
		b.write(")")
	} else {
		c.t.renderFrom(b)
		if len(c.cols) > 0 {
			b.write(" (")
			b.identList(colNamesOf(*c.t, c.cols, b))
			b.write(")")
		}
	}
	b.write(" TO STDOUT")
	renderCopyOptions(b, c.opts, false)
}

// ---------------------------------------------------------------------------
// Maintenance: VACUUM, ANALYZE, REPACK, TRUNCATE, LOCK, CHECKPOINT
// ---------------------------------------------------------------------------

// MaintTarget is a table (and optionally columns) for VACUUM or ANALYZE.
type MaintTarget struct {
	t    TableRef
	cols []ColumnRef
	only bool
}

// Target names a table for VACUUM or ANALYZE.
func Target(t Table, cols ...ColumnRef) MaintTarget {
	return MaintTarget{t: t.tableRef(), cols: slices.Clone(cols)}
}

// Only excludes partitions and inheritance children (PostgreSQL 18).
//
// Requires PostgreSQL 18.
func (m MaintTarget) Only() MaintTarget { m.only = true; return m }

func (m MaintTarget) render(b *builder) {
	if m.only {
		b.need(PG18, "ONLY with VACUUM/ANALYZE")
		b.write("ONLY ")
	}
	m.t.renderFrom(b)
	if len(m.cols) > 0 {
		b.write(" (")
		b.identList(colNamesOf(m.t, m.cols, b))
		b.write(")")
	}
}

// MaintOption is a VACUUM or ANALYZE option.
type MaintOption struct {
	kw  string
	min int
}

var (
	VacFull                = MaintOption{kw: "FULL"}
	VacFreeze              = MaintOption{kw: "FREEZE"}
	VacVerbose             = MaintOption{kw: "VERBOSE"}
	VacAnalyze             = MaintOption{kw: "ANALYZE"}
	VacSkipLocked          = MaintOption{kw: "SKIP_LOCKED"}
	VacNoTruncate          = MaintOption{kw: "TRUNCATE false"}
	VacNoIndexClean        = MaintOption{kw: "INDEX_CLEANUP off"}
	VacDisablePageSkipping = MaintOption{kw: "DISABLE_PAGE_SKIPPING"}
)

// VacParallel sets PARALLEL n.
func VacParallel(n int64) MaintOption { return MaintOption{kw: "PARALLEL " + fmt.Sprint(n)} }

func renderMaint(b *builder, verb string, opts []MaintOption, targets []MaintTarget) {
	b.write(verb)
	if len(opts) > 0 {
		b.write(" (")
		for i, o := range opts {
			if i > 0 {
				b.write(", ")
			}
			if o.min > 0 {
				b.need(o.min, verb+" "+o.kw)
			}
			b.write(o.kw)
		}
		b.write(")")
	}
	for i, t := range targets {
		if i == 0 {
			b.write(" ")
		} else {
			b.write(", ")
		}
		t.render(b)
	}
}

// Vacuum is VACUUM (options) targets.
func Vacuum(opts []MaintOption, targets ...MaintTarget) Command {
	opts, targets = slices.Clone(opts), slices.Clone(targets)
	return cmd(func(b *builder) { renderMaint(b, "VACUUM", opts, targets) })
}

// AnalyzeTables is ANALYZE (options) targets.
func AnalyzeTables(opts []MaintOption, targets ...MaintTarget) Command {
	opts, targets = slices.Clone(opts), slices.Clone(targets)
	return cmd(func(b *builder) { renderMaint(b, "ANALYZE", opts, targets) })
}

// RepackOption is a REPACK option (PostgreSQL 19).
type RepackOption struct{ kw string }

var (
	RepackConcurrently = RepackOption{"CONCURRENTLY"}
	RepackVerbose      = RepackOption{"VERBOSE"}
	RepackAnalyze      = RepackOption{"ANALYZE"}
)

// Repack is REPACK [(options)] [table [USING INDEX index]], which replaces
// VACUUM FULL and CLUSTER in PostgreSQL 19.
//
// UNVERIFIED: the grammar below follows the PostgreSQL 19 RC1 release notes
// as understood when this was written; confirm it against a 19 server (the
// PREPARE-based test in pg_test.go does not cover utility commands).
//
// Requires PostgreSQL 19.
func Repack(opts []RepackOption, t Table, usingIndex string) Command {
	return cmd(func(b *builder) {
		b.need(PG19, "REPACK")
		b.write("REPACK")
		if len(opts) > 0 {
			b.write(" (")
			for i, o := range opts {
				if i > 0 {
					b.write(", ")
				}
				b.write(o.kw)
			}
			b.write(")")
		}
		if t != nil {
			b.write(" ")
			t.tableRef().renderFrom(b)
			if usingIndex != "" {
				b.write(" USING INDEX ")
				b.ident(usingIndex)
			}
		}
	})
}

// CheckpointOption is a CHECKPOINT option (PostgreSQL 19).
type CheckpointOption struct{ kw string }

var (
	CheckpointFast          = CheckpointOption{"MODE FAST"}
	CheckpointSpread        = CheckpointOption{"MODE SPREAD"}
	CheckpointFlushUnlogged = CheckpointOption{"FLUSH_UNLOGGED"}
)

// Checkpoint is CHECKPOINT, with an option list on PostgreSQL 19.
//
// UNVERIFIED: option names follow the 19 RC1 release notes; confirm on a 19 server.
func Checkpoint(opts ...CheckpointOption) Command {
	return cmd(func(b *builder) {
		b.write("CHECKPOINT")
		if len(opts) == 0 {
			return
		}
		b.need(PG19, "CHECKPOINT options")
		b.write(" (")
		for i, o := range opts {
			if i > 0 {
				b.write(", ")
			}
			b.write(o.kw)
		}
		b.write(")")
	})
}

// TruncateStmt is TRUNCATE.
type TruncateStmt struct {
	ts      []TableRef
	only    bool
	restart bool
	cascade bool
}

// Truncate empties tables.
func Truncate(ts ...Table) TruncateStmt {
	s := TruncateStmt{}
	for _, t := range ts {
		s.ts = append(s.ts, t.tableRef())
	}
	return s
}

func (s TruncateStmt) Only() TruncateStmt            { s.only = true; return s }
func (s TruncateStmt) RestartIdentity() TruncateStmt { s.restart = true; return s }
func (s TruncateStmt) Cascade() TruncateStmt         { s.cascade = true; return s }

func (s TruncateStmt) render(b *builder) {
	if len(s.ts) == 0 {
		b.fail(fmt.Errorf("%w: TRUNCATE needs a table", ErrShape))
		return
	}
	b.write("TRUNCATE ")
	for i, t := range s.ts {
		if i > 0 {
			b.write(", ")
		}
		if s.only {
			b.write("ONLY ")
		}
		t.renderFrom(b)
	}
	if s.restart {
		b.write(" RESTART IDENTITY")
	}
	if s.cascade {
		b.write(" CASCADE")
	}
}

// LockMode is a table lock mode.
type LockMode string

const (
	LockAccessShare          LockMode = "ACCESS SHARE"
	LockRowShare             LockMode = "ROW SHARE"
	LockRowExclusive         LockMode = "ROW EXCLUSIVE"
	LockShareUpdateExclusive LockMode = "SHARE UPDATE EXCLUSIVE"
	LockShare                LockMode = "SHARE"
	LockShareRowExclusive    LockMode = "SHARE ROW EXCLUSIVE"
	LockExclusive            LockMode = "EXCLUSIVE"
	LockAccessExclusive      LockMode = "ACCESS EXCLUSIVE"
)

// LockTable is LOCK TABLE t IN mode MODE [NOWAIT].
func LockTable(mode LockMode, noWait bool, ts ...Table) Command {
	ts = slices.Clone(ts)
	return cmd(func(b *builder) {
		if len(ts) == 0 {
			b.fail(fmt.Errorf("%w: LOCK TABLE needs a table", ErrShape))
			return
		}
		b.write("LOCK TABLE ")
		joinParts(b, len(ts), func(i int) { ts[i].tableRef().renderFrom(b) })
		b.write(" IN " + string(mode) + " MODE")
		if noWait {
			b.write(" NOWAIT")
		}
	})
}

// ---------------------------------------------------------------------------
// GRANT / REVOKE
// ---------------------------------------------------------------------------

// Privilege is a table privilege.
type Privilege string

const (
	PrivSelect     Privilege = "SELECT"
	PrivInsert     Privilege = "INSERT"
	PrivUpdate     Privilege = "UPDATE"
	PrivDelete     Privilege = "DELETE"
	PrivTruncate   Privilege = "TRUNCATE"
	PrivReferences Privilege = "REFERENCES"
	PrivTrigger    Privilege = "TRIGGER"
	PrivAll        Privilege = "ALL PRIVILEGES"
)

// Role names a role; PublicRole is PUBLIC.
type Role struct {
	name   string
	public bool
}

func NewRole(name string) Role { return Role{name: name} }

var PublicRole = Role{public: true}

func (r Role) render(b *builder) {
	if r.public {
		b.write("PUBLIC")
		return
	}
	b.ident(r.name)
}

// GrantStmt is GRANT or REVOKE on tables.
type GrantStmt struct {
	revoke    bool
	privs     []Privilege
	tables    []TableRef
	roles     []Role
	grantOpt  bool
	grantedBy *Role
	cascade   bool
}

// Grant starts GRANT privileges.
func Grant(privs ...Privilege) GrantStmt { return GrantStmt{privs: slices.Clone(privs)} }

// Revoke starts REVOKE privileges.
func Revoke(privs ...Privilege) GrantStmt { return GrantStmt{revoke: true, privs: slices.Clone(privs)} }

func (g GrantStmt) OnTables(ts ...Table) GrantStmt {
	for _, t := range ts {
		g.tables = append(g.tables[:len(g.tables):len(g.tables)], t.tableRef())
	}
	return g
}
func (g GrantStmt) To(rs ...Role) GrantStmt   { g.roles = app(g.roles, rs...); return g }
func (g GrantStmt) From(rs ...Role) GrantStmt { g.roles = app(g.roles, rs...); return g }

// WithGrantOption adds WITH GRANT OPTION (GRANT only).
func (g GrantStmt) WithGrantOption() GrantStmt { g.grantOpt = true; return g }

// GrantedBy adds GRANTED BY role (PostgreSQL 19 for table privileges).
//
// Requires PostgreSQL 19.
func (g GrantStmt) GrantedBy(r Role) GrantStmt { g.grantedBy = &r; return g }

// Cascade adds CASCADE (REVOKE only).
func (g GrantStmt) Cascade() GrantStmt { g.cascade = true; return g }

func (g GrantStmt) render(b *builder) {
	if len(g.privs) == 0 || len(g.tables) == 0 || len(g.roles) == 0 {
		b.fail(fmt.Errorf("%w: GRANT/REVOKE needs privileges, tables and roles", ErrShape))
		return
	}
	if g.revoke {
		b.write("REVOKE ")
	} else {
		b.write("GRANT ")
	}
	for i, p := range g.privs {
		if i > 0 {
			b.write(", ")
		}
		b.write(string(p))
	}
	b.write(" ON ")
	joinParts(b, len(g.tables), func(i int) { g.tables[i].renderFrom(b) })
	if g.revoke {
		b.write(" FROM ")
	} else {
		b.write(" TO ")
	}
	joinParts(b, len(g.roles), func(i int) { g.roles[i].render(b) })
	if g.grantOpt {
		if g.revoke {
			b.fail(fmt.Errorf("%w: WITH GRANT OPTION applies to GRANT only", ErrShape))
			return
		}
		b.write(" WITH GRANT OPTION")
	}
	if g.grantedBy != nil {
		b.need(PG19, "GRANTED BY on table privileges")
		b.write(" GRANTED BY ")
		g.grantedBy.render(b)
	}
	if g.cascade {
		if !g.revoke {
			b.fail(fmt.Errorf("%w: CASCADE applies to REVOKE only", ErrShape))
			return
		}
		b.write(" CASCADE")
	}
}

// ---------------------------------------------------------------------------
// WAIT FOR LSN (PostgreSQL 19)
// ---------------------------------------------------------------------------

// WaitForLSN is WAIT FOR LSN 'x/y' [WITH (TIMEOUT 'n ms', NO_THROW)], which
// makes a standby wait until it has replayed up to an LSN.
//
// UNVERIFIED: written from the PostgreSQL 19 RC1 release notes; the exact
// option grammar must be confirmed on a 19 server.
//
// Requires PostgreSQL 19.
func WaitForLSN(lsn string, timeoutMillis int64, noThrow bool) Command {
	return cmd(func(b *builder) {
		b.need(PG19, "WAIT FOR LSN")
		if !lsnRe.MatchString(lsn) {
			b.fail(fmt.Errorf("%w: %q is not an LSN such as 0/306EE20", ErrInvalidIdent, lsn))
			return
		}
		b.write("WAIT FOR LSN '" + lsn + "'")
		var opts []string
		if timeoutMillis > 0 {
			opts = append(opts, fmt.Sprintf("TIMEOUT '%dms'", timeoutMillis))
		}
		if noThrow {
			opts = append(opts, "NO_THROW")
		}
		if len(opts) > 0 {
			b.write(" WITH (")
			for i, o := range opts {
				if i > 0 {
					b.write(", ")
				}
				b.write(o)
			}
			b.write(")")
		}
	})
}

// noParams renders f and fails if it bound any parameter. The server accepts
// $n placeholders in few utility statements; COPY is not one of them, so a
// parameterized COPY would only fail at run time with a confusing message.
// With inline set (InlineArgs) the arguments are written as literals instead.
func (b *builder) noParams(inline bool, what string, f func()) {
	if inline {
		b.inline++
		defer func() { b.inline-- }()
		f()
		return
	}
	before := len(b.slots)
	f()
	if len(b.slots) != before {
		b.fail(fmt.Errorf("%w: %s cannot take parameters; filter on columns without runtime values, use InlineArgs, or use a cursor", ErrParams, what))
	}
}

// Target returns what a native COPY FROM needs: the table's schema and name
// and the target columns, checked to belong to the table. Drivers that copy
// through their own protocol call (pgx.CopyFrom) use it instead of the SQL.
func (c CopyFromStdin) Target() (schema, table string, cols []string, err error) {
	for _, col := range c.cols {
		r := col.columnRef()
		if !sameTable(r.t, c.t) {
			return "", "", nil, fmt.Errorf("%w: column %s.%s is not a column of %s", ErrScope, r.t.key(), r.name, c.t.key())
		}
		cols = append(cols, r.name)
	}
	return c.t.Schema, c.t.Name, cols, nil
}
