# ADR 0012: Investment Ledger, Subledger, And Reporting Boundaries

## Status

Accepted

## Date

2026-08-29

## Context

An investment trade has facts that do not change with a reporting convention:
the instrument and quantity traded, cash paid or received, fees and withholding,
trade and settlement dates, account, provider identity, and source evidence.
Realized and unrealized gain are different. They depend on a cost-basis method,
period/date basis, jurisdiction or accounting purpose, prices, FX, and the time at
which those prices were known.

The existing implementation correctly posts buys and sells as ordinary balanced
multi-commodity transactions and keeps lots beside them. It also has one mutable
authoritative lot projection and a current-state gains endpoint. Treating that
projection as either the whole ledger or as disposable reporting state is unsafe:
specific-lot instructions and basis adjustments are durable elections, while one
mutable projection cannot represent several legitimate bases at once.

## Decision

Investment accounting has four explicit layers.

1. **Canonical journal.** Posted transactions record the economic event: exact
   security and cash quantities, fees, income, withholding and tax postings. Buy
   and sell settlement balances through the `commodity_trading` equity account.
   For an open position that account contains a raw per-commodity clearing/cash-
   flow residual, not a method-independent realized gain: part of the cash
   residual is still capital attributable to the remaining holding. A basis
   policy is required to split it. The journal does not change merely because a
   report selects FIFO, LIFO, average cost, or a valuation method. An external
   transfer-out bridge may record the carried value calculated under the
   committed operational basis election. If corrected economic history
   changes that value, append a guarded, dated adjustment; an alternative
   reporting profile alone never changes posted journal amounts.
2. **Investment subledger.** Immutable acquisition, disposal, transfer,
   corporate-action, basis-adjustment, and lot-election events record the
   relationships the journal alone cannot express. An investment mutation is one
   domain operation: its journal and subledger consequences commit, reverse, or
   correct atomically. Generic transaction mutation must not be allowed to strand
   one side.
3. **Basis and valuation projections.** Server-side read models derive positions,
   realized gain, unrealized gain, and comparisons under a named basis profile.
   A profile identifies its purpose/scope and exact rules, including cost-basis
   method, effective period/date basis, price knowledge cutoff, valuation source
   policy, FX method, staleness, reporting currency, and rounding. Projections do
   not mutate canonical facts. More than one projection may coexist.
4. **Optional accounting postings.** If a future workflow needs realized gain,
   unrealized revaluation, or a tax liability in formal statements, it creates an
   explicit, auditable transaction linked to the projection/run and policy that
   produced it. For realized gain this would be an explicit method-specific
   reclassification between `commodity_trading` equity and the chosen income or
   expense account, leaving the selected basis of the open position in clearing.
   This is not a second recognition of gain: for a fully closed position whose
   opening basis, net proceeds and charges have corresponding clearing legs in
   the same cost currency, the clearing residual nets to its operational gain
   or loss. An open position's residual has not yet been split. An external
   transfer with unknown carried basis, a fee expensed outside clearing, or an
   unmatched foreign-currency charge breaks that simple identity; a report
   must flag or explicitly adjust it before reclassification. A report must
   never silently manufacture ledger postings.

For operational gains, a trade charge enters acquisition basis or reduces
disposal proceeds only to the extent its cost-currency value enters
`commodity_trading`, whether as part of net settlement or an explicit
clearing leg. A separately expensed charge stays out of operational
basis/proceeds, so the combined after-expense result must include that
expense separately. Do not count one charge in both places. Cross-currency
charges require explicit value/conversion legs before they can enter the
same-currency clearing identity. Tax-specific treatment remains a separate
reporting-policy question.

Each recognized charge's operational treatment is an immutable election
with the selected policy and version recorded at commit. Resolve it from
the transaction, account, global book policy, then a named fallback; later
default changes do not reinterpret prior trades. Ordinary same-currency
commissions default to inclusion in clearing, preserving the existing
net-cash trade behavior.

The operational disposal decision remains durable even when alternative reports
are available. Every committed disposal snapshots its resolved method, the tier
that supplied it (transaction, account, global, or fallback), the applicable
policy/profile version, explicit allocations when used, and audit provenance.
Changing a default later does not reinterpret that decision.

The immutable event stream is authoritative. Current lot quantities, remaining
basis, and future per-profile basis states are rebuildable materialized
projections. They must conserve quantity and basis across sequential events and
must be checked against the current posted journal.

The first correction slice may keep one operational projection, but its schema and
events must not preclude projections keyed by basis profile. Multi-method and
multi-jurisdiction reporting is built only after the event and policy contracts are
specified; it is not implemented by repeatedly mutating the operational lots.

## Consequences

- Cost-basis choices do not alter the settlement postings for a trade, but an
  actual lot election is still a durable accounting fact.
- Broker-displayed basis may be retained as sourced evidence without automatically
  becoming the book's authoritative policy.
- Period and unrealized-gain questions belong to reproducible read models with an
  explicit `as_of` and knowledge cutoff, not to current-price UI arithmetic.
- Investment edit, void, delete, restore, correction, transfer, and corporate-action
  workflows need subledger-aware lifecycle behavior.
- The existing average-cost projection must be corrected before it is trusted: the
  sum of disposed and remaining basis must equal the pre-disposal pool after every
  sale, including a sequence of partial sales.
- The gains endpoint remains a current operational view until the later projection
  contract ships; it must not be described as tax reporting or a reproducible
  historical report.
- ADR 0008 remains in force for instruments, generic prices, provider trust, and
  ordinary ledger posting. This ADR refines its ledger/lot/reporting boundary.
