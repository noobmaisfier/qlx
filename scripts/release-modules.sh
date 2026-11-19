#!/usr/bin/env bash
# Second half of a release, run after the root module is tagged and pushed:
#
#   scripts/release-modules.sh v0.1.0
#
# pgxexec, otelqlx, analyzer and qjson/validator are separate Go modules. In the repository they
# build against the local qlx through go.work; a user gets no go.work, only the
# go.mod. This script points each go.mod at the published root tag, tidies it
# and builds it with GOWORK=off, which is exactly what `go get` will see.
# It changes files and prints the commands for you to run; it does not commit,
# tag or push.
#
# Order of a release:
#   1. scripts/release.sh, tests, commit, push
#   2. git tag vX.Y.Z && git push origin vX.Y.Z               (the root module)
#   3. scripts/release-modules.sh vX.Y.Z                      (this script)
#   4. commit the four go.mod/go.sum files, push, then tag each sub-module with its
#      path prefix: pgxexec/vX.Y.Z, otelqlx/vX.Y.Z, analyzer/vX.Y.Z, qjson/validator/vX.Y.Z
set -euo pipefail
cd "$(dirname "$0")/.."

v="${1:-}"
[[ "$v" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { echo "usage: $0 vMAJOR.MINOR.PATCH" >&2; exit 2; }
git rev-parse -q --verify "refs/tags/$v" >/dev/null || { echo "tag $v does not exist locally: tag the root module first" >&2; exit 1; }
if ! git ls-remote --exit-code --tags origin "refs/tags/$v" >/dev/null 2>&1; then
  echo "tag $v is not on origin: push it first (git push origin $v)" >&2; exit 1
fi

# The proxy can lag behind a fresh tag; going direct avoids a stale "unknown revision".
export GOPROXY="${GOPROXY:-direct}" GOWORK=off GOFLAGS=-mod=mod

for m in pgxexec otelqlx analyzer qjson/validator; do
  echo "== $m"
  (
    cd "$m"
    if grep -q '^replace github.com/noobmaisfier/qlx ' go.mod; then
      echo "$m/go.mod still has a replace for qlx: remove it" >&2; exit 1
    fi
    # analyzer does not import qlx. Every other sub-module must name the
    # release explicitly: qjson/validator is new, so its go.mod may not have a
    # require yet, and `go mod tidy` alone would pick the latest final release
    # (which can predate the package it imports).
    if [ "$m" != analyzer ]; then
      go mod edit -require="github.com/noobmaisfier/qlx@$v"
    fi
    go mod tidy
    GOFLAGS= go build ./...
    GOFLAGS= go vet ./...
  )
done

echo
echo "all sub-modules resolve against $v without go.work. Next:"
echo "  git add pgxexec otelqlx analyzer qjson/validator && git commit -m 'modules: require qlx $v' && git push"
echo "  for m in pgxexec otelqlx analyzer qjson/validator; do git tag \$m/$v; done"
echo "  git push origin pgxexec/$v otelqlx/$v analyzer/$v qjson/validator/$v"
echo "  then bring the repository back to the development state if you bumped the require"
echo "  (go.work keeps working: it overrides whatever version the go.mod names)."
