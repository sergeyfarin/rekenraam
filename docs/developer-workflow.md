# Developer Workflow

This document defines default development workflow conventions for contributors and coding agents.

## Source Of Truth

Before changing durable behavior, read:

- `docs/product-requirements.md`
- `docs/conventions.md`
- `docs/early-architecture-decisions.md`
- `docs/adrs/`
- `docs/localization-glossary.md` — before adding or translating a domain term.

## Daily Commands

Run from repo root unless noted otherwise.

### Deployment audit

`bash scripts/test-docker-context.sh` uses Docker COPY with synthetic files to
check that `.dockerignore` retains required source and excludes private/runtime
data and generated output. It needs Docker, but reads no real private files.

For a provider-free production Compose install/upgrade/rollback drill, build
the two candidate images first, then run (substitute their tags and highest
migrations):

```sh
python3 scripts/audit-deployment.py \
  --old-image rekenraam-audit279-old:00fdbd8f \
  --new-image rekenraam-audit279-rekenraam:latest \
  --old-schema 14 --new-schema 18
```

This requires Python 3, Docker Compose with `!override` support, and a free
loopback port (16889 by default; change it with `--port`). It uses a unique
Compose project, synthetic credentials and a disposable volume, removed even
on failure. It retains evidence in a private temporary directory printed at
startup. It never uses a real deployment's data. The independent oracle is a
USD transfer of 25.00: exactly two CSV rows (-25.00/+25.00), with every prior
column preserved across upgrade, restart and restored rollback. Head's added
posting-version identity column is checked separately. Both versions must pass
self-check; a newer backup must be refused by the old binary. This is a
candidate drill, not an installed-release compatibility claim. The
[2026-10-18 report](reviews/deployment-audit-2026-10-18.md) records image builds,
scan results and limits.

The [2026-10-20 runtime-image review](reviews/runtime-image-advisory-audit-2026-10-20.md)
records a fresh image/DB scan, actual ELF/process/health-check boundaries,
the complete package/version/advisory matrix, a repeated Compose drill and
the owner-disposition blocker on #337. Its reproduction section separates the
networked database download from the network-disabled exported-tar scan.
The [bundled native SQLite review (#354)](reviews/native-sqlite-advisory-audit-2026-10-20.md)
authenticates source/compile options and exercises real search/JSON/verify/restore
consumers. Its reproduction section includes the named native audit tests and
the independent upstream-bug probe retained for driver-admission follow-up #356.
The [#356 prerequisite recheck](reviews/sqlite-driver-upgrade-blocker-2026-10-20.md)
adds an isolated C admission probe: it fails on official 3.53.4 and passes on
official 3.54.1. A released Go driver carrying those fixes is still required;
the production module pin remains unchanged.

### Install

```sh
pnpm install
```

### Development

```sh
pnpm dev
pnpm dev:backend
pnpm dev:frontend
```

The frontend development server binds to `127.0.0.1:1888`. It does not expose a
plain-HTTP LAN URL because secure authentication cookies only work over HTTPS or
on a browser's trusted localhost origin. For a headless development machine,
run this on the machine with the browser and keep the connection open:

```sh
ssh -N -L 1888:127.0.0.1:1888 user@development-host
```

Then open `http://localhost:1888`. Vite proxies `/api` to the backend on the
development host, so the frontend port is the only required forward. Use an
HTTPS reverse proxy for direct access from phones or other LAN clients.

### Public website

The separate `website/` project renders static HTML for the app's six locales.
It needs Node 24 and no npm dependencies or Go server. The website source and
the self-hosted application have independent builds; `pnpm build` still builds
only the app. See the [website contract](design/public-website.md) for content,
localization, privacy and publication boundaries.

```sh
pnpm website:dev              # build + loopback preview, watch catalogs/assets
pnpm website:check            # catalog parity, prerendered content, links, paths
pnpm website:build            # website/build/, default origin rekenraam.com
pnpm website:preview          # existing build at http://127.0.0.1:1889
pnpm website:check-browser    # needs E2E dependencies and Playwright Chromium
```

The dev command watches catalogs and assets; restart it after editing the
build template. `WEBSITE_PORT` changes the preview port. Stop the preview before
browser checks, which own port 1889. Website checks/build run in the existing
Frontend Check CI job. The browser command checks all locales at 320, 390, 768
and 1280 px, light/dark accessibility, theme persistence, image loading, no
external asset requests and language navigation without JavaScript. Screenshot
checks cover the manual carousel's selection, wraparound, keyboard focus,
announcements and three-image fallback without scripts.

The site uses `/` for English and `/nl/`, `/de/`, `/fr/`, `/es/`, `/ru/` for
the other languages. Catalogs and all user-facing metadata live under
`website/locales/`. Their language set is checked against
`frontend/project.inlang/settings.json`; native-language review remains #102.
All images/CSS/scripts are local. There are no analytics or external fonts.

The README's spending screenshot is reused during the build. Refresh the
other screenshots from the synthetic reference book with:

```sh
pnpm build
pnpm --dir e2e exec playwright test --config=website.config.ts
```

This generator creates and removes a disposable database, refuses
`E2E_BASE_URL`, redirects optional provider traffic to the test-only stub
address, and captures the transaction register and investment lot details.
It does not start a provider or read an existing installation. Images and
capture provenance belong in `website/assets/`; only presentation assets go
into the public build. Screenshots remain English with localized captions and
alternative text.

For a temporary project-path build instead of a custom domain:

```sh
SITE_ORIGIN=https://sergeyfarin.github.io SITE_BASE_PATH=/rekenraam pnpm website:build
SITE_BASE_PATH=/rekenraam pnpm website:preview
```

For `rekenraam.com`, leave both variables unset. The build includes canonical
and alternate-language links, a sitemap, robots.txt, a 404 page and `.nojekyll`.

#### GitHub Pages publication

Preparing the source and building an artifact do not publish the site. Keep
the existing roadmap's release/announcement gates and #335's separate public
hosting authorization. The manual **Website** workflow defaults to
`publish=false`, producing a review artifact without deployment. No push or
ordinary CI run deploys the website.

When public publication is explicitly authorized:

1. Verify ownership of `rekenraam.com` in GitHub and set the repository's Pages
   source to **GitHub Actions**.
2. Set the Pages custom domain to `rekenraam.com` before pointing DNS at GitHub.
3. Configure the apex domain using GitHub's documented DNS records, preserving
   the existing `retirement.rekenraam.com` record and its independent hosting.
   Optionally configure `www` as documented by GitHub. Enable **Enforce HTTPS**
   after certificate provisioning.
4. Dispatch **Website** on the reviewed revision with `publish=true`. Its Pages
   deployment uses the `github-pages` environment and the built static artifact.

Use the current [GitHub custom-domain guide](https://docs.github.com/en/pages/configuring-a-custom-domain-for-your-github-pages-site/managing-a-custom-domain-for-your-github-pages-site)
for DNS values. The workflow does not change DNS, create a release or deploy
the finance app. GitHub Actions custom-domain configuration lives in Pages
settings; the build does not supply a `CNAME` file.

### Backend Validation

The bounded runtime OpenAPI audit (#349) reuses named synthetic HTTP consumers:

```sh
python3 scripts/audit-api-contract.py --output /tmp/rekenraam-api-audit
python3 scripts/audit-api-contract.py --output /tmp/rekenraam-api-controls --controls-only
```

It requires Python 3.10+, PyYAML and jsonschema (recorded versions: 6.0.1 and
4.10.3). Use an isolated environment if these are not installed:
`python3 -m venv /tmp/rekenraam-api-venv`, then
`/tmp/rekenraam-api-venv/bin/pip install -r scripts/requirements-api-audit.txt`.
Run the script with that environment's Python. CSV/JSON output lists all declared
operations, sampled statuses, required root fields, actual response shapes,
schema failures, exact consumer names and retained limits. Raw synthetic bodies
are deleted with the private temporary directory. `--strict` fails for any
discrepancy, including retained findings if later added.
The #350/#370 exceptions are removed after their repairs. The #350 extension
exercises every mapped capped operation's 413 branch with known and unknown
content lengths and checks that refusal performs no SQLite writes. The optional
`REKENRAAM_BODY_INVENTORY=/tmp/body-operations.csv` environment variable on
`TestCappedJSONOperationsDeclare413` exports the source-to-contract inventory.
The default fails for new discrepancies and
failed sensitivity controls. This is a deliberate audit, not a new CI gate or
complete endpoint certification. Format checks depend on installed optional
format validators; `summary.json` records the asserted formats. In particular,
the recorded system environment has no RFC 3339 `date-time` checker.

The minimum Go toolchain is 1.27.2, declared in `backend/go.mod`. This patch
fixes the standard-library vulnerabilities reported by the 2026-10-09
`govulncheck` run. GitHub Actions reads that same minimum with
`go-version-file`; local Go commands automatically select it when needed.
The Docker builder pins `golang:1.27.2-trixie`; the runtime remains Debian
trixie. Native SQLite requires CGO and a C compiler with development headers
(Debian/Ubuntu: `sudo apt install build-essential`; macOS: Xcode command-line
tools). Windows needs a target C toolchain; no Windows release artifact is
claimed by this Linux/amd64 validation.

Supported commands source `scripts/native-sqlite-env.sh`, which checks CGO
and the compiler and enables `fts5,netgo,osusergo`. FTS5 is required by the
schema; the other tags preserve the previous pure-Go network/user lookup
behavior. Do not use the `libsqlite3` tag. `CGO_ENABLED=0` fails early with
setup guidance. For focused Go commands from repo root, use
`./scripts/go-backend.sh test ./internal/db -run TestNative -count=1` rather
than an untagged raw command. Advanced commands may source the same helper
after entering `backend/`; its required tags supersede a caller
`GOFLAGS=-tags=...`.

The Unix subprocess guard regressions (#105) run in the ordinary backend gate.
For a focused repeat:

```sh
./scripts/go-backend.sh test -race ./cmd/rekenraam -run '^TestTwoProcess' -count=3 -timeout=3m
```

They re-execute the same test binary with independent SQLite pools and pipe
barriers, retaining the selected native driver and Go race instrumentation.
Local TCP sockets are required for the real server/restore cases. #227 adds
paused-restore lifetime checks and repeated lock-file handoffs; their additional
child processes have a three-minute bound for this focused three-repeat command.
The full backend gate retains its five-minute per-package deadline. These tests
do not enable multiple servers on one database; see the
[#105 review](reviews/two-process-sqlite-guards-2026-10-14.md) and
[#227 review](reviews/restore-process-lock-2026-10-14.md).

Checks formatting (`gofmt -l`), runs `go vet ./...`, then the full Go suite with
the race detector. The formatting and vet gates live inside the wrapper script
so CI enforces them too — before they were added, `gofmt` drift could sit in the
tree with a green pipeline (backlog T-49).

The script resolves gofmt as `"$(go env GOROOT)/bin/gofmt"` rather than trusting
PATH. Run it that way by hand too: a version manager pinning an older Go leaves
its gofmt first on PATH even while the `go` command switches to the newer
toolchain `go.mod` requires, and gofmt's output is not stable across releases,
so a bare `gofmt` both misses drift CI will reject and flags files that are
correctly formatted for the toolchain actually compiling them.

```sh
./scripts/test-backend.sh
```

The race gate runs `go test -race -p 2 -timeout=5m ./...`: two packages run
concurrently, with an explicit five-minute deadline per package. The non-race
coverage pass uses the same package concurrency and deadline. Local validation
and CI use the same wrapper. Neither command sets a fixed GOMAXPROCS,
test `-parallel` limit or CPU affinity; use available Go CPU resources.
CI's integrated build runs independently of other checks; all jobs still
must pass. [ADR 0016](adrs/0016-native-backend-test-resources.md) and the
[native gate review](reviews/native-test-gate-2026-10-12.md) govern these limits.

[T-111 #126](https://github.com/sergeyfarin/rekenraam/issues/126) reviewed the
complete suite after the 2026-09-30 gate took 598.347 seconds against the old
600-second limit. The race profile and duplication review are recorded in
`docs/reviews/test-runtime-review-2026-09-30.md`. All financial, migration,
restore and race coverage is retained. Independent financial matrix cases
run in parallel with isolated fixtures; the owner authorized the explicit
15-minute bound to unblock further P0 work. Continue reviewing measured fixture
cost rather than treating that bound as a runtime target. The full revised
gate passed with the application package at 513.745 seconds, down from
598.347 seconds; the focused race run retained all 60 named matrix cases.
On 2026-10-11 the owner raised the bound from 15 to 20 minutes after the
application package reached 856 seconds of 900 on this workspace
([#164](https://github.com/sergeyfarin/rekenraam/issues/164)). #164 still
owns restoring headroom by removing cost, not by raising the bound again.

**Native SQLite cutover (2026-10-12):**
[ADR 0014](adrs/0014-native-sqlite-driver.md) and
[#195](https://github.com/sergeyfarin/rekenraam/issues/195) install
`github.com/mattn/go-sqlite3` v1.14.52 with bundled SQLite 3.53.4. Runtime,
tests, migrations, seed tooling and builds use the same driver/features.
The owner confirmed that no legacy databases/backups exist; current schema,
seed/checksum and backup/restore contracts remain unchanged. The
[delivery review](reviews/native-sqlite-driver-2026-10-12.md) records
compatibility, performance and build evidence. #164's measured headroom
acceptance is met: matched full race wall time improves from 27.67 to 3.76
minutes, with the same named test set and timeout under the historical
WAL/NORMAL writer policy. No race-instrumentation
exclusions are applied:
`-race` covers Go application and driver-wrapper code, while the native
SQLite C engine is outside Go's race instrumentation. A deliberate Go race
must still fail the gate.

**Commit durability follow-up (2026-10-12):**
[ADR 0015](adrs/0015-sqlite-commit-durability.md) requires WAL/FULL for the
writer and FULL consistently across migrations, tests and auxiliary opens.
The native driver's hook enforces FULL on direct and replacement connections;
startup checks its effective value. No schema/seed migration is needed.
The [FULL review](reviews/sqlite-full-durability-2026-10-12.md) records current
performance and the connection/build settings audit. The read pool's maximum
four connections is independent of CPU count. The shared gate does not cap
GOMAXPROCS or test parallelism at four; four-CPU affinity is a controlled
measurement condition. The owner's subsequent clarification requested updated
test limits: ADR 0016 replaces the old serial-package/20-minute gate with two
concurrent packages and five-minute deadlines after native measurements.

The financial property tests run in the normal backend suite. For a focused
iteration on ledger, cashflow, lot conservation and gains arithmetic:

```sh
./scripts/go-backend.sh test ./internal/app ./internal/api -run '^TestFinancial' -count=1
```

A test that builds a healthy known-basis book asserts self-check through one
owner, `requireKnownBasisSelfCheckPasses`: every applicable check passed and
the run passed, so a newly added check is asserted everywhere at once
([#397](https://github.com/sergeyfarin/rekenraam/issues/397)).
`requireInvestmentSelfCheckPasses` adds the investment fixture's oracles below;
`requireHealthyUnresolvedBook` is the separate unknown-basis contract.

Investment tests also run an independent conservation oracle
(`requireConservationOracle`, [#190](https://github.com/sergeyfarin/rekenraam/issues/190)).
Its expected values never come from replay or from the effective-movement
owner:

- dated long and named-short quantities against the posted journal, per
  holding and date;
- per-operation basis conservation, checked against the operation's own facts;
- unknown basis staying unknown until a sourced resolution.

Every shared investment self-check pass helper calls it, so a new
investment test should end with one of them. When a later step corrects or
reverses the revised operation, call the oracle in between: the correction
retires the revision a buggy reader would mishandle. The named multi-family
sequences are `TestOracle*`:

```sh
./scripts/go-backend.sh test ./internal/app -run '^TestOracle' -count=1
```

Operation kinds have a capability matrix
([#191](https://github.com/sergeyfarin/rekenraam/issues/191),
`backend/internal/app/investment_capability_matrix_test.go`). Writers record
only the kinds in `db.InvestmentOperationKinds`. Each kind's row names, for
replay behind a later depletion, replacement, reversal, export and self-check:

- the test that exercises it,
- the test that asserts its named refusal, or
- the GitHub issue that tracks the gap.

`TestEveryOperationKindExportsItsRowsAndPassesSelfCheck` records every kind,
checks the exported bundle rows and runs the self-checks. A second table covers
the replay-intent families. To add an operation kind, add it to the list, give
it a matrix row and record it in one of the capability scenarios:

```sh
./scripts/go-backend.sh test ./internal/app -run 'CapabilityMatrix|ReplayIntentCapabilities|EveryOperationKind' -count=1
```

These include independent exact expected amounts, fixed-seed trade sequences,
gross cashflow scope checks and HTTP scale/total assertions. Keep the full
backend script as the completion gate. See
[financial test hardening](reviews/financial-test-hardening-2026-09-19.md) for
scope, mutation evidence and limitations; coverage percentage alone is not a
financial correctness gate.
The test-hardening review records T-103, the partial-disposal allocation
defect it found; that is fixed (backlog T-103) and its regression passes under
all four methods, so the whole backend script is green again. Basis is split at
the cost commodity's maximum scale, one scale per position — when a test's
expected basis or gain looks deeper than the amount that produced it, that is
the policy, not a rounding slip.

The backend suite enforces the consolidated migration checksum and exercises
fresh and seeded baseline equivalence, including durable-data preservation
when migration admission is repeated. No legacy databases exist. A checksum
failure normally means the old migration must be restored and the change moved into the next numbered
migration. ADR 0013 permits an explicitly declared pre-release redesign of
the unused v0.1 baseline, with an updated checksum and fixtures. See
`docs/upgrades.md` for the operator path.

`COVERAGE=1 ./scripts/test-backend.sh` runs the same suite without `-race`,
writes `backend/coverage.out`, and prints the merged total. CI runs this in a
non-gating `backend-coverage` job and fails it below a soft floor
(`scripts/check-coverage-floor.sh`, `COVERAGE_FLOOR` default 73.0%) — raise
the floor deliberately when the level rises; never lower it to make a change
pass.

#### Large-book measurement

`./scripts/measure-large-book.sh` (`pnpm measure:large-book`) generates a book
at a realistic upper bound — 250,000 posted postings over 3,020 accounts in two
currencies across five years — and runs the bundle export and the self-check
against it (#108). It also measures the shell attention read (#458), asserting
successful aggregate reads and empty transaction-review queues, with ceilings
of 5 seconds and 64 MiB peak live heap, and the Home briefing read (#476),
asserting both headlines name the missing EUR rate, with ceilings of
10 seconds and 256 MiB. This does not certify complete FX
discovery from posted currencies (#464). It asserts the right answer at that size (every posting in
`ledger.csv`, one trial-balance row per leaf and commodity, a passing check)
and fails when wall time or peak live heap crosses a ceiling set several times
above the measured figures, so a sharp regression fails and a slow runner does
not. `BENCH=1` also runs the `testing.B` benchmarks with `ReportAllocs`. The
test skips unless `REKENRAAM_LARGE_BOOK=1`, so the push gate never pays for it;
CI runs it weekly in `.github/workflows/large-book.yml`.

Measured 2026-10-16 on an 8-core AMD EPYC 3251, without `-race`:

| Operation | Wall | Allocated (cumulative) | Peak live heap |
| --- | --- | --- | --- |
| Bundle export | 11.2 s | 413 MiB | 24 MiB |
| Self-check | 11.8 s | 137 MiB | 39 MiB |

Home was measured 2026-10-31 on the same machine: 5.9 s, 513 MiB allocated and
141 MiB peak heap while each headline ran its own net-worth series; 2.8 s,
261 MiB and 112 MiB once both headlines total one ledger read and fold
(`netWorthScopedSeriesTotals`). Its peak heap is the posting history the
series reads whole.

Both are SQLite-bound (over 90% of CPU in cgo), and the peak heap shows the
whole-book folds — the trial balance, account paths, the self-check
balances — stay per-account rather than per-posting. Ceilings: 60 s and
256 MiB each, in `backend/internal/app/large_book_test.go`. Raise them only
with a new measurement here.

#### Payment-in-lieu density measurement

#389's read-only diagnostic has a separate bounded measurement using the
same fixtures, with construction excluded from the timed read:

```sh
REKENRAAM_PIL_DENSITY=1 REKENRAAM_PIL_DENSITIES=0,10,100 \
  ./scripts/go-backend.sh test -count=1 -timeout=15m -v \
  -run '^TestPaymentInLieuAttributionCostByDensity$' ./internal/app
```

It checks the constructed supported/unsupported payments, including the
superseded control, and reports three wall/allocation/heap samples per shape.
The write measurement below stays separate from this explicit read diagnostic.

`./scripts/measure-payment-in-lieu-density.sh`
(`pnpm measure:payment-in-lieu-density`) now measures #388's accepted short
corrections, unrelated writes and rolled-back previews against 0/10/100/300
payments in both #379 history shapes. Fixture construction is excluded and
reported separately. It asserts unchanged cash, diagnostic outcomes and
rollback, without a latency ceiling. `REKENRAAM_PIL_DENSITIES`,
`REKENRAAM_PIL_SHAPES` and `REKENRAAM_PIL_REPEATS` select the matrix; 1,000
payments remains opt-in. The former global entitlement scan is gone; a
work-count regression independently denies attribution-context reads during
ordinary writes/previews. This manual profiler stays off push and scheduled CI.
The [historical baseline](reviews/payment-in-lieu-density-2026-10-23.md) records
why the old matrix took hours; [current delivery measurements](reviews/payment-attribution-repair-2026-10-23.md)
record the new outcomes and costs.

### Local Owner Recovery

Stop the server first: the command holds the same database lock as the
server and `restore`, and refuses while the server runs, so no in-flight
sign-in can finish with the old password after the reset (#286). Reset the
owner password locally with a verified SQLite backup first:

```sh
printf '%s\n' 'new-password' | DATABASE_URL=file:var/dev.sqlite ./scripts/go-backend.sh run ./cmd/rekenraam recover-owner --password-stdin
```

Write the verified backup to an explicit path when needed:

```sh
printf '%s\n' 'new-password' | DATABASE_URL=file:var/dev.sqlite ./scripts/go-backend.sh run ./cmd/rekenraam recover-owner --password-stdin --backup-path ../dist/recovery.sqlite
```

Only use the emergency override when backup creation or verification is impossible and you accept the risk of proceeding without a fresh verified backup:

```sh
printf '%s\n' 'new-password' | DATABASE_URL=file:var/dev.sqlite ./scripts/go-backend.sh run ./cmd/rekenraam recover-owner --password-stdin --allow-no-backup
```

### Online Provider Secret Key

`REKENRAAM_SECRET_KEY` encrypts online provider credentials **and the two-factor
shared secret** at rest; without it, both connection creation and MFA enrolment
return `CONFIG_REQUIRED` rather than storing anything in the clear. It must be
base64 for exactly 32 random bytes:

```sh
openssl rand -base64 32
```

Keep the value outside Git in the service environment or deployment secret
manager, and back it up with the SQLite database. If the key is lost, existing
online connection secrets cannot be decrypted. Restore the old key from backup,
or stop the app, take and verify a SQLite backup, start with a new key, delete
the affected online import connections, and re-add them with fresh provider
credentials. Existing imported ledger data and import history remain durable.

There is no in-place secret-key rotation command yet. Intentional rotation uses
the same backup-first delete-and-re-add procedure for stored online import
connections.

### Trading 212 Provider Stub (Development Only)

`TRADING212_BASE_URL` points the Trading 212 key probe and history fetches at
another server, such as the e2e stub in `e2e/stubs/trading212.mjs`:

```sh
node e2e/stubs/trading212.mjs
APP_ENV=development TRADING212_BASE_URL=http://127.0.0.1:16890/api/v0 pnpm dev:backend
```

It is honored **only** with `APP_ENV=development`. Startup refuses it in
production (and a hand-built runtime config refuses it too), because the API
key travels in the `Authorization` header of every provider request. The stub
answers only API keys registered through `PUT /__control/accounts/{key}` and
serves their orders, dividends and cash transactions in the provider's wire
shape; `e2e/playwright/support/trading212.ts` wraps that control call. Starting
an online import or refresh wakes the fetch worker immediately, so a stubbed
fetch is staged in seconds rather than on the worker's one-minute tick.

### Frontend Validation

Runs SvelteKit checks and the Vitest unit suite.

The dev, check and build commands first run `catalogs:check`. That gate runs
`svelte-kit sync` before Vitest so SvelteKit's generated `$app/tsconfig` exists
on a fresh checkout (#207). After `pnpm install --frozen-lockfile`, no separate
warm-up command is needed. The same bootstrap applies when running
`pnpm --dir frontend run catalogs:check` directly; catalog parity and OpenAPI
generation remain required parts of the normal frontend commands.

```sh
./scripts/test-frontend.sh
```

### Integrated Build

```sh
pnpm build
```

Builds the frontend, copies it into `backend/internal/web/dist/`, runs the
embed test, and compiles `dist/rekenraam`.

#### Reproducibility

Native SQLite builds use `CGO_ENABLED=1`, `-trimpath` and the shared
`fts5,netgo,osusergo` features. SQLite C source is bundled in the pinned module,
so the artifact does not need a separately installed `libsqlite3`. The default
Linux artifact dynamically links libc. A libc-independent static artifact is
separate work; cross-compilation is not a gate for a platform-native build.

```sh
pnpm build
sha256sum dist/rekenraam
ldd dist/rekenraam
./scripts/go-backend.sh build -trimpath -o /tmp/rekenraam-repeat ./cmd/rekenraam
sha256sum /tmp/rekenraam-repeat dist/rekenraam
```

Matching hashes require identical source, embedded frontend, Go patch version,
C compiler/linker, headers, libc target, build tags, CGO flags and module graph.
Record those inputs and base-image digests with release artifacts. `-trimpath`
removes builder paths; it does not make differing C toolchains equivalent.
The Docker build uses the Debian trixie C toolchain/runtime together. A binary
built on a newer libc may not run on an older host: build for the actual target
baseline or use the container. The delivery review records measured local and
container dependencies and same-input repeated-build hashes.

### Release candidates (#333)

[ADR 0020](adrs/0020-release-signing-and-platform-targets.md) adopts Sigstore
keyless GitHub Actions OIDC signing with Cosign. Initial server targets are
Linux amd64 and arm64 on Ubuntu 24.04 / glibc 2.39. Build jobs use native
architecture runners, inspect actual ELF requirements and record the exact
runner/toolchain inputs. The Debian trixie Docker runtime is unchanged.

The manual `Release Candidate` workflow (`release-candidate.yml`) runs only
from `main` in this repository. It creates Actions artifacts with a
`vMAJOR.MINOR.PATCH-rc.N` label, without creating a tag or release. It builds
both platforms, compares an unsigned repeated build, boots the packaged
server with a disposable database, and runs the focused browser release
preflight on amd64. A separate OIDC-enabled job then signs `SHA256SUMS` and
verifies its signature, expected identity, file hashes and tamper controls.
No ordinary build job gets an OIDC grant.

After the workflow has been committed and pushed, an operator can request a
candidate explicitly:

```sh
gh workflow run release-candidate.yml --ref main -f version=v0.1.0-rc.1
```

Download only the final `signed-release-candidate` Actions artifact; the
intermediate platform artifacts have no common signature. It contains the two
`.tar.gz` downloads, `RELEASE-NOTES.md`, `SHA256SUMS` and
`SHA256SUMS.sigstore.json`. Each platform archive contains the executable,
license, `BUILDINFO.json` and `MIGRATIONS.sha256`. Metadata includes the actual
highest migration and all hashes, source SHA, architecture, required libraries,
glibc symbol ceiling, Go build info, Node/pnpm/compiler/libc versions, runner
image and build-input hashes. Retain the artifacts and hosted run evidence
outside the workflow's 14-day Actions retention when accepting a candidate.

Install the pinned Cosign 3.1.4 from its
[official release](https://github.com/sigstore/cosign/releases/tag/v3.1.4).
Verify before extracting or running a downloaded executable:

```sh
gh run download RUN_ID -n signed-release-candidate -D /tmp/rekenraam-candidate
./scripts/verify-release.sh /tmp/rekenraam-candidate \
  https://github.com/sergeyfarin/rekenraam/.github/workflows/release-candidate.yml@refs/heads/main
```

The expected identity above is the verifier's trust policy; do not substitute
an identity supplied inside downloaded files. The verifier checks the GitHub
Actions issuer, authenticates the manifest, then refuses changed/missing files,
unlisted extras, duplicate paths and symlinks. To repeat the real signature and
wrong-identity/manifest/archive controls, run:

```sh
./scripts/test-release-signature.sh /tmp/rekenraam-candidate \
  https://github.com/sergeyfarin/rekenraam/.github/workflows/release-candidate.yml@refs/heads/main
```

Local packaging requires a clean committed checkout and the Linux native
toolchains from Daily Commands, plus Python 3 and binutils (`readelf`):

```sh
python3 scripts/test-release-artifacts.py
./scripts/build-release-candidate.sh v0.1.0-rc.1 amd64 dist/candidate-linux
```

Use the host's matching architecture (`arm64` on ARM), and an empty output
directory for each attempt. The script compares unsigned builds, packages
deterministically, then boots the extracted archive. It creates no signature;
local key-pair verification is not GitHub OIDC evidence. Ubuntu 24.04 native
hosted validation remains necessary for the declared runtime baseline.

Actual release publication is a separate, explicitly authorized step. Before
the first installed `v0.1.0`, pin every included migration in
`backend/migrations/freeze_test.go` and activate the documented installed-release
upgrade and branch/PR policies. Resolve the roadmap's remaining release gates,
use an exact approved release-tag identity, and publish the verified assets
through a draft with release immutability enabled. No publication job is
provided by the candidate workflow. A future Wails Windows target adds
Authenticode-signed executables/installers before the shared checksum/signature
stage; the Windows publisher certificate/provider is selected then.

### E2E

```sh
./scripts/test-e2e.sh
E2E_BASE_URL=http://localhost:16888 ./scripts/test-e2e.sh
```

Run just the fast journeys — everything except the serial release-preflight
suite. This is what CI runs for every main push and pull request:

```sh
./scripts/test-e2e-smoke.sh
```

Both E2E wrappers first run `pnpm --dir e2e run test:stubs`, which can also be
run alone. These Node HTTP tests launch the provider stub on a temporary
localhost port and verify that malformed input and provider transformations
return a fixed generic 500 JSON response without exception diagnostics, with
normal provider requests still working afterward (CodeQL alert #84).

Both wrappers also run `pnpm --dir e2e run test:harness`, whose deliberate
success/failure/skip and collection-failure controls verify the timing reporter.
Setup controls execute a prepared server artifact and check inherited Go caches,
app-binary reuse, external-app setup, audit-only omission and compilation failure.
Its browser controls hold pending/chained requests and fonts, abort a request,
and refuse late observation to check the sweep readiness helper independently.
Every browser run writes `e2e/timing-results/summary.json` and incremental
`attempts.ndjson`. Set `E2E_TIMING_DIR` to a separate directory for each
measurement you want to retain. The report records the source revision, local app
binary hash, worker/runner context, global setup and build time, all selected
cases and every attempt's status, hooks, fixtures and named sweep steps.
Setup's `reference_server_build_ms` records the test-server compilation within
`build_ms`, including failed compilation; it is outside case startup budgets.
API timing totals omit locator arguments; request/response bodies, assertion
values and browser stdout are not retained in these timing artifacts.
Step durations can nest and overlap; do not add them together as wall time.
CI uploads browser timings on successful and failed runs, separately from its
failure-only debug artifacts.

A failed attempt keeps a lightweight Playwright trace and a final screenshot
under `e2e/test-results/<case>/` (#414): the trace has every action's
actionability log, so you can see where a click or fill waited, plus call
stacks and console output. DOM snapshots, trace screenshots and browser network
capture are off by default: full tracing measured 25–35% slower and the light
trace 1–6%.
Use the worker's `app.log` for the server side of a request. Open the trace with
`pnpm --dir e2e exec playwright show-trace <path>/trace.zip`.

For a genuinely full local trace, set `E2E_FULL_TRACE=1`, for example while
reproducing one shared-search case:

```sh
E2E_FULL_TRACE=1 pnpm --dir e2e exec playwright test shared-search --grep "#472"
```

That selection records every case, including passing ones, with DOM snapshots,
trace screenshots, browser network records and sources. It is a focused
diagnostic, not a suite setting: full tracing is slower and its retained traces
are much larger. Do not expect `--trace=on` to do this: Playwright replaces only
the configured trace mode and keeps the light trace's disabled screenshots,
snapshots and sources, so the resulting ZIP still has no DOM or network evidence
(#473). CI and the normal wrappers leave `E2E_FULL_TRACE` unset.

`investments-capital-return-reads.spec.ts` additionally keeps DOM snapshots and
browser network evidence for its seven cases (#414), with trace screenshots
and sources still off. Its reversal phases are named timing steps, so a stalled
click is separate from navigation, reason entry and submit. All other specs
retain the global light trace, and `E2E_FULL_TRACE=1` upgrades this scoped spec
too. The [2026-10-29 follow-up](reviews/capital-return-stall-2026-10-29.md)
records the failure-artifact control, tracing cost and current reproduction
limits.

The route and sub-view sweeps run in separate projects before ordinary app
journeys, with their own worker books. Their complete matrices and assertions
remain selected. Worker-scoped seed fixtures retain one seed per worker book
without `beforeAll` batches pinning several long tests to the same worker.
Each sweep observes requests before navigation, then waits
for current/chained requests, font loading and two rendered frames instead of
repeating a fixed 500 ms network-idle delay. Route identity and explicit
heading/control readiness assertions still run. This helper cannot predict
future debounced work; keep state-specific assertions when reaching such views.
An external app still shares one book and runs on one worker.

CI runs the shared smoke wrapper twice on independent runners:
`E2E_SUITE=app` selects bootstrap, FX observation isolation and app journeys;
`E2E_SUITE=audits` selects the route/sub-view matrices. The selections are
disjoint and their union is the complete smoke gate. Each job retains three
workers, builds the integrated binary and runs the stub/harness controls.
Both jobs run on every existing CI event, and a failed group does not cancel
the other group. This uses two runners and repeats setup/build; compare the
slower job's wall time and both jobs' combined runner minutes. Leaving
`E2E_SUITE` unset (or `all`) runs the complete selection in one local command.

For comparisons, hold source cohort, binary, browser, worker count and cache
conditions constant and retain all repetitions, case/matrix inventories,
setup cost and runner minutes. `E2E_SKIP_BUILD=1` measures execution with a
previously validated binary; label that separately from the self-contained
wrapper's full build/run wall time. A timing from a different revision or
runner is an observation, not a controlled speedup claim.
The [#381 delivery report](reviews/ci-browser-runtime-2026-10-21.md) retains
the matched local comparison, complete inventories and extra runner cost.

Run the release preflight when preparing a release:

```sh
pnpm test:release-preflight
```

- **Do not read the exit code of a piped run.** `./scripts/test-e2e-smoke.sh | tail -25` reports `tail`'s status, which is always 0, so a failing suite looks green — that is shell pipeline semantics, and nothing inside the script can change it. Run it unpiped, or set `set -o pipefail` in the calling shell first, which is what the CI step that tees backend coverage does.
- `./scripts/test-e2e.sh` builds the integrated app once (Playwright global setup), then every worker starts its own instance of `dist/rekenraam` on its own port (consecutive from `127.0.0.1:16900`; move the base with `E2E_PORT`) against its own SQLite file and backup directory under `backend/var/e2e/<project>-<index>/`, where its `app.log` also lands. The fixture lives in `e2e/playwright/support/fixtures.ts`; specs import `test` and `expect` from there, not from `@playwright/test`, or they would reach no app at all.
- Workers run side by side because no two share a book: 50% of the cores locally, 3 under `CI` (the 4-vCPU GitHub runner). Override with `E2E_WORKERS` or `--workers`. Files still run serially inside a worker, and a worker restarted after a failure reuses its database, so a spec may rely on its own file's earlier tests but never on another file's data — which file runs in which worker is not fixed.
- `auth.spec.ts` needs an owner-less database. It runs in its own `bootstrap` project, which gets its own worker database, so nothing has to run before or after it.
- `owner-current-reference.spec.ts` uses `support/reference-date.ts`: it starts
  an isolated test-only Go HTTP server with fixed financial clocks, real
  auth/SQLite and the built frontend. Global setup compiles its executable once
  with the native Go wrapper before any case starts, including when
  `E2E_SKIP_BUILD=1` reuses the app or `E2E_BASE_URL` selects an external app.
  Audit-only setup omits it. Each case launches that executable directly, so
  cold compilation cannot consume its unchanged 45-second readiness budget.
  Normal backend tests skip that server test; no clock override is present in
  the production binary.
- A test that asserts a book-wide read — self-check run status, counts or the
  20-record samples — imports `test` from `support/owned-book.ts` instead (#398).
  Each case starts its own `dist/rekenraam` on a free port against a fresh
  database under `backend/var/e2e/owned-*/`, so every finding in the run is the
  test's own. Facts about one transaction stay on the worker book and read that
  transaction's own diagnostic. Owned-book cases skip against `E2E_BASE_URL`.
- Run browser invocations sequentially: global setup clears the shared worker
  directory even with `E2E_SKIP_BUILD=1`.
- Set `E2E_SKIP_BUILD=1` to reuse the existing `dist/rekenraam` while iterating on specs, e.g. `E2E_SKIP_BUILD=1 pnpm --dir e2e exec playwright test investments-split`.
- Set `E2E_BASE_URL` when you want Playwright to target an already-running app instead of booting its own. That app has one book, so the run drops to one worker and orders `bootstrap` first.
- The harness also boots the Trading 212 provider stub on `127.0.0.1:16890` (move it with `TRADING212_STUB_PORT`) and starts the app with `TRADING212_BASE_URL` pointing at it, so import journeys can create connections and fetch fills without the real provider. `MARKET_DATA_BASE_URL` points every built-in quote/FX provider at the same stub, whose 404s keep background FX coverage from importing live rates into the shared e2e book (#226); both overrides are refused outside `APP_ENV=development`. Specs that need it call `requireTrading212Stub()`; against an external `E2E_BASE_URL` they skip unless `E2E_TRADING212_STUB_URL` names a stub that app is configured to use.
- The harness sets a throwaway `REKENRAAM_SECRET_KEY` for the instance it boots, because the MFA journey cannot enrol without one — enrolment returns `CONFIG_REQUIRED` rather than storing the shared secret in the clear. Export your own `REKENRAAM_SECRET_KEY` to override it.
- `mfa.spec.ts` shares its worker's database with the other files that worker runs, so it turns MFA back off in `afterAll`. A spec that changes account-wide authentication state must clean up the same way. A hook (`beforeAll`/`afterAll`) that opens its own page or context passes `baseURL: backend.url` from the worker fixture, since test-scoped `baseURL` is not available there.
- Browser MFA enrollment uses `support/mfa-enrollment.ts` (#384). Locale
  journeys use the current TOTP step because sign-in consumes a recovery code.
  Main and proxy TOTP journeys reserve the current step and track the step
  actually consumed. Refresh a previous-step code only after the exact
  invalid-code 401 and an observed advancing step, with at most three total
  submissions. Other refusals and failures without advancement fail immediately.
  `mfa-enrollment-fixture.spec.ts` exercises the real UI and enrollment API with
  stale fixture samples, including the boundary, refusal and attempt-limit
  controls; the app clock and accepted security window remain unchanged.
- Set `PLAYWRIGHT_CHROMIUM_EXECUTABLE` to an existing Chromium binary when the sandbox or image cannot download the revision this Playwright release pins (`pnpm exec playwright install` fails, and the run dies with "Executable doesn't exist"). Container images that preinstall a browser usually expose one at `/opt/pw-browsers/chromium`. Prefer this over patching the browser cache by hand; leave it unset locally so Playwright uses its own pinned build.

## Area Notes

- Backend code lives in `backend/`; focused commands use `./scripts/go-backend.sh`. The full validation gate remains `./scripts/test-backend.sh`.
- Finish an integrated build before running Go gates that embed its assets.
  Building replaces `backend/internal/web/dist`; overlapping it with compilation
  can remove files after Go has enumerated them. Browser checks can reuse that
  completed, current binary with `E2E_SKIP_BUILD=1`.
- `scripts/test-e2e-acceptance.sh` runs only the browser cases tagged
  `[acceptance]` — the ones that map onto a plan's validation matrix, so closing
  an initiative can point at a suite rather than at an argument (T-61). The
  broader suites are split by cost instead: smoke in every CI run, release
  preflight before a release.
- The backend binary also exposes local maintenance commands: `recover-owner`
  (backup-first password recovery and session revocation), `verify-backup`
  (integrity, schema version, row counts, and whether sealed rows decrypt under
  the configured `REKENRAAM_SECRET_KEY`), and `restore` (refuses while the
  server holds its lock, preserves the previous database as a whole file set,
  installs atomically).
- Frontend code lives in `frontend/`; SvelteKit builds static output that is copied into `backend/internal/web/dist/` for the single binary.
- Frontend foundation libraries are pinned in `frontend/package.json`: Tailwind CSS, Bits UI, shadcn-svelte, Paraglide JS, `@tanstack/svelte-query`, `@tanstack/svelte-table`, `openapi-typescript`, `openapi-fetch`, `date-fns`, and MiniSearch. Add TanStack Virtual only when a concrete screen needs it.
- Money parsing, editable prefills and exact arithmetic belong to `frontend/src/lib/money/amount.ts`; read-only display belongs to `frontend/src/lib/money/format.ts` (`formatQuantity`). These use strings/BigInt and `Intl.NumberFormat`, with no money-formatting dependency. Use `date-fns` for date manipulation and `Intl.DateTimeFormat` for final locale-aware date display. See [the formatting conventions](conventions.md#frontend-conventions) and the frontend-screen skill.
- The frontend runs **TypeScript 6.0** as a deliberate, tested exception: `openapi-typescript` 7.13 still declares `peerDependencies.typescript: ^5.x`, but schema generation, `pnpm --dir frontend run check` and the build pass on 6.0 with unchanged generated output. Re-run those three checks on every TypeScript or generator bump, and fall back to the newest 5.x if generation breaks. TypeScript 7 still breaks the schema generator; that upgrade stays tracked as T-48 [#112](https://github.com/sergeyfarin/rekenraam/issues/112), which the 6.0 upgrade does not close.
- Frontend message catalogs live in `frontend/messages/`, split by domain as `frontend/messages/<domain>/<locale>.json`. Keep message IDs flat and prefixed by domain or screen intent instead of using one growing locale file or deep nested objects. Generated Paraglide output lives in `frontend/src/lib/paraglide/`. Regenerate the typed message layer with `pnpm --dir frontend run paraglide:compile` when changing catalog structure outside the normal `dev`, `check`, or `build` scripts.
- Shipping locales are `en, es, fr, nl, de, ru`, listed in `frontend/project.inlang/settings.json`. All configured message directories compile into the single `m` namespace, so **a key must be unique across all configured domains** — which file it lives in is grouping, not namespacing.
- Add each new message to the maintained locales (`en`, `nl`, `ru`) in the same change. `es`, `fr` and `de` are partial (owner decision 2026-11-01, #493): new or changed messages may be missing there and fall back to English, tracked in a generated list and caught up in batches, complete before the v0.1.0 tag. Until #493 lands the parity gate still rejects a key missing from any of the six. Per-message English fallback remains a runtime safeguard. Adding a *term* requires review — put it in `docs/localization-glossary.md` first, because a plausible-but-wrong financial term is worse than English.
- **Locale resolution is configured in two places that must agree**: `strategy` in `frontend/vite.config.ts` and the `--strategy` flag on the `paraglide:compile` script in `frontend/package.json`. Both list `localStorage`, `preferredLanguage`, `baseLocale` — the CLI flag is variadic, so the names are space-separated there, not comma-separated. If they drift, the plugin's compile usually wins and hides the mistake — until something skips it and the app silently ships English only.
- Run `pnpm --dir frontend run catalogs:check` to compare every configured domain and locale with the Inlang base locale, including keys, placeholders, nonempty values and shared-namespace collisions. Frontend dev/check/build and the shared frontend/CI gate run it automatically. Catalog parity does not establish native terminology approval; use [the #102 review packet](reviews/localization-native-review-2026-10-13.md). `e2e/playwright/language.spec.ts` covers the switcher end to end and deliberately asserts strings from both the app and settings catalogs, since a missing file would otherwise show up only as English on half a screen.
- Multi-state Svelte routes should stay thin route entrypoints. Move reusable workflow UI and route-specific helpers into feature folders under `frontend/src/lib/`, as with `frontend/src/lib/install-gate/`.
- Frontend OpenAPI types are generated from `api/openapi/openapi.yaml` into `frontend/src/lib/api/schema.d.ts`. The root OpenAPI file is only the entrypoint: keep top-level metadata, global components, and `$ref` registrations there rather than inline path or schema bodies.
- Add new API endpoints by creating or extending a path item under `api/openapi/paths/`, then referencing that file from `api/openapi/openapi.yaml` under `paths`. Keep each path item focused on one route pattern such as `institutions.yaml` or `institution-versions.yaml`.
- Add or extend schema groups under `api/openapi/components/schemas/`, then expose individual schemas through the root `components.schemas` map. Path files should reference shared schemas through `../openapi.yaml#/components/schemas/...`; schema group files should reference siblings through `../../openapi.yaml#/components/schemas/...`.
- Regenerate frontend types with `pnpm --dir frontend run openapi:generate` when changing the contract outside the normal `dev`, `check`, or `build` scripts.
- Keep the OpenAPI client seam language-agnostic. Do not embed user-facing English fallbacks in `frontend/src/lib/api/`; translate API status and error presentation at the UI layer with Paraglide using stable error codes or screen-specific fallback copy.
- API examples and contract assets live in `api/`; use the Bruno `local` environment for the backend dev server and `app` for an integrated binary or Docker app.
- Login-created sessions default to 30 days. Set `SESSION_LIFETIME_HOURS` to a positive integer number of hours when testing shorter or longer session expiry behavior.
- Production generates a one-time setup token when `SETUP_TOKEN` is absent; operators should set a durable random token of at least 32 characters and enter it only for the first owner-creation request. See `docs/deployment-security.md`.
- Forwarded proxy headers are ignored by default. Set `TRUST_PROXY_HEADERS=1` only when the app is behind a trusted reverse proxy that rewrites those headers, and set `TRUSTED_PROXY_CIDRS` to the proxy source ranges that are allowed to supply them.
- Browser e2e tests live in `e2e/` and use Playwright. Keep them focused on user journeys that need a browser.
- `e2e/playwright/release-preflight.spec.ts` is the local release preflight for critical financial workflows. It is intentionally serial against one fresh SQLite database and is not part of push CI; `.github/workflows/release-preflight.yml` runs it weekly and via `workflow_dispatch` so it cannot drift unseen (#306). Because it is `describe.serial`, a failure in the first test skips the rest of the file — check "did not run" counts before concluding a journey passed.
- **Never hard-code a calendar date for a transaction, statement, or imported row in an e2e spec.** Everything a spec seeds is versioned as effective from *today*, and posting validation resolves account and commodity rules as of the entry date, so a date in the past relative to that seeded data matches no version and the posting is rejected. Use `todayISO()` / `todayQIF()` from `e2e/playwright/support/dates.ts`. A hard-coded `2026-07-08` silently broke every browser transaction-entry journey once the wall clock passed it (fixed 2026-08-06).
- **Per-locale translation journeys run in `en` and `ru` once #493 lands**; formatting-only tests such as `locale-amounts` keep all six. **Give each locale of a multi-locale journey its own test.** Loop `for (const locale of [...]) test(...)` around the body rather than walking every locale inside one test: six passes in one body exceed the default 30 s budget under full-suite load (#250), and a fresh browser context per test means a stored `PARAGLIDE_LOCALE` needs no `finally` reset — which itself fails once a timeout has closed the page. When the locale passes check state an earlier journey built (a saved report, a currency's cumulative totals), put the journey and the per-locale tests in one `test.describe.serial`, seeding in the journey test or a `beforeAll`; keep the `[acceptance]` tag on split-off tests whose journey carried it, since `--grep` selects tests by title (#251).
- **Assert a saved transaction with `expectSavedTransaction()`, not a bare `getByText(description)`.** Saving opens a detail panel, so the description can be on screen three times at once and a loose text locator fails Playwright strict mode — a failure that reads as "not saved" when the opposite is true. Note also that the list row shows the *payee* instead of the description when there is one, so asserting against the row alone is not portable either.
- Docker assets live in `deploy/docker/` and must preserve the same single-app production shape as the binary.
- `deploy/docker/proxy-audit/run.sh` stands up a disposable Caddy TLS proxy (local CA) in front of the supplied Compose app. It runs the #268 proxy/shared-browser checks (`e2e/proxy-audit.config.ts`, outside the main suite) and tears everything down. Rerun it when changing cookies, proxy trust, security headers, session exit or MFA key handling. See its README.
- `backend/var/`, `backend/internal/web/dist/`, and `dist/` contain local or generated files. Their README files are placeholders that keep those ignored directories present in Git.

## Default Change Workflow

1. Read the active requirements and conventions for the touched area.
2. Make the smallest durable change that satisfies the slice.
3. Run the narrowest relevant validation.
4. Update docs or ADRs if the change introduces a durable new rule.
5. Keep commits focused.

## Migrations And Resetting Your Database

The governing rule is **Project Lifecycle And Migration Immutability** in
`docs/conventions.md`; this section is only how to carry it out. The unused
v0.1 candidate baseline may be redesigned under ADR 0013; installed release
migrations are immutable. Local development databases are disposable.

Adding schema:

```sh
# new sequential file, goose format (-- +goose Up / -- +goose Down)
$EDITOR backend/migrations/00NN_short_name.sql
./scripts/test-backend.sh
```

Reset only a disposable development database after a baseline rewrite. Stop the
app first, export anything worth keeping, and remove the SQLite file selected
by `DATABASE_URL` together with its `-wal` and `-shm` sidecars. For the default
`pnpm dev` database:

```sh
rm -f backend/var/dev.sqlite backend/var/dev.sqlite-wal backend/var/dev.sqlite-shm
```

Restart the app to run migrations and repeat owner setup. If `DATABASE_URL`
names another file, remove that file and its sidecars instead. For a populated
installation, follow [the upgrade and restore guide](upgrades.md).

The per-change reset notices are retained in the
[pre-release baseline rewrite archive](archive/pre-release-baseline-rewrites.md).
Databases and backups made before a baseline rewrite or consolidation are
incompatible with that baseline: export anything worth keeping before updating,
then recreate only the disposable development database. Do not use restore to
cross a rewritten-baseline boundary. Future forward migrations preserve data
only when the existing database uses the same `0001` baseline.

**#405 baseline (2026-10-26):** migrations 0001–0027 are consolidated into
`0001_initial_schema.sql`. The head is **1**. No legacy or installed database
exists. Forward migration numbers from the previous candidate are historical;
their capabilities remain in the baseline, not as supported upgrade targets.
The next additive schema change uses `0002`.

**#417 (2026-10-27):** forward migration `0002_category_version_delete_guard.sql`
drops the custom-category exemption from `account_versions_no_delete`, so the
head is **2** on the same baseline identity. Existing #405 databases and
backups upgrade in place; nothing needs a reset.

The baseline records generation **20261026** in SQLite's `PRAGMA user_version`,
separately from Goose's forward migration number. Startup refuses older
candidate files (including old version-1 files) with `incompatible pre-release
baseline`, before running migrations. Populated unversioned files also refuse;
`recover-owner` still backs them up before refusing admission without resetting
credentials. Restore refuses backups from that
baseline before preserving or replacing the destination. `verify-backup`
reports the mismatch and exits `3`, distinct from an unusable file (`1`). Export with the original build before updating, then
reset only disposable development data using the procedure above. A genuinely
newer forward-schema backup with the same baseline still has its separate
"newer schema" refusal. Never use restore across this boundary.

The e2e database resets itself on every self-managed run and needs no manual reset.

- Keep nothing in a local database you would be sorry to lose. Reproducible
  fixtures belong in the repo; `backend/var/*.sqlite` is scratch.
- Rewriting an already-committed migration requires `BREAKING DEV DATABASE` as
  the first line of the commit body, plus the reset instruction for other
  developers. Grep for past ones with `git log --grep='BREAKING DEV DATABASE'`.
- Two branches that both added `00NN_` collide. The one merged second renumbers
  **its own** file to the next free number while resolving the merge. Renumbering
  a migration already on `main` counts as a rewrite.
- Migration numbers are monotonic database sequence numbers, not release
  numbers. Release notes record the highest migration included in each release.
- CI validates the fresh-install path on every run. Candidate seeded equivalence
  is covered by `TestMigrateSeededBaselineMatchesFreshSchema` in `internal/db`:
  it loads `internal/db/testdata/v01_seed.sql` into migration 0001 alone,
  applies any later embedded migrations, repeats migration admission, and
  checks schema equivalence and every durable
  figure — row counts, exact coefficients/scales, lifecycle states, lot and
  checkpoint conservation, per-commodity balance and `foreign_key_check`.
  Legacy upgrade fixtures are added once installed releases actually exist.
- **Installed-release seed fixtures are frozen.** The unused v0.1 candidate
  seed may be revised with its baseline under ADR 0013. Once a release is
  installed, its seed stands in for a database written by that version, so
  regenerating it against a later schema defeats the test it feeds. A new
  installed release gets a *new* seed file, built with
  `./scripts/build-release-seed.sh <out.sql>` — which drives the real HTTP API
  (`TestBuildReleaseSeedFixture` in `internal/api`) so the fixture is a book the
  app would actually have written, then shapes the dump. It restores the book
  currency before account/posting admission and orders operation headers/journal
  links before lot consumers. Before writing the proposed output,
  `TestValidateReleaseSeedFixture` loads it through the native baseline and
  compares source row counts and exact figures, runs the ledger/lot/checkpoint
  oracles and repeats migration admission. Output is not
  byte-reproducible: it carries real timestamps and request UUIDs.
- This is what stands between a post-`v0.1.0` schema redesign and silent data
  loss. SQLite's twelve-step table rebuild is how such a redesign has to happen,
  and a rebuild that re-points the wrong foreign key or rescales a coefficient
  leaves every row count identical. Widen the seed before writing a migration
  that touches something it does not cover.

## Work Tracking

Use GitHub issue numbers for active work and the
[work-item template](../.github/ISSUE_TEMPLATE/work-item.md) for a bounded
problem, delivery scope, acceptance, dependencies and validation. Future
initiatives may stay brief; complete actionable acceptance before coding.
Existing local codes remain historical aliases in `docs/backlog.md`.

Follow [Work Tracking](conventions.md#work-tracking) for event-driven progress
and closure. Update the roadmap/capability ledger when their facts change;
post validation and commit evidence on the feature issue. Delivery checklists
live on GitHub, while durable behavior and scope decisions live in plans/ADRs.

## Commit Conventions

This repo uses Conventional Commits.

Preferred examples:

- `feat(backend): add opening balance endpoint`
- `feat(frontend): add account list empty state`
- `fix(api): validate unbalanced postings`
- `docs(requirements): lock export scope`
- `test(backend): cover reconciliation lock behavior`

Use the smallest honest scope. Avoid mixing unrelated concerns in one commit.

A commit that rewrites an already-committed migration must additionally start
its body with `BREAKING DEV DATABASE` — see *Migrations And Resetting Your
Database* above.

## Branches And PRs

**Commit directly to `main`.** This is the default for all work, including
changes to product rules, conventions, workflows, and CI. Do not open a feature
branch unless there is a specific reason to (owner decision, 2026-08-23).

Why: the project is pre-release with a single contributor, so there is no
release to protect and no second reviewer for a PR to reach. Branches bought
nothing and cost something — the 2026-08 merge of two long-diverged branches is
where the duplicated `docs/conventions.md` paragraph, the duplicated
`writePricingServiceError` arms, the duplicate CodeQL workflow, and the
colliding T-42–T-47 backlog IDs all came from.

What replaces the PR as the safety net:

- **`main` stays green.** Run the narrowest relevant validation before pushing
  (see *Validation Expectations By Area* below). CI runs the same scripts, so a
  local failure is a push that should not happen.
- **Commits stay focused and honest.** The commit message is now the only
  review artifact, so it carries the reasoning a PR summary would have. A commit
  that changes product requirements, conventions, or ADRs says so in its body.
- **Push often.** Long-lived local work recreates exactly the divergence this
  rule exists to avoid.

Branch when the work genuinely needs isolation — a spike you may throw away, or
a change you want CI to run before it reaches `main`. That is a judgement call,
not a default.

**This flips at the `v0.1.0` release**, the same milestone that freezes
migrations (see *Project Lifecycle And Migration Immutability* in
`docs/conventions.md`). From that tag, feature branches and PRs become the norm:
once a released version exists, `main` is something users can be running.

### Post-Merge Audit

After any non-trivial merge — two branches that both changed code, or any merge
with resolved conflicts — run, before pushing:

```sh
./scripts/check-post-merge.sh                  # after committing the merge, or mid-resolution
./scripts/check-post-merge.sh REF_A REF_B      # compare tests against chosen refs
```

It mechanically checks the shapes the 2026-08 merge shipped: migration and ADR
number collisions, a generated OpenAPI client that changes on regeneration
(`SKIP_OPENAPI=1` skips it without frontend deps), duplicated paragraphs in
Markdown docs, workflow files that share a name, action, or run command,
work-item IDs mapped or headed twice, and Go/Vitest/Playwright tests present
on a merged side but missing from the result. Every finding is a prompt to
look, not proof: a test the merge deliberately replaced is reported too, and is
acknowledged once its coverage is confirmed elsewhere. Run on that merge
(`d1ba2c10`), the lost-test check lists the reports tests of the side whose
implementation was dropped — exactly the review that merge never got.

## Validation Expectations By Area

### Backend

- Usually validate with `./scripts/test-backend.sh` — it gates on `gofmt -l`, then runs `go vet ./...` and `go test -race -p 2 -timeout=5m ./...`. Two packages run concurrently with a five-minute deadline each; normal test parallelism uses available Go CPU resources.
- If backend changes affect the integrated binary shape, also run `pnpm build`.

### Frontend

- Usually validate with `./scripts/test-frontend.sh`.
- If the change affects static build output, also run `pnpm build`.

### Docs And Requirements

- Keep cross-references and commands consistent.
- Run `git diff --check -- ...` for touched docs when no narrower executable validation exists.

### Testing

- Treat local validation scripts as the development contract.
- CI commands must match the local wrapper scripts exactly.
- Ordinary backend integration tests open an isolated copy of the process-wide
  migrated template through `internal/testdb.Open`. Use `db.Open` plus
  `db.Migrate` directly only when the test is specifically proving migration,
  restore, or fresh-start behavior; repeating the complete schema setup in
  ordinary fixtures makes the race-detector feedback loop needlessly expensive.

## GitHub Actions

The repo uses a fast CI workflow in `.github/workflows/ci.yml`.

Workflow conventions:

- Fast CI covers race-detected backend tests, frontend checks plus Vitest, and
  the integrated build.
- Go vulnerability scanning runs in `.github/workflows/govulncheck.yml` with
  `govulncheck ./...` from `backend/` on a weekly schedule, manually via
  `workflow_dispatch`, and on backend-affecting pushes to main and pull requests.
- The large-book export and self-check measurement runs in
  `.github/workflows/large-book.yml` weekly and via `workflow_dispatch`, never
  on push: `./scripts/measure-large-book.sh` (see Large-book measurement).
- CodeQL code scanning runs as GitHub's **default setup**, configured in
  repository settings (Security → Code scanning), covering `actions`, Go, and
  the JS/TS languages. There is no committed CodeQL workflow.

  From 2026-08-23 to 2026-09-10 this was an *advanced* setup — a committed
  `.github/workflows/codeql.yml` running `actions/setup-go` with
  `go-version-file: backend/go.mod` — because default setup builds Go with
  whatever the runner preinstalls under `GOTOOLCHAIN=local`, and CodeQL's own
  bundled Go extractor could not parse a 1.27 module besides. Both halves are
  now fixed: the extractor gained 1.27 support (github/codeql#22042, CodeQL
  bundle 2.26.4, early September 2026) and the hosted runners preinstall Go
  1.27, so default setup extracts the module cleanly. The `codeql.yml` workflow
  was removed 2026-09-10.

  Default and advanced setup are mutually exclusive. While default setup is
  enabled, re-adding a `codeql.yml` gets its SARIF rejected on upload with
  "CodeQL analyses from advanced configurations cannot be processed when the
  default setup is enabled". If the Go-toolchain problem ever recurs, restore
  advanced setup by disabling default setup in repository settings *first*,
  then committing a single workflow file that pins the toolchain with
  `go-version-file: backend/go.mod`.
- `gosec` scanning was removed 2026-09-10 (backlog T-59). It existed only as
  interim first-party Go coverage while CodeQL's Go analysis was paused; with
  CodeQL's Go queries restored under default setup, `govulncheck.yml` covers
  known dependency CVEs and CodeQL covers first-party taint bugs (SQL
  injection, SSRF, path traversal). Its 53 stale analyses and 71 orphaned
  Security-tab alerts — almost all false positives, see T-59 — were deleted
  through the code-scanning analyses API at the same time.
- Dependabot version updates are configured in `.github/dependabot.yml` for
  GitHub Actions, the backend Go module, the root pnpm workspace, and the
  Docker runtime image. The root npm entry updates `frontend/package.json`
  together with `pnpm-lock.yaml`; a separate `/frontend` entry creates PRs
  whose manifests fail CI's frozen install.
- `ci.yml` runs one full validation per context (#383): each push to `main`,
  each pull request update (GitHub's `refs/pull/N/merge` merge context) and
  each `workflow_dispatch`. A push to any other branch runs nothing on its own;
  open a pull request, or dispatch it with
  `gh workflow run ci.yml --ref <branch>` to validate a branch without one. A
  newer run cancels only an in-flight run of the same event and ref, so a main
  push never cancels a dispatch and pull requests never cancel each other.
  There are no path filters or job-level `if:` conditions, so the checks
  required on `main` (`Backend Tests`, `Frontend Check`, `Integrated Build`;
  strict) always report. `scripts/test-ci-events.py` pins this policy and runs
  in the `Backend Tests` job. Evidence: `docs/reviews/ci-event-overlap-2026-10-23.md`.
- Browser journeys run in the `E2E Smoke` job groups of `ci.yml` in every CI run:
  `./scripts/test-e2e-smoke.sh`. Chromium and its apt packages install through
  `scripts/ci-install-chromium.sh` (also used by release preflight): each phase
  has a 180 s deadline and one retry. Before the package retry it stops the
  `apt-get` that `sudo` leaves running, waits for the dpkg lock and moves apt
  from the Azure mirror to `archive.ubuntu.com`. The step has a 15-minute
  backstop, so a stalled mirror costs one deadline rather than ten minutes
  (#415). Each app/audit job builds the
  single binary and boots a throwaway instance per worker (3 workers per job).
  Together they run every spec except `release-preflight.spec.ts`.
  The Playwright report, failed attempts' traces and screenshots
  (`e2e/test-results`) and each worker's `app.log` upload as an artifact on failure. The preflight suite
  stays out of push CI and is run deliberately before a release; it also runs
  weekly and via `workflow_dispatch` in `.github/workflows/release-preflight.yml`,
  because unrun it drifted for months behind a stale selector (#306).
  Bootstrap and observation-selection each have a dedicated Playwright project
  and self-managed worker book: the former needs no owner and the latter needs
  an otherwise empty built-in FX-pair history (#352). An explicit
  `E2E_BASE_URL` still uses the supplied external book; project separation does
  not reset or isolate that operator-selected database.

- Use `pnpm install --frozen-lockfile` in CI.
- Keep CI commands aligned with the local wrapper scripts in `scripts/`.
- Use Node 24.
- Use the Go version declared in `backend/go.mod`.
- Keep job structure simple and readable.
- Keep validation frequency explicit; measure job cost before changing event selection.

### Go caches and measurements

The push/PR CI uses `.github/actions/setup-go-cache` after checkout. It disables
setup-go's automatic cache, resolves `go env GOCACHE GOMODCACHE`, exports both
paths to subsequent steps and restores those exact directories. Playwright's
`pnpm build` inherits them without a private `/tmp` override. Local commands
likewise honor explicit overrides or Go's defaults; no CI-only build path is
required. Weekly/manual workflows retain setup-go's default caching and now
also share their effective path with E2E builds.

Each of `race`, `coverage`, `build`, `e2e-app` and `e2e-audits` owns a separate
primary key:

```text
go-v1-<native-config>-<go.sum>-<workload>-<source-inputs>
```

The native fingerprint covers the selected Go version, platform/architecture,
CGO/compiler/flags, runner image, OS and libc/compiler versions, including the
shared FTS5/netgo/osusergo tags. Restore falls back first to the same workload,
then another workload with the same dependencies, then older dependencies with
the same native configuration. Go validates individual artifact hashes, so
ordinary, race, coverage and embedded-web/trimpath artifacts can coexist. A
fallback is saved under the new workload/source key only after its checks pass.
Successful exact hits skip saving. Save and restore steps log actual keys,
paths, source commit, cache sizes and transfer durations; the job summary also
records the primary and matched restore keys.

The source digest covers backend, frontend, API, shared scripts, workspace
manifests/lockfile and CI cache configuration. `scripts/ci-go-cache.sh` takes it
from the git object IDs of those paths in the checked-out commit (#415), so it
is instant and never sees installs or generated output; `hashFiles` over the
same trees once exceeded Actions' fixed 120 s expression limit and failed the
job. A source-only change refreshes the key without changing `go.sum`;
unchanged source reruns reuse it. There are no timestamp or run-ID keys, except
that a run which cannot compute the fingerprint warns and uses a run-unique
source (`unavailable-<run>-<attempt>`): an exact miss that still restores by
prefix, never a failed job. One writer per
workload (including one per E2E suite), branch-scoped caches and the existing
per-ref workflow cancellation avoid competing job writers. Storage is bounded
by GitHub's configured repository cache quota and inactivity eviction; this
change grants no cache-deletion permission and does not retain a cache per run.
Monitor cache sizes/evictions when evaluating whether the workload copies fit
the repository quota. See [GitHub's cache reference](https://docs.github.com/en/actions/reference/workflows-and-actions/dependency-caching)
and [parallel-build guidance](https://github.com/actions/setup-go/blob/main/docs/advanced-usage.md#parallel-builds).

`GO_PHASE_TIMINGS=1` enables diagnostic phases in the existing wrappers
(Linux/GNU `/usr/bin/time`, as on CI):

```sh
GO_PHASE_TIMINGS=1 ./scripts/test-backend.sh
GO_PHASE_TIMINGS=1 COVERAGE=1 ./scripts/test-backend.sh
GO_PHASE_TIMINGS=1 pnpm build
GO_PHASE_TIMINGS=1 ./scripts/test-e2e-smoke.sh
python3 scripts/test-ci-go-cache.py
node --test e2e/global-setup.test.mjs
```

Module downloads are timed before Go compilation. Backend vet is timed
separately, then an instrumented compile/link/default-vet pass uses
`go test -exec /bin/true -count=1`: no TestMain or test body runs, and synthetic
successes are not cached as test results. The original complete race/coverage
command follows, with its two-package schedule and five-minute deadline intact.
Its time still includes Go driver/cached build/link overhead and may use valid
Go test-result cache hits; it is not pure test-body CPU time. The build wrapper
separates embed compilation/vet, the real embedded-output test and the trimpath
production build. Frontend compilation and browser execution remain separate
costs; the entire E2E setup duration is never treated as recoverable Go work.
Normal local commands omit the extra precompile passes unless opted in.

**Race-suite cost thresholds.** Thresholds are defined in the
[ADR 0016 amendment](adrs/0016-native-backend-test-resources.md#amendment--race-suite-cost-thresholds-2026-10-24),
and #402's [record](reviews/race-suite-cost-2026-10-24.md) has the baseline;
#404's [record](reviews/statement-compilation-2026-10-24.md) has the latest
matched runs.
Measure against them uncached, with matched runs:

```sh
# full gate: two uncached runs, interleaved with the base when comparing
GOFLAGS=-count=1 GO_PHASE_TIMINGS=1 ./scripts/test-backend.sh
# single package: three interleaved rotations, test binaries prebuilt
./scripts/go-backend.sh test -race -count=1 -exec /bin/true ./internal/app
./scripts/go-backend.sh test -race -count=1 -timeout=10m -json ./internal/app > run.json
python3 docs/reviews/fixtures/race-suite-cost-leaves.py run.json
```

Without `-count=1`, a rerun of unchanged packages reports `(cached)`, not a
time. Compare case-name digests as well as wall time. Attribute cost with a
CPU profile (`-cpuprofile`), not with test-name patterns.

For the hosted comparison, use one source/toolchain/runner image and fresh
runners: one cold run under the new namespace, then at least two exact-hit
warm runs. Record Actions run URLs, source SHA, configuration, keys, paths,
module/vet/compile/test timings, and cache restore/save duration/size from the
separate cache steps. Then change only backend source (leave `go.sum` fixed),
verify a fallback restore plus a new save key, and rerun that source to prove
its artifacts restore usefully. Retain all five logical CI gates (six jobs
with the browser matrix). Use logs to confirm every check passed; a local cache
copy is not fresh-runner Actions evidence. See the [#382 local validation](reviews/go-cache-2026-10-22.md)
and [hosted measurements](reviews/go-cache-hosted-2026-10-22.md), which separate
runner-image rollouts and cold workload variants from complete cache misses.

## Documentation Update Rules

- Update `docs/product-requirements.md` when scope or durable product behavior changes.
- Update `docs/conventions.md` when a repo-wide rule changes.
- Add or update an ADR in `docs/adrs/` when a decision locks a long-lived tradeoff.
- Update `README.md` and this document when developer commands, repo layout, or workflow expectations change. Avoid reintroducing area README files for short command notes; use them only when an area needs substantial local guidance.

## Archive Rules

- `.archive/` is reference only.
- Do not port archive implementation details directly.
- If an archive idea is adopted, rewrite it in current-stack terms and add it to active docs.

## Quote provider configuration (#117)

`COINGECKO_DEMO_API_KEY` enables the CoinGecko Demo quote adapter;
`TWELVE_DATA_API_KEY` enables Twelve Data quotes with the operator's own market
and display entitlements. Keys belong in deployment environment/secrets, never
in instrument identifiers. Yahoo Finance is keyless and explicitly unofficial.
Create/edit instruments and explicit provider identifiers under Investments →
Instruments, select a quote currency, save, and enable its provider assignment.
**Refresh quotes** queues durable work; source health and pricing refresh history
show completion. The saved daily pricing schedule also queues assigned quotes.
Twelve Data assignments with a symbol and MIC also feed **Check provider
dividends** on Investments → Suggestions (#111): it queues durable
`investment.dividend_events` work only when clicked (no schedule), needs a plan
whose entitlement includes `/dividends`, and reports failures in pricing failed
work. The e2e harness sets a placeholder `TWELVE_DATA_API_KEY` that only ever
reaches the `MARKET_DATA_BASE_URL` stub; that stub announces 0.50 per share
for `E2EDIV*` symbols and revises `E2EDIVREV*` ones to 0.55 from their second
fetch, so the revision disclosure (#254) has a browser case.
Each provider-dividend scenario captures one host-calendar reference and derives
its opened, acquisition, ex-date and payment dates from it. Before fetching,
`support/provider-dividends.ts` registers the explicit ex-date per symbol with
`PUT /__control/dividends/{symbol}`; unregistered symbols return 404. Registering
again resets only that symbol's revision sequence. The stub never chooses dates
from its own clock. `pnpm --dir e2e run test:stubs` checks exact independent dates
with frozen UTC, Amsterdam and Los Angeles clocks at midnight and DST boundaries
(#386). The three existing dividend browser journeys retain exact ex-date,
native posting, revision and payment-in-lieu linking assertions.
For provider evidence, failure recovery and the fixed crypto CSV columns, see the
[quote/crypto contract](plans/quote-providers-crypto-plan.md).
