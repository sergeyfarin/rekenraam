# Unknown investment basis: T-145 / #160

Implementation contract for [#160](https://github.com/sergeyfarin/rekenraam/issues/160).
ADR 0013 and the [slice 5 contract](investment-operation-slice-5-contract.md)
govern; `roadmap.md` alone defines execution order. This work is in progress.

The existing nullable **remaining** basis projection is insufficient to admit
an unknown transfer. Original opening facts, lot events, disposal snapshots,
allocations and replay revisions still require numeric basis. Admission stays
closed until these paths carry knowledge end to end. Known zero remains known.

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
This prerequisite does not implement unknown openings, disposals or resolution,
and does not complete any of #160's end-to-end acceptance items.

## Remaining implementation boundaries

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

## Confirmed implementation touchpoints

| Path | Required change before admission |
| --- | --- |
| `backend/migrations/0001_initial_schema.sql` | Nullable immutable opening/event/decision/allocation and revision pairs, explicit knowledge, sourced resolution facts, effective views and historical conservation guards. Transfer revision admission currently requires a known original link; resolution needs an explicitly validated unknown-to-known path. `effective_investment_transfer_links` currently always reports original knowledge, even when selecting a revision's amount. |
| `backend/internal/db/investments.go` | Original lot scanners currently require numeric basis. Separate availability/selection from known-basis arithmetic and precision/range admission; never replace NULL with zero. |
| `backend/internal/db/investment_replay_intents.go` | Opening intents and transfer depletions need knowledge. Select effective basis with its own effective knowledge rather than using independent amount `COALESCE`s. |
| `backend/internal/db/investment_replay_simulation.go`, `investment_replay_revisions.go` | Reset, activation and persistence currently write `basis_knowledge = 'known'` unconditionally. Carry knowledge through each output and avoid erasing unresolved history on depletion. |
| `backend/internal/db/investment_replay_propagation.go` | Propagate knowledge across the dependency closure and distinguish the first omitted bridge from a later numeric bridge adjustment. |
| `backend/internal/db/investment_gain_impact.go` | Effective snapshot construction currently sets every disposal to known. Read original/revised knowledge, preserve NULL totals and bind unknown-to-known transitions into the acknowledgement token. |
| `backend/internal/db/self_check.go`, `self_check_replay.go`; `backend/internal/app/self_check.go` | Original/event scans and conservation folds require numeric basis. Check quantity independently; audit knowledge and NULL pairs for every historical set, then compare effective replay knowledge and amounts. |
| `backend/internal/api/investments.go`, OpenAPI, export bundle writers and investment forms | Original and disposed basis fields are still numeric. Make their contracts nullable with explicit knowledge before any writer can return unknown evidence; update bundle schema and six-locale operator flows together. |
