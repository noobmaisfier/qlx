# Releasing QLX

This is the maintainer procedure for a versioned release. It is written for the
repository's multi-module architecture: four publishable modules (`qlx`, `pgxexec`,
`otelqlx`, `analyzer`), an internal test/benchmark module (`bench`), and a local Go
workspace (`go.work`).

Read the whole page before starting. The order of steps matters: because sub-modules
depend on a published `qlx` version without using `replace` directives, the root module
must be tagged and pushed before sub-modules can reference it.

---

## Architecture of a release

The repository publishes four Go modules under coordinated version tags:

| Tag format | Module path | Content |
| --- | --- | --- |
| `vX.Y.Z` | `github.com/noobmaisfier/qlx` | Root query builder and core packages |
| `pgxexec/vX.Y.Z` | `github.com/noobmaisfier/qlx/pgxexec` | pgx v5 execution engine |
| `otelqlx/vX.Y.Z` | `github.com/noobmaisfier/qlx/otelqlx` | OpenTelemetry instrumentation |
| `analyzer/vX.Y.Z` | `github.com/noobmaisfier/qlx/analyzer` | `qlxvet` static analysis tool |

`bench` is internal and is never published or tagged.

### How `go.work` and publishable `go.mod` interact

During day-to-day development, `go.work` at the root lists all modules. This allows
`pgxexec`, `otelqlx` and `analyzer` to resolve `github.com/noobmaisfier/qlx` locally
from your checkout without needing `replace` directives in their `go.mod` files.

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
3. Updates `pgxexec/go.mod` and `otelqlx/go.mod` to require `github.com/noobmaisfier/qlx@vX.Y.Z`.
4. Runs `go mod tidy` in each sub-module, downloading the newly published `qlx@vX.Y.Z` and recording real hashes in `go.sum`.
5. Compiles and vets each sub-module standalone (`GOWORK=off`), verifying that what users download builds cleanly.

### 2.2 Commit sub-module dependency bumps

Commit the updated `go.mod` and `go.sum` files for the sub-modules to `main`:

```bash
git add pgxexec otelqlx analyzer
git commit -m "Bump internal module references to vX.Y.Z"
git push origin main
```

### 2.3 Tag and push sub-modules

Tag each sub-module using its path prefix, and push the tags:

```bash
for m in pgxexec otelqlx analyzer; do
  git tag -a "$m/vX.Y.Z" -m "$m vX.Y.Z"
done

git push origin pgxexec/vX.Y.Z otelqlx/vX.Y.Z analyzer/vX.Y.Z
```

---

## Phase 3: GitHub Release and verification

### 3.1 Create the GitHub release

Only the root tag gets a GitHub release object:

```bash
gh release create vX.Y.Z --title "vX.Y.Z" --notes-file /tmp/release-notes.md --latest
```

Write notes summarizing user-facing changes and bug fixes, and list the four published tags.

### 3.2 Watch CI workflows

1. The push to `main` triggers standard CI (`ci.yml`).
2. Pushing the tags triggers the `modules.yml` workflow, which builds and vets the published sub-modules (`pgxexec`, `otelqlx`, `analyzer`) with `GOWORK=off` against the published versions.

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

GOWORK=off GOPROXY=https://proxy.golang.org \
  go mod download -json github.com/noobmaisfier/qlx/pgxexec@vX.Y.Z

# Test binary installation
GOBIN=/tmp/qlxbin GOWORK=off GOPROXY=https://proxy.golang.org \
  go install github.com/noobmaisfier/qlx/analyzer/cmd/qlxvet@vX.Y.Z
```

---

## Release checklist

- [ ] `main` is green, working tree clean.
- [ ] `scripts/release.sh` run (catalog & docs updated).
- [ ] Full test suite passes across targeted PostgreSQL majors.
- [ ] `README.md` bumped to `vX.Y.Z` and committed.
- [ ] Root tag pushed: `git tag vX.Y.Z && git push origin vX.Y.Z`.
- [ ] `scripts/release-modules.sh vX.Y.Z` executed cleanly.
- [ ] Sub-module `go.mod` / `go.sum` committed and pushed to `main`.
- [ ] Sub-module tags pushed (`pgxexec/vX.Y.Z`, `otelqlx/vX.Y.Z`, `analyzer/vX.Y.Z`).
- [ ] GitHub release created for `vX.Y.Z`.
- [ ] CI `modules.yml` green on tag push.
- [ ] Standalone test with `go install` succeeds through the Go proxy.
