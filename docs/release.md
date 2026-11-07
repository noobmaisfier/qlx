# Releasing QLX

This is the maintainer procedure for a versioned release. It is written for the
repository as it is today: four publishable modules, a local Go workspace, and a
module proxy that makes a published version permanent.

Read the whole page before starting. The order of the steps matters, and two of
them are hard to undo once a tag is public.

## What a release is

One version number reaches four tags, all on the same commit:

| Tag | Module |
| --- | --- |
| `vX.Y.Z` | `github.com/noobmaisfier/qlx` (root) |
| `analyzer/vX.Y.Z` | `github.com/noobmaisfier/qlx/analyzer` |
| `pgxexec/vX.Y.Z` | `github.com/noobmaisfier/qlx/pgxexec` |
| `otelqlx/vX.Y.Z` | `github.com/noobmaisfier/qlx/otelqlx` |

`bench` is internal and is never tagged or published. A Go submodule is resolved
by `<module path>/vX.Y.Z`, so a bare `vX.Y.Z` does **not** publish `analyzer`,
`pgxexec` or `otelqlx`.

The four tags share one commit so that a user who requires `vX.Y.Z` and a user
who requires `pgxexec/vX.Y.Z` get the same tree. The GitHub release is attached
to the root tag only; the submodule tags need no release object.

## Preconditions

- `main` is green on the CI matrix (PostgreSQL 16–19, pgx min and latest) and
  the working tree is clean.
- Go 1.27 or later and `gh` are installed and authenticated.
- `proxy.golang.org` is reachable. Every check below assumes the real proxy is
  what users will see.
- One reachable PostgreSQL server per targeted major, through `QLX_PG_URLS`,
  if the catalog has to be re-extracted.
- No `go.work` effect leaks into the checks: run module commands with
  `GOWORK=off`, otherwise a local workspace hides a stale `go.mod` or a missing
  `go.sum` entry.

## 1. Prepare the tree

The generator step can change the tree, so it runs before the version commits and
before any tag.

```bash
QLX_PG_URLS="16=postgres://… 17=postgres://… 18=postgres://… 19=postgres://…" \
  scripts/release.sh
```

Without `QLX_PG_URLS` the script keeps the committed `pgcat.json`; that is only
valid when it already lists every targeted major. The script merges the ledgers,
regenerates `pgcat/`, `pg/` and `docs/pgcat`, runs `go mod tidy` in the modules
that have dependencies, checks the root module, and rejects `gofmt` differences.
Commit whatever it changes, in its own commit, before continuing.

Do not run `scripts/release.sh` again after tagging: `go mod tidy` can rewrite
`go.sum`, which changes the tree the tag points at.

## 2. Verify the tree you are about to tag

```bash
gofmt -l .
go vet ./...
go test ./...

for m in analyzer pgxexec otelqlx bench; do
  (cd "$m" && GOWORK=off go test ./...)
done

scripts/with_pg.sh go test ./...       # repeat with PGBIN set to each major
```

`GOWORK=off` is not optional here: with a local workspace the modules resolve
the root through `use`, not through the version in `go.mod`, so a broken bump
would pass. CI runs the same matrix on the push; the local run is what avoids
spending a published version on a fixable mistake.

## 3. Bump the version

Two commits, in this order:

1. **`Bump current release to vX.Y.Z`** — `README.md` only: the
   `Current release:` line near the top and the list of published module tags
   further down. Keep the two in step.
2. **`Bump internal module references to vX.Y.Z`** — the `require` lines in
   `pgxexec/go.mod`, `otelqlx/go.mod` and `bench/go.mod`, plus the new `go.sum`
   entries described next.

The root module has no `go.sum` and no dependencies; `scripts/check_root.sh`
enforces that.

### The `go.sum` chicken-and-egg

The new `go.sum` lines are hashes of the tagged tree, and the tag has to exist
before they can be computed:

```bash
git tag -a vX.Y.Z            -m "qlx vX.Y.Z"
git tag -a analyzer/vX.Y.Z   -m "analyzer vX.Y.Z"
git tag -a pgxexec/vX.Y.Z    -m "pgxexec vX.Y.Z"
git tag -a otelqlx/vX.Y.Z    -m "otelqlx vX.Y.Z"
```

Resolve the local tag instead of the network, then record the hashes:

```bash
# ~/.gitconfig.release
# [url "file:///absolute/path/to/qlx"]
#     insteadOf = https://github.com/noobmaisfier/qlx
# [protocol "file"]
#     allow = always

for m in pgxexec otelqlx bench; do
  (cd "$m" && GOWORK=off GOPROXY=direct GOSUMDB=off \
    GOPRIVATE='github.com/noobmaisfier/*' \
    GIT_CONFIG_GLOBAL=~/.gitconfig.release \
    go mod download github.com/noobmaisfier/qlx)
done
```

That writes the `h1:` and `/go.mod h1:` pairs into each module's `go.sum`. Add
them to the *module-reference* commit, amend it, then re-create the tags so they
point at the amended commit:

```bash
git add pgxexec/go.sum otelqlx/go.sum bench/go.sum
git commit --amend --no-edit
git tag -f -a vX.Y.Z          -m "qlx vX.Y.Z"
git tag -f -a analyzer/vX.Y.Z -m "analyzer vX.Y.Z"
git tag -f -a pgxexec/vX.Y.Z  -m "pgxexec vX.Y.Z"
git tag -f -a otelqlx/vX.Y.Z  -m "otelqlx vX.Y.Z"
```

Why this is stable: the Go module zip for the root module excludes directories
that contain their own `go.mod`. Editing `pgxexec/go.sum` does not change the
root hash. Editing any file outside those submodules does. If that happens
after the tag, start this step again with re-created tags — and if the tags are
already pushed, you cannot: ship a new patch version instead.

Amended commits and moved tags are acceptable **only before the first push**.
Never `git tag -f` a tag that has been pushed, and never force-push a tag.

## 4. Push tags first, then `main`

```bash
git push origin vX.Y.Z analyzer/vX.Y.Z pgxexec/vX.Y.Z otelqlx/vX.Y.Z
git push origin main
```

Tags first: CI on `main` resolves the bumped modules through the proxy, and the
proxy needs the tags to exist. `main` first would start a red run for a tree
that is fine.

Both pushes are the point of no return. Once `proxy.golang.org` has fetched a
version, that version is immutable: the proxy caches the zip and its hashes
forever. A wrong `vX.Y.Z` cannot be replaced; the only remedy is a new patch
release, and `retract` in `go.mod` for users who already picked it up.

## 5. Create the GitHub release

Only the root tag gets a release object:

```bash
gh release create vX.Y.Z --title vX.Y.Z --notes-file /tmp/release-notes.md --latest
```

Write the notes for users, not for maintainers: what changed, in behaviour, and
the module tags at the bottom. It is the only user-facing summary of the
version.

## 6. Verify what users will see

```bash
gh run list --branch main --limit 3
gh run watch <run-id> --exit-status

# fetch the published version through the real proxy, from a scratch module
mkdir -p /tmp/qlx-verify && cd /tmp/qlx-verify && go mod init verify
GOWORK=off GOPROXY=https://proxy.golang.org \
  go mod download -json github.com/noobmaisfier/qlx@vX.Y.Z

# prove the module is installable the way a user installs it
GOBIN=/tmp/qlxbin GOWORK=off GOPROXY=https://proxy.golang.org \
  go install github.com/noobmaisfier/qlx/analyzer/cmd/qlxvet@vX.Y.Z
```

Compare the `Sum` from `go mod download` with the `h1:` line committed in
`go.sum`; they must be identical. A mismatch after the push means the tag and
the committed hash describe different trees: do not reuse the version.

## Checklist

- [ ] `main` green, tree clean, `gh` authenticated.
- [ ] `scripts/release.sh` run; generated changes committed.
- [ ] Tests pass in every module with `GOWORK=off`, on each major.
- [ ] `README.md` says `vX.Y.Z` in both places.
- [ ] Module `require` lines say `vX.Y.Z`; `go.sum` has the matching hashes.
- [ ] Four annotated tags on the same commit.
- [ ] Tags pushed, then `main`.
- [ ] GitHub release created from the root tag.
- [ ] CI green on `main`.
- [ ] `go install …@vX.Y.Z` succeeds through the proxy and the hash matches.

## Known rough edges

- Steps 3 and 4 are manual. Nothing in the repository refuses to tag a dirty
  tree, a stale `README.md`, or a tag that already exists.
- The hash calculation needs a temporary `insteadOf` Git config. It is only
  needed because the tag is not on the network yet; the alternative is pushing
  the tag first and risking a bad published version.
- The release notes are hand-written. Nothing compares them with the commits
  since the previous tag.
