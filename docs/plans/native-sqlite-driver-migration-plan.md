# Native SQLite driver migration plan

**Status:** delivered and validated, including matched performance (2026-10-12).
See the [delivery review](../reviews/native-sqlite-driver-2026-10-12.md) for
the admitted pin/features, validation and measurements.
The original driver comparison retained WAL/NORMAL. The subsequent owner
decision in [ADR 0015](../adrs/0015-sqlite-commit-durability.md) requires FULL
consistently; its [review](../reviews/sqlite-full-durability-2026-10-12.md)
records the current policy and fresh validation.
The owner's subsequent test-limit clarification is delivered through
[ADR 0016](../adrs/0016-native-backend-test-resources.md): two concurrent
packages, five-minute deadlines and available Go CPU resources. The original
driver and FULL comparisons below retained their historical 20-minute bound.
Governed by [ADR 0014](../adrs/0014-native-sqlite-driver.md), ADRs 0004/0005/0013,
and the financial requirements. The [roadmap](../roadmap.md) owns execution
order; [#195](https://github.com/sergeyfarin/rekenraam/issues/195) owns the driver
delivery checklist and #164 owns the performance outcome.

## Outcome and scope

Replace modernc with bundled native SQLite through `github.com/mattn/go-sqlite3`
throughout the existing Go application. Preserve financial behavior and make
the development-feedback improvement measurable. Keep the browser-based,
single-user, one-app-process deployment with a local SQLite file.

This is a driver migration, not a data-model conversion. Preserve schema
checksums, seed data, exact amounts, NULL knowledge pairs, correction chains,
replay elections and provenance. PostgreSQL, desktop packaging, Rust, a generic
database abstraction and a permanent dual-driver test matrix are outside this
delivery. #189 and #192 keep their independent original requirements; the
native driver does not fix the embedded-NUL CHECK gap.

## Design contracts

### Dependency and feature selection

Use the driver's bundled amalgamation, with an exact Go module pin. The
current admitted version is v1.14.53, bundling SQLite 3.54.1, with
`fts5,netgo,osusergo` (#356; the owner accepted its performance cost on
2026-11-11). The
implementation stages below define its admission. Record `sqlite_version()`, `sqlite_source_id()` and
`PRAGMA compile_options` for both drivers. If their engine versions differ,
identify the engine update rather than attributing all differences to CGO.

Enable FTS5 consistently in tests, development, build, Docker and seed tooling.
Retain the schema's required JSON functions and current SQL features. Avoid
system-library linkage (`libsqlite3` tag). Evaluate whether `netgo` and
`osusergo` are needed to preserve the resolver/user-lookup behavior of the
current CGO-disabled artifact. Pin the final feature set in shared commands.

Keep the native statement cache disabled for the first comparison (the
candidate defaults to zero). Cache tuning, explicit statement reuse and
fixture-pool reuse are separately measured changes, so their gains are not
confused with the engine switch. #404 measured the first two: explicit reuse in
hot loops shipped, and the cache stays at zero
([record](../reviews/statement-compilation-2026-10-24.md)).

### Connections, transactions and values

`internal/db/sqlite.go` remains the owner of opening, initializing and checking
connections. Adapt the driver registration/name and URL construction centrally.
Normalize reserved options so an existing query parameter cannot disable a
required setting. Preserve compatible caller URI options; explicitly translate
or refuse incompatible modernc-specific options rather than silently ignoring
them. Keep goose's SQLite dialect and separate migration admission path.
Use supported native DSN options for foreign keys, busy timeout, WAL and
synchronous state; use a connection hook for settings without an equivalent
option, notably `wal_autocheckpoint`. Apply initialization to replacement
connections too, not just the first pool connection.

| Contract | Required behavior |
| --- | --- |
| Write pool | One connection; foreign keys on; busy timeout 5000 ms; WAL; synchronous FULL (ADR 0015 follow-up); autocheckpoint 1000 pages |
| Read pool | Separate read-only connections; maximum four/idle two; foreign keys and busy timeout installed; FULL synchronous mode; writes refused |
| Transactions | Current begin/commit/rollback and savepoint behavior; no incidental switch to immediate/exclusive BEGIN |
| Snapshot | Self-check reads one consistent state while the separate writer persists results |
| Values | Canonical TEXT coefficients and exact scales; signed 38-digit values survive; unknown stays paired NULL; known zero stays numeric |
| Dates | Preserve date/timestamp strings and existing scan behavior; no accidental timezone conversion |
| Errors | Preserve application/API conflict and resource-busy contracts; classify native error codes without depending on driver error wording |
| Paths | Preserve URI options, read-only modes and filesystem paths, including spaces and URI punctuation; #128 remains separately tracked |

Cover binding and scans in both directions, including TEXT/BLOB/NULL, custom
`driver.Valuer`/`sql.Scanner` implementations, `LastInsertId`, affected rows,
context deadlines and cancellation. A cancelled query must not interrupt the
next healthy operation on a reused connection.

### Online backup and restore

`internal/db/backup_online.go` formerly depended on modernc's `NewBackup`.
The native API starts the copy from the destination connection and requires
both source and destination raw connections. Keep both acquired connections
and raw lifetimes valid until completion, and finish before either is released.
Keep this adaptation local to the repository; do not pass native handles into
application services or introduce a multi-driver framework.

The candidate's `Step` returns **done**, while current code uses **more**.
Account for that inversion explicitly. The native API can return not-done
with no error on BUSY/LOCKED; use a bounded, context-aware pause/progress policy
so contention cannot create a tight spin or an unbounded copy. Handle Finish
once, including error/cancellation cleanup, and read metadata while valid.

Retain copying uncheckpointed WAL content, bounded page steps and deadline,
private directory/file permissions, explicit FULL synchronous mode on auxiliary
connections, partial-artifact cleanup, consolidation
into one verified backup file, and safety copies on restore. Keep `VACUUM INTO`
as the separate operator path. The owner confirmed on 2026-10-12 that no legacy
databases or backups exist. Do not add historical binary fixtures or an operator
data-upgrade workflow for this cutover; retain the current schema/seed and
backup/restore regressions.

### Builds and race coverage

Update `scripts/build-single-binary.sh`, `scripts/test-backend.sh`, seed/build
helpers, package commands, CI and `deploy/docker/Dockerfile` together with the
cutover. Tests must compile the same SQLite features as production. Fail early
with actionable guidance when CGO or the C compiler is unavailable.

The Docker builder/runtime remain on the current Debian generation; the
runtime needs the executable's declared shared libraries, not a compiler or
an independently versioned SQLite library. Validate the actual final image,
including HTTPS provider access and embedded frontend serving. The initial
measured platform is the current Linux/amd64 workspace; prove every additional
architecture for which an artifact is actually claimed, using target runners
or explicit target toolchains. This does not create new OS-support promises.

Keep `go test -race` for every Go package and preserve all named tests and
subtests. Do not add modernc instrumentation exclusions, drop the 260-hop
regression, or raise the 20-minute package deadline. Record the C-engine
coverage boundary and verify a deliberate Go race is still detected in a
throwaway probe; do not retain a deliberately failing test in the normal suite.

Two release builds with identical pinned Go/C toolchain inputs and source
must have matching hashes, or the remaining source of nondeterminism must be
resolved before claiming reproducibility. Inspect dependencies on the actual
binary; do not keep the current promise that CGO-disabled static linkage
describes a native-driver build. Fully static linking is separate work.

## Implementation stages

These stages describe how to deliver this change, not roadmap work order.

| Stage | Concrete result and evidence |
| --- | --- |
| Baseline | Record revision, module verification, engine fingerprints, C/Go toolchain, CPU budget, test names/counts and uncached ordinary/race timings. No installed legacy databases/backups exist; retain the existing SQL seed and regression suite. |
| Compatibility spike | Admit the pinned native driver in an isolated checkout against current migrations, exact-value scans, connection replacement, FTS search, cancellation and online backup/restore. Build and boot the integrated artifact. |
| Atomic cutover | Switch every opening/build/test path; remove modernc-specific types and dependencies; synchronize commands, CI, developer requirements and task guides. Each committed implementation slice stays runnable. |
| Data and behavior proof | Run unchanged service/API/financial suites, fresh/seeded migration equivalence, schema checksum, restore and runtime lifecycle checks. Preserve exact values and audit evidence through the existing financial and restore cases. |
| Performance proof | Compare identical workloads and CPU resources, then measure fixture-pool reuse or scheduling separately under #164. Record full-gate wall time and per-package/test time; distinguish compilation from execution. |
| Delivery record | Update the capability ledger, replace pending workflow notes with actual commands/prerequisites, and post commits, validation, performance and retained limits on the delivery issue and #164. |

Do not commit an intermediate driver switch that breaks integrated builds or
silently tests a different driver. If compatibility fails, fix or document the
specific blocker within the migration issue; do not reset away the evidence.

## Validation and performance evidence

Feature-scoped tests first, including the existing named contracts:

- `TestOpenAppliesRequiredPragmas`, `TestOpenReadOnlyRefusesWrites` and
  `TestReadOnlySnapshotDoesNotSeeLaterWrites`, plus forced connection replacement.
- `TestMigrateSeededBaselineMatchesFreshSchema` and
  `TestPinnedMigrationChecksums`, without regenerating the baseline/seed.
- `TestRestoreDrillMatchesSourceTrialBalance`,
  `TestRestorePreservesUncheckpointedWALContent` and
  `TestSealedDataDecryptsAfterRestoreWithRetainedKey`.
- Named backup completion/cancellation/contention and subsequent-connection
  reuse cases; FTS insert/search/correction; URI and exact-value scan cases.
- `TestTransferChainDeeperThanFormerRoundLimitPropagatesInOnePass`,
  `TestFinancial*`, `TestOracle*`, investment capability/export and self-check
  suites, and background-work concurrency cases.

Final validation uses the shared wrappers: `./scripts/test-backend.sh`, the
ordinary coverage pass, `pnpm build`, and `./scripts/test-e2e-smoke.sh`, followed
by a final-container smoke using an isolated disposable volume. Run frontend
checks if shared package/build commands or generated contracts change.

Controlled timing runs bypass test-result caching, preserve all tests and
their inputs, use the same toolchain and fixed CPU budget, and report at least
three runs per compared driver for the main performance claim. Report medians
and ranges; record warm and cold build costs separately. Benchmark commands
are diagnostic, not a replacement for the shared correctness gate. Keep driver
choice, statement cache, fixture reuse and scheduling as separate experiments.

#164's required headroom is at least 30% under the current 1200-second package
bound: application race execution must be at most 840 seconds under the
declared measurement conditions. Target at least a 50% reduction in median
full-race gate wall time against the matched baseline; this is a planning
target, not a measured result or a relaxation of any assertion. Report focused
development-feedback time too, so passing a generous timeout is not the only
success metric. #164 remains open until its acceptance is demonstrated; a
functionally complete driver migration alone does not close it.

## Compatibility and recovery disposition

The driver cutover does not require a schema migration or checksum change.
If a discovered difference requires one, handle it explicitly under the
governing migration policy rather than silently bundling a schema redesign.

Keep the baseline revision for measured comparison. No legacy databases or
backups exist (owner confirmation, 2026-10-12), so an installed-data migration
and frozen old-driver binary fixtures are not delivery requirements. Current
schema/seed, recovery behavior and future backup guarantees remain covered.

Per-fixture read-pool reuse, native statement caching, CI/process sharding and
changes to how often the full local race gate runs are follow-up optimizations
for separately scoped issues; #164's original headroom acceptance is met by
the delivered migration. Native desktop packaging and
PostgreSQL remain outside this migration.
