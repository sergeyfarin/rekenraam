# R10 core acceptance review — 2026-09-07

R10's eight-slice core projected-balances milestone is accepted. The shipped
screen answers how recorded entries and known recurring assumptions change
selected cash balances without writing forecasts into the ledger. The approved
learned-spending extension remains deliberately open as M1–M4; this review does
not call the whole R10 initiative complete and does not start R8.

Reviewed the implementation through `33ebf631` plus the acceptance correction
recorded below against the product requirements, conventions, ADRs 0004, 0009,
0010 and 0012, and `docs/plans/projected-balances-plan.md`. This is code and
automated-workflow acceptance, not longitudinal owner research or a claim that
learned spending is available.

## Commitment audit

| Commitment | Result and evidence |
|---|---|
| User outcome and navigation | Accepted. `/app/forecast` is linked from the authenticated navigation. URL-backed horizon, account, descendant and optional conversion filters survive reload and browser navigation; malformed filters have an explicit reset path. |
| D1: owner-local dates and opening | Accepted. The service captures one clock instant, resolves the owner zone in the snapshot, rejects caller-supplied as-of dates and emits exactly H future calendar days after a separate opening at A. P01/P02 cover entry-date boundaries and current posted input rules; date-bound and API ambiguity cases cover 1–366 and year limits. |
| D2: account/currency scope | Accepted. Default scope is active liquid cash; explicit asset/liability roots expand and deduplicate as-of-A descendants or remain root-only. System/category/security-holding roots are rejected, a group with no eligible posting descendant stays empty, closed explicit accounts remain inspectable, and response metadata names the resolved scope. P12 and A04 are the primary evidence. |
| D3: recurrence identity and precedence | Accepted. `(template_id, occurrence_date)` suppresses computed replacements. A current generated draft replaces its template; posting moves the amount to recorded facts; skipped, discarded, voided, deleted, blocked and broken identities cannot resurrect assumptions. P03–P06 and the real R9 generation/promotion service path prove lifecycle transitions rather than matching by amount or description. |
| D4: overdue assumptions | Accepted. Draft and computed dates due on/before A move together to F while retaining source dates and without changing watermarks or saved transactions. P07/P08 expose carried-forward counts and diagnostics and retain R9 count/end constraints. |
| D5: invalid and blocked assumptions | Accepted. Full entries are validated before selected-account filtering; an invalid counterpart excludes the whole event. Account/commodity lifecycle, precision and currency-only rules apply at the source and carry dates, while posted history is not revalidated under later rules. P10/P11/P17 and the blocked browser fixture expose bounded, translated diagnostics and `assumptions.complete=false`. |
| D6: exact daily identities | Accepted. Coefficients remain canonical strings and all financial accumulation uses `exact.ScaledInt`; no browser total becomes canonical. P13–P16 cover transfer/liability signs, shuffled deterministic input, mixed scales, values above 2^53, overflow, flat series, earliest minima and cash-only negative dates. Daily account and aggregate balances reconcile from separately named posted/draft/template components. |
| D7: constant-as-of FX | Accepted. Conversion is paired, optional and additive. Stored direct observations are selected inside the snapshot at A with the seven-day/tie/void rules, then fixed across the horizon. Same-currency identity needs no observation; coverage is determined before account netting; each source component rounds half away from zero. F01–F08 and A08 prove that incomplete coverage removes the whole combined curve while exact currencies remain. |
| D8: coherent read-only basis | Accepted after T-88. The repository uses the existing read-only pool and one SQLite snapshot for zone, rules, postings, occurrences, drafts and optional rates. D04 exercises real generation through an independent writer pool. X01 compares reports, CSV/QIF and financial-domain counts around balances/events reads. Strengthened X02 now begins with a real investment lot, lot event and active reconciliation checkpoint rather than empty tables. |
| API and event explanations | Accepted. Authenticated `/api/v1/forecasts/balances` and `/balance-events` use strict query parsing, private no-store responses, exact quoted quantities, bounded stable cursors and a basis token. More than 200 events reconcile across pages; stale bases and changed recipes cannot mix detail pages. OpenAPI, generated client types and Bruno examples agree. |
| Bounds and failure behavior | Accepted. B01–B04 cover long daily catch-up, exhausted/sparse schedules, input/candidate/output limits and cancellation. Limits return an explicit 422 instead of a plausible prefix; 39-digit arithmetic fails as `LEDGER_OVERFLOW`. No relevant failure path substitutes a partial success total. |
| Screen states and accessibility | Accepted. The eight forecast browser journeys cover real recorded/recurring sources, edited drafts, URL state, deferred loading, backend error/retry, flat and excluded-assumption states, missing FX, stale event recovery, Russian localization, keyboard use, axe, dark mode and 390px containment. The table remains the exact authority and event details consume their cursor. |
| Cross-system isolation | Accepted. Forecast reads leave net worth, spending, cashflow, ledger CSV and QIF byte/JSON output unchanged. Transaction/version/posting, occurrence, audit, background-work, checkpoint and investment-lot counts remain unchanged. Visiting or refreshing never generates a draft, posts a transaction, requests FX coverage or mutates an investment projection. |

## Implementation and test map

| Area | Shipped code/contract | Primary evidence |
|---|---|---|
| Snapshot and basis | `backend/internal/db/forecast.go`, `backend/internal/app/forecast_inputs.go` | Seven snapshot repository cases plus P01, P05, P06, P11, P12, B03, B04 and D04. |
| Exact projection | `backend/internal/app/forecast.go`, `forecast_projection.go`, `forecast_events.go` | P01–P18 and B01–B04 in `backend/internal/app/forecast_test.go`; real R9 generation/promotion path included. |
| Constant FX | `backend/internal/app/forecast_rates.go` and the snapshot rate reader | F01–F08 across app/db/API tests, including >2^53, tie/staleness, pre-netting coverage, rounding and no queued work. |
| HTTP contract | `backend/internal/api/forecast.go`, `api/openapi/paths/forecast-*.yaml`, `api/openapi/components/schemas/forecast.yaml` | A01–A08 and D01–D03 in `backend/internal/api/forecast_test.go`; generated `frontend/src/lib/api/schema.d.ts` and Bruno recipes. |
| Typed client and screen | `frontend/src/lib/api/forecast.ts`, `frontend/src/lib/forecast/`, thin `/app/forecast` route | API/model unit tests and all eight named cases in `e2e/playwright/forecast.spec.ts`. |
| Cross-system isolation | Existing report/export/recurring/investment production paths invoked through the real API handler | X01/X02 in `backend/internal/api/forecast_test.go`, including reports, CSV/QIF, a real producer draft, investment lot/event and active checkpoint. |

## Required scope and defaults revisited

The planned defaults remain appropriate for the core milestone: active liquid
cash accounts, a 90-day default, presets at 30/90/180/365, custom 1–366 days,
tomorrow carry-forward for overdue unposted assumptions, and optional complete-
coverage constant FX. The event basis/cursor UX remains necessary: it prevents
an explanation from silently describing a different forecast after a producer
mutation. No decision changed during acceptance, so no ADR amendment is needed.

The forecast is intentionally a live current projection. It is not a stored
knowledge-time snapshot, bank available balance, budget, overdraft/credit-limit
promise, or reproducible market valuation. Constant FX can change on a later
refresh when A changes or a stored observation is corrected; the screen labels
that assumption before presenting a combined total.

## Excluded scope disposition

| Item | Include in accepted core? | Reason |
|---|---|---|
| Automatic posting or forecast commit | No | R9's explicit review/post/discard boundary remains authoritative. |
| Manual what-if adjustments, saved scenarios, probability bands | No | They require durable scenario identity and separate editing semantics. |
| Historical averages and learned seasonal spending | No, but approved next | M1–M4 in `forecast-learning-plan.md` own opt-in local models, evaluation and final R10 closure. |
| Loan amortization, interest and card-payment calculation | No | These need explicit liability models; balances alone are insufficient. |
| Forecasted FX, security/crypto prices and returns | No | Core includes currency postings and labeled constant-as-of FX only. |
| Forecast CSV/print/snapshots/notifications | No | Existing exports remain posted actuals; no forecast persistence exists. |
| Arbitrary as-of dates | No | Core v1 is owner-local today from current records. |
| Budget integration, RRULE and banking calendars | No | R8 or later recurrence decisions own them. |
| New storage, workers, provider calls or schedule mutation | No | The forecast remains a bounded read model over existing records. |

## Finding resolved during acceptance

- **T-88:** `TestForecastDoesNotTouchInvestmentSubledgerOrCheckpoints` compared
  zero-row investment and reconciliation tables. That proved a forecast did not
  create the first row, but not that existing durable state survived unchanged.
  The fixture now buys an instrument to create a real lot and lot event, finishes
  reconciliation to create an active checkpoint, calls both forecast endpoints,
  and asserts every domain count is identical and each critical precondition was
  non-empty. The focused regression passes.

The audit also corrected stale execution prose that still described the screen
or slices 7–8 as unimplemented. No production-code defect, financial-semantic
change or new unresolved backlog item was found.

## Validation and remaining limits

Final acceptance uses the repository wrappers and production binary gate:

- `./scripts/test-backend.sh` — formatting, vet and all race-enabled backend tests.
- `./scripts/test-frontend.sh` — generated contracts/catalogs, zero Svelte
  diagnostics and 372 passing tests in 24 files.
- `pnpm --dir e2e test forecast.spec.ts recurring.spec.ts transactions.spec.ts accessibility.spec.ts` — production build plus 27 passing forecast/R9/shared-entry/accessibility journeys, with no dependency-skipped cases counted.
- `pnpm build` and `git diff --check` — static frontend embedded in the Go binary; clean documentation/code diff.
- Catalog audit — all six locales contain the same 100 `forecast_*` message
  keys; generated Paraglide output compiles in the frontend and production gates.

Synthetic and deterministic fixtures establish invariants, not forecast quality
for a real household. Native-language review remains broader product work. Open
backlog items such as T-76, T-75b, T-79 and T-87 are independent and are not
silently absorbed into core acceptance.

Next is M1 of `docs/plans/forecast-learning-plan.md`: complete-history reads,
cadence classification, baselines and exact residual allocation. Core behavior
must remain unchanged when learning is off. R8 follows only after M4 performs
the final R10 learning acceptance.
