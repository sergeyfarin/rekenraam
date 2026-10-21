# Maintainability: invariant ownership — 2026-10-21

**Status: third slice of [#284](https://github.com/sergeyfarin/rekenraam/issues/284),
partial audit.** This slice inventories 34 named money, ledger, lot and security
rule groups and their enforcement boundaries. It narrows the provisional
policy-consolidation candidate: several important owners already exist. Repeated
checks at service, writer and storage admission often protect different entry
points and must survive any extraction. No application behavior changes or
refactors are accepted here.

One independent measurement issue is filed:
[#379](https://github.com/sergeyfarin/rekenraam/issues/379), payment-dense
investment command admission. This is an unmeasured workload, not a reproduced
performance defect. Remaining #284 methods and final ranking stay in #284.

## Baseline, coverage and retained evidence

Source: `13d15eabd68ed2d3b6177512c9c8fe56c528bbf0`, the same fixed source as
the [history/input slice](maintainability-audit-2026-10-21.md) and
[runtime clone slice](maintainability-duplication-2026-10-21.md). Working HEAD
before this slice was `4ff4a01047143817f8e8d08205e4c5d174147ba8`; intervening
commits changed audit artifacts/docs only. Runtime and test sources match the
baseline. Source paths and function names below refer to that SHA.

The matrix is a manually reviewed ownership map, with a reproducible index to
make its selected gates discoverable. It is **not** proof that every financial
condition in every function has been enumerated. Anonymous/inline conditions,
runtime receiver dispatch, unselected rules and future consumers need semantic
review; the later layer/test scans can still add entries.

| Retained artifact | Measured coverage | Interpretation limit |
|---|---|---|
| [Go selected-gate index](fixtures/maintainability-invariant-sites-2026-10-21.csv), [101 selected names](fixtures/maintainability-invariant-gates-2026-10-21.json), [AST tool](fixtures/maintainability-invariant-sites.go) | 311 runtime Go files, 3,336 function declarations; 1,306 sites: 105 definitions, 834 calls, 367 references | Basenames, not type-resolved calls. `Parse` includes unrelated parsers; `Int64` includes logging. References include field names as well as function aliases. These are navigation counts, **not enforcement counts** |
| [Frontend index](fixtures/maintainability-invariant-client-sites-2026-10-21.csv) | 169 selected call-shaped matches in 40 files | Lexical calls/declarations; not a Svelte/TS semantic inventory or frontend arithmetic/fan-out audit |
| [SQL index](fixtures/maintainability-invariant-sql-sites-2026-10-21.csv) | 20 migration files scanned; 177 Up trigger declarations in 14 of them | Source declarations, not 177 simultaneously active triggers. Later replacements, including posting-rule and import-identity migrations, remain visible |
| [Provenance/counts](fixtures/maintainability-invariant-provenance-2026-10-21.json), [frontend/SQL tool](fixtures/maintainability-invariant-boundaries.py) | Fixed Git objects and deterministic output | Whitelists are deliberate review scope, not completeness detectors |
| [Focused test results](fixtures/maintainability-invariant-tests-2026-10-21.json) | 34 distinct top-level tests, race mode, app/db/API | Existing controls selected for these boundaries; not a whole-suite or deployment certification |

All paths in the matrix are under `backend/internal/` unless otherwise stated.
Storage constraints/triggers are independent admission protection, not reasons
to remove service feedback or writer-time checks. UI review tokens/versions
express what the owner saw; they do not authorize a stale write.

## Money and ledger ownership

| ID / rule | Service / client admission | Writer, read or storage owner | Disposition |
|---|---|---|---|
| M1. Exact canonical stored coefficients | `exact.Parse` normalizes input through integer parsing; API/service parsing gives early errors | `exact.Coefficient`; migration numeric CHECKs reject noncanonical stored text, excessive digits and embedded NUL | **Retain both.** Parsing acceptance/normalization and canonical storage rejection are different contracts; SQL is reachable without the service |
| M2. Commodity scale, bounded conversion and rounding | Commodity-kind limits and command validation; frontend shared decimal-string helpers | `exact.MaxScaleForCommodityKind`, `ScaledInt.Int64`, `MulDivRound`, `TruncatedTo`; stored scale checks; allocation range guards | **Retain distinct limits.** Ledger coefficients can exceed int64; investment allocations have bounded int64 contracts. Do not replace checked conversions or prove safety from the return type alone |
| M3. Each journal entry balances per commodity | `app/transactions_validate.go:validateBalanced` aligns scales with `big.Int`/`exact.Pow10`, then sums by entry/commodity | Writer admits prepared journals; independent self-check balance reconstruction and SQL shape constraints | Manual alignment remains after July's `ScaledInt` delivery. **Later candidate**, requiring mixed-scale/per-entry characterization; no imbalance found. Cross-entry cancellation is not valid balancing |
| M4. Same book, owner and referenced objects | Services resolve book-scoped IDs; `cleanPosting` and command preparation validate references | Writer dependency reads, same-book FKs/triggers, shared investment writer book/actor checks | **Retain.** Runtime single-book scope does not justify removing `book_id` or persistence protection |
| M5. Calendar and knowledge boundaries | Server `OwnerTodayAt`; frontend `useOwnerToday`; explicit financial dates remain explicit | Dated financial reads and observation selector use separate effective-date/knowledge-cutoff contracts | **Retain.** Browser UTC today is not the owner day; current observation admission is not a full historical-known-then reconstruction |
| L1. Account eligibility, hierarchy and dated cycles | Account/transaction services reject invalid parent, category, archived or managed-account combinations early | `db/accounts.go:requireArchiveConsistencyTx`, `requireNoDatedParentCycleTx`, `requirePostingsStayEligibleTx`; versioned parent changes and posting dependencies | **Retain writer authority.** #293/#294 repaired the older parent/write concern; do not reopen the historical note as a current race |
| L2. Immutable posted history and lifecycle | Update/void/unvoid/delete services prepare new versions with reasons | Transaction lifecycle writers pin current version and append audit/version rows; immutable financial tables/triggers; hard deletion is limited to eligible never-posted drafts | **Retain.** Void/correction is not physical deletion of posted evidence |
| L3. Producer and system-draft boundaries | Import, recurring and investment producers prepare their own eligible drafts/commands | `isSubledgerManagedAccount`, prepared transaction options and writer guards; financial readers select the intended posted/effective history | **Retain distinctions.** A draft preview, posted ledger read and immutable producer evidence are not interchangeable datasets |
| L4. Optimistic versions and account dependency races | Expected version and account selection accompany lifecycle commands | `requireExpectedVersionTx`, `requireAccountRuleDependenciesTx`, structural account writer checks | **Retain early feedback plus final checks.** The passing stale-version/account-race controls cover real consumers, not just helper calls |
| L5. Investment-linked generic mutation refusal | `app/transactions_write.go:rejectInvestmentLinkedMutation` directs the owner to the investment workflow | Generic lifecycle writers pin the prepared version; dividend conversion `RepairImportDividend` rechecks source/ordinary status and appends a version plus operation atomically | No reproduced bypass. **Retain/inventory**, rather than infer a defect from the absence of an identical link query in generic writers. Other link-creation paths still need the later semantic sweep |
| L6. Reviewed checkpoint identity and invalidation | Early `rejectIfCheckpointAlreadyInTheWay`, review impact and frontend `checkpointOverride` | `enforceCheckpointBoundaryTx`; shared `bindCheckpointOverride`; batch-reviewed context deliberately permits the reviewed superset; invalidation inside the write | **Retain authority.** Six direct callers of the generic enforcer and four callers of the binding leaf include specialized command-netted/move/reorder paths. Courtesy refusals do not replace binding |
| L7. Positional checkpoint crossings and ordering | Transaction/record impact walkers copy date/sequence/account/commodity candidate fields | Dedicated move/reorder writers calculate positional crossings; investment writer nets the entire command before checkpoint comparison | **Do not unify different algorithms.** The two candidate walkers remain a small later helper candidate; moving checks outside the transaction would weaken review binding |
| L8. Atomic imported identity/source acceptance | Import routing prepares source identities, dividend review and buy/sell corrections prepare native facts | `RepairImportDividend`, transactional source-revision callbacks/`CommitSourceRevisionInTx`, identity uniqueness and the shared writer's final callback | **Retain atomicity.** Journal, lots, source acceptance and audit must commit together; similar source callbacks do not justify a separate post-commit write |

## Investment and reporting ownership

| ID / rule | Preparation / consuming boundary | Authoritative owner and intentional differences | Disposition |
|---|---|---|---|
| I1. Typed operation identity and lot provenance | Per-family services and typed wrappers | `requireKnownInvestmentOperationKind`, `recordInvestmentFoundationTx`, same-book operation/lot/link constraints | **Retain typed families.** An operation registry is not justified by file length or dispatch repetition |
| I2. Side, dated admission and effective movements | Long/short command preparation and dated holdings | `requirePositionSideAvailableTx`, `requirePositionEventInOrderTx`, `requirePositionBasisRangeQueryTx`; `EffectiveLotMovements` owns admitted committed movement reads | **Reuse existing owner.** Replay inputs, original audit/export evidence and alternative profiles intentionally differ; do not recreate #189 or merge these contracts |
| I3. Method tiers, elections and allocation scale | Services resolve requested/default methods for preparation | `internalTransferPolicyTx` rereads transactional policy; `InternalTransferAllocation` is shared by dated holdings, current positions and writer policy; allocation-scale guards | **Retain write-time resolution.** A global/account default change must not reinterpret recorded historical elections |
| I4. Known, known-zero and unknown basis | Command-specific knowledge validation and UI disclosure | Paired knowledge/nullable-amount tuples in immutable events, replay/revision selection, aggregate readers and `valuePositions` | **Retain representation boundaries.** Unknown basis cannot become zero gain or a known-subset pool rate; selecting a revision replaces the tuple together |
| I5. Quantity, basis and proceeds conservation | Disposal preparation and preview | `disposeLotTx`, `proratedCostBasis`, native operation writers, immutable allocation snapshots; independent lot/register oracle | **Retain independent tests.** The unchecked integer extraction inside proration relies on its bounded quantity/range proof; it is not interchangeable with general coefficient conversion |
| I6. Transfer lineage and bridges | Transfer service validates accounts/holding/currency and knowledge | Shared allocation decision plus transactional transfer writers preserve source lineage, cost currency, method and admitted unknown-to-known links | **Retain family semantics.** No new generic basis-rewrite abstraction; basis bridges do not create external portfolio cash flows |
| I7. Correction ancestry and exact source evidence | Three corporate-action correction preparation pipelines | Shared `correctedSourceEvidence`/`sameSourceEvidence`; per-family inverse journal, entitlement and replacement guards; durable correction links | **Candidate remains narrow:** preparation orchestration backed by #309/#310. Exact source helpers already exist. Do not merge family financial effects or no-op/refusal rules |
| I8. Preview rollback and command atomicity | Typed preview/write wrappers select one shared boundary | `runInvestmentJournalsWithGuardTx`: guard, dependencies, journals, effects, payment recheck, net checkpoints, gain acknowledgement, final source callback, commit/rollback. Preview rejects a post-write callback; explicit audit-only commands are special cases | **Retain shared writer.** Splitting it into service checks and independent commits would lose admission/rollback guarantees |
| I9. Committed gain acknowledgement | Preview disclosure and frontend `gainAcknowledgement` bind what was reviewed | Writer compares before/after effective disposal snapshots and `requireInvestmentGainAcknowledgement` before committing | **Retain server authority.** Gross/signed proceeds and positive disposal-basis representations differ; `shortCoverResult`, realized folding and `investmentGainValue` are not one identical formula |
| I10. Payment-in-lieu entitlement survives later edits | Payment entry checks short holding/ex-date | Before-write `entitledPaymentsInLieuTx` and after-effect `requirePaymentsInLieuStillEntitledTx` use `checkPaymentInLieuEntitlementTx` for dated short eligibility | **Retain invariant; measure separately in #379.** Book-wide scans for every command are observed, but payment-density cost is unmeasured |
| I11. Valuation, quote/FX coverage and saved profile | Current, dated and alternative gains readers; formatted live/saved responses | All three use `db/investments.go:valuePositions`; `db/observation_selection.go` owns live quote admission. Immutable operational run bytes, alternative profiles and FX/date policies retain their own contracts | **Narrow broad policy-move candidate.** Reuse these owners for R13. No new universal read engine or blanket move of transaction-coupled policy into services |
| I12. Stale browser review protocol | Thirteen components in slice 2 repeat preview/review/submit/refusal refresh; shared money/owner-day helpers remain input boundaries | Expected versions, checkpoint sets and gain tokens are rechecked by backend writers | **Frontend controller candidate remains provisional.** It can share lifecycle plumbing while preserving each request's financial fields, disclosures and accessibility; lexical counts do not prove safe extraction |

## Security and durability ownership

| ID / rule | Entry boundary | Persisted / final owner and intentional differences | Disposition |
|---|---|---|---|
| S1. Credential shape, password cost and secret hashing | Username/password validation, Argon parameter bounds, random token/recovery generation | `verifyPassword`/`hashPassword`, `hashSessionToken`, `hashRecoveryCode`; only appropriate hashes persist | **Retain bounded parsing and runtime verification.** Shared hashing is already present; do not create a generic credential abstraction from setup/login similarity |
| S2. Throttle admission and generation settlement | Password login, MFA completion, password reauth and activation reserve before verification | `reserveAttempt`/`reserveScope`; persisted generation identity; `successSettlement`, conditional `refundLoginAttemptExec`/`deleteLoginThrottleExec` | **Retain generation pins.** Three direct reserve calls include a reauth wrapper serving multiple verifiers. IP refund differs deliberately from username/device/session success reset; replacement or missing generations must stay untouched |
| S3. Session/device/PAT expiry after admission waits | Fresh `authDeadlineOpen` for session/device read results; token management/authentication | `tokenAdmissionTime` acquires SQLite writer before sampling write guards; `tokenManagementSession` rechecks identity, expiry and password; token-use write guards again | **Retain read/write distinctions.** Pure reads need fresh time after the pool wait, not a writer lock; queued mutations need final write admission. Setup and login session creation have different lifecycle inputs |
| S4. Factor pinning, replay and challenge cutoffs | TOTP/recovery verification and MFA challenge flow | `admitTOTPStep`, `RecordMFATOTPUse`, conditional recovery/challenge consumption; final `CompleteLogin` pins challenge/enrollment state | **Retain fail-closed cuts.** Accepted concurrency limits in #267's report remain: a failed final login can spend a factor/challenge; concurrent recoveries can spend two codes while only one challenge wins. This slice changes no policy |
| S5. Revocation, owner recovery and audit settlement | MFA/token/device lifecycle services; stopped-server recovery command | Transactional management audit where required; final login predicates; recovery file-lock/revocation boundary. Login event logging deliberately does not make logging failure lock out the owner | **Retain explicit event ownership.** MFA activation's existing-session policy and already-admitted password attempt limits remain those documented by the identity audit |
| S6. PAT grants, route scope and authentication precedence | Supplied Authorization selects token authentication without cookie fallback | Exact read-route allowlist/default deny, hashed expiring/revocable token and management session guards; read-only grants per ADR 0018 | **Retain separate scope and authentication checks.** No general write-token or arbitrary route grant is implied |
| S7. Browser mutation, origin and proxy trust | `requireAuthenticatedMutation`; request origin/HTTPS/client-IP helpers | Same-origin mutation admission plus configured trusted proxy/Caddy boundary; forwarded header contracts differ among helpers | **Inventory, no new bypass established.** Do not equate first-value HTTPS interpretation with whole-header origin parsing or consolidate without malformed-header controls |
| S8. Secret/export/provider boundaries | Config loading, provider construction, redaction, ledger export and native backup contracts | Runtime provider override refusal protects hand-built config too; backups preserve credential hashes while ledger exports exclude credentials; secret key is retained separately | **Retain distinct entry points.** #336 owner recovery/key/off-device evidence remains external work; this source inventory cannot satisfy it |
| S9. SQLite connection durability and writer isolation | Native SQLite open/DSN and connection setup | `db/sqlite.go` connection hook sets FULL per connection; single writer and read pool; FK/busy/WAL policy under ADRs 0014–0016 | **Retain connection-wide enforcement.** One successful PRAGMA on one connection is not proof for all future connections; no persistence-stack change is proposed |

## Consequences for the provisional shortlist

1. **Narrow side/knowledge policy consolidation.** `valuePositions` already has
   three consumers: `PositionsWithGains`, `DatedPositionsWithGains` and
   `AlternativeMethodGains`. `InternalTransferAllocation` likewise has three
   consumers: dated holdings, current positions and transactional transfer policy.
   These are shared owners, not six new edit sites for R13. Sign formulas use
   different tuples: cover result combines signed cash with released opening
   proceeds, whereas gain impact subtracts positive disposed basis. Any smaller
   proposed helper needs identical-input characterization first. Moving policy
   from `db` solely because of the directory name is rejected; transactional
   policy must see the same admitted facts as its effects.
2. **Retain correction preparation and browser review candidates for final
   disposition.** This slice supplies ownership constraints, not permission to
   implement them. Checkpoint traversal remains a later small candidate; the
   binding leaf is already shared. July arithmetic consolidation shipped, but
   ledger balancing still contains manual scale alignment worth recording.
3. **Close stale audit interpretations.** Account parent/write checks are now
   fenced. The ordinary-dividend conversion path increments the transaction
   version atomically, so a generic mutation prepared earlier fails its version
   guard. Those inspected seams do not justify new defect issues. This does not
   certify every possible future operation-link writer.
4. **Track the independent workload in #379.** The payment check runs before and
   after command effects, including unrelated investment commands. #280's cash
   ledger/read benchmark does not measure this shape. #379 asks for density and
   history measurements with entitlement/refusal/rollback controls before any
   optimization. It is P3, needs no provider or owner data, and is not a blanket
   R13 blocker. Remaining audit checklist work stays in #284; no speculative
   refactor issues were opened.

## Reproduction and validation

Prepare a fresh baseline source directory using the second slice's retained
preparation tool (the source directory contains the original runtime files):

```sh
python3 docs/reviews/fixtures/maintainability-clones.py \
  --ref 13d15eabd68ed2d3b6177512c9c8fe56c528bbf0 \
  --output /tmp/rekenraam-284-invariants-reproduction
GOCACHE=/tmp/rekenraam-go-build-cache go run \
  docs/reviews/fixtures/maintainability-invariant-sites.go \
  -root /tmp/rekenraam-284-invariants-reproduction/source \
  -targets docs/reviews/fixtures/maintainability-invariant-gates-2026-10-21.json \
  > /tmp/rekenraam-284-invariants-reproduction/go.csv
python3 docs/reviews/fixtures/maintainability-invariant-boundaries.py \
  --ref 13d15eabd68ed2d3b6177512c9c8fe56c528bbf0 \
  --output /tmp/rekenraam-284-invariants-reproduction
```

The Go index control uses two local functions, `guard` and `generic[T any]`,
called directly and through `generic[int]()`, plus `alias := guard; alias()`.
The selected names produce two definitions, two direct calls and one reference;
the alias invocation is deliberately unresolved. A string and comment containing
`guard()` add no AST sites. A missing selected definition fails rather than
silently emitting an empty result.

The retained test result JSON names every selected test. Reproduce the same
selection from that file without maintaining another test-name list:

```sh
python3 - <<'PY'
import json, os, subprocess
from pathlib import Path
results = json.loads(Path('docs/reviews/fixtures/maintainability-invariant-tests-2026-10-21.json').read_text())
for selection in results['selections']:
    names = sorted({test['Test'] for test in selection['tests']})
    packages = sorted({test['Package'].replace('rekenraam/backend/', './', 1)
                       for test in selection['tests']})
    subprocess.run(['./scripts/go-backend.sh', 'test', '-race', '-p', '2',
                    '-timeout=5m', *packages, '-run', '^(' + '|'.join(names) + ')$'],
                   env={**os.environ, 'GOCACHE': '/tmp/rekenraam-go-build-cache'}, check=True)
PY
```

**Passed:** 34 selected top-level tests under race mode, including canonical
stored numbers, effective-history ownership, stale lifecycle/account admission,
dividend source/version pinning, unknown basis, named shorts, recorded method
elections, lot conservation, payment dependencies, real throttle-generation
consumers, auth/PAT expiry after waits, API token default deny, the independent
reference-book register and R18 read purity/dated-FX consumers. Go race mode does
not instrument bundled SQLite C. These are existing correctness controls, not a
new payment-density benchmark.

**Artifact checks passed:** all three indexes reproduced byte for byte;
source-line/count checks, AST sensitivity/missing-name controls, all 103 relative
links/anchors across the six touched Markdown documents, helper formatting/syntax,
`./scripts/check-post-merge.sh HEAD` and `git diff --check`.
Runtime, API, schema and shipped capability ledger are
unchanged. No new CI gate is introduced.

## Remaining acceptance

#284 remains open. This bounded ownership matrix is delivered; it is not an
exhaustive receiver-resolved proof. Fresh handler/repository policy and frontend
arithmetic/date/fan-out scans, dead-code/single-caller analysis, broader
test-fixture/oracle and test-duplication work, and final investment/reporting
ranking/dispositions remain. Those passes must extend this map if they find
additional enforcement sites or rules. R13 specification may proceed; its
implementation still waits for #284's investment/reporting dispositions and its
own calculation contract. No owner/release gate is cleared by this slice.

Subsequent evidence: the [fourth slice](maintainability-layers-2026-10-21.md)
delivers the bounded layer/arithmetic/date/fan-out sweep named above. It records
native realized-total policy in the API response builder, residual frontend
numeric leaves/adapters and the confirmed forecast-date admission issue #380.
The matrix remains source evidence; its historical remaining-work list does
not undo the later delivery. Final dispositions and test/dead-code work remain.
