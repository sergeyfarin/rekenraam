# Investment review disposition — 2026-10-02

Reviewed the supplied twelve findings against starting commit `4cf39833`, the
accepted ADRs, current writers/readers, named regression tests and live GitHub
issue bodies/states. This is an independent disposition, not a claim to have
repeated the original review of all 51 commits. The working tree was clean at
start. GitHub #125/#126/#127 were closed; #99/#114/#120 were open.

## Earlier findings and positive claims

The supplied earlier-finding statuses align with the current code and named
regressions; they do not justify reopening completed integrity #125:

| Claim | Current evidence / qualification |
|---|---|
| Transferred FIFO/LIFO uses original acquisition date | `TestTransferOriginalAcquisitionDateOrdersFIFOAndLIFO`; opened_on remains the eligibility date |
| Negative proceeds survive replay/correction | `TestNegativeSaleProceedsSurviveLaterReplayAndReplacement` |
| Decisions are operation/sequence keyed | Baseline unique operation/decision sequence; `TestDisposalDecisionSequenceAllowsSharedJournalProvenance` |
| Operation transaction header retired | Baseline lacks the header; correction/import/export journal-link tests exercise that schema |
| Components/proceeds checked against journal | Pinned posting components and per-decision clearing attribution; independent conservation mutation tests |
| Immutable lots separated from current state | `investment_lot_state` and reconstruction/missing-state tests; duplicated opening evidence remains item 7 |
| Average-cost internal transfer fixed | **Qualified:** unsafe selected-lot basis is refused; pooled transfer remains missing, item 3 |
| Shared correction writer and source acceptance | Reversal/replacement stale-source and late checkpoint/acceptance rollback tests |
| Backdated buys, buy reversal and imported corrections | Shipped long-trade scope; native source-linked corrections plus Trading 212 quantity/net acceptance, not all providers/fields/families |
| Unknown basis is explicit | Projection/API/export knowledge is explicit; unknown immutable opening commands/resolution remain a separate gate |

The prior statement that CI was green is a statement about a particular earlier
run, not a substitute for validating this change. This review runs the current
full backend gate; it does not claim every new follow-up is implemented.

## Findings and decisions

| Item | Disposition | Action / issue |
|---|---|---|
| 1. Silent gain restatement | Confirmed correctness/UX gap; changed operational gains are intended replay results, undisclosed changes are the defect | T-114 #129 first |
| 2. Missing splits | Confirmed workflow gap; conditional import blockage, not proof every later sell fails | T-122 #137 promoted before further correction families |
| 3. Average-cost transfer refusal | Confirmed safe but incomplete operational workflow | T-123 #138 |
| 4. Only buys backdate | Partly true: transfer-in is refused, while reinvestment and some earlier sales already replay/work | T-117 #132 |
| 5. Register grouping / reconciliation netting | Confirmed missing grouping and conservative per-journal checkpoint effects; proposed date-only netting is insufficient | T-120 #135 |
| 6. Plain-buy preview skips replay | Reproduced and fixed in this review | Shared rolled-back buy writer; named regressions |
| 7. Duplicate facts / selectors | Confirmed maintenance debt; “exactly” and dropping all transaction links overstate the safe change | T-124 #139 |
| 8. Cross-position replay | Confirmed missing propagation; unchanged transfers already replay; full rebuild is a proposal, not a demonstrated solution | T-124 #139 before outbound/compound work |
| 9. Baseline leftovers | Confirmed schema/contract debt, not a proven current financial defect | T-124 #139; compound-date gate retained |
| 10. Open-ended #99 | Confirmed issue lacks bounded acceptance and includes evidence-blocked work | Close delivered long-trade scope, eight separately accepted families #129–#136 |
| 11. Docs drift/history | Confirmed stale gates and contradictory status | Active plan reduced to contract/status/gates; history moved to dated snapshot; roadmap/todo/#120 aligned |
| 12. Slow race gate | Confirmed runtime concern; claim #126 merely raised timeout is false | T-125 #140; preserve race/financial coverage |

### 1 — gain impact

`ReconciliationImpact` in `backend/internal/app/transactions_types.go` contains
only `AffectedCheckpoints`. `CreateTransactionAndLot` in
`backend/internal/db/investments.go` admits an opening behind a later rewrite,
simulates effective intents, then persists disposal revisions. Gains readers
select effective basis. Therefore a historical sale can change gain without any
cash checkpoint being affected. That is not a violation of immutable snapshots:
the original allocation remains audit evidence, while current gain uses replay.

`TestBuyPreviewReplaysAndRollsBackReconciledHistory` now exercises the concrete
FIFO example: 10 shares bought for 200.00, 5 sold for 150.00, followed by an
admitted earlier 10-share buy for 20.00. The gain changes from 50.00 to 140.00.
The successful preview rolls back every durable row and reports checkpoints;
it still has no gain field. Existing average/LIFO/specific-lot temporal and
replacement tests cover recorded-method replay. All replaying entry/import/
correction paths need the same disclosure contract, rather than a buy-only UI.

[T-114 #129](https://github.com/sergeyfarin/rekenraam/issues/129) requires old/new
exact basis/gain, stable disposal identity, explicit acknowledgement and commit
revalidation. The suggested persistent “gains acknowledged through” date is
**not adopted**: the app has no filed-year state, and a date alone cannot bind
acknowledgement to a particular replay result. Decide any permanent cutoff
semantics separately. ADR 0012 still distinguishes operational gains from
reproducible tax/historical reports owned by R18.

### 2 — splits and import

`backend/internal/app/import_fill_type_test.go` explicitly covers `STOCK_SPLIT`
and other non-TRADE types as review-only. No native split command exists.
The [slice 5 contract](../plans/investment-operation-slice-5-contract.md)
already specifies ratio, dated eligibility, exact fractions, security-delta
postings, conserved basis and dependent replay.

Without the quantity adjustment, later sales **can** exceed available lots.
The supplied claim that every later sale must fail is too strong: a smaller
sale or subsequent acquisition can fit the recorded quantity. Neither that
qualification nor unsupported estimates of how many portfolios are affected
reduces the migration priority. [T-122 #137](https://github.com/sergeyfarin/rekenraam/issues/137)
puts manual split/reverse split and verified mapping ahead of other correction
families, after gain safety. The provider Fill schema names `STOCK_SPLIT` but
does not define a split-ratio field; the type alone cannot justify inferred
entitlement. Mapping must use verified payload evidence or link a manually
entered action without duplicate posting.

### 3 — operational pool transfers

`TestInternalTransferRefusesLotBasisFromOpenAverageCostPool` and
`TestInternalTransferRefusesAverageCostDefaultBeforeFirstSale` verify the fence.
`requireInternalTransferBasisMethodTx` checks the source policy and lock. Safe
refusal is preferable to carrying the selected lot's original basis from a
redistributed pool, but the remaining workflow is useful.

[T-123 #138](https://github.com/sergeyfarin/rekenraam/issues/138) specifies dated
pool allocation, exact final remainder, conservation, source lineage and
destination integration. “Small” is not verified: partial pool depletion,
repeated transfers and dependency replay all need tests. Do not describe this
account-scoped operational average as full UK Section 104 support: HMRC's
[share identification guidance](https://www.gov.uk/government/publications/shares-and-capital-gains-tax-hs284-self-assessment-helpsheet/hs284-shares-and-capital-gains-tax-2024)
puts same-day and next-30-day identification ahead of the Section 104 holding.
Tax-policy completeness remains R18. French tax completeness was not assessed
and is not needed to justify the operational feature.

### 4 — backdating scope

`CreateExternalTransferIn` calls `createLotWithAuditTx(..., false)` and existing
`TestExternalTransferInRefusesBackdatingBehindSaleWithoutPartialWrite` verifies
the chronological refusal. But reinvestment calls `CreateTransactionAndLot`,
which already enables replay admission for earlier openings. That branch was
present at the starting commit; this review changes only how it is simulated.

A temporary named `TestReviewProbeReinvestedDividendBackdating` recorded a May
buy, July FIFO sale and March reinvestment. The reinvestment **succeeded**,
changed the sale gain from 50.00 to 90.00, and self-check passed. The probe was
removed after execution. The supplied claim of `ErrOutOfOrderPositionEvent`
for reinvestment is not reproduced on current code; its test conditions were
not supplied. Its reconciliation preview still builds only a journal plan,
so replay feasibility/disclosure parity belongs to T-114 #129.

`TestBackdatedSaleStillWorksWhenOnlyPurchasesFollowIt` also proves an earlier
sale is allowed in the non-rewrite case. [T-117 #132](https://github.com/sergeyfarin/rekenraam/issues/132)
now scopes missing transfer-in admission and sales/write-offs behind dependent
disposals, not reinvestment admission that already works. Calling transfer
admission “cheap” is unsupported: opened_on availability and original
acquisition ordering are separate dates, and impacts/dependencies must roll
back together. Pin existing reinvestment behavior rather than implement it
again; native reinvestment **correction** remains T-115 #130.

### 5 — correction register and checkpoints

The frontend search for `correction_of_transaction_id` finds generated schema
types, with no register grouping consumer. ADR/plan grouping is a missing
presentation requirement. The shared `runInvestmentJournalsWithGuardTx` invokes
checkpoint invalidation once for each journal; replacement previews similarly
merge separate journal impacts. The conservative guard is not evidence of a
silent reconciliation bypass, but it prompts/invalidate more than necessary.

[T-120 #135](https://github.com/sergeyfarin/rekenraam/issues/135) requires one
backend-composed chain read model and one compound impact calculation. Netting
by account/commodity/date alone cannot prove zero impact at a checkpoint with a
same-day account-sequence boundary; settlement/fee entries also have distinct
financial dates. Check the combined exact change at **each checkpoint boundary**,
and guard changed holding quantity even if cash nets to zero. Preserve normal
reconciled-posting edit rules and define how new offsetting rows appear in
reconciliation. This is a service/repository contract change, not simply
suppressing frontend warnings.

### 6 — preview feasibility fix

Before the fix `TradeReconciliationImpact(buy)` built `buyPlan` and checked
journal checkpoints without creating/replaying the prospective lot. The new
`TestBuyPreviewRejectsChangedInternalTransferBasisWithoutWriting` first proves
that commit refuses a carried-basis dependency, then failed because preview
returned success. It uses a rounding remainder after a partial FIFO sale;
an earlier buy changes which opening funds that sale, affecting the linked
transfer's basis.

`SimulateBuy` now selects the shared single-journal preview writer and runs
exactly the same lot creation, replay, prices and checkpoint effects as commit,
then rolls back. Preview authorizes temporary checkpoint simulation only;
commit still requires its own explicit override and rechecks dependencies.
Error mapping is shared with commit. Repeated successful previews and refused
previews compare complete durable-row snapshots, not merely row counts; no
temporary IDs escape. This resolves feasibility only, **not item 1's disclosure**.

### 7 — duplicate evidence/read selection

`investment_lot_facts_same_book` requires account, instrument, side, opening
date, quantity, basis and currency to equal `investment_lots`. The duplicate
opening values are real; columns are not literally identical (`consideration`
versus `cost_basis`, plus operation/audit linkage). Repeated effective-operation
and latest-revision selection exists in replay, gains and self-check readers.
The exact “13 in 7 files” count depends on the search pattern and is not used as
an acceptance criterion.

[T-124 #139](https://github.com/sergeyfarin/rekenraam/issues/139) calls for shared
effective selectors and a mapped opening-facts consolidation. Audit self-check
must still examine **all** original and superseded allocations; replacing every
reader with an effective-only view would hide historical corruption. Remaining
transaction/version links can identify a decision's specific journal in a
compound action; they are not automatically redundant with the parent operation.
Retain justified provenance, remove unexplained duplicate authority, and cover
baseline/checksum/seed/export/restore together.

### 8 — replay scope

Replay is currently position-scoped. `simulateInvestmentReplayTx` includes
transfer-out depletion and rejects changed immutable carried basis. The existing
`TestInternalTransferDepletionSurvivesLaterSaleReversalReplay` contradicts the
claim that every cross-account dependency is refused: unchanged transfer effects
can be replayed. Propagating changed source basis into destination lots and
subsequent sales is still missing.

A whole-book rebuild may simplify orchestration but does not remove invalid
specific-lot elections, missing basis evidence, same-day lineage ordering or
canonical bridge adjustments. [T-124 #139](https://github.com/sergeyfarin/rekenraam/issues/139)
requires a tested choice between affected dependency closure and whole-book
rebuild before outbound/compound commands. No speculative implementation or ADR
change is adopted in this review. The current named refusal remains until
cross-position revisions can commit atomically.

### 9 — schema leftovers

The consolidated baseline still has `ALTER TABLE` additions and recreates
`current_account_versions`. That is readability debt, not proof a fresh schema
is incorrect. `investment_operation_dates` has `(operation_id, date_role)` as its
key and no repeated-date sequence, contrary to the planned compound contract.
Disposal decisions still require journal transaction/version IDs.

These are gates in [T-124 #139](https://github.com/sergeyfarin/rekenraam/issues/139).
Add repeated date cardinality before a compound kind consumes it. A decision's
journal link should not be dropped just because the operation supports multiple
journals. Any baseline change must update fixtures/checksum/equivalence and
export/restore under ADR 0013; no migration is rewritten merely for style here.

### 10 — finite correction scope

Live #99 had a short P0 body with no measurable acceptance; its plan accumulated
4a–4ai and kept provider cancellation plus unrelated operation families as gates.
The corrected scope is delivered long buy/sale reversal/replacement, dependent
recorded-method replay, backdated buys, manual corrections of source-linked
trades, Trading 212 BUY/SALE quantity/net source revision acceptance and shared
writer atomicity. Remaining gain safety is explicitly tracked separately, so
closing #99 does not claim all investment corrections are complete.

New families: [gain impact #129](https://github.com/sergeyfarin/rekenraam/issues/129),
[dividends #130](https://github.com/sergeyfarin/rekenraam/issues/130),
[trade fields #131](https://github.com/sergeyfarin/rekenraam/issues/131),
[backdating #132](https://github.com/sergeyfarin/rekenraam/issues/132),
[write-offs #133](https://github.com/sergeyfarin/rekenraam/issues/133),
[transfers #134](https://github.com/sergeyfarin/rekenraam/issues/134),
[register/reconciliation #135](https://github.com/sergeyfarin/rekenraam/issues/135),
[verified provider cancellation #136](https://github.com/sergeyfarin/rekenraam/issues/136).
Each has acceptance and named test scenarios. Local IDs continue T-114 onward;
use both `T-nn` and `#nn`, rather than skipping IDs to avoid a coincidental
number match with an older GitHub issue.

The current official [Trading 212 OpenAPI](https://docs.trading212.com/_bundle/api.json?download=)
Fill type enumeration has corporate actions/FOP correction but no explicit
execution-cancellation type. Its order statuses include cancellation, which
alone does not reverse a previously executed fill. This is a bounded negative
finding about the reviewed schema, not a guarantee that the provider never
revises executions. #136 is P3/blocked pending verified semantics and evidence.

### 11 — documentation and issue hygiene

The active plan was 1,131 lines, with 100-plus lines before its outcome section.
`roadmap.md` simultaneously left compound writer convergence open and called it
complete/P0 remaining. `todo.md` told agents to fence all backdated writes. Live
#120 still named closed #125 as an active gate.

The plan now keeps the contract, short status table, delivery gates and common
acceptance cases. Former slice history is preserved in a clearly superseded
[dated snapshot](investment-operation-progress-2026-10-01.md), rather than
relabelled as current requirements. Roadmap, todo, slice-5 sequence, issue index
and #99/#114/#120 bodies agree. Existing issue comments remain audit history;
new resolution comments link to evidence instead of copying the full ledger.

### 12 — test gate performance

The [2026-09-30 runtime review](test-runtime-review-2026-09-30.md) records a full
race profile, duplication review, 60 financial matrix cases retained and newly
parallelized, and application runtime 598.347 → 513.745 seconds. API and DB
remained about 200 and 516 seconds, so the complete serial gate was still around
20 minutes. Raising the deadline was authorized after that review; claiming no
optimization occurred is incorrect. The 45-second non-race comparison supplied
in the comments was not independently benchmarked here under equal resources.

More isolated-test parallelism and bounded package concurrency merit controlled
measurement. Isolated SQLite files alone do not prove absence of process/global
state or environment hazards. [T-125 #140](https://github.com/sergeyfarin/rekenraam/issues/140)
requires those audits and before/after full-gate measurements. Restricting race
coverage to a guessed package list is rejected without evidence and a separate
decision. The wrapper/CI/default race gate is unchanged in this review.

## Validation and delivery

- The plain-buy dependency regression failed before the implementation fix:
  commit refused the changed transfer basis while checkpoint preview succeeded.
- Review-scoped preview, temporal and transfer regressions passed (1.098 seconds,
  non-race). The temporary reinvestment-backdating probe passed (0.263 seconds)
  and was removed; no diagnostic file is shipped.
- `./scripts/test-backend.sh` passed formatting, vet and the **entire race suite**.
  API: 204.260 seconds; application: 553.964 seconds; database: 513.115 seconds;
  command: 65.586 seconds; application runtime: 11.719 seconds. Other packages
  passed or reused valid Go test cache entries. These are validation timings,
  not a controlled before/after performance benchmark.
- The final named buy-preview regressions also passed separately after the
  exact 50.00 → 140.00 scenario was pinned (race run: 6.660 seconds). The
  recurring preview bug checklist now includes ordinary replaying entry paths.
  Documentation whitespace and local
  Markdown targets were checked. GitHub #129–#140 and their priority labels,
  including #136's evidence block, were read back and verified.
- Commit/push is scoped to the preview fix, feature boundary, review disposition
  and synchronized plans. No schema, tax-profile or race-coverage change is
  included. No unrelated user changes were present or included.

Preview implementation: [a19b1344](https://github.com/sergeyfarin/rekenraam/commit/a19b1344).
