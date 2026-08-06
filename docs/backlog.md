# Technical Backlog

This is the **live** technical-debt list: items that are actionable now and are
not already scheduled as product work. It is intentionally short.

- **Feature sequence:** `docs/roadmap.md`.
- **Short-horizon queue:** `docs/todo.md`.
- **Shipped capability:** `docs/implemented.md`.
- **Resolved and deliberate decisions:** `docs/reviews/resolved-backlog-2026-07.md`.

Status legend: `[ ]` open.

## General

### T-43 User-created categories default to opening today `[ ]`

**Files:** `backend/internal/app/categories.go` (`cleanEffectiveFrom` default),
`backend/internal/api/categories.go`.

Seeded starter categories are stamped `0001-01-01`; a category the user creates
themselves gets today. So a category added now cannot take an imported
transaction from last year — `posting date is before account opened date`.

Lower severity than T-42 was: the category API accepts `opened_on` and
`effective_from`, so callers can work around it, and the import's default
category picker can point at a seeded one. But the default is still wrong for
the same reason T-42's was — a category is a classification bucket, not
something you "open" on a date, which is exactly why the seeded ones are
backdated. Found 2026-08-06 while fixing T-42.

Decide whether user-created categories should join system accounts and seeded
categories on the genesis date. Asset/liability accounts must **not** change:
`opened_on` is a real financial fact there.

### T-44 A later import carrying an earlier trade fails on the holding account `[ ]`

**Files:** `backend/internal/app/import_trading212_invest.go` (holding-account
creation uses the trade's own date as `opened_on`).

An import creates the holding account with `opened_on` set to the first fill it
happens to see, and never revisits it. If a later sync or a backfill then
carries an *earlier* trade for that instrument, the posting fails
`posting date is before account opened date`. Proven 2026-08-06 while fixing
T-42 (which removed the commodity half of the same problem).

Not a one-liner: the honest fix is to widen the account's `opened_on` backwards
when an earlier trade arrives, which collides with the locked-structural-fields
rule (`docs/conventions.md`: opened date is locked once an account has posted
activity). Alternatives are opening import-created holding accounts at the
genesis date, or rejecting the row with an actionable message telling the user
to adjust the account. Needs a design note before code.

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

