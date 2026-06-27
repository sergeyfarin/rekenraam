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

### T-08 No encrypted-secret store for reusable third-party credentials `[ ]`
**File:** `backend/internal/config/config.go`, `backend/internal/app/auth.go`.

The repo stores passwords as one-way argon2 *hashes* and the only third-party key
(`OPEN_EXCHANGE_RATES_APP_ID`) as an **env var**. There is no facility to store a
*reusable, reversible* secret a user pastes at runtime and we must replay to a
provider (e.g. a Trading 212 API key). Online import (R7) needs one.
**Planned fix:** `internal/secretbox` (AES-256-GCM, stdlib only) + a
`REKENRAAM_SECRET_KEY` env var, designed in `docs/trading212-import-plan.md`
(Slice 1). Built once, reused by every online provider. Listed here so the gap is
visible outside the Trading 212 plan.

### B-T212-INVST Trading 212 investment lots not imported `[ ]`
**File:** `docs/trading212-import-plan.md` (scope), future `internal/onlinesource/trading212`.

Online import (R7, Trading 212) imports **cash-account movements** only. Instrument
buy/sell fills are flagged `needs_attention` and not booked as lots, because the
investments ledger UI does not exist yet — the same stance QIF takes on
`!Type:Invst`. Revisit after the investments UI ships.

### B-T212-SCHED Trading 212 scheduled auto-refresh `[ ]`
**File:** `docs/trading212-import-plan.md` (Slice 4).

R7 ships **manual "refresh now"** on the durable queue. A per-connection scheduled
auto-refresh (daily domain trigger enqueuing `import.fetch.trading212`) is a thin
follow-up on the same machinery — deferred to keep the first online slice small.

### T-09 Durable work queue does not coalesce duplicate enqueues `[ ]`
**File:** `backend/internal/db/background_work.go:27` (`EnqueueBackgroundWork`).

`EnqueueBackgroundWork` is a plain `INSERT` — no uniqueness constraint and no
`WHERE NOT EXISTS` on `(book_id, kind, payload)`. The FX-refresh plan
(`docs/fx-refresh-implementation-plan.md`, Slice 1) describes "enqueue/coalesce"
but only enqueue shipped; FX tolerates it because coverage work is idempotent.
**Impact:** any caller that must not run twice concurrently (e.g. Trading 212
"refresh now", T212 plan) has to guard at the service layer instead. **Fix:**
add a partial unique index on un-completed work `(book_id, kind, payload_json)
WHERE status IN ('pending','running')` and make enqueue `INSERT … ON CONFLICT DO
NOTHING`, returning the existing item. Then the service-layer guards become
defence-in-depth rather than the only protection.

### T-10 Generic work queue methods live on `PricingRepository` `[ ]`
**File:** `backend/internal/db/background_work.go` (all methods are
`func (r *PricingRepository) …`).

The queue is generic (`kind`-dispatched) but its repository methods hang off
`*PricingRepository`. Any second consumer (the Trading 212 import worker, T212
plan Slice 3) would have to depend on the pricing repository — a wrong dependency
edge (`import → pricing`). **Fix (pure move):** extract a standalone
`*db.BackgroundWorkRepository` over the same `*sql.DB`; have `PricingRepository`
embed/delegate to it so existing call sites are untouched. Prerequisite for the
import fetch worker.

### I-01 Explicit lot allocations bypass the cost-basis method `[ ]`
**File:** `backend/internal/db/investments.go:899` (`disposeLotsWithAuditTx`),
`backend/internal/app/investments.go:802` (sell path passes `Allocations`).

FIFO is enforced only on the **implicit** disposal path (`ORDER BY opened_on, id`,
`ErrInsufficientLots` guards oversell). When a sell request carries explicit
`LotAllocations`, the backend disposes exactly those lots with **no validation**
that the selection is permitted for the account's cost-basis method — a client can
sell newest-first under a "FIFO" account. Not reachable today (no sell UI), but a
**hard gate before the investments sell UI** (`docs/investments-plan.md` Slice 1).
**Fix:** derive the allocation server-side from the method for fifo/lifo; allow
explicit allocations only for `specific_lot`, still validated for ownership/open
status/quantity.

### I-02 `lifo` / `average_cost` cost-basis methods accepted but not implemented `[ ]`
**File:** `backend/internal/app/investments.go:1267` (validation),
`backend/internal/db/investments.go:899` (disposal only does FIFO).

The cost-basis method is validated against `{fifo, lifo, average_cost,
specific_lot}` and stored on a profile, but **disposal only ever does FIFO**. A
user can save a "LIFO" profile and a sell silently disposes FIFO — a
correctness/trust bug. **Fix (investments plan Slice 1):** ship `fifo` +
`specific_lot` working; make `lifo`/`average_cost` return a clear `NOT_IMPLEMENTED`
error at profile-save time rather than silently doing FIFO.

### I-03 Implement remaining cost-basis methods (lifo, average_cost) `[ ]`
**File:** `backend/internal/db/investments.go` (`disposeLotsWithAuditTx`).

Follow-up to I-02: implement `lifo` (`ORDER BY opened_on DESC, id DESC`) and
`average_cost` (weighted-average disposal cost) once the read-side reporting
semantics are decided (`docs/investments-plan.md` risk 2). Until then they are
gated by I-02's error.

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
