# Test Coverage Review — 2026-07-07

Point-in-time review of test coverage and test quality across the codebase.
Method: merged cross-package Go coverage (`go test ./... -coverpkg=./...`),
function-level coverage inspection, reading a sample of test files for
meaningfulness, and checking what CI actually executes. Companion to
`security-audit-2026-07.md`.

## Headline numbers

| Area | Volume | Tests | Coverage |
| --- | --- | --- | --- |
| Backend Go | ~38,400 LOC | ~13,400 test LOC, 361 test funcs | **64.3%** total (merged) |
| Frontend (Svelte/TS) | ~48,300 LOC | 172 test LOC in 3 vitest files | effectively untested |
| E2E (Playwright) | full app | 2 specs (auth, health) | known gap (T-20) |

Backend per-package (merged profile, statements):
`marketdata` 88%, `secretbox` 88%, `trading212` 83%, `config` 83%, `web` 73%,
`api` 52% (own-package view), `app` 34%, `db` 17% own-view — but the merged
64.3% total is the honest number, since API tests drive `app` and `db`
through the full stack.

## What is genuinely good

The tests that exist are meaningful — behavior-driven, invariant-focused, and
almost mock-free:

- **The financial core is the best-tested code in the repo.**
  `db/investments_test.go` (~30 tests) covers FIFO/LIFO/average-cost disposal
  math, cross-lot disposal, rounding proration into the final disposal,
  basis conservation, rollback on insufficient lots, and
  simulate-doesn't-mutate. `app/ledger_math_test.go` (27 tests) plus
  `quantity_precision_test.go` cover balance math.
- **API tests run the real stack**: real handler chain + real SQLite, with
  actual session cookies and CSRF tokens, asserting full lifecycles
  (create → list → register → void → restore) through the public HTTP
  contract. Payee-snapshot-after-archive style regression checks show the
  tests encode business rules, not implementation details.
- **Auth is well covered**: login state, throttling, proxy-header trust
  (`auth_login_proxy_test.go`), middleware behavior, timing-oracle dummy
  hash. `app/auth.go` sits at ~72% with the untested remainder being mostly
  error plumbing.
- Provider code (`marketdata`, `trading212` fetcher) is tested against fake
  HTTP servers including the adversarial cases (absolute `nextPagePath`
  refusal, pagination caps, cursor stops).

## Gaps

### G-01 CI does not run the frontend unit tests or e2e suite `[x]` (Vitest portion)

At review time, `.github/workflows/ci.yml` → `scripts/test-frontend.sh` ran
only `pnpm run check` (svelte-check type checking) — **`vitest` was never
executed in CI**, and the Playwright suite was not wired into CI. The three
existing Vitest files could silently rot. The cheapest high-value fix was to
add `pnpm --dir frontend test` to the frontend CI path.

Closed 2026-07-11: `scripts/test-frontend.sh` now runs both `pnpm run check`
and `pnpm run test`; the existing frontend CI job already calls this wrapper.
Playwright remains a separate, intentionally unscheduled CI decision.

### G-02 Frontend is effectively untested

172 LOC of tests against ~48,300 LOC of source. Per conventions, the frontend
does real money work — Dinero.js arithmetic, balance checks before
submission, input parsing, running totals — none of it unit-tested beyond
`statement-balance.test.ts`. The transaction form's balance validation,
amount parsing, and multi-currency display logic are the highest-value
targets.

### G-03 Import API surface near zero at the HTTP layer `[x]`

`api/import_connections.go` is **0%** — no handler test exercises the routes
that accept and update third-party API keys (auth/CSRF wiring, error mapping,
`key_hint` shaping all unverified). `api/imports.go` handlers are ~3%, and in
`app/import_service.go` the `StartImport`, `GetImportBatch`,
`PatchImportBatch`, `PreviewCommit`, `DiscardImportBatch`, and
`ListImportBatches` service methods are all 0% (T-13 tracks part of this).
Worker/scheduler paths are tested; the interactive request path is not —
that is exactly the layer where T-22 ("every import commit failed, never
caught by any test") lived.

Closed 2026-07-15: full HTTP-level lifecycle and edge-case suites added —
`api/imports_test.go` (18 tests: start/patch/preview-commit/commit/discard
lifecycle, malformed/missing file, not-found, auth/CSRF, double-commit and
discard-after-commit conflicts, cross-batch dedupe, pagination) and
`api/import_connections_test.go` (8 tests: create/list/update/rotate/delete
lifecycle, a raw-bytes scan proving the API key never appears in any
response, provider-rejection and duplicate-name error mapping, PATCH
auto-refresh omission semantics). `api/import_connections.go` 0% → 72.6%,
`api/imports.go` ~3% → 72.7%, `app/import_service.go` 51% → 82.4% (merged,
statement coverage). See `docs/plans/backend-test-coverage-plan.md` Workstreams
1–2 for the full design and the two real bugs this work found and fixed
along the way:

- A concurrent commit on the same batch for an **ordinary** (non-investment)
  row raised a raw `record commit row: ... commit identity already exists`
  error instead of resolving idempotently — the Trading 212 investment path
  already handled this race via `resolveConcurrentImportCommit`, but the
  plain-row path never got the same treatment. Fixed by routing the plain
  path's `db.ErrCommitIdentityConflict` through the same resolver. Proven by
  `TestCommitImportBatch_ConcurrentPlainRowCommitsKeepOneWinner`
  (`app/import_service_test.go`), which failed before the fix on every run.
- `PATCH /imports/{batch_id}` with a malformed `resolution` JSON string or an
  invalid `dedupe_status` value hit the `import_staged_rows` table's DB
  `CHECK` constraints and surfaced as a raw `500 INTERNAL_ERROR` instead of a
  clean `400 VALIDATION_FAILED` — client input validation errors should
  never look like server failures. Fixed by validating both fields in
  `ImportService.PatchImportBatch` before writing. Proven by
  `TestPatchImportBatch_InvalidResolutionJSONRejectedAsValidationError` and
  `TestPatchImportBatch_InvalidDedupeStatusRejectedAsValidationError`.

### G-04 Investment service orchestration is thin above the db layer `[x]`

The db-layer lot math is excellent, but in `app/investments.go` (1,940 LOC):
`PreviewSell` and `computeSellDisposals` **0%**, `ReinvestedDividend` **0%**,
cost-basis profiles, dividend defaults, automation rules, event suggestions,
and `ListRealizedGains`/`ListUnrealizedGains` all **0%**. `Sell` is 57%,
`Dividend` 62%, `Buy` 75% (mostly via import-path tests, not direct service
tests). `api/investments.go` handlers are ~13%. A sell-preview that disagrees
with the actual sell, or a wrong reinvested-dividend posting, would not be
caught today.

Closed 2026-07-15: `app/investments_service_test.go` (~40 tests) and
`api/investments_test.go` (15 tests) added — every exported
`InvestmentService` method now has a direct test, including a table-driven
preview/commit equivalence check across all four cost-basis methods (the
single highest-value test: a preview/commit divergence is a user-facing trust
bug). `app/investments.go` merged coverage 43% → 83.7%; `api/investments.go`
12.7% → 79.7%. See `docs/plans/backend-test-coverage-plan.md` Workstream 3 for the
full design.

Writing these tests surfaced two real product gaps, bigger than ordinary
missing coverage, found by reading the actual code against its documented
contract before trusting the plan's assumptions about it:

- **`PUT /investments/automation-rules` didn't replace**, despite OpenAPI
  ("Save (replace)...") and `docs/implemented.md` ("PUT replaces full set")
  both documenting replace semantics. The code only upserted the rules in the
  request — an existing active `auto_post` rule omitted from a later PUT
  stayed active untouched, able to keep auto-posting real trades unattended.
  Fixed with `db.InvestmentRepository.ReplaceAutomationRules` (one
  transaction: upsert every given rule, archive every other active rule for
  the book). Proven by
  `TestReplaceAutomationRulesArchivesOmittedRules` (`db/investments_test.go`),
  which failed on the pre-fix code on every run.
- **`POST .../accept` didn't post anything**, despite OpenAPI documenting it
  as "posts the proposed transaction" and `implemented.md` marking the whole
  suggestions feature ✅ shipped. The code only flipped the suggestion's
  `status` column — `proposed_transaction_json` was parsed nowhere in the
  codebase, `generated_transaction_id` was never written by anything, and no
  status-transition guard existed (accepting an already-accepted suggestion
  silently re-wrote the status). Fixed by defining a `dividend_income`
  proposed-transaction contract and routing acceptance through the existing
  `dividendWithPostWrite` pattern, so the transaction post and the
  suggestion's acceptance marker land in one DB transaction — no
  split-transaction crash hole (the T-06 bug class). A malformed/unsupported
  proposal or a downstream validation failure now moves the suggestion to
  `failed` with a reason instead of silently doing nothing or crashing.
  Proven by `TestAcceptSuggestion_PostsProposedDividendAndMarksAccepted` and
  four sibling failure-path tests.
  **Deeper finding:** grepping the whole codebase found **no code anywhere
  writes to `investment_provider_events` or `investment_event_suggestions`**
  — the entire "detect a corporate action → suggest it → review" pipeline
  was never built, only its schema and CRUD/read scaffold. This fix makes
  the existing endpoints correct for whatever suggestion data exists (seeded
  directly in tests today); it does not create a producer. Tracked as T-34
  in `docs/backlog.md`, deliberately out of scope for this pass — needs a
  chosen data source and its own design.

### G-05 Pricing service barely tested `[x]`

`app/pricing_scheduler.go` **0%**, `pricing_worker.go` ~17%, and in
`app/pricing.go`: `CreatePrice`, `CreateTradeImpliedPrice`, `SavePolicy`,
`SaveSourceAssignment`, `cleanPricingPolicySpec` all **0%**. The import-side
scheduler/worker pair has good tests to use as the pattern
(`import_scheduler_test.go`, `import_fetch_worker_test.go`).

Closed 2026-07-15: extended `app/pricing_test.go` (23 new tests) and added
`api/pricing_test.go` (new file, 11 tests). `app/pricing.go` merged coverage
→ 87.8%; the testable scheduler/worker units (`runScheduledRefreshIfDue`,
`runDueFXCoverage`, `processFXCoverageWork`) went from 0% to 65–79% — the
`Start*` goroutine-launching wrappers stay untested by design, same as the
import-side scheduler/worker pair they were modeled on. Includes a DST
transition test (`localDailyTimeUTC` across the 2026-03-29 Europe/Amsterdam
spring-forward) — the classic silent-drift bug in daily schedulers that
computes a fixed UTC offset once instead of recomputing per run.

Writing these tests found (not a product bug this time, just a test-design
trap worth recording): the fixture's per-currency account seeding fires a
real DB trigger (`migrations/0001`, "currency_activated") that auto-enqueues
its own `fxCoverageWorkKind` background-work item. Worker tests that count
items by status must drain or baseline around this, or they silently count
the trigger's item alongside the one the test itself enqueued.

### G-06 No race detector in CI `[x]`

At review time, `scripts/test-backend.sh` ran plain `go test ./...`. The app
runs four long-lived background goroutines (pricing + import schedulers and
workers) against shared services; `-race` in CI is cheap insurance.

Closed 2026-07-11: `scripts/test-backend.sh` now runs `go test -race ./...`,
which the backend CI job already invokes.

### G-07 No coverage signal in CI

Nothing measures or reports coverage, so regressions in test coverage are
invisible. Even a non-blocking `-coverprofile` upload (or a soft threshold on
the merged number) would make trend erosion visible.

## Overall assessment

Test *quality* is high; test *distribution* is uneven. Coverage is
concentrated where the project has historically been burned (ledger math,
lot disposal, auth, import fetch/cursor logic) and thin on newer surface:
interactive import endpoints, investment service orchestration, pricing
config, and nearly the entire frontend. The pattern matches the backlog's own
history — T-21/T-22 both escaped because the *wiring* layers between
well-tested units were untested, and that is exactly where today's 0% zones
sit.

## Recommended order of work

1. ~~**HTTP-layer tests for import connections + imports** (G-03): one
   lifecycle test each in the existing `api` test style (create connection →
   list shows `key_hint` not key → refresh → delete; start QIF import →
   preview → commit → batch status).~~ **Closed 2026-07-15** — see G-03 above.
2. ~~**Service tests for `PreviewSell` vs `Sell` consistency and
   `ReinvestedDividend` postings** (G-04) — these guard real money math.~~
   **Closed 2026-07-15** — see G-04 above.
3. ~~**Pricing scheduler/worker tests** cloned from the import-side pattern
   (G-05).~~ **Closed 2026-07-15** — see G-05 above.
4. **Frontend unit tests for money input parsing and balance validation**
   (G-02), then grow e2e per the already-tracked T-20.
