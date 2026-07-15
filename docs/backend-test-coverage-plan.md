# Backend Test Coverage Plan

**Status:** Workstreams 1, 2, and 3 complete (2026-07-15) — see the status
notes in each section below. Workstream 3 also fixed two confirmed product
gaps found while writing its tests (automation-rules replace, suggestion
accept-posts-transaction — see its status note). Workstreams 4–7 remain open.

A concrete plan to close the backend coverage gaps identified in
`docs/test-coverage-review-2026-07.md`, verified against a fresh merged
coverage run on **2026-07-15** (`go test ./... -coverpkg=./...`, merged total
**65.4%**, up from 64.3% at review time). Scope is **backend only** — frontend
(G-02) and Playwright e2e (T-20, R3a) are tracked elsewhere and deliberately
excluded here.

The plan has two goals, in this order:

1. **Meaningful tests where money and wiring are untested today** — the
   interactive import path, investment service entrypoints, import-connection
   HTTP surface, and pricing configuration. These are exactly the layers where
   T-21/T-22-class bugs escaped before.
2. **Test infrastructure that future work lands into** — R5 CSV import,
   a second provider (IBKR Flex), R13 return analytics, and I-03/I-04 should
   arrive to find harnesses, invariant suites, and fixtures already waiting.

## What changed since the 2026-07-07 review (verified)

- **App-layer import connections are now well tested** —
  `app/import_connections_test.go` has ~40 tests (secrets-never-plaintext,
  probe failures, key rotation, cash-account validation). The review's G-03
  is now **HTTP-layer only**: `api/import_connections.go` is still **0%**.
- **`PreviewSell` / `computeSellDisposals` are no longer 0%** (65.6% / 75%)
  via the F6 scale-alignment fix and its test. The rest of G-04 stands.
- **G-06 (race detector) and G-01 (frontend vitest in CI) are closed.**
  G-07 (no coverage signal in CI) is still open.

Still at or near zero, confirmed by function-level inspection:

| Surface | Coverage | Untested functions (selection) |
| --- | --- | --- |
| `api/import_connections.go` | **0%** | all 5 handlers, error mapping, `key_hint` shaping |
| `api/imports.go` | ~3% | `startImport`, `getImportBatch`, `patchImportBatch`, `previewCommitImportBatch`, `commitImportBatch`, `discardImportBatch` |
| `app/import_service.go` (interactive) | 0% each | `StartImport`, `GetImportBatch`, `PatchImportBatch`, `PreviewCommit`, `DiscardImportBatch`, `ListImportBatches` |
| `api/investments.go` | ~13% | trade/dividend mutations, gains, suggestions, rules — handlers at 3–12% |
| `app/investments.go` (service) | 43% avg | `Sell`, `Dividend`, `ReinvestedDividend` (+ its validator), `Search`, `UpdateInstrument`, cost-basis profile CRUD, dividend-default list, suggestions accept/ignore, automation rules, `ListRealizedGains`, `ListUnrealizedGains` — all **0%** |
| `app/pricing.go` (config) | — | `CreatePrice`, `CreateTradeImpliedPrice`, `SavePolicy`, `SaveSourceAssignment`, `cleanPricingPolicySpec`, `localDailyTimeUTC` — all **0%** |
| `app/pricing_scheduler.go` | **0%** | `StartScheduler`, `runScheduledRefreshIfDue` |
| `app/pricing_worker.go` | ~17% | `StartBackgroundWorker`, `runDueFXCoverage`, `processFXCoverageWork`, `backgroundRunTrigger` |

Note on reading the numbers: the exported `Sell`/`Dividend` methods are
one-line wrappers over `sell`/`dividend` (~70% via Trading 212 import-commit
tests). The gap is not "sell math is untested" — it is that **no test calls
the service the way the API handlers do**, so validation, error mapping
(`writeInvestmentServiceError` 0%), and the handler↔service contract are
unverified end to end.

## Test-design rules (what "meaningful" means here)

Inherited from the existing suite's strengths — keep these properties:

- **Test through the public contract.** API tests use the real handler chain +
  real SQLite + real session cookies/CSRF (pattern:
  `api/setup_test.go:newSetupTestHandler`, helpers in
  `api/transactions_test.go`). Service tests use a real DB, never mocks.
  Provider tests use fake HTTP servers (`onlinesource/trading212/fetcher_test.go`).
- **Assert business rules, not shapes.** A good test encodes an invariant
  ("preview equals commit", "basis is conserved", "the key never appears in
  plaintext anywhere in the DB file"), not a JSON echo.
- **Every bug class gets a named regression test** (existing convention —
  F1–F11 all have one). New tests follow
  `Test<Surface><Behavior>` naming so failures read as sentences.
- **Do not chase transformer coverage.** The `to*Response` mappers at 0% get
  covered incidentally by lifecycle tests; writing direct tests for them is
  noise. Coverage % is the map, not the goal — the exit criterion for each
  workstream is its invariant list, not a number.
- **Error paths are first-class.** The 0% zones are mostly error plumbing and
  validation; each workstream below names its rejection cases explicitly.

---

## Workstream 1 — Interactive import pipeline (highest value)

**Status: done 2026-07-15.** `api/imports_test.go` (18 tests) and
`app/import_service_test.go` (1 test) shipped. `api/imports.go` merged
coverage 72.7% (exit criterion was ≥70%, met). Two real bugs found and fixed
along the way — see `docs/test-coverage-review-2026-07.md` G-03 for the
details: a concurrent-commit race on the plain (non-investment) row path that
surfaced as a raw error instead of resolving idempotently, and a raw 500
instead of 400 on malformed PATCH resolution JSON / dedupe_status.
Deliberately **not** done, scoped out during implementation: 1c (Trading 212
interactive commit-path test) and the "wrong-book" 404 case in 1b — this app
has exactly one book (`BookID` is a package constant, not a per-request
value), so cross-book isolation isn't a reachable scenario today. An
"archived account" resolution case remains open — it hits a different
`cleanPosting` branch (`posting account is not active for postings`) than the
unknown-account case this workstream did cover
(`TestCommitImportBatch_UnknownAccountResolutionFailsRow`, which proves
`posting account is invalid`); worth a follow-up test but not blocking.

**Why first:** this is where T-22 ("every import commit failed, never caught")
lived, and both the HTTP handlers (~3%) and the six interactive service
methods (0%) are untested. One well-built lifecycle suite closes both layers
at once.

**New file:** `api/imports_test.go` (HTTP-level, real stack), plus targeted
service tests in `app/import_service_test.go` where HTTP can't reach a branch.

### 1a. QIF lifecycle through HTTP (the backbone)

One test walking the full happy path, asserting state at every step:

```
POST /imports (QIF payload) → batch created, staged rows returned
GET  /imports/{id}          → status, row counts, needs_attention flags
PATCH /imports/{id}         → resolve a row (account mapping / category)
POST /imports/{id}/preview  → previewed transactions match resolutions
POST /imports/{id}/commit   → transactions exist in ledger, batch terminal
GET  /imports               → batch listed with terminal status
```

Then the invariant that guards against T-22 recurrence: **after commit, the
ledger transactions actually exist and balance** — assert via
`GET /transactions`, not via the import response.

### 1b. Edge cases (each a separate named test)

- **Malformed QIF** → 4xx with the standard error envelope; no batch row
  persisted (no orphaned staging data).
- **Empty batch / all-rows-ignored commit** → explicit behavior pinned
  (whichever it is: reject or no-op commit).
- **Double commit** → second commit is rejected or idempotent — pin it;
  `resolveConcurrentImportCommit` (62.5%) needs its conflict branch driven:
  two commits racing on one batch (the F-series concurrency pattern).
- **Discard after commit** → rejected; **discard mid-review** → staged rows
  gone, ledger untouched.
- **Patch on a committed/discarded batch** → rejected with a stable error code.
- **Patch resolution JSON**: invalid JSON, unknown account ID, archived
  account, wrong-book account — all rejected (`parseResolutionJSON` 66.7%,
  the miss is the error path).
- **Duplicate-import dedupe**: importing the same QIF twice → second batch's
  rows flagged as duplicates, commit skips them (fingerprint path).
- **Batch ID not found / batch from another book** → 404, not 500 and not a
  cross-book leak (this is an authz test).
- **CSRF/auth**: mutating import endpoints without CSRF token → 403 (one
  table-driven test over all five mutating routes).

### 1c. Trading 212 interactive path

The worker/fetch side is tested; add the *interactive* seam: a batch produced
by a (seeded) T212 fetch → `GET` shows investment rows → commit routes
through `Buy`/`Sell`/`Dividend` → lots exist. Reuses the fake-provider harness
from Workstream 6.

**Exit criteria:** `api/imports.go` ≥ 70%; the six interactive service
methods each exercised through HTTP; every edge case above has a named test.

---

## Workstream 2 — Import connections HTTP surface ("API connections")

**Status: done 2026-07-15.** `api/import_connections_test.go` (8 tests)
shipped. `api/import_connections.go` merged coverage 72.6% (exit criterion
was ≥80%, not quite met — the gap is mostly the refresh-when-fetch-in-progress
and secret-key-not-configured branches, which need the background-worker
fixture from `import_fetch_worker_test.go` to reach cleanly; left for a
follow-up rather than forcing it into this pass). The raw-bytes secret-leak
test, duplicate-name/provider-rejection error mapping, and the PATCH
auto-refresh omission test (this repo's recurring "plain bool overwrites an
omitted field" bug class) are all in place. "Wrong-book invisible" was
dropped for the same single-book reason as Workstream 1.

**Why:** `api/import_connections.go` is the last **0% file** that handles
secrets. The service layer is thoroughly tested, so this workstream is thin
by design — it verifies the *wiring*, not re-tests the service.

**New file:** `api/import_connections_test.go`.

- **Lifecycle:** create (T212 key) → list → update (rename) → rotate key →
  refresh trigger → delete. Provider probe faked with the existing fake-T212
  server.
- **The security-critical response shape:** `key_hint` present, **full key
  absent from every response body** — assert on raw response bytes, not
  parsed fields, so a renamed field can't sneak the secret through. Also
  assert the key never appears in the create/update *request echo* or error
  messages (probe-failure error must not include the submitted key).
- **Error mapping** (`writeImportConnectionServiceError`, 0%): duplicate name
  → 409-class envelope; validation failure → 400; probe failure → the
  documented error code; not-found and wrong-book ID → 404.
- **Refresh endpoint:** refresh on a connection with `auto_refresh` off still
  works (manual trigger); refresh on missing connection → 404; refresh
  enqueues exactly one fetch (idempotent enqueue is already service-tested —
  here just assert the HTTP contract and status code).
- **CSRF/auth** on all mutating routes; connections of another book invisible.

**Exit criteria:** file ≥ 80%; the "secret never in any HTTP response" test
exists and scans raw bytes.

---

## Workstream 3 — Investment service + HTTP (current functionality)

**Status: done 2026-07-15.** `app/investments_service_test.go` (new file, ~40
tests covering 3a–3c) and `api/investments_test.go` (new file, 15 tests
covering 3d) shipped. `app/investments.go` merged coverage 43% → 83.7%;
`api/investments.go` 12.7% → 79.7% (both well past the ≥60%/"every exported
method" exit criteria). Gains math itself was already hand-verified across
multiple currencies at the DB layer (`db/investments_test.go`'s
`ListRealizedGains`/`PositionsWithGains` suites); the app-layer tests here
prove the thin service wrapper passes results through correctly (nil-vs-value
shape, sign, date-filter inclusivity) rather than re-deriving that arithmetic.

Reading the real code before writing tests surfaced two confirmed gaps
between documented and actual behavior — bigger than ordinary missing
coverage, so they were fixed first (see
`docs/test-coverage-review-2026-07.md` G-04 for the full writeup, and
`docs/backlog.T-34` for what's still open):

- **`PUT /investments/automation-rules` didn't replace** — it only upserted
  the rules present in the request; an existing active `auto_post` rule
  omitted from a later PUT stayed active untouched, able to post real trades
  unattended forever. Fixed with a new `db.InvestmentRepository.ReplaceAutomationRules`
  (one transaction, archives every active rule not in the new set) and a
  regression test that failed on the old code
  (`TestReplaceAutomationRulesArchivesOmittedRules`, `db/investments_test.go`).
- **`POST .../accept` didn't post anything** — it only flipped the
  suggestion's status column; `proposed_transaction_json` was parsed nowhere
  and no status-transition guard existed. Fixed by defining a
  `dividend_income` proposed-transaction contract and routing acceptance
  through the existing `dividendWithPostWrite` pattern (transaction + status
  update in one DB transaction — no split-transaction crash hole), with
  malformed/unsupported proposals and downstream validation failures moving
  the suggestion to `failed` with a reason instead of surfacing as an API
  error. Proven by `TestAcceptSuggestion_PostsProposedDividendAndMarksAccepted`
  and its sibling failure-path tests in `app/investments_service_test.go`.
  Digging further revealed **no code anywhere produces
  `investment_provider_events`/suggestions** — this fix makes the existing
  endpoints correct for whatever data exists, but nothing generates that data
  yet (T-34, deliberately out of scope here: needs a chosen data source and
  its own design).

Also corrected during this pass: the plan's original assumption that
`SaveAutomationRules` already had "replace-all semantics" and that "accept
posts the proposed transaction and flips status" were both wrong — the code
review above superseded them before any test was written against the
incorrect assumption.

**Why:** real-money surface. The lot math below it is excellent; the service
orchestration and handler layer above it are the gap (G-04). Split into
service-level tests (fast, precise) and one HTTP lifecycle test (wiring).

**Files:** extend `app/investments_test.go`; new `api/investments_test.go`.

### 3a. Trade entrypoints (service level)

- **`Sell` direct**: seeded buys → `Sell` → assert the four-leg transaction
  shape, disposal records, realized gain — *called as the handler calls it*,
  not via the import path.
- **Preview/commit equivalence (the invariant from `investments-plan.md`):**
  for each cost-basis method (fifo, lifo, average_cost, specific_lot):
  `PreviewSell(input)` then `Sell(input)` → allocations, disposed basis, and
  realized gain **identical**. Table-driven over methods. This is the single
  most important new test in the plan — a preview/commit divergence is a
  user-facing trust bug that nothing currently catches.
- **`ReinvestedDividend` (0% including its validator):**
  - happy path: income posting + new lot with basis = reinvested amount,
    implied price rounding half-up (assert exact minor units);
  - `validateReinvestedDividendInput` rejections: zero/negative quantity,
    missing instrument/account, bad date, mismatched commodity;
  - dividend-default application (withholding account resolution).
- **`Dividend` direct**: with and without a dividend default; withholding leg
  present only when configured.
- **Validation edges shared by trades** (`validateTradeInput` 55%,
  `validateDividendInput` 58% — drive the missing branches): zero quantity,
  negative price, unknown holding account, instrument/account commodity
  mismatch, quantity scale exceeding the account override.

### 3b. Configuration CRUD (service level, all 0%)

- **Cost-basis profiles**: save → list → update; `cleanCostBasisProfileSpec`
  rejections (unknown method, blank name); default-profile semantics
  (exactly one default per book survives concurrent saves).
- **Dividend defaults**: `ListDividendDefaults` returns saved defaults;
  update-in-place; validation rejections.
- **Automation rules** (`SaveAutomationRules` replace-all semantics): save 2
  rules → save a list with 1 → only 1 remains (pin the replace-all
  contract); `cleanAutomationRuleSpec` rejections (bad event family, bad
  mode, `effective_to` before `effective_from`, confidence bps out of range);
  archived rules excluded from the active list.
- **Suggestions** (`AcceptSuggestion`/`IgnoreSuggestion`/`setSuggestionStatus`):
  accept posts the proposed transaction and flips status; ignore flips status
  without posting; **accepting an already-accepted/ignored suggestion is
  rejected** (status-transition guard); accepting a suggestion whose
  proposed transaction no longer validates → `failed` status, not a crash.
- **`Search` / `UpdateInstrument` / `ListProviderEvents`**: one behavior test
  each (search matching + book isolation; update creates the expected
  version; provider events listed newest-first).

### 3c. Gains read models (0%, feeds R13 later)

- `ListRealizedGains`: seeded buy→sell with gain > 0 and a second with loss →
  correct sign and exact minor units (hand-computed); `from`/`to` filters
  **inclusive on both ends** (boundary-date test); multi-currency: two cost
  commodities don't sum together.
- `ListUnrealizedGains`: position with a price → exact gain; position
  **without** a price → nil gain/market-value fields (never zero — the
  documented degrade-gracefully rule); stale price still reported with its
  date.
- Cross-check invariant: for a fully-closed position,
  `Σ realized gain == Σ proceeds − Σ original basis` to the minor unit.

### 3d. HTTP layer (`api/investments_test.go`)

One lifecycle test in the established style: create instrument → holding
account → buy → preview sell → sell → dividend → lots/positions/gains all
reflect it — plus the error-envelope mapping (`writeInvestmentServiceError`,
0%): `ErrInsufficientLots` → the documented 4xx code (assert the exact error
code string the frontend switches on), validation → 400, foreign IDs → 404,
CSRF on all mutations. Handler-level parsing edges: non-integer path ID
(`readPathInt64`), malformed JSON body.

**Exit criteria:** every exported `InvestmentService` method has ≥ 1 direct
test; preview/commit equivalence table exists; `api/investments.go` ≥ 60%;
gains math hand-verified in at least two currencies.

---

## Workstream 4 — Pricing configuration, scheduler, worker

**Why:** pricing feeds unrealized gains and FX-converted reports; config
writes are 0% and the scheduler/worker pair has no equivalent of the good
import-side tests. Clone the proven patterns from
`import_scheduler_test.go` / `import_fetch_worker_test.go`.

**Files:** extend `app/pricing_test.go`; add `api/pricing_test.go` for the
handler gaps (file at ~30%).

- **`CreatePrice`** (manual observation): happy path; duplicate (same source,
  pair, date) behavior pinned (upsert vs reject); bad scale/negative price
  rejected; `cleanPriceObservationSpec` already tested — don't duplicate it.
- **`CreateTradeImpliedPrice`**: a buy produces the implied observation at
  the documented half-up rounding; idempotent per transaction (re-posting the
  same trade doesn't duplicate the observation).
- **`SavePolicy` + `cleanPricingPolicySpec`**: valid policy round-trips;
  rejections: unknown cadence, bad local time, bad timezone name;
  **`localDailyTimeUTC` (0%)**: table-driven across timezones **including a
  DST transition day** (Europe/Amsterdam spring-forward) — this is the
  classic silent-drift bug in daily schedulers.
- **`SaveSourceAssignment`**: assign source to pair; reassignment replaces;
  unknown source rejected; `sourceAssignmentOperation` audit op recorded.
- **Scheduler (`runScheduledRefreshIfDue`, 0%)**: with `SetNowForTest` —
  not-yet-due → no run; due → exactly one run enqueued; still-due-but-
  already-ran-today → no second run; policy disabled → nothing. Same
  structure as `TestImportScheduler*`.
- **Worker (`processFXCoverageWork`, `runDueFXCoverage`, 0%)**: leased work
  processed exactly once; provider failure → `failFXCoverageRun` (0%) records
  the failure and the lease releases for retry with backoff (`retryDelay`
  exists — assert it's honored); poison item doesn't wedge the queue.
  `TestBackgroundWorkExpiredLeaseCanBeReclaimed` already covers lease expiry —
  extend, don't re-test.
- **HTTP** (`api/pricing.go` handlers at 4–12%): one policy round-trip test,
  one price-create test, error mapping (`writePricingServiceError`, 0%).

**Exit criteria:** scheduler/worker parity with the import-side suite; DST
test exists; `app/pricing.go` config methods all non-zero with rejection
cases.

---

## Workstream 5 — Financial-core edge-case hardening (invariant suite)

**Why:** the lot math is well tested example-by-example; this workstream
makes the invariants *reusable* so current code is hardened and future
methods (I-03 analytical reporting, I-04 gain postings, R13 analytics) get
checked for free.

**New file:** `db/investments_invariants_test.go`.

Extract the invariants already latent in `db/investments_test.go` into
assertion helpers, then run them across a table of scenarios × all four
cost-basis methods:

1. **Basis conservation:** `Σ disposed_basis + Σ remaining_basis ==
   Σ acquired_basis` exactly, after any sequence of buys/sells.
2. **No negative remainders:** every lot's remaining quantity and basis ≥ 0;
   closed ⇔ remaining quantity == 0 ⇔ remaining basis == 0.
3. **Residual determinism:** repeating the identical disposal on an identical
   seed yields byte-identical lot events (guards the truncation-residual
   assignment rules from `investments-plan.md` §average-cost).
4. **Method actually matters:** FIFO vs LIFO vs average-cost over the same
   3-lot seed produce three *different* disposed-basis totals (guards
   against a regression to silent-FIFO — the original I-02 bug class).
5. **Oversell always fails atomically:** `ErrInsufficientLots` and *zero*
   mutation (assert via full lot-table snapshot compare, not spot checks).
6. **Mismatched quantity scales fail loudly** under average-cost (already
   specified in the plan; make sure the test exists for the *service* path,
   not just `disposeLotTx`).

Scenario table to run them over: single lot exact close; partial across 3
lots; 1-minor-unit quantities (max truncation stress); large-value lots near
int64 range via `exact.Coefficient`; interleaved buy/sell/buy/sell; sale in a
different day order than lot `opened_on` (day-sequence interaction).

Deliberately example-based with adversarial values, not a property-testing
framework — keeps the suite deterministic and greppable, per house style.

**Exit criteria:** helper assertions exist and are exported for reuse by
future method implementations; scenario × method matrix green.

---

## Workstream 6 — Infrastructure for future functionality

Small, targeted scaffolding so the roadmap's next items start tested. Build
each **alongside its first consumer** in Workstreams 1–4 — no speculative
abstraction beyond what those tests already need.

- **6a. Statement-parser golden-file harness** (for R5 CSV, later R6
  OFX/QFX/XLSX). Generalize the QIF test approach: a directory of input
  fixtures + expected staged-row JSON, one table-driven runner. Adding a
  parser or a bank quirk = dropping in two files. Include today's QIF
  adversarial fixtures (date formats via `parseQIFDate`, decimal comma vs
  point via `parseDecimalAmount`, splits) so the harness proves itself on
  the existing parser before CSV lands.
- **6b. Provider-adapter contract suite** (for the second provider, likely
  IBKR Flex). Extract the T212 fake-server patterns into a reusable
  `providertest` helper asserting the contract every adapter must honor:
  pagination cursor termination, absolute-URL refusal, rate-limit/backoff
  handling, dedupe on refetch, secret never logged. When IBKR arrives, its
  adapter runs the same suite plus provider-specific fixtures.
- **6c. Gains fixtures for R13.** The Workstream 3c seeds (multi-lot,
  multi-currency, gain + loss, open + closed positions) live in exported
  seed helpers so TWR/MWR/allocation tests reuse the same books —
  return-analytics results can then be cross-checked against the already-
  verified realized/unrealized figures.
- **6d. Draft-workflow seams (R9).** No tests yet (feature doesn't exist),
  but note: the transaction-lifecycle taxonomy tests around draft/posted are
  the acceptance harness R9 plugs into; keep them invariant-shaped.

---

## Workstream 7 — CI coverage signal (closes G-07)

The last open CI item from the review. Minimal, non-flaky version:

- `scripts/test-backend.sh`: add a second mode (env var, e.g.
  `COVERAGE=1`) running `go test ./... -coverpkg=./... -coverprofile=...`
  and printing the merged total via `go tool cover -func | tail -1`. Keep the
  default `-race` run unchanged (race and coverage runs stay separate — a
  combined run is slow and muddies both signals).
- CI: run the coverage mode as a **non-blocking** step that uploads the
  profile artifact and echoes the total into the job summary. After the
  workstreams above land, add a **soft floor** (fail if merged total drops
  below a value set ~2 points under the then-current number) — a tripwire
  against erosion, not a target to game.

---

## Sequencing and effort

Order matches risk: untested wiring around money first, config second,
hardening third, meta-work last. Estimates are for focused sessions
including debugging.

| # | Workstream | Est. | Depends on | Status |
| --- | --- | --- | --- | --- |
| 1 | Interactive import pipeline (W1) | 2–3 sessions | — | **done 2026-07-15** |
| 2 | Import connections HTTP (W2) | 1 session | — | **done 2026-07-15** |
| 3 | Investment service + HTTP (W3) | 3–4 sessions | — | **done 2026-07-15** |
| 4 | Pricing config/scheduler/worker (W4) | 2 sessions | — | open |
| 5 | Financial-core invariant suite (W5) | 1–2 sessions | helps to have W3 seeds | open |
| 6 | Future-proofing harnesses (W6) | folded into W1/W3/W4 | its consumers | partial (6a/6b/6c not started) |
| 7 | CI coverage signal (W7) | ½ session | best after W1–W4 for the floor | open |

W1–W4 are independent and can be interleaved; each should land as its own
commit(s) with the doc updates per `validate-and-ship`. Expected merged
coverage after W1–W4: roughly **75–80%**, with the remaining gap being
transformers, `main` wiring, and deliberate exclusions — at which point the
W7 floor makes the level self-defending.

## Non-goals

- Frontend unit tests (G-02) and Playwright growth (T-20/R3a) — separate
  track, unchanged priority per the review.
- Coverage on `to*` transformers, `cmd/rekenraam`, or generated code as an
  end in itself.
- A property-based-testing dependency; W5 gets the same value with
  deterministic adversarial tables.
- Testing `secretbox`/`marketdata`/`exact` further — already at healthy
  levels with meaningful tests.
