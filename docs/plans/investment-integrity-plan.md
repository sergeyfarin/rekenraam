# R12a Investment Integrity Correction Plan

Status: current. Opened 2026-08-29 by the ledger/subledger boundary review.

This is a correctness gate over the shipped R12 investment feature. It does not
add multi-basis tax reporting. ADR 0012 governs the boundary; backlog T-74–T-76
carry the defect evidence and named acceptance requirements; the completed
`docs/plans/investments-plan.md` remains the historical R12 design record.

## Outcome

An investment transaction and its subledger consequences have one lifecycle.
Average-cost basis conserves exactly through a sequence of sales. Every disposal
can explain the resolved policy used at commit time. The self-check detects a
disagreement in either direction. Only after those statements are proven does R9
resume and `implemented.md` restore the affected capabilities to complete.

## Non-goals

- Alternative FIFO/LIFO/average comparisons, tax-jurisdiction rules, and
  reproducible as-of gains reports. R18 owns those read-side projections.
- Corporate-action lot mutation and return of capital. R16 owns them, using the
  corrected event/lifecycle contract from this slice.
- Quote providers and instrument-price refresh. R17 owns them.
- Automatically posting realized gain, unrealized revaluation, or tax liability.
  ADR 0012 requires a future explicit linked accounting workflow.

## Slice 1 — Immediate mutation fence and diagnostic coverage (T-75a)

Before changing the representation, stop creating new divergence.

- Add a repository/service query that identifies a transaction referenced by an
  investment lot or lot event.
- Reject generic financial update, correction, void, unvoid, soft-delete, and
  restore for such transactions with one stable API error. The backend is the
  trust boundary; hiding UI actions is only the matching presentation.
- Decide descriptive-only editing by explicit field classification. If the
  existing generic editor cannot prove the change is non-financial, reject it.
- Replace the unsafe transaction-detail actions with a link/explanation for the
  investment-native workflow. Until Slice 4 exists, the honest state is
  “correction not yet supported,” not a button that corrupts lots.
- Make `lotReconciliationCheck` compare the union of posted holding keys and lot
  keys. Include all-lots-closed, lot-only, journal-only, negative, over-consumed,
  event-total, and basis-conservation failures.

**Exit:** named service/API/UI tests cover a buy, a partial sell, and a
fully-closing sell; every unsafe action is refused before any row changes; the
self-check detects both directions of disagreement.

## Slice 2 — Conserved average-cost projection (T-74)

- Replace the shipped split accounting in which disposal events use a pool rate
  while remaining lots use their own rate.
- Preserve immutable acquisition and disposal facts and retain lot identity for
  FIFO, LIFO, and specific-lot projections.
- Choose and document one rebuildable materialization: redistribute the exact
  post-sale pool basis over surviving lot rows, or introduce an explicit
  operational pool state. In either design, residual assignment is deterministic,
  exact, and cannot make a remaining lot negative.
- Rebuild unrealized gain and future disposal inputs from the same conserved
  remainder. Do not maintain a second “reported basis” that disagrees with the
  position basis.

**Exit:** the T-74 sequence test proves after each partial sale that disposed plus
remaining basis equals the pre-sale pool; closing the position makes cumulative
disposed basis equal cumulative acquired basis exactly. FIFO/LIFO/specific-lot
regressions remain green.

## Slice 3 — Durable disposal decision and policy provenance (T-76)

- Introduce a typed disposal-decision/election record linked to transaction,
  transaction version, lot events, and audit event.
- Snapshot resolved method, resolution tier, policy/profile identity and version
  or effective state, allocations, and exact basis totals. Do not make generic
  metadata JSON authoritative.
- Version global cost-basis policy rather than relying only on an in-place updated
  profile. Account defaults already live in account versions; persist the exact
  version/effective state used.
- Return the committed decision in the sell/write-off response and include it in
  the durable structured export. Preview and commit use the same resolver and
  return the same decision contract.
- Preserve broker-reported cost/basis as separately attributed source evidence.
  It does not silently select the book policy.

**Exit:** changing defaults after a sale leaves the historical decision fully
explainable; every resolution tier and specific-lot allocation round-trips through
API and export; fresh-schema and self-check tests cover the new relations.

## Slice 4 — Investment-native correction lifecycle (T-75b)

Build on the fence rather than removing it.

- Define correction/reversal commands over investment operations, not arbitrary
  posting edits. A command plans both journal and subledger consequences.
- Append corrective/reversal lot events and new transaction versions or linked
  corrective transactions; never delete original lot or election history.
- Apply reconciliation impact before writing and commit journal, subledger,
  audit attribution, import identity consequences, and any dependent
  trade-implied-price retirement/correction in one SQLite transaction.
- Recompute the operational projection deterministically from the corrected event
  history. Refuse a correction that would make a later disposal impossible unless
  the same operation corrects the dependent chain explicitly.
- Keep generic investment mutation fenced after this ships; the supported path is
  the domain command.

**Exit:** named tests correct and reverse a buy, partial sale, full sale, and
write-off with later dependent activity. Success leaves journal, lots, gains,
prices, audit, import identity, and reconciliation coherent; injected failures at
each post-write boundary roll everything back.

## Slice 5 — Acceptance review and handoff

Run the backend, frontend, build, and focused browser journeys required by the
changed surfaces. Exercise one imported and one manually entered investment from
buy through sequential sales, correction, export, self-check, and gains.

Reconcile the result in `implemented.md`, `roadmap.md`, `todo.md`, and backlog
T-74–T-76. Record any deliberately deferred projection question in R18 rather
than leaving it implicit here. R9 slices 2–6 resume only after this review closes.

## Acceptance matrix

| Invariant | Required proof |
|---|---|
| Generic paths cannot strand lots | Service/API tests for update, correct, void/unvoid, soft-delete/restore on buy and sell |
| Journal and subledger mutate atomically | Failure-injection tests around transaction, lot event, election, audit, import identity, and price writes |
| Average cost conserves | Sequential partial-sale and final-close exact-basis tests |
| Historical method is explainable | Change defaults after disposal; API/export still name original method, tier, and policy version |
| Self-check is symmetric | Lot-only, journal-only, all-lots-closed, event-total, and basis mismatch fixtures |
| Reconciliation remains trustworthy | Impact preview, required override, checkpoint invalidation, and rollback tests |
| Alternative reporting remains read-only | No R12a path mutates facts merely to compare a method |
