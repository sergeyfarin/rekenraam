# SQLite 3.54.1 speed-up — query planner stability guarantee — 2026-11-11

**Proposal: build the bundled engine with `SQLITE_ENABLE_QPSG`.** The owner
accepted the go-sqlite3 v1.14.53 / SQLite 3.54.1 performance cost on
2026-11-11 ([admission review](sqlite-driver-admission-2026-11-10.md#owner-acceptance-2026-11-11))
and asked for a follow-up that finds ways to recover the speed. This review
identifies the cause of the 3.54.1 cost, measures one mitigation and records
its plan and correctness evidence. Merging the change is the owner's
acceptance of the ADR 0014 build amendment below.

## Cause

SQLite can mark a prepared statement for re-preparation when the value bound
to a parameter might change the best plan. The first `sqlite3_step()` after
such a bind recompiles the statement with the bound values visible
(`sqlite3VdbeReprepareOnBind` in 3.54.1; `sqlite3VdbeSetVarmask` before).

In 3.54.1, a bound equality on a joined column (for example
`WHERE a.id = ?` with `JOIN account_versions av ON av.account_id = a.id`)
sets that mark. The application prepares a fresh statement for every query
(`_stmt_cache_size=0`), so every such query compiles **twice**. A
statement-level probe against the migrated application schema counted
compiles with the native authorizer:

| Query shape | 3.53.4 | 3.54.1 | 3.54.1 + QPSG |
|---|---:|---:|---:|
| `accounts` ⋈ versions ⋈ kinds, `book_id = ? AND id IN (?, ?)` | 1 | **2** | 1 |
| same, `book_id = 1` literal | 1 | 1 | 1 |
| same, `book_id = +?` | 1 | 1 | 1 |
| `accounts` alone, `book_id = ? AND id IN (?, ?)` | 1 | 1 | 1 |
| `accounts` ⋈ `account_versions`, `id = ?` | 1 | **2** | 1 |

Compiles were counted as native-authorizer `SQLITE_SELECT` callbacks per
execution, divided by the statement's `SELECT` count. The cost is the engine's second
compilation, which explains why it scaled with statement complexity in the
admission review rather than with rows returned.

## Options considered

1. **Build with `SQLITE_ENABLE_QPSG` (chosen).** The query planner stability
   guarantee turns off every re-prepare-on-bind path in the engine. In 3.54.1
   these are: STAT4 samples (not compiled in), `LIMIT`/`OFFSET` values,
   partial-index and expression comparisons against bound values, and the
   `LIKE`/`GLOB` prefix optimisation for a bound pattern. One flag in
   `scripts/native-sqlite-env.sh` reaches every supported build: tests,
   development, Docker, the single binary, release and the CI cache key.
2. **Rewrite bound equalities as `= +?`.** This also avoids the second
   compile, but it would touch hundreds of statements and every new query
   would need the same idiom. Rejected as brittle.
3. **Native statement cache or explicit reuse.** Reuse does not help: a new
   value bound to a marked parameter still expires the statement. The owner
   already declined the global cache (#404).
4. **Return to 3.53.4.** Rejected; #356 requires 3.54.1's two correctness
   fixes.

## What QPSG can change

The application never runs `ANALYZE`, so plans depend only on the schema and
the SQL. Of the four affected mechanisms:

- **STAT4** is not compiled in.
- **`LIKE ?`**: every application pattern is `'%' + term + '%'`, which never
  qualifies for the prefix optimisation.
- **Partial indexes**: 23 exist. Only conditions with constants
  (`= 1`, `= 'active'`, `IN ('pending','running')`, `<> ''`) could depend on a
  bound value; the plan scan below covers every statement that reads them.
- **`LIMIT ?`**: the planner no longer sees the page size.

### Plan scan

Every statement executed by the `app`, `db`, `api` and `appruntime` test
packages was recorded with its bound values (a scratch `sqlite_trace` build,
not committed). For each of the **1,802** distinct parameterised statements,
`EXPLAIN QUERY PLAN` ran with the recorded values bound, once against the
default 3.54.1 build and once against the QPSG build, on the migrated schema.
The 41 statements that could not be planned in isolation are migration DDL
and test-only tables.

- **1,792** plans are identical.
- **6** differ only in the order of primary-key lookups inside the same join
  (`investment_capital_return` sets ×4, transfer depletion, transfer link
  revisions).
- **2** split-fact reads with the recursive `correction_roots` CTE scan
  `investment_split_facts` and probe an automatic index on the CTE, instead
  of scanning the CTE and probing split facts by primary key.
- **1** self-check (`investment_cash_payment_identity` ⋈ suggestions) uses the
  two partial unique indexes on generated/linked transaction IDs through a
  multi-index OR, instead of an automatic index on `book_id`.
- **1** forecast read (`ForecastSnapshotReader.draftPostings`) differs only
  because the recorded test values were all `1`. With book ID, template ID and
  `LIMIT` value equal, 3.54.1 starts from the template's occurrences; with any
  other template ID, 3.54.1 already uses the plan QPSG always uses
  (verified with the same SQL and IDs 1/5). QPSG removes that value
  dependence; it does not introduce the plan.

No changed plan adds a full scan of a book-scoped financial table.

## Correctness

- The retained [Go-driver probe](fixtures/native-sqlite-upstream-probe.go.txt)
  returns the same result with QPSG: JSON validity **1**, indexed count **1**,
  update **`new`**, `NOT INDEXED` control **1**. Bound-parameter variants of
  the expression-index probe (`a+1 = (SELECT ?) AND a = ?`) also return 1 with
  and without the index.
- `TestNativeSQLiteEngineFeaturesAndFingerprint` now requires `ENABLE_QPSG`
  in `PRAGMA compile_options`, so a build that bypasses
  `scripts/native-sqlite-env.sh` fails the backend gate.
- `TestNativeBoundJoinedEqualityCompilesOnce` requires one compile for a
  joined lookup with a bound equality and for a bound `IN` list.
- `TestTransactionResponseEnrichmentBatchesCompoundAndRepeatedJournals` again
  requires exactly one account and one commodity batch compile, the assertion
  #557 had relaxed to "one or two".

## Production-query cost

The admission review's [retained probe](fixtures/sqlite-driver-production-query-cost.go.txt),
run three times per build, alternating, on one four-CPU workspace, 3.54.1
throughout:

| Production path | 3.54.1 | 3.54.1 + QPSG | Delta |
|---|---:|---:|---:|
| Accounts batch, 2 IDs | 134.28 µs | 78.55 µs | **−41.5%** |
| Commodities batch, 2 IDs | 95.30 µs | 43.04 µs | **−54.8%** |
| Enrich 2 transaction records | 285.01 µs | 142.12 µs | **−50.1%** |
| Transaction by ID | 406.14 µs | 245.69 µs | **−39.5%** |
| List transactions, page 50 | 3,739.69 µs | 2,260.51 µs | **−39.6%** |
| Account register, page 50 | 1,353.56 µs | 784.80 µs | **−42.0%** |

Medians of three; raw samples are in the
[structured evidence](sqlite-qpsg-2026-11-11.json). This host differs from
the admission review's six-CPU host, so the 3.53.4 column there is not
directly comparable; the QPSG medians are nevertheless below every 3.53.4
median recorded there.

## Race-suite timing

Matched race runs of the application package (default 3.54.1 vs QPSG, alternating) are in progress on the review host and will be recorded here.

## Limits

- Same-host measurements, not a production load test or latency SLA.
- The plan scan covers statements the test suites execute, with their test
  values, on an unanalysed schema. Statements no test executes are not
  covered; the application never runs `ANALYZE`.
- A build that does not source `scripts/native-sqlite-env.sh` loses QPSG; the
  fingerprint test catches this in the backend gate, not at runtime.
- No statement cache, deadline, scheduling, durability, coverage or schema
  policy changes.
