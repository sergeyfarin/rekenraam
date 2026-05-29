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

## Review Method

Before each phase starts:

1. Read only the archive documents relevant to that phase.
2. Classify each discovered requirement as `adopt now`, `adapt later`, `defer`, `drop`, or `ask owner`.
3. Translate adopted ideas into current-stack terms.
4. Update `docs/product-requirements.md`, `docs/feature-roadmap.md`, `docs/conventions.md`, or an ADR.
5. Implement one backend and UI slice at a time, with the narrowest relevant validation.

No archive behavior becomes active scope just because it existed before.

## Phase Review Order

### Phase 0: Foundation

Review auth, migrations, SQLite pragmas, backup/restore, API shape, translation boundary, theme tokens, and test commands.

Questions to settle before implementation:

- What is the smallest acceptable MFA implementation for public VPS real-data deployment?
- What password-reset model fits a self-hosted single-owner app?
- Which first-class UI languages should follow English after the translation boundary exists?
- Which CGO SQLite driver and migration approach should be locked before the first real schema?

### Phase 1: Books, Commodities, And Accounts

Review account tree rules, commodity precision, opening-balance behavior, and single-book runtime guardrails.

Questions to settle before implementation:

- Should the initial setup create a default book automatically?
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

- Go `1.22` as declared by `backend/go.mod`.
- Go standard `net/http` routing until a concrete backend feature proves a framework is needed.
- SvelteKit with static adapter output served by the Go binary.
- `pnpm` `11.5.0` for JavaScript workspace tasks.
- Svelte checks for frontend validation and Go tests for backend validation.
- Playwright only for critical user journeys, run manually for now.
- `/api/v1` for real domain endpoints; `/api/hello` remains scaffold-only.
- Same-origin production shape: one Go binary serving API and static frontend.
- No GitHub Actions workflows for now.
- CGO SQLite drivers are acceptable.
- Public VPS deployment with real financial data requires MFA, even if that delays public VPS readiness.
- Browser-based first-run setup creates the single owner with a username and password.
- English-first UI and built-in data with translation boundaries ready for additional languages.
- SQLite database encryption is deferred for early local use, but documentation must explain when encrypted-at-rest storage may be needed.

Do not lock these yet:

- Authentication/password/session packages.
- Password-reset mechanism.
- Final UI component abstraction beyond the currently installed Svelte ecosystem.
- Initial non-English language set.
- Import formats beyond CSV for the first import milestone.
- Attachment storage approach.

Lock these before the first related implementation slice:

- Specific SQLite driver, migration runner, migration transaction behavior, and connection pragmas.
- Password hashing, session storage, cookie settings, setup completion semantics, and password-reset behavior.
- Translation catalog file format, built-in data label keys, and locale fallback behavior.
- Theme token names, persistence key, and light/dark token minimums.
- API error envelope and request identifier behavior.
- Money quantity type, scale limits, and commodity code validation.
