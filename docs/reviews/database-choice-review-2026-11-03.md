# Database choice review — 2026-11-03

**Status:** Analysis only. This document revalidates SQLite; it does not propose
a database migration and does not change an ADR, requirement or roadmap item.
The accepted decisions remain [ADR 0004](../adrs/0004-sqlite-connection-migrations-and-backup.md),
[ADR 0014](../adrs/0014-native-sqlite-driver.md) and
[ADR 0015](../adrs/0015-sqlite-commit-durability.md).

## Question

Rekenraam stores its books in one local SQLite file. Is SQLite still the right
database for a single-user, self-hosted finance app, and what would PostgreSQL,
MySQL/MariaDB, DuckDB, libSQL/Turso, a distributed SQL database, a document
database or an embedded key-value store add or take away?

This review compares the **from-scratch fit** of each option with the **cost of
transitioning from the current SQLite implementation**. Those are separate
questions. A database can be excellent in general and still be the wrong
migration for this product today.

## Answer

**Keep SQLite.** It is not a compromise for this product; it is a direct fit for
the deployment model. SQLite provides ACID transactions, foreign keys, CHECK
constraints, triggers, views, FTS5, JSON and a mature online-backup API in a
library that needs no server process, network port, credentials or separate
backup agent. The app already uses those properties for the ledger, audit
history, reconciliation guards and exact-decimal storage.

The measured workload is also far below SQLite's practical ceiling. A synthetic
250,000-posting book exports and self-checks in about 11 seconds each on this
host, with peak live heaps of 12–41 MiB for those operations. The database is
the dominant backend cost, but that is a reason to keep optimizing SQL and
schema, not a reason to add a database server.

PostgreSQL becomes the stronger choice when the product needs concurrent
writers, multi-user or multi-tenant isolation, network clients, replication/HA,
roles/row-level security or Postgres-specific extensions. Those are future
product directions, not current requirements. Adopting PostgreSQL now would add
an operations and security surface that the self-hosted single-binary product
deliberately avoids.

## What would and would not change

| Concern | A database switch would change it | Current evidence |
|---|---|---|
| Deployment shape | PostgreSQL/MySQL need a server, network, credentials and lifecycle management | SQLite is embedded in the same ~33 MiB binary; warm startup 76 ms, idle RSS 23–28 MiB |
| Backup and restore | Server dump/basebackup/WAL policy instead of one file and SQLite's online backup API | Current product backs up/verifies the SQLite file and restores it with the app stopped |
| Concurrency | SQLite has one writer; PostgreSQL/MySQL support concurrent writers and richer isolation | Current scope is one owner and one app process; workers already serialize writes |
| Large reads/reports | A server may parallelize more work across connections/nodes | 250k-posting export 11.1 s and self-check 10.9 s; both SQLite-bound |
| Desktop | SQLite is a natural local-file database; a server database conflicts with local desktop ownership | Desktop review assumes local SQLite and no separate service |
| Hosted SaaS | SQLite can serve single-tenant instances; multi-tenant SaaS usually wants PostgreSQL | Current product is self-hosted and single-tenant by design |
| Exact decimals and constraints | Schema, triggers, CHECKs and tests would be ported and re-proved | 118 tables, 168 triggers, 106 indexes, 16 views, about 214 KiB of schema SQL |
| Durability | PostgreSQL WAL/fsync is stronger operationally at scale; SQLite FULL is already durable locally | WAL + `synchronous=FULL`, one writer, busy timeout 5000 ms, foreign keys on |
| Operations | Server upgrades, tuning, monitoring, connection pooling, TLS, auth and backups become user/operator work | SQLite needs none of that; the app owns migrations and backups |
| Testing | Server-backed tests need a service or container; SQLite uses the same engine in tests | Runtime and tests share one bundled native SQLite engine |

## Criteria

Ratings describe this product, not a generic benchmark.

| Criterion | SQLite (current) | PostgreSQL | MySQL/MariaDB | DuckDB | libSQL/Turso | Distributed SQL | MongoDB | Embedded KV |
|---|---|---|---|---|---|---|---|---|
| ACID ledger transactions | Strong | Strong | Strong | Adequate for analytics, not OLTP-first | Strong (SQLite lineage) | Strong | Adequate, weaker relational constraints | Weak for multi-row invariants |
| Relational constraints/triggers | Strong | Strongest | Strong, dialect differences | Limited | Strong (SQLite lineage) | Strong | Weak/no SQL constraints | None |
| Single-writer concurrency | Adequate for one owner | Strong | Strong | Single-process analytics | Strong local, replication adds concerns | Strong | Strong | Strong per key |
| Multi-user / multi-tenant | Weak without app-level isolation | Strongest | Strong | Weak | Emerging | Strongest | Strong | Weak |
| Embedded/no-server deployment | Strongest | Weak | Weak | Strong | Strong local / hosted service | Weak | Weak | Strongest |
| Backup/restore simplicity | Strongest | Moderate | Moderate | Strong | Moderate | Complex | Moderate | Moderate |
| HA/replication | Weak | Strong | Strong | Weak | Emerging | Strongest | Strong | Varies |
| Analytics/OLAP | Adequate | Strong | Adequate | Strongest | Adequate | Strong | Adequate | Weak |
| Desktop/local-first | Strongest | Weak | Weak | Strong for analysis | Strong | Weak | Weak | Strong |
| Go ecosystem | Strong, native driver admitted | Strong | Strong | Adequate | Emerging | Adequate | Strong | Strong |
| Operational burden for this product | Lowest | High | High | Low | Medium | Highest | High | Low |

## SQLite as-is

**What it brings**

- A real transactional database in one file, with no server process. The Go
  binary embeds the native engine and the static frontend; Docker runs one app
  process, and a home-lab or desktop user has no database service to install,
  expose, upgrade or monitor.
- Strong local correctness tools: foreign keys, CHECK constraints, triggers,
  views, FTS5 and JSON. The financial invariants are not only application code;
  the schema carries guards that survive service-layer mistakes.
- WAL with `synchronous=FULL` for acknowledged commit durability, a single
  writer, a bounded read-only pool, busy timeout and explicit checkpoint policy.
  These are deliberate settings, not SQLite defaults.
- SQLite's online backup API and `VACUUM INTO` fit the product's backup/restore
  feature. A backup is a verified file, not a dump from a running service.
- The same engine runs in production, tests, migrations, backup/restore and the
  integrated binary. Tests do not need a database container, and a failing
  migration is exercised against the same SQLite version users run.
- A local file is a natural fit for the optional desktop host and for a
  single-tenant hosted instance. It also keeps the door open to a future
  export/import migration without making a server database mandatory now.

**What it costs**

- One writer. Concurrent write-heavy workloads queue; long reads can still
  compete for the write connection unless the read pool is used carefully.
- No built-in network protocol, roles/row-level security, replication or
  automatic failover. Those are application or deployment concerns if the
  product ever becomes multi-user or hosted.
- SQLite's type system is flexible, so exact decimal storage, canonical strings
  and CHECK constraints must be explicit. The repo already does this.
- Very large analytical scans are not its strongest workload. The current
  250k-posting book is still comfortable, and reports already fold in Go.
- Backup safety requires SQLite-aware handling while the app is running. The
  repo uses the online backup API and a stopped-app restore path rather than
  copying a live database file.
- The schema is large enough that connection/schema load and statement
  preparation are measurable test costs. That is a schema/connection-policy
  concern, not evidence that another database would make the ledger faster.

**Transition cost from here:** None; this is the current database.

## PostgreSQL

**What it would bring from scratch**

- The strongest general-purpose relational platform for a multi-user or hosted
  finance product: MVCC, concurrent writers, rich isolation, roles and
  row-level security, extensions, JSONB, full-text search, logical replication,
  streaming replication and mature operational tooling.
- A natural fit for a multi-tenant SaaS, a team/household deployment with many
  concurrent writers, or a service that needs network clients and HA.
- A large Go ecosystem and managed offerings reduce some operational work for a
  hosted product.

**What it would cost from scratch**

- A server process, network listener, credentials, TLS, connection pooling,
  tuning, upgrades, monitoring and a separate backup/restore policy.
- More deployment complexity for the self-hosted user: Compose now needs a
  database service, persistent volume, health checks and an initialization
  contract. The single-binary simplicity is lost.
- More test infrastructure: a database service or container, migrations that
  must be tested against the server, and a second set of operational failure
  modes.
- SQL dialect and behavior differences: trigger/function syntax, sequences,
  type affinities, `RETURNING`, FTS, `ON CONFLICT`, JSON operators, locking and
  transaction semantics. The current SQLite triggers/views would need a
  deliberate port.

**Transition cost from here:** High. It would touch the migration chain, every
repository and raw SQL path, backup/restore, test fixtures, startup/lifecycle,
Docker Compose and deployment documentation. The exact-decimal/constraint
behavior and reconciliation guards would need a full conformance pass. It is
the right migration if the product becomes multi-user, multi-tenant or
HA-hosted; it is not justified by the current single-user workload.

## MySQL / MariaDB

**What they would bring from scratch**

- A mature server database with broad hosting support, replication and a large
  ecosystem.
- Familiar operations for many teams and managed providers.

**What they would cost from scratch**

- The same server/network/backup/operations burden as PostgreSQL.
- No clear advantage over PostgreSQL for this product's relational, JSON,
  constraint and extension needs; there are dialect and trigger/CHECK
  differences to learn.
- MariaDB/MySQL licensing and feature differences add a decision without
  improving the current workload.

**Transition cost from here:** High, comparable to PostgreSQL, with less
strategic benefit for a product that would otherwise choose PostgreSQL.

## DuckDB

**What it would bring from scratch**

- An embedded columnar engine with excellent analytical scan performance and a
  simple file-based deployment.
- A good fit for ad-hoc analysis, reporting snapshots or a read-only analytics
  sidecar.

**What it would cost from scratch**

- It is optimized for analytical workloads, not the small, frequent,
  transactionally guarded writes of a ledger.
- Concurrency and multi-writer behavior are not a substitute for a server
  database, and the SQL/constraint/trigger ecosystem is different.
- Making it the primary store would require re-proving the financial write path
  and backup/restore model.

**Transition cost from here:** High for a primary database. Low only for a
future read-only analytics sidecar built from exports, which is not current
scope.

## libSQL / Turso

**What they would bring from scratch**

- SQLite-compatible SQL with an optional hosted/replicated service and
  edge-friendly access.
- A possible path to desktop/server sync or a managed SQLite offering without
  abandoning the SQLite data model.

**What they would cost from scratch**

- A hosted provider or replication topology becomes part of the trust and
  operations boundary. Credentials, network availability, data residency and
  provider continuity matter.
- It diverges from the admitted bundled native SQLite engine and would need its
  own driver, backup, migration and compatibility testing.
- Sync/conflict semantics are a product design problem, not a drop-in database
  swap.

**Transition cost from here:** Medium–high. The SQL is close, but the driver,
replication, hosting, backup and test story would be new. Watch it if
desktop/server sync becomes a real requirement; do not adopt it for the current
single-user local app.

## Distributed SQL (CockroachDB, YugabyteDB and similar)

**What they would bring from scratch**

- Strong consistency across nodes, replication, failover and horizontal scale.
- A fit for a multi-region hosted service with strict availability needs.

**What they would cost from scratch**

- Significant operational complexity, latency and cost for a single-user local
  finance app.
- A distributed cluster is the opposite of the current one-file, one-process
  deployment promise.

**Transition cost from here:** Very high, with no current requirement that
justifies it.

## MongoDB and document databases

**What they would bring from scratch**

- Flexible document schemas and easy horizontal distribution for some workloads.

**What they would cost from scratch**

- The ledger is inherently relational: balanced postings, account trees, lots,
  reconciliation checkpoints and cross-row invariants. Document stores make
  those constraints application-level concerns.
- Reporting and audit queries are SQL-shaped; transactions across documents are
  possible but do not restore the relational guarantees this product relies on.
- A document model would be a financial-model rewrite, not a database swap.

**Transition cost from here:** Very high and strategically wrong for a
double-entry ledger.

## Embedded key-value stores (BoltDB, Badger and similar)

**What they would bring from scratch**

- Very simple embedded storage and fast key lookups.
- No server and a small dependency surface.

**What they would cost from scratch**

- No SQL, joins, constraints, triggers, views, FTS or ad-hoc reporting. Every
  index, invariant and migration would become application code.
- Backup/restore and query tooling would be bespoke.
- The current product's most valuable correctness guarantees live in the
  relational schema; discarding them is a large regression in trust.

**Transition cost from here:** Very high and not recommended.

## Current measurements

Measured 2026-11-03 at `bc9a2bfb` (latest `main` at the time) on the six-vCPU
local host, using the same native SQLite configuration as production. The
[evidence JSON](fixtures/database-deployment-profiles-2026-11-03.json) retains
the large-book benchmark, commit-probe, schema and startup results:

| Measurement | Result |
|---|---:|
| 250k-posting bundle export | 11.113 s |
| 250k-posting self-check | 10.918 s |
| Attention read | 2.41 s |
| Home briefing | 3.791 s |
| Bundle export peak live heap | 12 MiB |
| Self-check peak live heap | 41 MiB |
| Attention peak live heap | 3 MiB |
| Home peak live heap | 117 MiB |
| `BenchmarkLargeBookBundleExport` | 10.59 s/op, 454 MB/op, 14.7M allocs/op |
| `BenchmarkLargeBookSelfCheck` | 10.64 s/op, 144 MB/op, 6.73M allocs/op |
| Warm app startup on the migrated file | 76 ms, 23 MiB RSS |
| Cold startup including migrations | 269 ms, 28 MiB RSS |
| Empty migrated database | 1.58 MiB |
| Schema | 118 tables, 106 indexes, 168 triggers, 16 views, ~214 KiB SQL |

The startup figures used the existing `da2563b4` binary; later `main` commits
only patch frontend dependencies. This fresh large-book repeat is slightly
faster than the earlier performance-profile run (12.9 s export / 12.8 s
self-check at `da2563b4`); shared-host variance is expected. The earlier
application and large-book profiles attribute the backend's time to SQLite
itself: 58.7% cgo in the application test profile, 86.6% cgo in the large-book
profile, with statement preparation and connection/schema loading as the
largest named subsets. That is the database doing the work; it is not
Go overhead, and it does not disappear by replacing SQLite with PostgreSQL. A
PostgreSQL deployment would move the same logical work to a server plus a
network protocol, with different concurrency and operational trade-offs.

A separate 500-transaction commit probe compared the admitted FULL policy with
the diagnostic NORMAL mode on the same native engine:

| Mode | Median transaction | p95 | Max |
|---|---:|---:|---:|
| FULL (shipped) | 4.45 ms | 5.21–74.50 ms | 74.4–79.3 ms |
| NORMAL (diagnostic only) | 0.073–0.078 ms | 0.109–0.113 ms | 13.1–14.5 ms |

FULL is the accepted financial-durability choice. The probe includes
preparation, writes and commit; it is not a complete application workflow or a
production latency guarantee. Its outlier p95 shows why the result is reported
as a range rather than a single promise.

No live PostgreSQL/MySQL benchmark was run. A meaningful comparison would need
the full schema, repository workload and deployment shape ported, and a generic
`pgbench` number would not describe this ledger. The comparison above therefore
separates measured SQLite behavior from the architectural and transition costs
of a server database.

## Transition cost from the current SQLite app

| Option | Transition from here | Main risks |
|---|---|---|
| SQLite | None | None |
| PostgreSQL | High | Full schema/SQL/trigger port, repository and test rewrite, backup/restore, Compose and operator contract |
| MySQL / MariaDB | High | Same server migration plus dialect/feature differences, with less strategic benefit |
| DuckDB | High as primary | Re-prove write path, constraints, concurrency and backups; better as a read-only sidecar |
| libSQL / Turso | Medium–high | New driver/provider, replication/sync semantics, backup and compatibility evidence |
| Distributed SQL | Very high | Cluster operations, latency/cost, complete deployment rewrite |
| MongoDB | Very high | Financial model and reporting rewrite; weaker relational guarantees |
| Embedded KV | Very high | Rebuild SQL, constraints, indexes, reporting and migrations in application code |

## Revisit triggers

Keep SQLite until one of these is a real product requirement, not a hypothetical
scale concern:

1. **Multi-user/household or multi-tenant SaaS** with concurrent writers,
   tenant isolation, roles or row-level security.
2. **Hosted/HA requirements**: replication, failover, point-in-time recovery,
   read replicas or network clients.
3. **A measured workload beyond the single writer**: sustained concurrent write
   contention or a dataset/reporting pattern that SQLite cannot serve within
   the product's latency bounds after query/schema optimization.
4. **A Postgres-only capability** that is central to a product feature:
   extensions, logical replication, advanced JSON/query features or mature
   operational tooling.
5. **Desktop/server sync or multi-device collaboration**, which may favor
   libSQL/Turso or a server database depending on the sync model.
6. **A deployment/business decision** to run a first-party hosted service;
   PostgreSQL then becomes part of that separate product architecture.

Do not migrate merely because PostgreSQL is more familiar, more "standard" for
web apps, or theoretically scales further. Those are from-scratch arguments;
they do not answer the current transition-cost question.

## If a migration is ever justified

- Keep the repository boundary and avoid SQLite-only SQL in ordinary queries,
  as the conventions already require. Do not weaken constraints/triggers to
  make a future port easier.
- Add a new ADR that names the triggering requirement, the target database, the
  backup/restore contract and the supported deployment shapes.
- Port the schema in additive steps with conformance tests for exact decimals,
  triggers, reconciliation guards, audit history and every report.
- Migrate through export/restore with verified trial balances, not by a one-off
  copy of the SQLite file into a server database.
- Do not run two production database dialects indefinitely. A temporary
  comparison branch is evidence; dual production dialects are a maintenance
  and correctness liability.

## Reproduction

```sh
# Large-book runtime and benchmarks
BENCH=1 ./scripts/measure-large-book.sh

# FULL versus diagnostic NORMAL commit cost
TMPDIR=.tmp/go-tmp ./scripts/go-backend.sh run \
  ../docs/reviews/sqlite-full-commit-probe.go

# Migrated-file schema size and pragmas
python3 - <<'PY'
import os, sqlite3
p = '.tmp/deploy-profile/rekenraam.sqlite'
con = sqlite3.connect(p)
cur = con.cursor()
print(cur.execute('pragma page_size').fetchone())
print(cur.execute('pragma page_count').fetchone())
print(cur.execute("select type, count(*) from sqlite_master where name not like 'sqlite_%' group by type").fetchall())
print(os.path.getsize(p))
PY
```

The large-book script uses a temporary generated book. The commit probe uses a
small synthetic schema and a separate diagnostic connection to enable NORMAL;
the shipped app remains WAL/FULL. Raw profiles and temporary databases are not
committed.

## References

- [Database and deployment profile evidence — 2026-11-03](fixtures/database-deployment-profiles-2026-11-03.json).
- [Early Architecture Decisions](../early-architecture-decisions.md#database-and-migrations).
- [ADR 0004: SQLite Connection, Migrations, And Backup](../adrs/0004-sqlite-connection-migrations-and-backup.md).
- [ADR 0014: Native SQLite Driver](../adrs/0014-native-sqlite-driver.md).
- [ADR 0015: SQLite Commit Durability](../adrs/0015-sqlite-commit-durability.md).
- [SQLite FULL durability and settings review](sqlite-full-durability-2026-10-12.md).
- [CI and test performance profile — 2026-11-03](ci-test-performance-profile-2026-11-03.md).
- [Backend language choice review — 2026-11-03](language-choice-review-2026-11-03.md).
