# Implemented Features

This is the feature ledger: what Rekenraam actually ships today, by area, with
backend and UI status tracked separately. It replaces the per-step trackers
(`transaction-table-plan.md`, `phase-1-implementation-plan.md`,
`setup-auth-implementation-plan.md`, `fx-refresh-implementation-plan.md`) as the
single answer to "what is done."

- **Source of truth for "what's next":** `docs/roadmap.md`.
- **Short-horizon working queue:** `docs/todo.md`.
- **Source of truth for product intent and phase boundaries:**
  `docs/product-requirements.md`.
- **Technical debt and polish:** `docs/backlog.md`.
- **Documentation map:** `docs/README.md`.

Status legend: ✅ shipped · 🟡 backend only (no UI) · 🟦 partial · ⬜ not started.

Last reconciled with the codebase: 2026-07-13 (Trading 212 investment-import
hardening through migration `0011`; documentation consolidation; R2 net-worth
series backend/API foundation).

## Foundation (Phase 0) — ✅ Complete

| Capability | Status | Notes |
|---|---|---|
| SQLite migrations + schema version | ✅ | `backend/migrations/`, auto-run before serving. |
| Connection PRAGMAs (WAL, FK, busy timeout) | ✅ | `db/sqlite.go`; single-connection contract documented. |
| Browser first-run setup (owner → book → currencies → system accounts → categories) | ✅ | Persisted `setup_steps`, derived install state. |
| Auth: Argon2id, sessions, CSRF, origin checks | ✅ | `app/auth.go`, `api/auth.go`; rehash-on-login, dual-scope throttling. |
| Lockout-safe login throttle (approved devices) | 🟡 | `login_trusted_devices` (migration 0004) + `GET`/`DELETE /api/v1/auth/trusted-devices`, S-04 — backend only, no UI. A device that completes a successful login (or first-run setup) gets an HttpOnly approval cookie; its attempts then spend a device-scoped throttle budget instead of the shared username/IP budgets, so an attacker cannot lock the publicly-known owner username out. The cookie is a throttle-scope selector, **never a credential** — it authenticates nothing, and the device keeps the same 5-in-15 budget so a stolen cookie buys no extra guesses. Hash-at-rest, 180-day sliding expiry, revocable. |
| Authentication-event visibility | 🟡 | `authentication_events` (migration 0003) + `GET /api/v1/auth/events`, S-07 — backend only, no UI. Records login success/failure/blocked and logout with the proxy-aware client IP, attempted username, failure reason and request id; mirrored to structured `slog` (failures at WARN) so a log shipper works without querying SQLite. Never stores password material or session tokens. `failed_last_24h` is the brute-force signal. Pruned to a 90-day retention window by the existing daily session-cleanup pass. |
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
| Historical entry predating setup | ✅ | A commodity's first version is stamped `db.CommodityGenesisDate` (`0001-01-01`), not its creation date (T-42), so installing today and importing years of history works. When you enabled a currency is app bookkeeping, not a financial fact; real later changes are new versions with real dates. Account `opened_on` deliberately still rejects earlier postings. |
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

## Online Connections (R7) — 🟩 Trading 212 fully shipped (Slices 1–4b: cash movements + investment lots)

| Capability | Status | Notes |
|---|---|---|
| `internal/secretbox` (AES-256-GCM, random nonce, base64 wire format) | ✅ | `backend/internal/secretbox/secretbox.go`; pure stdlib, 11-test suite. |
| `REKENRAAM_SECRET_KEY` config (base64 32-byte key, optional boot) | ✅ | `internal/config/config.go`; absent = nil (boots); invalid = hard error; loss/rotation recovery documented in `README.md` and `docs/developer-workflow.md`. |
| `SESSION_LIFETIME_HOURS` config | ✅ | `internal/config/config.go`; default `720`, must be a positive integer number of hours; controls login-created session expiry. |
| Beta schema baseline (`0001_initial_schema.sql`) | ✅ | `import_connections` table + `connection_id` FK on `import_batches`. |
| `ImportConnectionRepository` (CRUD) | ✅ | `internal/db/import_connections.go`; conditional key rotation on update. |
| `ImportConnectionService` (probe-before-store, key masking) | ✅ | `internal/app/import_connections.go`; `ConnectionProber` interface; `NoOpProber` for Slice 1. |
| 4 REST endpoints (`GET/POST /import-connections`, `PATCH/DELETE /{id}`) | ✅ | `internal/api/import_connections.go`; `CONFIG_REQUIRED`/`PROVIDER_ERROR`/`CONFLICT` error codes. |
| OpenAPI spec + generated TS types | ✅ | `api/openapi/components/schemas/import-connections.yaml` + path files; `schema.d.ts` regenerated. |
| Frontend connections client | ✅ | `frontend/src/lib/api/connections.ts`; typed against generated schema. |
| Connections UI on import page | ✅ | Masked key hint list, add-connection form (probe-then-store), inline delete confirm. |
| **Slice 2: Trading 212 HTTP fetcher + adapter** | ✅ | `internal/onlinesource/trading212` (`Fetcher`: paging, 429/`Retry-After` backoff, cursor); `Trading212Adapter` (`internal/app/import_trading212.go`) registered in `NewImportService`; real `Trading212Prober` closes T-11. `stageParseResult` extracted from `StartImport` so file and fetch paths share staging logic (no queue wiring yet — Slice 3). |
| **Slice 3: Durable fetch worker + online batch flow** | ✅ | `app/import_fetch_worker.go` (`kind="import.fetch.trading212"`, same claim/lease/retry shape as `pricing_worker.go`); `POST /imports` content-negotiated (`application/json` → `202` fetch-driven batch, `multipart/form-data` unchanged `201`); `POST /import-connections/{id}/refresh` (incremental, `202`); atomic guard+create+enqueue via `ImportRepository.StartOnlineImportBatch` (one transaction, closing a real TOCTOU race — T-16) → `ErrImportFetchInProgress`/`409 CONFLICT`; terminal-vs-retryable fetch failure classification (401/403 fail fast, other errors retry up to 8 attempts) written to both `import_batches.status` and `source_meta_json` (the field the frontend actually polls); `import_batches.connection_id` now written + `connection_display_name` snapshotted into `source_meta_json` so deleting a connection doesn't erase batch provenance (closes T-12); pagination continuation past the fetcher's 50-page-per-call budget (T-14) via `reason="continuation"` work-item chaining; incremental cursor re-scans same-timestamp movements instead of dropping them, and refuses to follow an absolute `nextPagePath` (T-17). Frontend: per-connection Import/Refresh button + polling "fetching" step, handing off to the existing preview/commit UI unchanged. 15 service-level Go tests (`import_fetch_worker_test.go`, `fetcher_test.go`) including a 12-goroutine concurrency race test and a multi-chunk continuation test, a frontend unit test for the `source_meta` polling contract, plus manual HTTP smoke tests against a fake Trading 212 server. |
| **Slice 4a: scheduled auto-refresh (B-T212-SCHED)** | ✅ | Per-connection `auto_refresh_enabled` toggle drives `ImportService.StartScheduler`/`runDueTrading212AutoRefreshes` (`app/import_scheduler.go`) — a 24h-since-last-fetch cadence (not a fixed wall-clock time), reusing the existing manual-refresh path and its in-flight guard. Migration `0008`. |
| **Slice 4b: investment lot import (B-T212-INVST)** | ✅ | Order fills route through `InvestmentService.Buy`/`Sell` (real lots via `import_connection_holdings`, a per-connection per-instrument holding-account mapping); dividends through `InvestmentService.Dividend`. Instrument resolution: ISIN → ticker/symbol → create (`ResolveOrCreateInstrumentForImport`), creation deferred to commit time only (`docs/plans/import-connection-accounts-plan.md`, migration `0009`: `cash_account_id` + `import_connection_holdings`). Fetcher gained `FetchOrders`/`FetchDividends` against the real, spec-verified `/equity/history/orders`/`/equity/history/dividends` endpoints, sharing one generic `fetchPaginated[T]` pagination engine with the original cash-history `Fetch`. Multi-endpoint cursor tracking: `fetch_cursor` renamed to `transactions_cursor` + new `orders_cursor`/`dividends_cursor` (migration `0010`), the worker walking three stages per logical fetch. Rows that can't resolve (no cash account, no instrument match, insufficient lots, no dividend default) fall back to the pre-4b plain-cash-row behavior unchanged. P1 follow-up 2026-07-11: order fills now retain and sort on full `filled_at` to preserve intraday buy→sell lot order (T-28), and the investment transaction, lot/disposal, import identity, and staged-row commit marker are atomic in one SQLite transaction (T-26). P2 follow-up 2026-07-12: failed or fallback investment routing compensates its never-used setup without deleting referenced history (migration `0011`, T-29); settlement accounts require active, non-system assets (T-30/T-33); and transactions/lots carry Trading 212 provider provenance (T-31). Also found and fixed while building this: a severity-1, source-agnostic bug where `EntryKind: "main"` made every single `CommitImportBatch` call fail real validation since the import feature's first commit (T-22), and a holding-account creation date defaulting to "today" instead of the trade's own date. |

## Import Pipeline (Phase 4, Slice 1) — 🟦 Core pipeline + QIF shipped

| Capability | Status | Notes |
|---|---|---|
| `SourceAdapter` interface + auto-detect registry | ✅ | `app/import_adapter.go`; confidence-ranked selection. |
| QIF parser (full field set, splits, transfers, investment entries) | ✅ | `app/import_qif.go`; handles MS Money loose-QIF export format. |
| EU date and decimal-comma handling in file imports | ✅ | `app/import_locale.go` (T-35, T-36): profile `date_layout` / `decimal_separator` when set, otherwise whole-file date-order detection (a day above 12 settles it) and per-amount separator detection; all-ambiguous or contradicting files parse as `MM/DD` with a parse warning. |
| Import pipeline: parse → normalize → dedupe → stage → commit | ✅ | `app/import_service.go`. |
| SHA-256 fingerprint deduplication (within-batch + ledger-level) | ✅ | `import_commit_identities` table; `INSERT OR IGNORE` with conflict detection. |
| Import schema | ✅ | Included in the pre-beta Goose baseline (`migrations/0001_initial_schema.sql`). |
| 7 REST endpoints (`/api/v1/imports/*`) | ✅ | `api/imports.go`; auth-gated, CSRF-protected. |
| Transfer detection: QIF `[Account]` → `transfer_account_id` routing | ✅ | Parsed to `transfer_hint`; per-row account selector in preview UI. |
| Partial-commit semantics (per-row DB tx, failures don't block others) | ✅ | |
| Preview UI: upload → per-row account / currency / category / transfer-account assignment | ✅ | `routes/app/import/+page.svelte`. |
| Commit result UI with skip/fail counts | ✅ | |
| Import nav link in app shell | ✅ | |
| OpenAPI spec for import endpoints | ✅ | T-07 closed; all 7 import routes documented and frontend DTOs come from generated `schema.d.ts`. |
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
| Net worth read models | 🟦 | Single-point `GET /ledger/net-worth` plus exact calendar series `GET /reports/net-worth`; `/app/reports` consumes the date/bucket contract, while account/commodity filters remain pending. |
| Account balances read model | 🟡 | `GET /ledger/account-balances`; overflow-guarded (422 on precision limit). |
| Category totals (spending) read model | 🟡 | `GET /ledger/category-totals`. |
| **Reports UI (net worth / cashflow / spending)** | 🟦 | `/app/reports` ships net-worth date/bucket filters and an exact commodity-grouped table; spending and cashflow remain pending. |
| Cashflow read model | ⬜ | Not yet a dedicated endpoint. |

## FX & Pricing (Phase 6 foundations) — 🟡 Backend only

| Capability | Status | Notes |
|---|---|---|
| Durable background work queue (lease, retry, resume, bounded attempts) | ✅ | ADR 0010; restart-safe. Work gives up after its kind's attempt cap instead of retrying forever (T-39); failed pricing work is listed in source health and re-enqueued via `POST /pricing/background-work/{work_id}/retry`. |
| Demand-driven FX coverage (activate currency / backdated posting) | ✅ | Drafts/previews do not trigger downloads. |
| Price observations (manual, provider, FX, trade-implied) | ✅ | Source/quote-type/adjustment-basis series; valuation date preserved. Derived FX routes through up to `triangulation_max_hops` intermediate currencies, shortest chain first (T-40). |
| Price observation voiding | ✅ | `POST /pricing/prices/{price_id}/void` (T-37). Retires a poisoned observation with a required reason instead of editing or deleting it; cascades to every rate triangulated from it (derivation-leg lookup), one `audit_events` row per void referenced from each retired row (`voided_audit_event_id`, migration 0002). Voided rows leave all valuations and are listed only with `include_voided=true`. Not idempotent — a second void is 409. |
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
| Account-level cost-basis method | ✅ | `account_versions.cost_basis_method` (beta baseline). Exposed via account CRUD API. |
| Sell preview endpoint | ✅ | `POST /api/v1/investments/sell/preview` — read-only simulation using `SimulateDisposeLots` (always rolls back). Returns allocations + realized gain. |
| Zero-proceeds write-off (fund closure, worthless delisting) | 🟡 | `POST /api/v1/investments/write-off` (+ `/preview`), T-38, R16 slice 1 — backend only, no UI yet. A separate entry point from Sell on purpose: a mistyped sale amount must never silently become a total loss, so a **required reason** replaces the cash account/commodity/amount. Posts the two commodity legs only (holding credit + `commodity_trading` debit), disposes lots through the normal engine, and the whole remaining basis lands in realized gains as a loss. The loss is **not** posted to an expense account — realized P&L is a computed reporting value here, and open decision I-04 governs whether that changes. Records no trade-implied price (a zero price is rejected by `price_observations` and would corrupt price history). |
| OpenAPI spec for all 24 investment endpoints | ✅ | `api/openapi/components/schemas/investments.yaml` + 14 path files. Generated TS types in `schema.d.ts`. |
| Investments portfolio page (positions + lot drill-down) | ✅ | `routes/app/investments/+page.svelte`. Positions table; click a row to see open lots. Degrades gracefully when no market price. Investments nav link in app shell. |
| Security identity / provider matching | 🟦 | Trade autocomplete exists; full security master pending. |
| Buy / sell / dividend entry UI | ✅ | Buy form; sell form with server-computed preview + specific-lot picker; dividend + reinvested-dividend forms. Modal over positions page. |
| **Gains reporting** | ✅ | `GET /api/v1/investments/gains` — realized gains (lot events × cash proceeds join, FIFO arithmetic, date-range filter) + unrealized gains (positions × latest price). Portfolio/Gains/Suggestions tab UI. Sign convention: disposed_basis_value is stored negative; gain = proceeds + disposed_basis. |
| **Provider-event suggestions UI** | ✅ | Suggestions tab: pending suggestions with Accept/Ignore actions; history section. Accept parses `proposed_transaction_json` and posts a real transaction (currently only `kind:"dividend_income"`, covering dividend/distribution/cash_in_lieu/return_of_capital); ignore/accept both guard against re-transitioning a non-`suggested` suggestion. **No producer exists yet** — nothing in the app writes to `investment_provider_events`/`investment_event_suggestions` (verified 2026-07-15), so this UI has nothing to review until a fetcher is built (`docs/backlog.md`). Structural corporate actions (split/merger/spin_off/ticker_change/delisting/`corporate_action`) are explicitly rejected at accept time, not silently dropped — no lot-mutation design exists for them yet. |
| **Automation rules list** | ✅ | `GET /api/v1/investments/automation-rules`; rules listed in Suggestions tab (read-only; PUT genuinely replaces the full active set — any existing active rule omitted from a PUT is archived in the same transaction, fixed 2026-07-15). Rule *matching/execution* (auto-post dispatch against incoming provider events) has no consumer yet — moot until a producer exists. |

## Cross-cutting — ✅ in place

- Hybrid audit model: version/lifecycle tables record *what*; `audit_events`
  records *who/when/how/why* (request + session attribution).
- Backend-composed read models (no per-row frontend fan-out) per AGENTS.md.
- Mobile-responsive core workflows including transaction entry.
- Loading / empty / error / success states on shipped screens.

## Not started (see roadmap)

Exports (CSV/QIF), CSV/OFX/QFX import adapters, import profiles, budgets,
scheduled transactions, projected balances, loan/liability helpers,
multi-currency reporting, report snapshots, reports UI, and pricing UI.

Online import (R7) is fully shipped for Trading 212 (Slices 1–4b: connections,
fetch, durable worker, online batch flow, scheduled auto-refresh, investment
lot import — see "Online Connections" above, `docs/plans/trading212-import-plan.md`).
A second online provider is not started.
