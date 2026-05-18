# Tauri Rust Function Audit, 2026-05-18

This is the second-pass deletion audit for `src-tauri/`. It reviews the Rust
backend module by module, including private helper functions and test-only
coverage, not only registered Tauri commands.

Conclusion: the active web runtime no longer depends on Tauri or Rust. The
Rust tree can be deleted from a build/runtime point of view, but deletion
should be product-approved because several Rust behaviors were intentionally
dropped, deferred, or replaced with different web semantics.

## Audit Method

- Counted and reviewed the registered desktop command surface in
  `src-tauri/src/lib.rs`: 209 commands total.
- Walked every Rust source file under `src-tauri/src/`, including private
  helpers, row mappers, parsers, schedulers, and embedded tests.
- Cross-checked equivalent Python routes, schemas, services, repositories, and
  models under `apps/api/src/rekenraam_api`.
- Re-searched the active Svelte/API runtime for Tauri bindings. There are no
  active `@tauri`, `invoke`, `__TAURI__`, or Tauri API imports outside
  `src-tauri`, docs/TODO, `.vscode`, `.dockerignore`, and the unreferenced
  starter asset `static/tauri.svg`.

## File-Level Result

| Rust file | Review result | Delete disposition |
|---|---|---|
| `main.rs` | Calls `rekenraam_lib::run()` only. | Safe after Tauri removal. |
| `commands.rs` | Re-export surface only. No behavior. | Safe. |
| `state.rs` | Desktop `DbState`, backup scheduler, FX scheduler holders. FastAPI dependencies, auth sessions, workers, and request context replace this. | Safe. |
| `error.rs` | Tauri-facing serializable error wrapper. FastAPI/HTTPException/Pydantic replace it. | Safe. |
| `pagination.rs` | Generic `PaginatedResult<T>`. Python uses endpoint-specific page/cursor response models. | Safe. |
| `validation.rs` | Validators for account type, transaction status, names, memos, amounts, and LIKE escaping. LIKE escaping is already ported. Some length/value guards are still primarily DB/frontend contracts. | Safe, with guard gaps already tracked. |
| `lib.rs` | Desktop shell: window state, launch file handling, storage-location bootstrap, logging, command registration, scheduler startup. No product ledger logic. | Safe, desktop-only. |
| `db.rs` | SQLite path handling, migrations, audit user function, read connections, runtime undo/redo/session tables. Python Alembic/SQLAlchemy and auth request context replace storage/migration/audit concerns. Undo/redo tables are intentionally not direct-ported. | Safe if undo/redo direct port remains dropped. |
| `session.rs` | Desktop session ID plus undo/redo stack helpers. | Safe if server-side undo/redo is deferred. |
| `db_storage.rs` | Desktop SQLite file location, file pickers, backup settings, scheduler, `VACUUM INTO`, restore/move/copy storage, DB health/stats/vacuum. Web replacement is container SQLite operational docs, health/admin endpoints, and backup/restore smoke. | Safe, desktop-only except no web `VACUUM` analogue needed. |
| `db_transactions.rs` | Transaction/split/register logic. Core create/update/delete/list/register/payee defaults/duplicate/bulk flows are ported. Differences are listed below. | Safe after accepting listed semantic differences. |
| `db_accounts.rs` | Accounts, metadata, reconciliation primitives, fiscal close, notes, events, documents. Most account/metadata/reconciliation logic is ported, but countries, events, documents, and some lifecycle/audit semantics differ. | Safe after accepting listed gaps. |
| `import.rs` | CSV/XLS/XLSX/QIF/OFX/HBCI/MT940 imports, sessions, rules, matching, locale parsing, path helpers. Upload-based import workflow is ported. Local path helpers and some reverse-lookup helpers are not. | Safe after accepting listed gaps. |
| `db_reports.rs` | Saved definitions, builtin reports, cache metadata, generic SQL/template `run_report`, pruning. Builtin reports and run metadata are ported; generic executor and pruning are not. | Safe after accepting report executor/prune decision. |
| `db_currencies.rs` | Legacy currency/FX commands and rich append-only pricing history. Python pricing/currency APIs replace the main flows but not all version/history fields. | Safe after accepting pricing history simplification. |
| `fx_rates/mod.rs` | Provider implementations and cross-rate helper. Provider roster is ported in Python; simple via-currency derivation exists. Rich derivation audit fields are not. | Safe after accepting pricing audit simplification. |
| `fx_refresh.rs` | Desktop scheduler, task prep, source resolution, refresh-state recording. Python pricing worker/service replaces this. | Safe. |
| `db_commodities.rs` | Commodities, investments, lots, corporate actions, prices, source overrides, dividend category defaults. Investments and core price observations are ported; source overrides and dividend income category mappings are not. | Safe after accepting listed gaps. |

## Confirmed Missing Or Changed Behavior

These are the Rust behaviors found by the deeper function/helper review that
are not direct one-for-one Python ports.

### Desktop-Only By Design

- Window bounds/state, file association launch handling, native file/folder
  pickers, desktop storage location selection, desktop SQLite backup scheduler,
  move/copy/restore database file commands, and local app logging paths.
- SQLite-specific migration bootstrap and `audit_user()` UDF.
- Desktop runtime session tables and direct undo/redo stacks:
  `app_runtime_session`, `session_undo_stack`, `session_redo_stack`,
  `session_reverts`.
- Path-based import commands for reading local files. The web API accepts
  uploaded text/base64 content instead.

### Transactions

- Rust exposed `occurred_at_utc`, `occurred_tz`, `posted_at_utc`, and
  `posted_tz`, normalized UTC timestamps, and required timezone co-presence.
  Python database columns exist, but the public transaction schemas currently
  expose only date-level transaction fields. This is a real parity gap if
  time-of-day transaction ordering matters.
- Rust had `list_postings`, a date-filtered postings endpoint shape. Python
  exposes transactions, register, and reports, but no exact postings endpoint.
- Rust `bulk_delete_transactions` physically removed eligible transactions and
  splits. Python `bulk_delete_transactions` currently delegates to bulk void,
  preserving history. This is safer but semantically different.
- Rust auto-created "Currency Trading" balancing accounts for mixed-commodity
  transactions. Python intentionally rejects generic mixed-commodity balancing
  and uses `POST /api/v1/transactions/transfer` for explicit FX transfers.

### Accounts And Metadata

- Country CRUD is not fully ported. Python exposes `GET /countries`, while Rust
  had list/create/get/update/delete.
- Institutions, categories, payees, tags, people, projects, and commodities are
  largely ported, but many Python reference-data edits are mutable or
  hard-delete instead of the append-only version chains preserved by SQLite.
- Account open/close/reopen exists through account update, directives, booking
  policy, and closing validation, but Rust had first-class
  `create_account_open`, `create_account_close`, and `create_account_reopen`
  command shapes.
- Notes exist in Python, but the legacy Rust note commands were versioned and
  tombstoned. Python notes are the web markdown-note workflow and do not fully
  mirror that history model.
- User-facing event CRUD and document/attachment metadata CRUD are not direct
  Python ports. Admin audit events replace system history; user documents,
  attachments, OCR, and a separate event model remain deferred.

### Imports

- Rust had basic HBCI/MT940 parsing via `parse_hbci_mt940`, `parse_mt940_date`,
  and auto-detection of `.hbci`/`.mt940`. Python now ports and improves this
  parser with first-class `hbci`/`mt940` formats, field-continuation handling,
  structured `:86:` extraction, debit/credit signs, locale-aware amounts, and
  stable import IDs.
- Rust provided reverse lookup `list_transaction_import_sessions(tx_id)`.
  Python exposes sessions and session-to-transactions, but not an exact
  transaction-to-import-sessions route.
- Rust allowed a separate `start_import_session` then `commit_import_session`
  lifecycle. Python creates/commits sessions through the upload/commit workflow.
  The main user flow is covered, but the command shape is not identical.
- Rust amount parsing truncated extra fractional places; Python has a different
  decimal policy. The gap plan already tracks a future per-account/institution
  rounding policy.

### Reports

- Rust `run_report` executed custom SQL/template reports with JSON schema
  parameter validation, SELECT-only checks, row limits, params hashing, cached
  result rows, and book-state invalidation. Python has hardcoded report service
  methods and report run metadata, but no generic SQL/template executor.
- Rust `prune_report_runs(book_id, retain_per_definition)` bounded cached runs.
  Python currently has no equivalent pruning.
- Rust had `list_builtin_reports`. Python exposes concrete report endpoints and
  frontend tabs, not an exact builtin discovery endpoint.
- Rust `report_account_balances` is covered by account balances and account
  valuation/report endpoints, but not as the exact same report command.

### Pricing, FX, And Commodities

- Rust `commodity_price_sources` supported per-source ticker overrides,
  source-specific display names, primary flags, and append-only supersession.
  Python has general pricing source assignments, but no equivalent table/API
  for commodity/source ticker overrides such as exchange-specific symbols.
- Rust `dividend_income_categories` mapped commodity/category defaults with
  tax-withheld metadata and notes. Python dividend endpoints require explicit
  income accounts and do not expose this default mapping CRUD.
- Rust exposed `get_commodity` as a command. Python has commodity list,
  autocomplete, and update routes, plus repository-level lookups, but no public
  `GET /commodities/{id}` endpoint.
- Rust commodity price commands included implicit price extraction from a
  transaction and append-only price observation supersession. Python supports
  manual market prices and FX observations, but the richer supersession/audit
  and implicit-price command behavior is not direct-ported.
- Rust pricing policy and FX observations carried richer fields:
  `staleness_max_days`, `triangulation_max_hops`, `rounding_mode`,
  `prefer_official_fx`, `mode`, `is_derived`, `derived_via_commodity_id`,
  `triangulation_path_json`, `supersedes_observation_id`, `ingest_run_id`,
  `period_type`, `period_year`, and `period_month`. Python has a smaller
  pricing model and persisted refresh runs.
- Rust tracked base-currency changes in `book_base_currency_history`. Python
  mutates `Book.base_currency_code`; historical base-currency timeline is not
  ported.
- Rust and Python both support the main FX provider roster: ECB/Frankfurter,
  Federal Reserve/FRED, Bank of Canada, ExchangeRate.host, and Yahoo Finance.
  Python also has via-currency derivation for provider-specific bases, but it
  does not persist the same rich derivation audit trail.

### Reconciliation And Investments

- Balance checks, adjustments, constraints, reconciliation history/unlock, and
  account balances are ported, but Python reshapes them under
  `/api/v1/reconciliation`.
- Rust allowed multiple balance constraints per account. Python intentionally
  enforces a singleton active constraint per account.
- Buy, sell, dividends, reinvested dividends, lots, holding periods, realized
  gains, unrealized gains, positions, converted positions, account valuation,
  currency exposure, and split/reverse-split corporate actions are broadly
  ported.
- Complex corporate action/private/derivative scenarios are represented in
  Python, but automated accounting remains thinner than the Rust investment
  surface in some helper-specific cases.

## Safe-To-Delete Checklist

Before physically deleting `src-tauri/`, product sign-off should explicitly
accept the following:

- Desktop file/storage/backup/window behavior is replaced by web deployment
  operations and does not need a Rust carry-forward.
- Direct undo/redo is dropped until a server-safe design exists.
- Country CRUD, event CRUD, document/attachment CRUD, commodity price source
  overrides, dividend income category defaults, generic custom report
  execution, and report-run pruning are either deferred or reimplemented.
- Transaction timestamp/timezone fields and hard-delete-vs-void semantics are
  accepted as current web behavior or tracked as product gaps.
- Rich pricing history fields and append-only reference-data chains are either
  restored or consciously simplified.
- Non-runtime cleanup happens with the deletion: remove
  `.vscode/extensions.json`'s Tauri recommendation, `.dockerignore`'s
  `src-tauri/target` line, and `static/tauri.svg`.

## Deletion Decision

The Rust code is not needed to run, build, test, or ship the current web API and
frontend. It is safe to delete technically. It is not a "no product loss"
deletion unless the above deferred and changed behaviors are accepted.
