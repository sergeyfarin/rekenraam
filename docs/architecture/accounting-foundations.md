# Accounting Foundations

Last updated: 2026-05-12

This document captures the architectural decisions and required engineering
work to position Rekenraam as a **small-business accounting system with full
investment support** rather than the personal-finance-app scope currently
locked in by [`docs/product/v1-scope.md`](../product/v1-scope.md).

It is a sibling to the gap plan, not a replacement. The gap plan tracks
day-to-day work against the existing scope; this document defines a new
architectural baseline that, if adopted, **reshapes the v1 critical path**.

Read this with [`docs/product/v1-gap-plan.md`](../product/v1-gap-plan.md)
open in another tab; cross-references are dense.

---

## 1. Why this document exists

The current `v1-scope.md` defines a personal-finance product. The schema is
mostly built for that target (5 append-only chained tables, mutable reference
data, application-layer enforcement of locked ranges, an `audit_events` table
emitted from two service files).

We have since clarified that the intended product is a **small-business
bookkeeping tool with first-class investment support**: lots, corporate
actions, FX, capital-gains reporting, multi-currency, multi-user. That target
has stricter correctness requirements than personal finance. They are not
optional for a tool that intends to keep books that can be audited or used to
file taxes.

This document:

1. States the **accounting requirements** that a small-business system must
   satisfy (regardless of implementation).
2. Describes the **current state** of the repo against those requirements.
3. Lays out the **architectural foundations** needed to close the gap.
4. Proposes a **sequenced execution plan** and how it merges with the existing
   gap-plan phases.

Source-of-truth recommendation: this doc owns the *why* and the *architectural
shape*. The gap plan owns *which item ships when*. When the two disagree,
update both deliberately.

---

## 2. Product target: explicit statement

The product intent is a **self-hosted small-business accounting application
covering general-ledger bookkeeping, multi-account reconciliation, multi-
currency reporting, and full investment accounting** (lots, basis methods,
corporate actions, dividend treatment, capital-gains reporting). Users are
expected to range from "individual managing personal + business books" to
"small business with bookkeeper + accountant access."

This target is **stricter than `v1-scope.md` currently states**. Adopting
this document changes the v1 release gate — see §6.

### Explicit non-goals (still)

- Multi-tenant SaaS deployment with one Rekenraam instance per customer org.
- Country-specific tax-filing exports (1099-B export targeted; jurisdictional
  filing remains deferred).
- AR/AP, invoicing, customers/vendors — already deferred in `v1-scope.md` and
  this document does not change that.

---

## 3. Accounting requirements (non-negotiable for the target)

These are properties a small-business accounting system **must** have. They
are correctness requirements derived from GAAP/IFRS, common tax-jurisdiction
recordkeeping rules, and the conventions of every serious accounting product
on the market (QuickBooks, Xero, Sage, NetSuite, GnuCash, Beancount, etc.).

### R1. No hard deletes of accounting data

Once a transaction or master-data record participates in the books, it
**must physically survive forever**. "Delete" is implemented as a state
change (`voided` / `archived`) or as a new tombstone version, never as a
SQL `DELETE`. In several jurisdictions destruction of accounting records
is literally illegal.

### R2. Pre-reconciliation iteration is fine but must be reversible

Bookkeepers fix things constantly during the open period. Edits to
not-yet-reconciled transactions are allowed, but every revision must be
**recoverable** — the user (or an auditor) can roll back to any prior
version of a transaction. Append-only version chains satisfy this; the
existing `previous_tx_id` design is the right pattern.

### R3. Reconciled periods are immutable; corrections are reversing entries

Once a transaction is reconciled (or sits inside a closed period), it
cannot be edited or deleted in place. The standard accounting workflow is
to post a **reversing entry** plus, if needed, a new correct entry. Both
new entries are linked to the original via a `correction_of_*` reference.
The original transaction stays exactly as it was.

A small escape hatch — "amend reconciled tx with mandatory reason and
elevated permission" — is acceptable and matches what NetSuite/SAP allow,
but it must be auditable, attributed, and rarely-needed. Default UX is
the reversing-entry path.

### R4. Mandatory attribution: who, when, request-id, reason

Every mutation captures:
- `changed_by_user_id` (already done for chained tables)
- `changed_at` (already done as `created_at`)
- `change_request_id` (already done via request-context middleware)
- `change_reason` — **currently missing.** Auditors care *why* more than
  *what*. Without `reason`, the audit log is "user_42 changed payee #7 at
  14:23" which is useless when an auditor asks "why was this vendor
  renamed?" Reason can be system-generated for trivial changes ("import
  rule applied", "FX rate refreshed") but must always be present.

### R5. Versioned master data

Account names, vendor (payee) names, category names, commodity scales —
these *do* change over time, and the date a change happened matters.
"What was account #5000 called when the 2024 tax return was filed?" is a
real question that an auditor or accountant will ask. Versioning is
required not just for transactions but for everything that participates
in historical reporting.

### R6. Database-enforced integrity, not application-enforced

Application-layer enforcement is necessary but insufficient. Migrations,
ad-hoc fix-up scripts, future careless code, or direct-SQL emergency
edits — anything bypassing the service layer can corrupt the books.
Triggers (or constraints, or both) close that hole. The legacy SQLite
schema enforced append-only at the DB layer via triggers; the current
Postgres baseline does not.

### R7. Report reproducibility

Running the 2024 tax report tomorrow must produce **the same numbers** as
it did today, even if the data underlying it has since been amended.
Two ways to achieve this:

1. **Temporal versioning of everything that feeds reports**, with reports
   parameterized by an as-of time. Expensive.
2. **Snapshot the report inputs at run time** into a frozen artifact. The
   schema half-implies this via `report_runs.valuation_snapshot_id`, but
   `valuation_snapshots` / `valuation_snapshot_items` do not exist. The
   schema is broken in this respect.

Option (2) is what's needed and what's mostly already wired; see F4.

### Investment-specific extensions

#### R8. Lot history must be immutable

Once a lot is opened (acquisition) and closed (sale), the cost basis and
disposal date are tax-reportable facts. Mutating them retroactively
changes the reported capital gains. The current `lots` table is mutable
with no audit chain — **this is a tax-compliance gap.**

#### R9. Corporate-action history must be reproducible

A 2:1 stock split that happened in 2023 affected lot rewrites then. A
re-run of 2023 tax calculations needs to see lots *as they were after the
2023 split, not as they are now after subsequent splits and rewrites.*
`corporate_actions` is currently mutable.

#### R10. Price observations need correction semantics, not delete

Providers sometimes publish bad prices and correct them later. The
corrected observation should **supersede** the bad one without erasing
it; any report that ran against the bad price must remain reproducible.
The current `delete_market_price_observation` is a hard `DELETE`.

#### R11. Cost-basis method locked per holding

FIFO vs LIFO vs specific-lot is a tax-election decision per security.
Retroactively switching methods is a different transaction than switching
prospectively. The `cost_basis_profiles` enum exists, but the application
does not track per-holding history of which method was in force at any
given time.

#### R12. Bitemporal pricing

Two time dimensions matter:
- **Transaction time** — the date the price applies to (`price_date`).
- **Valid time** — when our system learned of it (`observed_at`, distinct
  from `created_at` for backfills).

A reproducible 2024 report must use prices that were *known on the report
run date*, not later corrections. Currently the schema tracks
`price_date` and `created_at` but doesn't reify the distinction
intentionally.

#### R13. Period close

A user-driven "close 2024" that:
- Locks every transaction with `occurred_date <= 2024-12-31` from further
  in-place edits.
- Triggers the reversing-entry workflow (R3) for any subsequent change.
- Rolls income/expense accounts into retained earnings (the schema has
  the system accounts; the close flow is partial in `services/admin.py`).

Distinct from per-account reconciliation. Currently absent as a
first-class feature.

---

## 4. Current state versus requirements

A condensed scorecard. Detail is in
[`v1-gap-plan.md` §Findings](../product/v1-gap-plan.md) and the audits
recorded in that file.

| Req | What we have | What's missing |
|---|---|---|
| R1 No hard deletes | 17 `session.delete(...)` sites in repositories | Most reference and pricing tables hard-delete. Planning tables (budgets, schedules, loans) hard-delete. Balance constraints hard-delete. |
| R2 Append-only pre-reconciliation | `transactions` + `accounts` + `account_balancings` chains work | No `rollback_to_version` API. The chain exists in schema; the UX/API for "undo to version N" doesn't. |
| R3 Reconciled immutability + corrective entries | `_ensure_unlocked` blocks edits in locked ranges | No `correct_transaction` service. No reversing-entry API. No UX hint pointing the user to the corrective path. Locked-range guard is app-layer only, no DB enforcement. |
| R4 Attribution incl. reason | `changed_by_user_id`, `_session_id`, `_device_id`, `_request_id` already on chained tables | `change_reason` column absent across the board. Audit emissions store only free-text `summary`. |
| R5 Versioned master data | `transactions`, `accounts`, `account_balancings`, `import_rules`, `report_definitions` are versioned (5 tables) | `payees`, `categories`, `commodities`, `tags`, `people`, `projects`, `institutions`, `lots`, `corporate_actions`, `price_observations`, `pricing_policies`, `pricing_source_assignments` are mutable. |
| R6 DB-enforced integrity | Application-layer enforcement throughout | No triggers, no temporal CHECK constraints. A raw `UPDATE transactions SET memo=...` succeeds and silently breaks the audit trail. |
| R7 Report reproducibility | `report_runs.params_hash + as_of_seq + valuation_snapshot_id` schema exists | `valuation_snapshots` / `valuation_snapshot_items` tables don't exist. `pricing_mode='frozen'` is structurally broken. |
| R8 Lot history | Lots track `cost_basis_minor`, can be closed by allocations | Lots are mutable, no audit chain, no version-pointers. |
| R9 Corporate-action history | `corporate_actions` table with rich enum and generated-transaction linkage | Mutable, not versioned. |
| R10 Price corrections | `observation_kind = 'valuation_override'` exists | Delete paths are hard `DELETE`. No supersession chain. |
| R11 Cost-basis lock per holding | `cost_basis_profiles` enum (FIFO/LIFO/avg/specific) | No history of which profile was active for which holding when. |
| R12 Bitemporal pricing | `price_date` + `created_at` | No explicit `observed_at` vs `effective_at` distinction. |
| R13 Period close | Per-account reconciliation lock works | No book-level period close. `services/admin.py` has a fiscal-year close path but it isn't surfaced as a first-class workflow. |

**Bottom line:** the codebase is at maybe 40% of what a small-business
accounting system requires. The append-only foundation exists for the
ledger; everything else (master data, pricing, lots, corp actions, audit
log, period close, report reproducibility) is missing or partial.

---

## 5. Architectural foundations (F-series)

Four foundational changes are required. Each is large enough to be a
distinct shipping unit. They are **prerequisites** to the small-business
target; without them, every other feature builds on shifting sand.

### F1 — Database-enforced audit log + no hard deletes

**Scope:** ~1 week.

- Add a generic `audit_log` table with `(table_name, entity_id,
  before_state JSONB, after_state JSONB, op, changed_by_user_id,
  changed_at, change_request_id, change_reason)`.
- Add a generic Postgres trigger function `audit_log_writer()` that
  captures `OLD` and `NEW` row state via `to_jsonb(...)` and inserts an
  `audit_log` row on every mutation.
- Apply the trigger to every business table.
- The trigger validates `changed_by_user_id` and `change_reason` are
  non-NULL; raises if they aren't. Application code must stamp these via
  request-context.
- Replace all 17 `session.delete(...)` calls with `state` transitions to
  `voided` / `archived`. Add `state` column to each affected table.
- Replace `list_*` filters that currently check `deleted_at IS NULL` with
  `state IN ('active', ...)` predicates.

**Acceptance:**
- Every `INSERT`/`UPDATE`/`DELETE` on a business table produces exactly one
  `audit_log` row.
- A new `pytest` rule scans the codebase for `session.delete(` and fails
  if any non-whitelisted occurrences appear.
- A direct-SQL `UPDATE` against any business table either succeeds and
  produces an audit row (mutable tables) or is rejected by trigger
  (append-only tables).
- The `change_reason` parameter flows end-to-end through every API
  endpoint that mutates business data.

### F2 — Version chains on master data

**Scope:** ~1 week.

Extend the existing `previous_X_id` pattern to: `payees`, `categories`,
`commodities`, `lots`, `corporate_actions`, `price_observations`,
`pricing_policies`, `pricing_source_assignments`. Tags/people/projects/
institutions are lower-stakes — audit-log-only coverage from F1 is
defensible for them.

For each promoted table:
- Add `previous_X_id` column + `UNIQUE(previous_X_id)`.
- Add `state` column (`active` / `voided` / `archived`).
- Service-layer writes create new versions instead of `UPDATE`.
- Reads via a `_current` view or an explicit `state = 'active' AND no
  newer version exists` predicate.
- Application FKs continue to reference the historical row id (matches
  the existing `previous_tx_id` model); a chain-walk helper resolves to
  the current head when the UI needs to display the "current" name.

**Open question — how to handle the chain-walk problem:**

When `splits.payee_id = 42` and the user renames payee 42 (creating
payee 99 with `previous_payee_id=42`), readers see "the old name" by
default. Three options:

- **(a) Chain-walk on read.** Readers resolve to head at query time. ~30
  meaningful join sites in `investments.py`, ~5 reports. Highest
  fidelity, most code touched.
- **(b) Logical entity_id** (separate `payee_registry` table holding the
  stable identity; versioned `payees` table FKs back; other tables FK
  the registry). Cleanest theoretical model; doubles table count.
- **(c) Update FKs on edit** (rename payee 42 also `UPDATE splits SET
  payee_id=99 WHERE payee_id=42`). Smallest reader change; weakens audit
  because old splits no longer reference what their payee actually was
  at the time.

Decision deferred until F1 lands. Recommendation: prototype on `payees`
with option (a), measure the reader-change burden, decide globally from
data.

**Acceptance:**
- Editing any covered master-data row produces a new version; the old
  row remains queryable.
- `list_*` returns only chain heads in `active` state.
- A new pytest verifies "after rename, original row is fetchable and
  shows historical name."

### F3 — Reconciled-immutability + corrective-entry workflow

**Scope:** ~3–4 days.

- Add `correction_of_tx_id` column to `transactions` (already references
  itself via `previous_tx_id`; this is a sibling concept for "this
  transaction is a correction posted against #N").
- New service method `correct_transaction(tx_id, correction_data,
  reason)`:
  - Posts a **reversing entry**: same accounts as the original, opposite
    signs, same date (or current date — configurable), memo prefixed
    "Reversal of #N", `correction_of_tx_id = N`.
  - Posts a **new entry** with the corrected values, `correction_of_tx_id
    = N`.
  - Both as a single atomic transaction.
  - Mandatory `change_reason`. Audited.
- New service method `rollback_to_version(tx_id, version)`:
  - Appends a new chain version of `tx_id` equal to a prior version's
    state.
  - Allowed only if no version in the chain is reconciled.
  - Mandatory `change_reason`.
- Modify `update_transaction(tx_id)` service:
  - If current version has `status='reconciled'` or any split is in a
    locked range: reject with **409 + structured error body** pointing
    at the corrective-entry path (`POST
    /api/v1/transactions/{id}/correct`).
  - Optional `force_amend=true` + non-trivial `reason` overrides the
    block. Restricted by RBAC ("editor with override" or admin only).
    Always logged with elevated audit severity.
- New API endpoints:
  - `POST /api/v1/transactions/{id}/correct`
  - `POST /api/v1/transactions/{id}/rollback`
- DB-level guard: a trigger on `transactions` (and `splits`) that rejects
  `UPDATE` and `DELETE`. Already enforced by the chain pattern in
  application code; this makes it a hard guarantee.

**Acceptance:**
- Editing a reconciled transaction via PUT returns 409 with a structured
  hint.
- `POST /correct` produces two new transactions linked via
  `correction_of_tx_id`, both audit-logged.
- `POST /rollback` works pre-reconciliation, blocked post-reconciliation.
- Direct SQL `UPDATE transactions SET memo=...` is rejected by trigger.

### F4 — Report input snapshots

**Scope:** ~3–4 days.

- Build `valuation_snapshots` and `valuation_snapshot_items` tables
  (currently absent — `report_runs.valuation_snapshot_id` is a dangling
  reference).
- Every report run with `pricing_mode='frozen'` materializes its inputs
  (prices used, FX rates used, balances at the time, lot positions at
  the time) into `valuation_snapshot_items` rows keyed by
  `valuation_snapshot_id`.
- Re-run with the same `valuation_snapshot_id` reads from the snapshot
  rather than live data → produces identical output regardless of
  subsequent edits.
- Pin every "core report" (trial balance, balance sheet, income
  statement, capital gains) to use frozen pricing by default; live
  pricing becomes an explicit opt-in via `pricing_mode='live'`.

**Acceptance:**
- A report run today and rerun tomorrow with the same
  `valuation_snapshot_id` produces byte-identical output even after
  intervening price observations and corrections.
- The "frozen" pricing mode is the default for accounting reports;
  "live" is opt-in.

### Foundation total

**~3 weeks of focused engineering, sequential.** F1 and F4 can be
partially parallelized; F2 depends on F1 (audit log needs to exist
before old rows start being preserved); F3 depends on F1 (corrective
entries need the audit log).

This is the floor. Calendar pressure is real.

---

## 6. Investment-accounting features (I-series) — post-foundation

After F1–F4, the following are conventional feature work. Each is
independent and can be sequenced based on user need.

| # | Feature | From gap plan | Notes |
|---|---|---|---|
| I1 | FX cross-rate triangulation | 1.6.4 | EUR→JPY when only EUR↔USD and USD↔JPY observed. |
| I2 | Effective-date supersession for source assignments | 1.6.5 | As-of pricing lookups pick correct row. |
| I3 | Rate-staleness warning | 1.6.6 | When last observation is older than `max_backfill_days`, surface in valuation views. |
| I4 | Mixed-consideration corporate actions | 1.6.2 | Cash + stock; cash-in-lieu for fractional shares. Currently recorded but not posted. |
| I5 | Short-cover semantics + cost-basis-for-shorts | 1.6.3 | Endpoint exists; basis handling for negative lots undocumented. |
| I6 | Cost-basis method history per holding | new (R11) | Per-holding effective-date history of which profile applied. |
| I7 | Wash-sale detection | new | Tax-loss-harvesting safeguards. ~3–5 days. |
| I8 | Reinvested dividends → lot opened_date = payment date | 2.3 (test coverage gap) | Schema supports; service-layer assertion. |
| I9 | 1099-B / capital gains export | new | Built on top of immutable lot history (R8). |

---

## 7. Reporting completeness (RP-series)

Required reports that don't exist yet. All depend on F4 to be
reproducible.

| # | Report | Notes |
|---|---|---|
| RP1 | Trial balance | First and most fundamental accounting report. As-of date. |
| RP2 | Balance sheet | Assets / Liabilities / Equity at a date. |
| RP3 | Income statement (P&L) | Revenue / Expense / Net income across a range. |
| RP4 | Account statement | Per-account register with running balance. Gap 1.5.1. |
| RP5 | Income-expense report | Same as P&L but with category drill-down. Gap 1.5.1. |
| RP6 | Capital gains report | Realized + unrealized; FIFO/LIFO/specific-lot per holding. |
| RP7 | Dividend / distribution report | Cash, reinvested, foreign-tax-credit candidates. |
| RP8 | Print-friendly views | Gap 1.5.4. |

Cashflow, category-spend, payee-totals, net-worth, account-trends are
already built. The accounting-fundamental reports above (RP1–RP3) are
not.

---

## 8. Period close (PC)

| # | Item | Notes |
|---|---|---|
| PC1 | Book-level period close | "Close 2024" locks all txns with `occurred_date <= 2024-12-31`. After close, edits route to the corrective-entry workflow (R3 / F3). |
| PC2 | Year-end roll-forward into retained earnings | Income/expense → `retained_earnings` system account. Partial in `services/admin.py`; promote to first-class workflow. |
| PC3 | Period-close audit row | Every close creates an audit entry recording the closing actor, time, and rolled-forward totals. |

---

## 9. Sequencing decision: how this merges with the existing plan

This is the part that's actually load-bearing. The existing plan has
shipped Phase 0 + 7 of 8 Phase 1 items and lists 12 Phase 2 items. Many
Phase 2 items become trivial (or are subsumed) once F1–F4 land. Some
become harder (need to be done on the new foundation). A few are still
needed regardless.

### What this changes about Phase 1

- **Phase 1 step 8 (Tauri removal)** stays. Not blocked by F-series; can
  ship in parallel with foundation work. Independent of accounting
  correctness.
- Phase 1 is otherwise unchanged.

### A new Phase 1.5: Accounting Foundations

**New phase between Phase 1 and Phase 2.** Calendar ~3 weeks.

1. **F1** — Database-enforced audit log + no hard deletes. ~1 week.
2. **F3** — Reconciled-immutability + corrective-entry workflow. ~3–4
   days. Depends on F1.
3. **F2** — Version chains on master data. ~1 week. Depends on F1.
4. **F4** — Report input snapshots. ~3–4 days. Can run in parallel with
   F2 or F3.

After Phase 1.5, the v1 release gate is updated to require:
- All business-table mutations go through audit_log.
- Reconciled transactions cannot be edited in place (only corrected).
- Master data has versioned history.
- Frozen-pricing-mode reports are reproducible.

### What changes about Phase 2

Phase 2 as originally scoped was "hardening of high-risk service code"
against the current architecture. With Phase 1.5 in place, several Phase
2 items get easier or disappear:

| Old Phase 2 item | After Phase 1.5 |
|---|---|
| 1. Reconciliation correctness | Still needed (overlap detection, race protection, void-of-reconciled policy). But "void-of-reconciled" is now defined by F3. |
| 2. Transactions correctness (DB-level locked-range) | Subsumed by F3 (trigger-enforced immutability). |
| 3. Investments math (FIFO/LIFO/etc.) | Still needed. Test coverage. |
| 4. Pricing supersession & triangulation | Becomes I1/I2/I3 above; still needed but on the new foundation (supersession via F2 chains, not delete + insert). |
| 5. Report cache invalidation | Largely solved by F4 (frozen snapshots don't need invalidation). |
| 6. Budget rollover / schedule recurrence / loan amortization | Unchanged. Independent. |
| 7. Auth depth | Unchanged. Independent. |
| 8. CSV export escaping | Unchanged. Independent. |
| 11. Health endpoint deauth | Unchanged. Trivial. |
| 12. Append-only audit trail for reference tables | Subsumed by F2. |

So Phase 2 shrinks to: reconciliation finishes (1), investment math (3),
budget/schedule/loan (6), auth depth (7), CSV export (8), health
endpoint (11), plus the test-coverage gaps in §2 of the gap plan.

### A new Phase 1.6 or Phase 5: Accounting Reports

The trial balance / balance sheet / income statement / capital gains
reports (RP1–RP7) need to exist for the system to be useful for the
target audience. None of them are on the current v1 scope; all should
be. They depend on F4 for reproducibility.

Recommend folding RP1–RP3 (the three foundational reports) into v1 must-
have, and pushing RP4–RP8 to "should have" or post-v1.

### A new Phase 1.7: Period close

PC1–PC3 are required for R13. Small (~3–5 days) but depends on F3 being
in place. Slot after Phase 1.5 finishes.

### Revised release gate

The current v1 release gate from `v1-scope.md` does not mention
accounting correctness. Adopting this document changes the gate to add:

- No hard deletes anywhere; audit_log captures every mutation.
- Reconciled transactions immutable in-place; corrective-entry workflow
  available.
- Master data history queryable.
- Trial balance, balance sheet, income statement reports available and
  reproducible.
- Period close + year-end roll-forward usable.

If we don't change the gate, this document becomes "v2 foundations" and
v1 ships as a personal-finance app.

### Total calendar impact

| Phase | Calendar | Status |
|---|---|---|
| Phase 1 (remaining: Tauri removal) | ~1 day | Open |
| Phase 1.5 (F1–F4 foundations) | **~3 weeks** | New |
| Phase 1.6 (RP1–RP3 foundational reports) | ~1 week | New (was implicit in v1-scope) |
| Phase 1.7 (PC1–PC3 period close) | ~3–5 days | New |
| Phase 2 (slimmed-down hardening) | ~1 week | Existed; now smaller |
| Phase 3 (frontend tests) | ~3–4 days | Existed |
| Phase 4 (nice-to-have) | optional | Existed |

**v1 ship-date moves out by roughly 4 weeks** if this plan is adopted.

---

## 10. Open questions

These need decisions before Phase 1.5 starts. Each is small but
load-bearing.

1. **`v1-scope.md` update.** The current scope says "personal finance
   web app." Adopting this document requires rewriting that to "small-
   business accounting + investments." Decide explicitly.
2. **F2 chain-walk strategy.** Option (a), (b), or (c) above. Prototype
   on `payees` to learn before committing.
3. **`change_reason` UX.** Mandatory on every write — does the UI show a
   modal? A free-text field on every form? System-generated for trivial
   changes? Needs a UX pass before the API enforces it across the board,
   or users will type "asdf" forever and the audit log will be useless.
4. **Period close granularity.** Month, quarter, or year? Many small
   businesses close monthly. Default to month, allow quarter/year as
   user preference.
5. **`force_amend` permission.** Who can override the reconciled-
   immutability lock? Default proposal: only book owners + a dedicated
   "amend reconciled" RBAC permission. Restrict in book_memberships.
6. **`audit_log` retention.** Permanent in v1 (regulatory-safe default).
   Long-term: per-book retention policy.

---

## 11. Recommendation summary

If the product target is **small-business accounting with investments**
(as discussed in conversation 2026-05-12):

- Adopt this document.
- Update `v1-scope.md` to match the new release gate (§9).
- Sequence: finish Phase 1 → Phase 1.5 (F1–F4 in order: F1, F3, F2, F4)
  → Phase 1.6 (RP1–RP3) → Phase 1.7 (PC) → slimmed Phase 2 → Phase 3.
- Accept the ~4-week additional calendar.

If the product target remains **personal finance web app** (current
`v1-scope.md`):

- Keep this document as the v2 foundations reference.
- Continue with the existing plan (Phase 1 step 8 → existing Phase 2).
- Add a lightweight SQLAlchemy `before_flush` audit listener (~80 lines)
  to give the *information* without the architectural change.
- Build `valuation_snapshots` to fix `pricing_mode='frozen'` regardless
  — it's useful even outside formal accounting.
- Defer F1–F4 and the rest of this document to a later major version.

The choice is product-shaped, not technical. The technical work falls
out cleanly once the product target is decided.
