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

### T-11 `ImportConnectionService` ships with a no-op key prober `[x]`
**File:** `backend/cmd/rekenraam/command.go:81`, `backend/internal/app/import_connections.go`.

Closed by Trading 212 Slice 2: `Trading212Prober` (`backend/internal/app/import_trading212.go`)
wraps `internal/onlinesource/trading212.Fetcher.Probe` and is wired in
`command.go` in place of `nil`/`NoOpProber`. `ConnectionProber.Probe` gained a
`configJSON` parameter so the prober can honor `config_json.base_url` (demo/sandbox
endpoint); the prober no-ops for any `sourceKind != "trading212"` so other future
source kinds still fall back to `NoOpProber` behavior until they get a real
implementation. The OpenAPI `201` description still says "saved but not verified"
for non-trading212 kinds — update it if/when a second provider ships a real prober.

### T-12 Deleting an import connection erases batch provenance `[x]`
**File:** `backend/migrations/0007_online_import.sql:27` (`ON DELETE SET NULL`),
`backend/internal/app/import_fetch_worker.go` (`trading212BatchMeta`).

Closed by Trading 212 Slice 3, option (b) from the two choices this item originally
posed: `startTrading212Fetch` and `runTrading212Fetch` snapshot
`connection_display_name` into `import_batches.source_meta_json` (`trading212BatchMeta`)
at batch-create and fetch-complete time. `import_batches.connection_id` is now
actually written (it sat unused since migration `0007`); when a connection is later
hard-deleted the FK still `SET NULL`s `connection_id`, but the batch's `source_meta`
retains the human-readable name, so import history stays readable. Verified by a
manual smoke test: create connection → import → delete connection → `GET
/imports/{id}` still shows `"connection_display_name":"..."` with `connection_id`
simply absent from the response.

### T-13 `ImportService.StartImport`/`stageParseResult` has no service-level test `[~]`
**File:** `backend/internal/app/import_service.go` (`StartImport`, `stageParseResult`).

Narrowed by Trading 212 Slice 3, not fully closed. `stageParseResult` — the shared
fingerprint-hash → dedupe → insert pipeline both `StartImport` (file path) and the
new fetch worker (online path) call — now has direct regression coverage via
`backend/internal/app/import_fetch_worker_test.go`
(`TestStartOnlineImport_WorkerStagesRowsAndUpdatesCursor`,
`TestRefreshImportConnection_IncrementalOnlyNewMovementsAndSkipsCommitted`), which
drive it against a real `*sql.DB` and assert dedupe-status transitions (`new` vs
`duplicate` against `import_commit_identities`) the same way this item asked for.
What remains untested at the service level is `StartImport`'s *own* thin wrapper
(`adapter.Parse` → `CreateImportBatch` → `stageParseResult`) for the file-upload
entry point specifically — still only covered by `import_qif_test.go`'s parser-level
tests and manual/UI testing. Low priority now that the shared core has coverage;
revisit if QIF-path regressions start slipping through.

### T-14 Trading 212 fetch silently truncates history beyond 50 pages `[ ]`
**File:** `backend/internal/onlinesource/trading212/fetcher.go:25` (`maxPages`),
`backend/internal/app/import_fetch_worker.go` (`runTrading212Fetch`).

Found while implementing Slice 3 (the Slice 2 code comment on `maxPages` already
flagged this as deferred: *"the durable queue's continuation payload (Slice 3) is
the intended mechanism for fetching more than this many pages in one logical
refresh"* — not built). `Fetcher.Fetch` loops at most `maxPages=50` pages and
returns whatever it collected with no signal of whether the provider had more.
`runTrading212Fetch` always treats a successful `Fetch` as complete, marking
`fetch_status="ready"`. For an account with more than ~50 pages of cash-movement
history, the **first full import silently omits everything beyond page 50** — and
because the saved cursor becomes the *newest* timestamp seen on a successful fetch,
a later "Refresh" looks only for movements *newer* than that cursor; it does not
resume backfilling the older tail that was cut off. There is currently no UI signal
and no recovery path other than re-running a full fetch with `cursor=""`, which
re-runs into the same 50-page cap. Fix requires: (1) `FetchResult` to report
`HasMore bool` (true only when the loop exhausted its page budget, not when it
stopped naturally or hit the incremental cursor — distinguishing the three cases
matters), and (2) the worker to enqueue a `reason="continuation"` work item using the
new cursor instead of marking the batch ready, accumulating into the same batch
across multiple worker ticks (same pattern as `pricing_worker.go`'s FX-coverage
continuation). Deferred rather than rushed: getting the cursor/ordering semantics
wrong here is worse than leaving the gap documented.

### T-15 Worker retry after partial success can re-stage duplicate rows `[ ]`
**File:** `backend/internal/app/import_fetch_worker.go` (`runTrading212Fetch`),
`backend/internal/app/import_service.go` (`stageParseResult`).

Found while implementing Slice 3. `stageParseResult`'s `InsertImportStagedRows` is a
plain bulk `INSERT` with no `(batch_id, dedupe_fingerprint)` uniqueness — it relies
on being called exactly once per batch. `runTrading212Fetch` calls it, then still has
two more fallible steps (`UpdateImportBatchSourceMeta`, `ImportConnectionService.UpdateFetchCursor`)
before returning success. If either of those fails (or the process is killed between
them), the wrapper treats the whole attempt as retryable and a later worker tick
re-runs `runTrading212Fetch` from scratch — re-fetching and re-staging the *same*
movements into the *same* batch as a second set of `import_staged_rows` (different
row ids, identical `dedupe_fingerprint`). **Not a ledger-correctness bug**: at
`CommitImportBatch` time, rows are processed in order and `FindCommitIdentity` is
checked per row against the live DB, so the first duplicate-fingerprint row to commit
creates the `import_commit_identities` row and the second is skipped as a duplicate —
no double-posting. It *is* a confusing preview-UI bug (the user sees the same
movement listed twice before committing). The narrow trigger window (a SQLite
single-row `UPDATE` failing) makes this low-probability; fixing it properly means
making `stageParseResult` idempotent (e.g. `INSERT OR IGNORE` keyed on
`(batch_id, dedupe_fingerprint)`, which also needs a new unique index and a decision
about what happens to a duplicate row's `row_index`), which is exactly the
crash-consistency territory T-06 already flags as "do not regress" — left open
rather than touched under time pressure.

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
