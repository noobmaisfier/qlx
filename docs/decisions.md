# Decision log

Decisions D1–D8 are in the plan. This log adds the implementation decisions that deviate from, or settle, a point the plan left open.

| # | Decision | Reason |
|---|---|---|
| D2 | No DDL builders | schema-first; migrations stay SQL |
| D9a | Config is YAML (`qlx.yaml`), JSON still read | plan says YAML; `internal/miniyaml` parses the subset used so the module stays free of dependencies |
| D9b | One module for CLI and library; `pgxexec`, `otelqlx`, `analyzer`, `bench` are separate modules | the CLI needs no separate version; modules with third-party dependencies stay out of the core |
| D9c | Core in package `qlx`, dialect rules in the same package (no `dialect/pg` split) | one package keeps the sealed AST unexported; the dialect is a value (`Dialect{Version}`), not a package |
| D9d | uuid is the standard library `uuid.UUID`; `uuid: bytes` in config yields `[16]byte` | Go 1.27 ships the type |
| D9e | JSONB columns are `qt.JSONB[T]`; `qt.JSON[T]` is a deprecated alias | the SQL type is jsonb; json is the rare case |
| D9f | Generated wrappers are split into area packages `pgcat/<area>` with a flat `pg` facade | one package of ~20k symbols is unusable in gopls; areas are importable alone |
| D9g | Nullability is `qt.Null[T]` only; pointer mode is rejected | one representation, one set of rules (S1 encoding C) |
| D9h | The analyzer is a separate module on `go/analysis` and the only static checker; `qlx vet` (the earlier stdlib-only subset) was removed | `x/tools` must not enter the core module; two tools with the same rule numbers confused users |
| D9i | Generic methods (Go 1.27) are used where a type parameter belongs to the call, not the receiver: `Expr.Cast[To]`, `TimeExpr.AtTimeZone[To]`, `Client.All[R]` | read left to right; package-level forms remain for operands that are not an `Expr` |
| D9j | The grammar ledger has one mapping file for all pinned versions | a production present in any pinned version must be mapped; stale entries fail |
| D9k | Strings are never SQL. Read-side helpers (`QueryName`, `Pretty`, `DescribeSQL`) take rendered SQL and are allow-listed in the API audit test | keeps "no API accepts SQL text" testable |
| D9l | The automatic static path hashes the statement by running the renderer in hash mode (no string building, same traversal), instead of keeping an incrementally maintained hash in every builder | one traversal, no per-node hash code to keep in step with the renderer; measured about 1.3 µs and 13 allocations for a simple select including building it, against 9.4 µs for build and render. The plan's 300 ns / 0 allocation figure for the lookup is not met; reaching it needs an incrementally maintained hash and is left as an optimisation |
| D9m | `date`, `time` and `timetz` are `qt.Date`, `qt.TimeOfDay` and `qt.TimeTZ`; `timestamptz` is `qt.Timestamptz` by default (`timestamptz: stdlib` in config gives `time.Time`) and `timestamp` stays `qt.LocalTime`; `date: stdlib` in config restores `time.Time` for dates | a date has no time and no zone, so `time.Time` lets a zone leak in and makes `==` unreliable; the three types are comparable value types (an int32 of days, an int64 of microseconds), carry PostgreSQL's `infinity`, and scan through `sql.Scanner` so pgx needs no registration |
| D9n | `ids: string` types integer primary keys and the foreign keys that reference them as `qt.ID[Space]`: an integer in the database, an 11-character string in JSON, URLs, logs and `fmt`. The codec is home-made: a keyed 6-round Feistel permutation of the 48-bit value, a keyed 16-bit tag, base58 at fixed length | the database keeps integers (joins, indexes, ordering unchanged) while the Go side cannot confuse a user id with an order id and nothing leaks numbering. Fixed length hides magnitude; the tag rejects typos and forged strings (a forged string passes with probability 2^-16); keys are derived per table from one secret on the stack, so no registry or lock; about 90 ns and 0 allocations per encode or decode. It is obfuscation, not encryption, and not Hashids-compatible. The key is set with `qt.SetIDKey`, never has a default, and an unset key is an error rather than a silent fallback |
