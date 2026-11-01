package catalog

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/noobmaisfier/qlx/internal/pgtool"
)

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$]*$`)

// FingerprintSQL returns one row (table, hash) per relation in the schemas
// passed as $1::text[]. The hash covers every column's name, type,
// nullability, default, identity and generation, so any change that could
// break a generated query changes it. The same text is embedded in generated
// code, which is how VerifySchema compares only the tables an application
// uses. $1 must be bound as a text[] parameter.
const FingerprintSQL = `SELECT n.nspname || '.' || c.relname AS tbl,
       md5(string_agg(a.attname || ':' || format_type(a.atttypid, a.atttypmod) || ':' ||
           a.attnotnull::text || ':' || a.atthasdef::text || ':' || a.attidentity::text || ':' ||
           a.attgenerated::text, ',' ORDER BY a.attnum)) AS fp
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
WHERE c.relkind IN ('r', 'p', 'v', 'm', 'f') AND n.nspname = ANY($1::text[]) AND NOT c.relispartition
GROUP BY 1 ORDER BY 1`

func textArray(schemas []string) (string, error) {
	parts := make([]string, len(schemas))
	for i, s := range schemas {
		if !identRe.MatchString(s) {
			return "", fmt.Errorf("schema name %q: only letters, digits, _ and $ are supported in the config", s)
		}
		parts[i] = "'" + s + "'"
	}
	return "ARRAY[" + strings.Join(parts, ",") + "]::text[]", nil
}

const loadSQL = `
WITH RECURSIVE
rel AS (
  SELECT c.oid, n.nspname AS schema, c.relname AS name, c.relkind::text AS kind
  FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
  WHERE c.relkind IN ('r', 'p', 'v', 'm', 'f') AND n.nspname = ANY(%[1]s) AND NOT c.relispartition
),
fn AS (
  SELECT p.oid, n.nspname AS schema, p.proname AS name, p.prokind::text AS kind, p.prokind, p.proargtypes,
         p.proargnames, p.proargmodes, p.prorettype, p.proretset, p.proisstrict, p.provolatile,
         p.provariadic, p.pronargdefaults
  FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
  WHERE n.nspname = ANY(%[1]s) AND p.prokind IN ('f', 'p')
    AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.objid = p.oid AND d.deptype = 'e')
),
seed(oid) AS (
  SELECT a.atttypid FROM pg_attribute a JOIN rel ON rel.oid = a.attrelid WHERE a.attnum > 0 AND NOT a.attisdropped
  UNION SELECT unnest(string_to_array(fn.proargtypes::text, ' ')::oid[]) FROM fn WHERE fn.proargtypes::text <> ''
  UNION SELECT fn.prorettype FROM fn WHERE fn.prokind = 'f'
),
ref(oid) AS (
  SELECT oid FROM seed
  UNION
  SELECT x.oid FROM ref
    JOIN pg_type t ON t.oid = ref.oid
    LEFT JOIN pg_range r ON r.rngtypid = t.oid OR r.rngmultitypid = t.oid
    CROSS JOIN LATERAL (VALUES (t.typelem), (t.typbasetype), (r.rngsubtype), (r.rngtypid)) AS x(oid)
  WHERE x.oid IS NOT NULL AND x.oid <> 0
)
SELECT json_build_object(
 'server_version_num', current_setting('server_version_num')::int,
 'schemas', to_json(%[1]s),
 'tables', COALESCE((SELECT json_agg(json_build_object(
     'schema', rel.schema, 'name', rel.name, 'kind', rel.kind,
     'columns', COALESCE((SELECT json_agg(json_build_object(
          'num', a.attnum, 'name', a.attname, 'type_oid', a.atttypid::int,
          'type_name', format_type(a.atttypid, a.atttypmod),
          'not_null', (a.attnotnull AND COALESCE(nn.convalidated, true)),
          'has_default', a.atthasdef, 'identity', a.attidentity::text, 'generated', a.attgenerated::text
        ) ORDER BY a.attnum)
        FROM pg_attribute a
        LEFT JOIN pg_constraint nn ON nn.conrelid = a.attrelid AND nn.contype = 'n' AND nn.conkey = ARRAY[a.attnum]
        WHERE a.attrelid = rel.oid AND a.attnum > 0 AND NOT a.attisdropped), '[]'::json),
     'primary_key', COALESCE((SELECT json_agg(a.attname ORDER BY k.ord)
        FROM pg_index i
        CROSS JOIN LATERAL unnest(i.indkey::int2[]) WITH ORDINALITY k(attnum, ord)
        JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = k.attnum
        WHERE i.indrelid = rel.oid AND i.indisprimary), '[]'::json),
     'constraints', COALESCE((SELECT json_agg(json_build_object(
          'name', co.conname, 'type', co.contype::text,
          'columns', COALESCE((SELECT json_agg(a.attname ORDER BY u.ord)
              FROM unnest(co.conkey) WITH ORDINALITY u(attnum, ord)
              JOIN pg_attribute a ON a.attrelid = co.conrelid AND a.attnum = u.attnum), '[]'::json),
          'ref_schema', (SELECT n2.nspname FROM pg_class c2 JOIN pg_namespace n2 ON n2.oid = c2.relnamespace WHERE c2.oid = co.confrelid),
          'ref_table', (SELECT c2.relname FROM pg_class c2 WHERE c2.oid = co.confrelid),
          'ref_columns', (SELECT json_agg(a.attname ORDER BY u.ord)
              FROM unnest(co.confkey) WITH ORDINALITY u(attnum, ord)
              JOIN pg_attribute a ON a.attrelid = co.confrelid AND a.attnum = u.attnum)
        ) ORDER BY co.conname)
        FROM pg_constraint co WHERE co.conrelid = rel.oid AND co.contype IN ('p', 'u', 'f', 'x')), '[]'::json)
   ) ORDER BY rel.schema, rel.name) FROM rel), '[]'::json),
 'types', COALESCE((SELECT json_agg(json_build_object(
     'oid', t.oid::int, 'schema', n.nspname, 'name', t.typname, 'kind', t.typtype::text,
     'category', t.typcategory::text, 'elem', t.typelem::int, 'base', t.typbasetype::int,
     'range_subtype', COALESCE(r.rngsubtype, 0)::int, 'multi_of', COALESCE(mr.rngtypid, 0)::int,
     'extension', COALESCE((SELECT e.extname FROM pg_depend d JOIN pg_extension e ON e.oid = d.refobjid
                            WHERE d.objid = t.oid AND d.deptype = 'e' LIMIT 1), '')
   ) ORDER BY t.oid)
   FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace
   LEFT JOIN pg_range r ON r.rngtypid = t.oid
   LEFT JOIN pg_range mr ON mr.rngmultitypid = t.oid
   WHERE t.oid IN (SELECT oid FROM ref)), '[]'::json),
 'enums', COALESCE((SELECT json_agg(json_build_object('type_oid', x.typ::int, 'labels', x.labels) ORDER BY x.typ)
   FROM (SELECT e.enumtypid AS typ, json_agg(e.enumlabel ORDER BY e.enumsortorder) AS labels
         FROM pg_enum e WHERE e.enumtypid IN (SELECT oid FROM ref) GROUP BY e.enumtypid) x), '[]'::json),
 'funcs', COALESCE((SELECT json_agg(json_build_object(
     'schema', fn.schema, 'name', fn.name, 'kind', fn.kind,
     'arg_types', CASE WHEN fn.proargtypes::text = '' THEN '[]'::json ELSE to_json(string_to_array(fn.proargtypes::text, ' ')::int[]) END,
     'arg_names', COALESCE(to_json(fn.proargnames), '[]'::json),
     'arg_modes', COALESCE(to_json(fn.proargmodes), '[]'::json),
     'return_type', fn.prorettype::int, 'returns_set', fn.proretset, 'strict', fn.proisstrict,
     'volatility', fn.provolatile::text, 'has_variadic', fn.provariadic <> 0, 'has_defaults', fn.pronargdefaults > 0
   ) ORDER BY fn.schema, fn.name, fn.oid) FROM fn), '[]'::json)
)::text;
`

// Load reads the model for the given schemas from a live server.
func Load(c pgtool.Conn, schemas []string) (*Schema, error) {
	arr, err := textArray(schemas)
	if err != nil {
		return nil, err
	}
	out, err := c.Query(fmt.Sprintf(loadSQL, arr))
	if err != nil {
		return nil, fmt.Errorf("introspection failed: %w", err)
	}
	var s Schema
	if err := json.Unmarshal(out, &s); err != nil {
		return nil, fmt.Errorf("introspection returned unexpected JSON: %w", err)
	}
	fp, err := c.Query(strings.ReplaceAll(FingerprintSQL, "$1::text[]", arr))
	if err != nil {
		return nil, fmt.Errorf("fingerprint query failed: %w", err)
	}
	fps := map[string]string{}
	for _, line := range strings.Split(string(fp), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "|"); ok {
			fps[k] = v
		}
	}
	for i := range s.Tables {
		t := &s.Tables[i]
		t.Fingerprint = fps[t.Schema+"."+t.Name]
	}
	s.Normalize()
	return &s, nil
}
