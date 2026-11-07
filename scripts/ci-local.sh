#!/usr/bin/env bash
# ci-local.sh runs the GitHub workflows of this repository on the local machine,
# with Docker, without act. It mirrors .github/workflows/ci.yml,
# .github/workflows/postgis.yml and .github/workflows/fuzz.yml step by step:
# same images, same environment, same commands, same failure conditions.
#
#   scripts/ci-local.sh                     # every "fast" job (see below)
#   scripts/ci-local.sh full                # + fuzz (10M) + postgis
#   scripts/ci-local.sh test                # the 16-way matrix
#   scripts/ci-local.sh test --pg 18 --pgx latest --race
#   scripts/ci-local.sh ledgers extensions bench
#   scripts/ci-local.sh postgis
#   scripts/ci-local.sh fuzz --fuzztime 100000x
#   scripts/ci-local.sh regress             # manual tool, needs a config+baseline
#
# Jobs: test, ledgers, extensions, bench, fuzz, postgis, regress.
# "all" (default) = test ledgers extensions bench.
# "full" = all + fuzz + postgis.
#
# Ports are assigned by Docker (127.0.0.1::5432), so nothing collides with a
# PostgreSQL already listening on 5432. Containers are named qlx-ci-* and are
# removed at the end unless --keep is given.
#
# Requirements: docker (with the daemon reachable), go, psql, git, gcc for
# regress (cgo, pg_query_go). Network for the ledger and extension jobs.
#
# Differences from GitHub are deliberate and listed by `--help`: the toolchain
# is your local `go` (not setup-go, not `tip`), and nothing is cached between
# runs. Everything that touches the repository is restored: the pgx pin writes
# go.work and the sql2go `go get` writes go.mod/go.sum; a trap puts them back.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

# ---------------------------------------------------------------- arguments
JOBS=()
PG_MAJORS=(16 17 18 19)
PGX_MODES=(latest min)
RACE=0
KEEP=0
VERBOSE=0
FUZZ_TIME=10000000x
BENCH_COUNT=5
PGX_MIN=v5.7.0

usage() { awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "$0"; }

while [ $# -gt 0 ]; do
	case "$1" in
	-h | --help) usage; exit 0 ;;
	--pg) PG_MAJORS=(${2//,/ }); shift 2 ;;
	--pg=*) PG_MAJORS=(${1#*=}); PG_MAJORS=(${PG_MAJORS//,/ }); shift ;;
	--pgx) PGX_MODES=(${2//,/ }); shift 2 ;;
	--pgx=*) PGX_MODES=(${1#*=}); PGX_MODES=(${PGX_MODES//,/ }); shift ;;
	--race) RACE=1; shift ;;
	--keep) KEEP=1; shift ;;
	-v | --verbose) VERBOSE=1; shift ;;
	--fuzztime) FUZZ_TIME=$2; shift 2 ;;
	--fuzztime=*) FUZZ_TIME=${1#*=}; shift ;;
	--bench-count) BENCH_COUNT=$2; shift 2 ;;
	--bench-count=*) BENCH_COUNT=${1#*=}; shift ;;
	-*) echo "ci-local: unknown flag $1 (try --help)" >&2; exit 2 ;;
	*) JOBS+=("$1"); shift ;;
	esac
done

if [ ${#JOBS[@]} -eq 0 ]; then JOBS=(all); fi

# ------------------------------------------------------------------ helpers
BOLD=$'\033[1m' RED=$'\033[31m' GREEN=$'\033[32m' DIM=$'\033[2m' OFF=$'\033[0m'
if [ ! -t 1 ]; then BOLD= RED= GREEN= DIM= OFF=; fi

log() { printf '\n%s==> %s%s\n' "$BOLD" "$*" "$OFF"; }
note() { printf '%s    %s%s\n' "$DIM" "$*" "$OFF"; }
die() {
	printf '%scil-local: %s%s\n' "$RED" "$*" "$OFF" >&2
	exit 2
}
need() { command -v "$1" >/dev/null 2>&1 || die "missing '$1' (required for $2)"; }

CACHE=${QLX_CI_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/qlx-ci}
mkdir -p "$CACHE"
TMP=$(mktemp -d "${TMPDIR:-/tmp}/qlx-ci.XXXXXX")
CONTAINERS=()
PGPASS=qlx

cleanup() {
	local c
	if [ "$KEEP" = 1 ]; then
		note "containers kept: ${CONTAINERS[*]:-none}"
		return
	fi
	for c in "${CONTAINERS[@]:-}"; do
		[ -n "$c" ] && docker rm -f "$c" >/dev/null 2>&1
	done
}

# Files that a `go get` rewrites are put back even on failure or Ctrl-C.
EXIT_HOOKS=()
on_exit() {
	local h
	for h in "${EXIT_HOOKS[@]:-}"; do
		[ -n "$h" ] && eval "$h" || true
	done
	cleanup
	rm -rf "$TMP"
}
trap on_exit EXIT
trap 'exit 130' INT TERM

# start_pg <name> <image> [docker run flags...] sets CI_PG_PORT.
start_pg() {
	local name=$1 image=$2 i
	shift 2
	docker rm -f "$name" >/dev/null 2>&1 || true
	CONTAINERS+=("$name")
	docker run -d --name "$name" -e POSTGRES_PASSWORD="$PGPASS" \
		-p 127.0.0.1::5432 "$@" "$image" >/dev/null
	for i in $(seq 1 90); do
		docker exec "$name" pg_isready -U postgres >/dev/null 2>&1 && break
		sleep 1
	done
	docker exec "$name" pg_isready -U postgres >/dev/null 2>&1 ||
		die "container $name ($image) did not become ready"
	CI_PG_PORT=$(docker port "$name" 5432/tcp | head -1 | sed 's/.*://')
	[ -n "$CI_PG_PORT" ] || die "could not read the published port of $name"
}

# psql_in <container> <db> [psql args...] — runs inside the container
psql_in() {
	local name=$1 db=$2
	shift 2
	docker exec -i -e PGPASSWORD="$PGPASS" "$name" \
		psql -U postgres -d "$db" -X -q -v ON_ERROR_STOP=1 "$@"
}

# prepare_dbs <container> loads fixture/schema.sql into qlx and qlx_test, as CI does.
prepare_dbs() {
	local name=$1
	psql_in "$name" postgres -c 'CREATE DATABASE qlx'
	psql_in "$name" qlx <fixture/schema.sql
	psql_in "$name" postgres -c 'CREATE DATABASE qlx_test'
	psql_in "$name" qlx_test <fixture/schema.sql
}

# export_pg <port> <major> points the test suite at a container.
# PGUSER is deliberately not exported: the cmd/qlx tests start a throwaway
# cluster with initdb, whose superuser is the current user, and PGUSER would
# make their createdb connect as "postgres" and fail.
export_pg() {
	export PGPASSWORD=$PGPASS PGHOST=127.0.0.1 PGPORT=$1
	export QLX_PG_HOST=127.0.0.1 QLX_PG_USER=postgres QLX_PG_DB=qlx
	export QLX_PG_VERSION=$2
	export QLX_TEST_DSN="postgres://postgres:$PGPASS@127.0.0.1:$1/qlx_test?sslmode=disable"
}

# restore_file <backup-dir> <file> puts back a file that a `go get` rewrote.
restore_file() {
	if [ -f "$1/$(basename "$2")" ]; then
		cp "$1/$(basename "$2")" "$2"
	else
		rm -f "$2"
	fi
}

GO_TEST_FLAGS=(-count=1)
[ "$RACE" = 1 ] && GO_TEST_FLAGS+=(-race)

# ------------------------------------------------------------------- jobs
job_test() {
	need docker test
	local pg pgx image port rc bad=0 tag
	for pg in "${PG_MAJORS[@]}"; do
		case "$pg" in
		19) image=postgres:19rc1 ;;
		*) image="postgres:$pg" ;;
		esac
		for pgx in "${PGX_MODES[@]}"; do
			tag="pg$pg-$pgx"
			log "test: PostgreSQL $pg, pgx $pgx"
			rc=0
			start_pg "qlx-ci-test-$tag" "$image"
			port=$CI_PG_PORT
			prepare_dbs "qlx-ci-test-$tag"
			export_pg "$port" "$pg"

			# CI pins the floor version through go.work (a replace), then runs everything.
			local bak="$TMP/pgx-$tag"
			if [ "$pgx" = min ]; then
				mkdir -p "$bak"
				cp go.work "$bak"
				cp go.work.sum "$bak" 2>/dev/null || true
				EXIT_HOOKS+=("restore_file '$bak' go.work; restore_file '$bak' go.work.sum")
				if ! { go work edit -replace "github.com/jackc/pgx/v5=github.com/jackc/pgx/v5@$PGX_MIN" &&
					(cd pgxexec && go mod download) >/dev/null 2>&1; }; then
					echo "  cannot pin pgx $PGX_MIN" >&2
					rc=1
				fi
			fi

			if [ $rc = 0 ]; then
				run_step "$tag" "go vet ./..." go vet ./... || rc=1
				run_step "$tag" "go test (root)" go test "${GO_TEST_FLAGS[@]}" ./... || rc=1
				run_step "$tag" "go test (pgxexec)" \
					bash -c 'cd pgxexec && go test '"${GO_TEST_FLAGS[*]}"' ./...' || rc=1
				run_step "$tag" "go test (otelqlx)" \
					bash -c 'cd otelqlx && go test '"${GO_TEST_FLAGS[*]}"' ./...' || rc=1
				run_step "$tag" "go test (analyzer)" \
					bash -c 'cd analyzer && go test '"${GO_TEST_FLAGS[*]}"' ./...' || rc=1
				out=$(gofmt -l .)
				if [ -n "$out" ]; then
					echo "  gofmt: files need formatting:" >&2
					printf '    %s\n' $out >&2
					rc=1
				fi
			fi

			if [ "$pgx" = min ]; then
				restore_file "$bak" pgxexec/go.mod
				restore_file "$bak" pgxexec/go.sum
			fi
			unset QLX_PG_HOST QLX_PG_USER QLX_PG_DB QLX_PG_VERSION QLX_TEST_DSN PGPORT
			docker rm -f "qlx-ci-test-$tag" >/dev/null 2>&1
			[ $rc = 0 ] || bad=1
		done
	done
	return $bad
}

# run_step <tag> <label> <command...> streams the output of one step.
run_step() {
	local tag=$1 label=$2
	shift 2
	if [ "$VERBOSE" = 1 ]; then
		"$@"
	else
		if ! out=$("$@" 2>&1); then
			printf '  %sFAIL%s %s\n' "$RED" "$OFF" "$label" >&2
			printf '%s\n' "$out" | tail -40 | sed 's/^/    /' >&2
			return 1
		fi
		printf '  ok   %s\n' "$label"
	fi
}

job_ledgers() {
	need docker ledgers
	need git "a checkout"
	local rc=0

	# ci.yml: grammar ledger against PG 18 and master (network).
	run_step ledgers "grammar ledger (18 + master)" \
		go run ./cmd/pgcat grammar \
		-y https://raw.githubusercontent.com/postgres/postgres/REL_18_STABLE/src/backend/parser/gram.y,https://raw.githubusercontent.com/postgres/postgres/master/src/backend/parser/gram.y \
		-map ledgers/grammar_map.json || rc=1
	run_step ledgers "command ledger" go test -run TestCommandLedger . || rc=1
	run_step ledgers "generated code is current" \
		go test ./cmd/pgcat -run TestGeneratedPackagesAreUpToDate || rc=1
	return $rc
}

job_extensions() {
	need docker extensions
	local rc=0 bad=0
	local -a rows=(
		"pgvector|pgvector/pgvector:pg18|vector"
		"pg_trgm|postgres:18|pgtrgm"
		"hstore|postgres:18|hstore"
		"ltree|postgres:18|ltree"
		"citext|postgres:18|citext"
	)
	# postgis is in job_postgis: its ledger comes from PGDG, not from an image.
	for row in "${rows[@]}"; do
		IFS='|' read -r pack image pkg <<<"$row"
		log "extensions: $pack on PG 18"
		start_pg "qlx-ci-ext-$pack" "$image"
		port=$CI_PG_PORT
		export_pg "$port" 18
		export QLX_PG_DB=postgres
		export DATABASE_URL="postgres://postgres:$PGPASS@127.0.0.1:$port/postgres?sslmode=disable"

		run_step "$pack" "extract + diff ledger" bash -c '
			set -e
			go run ./cmd/pgcat extract -ext "'"$pack"'" -o "'"$TMP/$pack.json"'"
			if [ -f ledgers/'"$pack"'.json ]; then
				go run ./cmd/pgcat diff ledgers/'"$pack"'.json "'"$TMP/$pack.json"'"
			fi' || rc=1
		run_step "$pack" "prepare every generated wrapper" \
			go test -count=1 "./pgcat/ext/$pkg" || rc=1
		if [ "$pack" = pgvector ]; then
			# ci.yml runs this one against the "postgres" database, not qlx_test.
			run_step "$pack" "pgvector round trip" \
				env QLX_TEST_DSN="postgres://postgres:$PGPASS@127.0.0.1:$port/postgres?sslmode=disable" \
				bash -c 'cd pgxexec && go test -count=1 -run TestPGVectorExtension .' || rc=1
		fi
		unset DATABASE_URL QLX_PG_HOST QLX_PG_USER QLX_PG_DB QLX_PG_VERSION QLX_TEST_DSN PGPORT
		docker rm -f "qlx-ci-ext-$pack" >/dev/null 2>&1
	done
	[ $rc = 0 ] || bad=1
	return $bad
}

job_regress() {
	need docker regress
	need git "a checkout"
	need gcc "sql2go (cgo)"
	local rc=0 src="$CACHE/postgres-REL_18_STABLE"
	# `qlx regress` is a manual tool, not a CI job: it needs a qlx.yaml +
	# schema.lock to resolve names, and a baseline ledger to compare against.
	# Neither is shipped, and with the fixture schema only 4 of the ~50,000
	# regression statements translate (a 9 MB ledger that gates nothing), so
	# the job reports why instead of failing on a missing file.
	if [ ! -f qlx.yaml ] && [ ! -f qlx.yml ] && [ ! -f qlx.json ]; then
		note "skipped: no qlx.yaml/qlx.yml/qlx.json at the repository root"
		note "'qlx regress' needs a config + schema.lock; see CONTRIBUTING.md"
		return 3
	fi
	if [ ! -f ledgers/regress.json ]; then
		note "skipped: ledgers/regress.json (the baseline ci.yml compares against) does not exist"
		return 3
	fi
	if [ ! -d "$src/src/test/regress/sql" ]; then
		log "regress: cloning PostgreSQL REL_18_STABLE (once, in $CACHE)"
		rm -rf "$src"
		git clone --depth 1 --branch REL_18_STABLE \
			https://github.com/postgres/postgres "$src"
	fi
	# ci.yml runs `go run -tags sql2go` without fetching pg_query_go, which fails
	# on a clean module. Fetch it here, exactly like scripts/sql2go.sh, and put
	# go.mod/go.sum back afterwards.
	local bak="$TMP/rootmod"
	mkdir -p "$bak"
	cp go.mod "$bak/go.mod"
	[ -f go.sum ] && cp go.sum "$bak/go.sum" || true
	EXIT_HOOKS+=("restore_file '$bak' go.mod; restore_file '$bak' go.sum")

	CGO_ENABLED=1 go get github.com/pganalyze/pg_query_go/v6 >/dev/null 2>&1 ||
		{ echo "  cannot fetch pg_query_go (network? cgo?)" >&2; return 1; }
	run_step regress "translate the regression SQL" \
		env CGO_ENABLED=1 go run -tags sql2go ./cmd/qlx regress \
		-dir "$src/src/test/regress/sql" \
		-ledger "$TMP/regress.json" -baseline ledgers/regress.json || rc=1
	restore_file "$bak" go.mod
	restore_file "$bak" go.sum
	return $rc
}

job_bench() {
	need docker bench
	local rc=0
	run_step bench "budgets" bash -c 'cd bench && go test -run TestBudgets -v' || rc=1
	run_step bench "benchmarks (count $BENCH_COUNT)" bash -c \
		"cd bench && go test -bench . -benchmem -count $BENCH_COUNT | tee '$CACHE/bench.txt'" || rc=1
	note "benchmarks written to $CACHE/bench.txt"
	return $rc
}

job_fuzz() {
	need docker fuzz
	local rc=0
	run_step fuzz "FuzzQuoteIdent ($FUZZ_TIME)" \
		go test -run '^$' -fuzz FuzzQuoteIdent -fuzztime "$FUZZ_TIME" . || rc=1
	run_step fuzz "FuzzRenderNeverPanics ($FUZZ_TIME)" \
		go test -run '^$' -fuzz FuzzRenderNeverPanics -fuzztime "$FUZZ_TIME" . || rc=1
	return $rc
}

job_postgis() {
	need docker postgis
	local rc=0 major image port
	for major in "${PG_MAJORS[@]}"; do
		case "$major" in
		19) image=postgres:19rc1 ;;
		*) image="postgres:$major-bookworm" ;;
		esac
		log "postgis: extract from PostgreSQL $major"
		start_pg "qlx-ci-postgis-$major" "$image"
		port=$CI_PG_PORT
		if ! docker exec -u root "qlx-ci-postgis-$major" sh -c \
			"apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq postgresql-$major-postgis-3" >/dev/null; then
			echo "  cannot install postgis for $major (no package for this image?)" >&2
			rc=1
			docker rm -f "qlx-ci-postgis-$major" >/dev/null 2>&1
			continue
		fi
		psql_in "qlx-ci-postgis-$major" postgres -c 'CREATE EXTENSION IF NOT EXISTS postgis'
		psql_in "qlx-ci-postgis-$major" postgres -Atc 'select postgis_lib_version()'

		export_pg "$port" "$major"
		export QLX_PG_DB=postgres
		export DATABASE_URL="postgres://postgres:$PGPASS@127.0.0.1:$port/postgres?sslmode=disable"
		run_step "postgis$major" "extract" \
			go run ./cmd/pgcat extract -ext postgis -o "$TMP/postgis-$major.json" || rc=1
		run_step "postgis$major" "prepare every generated wrapper" \
			go test -count=1 ./pgcat/ext/postgis || rc=1
		unset DATABASE_URL QLX_PG_HOST QLX_PG_USER QLX_PG_DB QLX_PG_VERSION QLX_TEST_DSN PGPORT
		docker rm -f "qlx-ci-postgis-$major" >/dev/null 2>&1
	done

	if [ $rc = 0 ]; then
		run_step postgis "pack status is consistent" go test ./internal/packs -count=1 || rc=1
		run_step postgis "merge the majors" \
			go run ./cmd/pgcat merge -o "$TMP/postgis.json" "$TMP"/postgis-*.json || rc=1
		run_step postgis "compare with the committed ledger" \
			go run ./cmd/pgcat diff ledgers/postgis.json "$TMP/postgis.json" || rc=1
	fi
	return $rc
}

# ------------------------------------------------------------------ driver
RESULTS=()
FAILED=0

# run_job <name> keeps going after a failure so the summary is complete.
run_job() {
	local name=$1 start=$SECONDS rc=0
	"job_$name" || rc=$?
	local dur=$(($SECONDS - start))
	if [ $rc = 0 ]; then
		RESULTS+=("ok|$name|${dur}s")
		printf '%s[ok]%s %s (%ss)\n' "$GREEN" "$OFF" "$name" "$dur"
	elif [ $rc = 3 ]; then
		RESULTS+=("skip|$name|${dur}s")
		printf '%s[skip]%s %s (%ss)\n' "$DIM" "$OFF" "$name" "$dur"
	else
		RESULTS+=("FAIL|$name|${dur}s")
		FAILED=1
		printf '%s[FAIL]%s %s (%ss)\n' "$RED" "$OFF" "$name" "$dur"
	fi
}

WORK=()
for j in "${JOBS[@]}"; do
	case "$j" in
	all) WORK=(test ledgers extensions bench) ;;
	full) WORK=(test ledgers extensions regress bench fuzz postgis) ;;
	test | ledgers | extensions | regress | bench | fuzz | postgis) WORK+=("$j") ;;
	*) die "unknown job '$j' (test ledgers extensions regress bench fuzz postgis all full)" ;;
	esac
done
# Deduplicate while keeping the order.
DEDUP=()
for j in "${WORK[@]}"; do
	skip=0
	for k in "${DEDUP[@]:-}"; do [ "$k" = "$j" ] && skip=1; done
	[ $skip = 0 ] && DEDUP+=("$j")
done
WORK=("${DEDUP[@]}")

go version
note "docker $(docker version --format '{{.Server.Version}}' 2>/dev/null || echo unavailable)"
note "jobs: ${WORK[*]}"
if [ "$RACE" = 1 ]; then note "race detector: on"; fi

START=$SECONDS
for j in "${WORK[@]}"; do run_job "$j"; done

log "summary"
for r in "${RESULTS[@]}"; do
	IFS='|' read -r st name dur <<<"$r"
	if [ "$st" = ok ]; then
		printf '  %s%-10s %-12s %s%s\n' "$GREEN" "ok" "$name" "$dur" "$OFF"
	elif [ "$st" = skip ]; then
		printf '  %s%-10s %-12s %s%s\n' "$DIM" "skip" "$name" "$dur" "$OFF"
	else
		printf '  %s%-10s %-12s %s%s\n' "$RED" "FAIL" "$name" "$dur" "$OFF"
	fi
done
note "total $(($SECONDS - START))s"
[ $FAILED = 0 ] || exit 1
