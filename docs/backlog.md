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

### T-07 Import endpoints missing from OpenAPI spec `[x]`
**File:** `api/openapi/openapi.yaml`, `api/openapi/components/schemas/imports.yaml`.

Closed by adding path items for all 7 import routes, import request/response schemas,
and generated TS types. The frontend import helper still uses raw `fetch` for multipart
upload, but its public DTO types now come from `frontend/src/lib/api/schema.d.ts`.

### T-08 No encrypted-secret store for reusable third-party credentials `[x]`
**File:** `backend/internal/config/config.go`, `backend/internal/app/auth.go`.

Closed by Trading 212 Slice 1: `internal/secretbox` (AES-256-GCM, stdlib only) +
`REKENRAAM_SECRET_KEY` env var (optional boot, hard error on bad key). Used by
`ImportConnectionService` to seal/open API keys; reusable by every future online
provider.

### T-11 `ImportConnectionService` ships with a no-op key prober `[ ]`
**File:** `backend/cmd/rekenraam/command.go:81`, `backend/internal/app/import_connections.go:33-37`.

`NewImportConnectionService(..., nil)` resolves to `NoOpProber`, so
`POST /import-connections` and key rotation on `PATCH` accept any non-blank
string as a valid API key — nothing is actually checked against Trading 212 (or
any provider) yet. The OpenAPI description for `201` was reworded to say the key
is "saved but not verified" so the contract isn't misleading in the meantime.
**Planned fix:** wire a real `ConnectionProber` (Trading 212 Slice 2,
`internal/onlinesource/trading212`) before this is presented to users as a
verified connection.

### T-12 Deleting an import connection erases batch provenance `[ ]`
**File:** `backend/migrations/0007_online_import.sql:27` (`ON DELETE SET NULL`),
`backend/internal/db/import_connections.go:186` (`DeleteImportConnection`).

The migration comment says `import_batches.connection_id` exists so batch history
stays queryable by connection, but deletion is a hard `DELETE` and the FK is
`ON DELETE SET NULL` — once a connection is deleted, any batches it produced lose
their link to it. **Not yet an active bug**: nothing currently writes
`import_batches.connection_id` (the fetch worker that would is Slice 2+), so no
data loss is possible today. Before the fetch worker ships, choose one: (a) make
delete a revoke/archive (e.g. `revoked_at` column, hide from list/create but keep
the row so the FK stays valid), or (b) denormalize immutable connection metadata
(source, display_name at time of fetch) onto `import_batches` so the historical
record survives connection deletion even with `ON DELETE SET NULL`.

### B-T212-INVST Trading 212 investment lots not imported `[ ]`
**File:** `docs/trading212-import-plan.md` (scope), future `internal/onlinesource/trading212`.

Online import (R7, Trading 212) imports **cash-account movements** only. Instrument
buy/sell fills are flagged `needs_attention` and not booked as lots — the same
stance QIF takes on `!Type:Invst`.

**No longer blocked.** The original blocker (no investments ledger UI) is gone: the
investments feature shipped end-to-end (R12, all four cost-basis methods, sell
preview, gains reporting — see `implemented.md`). This is now a scoped task inside
the Trading 212 work itself (`investments-plan.md` Slice 5): map order fills to
buys/sells and dividends to investment events through the now-UI-backed investment
service. It remains open only because R7/Trading 212 is itself deferred, not because
a prerequisite is missing.

### B-T212-SCHED Trading 212 scheduled auto-refresh `[ ]`
**File:** `docs/trading212-import-plan.md` (Slice 4).

R7 ships **manual "refresh now"** on the durable queue. A per-connection scheduled
auto-refresh (daily domain trigger enqueuing `import.fetch.trading212`) is a thin
follow-up on the same machinery — deferred to keep the first online slice small.

> **I-03 and I-04 are open *product/accounting decisions*, not blockers.** Both
> were deliberately scoped out of the shipped investments feature (R12) and neither
> gates anything currently planned. They become actionable only if/when the related
> reporting or accounting need is prioritized — recorded here so the choice is made
> consciously rather than by omission.

### I-03 Multi-method analytical gains reporting `[ ]`
**File:** `docs/investments-plan.md` ("Cost-basis methods → Future").

Tax vs. performance reporting can need *different* cost-basis methods. This is
**read-side only**: the authoritative method (I-02) drives lot disposal and the
realized-gain figure computed from it; analytical reports re-derive
realized/unrealized gains under alternative methods from the immutable lot+disposal
history **without mutating lots or posting**. Data-model requirement enforced now:
keep full per-lot acquisition + disposal history (already true; do not collapse lots
irreversibly — a reason to prefer the per-lot average-cost model). Slice 4 shipped
single-method (authoritative) gains reporting; this *multi-method analytical* layer
is the remaining, deferred read-side extension, designed if/when comparative
tax-vs-performance reporting is prioritized.

### I-04 Realized gain is computed, not posted to a gain/loss account `[ ]`
**File:** `backend/internal/app/investments.go:790` (sell transaction shape),
`backend/migrations/0001_initial_schema.sql:224` (account_class enum).

The sell transaction posts four legs (security out, security→trading, cash in,
cash←trading) and **no realized gain/loss account** — the taxonomy has no gain/loss
account kind (`account_class IN (asset, liability, equity, income, expense)`).
Realized gain is **derivable** (cash proceeds − disposed cost basis from the disposal
records) and the investments plan computes/surfaces it, but it is **not a ledger
posting**. Posting realized gains (so reports/registers show gain as an income/equity
movement) would need: a realized-gain account kind or convention, a per-instrument or
book-level gain-account mapping, and a sell transaction-shape change to add the gain
leg. Deferred as a conscious product/accounting decision, not an oversight. The
shipped gains report (Slice 4) surfaces realized gain as a *computed* figure; promoting
it to a *posted* ledger movement is the open decision here, interacting with I-03.

---

## Resolved (kept for traceability)

**Investments correctness gate (I-01, I-02) — closed by the investments feature
(R12, Slice 1).** Both holes the FIFO audit found are fixed in code:

- **I-01 (explicit allocations bypassed the method):** `disposeLotsWithAuditTx`
  (`backend/internal/db/investments.go`) now permits explicit `LotAllocations` only
  for `specific_lot` (validated for ownership, open status, and matching quantity);
  fifo/lifo/average_cost derive their allocation server-side. A client can no longer
  sell newest-first under a FIFO account.
- **I-02 (only FIFO was implemented):** all four methods — `fifo`, `lifo`
  (`ORDER BY opened_on DESC, id DESC`), `average_cost` (per-lot weighted-average,
  model 2), `specific_lot` — are real, with a `resolveCostBasisMethod` resolver
  implementing 3-tier selection (per-transaction → `account_versions.cost_basis_method`
  (migration 0005) → global default). A `POST /investments/sell/preview` endpoint shares
  the disposal calc with commit so they never diverge; an unimplemented method value
  fails loudly. Full DB + app test coverage.

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

**T-09/T-10 background work queue extraction — closed.** The generic queue now has
its own `db.BackgroundWorkRepository`, while `PricingRepository` embeds it so
existing pricing call sites remain intact. `EnqueueBackgroundWork` coalesces exact
active duplicates through a partial unique index on `(book_id, kind, payload_json)
WHERE status IN ('pending','running')`, returns the existing active item on
conflict, and permits equivalent work to be enqueued again after completion. The
FX trigger producers were recreated conflict-aware in the same migration.
