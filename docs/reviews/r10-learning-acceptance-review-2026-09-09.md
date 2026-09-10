# R10 learned-spending acceptance review — 2026-09-09

R10, including its opt-in learned-spending extension M1–M4, is accepted. The
core milestone remains independently accepted in
`r10-core-acceptance-review-2026-09-07.md`; this review covers only the local
statistical overlay and does not weaken the core forecast's exact, read-only
contract. R8 budget planning is next.

Reviewed the implementation through `6ee0a042` plus the M4 corrections recorded
below against `docs/plans/forecast-learning-plan.md`, the financial conventions,
and ADRs 0004, 0009, 0010 and 0012. This is implementation acceptance over
synthetic fixtures. It is not a claim that the estimates predict this owner's
future spending well, nor a measured minimum-hardware promise.

## Commitment audit

| Commitment | Result and evidence |
|---|---|
| Classification | Accepted. Only complete, current, posted ordinary purchase entries with one asset/liability funder, one currency and positive expense legs train a group. Split categories are retained; transfers/card repayments, refunds or mixed signs, income/equity, investments, multiple funders, drafts, voids, deletes, future entries and recurring-linked history are excluded. `TestForecastLearningClassifiesCompleteEntries` and `TestForecastLearningUsesPostedHistoryOnly` are the primary evidence. |
| Confirmed history and cadence | Accepted. The owner supplies `history_complete_from`; the first partial and current incomplete periods are removed. Daily/weekly use completed Monday–Sunday weeks, monthly uses calendar months, and annual seasonality requires 36 complete months with activity in at least two recent year blocks. Windows stop at 52 weeks or 60 months. Cold, sparse and seasonal-cold-start boundaries are named tests. |
| Recurring overlap and known spend | Accepted. Unarchived enabled or paused templates and retained drafts disable implicated groups by account/category/currency identity, without payee guessing. Known posted/core spending is subtracted per calendar period before allocation, never added twice. `TestForecastLearningDoesNotOverlapRecurringGroups`, the overlay overlap test and known-spend tests cover the boundary. |
| Daily/weekly/monthly behavior | Accepted. Daily spreads the weekly level over remaining days; weekly uses learned weekday weights; monthly uses day-of-month weights with short-month clamping and re-anchoring. Exact largest-remainder allocation preserves the rounded period total and clips beyond-horizon dates only after whole-period allocation. |
| Annual behavior | Accepted. Month numbers remain separate, including July and August peaks and zero off-season months. The two-year same-month candidate is compared with last-year seasonal and flat references over a full held-out year; a March payment remains March spending. No holiday date or travel month is inferred. |
| Chronological evaluation | Accepted. Level models tune two exact-rational SES candidates and two baselines on prefix-only origins, freeze the winners, then evaluate four held-out one-period predictions and a frozen four-period total. Annual selection uses twelve expanding origins plus a frozen twelve-month path. A learned candidate needs at least 10% lower MAE and cannot worsen the applicable frozen metrics; a perfect baseline always wins. No-future-leakage and gate-boundary mutants are caught by named tests. |
| Exactness and bounds | Accepted. Posted inputs and output deltas stay coefficient strings; period sums use `exact.ScaledInt`, fitting/errors use `math/big.Rat`, and output rounds once half away from zero. Values beyond 2^53 remain exact and 39-digit results surface as ledger overflow. Reads use limit+1; fitting is single-slot, cancellation/deadline aware, capped at 100 groups and never returns a prefix on resource refusal. |
| Snapshot and isolation | Accepted. Learning history joins the core inputs in the same deferred read transaction and is not read at all while off. The option never creates transactions, templates, occurrences, audits, background work, provider requests, checkpoints or lot effects. Core series are field-for-field identical on/off; estimates live in a nullable overlay and synthetic events have no saved-record IDs. |
| API and details | Accepted. `spending_model=adaptive_v1` requires a confirmed history date; repeated category and category-pattern options are strict, bounded and basis-token inputs. Ready, partial and unavailable states expose stable exclusion/warning codes. Estimated day events reconcile separately to learned deltas and participate in stale-basis event paging without becoming core components. |
| FX separation | Accepted after T-91. Native learned curves remain exact. When constant FX is requested, a separately converted learned curve is returned only when its own currencies have complete seven-day coverage. A model-only missing currency hides that curve without changing the already-computed core valuation status. M4 also corrected aggregate learned deltas so totals reconcile to events. |
| UI and disclosure | Accepted. `/app/forecast` makes estimates opt-in and reversible, with Recorded only / With recurring / With estimated spending visually distinct. It shows the history date, cadence controls, training/test windows, model or fallback, exact retrospective error, historical variation, annual month profile, exclusions and limited-history/long-horizon caveats in six locales. It uses neither probability/confidence language nor an AI-chat narrative. Keyboard, axe and 390 px containment are exercised by the learned-spending browser journey. |

## Evaluation and interpretation limits

- Four held-out weeks/months and one held-out year are small retrospective
  samples. They validate selection mechanics, not real-world accuracy.
- Evaluation uses today's current ledger versions, not a historical replay of
  what was known at each origin. Late imports and corrections can change it.
- Daily/weekly tests do not validate 90- or 366-day accuracy; longer curves
  continue the selected level. One held-out year does not guarantee the next.
- Patterns are owner choices per expense category. The model does not infer
  holidays, inflation, trends, category splits, income, transfers, FX, prices
  or investment returns, and it never creates recurring rules.
- Historical min/max values are observations, never prediction intervals. No
  random jitter or probability label is produced.

These limitations are visible in the screen rather than confined to this
review. Real-owner usefulness remains an observational product question; it is
not a correctness blocker for this explicitly optional, safely removable view.

## Resource evidence

The required 2026-09-09 rerun used Linux/amd64, Go 1.27.0, one benchmark CPU on
an AMD EPYC 3251 host. The full synthetic ceiling—100 groups split across all
four patterns at their maximum 52-week/60-month windows—measured 90.4 ms/op,
29.5 MB cumulative allocation and 1,038,951 allocations/op. Weekly selection
measured 0.95 ms/op, seasonal selection 0.36 ms/op, and 366-day exact allocation
0.79 ms/op. M2's separate `runtime.MemStats` measurement recorded about 2.7 MiB
incremental peak live heap, below the 64 MiB target; cumulative allocation is
not peak live memory.

The available host is not the provisional one-thread/roughly-1-GiB reference
machine. Therefore the ≤2 s and ≤64 MiB figures are accepted as engineering
targets supported by substantial headroom on the measured host, not as verified
minimum-machine guarantees. The product makes no such hardware promise. Busy,
deadline and resource-limit fallback states keep the core forecast available if
a smaller deployment cannot fit within those bounds.

## Validation evidence

- `./scripts/test-backend.sh`: passed, including the full race-enabled backend
  suite and all M1–M3 financial/resource cases.
- `./scripts/test-frontend.sh`: passed; OpenAPI generation, six-locale message
  compilation, Svelte diagnostics and all 377 tests are green.
- `pnpm --dir e2e test forecast-learning.spec.ts --project=app`: six passed,
  including bootstrap plus all four learned-spending acceptance journeys; its
  server step also completed the integrated single-binary build.
- `go test ./internal/app -run '^$' -bench ForecastLearning -benchmem -cpu 1`:
  passed with the measurements above.

## Findings resolved during acceptance

- **T-91:** the M3 response schema exposed `learned_spending.converted`, but the
  overlay never populated it; native aggregate rows also emitted a zero
  `estimated_delta`. M4 added independent learned-FX coverage, included
  model-only currencies in the snapshot rate candidates, taught the frontend to
  select the converted learned curve, and made aggregate deltas reconcile to
  estimated events. `TestForecastLearningSeparateFXCoverage`,
  `TestForecastLearningRateCandidatesIncludeModelOnlyCurrency`, the aggregate
  assertion and the converted frontend-model test prevent recurrence.

No ADR or product-scope change was needed. The remaining ideas in the extension
plan—automatic cadence discovery, calibrated uncertainty, missing-bill
suggestions, richer models and LLM explanations—remain later candidates, not
unfinished R10 work.
