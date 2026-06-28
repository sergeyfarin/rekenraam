# Transaction Ledger Core Plan

Status: **design reference — implemented.** The transaction-ledger schema,
lifecycle, reconciliation guard, and API slice described here all shipped (Phase 2,
see `implemented.md`). This document is kept for the schema/design rationale only;
it is not an active tracker. For current state see `docs/implemented.md`; for
what's next, `docs/roadmap.md`.

This document updates the transaction schema proposal after the tags,
categories, accounts, commodities, and system-account discussions.

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
- GnuCash's standard register order uses posted date, then a transaction-level
  number, then entry time. It can instead use the split/action field for the
  account shown in the register, demonstrating why transaction-wide same-day
  order and account-posting same-day order are separate concerns. Rekenraam uses
  explicit sequence fields rather than overloading check/action text.
  <https://wiki.gnucash.org/wiki/FAQ#Q:_How_do_I_order_transactions_in_a_register_so_deposits_are_before_withdrawals.3F>

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

## Lifecycle: Unsaved Entry, Draft, Posted, Voided, Soft-Deleted

This is not a single five-step ladder. There are **three persisted statuses**
(`draft`, `posted`, `voided`), plus **one pre-persistence condition** (unsaved
entry — a UI working copy with no database row) and **one independent flag**
(soft-delete — a nullable `deleted_at` orthogonal to status). Soft-delete is not
a status and not a point on the draft→posted→voided line.

### Unsaved entry (working copy — not a status)

An unsaved entry is in-progress UI entry that has **no `transactions` or
`transaction_versions` row yet**: a half-filled editor, autosave-pending form
state held only in the browser. This is the everyday "I started typing a
transaction and have not finished" condition.

Because nothing is persisted, an unsaved entry triggers **no** background work:
no FX/commodity-rate coverage, no base-currency conversion, no posting/balance
validation side effects. It becomes a `draft` or `posted` transaction only when
the user explicitly saves, autosave persists it, or the import commit step
persists it. "Unsaved entry" must never be called a "draft," and persisted
`draft` rows must never be called "unsaved."

### Persisted statuses — `transaction_versions.status`

- `draft`: reserved durable but unposted work that exists in the database and is
  intentionally excluded from the posted ledger and reports. It is **system-only,
  not a user-facing maturity step**. Manual entry never produces a draft and
  there is no user-facing "save as draft." No current workflow produces drafts;
  future import review, scheduled generation, or explicit crash-recovery
  autosave may activate the status and must own a dedicated review/discard
  surface. A future Unfinished Work inbox may link to those producer-owned
  surfaces. Draft does not mean unreconciled and never affects the posted ledger.
- `posted`: entered and participating in current ledger views and reports.
  "Entered" covers manual entry (the default and only outcome of manual entry),
  bank/file import after commit, and anything already existing in the ledger.
  Posted does not imply reconciled and stays directly editable. The user-facing
  maturity line is reconciled vs not reconciled, not draft vs posted: a posted
  transaction is freely editable and removable until a reconciliation checkpoint
  locks it.
- `voided`: the latest version intentionally removes this transaction from the
  posted ledger, but the transaction stays **visible in the UI marked as
  voided** as a deliberate reference/memory record. A voided transaction can be
  unvoided and edited.

### Soft-delete (separate flag, not a status)

Soft-delete is **distinct from voiding** and is a separate flag rather than a
`status` value. A soft-deleted transaction is hidden from the transactions table
and all ordinary views — it looks deleted to the user — while the row stays in
the database for audit/history and recovery.

The contrast is intentional:

- **Soft-delete** when the user knows the entry was a mistake and wants it gone
  from view. Looks deleted; recoverable from audit/history.
- **Void** when the user wants to keep the entry visible as a reference (for
  example, an entry that never appeared on the bank statement, kept while the
  user investigates whether it was a glitch).

Soft-delete is stored as nullable `transactions.deleted_at`; delete and restore
actions append actor, audit event, timestamp, and reason to
`transaction_deletion_events` (see `docs/conventions.md`). Soft-delete and void remain independently
reversible: a transaction can be unvoided, undeleted, or both, subject to the
reconciliation guard below.

### Background preparation and FX

Persisted work triggers FX/commodity-rate coverage: a future producer-created
`draft` row and every `posted` transaction both count. That preparation never
makes a draft part of ledger balances or reports. Unsaved entries and import
previews trigger nothing; future import rows trigger only after commit persists
them. Manual entry always saves directly to `posted`.

### Delete and discard

Physical (hard) delete is allowed only when a transaction has no posted or voided
versions. Provide a `DELETE /api/v1/transactions/{transaction_id}` or
`POST /api/v1/transactions/{transaction_id}/discard-draft` endpoint for
never-posted drafts. A later maintenance job may remove abandoned drafts, but
the first slice should make draft discard explicit. Removing a **posted**
transaction never hard-deletes: it uses soft-delete (hidden, recoverable),
void (visible, marked voided), or a corrective entry.

Lifecycle is independent of reconciliation state. A bank-imported transaction
can be draft while the user reviews it, and a manually entered transaction can be
posted but still uncleared.

## Void, Soft-Delete, And Correction Workflow

Voiding should append a new `transaction_version` with `status='voided'`. Do
not update the prior posted version in place. A voided transaction stays visible
in the UI, marked as voided, and can be unvoided (which appends a new
non-voided version) and then edited.

Soft-delete is a separate flag, not a `status` value, and is not a void variant.
A soft-deleted transaction is hidden from the transactions table and all ordinary
views while its rows stay durable for audit and recovery. It can be undeleted.
Soft-delete and void are independent: a transaction may be voided, soft-deleted,
or both, and each is reversed independently.

Current ledger views select the latest version per transaction and skip
soft-deleted transactions:

- soft-deleted: excluded from posted ledger and from the transactions table
- latest `status='posted'` (not soft-deleted): include its posting versions
- latest `status='draft'`: exclude from posted ledger
- latest `status='voided'`: exclude from posted ledger, but still shown in the
  transactions table marked as voided

Voiding and soft-deleting do not remove or rewrite `transaction_tags` or
`posting_tags`. Transaction tags are identity-level context and continue to
describe the historical transaction. Posting tags are keyed to posting lines. A
voided version retains a complete immutable posting snapshot, including
posting-line identity, ordering, and reconciliation state. Ledger queries exclude
those postings because the version status is `voided`, not because the posting
rows are absent. This keeps the visible voided row explainable and gives unvoid
an exact source snapshot.

**Guiding principle: if an operation changes a reconciled balance, it is
guarded** (explicit override plus invalidation of the affected active checkpoint
and all later active checkpoints for that account/commodity), never silent. A
checkpoint's `(statement_date, statement_account_sequence)` is the inclusive lock
boundary in that account register. A posting is inside the reconciled period when
its `entry_date` is earlier, or when its date is equal and its
`account_day_sequence` is at or before the checkpoint sequence.

Two situations change a reconciled balance:

1. **Reconciled posting facts change.** A reconciled posting's account,
   commodity, quantity value, quantity scale, or entry date changes.
2. **A posting enters or leaves a reconciled period.** Creating, editing,
   voiding, unvoiding, soft-deleting, restoring, or changing
   `account_day_sequence` across the inclusive checkpoint boundary is guarded,
   even if the posting is not itself marked `reconciled`.

This is broader than locking only reconciled posting facts: it protects the
integrity of a reconciled *period*, not just the individual reconciled rows. The
broader rule is intentional — a transaction dated inside a reconciled window but
whose postings were never individually flagged reconciled still shifts the
checkpoint's statement balance if added back or removed. A sequence move that
stays entirely before or entirely after the same checkpoint boundary does not
change its ending balance and is allowed. `transaction_day_sequence` affects
only global-table presentation and never reconciliation.

Always allowed without override (never changes a balance):

- Editing only category, description, payee, note, or tags — regardless of date.
  This lets a user recategorize or split the income/expense side of a reconciled
  transaction with the reconciliation intact, as long as no posting's account,
  commodity, amount, scale, or entry date moves and the transaction stays
  balanced.
- Any operation on a transaction that is already out of the ledger. Restoring a
  voided-and-soft-deleted transaction only restores visibility of an out-of-ledger
  record and changes no balance, so it is never reconciliation-guarded. (An
  unvoid, by contrast, *does* re-enter the ledger and is guarded under the
  period rule above.)

A corrective transaction that preserves the original reconciled record is always
an alternative to overriding.

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

## Same-Day Ordering

Dates remain the accounting facts; sequence numbers disambiguate order inside a
date. There are two deliberately independent values:

- `transaction_day_sequence` on `transaction_versions`, scoped to
  `(book_id, transaction_date)`, orders UI transactions in the global table.
- `account_day_sequence` on `posting_versions`, scoped to
  `(book_id, account_id, journal_entries.entry_date)`, orders postings in that
  account register. A transfer may therefore appear at sequence `2` in one
  account and sequence `5` in the other.

Both are positive integers, assigned at the end of their scope by default, and
normally hidden. The UI exposes Move earlier / Move later (rendered as Up / Down
for the active sort direction), not a free-form sequence input. `line_seq` still
orders split lines inside one journal entry and is unrelated to either day
sequence.

Ordering is a register and running-balance aid, not proof that a bank accepted a
posting. Moving a row never changes `reconciliation_status` or selects it into a
checkpoint; reconciliation membership remains an explicit account workflow.

Current rows in a scope must have a deterministic order. The application service
owns sequence allocation and reordering inside one SQLite write transaction.
Moving swaps adjacent current positions; insertion uses `MAX(sequence)+1`.
Gaps are allowed after date/account moves or hidden/voided rows, and the UI may
display dense ordinal positions derived from the sorted current rows. Sequence
gaps are not normalized automatically because an active checkpoint stores its
numeric cutoff. Because rows are append-only, a reorder appends replacement
transaction versions for every transaction whose current posting sequence
changes, under one audit event.

Ordering and pagination must use stable tuples:

- global list: `(transaction_date, transaction_day_sequence, transaction_id)`
- account register: `(entry_date, account_day_sequence, posting_version_id)`

Changing `transaction_day_sequence` is presentation-only. Changing
`account_day_sequence` affects intermediate running balances but not the day's
ending balance. It requires reconciliation override only when the posting crosses
an active checkpoint's `(statement_date, statement_account_sequence)` boundary;
moves wholly on one side remain allowed.

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
deleted_at TEXT
deleted_by_user_id INTEGER REFERENCES users(id)
deleted_audit_event_id INTEGER REFERENCES audit_events(id)
delete_reason TEXT
```

Constraints/triggers:

- correction target must be in the same book
- transaction cannot correct itself

Soft-delete current state lives on `transactions.deleted_at`. Each delete and
restore also appends an immutable `transaction_deletion_events` row containing
`book_id`, `transaction_id`, action (`soft_delete` or `restore`), timestamp,
actor, audit event, and reason. Restoring clears only current deleted state; it
does not erase lifecycle history.

```text
transaction_deletion_events
id INTEGER PRIMARY KEY
book_id INTEGER NOT NULL REFERENCES books(id)
transaction_id INTEGER NOT NULL REFERENCES transactions(id)
action TEXT NOT NULL CHECK (action IN ('soft_delete', 'restore'))
occurred_at TEXT NOT NULL
actor_user_id INTEGER NOT NULL REFERENCES users(id)
audit_event_id INTEGER NOT NULL REFERENCES audit_events(id)
reason TEXT NOT NULL
```

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
transaction_day_sequence INTEGER NOT NULL CHECK (transaction_day_sequence > 0)
payee_id INTEGER REFERENCES payees(id)
payee_name TEXT
description TEXT NOT NULL DEFAULT ''
external_ref_hint TEXT
note_markdown TEXT NOT NULL DEFAULT ''
metadata_json TEXT NOT NULL DEFAULT '{}'
needs_review INTEGER NOT NULL DEFAULT 0 CHECK (needs_review IN (0, 1))
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
- `transaction_day_sequence` is current-version ordering within
  `(book_id, transaction_date)`; historical versions may repeat prior positions
- inserting a superseding version must preserve reconciled posting account,
  commodity, amount, scale, and entry date unless the operation explicitly uses
  reconciliation override and invalidates the affected checkpoints

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
account_day_sequence INTEGER NOT NULL CHECK (account_day_sequence > 0)
account_id INTEGER NOT NULL REFERENCES accounts(id)
quantity_value TEXT NOT NULL
quantity_scale INTEGER NOT NULL CHECK (quantity_scale BETWEEN 0 AND 24)
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

`account_day_sequence` is independent of `line_seq`. It orders this posting
within its account register on `journal_entries.entry_date`; different postings
of one transfer may have different account-day positions.

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
- `quantity_scale` must not exceed commodity `max_quantity_scale` or the kind
  ceiling: 24 for crypto and 12 for all other commodity kinds
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

### reconciliation checkpoint ordering extension

Each account/commodity checkpoint adds:

```text
statement_account_sequence INTEGER NOT NULL CHECK (statement_account_sequence >= 0)
```

Together with `statement_date`, this is the inclusive same-day boundary. `0`
means the checkpoint ends before every posting on its statement date. On finish,
derive it as the greatest `account_day_sequence` among selected reconciliation
postings whose entry date equals the statement date, or `0` when none are
selected on that date. If an unselected same-day posting would fall before that
cutoff, the reconciliation UI lets the user move it after the selected statement
items before finishing. The checkpoint stores the chosen value permanently;
later sequence changes do not move the boundary silently.

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
  register rows ordered by `(entry_date, account_day_sequence,
  posting_version_id)`, with per-commodity running balances.

Read-model quantities keep the debit-positive ledger sign in `quantity_value`.
Responses also include `normal_quantity_value`; liability, equity, and income
balances are sign-flipped there for display and reporting. Values are normalized
exactly in Go using integer quantity plus scale, not floating point.

The global transaction list orders by `(transaction_date,
transaction_day_sequence, transaction_id)`. UI sort direction may reverse this
tuple but must not change the stored meaning of earlier/later.

## Reconciliation Guard

A reconciled balance must not be silently changed by a later operation.

Reconciliation is account- and commodity-scoped. Finished reconciliation
sessions create checkpoint rows containing statement date, statement balance,
same-day account sequence boundary, and the posting versions selected into the
reconciliation. These checkpoints
act as the trust/lock floor for account registers, reports, cash counts, and
statement-backed balances.

Because posting versions are immutable, reconciliation status changes are
implemented by appending transaction versions:

- marking postings `cleared`, `uncleared`, or `reconciled` creates a new
  transaction version containing the same financial postings with updated
  posting reconciliation status
- finishing a reconciliation requires zero difference and appends reconciled
  posting versions for the selected current postings
- cash reconciliation uses `source_kind='manual_cash_count'`; remaining
  differences are resolved by ordinary adjustment transactions before finish

The guard is:

- allow metadata-only edits such as payee, description, note, external
  reference, and tags
- treat `(statement_date, statement_account_sequence)` as the inclusive boundary
  for each account/commodity checkpoint
- require explicit reconciliation override when changing account, commodity,
  amount, scale, or entry date for a reconciled posting, or when any operation
  makes a posting enter or leave that boundary
- allow `account_day_sequence` moves that remain wholly before or wholly after
  the boundary; guard only a crossing
- never guard `transaction_day_sequence`, which affects only global presentation
- when the user confirms a guarded operation, invalidate the affected active
  reconciliation checkpoint and all later active checkpoints for the same
  account/commodity
- keep invalidated checkpoints visible for warning/history; do not physically
  delete checkpoint history
- defer closed-period guards until the period-close feature is designed

Examples:

- reconciled checking posting unchanged, expense category changed: allowed
- reconciled checking posting unchanged, expense side split across categories
  with the same total: allowed
- reconciled checking posting amount, account, commodity, or entry date changed:
  requires override and checkpoint invalidation
- posting moved from sequence `4` to `2` while the checkpoint ends at `5`:
  allowed because it remains inside the same boundary
- posting moved from sequence `6` to `4` while the checkpoint ends at `5`:
  requires override because it enters the reconciled period
- transaction create/void/unvoid/soft-delete/restore that adds or removes a
  posting inside the boundary: requires override and checkpoint invalidation,
  except restoring an already-voided transaction, which changes visibility only

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

Tag association write semantics:

- Creating a transaction writes the submitted transaction and posting tag sets.
- Updating or posting a draft transaction replaces the current transaction tag
  set and each submitted posting line's tag set in the same write transaction
  that appends the new `transaction_version`.
- Posting lines that are removed from the new current version have their
  `posting_tags` cleared so stale tags do not reappear in current register
  views.
- Voiding appends a `voided` transaction version but does not remove or rewrite
  `transaction_tags` or `posting_tags`; those associations remain historical
  context for audit/history views.
- Tag rename/archive changes the tag record itself. Existing associations keep
  pointing at the same tag id and display the tag's current label/status.

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
- `POST /api/v1/transactions/{transaction_id}/unvoid`
- `POST /api/v1/transactions/{transaction_id}/soft-delete` (hides a posted or
  voided transaction from the table; recoverable)
- `POST /api/v1/transactions/{transaction_id}/restore` (undeletes a
  soft-deleted transaction)
- `POST /api/v1/transactions/{transaction_id}/correct`
- `POST /api/v1/transactions/reconciliation-impact` for a proposed create
- `POST /api/v1/transactions/{transaction_id}/reconciliation-impact` for a
  proposed edit or lifecycle operation
- `POST /api/v1/transactions/{transaction_id}/move` to move a global same-day
  transaction earlier/later
- `POST /api/v1/accounts/{account_id}/postings/{posting_line_id}/move` to move an
  account-register posting earlier/later on its entry date
- `DELETE /api/v1/transactions/{transaction_id}` for never-posted drafts only
  (hard delete; soft-delete is a separate posted-transaction workflow)
- `GET /api/v1/accounts/{account_id}/register`

The `unvoid`, `soft-delete`, and `restore` endpoints implement the independent
void and deletion workflows above.

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

With no `status`, the ordinary list returns `posted` and `voided` transactions
and excludes reserved system drafts. `status=draft` is an explicit internal API
filter reserved for the future producer-owned workflow; the general table does
not expose it. Soft-deleted transactions are excluded and never appear for any
`status` value. A dedicated trash/recovery view surfaces them for restore.

The global cursor contains `(transaction_date, transaction_day_sequence,
transaction_id)` so same-day reordering remains deterministic.

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

The register cursor contains `(entry_date, account_day_sequence,
posting_version_id)`. Each response row exposes both sequence fields, although
ordinary UI hides the numeric values and offers Move earlier / Move later.

Use cursor pagination in the OpenAPI contract. Offset can be kept as an
internal development fallback, but the public API should not require stable
offsets over an append-only ledger.

`PATCH /api/v1/transactions/{transaction_id}` semantics:

- PATCH on a draft keeps the transaction in draft state, but still appends a
  new draft `transaction_version`; transaction version rows remain append-only.
- PATCH on a posted transaction creates a new posted version. This is the
  "directly editable" path for posted transactions; non-reconciled edits need no
  special acknowledgement.
- PATCH on a voided transaction is rejected; unvoid first (then it is editable),
  or use a corrective transaction if further accounting is required.
- PATCH on a soft-deleted transaction is rejected; restore it first.
- PATCH that changes a reconciled posting's account, commodity, amount, scale,
  or entry date, or moves a posting across an active checkpoint's date/sequence
  boundary, requires explicit reconciliation override and invalidates the
  affected checkpoints. PATCH that changes only category, description, payee,
  note, tags, global transaction sequence, or posting order wholly on one side
  of the boundary is allowed and keeps reconciliation intact.

Manual `POST /transactions` accepts only `status='posted'` and must run the same
period-impact check as edit/lifecycle operations. A backdated create that would
enter an active checkpoint boundary returns reconciliation-override-required;
the UI previews named checkpoints and may retry with
`reconciliation_override=true`. `draft` creation is available only to future
trusted internal producers, not the browser manual-entry route.

`POST /api/v1/transactions/{transaction_id}/void` must accept a JSON request
body with `change_reason` so the appended `status='voided'` transaction version
has audit attribution. `unvoid`, `soft-delete`, and `restore` likewise accept a
`change_reason` for audit attribution.

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
- changing reconciled posting facts requires override and checkpoint invalidation
- backdated create/edit/void/unvoid/soft-delete/restore that crosses an active
  `(statement_date, statement_account_sequence)` boundary requires override and
  checkpoint invalidation
- a voided+soft-deleted restore is exempt because it changes visibility only
- same-day posting movement wholly before or wholly after a checkpoint boundary
  is allowed; movement across it requires override
- global transaction same-day movement never requires reconciliation override
- global and register cursor pagination remains stable with same-day sequences
- category edit and category split allowed when reconciled account posting is
  unchanged, and the reconciliation stays intact
- description/payee/note/tag edit on a reconciled transaction keeps
  reconciliation intact
- draft delete allowed only for never-posted drafts (hard delete)
- posted financial records are not hard-deleted
- void appends a `voided` version and the transaction stays listed, marked voided
- unvoid restores an editable non-voided version
- soft-delete hides a posted transaction from the list but keeps the row durable
- restore undeletes a soft-deleted transaction
- soft-delete and void are independent: each can be applied and reversed without
  the other
- PATCH on a voided or soft-deleted transaction is rejected until unvoided/restored
- manual create rejects `draft`; the ordinary list excludes drafts by default
- voided versions retain their complete posting snapshot
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
