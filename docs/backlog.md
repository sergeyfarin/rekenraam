# Technical Backlog

This is the **live** technical-debt list: items that are actionable now and are
not already scheduled as product work. It is intentionally short.

- **Feature sequence:** `docs/roadmap.md`.
- **Short-horizon queue:** `docs/todo.md`.
- **Shipped capability:** `docs/implemented.md`.
- **Resolved and deliberate decisions:** `docs/reviews/resolved-backlog-2026-07.md`.

Status legend: `[ ]` open · `[~]` partly done, with the remainder stated ·
`[blocked]` open with a stated dependency that must land first · `[x]` closed
(kept briefly with the fix and the gate that prevents recurrence, then moved to
`docs/reviews/`).

**A note on IDs T-42–T-47.** Two long-diverged branches independently used
these six numbers for six *different* defects/features before merging. Rather
than silently pick one meaning and erase the other's history — both are real,
both are merged in, and both are referenced by name in already-committed test
names and code comments — the branch that used them first in already-resolved,
frozen history (`docs/reviews/resolved-backlog-2026-07.md`, `docs/design/`)
keeps T-42–T-47 with its original meaning (all genesis-date fixes and frontend
decimal-comma bugs, all closed). The other branch's same-numbered items are
renumbered **T-48–T-53** below. If you find `T-47` in a backend code comment
meaning "reconciliation override," that comment predates this renumbering and
means what is recorded here as **T-53** — the mapping is: T-42→T-48 (TS7),
T-43→T-49 (gofmt), T-44→T-50 (payee resolution), T-45→T-51 (net-worth perf),
T-46→T-52 (CSP), T-47→T-53 (investment reconciliation override).

## General

### T-34 No producer of investment provider events/suggestions `[blocked]`

**Depends on** (assessed 2026-08-07 — this is scheduled product work, not a
fix that can be taken out of order):

- **R15, third slice.** The roadmap sequences the T-34 producer explicitly
  after IBKR Flex and GoCardless (`docs/roadmap.md` "Deliberately later";
  `docs/plans/connections-plan.md` § Sequencing). It is an adapter slice —
  provider package, credentials, prober, background fetch, cursors — not a
  defect with a local fix.
- **A provider decision that is still open.** Financial Modeling Prep is the
  leading free candidate for dividends and splits, but its EU coverage is
  unverified, and `connections-plan.md` records provider verification as a
  *blocking slice-start precondition*, not a task inside the slice.
- **R16's lot-mutation design note**, for the structural half only. Splits,
  mergers, spin-offs, ticker changes, and delistings mutate historical lots;
  `AcceptSuggestion` rejects them today by design. Dividend suggestions need
  no such note and would work the day a producer exists.

Nothing in the app is broken by this: the review UI, accept/ignore, and the
automation rules all work, they simply have no data. Do not start it early.

**Files:** `backend/internal/app/investments.go` (`DividendProvider`,
`CorporateActionProvider` interfaces, declared but never implemented or
referenced); `investment_provider_events` / `investment_event_suggestions`
tables (`backend/migrations/0001_initial_schema.sql`).

Verified 2026-07-15 (Workstream 3 investment test coverage): nothing in the
codebase writes to `investment_provider_events` or
`investment_event_suggestions` — no fetcher, worker, or import path creates
them. The review UI and the accept/ignore/automation-rules endpoints all work
correctly, but have no data to act on until a producer exists.
`docs/product-requirements.md` lists "provider events and reviewable
suggestions" as a real requirement. Needs: a chosen data source (no candidate
picked yet), a fetch/detection design, and — separately — a lot-mutation
design for structural corporate actions (split, merger, spin_off,
ticker_change, delisting, `corporate_action`), which `AcceptSuggestion`
currently rejects outright since only the `dividend_income`
proposed-transaction kind (dividend, distribution, cash_in_lieu,
return_of_capital) is implemented.

### T-48 TypeScript 7 upgrade blocked by `openapi-typescript` `[ ]`

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

### G-02 Frontend money logic is effectively untested `[x]`

**Done 2026-08-08** (editor half, then the investment forms). From
`docs/reviews/test-coverage-review-2026-07.md` G-02. The gap was never
"components lack tests"; it is that **money logic living inside `.svelte`
files cannot be tested at all**, so the work is extraction, not test volume.

The backend equivalent of this gap is closed — merged Go coverage is 75.2%
with a CI floor (G-07, closed 2026-08-07) — which is what made the frontend
the remaining blind spot.

**Done (2026-08-08).** `frontend/src/lib/money/amount.ts` is now the single
money-parsing and money-formatting layer: `parseDecimalAmount`,
`formatLedgerAmount`, `negateCoefficient`, `inflowPositiveAmount`, and the
scale-aware `commodityImbalance`. It replaced two divergent copies of the
same logic — the private helpers inside `transactions/transaction-editor.svelte`
and `reconcile/statement-balance.ts`, which is now a thin alias over it. This
is the frontend counterpart of the backend's `exact.ScaledInt` consolidation
(T-41): one place where scale alignment happens, so a fix lands once.
`money/amount.test.ts` table-tests it on all four cases that have burned this
repo — cross-commodity scale mismatch, values past
`Number.MAX_SAFE_INTEGER`, decimal-comma input, negative/zero amounts —
plus the backend's 38-digit and 24-scale ceilings. Two real defects fell out
of the extraction: T-45 and T-46 (see `docs/reviews/resolved-backlog-2026-07.md`).

**Done (2026-08-08), and it turned up a live 100x bug — see T-47.** The three
investment forms held **seven** copies of three helpers, one more than the
survey above counted: `dividend-form.svelte` also carried its own `toSafeInt`
beside `rawLedgerToDisplay` and `parseDecimalField`.

The non-negative note above was exactly right and was the shape of the work.
The old parsers rejected a leading `-` only as a side effect — they
concatenated the integer and fractional digits and ran `/^\d+$/` on the
result, so `-5.00` built `"-500"` and failed the digit check.
`parseDecimalAmount` handles signs correctly, so the swap needed an explicit,
named non-negative guard per form or negative quantities would have started
being accepted.

Because there is **no component-test harness** in this project (no
`testing-library`, no `jsdom`/`happy-dom` — vitest runs plain `.ts` only), a
per-form behaviour change could not be tested in place. The validation was
extracted to `lib/investments/form-amounts.ts` (`parseMagnitude`,
`parseMoneyMagnitude`, `parseTradeAmounts`, `parseDividendAmounts`) with 35
named tests, and the components now just call it.

`toSafeInt` deliberately did **not** move onto `$lib/money`. It is not money
arithmetic that drifted; it is the adapter for the `integer/int64` contract
quirk described below. It now lives once as `toInt64Coefficient` in
`lib/api/investments.ts`, next to the contract it adapts — putting a
`BigInt`→`Number` converter into `amount.ts` would contradict that module's
invariant that no `Number` ever touches a coefficient.

Also fixed in passing: `dividend-form` and `sell-form` returned silently when
an amount failed to parse, so a typo left the submit button doing nothing with
no explanation. Both now surface an error, as `buy-form` already did.
`sell-form`'s debounced preview keeps its silence for half-typed values but
reports `too_large` immediately, since that will not become valid by typing
more.

G-02's stated scope — the editor, the reconcile form, and the investment
forms — is now closed. A sweep done afterwards found three further `.svelte`
files with private money math that were never part of that scope; they are
tracked separately as G-09 rather than reopening this item.

E2E growth is tracked separately (T-20, R3a) and is not a substitute:
Playwright is still an intentionally unscheduled CI decision.

### G-09 `.svelte` files carrying private money math `[~]`

Found 2026-08-08 by sweeping every `.svelte` file for inline coefficient
arithmetic, once G-02's investment forms were done.

**Done — `lib/transactions/category-transactions.svelte` (2026-08-08).** It
held a private `rescaleUp` plus per-commodity scale-aware summing with its own
income/liability/equity sign switch. That switch is the ledger's **normal-sign
convention** — the same rule `inflowPositiveAmount` applies to a single posting
and `balanceMapToQuantities` applies on the backend — so the component was
carrying a third copy of a rule that already existed twice. Retired onto a new
`sumByCommodity` in `$lib/money/amount.ts`, table-tested for the sign rule per
account class, scale alignment, commodity separation, refund netting, and
exactness past `Number.MAX_SAFE_INTEGER`.

**Not a defect after all — `lib/investments/gains-report.svelte`.** Recorded
initially as the highest-priority item of the three, on the strength of a
`BigInt(Math.trunc(Number(value)))` at line ~55. Checked properly: that
conversion feeds **only `gainClass`**, whose entire output is a CSS colour
chosen by a sign comparison, and `Number()` rounding preserves sign. Also
checked and correct: `formatGain(entry.realized_gain_value,
entry.proceeds_scale)` uses `proceeds_scale` deliberately — the OpenAPI
description of `realized_gain_value` is "proceeds_value − disposed_basis
(**aligned to proceeds_scale**)", so there is no `realized_gain_scale` to use.
And `formatScaledValue` in `investment-labels.ts` is already a thin alias over
the shared `formatQuantity`, not a duplicate. Nothing to do here.

**Open, and larger than first recorded —
`routes/app/settings/currencies/+page.svelte`.** Two separate issues in
`scaledRatioToDecimalText` (lines ~391–419), which renders an FX rate as a
ratio of two scaled values:

1. **Ad-hoc truncating division on an implied price.** It does integer division
   and strips trailing zeros, truncating at 8 decimals. `ledger-invariants`
   states that division and implied prices round **half-up via the shared
   helpers** and that ad-hoc rounding is never to be written. Truncation on a
   *displayed* rate is defensible, but it is inconsistent with the stated rule
   and with `scaledDivision` on the backend, and the difference is visible to
   the user at the 8th decimal.
2. **`Number(decimalText)` before formatting.** The function computes an exact
   decimal string and then throws the exactness away to hand a `double` to
   `Intl`. Harmless at realistic FX magnitudes, but it is the one remaining
   place in the frontend where a `Number` touches a computed money value, and
   `formatQuantity` exists precisely so it does not have to.

Issue 2 is a mechanical fix. Issue 1 needs a decision first — whether a
displayed rate truncates or rounds half-up — so this was **not** folded into
the 2026-08-08 sweep silently. Worth pairing with R16, which already owns the
`integer/int64` money-field inconsistency in the investments contract.

### G-08 Amount input is not locale-aware `[~]`

Found while doing G-02's first half, 2026-08-08. `frontend/src/lib/money/amount.ts`
reads "," as a thousands separator, which is right for the only locale the app
shipped when this was written (`en`) and wrong the moment a decimal-comma
locale is added: a user typing `1,50` means 1.5, not 150. Five non-English
locales (es, fr, nl, de, ru) shipped 2026-08-19 (see `todo.md`), which makes
this a live gap rather than a hypothetical one — none of the five is a
decimal-comma locale for `Intl` **display** purposes yet, since `formatQuantity`
already reads the separator from the active locale; only **input parsing**
still hard-codes the `en` convention.

The immediate hazard is closed (see `docs/reviews/resolved-backlog-2026-07.md`
T-45/T-47) — malformed grouping is now rejected rather than silently mangled,
so the user gets a rejection instead of a 100× error. What is missing is the
real fix: resolve the separator from the active locale, in both directions
(parsing input and rendering an editable value back into a form field), the
way `app/import_locale.go` does for file imports.

Deliberately not done yet: guessing per-value is how T-36 happened, and this
blocks nothing today, but it is a prerequisite for the multi-currency/expat
direction in `docs/roadmap.md` and should be picked up before a decimal-comma
locale (es/fr/nl/de/ru all use one) reaches real users.

**The display-layer half of this item is resolved; only input parsing is
still open.** `dinero.js` 2.0.2 was a declared dependency with zero imports
anywhere in `frontend/src`. `docs/conventions.md` claimed it was used "for all
frontend money arithmetic … input parsing," which was never true — parsing has
always been hand-rolled, correctly, because Dinero's default calculator is JS
numbers and breaks past `Number.MAX_SAFE_INTEGER`.

Decided 2026-08-08, at the start of R2's remaining reports work, because
cashflow and CSV output both needed a named display formatter and deferring
again would have meant a third round of inline formatting:

- **`Intl.NumberFormat` alone is enough; `dinero.js` was removed** from
  `frontend/package.json`. Dinero formats through `Intl` regardless, so it adds
  no capability; reaching it safely would mean wiring its BigInt calculator to
  get arithmetic we already do and test directly; and it models money as an
  amount in a currency with one fixed exponent, which cannot represent a book
  holding a 24-scale crypto quantity beside a 2-scale euro.
- The formatter that already existed — `formatQuantity`, previously buried in
  `$lib/transactions/transaction-labels.ts` and imported by seven unrelated
  modules including reports — moved to **`frontend/src/lib/money/format.ts`**,
  the read-only display half of `$lib/money`. `format.test.ts` pins how it
  differs from `amount.ts`'s editable `formatLedgerAmount` so neither half
  drifts into the other's job.

What remains under G-08 is only the input direction: resolving the separator
from the active locale when parsing, and when rendering an editable value back
into a form field.

### T-49 `gofmt` drift in four backend files `[x]`

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

### T-50 Free-text payees never group in reports `[x]`

**Files:** `backend/internal/app/transactions_validate.go` (payee resolution:
`payee_name` is stored as free text and only `payee_id` links a payee record);
`backend/internal/app/reports_spending.go` (payee grouping).

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
  in development and carries no real data, so a one-off "link unlinked
  payees" backfill tool this item might otherwise have called for is **not
  needed and has not been built**. Should that change — a pilot user, an
  imported book — it comes back as new work.
- **Imports still produce unlinked names, deliberately.** A bulk import cannot
  confirm each unknown payee, so unrecognized names commit as free text. Until
  import review can resolve them, the path is to open the transaction and edit
  the payee, where the editor now forces link-or-create. Making import review
  resolve distinct unknown names in one pass belongs with import work (R5/R6),
  not with this item; recorded there rather than left implied here. This is
  unrelated to local's own T-44 (later-import-carries-earlier-trade, a
  holding-account genesis-date fix — see `docs/design/holding-account-opened-date.md`)
  despite the coincidental former shared number; see the note at the top of
  this file.

### T-51 Net-worth series re-reads the whole ledger once per bucket `[x]`

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

### T-52 One inline style was blocked by CSP on every page load `[x]`

Fixed 2026-08-18. The blocked style was never a `<style>` element — it was a
`style` *attribute*, which is why an earlier pass found no stylesheet in
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

### T-53 Investment trades cannot override a reconciliation lock `[x]`

**Files:** `backend/internal/api/investments.go` (`investmentTradeRequest` has no
`reconciliation_override`); `backend/internal/app/investments.go`
(`InvestmentTradeInput.ReconciliationOverride` exists and is honoured).

Found 2026-08-19 while adding the write-off item (T-38, see
`docs/reviews/resolved-backlog-2026-07.md`). Every investment trade goes
through the transaction write guard, so a buy, sell, or write-off dated inside a
reconciled period is correctly refused with
`ErrReconciliationOverrideRequired` — but the investments API never exposed
`reconciliation_override`, so there was no way to proceed deliberately the way
the transaction editor allows.

**Backend done 2026-08-20.** `reconciliation_override` is now on
`investmentTradeRequest`, `dividendRequest`, and `reinvestedDividendRequest`,
carried into the service inputs, and specified in
`api/openapi/components/schemas/investments.yaml`.
`TestInvestmentTrade_ReconciliationOverrideProceedsAndInvalidates` proves a
backdated write-off is refused without the flag, accepted with it, and that the
checkpoint it backdates past ends up `invalidated` carrying the change reason —
a 201 alone would leave a checkpoint asserting a balance the new posting just
changed. Because T-38's write-off (see above) carries a **dedicated**
`InvestmentWriteOffInput` type rather than the shared trade type, it did not
inherit the override field for free; `WriteOffReconciliationImpact` and the
`reconciliation_override` field on `investmentWriteOffRequest` were added here
so write-off has the same capability as buy/sell/dividend.

**Done 2026-08-20.** The owner chose the preview endpoint over widening the
shared error envelope, so the investments routes now mirror the transactions
pattern. Spec-building was factored out of all five write paths into
`investmentTransactionPlan` builders (`buyPlan`, `sellPlan`, `writeOffPlan`,
`dividendPlan`, `reinvestedDividendPlan`), and preview routes plan the same
postings the write would without persisting anything:

- `POST /api/v1/investments/{buy,sell,write-off}/reconciliation-impact`
- `POST /api/v1/investments/{dividend,reinvested-dividend}/reconciliation-impact`

Sharing the builder is what makes the preview trustworthy — a preview that
guessed at the postings would name the wrong checkpoints, which is worse than
naming none — and
`TestInvestmentReconciliationImpact_NamesTheCheckpointsTheWriteInvalidates`
pins preview and write to the same answer rather than assuming it.

The buy, sell and dividend forms preview before submitting and, when
checkpoints are affected, show the same named warning the transaction editor
shows, over the same message catalog: invalidating a reconciliation from the
investments screen and from the register are the same event and must read
identically. `e2e/playwright/investments-reconciliation.spec.ts` walks it in a
browser, including that cancelling leaves the checkpoint active.

**Note for later:** the write-off route has typed client functions
(`previewWriteOff`, `recordWriteOff`, `writeOffReconciliationImpact` in
`frontend/src/lib/api/investments.ts`) but no form or screen calls them yet, so
write-off remains unreachable from the UI. That is a separate gap from this
item.

Separately fixed the same day: `ErrReconciliationOverrideRequired` was
**unmapped** in `writeInvestmentServiceError`, so it surfaced as a 500
"internal server error" rather than a 409 for buy and sell as well as
write-off. It told the user nothing and looked like a fault instead of the
deliberate refusal it is. Now a `CONFLICT`, matching the transactions API.

## Public-deployment security gates

**All closed as of 2026-08-07, parked 2026-08-19 (owner decision).** S-04
(lockout-safe throttle) and S-07 (authentication-event visibility) closed
2026-08-06; S-06 (multi-factor authentication) closed 2026-08-07 — see
`docs/reviews/resolved-backlog-2026-07.md`. The app is self-hosted locally for
now, so none of the three is scheduled work; the trigger to unpark is the
first time an internet-exposed deployment is planned, R3 at the earliest.

What remains is an operator step rather than a backlog item: exposing real
financial data to the internet requires the owner account to actually be
**enrolled** in two-factor authentication, and `REKENRAAM_SECRET_KEY` to be
configured. `docs/deployment-security.md` is the checklist.
