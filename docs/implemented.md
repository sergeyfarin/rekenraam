# Implemented Features

This is the feature ledger: what Rekenraam actually ships today, by area, with
backend and UI status tracked separately. It replaces the per-step trackers
(`transaction-table-plan.md`, `phase-1-implementation-plan.md`,
`setup-auth-implementation-plan.md`, `fx-refresh-implementation-plan.md`) as the
single answer to "what is done."

- **Source of truth for "what's next":** `docs/roadmap.md`.
- **Source of truth for product intent and phase boundaries:**
  `docs/product-requirements.md`.
- **Technical debt and polish:** `docs/backlog.md`.

Status legend: ✅ shipped · 🟡 backend only (no UI) · 🟦 partial · ⬜ not started.

Last reconciled with the codebase: 2026-06-28 (R1 reconcile workflow UI shipped — `routes/app/reconcile`).

## Foundation (Phase 0) — ✅ Complete

| Capability | Status | Notes |
|---|---|---|
| SQLite migrations + schema version | ✅ | `backend/migrations/`, auto-run before serving. |
| Connection PRAGMAs (WAL, FK, busy timeout) | ✅ | `db/sqlite.go`; single-connection contract documented. |
| Browser first-run setup (owner → book → currencies → system accounts → categories) | ✅ | Persisted `setup_steps`, derived install state. |
| Auth: Argon2id, sessions, CSRF, origin checks | ✅ | `app/auth.go`, `api/auth.go`; rehash-on-login, dual-scope throttling. |
| Operator owner-recovery (backup-first, override) | ✅ | `recover-owner` command. |
| `/api/v1` envelope, error codes, request IDs | ✅ | Inbound `X-Request-ID` honored. |
| i18n boundary (Paraglide/Inlang) | ✅ | All UI copy and built-in labels keyed. |
| Light/dark semantic theme tokens | ✅ | `settings/appearance`. |
| OpenAPI-first contract + generated TS client | ✅ | `api/openapi/`, `frontend/src/lib/api/schema.d.ts`. |

## Books, Commodities, Accounts (Phase 1) — ✅ Complete

| Capability | Status | Notes |
|---|---|---|
| Single runtime book (`books.id CHECK (id = 1)`) | ✅ | No book selector by design. |
| Embedded currency catalog + active-currency derivation | ✅ | Default currency is a preference, not a base currency. |
| Currency setup & management UI | ✅ | `settings/currencies`. |
| Commodity metadata (scale, max quantity scale) | ✅ | Lossless precision per ADR 0009. |
| Institutions: CRUD, archive/restore, versions | ✅ (API) / 🟦 (UI) | Backend + API client complete; standalone management UI minimal. |
| Accounts: classes, kinds, full lifecycle (close/reopen, archive/restore), versions | ✅ | `app/accounts.go`; append-only versioned tree. |
| Account list + account detail UI | ✅ | `routes/app/accounts`. |
| Account creation creates no balances | ✅ | Opening balances require posted transactions. |

## Ledger Transactions (Phase 2) — ✅ Complete

| Capability | Status | Notes |
|---|---|---|
| Transactions → versions → journal entries → postings model | ✅ | Immutable versioned ledger; debit-positive sign convention. |
| Per-commodity, scale-aware double-entry balancing | ✅ | `transactions_validate.go`. |
| Lifecycle: post, void, unvoid, soft-delete, restore, correct, approve | ✅ | See `docs/transaction-lifecycle` taxonomy. |
| Same-day ordering (global + per-account register) | ✅ | `transactions_move.go`, day-sequence migration. |
| Cursor pagination + FTS5 search | ✅ | Sanitized FTS phrase quoting. |
| Categories as income/expense accounts | ✅ | `routes/app/categories`; built-in keys localized. |
| Tags & payees: CRUD, archive/restore | ✅ | |
| Transaction editor (splits, multi-commodity) | ✅ | `transaction-editor.svelte`. |
| Global transactions page | ✅ | `routes/app/transactions`. |
| Account register route | ✅ | `routes/app/accounts/[id]/register`. |
| Category transactions route | ✅ | `routes/app/categories/[id]`. |
| Trash / recovery (soft-delete browse + guarded restore) | ✅ | `settings/trash`. |

## Import Pipeline (Phase 4, Slice 1) — 🟦 Core pipeline + QIF shipped

| Capability | Status | Notes |
|---|---|---|
| `SourceAdapter` interface + auto-detect registry | ✅ | `app/import_adapter.go`; confidence-ranked selection. |
| QIF parser (full field set, splits, transfers, investment entries) | ✅ | `app/import_qif.go`; handles MS Money loose-QIF export format. |
| Import pipeline: parse → normalize → dedupe → stage → commit | ✅ | `app/import_service.go`. |
| SHA-256 fingerprint deduplication (within-batch + ledger-level) | ✅ | `import_commit_identities` table; `INSERT OR IGNORE` with conflict detection. |
| 5 new DB tables + goose migration | ✅ | `migrations/0004_import_core.sql`. |
| 7 REST endpoints (`/api/v1/imports/*`) | ✅ | `api/imports.go`; auth-gated, CSRF-protected. |
| Transfer detection: QIF `[Account]` → `transfer_account_id` routing | ✅ | Parsed to `transfer_hint`; per-row account selector in preview UI. |
| Partial-commit semantics (per-row DB tx, failures don't block others) | ✅ | |
| Preview UI: upload → per-row account / currency / category / transfer-account assignment | ✅ | `routes/app/import/+page.svelte`. |
| Commit result UI with skip/fail counts | ✅ | |
| Import nav link in app shell | ✅ | |
| **OpenAPI spec for import endpoints** | ⬜ | Tracked as T-07; frontend uses handwritten types. |
| **Per-split category mapping** | ⬜ | All splits post to single category; per-split routing deferred to R6. |
| **CSV adapter** | ⬜ | R5. |
| **OFX/QFX adapter** | ⬜ | R6. |
| **Import history / audit trail UI** | ⬜ | R6. |
| **Batch rollback (void all committed rows)** | ⬜ | R6. |
| **Import profiles (saved column/account mappings)** | ⬜ | R5. |

## Reconciliation (Phase 3) — ✅ Core workflow shipped

| Capability | Status | Notes |
|---|---|---|
| Reconciliation sessions, checkpoints, finish/void | ✅ | `app/reconciliation.go`; API + UI. |
| Period-scoped reconciliation guard (edit/void/restore/move) | ✅ | Wired across all mutation paths; balance assertion enforced in DB tx. |
| Reconciliation-impact preview (named-checkpoint warnings) | ✅ | Surfaced in the transaction editor warning modal. |
| **Dedicated reconcile workflow screen** | ✅ | `routes/app/reconcile` (R1): pick account → statement date/balance → clear postings against a server-authoritative live difference → finish only at zero, or discard. Prior active checkpoints shown read-only. Reuses the existing typed client (`lib/api/reconciliation.ts`). |
| Void-checkpoint controls / out-of-session mark-cleared UI | ⬜ | API exists; deferred beyond the R1 trust loop. |

## Reports (Phase 3) — 🟡 Backend only

| Capability | Status | Notes |
|---|---|---|
| Net worth read model | 🟡 | `GET /ledger/net-worth`; no reports UI route. |
| Account balances read model | 🟡 | `GET /ledger/account-balances`; overflow-guarded (422 on precision limit). |
| Category totals (spending) read model | 🟡 | `GET /ledger/category-totals`. |
| **Reports UI (net worth / cashflow / spending)** | ⬜ | No `routes/app/reports`. |
| Cashflow read model | ⬜ | Not yet a dedicated endpoint. |

## FX & Pricing (Phase 6 foundations) — 🟡 Backend only

| Capability | Status | Notes |
|---|---|---|
| Durable background work queue (lease, retry, resume) | ✅ | ADR 0010; restart-safe. |
| Demand-driven FX coverage (activate currency / backdated posting) | ✅ | Drafts/previews do not trigger downloads. |
| Price observations (manual, provider, FX, trade-implied) | ✅ | Source/quote-type/adjustment-basis series; valuation date preserved. |
| Pricing sources, policy, source assignments, health | 🟡 | Full API (`/pricing/*`); no management UI. |
| Manual/scheduled refresh runs + history | 🟡 | API only. |
| **Pricing/FX management UI** | ⬜ | |

## Investments (Phase 6) — ✅ Core complete

| Capability | Status | Notes |
|---|---|---|
| Buy/sell with commodity-trading balancing | ✅ | `app/investments.go`; lossless implied-price rounding (half-up). |
| Dividend & reinvested-dividend (income/withholding defaults) | ✅ | Shared input validation; commodity-consistency guarded. |
| Cost-basis method: fifo / lifo / average_cost / specific_lot | ✅ | All four implemented in `db/investments.go`. Explicit allocations blocked for non-specific_lot (I-01 closed). lifo/average_cost now fully implemented (I-02 closed). Per-lot-own-rate average cost; closed lots absorb full remaining cost. |
| 3-tier cost-basis method resolution | ✅ | Per-transaction override → account default → global default → "fifo". `resolveCostBasisMethod` in `app/investments.go`. |
| Account-level cost-basis method | ✅ | `account_versions.cost_basis_method` (migration 0005). Exposed via account CRUD API. |
| Sell preview endpoint | ✅ | `POST /api/v1/investments/sell/preview` — read-only simulation using `SimulateDisposeLots` (always rolls back). Returns allocations + realized gain. |
| OpenAPI spec for all 24 investment endpoints | ✅ | `api/openapi/components/schemas/investments.yaml` + 14 path files. Generated TS types in `schema.d.ts`. |
| Investments portfolio page (positions + lot drill-down) | ✅ | `routes/app/investments/+page.svelte`. Positions table; click a row to see open lots. Degrades gracefully when no market price. Investments nav link in app shell. |
| Security identity / provider matching | 🟦 | Trade autocomplete exists; full security master pending. |
| Buy / sell / dividend entry UI | ✅ | Buy form; sell form with server-computed preview + specific-lot picker; dividend + reinvested-dividend forms. Modal over positions page. |
| **Gains reporting** | ✅ | `GET /api/v1/investments/gains` — realized gains (lot events × cash proceeds join, FIFO arithmetic, date-range filter) + unrealized gains (positions × latest price). Portfolio/Gains/Suggestions tab UI. Sign convention: disposed_basis_value is stored negative; gain = proceeds + disposed_basis. |
| **Provider-event suggestions UI** | ✅ | Suggestions tab: pending suggestions with Accept/Ignore actions; history section. Uses `POST .../accept` and `.../ignore`. |
| **Automation rules list** | ✅ | `GET /api/v1/investments/automation-rules`; rules listed in Suggestions tab (read-only; PUT replaces full set). |

## Cross-cutting — ✅ in place

- Hybrid audit model: version/lifecycle tables record *what*; `audit_events`
  records *who/when/how/why* (request + session attribution).
- Backend-composed read models (no per-row frontend fan-out) per AGENTS.md.
- Mobile-responsive core workflows including transaction entry.
- Loading / empty / error / success states on shipped screens.

## Not started (see roadmap)

Exports (CSV/QIF), CSV/OFX/QFX import adapters, import profiles, budgets,
scheduled transactions, projected balances, loan/liability helpers,
multi-currency reporting, report snapshots, reconcile UI, reports UI,
pricing UI.

Online import (R7) is planned but not started: Trading 212 is the first provider
(`docs/trading212-import-plan.md`), which also introduces the `internal/secretbox`
encrypted credential store the repo does not yet have.
