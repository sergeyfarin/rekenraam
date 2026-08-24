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

Last reconciled with the codebase: 2026-08-20 (merge of two long-diverged
branches: R2 reports code-complete with filters/drill-down, T-37 price-void
with cascade, T-38 write-off with its own reconciliation-impact preview, the
`$lib/money` consolidation, MFA/login-throttle/auth-events, and five
non-English locales all landed together).

## Foundation (Phase 0) — ✅ Complete

| Capability | Status | Notes |
|---|---|---|
| SQLite migrations + schema version | ✅ | `backend/migrations/`, auto-run before serving. |
| Connection PRAGMAs (WAL, FK, busy timeout) | ✅ | `db/sqlite.go`; single-connection contract documented. |
| Browser first-run setup (owner → book → currencies → system accounts → categories) | ✅ | Persisted `setup_steps`, derived install state. |
| Auth: Argon2id, sessions, CSRF, origin checks | ✅ | `app/auth.go`, `api/auth.go`; rehash-on-login, dual-scope throttling. |
| Lockout-safe login throttle (approved devices) | 🟡 | `login_trusted_devices` (migration 0004) + `GET`/`DELETE /api/v1/auth/trusted-devices`, S-04 — backend only, no UI. A device that completes a successful login (or first-run setup) gets an HttpOnly approval cookie; its attempts then spend a device-scoped throttle budget instead of the shared username/IP budgets, so an attacker cannot lock the publicly-known owner username out. The cookie is a throttle-scope selector, **never a credential** — it authenticates nothing, and the device keeps the same 5-in-15 budget so a stolen cookie buys no extra guesses. Hash-at-rest, 180-day sliding expiry, revocable. |
| Two-factor authentication (TOTP + recovery codes) | ✅ | `app/auth_mfa.go`, `api/auth_mfa.go`, `internal/totp/` (dependency-free RFC 6238), migration 0005, S-06 — backend **and** UI (`settings/security`, plus the code step in the install gate). Once active, a verified password yields a five-minute single-use challenge cookie instead of a session; `POST /api/v1/auth/login/mfa` completes it with an authenticator code or one of ten single-use recovery codes. The shared secret is sealed with `REKENRAAM_SECRET_KEY` (absent ⇒ enrolment refused, never plaintext); recovery codes are SHA-256 hashed and stay usable even if the key is lost. Replay guard on the accepted time step; wrong codes spend the same 5-in-15 throttle budget as wrong passwords; enrol/disable/regenerate each re-confirm the password. A pending enrolment never gates a login. |
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
| Payee resolution on entry | 🟦 | A typed payee name resolves to the record that carries it on every write path, manual or import (exact match on the normalized name, which is unique among active payees). In the editor, a name no payee carries stops the save and offers fuzzy near matches (`minisearch`) before a record is created; confirmation is skipped when the name was not edited. `lib/transactions/payee-matching.ts` + `transaction-editor.svelte`; shared resolution lives in `cleanTransactionSpec`, so manual entry and import commit get it in one change. 🟦 because existing rows can still carry an unlinked `payee_name` — there is no history-repair tool, and none is needed today since the app carries no real data yet (backlog T-50). |

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
| **Slice 4b: investment lot import (B-T212-INVST)** | ✅ | Order fills route through `InvestmentService.Buy`/`Sell` (real lots via `import_connection_holdings`, a per-connection per-instrument holding-account mapping); dividends through `InvestmentService.Dividend`. Instrument resolution: ISIN → ticker/symbol → create (`ResolveOrCreateInstrumentForImport`), creation deferred to commit time only (`docs/plans/import-connection-accounts-plan.md`, migration `0009`: `cash_account_id` + `import_connection_holdings`). Fetcher gained `FetchOrders`/`FetchDividends` against the real, spec-verified `/equity/history/orders`/`/equity/history/dividends` endpoints, sharing one generic `fetchPaginated[T]` pagination engine with the original cash-history `Fetch`. Multi-endpoint cursor tracking: `fetch_cursor` renamed to `transactions_cursor` + new `orders_cursor`/`dividends_cursor` (migration `0010`), the worker walking three stages per logical fetch. Rows that can't resolve (no cash account, no instrument match, insufficient lots, no dividend default) fall back to the pre-4b plain-cash-row behavior unchanged. P1 follow-up 2026-07-11: order fills now retain and sort on full `filled_at` to preserve intraday buy→sell lot order (T-28), and the investment transaction, lot/disposal, import identity, and staged-row commit marker are atomic in one SQLite transaction (T-26). P2 follow-up 2026-07-12: failed or fallback investment routing compensates its never-used setup without deleting referenced history (migration `0011`, T-29); settlement accounts require active, non-system assets (T-30/T-33); and transactions/lots carry Trading 212 provider provenance (T-31). Also found and fixed while building this: a severity-1, source-agnostic bug where `EntryKind: "main"` made every single `CommitImportBatch` call fail real validation since the import feature's first commit (T-22), and a holding-account creation date defaulting to "today" instead of the trade's own date (later superseded by the genesis-date fix, T-42). |

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

## Reports (Phase 3) — ✅ All three reports, filters, drill-down, export, print, and charts shipped; R2 acceptance review closed 2026-08-19

| Capability | Status | Notes |
|---|---|---|
| Net worth read models | 🟦 | Single-point `GET /ledger/net-worth` plus exact calendar series `GET /reports/net-worth`; `/app/reports` consumes the date/bucket contract. Asset/liability split per bucket is still pending in the series response. |
| Shared report filter contract | ✅ | Repeatable `account_id` / `commodity_id` plus `include_descendants` on `/reports/*`, echoed as `query.filters` with the resolved descendant expansion. Descendants resolve per reporting date because parent links are versioned. Inaccessible IDs are `VALIDATION_FAILED`, never a quietly narrower result — and so is an id that is semantically wrong for its dimension: the account filter takes asset and liability accounts only, and `category_id` takes income and expense accounts only, rather than accepting an id the report would then skip. |
| Transaction list drill-down filters | ✅ | `GET /transactions` takes `date_basis=entry` and `category_type`; on the entry basis the date, category, and direction must hold for the same journal entry, so a drill-down reproduces a spending row exactly. `/app/transactions` reads its filters from the URL. |
| Spending / income read model | ✅ | `GET /reports/spending` ranks category or payee totals over a range, exact per commodity — ranking is exact and stays within one commodity: the commodity present in the most groups orders the report and is named back as `ranking_commodity_id`, groups with no total in it sort after by identity, and fractions count, so amounts under one unit rank by value instead of all tying at zero, with within-commodity shares as integer basis points and a per-row drill-down query. Transfers cannot enter the basis (they have no category posting). `grouping_policy: direct_postings` — parents do not absorb children. |
| Account balances read model | 🟡 | `GET /ledger/account-balances`; overflow-guarded (422 on precision limit). |
| Category totals read model | 🟡 | `GET /ledger/category-totals`; a building block, superseded for reporting by `/reports/spending`. |
| **Reports UI — net worth** | 🟦 | `/app/reports` ships date/bucket controls, account and commodity filters, an exact commodity-grouped table, and a signed column chart. Still 🟦 because the series response carries no asset/liability split, so the screen cannot show one. |
| **Reports UI — filter controls** | ✅ | Multi-select account, commodity, category, and payee pickers on `/app/reports`, each writing the URL so a link reproduces the exact report. Only the dimensions a report can express are offered; a spending-only selection survives a switch to net worth in the URL. The account picker offers exactly what the endpoints accept (asset and liability), and the commodity picker and the report screens share one label map, so a security selected as `AAPL` reads as `AAPL` in the table, chart caption, printout, and CSV instead of `Commodity #123`. |
| **Reports UI — spending / income** | ✅ | View switch, dense table, category/payee and spending/income switches, single-commodity bar chart with a non-colour cue for negative rows, all four screen states, and drill-down links to the transactions the row was summed from. A multi-commodity report names the commodity that ordered its rows, from `ranking_commodity_id`, rather than describing a per-commodity ranking the group-major table does not have. |
| **Reports UI — drill-down** | ✅ | A cashflow row's in/out figures link into the spending report for the same bucket, resolved accounts, commodity, and direction — the breakdown cashflow itself cannot offer without losing its reconciliation guarantee. Spending's account filter matches per journal entry so the two agree exactly. A spending row links to `/app/transactions` with the report's own semantics — entry-date basis, posted only, the row's category or payee and direction. Rows the transactions route cannot express (unattributed, or several accounts) stay unlinked rather than showing a different set. |
| Cashflow read model | ✅ | `GET /reports/cashflow` classifies movement of the selected liquid cash as inflow, outflow, and transfer/financing, per commodity per calendar bucket. Each measure carries its own scale and two measures of one commodity may differ, because postings are recorded at any scale the commodity permits and the measures accumulate independently; restating them at a common scale was tried and reverted, since deepening costs a digit against the 38-digit ceiling and can overflow a measure that was representable as recorded. Classification is exact, not heuristic: journal entries balance per commodity, so each counterpart is classified on its own class and `net_movement = operating_net + transfer_net` is an identity. A transfer inside the selected scope nets to an explicit zero. Default scope is a named liquid-cash set (cash, checking, savings, brokerage_cash), echoed as resolved account ids. An explicit scope takes any asset or liability account; a system account or a non-balance class is `VALIDATION_FAILED`, so the response can keep saying `excluded_system_roles: ["all"]`. Category/payee filters deferred — see the plan. |
| Report CSV export | ✅ | Every report view downloads its table as CSV with exact unformatted decimals and the commodity in its own column, followed by a context block naming the period, basis, every active filter, the report's own policy (cashflow's cash scope and transfer rule, spending's grouping policy), and the endpoint's `excluded_system_roles`. The context sits *after* the table so the header row is still first, and every row is padded to the table's column count so the file stays rectangular for a strict RFC 4180 reader, delegating the actual decimal-digit-shifting to `$lib/money/amount.ts`'s `formatLedgerAmount` (`lib/reports/report-csv.ts`) rather than a third inline formatter — one more copy of decimal formatting is exactly the documented recurring bug class in this codebase (T-36/T-45/T-47; see `backlog.md`). Shares export as integer basis points. RFC 4180 quoting, UTF-8 BOM for Excel. Filename names the report and its range. |
| Report URL contract | ✅ | The URL is the whole report state, and a link that arrives missing or with a malformed `start_date`/`end_date`/`bucket` — including a date-shaped string that is not a date, like `2026-02-31` — is repaired in place — the view, grouping, mode, and every repeated ID filter survive (`lib/reports/report-range.ts`, tested). Rebuilding the URL from defaults instead used to discard the whole selection. |
| Report error states | ✅ | A failed report shows the localized message for the API's stable error code and offers retry only where repeating the identical request could succeed (`lib/reports/report-error.ts`). `LEDGER_OVERFLOW` is exact arithmetic hitting the 38-digit limit, so it explains itself instead of inviting a retry that cannot work; all three reports have named 422 cases. |
| Report print layout | ✅ | Printing drops navigation, filters, the view switch, and the export button, keeping headings, the stated scope, tables, and charts. The interactive filter form is replaced on paper by the same lines the CSV carries — the shell prints the common context, each report prints its own policy and exclusions from the one list its export uses — so a printout and an export of the same report cannot describe it differently. Rows avoid page breaks and use borders that survive without printed backgrounds. |
| Report summary charts | ✅ | Spending has a ranked bar chart; net worth and cashflow share a signed column chart. One accent hue, no legend, direction carried by side of the baseline plus a hatch, single commodity only. The table is always the accessible source of truth. |
| Cashflow view | ✅ | `/app/reports?view=cashflow`: per-bucket in, out, operating net, transfers, and net movement, exact per commodity. Names the cash scope it measured and states the transfer policy on screen. Signed figures carry an explicit sign as well as a colour; responsive priority keeps period, in, out, and net movement at every width. |
| Report ID filters + drill-down | ✅ | Shipped 2026-08-18 (superseded the R2 plan's original "not delivered" gap): repeatable `account_id`/`category_id`/`payee_id`/`commodity_id` filters, plus drill-down from both spending and cashflow rows into `/app/transactions`. |
| Reporting-currency valuation method | ⬜ | Deliberately deferred, approved 2026-08-19 to build after R3 (named, auditable valuation method — not a silent conversion). Reports show unlike commodities separately and say so until it lands. |

## Exports (R3) — 🟦 Slices 1-2: flat ledger CSV, preview, and the scoped archive shipped

| Capability | Status | Notes |
|---|---|---|
| Ledger CSV export | ✅ | `GET /api/v1/exports/ledger.csv` streams one row per posting of the current version of every posted, non-deleted transaction, ordered by entry date, transaction, entry, then line — a total order, so two exports of one snapshot are byte-identical. Amounts render from the stored coefficient and scale through `exact.Decimal` (point separator, no group separators, debit-positive, stored scale kept). UTF-8 with BOM, LF endings, RFC 4180 quoting, header row first and no context block — unlike R2's report CSVs, which are snapshots of a screen. |
| System accounts in exports | ✅ | Included, deliberately, where every report excludes them: an export without the `commodity_trading` counterpart leg would emit transactions that do not balance. Pinned by `TestLedgerExportIncludesCommodityTradingPostings`. |
| Export record policy | ✅ | Drafts, voided versions, superseded versions, and soft-deleted transactions never leave through the export; the SQLite backup remains the audit-complete artifact. The policy is served with the preview as a sentence a manifest can carry verbatim. |
| Export preview | ✅ | `GET /api/v1/exports/preview` — posting/entry/transaction/account/commodity counts, earliest and latest entry date, the stable column list, the record policy, the named exclusions, and the empty-but-declared attachments block (R14a fills it). It exists because a streamed download commits its status code with the first byte. |
| Scope refusal on the flat file | ✅ | `from`, `to`, `date_basis`, `account_id`, `include_descendants`, and `commodity_id` are refused by name with `EXPORT_SCOPE_UNSUPPORTED`, not ignored: a scoped CSV with no manifest is an artifact nobody can reproduce. Scoped exports arrive with `bundle.zip` (slice 2). |
| Read-only connection + snapshot | ✅ | `db.OpenReadOnly` opens a second `mode=ro`/`query_only` pool; each export runs in one deferred read transaction. The main pool is `SetMaxOpenConns(1)`, so a long export on it would stall every request; WAL readers are concurrent with the single writer. |
| Export format contract | ✅ | `docs/adrs/0011-ledger-export-contract.md` — grain and order, entry-complete filter semantics, the `in_scope` definition, the seven-column trial-balance identities, amount rendering, and append-only schema stability. |
| Account paths | ✅ | Every row carries `account_id` (join key) and `account_path` (colon-separated hierarchy, current names). A system account is named by its role rather than left blank; the path builder refuses to loop on a cyclic parent chain. |
| Export archive | ✅ | `GET /api/v1/exports/bundle.zip` — `README.txt`, `ledger.csv`, `accounts.csv`, `categories.csv`, `payees.csv`, `commodities.csv`, `tags.csv`, `lots.csv`, `prices.csv`, `trial-balance.csv`, and `manifest.json` written **last**, so a truncated download fails to open rather than looking complete. The manifest carries a SHA-256 and byte count per file, the resolved query, the incomplete-transaction count, and the counterpart-posting count. |
| Scoped export filters | ✅ | `from`, `to`, `date_basis=entry\|transaction`, repeatable `account_id` with `include_descendants`, repeatable `commodity_id` — on the archive only. Filters select whole **journal entries**: every posting of a selected entry is exported, so the archive balances per entry under every filter, and `transaction_complete` reports per row which transaction groups are whole. Any account class is accepted, unlike the reports filter, because exporting everything that touched a category is a reasonable request. A rejected filter is a 400 before the first byte (`ValidateFilter`), since a stream cannot change its status afterwards. |
| Trial balance | ✅ | `trial-balance.csv` states `in_scope`, `opening_balance`, `exported_in_range_movement`, `exported_out_of_range_movement`, `excluded_in_range_movement`, `derived_closing_balance`, and `actual_closing_balance`, with three exact identities tested over the two shapes that break simpler models: a scoped export whose counterpart account has unrelated activity, and a transaction whose entries straddle the range under both date bases. The derived figure is never called the account's closing balance. Each figure keeps the scale it accumulated at — the same decision R2 reached for cashflow measures, for the same 38-digit-ceiling reason — except a figure that accumulated nothing, which takes its row's scale, because zero is the same number at every scale and deepening it can widen nothing. A derived total wider than a stored coefficient (a 38-digit posting plus a 2.50 one) is written exactly through `exact.DecimalFromBig` rather than failing the archive; the ceiling governs stored values, not decimal text. |
| Cross-language decimal rendering | ✅ | `fixtures/decimal-rendering.json` is one specification with two readers: `exact.Decimal` (Go) and `formatLedgerAmount` (TypeScript) both test against it, so the twin implementations cannot drift. |
| QIF export, backups, restore, self-check | ⬜ | Slices 3–8 of `docs/plans/data-portability-plan.md`. |

## FX & Pricing (Phase 6 foundations) — 🟡 Backend only

| Capability | Status | Notes |
|---|---|---|
| Durable background work queue (lease, retry, resume, bounded attempts) | ✅ | ADR 0010; restart-safe. Work gives up after its kind's attempt cap instead of retrying forever (T-39); failed pricing work is listed in source health and re-enqueued via `POST /pricing/background-work/{work_id}/retry`. |
| Demand-driven FX coverage (activate currency / backdated posting) | ✅ | Drafts/previews do not trigger downloads. |
| Price observations (manual, provider, FX, trade-implied) | ✅ | Source/quote-type/adjustment-basis series; valuation date preserved. Derived FX routes through up to `triangulation_max_hops` intermediate currencies, shortest chain first (T-40). |
| Price observation voiding | ✅ | `POST /pricing/prices/{price_id}/void` (T-37). Retires a poisoned observation with a required reason instead of editing or deleting it; **cascades** to every rate triangulated from it (`derivation_json`'s `legs[].observation_id`, breadth-first, bounded by `maxVoidCascadeDepth`), one `audit_events` row per void referenced from each retired row (`voided_audit_event_id`, migration 0002). Voided rows leave all valuations and are listed only with `include_voided=true`. Not idempotent — a second void is 409. Proven by `TestVoidPrice_CascadesToRatesTriangulatedFromTheVoidedLeg` (`backend/internal/app/pricing_test.go`). |
| Pricing sources, policy, source assignments, health | 🟡 | Full API (`/pricing/*`); no management UI. |
| Manual/scheduled refresh runs + history | 🟡 | API only. |
| **Pricing/FX management UI** | ⬜ | R11. |

## Investments (Phase 6) — ✅ Core complete

| Capability | Status | Notes |
|---|---|---|
| Buy/sell with commodity-trading balancing | ✅ | `app/investments.go`; lossless implied-price rounding (half-up). |
| Dividend & reinvested-dividend (income/withholding defaults) | ✅ | Shared input validation; commodity-consistency guarded. |
| Cost-basis method: fifo / lifo / average_cost / specific_lot | ✅ | All four implemented in `db/investments.go`. Explicit allocations blocked for non-specific_lot (I-01 closed). lifo/average_cost now fully implemented (I-02 closed). Per-lot-own-rate average cost; closed lots absorb full remaining cost. |
| 3-tier cost-basis method resolution | ✅ | Per-transaction override → account default → global default → "fifo". `resolveCostBasisMethod` in `app/investments.go`. |
| Account-level cost-basis method | ✅ | `account_versions.cost_basis_method` (beta baseline). Exposed via account CRUD API. |
| Sell preview endpoint | ✅ | `POST /api/v1/investments/sell/preview` — read-only simulation using `SimulateDisposeLots` (always rolls back). Returns allocations + realized gain. |
| OpenAPI spec for every investment endpoint | ✅ | `api/openapi/components/schemas/investments.yaml` + one path file per route under `api/openapi/paths/`. Generated TS types in `schema.d.ts`. |
| Investments portfolio page (positions + lot drill-down) | ✅ | `routes/app/investments/+page.svelte`. Positions table; click a row to see open lots. Degrades gracefully when no market price. Investments nav link in app shell. |
| Security identity / provider matching | 🟦 | Trade autocomplete exists; full security master pending. |
| Buy / sell / dividend entry UI | ✅ | Buy form; sell form with server-computed preview + specific-lot picker; dividend + reinvested-dividend forms. Modal over positions page. Money parsing/validation in all three runs through `lib/investments/form-amounts.ts` (35 named tests), not private per-form helpers (G-02). |
| **Gains reporting** | ✅ | `GET /api/v1/investments/gains` — realized gains (lot events × cash proceeds join, FIFO arithmetic, date-range filter) + unrealized gains (positions × latest price). Portfolio/Gains/Suggestions tab UI. Sign convention: disposed_basis_value is stored negative; gain = proceeds + disposed_basis. |
| **Zero-proceeds write-off** | 🟡 | `POST /api/v1/investments/write-off` (+ `/preview`), T-38, R16 slice 1 — a **dedicated** `InvestmentWriteOffInput` type, separate from the shared buy/sell trade type, with no cash account/commodity/amount fields at all and a **required `reason`** distinct from the ordinary `change_reason` — a zero on a sale reads as a typo, so the destructive path is not reachable by clearing one field. `PreviewWriteOff` mirrors sell's preview endpoint. Posts only the two commodity legs (holding credit + `commodity_trading` debit); the whole remaining basis lands in realized gains as a loss. The loss is **not** posted to an expense account — realized P&L stays a computed reporting value, and open decision I-04 governs whether that changes. Records no trade-implied price (a zero price is rejected by `price_observations` and would corrupt price history). Backend only: the typed client functions (`previewWriteOff`, `recordWriteOff`) exist in `frontend/src/lib/api/investments.ts`, but no form or screen calls them yet, so a write-off is still unreachable from the UI. |
| **Backdated investment trades into a reconciled period** | ✅ | Every investment write goes through the transaction reconciliation guard, and `reconciliation_override` is accepted on the buy, sell, dividend, reinvested-dividend, **and write-off** routes (T-53; write-off's own `WriteOffReconciliationImpact` was added during this work since its dedicated request type did not inherit the shared trade type's override field). Six preview routes — `POST /investments/{buy,sell,dividend,reinvested-dividend}/reconciliation-impact` and `POST /investments/write-off/{preview,reconciliation-impact}` — plan the *same* postings the write would, via shared `investmentTransactionPlan` builders, and report which checkpoints would be invalidated without persisting anything. The buy, sell, and dividend forms preview first and raise the transaction editor's own named warning, over the same messages, so invalidating a reconciliation reads identically from either screen. `e2e/playwright/investments-reconciliation.spec.ts` covers it end to end, including that cancelling leaves the checkpoint active. |
| **Provider-event suggestions UI** | ✅ | Suggestions tab: pending suggestions with Accept/Ignore actions; history section. Accept parses `proposed_transaction_json` and posts a real transaction (currently only `kind:"dividend_income"`, covering dividend/distribution/cash_in_lieu/return_of_capital); ignore/accept both guard against re-transitioning a non-`suggested` suggestion. **No producer exists yet** — nothing in the app writes to `investment_provider_events`/`investment_event_suggestions` (verified 2026-07-15), so this UI has nothing to review until a fetcher is built (`docs/backlog.md`). Structural corporate actions (split/merger/spin_off/ticker_change/delisting/`corporate_action`) are explicitly rejected at accept time, not silently dropped — no lot-mutation design exists for them yet. |
| **Automation rules list** | ✅ | `GET /api/v1/investments/automation-rules`; rules listed in Suggestions tab (read-only; PUT genuinely replaces the full active set — any existing active rule omitted from a PUT is archived in the same transaction, fixed 2026-07-15). Rule *matching/execution* (auto-post dispatch against incoming provider events) has no consumer yet — moot until a producer exists. |

## Localization — 🟡 translated, not natively reviewed

| Capability | State | Notes |
| --- | --- | --- |
| Translation boundary | ✅ | Paraglide JS over `frontend/messages/{app,settings}/<locale>.json`. All user-facing copy goes through it; built-in database records keep stable codes and resolve display labels here. |
| **Five non-English locales** | 🟡 | `es, fr, nl, de, ru` — all **1,170** messages translated in every locale (283 app + 887 settings), key sets and placeholder sets verified against `en.json`. 🟡 because they are **drafted from a glossary, not reviewed by native speakers**; the language settings page says so to anyone using a non-English locale. First three items for a reviewer: Spanish *Punteado* ("cleared"), Russian *партия* (tax lot), Dutch *Instelling* (financial institution, collides with *instellingen*). |
| Terminology governance | ✅ | `docs/localization-glossary.md` decides terms before strings are written, anchored to GnuCash, localized MS Money/Quicken, and per-market banking language. Two deliberate departures from the literal are recorded there: *commodity* → *instrument* (the literal means physical goods in all five), and *cleared* vs *reconciled* must not collapse into one word. |
| Language picker | ✅ | `/app/settings/language`, each locale offered by its own autonym. Resolution is `localStorage → browser preference → English`, with per-message English fallback so a missing key never renders blank. Covered by `e2e/playwright/language.spec.ts`. |
| Backend translation | 🟦 | Export headers and server-generated content are still English only — deferred to Phase 3 (`nicksnyder/go-i18n`). |
| Locale-aware formatting | ✅ | Numbers, dates, and money are formatted by `Intl` from the active locale; catalogs never bake in a separator or currency symbol. `$lib/money/format.ts` (`formatQuantity`) is the sole read-only display formatter; `$lib/money/amount.ts` is the sole editable-amount parser/formatter (G-08 display half; input-parsing half of G-08 stays open, see `backlog.md`). |

## Cross-cutting — ✅ in place

- Hybrid audit model: version/lifecycle tables record *what*; `audit_events`
  records *who/when/how/why* (request + session attribution).
- Backend-composed read models (no per-row frontend fan-out) per AGENTS.md.
- Mobile-responsive core workflows including transaction entry.
- Loading / empty / error / success states on shipped screens.

## Not started (see roadmap)

QIF export (exports slices 1-2 shipped — see "Exports" above),
CSV/OFX/QFX import adapters, import profiles, budgets,
scheduled transactions, projected balances, loan/liability helpers,
multi-currency reporting, report snapshots, and pricing UI. (Reports UI itself
shipped in R2 — see the Reports section above.)

Online import (R7) is fully shipped for Trading 212 (Slices 1–4b: connections,
fetch, durable worker, online batch flow, scheduled auto-refresh, investment
lot import — see "Online Connections" above, `docs/plans/trading212-import-plan.md`).
A second online provider is not started.
