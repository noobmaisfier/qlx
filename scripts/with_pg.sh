#!/usr/bin/env bash
# Starts a throwaway PostgreSQL cluster (unix socket only), loads the fixture
# schema, runs the given command with QLX_PG_* set, then tears it down.
#
#   scripts/with_pg.sh go test ./...
#
# Needs PostgreSQL server binaries. Set PGBIN to override the location.
# When run as root it drops to the "postgres" user, as the server requires.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
PGBIN="${PGBIN:-$(ls -d /usr/lib/postgresql/*/bin 2>/dev/null | sort -V | tail -1)}"
[ -x "$PGBIN/initdb" ] || { echo "no PostgreSQL binaries found (set PGBIN)" >&2; exit 2; }
major="$("$PGBIN/postgres" --version | sed -E 's/.* ([0-9]+)(\.[0-9]+)?.*/\1/')"

work="$(mktemp -d)"
sock="$work/sock"
mkdir -p "$sock"
as_pg() { if [ "$(id -u)" = 0 ]; then runuser -u postgres -- "$@"; else "$@"; fi; }
if [ "$(id -u)" = 0 ]; then chown -R postgres "$work"; chmod 755 "$work"; fi

cleanup() { as_pg "$PGBIN/pg_ctl" -D "$work/data" -m immediate stop >/dev/null 2>&1 || true; rm -rf "$work"; }
trap cleanup EXIT

as_pg "$PGBIN/initdb" -D "$work/data" -A trust -E UTF8 >/dev/null
as_pg "$PGBIN/pg_ctl" -D "$work/data" -w -l "$work/log" \
  -o "-c listen_addresses='' -c unix_socket_directories='$sock' -c fsync=off" start >/dev/null

as_pg "$PGBIN/createdb" -h "$sock" qlx
as_pg "$PGBIN/psql" -h "$sock" -d qlx -X -q -v ON_ERROR_STOP=1 -f "$root/fixture/schema.sql"

export QLX_PG_HOST="$sock" QLX_PG_DB=qlx QLX_PG_VERSION="$major"
if [ "$(id -u)" = 0 ]; then export QLX_PG_USER=postgres; fi
echo "PostgreSQL $major ready on $sock" >&2
"$@"
