# Bug classes — Investments

Part of the `validate-and-ship` review checklist. Lots, disposals, replay,
corrections, corporate actions, gains, self-check, valuation quotes. Every
entry shipped as a real bug in this repo. Numbers are stable IDs cited from
code comments and docs; they are not an order and are never reused.

## 17. A sound current projection hiding damaged disposal snapshots

Original allocation quantity, basis and proceeds mutations passed self-check
because current lots were compared with lot events, while replayed allocations
only contributed quantity and basis to that projection. Proceeds corruption
and damage to superseded revisions could pass silently. Check every original
and revision allocation set independently against its snapshot totals,
including missing allocations, before using effective evidence for a
projection. Negative proceeds are valid; nonpositive allocated quantity and
negative basis are not. Named regression:
`TestSelfCheckDetectsDisposalAllocationConservationDamage`.

## 18. Offsetting disposal errors hiding behind sound group totals

Combined proceeds and per-lot conservation can both pass after two decisions
and their allocation proceeds are changed in opposite directions. Immutable
decision-to-clearing portions must conserve each decision and each pinned
posting independently. Check provenance even for equal-valued journals. Named
regression: `TestSelfCheckDetectsOffsettingDisposalProceedsDamage`.

## 19. Unknown basis converted to a numeric zero

Preserve NULL coefficient and scale with explicit knowledge, through read APIs
and CSV. One unknown lot makes the position's basis/gain unavailable; quantity
and independently priced market value stay available. Never use known-only
numeric fields without checking knowledge. Keep quantity self-check active and
refuse unresolved inputs before known-basis pooling/disposal/range arithmetic.
Named regression: `TestUnknownProjectedBasisDoesNotBecomeZeroGain`.

## 20. Source identity lost on an imported correction descendant

Import audit origin marks the replacement as imported, while committed
identity effects stay on the original fill. A direct-only identity lookup
incorrectly fences later sale corrections and hides effective specific-lot
elections. Read committed source provenance through immutable correction
ancestry, matching the writer's lineage guard; never copy or rebind original
effects. Named regression:
`TestCorrectTrading212SalePreservesSpecificElectionWithoutGuessingQuantityChanges`.

## 22. A preview reporting checkpoints for an impossible replay

Buy replacement and plain-buy previews validated journal shape without testing
dependent lots. Audit every replaying entry path, including admitted
reinvestment and imports, rather than only correction commands. Run the
proposed domain write in a rolled-back path before reporting impact, with the
same ordering, lot lineage and elections as commit. Never accept staged source
evidence or expose temporary IDs. Snapshot durable rows after failure and
repeated successful previews; the actual write still rechecks dependencies and
reconciliation. Named regression:
`TestBuyReplacementPreviewRejectsDependentDisposalWithoutWriting` and
`TestBuyPreviewMatchesCommitWhenTransferBasisPropagates` and
`TestReinvestmentPreviewPropagatesTransferBasisWithoutWriting` (since T-132 a
changed transfer basis propagates rather than refuses). Return the writer’s
actual invalidated checkpoint set, including later checkpoints; a second
latest-boundary calculation can underreport it. Pin multiple checkpoints in
`TestBuyPreviewReplaysAndRollsBackReconciledHistory`. Shared reconciliation
resolution must report all checkpoints the writer invalidates, once each,
rather than only the latest checkpoint per candidate. Fix the common selector
when generic transactions and other investment previews share it (T-127 #142).
Named preview/commit pairs:
`TestCreatePreviewReportsEveryCheckpointCommitInvalidates` and
`TestSalePreviewReportsEveryCheckpointCommitInvalidates`. Test each checkpoint
against its own `(date, sequence)` boundary through
`activeCheckpointRefsAtOrAfter`; judging only the latest checkpoint let a
same-day reorder across an earlier boundary through without override (T-120
#135, `TestPostingMoveAcrossEarlierSameDayCheckpointRequiresOverride`). A
multi-journal investment command is guarded once on its combined delta read
from every posting under its audit event; never re-add a per-journal guard or
an up-front per-journal rejection, which refuses an inverse its replacement
cancels
(`TestQuantityOnlyBuyCorrectionPreservesCashCheckpointAndGuardsHolding`). Gain
disclosure now also runs through these writer previews (T-114 #129 / T-126
#141).

## 23. Replay silently restating committed gains

A backdated or corrective command can change an earlier sale's basis/gain
without touching any reconciled balance, so the reconciliation guard never
fires. Any new replaying command must set `db.GainImpactPolicy` on its first
journal, return `gain_impact` from its preview, and accept
`gain_impact_acknowledgement` on commit; the writer recomputes and binds the
set in-transaction. Test the changed, empty, stale and late-rollback cases. A
preview must run the command's actual writer — reversal previews that planned
only the inverse journal missed both impossible replays and gain changes
(T-126). Comparing only inside replay persistence misses reversed/superseded
disposals. Named regressions:
`TestBuyGainImpactDisclosesFIFORevisionAndRequiresExactAcknowledgement`,
`TestBuyGainImpactRejectsAcknowledgementOfAnEarlierChangeSet`,
`TestGainImpactSnapshotDisclosesRemovedAndReplacedDisposals`.

## 24. A new replay intent kind that moves quantity without pinning its journal

Replay rebuilds lot state from intents, but a posted journal leg does not move
with it. A split replayed under a corrected history can multiply a different
number of shares than its posted `H +d`; holdings and lots then disagree. Each
journal-bearing intent must either refuse with itself named or post the
difference as an adjustment journal linked to its operation and revision under
the command's audit and checkpoint guard (T-129 does this for splits, ADR 0013
refinement); its per-lot replay output must be revisioned, and self-check must
reconcile the operation's journals with its effective effects, not the
originals. Once such an operation can be reversed or replaced, every reader of
its latest revision must also filter to effective operations, and the
self-check must count the successor's inverse journal (T-129). Named
regressions: `TestQuantityCorrectionBeforeSplitPostsAdjustmentJournal`,
`TestSplitReversalInvertsPrimaryPlusAdjustmentDelta`,
`TestReversalsBeforeSplitPostAdjustmentJournals`,
`TestEarlierAcquisitionBasisCorrectionReplaysThroughSplit`. A sourced split
initially without eligible lots is journal-free (#146), but must remain in
replay and activity-date admission. Revised movements anchor to its sourced
fact when there is no original event. Named cases:
`TestBackdatedAcquisitionEntitledToJournalFreeSplitPostsAdjustment`,
`TestJournalFreeSplitAdjustmentUsesGainAndCheckpointGuards`,
`TestJournalFreeSplitExemptionDoesNotHideMissingJournals`. Operation-addressed
correction (#196) covers both journal shapes: never promote an adjustment to a
primary, fabricate a zero inverse, or drop a journal-free successor's
source/memo. Keep source identity through cash-in-lieu correction and import
dedupe. Named cases: `TestSplitOperationReplacementCrossesZeroBothWays`,
`TestJournalFreeSplitReversalBeforeAndAfterEntitlement`,
`TestJournalFreeSplitReversalExemptionRequiresNonzeroInverseJournal`.

## 25. A replay branch that omits a side effect its writer performs

Replay re-derives a position from intents, so anything the commit path writes
besides lot effects (the method-family lock, a link's original date, a
revision row) must be re-derived by the matching replay branch, or the first
replay of that position silently drops it. The selected-lots transfer branch
cleared the source's individual-lot lock this way until the
`investment_replay_equivalence` self-check (T-134) found it. New commands or
intent kinds: assert the replay check passes after a replay of the touched
positions (the shared self-check pass helpers do). Named regression:
`TestReplayKeepsSelectedLotTransferMethodLock`.

## 26. Revision rows read without asking whether their operation is still effective

`latest_*_revisions` views select the newest revision per decision or link,
not per *effective* operation. Once an operation can be reversed, its
revisions stay as evidence and must not describe current state: self-check
counted a reversed transfer's link revision as a live lot event until T-119
joined `effective_investment_operations`. Any new reader of a latest revision
for current state joins it too. Named regression:
`TestReverseRevisedInternalTransferKeepsRevisionAsEvidence`.

## 30. Position identity dropping side

`investment_lots`, decisions and the method-family lock carry `position_side`.
Before #173 every lot was long, so the disposal engine selected lots by
account, instrument and currency only; once short lots exist, any such query
lets a long sale consume a short lot (or a cover a long one). Every lot
selection, method-family lock, basis-range guard and current-position read
filters or keys by side; synthetic self-check events take the side of their
lot. Named regressions: `TestShortCoverRefusesOverCoverAndOtherSideLots`,
`TestPositionSideConflictRefusesOverlappingSides`.

## 31. Transfer-link quantity read as the destination's quantity, or its instrument read as the destination's

A share exchange (#177) is an `investment_transfer_facts` row of kind
`exchange`: its link quantity is the *old* units, its destination lot holds
them times the ratio, and the destination instrument is
`destination_commodity_id`. A reader that opens or audits a destination from
`link.quantity_value` or `f.commodity_id` (the closure edge, the self-check's
revised-link events) is right for a transfer and wrong for an exchange. Read
the destination lot's own quantity and `COALESCE(f.destination_commodity_id,
f.commodity_id)`. Named regression:
`TestShareExchangeUpstreamBuyReplacementRevisesTheNewLot`.

## 32. A transfer link's source side assumed to be a `transfer_out` that moves units

A spin-off (#180) is a transfer fact of kind `spin_off` whose parent lot keeps
its units: its source effect is a quantity-0 `basis_reduction`. A reader that
finds a link's source event by `event_kind = 'transfer_out'`, or folds a
revised link as `−link quantity` out of the source (replay intents,
`effective_investment_lot_events`, the self-check's revised-link events),
silently drops or double-moves a spin-off. Key the source event on the
transfer kind. Named regressions:
`TestSpinOffUpstreamBuyReplacementRevisesBothSides`,
`TestSpinOffCarriesUnknownBasisAndResolvesThroughReplay`.

## 33. Every disposal decision assumed to dispose units

A cash-and-stock merger's cash decision (#181) has quantity 0, and so do its
allocations and its `disposal` lot events: it realizes basis, the units leave
through the exchange. A reader or validator that requires `quantity > 0`
(replay persistence, the allocation-set and lot-reconciliation self-checks) or
that replays every decision as an ordinary `disposal` intent breaks it.
Exclude the merger's decision from the disposal intents (its exchange group
replays it) and allow zero only for that kind. Named regressions:
`TestBuyCorrectionBeforeCashStockMergerRevisesRealizedAndCarriedBasis`,
`TestCashStockMergerUnknownBasisLeavesResultUnresolved`.

## 34. Reading `effective_investment_lot_events` as if it were complete

The view only *removes* events that a correction or a replay revision
superseded; it never adds the revised amounts back. A reader that folds it
alone forgets every revised cover, sale, split, return of capital and transfer
link. The named-short quantity did, so a cover revised by a backdated opening
stopped counting: 15 owed instead of 11, unexplained negative units passed as
short, and the net-worth series showed a total it should withhold. The side
guard did too, admitting overlapping long and short positions. Every existing
self-check passed (#188). A current-state reader folds
`db.EffectiveLotMovements` (#189), which owns those add-backs;
`TestEffectiveHistoryReadersUseTheOwner` refuses a new direct reader unless it
is allow-listed with a reason. A reader that needs only an operation's dates
and holding takes the original events of effective operations, because a
revision changes neither. Test a revised case, and test the visible warning,
not just the quantity. Named regressions:
`TestNamedShortCountsARevisedCoverOnItsDate`,
`TestUnexplainedNegativeUnitBesideARevisedCoverIsFlagged`,
`TestShortOpeningInsideALongClosedByARevisedTransferIsRefused`. Replay
equivalence cannot see this class, because stored state and replay agree. The
independent oracle (`requireConservationOracle`, #190) checks readers against
the posted journal and operation facts. It runs in
`requireInvestmentSelfCheckPasses`; call it before a later correction retires
the revision under test.

## 35. An operation kind added without its capabilities

Kinds are special-cased by string across generic code, so a new kind can ship
without replay, a correction, export rows or a passing self-check. Writers
refuse kinds missing from `db.InvestmentOperationKinds`. The capability matrix
(#191) fails until the kind has a row naming, for each capability, a test, a
refusal test or a gap issue, and until a capability scenario records it. A new
replay-intent family needs its own row too.

## 37. Validating a different representation from the one readers consume

Go struct JSON decoding matches keys case-insensitively, while immutable
source JSON and browser readers keep exact keys. `Reference` could override a
blank `reference` during validation, then history displayed the blank
canonical value (#198). Read required keys exactly when preserving raw JSON.
Also align text limits: JSON Schema counts Unicode characters, HTML maxlength
counts UTF-16 units, and Go len counts UTF-8 bytes. Source forms count Unicode
characters explicitly rather than depending on a native maxlength. Named
cases: `TestBasisResolutionRequiresSourceDeclarationWithoutWriting`,
`TestBasisResolutionSourceTextUsesUnicodeCharacterLimit`. Compare source
metadata without float decoding too: spin-off and merger correction treated
adjacent integers above 2^53 as unchanged evidence (#309). Reuse the
exact-number provenance comparator; named consumer:
`TestCorporateActionCorrectionDistinguishesExactSourceNumbers`. Share-exchange
replacement must compare that effective evidence too and retain it when
omitted (#310): `TestShareExchangeCorrectionRejectsUnchangedSourceEvidence`. A
server comparator cannot repair metadata the browser already parsed: a form
that spreads parsed evidence back into its payload rounds unedited numbers
(#311). Correction forms send only the fields the owner changed as
`source_evidence_edits` and the server merges them into the raw recorded
tokens. Named cases:
`TestSpinOffReplacementPreservesExactUneditedSourceEvidence`,
`TestSpinOffCorrectionAPIPreservesExactUneditedSourceEvidence`. Every
investment correction form follows it (#312): a form that does not edit
evidence sends none, and an omitted evidence keeps the recorded one (never
`{}`), so a no-op check compares effective evidence, not "evidence was sent".
Named cases:
`TestInternalTransferReplacementPreservesExactUneditedSourceEvidence`,
`TestSplitReplacementPreservesExactUneditedSourceEvidence`,
`TestSplitCorrectionAPIPreservesExactUneditedSourceEvidence`.

## 38. Treating source corrections as amount corrections

Comparing only an exact basis amount refused a genuinely changed declaration
(#197). Evidence changes at equal basis append their own immutable, attributed
revisions, without inverse journals, economic replacement or replay. Pin both
the effective resolution and evidence revision in the form and recheck inside
the writer. JSON layout/key order or a reason alone is a no-op. Preserve
additional metadata server-side as raw JSON: browser object parsing can round
numeric tokens, and rebuilding only typed fields drops metadata. History
renders the exact saved JSON. Named cases:
`TestProvenanceCorrectionPreservesDownstreamSalesTransfersAndCheckpoints`,
`TestProvenanceCorrectionRefusesNoOpsInvalidDeclarationsAndStaleWrites`,
`TestProvenanceCorrectionPreservesExactAdditionalMetadataOnServer`.

## 43. One response assembled from different effective histories

A shared price `knowledge_cutoff` does not freeze ledger corrections or
observation retirements (#237). Live gains and statements use `ReadGains` /
`ReadGainsStatement` to capture all operational, comparison, boundary, income,
historic-basis and FX dependencies once. Never combine standalone stage calls
against live repositories. Deterministic stage interleavings must retain the
same figures and evidence after source corrections and withdrawals, with no
durable read-side writes. Named regressions:
`TestLiveGainsCorrectionBetweenOperationalAndFXKeepsSameCurrencyGain`,
`TestLiveStatementBoundariesDisposalsIncomeAndFXShareEffectiveHistory`,
`TestLiveGainsAPICorrectionBetweenCompositionStages`.

## 44. A complete formula mistaken for a complete return

Period gains can correctly sum realized + dividends + unrealized change while
excluding capital-return excess (#238). Disclose the excluded amount and
unknown coverage beside the total; retain payment versus effective dates and
independent FX coverage. Never add the basis reduction again. New saved
disclosure needs its own document version; old output bytes remain frozen.
Named regression:
`TestGainsStatementCapitalReturnExcessIsSeparateFromLimitedTotal`.

## 80. A valuation quote chosen by sorting evidence by recording time (#203)

Valuation consumers use the shared `db` observation-selection owner
(`observation_selection.go`, `live_raw_v1`). Never choose an adjusted or
future quote by sorting all evidence by recording time. Keep same-date
manual/override, exact and approximate priority (including the existing
unadjusted `trade_implied` / `not_applicable` fill marker), the
valuation/knowledge boundary and each consumer's explicit age rule. A live
timestamp cutoff does not reconstruct past retirements or book state. Validate
through real holdings and reporting FX (`TestObservationSelection*`),
preserving original per-currency amounts.
