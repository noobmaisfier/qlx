# Watch list and re-pin plan

## Experimental nodes

Features reverted during the PostgreSQL 19 betas are not part of the 100% and have no public API. They are tracked in the grammar ledger the day they reappear:

| Feature | Status | Where it will land |
|---|---|---|
| SQL/PGQ `GRAPH_TABLE` | reverted in beta | `from` item, behind `ExperimentalGraphTable` |
| `GROUP BY ALL` | reverted in beta | `SelectQuery.GroupByAll` |
| `FOR PORTION OF` | reverted in beta | `UpdateQuery.ForPortionOf` / `DeleteQuery.ForPortionOf` |
| Partition `MERGE` / `SPLIT` | DDL | out of scope |

`pgcat grammar` fails on a new production, so a reverted feature that returns cannot go unnoticed.

## Re-pinning

| Date | Action |
|---|---|
| November 12, 2026 (RC2) | `pgcat extract` on RC2, `pgcat diff pgcat.json new.json`, `pgcat grammar` against the new tag, review the release-note diff, regenerate (`pgcat gen`) |
| November 19, 2026 (GA) | repeat on the GA tag; update `Targets` defaults if the major changed; tag |
| each later major | ledger diff within a week of GA, full support within four weeks |

Commands:

```
pgcat extract -o pg19.json                       # against a 19 server
pgcat merge -o pgcat.json pg18.json pg19.json    # entries get since/until markers
pgcat diff -report old.json pgcat.json           # what changed (without -report the exit status is 1 when they differ)
pgcat grammar -y <18 gram.y>,<19 gram.y> -map ledgers/grammar_map.json
pgcat gen -ledger pgcat.json -o pgcat
```
