# R12a Investment Integrity Correction Plan

Status: R12a complete 2026-08-30. T-76 and T-75b remain scheduled follow-ups.

This plan records the immediate correctness gate and its scheduled investment
follow-ups. It does not add multi-basis tax reporting. ADR 0012 governs the
boundary; backlog T-74–T-76 carry the evidence and named acceptance requirements;
the completed `docs/plans/investments-plan.md` remains the historical R12 design
record.

## Outcome

R12a makes unsafe generic investment mutation impossible and makes average-cost
basis conserve exactly through a sequence of sales. The self-check detects a
disagreement in either direction. Those correctness statements are the gate for
R9 to resume. Durable disposal provenance (T-76) follows before v0.1/schema
freeze and before R16/R18; investment-native correction (T-75b) belongs to R16.

## Non-goals

- Alternative FIFO/LIFO/average comparisons, tax-jurisdiction rules, and
  reproducible as-of gains reports. R18 owns those read-side projections.
- Corporate-action lot mutation and return of capital. R16 owns them, using the
  corrected event/lifecycle contract from this slice.
- Quote providers and instrument-price refresh. R17 owns them.
- Automatically posting realized gain, unrealized revaluation, or tax liability.
  ADR 0012 requires a future explicit linked accounting workflow.

## Existing-data policy

This is pre-v0.1 software with no supported production migration promise. Do not
write a repair migration that guesses which historical average-cost remainder or
edited journal version was intended. After Slices 1 and 2, reset and reimport
disposable development books. If any non-disposable book exists, stop before
resetting it, export its journal and investment events, and assess it explicitly;
repair must be evidence-led and recorded as a separate operation.

## Slice 1 — Immediate mutation fence and diagnostic coverage (T-75a) — complete

Before changing the representation, stop creating new divergence.

- Add a repository/service query that identifies a transaction referenced by an
  investment lot or lot event.
- Reject generic financial update, correction, void, unvoid, soft-delete, and
  restore for such transactions with one stable API error. The backend is the
  trust boundary; hiding UI actions is only the matching presentation.
- Reject any investment buy, sell, dividend, reinvested dividend, or write-off
  whose requested status is not `posted`. The current service mutates lots even
  for a caller-supplied `draft`, although drafts are outside the ledger. A future
  investment draft producer must define lot activation at promotion and reversal
  at discard before this restriction can be relaxed.
- Decide descriptive-only editing by explicit field classification. If the
  existing generic editor cannot prove the change is non-financial, reject it.
- Replace the unsafe transaction-detail actions with a link/explanation for the
  investment-native workflow. Until the R16 follow-up exists, the honest state is
  “correction not yet supported,” not a button that corrupts lots.
- Make `lotReconciliationCheck` compare the union of posted holding keys and lot
  keys. Include all-lots-closed, lot-only, journal-only, negative, over-consumed,
  event-total, and basis-conservation failures. The shipped check compares only
  quantities and cannot detect T-74.
- Prove realized-gain proceeds cannot drift: the current read joins disposal
  events to `current_transaction_versions`, so a generic edit of the sell cash
  posting otherwise rewrites proceeds while disposed basis remains frozen.

**Exit:** named service/API/UI tests cover a buy, a partial sell, and a
fully-closing sell; non-posted investment creation and every unsafe action are
refused before any row changes; the self-check detects both directions of
disagreement and basis/event corruption.

## Slice 2 — Conserved average-cost projection (T-74) — complete

- Replace the shipped split accounting in which disposal events use a pool rate
  while remaining lots use their own rate.
- Preserve immutable acquisition and disposal facts and retain lot identity for
  FIFO, LIFO, and specific-lot projections.
- Treat `investment_lots.cost_basis_value` as immutable original acquisition
  basis. Never rewrite it to make average cost balance; change only a rebuildable
  remaining-basis projection.
- Retain a materialized current projection rather than deriving every operational
  read from the full event stream. Sell preview/write, positions, export, and
  self-check all need the same repeated bounded state. Immutable events remain
  the rebuild source, and a rebuild-equivalence test prevents the materialization
  from becoming a second authority.
- Choose and document one rebuildable materialization: redistribute the exact
  post-sale pool basis over surviving projection rows, or introduce an explicit
  operational pool state. In either design, residual assignment is deterministic,
  exact, and cannot make a remaining lot negative.
- Add the minimal method epoch/lock needed by the one operational projection.
  Once an open position has a partial disposal under `average_cost`, reject a
  switch out; reject switching into `average_cost` after a partial disposal under
  another method. Closing the position ends the epoch. FIFO/LIFO/specific-lot may
  still select their preserved original lots. T-76 later records the full typed
  decision on every disposal.
- Rebuild unrealized gain and future disposal inputs from the same conserved
  remainder. Do not maintain a second “reported basis” that disagrees with the
  position basis.

**Exit:** the T-74 sequence test proves after each partial sale that disposed plus
remaining basis equals the pre-sale pool; closing the position makes cumulative
disposed basis equal cumulative acquired basis exactly. FIFO/LIFO/specific-lot
regressions remain green; rebuilding from events reproduces the materialized
state; forbidden mid-position method switches fail before mutation.

## Slice 3 — R12a acceptance and R9 handoff — complete

Run the backend, frontend, build, and focused browser journeys required by the
changed surfaces. Exercise imported and manually entered investments through a
buy and sequential sales; cover refused generic mutations, draft creation,
self-check, gains, audit, and reconciliation.

Reconcile `implemented.md`, `roadmap.md`, `todo.md`, and backlog T-74/T-75.
R9 slices 2–6 resume only after this review closes. Do not wait for T-76 or
T-75b to resume R9: recurring v1 templates contain only ordinary and transfer
transactions, so they cannot create investment lots.

## Scheduled follow-up — Durable disposal decision and policy provenance (T-76)

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

This follow-up is required before v0.1/schema freeze and before R16 or R18, but
is not part of the R12a/R9 gate.

## R16 follow-up — Investment-native correction lifecycle (T-75b)

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

## Acceptance matrix

| Invariant | Required proof |
|---|---|
| Generic paths cannot strand lots | Service/API tests for update, correct, void/unvoid, soft-delete/restore on buy and sell |
| Non-posted investments cannot mutate lots | Service/API tests reject every investment write with `status=draft` before mutation |
| Average cost conserves | Sequential partial-sale and final-close exact-basis tests |
| Self-check is symmetric | Lot-only, journal-only, all-lots-closed, event-total, and basis mismatch fixtures |
| Reconciliation remains trustworthy | Impact preview, required override, checkpoint invalidation, and rollback tests |
| Alternative reporting remains read-only | No R12a path mutates facts merely to compare a method |

T-76 additionally requires the historical-method/API/export proof described in
its exit criteria. T-75b additionally requires failure-injection proof that
journal, lot event, decision, audit, import identity, price, and reconciliation
changes commit or roll back together.
