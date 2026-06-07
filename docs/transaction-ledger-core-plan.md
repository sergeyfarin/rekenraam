# Transaction Ledger Core Plan

Status: planning document for the Phase 2 ledger transaction slice.

This document updates the transaction schema proposal after the tags,
categories, accounts, commodities, and system-account discussions. It is not an
implementation migration yet. Before coding the slice, accept or update this
plan, then keep the migration, OpenAPI, backend services, frontend API helpers,
and tests aligned with it.

## Source Of Truth Updates Already Locked

- `docs/conventions.md` now uses `posting` as the canonical ledger term. "Split"
  remains acceptable user-facing copy for split transaction entry.
- `docs/conventions.md` and `docs/early-architecture-decisions.md` now lock the
  debit-positive sign convention:
  - positive postings are debits
  - negative postings are credits
  - asset and expense increases are usually positive
  - liability, equity, and income increases are usually negative

## External Patterns Checked

- GnuCash models transactions as collections of splits. Splits point to
  accounts and are also the unit used by lots for investment, AR/AP, inventory,
  and gains workflows.
  <https://www.gnucash.org/docs/v5/C/gnucash-guide/chapter_txns.html>
  <https://wiki.gnucash.org/wiki/Concept_of_Lots>
- hledger, Beancount, and Ledger model transactions as postings and attach
  commodity/cost/lot semantics to postings rather than to categories.
  <https://hledger.org/SPEC-lots.html>
  <https://beancount.io/docs/Basics/inventories>
  <https://ledger-cli.org/3.0/doc/ledger3.html>

## Terminology

`transaction` has two meanings in ordinary finance software, so the schema must
be precise:

- UI transaction: the thing the user recognizes, such as "Albert Heijn,
  100 EUR".
- `transactions`: stable database identity for that UI transaction.
- `transaction_versions`: immutable revisions of the UI transaction.
- `journal_entries`: dated balanced accounting events inside one transaction
  version.
- `posting_lines`: stable user-visible split-line lineage across versions.
- `posting_versions`: immutable accounting split/posting rows.

Most ordinary UI transactions have one current transaction version, one journal
entry, and multiple posting versions.

```text
UI transaction: Albert Heijn, 100 EUR

transactions
  id = 123

transaction_versions
  transaction_id = 123
  version_seq = 1

journal_entries
  entry_date = 2026-06-06

posting_versions
  Assets:Checking             -100 EUR
  Expenses:Groceries           +20 EUR
  Expenses:Groceries:Dairy     +40 EUR
  Expenses:Beauty              +40 EUR
```

Different categories do not create multiple database transactions. They create
multiple posting versions inside one transaction version.

## Book Boundary

All ledger identity and child tables should carry `book_id`, even while runtime
uses only book `1`.

Tables with `book_id`:

- `transactions`
- `transaction_versions`
- `journal_entries`
- `posting_lines`
- `posting_versions`
- `payees`
- `payee_versions`
- `transaction_tags`
- `posting_tags`
- future investment lots and lot assignments

Future cross-book transfers should not be a single transaction spanning two
books. A future multi-book feature should create one book-local transaction in
each book and link them with a separate cross-book transfer/link table. This
preserves the convention that a book is the accounting boundary.

## Intent Labels

Avoid three overlapping intent axes.

Recommended model:

- `transaction_kind`: UI/workflow hint on `transaction_versions`.
- `entry_kind`: structural hint on `journal_entries`.
- no generic `action` enum on `posting_versions` in the first schema.

`posting_versions.action` is deliberately omitted. Buy, sell, dividend, fee,
tax, transfer, and cashback semantics should be inferred from accounts,
transaction kind, entry kind, tags, metadata, and later domain-specific child
tables. If investment-specific action becomes necessary, add it to investment
tables rather than as a general posting enum.

`transaction_kind` values:

- `ordinary`: default. Purchases, category splits, ordinary income, cashback,
  refunds, receivables, and payables can all be ordinary.
- `transfer`: UI transfer workflow.
- `investment`: investment workflow. Sells require lot matching before posting.
- `opening_balance`: setup/opening workflow.
- `adjustment`: explicit manual adjustment/correction workflow.

`entry_kind` values:

- `ordinary`
- `transfer_leg`
- `exchange`
- `investment`
- `opening_balance`
- `adjustment`

These are not sufficient by themselves to prove correctness. The application
service owns workflow-specific validation. The database should enforce only the
stable invariants it can check reliably: same book, account eligibility,
posting dates, commodity/default-commodity consistency, version ordering, and
immutability.

## Draft, Posted, Voided

`transaction_versions.status`:

- `draft`: incomplete or unposted work. It may come from manual entry, import
  preview, scheduled transaction generation, or autosave. Draft does not mean
  unreconciled. Drafts have never affected the posted ledger.
- `posted`: participates in current ledger views and reports.
- `voided`: latest version intentionally removes this transaction from current
  ledger views.

Physical delete is allowed only when a transaction has no posted or voided
versions. Provide a `DELETE /api/v1/transactions/{transaction_id}` or
`POST /api/v1/transactions/{transaction_id}/discard-draft` endpoint for
never-posted drafts. A later maintenance job may remove abandoned drafts, but
the first slice should make draft discard explicit.

Draft identity is lifecycle state, not reconciliation state. A bank-imported
transaction can be draft while the user reviews it, and a manually entered
transaction can be posted but still uncleared.

## Void And Correction Workflow

Voiding should append a new `transaction_version` with `status='voided'`. Do
not update the prior posted version in place.

Current ledger views select the latest version per transaction:

- latest `status='posted'`: include its posting versions
- latest `status='draft'`: exclude from posted ledger
- latest `status='voided'`: exclude from posted ledger

Voiding does not remove or rewrite `transaction_tags` or `posting_tags`.
Transaction tags are identity-level context and continue to describe the
historical transaction after voiding. Posting tags are keyed to posting lines;
the voided version has no current postings, but historical posting-line context
remains available for audit/history views.

Implement now: if any current posting version is reconciled, ordinary
superseding or voiding should be blocked. Use a corrective transaction instead.

Defer: closed-period posting guards until period close exists. The transaction
slice must not attempt to enforce closed periods because no closed-period model
exists yet.

`transaction_versions.change_reason` is also the void reason when the new
version has `status='voided'`. Do not add a separate `void_reason` column unless
the product later needs structured void metadata beyond the ordinary audit
reason.

Corrections are separate transactions linked back to the original:

- `transactions.correction_of_transaction_id` nullable
- trigger or app validation: a transaction cannot correct itself
- trigger or app validation: correction target must be in the same book

Do not require the corrected transaction to be `voided`; corrective entries can
adjust a posted reconciled transaction while preserving the original.

## Version Links

`transaction_versions.supersedes_version_id` links one version to the previous
version of the same transaction.

Required constraints:

- `version_seq INTEGER NOT NULL CHECK (version_seq > 0)`
- `UNIQUE (transaction_id, version_seq)`
- trigger or app validation: `supersedes_version_id` must reference a
  transaction version with the same `transaction_id`

Use a trigger if practical. If implemented in app logic first, the migration and
docs must explicitly say so and tests must cover cross-transaction rejection.

## Payees

Payees need their own identity/version model before transaction UI becomes
useful.

`payees`:

- `id`
- `book_id`
- `created_at`
- `created_by_user_id`
- `created_request_id`
- `created_audit_event_id`

`payee_versions`:

- `id`
- `payee_id`
- `version_seq CHECK (version_seq > 0)`
- `effective_from TEXT CHECK (effective_from GLOB '????-??-??')`
- `recorded_at`
- `changed_by_user_id`
- `change_reason`
- `change_audit_event_id`
- `status`: `active`, `archived`
- `name`
- `normalized_name`
- `default_account_id` nullable
- `default_category_account_id` nullable
- `metadata_json`

API:

- `GET /api/v1/payees`
- `POST /api/v1/payees`
- `GET /api/v1/payees/{payee_id}`
- `PATCH /api/v1/payees/{payee_id}`
- `POST /api/v1/payees/{payee_id}/archive`
- `POST /api/v1/payees/{payee_id}/restore`
- `GET /api/v1/payees/{payee_id}/versions`

`transaction_versions` should store both `payee_id` nullable and a
`payee_name` snapshot, so imports/manual entries remain understandable if a
payee is later renamed or archived.

## Tables

### transactions

Stable identity for a UI transaction.

```text
id INTEGER PRIMARY KEY
book_id INTEGER NOT NULL REFERENCES books(id)
correction_of_transaction_id INTEGER REFERENCES transactions(id)
created_at TEXT NOT NULL
created_by_user_id INTEGER NOT NULL REFERENCES users(id)
created_request_id TEXT
created_audit_event_id INTEGER REFERENCES audit_events(id)
```

Constraints/triggers:

- correction target must be in the same book
- transaction cannot correct itself

### transaction_versions

Immutable revision snapshot.

```text
id INTEGER PRIMARY KEY
book_id INTEGER NOT NULL REFERENCES books(id)
transaction_id INTEGER NOT NULL REFERENCES transactions(id)
version_seq INTEGER NOT NULL CHECK (version_seq > 0)
supersedes_version_id INTEGER REFERENCES transaction_versions(id)
status TEXT NOT NULL CHECK (status IN ('draft', 'posted', 'voided'))
transaction_kind TEXT NOT NULL CHECK (
  transaction_kind IN ('ordinary', 'transfer', 'investment', 'opening_balance', 'adjustment')
)
transaction_date TEXT NOT NULL CHECK (transaction_date GLOB '????-??-??')
payee_id INTEGER REFERENCES payees(id)
payee_name TEXT
description TEXT NOT NULL DEFAULT ''
external_ref_hint TEXT
note_markdown TEXT NOT NULL DEFAULT ''
metadata_json TEXT NOT NULL DEFAULT '{}'
recorded_at TEXT NOT NULL
changed_by_user_id INTEGER NOT NULL REFERENCES users(id)
change_reason TEXT NOT NULL
change_audit_event_id INTEGER REFERENCES audit_events(id)
UNIQUE (transaction_id, version_seq)
```

Rules:

- row is append-only
- `book_id` must match `transactions.book_id`
- `supersedes_version_id` must be from the same transaction
- `transaction_date` is the user-facing register display date; `entry_date` on
  each `journal_entry` is the accounting effective date
- inserting a superseding version must be rejected when the superseded current
  version has reconciled postings

Attribution lives here. `posting_versions` do not carry independent
`changed_by_user_id` or `change_reason` because a posting version is always
created inside a transaction version.

`transaction_date` is not the audit capture time; use `recorded_at` for that.
For a single-entry bank import, map the bank's posted/booked date to both
`transaction_date` and `journal_entries.entry_date` unless the import source
separately provides a more precise accounting date. Store extra bank dates such
as value date or authorization date in metadata until the product needs first
class fields.

### journal_entries

Dated balanced accounting event.

```text
id INTEGER PRIMARY KEY
book_id INTEGER NOT NULL REFERENCES books(id)
transaction_version_id INTEGER NOT NULL REFERENCES transaction_versions(id)
entry_seq INTEGER NOT NULL CHECK (entry_seq > 0)
entry_date TEXT NOT NULL CHECK (entry_date GLOB '????-??-??')
entry_kind TEXT NOT NULL CHECK (
  entry_kind IN ('ordinary', 'transfer_leg', 'exchange', 'investment', 'opening_balance', 'adjustment')
)
memo TEXT NOT NULL DEFAULT ''
metadata_json TEXT NOT NULL DEFAULT '{}'
UNIQUE (transaction_version_id, entry_seq)
```

`book_id` is denormalized intentionally for register/report queries and
same-book triggers. Cross-book transactions remain out of scope.

### posting_lines

Stable split-line lineage for UI diffing and import matching.

```text
id INTEGER PRIMARY KEY
book_id INTEGER NOT NULL REFERENCES books(id)
transaction_id INTEGER NOT NULL REFERENCES transactions(id)
line_key TEXT NOT NULL
created_at TEXT NOT NULL
created_by_user_id INTEGER NOT NULL REFERENCES users(id)
created_request_id TEXT
created_audit_event_id INTEGER REFERENCES audit_events(id)
UNIQUE (book_id, transaction_id, line_key)
```

`line_key` should be server-generated. Importers may send an external line id,
but that should be stored in import metadata or a future import matching table,
not trusted as `line_key`. This keeps `line_key` stable and collision-free
inside the book.

### posting_versions

Immutable split/posting rows.

```text
id INTEGER PRIMARY KEY
book_id INTEGER NOT NULL REFERENCES books(id)
transaction_version_id INTEGER NOT NULL REFERENCES transaction_versions(id)
journal_entry_id INTEGER NOT NULL REFERENCES journal_entries(id)
posting_line_id INTEGER NOT NULL REFERENCES posting_lines(id)
line_seq INTEGER NOT NULL CHECK (line_seq > 0)
account_id INTEGER NOT NULL REFERENCES accounts(id)
quantity_value INTEGER NOT NULL
quantity_scale INTEGER NOT NULL CHECK (quantity_scale BETWEEN 0 AND 12)
commodity_id INTEGER NOT NULL REFERENCES commodities(id)
memo TEXT NOT NULL DEFAULT ''
reconciliation_status TEXT NOT NULL CHECK (
  reconciliation_status IN ('uncleared', 'cleared', 'reconciled')
)
cleared_on TEXT CHECK (cleared_on IS NULL OR cleared_on GLOB '????-??-??')
metadata_json TEXT NOT NULL DEFAULT '{}'
UNIQUE (journal_entry_id, line_seq)
```

`line_seq` is unique within each journal entry, not across the whole transaction
version. This is intentional: a delayed transfer can have two journal entries,
each numbered from line `1`.

`posting_line_id` should be NOT NULL. System-generated postings also receive
server-generated posting lines. This keeps every posting traceable.

Rules:

- row is append-only
- `book_id` must match transaction version, journal entry, posting line,
  account, and commodity
- account must allow postings on `journal_entries.entry_date`
- account must not be archived on `journal_entries.entry_date`
- `entry_date` must be on or after `opened_on`
- `entry_date` must be on or before `closed_on` when `closed_on` is set
- if account version has `default_commodity_id`, posting commodity must match
- `quantity_scale` must not exceed commodity `max_quantity_scale`
- if account version has `quantity_scale_override`, `quantity_scale` must not
  exceed that override either

Posting validation must load the account version as of
`journal_entries.entry_date`, not the current account version as of today.
`current_account_versions` is useful for current UI state, but it is tied to the
current date and must not be used to validate backdated transactions. The
service or trigger should perform a parameterized as-of lookup:

```sql
WHERE effective_from <= :entry_date
ORDER BY effective_from DESC, version_seq DESC
LIMIT 1
```

Use application service validation for all of the above. Add SQLite triggers
for same-book, account posting eligibility, account date validity, and
default-commodity/scale matching if the trigger remains understandable.
Balance checking should stay in the application service because it must
validate a full set of postings atomically.

## Balancing Rule

Every posted journal entry must balance per commodity.

Service algorithm:

1. Load all posting versions for the journal entry.
2. Group by `commodity_id`.
3. Normalize integer quantities to a common scale per commodity.
4. Require sum = 0 for every commodity group.
5. Require at least two postings.

This supports currencies, securities, crypto, reward points, and physical
commodities without floating point.

## Ledger Read Model

Balances are computed from the current transaction versions and posting
versions. Do not maintain account balance columns in the first implementation;
cached balances or reconciliation checkpoints can be added later when a concrete
performance or locking need appears.

Backend read-model endpoints:

- `GET /api/v1/ledger/account-balances`: account direct balances and subtree
  rollups as of a date, grouped by commodity.
- `GET /api/v1/ledger/category-totals`: income/expense category direct totals
  and subtree rollups for a date range, grouped by commodity.
- `GET /api/v1/ledger/net-worth`: asset/liability net-worth totals as of a
  date, grouped by commodity. `transfer_clearing` remains included so delayed
  transfers do not temporarily reduce net worth; `commodity_trading` is listed
  as an excluded system role and must not inflate ordinary net-worth reports.
- `GET /api/v1/accounts/{account_id}/register`: posting-shaped account
  register rows with per-commodity running balances.

Read-model quantities keep the debit-positive ledger sign in `quantity_value`.
Responses also include `normal_quantity_value`; liability, equity, and income
balances are sign-flipped there for display and reporting. Values are normalized
exactly in Go using integer quantity plus scale, not floating point.

## Reconciliation Guard

A reconciled posting version must not be silently changed by a later edit.

Because posting versions are immutable, the practical guard is:

- reject insertion of a superseding `transaction_version` when the current
  version has any `posting_versions.reconciliation_status='reconciled'`
- require a separate corrective transaction linked through
  `correction_of_transaction_id`
- defer closed-period guards until the period-close feature is designed

Implement this in application service first. If feasible, add a trigger on
`transaction_versions` insert that rejects `supersedes_version_id` when the
superseded version contains reconciled postings.

## Tags

Tags already exist. The transaction migration should add junction tables.

`transaction_tags`:

```text
book_id
transaction_id
tag_id
created_at
created_by_user_id
created_audit_event_id
PRIMARY KEY (transaction_id, tag_id)
```

`posting_tags`:

```text
book_id
posting_line_id
tag_id
created_at
created_by_user_id
created_audit_event_id
PRIMARY KEY (posting_line_id, tag_id)
```

Use `posting_line_id`, not `posting_version_id`, for the first implementation
because tags are user context and should usually follow the split line across
edits. If immutable historical tag snapshots become necessary, add
`posting_version_tags` later.

At write time, reject associations to archived tags for both
`transaction_tags` and `posting_tags`. Existing historical associations may
continue to display after a tag is archived.

## System Accounts

Current system roles should be extended in the transaction slice.

Existing:

- `opening_balance`: equity/equity
- `import_imbalance`: equity/equity
- `retained_earnings`: equity/equity
- `unassigned_income`: income/income
- `unassigned_expense`: expense/expense

Add with the transaction migration and seeding logic:

- `transfer_clearing`: asset/receivable, multi-commodity, system-owned
- `commodity_trading`: equity/equity, multi-commodity, system-owned

`transfer_clearing` represents money in transit between dated transfer legs.
It is an asset because the user still controls or expects the value.

`commodity_trading` is an equity-style hidden counterparty used to make
multi-commodity exchanges balance per commodity. It should not be an account
kind and should not be user-facing ordinary equity.

The `commodity_trading` balance represents cumulative FX and commodity exchange
differences. It is not ordinary net-worth equity and should be excluded from
net-worth reports by default through the system-role reporting filter.

Defer:

- `rounding_adjustment`

Do not add rounding adjustment until a real rounding workflow requires it.

## Investment Rule

Investment buys can be represented as postings and can create lots.

Investment sells must not allow arbitrary user-entered realized gains once lot
accounting is in scope. The app service must compute realized gain/loss from
lot assignments and then generate the gain/loss postings.

Until lot assignment is implemented:

- allow simple buys that create lots if the lot table is part of the slice
- allow cash dividends
- block investment sells that require realized gain/loss computation, or mark
  them as unsupported in the API

Do not let users manually type realized gain postings for investment sells and
call them authoritative lot accounting.

Future tables:

```text
investment_lots
- id
- book_id
- account_id
- commodity_id
- opened_by_posting_version_id
- opened_on
- status
- metadata_json

investment_lot_assignments
- id
- book_id
- lot_id
- posting_version_id
- quantity_value
- quantity_scale
- assignment_kind: acquire | dispose | transfer_in | transfer_out
- cost_value
- cost_scale
- cost_commodity_id
```

## Example Patterns

Albert Heijn category split:

```text
Assets:Checking             -100 EUR
Expenses:Groceries           +20 EUR
Expenses:Groceries:Dairy     +40 EUR
Expenses:Beauty              +40 EUR
```

Delayed transfer:

```text
Entry 1, 2026-06-01:
Assets:Checking             -100 EUR
Assets:Transfer Clearing    +100 EUR

Entry 2, 2026-06-03:
Assets:Transfer Clearing    -100 EUR
Assets:Savings              +100 EUR
```

FX exchange:

```text
Assets:EUR Checking        -1000 EUR
System:Commodity Trading   +1000 EUR

Assets:USD Checking        +1080 USD
System:Commodity Trading   -1080 USD
```

Payment on behalf of someone:

```text
Assets:Checking              -80 EUR
Assets:Receivable:Alex       +80 EUR
```

Refund expected:

```text
Assets:Receivable:Refund     +40 EUR
Expenses:Clothing            -40 EUR
```

## API Slice

Minimum transaction endpoints:

- `GET /api/v1/transactions`
- `POST /api/v1/transactions`
- `GET /api/v1/transactions/{transaction_id}`
- `PATCH /api/v1/transactions/{transaction_id}`
- `POST /api/v1/transactions/{transaction_id}/post`
- `POST /api/v1/transactions/{transaction_id}/void`
- `POST /api/v1/transactions/{transaction_id}/correct`
- `DELETE /api/v1/transactions/{transaction_id}` for never-posted drafts only
- `GET /api/v1/accounts/{account_id}/register`

Minimum query parameters:

`GET /api/v1/transactions`:

- `account_id` optional
- `payee_id` optional
- `q` optional free-text search over `payee_name`, `description`, and external
  reference hints
- `status` optional, one of `draft`, `posted`, `voided`
- `kind` optional, one of the `transaction_kind` values
- `after_date` optional, inclusive `YYYY-MM-DD` filter on `transaction_date`
- `before_date` optional, inclusive `YYYY-MM-DD` filter on `transaction_date`
- `limit` optional with a bounded default
- `cursor` optional opaque pagination cursor

`GET /api/v1/accounts/{account_id}/register`:

- `after_date` optional, inclusive `YYYY-MM-DD` filter on accounting
  `entry_date`
- `before_date` optional, inclusive `YYYY-MM-DD` filter on accounting
  `entry_date`
- `status` optional, default `posted`; drafts are included only when requested
- `limit` optional with a bounded default
- `cursor` optional opaque pagination cursor

The account register response is posting-shaped, not transaction-shaped. Each
row represents one posting to the requested account, ordered by that posting's
`journal_entries.entry_date` and a stable posting tie-breaker. The row also
carries transaction and journal-entry context. This matters for delayed
transfers and clearing accounts where one UI transaction can produce multiple
dated register rows for the same account.

Use cursor pagination in the OpenAPI contract. Offset can be kept as an
internal development fallback, but the public API should not require stable
offsets over an append-only ledger.

`PATCH /api/v1/transactions/{transaction_id}` semantics:

- PATCH on a draft keeps the transaction in draft state, but still appends a
  new draft `transaction_version`; transaction version rows remain append-only.
- PATCH on a posted transaction creates a new posted version.
- PATCH on a voided transaction is rejected; use a corrective transaction if
  further accounting is required.

`POST /api/v1/transactions/{transaction_id}/void` must accept a JSON request
body with `change_reason` so the appended `status='voided'` transaction version
has audit attribution.

Payee endpoints are listed in the Payees section and should land before or with
transaction entry UI.

## Validation And Tests

Required backend tests:

- ordinary category split balances
- salary split balances
- credit-card purchase and payment signs
- same-day transfer
- delayed transfer through `transfer_clearing`
- FX exchange through `commodity_trading`
- payment on behalf of someone through receivable
- purchase made but not paid through payable
- refund expected through receivable
- opening balance through `opening_balance`
- posting before `opened_on` rejected
- posting after `closed_on` rejected
- archived or non-posting account rejected
- default commodity mismatch rejected
- unbalanced journal entry rejected
- per-commodity imbalance rejected
- `version_seq > 0` and unique sequence enforced
- cross-transaction `supersedes_version_id` rejected
- self-correction rejected
- superseding reconciled posting rejected
- draft delete allowed only for never-posted drafts
- posted financial records are not hard-deleted
- payee create, update, archive, restore
- transaction stores `payee_name` snapshot
- payee rename/archive does not rewrite historical transaction snapshots
- archived tag rejected when creating `transaction_tags` or `posting_tags`

Frontend/API validation:

- OpenAPI schemas for transaction, journal entry, posting line, and posting
  version request/response shapes
- generated frontend types
- typed API helpers
- later UI screens must use "posting" internally and may use "split" only in
  user-facing copy where clearer
