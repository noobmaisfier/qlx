package pgcat

import (
	"encoding/json"
	"fmt"

	"github.com/noobmaisfier/qlx/internal/pgtool"
)

// extractSQL returns one JSON document. Type names come from pg_type.typname
// (not format_type) so the ledger does not depend on typmods or search_path.
// Only pg_catalog is read: user schemas are the generator's job.
const extractSQL = `
WITH t AS (SELECT oid, typname FROM pg_type),
fns AS (
  SELECT p.proname AS name, p.prokind::text AS kind,
         COALESCE((SELECT json_agg(t.typname ORDER BY k.ord)
                   FROM unnest(string_to_array(p.proargtypes::text, ' ')::oid[]) WITH ORDINALITY AS k(o, ord)
                   JOIN t ON t.oid = k.o), '[]'::json) AS args,
         COALESCE(to_json(p.proargnames), 'null'::json) AS arg_names,
         (SELECT typname FROM t WHERE t.oid = p.prorettype) AS ret,
         p.proretset AS ret_set, p.proisstrict AS strict, p.provolatile::text AS vol,
         CASE WHEN p.proallargtypes IS NULL OR p.proargmodes IS NULL THEN 'null'::json ELSE COALESCE((SELECT json_agg(json_build_object('name', u.n, 'type', t2.typname) ORDER BY u.ord)
                   FROM unnest(p.proallargtypes, p.proargmodes,
                               COALESCE(p.proargnames, array_fill(''::text, ARRAY[cardinality(p.proallargtypes)])))
                        WITH ORDINALITY AS u(ty, md, n, ord)
                   JOIN t t2 ON t2.oid = u.ty WHERE u.md IN ('o','b','t')), 'null'::json) END AS outs,
         (p.provariadic <> 0) AS variadic, p.pronargdefaults AS defaults,
         (p.proallargtypes IS NOT NULL AND p.proargmodes IS NOT NULL
            AND EXISTS (SELECT 1 FROM unnest(p.proargmodes) m WHERE m <> 'i' AND m <> 'v')) AS out_modes
  FROM pg_proc p
  WHERE p.pronamespace = 'pg_catalog'::regnamespace AND p.prokind IN ('f','a','w')
),
ops AS (
  SELECT o.oprname AS name,
         COALESCE((SELECT typname FROM t WHERE t.oid = o.oprleft), '') AS "left",
         (SELECT typname FROM t WHERE t.oid = o.oprright) AS "right",
         (SELECT typname FROM t WHERE t.oid = o.oprresult) AS ret,
         o.oprcode::text AS proc
  FROM pg_operator o WHERE o.oprnamespace = 'pg_catalog'::regnamespace AND o.oprright <> 0
),
casts AS (
  SELECT (SELECT typname FROM t WHERE t.oid = c.castsource) AS "from",
         (SELECT typname FROM t WHERE t.oid = c.casttarget) AS "to",
         c.castcontext::text AS context
  FROM pg_cast c
)
SELECT json_build_object(
  'version', current_setting('server_version_num')::int / 10000,
  'funcs', (SELECT COALESCE(json_agg(row_to_json(fns)), '[]'::json) FROM fns),
  'ops', (SELECT COALESCE(json_agg(row_to_json(ops)), '[]'::json) FROM ops),
  'casts', (SELECT COALESCE(json_agg(row_to_json(casts)), '[]'::json) FROM casts));
`

type rawFn struct {
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	Args     []string `json:"args"`
	ArgNames []string `json:"arg_names"`
	Ret      string   `json:"ret"`
	RetSet   bool     `json:"ret_set"`
	Strict   bool     `json:"strict"`
	Vol      string   `json:"vol"`
	Variadic bool     `json:"variadic"`
	Defaults int      `json:"defaults"`
	OutModes bool     `json:"out_modes"`
	Outs     []Out    `json:"outs"`
}

type rawOp struct {
	Name  string `json:"name"`
	Left  string `json:"left"`
	Right string `json:"right"`
	Ret   string `json:"ret"`
	Proc  string `json:"proc"`
}

type rawCast struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Context string `json:"context"`
}

// Extract reads the built-in catalog of the connected server.
func Extract(c pgtool.Conn) (*Ledger, error) {
	out, err := c.Query(extractSQL)
	if err != nil {
		return nil, fmt.Errorf("pgcat extract: %w", err)
	}
	var raw struct {
		Version int       `json:"version"`
		Funcs   []rawFn   `json:"funcs"`
		Ops     []rawOp   `json:"ops"`
		Casts   []rawCast `json:"casts"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("pgcat extract: unexpected JSON: %w", err)
	}
	l := &Ledger{Versions: []int{raw.Version}}
	for _, f := range raw.Funcs {
		l.Funcs = append(l.Funcs, Fn{Name: f.Name, Kind: f.Kind, Args: nonNil(f.Args), ArgNames: f.ArgNames,
			Ret: f.Ret, RetSet: f.RetSet, Strict: f.Strict, Volatility: f.Vol, Variadic: f.Variadic,
			Defaults: f.Defaults, OutModes: f.OutModes, Outs: f.Outs})
	}
	for _, o := range raw.Ops {
		l.Ops = append(l.Ops, Op{Name: o.Name, Left: o.Left, Right: o.Right, Ret: o.Ret, Proc: o.Proc})
	}
	for _, c := range raw.Casts {
		l.Casts = append(l.Casts, Cast{From: c.From, To: c.To, Context: c.Context})
	}
	l.Normalize()
	return l, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// extensionSQL reads what one extension installed: its functions, operators
// and casts (by pg_depend membership, whatever schema they live in), plus
// the extension's schema. %[1]s is the validated extension name. Type names
// are typname as in extractSQL; schemas are recorded per entry.
const extensionSQL = `
WITH ext AS (SELECT e.oid, n.nspname AS schema FROM pg_extension e JOIN pg_namespace n ON n.oid = e.extnamespace WHERE e.extname = '%[1]s'),
member AS (SELECT d.classid::regclass::text AS cls, d.objid FROM pg_depend d, ext
           WHERE d.refclassid = 'pg_extension'::regclass AND d.refobjid = ext.oid AND d.deptype = 'e'),
t AS (SELECT oid, typname FROM pg_type),
fns AS (
  SELECT p.proname AS name, p.prokind::text AS kind, p.pronamespace::regnamespace::text AS schema,
         COALESCE((SELECT json_agg(t.typname ORDER BY k.ord)
                   FROM unnest(string_to_array(p.proargtypes::text, ' ')::oid[]) WITH ORDINALITY AS k(o, ord)
                   JOIN t ON t.oid = k.o), '[]'::json) AS args,
         COALESCE(to_json(p.proargnames), 'null'::json) AS arg_names,
         (SELECT typname FROM t WHERE t.oid = p.prorettype) AS ret,
         p.proretset AS ret_set, p.proisstrict AS strict, p.provolatile::text AS vol,
         CASE WHEN p.proallargtypes IS NULL OR p.proargmodes IS NULL THEN 'null'::json ELSE COALESCE((SELECT json_agg(json_build_object('name', u.n, 'type', t2.typname) ORDER BY u.ord)
                   FROM unnest(p.proallargtypes, p.proargmodes,
                               COALESCE(p.proargnames, array_fill(''::text, ARRAY[cardinality(p.proallargtypes)])))
                        WITH ORDINALITY AS u(ty, md, n, ord)
                   JOIN t t2 ON t2.oid = u.ty WHERE u.md IN ('o','b','t')), 'null'::json) END AS outs,
         (p.provariadic <> 0) AS variadic, p.pronargdefaults AS defaults,
         (p.proallargtypes IS NOT NULL AND p.proargmodes IS NOT NULL
            AND EXISTS (SELECT 1 FROM unnest(p.proargmodes) m WHERE m <> 'i' AND m <> 'v')) AS out_modes
  FROM pg_proc p JOIN member m ON m.cls = 'pg_proc' AND m.objid = p.oid
  WHERE p.prokind IN ('f','a','w')
),
ops AS (
  SELECT o.oprname AS name, o.oprnamespace::regnamespace::text AS schema,
         COALESCE((SELECT typname FROM t WHERE t.oid = o.oprleft), '') AS "left",
         (SELECT typname FROM t WHERE t.oid = o.oprright) AS "right",
         (SELECT typname FROM t WHERE t.oid = o.oprresult) AS ret,
         o.oprcode::text AS proc
  FROM pg_operator o JOIN member m ON m.cls = 'pg_operator' AND m.objid = o.oid WHERE o.oprright <> 0
),
casts AS (
  SELECT (SELECT typname FROM t WHERE t.oid = c.castsource) AS "from",
         (SELECT typname FROM t WHERE t.oid = c.casttarget) AS "to",
         c.castcontext::text AS context
  FROM pg_cast c JOIN member m ON m.cls = 'pg_cast' AND m.objid = c.oid
)
SELECT json_build_object(
  'version', current_setting('server_version_num')::int / 10000,
  'schema', (SELECT schema FROM ext),
  'funcs', (SELECT COALESCE(json_agg(row_to_json(fns)), '[]'::json) FROM fns),
  'ops', (SELECT COALESCE(json_agg(row_to_json(ops)), '[]'::json) FROM ops),
  'casts', (SELECT COALESCE(json_agg(row_to_json(casts)), '[]'::json) FROM casts));
`

// ExtractExtension reads one extension's functions, operators and casts from
// a server where the extension is installed (CREATE EXTENSION already ran).
func ExtractExtension(c pgtool.Conn, extension string) (*Ledger, error) {
	for _, r := range extension {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return nil, fmt.Errorf("pgcat extract: %q is not an extension name", extension)
		}
	}
	out, err := c.Query(fmt.Sprintf(extensionSQL, extension))
	if err != nil {
		return nil, fmt.Errorf("pgcat extract %s: %w", extension, err)
	}
	var raw struct {
		Version int    `json:"version"`
		Schema  string `json:"schema"`
		Funcs   []struct {
			rawFn
			Schema string `json:"schema"`
		} `json:"funcs"`
		Ops []struct {
			rawOp
			Schema string `json:"schema"`
		} `json:"ops"`
		Casts []rawCast `json:"casts"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("pgcat extract %s: unexpected JSON: %w", extension, err)
	}
	if raw.Schema == "" {
		return nil, fmt.Errorf("pgcat extract: extension %q is not installed on this server (run CREATE EXTENSION first)", extension)
	}
	l := &Ledger{Extension: extension, Schema: raw.Schema, Versions: []int{raw.Version}}
	for _, f := range raw.Funcs {
		l.Funcs = append(l.Funcs, Fn{Name: f.Name, Kind: f.Kind, Args: nonNil(f.Args), ArgNames: f.ArgNames,
			Ret: f.Ret, RetSet: f.RetSet, Strict: f.Strict, Volatility: f.Vol, Variadic: f.Variadic,
			Defaults: f.Defaults, OutModes: f.OutModes, Outs: f.Outs, Schema: f.Schema})
	}
	for _, o := range raw.Ops {
		l.Ops = append(l.Ops, Op{Name: o.Name, Left: o.Left, Right: o.Right, Ret: o.Ret, Proc: o.Proc, Schema: o.Schema})
	}
	for _, c := range raw.Casts {
		l.Casts = append(l.Casts, Cast{From: c.From, To: c.To, Context: c.Context})
	}
	l.Normalize()
	return l, nil
}

// Enrich copies the output columns (Outs) of every function that has OUT,
// INOUT or TABLE columns from the ledger read off a live server into l.
// Merged ledgers do not carry them unless they were extracted after this
// field existed, so run it once on any server of a targeted major; columns
// of a function the server does not have stay as they were.
func Enrich(l, fresh *Ledger) (n int) {
	by := map[string][]Out{}
	for _, f := range fresh.Funcs {
		if len(f.Outs) > 0 {
			by[f.Sig()] = f.Outs
		}
	}
	for i := range l.Funcs {
		if o, ok := by[l.Funcs[i].Sig()]; ok && len(l.Funcs[i].Outs) == 0 {
			l.Funcs[i].Outs = o
			n++
		}
	}
	return n
}
