# Conventions

This document fixes cross-cutting product, engineering, and workflow conventions for the active Go and SvelteKit codebase.

## Source Of Truth

- Active requirements live in `docs/product-requirements.md`.
- Active architecture rules live in `docs/early-architecture-decisions.md`.
- Active sequencing lives in `docs/feature-roadmap.md`.
- Archive review planning lives in `docs/archive-requirements-review.md`.
- `.archive/` is historical reference only.

When a feature introduces a durable new rule, update one of those documents in the same change.

## Delivery Model

- Build one feature slice at a time.
- Each slice must leave the app runnable.
- Avoid hidden partial migrations from `.archive/`.
- Prefer the smallest viable durable implementation over speculative abstractions.
- Keep single-user assumptions explicit until a later ADR changes them.

## Product Naming And Domain Model

- A book is the top-level accounting boundary.
- Accounts form a tree.
- Account types are asset, liability, equity, income, and expense.
- Categories are UI concepts mapped to income and expense accounts, not a separate ledger primitive.
- Transactions contain postings or splits.
- Transfers are ordinary transactions.
- Reconciliation status belongs to posting or account-specific ledger state, not only a transaction header.

## Data And Persistence Conventions

- Never store money or quantities as floating point.
- Store exact values as integer plus scale plus commodity code.
- Use calendar dates for financial facts.
- Use UTC timestamps for system facts.
- Keep stable codes in data; translated labels belong in localization assets.
- Built-in records not entered by users or imported from external sources must use stable keys or codes, not English display names as the only source of truth.
- Seeded categories, account types, currencies, commodities, system accounts, and other app-defined labels must be localization-ready.
- Preserve `book_id` in core schema even while runtime stays single-book.
- Use state transitions, voiding, archival, or corrective entries instead of hard-deleting business records.
- Schema changes require explicit migrations under `backend/migrations`.

## API Conventions

- Real endpoints live under `/api/v1`.
- DTOs stay near HTTP handlers.
- Business rules belong in application services.
- Database access belongs behind repository-style functions or methods.
- Error responses should be structured, consistent, and safe for user display.
- Public request and response shapes should be documented in OpenAPI as endpoints become real.

## Frontend Conventions

- The frontend i18n library is **Paraglide JS** (Inlang). Use its compile-time message functions for all user-facing copy.
- Backend translation (export file headers, server-generated content) is deferred to Phase 3. When needed, use `nicksnyder/go-i18n` with JSON message files.
- All user-facing copy goes through a translation boundary.
- English is the initial implementation language.
- UI code and built-in app-defined data must stay ready for additional languages without route, component, or schema rewrites.
- Do not concatenate translated fragments to form sentences.
- Formatting of numbers, dates, percentages, and money must be locale-aware and separate from message translation.
- Use **Dinero.js v2** for all frontend money arithmetic and display (balance checks before submission, running totals, input parsing). Use `Intl.NumberFormat` via Dinero's formatting layer for locale-aware rendering.
- Backend money arithmetic uses **`shopspring/decimal`**. All canonical balance and report calculations happen in Go, not in the browser.
- Themes must use semantic design tokens for color, spacing, typography, elevation, and motion.
- Themes start with light and dark only.
- Theme names and token roles must stay stable even if visual styling evolves.
- New screens must define loading, empty, error, and success states.
- New interactions must be keyboard-usable and accessible.
- Responsive behavior must be deliberate on both desktop and mobile.
- Prefer shared frontend helpers and API seams over route-local ad hoc logic.

## Design Conventions

- Aim for calm, trustworthy financial software rather than generic dashboard styling.
- Use clear information hierarchy, explicit totals, and obvious destructive-action warnings.
- Preserve readability for dense ledger and reporting screens.
- Charts and color usage must remain understandable in all supported themes.
- Theme support should include a non-color cue strategy for critical financial states such as positive, negative, warning, reconciled, and locked.

## Security And Deployment Conventions

- Treat local-network deployment as safer than public deployment, but never as fully trusted.
- Local authentication must exist before real data entry.
- First-run setup is browser-based and creates the single owner with a username and password.
- Public deployment requires HTTPS and explicit operator guidance.
- Public VPS deployment with real financial data requires MFA.
- Local-network use may ship before MFA if authentication and operator guidance are clear.
- SQLite database encryption is deferred for early local use, but docs must explain when encrypted-at-rest storage may be needed.
- Docker Compose must package the same app shape as the single binary.
- SQLite data must live in persistent storage outside the container image or binary.
- Backup and restore instructions are part of product documentation, not only operator folklore.
- Operator backups do not replace user-facing export features.

## Scope Conventions

- User-facing export support must include core ledger CSV and QIF in the first export milestone.
- Attachments are out of scope until explicitly brought in by a later requirement or ADR.
- Mobile support must cover full core workflows responsively, including transaction entry.
- User and permission naming may stay explicitly single-user until shared workflows become active scope.

## Testing Conventions

- Backend behavior gets Go tests.
- Frontend logic gets Svelte checks and focused component or unit tests when introduced.
- Bruno covers important API workflows.
- Playwright covers critical user journeys.
- Financial invariants, reconciliation behavior, imports, and calculations require named backend test cases.

## Archive Translation Rules

- Do not port desktop-only features without re-justifying them for web and self-hosted deployment.
- Do not copy Python, Rust, or Tauri implementation details directly into Go or SvelteKit without confirming the underlying requirement still matters.
- When archive documents conflict with active docs, active docs win.
- If an archive idea is adopted, rewrite it in current-stack terms and add it to active docs.

## Documentation Conventions

- Requirements belong in `docs/product-requirements.md`.
- Long-lived architectural constraints belong in `docs/early-architecture-decisions.md`.
- Sequencing belongs in `docs/feature-roadmap.md`.
- Decision records belong in `docs/adrs/`.
- Repo-wide agent guidance belongs in `AGENTS.md`.
- Developer workflow notes belong in README files near the relevant tooling.
