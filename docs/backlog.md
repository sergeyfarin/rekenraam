# Technical Backlog

This is the **live** technical-debt list: items that are actionable now and are
not already scheduled as product work. It is intentionally short.

- **Feature sequence:** `docs/roadmap.md`.
- **Short-horizon queue:** `docs/todo.md`.
- **Shipped capability:** `docs/implemented.md`.
- **Resolved and deliberate decisions:** `docs/reviews/resolved-backlog-2026-07.md`.

Status legend: `[ ]` open.

## General

### T-42 A currency's enable date blocks earlier history `[ ]`

**Files:** `backend/internal/db/transactions_rules.go`
(`PostingCommodityRule`), `backend/internal/app/transactions_validate.go`
(`cleanPosting`).

Posting rules resolve the commodity version **as of the entry date**. A
currency's first `commodity_versions` row is effective from the day it was
enabled in the app, so **every transaction dated before setup is rejected** —
`posting date is before the commodity was enabled`. Found 2026-08-06 while
tracing the e2e transaction-entry failure; the misleading wording was fixed
then, but the underlying rule was not.

This collides directly with the announcement's centerpiece: a migrating user
installs today, enables EUR today, and imports several years of history.
Every row fails. The account-side equivalent is fine and intended —
`opened_on` is a real financial fact the user can set — but a currency's
*enable date* is app bookkeeping, not a financial fact, and arguably should
not constrain history at all.

Needs a product decision, not just a patch. Options: (a) commodity versions
default to an open-ended past effective date; (b) posting validation resolves
the commodity's **earliest** version when the entry date precedes it;
(c) keep the rule and let setup/import backdate the enable date. Schedule
with R5 (CSV import) at the latest, since that slice is the migration path.

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

## Public-deployment security gates

These do not block private/local development. They must be complete before
allowing real financial data on an internet-exposed deployment.

### S-06 Multi-factor authentication `[ ]`

**File:** `docs/product-requirements.md` (public-deployment requirement).

Public deployment with real financial data remains prohibited until MFA has an
approved design and implementation. Select and implement TOTP, WebAuthn, or an
approved equivalent before lifting this product gate.

