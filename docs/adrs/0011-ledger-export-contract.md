# ADR 0011: Ledger Export Contract

## Status

Accepted

## Date

2026-08-23

## Context

Exporting core ledger data as CSV and QIF is a mandatory product requirement
(`docs/product-requirements.md`), not optional polish: a self-hosted finance app
whose data cannot leave it has failed its own premise. R3 delivers it
(`docs/plans/data-portability-plan.md`).

An export is an external contract. Once someone builds a script against it, its
column set, its filter semantics, and the meaning of its numbers cannot drift
without breaking them silently. Four decisions in particular are arithmetic, not
presentation, and belong in a decision record rather than in code comments:
which records leave, what a filter selects, what an account balance in an export
means, and how the export is read while the app keeps running.

The ledger these decisions are made of: postings carry an exact coefficient and
scale; every journal entry balances per commodity, scale-aware; categories are
income and expense accounts; and system accounts such as `commodity_trading`
carry the counterpart legs of investment trades.

## Decision

### 1. Grain, order, and record policy

The export is **posting-level**: one row per posting of the current version of
each posted, non-deleted transaction, ordered by entry date, transaction, entry
sequence, then line sequence, with the posting version's own id as a final
tiebreaker. That order is total, so two exports of one snapshot are
byte-identical.

Drafts, voided versions, superseded versions, and soft-deleted transactions are
**not** exported. The portable export carries current posted state; the
audit-complete history stays in the SQLite backup. The policy travels with the
file rather than living only here.

**Exports include system accounts. Reports exclude them.** A report answers a
human question and drops `commodity_trading` and its siblings; an export that
dropped them would emit transactions that do not balance. The divergence is
deliberate and permanent.

### 2. Filters select whole journal entries

A filter selects **journal entries**, never individual postings, and every
posting of a selected entry is exported — including postings in accounts,
commodities, and dates the filter did not name.

Selection composes in a fixed order:

1. The date basis picks the candidate set. `date_basis=entry` selects entries
   whose `entry_date` is in range; `date_basis=transaction` selects current
   transactions whose `transaction_date` is in range and then **every** entry of
   those transactions, including entries dated outside it.
2. Account and commodity filters narrow that set entry by entry: an entry stays
   if it contains at least one posting matching each supplied dimension.
3. Every posting of a surviving entry is exported.

Consequently the export **balances per journal entry under every filter**.
Per-transaction balance holds when every entry of that transaction is present,
which `ledger.csv`'s `transaction_complete` column reports per row and a
manifest's `all_transactions_complete` reports archive-wide.

`in_scope`, where an export reports it, is mechanical:
`(no account filter OR the account is in the resolved account set) AND (no
commodity filter OR the commodity was requested)`. It is true for every row of
an unfiltered export; false marks a row present only as a counterpart.

### 3. The flat CSV is unfiltered; scope belongs to the archive

`ledger.csv` downloaded on its own takes **no** scope parameters. A downloaded
file cannot carry response headers, and a scoped CSV with no manifest is an
artifact nobody can reproduce or audit later. Scope parameters on that endpoint
are refused with `EXPORT_SCOPE_UNSUPPORTED` rather than ignored. Scoped exports
are produced as `bundle.zip`, which carries `manifest.json`.

### 4. A trial balance in an export is a period statement with two closing figures

Where an export states balances, it states them per account and commodity as:
`in_scope`, `opening_balance`, `exported_in_range_movement`,
`exported_out_of_range_movement`, `excluded_in_range_movement`,
`derived_closing_balance`, `actual_closing_balance`, with three exact
identities:

- `opening_balance + exported_in_range_movement ≡ derived_closing_balance`
- `derived_closing_balance + excluded_in_range_movement ≡ actual_closing_balance`
- `exported_in_range_movement + exported_out_of_range_movement ≡` the exported
  movement for that account and commodity

Two things force this shape. Entry-complete selection pulls counterpart postings
from accounts the filter never named, so their exported movement is only part of
their real movement. And transaction-basis selection exports entries dated
outside the range, which a single closing figure would double-count or omit.

**The rule underneath: the selection basis decides which rows are in the file;
balances are always entry-date arithmetic.** The derived figure is never called
the account's closing balance, because for a counterpart account it is not one.

### 5. Amounts, identifiers, and text

Amounts are rendered from the stored coefficient and scale by `exact.Decimal`:
point separator, no group separators, sign as a leading `-`, stored scale kept.
No float touches an export at any layer.

**Scale, in two halves.** A figure that accumulated something keeps the scale it
accumulated at; figures are never restated at a common scale, because deepening
costs a digit per decimal place against the 38-digit coefficient ceiling and one
figure's precision must not widen another's. This is the rule the cashflow
report reached by reverting the opposite (`04a1d354`). A figure that accumulated
**nothing** is the exception: zero is the same number at every scale, so
deepening it multiplies zero by a power of ten and can widen nothing, and it is
written at its row's deepest scale rather than as a bare `0` beside a
neighbour's `0.00`.

**A derived total may exceed the stored-coefficient ceiling, and is still
written.** A 38-digit posting and a 2.50 posting in one account sum to 39
digits; both postings are legal and the sum is exact. That ceiling governs
values the app stores and returns as coefficient strings, not decimal text in a
file, so aggregates render through `exact.DecimalFromBig`. Refusing would fail
an entire archive over a figure it can write exactly — and an export is what a
user reaches for when something is already wrong.

Column headers and enumerated values are stable English identifiers, not
localized strings — a machine-readable contract must not change shape with the
reader's locale. Only the surrounding UI is translated.

Every account row carries both `account_id` (the stable join key) and
`account_path` (the colon-separated hierarchy plain-text-accounting tools read).
The path uses each account's **current** name and parent: an as-of-entry-date
path would print one account under two paths in a single file.

The CSV is UTF-8 with a byte order mark, LF line endings, RFC 4180 quoting, and
a header row first — no context block, unlike R2's report CSVs, which are
snapshots of a screen rather than a machine-readable table.

### 6. Schema stability

`ledger.csv`'s column list is published by `GET /api/v1/exports/preview` and is
append-only: columns are added at the end, never reordered, renamed, or removed
within a major export schema version. A change that cannot be made that way
requires a new ADR.

The pre-release investment foundation extends the bundle archive to manifest
schema version 2. It adds operation-to-journal links, typed dates, exact
source components, lot-opening facts, lot events and effects, and versioned fee
policies as separate CSV files. `prices.csv` appends approximate, source
transaction version, and audit identifiers. `disposal-decisions.csv` appends
operation ID and position side so each election stays tied to its operation.
Slice 5 adds separate transfer fact and lot-link CSV files; known carried
basis and an explicitly unknown original acquisition date remain distinguishable.
The original posting-level
`ledger.csv` contract and QIF archive version remain unchanged. The added
investment files retain their stored coefficients and scales so an unknown
gross amount is never inferred from a net settlement.
The same version adds `import-identities.csv` and
`import-identity-effects.csv` for committed source fingerprints and their
ordered operation/transaction links. Import profiles, batches, and staged raw
rows remain outside this portable ledger archive.
The same pre-installation schema also carries
`disposal-revisions.csv` and `disposal-revision-allocations.csv`. Original
decision and allocation files retain the first committed calculation; the
highest numbered revision per decision supplies the current effective
allocation. Every prior revision remains in the archive with its triggering
operation and audit event, so a restore or external reader can distinguish
original evidence from the present result.

### 7. Exports read through a dedicated read-only connection, in one snapshot

The main pool is `SetMaxOpenConns(1)` (ADR 0004): a read transaction held there
for the length of an export would stall every other request. Exports, the ledger
self-check, and the nightly backup's copy therefore read through a second,
read-only pool (`db.OpenReadOnly`, `mode=ro` with `query_only`), and each export
runs inside one deferred read transaction so every file of an archive sees one
state. WAL readers are concurrent with the single writer by design, so this
costs no write throughput.

The pool must be opened after migrations have run, or it would hold a snapshot
of a schema the app no longer speaks. `cmd/rekenraam` does so; nothing enforces
it, because a read-only pool cannot itself tell a pre-migration database from a
post-migration one.

## Consequences

- A consumer can rely on the column set, the order, and the balance guarantee,
  and can detect an incomplete transaction group per row rather than by
  inference.
- A beancount or ledger writer is a mechanical transform of this shape rather
  than a second read model — the acceptance test asserts the four fields such a
  transform requires (`entry_date`, `account_path`, `quantity`, `commodity`)
  plus a documented narration fallback.
- Reports and exports now diverge visibly on system accounts. Anyone comparing
  a report total to an export sum must account for that, which the export's own
  documentation states.
- The read-only pool is a second connection pool to keep correct: it must be
  opened after migrations, and it must never be handed to a writing service.
- Filters that "should" have cut a transaction in half instead return it whole
  or partially, labelled. A caller wanting exactly the rows it named must filter
  the result itself — the export will not break double entry to oblige.

## References

- `docs/plans/data-portability-plan.md` — the R3 plan this contract serves
- ADR 0004 — SQLite connection, migrations, and backup
- ADR 0009 — lossless quantity precision
- `docs/design/categories-design.md` — categories are income/expense accounts

### Lot-state export refinement (2026-09-30)

Manifest schema version 3 adds `investment-lot-state.csv`: lot ID, status,
remaining quantity/basis coefficients and scales, update time, actor and audit
ID. `lots.csv` retains its joined current-state summary and existing columns;
`investment-lot-facts.csv` retains immutable opening evidence. This reflects
ADR 0013's physical identity/projection split and carries both layers in a
portable snapshot. Missing state is an integrity finding and cannot be exported
as a fabricated zero balance.

### Disposal-clearing attribution refinement (2026-09-30)

Manifest schema version 4 adds `disposal-clearing-allocations.csv`, with
`decision_id`, `posting_version_id`, `proceeds_value`, `proceeds_scale`.
Coefficients are canonical signed exact integers. Each portion represents
operational proceeds, the opposite sign of the allocated clearing quantity.
Sum portions per decision to its immutable operational proceeds; sum portions
per clearing posting to the negated posting amount, aligning scales exactly.
One decision may span multiple dated legs and multiple decisions may share a
posting. The links remain original audit evidence after basis replay or
correction; replacement decisions have their own links. A zero-proceeds
write-off may have no cost-currency portion. Existing CSV columns are retained.

### Pooled internal transfer refinement (2026-10-03)

Within schema version 6, `investment-transfer-facts.csv` appends
`basis_allocation` (`selected_lots` or `average_cost_pool`), the applied
`cost_basis_method`, `method_resolution_tier` (`account`, `global`,
`fallback`, or `position_lock` when an open position's method family
overrode the default) and the resolving `method_account_version_id` /
`method_profile_version_id`. They are empty for external transfers. A pooled
transfer's per-source-lot depletions are `transfer_out` rows with
`cost_basis_method = average_cost`; its links carry each lot's pool-rate basis,
with the final touched lot absorbing the exact remainder.

### Split refinement (2026-10-03)

Manifest schema version 6 adds `investment-split-facts.csv` (operation,
holding account, security, effective date, lowest-terms ratio, source evidence,
audit), `investment-split-revisions.csv` and
`investment-split-revision-effects.csv`. A split's first committed per-lot
quantity changes are `split_adjustment` rows in `investment-lot-events.csv`
with signed deltas and zero basis. Replay appends a revision per split and cost
currency only when those per-lot effects change; the highest revision supplies
the current effect, and its sum equals the original aggregate pinned by the
posted journal. Existing files and columns are unchanged.

### Projected-basis knowledge refinement (2026-10-01)

Manifest schema version 5 appends `basis_knowledge` to `lots.csv` and
`investment-lot-state.csv`. It describes remaining projected basis, independently
of immutable opening evidence. Known basis includes known zero. Unknown basis
has an empty `remaining_cost_basis` summary cell and empty coefficient/scale
cells in state CSV; it is never exported as zero. Quantity remains exact and
available. Existing columns retain their order. Missing state or inconsistent
knowledge/amount pairs remain export errors, preserving the integrity boundary.

### Merged opening-fact source (2026-10-03, T-124)

ADR 0013's T-124 refinement stores immutable lot opening facts on
`investment_lots` (with its new `operation_id`) instead of a duplicate
`investment_lot_facts` table. `investment-lot-facts.csv` keeps its file name,
columns, order and values: one row per operation-opened lot, with
`consideration_value`/`consideration_scale` taken from the lot's opening
basis. The manifest stays at schema version 6. A lot with no opening
operation is not an opening fact and is not listed, exactly as before.

### Transfer link revisions (2026-10-03, T-132)

Manifest schema version 7 adds `investment-transfer-link-revisions.csv`:
`revision_id`, `operation_id`, `link_seq`, `revision_seq`,
`caused_by_operation_id`, `supersedes_revision_id`, `source_lot_id`,
`carried_basis_value`, `carried_basis_scale`, `created_at`, `audit_event_id`.
`investment-transfer-lot-links.csv` keeps each link's first committed carried
basis; the highest `revision_seq` per `(operation_id, link_seq)` is the basis
the destination lot currently carries and the source lot it is taken from.
Quantity, destination lot and original date are only in the link file. Existing
files and columns are unchanged.

### Split adjustment journals (2026-10-03, T-129)

`investment-split-revisions.csv` appends `adjustment_transaction_version_id`.
It names the adjustment journal a revision posted when its effects move a
different aggregate quantity than the previous effective effects; it is empty
when the aggregate is unchanged. The journal appears in the ledger files like
any posted journal and in `investment-operation-journal-links.csv` with role
`split_adjustment`. A split's `primary` plus `split_adjustment` journals sum to
its effective effects. The manifest stays at schema version 7; existing columns
keep their order.
