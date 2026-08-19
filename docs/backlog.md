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

### T-37 Price observations can never be voided `[x]`

**Files:** `backend/internal/db/pricing.go`, `backend/internal/app/pricing.go`,
`backend/internal/api/pricing.go`.

`price_observations.voided_at` exists and every read filters on it, but no
repository/service/endpoint ever sets it — the "correct a price by
superseding **or voiding**" invariant is half-implemented, and a poisoned
observation cannot be retired from historical listings or derivations.
Audit P3. Natural home: R11 pricing UI, but the endpoint can land earlier.

**Raised in priority, then fixed, 2026-08-19.** The approved reporting-currency
selector will make FX and price observations drive headline report figures, at
which point an unvoidable poisoned observation stops being a tidy-up — so this
landed before the currency work rather than after it.

`POST /api/v1/pricing/prices/{price_id}/void` retires an observation without
deleting it: the row stays, `voided_at` is stamped, and every read already
filtered on it, so a poisoned price immediately stops feeding listings and
derivations. Decisions worth not re-litigating:

- **A reason is required, never defaulted.** Voiding is a deliberate correction
  to financial data, and inventing the "why" for the caller would make the audit
  log lie.
- **A second void is a 409, not a silent success.** Two people, or two clicks,
  disagreeing about a price's state is worth surfacing.
- Migration `0002_price_observation_void_audit.sql` adds `voided_audit_event_id`.
  The table carried `created_audit_event_id` but had no equivalent for the void,
  so the audit model's rule — every row an operation touches references the
  event explaining it — could not be met from the row alone.

Proven by `TestVoidPrice_RetiresObservationWithoutDeletingIt` (the voided row
keeps its original price and leaves the listing; the good one stays) and
`TestVoidPrice_RequiresAReasonAndRejectsRepeats`. Remaining for R11: the pricing
UI that exposes it.

### T-38 Zero-proceeds disposal (write-off) is impossible `[x]`

**Files:** `backend/internal/app/investments.go` (`validateTradeInput`
requires `CashAmountValue > 0`; repository `DisposeLots` is unexposed).

A fund closure, worthless delisting, or any total-loss position cannot be
recorded: shares stay in open lots forever and the loss never reaches
realized gains. Audit P4.

**Decided 2026-08-19: a write-off is a disposal at zero proceeds**, booking
through the existing realized gain/loss treatment rather than a dedicated
write-off expense category — an investment loss is not spending, and this is
also the smaller change. Two guards belong in the implementation:

- **Zero proceeds must be explicit intent**, not an empty amount field
  defaulting to zero, or a mistyped sell silently becomes a write-off and the
  position disappears. Prefer a distinct flag or endpoint over `amount == 0`.
- It changes postings, so it goes through the **reconciliation guard** like any
  other mutation path.

Independent of the open I-03/I-04 gains-reporting research: this uses whatever
gains treatment is current.

**Done 2026-08-19.** `POST /api/v1/investments/write-off` — a separate route
rather than a flag on sell, so "no proceeds" is always stated: a cash amount is
*rejected* there, and a reason is required. Selling still demands a cash amount,
so the ordinary path was not loosened.

The write-off carries **no cash postings at all**, only the commodity legs
through `commodity_trading`. That is what makes the loss land correctly with no
new gains logic: the realized-gain engine derives proceeds by matching cash
postings to the disposal, and already reports zero when none match (the branch
was written for manual lot creation). Proceeds of zero against the disposed
basis is the whole basis as a loss. A zero-valued cash posting would also have
been noise for a reconciler.

Three named tests: the -1000.00 realized loss with a closed lot and a
two-posting transaction; the rejection paths; and one proving the new mutation
path goes through the reconciliation guard. That last test found a real defect —
`ErrReconciliationOverrideRequired` was unmapped in the investments API and
surfaced as a 500 for buy and sell as well. Fixed here; the missing override
*capability* is now T-47.

### T-44 Free-text payees never group in reports `[x]`

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
spelling and casing variants.

**Decided 2026-08-19: resolve on entry, but never silently.** Typing a new payee
name prompts for confirmation, offering existing payees through a fuzzy search
before a record is created. Auto-creating without confirmation was rejected
because it sprays near-duplicate records from typos.

**Entry side done 2026-08-19** (`lib/transactions/payee-matching.ts` +
`transaction-editor.svelte`). The editor loads the payee list once and ranks it
locally with `minisearch` rather than re-querying per keystroke: a server-side
`LIKE` cannot find "Market Hall" from "Markt Hall", and that near miss is
exactly what makes someone create the duplicate. Saving a name no payee carries
stops and asks, offering the near matches first. Confirmation is skipped when
the name was not edited, so an older transaction that already carries an
unlinked name never blocks an edit to an unrelated field.

Two consequences the decision implies and that this item must cover:

- **Imports cannot show a dialog per row.** The import path already resolves a
  `PayeeID` where it can; the rule there is auto-link exact matches, leave the
  rest unlinked, and surface unlinked names in the existing import review.
  **Done 2026-08-19:** the match happens in `cleanTransactionSpec`, which every
  write path shares, so manual entry and import commit got it in one change.
  Active payee names are unique by normalized name, so the match is
  unambiguous; the record's own capitalisation wins, so the stored name and the
  link never disagree. Archived payees are deliberately not matched — they are
  not offered for new entry, and linking to one silently would resurrect it in
  reports.
- **No history to repair (confirmed by the owner 2026-08-19).** The app is still
  in development and carries no real data, so the one-off "link unlinked
  payees" backfill tool this item previously called for is **not needed and has
  been dropped**. Should that change — a pilot user, an imported book — it comes
  back as new work, not as a resurrection of this item.
- **Imports still produce unlinked names, deliberately.** A bulk import cannot
  confirm each unknown payee, so unrecognized names commit as free text. Until
  import review can resolve them, the path is to open the transaction and edit
  the payee, where the editor now forces link-or-create. Making import review
  resolve distinct unknown names in one pass belongs with import work (R5/R6),
  not with T-44; recorded there rather than left implied here.

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

### T-47 Investment trades cannot override a reconciliation lock `[ ]`

**Files:** `backend/internal/api/investments.go` (`investmentTradeRequest` has no
`reconciliation_override`); `backend/internal/app/investments.go`
(`InvestmentTradeInput.ReconciliationOverride` exists and is honoured).

Found 2026-08-19 while adding T-38's write-off. Every investment trade goes
through the transaction write guard, so a buy, sell, or write-off dated inside a
reconciled period is correctly refused with
`ErrReconciliationOverrideRequired` — but the investments API never exposes
`reconciliation_override`, so there is no way to proceed deliberately the way
the transaction editor allows. The service field is already there and honoured;
only the request shape and a confirmation flow in the investments UI are
missing.

Separately fixed the same day: that error was **unmapped** in
`writeInvestmentServiceError`, so it surfaced as a 500 "internal server error"
rather than a 409. It told the user nothing and looked like a fault instead of
the deliberate refusal it is. Now a `CONFLICT`, matching the transactions API.

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

### T-46 One inline style was blocked by CSP on every page load `[x]`

Fixed 2026-08-18. The blocked style was never a `<style>` element — it was a
`style` *attribute*, which is why the 2026-08-17 pass found no stylesheet in
`index.html`, no `createElement('style')` in the bundle, and no `<style>` node
in the DOM. The violation report names `style-src-attr` (`style-src 'self'` is
its fallback), and its sample is
`position: absolute; left: 0; top: 0; cli…` — SvelteKit's
`#svelte-announcer` live region, whose visually-hidden rules are hardcoded as an
inline `style` attribute in the root component it generates
(`@sveltejs/kit/src/core/sync/write_root.js` →
`frontend/.svelte-kit/generated/root.svelte`). Svelte 5 builds that markup
through `template.innerHTML`, so parsing it trips the directive on every page
load. Nothing rendered wrong because the declarations survive on the cloned
live node; only the template parse is refused.

Fixed at the source rather than by widening the CSP: the
`rekenraam:svelte-announcer-csp` Vite plugin
(`frontend/vite/svelte-announcer-csp.js`, wired in `frontend/vite.config.ts`)
strips the attribute from the generated root before Svelte compiles it, and
`#svelte-announcer` is styled from `frontend/src/app.css` instead. `style-src`
is unchanged — no `'unsafe-inline'`, no `'unsafe-hashes'`, no hash to
re-pin on every SvelteKit release.

Proven by `e2e/playwright/csp.spec.ts` ("runs without Content-Security-Policy
violations"), which attaches a `securitypolicyviolation` listener in an init
script — the template is parsed during bundle startup, so a listener added later
misses it — walks the signed-in screens against the real binary and its real
CSP, and asserts no violations plus an announcer that is still 1×1 and clipped
with no `style` attribute. It fails with one `style-src-attr` violation when the
plugin is removed. Two further guards keep a SvelteKit upgrade from silently
reintroducing the problem: the plugin throws if it finds the announcer with an
opening tag it cannot parse, and its `buildEnd` throws if the announcer was
never seen at all.

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

**Parked 2026-08-19 (owner decision).** The app is self-hosted locally for now,
so none of S-04, S-06, or S-07 is scheduled work. **Trigger to unpark: the first
time an internet-exposed deployment is planned — R3 at the earliest.** They stay
listed because the product gate is real, not because anyone is working on them;
do not treat them as available pickups.

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
