# Technical Backlog

This is the **live** technical-debt list: items that are actionable now and are
not already scheduled as product work. It is intentionally short.

- **Feature sequence:** `docs/roadmap.md`.
- **Short-horizon queue:** `docs/todo.md`.
- **Shipped capability:** `docs/implemented.md`.
- **Resolved and deliberate decisions:** `docs/reviews/resolved-backlog-2026-07.md`.

Status legend: `[ ]` open, `[x]` closed (kept briefly with the fix and the
gate that prevents recurrence, then moved to `docs/reviews/`).

## General

### T-34 No producer of investment provider events/suggestions `[ ]`

**Files:** `backend/internal/app/investments.go` (`DividendProvider`,
`CorporateActionProvider` interfaces, declared but never implemented or
referenced); `investment_provider_events` / `investment_event_suggestions`
tables (`backend/migrations/0001_initial_schema.sql`).

Verified 2026-07-15 (Workstream 3 investment test coverage): nothing in the
codebase writes to `investment_provider_events` or
`investment_event_suggestions` — no fetcher, worker, or import path creates
them. The review UI and the accept/ignore/automation-rules endpoints all work
correctly (fixed the same day — see below), but have no data to act on until
a producer exists. `docs/product-requirements.md` lists "provider events and
reviewable suggestions" as a real requirement. Needs: a chosen data source
(no candidate picked yet), a fetch/detection design, and — separately — a
lot-mutation design for structural corporate actions (split, merger,
spin_off, ticker_change, delisting, `corporate_action`), which
`AcceptSuggestion` currently rejects outright since only the
`dividend_income` proposed-transaction kind (dividend, distribution,
cash_in_lieu, return_of_capital) is implemented.

### T-37 Price observations can never be voided `[ ]`

**Files:** `backend/internal/db/pricing.go`, `backend/internal/app/pricing.go`,
`backend/internal/api/pricing.go`.

`price_observations.voided_at` exists and every read filters on it, but no
repository/service/endpoint ever sets it — the "correct a price by
superseding **or voiding**" invariant is half-implemented, and a poisoned
observation cannot be retired from historical listings or derivations.
Audit P3. Natural home: R11 pricing UI, but the endpoint can land earlier.

### T-38 Zero-proceeds disposal (write-off) is impossible `[ ]`

**Files:** `backend/internal/app/investments.go` (`validateTradeInput`
requires `CashAmountValue > 0`; repository `DisposeLots` is unexposed).

A fund closure, worthless delisting, or any total-loss position cannot be
recorded: shares stay in open lots forever and the loss never reaches
realized gains. Audit P4. Needs a small product decision (which account the
loss books against) plus either `CashAmountValue == 0` support on Sell or a
dedicated write-off endpoint over the existing disposal engine.

### T-44 Free-text payees never group in reports `[ ]`

**Files:** `backend/internal/app/transactions_validate.go` (payee resolution:
`payee_name` is stored as free text and only `payee_id` links a payee record);
`backend/internal/app/reports.go` (`Spending`, payee grouping).

A transaction may carry a `payee_name` with no `payee_id` — the create path
accepts the name as free text and only sets `payee_id` when the caller supplies
one. The spending report groups by `payee_id`, so every free-text payee falls
into the single "no payee recorded" group. The report still reconciles exactly
(the unattributed group is emitted, never dropped), but a user who typed payee
names without picking records sees one undifferentiated bucket instead of a
ranking. Grouping on the raw text is not the fix: it would duplicate rows across
spelling and casing variants. Needs a product decision on whether transaction
entry should resolve or create a payee record from a typed name, then the
report follows for free.

### T-45 Net-worth series re-reads the whole ledger once per bucket `[x]`

**Files:** `backend/internal/app/ledger.go` (`NetWorthSeries` loops buckets and
calls `netWorthTotals` per bucket; each call issues
`LedgerAccountsAsOf` + `LedgerPostingsThrough(asOf)`, and
`LedgerPostingsThrough` reads every posting from the beginning of time through
that date).

Cost is buckets x postings, so a fine bucket over a long range degrades
quadratically. Measured 2026-08-17 against the integrated binary with only
**600 transactions (1200 postings)**, one year requested:

| Bucket | Buckets | Response time |
|---|---|---|
| `year` | 1 | 11-13 ms |
| `month` | 12 | 53-61 ms |
| `day` | 365 | **1359-1403 ms** |

`day` is ~120x the single-bucket cost, and the ledger is tiny. A few years of
ordinary use makes `bucket=day` unusable. The fix is to read the postings once
for the whole range plus an opening balance before `start_date`, then fold
forward across bucket boundaries in one pass — the accumulation is already exact and
order-independent (`exact.ScaledInt`), so this is a loop restructure, not a
financial change. `/reports/spending` does not have the problem (one range, one
read: 14 ms on the same data).

Fixed 2026-08-18. `NetWorthSeries` now reads the ledger once for the whole
series (`netWorthSeriesTotals` in `backend/internal/app/ledger.go`) and folds it
forward across buckets.

Two reads were per bucket, and both are now single reads:

- **Postings.** One `LedgerPostingsThrough(series end)` folded forward into a
  running balance, with one cursor over the date-ordered postings.
- **Accounts.** A new `LedgerAccountVersionsThrough` returns every account
  version effective at or before the series end, and `ledgerAccountTimeline`
  replays them in ascending order. Replaying and keeping the last version per
  account is the ascending mirror of the `effective_from DESC, version_seq DESC
  LIMIT 1` pick `LedgerAccountsAsOf` makes, so each bucket gets the same
  snapshot the per-bucket query would have returned.

The fold is kept **per account**, not collapsed straight into commodity totals.
Everything that turns balances into net worth is effective-dated — asset vs
liability, the excluded `commodity_trading` role, and how an account subtree
expands — and is still resolved against each bucket's own snapshot. Collapsing
earlier would bake one bucket's classification into every later bucket. Cost
goes from buckets x postings to postings + (buckets x accounts).

Measured on the same integrated path, best of three:

| Ledger | Bucket | Buckets | Before | After |
|---|---|---|---|---|
| 600 tx, 1 year | `day` | 365 | 1445 ms | **8.7 ms** |
| 3000 tx, 3 years | `month` | 36 | 709 ms | **29 ms** |
| 3000 tx, 3 years | `day` | 1096 | 21353 ms | **36 ms** |

Response time is now flat across bucket granularity (29-36 ms across
`year`/`month`/`day` on the 3000-transaction ledger) instead of scaling with
bucket count.

Guarded by three tests in `backend/internal/api/reports_test.go`: the series is
checked against the point-in-time `/ledger/net-worth` endpoint at every bucket
end for all five bucket sizes, the filtered series is checked against
single-bucket requests ending on the same dates, and a mid-series reparenting
pins that each bucket expands the subtree against its own account versions.

Prep had landed earlier the same day: account and commodity filtering became one
path (`reportFilterSet` in `reports.go`) shared by the SQL-side spending filter
and the in-Go net-worth filter, so the restructure only had to call
`reportFilterSetFrom` per bucket rather than re-derive a filter.

### T-46 One inline style is blocked by CSP on every page load `[ ]`

**Files:** `backend/internal/api/middleware.go` (CSP: `style-src 'self'` with no
`'unsafe-inline'`); source of the injected style not yet identified.

Every page load logs `Refused to apply inline style ... "style-src 'self'"` with
a stable hash (`sha256-S8qMpvofolR8Mpjy4kQvEm7m1q8clzU4dfDH0AmvZjo=`), so one
fixed stylesheet is being dropped. Investigated 2026-08-17 without a conclusion:
the shipped `index.html` contains no `<style>` element, the built bundle contains
no `createElement('style')`, and no `<style>` node is present in the DOM even
with `bypassCSP` enabled — so it is injected and discarded, or built through the
CSSOM, most likely by a dependency rather than app code.

Not reproduced as a visual defect: light, dark, and 390px-wide renders of the
install gate, reports, settings, and appearance screens were all checked and look
correct, and inline style *attributes* (theme swatches, table column widths) do
apply — they are not what is being blocked. Worth resolving anyway: a CSP
violation on every load is noise that hides real ones, and whatever is being
dropped is presumably meant to do something. Identify the injector, then either
give it a nonce/hash or remove it. Do **not** add `'unsafe-inline'` to
`style-src` as the fix.

### T-42 TypeScript 7 upgrade blocked by `openapi-typescript` `[ ]`

**Files:** `frontend/package.json` (`typescript`, `openapi-typescript`);
`frontend/src/lib/api/schema.d.ts` generation step.

TypeScript 7.0.2 is released, but `openapi-typescript` 7.13.0 (latest) still
declares `peerDependencies.typescript: ^5.x` and emits `schema.d.ts` through
the TypeScript JS compiler API (`ts.factory`). TS 7 no longer exposes it, so
`pnpm run openapi:generate` fails immediately with
`TypeError: Cannot read properties of undefined (reading 'createKeywordTypeNode')`,
taking `dev`, `check`, and `build` down with it. The frontend is pinned to
TypeScript 6.0.3 until `openapi-typescript` ships TS 7 support; re-check on
each `openapi-typescript` release. The stale `typescript@7.0.2` and
`@typescript/typescript-*@7.0.2` entries were left in
`pnpm-workspace.yaml`'s `minimumReleaseAgeExclude` so the retry is a one-line
version bump.

## Closed

### T-43 `gofmt` drift in four backend files `[x]`

Fixed 2026-08-17. `gofmt -w` applied to
`backend/internal/app/transactions_service.go`,
`backend/internal/db/reconciliation.go`, `backend/internal/db/recovery.go`, and
`backend/internal/web/static_integration_test.go` (struct-field alignment, a
gofmt comment-block reindent, two missing trailing newlines — no behavior
change).

The drift was invisible because the validation contract was wider than the
script: `docs/developer-workflow.md` called for `gofmt -l .` and `go vet ./...`
alongside the tests, but `scripts/test-backend.sh` only ran
`go test -race ./...`, and CI runs the script. Both gates now live in the
script, so CI enforces them and local matches. Proven by
`./scripts/test-backend.sh` failing on unformatted input and passing on the
formatted tree.

## Public-deployment security gates

These do not block private/local development. They must be complete before
allowing real financial data on an internet-exposed deployment.

### S-04 Username login throttle can cause owner lockout `[ ]`

**File:** `backend/internal/app/auth.go` (username-scope throttle).

An internet attacker can keep the known single-owner username rate-limited.
Design and implement an approved-device cookie or equivalent lockout-safe
throttle before public beta.

### S-06 Multi-factor authentication `[ ]`

**File:** `docs/product-requirements.md` (public-deployment requirement).

Public deployment with real financial data remains prohibited until MFA has an
approved design and implementation. Select and implement TOTP, WebAuthn, or an
approved equivalent before lifting this product gate.

### S-07 Authentication-event visibility `[ ]`

**File:** `backend/internal/api/auth.go`, `backend/internal/app/auth.go`.

Add privacy-conscious, durable or operator-consumable visibility for successful
and failed authentication events, including the proxy-aware client IP, so
operators can detect brute-force attempts and support incident response.
