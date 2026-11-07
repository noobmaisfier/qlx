#!/usr/bin/env bash
# Prepares a release tree: merged catalog ledger of every targeted major,
# regenerated packages and docs, go.sum of bench. The sub-modules are finished by release-modules.sh. Needs network access
# for `go mod tidy`, and one reachable server per major in QLX_PG_URLS.
#
#   QLX_PG_URLS="16=postgres://u@h16/db 17=postgres://u@h17/db 18=... 19=..." scripts/release.sh
#
# Without QLX_PG_URLS the ledger step is skipped and the committed
# pgcat.json is used as it is (only valid when it already lists every major).
set -euo pipefail
cd "$(dirname "$0")/.."

if [ -n "${QLX_PG_URLS:-}" ]; then
  files=()
  for kv in $QLX_PG_URLS; do
    v="${kv%%=*}"; url="${kv#*=}"
    DATABASE_URL="$url" go run ./cmd/pgcat extract -o "/tmp/pgcat-$v.json"
    files+=("/tmp/pgcat-$v.json")
  done
  go run ./cmd/pgcat merge -o pgcat.json "${files[@]}"
fi

go run ./cmd/pgcat versions -ledger pgcat.json -min 2 || { echo "set QLX_PG_URLS to merge every targeted major" >&2; exit 1; }

go run ./cmd/pgcat gen -ledger pgcat.json -o pgcat
for f in ledgers/*.json; do
  case "$(basename "$f")" in commands.json|grammar_map.json|regress.json) continue;; esac
  # An extension ledger must list the same majors as the catalog, or its
  # wrappers have no upper or lower bound on the servers it never saw.
  go run ./cmd/pgcat versions -ledger "$f" -like pgcat.json >/dev/null || { echo "extension ledger $f does not list every major: extract it on each server (pgcat extract -ext NAME) and merge" >&2; exit 1; }
  go run ./cmd/pgcat gen -ledger "$f" -o pgcat
done
go run ./cmd/pgcat docs -ledger pgcat.json -o docs/pgcat -pkg pg

# The root module has no dependencies and no go.sum: `go mod tidy` there would
# also read the files behind the sql2go build tag and add pg_query_go.
# pgxexec, otelqlx and analyzer require a published qlx version, so they are
# tidied after the root module is tagged, by scripts/release-modules.sh. bench
# keeps its replace and is never published.
(cd bench && go mod tidy)
scripts/check_root.sh
gofmt -l . | grep -v '^$' && { echo "gofmt differences above"; exit 1; } || true
echo "release tree ready: now run scripts/with_pg.sh go test ./... on each major,"
echo "commit, tag the root module (vX.Y.Z), then run scripts/release-modules.sh vX.Y.Z"
