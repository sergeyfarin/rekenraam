# Technical Backlog

Code-quality, technical-debt, and polish items — distinct from feature work.

- **Feature roadmap:** `docs/roadmap.md`.
- **Shipped features:** `docs/implemented.md`.

Status legend: `[ ]` open · `[x]` done · `[~]` won't fix / by design.

Each item names the exact file/line so it can be opened and fixed without
re-deriving the analysis. Items are verified against the actual code.

---

## Open

### T-01 Session lifetime is hardcoded (30 days) `[ ]`
**File:** `backend/internal/app/auth.go:207` (`sessionExpiresAt`)

`sessionExpiresAt(now)` takes no configuration. Add `SESSION_LIFETIME_HOURS` to
`config` (default 720, must be > 0), thread it into `AuthService`, and pass it to
`sessionExpiresAt`. Lets operators tighten sessions in higher-security
deployments without a recompile. Document the default and constraint in
`conventions.md` under the auth section.

### T-02 `BookID = int64(1)` is a hardcoded package constant `[~]`
**File:** `backend/internal/app/currencies.go:17`, referenced ~everywhere.

Deliberate single-book MVP scoping, not a bug — flagged so multi-book work is a
conscious change rather than a surprise. `book_id` is preserved in core tables
per AGENTS.md, so the migration path stays open. No action until multi-book is
scoped. A one-line comment at the definition marks intent.

### T-03 CSRF token does not rotate `[~]`
**File:** `docs/adrs/0002-http-security-policy.md` (documented design choice).

Acceptable: mitigated by `SameSite=Strict` + same-origin. True rotation needs
server-side nonce state (doubles session lookups) or a separate short-lived
double-submit cookie; neither is justified until cross-origin requests are
possible. Recorded as deliberate so it isn't reopened without justification.

### T-04 `unsafe-inline` in CSP `[~]`
**File:** `backend/internal/api/middleware.go` (`script-src 'self' 'unsafe-inline'`)

Present because SvelteKit injects an inline bootstrap script. Planned fix is a
build-time SHA-256 hash of the script. Not actionable until the build pipeline
emits the hash. Tracked, not forgotten.

### T-05 Frontend list pagination not consumed `[ ]`
**File:** `frontend/src/lib/api/transactions.ts` and reconciliation equivalents.

Backend returns `next_cursor`; some list views fetch once and ignore it. The
transactions/trash tables now use infinite-query options — audit the remaining
list helpers (reconciliation, pricing runs) to confirm they paginate or show a
hidden-results count rather than silently truncating.

### T-06 Import crash-consistency hole between ledger tx and identity write `[ ]`
**File:** `backend/internal/app/import_service.go` — `CommitImportBatch`, around line 420.

`CreateTransaction` commits its own internal DB transaction. The `import_commit_identities`
write and staged-row mark-committed happen in a separate transaction immediately after.
A crash between the two leaves an orphan posted ledger transaction with no identity row,
so a retry will duplicate it (the idempotency pre-check finds nothing). The fix requires
`CreateTransaction` to accept a caller-supplied `*sql.Tx` so the identity insert can join
the same transaction — a refactor that touches the transaction service signature. Deferred
because (a) real crashes are rare in practice, (b) the duplicate surfaces in the
`needs_review` queue where the user can catch it, and (c) the refactor is non-trivial.
When addressed, remove the `DB()` accessor on `ImportRepository` as it will no longer
be needed.

### T-07 Import endpoints missing from OpenAPI spec `[ ]`
**File:** `backend/api/openapi/openapi.yaml` — no `/api/v1/imports` paths exist.

The import API uses raw `fetch` on the frontend (multipart upload doesn't fit the typed
client) and was deferred as a known gap. Needs: path items for all 7 import routes
(`POST /imports`, `GET /imports`, `GET /imports/{batch_id}`, `PATCH /imports/{batch_id}`,
`POST /imports/{batch_id}/preview-commit`, `POST /imports/{batch_id}/commit`,
`POST /imports/{batch_id}/discard`), request/response schemas, and generated TS types
replacing the handwritten `frontend/src/lib/api/imports.ts` interfaces. Conflicts with
the OpenAPI-first convention (`docs/conventions.md` line 189).

---

## Resolved (kept for traceability)

The original code-quality audit (formerly root `backlog.md`, items B-01…B-22)
is fully closed: 20 fixed, 2 by-design. Highlights, so the fixes aren't
re-investigated:

- **Precision/arithmetic:** `scaledDivision` now half-up rounds (B-01); `pow10`
  is bounds-guarded and cached (B-02, perf); ledger overflow surfaces as HTTP 422
  `LEDGER_OVERFLOW` instead of 500 (B-03); dedicated `scaledAmount`/`scaledDivision`
  unit tests added (B-17, B-18).
- **Error handling:** `writeJSON` and `tx.Rollback` errors are logged, not
  dropped (B-05, B-06).
- **Security:** FTS5 phrase quoting sanitized (B-08); login dummy-bcrypt timing
  mitigation (B-09); `recover-owner` reads password with echo off (B-10).
- **Business logic:** drafts enforce ≥2-postings/≥1-entry structure and the
  `PostTransaction` draft-promotion bug fixed (B-14); `FinishReconciliation`
  asserts zero difference under the book write lock (B-13); dividend withholding
  commodity-consistency guarded (B-15); shared dividend input validation (B-20).
- **Lifecycle:** ledger endpoints reject `status=draft` (B-19); FX work no longer
  enqueued for drafts (B-21); `needs_review` import-review flag + `/approve`
  endpoint + queue filter added (B-22).

Backend review polish (formerly root `review2206.md`) is also resolved:
single-connection contract documented at `SetMaxOpenConns(1)`, request bodies use
`http.MaxBytesReader`, `pow10` cache landed, and inbound `X-Request-ID` is
honored.
