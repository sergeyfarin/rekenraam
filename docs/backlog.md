# Technical Backlog

Code-quality, technical-debt, and polish items — distinct from feature work.

- **Feature roadmap:** `docs/roadmap.md`.
- **Shipped features:** `docs/implemented.md`.

Status legend: `[ ]` open · `[x]` done · `[~]` won't fix / by design.

Each item names the exact file/line so it can be opened and fixed without
re-deriving the analysis. Items are verified against the actual code.

---

## Open

### T-01 Session lifetime is hardcoded (30 days) `[x]`
**File:** `backend/internal/app/auth.go:207` (`sessionExpiresAt`)

Closed by adding `SESSION_LIFETIME_HOURS` to config (default `720`, must be a
positive integer number of hours), threading it into `AuthService`, and passing
the configured duration into `sessionExpiresAt` for login-created sessions. The
owner setup flow still uses the product default unless setup is deliberately made
configurable too. Documented in `docs/conventions.md` and verified by config
parsing tests plus `TestSessionExpiresAtUsesConfiguredLifetime`.

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

### T-05 Frontend list pagination not consumed `[x]`
**File:** `frontend/src/lib/api/transactions.ts` and reconciliation equivalents.

Closed by auditing every frontend consumer of response shapes with
`next_cursor`. Transactions, account registers, and trash already use
TanStack infinite-query options. Reconciliation checkpoint/session helpers do
not expose cursor-paginated responses. Pricing refresh runs currently expose a
fixed latest-run/history summary with no cursor; the shipped currencies UI only
uses the latest run, so it is not silently truncating a visible list. The one
remaining cursor-loss path was import batch preview rows (tracked separately as
T-23) and is now fixed by `getFullImportBatch`, which follows every
`next_cursor` page before the preview can be committed. Verified by
`frontend/src/lib/api/imports.test.ts`.

### T-06 Import crash-consistency hole between ledger tx and identity write `[x]`
**File:** `backend/internal/app/import_service.go` — `CommitImportBatch`, around line 420.

Closed by splitting transaction creation into preparation plus a tx-scoped write
path (`TransactionRepository.CreateTransactionInTx`) and routing generic import
commits through `ImportRepository.CommitImportedTransaction`. The generic import
row now writes the ledger transaction, `import_commit_identities`, and the
staged-row `committed` marker in one SQL transaction, so a failure between those
steps rolls all three back together. The old `ImportRepository.DB()` escape hatch
was removed. Verified by
`TestCommitImportedTransaction_RollsBackLedgerWhenIdentityWriteFails`, which
forces the identity insert to fail after `createTransactionTx` and asserts that
no ledger transaction, identity row, or committed staged-row marker escapes.

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

**Retrospective note (2026-07-03):** this item undersold the actual gap.
Staging (`stageParseResult`) had coverage, but **committing** — the entire
point of an import — did not: no test anywhere in the suite drove a staged
row through `CommitImportBatch` against a real account/ledger, for *any*
source. That gap hid a severity-1 bug (T-22, `EntryKind: "main"`) that made
every single commit fail from day one. Closed by Slice 4b's tests; see T-22.

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

### T-18 No periodic cleanup of expired/revoked auth sessions `[x]`
**File:** `backend/internal/app/auth.go`, `backend/migrations/0001_initial_schema.sql`
(`auth_sessions_expires_revoked_idx`).

Closed by `AuthService.StartSessionCleanup`, which deletes rows where
`revoked_at IS NOT NULL OR expires_at <= now` at server startup and then every
24 hours. `AuthRepository.DeleteExpiredOrRevokedSessions` uses the existing
`auth_sessions_expires_revoked_idx`-supported predicate. Verified by
`TestCleanupExpiredAndRevokedSessionsDeletesOnlyInactiveRows`, which seeds active,
expired, and revoked sessions and confirms only the active session remains.

### T-19 `REKENRAAM_SECRET_KEY` has no rotation path `[x]`
**File:** `backend/internal/secretbox/secretbox.go`, `backend/internal/config/config.go`,
`backend/internal/db/import_connections.go`.

Closed by documenting the current operational path rather than adding a
maintenance command. `README.md`, `docs/developer-workflow.md`, and
`docs/conventions.md` now state that `REKENRAAM_SECRET_KEY` is a base64-encoded
32-byte key, must be backed up with the SQLite database, and cannot currently be
rotated in place. If the key is lost or intentionally rotated, the documented
procedure is: stop the app, take and verify a SQLite backup, start with the new
key, delete affected online import connections, and re-add them with fresh
provider credentials. Imported ledger data, lots, batches, and audit history
remain durable.

Validation: docs-only change; `git diff --check` over touched docs.

### T-20 E2E coverage is 2 specs vs a full shipped app `[x]`
**File:** `e2e/playwright/` (`auth.spec.ts`, `health.spec.ts`).

Closed by adding the first real money journey:
`e2e/playwright/transactions.spec.ts` seeds setup prerequisites through the same
browser session and creates a simple transaction through the UI form. The auth
spec was updated to complete the currency setup step before expecting the app
shell, and `playwright.config.ts` now uses `workers: 1` because the suite starts
one app instance over one SQLite database. Verified by `./scripts/test-e2e.sh`
(`4 passed`). Remaining browser journeys are tracked in T-27 so this closure
means "not only auth/health anymore," not "every shipped workflow has E2E."

### T-21 Trading 212 fetcher hit a nonexistent endpoint path with a wrong type enum `[x]`
**File:** `backend/internal/onlinesource/trading212/fetcher.go`, `backend/internal/app/import_trading212.go`.

Found and closed 2026-07-03 while scoping Slice 4b against the real,
published Trading 212 OpenAPI spec (`docs.trading212.com`) — the first time
this integration was checked against verified field names/paths rather than
the plan's documented-as-unverified assumptions. Two real bugs in
already-shipped (Slice 2/3) code: (1) `historyPath` was
`/history/transactions`, missing the `/equity` segment the real API requires
(`/equity/history/transactions`) — every live fetch and every connection
probe (which reused the same path) would 404; a real user could never
successfully create a working Trading 212 connection. (2) `cashMovementTypes`
included `WITHDRAWAL`, `DIVIDEND`, `INTEREST`, `CARD_CREDIT`, `CARD_DEBIT`,
`CARD_TOPUP`, `TRANSFER_IN`, `TRANSFER_OUT`, `LENDING_INTEREST` — none of
which the real endpoint's `type` enum (`WITHDRAW`/`DEPOSIT`/`FEE`/`TRANSFER`
only) ever emits; every real cash movement except `DEPOSIT`/`FEE` would have
been wrongly flagged `needs_attention`. Fixed: corrected `historyPath`;
`Probe` now hits the dedicated `/equity/account/summary` endpoint instead of
reusing history (avoids requiring the `history:transactions` scope just to
validate a key); `cashMovementTypes` matches the real enum exactly. Verified
by `TestFetchHitsRealHistoryPath`, `TestProbeUsesAccountSummaryEndpointAndSucceeds`,
and `TestTrading212Adapter_CashMovementTypesMatchRealAPIEnum` (asserts the old
guessed values now correctly flag `needs_attention`). This also resolved
Slice 4b's biggest open risk — the order-fill/dividend endpoint shapes are
now verified, not guessed (see `docs/trading212-import-plan.md` Slice 4b).

### T-22 Every import commit failed "entry kind is invalid" — never caught by any test `[x]`
**File:** `backend/internal/app/import_service.go` (`buildTransactionSpec`),
`backend/internal/app/transactions_validate.go` (`entryKinds`).

Found and closed 2026-07-03 while building Slice 4b's test fixtures — the
first tests in the whole suite to drive a staged row through the complete
`CommitImportBatch` → `TransactionService.CreateTransaction` path against a
real account (see T-13: "narrowed, not fully closed" undersold how much was
actually untested). Severity-1, and **not specific to Trading 212**: every
single `CommitImportBatch` call, for *any* import source (QIF file upload or
online), set `JournalEntryInput.EntryKind` to the literal string `"main"` —
not a member of the valid `entryKinds` set
(`ordinary`/`transfer_leg`/`exchange`/`investment`/`opening_balance`/`adjustment`).
The moment a committed row actually reached real validation, it failed with
`"entry kind is invalid"` and the whole batch reported `failed`/`0 committed`.
This has been present since the import feature's very first commit
(2026-06-27) — every prior test either checked `buildTransactionSpec`'s
returned struct in isolation (never posting it) or never called
`CommitImportBatch` at all, so nothing ever exercised the real posting path.
Fixed: `EntryKind: "ordinary"`. Verified by
`TestCommitImportBatch_GenericCashRowCommitsToRealLedger`
(`backend/internal/app/import_trading212_invest_test.go` — deliberately
source-agnostic, no Trading 212 involved, to prove this is a pipeline fix not
a Trading-212-specific one) plus the four B-T212-INVST commit tests in the
same file, all of which exercise the same real-ledger commit path.

### T-23 Import preview UI only reviews the first page; commit processes every staged row `[x]`
**File:** `frontend/src/routes/app/import/+page.svelte` (`pollFetchStatus`, `handleCommit`),
`backend/internal/app/import_service.go` (`GetImportBatch`, `CommitImportBatch`).

Found during a Slice-4b-era review pass (2026-07-04). The preview screen fetched
one page of staged rows (`getImportBatch(batchId, { limit: 500 })`) and never
followed `next_cursor`; the backend also silently clamps any `limit > 200` down
to 100 (`GetImportBatch`). `CommitImportBatch`, however, iterates *every*
staged row in the batch regardless of what the UI ever rendered.

Closed by `frontend/src/lib/api/imports.ts:getFullImportBatch`, which requests
the service's real max page size (200) and follows `next_cursor` until the
whole batch is loaded. The online-import poller now enters preview only after
loading all staged rows, so review, bulk apply, patch, and commit operate on
the same complete row set. Verified by `getFullImportBatch`'s unit test in
`frontend/src/lib/api/imports.test.ts`.

### T-24 Investment trade/dividend postings bypass the reconciliation guard entirely `[x]`
**File:** `backend/internal/app/investments.go` (`Buy`, `Sell`, `Dividend`,
`ReinvestedDividend`), `backend/internal/app/transactions_write.go` (`CreateTransaction`).

Found during a Slice-4b-era review pass (2026-07-04). `TransactionService.CreateTransaction`
is the only place that computes `reconciliationInvalidationRefsFromSpec` and enforces
`ErrReconciliationOverrideRequired` when a posting would change a reconciled balance
(`transactions_write.go:19-29`). `InvestmentService.Buy`/`Sell`/`Dividend`/`ReinvestedDividend`
never call `CreateTransaction` — they call `prepareCreateTransaction` (which only cleans/
validates the spec) and then post directly via `InvestmentRepository.CreateTransactionAndLot`/
`CreateTransactionAndDisposeLots`, skipping the guard entirely. Per
`.claude/skills/ledger-invariants`: "any operation that changes a reconciled balance must be
guarded... forgetting the guard on a new path is a severity-1 bug." Concretely: a backdated
Trading 212 buy/sell/dividend imported into an already-reconciled period posts successfully
with no override check and no checkpoint invalidation — the reconciled balance silently
becomes wrong with no record that it happened. This is a pre-existing gap in the investment
service itself (predates B-T212-INVST), not something the Slice 4b import work introduced,
but the import path is what makes it newly reachable with backdated, unreviewed data.
`CommitImportBatchInput.ReconciliationOverride` is threaded into the generic cash commit
path (`import_service.go:485`) but has no equivalent for the investment commit branch
(`import_trading212_invest.go`) — even if the guard existed, there's no override plumbing to
pass it through yet. Fix requires: (1) `InvestmentTradeInput`/`DividendInput` gain a
`ReconciliationOverride bool`, (2) `Buy`/`Sell`/`Dividend`/`ReinvestedDividend` compute
invalidation refs and enforce the guard the same way `CreateTransaction` does (likely by
having `InvestmentRepository.CreateTransactionAndLot`/`CreateTransactionAndDisposeLots`
accept `InvalidateCheckpointRefs`/`InvalidateCheckpointReason` the way `CreateTransactionParams`
already does), (3) the T212 investment commit path passes `input.ReconciliationOverride`
through. Needs a named test: sell/buy/dividend into a reconciled period without override
must fail `ErrReconciliationOverrideRequired`, and with override must invalidate the
checkpoint — mirroring the existing `transactions_write.go` coverage for the generic path.

Closed by routing investment creates through the same guard-aware transaction
preparation used by ordinary creates. `InvestmentTradeInput`, `DividendInput`,
and `ReinvestedDividendInput` now carry `ReconciliationOverride`; buy, sell,
and reinvested-dividend use `prepareCreateTransactionForWrite`; dividend
continues through `CreateTransaction` and now receives the override flag. The
Trading 212 investment commit path threads `CommitImportBatchInput.ReconciliationOverride`
through to buy/sell/dividend and classifies `ErrReconciliationOverrideRequired`
as a skipped row, matching the generic import path.

The fix also closed the lower-level invalidation gap found while testing:
`InvestmentRepository.CreateTransactionAndLot` and
`CreateTransactionAndDisposeLots` now apply `CreateTransactionParams` checkpoint
invalidation refs inside the same DB transaction as the ledger transaction plus
lot/disposal write. Verified by
`TestCommitImportBatch_InvestmentBuyCrossingReconciledPeriodRequiresOverride`,
which proves a Trading 212 buy into a reconciled period skips without override
and commits with override while invalidating the checkpoint.

### T-25 Pricing refresh run history has no cursor or hidden-results signal `[x]`
**File:** `backend/internal/api/pricing.go` (`listPricingRefreshRuns`),
`backend/internal/api/settings.go` (`currencySettingsPage`),
`backend/internal/app/pricing.go` (`ListRefreshRuns`).

Closed by changing `PricingService.ListRefreshRuns` to fetch one extra row and
return `PricingRefreshRuns{Runs, HasMore}`. `GET /pricing/refresh/runs` now
returns `has_more`, and the currency settings page read model returns
`refresh_runs_has_more`, so future history UI cannot mistake the fixed latest
50-row summary for a complete list. OpenAPI and generated frontend API types
were updated. Verified by
`TestListRefreshRunsReportsHasMoreWithoutReturningHiddenRows` and the composed
currency settings API test.

### T-26 Trading 212 investment import identity is still recorded after the investment ledger transaction `[ ]`
**File:** `backend/internal/app/import_trading212_invest.go`
(`commitTrading212InvestmentRow`), `backend/internal/app/import_service.go`
(`recordCommitIdentityAndMarkRow`), `backend/internal/db/investments.go`
(`CreateTransactionAndLot`, `CreateTransactionAndDisposeLots`).

Found while closing T-06 (2026-07-08). Generic cash import rows now commit the
ledger transaction, import identity, and staged-row marker atomically, but the
Trading 212 investment branch still posts through `InvestmentService.Buy` /
`Sell` / `Dividend` first and records the import identity in a second
transaction afterwards. A crash in that narrow window can orphan an investment
ledger transaction without an import identity, so retry can duplicate it. This
is separate from T-06's generic `CreateTransaction` path because buy/sell also
write lots/disposals in investment repository transactions. T-24 closed the
reconciliation guard/invalidation behavior for those repository transactions,
but the import identity is still a second commit. Fix next by adding tx-scoped
investment posting helpers so the investment transaction, lot/disposal rows,
import identity, and staged-row mark share one commit. Add named tests for buy,
sell, and dividend rollback when identity recording fails.

### T-27 Remaining critical browser journeys need E2E coverage `[ ]`
**File:** `e2e/playwright/`.

Follow-up after closing T-20's initial "only auth/health" gap. Transaction entry
now has one simple browser journey, but split/transfer transactions, reconcile
to zero, QIF import preview→commit, buy/sell with preview, and at least one
mobile viewport run still need Playwright coverage. Add one journey per feature
slice and keep the shared-DB suite serial unless the harness is changed to
provision an isolated database per worker.

### B-T212-INVST Trading 212 investment lots not imported `[~]`
**File:** `docs/trading212-import-plan.md` (Slice 4b), `docs/import-connection-accounts-plan.md`,
`backend/internal/app/import_trading212_invest.go`.

Closed 2026-07-03 (Slice 4b) for the core flow: order fills route through
`InvestmentService.Buy`/`Sell` (real lots, via a per-connection
per-instrument holding account, `import_connection_holdings`), dividends
through `InvestmentService.Dividend`. Instrument resolution is ISIN-first,
ticker/symbol-second, create-third
(`InvestmentService.ResolveOrCreateInstrumentForImport`); all creation
(instrument, holding account) happens at commit time only, never at fetch
time (the accounts-plan doc's discard-orphan concern). Anything that can't
resolve (no `cash_account_id` configured, no instrument match, insufficient
lots, no dividend default) falls back to the pre-4b plain-cash-row behavior
— a strict superset, never a regression. See
`docs/trading212-import-plan.md` Slice 4b for the full writeup, including
two bugs found and fixed while building this (a holding-account
`opened_on`/`effective_from` defaulting to "today" instead of the trade's
own date, and the unrelated severity-1 `EntryKind: "main"` bug, T-22).

**Remaining, by design (`[~]` not `[ ]`):** accounts-plan "scenario 2" (link
a Trading 212 instrument to an *already-existing*, manually-tracked holding
account, with explicit human confirmation) is not implemented — resolution
always either reuses a connection-linked account or creates a fresh one. A
user who tracked AAPL manually before connecting Trading 212 gets a
**second**, separate AAPL holding account once Trading 212 starts trading
it, rather than an automatic/confirmed merge. Revisit if a real user hits
this. Also not implemented: the preview UI does not show a
resolved/proposed instrument name before commit (resolution is commit-time
only) — a deferred UX polish, not a correctness gap.

### B-T212-SCHED Trading 212 scheduled auto-refresh `[x]`
**File:** `docs/trading212-import-plan.md` (Slice 4a), `backend/internal/app/import_scheduler.go`.

Closed 2026-07-01 (Slice 4a). Per-connection `auto_refresh_enabled` column
(migration `0008`) drives `ImportService.StartScheduler` /
`runDueTrading212AutoRefreshes`: a once-a-minute ticker, mirroring
`PricingService.StartScheduler`, that treats a connection as due once
`last_fetched_at` is null or at least 24h old (not a fixed wall-clock time,
to avoid the book-owner-timezone plumbing `pricing_scheduler.go` needs, and
to self-correct if the server was down at whatever the "usual" time would
have been) and calls the existing `RefreshImportConnection` path — no new
fetch logic. The scheduler treats `ErrImportFetchInProgress` (the existing
Slice 3 in-flight guard) as a normal per-tick skip, so it can never
double-enqueue against a manual refresh or a still-running deep backfill.
`ImportRepository.CurrentBookOwnerID` was added (mirroring
`PricingRepository.CurrentBookOwnerID`) so a scheduled refresh has a real
owner to attribute its batch/audit trail to. Frontend: a switch on each
connection row (`routes/app/import/+page.svelte`), calling the existing
`PATCH /import-connections/{id}` with the new `auto_refresh_enabled` field.
Verified by `internal/app/import_scheduler_test.go` (due-boundary at exactly
24h, disabled connections never trigger, never-fetched triggers immediately,
in-flight guard skips silently) and a manual pass driving the real built app
+ a real (headless) browser against a local fake Trading 212 server.

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
