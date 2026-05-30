# Archive FastAPI Backend Review

This document captures what the archive Python/FastAPI backend actually built and the decisions embedded in it. Use this as a reference when deciding what to port to the Go/SvelteKit rewrite — not as a direct requirement source.

## What The Archive Backend Was

- **Stack:** FastAPI (async Python), SQLAlchemy ORM (async), Alembic migrations, SQLite via `aiosqlite` (default) with a PostgreSQL dialect also supported.
- **Shape:** Single container, serving the FastAPI API on port `16888` alongside the statically built SvelteKit frontend.
- **Auth:** Cookie-based sessions (`HttpOnly`, `Secure`, configurable `SameSite`). Full MFA support. Invite-based user onboarding. Admin bootstrap endpoint to create first user.
- **API prefix:** `/api/v1`

---

## Domain Areas And What They Could Do

### 1. Authentication (`/api/v1/auth`)

- Bootstrap endpoint: checks whether first admin exists; creates it if not.
- Email + password login → session cookie. Session expiry configurable (default 30 days).
- MFA: TOTP-based. Setup, confirm, disable. Login with MFA token as a second step.
- Password reset: request reset link via email, confirm with token.
- Invite flow: admin creates invite, user accepts at a separate URL.
- Logout (clears cookie). `/auth/me` returns current user.
- Device fingerprinting: each session records device fingerprint, user agent, IP. Device table tracks first/last seen per fingerprint.
- Trusted-proxy CIDR list for `X-Forwarded-For` handling.

**Decisions:**
- Passwords stored as hashed text (nullable for invite-pending accounts).
- MFA is optional per user but can be required. Not enforced by default.
- Session token stored as hash (not plaintext) in `auth_sessions`.
- Hard expiry: `expires_at` column; soft revocation: `revoked_at` column.

---

### 2. Books (`/api/v1/books`)

- A **book** is the top-level container for all financial data.
- Fields: `slug` (unique URL-safe key), `name`, `base_currency_code` (3-char ISO 4217).
- Read-only CRUD in the API in v1 (list, get by slug). Create-book UX was deferred.
- **BookMembership** joins users to books with a role: `owner`, `editor`, `viewer`.
- `book_state` table tracks per-book mutable state (e.g. fiscal year position) separately from the immutable book row.

**Decisions:**
- All financial entities carry `book_id` as a scoping FK. This is the multi-tenancy boundary even while runtime stays single-book.
- `base_currency_code` is a plain 3-char string, not a FK to a commodity at creation time. The commodity is created separately.

---

### 3. Commodities And Metadata (`/api/v1/metadata`)

- A **commodity** is anything with a price: currencies (EUR, USD), stocks, ETFs, crypto.
- Fields: `kind`, `symbol`, `name`, `scale` (decimal places, 0–12).
- A **category** is a hierarchical expense/income label. `parent_id` self-reference. `kind` field distinguishes income vs expense vs equity.
- A **payee** is a named counterparty on a transaction. `kind` field.
- A **tag** is a free-form label attachable to splits. Has optional `color`.
- A **person** is a named individual used for split attribution (e.g. household member cost sharing). `role` field.
- A **project** is a user-defined cost center or tracking bucket for splits.
- An **institution** is a financial institution (bank, broker). Links to `country`.
- A **country** table holds ISO country codes per book.

**Decisions:**
- Categories are per-book and hierarchical (tree, not flat list).
- Tags and categories are separate dimensions — tags are ad-hoc, categories are structured.
- Person-level split attribution was a first-class concept for shared-household tracking.
- All metadata entities are scoped to `book_id`.

---

### 4. Accounts (`/api/v1/accounts`)

Account types supported by constraint:
`cash`, `checking`, `savings`, `credit`, `loan`, `investment`, `asset`, `liability`, `income`, `expense`, `equity`

- **Hierarchy:** accounts have `parent_id` (self-reference), enabling account trees.
- **Commodity:** each account holds a specific commodity (usually a currency).
- **Booking policy:** `fifo`, `lifo`, `strict`, `average` — controls lot-matching order for investment accounts.
- **Lifecycle:** accounts track their lifecycle via `lifecycle_event` (`open`, `close`, `reopen`, `update`) and `effective_at` date.
- **System accounts:** certain accounts are flagged `is_system` and carry a `system_role` (`opening_balance`, `imbalance_import`, `income_summary`, `expense_summary`, `retained_earnings`). System accounts cannot be updated or deleted via normal API.
- **Closing validation:** dedicated endpoint checks whether an account can safely be closed.
- **Account balancings:** recorded balance checkpoints with `as_of_date` and `balance_minor`. Supports voiding with reason. Used to lock past reconciliation periods.
- **Balance checks:** status-tracked balance assertions (`recorded`, `matched`, `failed`, `unbalanced`, `balanced`).

**Decisions:**
- `previous_account_id` linked list pattern for account version history. Account updates create a new row rather than mutating the old one — audit trail via linked list.
- Closing an account is a lifecycle event, not a hard delete.
- `number_last4` stores the last 4 digits of a bank account/card for recognition.
- System accounts are auto-created during book setup; they can never be edited.

---

### 5. Transactions And Splits (`/api/v1/transactions`)

- A **transaction** has one or more **splits** (double-entry: at least two splits that sum to zero in the book's base commodity).
- Transaction fields: `occurred_date`, `occurred_at_utc`/`occurred_tz` (optional precise timestamp), `posted_date`, `payee_id`, `memo`, `reference`, `status`.
- Transaction statuses: `uncleared`, `cleared`, `reconciled`, `void`.
- `previous_tx_id` linked list for corrective entries: voiding a transaction and posting a correction creates a chain.
- `import_id` and `import_session_id` track where the transaction came from.
- Created-by attribution: `created_by_user_id`, `created_session_id`, `created_device_id`, `created_request_id` on every transaction and split.

Split fields:
- `account_id`, `commodity_id`, `amount_minor` (integer, never float).
- Optional: `category_id`, `tag_id`, `person_id`, `project_id`.
- `share_bps` — basis-point share for cost-sharing splits (e.g. 5000 bps = 50% of a household expense).
- `memo`.

API capabilities:
- List transactions with filtering: `book_id`, `account_id`, `payee_id`, `status`, date range, text search, amount range, sort, pagination (offset or cursor).
- CRUD: create, get, update, delete.
- Create cross-currency transfer (separate endpoint handling FX-pair splits).
- Duplicate a transaction.
- Bulk void a list of transaction IDs.
- Payee defaults: look up (and save) the usual category and memo for a payee.

**Decisions:**
- All monetary amounts stored as `BigInteger` minor units (e.g. cents). `scale` on the commodity determines precision. Never float.
- A `void` is a status on the transaction row, not a delete. Hard delete was permitted only for unreconciled/uncleared transactions via the API, but this was controversial. The archive docs mention this is an open question.
- Transactions can span accounts in different commodities (multi-currency). The commodity is declared per split.
- `previous_tx_id` with a unique constraint ensures a transaction can be corrected at most once by a single successor.

---

### 6. Reconciliation (`/api/v1/reconciliation`)

- **Start reconciliation:** given an account and `as_of_date`, returns the current balance and uncleared items.
- **Finish reconciliation:** marks selected transactions as `reconciled`, records an `AccountBalancing` checkpoint.
- Reconciliation locks past periods: you cannot post a transaction dated before the latest active balancing without unlocking.
- **Unlock:** removes the lock on a balancing period. Requires a reason and explicit confirmation.
- **History:** returns all past reconciliation balancings for an account.
- **Balance constraints:** user-defined balance assertions on an account at a date. Used to validate that the actual balance matches expectations.

**Decisions:**
- Reconciliation creates an `AccountBalancing` row (a signed balance snapshot) that acts as a lock floor.
- Unlocking moves the lock date back. All unlock operations require `confirm: true` to prevent accidental unlocks.
- Balance constraints are separate from reconciliation balancings. They can be used for sanity checks independent of the reconciliation workflow.

---

### 7. Imports (`/api/v1/imports`)

- **Import flow:** preview → apply rules → commit (two-phase).
- **Preview:** parses a file (CSV, QIF), returns draft transactions without writing to the DB. Supports source-format detection.
- **Apply rules:** takes a list of draft transactions and runs import rules against them, filling in payee, category, account.
- **Commit:** writes the draft transactions to the DB, creating an `ImportSession` record.
- Import rules: `rule_kind` (`payee`, `memo`, `amount`, `date`, `account`), `match_type` (`contains`, `equals`), `match_text`, optional amount/date ranges. Rules assign `target_account_id`, `target_category_id`, `target_payee_id`.
- `ImportSession` tracks the file hash, file name, size, status (`started`, `committed`, `abandoned`).
- `ImportTransactionKey` table deduplicates re-imported transactions by `account_id` + `import_id`.
- Import sessions list with their constituent transactions visible for audit.

**Decisions:**
- Import is always two-phase (preview, then commit). This prevents partial writes.
- File hash prevents duplicate-session creation for the same file.
- Import rules have a priority ordering and a soft-delete (`deleted_at`), not hard delete.
- The `imbalance_import` system account is the counterpart for single-sided import rows that don't yet have a matching split.

---

### 8. Exports (`/api/v1/exports`)

Endpoints:
- `GET /exports/accounts.csv` — all accounts for a book.
- `GET /exports/transactions.csv` — all transactions for a book.
- `GET /exports/registers/{account_id}.qif` — QIF register for a specific account.
- `POST /exports/reports/{report_kind}.csv` — export a report result as CSV.

**Decisions:**
- QIF export is per-account register, not a global dump.
- CSV format for accounts and transactions is flat (not double-entry split-level).
- Report CSV export is driven by the same report input schema as the in-app reports.

---

### 9. Reports (`/api/v1/reports`)

Report types:
- **Cashflow:** income/expense flows over a date range.
- **Net worth:** asset/liability snapshot over time.
- **Account trends:** balance history for one or more accounts over time.
- **Category spend:** spending by category over a period.
- **Payee totals:** total amounts by payee over a period.
- **Realized gains:** capital gains realized by selling lots (investment).
- **Unrealized gains:** open positions valued at current prices.
- **Portfolio performance:** return on investment over a period.
- **Positions:** current holdings per account.
- **Converted positions:** holdings converted to a single base commodity.
- **Currency exposure:** total exposure per currency across accounts.
- **Account valuation rows:** per-account market value snapshots.

Report definitions and runs:
- **Report definitions:** saved report configurations with a name and parameters. CRUD.
- **Report runs:** recorded executions of a definition, storing input params and result summary. Used for snapshot reproducibility.

**Decisions:**
- Reports are driven by POST bodies (not query params) to support complex filter inputs.
- Report runs capture input parameters so a run can be referenced later without re-running.
- Investment reports are tightly coupled to the pricing and lot system.

---

### 10. Pricing (`/api/v1/pricing`)

- **FX rates (daily):** per-book daily observation of exchange rates (commodity A → commodity B). Manual entry or auto-fetched.
- **FX rates (official):** separate table for authoritative/regulatory rates distinct from market rates.
- **Market prices:** per-commodity price observations at a point in time, with a quote commodity.
- **Price sources:** named data providers (e.g. an external API). `kind`, `provider`, optional `plugin_id`, `base_url`.
- **Commodity price source assignments:** map a commodity to a price source with a symbol, effective date range, and primary flag.
- **Pricing policy:** per-book configuration: refresh schedule (UTC hour/minute), max backfill days, staleness threshold, triangulation max hops (up to 4), rounding mode, `prefer_official_fx` flag.
- **Pricing refresh worker:** a background job that fetches prices from configured sources. The API can trigger a manual refresh run. Refresh state and run history are exposed.
- **Price triangulation:** if no direct A→B rate exists, the system can traverse up to N hops of intermediate commodities.

**Decisions:**
- Price observations are immutable records (not updated in-place). New observations replace old ones by date.
- Official FX rates and market rates are stored separately so reports can choose which to apply.
- The pricing plugin system (`plugin_id`) was sketched but not fully activated in v1.
- `price_observations` has `mode` column: `market`, `daily`, `official`, `transaction_implied`.

---

### 11. Investments (`/api/v1/investments`)

- **Investment instruments:** define financial instruments (stocks, ETFs, mutual funds, bonds, crypto, etc.) with symbol, ISIN, CUSIP, FIGI, exchange, issuer, country, quantity/price scales.
- **Lots:** track cost basis positions for investment accounts. Each lot has `account_id`, `commodity_id`, `opened_date`, `cost_basis_minor`.
- **Cost basis profiles:** define a calculation method (`fifo`, `lifo`, `average_cost`, `specific_lot`) and make one the default.
- **Buy/sell:** dedicated trade-entry endpoints that record the transaction and update lots according to the booking policy.
- **Dividends:** record dividend income, optionally reinvested (DRIP).
- **Corporate actions:** stock splits, mergers, spin-offs, etc. tracked as structured events.
- **Dividend income categories:** map a commodity to income categories and withholding tax rates.
- **Holding periods:** query lots by holding duration (short-term vs long-term capital gains).
- **Positions:** current open lot balances per account.
- **Portfolio performance:** time-weighted or money-weighted return calculation.

**Decisions:**
- Investment transactions go through the normal `transactions`/`splits` tables. The investment layer adds meaning (lot assignment, performance calc) on top.
- `quantity_scale` and `price_scale` are configurable per instrument (0–12 decimal places).
- Cost basis tracking is per-lot. The booking policy (FIFO/LIFO/etc.) lives on the account.

---

### 12. Planning (`/api/v1/budgets`, `/api/v1/schedules`, `/api/v1/loans`)

**Budgets:**
- Named budget with `period` (`monthly` or `annual`), date range, base commodity.
- `BudgetTarget` assigns a target amount to a category within a budget. Optional `rollover_enabled`.
- Budget variance endpoint: compare actual spending to targets for a given period start.
- Projected cash: forward-looking cash position based on scheduled transactions.

**Scheduled transactions:**
- Templates with `frequency` (`daily`, `weekly`, `monthly`, `yearly`), `interval`, `start_date`, `end_date`, `reminder_days`.
- `ScheduledTransactionOccurrence` tracks each instance: `pending`, `skipped`, `posted`.
- When an occurrence is posted, it links to the created `Transaction`.
- The API can post a scheduled occurrence as a real transaction.

**Loans:**
- Model a loan with principal, interest rate, term. Fields: `principal_minor`, `rate_bps`, `term_months`, `start_date`, `payment_day`.
- Amortization schedule generation (returns projected payment rows).
- Loan payment drafts: generates the split breakdown for a payment entry.
- `LoanPaymentDraft` includes principal portion, interest portion, fee.

**Decisions:**
- Budgets are per-book, not per-account. They aggregate across categories.
- Scheduled transactions mirror the transaction/split shape (same fields).
- Loans are modeled as planning aids, not as a separate account type. The loan account itself lives in the accounts table.

---

### 13. Admin (`/api/v1/admin`)

- **Runtime status:** DB connection health, SQLite PRAGMA state, worker status.
- **Integrity check:** runs SQLite `PRAGMA integrity_check` and `PRAGMA foreign_key_check`.
- **Fiscal year close:** closes a fiscal year by posting summary/retained-earnings transfer entries. Requires a `close_date`.
- **User management:** admin CRUD for users (create, list, update, deactivate, reset password).
- **Invite management:** admin creates invite tokens; users accept at a separate URL.
- **Book membership management:** assign/remove users from books with a role.
- **Audit event log:** admin view of the structured `audit_events` table (search by book, user, event type).
- All admin endpoints require `is_admin: true`.

---

### 14. Ergonomics / UX Helpers

**Saved transaction views (`/api/v1/search/transaction-views`):**
- Users save filter configurations (search text, account, date range, etc.) as named views.
- CRUD per user + book combination.

**Transaction templates (`/api/v1/templates/transactions`):**
- Named templates that pre-fill a transaction's splits, payee, memo.
- Posting a template creates a real transaction.

**Payee defaults:**
- The system learns the most common category/memo for each payee from history.
- Explicit overrides can be saved per payee + book + optional account.

**Markdown notes (`/api/v1/notes`):**
- Free-text markdown notes attachable to accounts or transactions.
- CRUD per note.

**User preferences (`/api/v1/preferences`):**
- Per-user: `default_book_id`, `locale`, `date_format`, `number_format`, `theme`.

**Profile update / password change (`/api/v1/auth`):**
- Users can update their own display name and password.

---

### 15. Audit Logging

Two parallel audit mechanisms:

1. **`audit_events` table:** Curated, service-emitted events. Each row has `event_type`, `target_type`, `target_id`, `summary`, `metadata_json`, and full attribution (user, session, device, request ID). Example: `transaction.created`.

2. **`audit_log` table (SQLAlchemy listener):** Raw before/after JSON snapshots for every `INSERT`, `UPDATE`, `DELETE` on any `__audit_logged__ = True` model. Attribution stamped from a `ContextVar` set at request entry. Catches all ORM mutations but not direct SQL.

**Decisions:**
- The SQLAlchemy listener is the v1 approach. DB triggers were planned for v2 to also catch direct SQL.
- Both tables carry attribution: `actor_user_id`, `actor_session_id`, `actor_device_id`, `actor_request_id`.
- The listener coerces all column values to JSON-safe types (Decimal → string, datetime → ISO string, bytes → hex).

---

## Key Data Model Decisions Relevant To The Rewrite

| Decision | Detail |
|---|---|
| All money is integer minor units | `BigInteger`, never float or Decimal storage. `scale` on commodity defines precision. |
| `book_id` on every financial table | Multi-tenancy boundary even in single-book runtime. |
| Commodity is the unit of account, not currency | Stocks, ETFs, crypto are first-class alongside currencies. |
| `previous_*_id` linked list for version history | Accounts, import rules, and account balancings use linked list pointers for audit chains, not update-in-place. |
| No hard delete of posted records | Void flag / `voided_at` / lifecycle close instead of row deletion. |
| System accounts are auto-created | `opening_balance`, `imbalance_import`, `income_summary`, `expense_summary`, `retained_earnings` per book. |
| Session token stored as hash | `token_hash` on `auth_sessions`, not the raw token. |
| Request context via `ContextVar` | User/session/device/request-ID propagated to all writes via a thread-local equivalent for async code. |
| SQLite as default, Postgres as secondary dialect | Schema and code written to be portable. SQLite triggers added at migration time for constraint parity. |
| Scale (0–12 decimal places) per commodity | Allows sub-cent precision for crypto and precise lot accounting for stocks. |
| `share_bps` on splits | Basis-points share for cost-sharing (household expense splitting) is first-class. |

---

## What Was Deferred Or Explicitly Out Of Scope In v1

- Plugin execution and frontend plugin slots.
- Open banking / bank feed connectors.
- Attachment/document uploads and OCR.
- Server-side undo/redo.
- Multi-book create/switch UX (schema was ready; UI was gated).
- Granular per-resource permissions beyond owner/editor/viewer roles.
- Arbitrary remote CSS loading.
- WebAssembly or sidecar runtime.
- Price provider plugins were sketched (`plugin_id`) but not executed.

---

## 2026-05-30 Consolidation Pass

This pass reviewed the FastAPI backend path and the archived requirements documents together:

- `.archive/apps/api/src/rekenraam_api`
- `.archive/apps/api/alembic/versions`
- `.archive/apps/api/tests`
- `.archive/docs/product/*.md`
- `.archive/docs/architecture/*.md`
- `.archive/docs/deployment/self-hosting.md`
- `.archive/docs/audits/python-backend-v1-audit.md`
- `.archive/SELF_HOSTED_MIGRATION_PLAN.md`

The useful archive material mostly falls into three buckets:

1. **Already active:** decisions now captured in `docs/product-requirements.md`, `docs/conventions.md`, `docs/early-architecture-decisions.md`, and ADR 0001.
2. **Adapt when the feature arrives:** good workflows that need Go/SvelteKit translation and narrower single-user scope.
3. **Do not port:** Python, Tauri, multi-user, or over-broad v1 choices that conflict with the current product direction.

### Active Or Already Locked

These archive decisions are already represented in active docs and should be treated as settled unless a later ADR changes them:

- One Go binary serves `/api/v1` and the static SvelteKit frontend.
- Docker Compose preserves the same app shape.
- SQLite is the primary database.
- `book_id` stays on core financial tables even while the runtime is single-book.
- Money and quantities are exact values, never floats.
- Account trees, double-entry-capable transactions, postings/splits, and ordinary-transfer semantics are the ledger foundation.
- Local browser auth and first-run owner setup are required before real data entry.
- Public deployments require HTTPS guidance and MFA before real financial data is recommended.
- CSV core-ledger export and QIF export are the first mandatory user-facing export formats.
- Attachments, plugin runtimes, bank feeds, cloud sync, hosted operations, and household sharing remain out of early scope.

### Adapt By Phase

**Phase 0 foundation:**

- Use the archive bootstrap shape as a model: status endpoint plus create-owner endpoint.
- Keep session tokens stored hashed in SQLite, with expiry and revocation fields.
- Keep request IDs and request-scoped attribution from the first protected write flows.
- Consider device/user-agent/IP attribution as audit-quality data, but avoid building a full device-management product early.
- Carry forward admin/runtime health ideas: SQLite connection state, migration version, configured data path, backup status, and integrity checks.
- Surface public-host misconfiguration warnings in-product over time; do not rely only on deployment docs.
- Prefer online SQLite backup or stopped-app backup over copying live WAL-mode database files.

**Phase 1 books, commodities, and accounts:**

- Extend first-run setup with default book, base currency, optional additional currencies, and required system accounts when book/account setup lands.
- Keep runtime single-book guardrails centralized; avoid scattered `book_id = 1` assumptions.
- Create system accounts with stable roles when account setup lands: `opening_balance`, `imbalance_import`, `income_summary`, `expense_summary`, and `retained_earnings`.
- Treat account close/reopen as lifecycle state, not deletion.
- Defer investment booking policies on accounts until investment workflows exist.
- Translate archived categories into the active model: friendly category UI mapped to income/expense accounts, not a separate ledger primitive.

**Phase 2 ledger transactions:**

- Preserve `previous_tx_id` or an equivalent version/correction chain.
- Keep a clear status model for unreconciled, cleared, reconciled, and voided records.
- Do not copy the archive's hard-delete endpoint for ordinary transactions.
- Keep payee defaults, saved transaction views, and transaction templates as high-value ergonomics after the core transaction workflow is stable.
- Use server-side transaction search and cursor pagination per active conventions, not archive-era offset pagination.
- Keep explicit cross-currency transfer workflows for later multi-currency support.

**Phase 3 reconciliation and reports:**

- Use account balancing rows as reconciliation checkpoints and lock floors.
- Require confirmation and a reason when unlocking a reconciled range.
- Prefer corrective-entry workflows once records are reconciled or period-closed.
- Decide before report implementation whether saved report runs are reproducible snapshots or live recalculations.
- If "frozen" or reproducible reports are promised, snapshot the inputs needed to reproduce them rather than only storing filter parameters.

**Phase 4 import and cleanup:**

- Preserve the two-phase import flow: preview, apply rules, commit.
- Preserve import sessions with source filename, size/hash, status, and committed transaction links.
- Preserve duplicate detection based on source transaction identity scoped by book/account.
- Use an `imbalance_import` system account for incomplete single-sided imports that need later cleanup.
- Keep import rules ordered and recoverable; avoid hard-deleting rules.

**Later planning, pricing, and investments:**

- Budgets are per-book and category/account based; rollover rules can wait.
- Scheduled transactions should mirror the transaction/posting shape closely enough that posting an occurrence creates a normal transaction.
- Price observations should be superseded or voided, not destructively deleted, once reports can depend on them.
- Keep official FX rates separate from market rates if regulatory/accounting reports later need that distinction.
- Keep investment lots and cost-basis methods layered over ordinary transactions/postings rather than building a separate ledger.
- Defer cost-basis history, corporate actions, wash-sale-like checks, and performance reporting until the core ledger is stable.

### Owner Questions Preserved

These are not settled by the archive and should remain visible before implementation reaches the relevant slice:

- What password reset flow fits a single-owner self-hosted app?
- What is the minimum MFA implementation required before public VPS real-data deployment?
- What exact columns and files define the first core-ledger CSV export?
- Is first QIF export per-account register only, or should a broader export arrive in the same milestone?
- Should change reasons be mandatory on every write, only on financial corrections, or only after reconciliation?
- Should report runs be reproducible snapshots in the first reporting milestone?
- How should price corrections and supersession work before investment and multi-currency reports depend on them?

---

## What Is Worth Adopting In The Go/SvelteKit Rewrite

These patterns and decisions from the archive are worth carrying forward explicitly:

- **Exact integer values with explicit scale and commodity** — already locked in current conventions.
- **`book_id` on all financial tables** — already in Go conventions.
- **System accounts with stable roles** — adopt when account creation is implemented.
- **`previous_tx_id` corrective-entry chain** — needed once reconciliation is built.
- **Two-phase import (preview → commit)** — proven UX; adopt for the import feature.
- **Import deduplication by `account_id` + `import_id`** — prevents re-import noise.
- **Import rules engine** — high value for CSV import usability; adopt in import phase.
- **Bootstrap endpoint before MFA** — `/auth/bootstrap/status` + `/auth/bootstrap/admin` pattern is clean.
- **Device fingerprint + session attribution** — worth carrying for audit quality.
- **Audit log via DB-level capture** — in Go, use SQLite triggers (not ORM listeners) for this.
- **Account balancings as reconciliation lock floor** — adopt in reconciliation phase.
- **Budget variance report** — category-level actual-vs-target is core planning UI.
- **Scheduled transactions** — high daily-use value for recurring bills.
- **Payee defaults (learned + explicit)** — materially speeds up manual entry.
- **Saved transaction views** — high value for repeat filter patterns.
- **Transaction templates** — useful for common split patterns.
- **QIF export per account register** — good candidate shape for the first QIF milestone; confirm exact scope before implementation.
- **CSV export for accounts and transactions** — already in roadmap.

---

## What Should Not Be Ported Unchanged

- **Multi-user model** — the archive had full invite/role/membership flows. The Go rewrite is single-user until an ADR changes that. Schema keeps `book_id` but auth is simplified.
- **MFA as optional per user** — for public VPS with real data, MFA should be on by default or required. Revisit per the open question in Phase 0.
- **Pricing plugin architecture** — defer until pricing is stable without plugins.
- **Loan amortization model** — defer to planning phase.
- **Investment lot tracking / cost basis profiles** — defer to investment phase.
- **Fiscal year close as an explicit API action** — revisit when reporting is stable.
- **`AuditEvent` + `AuditLogEntry` dual logging** — for Go rewrite, start with a single structured audit log implemented via SQLite triggers; consolidate later.
