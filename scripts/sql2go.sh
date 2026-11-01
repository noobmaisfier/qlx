#!/usr/bin/env bash
# Builds and tests the optional SQL-to-Go converter. It parses with PostgreSQL's
# own parser (pg_query_go), which needs cgo and a C compiler; nothing else in
# the repository does, which is why it is behind a build tag.
set -euo pipefail
cd "$(dirname "$0")/.."
command -v gcc >/dev/null || command -v cc >/dev/null || { echo "sql2go needs a C compiler (gcc or cc)" >&2; exit 2; }
# `go get` adds pg_query_go to the root go.mod; the root module has no
# dependencies otherwise, so put go.mod and go.sum back when done.
keep=$(mktemp -d); cp go.mod "$keep/go.mod"; [ -f go.sum ] && cp go.sum "$keep/go.sum" || true
trap 'cp "$keep/go.mod" go.mod; if [ -f "$keep/go.sum" ]; then cp "$keep/go.sum" go.sum; else rm -f go.sum; fi' EXIT
CGO_ENABLED=1 go get github.com/pganalyze/pg_query_go/v6
CGO_ENABLED=1 go test -tags sql2go ./internal/sql2go/ ./cmd/qlx/
echo "build the tool with: go build -tags sql2go -o qlx ./cmd/qlx"
