# Maintainability audit — 2026-10-21

**Status: first slice of [#284](https://github.com/sergeyfarin/rekenraam/issues/284),
partial.** This slice reconciles the earlier proposals, inventories the audit
inputs and records reproducible history measurements. The ranked shortlist is
provisional. It does not complete #284, approve a refactor, or clear R13's
investment/reporting disposition requirement. No application, API, schema or
financial behavior changes in this slice.

## Baseline and method

Source SHA: `13d15eabd68ed2d3b6177512c9c8fe56c528bbf0`, committed
2026-10-21 07:24:28 +02:00. The working tree was clean before this slice.
The source inspections below refer to this SHA. The roadmap owns execution
order; this report records evidence and remaining audit acceptance only.

The retained [measurement script](fixtures/maintainability-history.py) reads
Git objects, never the current working-tree source. Reproduce from the repo root:

```sh
python3 docs/reviews/fixtures/maintainability-history.py \
  --ref 13d15eabd68ed2d3b6177512c9c8fe56c528bbf0 \
  > /tmp/rekenraam-maintainability-history.json
python3 docs/reviews/fixtures/maintainability-history.py \
  --ref 13d15eabd68ed2d3b6177512c9c8fe56c528bbf0 --format csv \
  > /tmp/rekenraam-maintainability-history.csv
```

The [retained CSV](fixtures/maintainability-history-2026-10-21.csv) contains
all 1,402 selected files, including unchanged files. Scope and definitions:

- All reachable non-merge commits whose **committer date** is on or after
  `2026-10-01T00:00:00Z`, through the source SHA; not just first-parent history.
  There are **308 commits**, including **94 `feat:`** and **105 `fix:`**
  subjects (optional Conventional Commit scope/`!` accepted).
- Backend: tracked `.go` files under `backend/`, excluding `_test.go`.
  Frontend: `.ts`, `.svelte`, `.css` under `frontend/src/`, excluding
  `.test.ts`, generated Paraglide files and generated `schema.d.ts`.
  Contracts: `api/openapi/**/*.yaml`. Tests: backend/frontend test files
  plus all E2E TypeScript, including its support code.
- Size is current physical lines, including comments and blank lines.
  Churn is added + deleted lines from `git log --numstat --no-renames`.
  Hotspot score is churn × current lines. Touches count commits, not hunks.
  The CSV's `fixes` counts `fix:` subjects touching a file, **not confirmed
  independent defects caused by that file**.
- Only files present at the source SHA enter the tables. Deleted files do
  not; a rename is deletion/addition, with no inferred lineage. New files'
  initial contents contribute churn. SQL, translations in JSON, scripts,
  archives and generated output are outside these size rankings.
- Feature co-change counts unordered pairs of selected runtime files in
  each `feat:` commit. They exclude contracts, tests and generated output.
  Commits can contain several concerns; this is a review signal, not a lower
  bound on the edits a feature inherently requires or proof of duplication.

| Group | Files | Physical lines |
|---|---:|---:|
| Backend runtime | 311 | 116,337 |
| Frontend runtime | 252 | 45,404 |
| OpenAPI contracts | 320 | 33,209 |
| Tests and E2E support | 519 | 117,510 |

## Churn and size observations

Top backend runtime files by score (paths below are under `backend/internal/`):

| File | Lines | Churn | Touches | `feat:` | `fix:` | Churn × lines |
|---|---:|---:|---:|---:|---:|---:|
| `api/investments.go` | 3,605 | 2,622 | 57 | 51 | 6 | 9,452,310 |
| `db/investments.go` | 3,822 | 1,382 | 31 | 23 | 5 | 5,282,004 |
| `app/investments.go` | 3,510 | 1,352 | 36 | 25 | 10 | 4,745,520 |
| `app/self_check.go` | 2,114 | 1,156 | 32 | 28 | 2 | 2,443,784 |
| `app/import_service.go` | 2,058 | 794 | 15 | 5 | 9 | 1,634,052 |
| `db/self_check.go` | 1,201 | 1,235 | 26 | 23 | 2 | 1,483,235 |

Top frontend runtime files (paths under `frontend/src/`):

| File | Lines | Churn | Touches | `feat:` | `fix:` | Churn × lines |
|---|---:|---:|---:|---:|---:|---:|
| `lib/api/investments.ts` | 2,473 | 1,695 | 36 | 35 | 0 | 4,191,735 |
| `lib/transactions/investment-correction-section.svelte` | 1,064 | 1,544 | 30 | 24 | 5 | 1,642,816 |
| `routes/app/import/+page.svelte` | 2,100 | 703 | 18 | 6 | 11 | 1,476,300 |
| `lib/transactions/transaction-editor.svelte` | 1,335 | 428 | 12 | 3 | 7 | 571,380 |

The largest contract hotspot is `api/openapi/components/schemas/investments.yaml`:
5,450 lines, 4,014 churn, 69 touches (60 `feat:`, 9 `fix:`), score 21,876,300.
Do not compare this directly with runtime code as a reason to split either.

The top test/support hotspots are `backend/internal/api/recovery_audit_test.go`
(1,164 lines, 1,218 churn, 2 touches) and
`e2e/playwright/subview-sweep.spec.ts` (824 lines, 946 churn, 4 touches,
2 `fix:`). Initial audit creation explains much of the first score. The
second also has independently recorded fixture failures (#344); history alone
would not distinguish the two.

Smaller correction services have more specific evidence than size alone:

| File under `backend/internal/app/` | Lines | Churn | Touches | `fix:` |
|---|---:|---:|---:|---:|
| `investment_share_exchange_correction.go` | 278 | 304 | 5 | 4 |
| `investment_spin_off_correction.go` | 292 | 332 | 5 | 4 |
| `investment_cash_stock_merger_correction.go` | 298 | 322 | 4 | 3 |

The [part-2 audit](investment-operations-2-audit-2026-10-18.md#maintainability-notes)
records actual sibling divergence (#309/#310), separately from these counts.
Source evidence is already shared through `correctedSourceEvidence` and
`sameSourceEvidence`; their existence narrows any remaining consolidation to
orchestration, rather than proposing those helpers again.

## Feature change amplification

| Recorded feature commit | Selected runtime files changed | Interpretation |
|---|---:|---|
| `8b2ea768`, investment entry from holdings/instruments | 29 | Large UI/API integration; no duplicate-rule conclusion from the count |
| `41a45708`, replay gain acknowledgement across existing paths | 28 | Cross-family guard rollout; candidate for the invariant inventory |
| `3bbc4d1d`, immutable operational gains runs | 16 | Includes an intentional saved-output boundary |
| `95fb5428`, named gains FX methods | 12 | Native and converted policies need distinct characterization |
| `270f4a71`, operational period statement | 7 | A smaller reporting seam to inspect before R13 |

Selected pairs counted across the 94 feature commits:

| Runtime file pair | Co-changing feature commits |
|---|---:|
| `api/investments.go` ↔ `frontend/src/lib/api/investments.ts` | 28 |
| `api/investments.go` ↔ `app/investments.go` | 23 |
| `db/investment_replay_intents.go` ↔ `db/investment_replay_simulation.go` | 23 |
| `app/exports_bundle.go` ↔ `db/investment_foundation_export.go` | 22 |
| `frontend/src/lib/api/investments.ts` ↔ `frontend/src/lib/transactions/investment-correction-section.svelte` | 21 |

Backend paths in this table are under `backend/internal/`. Contract/client
coordination and export persistence coordination are expected. The replay pair
deserves inspection for repeated family dispatch, but shared changes alone do
not justify replacing typed families or merging their financial rules.

## Input reconciliation

Each row records what remains useful from the historical audit. A repaired
finding is not a current defect. These notes are inputs, not accepted refactors.

| Source | Surviving question or current disposition |
|---|---|
| [Setup/auth/privacy](setup-auth-privacy-audit-2026-10-17.md) | No maintainability heading; use its security boundaries alongside the later concurrency notes |
| [#267 identity concurrency](identity-concurrency-audit-2026-10-17.md#maintainability-notes) | Verifier reservation entry points, two session creation paths, manual event attribution; later expiry/generation fixes must constrain any extraction |
| [#268 proxy/browser](proxy-browser-privacy-audit-2026-10-17.md#maintainability-notes) | Forwarded-proto parsing differs between origin/cookie helpers; cookie clearing has an ignored argument. No current bypass is established by this slice |
| [#269 reference data](reference-data-audit-2026-10-17.md#maintainability-notes) | Category identity, dated account selection, two category-key lists, parent/write guard seams and mixed fixture clocks |
| [#270 ledger](ledger-reconciliation-audit-2026-10-17.md#maintainability-notes) | Checkpoint candidate walkers and early refusals repeat; three reorder writers and lifecycle/write rechecks need a rule-by-rule inventory. Entry-filter divergence #298 is repaired |
| [#271 import](import-audit-2026-10-17.md#maintainability-notes) | #299/#300/#302 repaired identity/decimal seams; ordered commit routing and warning persistence remain questions, not reasons to unify different providers |
| [#272 investments part 1](investment-operations-1-audit-2026-10-18.md#maintainability-notes) | Side-aware signs across cover/realized/unrealized readers; payment dependency scans lack a payment-dense benchmark. Unknown-basis status gap was repaired by #313 |
| [#273 investments part 2](investment-operations-2-audit-2026-10-18.md#maintainability-notes) | Correction siblings, exact source preservation, manually enumerated durable snapshots and three whole-run self-check helpers |
| [#274 pricing/work](pricing-durable-work-audit-2026-10-18.md#maintainability-notes) | Worker retry/completion differences and two HTTP boundaries. #315 repaired progress ownership; #316 repaired secret disclosure. Intentional outcome differences must survive any shared worker helper |
| [#275 reports/R18](reports-r18-audit-2026-10-18.md#maintainability-notes) | Live/saved metadata assembly and complete-only coverage cross several mappings. Current `formatGainsRun` already calls shared response builders; do not claim a wholly independent saved formatter. Date scopes and save exclusions intentionally differ |
| [#276 planning](planning-audit-2026-10-18.md#maintainability-notes) | Funding interpretation missed bridges in #319; category visibility/sign coupling caused #320. Budget/forecast cash sets intentionally differ; owner-day and forecast/writer parity need inspection |
| [#277 recovery](recovery-audit-2026-10-18.md#maintainability-notes) | Export ID vocabulary, two exclusion texts, test/runtime wiring and restore lock pairing; #322's closure does not make future table additions automatic |
| [#278 frontend](frontend-audit-2026-10-18.md#maintainability-notes) | Shared `ModalDialog` now exists (#327); #326 now preserves URL selections. Token and alert fixes shipped; remaining screen-specific state/fixture questions need fresh scanning |
| [#279 deployment](deployment-audit-2026-10-18.md#maintainability-notes) | Provider override refusal in both config loading and runtime construction protects different entry points; retain both enforcement boundaries |
| [#280 architecture](architecture-quality-audit-2026-10-19.md#maintainability-notes) | Repository financial policy, error inventory, fixture ownership, scanner heuristics and Go/SQL coefficient checks. Large cash-book evidence does not benchmark long investment replay histories |
| [#331 subviews](subview-audit-2026-10-19.md#maintainability-notes) | Correction/import UI state ownership, unreachable restore modal and fixture day/ordering assumptions; focus is already shared |
| [#308 diagnostics](unresolved-basis-diagnostics-audit-2026-10-18.md#maintainability-notes) | #313 repaired incomplete outcome semantics. Independent replay and lot reconciliation catch different damage; agreement is not a reason to remove one |

### #266 fixture and oracle inspection

The [reference-book report](reference-book-audit-2026-10-17.md) predates the
maintainability-notes requirement. Direct inspection covered
[`fixtures/reference-book.json`](../../fixtures/reference-book.json),
[`reference_book_test.go`](../../backend/internal/api/reference_book_test.go)
and [`support/reference-book.ts`](../../e2e/playwright/support/reference-book.ts).
The expected figures and `why` derivations are literal fixture data; API and
browser seeders execute the same public steps. `assertReferenceBookRegister`
reads actual API figures, while `exactValue` compares decimal strings as
`big.Rat`. Actual coefficient rendering uses production `exact.Decimal`; that
is a shared representation helper, not an independent test of formatting.
The fixture does not call replay/valuation code to calculate expected gains.

Maintain that financial independence. The restore reader reuses seeded IDs
because native restore preserves them; a second fresh book needs new IDs.
Shared-worker browser seeds append suffixes, but whole-book expectations still
require a dedicated book. Three whole-run assertions remain separately named
in `investment_unknown_disposal_replay_test.go`,
`investment_transfer_propagation_test.go` and `investment_spin_off_test.go`.
Whether to share their assertion plumbing is separate from keeping independent
financial expectations. This slice inspected these seams; it did not rerun
the reference book or certify every test oracle in the repository.

## July proposals: current disposition

The source is [the July audit, sections 5–7](backend-comprehensive-audit-2026-07-19.md#5-refactoring-suggestions-highest-leverage-first).
“Done” below means the bounded shipped capability, not every extension implied
by the older prose. The [capability ledger](../implemented.md) and named current
source are the delivery evidence; old proposals do not reopen completed work.

| July item | Disposition at this SHA |
|---|---|
| §5.1 Shared scaled-integer arithmetic | **Done**, `549b3814`; `internal/exact/scaled.go` owns `ScaledInt` |
| §5.2 Split large investment files | **Rejected as stated**: size alone fails #284's criteria. Inspect named policy/correction seams instead |
| §5.3 Flatten baseline/delete legacy pricing fallback | **Superseded in part**: baseline starts with declarative current tables under ADR 0013. **Candidate remains**: `db/pricing.go:ListPriceObservations` still performs one `PRAGMA table_info` query per call and has a legacy reader. Supported-schema/fixture reachability must be checked before deletion; no further schema rewrite is justified here |
| §5.4 Consolidate error writers | **Candidate**, narrowly: four writers still repeat validation classification; transaction/ledger/investment repeat typed overflow handling. Pricing's exact-range mapping has distinct copy. Shared checkpoint-refusal formatting already exists |
| §5.5 Merge transaction/record reconciliation walkers | **Candidate**, not gone: renamed to `periodScopedCandidatesFromTransaction`/`periodScopedCandidatesFromRecord` in `app/transactions_reconciliation.go`; both still traverse entries/postings into the same four fields |
| §5.6 Avoid per-bucket ledger scans | **Done for the stated minimum**: `app/ledger.go:netWorthSeriesTotals` reads postings once and folds forward. A materialized balance/cache proposal is **rejected without a new workload result** |
| §6 Scheduled backups | **Done**, R3; schedule/retention/status and restore workflow ship |
| §6 Trial-balance self-check | **Done**, R3 plus investment checks and #313 incomplete coverage; retain independent checks |
| §6 Dead-letter visibility/retry | **Done for bounded pricing work**, R11 failed-work status/manual retry; not a promise that every worker has one generic lifecycle |
| §6 Import worker observability | **Superseded as a blanket absence claim**: online import exposes batch `source_meta.fetch_status` and source status. Additional counters/stage telemetry remain a feature proposal, not an accepted refactor |
| §6 Price provenance surfacing | **Done for the shipped pricing/reporting surface**: pricing responses carry `derivation`/`derivation_text`; R11/R18 disclose selection/coverage. No claim of every conceivable provenance view |
| §7.1 Corporate-action engine | **Done for admitted manual R16 operations** (splits, share exchange, spin-off, cash/stock merger and write-off); broader automatic feeds and a separate ticker-change command are not implied |
| §7.2 Zero-proceeds write-off | **Done**, dedicated write-off entry/preview/correction; not a zero-price sale |
| §7.3 Personal-access tokens | **Done for ADR 0018's read-only ledger/report grants**, #334; arbitrary write tokens remain outside that contract |
| §7.4 First-class trade fees | **Done for R16 buy/sell gross/typed-charge/net settlement**; not a claim of every broker/tax fee convention |
| §7.5 Return of capital | **Done**, sourced basis reduction/excess, corrections and unknown knowledge |
| §7.6 Quote provider | **Done**, R17 registry/adapters under bring-your-own-key/coverage limits |
| §7.7 Crypto instruments | **Done**, R17 manual/file workflows and admitted quote sources; not exchange/DeFi automation |
| §7.8 Price void endpoint | **Done**, R11 void/impact workflow |

The [October schema review](investment-schema-design-review-2026-10-11.md#proposals)
also predates this baseline. Its P1–P6 shipped as #188–#193: revised shorts,
the committed movement owner, independent conservation oracle, executed
capability matrix, canonical numeric checks and precise backdating docs.
`investment_effective_movements.go` explicitly excludes replay inputs, original
audit/export evidence and alternative reporting profiles. A new “one effective
reader for everything” would erase those boundaries. Replay-only writes, a
generic revision header and typed intents remain conditional proposals; this
slice supplies no new amount divergence or workload benchmark to promote them.

## Provisional ranked shortlist

The criterion numbers are #284's: 1 = demonstrated divergence history;
2 = named upcoming change with counted edit sites; 3 = repeated financial or
security rule; 4 = measurable single-caller abstraction cost;
5 = meaningful-test construction difficulty. Each row below has evidence for
at least one criterion. Counts of touched files do not establish criterion 2.
Costs are relative S/M/L estimates, not commitments. “Before R13” means obtain
the disposition before implementation; it does not pre-approve moving code.

| Rank / candidate | Evidence | Criterion | Affected modules | Cost | Payoff tied to named work | Risk | Safety net | Consequence of doing nothing | Recommendation |
|---|---|---|---|---|---|---|---|---|---|
| 1. Corporate-action correction orchestration | #309/#310 sibling divergence; three current services, 3–4 `fix:` touches each | 1, 3 | Three `app/investment_*_correction.go` siblings above | M | #119 can rely on stable correction/source/gain semantics while adding return reads | A generic wrapper could erase distinct entitlement/no-op rules or gain acknowledgements | Part-2 independent register; each family’s replacement, late-refusal and source-evidence tests | Future corrections still require coordinated review of three pipelines | **Before R13**: inspect clone output and pin a minimal seam or explicitly retain siblings |
| 2. Side/knowledge-aware projection policy ownership | #272 identifies sign treatment in cover preview, realized and unrealized reads; #280 identifies policy in persistence; current dated valuation adds another reader | 3 | `app/investment_short.go`, `db/investments.go`, `db/investment_dated_valuation.go`, transfer policy/writer admission | M | #119 needs an explicit reusable valuation/flow boundary; existing effective movements already have one bounded owner | Combining gross cash, signed basis and native/FX amounts could alter correct results; moving write admission could break atomicity | #266 hand figures, unknown-basis controls, named-short tests, R18 dated/FX tests | R13 authors must rediscover sign/knowledge/transaction boundaries | **Before R13**: complete invariant inventory; accept a helper only for genuinely identical policy |
| 3. Owned browser fixtures and whole-run assertions | #344 shared-suite failures; shared-worker selection and three self-check assertion helpers are recorded and inspected | 5 | `e2e/playwright/support/`, `subview-sweep.spec.ts`, three app test helpers | M | #119 adds whole-portfolio scenarios that need deterministic book scope and honest incomplete outcomes | Shared builders can hide independent expectations or change seeded order | #266 literal register, existing owned fixture controls, complete/incomplete/damage cases | More suite-order assumptions and repeated assertion changes | **Before R13**: disposition test ownership; keep financial oracle independent |
| 4. Checkpoint candidate traversal | Current two walkers copy the same four posting fields; early courtesy refusal also repeats (#270) | 3 | `app/transactions_reconciliation.go`, `transactions_write.go` | S | Protects later ledger producers and #119’s input trust without reworking checkpoint policy | Losing sequence/date fields or moving authoritative checks outside writer admission | Existing lifecycle/reorder/checkpoint-impact HTTP tests, #270 reference figures | A future candidate-field change must update both walkers | **Later** unless invariant inventory finds an active omission; no new defect found here |
| 5. Common error classification leaves | July missing-overflow history; four current validation writers, three typed-overflow arms; #280/#345 contract inventory drift | 1 | `api/transactions_errors.go`, `ledger.go`, `investments.go`, `pricing.go` and error contract | S | Limits repeated mapping work when #119 defines numerical failure outcomes | Case precedence, domain-specific copy and checkpoint payloads are public behavior | Runtime response-schema audit, named API precision/refusal tests, generated error-code typing | New shared errors can still miss one writer | **Later**, pending clone/precedence inventory; retain domain extensions |

The residual legacy pricing probe is a separate **unranked investigation**:
one schema query per list call is observed, but support/test reachability and
change payoff are not yet established well enough to rank its removal.
Other input notes remain in the table above rather than being silently rejected.

## Considered and rejected in this slice

- Splitting files or introducing an operation registry merely because files
  are long: no qualifying behavioral/change-cost evidence for that proposal.
- Repeating #189's effective-movement consolidation: already delivered;
  replay inputs and immutable evidence deliberately use different contracts.
- Collapsing native/FX/saved reporting into one calculation or changing saved
  schemas: date policies, exact numeric types and operational-only immutable
  bytes are intentional R18 boundaries, not proven duplication.
- Removing SQL CHECKs because `exact.Coefficient` validates in Go, or dropping
  runtime override checks because config loading checks too: separate admission
  boundaries protect direct persistence and hand-built configs.
- Replacing independent expected figures with output of shared production
  replay/valuation helpers: agreement would weaken #266's financial oracle.
- Whole-book replay per write, a generic revision header or baseline rewrite
  as “cleanup”: existing ADR/schema-review limits stand; no new justification.
- Rebuilding modal or unknown-basis status abstractions: #327/#342/#313 already
  delivered the relevant shared behavior. Remaining callers/tests can still be
  measured without reopening those deliveries.

## Remaining acceptance and validation

#284 remains open. This slice completes the input/history baseline and July
reconciliation. Still unverified: one duplication pass per language and manual
triage; the exhaustive money/ledger/lot/security enforcement inventory; fresh
handler/repository/frontend arithmetic and fan-out scans; dead-code and
single-caller analysis; a repository-wide test-fixture/oracle survey; and final
investment/reporting dispositions. The provisional ranking can change with
that evidence. No refactor issues are accepted or created in this slice.

Validation **passed**: a second fixed-SHA run produced byte-identical CSV;
all 1,402 rows agree with JSON output; an independent Git count confirms 308
commits and direct numstats confirm the API investment/import-page totals.
`./scripts/check-post-merge.sh HEAD` is clean, including generated-client
stability and no test loss. All 57 relative links/anchors across this report
and the roadmap resolve; Python syntax and `git diff --check` pass.
These are audit-artifact checks; application suites were not rerun because
runtime and test inputs are unchanged. No push, remote CI, release acceptance,
household evidence or full #284 completion is claimed.
