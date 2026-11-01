#!/usr/bin/env python3
"""Regenerates ledgers/grammar_map.json from `pgcat grammar -o` ledgers.

The map is hand-owned: this script only seeds it. Rules below decide the status
of each statement/production; anything the rules do not know stays "unmapped"
and fails `pgcat grammar`, which is the point.

usage: triage_grammar.py ledger18.json ledger19.json > ledgers/grammar_map.json
"""
import json, re, sys

DDL = "DDL: the schema is owned by SQL migrations (decision D2), QLX builds no DDL"
SUP = {
 "SelectStmt": "qlx.Select", "InsertStmt": "qlx.InsertInto", "UpdateStmt": "qlx.Update",
 "DeleteStmt": "qlx.DeleteFrom", "MergeStmt": "qlx.Merge", "CopyStmt": "qlx.CopyIn / CopyTableOut / CopyQueryOut",
 "ExplainStmt": "qlx.Explain", "TransactionStmt": "qlx.Begin / Commit / Rollback / Savepoint / PrepareTransaction",
 "PrepareStmt": "qlx.Prepare", "ExecuteStmt": "qlx.Execute", "DeallocateStmt": "qlx.Deallocate / DeallocateAll",
 "DeclareCursorStmt": "qlx.DeclareCursor", "FetchStmt": "qlx.FetchFrom / MoveIn", "ClosePortalStmt": "qlx.CloseCursor",
 "CallStmt": "qlx.Call", "ListenStmt": "qlx.Listen", "UnlistenStmt": "qlx.Unlisten / UnlistenAll", "NotifyStmt": "qlx.Notify",
 "VacuumStmt": "qlx.Vacuum", "AnalyzeStmt": "qlx.AnalyzeTables", "TruncateStmt": "qlx.Truncate", "LockStmt": "qlx.LockTable",
 "DiscardStmt": "qlx.DiscardAll / DiscardPlans / DiscardSequences / DiscardTemp", "CheckPointStmt": "qlx.Checkpoint",
 "GrantStmt": "qlx.Grant", "RevokeStmt": "qlx.Revoke", "GrantRoleStmt": "qlx.Grant", "RevokeRoleStmt": "qlx.Revoke",
 "VariableSetStmt": "qlx.SetConfig / SetRole / SetSessionAuthorization / SetTransaction", "VariableResetStmt": "qlx.Reset / ResetAll / ResetRole",
 "VariableShowStmt": "qlx.Show / CurrentSetting", "RefreshMatViewStmt": "qlx.RefreshMaterializedView", "ReindexStmt": "qlx.Reindex",
 "ClusterStmt": "qlx.Cluster", "ConstraintsSetStmt": "qlx.SetConstraints / SetConstraintsAll", "RepackStmt": "qlx.Repack",
 "WaitStmt": "qlx.WaitForLSN", "CreateRoleStmt": "qlx.NewRole",
}
OUT = {
 "DoStmt": "anonymous code blocks take their body as SQL/PL text, which QLX never accepts (no raw SQL in the API)",
 "LoadStmt": "loads a server-side shared library by file path: an administrator action, not an application query",
 "ReassignOwnedStmt": "role administration, run by operators with a superuser tool",
 "DropOwnedStmt": "role administration, run by operators with a superuser tool",
 "CreateUserStmt": "role administration (CREATE USER is CREATE ROLE ... LOGIN)",
 "CreateGroupStmt": "role administration", "AlterGroupStmt": "role administration", "AlterRoleStmt": "role administration",
 "AlterRoleSetStmt": "role administration", "DropRoleStmt": "role administration",
}
def stmt_entry(s):
    if s in SUP: return {"status": "supported", "symbol": SUP[s]}
    if s in OUT: return {"status": "out-of-scope", "reason": OUT[s]}
    return {"status": "out-of-scope", "reason": DDL}

NT = {
 "select_no_parens": "qlx.Select", "select_with_parens": "qlx.Select (parenthesised as a subquery or set-operation operand)",
 "simple_select": "qlx.Select / Values / Union / Intersect / Except", "select_clause": "qlx.Select",
 "SelectStmt": "qlx.Select", "with_clause": "SelectQuery.With", "cte_list": "SelectQuery.With", "common_table_expr": "qlx.CTE / Recursive",
 "opt_materialized": "CTE.Materialized / NotMaterialized", "opt_search_clause": "CTE.SearchDepthFirst / SearchBreadthFirst", "opt_cycle_clause": "CTE.Cycle",
 "distinct_clause": "SelectQuery.Distinct / DistinctOn", "all_or_distinct": "set operations", "from_clause": "SelectQuery.From", "from_list": "SelectQuery.From / Join",
 "table_ref": "FromItem: tables, qlx.Subquery, Lateral, JSONTable, table functions, TableSample", "joined_table": "SelectQuery.Join / LeftJoin / RightJoin / FullJoin / CrossJoin",
 "join_type": "Join family", "join_qual": "JoinBuilder.On / OnFK / Using", "where_clause": "SelectQuery.Where", "group_clause": "SelectQuery.GroupBy",
 "group_by_list": "SelectQuery.GroupBy", "group_by_item": "GroupBy, Rollup, Cube, GroupingSets", "empty_grouping_set": "qlx.SetOf()", "rollup_clause": "qlx.Rollup",
 "cube_clause": "qlx.Cube", "grouping_sets_clause": "qlx.GroupingSets", "having_clause": "SelectQuery.Having", "window_clause": "SelectQuery.Windows",
 "window_definition": "qlx.NewWindow", "window_specification": "qlx.Window()", "frame_clause": "WindowSpec.Rows / Range / Groups", "frame_extent": "WindowSpec frame",
 "frame_bound": "Preceding / Following / UnboundedPreceding / CurrentRow", "opt_window_exclusion_clause": "WindowSpec.Exclude*", "sort_clause": "SelectQuery.OrderBy",
 "sortby": "OrderTerm (Asc/Desc/NullsFirst/NullsLast, USING via operator wrappers)", "opt_asc_desc": "OrderTerm.Asc / Desc", "opt_nulls_order": "OrderTerm.NullsFirst / NullsLast",
 "select_limit": "SelectQuery.Limit / Offset / FetchFirst", "limit_clause": "SelectQuery.Limit / FetchFirst / WithTies", "offset_clause": "SelectQuery.Offset",
 "select_limit_value": "SelectQuery.Limit", "select_fetch_first_value": "SelectQuery.FetchFirst", "row_or_rows": "FetchFirst", "first_or_next": "FetchFirst",
 "for_locking_clause": "SelectQuery.ForUpdate / ForShare ...", "for_locking_item": "SelectQuery.ForUpdate ... Of", "for_locking_strength": "ForUpdate / ForNoKeyUpdate / ForShare / ForKeyShare",
 "opt_nowait_or_skip": "SkipLocked / NoWait", "values_clause": "qlx.Values1..4", "set_operator": "Union / Intersect / Except", "set_operator_with_all_or_distinct": "Union / UnionAll ...",
 "InsertStmt": "qlx.InsertInto", "insert_target": "qlx.InsertInto", "insert_rest": "InsertQuery.Row / Values / Columns / FromQuery / DefaultValues", "override_kind": "InsertQuery.OverridingSystemValue / OverridingUserValue",
 "opt_on_conflict": "InsertQuery.OnConflict ... DoNothing / DoUpdate / DoSelect", "opt_conf_expr": "OnConflict / OnConflictConstraint", "returning_clause": "Returning",
 "returning_options": "Returning with Old()/New()", "returning_option": "Returning with Old()/New()", "returning_option_kind": "Old / New",
 "UpdateStmt": "qlx.Update", "set_clause_list": "UpdateQuery.Set", "set_clause": "Column.To / ToX / ToDefault; multi-column form via SetRow", "set_target": "UpdateQuery.Set",
 "DeleteStmt": "qlx.DeleteFrom", "using_clause": "DeleteQuery.Using", "MergeStmt": "qlx.Merge", "merge_when_list": "MergeQuery.When*", "merge_when_clause": "MergeQuery.WhenMatched / WhenNotMatched",
 "merge_when_tgt_matched": "MergeQuery.WhenMatched", "merge_when_tgt_not_matched": "MergeQuery.WhenNotMatched / WhenNotMatchedBySource", "merge_update": "MergeQuery update action",
 "merge_delete": "MergeQuery delete action", "merge_insert": "MergeQuery insert action", "opt_merge_when_condition": "MergeQuery And(pred)",
 "CopyStmt": "qlx.CopyIn / CopyTableOut / CopyQueryOut", "copy_from": "CopyIn / CopyTableOut", "copy_options": "Copy options", "copy_generic_opt_elem": "WithFormat ... WithHeader", "copy_opt_item": "legacy COPY options: the WITH(...) forms are covered, the pre-9.0 spelling is not generated",
 "ExplainStmt": "qlx.Explain", "explain_option_list": "ExplainOption", "explain_option_elem": "ExplainOption", "ExplainableStmt": "qlx.Explain accepts Select/Insert/Update/Delete/Merge/Prepare-ables",
 "func_expr": "qlx.FuncCall and generated wrappers", "func_application": "generated pgcat wrappers; FuncCall; AggExpr", "func_expr_common_subexpr": "see key",
 "within_group_clause": "OrderedAgg.WithinGroup", "filter_clause": "AggExpr.Filter", "over_clause": "AggExpr.Over / WinFn.Over", "json_table": "qlx.JSONTable",
 "json_value_expr": "JSON constructors", "json_behavior": "JSONValue/Query/Exists On* methods", "json_aggregate_func": "qlx.JSONObjectAgg / JSONArrayAgg", "json_object_constructor_null_clause_opt": "JSONObjectBuilder.NullOnNull / AbsentOnNull",
 "a_expr": "see key", "b_expr": "see key", "c_expr": "see key", "case_expr": "qlx.Case", "when_clause": "CaseBuilder.When", "in_expr": "Expr.In / InSelect", "array_expr": "qlx.Array / ArrayFromQuery",
 "row": "qlx.Row", "type_list": "Prepare/Declare", "TransactionStmt": "qlx.Begin / Commit / Rollback / Savepoint family", "VacuumStmt": "qlx.Vacuum", "AnalyzeStmt": "qlx.AnalyzeTables",
 "TruncateStmt": "qlx.Truncate", "LockStmt": "qlx.LockTable", "NotifyStmt": "qlx.Notify", "ListenStmt": "qlx.Listen", "UnlistenStmt": "qlx.Unlisten / UnlistenAll", "PrepareStmt": "qlx.Prepare",
 "ExecuteStmt": "qlx.Execute", "DeallocateStmt": "qlx.Deallocate / DeallocateAll", "DeclareCursorStmt": "qlx.DeclareCursor", "FetchStmt": "qlx.FetchFrom / MoveIn", "ClosePortalStmt": "qlx.CloseCursor",
 "CallStmt": "qlx.Call", "DiscardStmt": "qlx.Discard*", "CheckPointStmt": "qlx.Checkpoint", "GrantStmt": "qlx.Grant", "RevokeStmt": "qlx.Revoke", "VariableSetStmt": "qlx.SetConfig / SetRole / SetSessionAuthorization / SetTransaction",
 "VariableResetStmt": "qlx.Reset / ResetAll / ResetRole",
}
# (regex on production key, entry). First match wins.
OVERRIDE = [
 (r"XML|xml|DOCUMENT_P", {"status": "unsupported", "reason": "XML keyword syntax is not modelled; XML functions that are ordinary calls are generated in pgcat/xml"}),
 (r"^a_expr: UNIQUE ", {"status": "unsupported", "reason": "the server parses UNIQUE (subquery) and then rejects it as not implemented"}),
 (r"TREAT", {"status": "unsupported", "reason": "the server parses TREAT and then rejects it as not implemented"}),
 (r"^func_expr_common_subexpr: (CURRENT_TIME|LOCALTIME)\b(?! ?'\(' Iconst)|^func_expr_common_subexpr: (CURRENT_TIME|LOCALTIME) '\(' Iconst", {"status": "unsupported", "reason": "time-of-day (time/timetz) has no Go type in the default mapping; CURRENT_TIMESTAMP and LOCALTIMESTAMP are supported"}),
 (r"^func_expr_common_subexpr: JSON '\(' json_value_expr|JSON_SERIALIZE|JSON_SCALAR", {"status": "unsupported", "reason": "tracked gap: JSON() / JSON_SERIALIZE / JSON_SCALAR constructors are not built yet (the catalog wrappers json_scalar-style functions exist in pgcat/jsonb)"}),
 (r"^func_expr_common_subexpr: JSON_(OBJECT|ARRAY)", {"status": "supported", "symbol": "qlx.JSONObject / JSONArray"}),
 (r"^func_expr_common_subexpr: JSON_(EXISTS|QUERY|VALUE)", {"status": "supported", "symbol": "qlx.JSONExists / JSONQuery / JSONValue"}),
 (r"^func_expr_common_subexpr: MERGE_ACTION", {"status": "supported", "symbol": "qlx.MergeAction"}),
 (r"^func_expr_common_subexpr: (CAST|COALESCE|GREATEST|LEAST|NULLIF|EXTRACT)", {"status": "supported", "symbol": "qlx.Cast / Coalesce / Greatest / Least / NullIf / Extract"}),
 (r"^func_expr_common_subexpr: (CURRENT_USER|SESSION_USER|SYSTEM_USER|USER|CURRENT_ROLE|CURRENT_SCHEMA|CURRENT_CATALOG|CURRENT_DATE|CURRENT_TIMESTAMP|LOCALTIMESTAMP)", {"status": "supported", "symbol": "qlx.CurrentUser / SessionUser / SystemUser / CurrentRole / CurrentSchema / CurrentCatalog / CurrentDate / CurrentTimestamp[P] / LocalTimestamp[P] (USER is CURRENT_USER)"}),
 (r"^func_expr_common_subexpr: (NORMALIZE|OVERLAY|POSITION|SUBSTRING|TRIM|COLLATION)", {"status": "supported", "symbol": "pgcat/text wrappers (function form: normalize, overlay, position, substring, btrim/ltrim/rtrim), pg_collation_for"}),
 (r"^a_expr: a_expr (NOT_LA )?(LIKE|ILIKE|SIMILAR)", {"status": "supported", "symbol": "TextExpr.Like / ILike / NotLike / NotILike / SimilarTo / NotSimilarTo / LikeEscape"}),
 (r"^a_expr: a_expr (NOT_LA )?BETWEEN", {"status": "supported", "symbol": "NumExpr/TimeExpr.Between / NotBetween / BetweenSymmetric / NotBetweenSymmetric"}),
 (r"^a_expr: a_expr (NOT_LA )?IN_P", {"status": "supported", "symbol": "Expr.In / InSelect / NotInSelect"}),
 (r"^a_expr: a_expr IS (NOT )?DISTINCT", {"status": "supported", "symbol": "IsDistinctFrom / IsNotDistinctFrom"}),
 (r"^a_expr: a_expr IS (NOT )?(NORMALIZED|unicode_normal_form NORMALIZED)", {"status": "supported", "symbol": "pgcat/text.IsNormalized"}),
 (r"^a_expr: a_expr IS (NOT )?json_predicate", {"status": "supported", "symbol": "qlx.IsJSON"}),
 (r"^a_expr: a_expr IS (NOT )?(TRUE_P|FALSE_P|UNKNOWN)", {"status": "supported", "symbol": "Pred.IsTrue / IsFalse / IsUnknown / IsNot*"}),
 (r"^a_expr: a_expr (IS NULL_P|IS NOT NULL_P|ISNULL|NOTNULL)", {"status": "supported", "symbol": "Expr.IsNull / IsNotNull"}),
 (r"^a_expr: a_expr AT ", {"status": "supported", "symbol": "TimeExpr.AtTimeZone / AtTimeZoneX / AtLocal (Go 1.27 generic methods)"}),
 (r"^a_expr: a_expr COLLATE", {"status": "supported", "symbol": "Expr.Collate"}),
 (r"^a_expr: a_expr TYPECAST|^b_expr: b_expr TYPECAST", {"status": "supported", "symbol": "qlx.Cast / Expr.Cast (the :: spelling renders as CAST)"}),
 (r"^a_expr: a_expr subquery_Op sub_type", {"status": "supported", "symbol": "qlx.AnySelect / AllSelect; Expr.Any / NotAll for array operands"}),
 (r"^a_expr: DEFAULT", {"status": "supported", "symbol": "Column.ToDefault"}),
 (r"^a_expr: row OVERLAPS", {"status": "supported", "symbol": "qlx.Overlaps"}),
 (r"^a_expr: (a_expr qual_Op|qual_Op)|^b_expr: (b_expr qual_Op|qual_Op)", {"status": "supported", "symbol": "generated operator wrappers (pgcat/<area>/ops.go)"}),
 (r"^c_expr: GROUPING", {"status": "supported", "symbol": "qlx.GroupingFn"}),
 (r"^c_expr: EXISTS", {"status": "supported", "symbol": "qlx.Exists"}),
 (r"^c_expr: ARRAY select_with_parens", {"status": "supported", "symbol": "qlx.ArrayFromQuery"}),
 (r"^c_expr: ARRAY array_expr", {"status": "supported", "symbol": "qlx.Array / generated array wrappers"}),
 (r"^c_expr: (select_with_parens|PARAM)", {"status": "supported", "symbol": "qlx.Scalar / ScalarN; qlx.Param[T]"}),
 (r"^c_expr: AexprConst", {"status": "supported", "symbol": "qlx.Val (always a bound parameter)"}),
 (r"^c_expr: (columnref|'\(' a_expr)", {"status": "supported", "symbol": "table column symbols; parenthesisation is automatic"}),
 (r"^c_expr: (explicit_row|implicit_row)", {"status": "supported", "symbol": "qlx.Row"}),
 (r"^c_expr: (case_expr|func_expr)", {"status": "supported", "symbol": "qlx.Case; function wrappers"}),
 (r"^(a_expr|b_expr): ('[-+*/%<>^=]'|[A-Z_]+_EQUALS|NOT_EQUALS|AND|OR|NOT|NOT_LA|c_expr)", {"status": "supported", "symbol": "NumExpr arithmetic and comparison methods, Pred.And / Or, qlx.Not, generated operator wrappers"}),
 (r"^(a_expr|b_expr): (a_expr|b_expr) ('[-+*/%<>^=]'|GREATER_EQUALS|LESS_EQUALS|NOT_EQUALS) ", {"status": "supported", "symbol": "NumExpr arithmetic and comparison methods; generated operator wrappers"}),
 (r"^a_expr: a_expr (AND|OR)", {"status": "supported", "symbol": "Pred.And / Or"}),
 (r"^a_expr: '[-+]' a_expr|^b_expr: '[-+]' b_expr", {"status": "supported", "symbol": "generated prefix-operator wrappers"}),
 (r"^b_expr: b_expr IS", {"status": "supported", "symbol": "IsDistinctFrom / IsNotDistinctFrom"}),
 (r"^copy_opt_item", {"status": "partial", "symbol": "WithFormat / WithDelimiter / WithNull / WithHeader / WithQuote / WithForceQuote", "reason": "the pre-9.0 option spellings render as the modern WITH (...) form; PostgreSQL accepts both"}),
 (r"^func_application: func_name '\(' VARIADIC|^func_application: func_name '\(' func_arg_list ',' VARIADIC", {"status": "partial", "symbol": "generated wrappers for variadic functions take a slice", "reason": "VARIADIC is applied by the wrapper, not spelled by the caller"}),
 (r"^func_application", {"status": "supported", "symbol": "generated pgcat wrappers, AggExpr (DISTINCT via CountDistinct, ORDER BY via OrderedAgg)"}),
 (r"^table_ref: .*xmltable", {"status": "unsupported", "reason": "XMLTABLE is not modelled (XML)"}),
]
def prod_entry(key, nt):
    for rx, e in OVERRIDE:
        if re.search(rx, key):
            return dict(e)
    sym = NT.get(nt)
    if sym and sym != "see key":
        return {"status": "supported", "symbol": sym}
    return {"status": "unmapped"}

out = {"statements": {}, "productions": {}}
for path in sys.argv[1:]:
    l = json.load(open(path))
    for s in l["statements"]:
        out["statements"][s] = stmt_entry(s)
    for p in l["productions"]:
        k = (p["nt"] + ": " + p["alt"]) if p["alt"] else p["nt"] + ": <empty>"
        out["productions"][k] = prod_entry(k, p["nt"])
json.dump(out, sys.stdout, indent=2, sort_keys=True)
print()
