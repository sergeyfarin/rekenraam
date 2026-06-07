# Conventions

This document fixes cross-cutting product, engineering, and workflow conventions for the active Go and SvelteKit codebase.

## Source Of Truth

- Active requirements live in `docs/product-requirements.md`.
- Active architecture rules live in `docs/early-architecture-decisions.md`.
- Active sequencing lives in `docs/product-requirements.md`.
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
- Account classes are asset, liability, equity, income, and expense.
- Account kinds are catalog-backed behavior/UI profiles constrained by class;
  they are not categories, tax wrappers, budget-visibility wrappers, system
  roles, or investment lot concepts.
- Account budget treatment is a separate planning/reporting axis from
  `account_kind`, because the same kind of account can be on-budget,
  off-budget, or excluded from budget views.
- System accounts are identified by `system_role` and hidden from ordinary
  account lists by default.
- Hidden income and expense fallback accounts use `account_class=income` and
  `account_class=expense` respectively; they are not equity summary accounts.
- Categories are UI concepts mapped to income and expense accounts, not a separate ledger primitive.
- Category API behavior, built-in metadata, lifecycle rules, and the starter
  taxonomy are documented in `docs/categories-design.md`.
- Transaction ledger schema planning is documented in
  `docs/transaction-ledger-core-plan.md`.
- Transactions contain postings. "Split" is acceptable user-facing copy for a
  split transaction UI, but schema, service, API, and docs should use
  "posting" as the canonical ledger term.
- Transfers are ordinary transactions.
- Reconciliation status belongs to posting or account-specific ledger state, not only a transaction header.

## Data And Persistence Conventions

- Never store money or quantities as floating point.
- Store exact values as integer plus scale plus commodity code.
- Ledger posting quantities use debit-positive sign convention. Positive
  postings are debits; negative postings are credits. Asset and expense
  increases are usually positive. Liability, equity, and income increases are
  usually negative.
- Use calendar dates for financial facts.
- Use UTC timestamps for system facts.
- Calendar dates travel over the wire as ISO 8601 strings (`YYYY-MM-DD`). UTC timestamps travel as RFC 3339 strings (`2025-01-15T14:30:00Z`). Go's `time.Time` marshals as RFC 3339 by default; use a plain `string` or a validated date type for calendar-date fields.
- Keep stable codes in data; translated labels belong in localization assets.
- Built-in records not entered by users or imported from external sources must use stable keys or codes, not English display names as the only source of truth.
- Built-in labels are resolved at render time by the frontend localization boundary. Do not store localized built-in names as canonical database values or require them in setup/API requests.
- Seeded categories, account classes, account kinds, currencies, commodities, system accounts, and other app-defined labels must be localization-ready.
- Preserve `book_id` in core schema even while runtime stays single-book.
- Use state transitions, voiding, archival, or corrective entries instead of hard-deleting business records.
- Physical delete is allowed only for never-posted drafts. Posted financial records must use void, archive, or corrective-entry workflows even when unreconciled.
- Keep transaction lifecycle separate from reconciliation state. `draft`, `posted`, and `voided` determine whether a transaction affects the ledger; `uncleared`, `cleared`, and `reconciled` describe account-posting verification.
- Reconciliation is account- and commodity-scoped. Finished reconciliation checkpoints are lock floors for reporting and register trust; edits that change a reconciled posting's account, commodity, amount, or entry date require explicit override and must invalidate affected checkpoints rather than silently preserving the old lock.
- Use the hybrid audit model for durable domain changes: domain version/lifecycle tables explain what changed, while `audit_events` rows explain who, when, how, and why the operation was initiated.
- One `audit_events` row should represent one user/system operation, not one changed column. Multi-row workflows such as setup, imports, reconciliation, and transaction correction should create one event inside the same database transaction and reference it from every created version/lifecycle row.
- Keep essential attribution columns such as `created_at`, `recorded_at`, `created_by_user_id`, `changed_by_user_id`, `created_request_id`, and `change_reason` on domain rows as denormalized snapshots for straightforward history and export queries. The referenced audit event is the canonical home for request, session, origin, operation, and grouped-workflow provenance.
- `audit_events.auth_session_id` is nullable and must not block session cleanup; request IDs and actor IDs remain the stable attribution floor.
- `audit_events.operation` must be non-empty and follow stable dotted-code naming such as `account.create` or `transaction.correct`, but the database must not use a full enum that requires a migration for every new operation.
- Soft-delete column convention and the specific schema pattern (e.g., `deleted_at` vs `archived_at` vs a status enum) will be documented when first introduced in a migration. Consistency across tables is required; do not mix approaches.
- Schema changes require explicit migrations under `backend/migrations`.
- The migration runner is **`pressly/goose`** with embedded SQL files. Migrations are sequential numbered `.sql` files under `backend/migrations/`. Run at startup; goose tracks applied versions in the database.
- The SQLite Go driver is **`modernc.org/sqlite`** (pure Go, no CGO). Do not use `mattn/go-sqlite3`.
- SQLite connections must install PRAGMAs on every physical connection through the modernc DSN: `busy_timeout(5000)`, `foreign_keys(1)`, `journal_mode(WAL)`, `synchronous(NORMAL)`, and `wal_autocheckpoint(1000)`.
- Startup must validate effective SQLite PRAGMA state before serving requests.
- Early runtime uses one application `*sql.DB` pool with `SetMaxOpenConns(1)`; revisit only when real workload pressure justifies more concurrency.
- Database access uses repository-style methods over raw `database/sql`. Introduce **`sqlc`** only when the query surface is large enough to justify code generation, and document the generation command before generated files land.

## API Conventions

- Real endpoints live under `/api/v1`.
- DTOs stay near HTTP handlers.
- Business rules belong in application services.
- Database access belongs behind repository-style functions or methods.
- Error responses should be structured, consistent, and safe for user display.
- Error response shape is a JSON envelope: `{"error": {"code": "STABLE_CODE", "message": "human-readable detail"}}`. The `code` field is a stable uppercase string that the frontend translation layer keys off of. Never return raw Go error strings to the client.
- Initial API error codes are `VALIDATION_FAILED`, `UNAUTHENTICATED`, `FORBIDDEN`, `NOT_FOUND`, `CONFLICT`, `CSRF_INVALID`, `RATE_LIMITED`, `RESOURCE_BUSY`, `SETUP_REQUIRED`, `SETUP_ALREADY_COMPLETE`, and `INTERNAL_ERROR`.
- Public request and response shapes should be documented in OpenAPI as endpoints become real.
- Stable `/api/v1` endpoints are OpenAPI-first. `api/openapi/openapi.yaml` is the checked source of truth, handler changes and OpenAPI changes land together, and frontend API types must be generated from the checked OpenAPI artifact once frontend API client code lands.
- Date fields in request and response bodies use ISO 8601 (`YYYY-MM-DD`) for calendar dates and RFC 3339 for timestamps, consistent with the data layer convention.
- List endpoints that can return large result sets must use **cursor-based pagination**, not offset pagination. Cursors must be stable under concurrent inserts. Return `next_cursor` in the response; a missing or null `next_cursor` means the last page.
- Transaction list endpoints must support a `search` query parameter backed by **SQLite FTS5** on the backend. Do not implement client-side full-text search over server-fetched pages.
- API code must treat `SQLITE_BUSY` as possible even with `busy_timeout`. If the timeout is exhausted, return a retryable server error instead of hiding or partially applying the operation.

## Authentication And Session Security

- Username validation requires: non-empty after trimming, at most 64 bytes, no control characters (Unicode < U+0020 or U+007F). These constraints apply at both owner creation and login. OpenAPI schemas for `CreateOwnerRequest` and `LoginRequest` must document the same limits.
- Any feature that changes the owner password must revoke all existing sessions atomically in the same database transaction. The `recover-owner` command demonstrates the required pattern. Do not implement a "change password" endpoint that updates the hash without revoking sessions.
- Login throttle scopes use `username` and `client_ip` as independent scope keys. The `username` throttle is cleared on successful login; the `client_ip` throttle is intentionally retained. When multi-user support is added, confirm that throttle scope keys remain globally unique and cannot be confused across user classes.
- `429 Too Many Requests` responses from the login endpoint must include a `Retry-After` header with the remaining wait in seconds. The blocked-until time is known at the point the error is returned; derive `Retry-After` from it rather than from a fixed constant.
- Expired and revoked `auth_sessions` rows accumulate over time. A periodic cleanup should delete rows where `revoked_at IS NOT NULL OR expires_at <= now`. The index `auth_sessions_expires_revoked_idx` exists to support this. Document and implement the cleanup strategy before the app is recommended for long-running deployments.
- The `script-src 'unsafe-inline'` directive in the Content-Security-Policy header exists because SvelteKit `adapter-static` injects an inline bootstrapping script into `index.html` at build time. The correct fix is to compute a SHA-256 hash of that script during the frontend build and embed it in the Go middleware's CSP. This is deferred; when implemented, the build script must output the hash and the Go source must consume it at build time. Do not remove `'unsafe-inline'` until that mechanism is in place.
- `Strict-Transport-Security` (HSTS) is emitted only when the request is served over HTTPS (direct TLS or trusted proxy signalling `X-Forwarded-Proto: https`). Do not emit HSTS on plain HTTP responses.

## Frontend Conventions

- SvelteKit builds static output with `@sveltejs/adapter-static`; production does not run a SvelteKit server.
- Do not add `+page.server.ts`, `+layout.server.ts`, `+server.ts`, SvelteKit form actions, or production server hooks without first accepting an ADR that changes the runtime shape.
- Frontend routes may rely on the Go static handler's SPA fallback for extensionless browser paths. Missing asset-like paths with file extensions should 404.
- All production data access goes through Go API endpoints under `/api/v1`; unknown `/api/` paths must not fall back to the frontend shell.
- The frontend foundation stack uses **SvelteKit**, **Tailwind CSS**, **Bits UI**, and **shadcn-svelte**. Bits UI provides accessible primitives; shadcn-svelte is a component scaffolding source, not the product design system.
- The frontend i18n library is **Paraglide JS** (Inlang). Use its compile-time message functions for all user-facing copy.
- Add the Paraglide/Inlang message catalog structure before the first non-placeholder user-facing screen.
- Backend translation (export file headers, server-generated content) is deferred to Phase 3. When needed, use `nicksnyder/go-i18n` with JSON message files.
- All user-facing copy goes through a translation boundary.
- English is the initial implementation language.
- UI code and built-in app-defined data must stay ready for additional languages without route, component, or schema rewrites.
- Do not concatenate translated fragments to form sentences.
- Formatting of numbers, dates, percentages, and money must be locale-aware and separate from message translation.
- Use **Dinero.js v2** for all frontend money arithmetic and display (balance checks before submission, running totals, input parsing). Use `Intl.NumberFormat` via Dinero's formatting layer for locale-aware rendering.
- Backend money arithmetic uses **`shopspring/decimal`**. All canonical balance and report calculations happen in Go, not in the browser.
- Use Tailwind for implementation ergonomics, layout composition, and utility authoring, but keep semantic design tokens in CSS custom properties as the source of truth for theme roles. Tailwind utilities do not replace the token system or the app's product-specific visual language.
- Build app screens from shared shell and surface primitives (`PageHeader`, `Panel`, `StatePanel`, status badges, toolbar/list-row patterns) before adding route-local visual styling. Route files may compose layout, but token roles and reusable financial-app chrome should stay centralized.
- Themes must use semantic design tokens for color, spacing, typography, elevation, and motion.
- Themes start with light and dark only.
- Theme names and token roles must stay stable even if visual styling evolves.
- Define semantic theme tokens as CSS custom properties before the first non-placeholder user-facing screen. Persist theme preference only after a real user-facing theme control exists; until then, respect system preference where practical.
- New screens must define loading, empty, error, and success states.
- New interactions must be keyboard-usable and accessible.
- Responsive behavior must be deliberate on both desktop and mobile.
- Prefer shared frontend helpers and API seams over route-local ad hoc logic.
- Use **`openapi-typescript`** (type generation) + **`openapi-fetch`** (typed HTTP client) for all frontend API calls. The OpenAPI spec is the single source of type truth.
- Use **`@tanstack/svelte-query`** as the data layer for all server state. Use `createInfiniteQuery` for paginated lists; use query key composition to cache search results per search string.
- Add **`@tanstack/svelte-table`** and TanStack Virtual only when the first dense table or virtualized list screen actually needs them; do not lock them into the baseline dependency set before first use.
- Use **`minisearch`** for client-side fuzzy filtering of small in-memory sets (account name dropdowns, payee autocomplete). Do not use client-side search for full transaction lists.
- Use **`date-fns` v4** for frontend date manipulation (parsing, arithmetic, formatting helpers). Use `Intl.DateTimeFormat` for final locale-aware display output. Do not use `luxon`, `moment`, or the browser `Date` constructor for financial date logic.
- All new frontend files must be **TypeScript** (`.ts`, `.svelte` with `<script lang="ts">`). No JavaScript-only files in `frontend/src`.
- Component state uses **Svelte 5 runes** (`$state`, `$derived`, `$effect`, `$props`). Cross-component and cross-route shared state uses `$state` in `.svelte.ts` module files. No Svelte 4 stores (`writable`, `readable`, `derived` from `svelte/store`) in new code.

## Design Conventions

- Aim for calm, trustworthy financial software rather than generic dashboard styling.
- Use clear information hierarchy, explicit totals, and obvious destructive-action warnings.
- Preserve readability for dense ledger and reporting screens.
- Charts and color usage must remain understandable in all supported themes.
- Theme support should include a non-color cue strategy for critical financial states such as positive, negative, warning, reconciled, and locked.

## Security And Deployment Conventions

- Treat local-network deployment as safer than public deployment, but never as fully trusted.
- Local authentication must exist before real data entry.
- First-run setup is browser-based and guided. The first implementation creates the single owner with a username and password; later setup steps add the default book, default currency preference, optional additional currencies, system accounts, default categories, and optional additional categories as those domains are implemented.
- First-run setup uses `GET /api/v1/setup/status` and `POST /api/v1/setup/owner` for the first slice. Setup progress is persisted as named steps, not as a single boolean.
- Early password recovery has no unauthenticated browser or email reset flow. Use an operator-controlled local recovery path requiring server or database access, and invalidate existing sessions after password reset.
- Session management uses **HTTP-only secure cookies** backed by a **SQLite session table**. Sessions are revocable by deleting the row. Do not use JWTs for session tokens.
- Session tokens are opaque random values. Store only token hashes in SQLite.
- Session cookies use `HttpOnly`, `SameSite=Strict`, `Path=/`, and no `Domain`. Use `Secure` whenever the app is served over HTTPS. HTTPS deployments should use a `__Host-` prefixed cookie name once auth implementation lands.
- CSRF protection for mutating API requests uses SameSite cookies as a baseline plus server-side `Origin` validation and a session-bound or signed CSRF token sent in a custom header such as `X-CSRF-Token`.
- `GET`, `HEAD`, and `OPTIONS` endpoints must not mutate durable state.
- Password hashing uses **Argon2id** via `golang.org/x/crypto/argon2`. Store hashes in a self-describing format, such as PHC string format, with algorithm and parameters. Start from the OWASP minimum profile of 19 MiB memory, 2 iterations, and parallelism 1, then tune if local hardware requires it. Never store plaintext or reversibly encrypted passwords.
- Owner passwords must be at least 12 Unicode characters and at most 1024 bytes. Apply the same policy to owner setup and local owner recovery; login may accept existing shorter passwords only if such hashes were created by an older build.
- Mutating browser API routes must reject non-JSON request bodies unless a feature explicitly documents another content type. Mutating browser API routes must validate same-origin requests before business logic runs; authenticated mutations must also validate the session-bound CSRF token.
- Public deployment requires HTTPS and explicit operator guidance.
- Public VPS deployment with real financial data requires MFA.
- Localhost development may use HTTP.
- LAN/private-network deployments should use HTTPS and may use either a reverse proxy or app-provided certificate and key configuration. Browser-warning-free LAN/private HTTPS requires either a real trusted certificate for a domain or installing a trusted local certificate authority on every client device.
- Local-network use may ship before MFA if authentication and operator guidance are clear.
- SQLite database encryption is deferred for early local use, but docs must explain when encrypted-at-rest storage may be needed.
- Docker Compose must package the same app shape as the single binary.
- The Docker production runtime image uses the official Debian 13 slim base: **`debian:trixie-slim`**. The app runs as a non-root numeric user.
- SQLite data must live in persistent storage outside the container image or binary.
- Backup and restore instructions are part of product documentation, not only operator folklore.
- Operator backups do not replace user-facing export features.
- Live SQLite backups must use SQLite-aware flows: the online backup API as the preferred in-app approach, or `VACUUM INTO` when a compact operator-triggered copy is acceptable. Do not recommend raw live file copy as the normal backup path.

## Observability Conventions

- Use **`log/slog`** (Go stdlib) for all structured logging. Output JSON in production, text in development. No third-party logging library.
- Log at `Info` for normal request lifecycle events, `Warn` for recoverable anomalies, `Error` for failures that need operator attention.
- Do not log financial record content (amounts, payees, account names) at any level.
- The application mode is controlled by the **`APP_ENV`** environment variable. Accepted values: `development`, `production`. Default to `production` when unset. Mode gates log format (text vs JSON) and any dev-only middleware.
- HTTP middleware layers run in a consistent order per request: request ID injection → panic recovery when added → request logging → auth check → CSRF validation for mutating authenticated requests → handler. Each layer is a plain `http.Handler` wrapper; no framework-specific middleware interface.
- A server-generated request-scoped UUID is generated for every inbound request, included in all log entries for that request, and echoed in the `X-Request-ID` response header. Incoming `X-Request-ID` may be logged as an external/caller request ID, but it does not replace the server-generated ID.
- The HTTP server must handle `SIGTERM` and `SIGINT` with `http.Server.Shutdown(ctx)` and a short grace period before exiting.

## Infrastructure And Port Conventions

- Backend dev server listens on **`:16888`** (via `HTTP_ADDR` env var).
- Frontend dev server (SvelteKit + Vite) listens on **`:1888`** and proxies `/api` to `http://localhost:16888`.
- Production single binary and Docker container serve on **`:16888`**.
- Production static frontend files are copied from `frontend/build/` into `backend/internal/web/dist/` before compiling the Go binary.
- Entity IDs in SQLite use **auto-increment integers** (`INTEGER PRIMARY KEY`). External-facing identifiers in API responses may use integers directly; do not convert to UUIDs unless a later ADR introduces distributed ID requirements.

## Scope Conventions

- User-facing export support must include core ledger CSV and QIF in the first export milestone.
- Attachments are out of scope until explicitly brought in by a later requirement or ADR.
- Mobile support must cover full core workflows responsively, including transaction entry.
- User and permission naming may stay explicitly single-user until shared workflows become active scope.

## Testing Conventions

- Backend behavior gets Go tests.
- Use **`testify`** (`testify/assert` for non-fatal checks, `testify/require` for fatal checks) in all Go tests. Do not write verbose `if got != want` assertion blocks.
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
- Sequencing belongs in `docs/product-requirements.md`.
- Decision records belong in `docs/adrs/`.
- Repo-wide agent guidance belongs in `AGENTS.md`.
- Developer workflow notes belong in `README.md` and `docs/developer-workflow.md` unless a tooling area needs substantial local guidance.
- Placeholder README files are acceptable for ignored or generated directories that must remain present in Git.
