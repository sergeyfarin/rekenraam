# Ledger invariants — investment subledger

Part of the `ledger-invariants` skill; its `SKILL.md` holds the rules every
money-touching change obeys. This file is the per-family contract for the
investment subledger. Read it before changing lots, disposals, basis
knowledge, transfers, corporate actions, shorts, dividends, payments in lieu,
replay, or any reader of investment history. Authority: ADRs 0012, 0013, 0019
and the plans under `docs/plans/` named in each rule. Shipped bugs in this
area, with their named regressions, are in
`.claude/skills/validate-and-ship/bug-classes/investments.md`.

## Lots and disposal snapshots

- Every lot requires its immutable opening operation and source journal
  (`operation_id`/`source_transaction_id` are `NOT NULL`, #148), including test
  fixtures. Journal-free operations never open lots.
- A committed disposal snapshots the resolved method, resolution tier,
  policy/profile version, allocations, and audit provenance. Later defaults do
  not reinterpret it. Current lot state is a rebuildable projection and must
  conserve quantity and basis across sequential events.
- Original lot `cost_basis_value` is immutable acquisition evidence. Average cost
  may redistribute only remaining-basis projection state. With one operational
  projection, switching into or out of average cost after a partial disposal is
  rejected until the position closes.

## Basis knowledge and its corrections

- Basis knowledge is explicit (`known`/`unknown`, NULL amount pairs; known
  zero stays numeric). Sales, write-offs, cash in lieu and in-kind transfers
  may consume unknown basis (`AdmitUnknownBasis`); a disposal's result is unresolved — NULL with
  `basis_knowledge`, never a gain against a partial or zero basis — and an
  outbound with any unknown link posts no bridge. Unknown inbound basis is
  resolved only by an appended sourced resolution (complete omitted bridge at
  the transfer date); transfer-link revisions may go unknown→known, never
  known→unknown. Disposal/action results may return to unknown after resolution
  withdrawal, with gain acknowledgement.
  A resolution is corrected by its own replace/reverse commands (#168), with
  at most one effective resolution per link. A reversal that would return a
  known outbound or onward link to unknown refuses with that operation named.
  A sourced known zero is a journal-free operation (audit-only header), never
  a zero journal; sourced zero-delta splits also have a declared exemption
  (#146). A split recorded without holdings remains a dated ratio intent;
  entitled backdated acquisitions append its effects and guarded adjustment
  journals. Per-split journal/effect conservation must still pass. Operation-based
  split correction (#196) can cross zero: inverse and successor headers share
  one command audit and post only their nonzero deltas. A journal-free reversal
  still retires the source, and every nonzero inverse remains journal-linked.
  Cash-in-lieu and import identities pin the split operation without inventing
  a primary transaction.
  #392 keeps that original parent as provenance across all four corporate-action
  families. Validate corrections through actual disposal replay, gain review and
  checkpoint commitments; parent standing alone never vetoes them. #394 applies
  the same rule to entry and dated lot selection against explicitly selected
  historical parents. Keep original type/book/holding/instrument/date integrity;
  a replacement retains that exact reference. Current ancestry/mismatch is
  read-only attribution; missing/broken facts remain integrity failures. Never drop quantity or specific-lot guards.
  Those four families' correction preparation has one owner (#401,
  `app/investment_corporate_action_correction.go`): admission, the
  already-corrected and imported-lineage fences and the posted-original check
  run after each family's typed lookup; the writer rechecks them in its
  transaction. Change a shared correction policy there, not per family.
  Return-of-capital cash/entitlement stay exact; unknown reduction and excess
  remain NULL. Known explicit entitlement beside independent unknown lots is
  supported; an established unknown average pool never supplies a known rate
  from its known subset. See the #169 non-sale plan.

## Transfers

- Internal transfers follow the source's method lock, else its default: lots
  for individual-lot sources, a pooled quantity for average-cost sources, which
  reuses the sale's pool depletion and conserves basis exactly (T-123). An
  average-cost source opens one `pooled_lot` destination lot by default whose
  basis, latest-unit original date and source depletions replay may revise;
  its lot ID and quantity never change (T-135). Read a transferred lot's basis
  or original date through `effective_investment_transfer_links`.
- Transfers are corrected by their own commands (T-119, ADR 0013 *Transfer
  Correction Refinement*): a reversal inverts the whole journal (bridge
  included) and replays source then destination, so removed destination lots
  seed propagation. Revisions of a reversed transfer are evidence only; read
  current state through revisions of effective operations. A replacement computes
  its source depletion with a subject replay intent at the correction-root
  slot (as a backdated split does), never from today's lot state. A new
  internal transfer behind a later depletion of either holding uses the same
  subject intent at its own slot and then replays both holdings (#167).
  A share exchange (#179) is corrected by its own reversal and replacement
  the same way. A backdated exchange takes the whole holding open at its
  slot through the same subject intent.

## Spin-offs and cash-and-stock mergers

- A spin-off (#180) keeps every parent lot's units and moves remaining basis
  × an exact sourced fraction (truncated at the allocation scale, remainder
  stays) to one new lot per parent lot: the parent's `basis_reduction` equals
  the new lot's opening basis, so basis is conserved per lot. Unknown stays
  unknown on both sides. It is corrected by its own reversal and replacement
  (#183), and a backdated spin-off entitles every lot open at its slot
  through a subject `spin_off` replay intent, as a backdated exchange does.
- A cash-and-stock merger (#181) is an exchange (`transfer_kind
  'exchange'`, operation `cash_and_stock_merger`) whose cash recovers an
  explicit elected fraction (0..1) of each lot's basis, truncated at the
  allocation scale: a quantity-0 `disposal` event of `−a` before the
  exchange's `transfer_out` of the rest, so basis is conserved per lot. The
  cash is one quantity-0 long decision on the old position whose proceeds are
  the cash, posted on its payable date (never earlier) in the lots' cost
  currency. The app never picks a jurisdiction's rule. It is corrected by
  its own reversal (both journal entries inverted; its cash result leaves
  the gain set as removed) and replacement (#184), and a backdated merger
  realizes and takes every lot open at its slot through a subject
  `exchange_out` intent carrying the election. Cash in lieu
  may settle a split's, exchange's, merger's or spin-off's fraction
  (`settled_operation_id`).

## Short positions

- Named shorts (#173): a short lot holds owed units at a positive quantity
  and its exact opening proceeds in the basis columns; a cover's decision is
  `position_side = 'short'` with signed (negative) proceeds, so its result is
  disposed opening proceeds + proceeds. One holding is never long and short
  of an instrument over overlapping dates (`ErrPositionSideConflict`); every
  lot query keys by side. Short replay, backdating and native correction
  (#175) thread the side through `simulateInvestmentReplaySideTx`; shorts
  have only openings and covers. See the operation plan's *Short positions
  (#103)* section.

## Dividends, withholding and payments in lieu

- Cash dividends and standalone withholding (#393) keep payment shape separate
  from nullable payer. Reuse nullable-field/patch presence: entry omission/null
  unassigned; replacement omission preserves, null clears, value assigns. A
  neutral payer repair changes no cash/tax/lots/checkpoints. Unknown tax is never
  invented; standalone tax remains tax without income. Preserve source identity
  across original/inverse/replacement journals for both cash families, raw
  evidence and settled suggestions. Compare retained source claims read-only;
  do not add holdings authorization or a review queue for manual unassigned cash.
  CSV income permits absent payer; asserted unresolved codes still need review.
  Their date, cash account and currency are correctable financial inputs
  (#395): the inverse stays on the original's date/accounts, the replacement
  carries the corrected facts through the shared writer's boundary review, and
  an imported lineage discloses `source_differences` against its root, never
  vetoing. Payment shape stays fixed.
- A short holder's payment in lieu (#255, refined by #388/#389 and ADR 0019)
  is lot-free cash/expense/recovery. Holding, instrument and ex-date are
  optional auditable attribution, not write authorization. Entry omission/null
  is unassigned; replacement omission preserves, null clears and values replace.
  Use the existing immutable correction chain; attribution-only repair nets to
  zero at every checkpoint and changes no lots/gains. Its date, cash account
  and currency are correctable financial inputs as for cash dividends (#395). Historical support,
  chronology and source disagreement are read-only diagnostics. Preserve source
  identity/evidence across repair. Ordinary writes never enumerate/replay all
  payments; real cover quantities, posting roles, recovery constraints and
  financial reconciliation guards remain active.
- A reinvested dividend's date, holding, instrument and cost currency are
  correctable through the acquisition writer's old/new-position replay
  (#395, T-116); it stays a `reinvested_dividend` opening, never a cash buy.

## Effective reads and replay

- Effective reads use the views `effective_investment_operations`,
  `latest_investment_disposal_revisions`, `latest_investment_split_revisions`,
  `effective_investment_lot_events` and `effective_investment_transfer_links`;
  never re-inline those predicates, and
  never route an audit through them. `effective_investment_lot_events` only
  removes superseded events; folds of lot history read
  `db.EffectiveLotMovements` (#189), which adds every family's latest
  revision back. `TestEffectiveHistoryReadersUseTheOwner` refuses other
  direct readers outside its reasoned allow-list (validate-and-ship item 34). Cross-position replay is scoped to the
  dependency closure (`InvestmentReplayClosure`, ADR 0013 T-124 refinement):
  `persistInvestmentReplayProjectionTx` appends transfer link revisions and
  replays the downstream closure once as a merged dated stream (T-132, T-140). Any new reader of a
  transferred lot's basis must use the latest link revision, not the link.
