# Accounts And Institutions System Draft

Status: draft for review, not yet an accepted source of truth.

This document proposes the Phase 1 account and institution system before any schema,
backend, or frontend implementation. It translates the active requirements, the
archived FastAPI implementation, and patterns from mature finance applications
into the current Go/SvelteKit/SQLite shape.

## Goals

- Give every institution and account a permanent internal integer id.
- Keep `book_id` on all financial tables even while runtime remains single-book.
- Model accounts as a tree that can support ordinary cash accounts, liabilities,
  income and expense categories, investments, securities, commodities, and
  rewards balances.
- Preserve auditability by treating account and institution attributes as
  append-only versions instead of mutable fields.
- Make closing and archiving explicit lifecycle states, never hard deletes.
- Seed required system accounts for double-entry workflows.
- Support multi-currency from the first account slice.
- Keep the first implementation small enough to ship, while leaving room for
  reconciliation, import cleanup, investments, and export.

## Inputs Reviewed

Active repo source of truth:

- `docs/product-requirements.md`
- `docs/conventions.md`
- `docs/early-architecture-decisions.md`
- `docs/adrs/`
- `docs/archive-fastapi-backend-review.md`

Archived FastAPI implementation:

- `.archive/apps/api/src/rekenraam_api/db/models/accounts.py`
- `.archive/apps/api/src/rekenraam_api/db/models/metadata.py`
- `.archive/apps/api/src/rekenraam_api/services/accounts.py`
- `.archive/apps/api/src/rekenraam_api/services/metadata.py`
- `.archive/apps/api/alembic/versions/0001_initial_schema.py`

External application patterns:

- GnuCash supports the five accounting classes plus practical subtypes such as
  bank, cash, credit card, stock, mutual fund, opening-balance equity, and
  parent/child type restrictions:
  <https://wiki.gnucash.org/docs/C/gnucash-manual/acct-types.html>
- KMyMoney treats institutions as optional account grouping/default metadata and
  separates asset/liability account kinds from income/expense categories:
  <https://docs.kde.org/stable_kf5/en/kmymoney/kmymoney/makingmostof.mapping.html>
- Moneydance distinguishes bank, credit card, investment, loan, asset, and
  liability accounts; investment accounts hold securities and cash:
  <https://infinitekind.zendesk.com/hc/en-us/articles/200975343-Types-of-accounts>
- hledger and Beancount keep account type declarations close to the five
  accounting classes, while using account hierarchy and commodities to express
  details such as brokerage accounts, cash, securities, and opening balances:
  <https://hledger.org/1.34/hledger.html> and
  <https://beancount.github.io/docs/getting_started_with_beancount/>

## Phase 1 Scope Boundary

Phase 1 should ship the accounting skeleton only:

- books
- commodities and currencies
- institutions
- accounts and system accounts
- account and institution management UI

Phase 1 must not create balances. Opening balances require posted transactions,
and transactions arrive in Phase 2. The `opening_balance` system account may be
seeded in Phase 1 so setup is complete, but there should be no opening-balance
endpoint or UI until the transaction/posting schema exists.

The implementation order should be:

1. Books.
2. Commodities and currencies.
3. Institutions.
4. Accounts and account tree.
5. System account seeding.

## Commodity Prerequisite

Design commodities before accounts. Accounts must not infer precision from a
free-text currency field.

`commodities` is the permanent identity row:

- `id`
- `book_id`
- `code`: stable internal code, unique per book
- `kind`: `currency`, `security`, `crypto`, `reward`, `commodity`
- `is_builtin`
- `created_at`
- `created_by_user_id`
- `created_request_id`

For currencies, `is_builtin` means the identity was created from the embedded
currency catalog rather than as a free-form custom commodity. It does not mean
the row was preloaded before user setup.

`commodity_versions` is the append-only state row:

- `id`
- `commodity_id`
- `version_seq`
- `effective_from`
- `recorded_at`
- `changed_by_user_id`
- `change_reason`
- `status`: `active`, `archived`
- `symbol`
- `display_symbol`
- `name`
- `standard_scale`
- `max_quantity_scale`
- `metadata_json`

Scale rules:

- `standard_scale` is the normal user-facing scale. For example, USD is 2.
- `max_quantity_scale` is the maximum scale accepted for stored quantities.
  It must be between 0 and 12 until a later precision decision changes that.
- Ledger postings will store `quantity_value`, `quantity_scale`, and
  `commodity_id`. They must never use floating point.
- An account version references `default_commodity_id`. Posting accounts must
  have one.
- An account version may also set `quantity_scale_override`. If it is null, data
  entry defaults to the commodity's `standard_scale`. If it is set, it controls
  account-level entry precision but still may not exceed the commodity's
  `max_quantity_scale`.
- Multi-currency support means multiple active currency commodities from day 1.
  It does not mean Phase 1 can post exchange-rate transactions; that arrives
  with transactions.

## Core Model

Use a stable identity table plus append-only versions.

`institutions` is the permanent identity row:

- `id`
- `book_id`
- `created_at`
- `created_by_user_id`
- `created_request_id`

`institution_versions` is the append-only state row:

- `id`
- `institution_id`
- `version_seq`
- `effective_from`
- `recorded_at`
- `changed_by_user_id`
- `change_reason`
- `status`: `active`, `archived`
- `name`
- `kind`
- `country_code`
- `website`
- `logo_url`
- `logo_small_url`
- `backdrop_url`
- `address_json`
- `comment_markdown`
- `metadata_json`

Logo and backdrop fields are optional URL/reference fields for UI
presentation. They do not introduce attachment storage or upload handling. A
small logo is intended for compact account and institution lists; the main logo
or backdrop can be used on institution detail/edit screens and account screens
grouped under that institution.

Do not store `activated_on` or `archived_on` separately. A lifecycle state is
effective on the version's `effective_from` date.

Multiple institution phone numbers, emails, and web links should not be jammed
into columns. Use a child table when this becomes UI scope:

`institution_contact_methods`:

- `id`
- `institution_id`
- `version_seq`
- `recorded_at`
- `status`: `active`, `archived`
- `kind`: `phone`, `email`, `website`, `other`
- `label`
- `value`
- `comment_markdown`


`accounts` is the permanent identity row:

- `id`
- `book_id`
- `is_system`
- `system_role`
- `created_at`
- `created_by_user_id`
- `created_request_id`

`is_system` and `system_role` belong to the account identity, not to account
versions. A system account cannot stop being a system account through a later
attribute version. Enforce one system account identity per `book_id` and
`system_role` with a unique partial index where `system_role IS NOT NULL`.
Treat `is_system` and `system_role` as immutable after insert.

`account_versions` is the append-only state row:

- `id`
- `account_id`
- `version_seq`
- `effective_from`
- `recorded_at`
- `changed_by_user_id`
- `change_reason`
- `status`: `active`, `closed`, `archived`
- `code`
- `name`: required for user accounts, nullable for system accounts
- `account_class`
- `account_kind`
- `parent_account_id`
- `institution_id`
- `country_code`
- `default_commodity_id`
- `quantity_scale_override`
- `allows_postings`
- `number_last4`
- `external_ref_hint`
- `comment_markdown`
- `metadata_json`

Do not store `activated_on`, `closed_on`, or `archived_on` separately. The
status transition date is the `effective_from` date of the version that changes
status.

Do not store `normal_balance`. It is deterministic:

- debit-normal: `asset`, `expense`
- credit-normal: `liability`, `equity`, `income`

Do not store `display_order` in account versions. Reordering a tree would create
unrelated append-only history across many accounts. Phase 1 should sort by
account class rank, parent, localized/user name, and id. If manual ordering is
needed later, add a separate ordering table or identity-level preference that is
explicitly not part of financial attribute history.

Do not physically store `valid_to` in either version table. Because rows are
append-only, `valid_to` should be derived for the canonical timeline.

Version ordering rules:

- `version_seq` is assigned by the application service inside the same database
  transaction as the insert.
- It is unique per identity row: `(account_id, version_seq)` or
  `(institution_id, version_seq)`.
- It is the authoritative tiebreaker; do not rely on timestamp precision.
- Multiple versions may share the same `effective_from`. This is allowed for
  corrections and reseeding.
- For a given identity and `effective_from`, the version with the greatest
  `version_seq` is canonical. Earlier same-date versions remain audit history
  but are superseded for timeline queries.
- For API history, derive `valid_to` by taking canonical versions only and using
  the next greater `effective_from`.

SQLite triggers should reject `UPDATE` and `DELETE` on version tables once this
is implemented. Corrections create a new version with a clear reason.

## Current-State Query Strategy

The backend should expose repository helpers or SQL views for current state.
Handlers should not hand-roll "latest version" joins.

Current account state for normal account management:

- Consider only versions with `effective_from <= today`.
- For each account, choose the row with the greatest `(effective_from,
  version_seq)`.
- Hide `archived` rows by default.
- Hide `closed` rows from transaction-entry pickers, but include them in account
  management when requested.

As-of account state for reports and future reconciliation:

- For each account, choose the canonical version with greatest
  `effective_from <= as_of_date`.
- If multiple versions share that date, choose greatest `version_seq`.

Current institution state:

- Same version rule as accounts.
- Institution list screens show empty active institutions so a newly created
  institution does not disappear before accounts are added.
- Account tree grouping hides institutions with no visible accounts.
- Archived institutions are hidden unless `include_archived=true`.

## Account Taxonomy

Separate accounting class from user-facing kind.

`account_class` is stable and drives reports, signs, and double-entry rules:

- `asset`
- `liability`
- `equity`
- `income`
- `expense`

`account_kind` is a localized/user-facing subtype. Initial supported kinds:

- Asset kinds: `cash`, `checking`, `savings`, `time_deposit`, `money_market`,
  `investment`, `brokerage_cash`, `security_holding`, `crypto_wallet`,
  `property`, `vehicle`, `collectible`, `points_miles`, `loan_receivable`,
  `other_asset`
- Liability kinds: `credit_card`, `line_of_credit`, `loan`, `mortgage`,
  `tax_liability`, `payable`, `other_liability`
- Equity kinds: `opening_balance`, `retained_earnings`, `current_earnings`,
  `trading`, `imbalance`, `equity`
- Income kinds: `salary`, `interest`, `dividend`, `realized_capital_gain`,
  `unrealized_capital_gain`, `reward_income`, `other_income`
- Expense kinds: `expense`, `fee`, `tax`, `interest_expense`,
  `investment_fee`, `other_expense`, `realized_losses`, `unrealized_losses`

This keeps the ledger orthodox while allowing personal-finance language. It also
avoids making every specialized account kind a top-level accounting type.

## Tree Rules

- Accounts form a per-book tree through `parent_account_id`.
- A current active account cannot be its own ancestor.
- A parent must belong to the same book.
- Root accounts should usually be class containers: Assets, Liabilities, Equity,
  Income, Expenses.
- Child accounts inherit `account_class` by default, but a version may override
  it only where explicitly allowed.
- Recommended first rule: active children should have the same `account_class`
  as the parent.
- Later exception: investment containers may hold asset children with different
  commodities, such as brokerage cash and security-holding accounts.
- `allows_postings=false` makes a container or placeholder account. Reports roll
  up child balances, but transaction entry cannot post directly to it.
- Active accounts cannot be parented under archived accounts.

This is stricter than free-form plain-text ledgers, but it gives a web UI safer
defaults and better validation.

Cycle enforcement strategy:

- Enforce cycle checks in the Go application service inside the same database
  transaction that inserts the new account version.
- The repository should use a recursive CTE over current account versions to
  collect ancestors of the proposed parent and reject the write if the target
  account appears in that ancestor set.
- Do not attempt a SQLite trigger for this in the first migration. Versioned
  current-state semantics make a trigger easy to get subtly wrong.
- Add backend tests for direct self-parenting, grandparent cycles, cross-book
  parent attempts, archived-parent attempts, and valid reparenting.

## Institutions

Institutions group accounts and supply defaults. They are not ledger accounts.

Institution kinds:

- `bank`
- `credit_union`
- `brokerage`
- `card_issuer`
- `lender`
- `insurance`
- `employer`
- `rewards_program`
- `government`
- `other`

Country behavior:

- `institution_versions.country_code` stores the institution's default country
  as an ISO 3166-1 alpha-2 code such as `US` or `NL`.
- `account_versions.country_code` stores the account country as the same kind
  of code.
- When creating an account with an institution and no explicit country, the
  service copies the institution's current country into the account version.
- A later institution country change does not silently change existing account
  versions. If an account country should change, create a new account version.

Sensitive identifiers:

- Store only recognition hints such as `number_last4`.
- Do not store full account numbers, card numbers, usernames, passwords, tokens,
  or bank-feed credentials in this slice.

## Account Commodity And Precision Rules

Every posting has a commodity. Account versions carry a default commodity for
entry defaults and validation.

Account commodity rules:

- Posting accounts that are restricted to one commodity must have
  `default_commodity_id`.
- Multi-commodity posting accounts such as income, expense, equity, brokerage
  containers, and system clearing accounts may leave `default_commodity_id`
  null. Their postings still carry explicit commodity ids and must balance per
  commodity unless an explicit FX transaction supplies conversion metadata.
- Container accounts may leave `default_commodity_id` null if their children use
  different commodities.
- `quantity_scale_override` is optional and account-specific.
- If `quantity_scale_override` is null, entry precision comes from the current
  commodity version's `standard_scale`.
- If `quantity_scale_override` is set, it must be between 0 and the commodity's
  `max_quantity_scale`.
- The posting schema still stores explicit `quantity_scale`; account precision
  is a default and validation aid, not a substitute for posting-level exactness.

Recommended account shapes:

- Checking account: asset/checking, default commodity USD or EUR,
  `allows_postings=true`.
- Credit card: liability/credit_card, default commodity matching the card.
- Brokerage: asset/investment, default commodity nullable,
  `allows_postings=false`.
- Brokerage cash: asset/brokerage_cash under brokerage, default commodity USD.
- Security holding: asset/security_holding under brokerage, default commodity
  set to the security commodity.
- Points or miles: asset/points_miles, default commodity set to a rewards
  commodity. Defer UI support until reward commodities and report-exclusion
  rules exist.
- Property or vehicle: asset/property or asset/vehicle, default commodity set to
  the relevant valuation commodity. Valuation changes should be explicit
  transactions or later price observations, not silent account edits.

Investment booking policy (`fifo`, `lifo`, `average`, `specific_id`) should be
deferred until investment transactions and lots are implemented. Do not put it
in the first account version unless the first investment slice needs it.

## Lifecycle

Lifecycle is represented by new versions, not row mutation.

`active`:

- Visible in normal account pickers.
- Can receive new postings if `allows_postings=true`.

`closed`:

- Historical account that should not receive new ordinary postings after
  the close version's `effective_from`.
- Still appears in history, reports, exports, and reconciliation history.
- May be shown in account lists behind a "show closed" control.

`archived`:

- Hidden from ordinary lists and pickers by default.
- Still available in history, exports, and audit views.
- Intended for accounts/institutions the user no longer wants in day-to-day UI.

Closing rules for the first implementation:

- System accounts cannot be closed or archived through normal UI.
- In Phase 1, closing does not inspect balances because transactions do not
  exist yet.
- Once transactions exist, a posting account can close only if its current
  balance is zero.
- A container account can close only if all active descendants are already
  closed or archived.
- Closing should fail if future scheduled transactions, import mappings, or
  unresolved import sessions still target the account once those features exist.
- Reopening creates a new version with `status=active`.

Archiving rules:

- An active account must be closed before archiving.
- Account tree grouping hides institutions with no visible accounts.
- Institution management still shows empty active institutions, so a user can
  create an institution before adding accounts.
- Explicitly archived institutions are hidden from default institution lists,
  even if old closed or archived accounts still refer to them.
- Active accounts should not point at archived institutions.

## System Accounts

System accounts are ordinary accounts with `is_system=true` and a stable
`system_role`. They are seeded per book and localized by stable role keys.

Required in Phase 1:

- `opening_balance`: equity/opening_balance, counterpart for explicit opening
  balance transactions.
- `imbalance_import`: equity/imbalance, temporary counterpart for imported
  single-sided entries that need cleanup.
- `retained_earnings`: equity/retained_earnings, future close-period target.
- `income_summary`: equity/current_earnings, future income close workflow.
- `expense_summary`: equity/current_earnings, future expense close workflow.

Recommended later roles:

- `currency_trading`: equity/trading, multi-currency conversion counterpart.
- `rounding`: expense/fee or equity/imbalance, explicit tiny rounding
  adjustments.
- `unrealized_gain_loss`: income/capital_gain, valuation reporting support.
- `realized_gain_loss`: income/capital_gain, investment sale support.

Rules:

- One system account identity per `book_id` and `system_role`.
- System accounts cannot be edited, closed, archived, or reparented through
  ordinary account management.
- System account names come from translation keys, not English-only database
  labels.

## Opening Balances

Account creation must not create money by setting a balance attribute.

Opening balances should be explicit posted transactions in Phase 2:

- Debit or credit the new account.
- Use the `opening_balance` system account as the counterpart.
- Use the user's chosen opening date.
- Store a clear source label/reason.

This follows the same durable ledger shape needed for imports, reconciliation,
and export.

Phase 1 implementation rule:

- Do not expose `POST /api/v1/accounts/{account_id}/opening-balance`.
- Do not put an opening-balance amount field on account create or edit forms.
- Seed the `opening_balance` system account role only as a future transaction
  counterpart.

## API Surface Proposal

Stable endpoints should be OpenAPI-first under `/api/v1`.

Commodities and currencies:

- `GET /api/v1/commodities` returns current commodities for the active book.
- `POST /api/v1/commodities` creates a custom non-currency commodity such as a
  security, crypto asset, reward point, or physical commodity.
- `GET /api/v1/commodities/{commodity_id}`
- `PATCH /api/v1/commodities/{commodity_id}` creates a new commodity version.
- `GET /api/v1/commodities/{commodity_id}/versions`
- `GET /api/v1/currencies` returns current currency commodities.
- `POST /api/v1/currencies` creates or activates a currency commodity.
- `POST /api/v1/currencies/{commodity_id}/default` sets the book default
  currency when book setup allows that.

Institutions:

- `GET /api/v1/institutions` returns current institutions without cursor
  pagination in Phase 1. It accepts `status`, `q`, and `include_archived` query
  parameters and enforces a hard server cap.
- `POST /api/v1/institutions`
- `GET /api/v1/institutions/{institution_id}`
- `PATCH /api/v1/institutions/{institution_id}`
- `POST /api/v1/institutions/{institution_id}/archive`
- `POST /api/v1/institutions/{institution_id}/restore`
- `GET /api/v1/institutions/{institution_id}/versions`

Accounts:

- `GET /api/v1/accounts` returns current accounts without cursor pagination in
  Phase 1. It accepts `status`, `account_class`, `q`, and `include_archived`
  query parameters and enforces a hard server cap.
- `GET /api/v1/accounts/tree` returns the current account tree, active and
  closed accounts by default, archived accounts only when requested.
- `POST /api/v1/accounts`
- `GET /api/v1/accounts/{account_id}`
- `PATCH /api/v1/accounts/{account_id}`
- `POST /api/v1/accounts/{account_id}/close`
- `POST /api/v1/accounts/{account_id}/reopen`
- `POST /api/v1/accounts/{account_id}/archive`
- `POST /api/v1/accounts/{account_id}/restore`
- `GET /api/v1/accounts/{account_id}/versions`
- `GET /api/v1/accounts/{account_id}/closing-validation`

Later, when transactions exist:

- `POST /api/v1/accounts/{account_id}/opening-balance`
- `GET /api/v1/accounts/{account_id}/register`
- `GET /api/v1/accounts/{account_id}/reconciliation-checkpoints`

Pagination rule:

- Accounts and institutions are expected to be small user-managed sets, so Phase
  1 should not add cursor pagination.
- To avoid endless lists, list endpoints should have a hard maximum such as
  2,000 rows and return a structured conflict or validation error if the result
  would exceed it.
- If real users hit that limit, promote these endpoints to cursor pagination
  using the repo's existing list conventions.

## Frontend Proposal

Before account management, the setup/settings UI needs a commodity and currency
slice so accounts can choose valid default commodities and precision.

First account UI slice:

- Institution list with active and archived sections.
- Institution create/edit/archive dialog.
- Account tree grouped by accounting class and institution.
- Account create/edit dialog with:
  - name
  - class
  - kind
  - parent
  - institution
  - country, defaulting from institution
  - default commodity
  - optional quantity scale override
  - number last four
  - posting/container toggle
- Account detail page showing attributes, lifecycle state, children, and version
  history.
- Close/archive flows with validation and confirmation.

States to design explicitly:

- loading
- empty book with no accounts yet
- no institutions yet
- archived-only institutions/accounts
- validation errors
- conflict errors for stale account tree or system-account edits
- success feedback after create/update/close/archive

All user-facing copy must go through Paraglide messages. Built-in account class,
kind, institution kind, and system-role labels use stable keys.

Localization strategy:

- API responses return stable codes for `account_class`, `account_kind`,
  `institution.kind`, `commodity.kind`, `system_role`, statuses, and validation
  error codes.
- The frontend owns visible labels for those codes through Paraglide message
  functions. Do not embed English fallback labels in API client helpers.
- User-entered names such as account names and institution names are data and are
  displayed as entered.
- System account names are not user-entered names. The UI derives their display
  labels from `system_role`; the database may store a nullable internal `name`
  only if future import/export needs it, not as English source text.
- Form field labels, empty states, validation presentation, and table headings
  live in frontend message catalogs.
- Categories later follow the same rule: built-in categories use stable keys;
  user-created categories use user-entered names.
- Backend-generated exports are a later Phase 3 concern. Until backend
  translation is introduced, export schemas should prefer stable codes and
  documented column names over localized prose.

## Differences From The FastAPI Archive

Carry forward:

- Account trees.
- Stable `book_id`.
- System account roles.
- Closing as lifecycle, not deletion.
- Account balancing/checkpoint concept for reconciliation later.
- `number_last4` recognition hint.
- Append-only account history idea.

Change:

- Add an explicit commodity identity/version model before accounts.
- Use `accounts` plus `account_versions` rather than a linked list of full
  account rows. This keeps permanent ids stable and makes current/history queries
  clearer.
- Put `is_system` and `system_role` on account identity rows, not versions.
- Do not store mutable institution attributes directly on `institutions`; use
  `institution_versions`.
- Do not hard-delete institutions or accounts.
- Split account type into `account_class` and `account_kind`.
- Do not store `normal_balance`; derive it from account class.
- Do not store `display_order` in version rows.
- Do not implement investment booking policy until investment lots exist.
- Use country codes directly unless a later feature needs a per-book country
  table.

## Open Review Questions

1. Should the first implementation enforce same-class parent/child relationships,
   or allow mixed-class trees for power users?
2. Should account `code` be user-visible and editable, or purely an optional
   import/export aid?
3. Do we want `specific_id` as the eventual investment booking policy name, or
   the archive's `strict` term?
4. Should commodity `kind` be immutable on the commodity identity row, as this
   draft proposes, or should it be versioned in case a user classifies a custom
   commodity incorrectly?

## Recommended First Slice

Implement after review:

1. Books setup if not already present.
2. Commodity and currency identity/version tables and APIs.
3. Institution identity/version tables and APIs.
4. Account identity/version tables and APIs.
5. System account seeding for the required roles.
6. Account tree UI and account/institution management UI.
7. Opening balances only when transaction posting exists in Phase 2.
