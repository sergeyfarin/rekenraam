# AGENTS.md

This file gives repo-wide guidance to coding agents and automation working in this repository.

## Project Shape

- Active stack: Go backend, SvelteKit frontend, SQLite database.
- Production shape: one Go binary serving the statically built frontend.
- Docker Compose must preserve the same app shape.
- `.archive/` contains previous Rust, Tauri, and Python experiments and is not source of truth.

## Source Of Truth Documents

Read these before making durable product or architecture changes:

- `docs/product-requirements.md`
- `docs/conventions.md`
- `docs/early-architecture-decisions.md`
- `docs/feature-roadmap.md`
- `docs/archive-requirements-review.md`
- `docs/adrs/`
- `docs/developer-workflow.md`
- `.github/copilot-instructions.md`

If a change introduces a durable new rule, update the relevant document in the same change.

## Working Rules

- Deliver incrementally, one feature slice at a time.
- Do not attempt broad archive-to-current-stack conversion in one change.
- Prefer minimal, durable implementations over speculative abstractions.
- Keep the app runnable after each slice.
- Use `pnpm` for workspace JavaScript tasks.
- Respect accepted ADRs unless the change explicitly updates or supersedes them.

## Financial Invariants

- Preserve double-entry-capable ledger design.
- Never store monetary values as floating point.
- Keep `book_id` in core financial tables even while runtime remains single-book.
- Do not hard-delete posted financial records.
- Prefer void, archive, or corrective-entry workflows for durable financial changes.
- Reconciliation-related edits must preserve auditability and trust.

## Backend Rules

- Real API endpoints belong under `/api/v1`.
- Business rules belong in application services, not directly in HTTP handlers.
- Schema changes require migrations under `backend/migrations`.
- SQLite remains the primary database unless an active architecture document is updated.

## Frontend Rules

- Route all user-facing copy through a translation boundary.
- Keep built-in database labels localization-ready with stable keys/codes, not English-only source text.
- Keep formatting of money, dates, and numbers locale-aware and separate from translation strings.
- Use semantic design tokens for theming; avoid one-off hard-coded visual values in new UI.
- New screens should define loading, empty, error, and success states.
- Accessibility and mobile usability are required, not optional polish.
- Core workflows must remain usable on mobile, including transaction entry.

## Archive Handling

- Treat `.archive/` documents as research input only.
- Do not port desktop-only assumptions such as native file pickers, local path selection UX, or Tauri-specific runtime features unless explicitly re-approved.
- When reusing an archive idea, rewrite it in current-stack terms and place it in active docs.

## Current Scope Locks

- Keep the product explicitly single-user until a later decision changes that scope.
- First export milestone must include core ledger CSV and QIF.
- Attachments are currently out of scope.

## Validation Expectations

- Run the narrowest relevant validation after edits.
- Prefer feature-scoped tests first, then broader checks.
- If docs or conventions change, keep references and commands consistent across the repo.
- Keep README guidance and developer commands aligned.
