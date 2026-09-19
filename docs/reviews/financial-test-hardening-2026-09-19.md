# Financial test hardening — 2026-09-19

Starting point: `598c4afc`, which fixes T-100, T-101 and T-102. This change adds
tests and testing guidance; it changes no application behavior or schema. All
fixtures use disposable migrated databases, not household records.

## Why double entry did not catch inflated cashflow

A 100 EUR purchase of 10 shares has four postings:

| Account | Commodity | Quantity |
| --- | --- | ---: |
| Cash | EUR | -100 |
| Commodity trading | EUR | +100 |
| Holding | Shares | +10 |
| Commodity trading | Shares | -10 |

Each commodity balances. The journal is correct. The old cashflow classifier
also treated the two share postings as cash transfers, despite neither touching
the selected cash account. Valuing those shares at 10 EUR reported 100 EUR in
and 200 EUR out instead of zero in and 100 EUR out. Both presentations have
net movement of -100 EUR. Therefore neither journal balancing nor the report's
`net = in - out` identity distinguishes the wrong report from the right one.

The prior tests were insufficient for these bug classes:

- Net identities cannot detect balanced additions to gross flows.
- A single decimal representation cannot establish representation independence.
- Conservation alone cannot establish which cost-basis method was applied.
- Checking a version at commit cannot prove it came from the original decision;
  the test must put the account edit between the relevant reads.
- Service arithmetic cannot prove an HTTP response carries the correct scale
  or combines same-currency totals correctly.

The latest fix already adds the role-read interleaving tests and direct
regressions. This pass broadens the financial assertions rather than replacing
them with another count of covered lines.

## Added coverage

Eight passing named tests in three files broaden the ordinary Go suite. A
ninth, active failing regression is described under T-103 below:

- `backend/internal/app/financial_properties_test.go`
  - 60 deterministic shuffled cashflow cases vary signs, account classes,
    scales, and wide coefficients of unrelated balanced commodity groups.
    Those groups must not change any gross or net cashflow measure.
  - A real household sequence includes salary, split spending, refund,
    savings transfer, FX transfer, buy, sell and a withheld dividend. Four
    account scopes have independently stated gross and net expected amounts.
  - Invalid postings whose errors cancel across entries, commodities, or raw
    coefficients at different scales must be rejected without durable residue.
  - Split, mixed-scale 30-digit monetary coefficients survive the real write
    and report paths exactly.
- `backend/internal/app/investments_sequence_properties_test.go`
  - 12 fixed-seed command sequences (three seeds for each disposal method)
    compare journal quantities and cash against the entered facts after each
    command. They check lot quantity, original and remaining basis, reported
    gains, lot closure, preview allocations, and preview/report non-mutation.
    Oversells must roll back journal, lots, disposal evidence and audit rows.
  - 48 full-position closures cross four methods, four equivalent decimal
    representations and positive/zero/negative gain. Expected results use
    independent rational arithmetic rather than the production scale helper.
  - Four hand-calculated partial sales distinguish FIFO, LIFO, average cost
    and specific lot. Rounding residuals must survive until final closure.
- `backend/internal/api/investments_gains_properties_test.go`
  - Exercises authenticated writes and reads. Mixed-scale gains and losses
    produce one correct total per currency; date filtering is respected;
    gain scale is explicitly present in JSON, including zero. A second currency
    remains separate with its own independently expected total.

The partial-sale oracle buys three shares for 1.01 and two for 2.03, then sells
two for 2.00. Expected disposed bases are 0.67 (FIFO), 2.03 (LIFO), 1.21
(average cost), and 1.34 (one share from each specific lot). A final sale of all
remaining shares for 0.03 must leave zero basis and total gain of -1.01 under
every method. Correctly conserving 3.04 alone would not establish those first
four answers.

## Mutation checks

Each deliberate mutation was applied separately in an isolated archive of
`598c4afc` with the new tests copied in. The original source was restored after
each run. Every mutation compiled and failed a test assertion; compile failures
were explicitly excluded as evidence. No mutation touched the working tree.

| Deliberate defect | Test that detected it |
| --- | --- |
| Classify all commodity groups once any group touches cash | `TestFinancialCashflow*` |
| Truncate disposed basis to proceeds scale | `TestFinancialClosedPositionGainIgnoresDecimalRepresentation` |
| Sum API gain coefficients using proceeds scale | `TestFinancialGainsHTTPPreservesScaleAndSumsOneTotalPerCurrency` |
| Replace the first observed account version with the latest | Existing `TestBuyPlannedBeforeItsHoldingAccountChangedIsRefused` |
| Add one minor unit to remaining lot basis | `TestFinancialTradeSequencesPreserveJournalLotsBasisAndGains` |
| Disable the per-entry balance rejection | `TestFinancialPostingBoundaryRejectsCancellationAcrossEntriesOrCommodities` |
| Use FIFO ordering for LIFO | `TestFinancialPartialDisposalsUseChosenMethodAndRetainResidual/lifo` |
| Allocate one extra minor unit in average cost, conserving the pool | `TestFinancialPartialDisposalsUseChosenMethodAndRetainResidual/average_cost` |

Seven checks exercise the new tests; the account-version mutation independently
confirms the existing fix's regression. These are targeted fault injections,
not a claim of exhaustive mutation coverage. Their purpose is to demonstrate
that green tests discriminate between correct and plausible incorrect logic.

## New finding: T-103 — partial basis depends on purchase representation

**P2, confirmed on `598c4afc`; unresolved.** Buy three shares for exactly 10 EUR,
then sell one for 5 EUR. Only the purchase's coefficient/scale changes:

| Purchase representation | Disposed basis | Reported partial gain |
| --- | ---: | ---: |
| `10` at scale 0 | 3 EUR | 2 EUR |
| `1000` at scale 2 | 3.33 EUR | 1.67 EUR |
| `100000` at scale 4 | 3.3333 EUR | 1.6667 EUR |

This reproduces under FIFO, LIFO, average cost and specific lot. The gain
subtraction fixed by T-101 is now correct for the basis it receives; this is an
earlier allocation defect. `disposeLotTx` prorates the remaining coefficient
using `proratedCostBasis`, which truncates integer division and retains the
lot's recorded basis scale. Average cost does the same at its pooled scale.
Since that scale comes from the purchase input, text formatting becomes an
implicit allocation-precision policy.

The residual remains in the lots, so aggregate basis conservation and final
closure are both correct. That is precisely why the successful sequence and
full-closure checks do not prove that an earlier period's gain was correct.
`docs/plans/investments-plan.md` explicitly describes truncating division and
residual conservation; it does not resolve this input-dependent precision.
Changing truncation to half-up alone would not make the representations equal.

The regression is **active**, in
`backend/internal/app/investments_partial_precision_test.go`:
`TestFinancialPartialDisposalGainMustNotDependOnPurchaseTextPrecision`.
All four method subtests fail. It requires equal economic inputs to allocate
equal basis under one policy; it deliberately does not choose an arbitrary
number of allocation decimal places. No skipped test, expected-bug assertion,
or uncompiled probe conceals the failure from CI.

Required follow-up: define an allocation precision independent of input text,
apply it consistently to lot and pooled projections, and preserve exact residual
conservation. Handle coefficient range explicitly and assess existing lots and
disposal history; do not silently rewrite past financial evidence. Then make
this regression pass while retaining the independent method and closure checks.
No application fix or new rounding policy is included in this test-hardening
change. Hold the financial correctness gate until T-103 is resolved.

## Validation and limits

Focused command (also documented in the developer workflow):

```sh
cd backend
go test ./internal/app ./internal/api -run '^TestFinancial' -count=1
```

Validation before the final T-103 discovery:

- `./scripts/test-backend.sh`: exit 0 (formatting, vet and full backend race
  suite); the application package completed in 335.6 seconds.
- The eight new passing tests passed focused race runs, including the final
  two-currency HTTP extension.
- Eight isolated mutations: eight compiled assertion failures, as listed above.

Final validation with T-103 active:

- `./scripts/test-backend.sh`: **exit 1**. Formatting and vet passed; the full
  race suite failed only on
  `TestFinancialPartialDisposalGainMustNotDependOnPurchaseTextPrecision` and
  its four method subtests. All other tests/packages passed; no race warning
  was reported. The application package completed in 316.1 seconds.
- A focused race run of that regression also failed on the same four methods.
- Final `go vet ./internal/app ./internal/api` and `git diff --check`: passed.

This is an intentional red correctness gate for the newly discovered defect,
not an environmental error. The application code and schema are unchanged.

The tests do not exhaust every amount, event sequence or concurrency interleaving, and they
do not expand supported investment workflows. Existing reconciliation,
lifecycle, migration and restore tests remain necessary. Browser behavior was
not changed or revalidated in this test-only backend change. Coverage percentage
was not recomputed: adding decisive assertions, including ones on already
covered lines, is the relevant improvement here.
