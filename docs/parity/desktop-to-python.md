# Desktop To Python Parity Matrix

Last updated: 2026-05-18

The active runtime is the SvelteKit/FastAPI web app with SQLite. The legacy
Tauri/Rust tree is no longer used by builds or runtime code.

## Ported Capability Groups

| Capability | Python/API status |
| --- | --- |
| Storage and migrations | Alembic baseline on SQLite, admin runtime status, health checks, WAL runtime pragmas |
| Books | Seeded book read endpoints; multi-book schema retained, create-books still gated |
| Auth | Bootstrap admin, login/logout, sessions, password reset, MFA, invites, memberships |
| Accounts | List/detail/tree/balances, create/update/delete, close validation, directives, booking policy |
| Transactions | Register, split editing, duplicate/bulk actions, locked-range validation, payee defaults |
| Reconciliation | Start/finish/history/unlock, constraints, preferences, SQLite lock-row serialization |
| Imports | CSV, XLS, XLSX, QIF, OFX/QFX, HBCI/MT940 preview/commit/matching/rules |
| Exports | Accounts CSV, transactions CSV, register QIF, report CSV |
| Reports | Cashflow, category spend, payee totals, net worth, trends, gains, performance, saved report runs |
| Metadata | Institutions, categories, payees, tags, people, projects, commodities, currencies |
| Pricing | Sources, policies, source assignments, manual FX/market prices, refresh state and history |
| Investments | Instruments, lots, buy/sell/short/dividend flows, actions, positions, performance |
| Planning | Budgets, schedules, projected cash, loans, amortization |
| Ergonomics | Notes, search, saved views, templates, preferences |
| Admin | Runtime status, integrity checks, fiscal-year close, users, invites, audit events |
| Backup/restore | SQLite online backup command and restore-smoke validation |

## Dropped Or Deferred Desktop Behaviors

- Native file/folder pickers and desktop database location management.
- Desktop undo/redo session tables.
- Desktop event/document attachment CRUD.
- Path-based import commands; the web app uses uploaded content.
- Legacy database import tooling.
- Generic SQL report execution.
- Richer price-source override/history fields not currently needed by V1.
- Server-side undo/redo, OCR, attachment storage, open banking, and plugin/theme
  runtime remain deferred.

## Confirmed Missing Or Changed Behavior

These points were carried in more detail before the last cleanup and should stay
visible until `src-tauri/` is deleted.

### Transactions

- The Rust desktop runtime exposed timestamp and timezone fields more directly;
  the web transaction surface is still more date-centric.
- Rust hard-delete bulk transaction behavior was replaced by safer void-style
  semantics in the web runtime.
- Rust auto-created balancing accounts for generic mixed-commodity transfers;
  the web runtime intentionally routes these through explicit cross-currency
  transfer flows instead.

### Accounts And Metadata

- Country CRUD is not fully ported.
- Some reference-data edits are simpler than the old append-only desktop model.
- Account open/close/reopen semantics exist, but not as the same first-class
  command shapes the desktop app had.
- Notes are ported, but not with the same versioned/tombstoned history model.
- User-facing event CRUD and document/attachment CRUD are not direct Python
  ports.

### Imports And Reports

- The web import flow is upload-based rather than local-path based.
- Reverse lookup from transaction to import session is not an exact one-for-one
  port.
- Generic SQL/template report execution and report-run pruning were desktop
  features that remain consciously deferred.

### Pricing, FX, And Investments

- Rich pricing history, source-override, and derivation audit fields were
  simplified in the web runtime.
- Dividend-income-category defaults are not exposed as the same CRUD surface.
- The main investment workflows are ported, but some helper-specific automated
  accounting remains thinner than the Rust surface.

## Delete Decision

The Rust tree is safe to delete from a build/runtime point of view after product
sign-off accepts the dropped/deferred behaviors above. The detailed function
audit is in
[tauri-rust-function-audit-2026-05-18.md](./tauri-rust-function-audit-2026-05-18.md).

## Extension Guardrails

Plugin/theme runtime deferred: no `/api/v1/plugins/*` or
`/api/v1/themes/*` endpoints in b1. Any future extension system needs semantic
CSS tokens, WebAssembly or sidecar isolation, manifest-declared capabilities,
admin review, disabled/failed-plugin isolation, deterministic fallback, and no arbitrary remote CSS loading.
