#!/usr/bin/env bash
# Extracts one or more extension packs from a server per PostgreSQL major,
# merges them into ledgers/<pack>.json, checks that the ledger lists the same
# majors as pgcat.json, shows what changed against the previous ledger, and
# regenerates pgcat/ext/<pack>.
#
#   QLX_PG_URLS="16=postgres://u:p@h:5416/db 17=... 18=... 19=..." \
#     scripts/extract_ext.sh pg_trgm hstore ltree citext
#
# `pgcat extract -ext` runs CREATE EXTENSION IF NOT EXISTS itself, so every
# server needs the extension files installed and a role allowed to create it
# (contrib extensions are in the official images). The ledger is whatever the servers report: never edit it by hand.
set -euo pipefail
cd "$(dirname "$0")/.."

[ -n "${QLX_PG_URLS:-}" ] && [ $# -gt 0 ] || { sed -n '2,12p' "$0" >&2; exit 2; }

for pack in "$@"; do
  # pack name (qlx.yaml, ledgers/) -> name in pg_extension -> generated directory
  case "$pack" in
    pgvector) ext=vector ;;
    *)        ext="$pack" ;;
  esac
  dir="pgcat/ext/${ext//_/}"
  old=""
  if [ -f "ledgers/$pack.json" ]; then old="$(mktemp)"; cp "ledgers/$pack.json" "$old"; fi

  files=()
  for kv in $QLX_PG_URLS; do
    v="${kv%%=*}"; url="${kv#*=}"
    out="/tmp/$pack-$v.json"
    DATABASE_URL="$url" go run ./cmd/pgcat extract -ext "$pack" -o "$out"
    files+=("$out")
  done

  go run ./cmd/pgcat merge -o "ledgers/$pack.json" "${files[@]}"
  go run ./cmd/pgcat versions -ledger "ledgers/$pack.json" -like pgcat.json
  if [ -n "$old" ]; then
    echo "== $pack: changes against the previous ledger"
    go run ./cmd/pgcat diff -report "$old" "ledgers/$pack.json"
    rm -f "$old"
  fi
  rm -rf "$dir"
  go run ./cmd/pgcat gen -ledger "ledgers/$pack.json" -o pgcat
  sha256sum "ledgers/$pack.json"
done
echo "now: go test ./... on every major, and scripts/with_pg.sh for the server tests of pgcat/ext/*"
