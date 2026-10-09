# In-kind transfers and basis actions: #114 closure audit

Reviewed 2026-10-09 at `e0cf4a9e`, after the #160 delivery and Go 1.27.2
security fix. Product requirements, conventions and ADRs 0012/0013 govern.
This review records evidence and disposition; `docs/roadmap.md` alone defines
work order, and GitHub owns issue acceptance, state and priority.

## Decision

Close [#114](https://github.com/sergeyfarin/rekenraam/issues/114) as completed
within its bounded delivery contract. Its intent is addressed: a long holding
can enter, move within and leave the book with explicit basis knowledge;
ordinary basis actions and splits have native commands; dated replay,
correction, gains disclosure, reconciliation, immutable history, export and
self-check are integrated rather than independent bookkeeping shortcuts.

Closure does **not** mean every historical-entry path, corporate action,
provider mapping or recovery workflow is complete. The audit reproduced
workflow gaps and identified untracked safe refusals and presentation limits.
Track those with finite acceptance rather than leaving the old umbrella open
indefinitely or reopening its delivered children. R16 stays open. Historical
entry and resolution recovery are now explicit R16 completion gates; the
other extensions keep their stated later boundaries.

## Evidence of delivered intent

| Delivery area | Evidence inspected | Assessment |
| --- | --- | --- |
| Gain safety and chronological replay | #129/#141; `executeInvestmentWriteTx`, writer previews and gain-impact snapshots; #132/#139/#147/#155 | Changed effective gains require a bound acknowledgement, and impossible histories refuse atomically. Dependency-closure replay is delivered; new internal-transfer admission remains a separate gap below. |
| External inbound | `investment_transfer_in.go`, unknown inbound and replacement tests, mobile inbound/correction forms | Known zero is explicit; unknown entry posts security legs only. Original opening/link knowledge survives resolution and correction. Original acquisition ordering is distinct from account-entry eligibility. |
| Internal transfers and pools | #138/#150; `investment_transfer_internal.go`, transfer propagation/replacement and pooled-lineage cases | Selected lots carry individual basis; average-cost sources use dated pooled depletion and preserve method locks. Destination lineage and revised source basis propagate. New backdated internal entry still refuses. |
| External outbound | #157/#158/#159; `investment_transfer_out.go`, bridge propagation, correction and checkpoint tests | Depletion determines basis; broker basis is evidence only. Unknown emits no partial bridge. Replay appends the omitted/full bridge or later adjustments at the outbound date. Native reversal/replacement preserves history. |
| Unknown basis and resolution | #160; immutable NULL-pair, mixed sales/pools, resolution and API suites; resolution form | Unknown remains NULL, quantities remain conserved, and unresolved gains are labelled. Sourced positive resolution reaches partial/full sales, internal destinations, pools and outbounds. Correcting a resolution and known-zero resolution remain unavailable. |
| Splits and cash in lieu | #137/#144/#151/#162/#165; split and disposal writers, adjustment, split-link and cash-in-lieu tests | Splits conserve basis, retain exact fractions or refuse, and reconcile journal adjustments with replay. Cash in lieu is a separate linked disposal with distinct disposal/payment dates. Unknown cash-in-lieu remains refused; zero-delta split admission and verified provider mapping have their own issues. |
| Return of capital | #161/#163; capital-return writer, entitlement, revision, correction, conservation and bundle cases | Receipt cash, basis reduction and excess are separate exact facts. Explicit quantities cap only entitled basis. Original/superseded effect sets stay audited; unresolved excess needs better post-entry visibility. |
| Lifecycle, portability and trust | Transfer/action correction services; effective-operation views; base-table export; foundation/replay self-checks; #134/#135/#149/#154 | Financial changes use native atomic writers, generic mutation remains fenced, and export/audits retain superseded facts. No ledger/subledger corruption was reproduced in this audit. |

The delivery gates #129, #137, #138, #132 and #139 are closed, as are the
six bounded closure children #157–#162 and completion follow-ups #163/#165.
Earlier comments listing missing outbound, pooled transfers or unknown basis
are historical progress notes, not the present boundary.

## New follow-ups

| Issue | Finding and exact scope | Disposition |
| --- | --- | --- |
| [T-151 #166](https://github.com/sergeyfarin/rekenraam/issues/166) | Entry forms select today's positive positions/lots. A backdated split or return cannot select a holding now closed; outbound entry hides already-consumed source units that dated replay can use. Add composed dated selection and mobile cases. | P2, R16 workflow-completion gate. |
| [T-152 #167](https://github.com/sergeyfarin/rekenraam/issues/167) | New internal transfer entry checks chronological order on both positions and refuses before later depletions. Correction/replay support does not imply admission of a new transfer. Add proposed-transfer replay across both holdings and the dependency closure. | P2, R16 workflow-completion gate. |
| [T-153 #168](https://github.com/sergeyfarin/rekenraam/issues/168) | A positive resolution permanently fences transfer correction, with no resolution successor/reversal command. Known zero requires replacing the opening instead. Add audited recovery and an explicit journal-free zero-resolution contract. | P2, R16 correction-completion gate; preserve known-to-unknown outbound refusal. |
| [T-154 #169](https://github.com/sergeyfarin/rekenraam/issues/169) | Public unknown holdings cannot take write-off, cash in lieu or return of capital. Their NULL result/reduction/excess and replay contracts are not implemented. | P2, explicitly later extension; keep refusal until each family is specified. |
| [T-155 #170](https://github.com/sergeyfarin/rekenraam/issues/170) | A resolution can omit source evidence and store `{}`. Actor and reason exist, but the source declaration is optional in API and UI. Define statement versus explicit owner-assertion provenance and enforce that decision. | P2, auditability hardening; not a reproduced arithmetic defect. |
| [T-156 #171](https://github.com/sergeyfarin/rekenraam/issues/171) | Return-of-capital excess is visible in entry/correction preview and export, but absent from normal persistent gains/detail read surfaces. Expose effective unresolved excess without calling it tax gain or posting income. | P2, visibility follow-up; coordinate with R18 #118. |
| [T-157 #172](https://github.com/sergeyfarin/rekenraam/issues/172) | Write-off creation/preview APIs exist; the portfolio offers no creation action. Existing correction UI does not solve entry. | P3, existing deferred mobile workflow now tracked. |

No direct data-loss or calculation regression was reproduced. The first two
findings are workflow defects; the resolution lock is a deliberately safe
but incomplete recovery contract; the other findings are explicit admission,
provenance or visibility gaps. This distinction determines how they should be
implemented and tested rather than weakening the existing guards to make a
form appear usable.

## Existing neighbors: reuse, do not duplicate

- [#146](https://github.com/sergeyfarin/rekenraam/issues/146): journal-free
  zero-delta split and later backdated entitlement. Still an R16 completion
  gate; coordinate journal-free resolution infrastructure without assuming
  that a split and resolution have the same effects.
- [#145](https://github.com/sergeyfarin/rekenraam/issues/145): verified Trading
  212 split/fragment mapping. Raw split fills are not sufficient ratio or
  entitlement evidence. Manual split-row linking already ships; unsupported
  provider fills stay in review. Evidence-blocked, not a #114 closure gate.
- [#111](https://github.com/sergeyfarin/rekenraam/issues/111): broader provider
  events/suggestions. Transfer/FOP and basis-action mappings need verified
  per-kind semantics, identity ownership and native command admission; the
  existence of a manual command does not make its provider row admissible.
  Extend this neighbor's planning context rather than reopen #114.
- [#136](https://github.com/sergeyfarin/rekenraam/issues/136): execution
  cancellation evidence. Neither cancelled order status nor `FOP_CORRECTION`
  is cancellation proof. Remains evidence-blocked.
- [#152](https://github.com/sergeyfarin/rekenraam/issues/152): older revised
  fills unreachable through Refresh. Independent import UX and R16 gate.
- [#148](https://github.com/sergeyfarin/rekenraam/issues/148): mandatory lot
  opening provenance in the schema. Release gate; the nullable test-fixture
  path is not a second production opening writer.
- [#164](https://github.com/sergeyfarin/rekenraam/issues/164) and
  [#108](https://github.com/sergeyfarin/rekenraam/issues/108): race feedback
  headroom and realistic export/self-check scale. Validation and operational
  confidence remain measurable work, not reasons to omit invariants.
- [#100](https://github.com/sergeyfarin/rekenraam/issues/100),
  [#101](https://github.com/sergeyfarin/rekenraam/issues/101),
  [#102](https://github.com/sergeyfarin/rekenraam/issues/102): locale amount
  entry, owner-local date defaults and translation parity/native review.
  Six-locale message boundaries do not establish native language review.
- [#103](https://github.com/sergeyfarin/rekenraam/issues/103) and
  [#115](https://github.com/sergeyfarin/rekenraam/issues/115): named short
  positions and compound actions. Separate R16 families, not #114 leftovers.
- [#116](https://github.com/sergeyfarin/rekenraam/issues/116),
  [#118](https://github.com/sergeyfarin/rekenraam/issues/118),
  [#119](https://github.com/sergeyfarin/rekenraam/issues/119): valuation UI,
  reproducible gains and return analytics. Operational average-cost handling
  is not jurisdiction-specific tax reporting; viewing a report never posts.

## Validation and reproducible audit cases

The unchanged implementation at `e0cf4a9e` passed the complete local
`./scripts/test-backend.sh` gate on Go 1.27.2 (formatting, vet, race; API
286 s, app 719 s, database 145 s). The [GitHub CI run](https://github.com/sergeyfarin/rekenraam/actions/runs/37912185615)
is green for backend race tests/coverage, frontend check, browser smoke and
integrated build. The [vulnerability run](https://github.com/sergeyfarin/rekenraam/actions/runs/37912185649)
is green. These are validation results, not a controlled performance benchmark.

This audit added isolated temporary probes through Go's `-overlay` facility;
no production or permanent test source was changed. All six probes passed
under the race detector using real migrated SQLite fixtures:

1. `TestAudit114CapitalReturnEntryAfterPositionClosed`: buy 2 units on May 1,
   sell both July 1, then preview and acknowledge a June 1 return. Writer and
   self-check pass; the form cannot select that closed holding.
2. `TestAudit114SplitEntryAfterPositionClosed`: the same closed holding admits
   a June 1 2:1 split, discloses changed gain and passes self-check.
3. `TestAudit114OutboundEntryFromCurrentlyClosedLot`: buy 3 on May 1 and 3 on
   May 15, sell 3 FIFO July 1, then transfer 2 from the closed first lot on
   June 1. Acknowledged replay supplies the later sale from the remaining
   first unit and second lot, and self-check passes.
4. `TestAudit114ResolutionAcceptsNoSourceEvidence`: resolve an unknown inbound
   with a positive amount and reason but no source JSON; `{}` persists and
   self-check passes. This proves optional provenance, not corrupted basis.
5. `TestAudit114ExplicitKnownCapitalReturnBesideUnknownLot`: both full discovery
   and a known-only explicit entitlement beside unknown basis refuse. Do not
   describe the present UI discovery problem as hiding a supported mixed-basis
   command; the action writer also fences that case.
6. `TestAudit114BackdatedInternalTransferIsRefused`: the two-buy/source-sale
   scenario above, moving 2 internally on June 1, refuses as out of order;
   complete durable snapshots are unchanged. Source and destination both
   require dated admission, not just a less restrictive picker.

The seven delivered transfer/action browser suites plus a temporary historical
selector probe and authentication setup passed **21/21** at 390 px, with a
fresh single-binary build. The probe confirmed absence of the closed holding
in split, return and outbound selectors even after choosing its historical
date. It was removed after execution. The existing suites continue covering
entry, corrected dates/quantities/currencies, reversal, gain confirmation,
unknown labels, and impossible dependent disposals.

This audit has not implemented the new workflows, verified new provider payloads
or assessed tax compliance. Each follow-up carries its own acceptance and
validation contract. Documentation and issue hygiene are the durable changes.
