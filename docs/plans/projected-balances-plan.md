# Projected Balances Plan (R10)

Status: **in progress — core slices 1–5 complete**. Written 2026-08-31 against `b28c5d57`,
after R9 acceptance. This is the execution specification for the next initiative
in `docs/roadmap.md`: **R9 → R10 → R8**. Planning is complete when this document
lands; the forecast API, optional conversion and read-only screen now ship.

Implementation update (2026-09-07): the coherent snapshot reader, exact
per-currency application projection, authenticated balances/event endpoints,
optional complete-coverage constant-as-of FX conversion and responsive forecast
screen are complete. Core slice 6 is next.

Scope amendment (2026-08-31): after these eight **core** slices, execute M1–M4
in [lightweight learned spending](forecast-learning-plan.md). The owner requested
local CPU learning with daily/weekly fluctuations, monthly costs and annual
seasonality (for example July–August travel). That companion specifies opt-in
models, history/quality/resource gates and final R10 acceptance. Core slice 6
remains next; event-detail UI and learning code are not implemented yet.

Navigation: [decisions](#3-financial-and-date-decisions) ·
[backend algorithm](#4-backend-read-model-and-algorithm) ·
[API contract](#5-http-contract-openapi-first-in-slice-3) ·
[worked fixtures](#6-worked-fixtures--expected-numbers-not-screenshots) ·
[execution slices](#10-ordered-execution-slices) ·
[test matrix](#11-required-test-matrix) ·
[handoff](#13-copyable-execution-and-handoff-instructions).

## 0. Instructions to the implementing agent

Implement **one numbered slice at a time**, in order. Do not implement the whole
plan in one turn or commit. Sections 1–9 are the design contract; section 10 is
the execution queue; sections 11–13 are the test matrix, completion gate and
handoff template. Read the contract before starting slice 1.

1. Read `AGENTS.md`, `docs/product-requirements.md`, `docs/conventions.md`,
   `docs/early-architecture-decisions.md`, relevant accepted ADRs, and
   `docs/developer-workflow.md`. Accepted ADRs govern conflicts.
2. Read `.claude/skills/README.md`, then `ledger-invariants` and
   `validate-and-ship`. For backend slices also read `backend-slice`; for API
   slices, `api-contract`; for frontend slices, `frontend-screen`. These are
   files under `.claude/skills/<name>/SKILL.md`, not external plugins.
3. Read this whole plan and the R9 acceptance review. Inspect current files:
   references below are verified at the planning revision, not permission to
   overwrite later work. Start with a clean-status inspection and preserve
   unrelated changes.
4. Implement the first incomplete slice only. Write the named tests before
   fixing a discovered defect. Do not weaken a test or change financial
   semantics simply to make implementation easier.
5. Run that slice's checks, update its execution record, and commit the scoped
   result directly to `main` under the repo workflow. Do not push, publish or
   open a PR unless requested.
6. Hand off the exact next slice using section 13. A partial implementation
   stays partial in `implemented.md`; an API-only slice is not a shipped UI.

All file paths in this plan are relative to the repository root. **Existing**
references identify code to inspect; **new** paths are proposed destinations.
A filename without a directory in a file list belongs to the last explicitly
named directory in that list. The proposed names are fixed unless a real
collision exists. Do not invent a generic forecasting framework, queue, event store,
scenario database, new decimal library, or second transaction editor.

### Stop conditions versus ordinary implementation choices

Stop and report a concrete design conflict if implementation requires changing
an accepted ADR, posting forecasts to the ledger, forecasting security prices,
weakening exact precision, deleting financial history, or silently omitting
inputs to meet a limit. Do not stop for file organization, private helper names,
normal error handling, or test-fixture construction: resolve those within this
contract. A failed runtime test is work to investigate, not permission to skip
acceptance. If a slice is too large for one session, commit a runnable smaller
part with its unfinished checklist explicitly recorded.

## 1. User outcome and scope

The user can answer: **“Given what I have recorded and the recurring entries I
expect, how will my selected account balances change over the next few months,
and which entries explain a low balance?”**

The screen is a projection from current records, not a bank's available balance,
a promise of future cash, a budget, or an investment-return forecast. It never
saves or posts a transaction, generates a recurring occurrence, fetches rates,
or changes a checkpoint merely because the user opens or refreshes it.

### Required for core R10 acceptance

- `/app/forecast`, linked from the main navigation, with URL-addressable filters.
- A recorded opening balance and daily future balance series, exact per account
  and currency; an exact same-currency aggregate across the selected accounts.
- Separate contributions from future posted entries, generated recurring
  drafts, and not-yet-generated recurring dates. Both a **recorded-only** and
  **with-recurring** balance are shown; this is not an unlabeled confidence band.
- Default scope: active liquid-cash posting accounts. Explicit account selection
  also supports currency-denominated assets/liabilities, including credit cards.
- Default horizon 90 days; 30/90/180/365-day presets and integer custom 1–366 days.
- Overdue/today unposted recurring assumptions moved to tomorrow, visibly
  identified; their saved dates remain untouched.
- Lowest daily balance, its earliest date, and first negative date for each
  cash-account/currency series, plus an explanation of contributing events.
- Optional combined total using **constant FX at the forecast's as-of date**;
  missing conversion coverage never becomes a partial headline total.
- Loading, no-account, no-movement, error, incomplete-assumption and success
  states; desktop/mobile, keyboard, light/dark, all six shipping locales.
- A read-only, coherent snapshot, bounded computation, named tests and explicit
  disclosure of excluded/uncertain assumptions.

### Deliberately excluded

| Excluded from core slices 1–8 | Why / later home |
|---|---|
| Automatic posting or a forecast “commit” button | R9 owns explicit review/post/discard. |
| Manual what-if adjustments, saved scenarios, probability ranges | Requires scenario identity and editing semantics; no need for baseline v1. |
| Historical averages and learned variable/seasonal spending | Approved in the separate `forecast-learning-plan.md` extension M1–M4, after core acceptance. Automatic inferred bill creation remains excluded. |
| Loan amortization, interest accrual, card due-date/payment calculation | Optional later work, not something inferred from an account balance. |
| Forecasted FX, securities/crypto/reward prices, portfolio returns | Only currency postings and a labeled constant-FX assumption are in scope. |
| Forecast snapshots, exports/CSV, print layouts, notifications | Future work only; existing ledger exports remain actual posted facts. |
| Arbitrary historical/future as-of dates | v1 means “from today, using current records,” not knowledge-time reporting. |
| Budget integration, RRULE or banking calendars | R8 or a later recurrence/projection decision. Lightweight calendar expense profiles are in the learning extension; known daily/weekly/monthly/yearly schedules reuse R9. |
| New storage tables, workers, provider calls or schedule mutation | This feature computes read models from existing records. |

The defaults below are design decisions for this plan, not findings from user
research. Revisit them explicitly at acceptance; do not silently change them.

## 2. Verified starting points and reuse limits

| Existing code/document | Reuse or constraint |
|---|---|
| `backend/internal/recur/schedule.go`, `schedule_test.go`, `calendar_bounds_test.go` | Pure date enumeration, clamping/re-anchoring, count-from-anchor limits, year bounds; 4,000 occurrences per call. Never create another schedule enumerator. |
| `backend/internal/app/recurring.go` | `recurringSchedule`, schedule validation, template mapping and revision semantics. `cleanTemplate` performs repository reads; do not call it once per forecast date. |
| `backend/internal/app/recurring_generation.go` | Watermark rules and 50-write/4,000-day catch-up bounds. **Do not call `GenerateDue` from a forecast.** |
| `backend/internal/db/recurring.go`, `recurring_review.go`, `recurring_generation.go` | Current templates/postings, acted-on occurrence identities, current draft links. The paginated due inbox is not a complete forecast input source. |
| `backend/internal/db/ledger.go` | Effective-dated account resolution; current-version posted/non-deleted posting basis. Existing readers use their own repository pool: copying their calls into a loop is not a coherent snapshot. |
| `backend/internal/app/ledger.go`, `cashflow.go`, `reports.go` | Exact balance types, named liquid-cash kinds, descendant resolution and sign conventions. Reports describe actuals; do not add assumptions to their queries. |
| `backend/internal/db/exports.go` and `backend/cmd/rekenraam/command.go` | Read-only pool and snapshot pattern. Reuse the existing `readOnlyDatabase`; no second database or pool per request. |
| `backend/internal/exact/{coefficient,scaled,rate}.go` | String coefficients, `ScaledInt`, 38-digit boundary, `MulDivRound`. No SQL `SUM(quantity_value)`, floats, integer casts or `int64` accumulation. |
| `backend/internal/app/valuation.go`, `backend/internal/db/pricing.go` | Existing 7-day nearest-earlier rate policy and deterministic stored observation ordering. `NewRateTable` reads outside the forecast snapshot and can query per missing currency; do not directly use it here. |
| `frontend/src/lib/api/reports.ts`, `recurring.ts` | Typed client/error/query conventions. Generated source is `frontend/src/lib/api/schema.d.ts`. |
| `frontend/src/lib/reports/report-series.ts`, `bucket-column-chart.svelte` | BigInt-based presentation geometry and accessible single-currency chart pattern. Graph ratios may be numbers; money cannot. |
| `frontend/src/lib/money/{amount,format}.ts` | Exact input/formatting boundary. No frontend recomputation of financial projection totals. |
| `frontend/src/routes/app/+layout.svelte` | Navigation, title/section state and shell descriptions; update all route branches. |
| `backend/internal/api/server.go`, `health.go`, `setup_test.go` | Service struct, route registration, test-handler wiring. Inspect setup fixture before adding a service dependency. |
| `api/openapi/openapi.yaml`, `paths/`, `components/schemas/`, `api/bruno/` | Modular OpenAPI-first contract and executable request examples. |
| `docs/reviews/r9-acceptance-review-2026-08-31.md` | R9 closure; T-82 catch-up, T-83 FX isolation, T-84 editor reset, real-producer tests. |
| ADRs 0004, 0009, 0010, 0012 | SQLite snapshots, exact precision, no draft/forecast FX demand, no implicit investment/ledger effects. |

### Proposed files (not present at planning time)

- Backend: `backend/internal/db/forecast.go`, `forecast_test.go`;
  `backend/internal/app/forecast.go`, `forecast_inputs.go`, `forecast_projection.go`,
  `forecast_rates.go`, corresponding `*_test.go`; `backend/internal/api/forecast.go`,
  `forecast_test.go`.
- API: `api/openapi/paths/forecast-balances.yaml`, `forecast-balance-events.yaml`,
  `api/openapi/components/schemas/forecast.yaml`; Bruno `forecast-balances.bru`
  and `forecast-balance-events.bru`.
- Frontend: `frontend/src/lib/api/forecast.ts`, `forecast.test.ts`;
  `frontend/src/lib/forecast/forecast-screen.svelte`, `forecast-model.ts`,
  `forecast-model.test.ts`, `forecast-chart.svelte`, `forecast-events.svelte`;
  thin route `frontend/src/routes/app/forecast/+page.svelte`.
- Browser: `e2e/playwright/forecast.spec.ts`.
- Closure: `docs/reviews/r10-acceptance-review-YYYY-MM-DD.md` (actual close date).

Add a migration **only if measured query-plan evidence requires an index**.
Do not preassign a migration number; use the next free number at implementation.
Never add persisted forecast rows to avoid writing the projection algorithm.

## 3. Financial and date decisions

### D1 — Dates and opening balance

Capture `nowUTC` once per request using an injectable service clock. Read the
owner's saved IANA time zone inside the snapshot and compute `as_of_date = A`.
Reuse the existing settings fallback for a missing preference; do not use the
browser's or host process's time zone. Do not accept caller-supplied `as_of`.

Let `H` be the horizon, `F = A + 1 calendar day`, and `E = A + H calendar days`.
There are **H future points** for F through E, inclusive, plus a separate
opening balance at A. Addition is calendar arithmetic, not 24-hour durations.
If E exceeds `9999-12-31`, reject the request; do not shorten H silently.

`opening[a,c]` is the exact sum of **current-version, posted, non-soft-deleted**
postings on account a and currency c whose **journal entry date ≤ A**. It is
“recorded through today,” not an intraday bank balance and not a historical
snapshot of what was known on A. Voided/superseded versions do not contribute.
Do not add an account opening-balance field: opening balances are postings.

Future posted entries with `A < entry_date ≤ E` contribute on their entry dates.
The transaction header date, creation timestamp, reconciliation status and
current register page are not this filter. An old transaction with a future
entry must be included; a future header with an already-dated entry must not be
counted twice. Future posted investment transactions contribute their selected
**currency** legs without valuing securities or reading lots.

### D2 — Account scope and currency scope

With no `account_id`, resolve active, non-system, posting-enabled accounts in
classes asset/liability whose kind is `cash`, `checking`, `savings` or
`brokerage_cash` (the current cashflow default). Echo the actual resolved IDs.
Do not silently broaden to all assets or all account descendants.

With explicit IDs, validate each selected root as an existing non-system asset
or liability account as of A. With `include_descendants=true` (default), expand
its tree **as of A**, deduplicate, then keep eligible posting accounts. With
false, keep only selected roots that allow postings. A group-only selection
with no eligible descendants is a valid empty scope, not “all accounts.”
Unknown/foreign-book/system/income/expense/equity IDs are 400. Explicitly closed
asset/liability accounts may be inspected; label them closed. Exclude
`security_holding` accounts from currency-account forecasts, even if selected
explicitly (400 for an explicitly selected posting account of that kind).
Do not suppress a recorded balance just because its account closed later.

Account membership and display labels are frozen as of A for this request;
future reparenting never moves balances between the selected scope. Known future
account/commodity lifecycle rules still apply to unposted assumptions (D5).
Include only commodity kind `currency`. Every currency with opening/activity in
the scope gets a series, even if its net is zero. Also include a zero series for
an eligible account's default currency when it has no activity. A multi-currency
account with neither default currency nor activity has no invented currency;
return it in resolved account metadata with no series.

There is no request-level currency filter in v1. Currency selection is a local
presentation choice over the response, so a hidden currency cannot alter the
account scope or combined total. All-accounts totals mean **the resolved selected
accounts**, never a whole-book net-worth claim.

### D3 — Recurrence identity and precedence (the critical table)

Use `(template_id, occurrence_date)` as identity. Never match recurring entries
by amount/payee/description/date similarity or by `transaction_date` alone.
First load durable occurrence identities, then consider computed dates.

| Source/state | What contributes? | Does its identity suppress a computed template date? |
|---|---|---|
| No occurrence row, enabled/unarchived template, date ≥ `max(starts_on, generate_from)` | Current template postings as an assumption, subject to D4/D5. | No stored identity; create an in-memory event only. |
| Generated occurrence linked to current draft | Current saved draft entries/postings, not original template amount/date. Include even if template is paused/archived. | Yes, always, including if edited outside the horizon or excluded as invalid. |
| Generated occurrence linked to posted transaction | Only the ordinary posted ledger basis, including edited entries on their actual dates. Never add the template or a second “recurring posted” amount. | Yes, regardless of whether its posting date lies in this horizon. |
| Generated occurrence linked to voided or soft-deleted transaction | No recurring amount. Ordinary ledger rules already exclude it. | Yes; do not resurrect cancelled money. |
| Skipped occurrence, including discarded draft tombstone | Nothing. | Yes. |
| Blocked occurrence | Nothing; emit an excluded-assumption diagnostic with review destination. Do not project a repaired template over a blocked identity until the owner explicitly retries. | Yes. |
| Generated row with missing link/current transaction (not a legitimate skipped tombstone) | No amount; diagnostic `broken_occurrence_link`. | Yes; never invent a replacement. |
| Paused or archived template with no acted-on row | No new assumption. Existing generated drafts remain governed above. | Existing rows still suppress identities. |
| A non-recurring draft from another producer | Nothing; outside R10 v1's sources. | Not applicable; do not guess its origin. |

The generator watermark suppresses **computed** history, not already-linked
saved drafts. Do not filter drafts by the current watermark or enabled flag.
`lead_days` determines materialization timing only; it does not limit the
forecast horizon. `max_occurrences` counts from the original anchor, not from A.
Changing schedule fields may reset the watermark, but existing identities keep
their original authority even when no longer on the new schedule.

Materialization without changing saved financial values must leave the projected
curve unchanged. Posting an unchanged future draft must also leave the projected
curve unchanged, while moving its contribution from draft to posted. Posting a
backdated/today draft changes the opening and removes its tomorrow assumption;
that is an expected change of timing, not a double count.

### D4 — Overdue and today assumptions

For every eligible unposted journal entry with saved/scheduled date D, use
`projection_date = max(D, F)`. Preserve `source_date = D`; set
`carried_forward = (D < F)`. Include overdue materialized drafts and unmaterialized
dates from the watermark, not just dates after A. This prevents scheduler lag
or a restart from making the same expected payment disappear from the forecast.

All carried-forward entries are assumed on tomorrow together. Show a prominent
count and explanation: **“Unposted entries due today or earlier are assumed
tomorrow. Their saved dates have not changed.”** Do not call this a prediction
of a bank's processing date. Never spread the backlog across days or skip it.
An edited draft can have several journal entries: project each at its own date.
For computed templates, the occurrence date is the source entry date.

### D5 — Invalid and blocked assumptions

Forecast inclusion does not certify that a draft can be posted: reconciliation
and all current write guards still apply when the owner posts it in R9.
Nevertheless, do not pretend an incomplete or financially invalid draft is a
balanced future event.

Before account filtering, validate an entire draft transaction or template
occurrence using its **full posting set**, including category and clearing legs:
valid coefficients/scales, known references, supported ordinary/transfer kind,
at least two postings per entry, no security/non-currency legs, and exact
per-entry per-currency balance. An invalid entry excludes the entire draft
transaction from assumptions, not just its inconvenient side. Overflow is a
request-level error, not a skippable warning.

Use a pure forecast eligibility helper over snapshot records for known account
and commodity lifecycle/precision constraints. Inspect
`TransactionService.cleanPosting` and `validateBalanced` in
`backend/internal/app/transactions_validate.go`, plus the repository
`PostingAccountRule` and `PostingCommodityRule` lookups.
Reuse the existing pure balance helper where its types permit. Do not call
transaction creation, `prepareCreateTransactionForWrite`, payee creation,
`cleanTemplate`, or per-posting repository lookups from this helper.

Resolve rules at each saved source entry date, consistent with the producer.
For carried-forward entries, also require that the account/currency can accept
an ordinary posting at F; a payment through an account already closed today is
not a reasonable tomorrow assumption. For future template dates, known opening,
closing, archive, posting-allowed and currency-scale changes take effect at that
date. Keep the selected reporting account set fixed under D2. The eligibility
helper must explicitly check active/posting-enabled accounts,
opened/closed dates, equality to a non-null account default commodity, the
account scale override, active commodities, commodity-kind scale ceiling and
commodity max scale. Currency-only system clearing counterpart postings are
allowed; the reporting scope exclusion of system accounts is not a ban on
validating those counterpart legs. Do not revalidate posted facts against later
rules or drop their money.

Only model financial structure/lifecycle/precision here. Payee spelling, tags,
notes, audit metadata and reconciliation readiness are not forecast validators;
copying the entire transaction writer would be both wrong and fragile. Add
parity fixtures for the financial checks above and document this narrower scope.

Emit stable diagnostic codes, reference IDs/dates and counts; translate them in
the UI. Do not send raw database/validation error strings as UI copy. Diagnostics
are scoped to candidate events whose full posting set touches a resolved account
and whose projected date could fall in the horizon. If an invalid/broken draft
has lost all usable account references, fall back to its template's references
for diagnostic scope only, never for financial contributions; count one
excluded event if no usable entry remains. A blocked linked template
with an unreadable posting set gets one scoped/template diagnostic when its
known account references touch the scope; do not invent amounts.

`assumptions.complete=false` if a relevant expected item was excluded because
it is blocked, invalid or broken. Valid but deliberately skipped/paused future
items do not make this flag false. Carried-forward items are included and get
an informational diagnostic. The screen may show usable modeled totals with
these warnings, but never call them a complete account of future spending.

### D6 — Exact arithmetic, signs and daily identities

All quantities are `{quantity_value: canonical string, quantity_scale: integer}`.
Keep debit-positive signs: assets usually positive, liabilities usually negative.
A debt payment adds a positive posting to a liability and reduces the amount
owed. Do not `abs()` liabilities before aggregation; label the convention.

For each `(account_id, commodity_id)` choose one scale for the whole response:
maximum scale among its included opening postings and future contributions,
or default currency/account display scale for a zero-only series. Accumulate
with `exact.ScaledInt`; derive response coefficients only at the checked 38-digit
boundary. Do not round exact source series down to two decimals.

For each future date d:

```text
recorded_balance[d] = recorded_balance[d-1] + posted_delta[d]
projected_balance[d] = projected_balance[d-1]
                     + posted_delta[d] + draft_delta[d] + template_delta[d]
recorded_balance[A] = projected_balance[A] = opening_balance
```

Emit all days, including zero movement. Same-currency aggregate series use the
same identities over the exact sum of selected account components. Summing a
parent and child twice is forbidden. Full-scope internal transfers net to zero;
a transfer across the selection boundary changes the selected balance. Never
classify a transfer as income/spending here: this is a balance movement model.

Find the minimum over the opening point and every projected day, choosing the
earliest date on ties. `first_negative_date` is the earliest negative projected
point **including A**, but is only populated for cash-kind asset series; for
liabilities/non-cash series it is null. Label this “negative recorded/projected
cash balance,” not an overdraft fee, credit limit or available funds prediction.

### D7 — Constant FX is an assumption, not future market data

Conversion is optional and additive. Require both `reporting_currency_id` and
`fx_method=constant_as_of`; reject either on its own or any other method.
Do not reuse the historical method name `observed_on_or_before` to describe the
forecast policy. That is the **rate selection rule inside** constant-as-of.

Read stored non-voided source→reporting-currency observations inside the same
snapshot. For each needed currency, choose the latest valuation date in
`[A-7 calendar days, A]`; tie-break by latest `recorded_at`, then highest ID,
as the current reporting reader does. Clamp the lower bound at year 1. Echo
observation ID, source/quote currency IDs, valuation date, recorded timestamp,
price numerator/base quantity with their scales, and derived flag. A derived
stored observation is usable; do not synthesize inverses/cross rates or call a
provider. A same-currency conversion is exactly 1 with no observation.

Hold those rates fixed for **every future day**, even if the database already
contains observations with future valuation dates. Staleness is measured
against A, not the future day. A rate three days before A is stale but usable;
a rate eight days before A is unavailable. Refreshing later may choose a new
rate because this is a live current projection, not a stored reproducible run.

Convert only the selected-scope aggregate, not each account independently.
For each source currency, round the opening aggregate and each day's posted,
draft and template delta separately to the reporting currency's standard scale
using `exact.MulDivRound` (half away from zero). Sum those rounded components;
derive converted recorded/projected balances using D6's recurrence. Name this
`rounding=per_currency_component_half_away_from_zero` in the response. A converted
closing balance may differ from converting the exact closing balance once;
this is intentional so the displayed opening/movements/closing reconcile.
Do not independently convert and round the displayed closing balance.

Needed currencies are those with a nonzero opening or any nonzero component in
any **account** series, before same-currency aggregation/netting. This prevents
opposing account balances hiding missing currency coverage. Zero-only series
do not require an external rate. If any needed currency lacks a rate, return
`valuation.complete=false`, structured gaps, and **no converted series at all**.
Keep every exact per-currency series. Never substitute zero, a one-to-one rate,
a future observation, or an unlabelled partial total.

### D8 — No read side effects; one coherent basis

Read the owner zone, all known account/commodity versions, posted postings, templates,
occurrence identities, linked current drafts and optional rates under one
read transaction on the existing read-only pool. A forecast may be entirely
before or entirely after concurrent materialization/posting; it may not combine
before-and-after states and double count them.

Materialize the bounded input records, finish the read transaction, then do
pure computation and response serialization. Do not hold a read transaction
while a browser pages event details or while making network requests. A request
causes no audit event, background item, occurrence, template revision, watermark,
transaction, reconciliation or investment-lot change. Auth middleware behavior
is separate; financial read-only tests compare domain state, not session expiry.

## 4. Backend read model and algorithm

### 4.1 Ownership and proposed interfaces

Create `app.ForecastService` and `db.ForecastRepository`; do not enlarge
`TransactionService` with forecast orchestration. Construct the repository from
`readOnlyDatabase` in `command.go` and wire a `Forecast` field into `api.Services`.

Suggested public service seam (these are proposed types, not existing APIs):

```go
type ForecastInput struct {
    OwnerUserID        int64
    HorizonDays        int
    AccountIDs         []int64
    IncludeDescendants bool
    ReportingCurrencyID *int64
    FXMethod           string
}
// Service has an injected now func() time.Time, defaulting to time.Now.
// Balances and Events call the same input loader and pure projection builder.
func (s *ForecastService) Balances(ctx context.Context, in ForecastInput) (ForecastResult, error)
func (s *ForecastService) Events(ctx context.Context, in ForecastEventsInput) (ForecastEventsResult, error)
```

Keep database row types in `db`, financial projection types in `app`, wire DTOs
in `api`, and frontend aliases derived from OpenAPI. Explicitly parse omitted
`include_descendants` as true in the handler; do not let a Go zero-value bool
silently change the default. `ForecastInput` receives normalized values.

The repository exposes a snapshot callback with a concrete reader carrying its
`*sql.Tx` privately. Every reader method queries that transaction. The app
orchestrates methods and decides scopes/dates; SQL stays in `db`. Do not make a
new general repository/transaction framework for the rest of the app.

```text
ForecastRepository.WithSnapshot(ctx, callback(reader))
  reader.OwnerTimeZone(bookID, ownerUserID)
  reader.AccountVersions(bookID)
  reader.CommodityVersions(bookID)
  reader.PostedPostings(bookID, resolvedAccountIDs, throughDate)
  reader.RecurringInputs(bookID, resolvedAccountIDs, throughDate)
  reader.RatesAtOrBefore(bookID, quoteID, neededCurrencyIDs, asOf)
```

Names above are proposed and may be split into private bulk queries; their
semantic boundaries and single-snapshot behavior are required. Load account
versions including `effective_from`, `version_seq`, status, parent, class/kind,
posting permission, opened/closed dates, default commodity and scale override.
Load commodity versions including identity, kind, status and precision fields.
The existing `LedgerAccountRecord` omits several of these; do not assume it is
sufficient. Carry stable built-in keys/metadata needed for localized labels.

### 4.2 Bulk read recipe

1. Begin the read transaction; read owner preference and bind A/F/E once.
2. Read all known account/commodity version data in bulk, including stored
   future-effective versions beyond E. Whole-draft validation includes entries
   beyond the horizon, so loading rules only through E is insufficient. The
   version-row budget still applies. Resolve selected account IDs using D2. Empty resolved scope returns a true
   empty result; never pass an empty ID list into a query meaning “all.”
3. Read selected-account currency postings from current posted/non-deleted
   transactions through E. Preserve entry/transaction/version IDs and entry
   date. Sum the prefix through A in Go, keeping future entry groups. No report
   balance endpoint per account/date, and no paginated transaction-list reader.
4. Identify relevant templates by the union of (a) current full template
   postings touching resolved accounts and (b) current linked recurring draft
   postings touching resolved accounts. The second branch retains drafts after
   template edits/archive or moving their accounts away from the original.
5. Bulk-load those templates' current specs and full postings; all relevant
   occurrence identities needed to suppress dates from their generation
   watermark through E; **also** old blocked rows and all linked current drafts
   whose entry dates could project in F..E. A draft edited into the horizon
   from an occurrence originally after E must still be read. A generated row
   outside the entry-date horizon can still suppress its original computed
   date, so occurrence loading cannot be joined only to in-window drafts.
6. Load complete current entries/postings for each relevant draft transaction,
   including counterpart accounts outside the selected scope and entries
   outside the horizon, so whole-draft financial validation is possible. Load
   needed linked payee labels in bulk; no query per row. Never use a listing's
   default 50/100/200-item page limit for these internal reads.
7. If conversion is requested, load stored rates in bulk for the superset of
   candidate currencies present in loaded selected postings/template/draft
   inputs. The pure builder later determines the exact needed set under D7;
   unused candidates must not create gaps or enter selected-rate provenance.
   A grouped/window SQL query may choose the latest observation per source
   currency at or before A, then the service checks 7-day age. Keep old nearest-date metadata for a
   gap. Filtering/tie-breaking must match D7; only currency→currency pairs.
8. End the read transaction. No financial data is changed on this path. All
   candidate rates are already in memory before pure computation; never reopen
   a snapshot to fetch a rate discovered during calculation.

A few bulk queries are acceptable; a query per day/account/template occurrence
is not. Tests must cover coherent reads across concurrent generation/posting.
Do not call existing repository methods bound to the writer pool inside the
snapshot and assume they share it. Avoid `SELECT *` on financial joins.

### 4.3 Pure projection recipe

Implement a deterministic builder over the loaded snapshot. It must have no
clock, database, network, logging side effects, or mutation-service dependency.

```text
resolve and validate normalized scope, date bounds and budgets
build immutable account/commodity rule lookup tables
initialize exact opening[account,currency] from posted prefix through A
index acted-on dates by (templateID, occurrenceDate), all statuses

for each future posted entry:
    add a posted event on entry_date using selected currency postings

for each linked current recurring draft:
    validate full transaction before filtering
    if invalid: diagnose once, suppress its original occurrence anyway
    else for each entry with source_date <= E:
        add a draft event on max(source_date,F), retain original dates/IDs

for each enabled, unarchived relevant template:
    from = max(starts_on, generate_from)
    enumerate through E using internal/recur in <=4,000-calendar-day chunks
    count every examined candidate against the budget
    for each date not in the acted-on index:
        validate full template occurrence using snapshot rules
        diagnose invalid candidate, otherwise add template event on max(date,F)

merge and deterministically sort events
choose stable scales, aggregate daily components, emit every future date
derive recorded/projected balances and summary dates
build same-currency selected-scope totals; optionally derive constant-FX series
compute basis_token from canonical relevant inputs + normalized recipe
```

Do not call `recur.Occurrences` once over decades of downtime (T-82), reset the
series anchor for each chunk, or silently stop after the first chunk. Preserve
`max_occurrences` counting from `starts_on` in every chunk. Check context
cancellation in bounded loops. An empty chunk advances its enumeration cursor.

Events represent journal entries, not complete transactions: one posted/draft
transaction with multiple dates has multiple events. For templates there is
one synthetic entry per date. Give each event a deterministic key:

- `posted:<transaction_id>:<version_id>:<entry_id>`;
- `draft:<occurrence_id>:<version_id>:<entry_id>`;
- `template:<template_id>:<occurrence_date>`.

Sort by projected date, source order `posted,draft,template`, numeric source ID,
entry ID (zero for synthetic), then occurrence date. Do not rely on map order,
lexicographic decimal-ID ordering, account-register ordering or translated names.
Daily balances have no intraday ordering claim. An event carries only its
selected-account currency amounts in the response, but was validated with its
full counterpart set. Internal transfers remain visible in event details even
when their selected-scope total is zero.

### 4.4 Explicit budgets and failure behavior

Define these named service constants and document them in OpenAPI. They bound
v1; never let a `LIMIT` turn them into an apparently complete prefix response.

| Budget | v1 limit | On exceeding |
|---|---:|---|
| Requested root account IDs | 100 unique | 400 validation |
| Resolved posting accounts | 200 | 422 `FORECAST_TOO_LARGE` |
| Account/commodity version rows loaded, combined | 100,000 | 422 |
| Selected posted posting rows through E | 1,000,000 | 422 |
| Relevant templates | 2,000 | 422 |
| Relevant durable occurrence rows | 100,000 | 422 |
| Full draft/template posting rows loaded, combined | 250,000 | 422 |
| Examined recurrence candidates plus saved future/draft entry events | 20,000 | 422 |
| Output daily points across account, currency-aggregate and converted series | 200,000 | 422 |
| Event detail page | default 50, max 200 | invalid limit: 400 |
| Diagnostics displayed in balances response | first 50 in stable order | return exact total/hidden counts; financial computation still considers all |

Use limit+1 or checked streaming counters to detect overflow. Never truncate
opening-history inputs or exclude old assumptions to pass a limit. The response
has no success totals on a size error; show “Narrow the selected accounts or
forecast range.” For large opening history, narrowing the horizon alone may
not help; do not promise it will. Budget checks happen before large allocations.
No hard wall-clock performance threshold in a correctness test; add benchmarks
for 365 daily dates × 50 templates and large posted prefixes. Review query plans
for the actual bulk joins before adding indexes.

## 5. HTTP contract (OpenAPI first in slice 3)

### 5.1 Balance endpoint

`GET /api/v1/forecasts/balances` — authenticated, read-only, no CSRF requirement,
private/no-store response. Owner/book authorization follows existing handlers.
Unknown `/api/` routes continue to return API 404, not the static app shell.

| Query | Contract |
|---|---|
| `horizon_days` | Integer 1–366; default 90. Empty, zero, negative, decimal, overflow or repeated scalar: 400. |
| repeated `account_id` | Positive integer IDs; deduplicate/sort before computation and keying; absent means named default scope. |
| `include_descendants` | Exactly `true` or `false`; default true. Echo it even with default scope. |
| `reporting_currency_id` | Optional existing currency ID; missing/non-currency: 400. Never default silently to the book/account currency. |
| `fx_method` | Required with reporting currency; only `constant_as_of`. Both omitted means no conversion. |

Reject unknown parameters, including `as_of`, `include_drafts`, `auto_post`,
`scenario_id` and arbitrary `fx_method`. There is no drafts-off toggle: the
recorded-only comparison already supplies that view without changing identity
precedence. Present custom range as horizon days, not two ambiguous date inputs.

Response objects (field names are the implementation contract):

```text
ForecastBalancesResponse
  as_of_date, start_date, end_date: ISO date
  time_zone: IANA name
  computed_at: captured UTC RFC3339 timestamp
  horizon_days: integer
  basis_token: lowercase SHA-256 hex (see 5.3)
  policy_version: "recurring_balance_v1"
  scope: {mode:"default_cash"|"selected_accounts", requested_account_ids:[],
          resolved_account_ids:[], include_descendants:boolean,
          accounts:[ForecastAccount], account_options:[ForecastAccount]}
  currency_options: [ForecastCommodity] // all as-of reporting-currency choices
  series: [ForecastAccountSeries]       // sorted account_id, commodity_id
  totals: [ForecastCurrencySeries]      // same selected scope, by commodity_id
  assumptions: {complete:boolean, carried_forward_event_count:integer,
                excluded_event_count:integer, source_event_counts:{posted,draft,template}}
  diagnostics: [ForecastDiagnostic]     // up to 50, stable order
  diagnostic_total_count, diagnostic_hidden_count: integer
  valuation: null | ForecastValuation
  converted: null | ForecastConvertedSeries
```

`ForecastAccount`: id, parent_account_id (nullable), name (nullable), built-in
label key/code when applicable, account_class, account_kind, status,
allows_postings, default_commodity_id (nullable). `account_options` lists valid
as-of selectable non-system asset/liability roots and groups; excludes security
holding posting accounts. It is backend-composed so the page does not fetch
one account per series. Its rows count against the metadata budget.

`ForecastCommodity`: id, code and standard_scale. `currency_options` lists all
as-of currencies accepted as a reporting currency so the composed page does not
issue a separate catalog request merely to populate its filter.

`ForecastQuantity`: quantity_value (string), quantity_scale (integer).
`ForecastAccountSeries`: account_id, commodity_id, commodity_code, opening_balance,
points, minimum_balance, minimum_date, first_negative_date (nullable).
`ForecastCurrencySeries`: the same without account_id or first_negative_date.
`minimum_balance` uses the earliest tie over opening+future points.

Each `ForecastPoint` contains date, posted_delta, draft_delta, template_delta,
recorded_balance and projected_balance as `ForecastQuantity`. Also include
`source_event_counts:{posted,draft,template}` and `carried_forward_event_count`.
Counts in a currency aggregate count unique contributing entry events, not
postings; an internal transfer counts once even though it touches two accounts.
Do not sum per-account counts to derive aggregate counts.

`ForecastDiagnostic`: code, severity (`info` or `warning`), template_id (nullable),
occurrence_id (nullable), transaction_id (nullable), occurrence_date (nullable),
source_date (nullable), projected_date (nullable), event_count. Codes:
`carried_forward`, `blocked_occurrence`, `invalid_draft`,
`invalid_template_occurrence`, `broken_occurrence_link`.
Group carried-forward info into one count; invalid drafts once per transaction;
other exclusions once per occurrence. `diagnostic_total_count` counts these
normalized diagnostic records before the display cap; `excluded_event_count`
counts excluded projected journal-entry events (an invalid whole draft counts
its entries that would touch the scope/horizon; broken/blocked identity counts
once; an invalid draft with no usable entry also counts once). Sort diagnostics
by code, template_id (null=0), occurrence_date (null=empty), occurrence_id
(null=0), then transaction_id (null=0). For a whole-draft diagnostic use the
earliest relevant entry date/projected date, or null when unreadable.
Counts and `complete` are calculated before limiting diagnostic display. Do not expose financial content in logs.

`ForecastValuation`: method, rate_selection=`observed_on_or_before`, as_of_date,
reporting_currency_id/code/scale, max_staleness_days=7, rounding (D7), complete,
used_rates ([ForecastRateUse]), gaps [{commodity_id, reason,
nearest_observation_date nullable}]. Use `no_observation_in_window` for a gap;
nearest date distinguishes absence from an old rate. `converted` follows the
aggregate-series shape in the reporting currency and has no first-negative
cash alert. If valuation is incomplete, converted is null for the whole range.
`ForecastRateUse` fields are observation_id, base_commodity_id,
quote_commodity_id, valuation_date, recorded_at, price_value (string),
price_scale, base_quantity_value (string), base_quantity_scale, is_derived and
stale. Existing price rows use int64 coefficients; convert them losslessly to
strings/big.Int, not through a floating-point intermediate. Same-currency
identity conversion needs no `used_rates` row.

All arrays are present as `[]`, not null. Optional objects/IDs are explicitly
nullable, not ambiguous missing fields. `assumptions.complete` and
`valuation.complete` describe different things; neither means “the future is
certain.”

### 5.2 Event-detail endpoint

`GET /api/v1/forecasts/balance-events` — one request for the expanded day panel,
not a request per row/account. Same auth/no-store and recipe parameters as
balances, plus:

- `date`: required projected date in F..E;
- `basis_token`: required token from the displayed balance response;
- `detail_account_id`: optional single member of resolved accounts;
- `detail_commodity_id`: optional single currency present in the response;
- `limit`: default 50, maximum 200; `cursor`: optional opaque string.

Reuse exactly the same snapshot loader and projection builder. Filter its
included events by date/detail filters, then cursor-page the stable event order.
A day panel's per-account/currency totals must reconcile to that displayed day's
three components when all pages are read. The detail endpoint does not return
excluded diagnostics as zero-valued financial events.

Response: `{basis_token, date, items:[ForecastEvent], total_count, next_cursor}`.
`next_cursor` is null at the end; never use offset pagination. Event fields:
key, source (`posted|draft|template`), source_date, projected_date,
carried_forward, transaction_id/version_id/entry_id (nullable for templates),
template_id/occurrence_id/occurrence_date (nullable for non-recurring posted
entries), description, payee_name (nullable), and
`amounts:[{account_id,commodity_id,commodity_code,quantity_value,quantity_scale}]`.
Aggregate multiple postings within an entry to each selected account/currency
pair exactly, retaining zero-net pairs if needed to explain an internal entry.
Do not place arbitrary HTML or a backend-built URL in the response.

### 5.3 Stale details and cursor validation

Compute `basis_token` by SHA-256 over a deterministic encoding of normalized
recipe, policy version, A/time zone, resolved scope, relevant input versions,
postings, templates, occurrence statuses/links and selected FX observations.
Include metadata needed to display the result; exclude `computed_at`, session
IDs and irrelevant book rows. Sort maps/records before encoding. This is an
identity/freshness token, **not authentication, a stored snapshot or an audit ID**.

On event read, recompute the basis. If it differs from the caller's token, return
409 `FORECAST_BASIS_CHANGED`, no stale rows. The UI closes/clears that day's old
result and refetches balances, then asks the user to reopen details. It must
not silently append events from a new basis to an old page.

Encode cursors using the repo's existing opaque/base64 JSON pattern, containing
version, basis token, normalized detail filters/date, and last event sort tuple.
Reject malformed/version-invalid/filter-mismatched cursors with 400. A cursor
with an old basis gets 409. Validate decoded fields and cap encoded length at
4 KiB before decoding. Never use a token/cursor to bypass owner authorization.

### 5.4 Error mapping

| Condition | Status/code |
|---|---|
| Unauthenticated | 401 `UNAUTHENTICATED` |
| Invalid parameters, selection or date bounds | 400 `VALIDATION_FAILED` |
| Domain computation budget exceeded | 422 `FORECAST_TOO_LARGE` (new) |
| Exact result cannot fit coefficient contract | 422 `LEDGER_OVERFLOW` (reuse) |
| Event detail basis changed | 409 `FORECAST_BASIS_CHANGED` (new) |
| SQLite busy / infrastructure error | Existing `RESOURCE_BUSY` / `INTERNAL_ERROR` mapping |
| Blocked/invalid expected item or missing FX | 200 with diagnostics/coverage; never a fabricated total |

Register new error codes in OpenAPI and localized UI error mappings. Log a
request ID and safe operation label, never serialized postings, descriptions,
payees, account balances or the entire service input.

## 6. Worked fixtures — expected numbers, not screenshots

Use fixed clocks in Go tests. In browser tests derive dates from the server's
owner-local date; never paste these historical dates into a live e2e fixture.
Every setup operation must use existing services/public APIs where available;
use deliberate SQL corruption only to test corruption or broken-link defenses.

### Fixture A: sources, transfers, overdue dates and edited drafts

Owner date A = 2026-08-31, H = 5. EUR checking = 1,000.00; EUR savings = 500.00;
USD cash = 200.00, created through real opening-balance postings. Default scope
contains those three cash accounts. A separate EUR credit card owes 300.00 and
is not in the default scope. Assume ordinary balanced counterpart postings for
every item below; expense/income/clearing accounts are not selected cash.

| Input | Saved date | Forecast treatment |
|---|---|---|
| Posted EUR income +200 checking | Sep 1 entry date | Posted +200 on Sep 1; not in opening. |
| Generated EUR fee draft -25 checking | Aug 30 entry date | Draft -25 on Sep 1, carried forward; original date unchanged. |
| Posted transfer -300 checking / +300 savings | Sep 2 | Both account series move; aggregate EUR delta zero. |
| Generated rent draft, template says -100 on Sep 2; draft edited to -125 on Sep 3 | Sep 3 entry date, original occurrence Sep 2 | Exactly -125 draft on Sep 3; no -100 template event on Sep 2. |
| Skipped recurring -60 checking | Sep 4 occurrence | Nothing, no warning of incompleteness. |
| Posted card payment -100 checking / +100 card | Sep 4 | Default cash scope loses 100; explicitly selecting card too makes the transfer net zero. |
| Ungenerated recurring salary +1,000 checking | Sep 5 occurrence, lead 0 | Template +1,000 on Sep 5; forecast does not generate it. |

Expected EUR aggregate (all amounts scale 2 on the wire):

| Date | Posted delta | Draft delta | Template delta | Recorded-only balance | With-recurring balance |
|---|---:|---:|---:|---:|---:|
| Opening Aug 31 | — | — | — | 1,500.00 | 1,500.00 |
| Sep 1 | +200.00 | -25.00 | 0.00 | 1,700.00 | 1,675.00 |
| Sep 2 | 0.00 | 0.00 | 0.00 | 1,700.00 | 1,675.00 |
| Sep 3 | 0.00 | -125.00 | 0.00 | 1,700.00 | 1,550.00 |
| Sep 4 | -100.00 | 0.00 | 0.00 | 1,600.00 | 1,450.00 |
| Sep 5 | 0.00 | 0.00 | +1,000.00 | 1,600.00 | 2,450.00 |

EUR checking projected points: 1,175.00; 875.00; 750.00; 650.00; 1,650.00.
Savings points: 500.00; 800.00; 800.00; 800.00; 800.00. USD stays 200.00.
EUR aggregate minimum = 1,450.00 on Sep 4; no negative cash date. The final
EUR wire coefficient is `"245000"`, scale 2. No EUR+USD number exists unless
conversion is selected.

With USD→EUR rate 0.9 stored on Aug 31 and constant-as-of requested, converted
opening = EUR 1,680.00; projected points = 1,855.00; 1,855.00; 1,730.00;
1,630.00; 2,630.00. A USD→EUR observation dated Sep 2 at 1.2 must not change
any of those points. Voiding Aug 31's only eligible rate removes the **entire**
converted series, while exact EUR/USD series remain identical.

Materialize Sep 5's unchanged template using the real recurring service with a
suitable test clock/lead; its projected amount must not change. Its source moves
from template to draft. Post the unchanged future draft; source moves to posted
and recorded-only now includes it, but with-recurring remains unchanged.

### Fixture B: signs, minima and closed accounts

- EUR checking opening 100.00, future -120.00 on day 2: first negative day 2,
  minimum -20.00. Equal -20.00 on days 2 and 3 keeps day 2 as minimum date.
- Opening -10.00: first negative date is A, not F. Do not call it a new forecast
  shortfall; it is already recorded.
- Credit card opening -300.00, repayment +100.00: projected -200.00 and no cash
  first-negative alert. The UI explains a negative liability means money owed.
- Select parent plus child: child's opening and movements count once.
- Explicitly select a closed cash account: keep posted opening history; exclude
  an assumed payment through it tomorrow with a lifecycle diagnostic.
- Empty eligible account with EUR default: zero opening and H flat zero points.
  Account with no default and no currency activity: metadata only, no invented USD.

### Fixture C: precision and rounding

- Preserve `"9007199254740993"` at scale 2 through opening, a +1 coefficient
  delta, daily totals, event details and UI formatting. No JS number conversion.
- Mix 1.23 (123/2) and 0.004 (4/3): exact result 1.234 (1234/3).
- A result with 39 coefficient digits must return `LEDGER_OVERFLOW`; no wrapping,
  float fallback, exponent notation or partial successful response.
- Constant rate 0.5, source currency scale 2, quote scale 2, daily source
  components +0.01 posted and -0.01 draft: convert to +0.01 and -0.01, net zero.
  Keep symmetry on negative half ties.
- Two separate days with +0.01 source movement each at 0.5 convert to +0.01 each;
  converted closing increases 0.02 although converting 0.02 once gives 0.01.
  This is the named component-rounding policy, not an arithmetic bug.

## 7. Frontend behavior

### 7.1 Route and query state

Thin static route at `/app/forecast`, feature UI under `$lib/forecast`. Add a
navigation item “Forecast” adjacent to Recurring/Reports, an appropriate
existing Lucide icon, `aria-current`, translated document title and shell copy.
Do not add SvelteKit server routes, server loads or form actions.

URL parameters mirror the balances endpoint. Normalize sorted/deduplicated
account IDs and strict horizon/boolean values in `forecast-model.ts`. Invalid
URL input shows a recoverable filter error with a reset action; do not silently
replace a malformed value with a different financial question. Keep unrelated
URL parameters only if already supported by the app's navigation pattern.

Use `forecastQueryKey = ['api','forecasts']`; include normalized recipe in the
balances key and basis/date/detail filters/cursor in event keys. Initial page
data uses one balances request; authentication/navigation queries already owned
by the shell are not duplicated. Account options and available currencies come
from the response; no per-account/per-currency API calls.

Use a 5-second stale time, refetch on mount/window focus, and a visible Refresh
button. No forecast-owned scheduler or minute polling required in v1. Show
computed-at/as-of metadata so a long-open page never claims to be live. A refresh
that changes the basis clears open event pages. The server owns the date even
when browser and owner time zones differ. Reset returns to default scope/90 days
and removes conversion parameters.

### 7.2 Layout and explanation

1. Heading and one sentence: recorded entries plus recurring assumptions; no
   transactions are posted by this screen. Show owner-local as-of date.
2. Horizon, account selector with descendants choice, and optional reporting
   currency selector. Controls have Apply/Reset so typing does not issue an
   expensive request on every keystroke. Disable Apply only for invalid inputs.
3. Persistent assumptions notice when items are carried forward/excluded. Show
   exact counts and a link to `/app/recurring`; names/dates remain available in
   details, but do not invent unsupported deep-link parameters on R9 routes.
4. Account/currency summaries with opening, end recorded-only, end with-recurring,
   minimum/date and cash-negative indicator. Render negative liabilities with
   the same sign convention and explanatory copy, not absolute values.
5. One selected account/currency or same-currency aggregate chart, with a matching
   exact daily table. Provide “Recorded entries only” and “With recurring entries”
   labeled series. A two-line chart or paired columns is acceptable; the table
   is authoritative. Both curves share one scale computed from their union of
   values plus zero; never normalize each curve independently. Do not plot
   different currencies on a shared money axis.
6. If requested and complete, a separate “Combined at constant FX” view with
   rate date/staleness/rounding disclosure; if incomplete, a coverage notice and
   exact series only. Do not hide source-currency summaries when it is selected.
7. Expand a day's row via a real button to load event details. Details show
   source badge, description/payee, original and assumed date when different,
   and exact selected-account amounts. Load more using the cursor. Each day
   panel is one shareable component with its own pending/error/empty state.

Provide ordinary links to Recurring review for draft/template events and to the
Transactions screen for posted facts. If filtering Transactions, use its existing
filter helpers/contract; do not invent `transaction_id` deep links. No edit,
post, discard, skip, retry or create-template button is implemented inside the
forecast. Users perform those actions in the producing workflow, then refresh.

### 7.3 Explicit state matrix

| State | Required visible behavior |
|---|---|
| Initial loading | Named loading status/skeleton; no zero-valued forecast masquerading as data. |
| Refresh loading | Keep previous result with “Updating” status, but prevent new event detail requests against an obsolete basis. |
| No eligible accounts | Explain scope is empty; link to Accounts and offer Reset; no blank chart. |
| Accounts but no movements | Show flat balances and “No scheduled changes in this range,” not “No data.” |
| Some blocked/invalid assumptions | Show computed usable series plus persistent exclusion warning/count; never only a transient toast. |
| Missing FX | Keep exact series; no converted summary/chart; name missing currency and rate policy. |
| API/size error | Translated error, retry/reset/narrow-scope action. Do not keep an old chart labeled with new filter values. |
| Event basis conflict | Clear stale event pages, refresh balances, announce that records changed and details must be reopened. |
| Event page empty | Explain no included event for selected day/scope; do not present exclusion as a zero transaction. |
| Successful populated result | Explicit source breakdown and assumptions; exact accessible table available. |

Use semantic tokens. Keyboard order must reach every filter, series selector,
day expand button, source link and Load more action. Preserve focus on expansion
and refresh; do not move focus to the chart. At 390px the page itself must not
scroll horizontally; a labeled scrollable data table may scroll within its own
container. Charts convey nothing absent from the table and do not rely on color
alone. Respect reduced motion; no animation is required.

### 7.4 Localization and cache integration

Before introducing strings, add needed terminology to
`docs/localization-glossary.md` for forecast/projected balance/recorded-only/
constant exchange rate/carried forward. Add `forecast_*` keys to all six app
catalogs; keep placeholders and key sets equal. Existing unrelated T-80 gaps
are not permission for new English-only keys or a reason to claim all locales
have been fully reviewed. Native terminology review remains separate.

Use existing money/date formatting and BigInt chart-geometry helpers; never
parse the displayed localized string back into arithmetic. Do not initialize
form fields in reactive effects that read their own edit state (T-84).

Where existing successful transaction/recurring/import/account/price mutations
already invalidate related queries, add the forecast prefix as appropriate;
inspect the actual mutation call sites before editing. Do not refactor the
whole cache architecture. Route mount/focus refetch and explicit Refresh remain
the safety net. No extra request per mutation is needed if the forecast is not
mounted. A cache invalidation does not count as generation or rate fetching.

## 8. Risks and deliberate implementation safeguards

| Failure mode | Required safeguard |
|---|---|
| Generated draft plus computed template both counted | Index every acted-on identity before enumeration; D3 tests. |
| Draft edited to another date disappears or counts twice | Load linked drafts by current entry dates, suppress original identity independently. |
| Posting during forecast creates a mixed snapshot | All domain inputs/rates through one read transaction; concurrent writer test. |
| Pausing a template hides existing drafts | Separate saved-draft branch from enabled-template branch. |
| Skipped/voided/deleted occurrence regenerates in projection | All terminal identities suppress computation without implying cash movement. |
| Large book returns plausible prefix totals | Checked budgets; no partial success and no list-reader default limits. |
| Closed account history disappears | Recorded facts use selected identity; only assumptions get eligibility checks. |
| Transfer breaks totals or debt sign flips | Direct selected-posting sums; no income/expense allocation or `abs()`. |
| Missing currency disguised by netting or zero | Determine FX coverage needs before cross-account cancellation; no partial converted series. |
| Future stored rate sneaks into forecast | Every lookup uses A, never projected day E. |
| Forecast read queues FX work | Read-only pool; no pricing/recurring mutation service references; counts test. |
| Event pagination disagrees with curve after edits | Basis token on every page and 409 refresh workflow. |
| Long-open page crosses midnight | Display server basis and refetch on focus/manual refresh; no browser date substitution. |
| Huge negative/minimum values lose precision in graph | Backend exact summaries, BigInt geometry; number only for bounded pixel ratios. |

## 9. Validation commands and working discipline

Commands run from repo root unless an explicit `cd backend` is shown. Narrow
feature tests first, then full relevant checks. Test names below are **required
new tests**, not claims that they already exist.

```sh
cd backend
# During backend development; match the tests added in the current slice.
go test ./internal/db ./internal/app ./internal/api -run 'TestForecast' -count=1
# Return to repo root before running repository wrappers.
cd ..
./scripts/test-backend.sh
pnpm --dir frontend run openapi:generate
./scripts/test-frontend.sh
pnpm --dir e2e test forecast.spec.ts recurring.spec.ts transactions.spec.ts accessibility.spec.ts
```

The browser command builds the single binary and uses its isolated test DB when
`E2E_BASE_URL` is unset. Run frontend checks and browser builds **sequentially**:
both regenerate Paraglide and SvelteKit output, and overlapping them caused
false failures during R9 acceptance. Backend race checks may run independently.
Use the documented browser executable override only if the environment needs it;
do not patch browser caches or point acceptance at a real owner's database.

For docs-only planning, do not run a full application test suite as ceremonial
validation. Check paths, examples, arithmetic, source references and
`git diff --check`; implementation slices must run their executable checks.

## 10. Ordered execution slices

All eight slices are initially unchecked. Completion requires code, tests,
documentation and a scoped commit; writing a function or passing one happy-path
test is not enough. Each slice records actual evidence in the execution table.

### Slice 1 — Coherent read-only forecast inputs

**Goal:** obtain complete bounded inputs under one snapshot without an endpoint.

**Read first:** existing `backend/internal/db/ledger.go`, `backend/internal/db/recurring*.go`, `backend/internal/db/exports.go`,
`backend/internal/db/pricing.go`, `backend/internal/db/settings.go`, `backend/internal/db/sqlite.go`; ADRs 0004/0009/0010.

**Files:** new `backend/internal/db/forecast.go`, `backend/internal/db/forecast_test.go`; existing migration files
only for inspection unless an index is justified.

1. Define forecast row types and snapshot reader; keep `*sql.Tx` private in db.
2. Implement preference/version/posted/recurring bulk reads from section 4.2.
   Use book filters on all core financial joins and explicit empty-set handling.
3. Return all current draft entries/postings needed for full validation, without
   losing counterparts to account filters. Include archived-template drafts.
4. Add counters/limit+1 handling and ensure rollback/close on callback error,
   query/scan error and canceled context. No reads through the writer pool.
5. Test concurrent writing between two reader operations with separate pools:
   after the first read establishes the snapshot, commit generation/posting
   from the writer and assert later reads still see the same old snapshot.
   A new snapshot must see the new state. Do not test only sequential reads.
6. Inspect query plans with realistic indexes. Add an additive index only if
   necessary; document exactly which join it serves.

**Required tests:** `TestForecastSnapshotReadsCurrentPostedVersionsOnly`,
`TestForecastSnapshotKeepsDraftsAfterTemplateArchiveAndAccountEdit`,
`TestForecastSnapshotLoadsFullDraftCounterparts`,
`TestForecastSnapshotSeesOneSideOfConcurrentPosting`,
`TestForecastSnapshotRejectsTruncatedInputs`,
`TestForecastSnapshotEmptyScopeNeverMeansAllAccounts`.

**Gate:** focused db tests plus backend wrapper. No new public route or UI.
Update `implemented.md` as internal read infrastructure only, and this table.
Suggested commit: `feat(forecast): add coherent read-only input snapshots`.

### Slice 2 — Exact per-currency projection and source precedence

**Goal:** make the service produce the worked balances without HTTP or UI.

**Files:** new `backend/internal/app/forecast.go`, `forecast_inputs.go`, `forecast_projection.go`,
tests; minimal shared pure helper extraction only when justified.

1. Define normalized recipe/result/event/diagnostic types and injected clock.
2. Resolve A/F/E, account scope and effective-dated lookups inside the snapshot
   orchestration. Reject invalid IDs/horizons/empty-as-all behavior.
3. Build opening from posted entry dates; apply D3's identity table before
   computing new template dates. Implement bounded catch-up enumeration.
4. Implement pure financial eligibility checks and stable exclusions. Compare
   shared account/commodity precision/lifecycle rules with the actual writer;
   do not silently broaden accepted assumptions or call write preparation.
5. Aggregate exact daily components, both balance curves, scales, totals,
   source counts, minima and negative cash dates. No conversion in this slice.
6. Implement deterministic event ordering and basis digest over normalized
   relevant inputs (exclude `computed_at`). Add calculation/output budgets.
7. Exercise real recurring generation and promotion in service fixtures;
   compare whole amount series before/after, not merely final balances.

**Required tests:** all P01–P18 and B01–B04 in section 11 applicable here.
**Gate:** fixture A exact balances and all identity cases pass; backend wrapper.
No transaction/report query is modified to include assumptions.
Suggested commit: `feat(forecast): calculate exact recurring balance projections`.

### Slice 3 — Authenticated balances and consistent event-detail API

**Goal:** expose per-currency forecasts and paginated explanations.

**Files:** new API handler/tests, two OpenAPI path files and schema group, Bruno
requests; `backend/internal/api/server.go`, `backend/internal/api/health.go`, `backend/cmd/rekenraam/command.go`, setup test
wiring; generated frontend schema and new typed `frontend/src/lib/api/forecast.ts` tests.

1. Write OpenAPI for the implemented **per-currency subset** of section 5,
   registering all schemas and new errors. Do not advertise FX parameters or
   conversion fields as supported before slice 4; add them atomically there.
2. Add `Forecast` service wiring with the existing read-only pool and correct
   cleanup/lifetime. Test handlers use the same read-only shape over test DBs;
   do not open a new connection pool per request.
3. Implement strict query parsing, auth, DTO mapping, no-store and safe errors.
   One service call per handler; no SQL/arithmetic in HTTP code.
4. Implement event filtering, page cursor encoding/validation and basis mismatch
   response. Share the projection code; do not write a second calculation path.
5. Add generated-type aliases, URL serialization, query keys and standard error
   conversion in the frontend API module. No user-facing English in that module.
6. Add Bruno examples for default/narrow scope, event cursor, invalid horizon,
   and stale basis. Never commit real credentials, tokens or owner data.

**Required tests:** A01–A07, D01–D04 in section 11.
**Gate:** backend wrapper, OpenAPI generation and frontend type/unit checks.
Record API-only status; do not add a nav item until a usable screen exists.
Suggested commit: `feat(api): expose balance forecasts and source events`.

### Slice 4 — Constant-as-of FX with complete coverage and provenance

**Goal:** add the optional combined series without changing exact source totals.

**Files:** new `backend/internal/app/forecast_rates.go`/tests; extend `backend/internal/db/forecast.go`, API schemas,
handler/client tests and Bruno request. Inspect existing valuation helpers but
avoid unrelated changes to historical reporting behavior.

1. Add bulk stored observation selection and required provenance to the reader,
   using the same snapshot and A cutoff. Do not invoke `NewRateTable`'s existing
   pool-bound reads or create a request per missing currency.
2. Implement D7 eligibility, stable ties, fixed rate date, missing/stale coverage,
   same-currency identity and exact component rounding.
3. Derive converted curves from converted components, not independently rounded
   closing balances. Check every response coefficient for overflow.
4. Add the paired `reporting_currency_id`/`fx_method` parameters and valuation/
   converted fields to both API recipes in the same commit as behavior.
5. Include selected rates in the basis digest; changed or voided rates must
   stale event cursors as well as balances. No rate download on read.
6. Prove enabling conversion does not change any exact per-currency result.

**Required tests:** F01–F08 and A08 in section 11; fixture C rounding examples.
**Gate:** focused forecast/pricing/valuation tests, full backend wrapper,
OpenAPI generation and frontend checks. Missing coverage is a tested successful
exact forecast with null conversion, not a failing page.
Suggested commit: `feat(forecast): add explicit constant-FX combined balances`.

### Slice 5 — Forecast screen, filters and exact daily views

**Goal:** a reachable responsive read-only forecast screen with both curves.

**Read first:** frontend-screen skill, existing reports screen/filter helpers,
chart geometry, app layout and localization glossary.

**Files:** thin route, new forecast screen/model/chart and model tests,
`frontend/src/routes/app/+layout.svelte`, six app catalogs and glossary.

1. Add terminology rows before strings; use the established six-locale message
   and placeholder conventions. Do not claim native review.
2. Implement normalized URL/apply/reset filters and correct query keys.
   Account/currency options come from the composed response.
3. Render default/selected scope, server date metadata, opening/end/minimum
   summaries, both curves, exact daily table and source components.
4. Render complete/null FX states and persistent carried-forward/exclusion
   notices. Invalid UI query state must not silently change scope.
5. Add navigation/title/shell copy and all state-matrix rows relevant to the
   balances page. Keep the UI read-only and link to existing workflows.
6. Test URL round-trip, duplicates, descendant false, empty scope, large exact
   coefficients, chart signs/zero points and no mutation of query/form state.
7. Verify a production build and one real browser happy path before marking
   the screen as shipped; remaining details/acceptance are still incomplete.

**Gate:** frontend type/unit checks, catalog parity for new keys, focused
forecast browser check and production build. No page-level horizontal overflow
at 390px; amounts are available without relying on the chart.
Suggested commit: `feat(frontend): add projected balance screen`.

### Slice 6 — Event explanations, stale-basis recovery and refresh integration

**Goal:** the user can explain a day's balance safely and return from review.

**Files:** new `forecast-events.svelte`; forecast screen/model tests;
existing mutation invalidation sites only where needed; browser spec.

1. Add keyboard-accessible day expansion with one event query per open detail
   component. Keep all pages under their exact basis token and detail filters.
2. Show original/assumed dates, source badges, exact account/currency amounts,
   total count and cursor Load more; no per-row transaction/payee fetches.
3. Handle 409 by clearing details, refreshing balances and announcing the change.
   Also clear details when Apply/Refresh returns a different basis.
4. Add appropriate forecast invalidation alongside existing transaction,
   recurring, import, account and price mutation invalidations; retain mount/
   focus refetch as a fallback. Do not introduce global polling.
5. Verify focus/keyboard behavior, loading/error/empty details, light/dark/mobile,
   and that source links lead to existing usable workflows.
6. Add the real browser journey: inspect a planned event → review/generate/edit
   through R9 → return/refresh → see the changed saved amount once.

**Gate:** frontend checks, forecast/recurring/transaction browser specs. A
stale-cursor response must not leave old and new events mixed on screen.
Suggested commit: `feat(forecast): explain daily movements and refresh stale details`.

### Slice 7 — Cross-system acceptance and adversarial regression pass

**Goal:** prove the boundaries using real producers, large inputs and failures.

**Files:** forecast db/app/API tests and browser spec; minimal defect fixes and
backlog entries only when this review finds real gaps.

1. Run every remaining row of section 11. Add missing evidence rather than
   substituting a similarly named test that does not exercise the condition.
2. Compare transaction/occurrence/audit/background/reconciliation/lot state
   before and after forecast reads; check reports and ledger CSV/QIF unchanged.
3. Exercise generation/posting in independent pools during snapshot reads;
   verify baseline and recurrence identity cannot describe different states.
4. Test >4,000 missed daily occurrences, exhausted max-occurrence schedules,
   empty enumeration chunks, all output caps, and more than one event page.
5. Check >2^53 precision, 38/39-digit boundaries, currency netting and FX gaps.
6. Run browser state/accessibility tests using deterministic fixtures and
   deferred route responses where loading must be observed. Do not call an
   error-only test “loading/empty/error coverage.”
7. Inspect the whole touched pattern for defects; record each finding in the
   next available T-number with failing test/fix/evidence. Do not reuse T-82–84.

**Gate:** full backend wrapper, frontend wrapper, production-build forecast +
recurring + transactions + accessibility browser run, sequential frontend builds.
All cases must actually run; dependency-skipped browser cases are not passes.
Suggested commit: `test(forecast): verify lifecycle isolation and acceptance edges`.

### Slice 8 — Core acceptance review and documentation milestone

**Goal:** accept the core forecast honestly, using the R9 review pattern;
leave the approved learning extension explicitly open.

1. Write a dated `docs/reviews/r10-acceptance-review-YYYY-MM-DD.md` mapping every
   required scope item and D1–D8 to actual code and tests. Record final commands,
   counts and real limitations, not just “all tests pass.”
2. Revisit default scope, 90/366-day range, tomorrow carry-forward, constant FX,
   rounding, limits and detail-token UX explicitly. A changed decision needs
   corresponding code/test/doc updates before acceptance.
3. Answer every excluded item in section 1 with include/no and reason. Do not
   quietly turn a required item (e.g. event explanations or optional FX support)
   into deferred scope just because it is unfinished.
4. Update this plan's status, `implemented.md`, `roadmap.md`, `todo.md` and the
   Rekenraam column of `competitor-comparison.md`. Do not refresh competitor
   claims without separate research; keep their snapshot caveat.
5. Update any durable rule changed during implementation in the governing docs;
   use an ADR if a new long-lived architectural tradeoff departs from existing
   read-model/precision/background-work boundaries.
6. Mark the eight core slices accepted only after their screen and all gates
   are satisfied. Next is M1 in `forecast-learning-plan.md`; R10 remains open
   until M4 accepts that extension. R8 budget planning follows final R10 closure.
   Do not begin M1 or R8 in this acceptance commit.

**Gate:** all required rows below have evidence; no untracked core blocker
remains; learning remains planned.
Suggested commit: `docs(forecast): accept R10 core forecast`.

### Execution record — maintain this table after every slice

| Slice | Status at planning | Completion commit/date | Evidence / remaining work |
|---|---|---|---|
| 1. Read-only snapshot inputs | [x] Complete | This commit, 2026-08-31 | `ForecastRepository` reads all source rows through one `OpenReadOnly` transaction. Seven named repository tests cover current posted versions, the posted bulk query plan using `posting_versions_account_idx`, archived/edit-changed template drafts, full draft counterparts, concurrent write isolation, limit+1 failure and empty scope. Existing recurring occurrence/template and entry/version indexes cover the remaining joins; no new index was justified. Next: slice 2 exact projection. |
| 2. Exact projection | [x] Complete | This commit, 2026-09-07 | `ForecastService` resolves owner-local bounds and effective-dated account scope inside the coherent snapshot, then produces exact per-account/per-currency and same-currency aggregate curves. Posted, saved-draft and computed-template sources have deterministic precedence and ordering; overdue assumptions carry to tomorrow without mutation; invalid/broken inputs become bounded diagnostics. Named tests cover the P01–P18/B01–B04 behavior applicable before the API, including real template generation and draft promotion, exact mixed scales and values above 2^53, overflow, lifecycle rules, schedules, limits and cancellation. Next: slice 3 balances/events API. |
| 3. Balances/events API | [x] Complete | This commit, 2026-09-07 | OpenAPI-first authenticated read routes expose exact per-currency balances and stable cursor-paged source events. Strict parsing rejects repeated/unknown scalars and unsupported FX parameters; event pages recompute the same projection, enforce the basis token and filter recipe, return non-null arrays, and map size/overflow/stale-basis failures to stable codes. A production/test service is wired to the shared read-only pool; generated frontend types, typed query helpers, localized errors and Bruno examples ship with A01–A07 and D01–D04 evidence. Next: slice 4 constant-as-of FX. |
| 4. Constant FX | [x] Complete | This commit, 2026-09-07 | Both forecast recipes accept the paired `reporting_currency_id` and `fx_method=constant_as_of` options. Stored direct currency rates are selected inside the coherent snapshot with the as-of cutoff and deterministic tie rules; missing/stale coverage returns provenance gaps and a null combined series while exact source series remain unchanged. Same-currency identity, pre-netting coverage, 7-day staleness, future/void exclusion, per-currency component half-away-from-zero rounding, basis participation and no background-work side effects are covered by F01–F08/A08 tests. OpenAPI, generated client types and a Bruno recipe ship with the behavior. Next: slice 5 forecast screen. |
| 5. Forecast screen | [x] Complete | This commit, 2026-09-07 | `/app/forecast` is reachable from app navigation and uses one composed forecast request for exact series plus account/currency options. Strict canonical URL filters cover horizon, account scope, descendants and paired constant-FX conversion. Responsive cards, an accessible two-curve chart and authoritative daily tables expose exact values, movements, assumptions, diagnostics and FX provenance across loading, invalid, empty, no-movement, error and success states. All six locales ship; focused model/API tests, a production build and a real browser flow including 390px overflow coverage pass. Next: slice 6 event explanations and refresh recovery. |
| 6. Details and refresh | [ ] Not started | — | — |
| 7. Cross-system acceptance | [ ] Not started | — | — |
| 8. Acceptance closure | [ ] Not started | — | — |

## 11. Required test matrix

Suggested names are fixed enough to search during acceptance. Table-driven
subtests are encouraged, but the cases and numerical assertions are mandatory.
Test via pure builder when possible, real service/repository when identity or
snapshot matters, HTTP when parsing/auth/error mapping matters, and browser
only for behavior that requires a browser.

| ID | Required named test / assertion | Layer |
|---|---|---|
| P01 | `TestForecastOpeningUsesPostedEntryDates`: draft/void/delete/superseded excluded; header date differs from entry date. | app/db |
| P02 | `TestForecastFuturePostedEntriesMoveOnEntryDate`: multiple entries and both bounds F/E; beyond E excluded. | app |
| P03 | `TestForecastRecurringMaterializationDoesNotChangeAmounts`: real template→draft→future posted preserves projected points, moves source component. | app |
| P04 | `TestForecastEditedDraftOverridesTemplateDateAndAmount`: original date suppressed, edited date/amount used, including edits from beyond E into range and out again. | app |
| P05 | `TestForecastTerminalOccurrencesNeverReappear`: skipped, discarded, voided, soft-deleted, and broken generated links; diagnostic behavior differs as D3 requires. | app/db |
| P06 | `TestForecastPausedAndArchivedTemplatesKeepSavedDrafts`: no new assumptions, existing draft remains; edited draft account differs from template. | app |
| P07 | `TestForecastCarriesOverdueAndTodayToTomorrow`: saved and unmaterialized dates match, no date/watermark mutation, all backlog included. | app |
| P08 | `TestForecastUsesWatermarkButNotLeadWindow`: past anchor does not backfill before watermark; lead=0 still forecasts 365 days; count/end constraints survive. | app |
| P09 | `TestForecastReusesClampedCalendarSchedules`: Jan 31, leap day, interval and year boundary; DST and owner/browser date disagreement. | app |
| P10 | `TestForecastInvalidDraftExcludesWholeTransaction`: unbalanced or invalid counterpart outside selected accounts, including an invalid out-of-horizon entry with future-effective rules; empty draft diagnostics; no one-sided contribution. | app |
| P11 | `TestForecastLifecycleEligibilityUsesSnapshotDates`: opened/closed/disabled account, precision/currency changes, source vs carry date, posted history retained. | app |
| P12 | `TestForecastScopeDefaultsAndDescendants`: default kinds, selected groups, overlapping parent/child, include_descendants=false, closed explicit account, invalid roots. | app |
| P13 | `TestForecastTransfersAndLiabilitySigns`: internal/outside-scope transfers, FX clearing and credit-card repayment; fixture A/B. | app |
| P14 | `TestForecastExactDailyIdentities`: every account/currency/day and aggregate satisfies both D6 equations; shuffled inputs produce identical result except timestamp. | app |
| P15 | `TestForecastPreservesLargeAndMixedScaleAmounts`: >2^53, scale alignment and checked overflow; fixture C. | app/API |
| P16 | `TestForecastFlatSeriesAndMinimumTieDates`: no movement, zero default currency, negative opening, equal minima, no liability cash alert. | app |
| P17 | `TestForecastDiagnosticsDoNotHideExclusions`: >50 diagnostics, exact hidden/total counts, complete flag computed before truncation. | app |
| P18 | `TestForecastIgnoresNonRecurringDraftsAndSecurityValues`: other producer draft excluded; posted investment currency legs included without lot/price valuation. | app |
| B01 | `TestForecastLongCatchUpIsBoundedWithoutTruncation`: >4,000 dates progresses across enumeration chunks or explicit 422 at budget; no prefix total. | app |
| B02 | `TestForecastEmptyChunksAndExhaustedSchedules`: sparse interval across empty windows, max count exhausted, no loop/hang or anchor reset. | app |
| B03 | `TestForecastInputAndOutputBudgets`: each named cap at limit and limit+1; early allocation guard; empty account scope safe. | db/app |
| B04 | `TestForecastHonorsContextCancellation`: cancellation in bulk reads and long pure loops releases resources. | db/app |
| A01 | `TestForecastAPIRequiresOwnerAndIsReadOnly`: auth and financial-domain no-write assertion on both endpoints, even with missing rates. | API |
| A02 | `TestForecastAPIRejectsAmbiguousQuery`: bad/repeated/unknown scalars, invalid IDs, horizons, booleans and unsupported as-of; valid defaults echoed. | API |
| A03 | `TestForecastAPIReturnsExactWireQuantities`: all coefficients quoted, arrays non-null, sort order stable, 39-digit result is 422. | API |
| A04 | `TestForecastAPIDefaultAndEmptyScopes`: empty selection result never queries whole book; account options labels/scales complete. | API |
| A05 | `TestForecastAPISizeErrorHasNoPartialTotals`: 422 code and translated recovery path, no success body mixed in. | API |
| A06 | `TestForecastAPIWiringUsesReadOnlyPool`: production/test wiring supplies service and `/api/` misses remain JSON errors. | API/command |
| A07 | `TestForecastAPINoStore`: private no-store response policy on balances/events; errors do not leak source financial input. | API |
| A08 | `TestForecastAPIValidatesPairedFXOptions`: both-or-neither rule, currency identity, supported method only. | API |
| D01 | `TestForecastEventPagesReconcileToDailyComponents`: >200 events, no omissions/duplicates, stable ordering, all pages sum to shown day. | app/API |
| D02 | `TestForecastBasisChangesAfterFinancialMutation`: draft edit/post/discard, template edit/skip, account rule/rate change stale old tokens; identical refresh has same token. | app |
| D03 | `TestForecastCursorRejectsChangedRecipeAndMalformedPayload`: filters/date/limit semantics, invalid encoding/version/oversize, auth not bypassed. | API |
| D04 | `TestForecastConcurrentMaterializationKeepsOneBasis`: snapshot sees old computed event or new saved event once, never both/none; new request sees new basis. | db/app |
| F01 | `TestForecastConstantFXNeverUsesFutureObservations`: future rate exists but all points use A; fixture A. | app |
| F02 | `TestForecastConstantFXStalenessAndTies`: 0/7/8-day age, recorded-at/ID ties, voided/latest replacement, provenance. | db/app |
| F03 | `TestForecastMissingFXOmitsWholeConvertedSeries`: exact results unchanged, old/missing gap metadata, zero-only currency exempt. | app/API |
| F04 | `TestForecastFXCoverageBeforeAccountNetting`: offsetting foreign balances still require a rate. | app |
| F05 | `TestForecastConstantFXRoundingReconciles`: negative half ties, multi-day minor units, no independent closing round; fixture C. | app |
| F06 | `TestForecastSameCurrencyConversionNeedsNoObservation`: exact identity rate, correct quote scale, >2^53 values. | app |
| F07 | `TestForecastFXReadDoesNotQueueCoverage`: real foreign recurring draft and missing FX, before/after work/coverage state unchanged. | API |
| F08 | `TestForecastFXDoesNotChangePerCurrencySeries`: equality of exact series with and without conversion; overflow remains explicit. | app |
| X01 | `TestForecastReadsLeaveLedgerReportsAndExportsUnchanged`: real recurring sources, before/after domain counts, net worth/spending/cashflow, CSV and QIF. | API |
| X02 | `TestForecastDoesNotTouchInvestmentSubledgerOrCheckpoints`: lot/checkpoint/audit state unchanged; no write service called. | API |

Browser cases in `forecast.spec.ts`:

1. **`[acceptance] forecasts recorded and recurring movements without posting`**:
   real source data; both curves/daily table and event explanation agree; no
   generated transaction caused by visiting the forecast.
2. **`[acceptance] edited recurring draft replaces the template assumption`**:
   real R9 workflow, return/refresh, exact edited amount appears once.
3. **`[acceptance] forecast filters survive reload and back navigation`**:
   selected account/descendants/horizon/FX, malformed URL and Reset behavior.
4. **`[acceptance] forecast is usable on mobile and by keyboard in both themes`**:
   axe, focus, filters/day detail/Load more, 390px layout and exact table.
5. **`forecast exposes loading empty error and excluded-assumption states`**:
   deliberately delay fulfillment to assert loading; separately test no accounts,
   flat balances, blocked diagnostic, backend error and successful retry.
6. **`forecast keeps exact currencies when constant FX is unavailable`**:
   no partial combined total; rates notice and per-currency amounts remain.
7. **`forecast refreshes after stale event basis without mixing pages`**:
   real mutation where practical; deterministic 409 injection for recovery edge;
   clear old rows, refresh and reopen, preserving keyboard focus.
8. **`forecast renders all new messages in a non-English locale`**:
   Dutch formatting plus one non-Latin locale; assert meaningful forecast,
   assumption and FX messages, not just the locale switcher itself.

Go tests may use `recurringFixture` in `backend/internal/app/recurring_test.go` and the real
`generateRecurringDraft` helper in `backend/internal/api/recurring_generation_test.go` where
appropriate. `newSetupTestHandler` is in `backend/internal/api/setup_test.go`. Extend helpers
carefully; do not open an exemption in the browser draft guard for fixtures.
Use `backend/internal/testdb` for normal isolated DB tests; independent pools must point
to the same test file only for concurrency cases.

## 12. Definition of done

The eight core slices are accepted only when all are true. Full R10 completion
also requires M1–M4 and their dated acceptance evidence in
`docs/plans/forecast-learning-plan.md`:

- [ ] All section 1 required scope items and D1–D8 have implementation evidence.
- [ ] Every test-matrix case has run and passed, or an equivalent named test is
      mapped with the exact missing/covered assertions; no vague substitutions.
- [ ] No unresolved defect can double count, drop posted money, invent a rate,
      mutate the ledger on read, or silently truncate a forecast.
- [ ] Coherent-snapshot and no-side-effect tests use real recurring production
      paths, not manually inserted synthetic drafts alone.
- [ ] API, generated types, client and actual parameter defaults agree.
- [ ] Both curves, warnings, exact table and event pagination work on mobile,
      keyboard and both themes; new strings cover all six locales.
- [ ] Rates/rounding/overdue assumptions are visible before totals can mislead.
- [ ] Final backend/frontend/browser checks pass, with no skipped acceptance
      cases counted as passes and no concurrent translation build artifacts.
- [ ] Dated review, roadmap, feature ledger, working queue and backlog agree.
- [ ] Scoped commits exist, working tree is clean except explicitly preserved
      unrelated changes, and the next task is stated without starting it.

## 13. Copyable execution and handoff instructions

Start the next implementation session with:

> Implement **R10 slice 1 only** from `docs/plans/projected-balances-plan.md`.
> Read AGENTS.md and the plan's required source documents/skills. Follow its
> snapshot, precision and no-write rules. Add the named slice-1 tests, run the
> backend checks, update the execution record and feature ledger honestly, and
> commit the scoped result. Do not add forecast endpoints or a UI yet. Report
> the commit, checks and exact next slice. Do not push or publish.

For later sessions, replace only the slice number and its deliverables; do not
copy slice 1's no-endpoint restriction into a slice that explicitly adds one.

Every slice handoff must contain:

```text
R10 slice <N>: complete / partial / blocked
Commit(s): <hashes, or why no commit>
Changed: <files and externally meaningful behavior>
Tests run: <exact commands and actual outcomes>
Design deviations: <none, or section + reason + reconciled docs/tests>
Remaining in this slice: <explicit checklist; never silently advance>
Next: <first unfinished slice and its initial action>
Warnings: <environment restrictions or unrelated user changes preserved>
```

Do not report “R10 done” after the calculation engine or first graph. The last
step is acceptance against this contract and the approved learning extension,
not the first plausible forecast. After core slice 8 hand off M1, not R8.


Planning validation (2026-08-31): existing source paths and helper names were
checked against `b28c5d57`; proposed new paths are explicitly identified. The
worked fixture's daily/converted balances were checked with integer arithmetic;
the matrix has 44 unique case IDs plus six named snapshot tests and eight
browser journeys. Implementation and its runtime tests remain unstarted.
