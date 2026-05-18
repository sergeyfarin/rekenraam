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

## Delete Decision

The Rust tree is safe to delete from a build/runtime point of view after product
sign-off accepts the dropped/deferred behaviors above. The detailed function
audit is in
[tauri-rust-function-audit-2026-05-18.md](./tauri-rust-function-audit-2026-05-18.md).

## Extension Guardrails

Plugin/theme runtime deferred: no `/api/v1/plugins/*` or
`/api/v1/themes/*` endpoints in b1. Any future extension system needs semantic
CSS tokens, WebAssembly or sidecar isolation, manifest-declared capabilities,
admin review, disabled/failed-plugin isolation, deterministic fallback, and no
arbitrary remote CSS loading.
