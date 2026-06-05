# Archive Requirements Review

This plan keeps `.archive/` useful without letting old Rust, Tauri, or Python experiments drive a big-bang rebuild.

## Review Goal

- Preserve product intent that still matters for the Go, SvelteKit, SQLite app.
- Convert archive ideas into active requirements only one feature slice at a time.
- Keep each backend and UI step small enough for owner review before the next step starts.
- Record explicit owner questions instead of silently locking doubtful scope.

## Review Inputs

Use these archive documents as historical input:

- `.archive/docs/product/v1-scope.md` for broad product ambition.
- `.archive/docs/product/v1-gap-plan.md` for known risk areas and cleanup notes.
- `.archive/docs/architecture/accounting-foundations.md` for ledger and correction semantics.
- `.archive/docs/architecture/sqlite-schema.md` for SQLite runtime and schema lessons.
- `.archive/docs/architecture/frontend-testing.md` for testing tradeoffs.
- `.archive/docs/parity/desktop-to-python.md` and `.archive/docs/parity/tauri-rust-function-audit-2026-05-18.md` for consciously dropped desktop behavior.
- `.archive/docs/deployment/self-hosting.md` and `.archive/SELF_HOSTED_MIGRATION_PLAN.md` for deployment, backup, and restore lessons.
- `docs/archive-fastapi-backend-review.md` for the consolidated pass over `.archive/apps/api` and the FastAPI-era requirements documents.

## Review Method

Before each phase starts:

1. Read only the archive documents relevant to that phase.
2. Classify each discovered requirement as `adopt now`, `adapt later`, `defer`, `drop`, or `ask owner`.
3. Translate adopted ideas into current-stack terms.
4. Update `docs/product-requirements.md`, `docs/conventions.md`, or an ADR.
5. Implement one backend and UI slice at a time, with the narrowest relevant validation.

No archive behavior becomes active scope just because it existed before.

## Phase Review Order

### Phase 0: Foundation

Review auth, migrations, SQLite pragmas, backup/restore, API shape, translation boundary, theme tokens, and test commands.

Questions to settle before the related deployment or localization slice:

- What is the smallest acceptable MFA implementation for public VPS real-data deployment?
- Which first-class UI languages should follow English after the translation boundary exists?

Already settled since this review plan was first written:

- Use `modernc.org/sqlite`, not a CGO SQLite driver.
- Use `pressly/goose` with embedded SQL migrations.
- Install SQLite PRAGMAs per physical connection: `busy_timeout(5000)`, `foreign_keys(1)`, `journal_mode(WAL)`, `synchronous(NORMAL)`, and `wal_autocheckpoint(1000)`.
- Run pending migrations automatically at startup before serving HTTP; fail startup if migrations fail.
- Use goose's default per-file migration transaction behavior unless a migration documents why `NO TRANSACTION` is required.
- Prefer SQLite's online backup API for in-app live backups; `VACUUM INTO` is acceptable for compact operator-triggered backups.

### Phase 1: Books, Commodities, And Accounts

Review account tree rules, commodity precision, opening-balance behavior, and single-book runtime guardrails.

Questions to settle before implementation:

- What exact fields and choices belong in the default book, base currency, optional additional currency, and system-account setup step?
- Which currency metadata source is acceptable for the first slice?
- How much account-type customization is allowed before real reporting exists?

### Phase 2: Ledger Transactions

Review split transactions, balancing rules, category mapping, void/correct/delete semantics, and import-source metadata.

Questions to settle before implementation:

- Should ordinary unreconciled transactions support hard delete, soft delete, or void only?
- Should change reasons be required immediately or only after reconciliation exists?
- What is the minimum transaction entry UX on mobile?

### Phase 3: Reconciliation And Reporting

Review reconciliation locks, corrective entries, report basis, and export guarantees.

Questions to settle before implementation:

- What exact CSV files and columns define "core ledger CSV"?
- Should QIF export cover all accounts initially or register/account-level export first?
- Should report runs be reproducible snapshots or live recalculations in the first milestone?

### Later Phases

Review imports, cleanup helpers, budgets, schedules, pricing, investments, and advanced finance only when earlier phases are stable enough to support them.

## Package And Approach Locks

Lock these now:

- Go `1.26` as declared by `backend/go.mod`.
- Go standard `net/http` routing until a concrete backend feature proves a framework is needed.
- SvelteKit with static adapter output served by the Go binary.
- `pnpm` `11.5.2` for JavaScript workspace tasks.
- Svelte checks for frontend validation and Go tests for backend validation.
- Playwright only for critical user journeys, run manually for now.
- `/api/v1` for real domain endpoints; removed scaffold endpoints must not be reintroduced as product API.
- Same-origin production shape: one Go binary serving API and static frontend.
- Keep GitHub Actions CI aligned with the local validation wrapper scripts.
- Public VPS deployment with real financial data requires MFA, even if that delays public VPS readiness.
- Browser-based first-run setup is staged: owner first, then default book/currency/system-account setup, then category choices as those domains exist.
- Password hashing uses Argon2id with self-describing stored hashes and upgradeable parameters.
- English-first UI and built-in data with translation boundaries ready for additional languages.
- SQLite database encryption is deferred for early local use, but documentation must explain when encrypted-at-rest storage may be needed.
- `modernc.org/sqlite`, `pressly/goose`, repository-style `database/sql` access, and the official Debian 13 slim production runtime image as documented in `docs/conventions.md`.
- SQLite runtime PRAGMAs, migration behavior, busy timeout, and backup approach are locked in ADR 0004.
- Foundation coding gates for setup endpoints, setup progress, password recovery, OpenAPI workflow, error codes, request IDs, i18n scaffolding, and theme token scaffolding are locked in ADR 0006.

Do not lock these yet:

- Final UI component abstraction beyond the currently installed Svelte ecosystem.
- Initial non-English language set.
- Import formats beyond CSV for the first import milestone.
- Attachment storage approach.

Lock these before the first related implementation slice:

- Translation catalog file format, built-in data label keys, and locale fallback behavior.
- Theme token names, persistence key, and light/dark token minimums.
- Exact OpenAPI type generation/check command and frontend generated-client path.
- Money representation limits: maximum quantity scale and commodity code validation.
- Domain lifecycle status taxonomy for drafts, posted records, voided records, archived records, and corrective entries.

## Consolidated Carry-Forward From FastAPI Pass

The 2026-05-30 FastAPI archive pass found several decisions worth preserving as current-stack requirements when their feature slices arrive:

- **Setup and auth:** keep browser-based bootstrap status/create-owner endpoints, hashed cookie session tokens, revocable session rows, and request IDs. Device attribution is useful for audit quality, but full invite, membership, and role workflows stay deferred while scope is single-user.
- **SQLite operations:** keep the one-app deployment shape, persistent SQLite data directory, online-backup preference, restore-smoke checks, `PRAGMA integrity_check`, and explicit warnings for unsafe public deployments.
- **Ledger foundations:** keep `book_id`, account trees, system account roles, exact quantities, transaction postings, import-source metadata, and no hard delete for posted financial records. Translate archived category rows into the current convention: categories are UI concepts mapped to income/expense accounts.
- **Auditability:** preserve actor/session/request attribution and plan for change reasons before reconciliation and correction workflows become serious. In Go, prefer SQLite constraints/triggers for invariants that must survive service-layer bypass.
- **Reconciliation:** use account balancing/checkpoint rows as the reconciliation lock floor. Unlocking a reconciled range should require an explicit reason and confirmation.
- **Imports:** keep the preview -> rules -> commit workflow, import sessions, source file metadata/hash, duplicate keys, and an `imbalance_import` system-account path for incomplete imported rows.
- **Exports and reports:** keep account/register-level QIF, core ledger CSV, report CSV output, saved report definitions as a later convenience, and report-run input capture as an open reproducibility decision.
- **Ergonomics:** saved transaction views, transaction templates, and payee defaults are good later slices because they speed daily entry without changing ledger semantics.

The same pass also identified archive decisions not to port directly:

- Full multi-user role, invite, and book membership workflows.
- Person/project cost-sharing as first-class early scope.
- Broad import support beyond the first approved import formats.
- Generic SQL report definitions exposed to users.
- Plugin/theme runtimes or reserved placeholder endpoints.
- Desktop path selection, desktop undo/redo tables, native file-picker assumptions, and attachment/document workflows.
- Hard-delete APIs for transactions, pricing observations, planning records, or financial/reference records whose history matters.
