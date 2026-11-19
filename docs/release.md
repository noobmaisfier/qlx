# Releasing QLX

This is the maintainer procedure for a versioned release. It is written for the
repository's multi-module architecture: five publishable modules (`qlx`, `pgxexec`,
`otelqlx`, `analyzer`, `qjson/validator`), an internal test/benchmark module (`bench`), and a local Go
workspace (`go.work`).

Read the whole page before starting. The order of steps matters: because sub-modules
depend on a published `qlx` version without using `replace` directives, the root module
must be tagged and pushed before sub-modules can reference it.

---

## Architecture of a release

The repository publishes five Go modules under coordinated version tags:

| Tag format | Module path | Content |
| --- | --- | --- |
| `vX.Y.Z` | `github.com/noobmaisfier/qlx` | Root query builder, core packages and `qjson` |
| `pgxexec/vX.Y.Z` | `github.com/noobmaisfier/qlx/pgxexec` | pgx v5 execution engine |
| `otelqlx/vX.Y.Z` | `github.com/noobmaisfier/qlx/otelqlx` | OpenTelemetry instrumentation |
| `analyzer/vX.Y.Z` | `github.com/noobmaisfier/qlx/analyzer` | `qlxvet` static analysis tool |
| `qjson/validator/vX.Y.Z` | `github.com/noobmaisfier/qlx/qjson/validator` | `go-playground/validator` adapter for DTOs |

`bench` is internal and is never published or tagged.

### How `go.work` and publishable `go.mod` interact

During day-to-day development, `go.work` at the root lists all modules. This allows
`pgxexec`, `otelqlx` and `qjson/validator` to resolve `github.com/noobmaisfier/qlx` locally
from your checkout without needing `replace` directives in their `go.mod` files.
`analyzer` does not import the root module, so it needs no `qlx` require at all.

When users run `go get github.com/noobmaisfier/qlx/pgxexec`, Go does **not** read
`go.work`. It reads only `pgxexec/go.mod`. Therefore, each sub-module's `go.mod` must
require a real, published tag of `qlx`.

To make this work cleanly without circular dependency workarounds:
1. **Release the root module first** (`vX.Y.Z`).
2. **Point sub-modules at that published tag** via `scripts/release-modules.sh`, then tag them (`pgxexec/vX.Y.Z`, etc.).

---

## Preconditions

- `main` is green on the CI matrix (PostgreSQL 16–19, pgx min and latest).
- Working tree is clean (`git status` shows nothing unstaged or untracked).
- Go 1.27.4 or later and GitHub CLI (`gh`) are installed and authenticated.
- One reachable PostgreSQL server per targeted major (16–19) if the catalog ledger needs refreshing.

---

## Phase 1: Prepare and release the root module (`vX.Y.Z`)

### 1.1 Run code generation and ledger update

If database catalogs need refreshing across majors:

```bash
QLX_PG_URLS="16=postgres://… 17=postgres://… 18=postgres://… 19=postgres://…" \
  scripts/release.sh
```

Without `QLX_PG_URLS`, the script uses the committed `pgcat.json` ledger as-is.
The script regenerates `pgcat/`, docs, checks `root` module isolation, and verifies formatting.

### 1.2 Verify the test suite

Run tests with the workspace active:

```bash
gofmt -l .
go vet ./...
go test ./...
(cd pgxexec  && go test ./...)
(cd otelqlx  && go test ./...)
(cd analyzer && go test ./...)
(cd qjson/validator && go vet ./... && go test ./...)
(cd bench    && go test -run TestBudgets)

scripts/with_pg.sh go test ./...
```

### 1.3 Bump the release version in documentation

Update `README.md` to reference `vX.Y.Z` in:
- The "Current release:" status callout.
- The "Modules" table.
- The "Verification" section ("including the vX.Y.Z release").

Commit the changes:

```bash
git add README.md (plus any generated files from step 1.1)
git commit -m "Bump current release to vX.Y.Z"
git push origin main
```

### 1.4 Tag and push the root module

Tag the root module with `vX.Y.Z` and push it to GitHub:

```bash
git tag -a vX.Y.Z -m "qlx vX.Y.Z"
git push origin vX.Y.Z
```

At this point, the root module version `vX.Y.Z` is officially published on GitHub and visible to Go module proxies.

---

## Phase 2: Update and release sub-modules

### 2.1 Run `scripts/release-modules.sh`

Now that `vX.Y.Z` exists on `origin`, run the automated sub-module release script:

```bash
scripts/release-modules.sh vX.Y.Z
```

This script:
1. Validates that `vX.Y.Z` exists locally and on `origin`.
2. Sets `GOWORK=off` and `GOPROXY=direct` (bypassing proxy caching delays).
3. Updates `pgxexec/go.mod`, `otelqlx/go.mod` and `qjson/validator/go.mod` to require `github.com/noobmaisfier/qlx@vX.Y.Z`; `analyzer/go.mod` is left alone because it does not import the root module.
4. Runs `go mod tidy` in each sub-module, downloading the newly published `qlx@vX.Y.Z` and recording real hashes in `go.sum`.
5. Compiles and vets each sub-module standalone (`GOWORK=off`), verifying that what users download builds cleanly.

The explicit `require` matters for `qjson/validator`: its `go.mod` is created
without a `qlx` requirement, and a bare `go mod tidy` would resolve the latest
final release, which may predate the `qjson` package it imports. The script pins
the new tag explicitly.

### 2.2 Commit sub-module dependency bumps

Commit the updated `go.mod` and `go.sum` files for the sub-modules to `main`:

```bash
git add pgxexec otelqlx analyzer qjson/validator
git commit -m "Bump internal module references to vX.Y.Z"
git push origin main
```

### 2.3 Tag and push sub-modules

Tag each sub-module using its path prefix, and push the tags:

```bash
for m in pgxexec otelqlx analyzer qjson/validator; do
  git tag -a "$m/vX.Y.Z" -m "$m vX.Y.Z"
done

git push origin pgxexec/vX.Y.Z otelqlx/vX.Y.Z analyzer/vX.Y.Z qjson/validator/vX.Y.Z
```

### 2.4 Point `bench` at the release

`bench` is internal and never tagged, but its `go.mod` names a root version and
its `go.sum` needs the real hash. Once the root tag exists:

```bash
(cd bench && GOWORK=off go mod edit -require=github.com/noobmaisfier/qlx@vX.Y.Z && GOWORK=off go mod tidy)
git add bench
git commit -m "Point bench at qlx vX.Y.Z"
git push origin main
```

This is a consistency commit, not part of the published modules; it can land
before or after the sub-module tags.

---

## Phase 3: GitHub Release and verification

### 3.1 Create the GitHub release

Only the root tag gets a GitHub release object:

```bash
gh release create vX.Y.Z --title "vX.Y.Z" --notes-file /tmp/release-notes.md --latest
```

Write notes summarizing user-facing changes and bug fixes, and list the five published tags.

### 3.2 Watch CI workflows

1. The push to `main` triggers standard CI (`ci.yml`).
2. Pushing the four sub-module tags triggers the `modules.yml` workflow, which builds and vets the published sub-modules (`pgxexec`, `otelqlx`, `analyzer`, `qjson/validator`) with `GOWORK=off` against the published versions. The root `vX.Y.Z` tag does not trigger it: at that commit the sub-module `go.mod` files still name the previous release by design, and a brand-new sub-module may not have a `qlx` require yet.

Verify in GitHub Actions that both workflows pass:

```bash
gh run list --limit 5
```

### 3.3 Verify external installation

Verify that an external consumer can install and download the release without `go.work`:

```bash
mkdir -p /tmp/qlx-verify && cd /tmp/qlx-verify && go mod init verify

# Test fetching root and sub-modules through the proxy
GOWORK=off GOPROXY=https://proxy.golang.org \
  go mod download -json github.com/noobmaisfier/qlx@vX.Y.Z

for m in pgxexec otelqlx analyzer qjson/validator; do
  GOWORK=off GOPROXY=https://proxy.golang.org \
    go mod download -json "github.com/noobmaisfier/qlx/$m@vX.Y.Z"
done

# Test binary installation
GOBIN=/tmp/qlxbin GOWORK=off GOPROXY=https://proxy.golang.org \
  go install github.com/noobmaisfier/qlx/analyzer/cmd/qlxvet@vX.Y.Z
```

For a module with no package of its own to download, `go mod download` still
checks that the tag exists and that the module zip is fetchable; the build/vet
check in `modules.yml` proves it compiles against the published root.

---

### 3.4 Release candidates

A candidate uses the same phases with a suffix, for example `v1.0.0-rc1`:

- `scripts/release-modules.sh v1.0.0-rc1` accepts the suffix and sets the same
  version in `pgxexec`, `otelqlx` and `qjson/validator`.
- Create the GitHub release with `--prerelease --latest=false`:
  `gh release create v1.0.0-rc1 --title "v1.0.0-rc1" --notes-file /tmp/release-notes.md --prerelease --latest=false`.
- `@latest` keeps resolving to the previous final release until the final tag
  is published, so consumers must request the candidate explicitly.

## Release checklist

- [ ] `main` is green, working tree clean.
- [ ] `scripts/release.sh` run (catalog & docs updated).
- [ ] Full test suite passes across targeted PostgreSQL majors.
- [ ] `README.md` bumped to `vX.Y.Z` and committed.
- [ ] Root tag pushed: `git tag vX.Y.Z && git push origin vX.Y.Z`.
- [ ] `scripts/release-modules.sh vX.Y.Z` executed cleanly (including `qjson/validator` built with `GOWORK=off`).
- [ ] Sub-module `go.mod` / `go.sum` committed and pushed to `main`.
- [ ] `bench` pointed at `vX.Y.Z` (internal, not tagged).
- [ ] Sub-module tags pushed (`pgxexec/vX.Y.Z`, `otelqlx/vX.Y.Z`, `analyzer/vX.Y.Z`, `qjson/validator/vX.Y.Z`).
- [ ] GitHub release created for `vX.Y.Z`.
- [ ] CI `modules.yml` green on the four sub-module tag pushes (the root tag does not trigger it).
- [ ] Standalone test with `go install` succeeds through the Go proxy.
