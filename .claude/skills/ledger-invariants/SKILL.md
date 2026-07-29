---
name: ledger-invariants
description: Financial correctness rules for Rekenraam's double-entry ledger. Use BEFORE writing or reviewing any code that touches money, transactions, postings, account balances, reconciliation, investments, lots, prices, or FX. Also use when deciding how to delete, edit, or correct financial records.
---

# Ledger Invariants

These rules are non-negotiable. A change that violates one is wrong even if all
tests pass. Full authority: `docs/conventions.md` (§ Product Naming, § Data And
Persistence) and `docs/product-requirements.md`. When in doubt, read those.

## Money and quantities

- **Never floating point.** Not in Go, not in SQLite, not in JSON, not in JS.
- Exact values = integer coefficient + scale + commodity id. The coefficient type
  is `exact.Coefficient` (`backend/internal/exact/coefficient.go`): a canonical
  signed decimal **string** in SQLite and JSON, max 38 digits, computed with
  `math/big`.
- **Never aggregate coefficient columns with SQLite `SUM`** — they are strings.
  Sum in Go with `big.Int` (see `backend/internal/app/ledger.go`).
- Scale ceilings: 24 for crypto, 12 for everything else; a commodity's own
  `max_quantity_scale` may be lower. Display scale is independent from storage
  scale — don't render trailing zeros just because they're permitted.
- Division/implied prices round **half-up** via the shared helpers
  (`scaledDivision`); never write ad-hoc rounding.
- Overflow surfaces to the API as HTTP 422 `LEDGER_OVERFLOW`, never a 500.
- Frontend money math uses Dinero.js v2 for display only; **canonical balance
  and report calculations happen in Go**, never in the browser.

## Double entry

- A transaction contains postings (canonical term; "split" is UI copy only).
- **Debit-positive sign convention**: positive = debit, negative = credit.
  Asset/expense increases are usually positive; liability/equity/income
  increases are usually negative.
- Every transaction must balance **per commodity, scale-aware**
  (`backend/internal/app/transactions_validate.go`).
- Transfers are ordinary balanced transactions, not a special type.
- Investment trades balance through the `commodity_trading` system account.

## Transaction lifecycle (exact taxonomy — misusing these words causes bugs)

- **Unsaved entry**: browser-only working copy, no DB row, triggers nothing
  (no FX coverage, no side effects). Not a status.
- **`draft`**: a real persisted `transaction_versions` row, excluded from ledger
  and reports. **System-only** — no user-facing "save as draft"; reserved for
  future producers (import review, scheduled generation). Drafts do **not**
  trigger background FX coverage — the coverage trigger fires on posted
  versions only, so a promoted draft's foreign-currency dates get coverage at
  promotion (see `implemented.md`: "Drafts/previews do not trigger downloads").
- **`posted`**: in the ledger and reports. Manual entry goes directly to posted.
  Posted ≠ reconciled; posted stays editable until reconciliation locks it.
- **`voided`**: stays visible in the UI marked voided (intentional reference
  record); can be unvoided.
- **Soft-delete**: `transactions.deleted_at` — looks deleted to the user, row
  kept for audit/recovery, events in `transaction_deletion_events`. It is a
  flag, **not** a lifecycle status, and independent of voided.
- **Hard delete is allowed only for never-posted drafts.** Posted records get
  void, soft-delete, or corrective entries. No exceptions.

## Reconciliation guard (the trust boundary)

- Reconciliation is account- and commodity-scoped; checkpoints are lock floors
  at `(statement_date, statement_account_sequence)`.
- Guiding principle: **any operation that changes a reconciled balance must be
  guarded** (explicit override + invalidation of that and all later active
  checkpoints), never silent. Two cases: (1) a reconciled posting's financial
  facts change; (2) a posting enters or leaves a reconciled period (create,
  edit, void, unvoid, soft-delete, restore, reorder across the boundary).
- Non-financial edits (category, payee, description, note, tags) never change a
  balance and are always allowed.
- Every new mutation path over postings must be wired through the guard — see
  how `app/transactions_write.go` and `app/reconciliation.go` do it. Forgetting
  the guard on a new path is a severity-1 bug.

## Audit model

- Hybrid: append-only version tables record **what** changed;
  `audit_events` records **who/when/how/why** — one row per user operation
  (not per column), created in the same DB transaction and referenced from
  every version row that operation produced.
- `audit_events.operation` is a stable dotted code (`account.create`,
  `transaction.correct`); non-empty, but not a DB enum.
- Do not remove version tables in favor of a leaner audit log. This was tried
  (branch `experiment-remove-versions`) and abandoned; versions are the model.

## Accounts and structure

- `book_id` stays in core financial tables even though runtime is single-book
  (`books.id CHECK (id = 1)`, `app.BookID` constant).
- Accounts with posted activity have **locked structural fields** (class/kind,
  parent, institution, default commodity, quantity scale, opened date);
  descriptive fields stay editable.
- Categories are income/expense accounts, not a separate ledger primitive.
- Built-in records use stable keys/codes; translated labels live in the
  frontend localization layer, never as canonical DB values.

## Prices, FX, investments

- Prices/FX use `price_series` + `price_observations` (exact integer+scale).
  Correct a price by **superseding or voiding**, never overwriting.
- Observations distinguish valuation date from recorded-at time; preserve
  provider timestamps.
- External market data is untrusted: provider dividends/corporate actions
  become **suggestions** unless an automation rule explicitly permits
  auto-posting.
- Lots and lot events are durable accounting facts. Cost-basis method is
  resolved 3-tier (per-transaction → account → global default → fifo) in
  `resolveCostBasisMethod` (`app/investments.go`). Explicit lot allocations are
  only legal for `specific_lot`. Never infer cost basis from current holdings.
- Never collapse per-lot acquisition/disposal history irreversibly.

## Dates and times

- Financial facts: calendar dates (`YYYY-MM-DD` strings on the wire; Go uses a
  plain string or validated date type, **not** `time.Time` which marshals RFC 3339).
- System facts: UTC timestamps (RFC 3339).
- User-facing recurring schedules: local wall-clock + IANA zone stored; actual
  runs recorded as UTC.

## Pre-merge checklist for ledger-touching changes

1. No float anywhere in the money path; no `SUM` over coefficients.
2. Balancing validated per commodity; overflow → 422 `LEDGER_OVERFLOW`.
3. Lifecycle words used exactly (unsaved ≠ draft ≠ posted; void ≠ soft-delete).
4. Every new/changed mutation path goes through the reconciliation guard.
5. One `audit_events` row per operation, same DB transaction as the versions.
6. No hard delete of posted records; corrective workflow instead.
7. Named backend test cases exist for the financial behavior
   (docs/conventions.md § Testing: financial invariants require named tests).
