# Investment operation and subledger refactor plan

Status: proposed implementation plan, 2026-09-26. No behavior described below
is shipped merely because it appears here. ADR 0012 and ADR 0013 govern; this
plan makes their next slices concrete. R16 owns the near-term work, T-75b owns
native correction, and T-108 owns short sales and covers.

## Outcome and boundaries

Make the canonical journal, investment operation, and immutable lot effects
three linked views of the **same** economic event. Preserve the balanced
multi-commodity journal, exact decimal quantities, `book_id`, one audit event
per command, reconciliation guards, and the existing operational cost-basis
methods. Do not infer an operation from a negative position, a transaction
description, or a broker code. A report never creates accounting postings.

Target stock/ETF/fund and ordinary broker cash activity first. Model bond and
derivative contracts separately before allowing their lifecycle events to
post; `investment_instrument_versions.instrument_type` alone does not provide
coupon, maturity, exercise, assignment, or expiry semantics. Keep unsupported
broker activity in review, with its raw evidence and reason, rather than
force-fitting it into a dividend or ordinary sale.

This design follows the distinctions present in
[IBKR activity codes](https://www.interactivebrokers.com/campus/ibkr-reporting/reporting-integration/),
[OFX investment statements](https://financialdataexchange.org/common/Uploaded%20files/OFX%20files/OFX%20Banking%20Specification%20v2.3.pdf),
and [KMyMoney investment activities](https://docs.kde.org/trunk_kf6/en/kmymoney/kmymoney/details.investments.ledger.html).
Those are input taxonomies, not the app's database enum or accounting policy.

## Why the current shape will not stretch safely

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

## Proposed data contract

Treat an **investment operation** as the durable parent of one user or
imported economic action. It has `book_id`, stable domain `operation_kind`,
effective date, optional settlement date, created/recorded time, audit event,
optional correction/reversal relationship, and source evidence reference.
Operation codes remain application-validated strings, not a growing SQLite
`CHECK` enum. The broker's raw activity code and row ID remain separate from
the normalized operation kind. A correction is a new operation referencing the
original, never an update to the original facts.

Change the current mandatory one-to-one `investment_operations.transaction_id`
into an **operation-to-journal link** (`investment_operation_transactions`):
`(book_id, operation_id, transaction_id, role)`, unique on `transaction_id`.
The normal trade links one posted transaction; a basis-only event may link
none; a compound action may link several if distinct settlement dates or
broker records require them. Every linked transaction must be posted and in
the same book. Absence of a journal link is permitted only for command kinds
whose effects cannot change cash or security ledger balances; enforce this in
the service and self-check. A split that changes security quantity **does**
need balanced security postings. Do not create a zero-value transaction to
represent a basis-only change.

Add immutable **operation components** for exact broker and economic facts:
typed role (`gross_consideration`, `commission`, `other_fee`,
`transaction_tax`, `withholding`, `net_settlement`, `income`, `cash_in_lieu`,
etc.), exact signed coefficient and scale, commodity ID, optional account and
linked posting, source line reference, and sequence. Record whether source
economics are complete or net-only. A signed cash convention is from the
owner's perspective: receipts positive, payments negative.
Validate each trade's gross + charges = net settlement *per currency*, using
exact arithmetic; do not silently convert fees in another currency. Allocate
sale proceeds across consumed lots with one explicit exact rounding/remainder
rule, and snapshot that allocation beside the disposal decision. When
the source reports only net, record that as net with `gross_unknown`, not a
fabricated zero commission. Separate the **source cash facts** from the
operational basis treatment: the acquisition or disposal lot effect records
the exact basis/opening-proceeds amount and the policy/election that produced
it. Neither the broker's displayed tax basis nor a later report silently
overwrites that election. The journal posts the actual cash, expense/income,
and security legs and balances per commodity.

Link each immutable **lot effect** to `operation_id`, with side (`long` or
`short`), effect kind, quantity delta, basis/opening-proceeds delta, currency,
and source/destination lot references as appropriate. Retain the present
disposal-decision and allocation provenance, extending it to short covers.
Record operation-specific attributes in typed companion tables when they
are necessary to validate an event: rational split ratio; transfer source
and destination; corporate-action old/new instruments and basis allocation;
cash-in-lieu disposed fraction; broker trade and settlement IDs. Avoid an
opaque `metadata_json` contract for facts needed by gains, replay, or export.
Multiple lots may be affected by one operation, including lots in different
instruments. Historical lot events and original acquisition/opening amounts
are immutable; remaining quantity/basis and status are rebuildable state.

Keep operation provenance explicit: source type and stable external ID with
an idempotency key scoped to book/source/account, raw payload reference or
snapshot, import batch/row linkage where present, and the mapping version.
An importer maps `SHORT` to `short_sale` and `COVER` to `short_cover` only
when the row's meaning is unambiguous; a negative ordinary holding remains
an unclassified diagnostic. A source correction links its original source
record and normalized operation. Prevent one source row from posting twice.

### Operation families to support

| Family | Named operations and effects |
| --- | --- |
| Trades | Long buy/sell; short sale/cover; explicit gross, charges, net, trade/settlement dates; write-off. A trade crossing zero is two operations. |
| Cash and distributions | Cash dividend, reinvestment, fund capital-gains distribution, return of capital, interest, withholding, fees/rebates, margin or borrow interest, payment in lieu, cash FX conversion. Some need only journal postings; return of capital also changes basis. |
| Position movements | Security transfer in/out and between holding accounts, with carried lot basis and source/destination linkage; split/reverse split; stock dividend; cash in lieu. |
| Corporate actions | Merger, spin-off, conversion/class change, tender/partial redemption, rights issue/exercise/expiry, delisting. Each uses explicit multi-instrument effects, consideration, and basis allocation. Start with manual review and one supported action at a time. |
| Later instrument domains | Bond coupon/accrual, principal repayment, call/maturity, debt conversion; option/future open/close, exercise/assignment/expiry/cash settlement. Require instrument-specific contract terms and accounting decisions first. |

`ticker_change` may be an instrument identity/version change without a
position quantity or basis change. Do not manufacture a sale. A broker's
"add/remove shares" is a review category until its cause and carried basis
are known; otherwise it can conceal a transfer or corporate action.

## One command pipeline

Use one application-level `InvestmentOperationPlan` for the kinds that are
implemented. It contains normalized facts, planned journal postings, typed
lot effects and elections, required source links, account-role dependencies,
and expected reconciliation impact. Kind-specific builders own their
business rules; they call shared validation for exact amounts, dates,
commodities, account roles, side, source identity, and book boundaries.
Preview, reconciliation-impact preview, and commit use the **same plan**.

A shared repository executor opens one SQLite transaction, checks all
dependencies and idempotency, writes one audit event, the operation, its
posted journal transaction(s) and components, lot effects/decisions,
checkpoint invalidations, and import/provider acceptance links, then commits.
Any failure rolls back the whole set. Preserve the existing generic mutation
fence. Keep command-specific result types; a shared executor should not turn
all event logic into a giant switch or allow callers to append arbitrary
holding-account postings without corresponding lot effects.

### Replay and correction

Build the projection from ordered immutable effects for each affected
`(book_id, holding_account_id, commodity_id, side, cost_commodity_id)`.
Use effective date plus durable operation/effect sequence for same-day order.
On a correction/reversal, append the new operation and corrective journal
transaction(s), then replay from the first affected event within the same
SQLite transaction. Re-resolve only decisions explicitly being corrected;
preserve existing disposal method, chosen lots, profile version, and basis
elections. If the corrected history makes a later disposal/cover impossible,
return a conflict naming that dependent operation, leaving all records and
checkpoints unchanged. The reconciliation impact preview must account for
every changed posted account balance and date. Accept older imports only when
the replay and dependency checks can complete; before then, keep T-95's
chronological restriction. Independent negative dated balances can be
accepted and flagged, but cannot be silently reclassified as shorts.

## Delivery slices and gates

Each slice keeps the app runnable, updates OpenAPI/types, export/restore and
self-check contracts if it changes them, and adds named tests with independent
expected amounts and conservation checks. Stop at a gate before widening the
next family.

1. **Baseline contract and fixtures.** Specify exact equations and posting
   matrices for current buy/sell/dividend/reinvestment/write-off, including
   mixed-scale and multi-currency charges. Write migration and export contract
   tests against fresh and seeded candidate databases. **Revise the unused
   `0001` baseline** under ADR 0013: declare `BREAKING DEV DATABASE`, update
   checksum, fixture, upgrade/equivalence tests, and reset disposable developer
   databases. No installed v0.1 data needs a compatibility migration.
2. **Operation and component schema.** Introduce the parent/link/component
   tables and direct effect links. Preserve present APIs and postings. Route
   existing commands through the new writer; prove their journal, lots,
   gains, audit count, checkpoint behavior, and import idempotency remain
   equivalent for old cases. Version the investment export contract to
   include new operation/component/link files without losing old facts. No
   new user-facing operation yet.
3. **Exact trade economics.** Add gross, fee/tax components, currencies,
   net settlement, and settlement date to buy/sell and import mapping. Move
   gains from cash-posting inference to explicit per-disposal economics;
   preserve write-off as zero proceeds. Show gross, charges, and net in
   preview and UI. For fees in another currency, require explicit cash legs
   and rate/source facts; do not hide an implicit conversion.
4. **Replay and investment-native correction (T-75b).** Implement rebuild,
   correction/reversal, dependency conflicts, and reconciliation preview.
   Exercise a corrected old buy followed by several sells under each basis
   method, same-day ordering, import replacement, and failure rollback.
5. **Short sale and cover (T-108).** Add side-aware lot opening and covering,
   disposal allocation/realized-result logic, portfolio/gains/self-check,
   mobile UI, import classification, and cash-only borrow costs. A negative
   journal share balance with no short operation stays an explicit warning.
6. **Transfer and basis actions.** Transfer lots in kind across accounts
   without a gain; return of capital with exact basis effects; split and
   reverse split with conserved basis; cash in lieu with allocated fraction.
   Add manual commands before provider auto-acceptance.
7. **Compound corporate actions.** Implement merger/spin-off/stock dividend,
   then tender/rights/conversion as actual broker examples justify them.
   Require per-kind posting and basis-allocation specifications before code.
8. **Instrument-specific expansion.** Design bonds and derivatives as
   separate contracts when demanded by product scope. Do not call a generic
   buy/sell of an `option` or `bond` complete support for that instrument.

### Required acceptance cases across slices

- Buy and sell with gross 100, fee 2, and net 102/98 produce the expected
  journal postings, basis/proceeds, and gain; rebate and separately paid fee
  cases reconcile exactly. Source-only net remains marked as such.
- Every operation's linked posted journal and lot effects agree after each
  command and after rebuilding projections; failure injected at each write
  stage leaves no partial operation, audit, checkpoint invalidation, or import
  identity.
- A two-lot partial sale, later backdated buy, correction, short opening and
  partial cover preserve lot quantity, basis/opening proceeds, and side
  independently; an impossible dependent disposal is rejected atomically.
- Transfer and split preserve total basis. Return of capital changes basis
  without dividend income; cash in lieu consumes exactly its fractional lot
  allocation. A cross-instrument action reconciles old/new quantities and
  allocated basis in one operation.
- Exports and restore retain the operation, components, links, lot effects,
  policies, source IDs, and correction chain. A newly restored database gives
  the same positions, gains, warnings, and reconciled checkpoints.

## Decisions to settle before schema implementation

1. Confirm that ADR 0013's operation-to-journal wording permits zero or
   multiple linked transactions; amend the ADR in the schema slice if the
   link-table design is chosen. Do not leave two competing contracts.
2. Define the operational treatment of each fee/tax component for lot basis
   and proceeds, including source-only net and fees in another currency.
   Keep tax-jurisdiction projections outside this decision.
3. Choose whether the operational projection continues to store `int64`
   basis values or adopts the canonical coefficient string. Test upper-range
   and mixed-scale values before widening the schema.
4. Define which cash-only activities belong to investment operations rather
   than ordinary ledger transactions, and whether linking them to an
   instrument is optional. Their journal and export identity must stay
   coherent whichever way is chosen.
