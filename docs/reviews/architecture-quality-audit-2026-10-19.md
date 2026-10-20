# Architecture, API contracts, test quality and performance audit — 2026-10-19

**Status:** [#280](https://github.com/sergeyfarin/rekenraam/issues/280), area 12
of [#260](https://github.com/sergeyfarin/rekenraam/issues/260).
**Review, measurements and all four local shared gates complete.** Following
[#344](https://github.com/sergeyfarin/rekenraam/issues/344), the green final-gate
SHA is `af233abb82fe88e0b30c1274eb698d280069820f`. The earlier failed attempt remains below as historical evidence;
see the [repair and final validation](subview-shared-book-fixes-2026-10-19.md).
Publication / main CI evidence is recorded on #280 and #344. This does not
certify release/household acceptance.

## Scope and audited revisions

Clean checkout of baseline `ee23e45509f8b90da4079fd51f8873f56963f795`.
The initial application/performance review used `ee23e455`. Bounded fixes are
`7b7af72b` (#345), `c64886a4` (#346), `a5b42fc9` (#347) and `91563e00` (#345
presentation); `dea2c418` adds the opt-in representative-read measurement.
**Initial failed gate candidate: `25a24f4ed5dff81528535fb6b238c37dbd2345bb`.**
The first shared-gate attempt used `91563e00`; its full browser selection exposed
#352. The separate #351 checker repair landed as `0a891aa5` with before/after
controls; #352 then moved the unchanged FX-selection journeys to their own
worker book/project in `25a24f4e`. The full browser retest and shared wrappers
are recorded at that final revision below. Git tree hashes confirm that
`backend`, `frontend`, `api` and `pnpm-lock.yaml` are unchanged between
`91563e00` and `25a24f4e`; the follow-ups affect test infrastructure. Later
evidence/status changes through `7e1ffa6b` are docs only. The subsequent #344
repair changes frontend layout and browser fixtures, as recorded below.
No changes were made to financial rules,
database schema, provider admission or saved-output contracts.

All preceding #266–#279 audit dependencies have published evidence and landed
their integrity fixes. Existing #343 (P3 API/internal blank UTC-day defaults),
#326 (P3 URL state), #102 (human native
language review), #336 (owner deployment/household evidence) and #337 (runtime
image advisory disposition) retain their own acceptance. Their open status does
not imply that this review repeated those audits. #284 owns refactoring decisions.

All books, owners and provider responses used here are synthetic. Browser workers
use separate SQLite files and app processes; provider URLs point to the existing
`MARKET_DATA_BASE_URL`/Trading 212 stub. No live credentials, owner book or new
provider were required (ADR 0017).

## Findings and disposition

| Finding | Independent expectation and actual evidence | Severity / disposition |
|---|---|---|
| #345: request-body error codes absent from OpenAPI | A real oversized setup POST must return a representable error envelope and create no owner. `TestOversizedJSONRequestUsesDocumentedError` gets HTTP 413 `REQUEST_TOO_LARGE`, then checks its presence in the contract; the assertion fails against the original enum and passes with the fix. Literal inventory also found the decoder fallback `DECODE_ERROR` undeclared. | P3, **confirmed defect, fixed**. Declare both existing codes, regenerate types, map decode to existing validation guidance and add size-reduction guidance in all six catalogs. No runtime refusal semantics changed. Native wording sign-off remains #102. |
| #346: two workspace package-manager pins | Root install and the browser wrapper should use the same exact manager. Root pins 12.11.2; e2e pinned 11.20.0. | P3, **confirmed tooling drift, fixed**. e2e now uses the root exact pin; frozen install and `pnpm --dir e2e --version` use 12.11.2. |
| #347: structural gate rejects legitimate browser setup | CI and release-preflight each independently need Chromium. The checker previously reported their identical install command as duplicate work. | P3, **confirmed false positive, fixed**. Exempt only that exact setup command. A temporary separately named workflow repeating `./scripts/test-backend.sh` still produces exit 1; the ordinary workflows produce exit 0. |
| #351: duplicate scanner misses inline YAML steps | A valid temporary `- run: ./scripts/test-backend.sh` workflow must be detected just like a named step. Before the fix the checker returned 0; after normalizing both spellings it returns 1 with the duplicate diagnostic. Normal workflows remain green. | P3, **confirmed checker defect, fixed** in `0a891aa5`. This is a separate finding from #347’s setup false positive. |
| #348: nested-dialog contrast override has an unsound oracle | Text contrast must concern text and its background. An actual Chromium negative control used nested dialogs with `#999` paragraph text, white background and a black 2px border. Axe found one color-contrast violation at **2.84:1**. Invoking the unchanged `withRenderedContrast` function from the sweep returned zero retained findings and reported **21.00:1**, the border contrast. | P2, **confirmed test-evidence defect, open**. This does not prove an app dialog fails contrast. Replacing the override requires rendered text/background evidence and rechecking the affected dialog cases; simply dropping it revives the disputed axe readings. #331's pixel-override rows are not accepted contrast certification. |
| #350: HTTP 413 status absent from JSON operation contracts | The same real setup POST returns 413, while `paths/setup-owner.yaml` declares only 201/400/403/409/500. Static inventory finds 213 JSON request operations without 413; each needs mapping to capped decoder callers before a blanket change. | P3, **confirmed contract gap, open**. #345 repairs the code enum only. #350 owns public/authenticated/optional-body status responses and accurate shared 1 MiB versus multipart limits; #349 includes the matrix. Runtime refusal is correct. |
| #352: FX-selection test assumes exclusive built-in pair history | The full gate at `91563e00` failed `observation-selection.spec.ts:54`: after withdrawing its current-day rates it expected “Only future prices available”, but the report correctly retained USD 100.00 with “No compatible price within the allowed age”. Read-only inspection of its disposable `app-1` book found prior live USD/EUR manual observations dated January 31 and March 31, older than its October 19 boundary. | P2, **confirmed test-isolation defect, fixed** in `25a24f4e` and full-suite validated at `af233abb`: isolate the spec in a dedicated self-managed Playwright project, keeping every assertion. An explicit external `E2E_BASE_URL` remains a shared operator book and gains no isolation. Focused/final validation recorded below. |
| #344: existing full-suite sweep isolation/fit failures | A sweep must target its own created operations and fit real long data. Its `seedFixture` reads only one investment transaction page from the worker's shared book, then groups all those rows by date; its lot opener selects the first unrelated position. Earlier CI/full-run evidence is recorded on #344. | P2, **confirmed existing defect, subsequently fixed by #344**. The following describes the initial failed gate; the repair and green candidate are recorded below. Eight focused configurations passed on a fresh book; a second 26-journey run (12 upload-limit, 6 share-exchange, 8 limited sweep configurations) also passed on one shared worker. Neither result retires the full-suite failure. The corrected full run reproduces all three classes: 11/13 unopened views from missing/wrong IDs, 32 px overflow in five import states, and 2 px lot-panel overflow. See the actual final counts/matrix below. |

Each new finding is a separate bounded GitHub issue (#345–#348 and #350–#352), including those
fixed here. Additional audit **#349** covers runtime response-schema conformance;
route parity and generation do not establish all requiredness/nullability cases.
No refactor issues were filed under this audit.

## Evidence by delivery-scope bullet

Unless separately stated, static review refers to baseline `ee23e455` plus the
bounded candidate fixes; final executable evidence refers to the candidate above.

| Scenario | Independent expected result / consumer and command | Actual result, limitation and disposition |
|---|---|---|
| Handler/service/repository boundaries | HTTP owns decode/auth/envelopes; services own financial policy; repositories own SQL/atomic persistence. Inspected `api/health.go`, `server.go`, transaction/investment error handlers; `app/transactions_validate.go`, `transactions_write.go`, investment correction/replay and report services; repository writers and snapshot reads. Greps for SQL calls/float parsing in API and SQL calls in app. | No SQL execution or financial float parsing found in handlers; services orchestrate commands. **The literal “repositories hold no policy” target is not met:** `db/investments.go` computes FIFO/LIFO/average-cost depletion, `investment_transfer_internal.go:InternalTransferAllocation` and `internalTransferPolicyTx` select allocation/method/lineage, and `investment_dated_valuation.go` applies valuation/knowledge rules. These exceed persistence guards. **Accepted/deferred structural limit**, recorded for #284 under #280’s explicit maintainability-only instruction; no refactor or refactor issue is authorized here. Named financial consumer tests remain the correctness evidence; layering alone is not certified. |
| Composed page reads / frontend fan-out | Page/row rendering must not request once per item. Reviewed query construction and loops throughout `frontend/src/lib` and app routes, especially transaction workspace, account register, investments, recurring, reports, import and pricing. | Positions/lots fetch once for the list/selected detail; register uses its composed rows; report APIs return totals/coverage. Recurring bulk-post review does fetch each selected draft's impact, but only after explicit action, not to render each due row. Import row writes likewise are commands, not page-data queries. No per-rendered-row query loop found. **Verified within static sweep**; no request-count upper-bound proof for every interactive state. |
| Exact quantities, monetary ranges and overflow | Canonical coefficient strings; scale ≤24 for crypto, ≤12 otherwise; no float money or SQL `SUM` of quantity text. Named controls include `TestSpendingAndNetWorthOverflowAreA422`, `TestPriceCoefficientsRemainExactAboveBrowserSafeInteger`, `TestUnknownNonSaleAPIKeepsExactFactsAndNullResults`, exact coefficient/scaled tests and DB coefficient constraints. | API strings/`moneyCoefficient`, `exact.Coefficient`/`ScaledInt`, checked monetary result boundaries and SQL legality constraints are retained. Only two explicit app/db float conversions found: backup disk-space estimate and retry exponent; neither computes money. No quantity-text SQL aggregate found. **Verified for inventory and named controls**, no new financial defect. |
| OpenAPI ↔ handlers ↔ generated client ↔ Bruno | Normalize placeholder names and compare registered method/path sets with every root path reference; check Bruno URLs/methods; regenerate the client without drift. | **329/329** method/path pairs match both ways; **26** Bruno requests name declared operations. The 90 literal API error-code inventory found #345; after #345 all literal codes are declared. Operation-level HTTP 413 coverage remains #350. Generation is stable. **Verified for route/code parity and sampled real consumers**. Bruno was inspected as a manual request collection, not executed as 329 endpoint acceptance. Runtime response requiredness/nullability is **not comprehensively verified**; additional audit #349 owns it. |
| Independent expectations | `fixtures/reference-book.json` has 62 public-API steps and a hand-derived register; `TestReferenceBookProducesItsRegister` checks balances, reconciliation, reports, budgets, lots, gains and knowledge. Review `exactValue`: expected decimal strings are reduced with standard-library `big.Rat`, not the application calculator. | Existing named consumer/oracle checks are reused; the performance probe asserts that register before timing. Conservation tests compare journals/immutable facts with projections, supplementing rather than replacing known figures. Some tests intentionally compare two projections to prove parity, and fixtures use production writers to create realistic evidence; these are not described as independent financial oracles. **Verified for reference register and sampled property tests**. |
| Vacuous assertions / test oracle quality | An absent-control assertion needs evidence the intended view/data was reached. Swept `toHaveCount(0)`, expected-value helpers and screenshot overrides. | Reviewed zero-count assertions generally accompany visible tables, explicit record/transaction controls or before/after lifecycle assertions (`reports.spec.ts`, `transaction-row-actions.spec.ts`, reorder/short/provider tests). No mechanical claim that every negative assertion is independent. #344 exposes fixture targeting weakness; #348's real negative control confirms an unsound contrast exception. **Confirmed defects / explicit retained limits**, not blanket test certification. |
| Race isolation / deterministic stubs | Real temp-file DBs; per-worker app instances; race gate on unchanged assertions; provider-free requests. | `internal/testdb` copies migrated templates, and app/db test parallelism is structurally checked. Worker DBs isolate processes, but specs within a worker share state (#344). Stubs key accounts by synthetic API key and unknown market-data paths return 404. Native SQLite's C code is outside Go race instrumentation. **Verified mechanisms and final gate below**, not proof against every logical SQL interleaving. Prior #267 evidence remains relevant. |
| CI/local equivalence | Shared wrappers must be actual CI commands with the same tags/deadlines. Inspect `ci.yml`, `large-book.yml`, release-preflight and test scripts. | Backend race and separate coverage use the same wrapper; Node 24/frozen pnpm install, frontend wrapper and integrated build match. Push CI uses `test-e2e-smoke.sh`, deliberately excluding release-preflight; this audit runs **full** `test-e2e.sh`, including it. Large-book is weekly/manual. #346/#347 corrected tooling drift. No coverage percentage or old timing is presented as fresh evidence. **Verified configuration**; publication/remote-check disposition is recorded below and on #280. |
| Performance | `scripts/measure-large-book.sh`; opt-in `TestReferenceBookReadPerformanceAudit` through the HTTP handler after independent register assertions. | Fresh measurements and machine context below. **Verified measured workloads**, not a latency SLA, browser-network benchmark, investment-heavy large-book test or owner-machine guarantee. |

## Performance evidence

Machine: Ubuntu 24.04.5 LTS, Linux 6.8.0-147-generic, x86_64 virtual machine,
AMD EPYC 3251 reported by the host, 10 visible logical CPUs, 19 GiB RAM and 8 GiB
swap. Node 24.21.0, pnpm 12.11.2; the project selects Go 1.27.2 through `go.mod`
(the launcher is Go 1.27.0). Native bundled SQLite, CGO, `fts5,netgo,osusergo`,
FULL durability. The representative measurement was a standalone opt-in Go test process; other
validation jobs shared the machine; results are wall
measurements, not isolated CPU microbenchmarks. No fixed CPU cap was introduced.

Large book at baseline `ee23e455`:

```sh
GOCACHE=/tmp/rekenraam-go-build-cache ./scripts/measure-large-book.sh
```

125,000 transactions, **250,000 postings**, 3,020 accounts, USD and EUR over
five years. The bundle manifest must contain 250,000 ledger rows, 3,020 accounts
and 6,000 trial-balance rows, and self-check must report passed. All those
independent size/status assertions passed.

| Read | Wall | Allocated total | Sampled live-heap increase | Existing ceilings |
|---|---:|---:|---:|---|
| Bundle export | 14.917 s | 433 MiB | 20 MiB | <60 s, <256 MiB heap |
| Whole-book self-check | 15.214 s | 137 MiB | 39 MiB | <60 s, <256 MiB heap |

Total test including generation: 46.68 s. Allocated bytes are cumulative churn,
not simultaneously live memory. The sampled heap delta is above the pre-read
heap after GC, not total process RSS; SQLite C allocations and the OS file
cache are outside Go heap. These are not total process-memory ceilings, and
2 ms sampling can miss shorter peaks.
The candidate changes no runtime export/self-check implementation or fixture.
This run is explicitly baseline performance evidence, not relabelled as a later
SHA execution.

Representative provider-free #266 book measured directly at `91563e00`
(test introduced in `dea2c418`; identical app/API trees at `25a24f4e`):

```sh
GOCACHE=/tmp/rekenraam-go-build-cache REKENRAAM_AUDIT_READS=1 \
  ./scripts/go-backend.sh test -count=1 -v \
  -run '^TestReferenceBookReadPerformanceAudit$' ./internal/api
```

Each endpoint receives one warm-up read and **20 measured real HTTP-handler
reads**, including JSON encoding. The timer excludes seed/register assertions
and per-endpoint initial GC. This is an in-process consumer; no TCP/TLS/Chromium
cost is included. Heap deltas cover the 20-read batch, sampled every 1 ms;
allocations are per read. Forecast uses the fixture's injected clock and its
supported default date, rather than an invented `as_of_date` query parameter.

| Main read | Mean wall/read | Allocated/read (B) | Sampled heap delta (B) |
|---|---:|---:|---:|
| Transactions, first 50 | 19.949 ms | 658,784 | 3,489,696 |
| Account balances, 2026-03-31 | 1.772 ms | 193,558 | 2,506,968 |
| Net-worth monthly series | 4.361 ms | 152,093 | 2,508,064 |
| Category spending | 1.221 ms | 45,442 | 802,024 |
| Cashflow monthly series | 2.017 ms | 96,044 | 1,582,040 |
| Investment positions | 7.933 ms | 48,059 | 682,752 |
| Operational investment gains | 174.570 ms | 354,157 | 1,552,144 |
| February budget | 4.373 ms | 69,124 | 1,080,928 |
| 30-day forecast | 9.828 ms | 1,141,257 | 3,827,408 |

The preliminary run before the shared candidate recording averaged 91.633 ms for
gains; the directly recorded candidate run above, alongside browser validation
and backend vet/build work, averaged 174.570 ms. The gains code is unchanged. The slower timing is consistent with concurrent
machine load; these two runs do not establish its cause or a code regression.

The gains read includes effective-history replay and rolls back temporary work;
its higher latency is recorded without inferring a regression or silently
setting a new performance requirement. The large generated book is cash-ledger
work, not evidence for large investment-history gains/alternative projections.

## Initial shared gates — failed candidate `25a24f4e`

| Gate at `25a24f4e` | Actual result |
|---|---|
| `GOCACHE=/tmp/rekenraam-go-build-cache ./scripts/test-backend.sh` | **Passed**: formatting, vet and every package under `-race -p 2 -timeout=5m`. Go reused valid package results for unchanged inputs; the initial uncached attempt also passed, including app 276.942 s, API 145.270 s and DB 185.545 s. Native C is outside the race detector. |
| `./scripts/test-frontend.sh` | **Passed**: zero Svelte errors/warnings, 66 catalog cases, 54 unit files / 691 unit cases; generation leaves no diff. |
| `./scripts/check-post-merge.sh ee23e455` | **Passed**, including numbered-file uniqueness, generated-client stability, docs/workflows and no test loss relative to the audited baseline. Direct/named duplicate command controls fail as expected. |
| Full `./scripts/test-e2e.sh` selection | **FAILED: 346 passed, 6 failed, 352 selected (22.5 minutes)**. Every failed test is a sub-view sweep configuration under #344; no retries, interruptions or other failed journeys in this final run. Both isolated FX-selection journeys and all five release-preflight cases pass. #280 remains open, blocked by #344. |

The first browser run at `91563e00` was stopped after diagnosis of #352:
310 passed, 1 failed, 3 interrupted, 38 not yet run (17.8 minutes). It is not a
passing final gate. The corrected run uses a disposable local checkout at
`25a24f4e`, three workers, ports from 17400 and a separate stub at 17490:

```sh
E2E_SKIP_BUILD=1 E2E_WORKERS=3 E2E_PORT=17400 TRADING212_STUB_PORT=17490 \
  ./scripts/test-e2e.sh
```

It reuses the integrated binary already freshly built from the identical app
and frontend source trees. The disposable checkout links the original workspace's
already frozen dependencies; pnpm warns that its attempted relocation install
cannot replace the hoist-directory symlink, then runs the tests. This is not a
second clean-install claim. The original workspace's frozen install passed at
the aligned 12.11.2 pin, and all manifests/lockfile and runner versions are
identical. No test, contrast threshold, race option or timeout was reduced.


The eight completed sub-view matrices distinguish a view that could not open
from a view whose rendered geometry failed. Each has five explicitly unreachable
branches retained from #331. An “axe” zero below does not certify #348’s disputed
nested-dialog pixel-override rows.

| Configuration | Could not open (fixture IDs) | Other axe failures | Fit failures | Close failures |
|---|---:|---:|---:|---:|
| dark-390 | 0 | 0 | 5 | 0 |
| dark-desktop | 13 | 0 | 0 | 0 |
| de-390 | 0 | 0 | 5 | 0 |
| de-desktop | 0 | 0 | 0 | 0 |
| light-390 | 13 | 0 | 0 | 0 |
| light-desktop | 11 | 0 | 0 | 0 |
| ru-390 | 0 | 0 | 0 | 0 |
| ru-desktop | 0 | 0 | 1 | 0 |

The five 32 px fit failures are staged CSV/payee resolution, staged rejected-row
preview, failed-row review, provider short-link preview and provider split-link
preview. Lot-panel overflow is 2 px. These are #344’s known classes, freshly
reproduced; no new confirmed financial defect was found. Fixing only the empty-ID
lookup would leave the rendered overflow failures unresolved. #344 must cover
both before a new full gate can qualify #280/#260 for closure.

The first frontend attempt at `dea2c418` correctly failed because the new error
enum required localized presentation mappings. `91563e00` adds them; the later
candidate's result is the governing one. The superseded `91563e00` full browser
attempt was stopped after the #352 failure was diagnosed and fixed, preserving
its log/database and completed matrices. It is not counted as a full pass;
the complete `25a24f4e` retest was the final browser evidence for the initial
blocked delivery, superseded by the green #344 follow-up below. The first broad browser invocation was
stopped after its build, before tests, to select focused probes first; it is
not counted as a gate pass. Exploratory performance requests that omitted
`group_by=category` or invented `as_of_date` were rejected with 400; the final
probe uses the declared API, as shown above.

## Publication and remote checks

The initial publication of the fixes/evidence is `4ab5ff18`, pushed directly to
`main` as requested. GitHub accepted that normal fast-forward push and reported
that the repository’s PR and required-status rules were bypassed through the
existing owner permissions; no force push or rule-setting change was made.
At that observation, the [Go vulnerability run](https://github.com/sergeyfarin/rekenraam/actions/runs/39206965747)
passed, while [CI](https://github.com/sergeyfarin/rekenraam/actions/runs/39206965750)
was in progress. These are publication-time observations, not a claim that
remote CI is green. Current remote status is recorded in #280’s delivery
comment. Later publication-note commits change documentation only; the failed
shared-gate candidate for that initial blocked delivery was `25a24f4e`. The
subsequent #344 follow-up below resolves its shared-gate failures.

## #344 follow-up — green final shared gates

The [bounded repair and before/after evidence](subview-shared-book-fixes-2026-10-19.md)
records owned write-response transaction IDs, explicit long account options and
24-place crypto lots, and the two UI width fixes. The saved-book controls fail
against the unchanged UI and pass after rebuilding. No fit assertion was weakened.

At `af233abb82fe88e0b30c1274eb698d280069820f`, all four shared wrappers pass: backend race, frontend (zero diagnostics,
691 unit cases / 66 catalog cases), structural audit, and the complete browser
selection: **352 passed (21.5m)**. All eight sub-view matrices have zero failed
cells; the five unreachable branches per configuration remain explicit. Both
isolated FX journeys and all five release-preflight cases pass. The build is
fresh; `E2E_SKIP_BUILD=1 E2E_WORKERS=3 ./scripts/test-e2e.sh` reuses it. There are
no retries, skips or interruptions. #260 can cite this SHA as its green shared-gate
candidate; #352's literal complete-suite condition is met. #348's independently
confirmed contrast-oracle limit and the other retained findings remain unchanged.
Remote publication / E2E Smoke acceptance is recorded on #344 and #280.

## Maintainability notes

Record only for #284; no refactors or refactor issues are included here.

- **Domain policy inside repository transactions:** `backend/internal/db/investments.go`
  owns FIFO/LIFO/average-cost depletion; `investment_transfer_internal.go`
  chooses locked/default method, allocation and destination lineage;
  `investment_dated_valuation.go` computes dated basis/unknown knowledge and
  pricing selection. These rules must change alongside application command
  validation and previews; their transaction coupling makes a pure policy
  oracle harder to isolate. The literal repository boundary is not met. This
  is recorded for #284, with no refactor or refactor issue under this audit.
- **Error-code change amplification:** `api/openapi/components/schemas/errors.yaml`,
  generated `frontend/src/lib/api/schema.d.ts`, `api-error-messages.ts` and the
  backend writers carry related inventories. #345 demonstrates actual drift;
  the `Record<APIErrorCode, ...>` compiler check usefully caught missing frontend
  mappings when the contract was repaired.
- **Browser fixture ownership:** `e2e/playwright/support/fixtures.ts` isolates
  workers, while `subview-sweep.spec.ts:seedFixture` discovers a shared worker's
  first transaction page and `transactionWith` tries whichever dated rows it
  found. #344 demonstrates that combining individually passing specs changes
  the fixture meaning. The first-position lot opener similarly lacks fixture
  ownership. This makes broad meaningful tests harder, beyond file length/style.
- **Workflow parsing heuristics:** `scripts/check-post-merge.sh` normalizes
  one-line YAML commands with paired grep/sed expressions. #347's legitimate
  setup false positive and #351's direct-step false negative demonstrate two
  different failure modes in the same scanner. Any spelling rule must remain
  aligned between selection and normalization. Recorded for #284; no parser
  refactor is proposed here.
- **Duplicated invariant enforcement:** canonical coefficient legality lives in
  SQL CHECKs across migrations and in `internal/exact/coefficient.go`; range
  legality is also checked by services. `TestExactNumberColumnChecks` and the
  exact-number tests guard the two persistence/admission boundaries. Any change
  to those limits needs coordinated edits; #192's prior NUL bug is the recorded
  evidence for keeping the enforcement inventory visible.

## Additional audit and retained acceptance limits

[#349](https://github.com/sergeyfarin/rekenraam/issues/349) owns a dated runtime
response-schema matrix with requiredness/nullability, exact string numbers,
cursors, refusals and downloads. It must reuse named cases and deliberate schema
negative controls, not expand providers or add new product capabilities.

#348 requires a text-specific negative control before the nested-dialog override
can support contrast acceptance. The witness above invokes the actual production
test helper on real rendered pixels;
[its standalone reproduction is on #348](https://github.com/sergeyfarin/rekenraam/issues/348#issuecomment-6260489959).
It is not a financial/security defect probe.
#344 retained its broader isolation/layout acceptance during the initial audit, even when a smaller probe
passed. The complete local acceptance now passes at `af233abb`; main CI evidence
is recorded on the issue. Human language, actual owner deployment/key custody/household acceptance,
release signing/PAT/demo work and image-advisory disposition remain outside this
bounded area; the umbrella must name them rather than infer all-app readiness.


## Runtime contract follow-up — 2026-10-20

Additional audit #349 is complete with a [dated response/status matrix and
sensitivity controls](runtime-api-contract-audit-2026-10-20.md): 425 real
responses sample 80 of 332 current declared operations; 252 operations and
unobserved statuses remain explicitly unverified. #367–#369 repair null
unions, setup/exclusion disclosure and empty tag arrays. #350 and #370 remain
open. This supplements the earlier route/client-parity evidence without
claiming universal runtime conformity or changing its historical gate SHA.
