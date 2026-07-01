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

### T-14 Trading 212 fetch silently truncates history beyond 50 pages `[x]`
**File:** `backend/internal/onlinesource/trading212/fetcher.go` (`maxPages`, `Fetch`),
`backend/internal/app/import_fetch_worker.go` (`runTrading212Fetch`).

Closed by a post-Slice-3 review pass (2026-07-01). `FetchResult` now reports
`HasMore`/`NextPageToken`; `Fetch` takes a separate `resumeFrom` argument decoupled
from the incremental boundary (`cursor`) — reusing the boundary as a resume point was
an early draft of this fix and was wrong: every `Fetch` call restarts pagination at
page 1, so a boundary-as-resume-point would immediately re-trigger the incremental
stop on page 2 of the very next chunk and never reach the unfetched older pages.
`runTrading212Fetch` now enqueues a `reason="continuation"` work item
(`ResumePath`/`MaxCursorSoFar` fields on the payload) instead of marking the batch
ready when `HasMore` is true, and only persists the connection's cursor once the
whole chain (all chunks) naturally exhausts. `mergeTrading212BatchMeta` accumulates
hints/warnings across chunks instead of each chunk overwriting the last.
`stageParseResult` was changed to seed its within-batch dedupe set from rows already
staged by an earlier call on the same batch (previously always started an empty set,
correct only when called once per batch) — required because the incremental
boundary's overlap re-scan (see T-16) can legitimately re-see the same movement
across two chunks of one fetch, and needed to avoid mislabeling it. `row_index` now
continues from the batch's existing row count rather than restarting at 0 per call.
Verified: `internal/onlinesource/trading212/fetcher_test.go` (`TestFetchReportsHasMoreWhenPageBudgetExhausted`,
`TestFetchHasMoreFalseWhenProviderExhaustsNaturally`, `TestFetchHasMoreFalseWhenIncrementalCursorStops`)
and `internal/app/import_fetch_worker_test.go`
(`TestStartOnlineImport_ContinuesPastPageBudgetWithoutTruncatingOrDuplicating`, using
`trading212.SetMaxPagesForTest` to exercise a multi-chunk fetch without a real 50+
page server).

### T-15 Worker retry after partial success can re-stage duplicate rows `[x]`
**File:** `backend/internal/app/import_fetch_worker.go` (`runTrading212Fetch`),
`backend/internal/app/import_service.go` (`stageParseResult`).

Substantially closed as a side effect of the T-14 fix (2026-07-01), not a separate
change. The original concern: `stageParseResult`'s `InsertImportStagedRows` is a
plain bulk `INSERT` with no `(batch_id, dedupe_fingerprint)` uniqueness, so a retry
that re-ran `runTrading212Fetch` after staging had already succeeded (but a later
step failed) would silently re-insert the same movements as confusing duplicate
"new"-looking rows. Fixing T-14 required `stageParseResult` to seed its
`seenInBatch` dedupe set from rows already staged by an earlier call on the same
batch (for the continuation-chunk overlap case) — which incidentally makes exactly
this retry scenario correctly flag the re-staged rows `needs_attention` instead of
`new`. **Ledger safety was never actually at risk** (unchanged from the original
finding): `CommitImportBatch` checks `FindCommitIdentity` per row against the live
DB in commit order, so the first duplicate-fingerprint row to commit creates the
identity and the second is skipped regardless of its staged `dedupe_status` — this
fix only improves the preview-UI signal. `row_index` uniqueness across multiple
`stageParseResult` calls on one batch is also now guaranteed (see T-14) rather than
merely harmless-but-odd. Verified by
`internal/app/import_fetch_worker_test.go`'s
`TestStageParseResult_ReStagingSameBatchFlagsNeedsAttentionNotDuplicateNew`, which
calls `stageParseResult` twice against the same batch with freshly-constructed
(unhashed) `ParseResult` values per call — mirroring production, where each retry
re-parses fresh JSON — and confirms the second call's row is flagged
`needs_attention`, not `new`.

### T-16 Trading 212 online-import start had a real TOCTOU race and a stranded-batch failure mode `[x]`
**File:** `backend/internal/db/imports.go` (`StartOnlineImportBatch`),
`backend/internal/app/import_fetch_worker.go` (`startTrading212Fetch`).

Found and closed by a post-Slice-3 review pass (2026-07-01). The original
`startTrading212Fetch` ran the in-flight guard check, the batch insert, and the
background-work enqueue as three separate calls. Two real bugs followed: (1) if the
enqueue failed after the batch insert had already committed, the batch was stranded
permanently in `fetch_status=fetching` with no work item to ever process it, and
every later start/refresh for that connection would then hit the in-flight guard
forever with no recovery; (2) because the guard read and the batch write were
separate statements, two concurrent callers could both observe "no in-flight fetch"
before either had inserted its batch, both passing the guard and creating duplicate
fetches. Fixed with `ImportRepository.StartOnlineImportBatch`, which does the guard
check, batch insert (+ its audit trail), and work-item insert in one transaction.
Because the database is opened with `SetMaxOpenConns(1)`, one transaction here is a
full mutual-exclusion lock: a second caller's `BeginTx` blocks until the first
commits or rolls back, and a failure at any step rolls back everything so a
half-finished attempt never leaves a batch behind. Verified by
`internal/app/import_fetch_worker_test.go`'s
`TestStartOnlineImport_ConcurrentStartsRaceSafely` (12 concurrent
`StartOnlineImport` calls, exactly one must win — this test would have been flaky
against the old code) and
`TestStartOnlineImportBatch_PayloadFailureLeavesNoStrandedBatch` (a failing
work-payload builder must leave zero rows in both `import_batches` and
`background_work_items`).

### T-17 Trading 212 incremental cursor could silently drop same-timestamp movements `[x]`
**File:** `backend/internal/onlinesource/trading212/fetcher.go` (`Fetch`).

Found and closed by a post-Slice-3 review pass (2026-07-01). The incremental stop
condition was `item.Timestamp <= cursor`, treating any movement at exactly the saved
cursor's timestamp as already-seen. Since the cursor is a single timestamp value
rather than a per-movement watermark, two distinct movements can legitimately share
it (e.g. a page boundary split them across two different fetches); whichever one
wasn't captured in the fetch that first advanced the cursor to that instant would
never be re-scanned on any later incremental refresh. Changed to strict `<`, so a
same-timestamp movement is re-scanned rather than dropped — safe because the dedupe
fingerprint keys on provider ID, not timestamp, so re-scanning an already-known
movement is idempotent (flagged `needs_attention` if still un-committed and sitting
in an older batch, or `duplicate` if already committed). Verified by
`TestFetchIncludesMovementsAtExactCursorTimestamp` and updated assertions in
`TestFetchIncrementalStopsAtCursor`. Also hardened alongside: `Fetcher.fetchPage`
now refuses to follow an absolute (`http://`/`https://`) `nextPagePath`, since
blindly following one would attach the `Authorization` header (the user's real
Trading 212 API key) to whatever host a malicious/compromised/misconfigured provider
response pointed at — only paths relative to the configured base URL are trusted
with the key. Verified by `TestFetchRefusesAbsoluteNextPagePath`.

Deeper cursor-ordering questions remain open by design, not oversight: same-timestamp
re-scanning only guards the exact-tie case, not "the provider returns pages out of
strict chronological order" or "a backdated correction arrives with a timestamp
before the current cursor" (which would need a full re-fetch, `cursor=""`, to ever
surface, since normal incremental refresh only looks forward from the cursor). Both
depend on the live Trading 212 API's actual pagination/backdating semantics, which
`docs/trading212-import-plan.md`'s "Risks & open questions" already flags as
unverified assumptions — not fixed here to avoid guessing at behavior no one has
validated against the real API yet.

### B-T212-INVST Trading 212 investment lots not imported `[ ]`
**File:** `docs/trading212-import-plan.md` (Slice 4b), `docs/import-connection-accounts-plan.md`.

Online import (R7, Trading 212) imports **cash-account movements** only. Instrument
buy/sell fills are flagged `needs_attention` and not booked as lots — the same
stance QIF takes on `!Type:Invst`.

**Planned 2026-07-01, not yet built.** The investments-UI blocker is gone (R12
shipped), but scoping this for real implementation surfaced a second, unrelated
prerequisite: `import_connections` has no relationship to `accounts` at all today,
and a holding account in this codebase is 1:1 with a single instrument
(`CreateHoldingAccount`), so "the" holding account for a brokerage connection is a
growing per-instrument set, not one value — the plain per-batch account picker
cash-movement rows use doesn't generalize. `docs/import-connection-accounts-plan.md`
designs the fix (a `cash_account_id` column + an `import_connection_holdings`
mapping table, auto-created/linked per instrument). `docs/trading212-import-plan.md`
Slice 4b is the concrete implementation plan once that lands: new fetcher fields for
order fills (ticker/quantity/price — endpoint shape still unverified against the
live API, same caveat Slice 2 had for cash history), instrument find-or-create,
and a `CommitImportBatch` branch that routes to `InvestmentService.Buy/Sell/Dividend`
instead of the generic transaction builder.

### B-T212-SCHED Trading 212 scheduled auto-refresh `[ ]`
**File:** `docs/trading212-import-plan.md` (Slice 4a).

R7 ships **manual "refresh now"** on the durable queue. A per-connection scheduled
auto-refresh is a thin follow-up on the same machinery — deferred to keep the first
online slice small. **Planned 2026-07-01, not yet built:** Slice 4a specifies a
24h-since-last-successful-fetch cadence (not a fixed wall-clock time, to avoid
needing the book-owner-timezone plumbing `pricing_scheduler.go` has, and to be
self-correcting if the server was down), reusing the existing
`RefreshImportConnection` path and its in-flight guard — no new fetch logic, just
a scheduler goroutine mirroring `PricingService.StartScheduler`.

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
