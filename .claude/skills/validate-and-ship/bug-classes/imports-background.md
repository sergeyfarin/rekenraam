# Bug classes — Imports, providers and background work

Part of the `validate-and-ship` review checklist. Import pipeline, provider
adapters, work queue, FX refresh. Every entry shipped as a real bug in this
repo. Numbers are stable IDs cited from code comments and docs; they are not
an order and are never reused.

## 5. Cursor boundaries

`<=` vs `<`, resume token vs incremental boundary (see `background-work`).

## 21. Order side mistaken for provider event type

BUY/SELL does not establish an ordinary execution. Preserve the provider fill
taxonomy and require explicit TRADE before native import, generic cash
fallback or source correction. Unknown/missing types fail closed. Order
lifecycle status cannot cancel an execution and must not create economic
revisions. Named regressions: `TestImportUnsupportedFillCannotPost`,
`TestUnsupportedSaleFillCannotUseCashFallback`,
`TestUnsupportedFillCannotCorrectAcceptedBuyOrSale`.

## 46. Missing defaults silently discarding known financial attribution

Trading 212's dividend writer used to fall back to ordinary cash when an
income default was absent, losing a payer already resolved (#242). Route known
dividends through explicit owner review and the named writer. Missing
gross/tax stays pending beside unrelated valid rows; cash does not prove gross
or zero tax. Historical repair must retain the same payment/source identity,
pin its current version inside the writer, and preserve unchanged
reconciliation. Test late identity failure, setup cleanup/retry and dedupe
after native owner correction. Named evidence:
`TestTrading212DividendOwnerReviewRetainsKnownPayerWithoutDefaults`,
`TestImportedDividendRepairKeepsTransactionCashSourceAndNativeCorrections`,
`TestImportedDividendRepairWriterPinsVersionAndSourceEligibility`.

## 50. Continuation deduplicated against itself

An FX continuation enqueued its identical successor before completing, so
active deduplication returned the running item and completion lost the
remaining horizon (#314). Complete and enqueue atomically, with lease fencing
and rollback; exercise more than two chunks
(`TestFXCoverageWorkerFinishesEveryContinuationChunk`). #315 also fixes
starvation at missing publication dates: persist scan progress separately from
observations, retry failed dates independently, count every attempt against
the budget, and test fresh posting demand while a cursor is queued
(`TestFXCoverageFreshDemandRevisitsLatePublicationAndEarlierDates`).

## 75. A loaded empty demand treated as uninitialized

nil is the FX triangulation fallback's signal to query demand, so an empty
repository result must remain a non-nil loaded set (#466). Count actual demand
reads across failed derivations, including an empty book. Capture the owner
day at run start and pass that context through demand and target selection;
midnight must not change the selected currency versions mid-run. Named checks:
`TestFXRefreshEmptyDemandIsReadOnceAcrossTriangulationAttempts` and
`TestFXRefreshDemandUsesOwnerDayCapturedAtRunStart`.

## 76. Completing future demand before it becomes due

A zero-work sweep must atomically retain an owner-date wakeup (#471), with no
provider attempts or early latest fetch from manual/scheduled refresh. Test
the real outbox and worker through database reopen, local midnight/DST,
cancellation and replacement, repeated sweeps and new current demand while
waiting. Include successor-publication rollback, other-currency chunk
exhaustion and the eight-attempt provider retry cap (`TestFXFutureDemand*`).
