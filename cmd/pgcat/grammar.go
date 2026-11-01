package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/noobmaisfier/qlx/internal/grammar"
)

// trackedNonterminals are the parts of gram.y whose every production must be
// accounted for: the statements QLX builds, and the clause rules under them.
var trackedNonterminals = []string{
	"SelectStmt", "select_no_parens", "select_with_parens", "simple_select", "select_clause",
	"with_clause", "cte_list", "common_table_expr", "opt_materialized", "opt_search_clause", "opt_cycle_clause",
	"distinct_clause", "all_or_distinct", "from_clause", "from_list", "table_ref", "joined_table", "join_type", "join_qual",
	"where_clause", "group_clause", "group_by_list", "group_by_item", "empty_grouping_set", "rollup_clause", "cube_clause", "grouping_sets_clause",
	"having_clause", "window_clause", "window_definition", "window_specification", "frame_clause", "frame_extent", "frame_bound", "opt_window_exclusion_clause",
	"sort_clause", "sortby", "opt_asc_desc", "opt_nulls_order", "select_limit", "limit_clause", "offset_clause", "select_limit_value", "select_fetch_first_value", "row_or_rows", "first_or_next", "for_locking_clause", "for_locking_item", "for_locking_strength", "opt_nowait_or_skip",
	"values_clause", "set_operator", "set_operator_with_all_or_distinct",
	"InsertStmt", "insert_target", "insert_rest", "override_kind", "opt_on_conflict", "opt_conf_expr", "returning_clause", "returning_options", "returning_option", "returning_option_kind",
	"UpdateStmt", "set_clause_list", "set_clause", "set_target", "DeleteStmt", "using_clause",
	"MergeStmt", "merge_when_list", "merge_when_clause", "merge_when_tgt_matched", "merge_when_tgt_not_matched", "merge_update", "merge_delete", "merge_insert", "opt_merge_when_condition",
	"CopyStmt", "copy_from", "copy_options", "copy_generic_opt_elem", "copy_opt_item",
	"ExplainStmt", "explain_option_list", "explain_option_elem", "ExplainableStmt",
	"func_expr", "func_application", "func_expr_common_subexpr", "within_group_clause", "filter_clause", "over_clause",
	"json_table", "json_value_expr", "json_behavior", "json_aggregate_func", "json_object_constructor_null_clause_opt",
	"a_expr", "b_expr", "c_expr", "case_expr", "when_clause", "in_expr", "array_expr", "row", "type_list",
	"TransactionStmt", "VacuumStmt", "AnalyzeStmt", "TruncateStmt", "LockStmt", "NotifyStmt", "ListenStmt", "UnlistenStmt", "PrepareStmt", "ExecuteStmt", "DeallocateStmt", "DeclareCursorStmt", "FetchStmt", "ClosePortalStmt", "CallStmt", "DiscardStmt", "CheckPointStmt", "GrantStmt", "RevokeStmt", "VariableSetStmt", "VariableResetStmt",
}

func grammarCmd(y, mapPath, outPath, cheat string, fill bool, out io.Writer) error {
	if y == "" {
		return fmt.Errorf("grammar: -y gram.y is required (paths or https URLs, comma-separated: one per PostgreSQL version)")
	}
	var ledgers []*grammar.Ledger
	for _, src := range strings.Split(y, ",") {
		text, err := readSource(src)
		if err != nil {
			return err
		}
		g, err := grammar.Parse(text)
		if err != nil {
			return fmt.Errorf("%s: %w", src, err)
		}
		var tracked []string
		for _, nt := range trackedNonterminals {
			if _, ok := g.Rules[nt]; ok {
				tracked = append(tracked, nt)
			} else {
				fmt.Fprintf(out, "note: %s is not in %s (skipped)\n", nt, src)
			}
		}
		l, err := grammar.Build(g, src, tracked)
		if err != nil {
			return err
		}
		ledgers = append(ledgers, l)
	}
	l := grammar.Union(ledgers...)
	if outPath != "" {
		b, _ := json.MarshalIndent(l, "", "  ")
		if err := os.WriteFile(outPath, append(b, '\n'), 0o644); err != nil {
			return err
		}
	}
	m, err := grammar.LoadMap(mapPath)
	if err != nil {
		if !fill {
			return err
		}
		m = &grammar.Map{Statements: map[string]grammar.Entry{}, Productions: map[string]grammar.Entry{}}
	}
	if fill {
		n := grammar.Fill(l, m)
		b, _ := json.MarshalIndent(m, "", "  ")
		if err := os.WriteFile(mapPath, append(b, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(out, "added %d unmapped entries to %s\n", n, mapPath)
	}
	if cheat != "" {
		if err := os.WriteFile(cheat, []byte(grammar.Cheatsheet(m)), 0o644); err != nil {
			return err
		}
	}
	cov := grammar.Coverage(m)
	var keys []string
	for k := range cov {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(out, "%-32s %d\n", k, cov[k])
	}
	if ps := grammar.Check(l, m); len(ps) > 0 {
		for _, p := range ps {
			fmt.Fprintln(out, p)
		}
		return fmt.Errorf("grammar: %d unmapped, stale or incomplete entries", len(ps))
	}
	return nil
}

func readSource(y string) (string, error) {
	if strings.HasPrefix(y, "https://") {
		resp, err := http.Get(y)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("grammar: %s: %s", y, resp.Status)
		}
		b, err := io.ReadAll(resp.Body)
		return string(b), err
	}
	b, err := os.ReadFile(y)
	return string(b), err
}
