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
- **2** split-fact reads (`PricingRepository.InstrumentSplits`, two and three
  commodity IDs) change from scanning the recursive `correction_roots` CTE and
  probing `investment_split_facts` by primary key to a **full scan of
  `investment_split_facts`** (`SCAN f`) that probes an automatic index built
  on the CTE. The table has no index besides its primary key, so this scan
  reads every book's split facts, one row per recorded split. Both plans
  still read the whole CTE, which covers every investment operation in the
  book.
- **1** self-check (`investment_cash_payment_identity` ⋈ suggestions) uses the
  two partial unique indexes on generated/linked transaction IDs through a
  multi-index OR, instead of an automatic index on `book_id`.
- **1** forecast read (`ForecastSnapshotReader.draftPostings`) differs only
  because the recorded test values were all `1`. With book ID, template ID and
  `LIMIT` value equal, 3.54.1 starts from the template's occurrences; with any
  other template ID, 3.54.1 already uses the plan QPSG always uses
  (verified with the same SQL and IDs 1/5). QPSG removes that value
  dependence; it does not introduce the plan.

One changed plan adds a full scan of a book-scoped financial table: the
`investment_split_facts` scan in `InstrumentSplits` above. No other changed
plan adds a table scan. [#561](https://github.com/sergeyfarin/rekenraam/issues/561) measured it and fixed the cause; see
[Follow-up: split reads](#follow-up-split-reads-561).

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

Isolated, uncached race runs of the `internal/app` package on the same
four-CPU workspace, alternating builds from one clean worktree
(`go test -race -c`, then the test binary with `-test.count=1`):

| Build | Run 1 | Run 2 |
|---|---:|---:|
| 3.54.1 | 306.5 s | 297.2 s |
| 3.54.1 + QPSG | **255.2 s** | **250.0 s** |

QPSG is **16–17% faster** in each matched pair. The only failures, identical
in all four runs, are three backup tests that require an unwritable
destination; this workspace runs as root, so permission errors never occur.
These are isolated single-package runs on a four-CPU host, not ADR 0016's
`-p 2` full-gate threshold runs on the six-CPU threshold host. They show the
direction and size of the recovery but are not formal threshold evidence.

Hosted CI observations (pull-request runs, Go test cache enabled, the
`internal/app` package executed):

| Build | Commit | Run | `internal/app` |
|---|---|---|---:|
| 3.53.4 (main baseline) | `7d3d2059` | [42760293595](https://github.com/sergeyfarin/rekenraam/actions/runs/42760293595/job/129644138504) | 219.4 s |
| 3.54.1 | `b6655526` | [42767669769](https://github.com/sergeyfarin/rekenraam/actions/runs/42767669769/job/129667989356) | 252.7 s |
| 3.54.1 | `faeedf20` | [42876220590](https://github.com/sergeyfarin/rekenraam/actions/runs/42876220590/job/130040105987) | 253.2 s |
| 3.54.1 + QPSG | `5e5cf679` | [42985705399](https://github.com/sergeyfarin/rekenraam/actions/runs/42985705399/job/130406232475) | **167.1 s** |
| 3.54.1 + QPSG | `f95eabcb` | [42991474105](https://github.com/sergeyfarin/rekenraam/actions/runs/42991474105/job/130425311729) | **182.2 s** |

ADR 0016 measures its thresholds differently, so neither table is threshold
evidence. Act needs two uncached `-p 2` full gates on the six-CPU threshold
host. Investigate needs the `internal/app` median of three interleaved
uncached runs on the ten-CPU reference host, or of CI's latest six successful
`main` runs. [#560](https://github.com/sergeyfarin/rekenraam/issues/560) owns both.

## Follow-up: split reads (#561)

`InstrumentSplits` and `TradeQuotePendingSplits` (valuation and returns reads)
mapped operations to their correction roots with a recursive CTE over
**every** investment operation in the book, then joined the few splits and
quotes in range to it. Both plans, with and without QPSG, read that whole CTE,
so each read cost time in proportion to the book's investment history. Under
QPSG, `InstrumentSplits` additionally scanned `investment_split_facts`, which
had no index besides its primary key.

The fix:

- `correctionRootsOf` seeds the walk with only the operations a read needs and
  climbs each `correction_of_operation_id` chain by primary key. `CROSS JOIN`
  keeps the seeds first.
- `TradeQuotePendingSplits` matches trade-implied observations to the
  effective splits on their day once, in a materialized CTE, and seeds the
  walk from those matches.
- Migration `0012` adds `investment_split_facts_lookup_idx`
  (`book_id, commodity_id, effective_on`). Neither read now scans a book
  table; the remaining scans are of the small seed, ancestry and match CTEs.

Measured with the opt-in `TestSplitQueryCost` probe
(`SPLIT_QUERY_COST=1 SPLIT_QUERY_OPS=n`): one holding, *n* buys, three 2:1
splits, two instrument IDs. Medians of three alternating rounds per build on
the four-CPU workspace:

| Read | Operations | Before, 3.54.1 | Before, + QPSG | After, 3.54.1 | After, + QPSG |
|---|---:|---:|---:|---:|---:|
| `InstrumentSplits` | 200 | 239 µs | 216 µs | 210 µs | 137 µs |
| | 2,000 | 1,383 µs | 1,317 µs | 237 µs | 138 µs |
| | 5,000 | 3,439 µs | 4,325 µs | 203 µs | **98 µs** |
| `TradeQuotePendingSplits` | 200 | 764 µs | 528 µs | 850 µs | 498 µs |
| | 2,000 | 3,613 µs | 3,160 µs | 1,871 µs | 1,295 µs |
| | 5,000 | 9,658 µs | 9,582 µs | 2,906 µs | **2,116 µs** |

The "after" timings were taken before migration `0012`; the index removes the
remaining split-facts scan, which reads one row per recorded split.
`InstrumentSplits` no longer grows with the operation count.
`TradeQuotePendingSplits` still reads the instrument's observations in range
through `price_observations_lookup_idx`, as it did before, so it keeps a smaller
linear term in trade-implied quotes.

Correctness:

- `TestCorrectionRootsOfMatchesReplayOrderRoots` compares the new roots with
  the book-wide replay order (`investmentReplayOrderOperationIDsQuery`)
  through a two-step replacement chain, for one seed, duplicate seeds and an
  unknown operation. It fails when the root or the chain walk is wrong; no
  existing test did.
- `TestTradeQuoteBeforeALaterDaySplitIsNotPending` pins that only splits on
  the quote's own day are pending. Loosening that match passed every existing
  test.
- Dropping the trade seeds fails five existing split-day quote tests.
- `TestBaselineIndexesKeepOrderedAndForeignKeyLookups` requires the new index
  for split facts by instrument and date.

## Limits

- Same-host measurements, not a production load test or latency SLA.
- The plan scan covers statements the test suites execute, with their test
  values, on an unanalysed schema. Statements no test executes are not
  covered; the application never runs `ANALYZE`.
- A build that does not source `scripts/native-sqlite-env.sh` loses QPSG; the
  fingerprint test catches this in the backend gate, not at runtime.
- No statement cache, deadline, scheduling, durability, coverage or schema
  policy changes.
