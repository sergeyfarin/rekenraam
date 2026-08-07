# Technical Backlog

This is the **live** technical-debt list: items that are actionable now and are
not already scheduled as product work. It is intentionally short.

- **Feature sequence:** `docs/roadmap.md`.
- **Short-horizon queue:** `docs/todo.md`.
- **Shipped capability:** `docs/implemented.md`.
- **Resolved and deliberate decisions:** `docs/reviews/resolved-backlog-2026-07.md`.

Status legend: `[ ]` open · `[blocked]` open with a stated dependency that must
land first.

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

**All closed as of 2026-08-07.** S-04 (lockout-safe throttle) and S-07
(authentication-event visibility) closed 2026-08-06; S-06 (multi-factor
authentication) closed 2026-08-07 — see
`docs/reviews/resolved-backlog-2026-07.md`.

What remains is an operator step rather than a backlog item: exposing real
financial data to the internet requires the owner account to actually be
**enrolled** in two-factor authentication, and `REKENRAAM_SECRET_KEY` to be
configured. `docs/deployment-security.md` is the checklist.

