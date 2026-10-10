# Unknown investment basis: T-145 / #160

Implementation contract for [#160](https://github.com/sergeyfarin/rekenraam/issues/160).
ADR 0013 and the [slice 5 contract](investment-operation-slice-5-contract.md)
govern; `roadmap.md` alone defines execution order. This work is complete
(2026-10-09); the boundaries below record what shipped and its named cases.

Nullable **remaining** basis alone is insufficient to admit an unknown transfer.
Immutable openings, lot events, disposal decisions, allocations and their replay
revisions now carry explicit knowledge and NULL pairs, with faithful exports,
self-check and gain-impact reads. Sales, average pools and replay propagate
unknown basis into unresolved gains (boundary 2). Unknown inbound, outbound
and internal transfers and splits are admitted (boundary 3), and an unknown
inbound is resolved by a sourced, audited fact (boundary 4). Known zero
remains known.

## Evidence prerequisite delivered

An unknown immutable transfer link must have NULL carried-basis coefficient
**and** scale. It may retain a named cost currency for later resolution.
Known links require both amount fields and their currency. The earlier check
only excluded a completely populated known triple, allowing five partial
amount combinations to masquerade as unknown. The baseline constraint now
rejects those combinations.

`TestTransferBasisKnowledgeRequiresCompleteAmountPair` tests all eight NULL
combinations for both knowledge states; five unknown cases fail before the fix.
The existing `TestKnownZeroTransferBasisRemainsKnown` covers known zero.
This prerequisite alone did not implement unknown openings, disposals or
resolution; the delivered boundaries below complete the end-to-end acceptance.

## Delivered implementation boundaries

The opening/event portion of boundary 1 is implemented: independent original
knowledge, immutable paired NULLs, nullable original API fields, bundle schema 9,
opening intent/activation/output/persistence and replay-equivalence diagnostics.
Named cases cover NULL pairs, original-versus-projected knowledge, exports,
invented-zero and quantity damage, plus preservation of known-zero behavior.

The disposal portion of boundary 1 is implemented: decisions, allocations,
revisions and revision allocations store paired NULLs with explicit
knowledge; insert guards tie allocation knowledge to its total and lot event;
the decision and revision writers derive an unknown total from any unknown
allocation; exports leave unknown amounts blank; the gain-impact snapshot reads
revision knowledge as one tuple; self-check conserves quantity and proceeds
regardless of knowledge and replay equivalence compares allocation knowledge.
At boundary 1 the realized-gains read refused unknown evidence; boundary 2
replaced that with unresolved entries. Named cases:
`TestDisposalBasisKnowledgeRequiresCompleteAmountPair`,
`TestUnknownDisposalAllocationRequiresUnknownTotalAndMatchingLotEvent`,
`TestUnknownDisposalRevisionKeepsAllocationSetsHealthyAndDamageVisible`,
`TestUnknownDisposalRevisionExportsBlankAmountsAndKnowledge`,
`TestRealizedGainsReportUnknownEffectiveRevisionAsUnresolved`,
`TestInvestmentGainSnapshotReadsRevisionKnowledgeAsOneTuple`.
Boundary 1 is complete.

Boundary 2 is implemented for sales. Only a sale admits unknown basis
(`DisposeLotsParams.AdmitUnknownBasis`; replay admits committed `sell`
operations and a proposed backdated sale). FIFO, LIFO and specific-lot
selection take the exact elected quantities; each allocation keeps its lot's
knowledge, and one unknown allocation makes the decision unknown with a NULL
total. An unknown lot loses quantity while its remaining basis stays NULL,
including when it closes. An average pool holding any unknown lot has no
rate. Every allocation from it is unknown, and every lot in the pool keeps an
unknown remainder until the pool is exhausted. A new known acquisition after
that opens a known pool. At this boundary, write-off, cash in lieu, return of
capital, splits and transfers still refused unknown basis atomically; boundary 3
admitted splits and transfers. The allocation scale and range
guard of an admitting sale use the known subtotal. Sale preview and commit
results, realized gains and per-currency totals expose NULL basis and gain
with `basis_knowledge`; a total with any unresolved entry is NULL with
`unresolved_count`. The sell preview, gains report and cash-in-lieu allocation
list render unresolved labels in six locales. A backdated known purchase that
resolves an unknown sale through replay is a disclosed, acknowledged gain
change, and the original decision stays unknown evidence. Named cases:
`TestSaleOfMixedKnownAndUnknownLotsLeavesGainUnresolvedInEitherOrder`,
`TestSaleOfOnlyKnownLotsStaysKnownBesideAnUnknownLot`,
`TestClosingUnknownQuantityDoesNotManufactureKnownGain`,
`TestAverageCostPoolWithUnknownBasisStaysUnresolvedUntilExhausted`,
`TestBackdatedKnownBuyResolvingAnUnknownSaleIsDisclosed`,
`TestBackdatedSaleThroughReplayAdmitsUnknownBasis`,
`TestReversingAnUnresolvedSaleRestoresQuantityAndKeepsBasisUnknown`,
`TestWriteOffRefusesUnknownProjectionAtomically`,
`TestUnknownImmutableOpeningKeepsNonSaleDepletionsGated`,
`TestUnresolvedSaleReportsNullBasisGainAndTotal`.
Unknown transfer depletions and their dependent links shipped in boundary 3.

Boundary 3a is implemented: public unknown inbound and splits. The inbound
command and API take an explicit `basis_knowledge: unknown` with no amount; an
omitted amount, or unknown with an amount, is refused. Unknown inbound posts
security legs only and opens an unknown lot and link. Backdated, it replays
later decisions: a later sale it now feeds becomes a disclosed unresolved gain
change, and a later write-off it would feed is refused, naming that operation.
Boundary 3b subsequently admitted an unbridged unknown outbound; a previously
bridged known outbound becoming unknown remains refused. An inbound replacement
may change the knowledge. Replacing it
with sourced known basis posts the full bridge and resolves dependent sales
through replay with acknowledgement; the original link stays unknown evidence.
Correction terms report `basis_knowledge` and never prefill a known zero.
Splits conserve knowledge, since they move no basis. The transfer-in and its
correction form offer an explicit "cost basis unknown" option in six locales.
Named cases:
`TestExternalTransferInWithUnknownBasisPostsSecurityLegsOnly`,
`TestExternalTransferInRefusesAmountWithUnknownBasis`,
`TestBackdatedUnknownTransferInLeavesLaterSaleUnresolvedWithDisclosure`,
`TestBackdatedUnknownTransferInRefusesLaterWriteOffDependency`,
`TestSplitConservesUnknownBasisKnowledge`,
`TestReplacingUnknownTransferInWithKnownBasisResolvesDependentSale`,
`TestExternalTransferInAPIRequiresExplicitUnknownBasis`, and the browser case
`investments-unknown-basis.spec.ts` (390 px).

Boundary 3b is implemented: outbound and internal transfers of unknown
basis. Transfer depletions admit unknown basis. A link records its depletion's
knowledge, and an internal destination lot opens with it. An average-cost pool
holding unknown basis moves out unknown in either lineage. An outbound with
any unknown link posts security legs only, with no partial bridge; the
complete bridge is supplied by resolution in boundary 4. Transfer-link revisions
and their
pooled depletions carry paired NULL knowledge, and a database guard refuses a
revision that changes its link's knowledge except the unknown-to-known path
admitted in boundary 4. Replay still refuses known-to-unknown regression.
The effective-link view
and replay intents read revision knowledge and amounts as one tuple, never
filling a NULL from the original link. Self-check matches link, event and
destination knowledge NULL-safely and requires no bridge on an unknown
outbound. Pooled sets must agree on knowledge, and exports append
`basis_knowledge` to link revisions and their depletions. Outbound and internal
forms list unknown lots and preview "unknown" basis with no bridge, in six
locales. Named cases:
`TestInternalTransferCarriesUnknownBasisToDestination`,
`TestPooledTransferOfUnknownPoolCarriesUnknownInEitherLineage`,
`TestOutboundTransferOfUnknownBasisPostsNoBridge`,
`TestReplayRefusesToChangeAKnownOutboundToUnknown`,
`TestReversingUnknownInternalTransferRestoresUnknownSource`,
`TestUnknownTransferLinkRevisesLineageWithoutChangingKnowledge`, and the
browser case `investments-unknown-basis.spec.ts` (outbound preview, 390 px).
Boundary 3 is complete.

Boundary 4 is implemented: sourced resolution of an unknown inbound.
- **Fact and journal.** `POST .../transactions/{id}/resolve-basis` and its
  rolled-back reconciliation-impact preview append an immutable
  `investment_basis_resolutions` fact. A `basis_resolution` operation creates
  it, and its journal posts the complete omitted inbound bridge (trading +b,
  equity −b) in the transfer's cost currency, dated to the transfer.
- **Pinning.** The fact is pinned to the unknown link's lot, quantity and cost
  currency, and is read again inside the write. There is one resolution per
  link, and the original link and lot stay unknown evidence.
- **Effective reads.** The effective-link view applies an effective
  resolution, so replay revises every sale and transfer the lot reached: units
  sold, moved internally (link revisions may go unknown to known, never back)
  or held. Average pools become definitive.
- **Outbound bridges.** These are reconciled from the effective links. An
  outbound whose last unknown link becomes known posts its complete omitted
  bridge at its own date, even when its source has closed; later known
  changes post adjustments.
- **Correction rules.** While a resolution is effective, the transfer it pins
  cannot be reversed or replaced (`INVESTMENT_TRANSFER_BASIS_RESOLVED`); the
  chain reports `can_resolve_basis`. Correction of the resolution itself is
  delivered under #168 (see *Resolution correction* below), as is a
  journal-free known-zero resolution.
- **Self-check and export.** Self-check verifies each resolution's pinned link
  and that its journal is exactly its bridge. The bundle adds
  `investment-basis-resolutions.csv`.

Named cases:
`TestResolvingUnknownInboundAfterPartialSalePostsFullBridgeAndResolvesBoth`,
`TestResolvingUnknownInboundAfterFullSaleResolvesTheClosedLot`,
`TestResolvingUnknownInboundReachesInternalTransferDestination`,
`TestResolvingUnknownSourcePostsOmittedOutboundBridge`,
`TestResolvingUnknownLotResolvesAveragePoolDisposals`,
`TestTransferBasisResolutionRefusals`, `TestSelfCheckDetectsResolutionBridgeDamage`,
`TestResolveTransferBasisAPI`. Boundaries 1–4 are complete.

Boundary 5 is implemented, the operator surface and remaining diagnostics:
- **API.** Original and effective basis are exposed separately, unknown amounts
  are null with `basis_knowledge`, and unresolved gains and totals are null
  with `unresolved_count`.
- **UI.** Transfer-in, its correction, outbound and internal forms state or
  preview unknown basis. The transaction detail of an unknown transfer in
  offers "Resolve cost basis…": the sourced total in the transfer's own
  currency, a reference and a reason, previewed through the actual writer.
  Gain changes go to the shared confirmation, which binds the acknowledgement,
  and checkpoints are guarded. All of this is in six locales.
- **Exports and self-check.** Exports carry every knowledge column and the
  resolution facts. Self-check keeps quantity and provenance checks
  independent of basis availability.
- **Further named cases.**
  `TestTransferBasisResolutionLateRefusalsRollBackEverything` covers a stale
  acknowledgement and range overflow, both atomic.
  `TestResolutionKeepsItsTransferCostCurrencyWithoutFX` covers currencies.
  The browser case "a sourced statement resolves an unknown basis and its sale
  gain on mobile" runs in `investments-unknown-basis.spec.ts` (390 px).

**Resolution correction (#168).** Slice 1 is delivered (2026-10-10).
`POST .../transactions/{transfer_id}/replace-basis-resolution` and
`.../reverse-basis-resolution`, each with a rolled-back
`/reconciliation-impact` preview, correct the transfer's effective resolution
through its operation chain. They address the transfer rather than the
resolution journal, so a later journal-free resolution is addressable too. A replacement inverts the old bridge and appends a
successor fact pinned to the same link, lot, quantity and cost currency, with
its complete bridge at the transfer date. A reversal inverts the bridge and
returns the link to unknown. Replay of the dependency closure revises sales,
onward links and pools, and bridged outbounds post dated adjustments. A
reversal refuses with the operation named when a known outbound or onward
link would return to unknown. The resolved transfer stays uncorrectable until
its resolution is reversed. The transfer's chain exposes
`can_correct_basis_resolution` and `effective_basis_resolution`. The insert guard and self-check keep at most
one effective resolution per link. Named cases:
`TestReplacingResolutionRevisesPartialSaleAndKeepsEvidence`,
`TestReplacingResolutionRevisesSplitInternalDestinationAndBridgedOutbound`,
`TestRepeatedResolutionCorrectionKeepsOneEffectiveFact`,
`TestReversingResolutionRestoresUnknownBasisAndUnresolvedSale`,
`TestTransferCorrectionAfterResolutionReversal`,
`TestReversingResolutionRefusesKnownDependentWithOperationNamed`,
`TestResolutionCorrectionLateRefusalsRollBackEverything`,
`TestCorrectTransferBasisResolutionAPI`.

Slice 2 is delivered (2026-10-10): a sourced known zero. `resolve-basis`
admits a zero basis and records it without a fabricated zero journal. The
operation contract is the following.
- **Operation.** A journal-free `basis_resolution` operation is created
  under its own audit event through an audit-only command header. The same
  writer runs the guards, replay, checkpoint netting and gain
  acknowledgement, but posts no transaction. The operation records an
  `effective` date row and a `'0'` fact; no journal link is added.
- **Corrections.** A bridged resolution replaced by a zero posts only the
  inverse of its bridge, linked to the zero operation as `reversal`. A zero
  replaced by a positive basis posts only the successor's bridge, and a zero
  is reversed by a journal-free `reversal` operation. An outbound it reaches
  becomes known with a zero bridge, so nothing posts.
- **Self-check.** The journal-link check declares this exemption, and only
  this one. A zero must have no primary journal and a positive basis exactly
  its bridge. A zero or its reversal that posts any journal other than a
  predecessor's inverse is damage.
- **Export and restore.** The export lists the operation with a blank
  transaction and the fact with a numeric `0`. A backup restore keeps both,
  and self-check passes on the restored file.
- **APIs.** API responses return a null `transaction` (resolve and reverse)
  or null `inverse`/`replacement` (replace) where no journal posts. The
  effective terms carry a null `transaction_id`.

This is a separate operation kind from #146's zero-delta split. #146 may
reuse the audit-only header and must declare its own self-check exemption;
neither exemption covers the other. Named cases:
`TestResolvingUnknownInboundAsKnownZeroIsJournalFree`,
`TestKnownZeroResolutionMakesOutboundKnownWithoutABridge`,
`TestReplacingResolutionBetweenZeroAndPositiveBasis`,
`TestReversingKnownZeroResolutionIsJournalFree`,
`TestSelfCheckDetectsKnownZeroJournalMismatch`,
`TestKnownZeroResolutionExportsAndSurvivesRestore`,
`TestKnownZeroBasisResolutionAPI`. Still open in #168: the correction and
history UI in six locales, with mobile browser cases.

**Deliberately later.** These are not #160 acceptance; each needs its own
contract:
- unresolved write-off, cash-in-lieu and return-of-capital results ([#169](https://github.com/sergeyfarin/rekenraam/issues/169));
- mandatory provenance declarations: the current API/UI source reference is
  optional; [#170](https://github.com/sergeyfarin/rekenraam/issues/170)
  defines the statement or explicit owner-assertion contract.


1. **Immutable knowledge.** Add explicit knowledge with paired nullable basis
   fields to opening facts, lot events, disposal decisions and allocations,
   and their replay revisions. Preserve original evidence after resolution.
   Keep each position's named cost currency; resolution never infers an FX
   conversion. Separate original opening knowledge from effective remaining
   knowledge, including after a fully consumed lot is resolved.
2. **Disposal and replay.** FIFO, LIFO and specific-lot selection still consume
   the exact elected quantities. A disposal touching unknown basis has NULL
   total basis and gain, while independently known allocation amounts remain
   available. An average pool containing unknown basis cannot produce a
   definitive pool rate: every affected pool disposal remains unresolved.
   Rebuild quantities, knowledge, locks and dependent transfer links from
   immutable inputs; a replay must not unconditionally initialize known zero.
   Closing unknown quantity must not manufacture a known historical gain.
3. **Transfer admission.** Unknown external inbound opens its lot with security
   legs only. Unknown outbound depletes its dated source quantities and posts
   security legs only. Internal transfers must preserve unknown knowledge
   through their destination lineage before an unknown source can move internally.
   Splits conserve knowledge and basis; return-of-capital reductions and
   cash-in-lieu remain explicitly refused when their required basis is unknown
   until their separate unresolved-result contracts are implemented.
4. **Sourced resolution.** Append an audited basis fact linked to the effective
   transfer/opening and source evidence, then replay the dependency closure.
   Resolve the inbound opening's total basis, not just the remaining units.
   Post the complete omitted inbound bridge `T +b / E -b`, dated to that
   transfer; replay resolves quantities already sold or transferred as well
   as those still held. An unknown outbound becoming known posts its complete
   omitted bridge `T -b / E +b` at its own transfer date, even if its source
   quantity has already closed. An outbound amount comes from source depletion;
   broker-reported outbound basis cannot independently override the source
   lots. Resolve source opening facts to derive it. Subsequent known changes
   append bridge adjustments. Refuse a change that would make an already
   bridged known outbound unknown. Define audited correction/reversal rules
   for resolution facts before exposing their mutation.
   Pin each resolution to the effective unknown opening and its quantity and
   cost currency. A correction changing those source facts must either update
   the linked resolution through an explicit audited command or refuse with
   the resolution named; never silently reuse evidence for a different opening.
5. **Operator surface and diagnostics.** Expose nullable original/effective
   basis separately; label unresolved basis in six locales, retain quantities
   and independently priced market values, and export all knowledge and sourced
   facts. Self-check verifies quantities and provenance independently of basis
   availability. An intentionally unknown opening is unresolved information;
   inconsistent NULL pairs or an unjustified known projection are damage.
   Previews use the actual writer, roll back every side effect and disclose
   unknown-to-known gain transitions. Writes bind the gain acknowledgement and
   guard the dated bridge against each reconciliation checkpoint.

## Acceptance cases required before closing #160

- Unknown versus known zero; every invalid NULL pair; mixed known/unknown lots
  in both insertion orders; distinct cost currencies without implicit FX.
- Partial and full FIFO/LIFO/specific disposal; average pooling and later pool
  exhaustion; unknown quantity remains selectable without a definitive gain.
  After exhaustion, a new known acquisition opens a new known pool without
  relabeling earlier unresolved disposals. Resolution overflow refuses atomically.
- A split and internal transfer between opening and resolution, including
  destination sales and the dependency closure.
- Resolve an inbound after partial sale, after full sale, and after unknown
  outbound. Check the exact full dated bridges, subsequent disposal revisions
  and the unchanged original unknown evidence.
- Correct/reverse the relevant effective operations; preserve root ordering,
  lineage, method locks and historical snapshots. Refuse known-to-unknown
  outbound bridge regression atomically.
- Preview/commit parity, repeat previews with unchanged durable rows, missing
  and stale gain acknowledgements, reconciled transfer dates and later
  checkpoints, and a forced late failure rolling back facts and bridges.
- Healthy unknown information and deliberately damaged quantity, knowledge,
  evidence and superseded snapshots in self-check; API NULL fields and CSV
  blank amounts; mobile entry and sourced resolution in all UI states.

## Delivered implementation touchpoints

| Path | Delivered behavior |
| --- | --- |
| `backend/migrations/0001_initial_schema.sql` | Paired nullable knowledge on immutable openings, events, decisions, allocations and transfer revisions; immutable pinned resolution facts; effective links read resolved knowledge and amounts together. |
| `backend/internal/db/investments.go` | Lot scanners expose original and projected knowledge; admitting sales and transfer depletions conserve quantity without inventing basis. |
| `backend/internal/db/investment_replay_intents.go`, `investment_replay_simulation.go`, `investment_replay_revisions.go` | Replay preserves unknown openings, disposals, pools, splits and transfers, with revisioned knowledge and paired NULL amounts. |
| `backend/internal/db/investment_replay_propagation.go`, `investment_basis_resolution.go` | Sourced resolution replays the dependency closure and posts omitted outbound bridges or later bridge adjustments. |
| `backend/internal/db/investment_gain_impact.go` | Reads original/revised knowledge as one tuple and binds unknown-to-known gain transitions into the acknowledgement. |
| `backend/internal/db/self_check.go`, `self_check_replay.go`; `backend/internal/app/self_check.go` | Quantity, provenance, historical allocation conservation, resolution bridges and replay equivalence remain checked independently of basis availability. |
| `backend/internal/app/investment_basis_resolution.go`, `backend/internal/api/investment_basis_resolution.go`, OpenAPI | Atomic sourced resolution and rolled-back writer preview, with gain acknowledgement and reconciliation guards. |
| Export bundle writers and investment forms | Nullable API amounts and CSV blanks preserve unknown knowledge; bundle exports resolution facts; six-locale forms and labels support unknown entry and sourced resolution. |

Cash in lieu, write-off and return of capital still require known basis; their
unresolved-result contracts remain deliberately later, as described above.
