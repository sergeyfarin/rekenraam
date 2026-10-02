# Investment operation progress snapshot — 2026-10-01

Historical slice history extracted from the former 2026-10-01 active plan at
`4cf39833`. The [active contract](../plans/investment-operation-refactor-plan.md)
supersedes this snapshot. Status and future-gate wording below records what the
plan said then; it does not set current priority or override accepted ADRs.
The contract and common acceptance cases remain in the active plan rather than
being duplicated here.

## Progress at capture

The 2026-09-29 review found that calling 2a and all of slice 4 complete was
premature. The nullable unique operation transaction header has now been
retired from the pre-release baseline, writer and frozen seed. Pinned
`investment_operation_journal_links` are the sole operation-to-journal
relationship. Foundation self-check, replay, correction history, trade source
facts, correction admission and import effect inference use these links.
Writer bootstrap passes its inserted operation ID directly into link creation.
Correction history and export summaries display the first primary link once
per operation; the journal-link CSV retains every linked version. An inverse
resolves the same correction history but is not offered as a replacement trade
or inferred as a primary import operation. Unlinked investment journals are
refused. Fresh and seeded tests exercise the actual retired-header schema,
retain the seeded operation CSV contract, and cover correction/reversal,
transfer, reinvestment and link-integrity mutations.

**BREAKING DEV DATABASE, R16 journal-link authority (2026-09-30):** this
ADR 0013 pre-release baseline rewrite removes `investment_operations.transaction_id`
and its header-dependent trigger checks. The checksum and frozen seed are
updated together. Reset disposable development databases as documented in
`docs/developer-workflow.md`; this is not an installed-release upgrade.
No legacy databases exist; source revision tables and the lot opening guards
are consolidated into migration `0001` with an updated checksum and fresh/seeded
equivalence coverage. Lot identity, opening quantity/basis, source evidence and creation attribution
are now guarded against updates and deletion by the consolidated baseline.
Remaining balances, status and projection update attribution remain mutable
for disposal and replay. Seeded mutation tests cover every opening field and
confirm that projection-only writes preserve acquisition facts. The physical
split is now implemented: `investment_lots` contains immutable identity and
opening facts, `investment_lot_state` holds remaining balances/status and update
attribution, and `current_investment_lots` provides the joined read model.
Effective long-position replay reconstructs missing lot and basis state in one
transaction without changing opening/event facts. Self-check reports missing
or corrupt projection state. Bundle schema 3 exports the state separately while
preserving the existing lot summary. Fresh/seeded fixtures, correction and
transfer flows, reconstruction and installation-failure rollback are covered.
Unknown carried-basis knowledge still comes from immutable transfer facts.
Current lot state now stores explicit `basis_knowledge` with nullable remaining
basis coefficient and scale. Known zero is numeric; unknown requires both
fields NULL. Lots/positions API responses preserve quantity and return NULL
basis fields with knowledge; one unknown lot makes its entire cost-currency
position basis unknown. Market value remains available when a price fits,
while gain is omitted with `gain_unavailable: unknown_basis`. Portfolio, lot,
gain and transfer-picker views label this state through translations.
Self-check reports unresolved or inconsistent knowledge/amount pairs without
comparing unknown with zero, and still verifies quantities against both lot
events and journal holdings. Bundle schema 5 appends knowledge to the lot
summary/state CSVs and represents unknown amounts/scales as empty cells.
Known-basis writers refuse unresolved positions before disposal, average-pool,
precision or range arithmetic. Current replay derives known state from known
immutable opening evidence and preserves NULL/knowledge on rollback. This
projection contract does not admit unknown-basis transfers: nullable immutable
opening/event/disposal facts and sourced unknown-basis replay/resolution remain
slice 5 command gates. Trade net-settlement and separately
posted fee components now link to the exact journal posting line keys chosen by
their command, including when another leg has identical account, currency,
date and amount. Self-check compares their account, commodity, date, signed
exact amount and operation version, and finds trade cash, expense or
charge-clearing postings without a source component in each trade's primary
journal. An inverse journal linked as a correction reversal has no new source
components. Gross and fees included within net clearing have
no individual posting link. Every currently shipped operation kind requires a
posted journal link; a future basis-only kind needs an explicit exemption.
Self-check also compares aggregate sell/write-off proceeds with their clearing
postings per operation, pinned journal version and cost currency, streaming each
decision and shared posting once and summing in exact Go arithmetic. Multiple
decisions are no longer silently excluded; negative proceeds, differing scales
and wide coefficients retain their exact values. A decision must have a matching
non-reversal operation journal link, even if another journal has equal amounts.
The lot check validates each original disposal allocation set and every replay
revision independently: exact quantity and signed proceeds sum to the immutable
decision, and basis sums to that snapshot's original or revised basis total.
Missing allocations, nonpositive allocated quantities and negative basis are
failed findings. Superseded snapshots remain checked as audit evidence; only
effective snapshots contribute to the current position projection. Allocation
damage is reported before projection arithmetic and is never repaired by the
check. Negative proceeds and equivalent scales remain valid.
Individual disposal attribution now has immutable
`investment_disposal_clearing_allocations`: each row names a decision, pinned
clearing posting version and signed exact proceeds portion. One decision can
span dated settlement/fee legs and several decisions can share a leg. Current
single-disposal writers attribute whole legs atomically; compound commands
must supply explicit portions. Self-check independently compares each decision
with its portions and each clearing leg with all assigned portions, validating
book, operation journal link, version, currency and trading account. Equal and
opposite decision/allocation errors can no longer hide behind a sound group
total or lot projection. Separately expensed fees are excluded; a zero-proceeds
write-off needs no cost-currency leg. Correction/replay retains the original
attribution as audit evidence; a replacement sale records its own attribution.
Bundle schema 4 adds `disposal-clearing-allocations.csv`. The ADR 0013 baseline,
checksum and frozen seed are updated together; no legacy databases exist.
T-110 #125's current-command integrity acceptance is complete, including the
nullable projection/read/export gate. Unknown-basis source
and command replay remain scoped to slice 5; remaining correction gates
belong to T-75b. The reviewed
baseline now keys disposal decisions by `(operation_id, decision_seq)`; current
single-disposal writers emit sequence 1. Buy/sale reversals and compound buy/sale replacements now share the
investment transaction writer, including atomic source acceptance. Complete the remaining T-75b correction
gates before adding outbound transfers or basis actions.

## Original gaps the slices address

These bullets describe the pre-refactor implementation. Slices 2a–4u added
operation links, exact trade economics, explicit proceeds and manual
buy/sale correction, but the integrity and correction gates listed above
remain open. Slice 5b added known-basis external inbound transfers; general
dated admission, the other transfer and basis actions, and short positions
remain.

- `investment_operations` has a name, date, and mandatory unique transaction
  link, but no exact trade consideration, charges, source identity, or links
  directly to its lot effects. A basis-only change need not make a journal
  posting; a complex event can touch several securities and cash accounts.
- `investment_lot_events.event_kind` only admits acquisition, disposal, split
  adjustment, reinvestment, and manual adjustment. The optional transaction ID
  and free-form metadata do not distinguish transfer, basis-only change,
  short-cover, or cross-instrument conversion.
- `InvestmentTradeInput` has one cash amount. The buy path treats it as lot
  cost; realized gains find sale proceeds by summing cash postings. Once a
  trade has separate commission, transaction tax, withholding, rebates, or
  multiple cash legs, that inference can produce the wrong result.
- Buy, sell, dividend, and reinvestment use separate repository write paths.
  Adding an event type to each path risks drift in audit, reconciliation,
  import identity, and preview behavior.
- Lots contain both original evidence and mutable remaining-balance fields.
  The latter need a deterministic rebuild before an older operation can be
  corrected safely. T-95 currently rejects backdating behind disposals.
- Disposal decisions are unique by transaction, import identities require a
  committed transaction, and the position basis-state key has no side. Those
  keys cannot describe multi-disposal actions, basis-only imports, or separate
  long and short pools without redesign.

## Delivery slices and gates

Each slice keeps the app runnable, updates OpenAPI/types, export/restore and
self-check contracts if it changes them, and adds named tests with independent
expected amounts and conservation checks. Stop at a gate before widening the
next family.

1. **Baseline contract and fixtures — complete 2026-09-26.** Specify exact
   equations and posting matrices for current buy/sell/dividend/reinvestment/
   write-off, including mixed-scale and multi-currency charges. Write migration and export contract
   tests against fresh and seeded candidate databases. Settle the operational
   fee/basis and cross-currency rules here, including capitalized-in-clearing
   versus separately expensed charges. Define versioned book/account fee
   defaults, the per-charge election and provenance fields, and the
   same-currency commission fallback before shaping the schema. The accepted
   equations and first-slice admission limits are in
   [the slice 1 contract](../plans/investment-operation-slice-1-contract.md). This
   slice does not rewrite `0001`.
2. **Foundation in two independently validated sub-slices.** Both keep the
   app runnable; neither opens a new user-facing investment operation.

   - **2a — investment schema and writer — partial, integrity gates reopened 2026-09-29.** Introduce the parent/link/date/
     component tables, operation-keyed disposal decisions, side-keyed basis
     state, fact/projection split, canonical TEXT coefficients, and direct
     effect links. Add and seed `external_investment_transfer_equity` in the
     baseline account-role `CHECK`, with export and self-check coverage.
     Implement ADR 0013's link cardinality. Route existing
     commands through the new writer while preserving current import
     identity callbacks. Move trade-implied price creation into that same
     SQLite transaction, with typed source-transaction-version provenance
     and an approximate flag for net-derived observations. Preserve the
     existing latest-price selection and valuation availability in this
     slice; the flag does not filter a price out. Reuse the command's one
     audit event for the observation and any new price series, including
     `price_observations.created_audit_event_id`; no second pricing audit
     event is inserted. A valid trade whose derived unit quote overflows the
     price table's int64 coefficient or rounds to zero remains posted without
     an observation; price creation is atomic when the quote is representable.
     Store the fee-treatment election/provenance columns
     and versioned defaults even though the new charge-entry UI arrives in
     slice 3. Prove equivalent journal, lots, gains, valuations,
     checkpoint behavior, and import idempotency for old cases; assert one
     audit event and shared audit IDs in the price-write test.
     Version the investment export contract without losing old facts.
   - **2b — generalized import identity — complete 2026-09-26.** Replace the one-transaction
     result with ordered operation/transaction child links. Keep the current
     unique `(book_id, dedupe_fingerprint)` admission rule. Test bank CSV,
     bank QIF, and Trading 212 retries, overlaps, duplicate fingerprints,
     partial failures, and staged-row results through the actual commit
     path. No basis-only import is accepted until this slice passes.

   Revise the unused `0001` baseline under ADR 0013 for 2a and again for
   2b if still pre-release: each rewrite declares `BREAKING DEV DATABASE`,
   updates checksum, fixture, upgrade/equivalence tests, and resets
   disposable developer databases. After release, 2b uses a forward
   migration. Do not combine 2a and 2b merely to save a migration number.
3. **Exact trade economics — complete 2026-09-27.** Add gross, fee/tax components, currencies,
   net settlement, and settlement date to buy/sell input/API and import
   mapping. Move gains from cash-posting inference to explicit per-disposal
   economics; preserve write-off as zero proceeds. Show gross, charges, and net in
   preview and UI. For fees in another currency, require explicit cash legs
   and a mapped expense account; require rate/source facts and an approved
   bridge before such a fee affects cost-currency basis or proceeds. Do not
   hide an implicit conversion. Derive a
   trade-implied unit price from gross consideration in the quote commodity
   divided by quantity, excluding fees/taxes and hidden FX. If gross is
   unknown, keep the net-derived estimate usable with an approximate label
   in the API and UI. On the same valuation date, an explicit manual or
   valuation override retains priority; a trusted provider or gross-derived
   price ranks above an approximate net-derived price. Define a stable tie
   break for equal-ranked sources and test that an approximate observation
   cannot displace a trusted same-date price merely by arriving later.
   Supersede an estimate when sourced gross for its trade becomes available.
   The current Trading 212 order-fill payload contains a unit price and net
   wallet amount but no sourced gross or charge breakdown, so those imports
   retain `gross_unknown` and an approximate price. Superseding their estimate
   awaits an investment-native correction with sourced gross in slice 4.
4. **Replay and investment-native correction (T-75b).** Implement rebuild,
   correction/reversal, dependency conflicts, and reconciliation preview.
   Exercise a corrected old buy followed by several sells under each basis
   method, including the different allocation rules above, same-day ordering,
   import replacement, trade-price provenance, and failure rollback. Switch
   realized gains, positions, and self-checks to the latest effective
   allocation set and rebuilt state; test a correction where the original
   and effective FIFO allocations differ. Exact trade economics precede
   this slice so replay has one authoritative source for proceeds and charges;
   this refines ADR 0013's foundation-to-correction sequence.
   The named sub-slices below are complete individually, while source-file-driven
   imported correction, broader
   operation corrections remain open; shared buy/sale orchestration is complete
   in 4ag–4ah.
   - **4a — immutable intent reader — complete 2026-09-27.** The long-position
     reader takes opening terms from lot facts and disposal terms from decisions,
     retains method/provenance and specific-lot elections, and orders them by
     financial date, operation ID and effect sequence. FIFO/LIFO/average
     allocations remain replay outputs. This is a read-only foundation;
     backdated writes and native correction remain fenced until the rebuild
     and effective revision writer land.
   - **4b — reversible long-position simulation — complete 2026-09-27.** A
     SQLite savepoint resets the current lot projection from immutable opening
     facts, replays decisions through the existing FIFO/LIFO/average/specific
     disposal rules, captures resulting lot state and allocations, and rolls
     back the simulated writes. It rejects a position with lots lacking source
     facts or an impossible dependent disposal by name. The next step persists
     numbered effective revisions and wires the correcting journal command;
     this simulation alone does not admit backdated writes.
   - **4c — effective revision storage foundation — complete 2026-09-27.**
     The unused candidate baseline has append-only, numbered disposal
     revisions and allocation sets. A transaction-scoped writer validates
     replay conservation, preserves the original snapshot and lot events, and
     installs the rebuilt current lot state under the triggering operation's
     audit event. Read models and the native correction command still need to
     select and produce these revisions before backdated writes are enabled.
   - **4d — effective realized-gains reader — complete 2026-09-27.** The
     realized-gains repository reads one SQLite snapshot, selects the latest
     numbered allocation set for a revised disposal, and retains the original
     lot events only for unrevised disposals. A second revision replaces the
     first in current gains. The original history remains available for audit;
     self-check, exports, correction chains, and the native write command are
     still required before replay is exposed to users.
   - **4e — effective self-check and export reads — complete 2026-09-27.**
     Self-check reconciles the current lot projection against original opening
     and unrevised events plus the latest replay allocation for each revised
     disposal. The bundle exports both the first committed disposal calculation
     and every numbered effective revision with its allocations. The original
     event rows remain immutable history. The native command still needs the
     same effective selection.
   - **4f — immutable correction-chain identity — complete 2026-09-27.** The
     unused candidate baseline gives an investment operation one earlier
     same-book predecessor, one immutable `replace` or `reverse` mode and a
     required reason; a predecessor can have only one successor. Pure reversals
     are terminal. The long-position intent reader omits superseded and
     reversed operations while retaining their immutable source facts, and
     the bundle exports each operation's correction identity. The
     native write command, replay revision installation, reconciliation preview
     and correction-chain register remain fenced until implemented together.
   - **4g — correction-aware current reads — complete 2026-09-27.** Realized
     gains and lot self-check select the same effective operation chain as
     replay. They omit original disposal events and effective revisions of a
     superseded sale while leaving the source rows and bundle history intact.
     The write command still must install the rebuilt lot projection and
     balanced correcting journals atomically before corrections are exposed.
   - **4h — manual long-sale pure reversal — complete 2026-09-27.** The native
     command takes a posted manual sale transaction ID and required reason,
     posts its exact inverse as a new transaction, records a terminal operation
     correction link, replays dependent long-position decisions, retires the
     source trade price and its derived observations, and applies the
     reconciliation boundary inside one SQLite transaction and one audit
     event. The original sale and its journal remain posted history. A
     reconciliation-only transaction version is eligible when its dated
     economic postings still match the source version. The endpoint includes
     a read-only reconciliation-impact preview. Imported fills are refused
     until source identity correction is part of the same command. Buy
     correction, sale replacement, short positions, and UI entry remain later
     slices; backdated writes stay fenced.
   - **4i — correction-chain read model — complete 2026-09-27.** A single
     authenticated read by transaction ID returns the immutable root and each
     successor, the effective end (or none after a terminal reversal), source
     provenance and the reversal eligibility hint. It serves transaction
     details without per-operation requests and keeps corrected posted
     journals visible as history. The write command still rechecks eligibility
     in its own transaction. UI presentation follows separately.
   - **4j — transaction-detail correction history and sale reversal — complete
     2026-09-27.** Investment transaction details load one correction-chain
     read, show original and successor entries with effective state, source and
     reason, and offer the native reversal only for an eligible manual sale.
     The action requires a reason, previews reconciliation checkpoints, and
     requires explicit override after showing affected statements. Loading,
     empty, error, and success states are provided on mobile and desktop.
     Imported sales remain visible without an unsafe action. Replacement and
     buy correction are the next write slices.
   - **4k — compound correction writer seam — complete 2026-09-28.** The
     existing transaction writer can insert multiple posted journals under a
     single verified same-book audit event, and ordinary sale entry and native
     replacement can prepare identical journal economics and disposal
     elections. Existing writes retain their one-audit behavior. The next
     slice uses this seam for an inverse plus a replacement journal; this
     foundation alone does not expose replacement.
   - **4l — latest manual long-sale replacement — complete 2026-09-28.** A
     posted manual sale may be replaced when it is the last lot-affecting
     intent for its exact long position. The native command keeps its date,
     holding, instrument and cost currency, but accepts corrected quantity,
     proceeds, fees and other sale fields. The replacement requires an
     explicit cost-basis method and treatment for each charge, so current
     defaults cannot silently change the correction. One SQLite transaction posts the
     old sale's inverse and the replacement journal beneath one audit event,
     links both journals to one replacement operation, rebuilds the position
     without the old disposal, records the replacement's elected disposal,
     retires the old trade price, and guards both journals at reconciliation
     checkpoints. The preview returns distinct affected checkpoints. A later
     buy or sale, an imported source, and an already corrected sale are
     refused before any partial write. Older dependent-sale replacement, buy
     correction, import source identity correction, and UI entry remain
     follow-up slices; backdated writes remain fenced.
   - **4m — old-buy correction preparation — complete 2026-09-28.** Normal
     buy entry and native replacement prepare the same exact journal and
     lot-opening terms. Replay now sorts a replacement by its correction
     root's original same-day operation slot, so a corrected old buy remains
     before a later sale recorded on that date. This is a deterministic
     replay rule only; the native buy correction command and backdated
     admission remain fenced until dependency, rollback and journal tests pass.
   - **4n — manual long-buy replacement with dependent-sale replay — complete
     2026-09-28.** A posted manual buy may be replaced with a full corrected
     buy at the same date, holding, instrument and cost currency. Under one
     audit event and SQLite transaction, post an inverse and replacement
     journal, create the replacement lot and immutable source facts, replay
     every dependent long disposal using its recorded method and effective
     specific-lot lineage, append allocation revisions, retire the old trade
     price, and enforce reconciliation boundaries for both journals. The
     preview returns distinct checkpoints. An impossible later disposal
     rejects the whole command, leaving old journal, lots, gains and price
     untouched. Imported buys stay fenced until source identity correction
     is part of the same command. UI entry, later-sale replacement and
     backdated admission remain follow-up work.
   - **4o — correction source-fact read model — complete 2026-09-28.** A
     single transaction-scoped read returns the original buy or sale's dated
     position, exact quantity, primary signed settlement, optional sourced
     gross, typed charge amounts and snapshotted treatments, settlement date,
     disposal method and original specific-lot election. The API includes
     import and already-corrected hints; only the write command decides
     eligibility and rechecks it atomically.
     Typed clients prepare the buy and sale replacement writes and their
     reconciliation previews. Transaction-detail correction forms follow.
   - **4p — transaction-detail manual buy correction — complete 2026-09-28.**
     The effective manual long buy offers a correction form in transaction
     detail. It pre-fills exact original quantity, net settlement, optional
     gross and snapshotted charges from 4o, keeps date, holding, instrument
     and cost currency fixed, requires a reason and explicit fee treatments,
     previews affected reconciliation checkpoints, and refreshes positions,
     lots, gains and history after the native replacement. The form uses the
     existing mobile buy entry fields and the shared exact amount parser.
     Sale replacement UI and older dependent-sale replacement follow.
   - **4q — pre-sale lot context for sale correction — complete 2026-09-28.**
     The correction read model reports whether a manual long sale is the
     latest effective position intent. A rolled-back replay of all preceding
     intents supplies exact available lots immediately before that sale,
     plus its specific-lot election mapped to effective acquisition lots.
     Historical source lot choices remain separate and immutable. This
   prepares a correction picker without using today's remaining balance,
   which already includes the sale being replaced.
   - **4r — transaction-detail manual sale correction — complete 2026-09-28.**
     An eligible latest manual long sale offers a prefilled correction form in
     transaction detail. It preserves exact recorded amounts and fee elections,
     requires a reason, and keeps the original date, holding, instrument and
     cost currency fixed. Specific-lot replacement starts from the effective
     prior election and lists lots available immediately before the sale;
     the form checks each allocation and the exact total without floating
     point. It uses the native replacement reconciliation preview, requires an
     explicit checkpoint override when affected, and refreshes investment
     reads after the write. Older dependent-sale replacement, imported source
     identity correction and backdated admission remain.
   - **4s — historical pre-sale context — complete 2026-09-28.** The correction
     read now finds an older effective manual sale within the ordered position
     intents and replays only the prefix before it. It reports those historical
     available lots and effective specific-lot elections even when later
     activity has consumed them. The existing latest-sale write eligibility
     stays false for that older sale. Replay is rolled back and creates no
     durable audit or lot event. This supplies the historical allocation
   context needed by the dependent-sale replacement command.
   - **4t — dependent-sale replacement simulation — complete 2026-09-28.**
     A repository check substitutes corrected quantity, proceeds, basis
     method and explicit lot choices into the older sale's original replay
     slot, then simulates every later position intent in a rolled-back
     savepoint. It identifies a later disposal that the corrected sale makes
     impossible and leaves journals, audit events, decisions and lot events
     untouched. The eventual write must repeat the check inside its own
     transaction before persisting the new decision and effective revisions.
   - **4u — older manual long-sale replacement — complete 2026-09-28.**
     The sale replacement command now accepts an effective manual long sale
     with later position activity. Inside one audited transaction it posts the
     inverse and corrected journals, simulates the corrected sale in its
     original replay slot, records its immutable historical disposal evidence,
     and appends effective revisions for the corrected and dependent sales.
     An impossible later disposal names its operation and decision and rolls
     back every effect. The reconciliation preview runs the same dependency
     simulation; a checkpoint still needs explicit override before the
     command invalidates it. Transaction detail uses the existing sale form
     and historical pre-sale lots. FIFO, LIFO, average and specific-lot
     replacements, corrected acquisition lineage, dependency refusal,
     reconciliation and self-check have targeted coverage. Imported source
     identity correction remains.
   - **4v — backdated long-buy admission — complete 2026-09-29.** The shared
     buy writer admits an acquisition before a later depletion by replaying
     the effective long-position intents inside the same SQLite transaction
     as the journal, lot, audit event, reconciliation guard and optional import
     identity. It appends effective disposal revisions and installs the lot
     projection only after the dependency simulation succeeds. FIFO, LIFO,
     average-cost and specific-lot choices retain their recorded meaning;
     dependent transfers still reject a changed carried basis. Imported
     source correction was left to the following slice.
   - **4w — source-linked imported fill replacement — complete 2026-09-29.**
     An imported buy or sale can use the native replacement command when its
     committed identity effect still links the exact source operation. The
     command rechecks that link inside the write transaction and retains the
     original identity, fingerprint, source effect, journal and lot facts as
     history. The correction chain links the replacement to that source
     operation, and a repeated import of the same fill remains deduplicated.
     Transaction detail exposes the correction with the recorded source
     identity and keeps orphan imported operations unavailable. Terminal
     imported sale reversal and source-file-driven correction are still gated.
   - **4x — terminal manual long-buy reversal — complete 2026-09-29.** A
     reasoned `reverse-buy` command posts the exact inverse journal, links a
     terminal correction operation, retires the source trade price, and
     installs effective long-position replay under one audit event and SQLite
     transaction. The read-only reconciliation preview first simulates the
     same removal. A later sale may reselect surviving lots under its recorded
     method, but insufficient quantity or a specific-lot election tied to the
     removed buy rejects the entire write. Reconciliation checkpoints require
     explicit override. A later slice admits terminal reversal of imported
     buys and sales only when their immutable correction lineage contains a
     committed source identity.
   - **4y — source-linked imported fill reversal — complete 2026-09-29.**
     The buy and sale reversal commands accept an imported fill or its manual
     replacement descendant when a committed import identity effect links an
     ancestor operation. The read hint and write transaction both check that
     lineage. Reversal preserves the original identity, source effect and
     fingerprint, so a repeated source row remains deduplicated even though
     the correction chain has no effective transaction. Orphan imported fills
     stay fenced. Source-file-driven corrections remain separate work.
   - **4z — changed-source fill review gate — complete 2026-09-29.** A
     Trading 212 order fill with an already committed stable fingerprint is
     compared with the latest accepted committed source snapshot. Changed provider
     payload is shown as a distinct review state and skipped at commit even
     if its staged dedupe status is changed. Locally resolved instrument and
     holding IDs are excluded from the comparison. This closes silent
     changed-fill deduplication; the buy source-revision correction arrives in
     4ac.
   - **4aa — source transaction review link — complete 2026-09-29.** The
     import batch read returns the committed original transaction for a
     Trading 212 fill identity. A changed-source row links directly to that
     transaction's detail panel and correction chain; a page reload preserves
     the link. The buy source-revision write command arrives in 4ac.
   - **4ab — atomic source revision foundation — complete 2026-09-29.** An
     append-only `import_source_revisions` record links an accepted staged
     Trading 212 fill to its original source identity and a descendant
     investment correction operation. The buy replacement writer has a
     transaction callback so the inverse journal, replacement lot replay,
     reconciliation invalidation, staged-row result, and revision record
     succeed or roll back together. The repository checks the latest accepted
     payload and rejects an unchanged or already committed row. Import
     preview and staging now compare with the latest accepted source snapshot.
     This backend writer seam is consumed by the buy command in 4ac.
   - **4ac — Trading 212 buy source revision — complete 2026-09-29.** Import
     review can accept a changed buy fill using its staged provider quantity
     and net settlement. The `/api/v1/imports/{batch_id}/rows/{row_id}/correct-buy`
     command posts an audited buy replacement and accepts the source revision
     in one transaction, including dependent long-sale replay and the normal
     reconciliation override/invalidation. A second provider revision follows
     the original source identity through the correction chain. Date,
     instrument, holding, cash account, and cost-currency changes are rejected
     until their own correction commands exist; sale source revisions arrive
     in 4ae, while cancellation revisions remain pending. The import review action exposes the supported
     scope and a reason field and remains available for skipped rows after
     batch commit. The batch read model identifies source identities whose
     original effect was a native buy, so cash-fallback rows do not offer the
     buy correction control. The writer rejects a revision staged before the
     latest accepted snapshot and rechecks batch eligibility in the same
     transaction, so an older row or a discarded batch cannot advance the
     source history. Positive buy quantity and negative owner-perspective
     settlement are required; cancellation-shaped payloads stay under review.
     Buy replacement preflight and transaction-detail availability follow the
     committed source ancestry, including an imported correction descendant
     whose identity effect remains on the original operation.
     A freshly staged provider revision can deliberately restore an earlier
     payload; an old staged row cannot be reused for that reversion. Source
     reconciliation-impact preview arrives in 4ad; sale follows in 4ae and
     cancellation commands remain open.
   - **4ad — source buy reconciliation preview — complete 2026-09-29.** The
     read-only `POST /api/v1/imports/{batch_id}/rows/{row_id}/correct-buy/reconciliation-impact`
     endpoint uses the same staged provider quantity/net settlement and source
     eligibility checks as the correction command. It returns distinct
     affected checkpoints for the inverse and replacement journals without
     accepting the revision, posting journals, changing lots, or invalidating
     checkpoints. Import review previews first, names the affected account,
     currency, and statement date, and requires explicit override before
     applying an affected correction. Changing the reason or selected row, or a failed command,
     clears the preview and override. The command rechecks eligibility and
     reconciliation at commit time; the preview does not reserve the staged
     source or guarantee dependent replay can succeed. Database query failures
     remain errors rather than being disguised as source eligibility conflicts.
   - **4ae — Trading 212 sale source revision — complete 2026-10-01.** Import
     review accepts changed sale quantity and positive owner-perspective net
     settlement through `/api/v1/imports/{batch_id}/rows/{row_id}/correct-sale`
     and its read-only `/reconciliation-impact` preview. The native sale
     writer atomically posts inverse/replacement journals, fresh disposal and
     clearing attribution, dependent allocation revisions, price retirement,
     reconciliation invalidation, staged acceptance and source revision under
     one import audit. Original source identity/effects remain unchanged;
     repeated and stale staged revisions are refused, and further revisions
     follow the effective correction descendant. BUY and SALE share source
     eligibility and scope checks. The composed batch read identifies native
     sale effects; cash fallback rows are not offered this action. The existing
     mobile review form provides a reason, names affected checkpoints and
     requires explicit override. The command retains the effective recorded
     basis method. A specific-lot settlement-only revision preserves its
     effective election; quantity changes require a manual explicit election.
     Nonpositive proceeds, cancellation-shaped quantities, date, instrument,
     account or currency changes stay under review. Named tests cover all four
     methods, older-sale dependent replay, read-only preview, source acceptance
     failure rollback and reconciliation guards. Source cancellation and wider
     source scope, other correction families and writer convergence remain
     T-75b gates.
   - **4af — Trading 212 fill taxonomy admission — complete 2026-10-01.**
     Preserve exact `fill.type` and `order.status` as staged provider evidence.
     Only explicit `TRADE` executions enter ordinary BUY/SELL, cash fallback,
     or BUY/SALE source correction/preview. Unsupported, future and missing
     types remain review-only regardless of local resolution/dedupe edits;
     they consume no committed identity and create no instrument/holding or
     financial facts. The composed review uses localized hold text and blocks
     ordinary editing/correction actions. Order lifecycle status is excluded
     from economic source comparison in staging, read models and revision
     acceptance. An executed TRADE on a cancelled order remains an execution.
     [Provider contract](https://docs.trading212.com/api/historical-events/orders_1)
     checked 2026-10-01 documents CANCELLED as order status, and corporate
     action/FOP variants as fill types; it supplies no documented execution
     cancellation fill type. Do not infer cancellation from order status,
     FOP_CORRECTION, negative quantities or opposite cash signs. Native source
     cancellation remains a T-75b gate pending verified provider evidence.
     Typed corporate-action producers/commands stay under #114/#115.
   - **4ag — shared buy/sale reversal orchestration — complete 2026-10-01.**
     Both native reversal repositories use the existing investment writer's
     commit/rollback, journal/audit creation and reconciliation invalidation.
     The writer accepts a transaction-scoped guard before journal insertion;
     each reversal rechecks its pinned source, correction status and committed
     import ancestry there. Checking after insertion would mistake its own
     successor for a prior correction. Kind-specific replay and price retirement
     remain effect callbacks inside that same transaction. Ordinary investment
     commands retain their existing writer interface and behavior. Named tests
     submit stale prepared reversals directly to each repository and inject
     a late checkpoint-update failure after journal, replay and price effects;
     source history, audit/journal counts, lot balances, gains, prices and active
     checkpoints remain intact, and retry succeeds. Compound replacements and
     their source-acceptance callback still need shared orchestration; this
     does not complete T-75b's other source/correction-family gates.
   - **4ah — shared compound replacement orchestration — complete 2026-10-01.**
     BUY and SALE replacements now use the same investment writer as ordinary
     commands and native reversals. It owns the SQLite transaction, one audit
     for ordered inverse/replacement journals, checkpoint invalidation, source
     acceptance and commit/rollback. Pinned source/import ancestry guards and
     historical-sale replay preparation run before either journal is inserted.
     Kind-specific lot creation, disposal decisions, dependent replay and price
     retirement remain atomic effects. Each journal retains its own affected
     checkpoint IDs; source acceptance runs after all checkpoint changes. Named
     tests cover both kinds' stale prepared commands, forced late checkpoint
     failure and acceptance callback failure, unchanged financial state and
     successful retry. Existing Trading 212 revision tests retain original
     source identity and staged acceptance behavior. No schema/API/UI scope
     changes; verified source cancellation, wider source revisions and other
     operation correction/backdating gates remain in T-75b.
   - **4ai — replay-aware buy replacement preview — complete 2026-10-01.**
     Manual/source-linked buy replacement and Trading 212 buy source-correction
     reconciliation previews now prove the proposed acquisition and dependent
     disposal/transfer replay before returning affected checkpoints. Preview
     and commit share journal/economic/lot preparation and the compound writer;
     preview rolls back the complete write transaction and never accepts source
     evidence or exposes temporary IDs. Using the actual replacement lot and
     correction chain preserves write-time selection, specific-lot lineage and
     root same-day ordering rather than approximating them with the old lot.
     An impossible disposal/transfer returns the existing named
     `INVESTMENT_BUY_DEPENDENCY` conflict. The write still repeats all guards and
     requires explicit reconciliation override. Named tests reproduce the old
     false-success preview under all four methods, check exact unchanged rows
     after failed/repeated successful previews, verify independent committed
     basis/gains, stale prepared sources, same-day ordering, transfer conflicts,
     staged source non-acceptance and late checkpoint-failure rollback. API and
     OpenAPI contracts cover both existing preview routes. This adds no date,
     account, instrument or currency correction scope; remaining T-75b source
     cancellation and correction-family/backdating gates stay open.
5. **Transfer and basis actions.** Transfer lots in kind across accounts
   without a gain; return of capital with exact basis effects; split and
   reverse split with conserved basis; cash in lieu with allocated fraction.
   The per-kind posting matrices, basis-allocation rules, dated eligibility,
   reconciliation guards, and unknown-basis behavior are fixed in
   [the slice 5 contract](../plans/investment-operation-slice-5-contract.md). Add manual
   commands before provider auto-acceptance. These address the
   common broker migration and everyday holding cases in R16 first. When
   these commands ship, test a changed transfer-out bridge adjustment and
   reconciliation refusal, plus return-of-capital and cash-in-lieu replay
   revisions against their posted and source facts.
6. **Short sale and cover (T-108).** Add side-aware lot opening and covering,
   disposal allocation/realized-result logic, portfolio/gains/self-check,
   mobile UI, import classification, and cash-only borrow costs. A negative
   journal share balance with no short operation stays an explicit warning.
7. **Compound corporate actions.** Implement merger/spin-off/stock dividend,
   then tender/rights/conversion as actual broker examples justify them.
   Require per-kind posting and basis-allocation specifications before code.
8. **Instrument-specific expansion.** Design bonds and derivatives as
   separate contracts when demanded by product scope. Do not call a generic
   buy/sell of an `option` or `bond` complete support for that instrument.

