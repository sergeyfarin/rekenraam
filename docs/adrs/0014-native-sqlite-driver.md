# ADR 0014: Native SQLite Driver

## Status

Accepted; implemented in #195 (2026-10-12); amended 2026-11-10 by #356 to
admit v1.14.53 / SQLite 3.54.1.

[ADR 0015](0015-sqlite-commit-durability.md) subsequently replaces the retained
WAL/NORMAL writer policy with FULL consistently. The original driver admission
and matched performance evidence remain historical results.

## Date

2026-10-12

## Context

The owner chose to keep SQLite and plan migration to a native SQLite driver
after comparing modernc, native SQLite and PostgreSQL. The decision serves
the current single-user, browser-based, self-hosted product. It does not
depend on shipping a desktop edition or introduce a hosted multi-user service.

The #164 measurements identify SQLite statement preparation, repeated schema
loading and race instrumentation of modernc's translated engine as major
development costs. The 2026-10-12 application race run took 953.627 seconds.
That run is validation evidence, not a controlled migration benchmark.

The [migration plan](../plans/native-sqlite-driver-migration-plan.md) defines
compatibility and performance evidence required for delivery. GitHub #164
owns the development-feedback outcome;
[#195](https://github.com/sergeyfarin/rekenraam/issues/195) owns the driver
delivery, and the roadmap owns work order.

## Decision

1. Keep SQLite and Go. Select `github.com/mattn/go-sqlite3`, using its bundled
   SQLite C source through CGO, as the replacement for `modernc.org/sqlite`.
   #195 admits v1.14.52 (SQLite 3.53.4) with `fts5,netgo,osusergo`.
   Pin the admitted driver version and record the SQLite engine version and
   compile options. Do not depend on an operator's system `libsqlite3`.
2. Use one driver in production, application tests, API tests, migrations,
   backup/restore and the integrated binary. A temporary comparison checkout
   may use the old driver; a permanent test-only driver or runtime driver
   selector is not the delivery shape.
3. Preserve the existing SQLite schema, coefficient/scale representation,
   operational elections, audit history and database-file workflows. The
   driver change is not grounds to rewrite the baseline, regenerate its seed
   or reset a development database. The owner confirmed no legacy databases
   or backups exist; retain current schema/seed and backup/restore regressions
   rather than adding an installed-data upgrade workflow.
4. Keep the single write connection and separate read-only pool, the current
   transaction begin policy, and all ADR 0004 PRAGMA values. Install required
   settings on every physical connection through supported DSN options and,
   where necessary, a connection hook. Continue validating effective state.
5. Keep online SQLite backup, verification, private file permissions and
   stopped-app restore. Adapt the native driver's resource lifetimes and
   completion semantics behind the existing repository boundary.
6. Build and test with CGO and consistent SQLite feature tags, including
   FTS5. The initial production container stays Debian trixie under ADR 0005;
   its builder and runtime must have compatible libc. Native compilation on
   a target runner is acceptable; cross-compilation is not a delivery gate.
7. A single app binary does not promise a libc-independent executable. State
   the supported runtime baseline and inspect the delivered artifact's
   dependencies. Fully static Linux linking is a separately measured build
   option, not a prerequisite for this driver migration.
8. Retain ordinary `go test -race` coverage of all Go packages and the Go
   driver wrapper. Go's detector does not instrument the native SQLite C
   engine as it instrumented modernc's translation. Document that boundary;
   do not describe it as complete engine race coverage or add compiler race
   exclusions. All financial and concurrency regressions remain.

This supersedes ADR 0004 only for the driver-specific binding and DSN/backup
API details. Its connection-state, migration, locking and recovery decisions
remain. ADR 0005's runtime base and one-process container decision remain.

## Consequences

The migration targets a measured development cost while retaining the current
deployment and financial model. Performance improvement remains to be
measured against an identical suite; it is not established by this ADR.

Building now needs a C toolchain. Reproducibility includes the C compiler,
linker, headers, libc baseline, build tags and bundled SQLite version as well
as Go, dependencies and source. First builds and dependency updates can incur
C compilation cost even if warm test execution improves substantially.

The native backup API differs from modernc's, and context cancellation,
result decoding, connection replacement and SQL error classification need
named compatibility tests. Sharing SQLite's file format does not alone prove
those behaviors.

Desktop remains deferred under the existing product requirements. PostgreSQL,
dual database support, financial schema redesign, weaker durability settings,
and changes to local-versus-CI validation frequency are separate decisions.

## Amendment — SQLite 3.54.1, 2026-11-10

[#356](https://github.com/sergeyfarin/rekenraam/issues/356) admits
`github.com/mattn/go-sqlite3` v1.14.53, bundling SQLite 3.54.1 (source ID
`2026-10-16 14:45:20 555e07ef33f69a0957d24d8e555b702464a131d6f1607f4c3c1e4551e8e6ca76`),
under this ADR's existing driver, bundled-source and feature decisions. The
[admission review](../reviews/sqlite-driver-admission-2026-11-10.md) records
source authentication, the retained correctness probes, full gates and the
measured statement-re-preparation cost. The compile options add only
`MAX_SCHEMA=10000000`. WAL/FULL, the disabled statement cache, the single
writer/read pool and the #403 FTS5 workaround remain unchanged.

## References

- [#164 measurements and proposal](https://github.com/sergeyfarin/rekenraam/issues/164#issuecomment-6109667983).
- [Driver v1.14.53 documentation](https://github.com/mattn/go-sqlite3/blob/v1.14.53/README.md).
- [Native backup implementation](https://github.com/mattn/go-sqlite3/blob/v1.14.53/backup.go).
- [SQLite 3.54.1 admission review](../reviews/sqlite-driver-admission-2026-11-10.md).
- [ADR 0004](0004-sqlite-connection-migrations-and-backup.md),
  [ADR 0005](0005-docker-runtime-base-image.md),
  [ADR 0013](0013-investment-operations-and-position-sides.md).
