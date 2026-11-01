#!/usr/bin/env bash
# Fails if the root module gained a dependency (go.mod `require`) or a go.sum.
# Run it after scripts/release.sh and scripts/sql2go.sh: `go mod tidy` and
# `go get` of the sql2go build tag can re-add pg_query_go.
set -euo pipefail
cd "$(dirname "$0")/.."
if grep -qE '^require|^[[:space:]]+[a-z0-9.-]+/' go.mod; then echo "root go.mod has dependencies:" >&2; cat go.mod >&2; exit 1; fi
[ ! -f go.sum ] || { echo "root go.sum exists: the root module must have no dependencies" >&2; exit 1; }
echo "root module has no dependencies"
