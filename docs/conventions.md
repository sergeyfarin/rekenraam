# Conventions

This document fixes cross-cutting product, engineering, and workflow conventions for the active Go and SvelteKit codebase.

## Source Of Truth

- Active requirements live in `docs/product-requirements.md`.
- Active architecture rules live in `docs/early-architecture-decisions.md`.
- Active sequencing lives in `docs/roadmap.md`.
- Archive review planning lives in `docs/archive/requirements-review.md`.
- `.archive/` is historical reference only.

When a feature introduces a durable new rule, update one of those documents in the same change.

## Delivery Model

- Build one feature slice at a time.
- Each slice must leave the app runnable.
- Avoid hidden partial migrations from `.archive/`.
- Prefer the smallest viable durable implementation over speculative abstractions.
- Keep single-user assumptions explicit until a later ADR changes them.

## Product Naming And Domain Model

- A book is the top-level accounting boundary.
- Accounts form a tree.
- Account classes are asset, liability, equity, income, and expense.
- Account kinds are catalog-backed behavior/UI profiles constrained by class;
  they are not categories, tax wrappers, budget-visibility wrappers, system
  roles, or investment lot concepts.
- Account management UI exposes a single user-facing "Account type" field
  based on `account_kind`; the accounting class is derived and kept out of
  ordinary account forms and filters.
- Once an account has posted ledger activity, structural fields are locked:
  account type/class, parent, institution, default commodity/currency,
  quantity scale override, posting eligibility, and opened date. Users should
  create a new account for a different structure and keep descriptive edits
  such as name, code, notes, and account-number hint editable.
- User-created equity/adjustment accounts are hidden from account management.
  Equity accounts are reserved for system workflows until a dedicated user
  workflow is designed.
- Child accounts inherit their parent account's institution. Institution
  assignment is editable only on root accounts; when an account has a parent,
  the backend derives the child institution from the parent.
- Account budget treatment is a separate planning/reporting axis from
  `account_kind`, because the same kind of account can be on-budget,
  off-budget, or excluded from budget views.
- Budget treatment is stored as its own effective-dated version history.
  Budget actuals resolve it on each journal entry date; changing treatment now
  must never reinterpret an earlier entry under today's setting.
- System accounts are identified by `system_role` and hidden from ordinary
  account lists by default.
- Hidden income and expense fallback accounts use `account_class=income` and
  `account_class=expense` respectively; they are not equity summary accounts.
- Categories are UI concepts mapped to income and expense accounts, not a separate ledger primitive.
- Category API behavior, built-in metadata, lifecycle rules, and the starter
  taxonomy are documented in `docs/design/categories-design.md`.
- Transaction ledger schema planning is documented in
  `docs/plans/transaction-ledger-core-plan.md`.
- Transactions contain postings. "Split" is acceptable user-facing copy for a
  split transaction UI, but schema, service, API, and docs should use
  "posting" as the canonical ledger term.
- Transfers are ordinary transactions.
- Reconciliation status belongs to posting or account-specific ledger state, not only a transaction header.
- Investment securities are commodities with `kind = 'security'`; crypto
  instruments use `kind = 'crypto'` and quantity scales through 24 (other
  instruments through 12). Kind and precision must agree with the commodity.
  The [quote/crypto contract](plans/quote-providers-crypto-plan.md) defines explicit
  provider identities, opt-in quote assignments and fixed-format CSV entry. Security
  identity, symbols, identifiers, provider links, and corporate-action metadata
  live in investment-specific tables rather than in account names or ad hoc
  transaction metadata.
- Security holding accounts are asset accounts with
  `account_kind = 'security_holding'`. Brokerage cash accounts remain ordinary
  currency-denominated asset accounts.
- Investment trades use the `commodity_trading` system account to keep posted
  transactions balanced by commodity. For an open position its balance is a raw
  clearing/cash-flow residual, not a method-independent realized gain; an explicit
  future gain posting is a policy-linked reclassification, never a side effect of
  viewing a report.
- Investment lots and lot events are durable accounting facts. FIFO is the
  default disposal method until a user-selected cost-basis policy says
  otherwise; never infer cost basis from current holdings alone. ADR 0012
  separates the canonical journal, investment subledger, read-side basis and
  valuation projections, and optional accounting postings.
- An investment write is one domain operation across the journal and investment
  subledger. Buy, sell, write-off, transfer, corporate-action, edit, correction,
  void, unvoid, soft-delete, and restore paths must commit, reverse, or correct
  both sides atomically. Generic transaction mutation must reject an investment
  transaction when it cannot preserve its subledger consequences.
- Investment domain writes are `posted` only until an investment-aware draft
  workflow defines atomic lot activation on promotion and no lot effects on
  discard. A caller-supplied draft must never create or consume lots.
- A committed disposal must snapshot its resolved cost-basis method, where that
  choice came from, the applicable policy/profile version, explicit allocations,
  and audit provenance. Later changes to account or global defaults must not
  reinterpret the historical election.
- Current lot balances and remaining basis are rebuildable projections over
  immutable acquisition, disposal, transfer, corporate-action, and basis-adjustment
  events. Every projection must conserve quantity and basis across sequential
  events. Alternative reporting methods run as separate read-side projections;
  they never repeatedly mutate the operational lots.
- A lot's original `cost_basis_value` is immutable acquisition evidence; average
  cost may redistribute only the remaining-basis projection. While the runtime has
  one operational projection, reject switching into or out of average cost after
  a partial disposal until the position closes and starts a new method epoch.
- An internal transfer allocates basis by the source position's method-family
  lock, else its resolved default: selected lots carry their own basis; an
  average-cost source moves a quantity through the same dated pool depletion as
  a sale (exact remainder on the last touched lot) and never carries a selected
  lot's own basis out of a pool. The transfer snapshots the applied method and
  its provenance, and any move locks the source family (T-123). An average-cost
  source opens one pooled destination lot by default, dated by the latest
  original date among the units moved; one lot per source lot is opt-in
  (`destination_lineage`, T-135). Moves into tax wrappers are a sale and a buy,
  not an internal transfer.
- An entry dated behind a later depletion of its position never reads the
  current projection. It is admitted only through full chronological replay at
  its same-day slot (after that day's earlier entries), with later decisions
  keeping their recorded method, provenance and elections, the shared writer's
  rolled-back preview, and gain acknowledgement; a path without that replay
  stays refused as out of order (T-95, T-117).
- Replay that changes a committed disposal's effective basis, proceeds, gain,
  knowledge, date, position or method must be disclosed, even when no reconciled
  balance changes (T-114). A command opts in by setting `GainImpactPolicy` on its
  first journal: the shared investment writer snapshots every effective disposal
  before guards and journals run, compares the effective set after its domain
  effects in the same SQLite transaction, and refuses a non-empty change set
  unless the caller echoes the preview's exact acknowledgement token. Removal and
  replacement are compared by correction root and decision sequence, never by
  preview-only IDs. Every existing replaying command is opted in (T-126 #141);
  a new replaying command must opt in as part of its own acceptance, preview
  through its actual rolled-back writer, and must not plan journals alone. An
  import that cannot be acknowledged in the batch keeps the row pending rather
  than skipping it.
- Effective investment reads select through the SQL views
  `effective_investment_operations`, `latest_investment_disposal_revisions`,
  `latest_investment_split_revisions`, `effective_investment_lot_events` and
  `effective_investment_transfer_links` rather than repeating successor or
  latest-revision predicates (T-124, T-135). Audit
  reads (self-check of original and superseded allocation sets, export) use the
  base tables, so an effective view never hides damaged history. A
  lot row is its opening operation's immutable opening fact. Every lot
  requires non-null `operation_id` and `source_transaction_id`; opening lots
  goes through the shared posted investment writer, including test fixtures
  (#148). Journal-free operations never open lots.
- Replay that crosses positions through internal transfers is scoped to the
  affected-position dependency closure, not a whole-book rebuild (ADR 0013
  refinement). A changed carried basis or corrected source lot appends a
  transfer link revision and replays each destination in the same command
  until nothing changes (T-132); units and destination lots never change. A
  `pooled_lot` revision may also change the destination's original date and
  the source depletions (T-135); a `source_lots` replay that would change its
  lots or dates is refused by name.
- Realized/unrealized gains are server-computed read models with named policy.
  Reproducible investment reports state their `as_of` date, price-knowledge
  cutoff, valuation/FX method, staleness policy, reporting currency, basis profile,
  period/date basis, and rounding. A report does not silently post gain, revaluation,
  or tax entries; a future accounting workflow may create explicit linked postings.
- A commodity's first version is effective from `db.CommodityGenesisDate`
  (`0001-01-01`), not its creation date. When you enabled a currency or added
  an instrument is app bookkeeping, not a financial fact, and posting rules
  resolve the commodity as of the entry date — so a creation-dated first
  version rejects all earlier history (T-42). Real later changes (archive,
  rename, scale) are new versions with real effective dates. Account
  `opened_on` is the opposite case: a genuine financial fact that legitimately
  rejects earlier postings.
- Market prices, FX rates, manual prices, provider prices, and trade-implied
  prices must use the generic commodity-pair pricing model: `price_series`
  identifies the source/quote-type/adjustment-basis stream, and
  `price_observations` stores exact integer-plus-scale observations.
- Price observations must distinguish the valuation date from system recording
  time, and should also preserve provider observation and publication
  timestamps when available. Historical valuation queries must be able to
  filter by both valuation date and recorded-at time.
- Derived FX observations may combine provider legs with different valuation,
  observation, or publication timestamps. Store the leg metadata in
  `derivation_json` and surface a user-visible warning anywhere derived FX
  rates are displayed.
- A derived observation's sources are also indexed in
  `price_observation_dependencies`, written in the transaction that inserts it
  (#104). The void cascade and the `price_dependency_integrity` self-check
  follow that index, never a scan of `derivation_json`; a derived observation
  whose legs name an observation that does not exist is refused. Links are
  immutable and always point at an older observation, so the graph is acyclic.
- Correct a price by superseding or voiding the observation, not by overwriting
  the old value.
- External market data is untrusted input by default. Provider dividends,
  distributions, and corporate actions become suggestions unless an explicit
  automation rule scopes the source/instrument/event family and permits
  auto-posting with required account mappings and audit attribution.
- Core features and provider automation have separate admission and delivery
  contracts ([ADR 0017](adrs/0017-provider-independent-features-and-incomplete-data.md)).
  Manual entry/correction and explicit file input must not depend on a provider
  connection, refresh or verified automatic mapping. Test core behavior locally.
- Missing optional attribution is an explicit supported partial state; preserve
  known facts, expose affected coverage and offer manual repair. Required effects
  still obey the named domain command, exactness, audit and reconciliation rules.
  Unknown is not zero; absent withholding does not prove a zero tax amount.
- Keep owner declarations separate from immutable provider evidence. Manual
  linking must not duplicate financial effects; later refresh must not overwrite
  owner corrections. A missing local mapping/default is an app review gap.
- Market-data provider secrets belong in operator configuration, not SQLite
  business tables.
- Market-data providers should be added in this order: built-in Go adapters for
  trusted/high-value sources first, declarative HTTP adapters for simple
  REST/CSV/JSON/XML mappings second, and external process plugins only after
  trust, lifecycle, and deployment boundaries are mature.
- Provider adapters must return normalized facts only. They must not directly
  mutate ledger, pricing, lot, dividend, or corporate-action tables.
- Provider downloads and other restart-sensitive background operations use the
  SQLite-backed at-least-once work queue defined by ADR 0010. Record follow-up work
  atomically with the originating domain change; never perform network access in
  that domain transaction.
- Background handlers must be idempotent, lease work for bounded periods, reclaim
  expired leases, and distinguish retryable failures from terminal failures.
- FX backfill covers every required provider publication date from the earliest
  active-account or posted transaction-entry date through today. Drafts do not
  enqueue work or extend planned coverage; promotion to posted does (ADR 0010).
  Unsaved entries (in-progress UI working copies with no database row) and import
  previews do not trigger coverage; import rows trigger only after posting.
  `max_backfill_days` is an execution chunk bound and must not silently truncate
  the required history. Missing-date traversal progress is distinct from rate
  coverage: durable per-sweep cursors advance past earlier observations, while
  failed dates retain independent bounded retries. Fresh demand must not
  coalesce against an already traversed scan prefix. See the
  [FX progress contract](plans/pricing-management-plan.md#historical-fx-progress-315).

## Financial Date Defaults

- Use the authenticated session's server-owned `owner_today` (`YYYY-MM-DD`)
  for financial entry defaults, report ranges/year and unchosen dated
  investment discovery. The settings service resolves it from the server clock
  and saved owner IANA zone; absent preferences follow the existing UTC default.
- Backend services bound and default owner-entered dates (account opened and
  closed dates, version effective dates, reconciliation cleared dates) by the
  same day through `SettingsService.OwnerTodayAt`, never
  `now.UTC().Format(time.DateOnly)` (#339).
- Blank transaction dates use the first journal-entry date if provided,
  otherwise the owner's day. Blank ledger `as_of` / category `before_date`
  and undated FX refresh source-assignment lookups use that same owner day
  (#343). Explicit dates retain their meaning, including historical FX
  assignment selection. Refresh scheduling does not choose the lookup date.
- Share the date through the app shell; missing/invalid authenticated date
  data is an error, never a browser-clock fallback. Session focus/minute refresh
  and preference saves update the shared date for newly opened forms.
- Capture editable defaults once; initializer effects must not track session
  date changes. Preserve selected dates, correction source dates and valid
  report URL ranges. Dated discovery follows an explicitly selected date;
  deliberately blank corporate-action/write-off dates remain deliberate.
- Pricing's composed workspace uses the shared date when its as-of field is
  blank and the explicit as-of value otherwise; the manual form captures that
  workspace date and corrections retain their source valuation date.

## Exact Quantity Precision

- Quantity coefficients are canonical signed decimal strings in SQLite and
  JSON, limited to 38 digits, and are calculated with arbitrary-precision
  integers. Never aggregate quantity coefficient text with SQLite `SUM`.
- Every coefficient column's CHECK enforces that canonical form (at most 38
  ASCII digits, optional leading `-`, no leading zero, no `-0`) over the whole
  stored value and states one legality rule: signed, signed nonzero,
  nonnegative or positive (#192). A new
  `*_value`/`*_scale` column copies the matching CHECK from the baseline and
  adds its rule to `TestExactNumberColumnChecks`, which fails until it does.
  An explicit `instr(value, char(0)) = 0` rejects embedded and trailing NULs that
  SQLite's text `length` and `GLOB` would otherwise ignore.
- The technical scale ceiling is 24 for crypto and 12 for every other
  commodity kind. A commodity's own `max_quantity_scale` may be lower.
- Standard/display scale is independent from maximum storage scale; do not
  render trailing places merely because a commodity permits them.
- **In OpenAPI a quantity coefficient is `type: string` with the shared
  `pattern: '^-?(0|[1-9][0-9]{0,37})$'`, never `type: integer`.** The
  generator turns `integer` into a TypeScript `number`, which silently
  reintroduces exactly the JavaScript precision loss the string form exists to
  prevent — and because `Coefficient.UnmarshalJSON` also accepts a bare JSON
  number, the backend keeps working and nothing fails loudly. Seven investment
  schemas drifted this way undetected until 2026-08-06. Investment money
  coefficients now also cross JSON as canonical decimal strings, even while
  their backend storage and arithmetic remain `int64`; their OpenAPI schemas
  must never describe them as `integer`.

## Data And Persistence Conventions

- Investment cost-basis projections currently use int64 coefficients. Acquisitions
  and disposals must reject a resulting open position whose exact basis cannot
  be represented at its recorded scale, inside the same transaction as all
  journal, lot and audit writes. Include future-dated lots when checking the
  current position; never truncate recorded residuals to satisfy this limit.
- Immutable lot openings and lot events carry explicit basis knowledge.
  Unknown opening/event basis has both coefficient and scale NULL; known zero
  stays numeric. `opening_basis_knowledge` describes original acquisition
  evidence independently of the current remaining projection. Replay preserves
  that distinction; it must not turn an unknown opening into known zero.
- Disposal decisions, allocations and their replay revisions carry the same
  explicit knowledge. One unknown allocation makes its decision or revision
  total unknown (NULL), while known allocation amounts stay on their rows and
  quantity and proceeds stay conserved. A revision's knowledge and amounts are
  read as one tuple and never filled from the original snapshot. Read models
  that cannot yet present an unresolved result refuse with
  `ErrUnknownInvestmentBasis`; they never read unknown as zero.
- Sales, write-offs, cash in lieu and in-kind transfers may consume unknown
  basis (`AdmitUnknownBasis`). Disposal basis and result are NULL when any
  allocation is unknown; independently known allocations remain numeric and
  quantity/proceeds remain exact. A currency's realized total is NULL while
  any entry in it is unresolved. An average pool holding unknown basis has no
  rate, so every disposal from it is unresolved.
- Return-of-capital entitlement and receipt allocation stay exact while an
  unknown effect has paired NULL reduction/excess amounts and leaves remaining
  basis unknown. Explicit entitlement to an independently known lot can remain
  known beside unrelated unknown lots. An established unknown average pool
  cannot supply a rate from a known subset. Sourced resolution appends revised
  calculations without changing original evidence; native correction and
  withdrawal preserve those knowledge transitions. See the
  [#169 contract](plans/investment-unresolved-non-sale-plan.md).
- Unknown inbound basis is stated, never inferred: `basis_knowledge: unknown`
  with no amount posts security legs only. A form or prefill must show unknown,
  never a known zero.
- A transfer link and its revisions carry knowledge as one tuple with their
  amounts. A revision may turn unknown to known (a resolution reaching it),
  never known to unknown. An outbound transfer with any unknown link posts no
  bridge; once all are known it posts the complete omitted bridge.
- Unknown inbound basis is resolved only by an appended, sourced resolution
  fact whose journal is the complete omitted bridge at the transfer date;
  the original evidence is never edited. A resolution is corrected only
  through its operation chain (replace or reverse), and a link has at most one
  effective resolution. A reversal never silently returns a known outbound or
  onward link to unknown: it refuses with that operation named (#168). A
  sourced known zero is journal-free, never a fabricated zero journal; any
  other journal-free operation kind must declare its own self-check exemption.
- A sourced split with no eligible long lots is a journal-free `split`
  operation (#146), with its date, ratio and audit retained. Replay reads its
  sourced fact even without an original lot event. A later entitled acquisition
  appends per-cost-currency effects and a dated `split_adjustment` journal under
  the triggering command's gain and checkpoint guards. The journal-link
  self-check exempts only sourced splits with no original lot effects; split
  journal/effect conservation still requires every nonzero effective delta to
  have posted journals. Both journal shapes are inspected and corrected by operation identity
  (#196), while transaction routes remain compatible. Inverse and replacement
  headers share one command audit; each posts only a nonzero delta. Cash in
  lieu and import dedupe bind the settled/source operation, and a missing primary
  transaction is NULL, never a fabricated zero ID.
- Projected investment basis has explicit `known`/`unknown` knowledge. Unknown
  coefficient and scale are NULL in storage and on the API, and empty in CSV;
  known zero remains numeric. Numeric read-model fields are usable only when
  knowledge is known. An unknown lot makes position basis/gain unavailable,
  while quantity and independently priced market value remain available.
  Known-basis algorithms must refuse unresolved inputs before arithmetic;
  self-check continues quantity verification and names unresolved basis.
- Money and countable commodities are not the same kind of quantity. A currency
  balance may be negative — an overdraft and a credit-card balance are real
  positions. A negative non-currency balance may be an out-of-order entry, an
  error, or an intentional short. Only units owed by open named short lots on
  that date classify it as a short (#173); any further negative quantity is
  unclassified, and dated self-checks and net-worth reports must flag it
  (T-105/T-106). The `commodity_trading` clearing
  account is exempt because it holds the other half of commodity movements.
- Never store money or quantities as floating point.
- Store exact values as a canonical integer coefficient plus scale and
  commodity identifier. Quantity coefficients use decimal strings at storage
  and API boundaries as defined by ADR 0009.
- Ledger posting quantities use debit-positive sign convention. Positive
  postings are debits; negative postings are credits. Asset and expense
  increases are usually positive. Liability, equity, and income increases are
  usually negative.
- Use calendar dates for financial facts.
- Use UTC timestamps for system facts.
- Store recurring user-facing schedules as local wall-clock time plus an IANA
  time zone, then record each actual run/attempt as UTC timestamps. Storing only
  a UTC hour for a daily local schedule causes daylight-saving-time drift.
- Calendar dates travel over the wire as ISO 8601 strings (`YYYY-MM-DD`). UTC timestamps travel as RFC 3339 strings (`2025-01-15T14:30:00Z`). Go's `time.Time` marshals as RFC 3339 by default; use a plain `string` or a validated date type for calendar-date fields.
- Keep stable codes in data; translated labels belong in localization assets.
- Built-in records not entered by users or imported from external sources must use stable keys or codes, not English display names as the only source of truth.
- Built-in labels are resolved at render time by the frontend localization boundary. Do not store localized built-in names as canonical database values or require them in setup/API requests.
- Seeded categories, account classes, account kinds, currencies, commodities, system accounts, and other app-defined labels must be localization-ready.
- The setup currency step creates only the book default currency. Currencies are
  active for everyday account UI when at least one active account uses them;
  closed or historical currencies remain durable commodities for history,
  reports, and exports.
- Preserve `book_id` in core schema even while runtime stays single-book.
- Use state transitions, voiding, archival, soft-delete, or corrective entries instead of hard-deleting business records.
- Physical (hard) delete is allowed only for never-posted drafts. Posted financial records must use void, soft-delete, archive, or corrective-entry workflows even when unreconciled.
- Keep transaction lifecycle separate from reconciliation state. The transaction lifecycle is `draft`, `posted`, and `voided`; whether a transaction affects the ledger is determined by that lifecycle plus the soft-delete flag. `uncleared`, `cleared`, and `reconciled` describe account-posting verification and are an independent axis.
- "Unsaved entry" (working copy) is **not** a transaction status. It is in-progress UI entry that has no `transactions` or `transaction_versions` row yet: half-typed forms and pre-save editor state held only in the browser. An unsaved entry triggers no background work — no FX/commodity-rate coverage, no base-currency conversion, no validation side effects — because nothing is persisted. It becomes a `draft` or `posted` transaction only when the user (or the import commit step) saves it. Do not call persisted `draft` rows "unsaved"; do not call an unsaved working copy a "draft."
- `draft` is a reserved persisted lifecycle status, distinct from an unsaved entry, and is **system-only, not a user-facing maturity step**. A draft is a real `transaction_versions` row excluded from the posted ledger and reports. Manual transaction entry goes directly to `posted`: there is no user-facing "save as draft" action and users do not pick `draft`. R9 scheduled generation produces drafts with a dedicated recurring review/discard surface. Other machine/async producers such as import commit awaiting review or explicit crash-recovery autosave must own equivalent review/discard surfaces before activation. The general transaction table excludes drafts. A future owner-facing Unfinished Work inbox may link to each producing workflow without turning draft into a maturity tier. Drafts do **not** enqueue or extend background FX/commodity-rate coverage (ADR 0010, amended at R9 acceptance): the coverage trigger (`fx_work_after_posting_version_insert`) fires on `posted` versions only, so a promoted draft's foreign-currency dates get coverage at promotion. The refresh planner also excludes drafts. A future producer needing converted review amounts must explicitly revisit ADR 0010 before extending coverage; such preparation would still never make a draft part of ledger balances or reports.
- `posted` means the transaction is entered and participates in the ledger and reports. "Entered" covers manual entry (the default and only outcome of manual entry), bank/file import (after commit), and anything already existing in the ledger; posted does not imply reconciled. Posted transactions remain directly editable.
- Recurring generation persists the draft, occurrence identity, and audit atomically. Discarding a generated never-posted draft keeps an audited skipped occurrence tombstone, so retry cannot recreate it. Creation and edits that stay draft never invalidate reconciliation; posting remains guarded. Browser-origin creation rejects `status="draft"`; producer scheduling/public run-now must stay disabled until the producer's review/discard surface exists. R9 satisfied this gate in slice 5 on 2026-08-31.
- The user-facing maturity line is **reconciled vs not reconciled**, not draft vs posted. Before a posting is locked by a reconciliation checkpoint it is freely editable and removable; after, changes are guarded (see the reconciliation rule below). Do not present draft-vs-posted to users as the thing that gates editability.
- Two distinct removal workflows apply to posted transactions, and they are not interchangeable:
  - **Void** keeps the transaction *visible in the UI, marked as voided*, as an intentional reference/memory record (for example: an entry that never appeared on the bank statement, kept while the user investigates). A void appends a `voided` transaction version and excludes the transaction from the posted ledger. A voided transaction can be unvoided and edited.
  - **Soft-delete** removes the transaction *from the transactions table and all ordinary views* — it looks deleted to the user — while keeping the row in the database for audit/history and recovery. Use it when the user knows the entry was a mistake. Soft-delete is the nullable `transactions.deleted_at` flag, separate from `voided`; soft-delete and restore actions append actor, audit event, timestamp, and reason to `transaction_deletion_events`.
- Same-day order has two independent axes. `transaction_day_sequence` orders UI transactions within `transaction_date` in the global table. `account_day_sequence` orders a posting within its `(account_id, entry_date)` register; a transfer can therefore occupy different positions in its two account registers. Both default to the end of their scope, and every other write keeps them (a reconciliation status change included). They are hidden in ordinary forms. Users change them in a screen's reorder mode (#185), off by default, which moves within a day only; in an account register an arrow at a day's edge instead offers, after confirmation, to move that account's posting to the adjacent calendar day (`POST /accounts/{id}/postings/{line}/move-day`, #187), an edit of its posting date rather than of an order, which lands at the end of the new day. A move places the transaction beside its visible same-day neighbour **everywhere both appear**: in the journal order when they share a transaction date, and in every account register holding both on that entry date (`POST /transactions/{id}/place`). A register where that relation already holds keeps its order. An account register's **Only in this account** switch moves that register alone (`POST /accounts/{id}/postings/{line}/move`), for a statement that lists the day differently; a category, having no statement, offers no switch. Any account-order change that crosses an active same-day checkpoint is guarded per account. `line_seq` remains split-line order inside one journal entry and must not be reused as register order. A posting line (`posting_lines`) is one posting in each transaction version — a write naming one line key twice is rejected and the schema enforces it (#224) — so the account register identifies a row by its posting line, never by its transaction (#221).
- Posting dates (#187). A transaction's `transaction_date` is its display date; each journal entry's `entry_date` is the day its postings count, in balances, reports and reconciliation. A writer states that an account posting posts on another day with `posted_on` on that posting, in a single-entry request, and the transaction service builds the dated entries (`groupPostingsByPostedOn`): only a user asset or liability has a posting day; with one such account every other posting follows its day (a card purchase posted later is one entry on the posting day); with several, other postings stay on the entry date and each dated entry is balanced per commodity through `transfer_clearing`, after the whole transaction is checked to balance so the bridge can never absorb an imbalance. Readers that present a journal as one editable entry drop the bridge of a multi-entry journal (`editablePostings`) and resend each account posting's day; never send the bridge back. A re-date by `move-day` refuses a journal the rule would not rebuild exactly as stored.
- Reconciliation is account- and commodity-scoped. Finished reconciliation checkpoints are lock floors for reporting and register trust, represented by `(statement_date, statement_account_sequence)` for that account. The guiding principle is: **if an operation changes a reconciled balance, it must be guarded** (explicit override plus invalidation of the affected active checkpoint and all later active checkpoints for that account/commodity), never silent. Two situations change a reconciled balance and are therefore guarded:
  - **Reconciled posting facts change.** Changing a reconciled posting's account, commodity, amount, scale, or entry date.
  - **A posting enters or leaves a reconciled period.** A posting is inside the period when its `entry_date` is before the checkpoint date, or when its date is equal and its `account_day_sequence` is at or before `statement_account_sequence`. Creating, editing, voiding, unvoiding, soft-deleting, restoring, or reordering across that boundary changes the period balance and is guarded even when the posting is not itself marked `reconciled`.
  A refused guarded write is previewed by an endpoint built from that write's own state checks and candidates, never by another operation's preview: edits use `POST /transactions/{id}/reconciliation-impact`, draft promotion `GET /transactions/{id}/post/reconciliation-impact`, and void, unvoid, soft delete and restore `GET /transactions/{id}/{action}/reconciliation-impact` (#220). The preview lists the complete invalidation set and writes nothing; the override write rechecks the guard itself. The override is bound to what the owner reviewed (#296): a browser write that sets `reconciliation_override` must name the previewed `reconciliation_override_checkpoint_ids`, and the write transaction refuses with 409 `RECONCILIATION_IMPACT_STALE`, invalidating nothing, when the set it would invalidate now differs (a later statement finished, or one was voided or invalidated). A reorder has no preview: its guard refusal names the set in the error's `checkpoint_ids`, and its override names that set. Every guard refusal of a generic ledger write carries `checkpoint_ids`. The write is also bound to the version the owner reviewed: `PATCH /transactions/{id}` requires `expected_version_id`, and post, void, unvoid, soft delete, restore and the update preview accept it; a different current version is refused with 409 `TRANSACTION_VERSION_STALE`, and the editor reloads the newer version rather than resending the stale form. A read model a lifecycle write is confirmed from carries the version it shows (recurring due rows: `transaction_version_id`), and the write sends it. The same binding covers every investment write and correction, provider completion, Trading 212 source correction and imported-dividend review (#304): their requests embed the shared override fields, the API refuses an override without its set at decode (400), the investment writer's one combined guard compares the set, and their refusals carry `checkpoint_ids`; the forms re-preview and show the current set on `RECONCILIATION_IMPACT_STALE`, as on a stale gain acknowledgement. Internal callers and previews pass an unbound override (`CheckpointOverride` with a nil set). The import batch commit binds per row (#305): `preview-commit` returns `reconciliation_checkpoints`, the union of the checkpoints its pending rows would invalidate, shown beside the override; the commit names those ids, and because each row writes in its own transaction (`db.WithBatchReviewedCheckpoints`) a row's set must be contained in the reviewed set rather than equal to it. A row reaching any other checkpoint is held pending with nothing invalidated and listed in `reconciliation_stale_row_ids`, like gain-review and reconciliation holds. No browser override is unbound.
  Reordering within the same side of a checkpoint boundary does not change the checkpoint balance and is allowed without reconciliation override. `transaction_day_sequence` is presentation-only and never affects reconciliation. Edits that touch only category, description, payee, note, tags, or other non-financial fields never change a balance and are always allowed, keeping reconciliation intact regardless of date. Operations on transactions that are already out of the ledger do not change any balance and are not guarded: restoring a voided-and-soft-deleted transaction only restores visibility of an out-of-ledger record, so it is never reconciliation-guarded.
  An investment command that only appends journals — a correction's inverse and replacement, plus any split adjustment its replay posts — is guarded once on its **combined** delta (T-120): for each account and commodity, the command's postings are summed up to each active checkpoint's own boundary, and the first boundary whose sum is non-zero is invalidated with every later checkpoint. Equal amounts on different sides of a boundary, including a same-day sequence boundary, do not cancel. A checkpoint the command leaves unchanged stays active; its new offsetting postings are uncleared and are offered to the next reconciliation session, where they net to zero whether both or neither are selected. Preview and commit use the same calculation. This never covers edits of existing postings, which keep the per-posting rules above.
- Re-reconciliation (#240): a posting's `reconciled` status records that a statement once verified it; it is not the lock. A posting is excluded from new reconciliation sessions, and its status cannot be changed by hand, only while a **covering** checkpoint links its posting line — an active checkpoint, or any checkpoint on an active checkpoint's `previous_checkpoint_id` chain, whatever that one's own status (a later active checkpoint chained from a voided one still counts its postings in its statement balance). After an invalidation or a void, the latest remaining active checkpoint is the starting balance and every posting it does not cover is offered again; once-reconciled ones start selected and are marked for re-verification. A status is a claim about the facts a statement showed (#297): a write that changes a kept posting line's account, commodity, amount, quantity scale or entry (posting) date resets that line to `uncleared` with no `cleared_on`, so after such an edit it is offered unselected like a new posting; metadata-only edits (description, memo, payee, tags) keep the status and `cleared_on`, and so does every line the write leaves unchanged (a category move resets only the category line). Checkpoint links stay on the posting line as evidence either way. Finishing appends ordinary reconciliation-status versions and a new checkpoint; invalidated and voided checkpoints keep their posting links and reasons as evidence, and no balancing transaction is created. A session finishes only while its starting checkpoint is still the account's latest active one (`RECONCILIATION_START_STALE` otherwise: void it and start again). Voiding a checkpoint is the recovery path for a wrong statement: void it (from the reconcile screen's "Reconciled through" panel, #248, with a required reason), then reconcile the period again.
- Use the hybrid audit model for durable domain changes: domain version/lifecycle tables explain what changed, while `audit_events` rows explain who, when, how, and why the operation was initiated.
- One `audit_events` row should represent one user/system operation, not one changed column. Multi-row workflows such as setup, imports, reconciliation, and transaction correction should create one event inside the same database transaction and reference it from every created version/lifecycle row.
- Keep essential attribution columns such as `created_at`, `recorded_at`, `created_by_user_id`, `changed_by_user_id`, `created_request_id`, and `change_reason` on domain rows as denormalized snapshots for straightforward history and export queries. The referenced audit event is the canonical home for request, session, origin, operation, and grouped-workflow provenance.
- `audit_events.auth_session_id` and `authentication_events.auth_session_id` are nullable plain integers, not foreign keys, so session cleanup never blocks on them or clears them (#289). `auth_sessions` uses `AUTOINCREMENT`, so a cleaned-up session's id is never handed to a later session and a kept event cannot name the wrong one. Request IDs and actor IDs remain the stable attribution floor.
- `audit_events.operation` must be non-empty and follow stable dotted-code naming such as `account.create` or `transaction.correct`, but the database must not use a full enum that requires a migration for every new operation.
- Soft-deletion uses a nullable `deleted_at` timestamp for current state and append-only deletion/restoration events for auditable history. Do not represent soft-delete as a lifecycle status. The transaction implementation is the reference pattern: it hides a transaction from ordinary views while keeping the row durable for audit and recovery.
- Schema changes require explicit migrations under `backend/migrations`.
- The migration runner is **`pressly/goose`** with embedded SQL files. Migrations are sequential numbered `.sql` files under `backend/migrations/`. Run at startup; goose tracks applied versions in the database.
- ADR 0014 selects **`github.com/mattn/go-sqlite3`** with bundled native SQLite
  through CGO, delivered in #195. Production and tests use one driver with
  `fts5,netgo,osusergo` via shared commands; do not link system `libsqlite3`. See
  [the migration plan](plans/native-sqlite-driver-migration-plan.md).
- SQLite connections must install PRAGMAs on every physical connection through
  supported driver DSN options or a connection hook: `busy_timeout(5000)`,
  `foreign_keys(1)`, `journal_mode(WAL)`, `synchronous(FULL)`, and
  `wal_autocheckpoint(1000)`. Initial and replacement connections must obey
  the same policy; setting a PRAGMA once on a pool is insufficient.
- ADR 0015 requires FULL consistently for runtime, migrations, tests, backup,
  inspection and stopped-database maintenance. The registered driver's hook
  also installs FULL on direct opens and replacement connections. Read-only
  connections share the setting, though reads gain no additional durability.
- Startup must validate effective SQLite PRAGMA state before serving requests.
- Early runtime uses one application `*sql.DB` pool with `SetMaxOpenConns(1)`; revisit only when real workload pressure justifies more concurrency.
- Database access uses repository-style methods over raw `database/sql`. Introduce **`sqlc`** only when the query surface is large enough to justify code generation, and document the generation command before generated files land.

## Project Lifecycle And Migration Immutability

The earlier release-candidate freeze of `0001_initial_schema.sql` preceded any
installed or used v0.1 database. ADR 0013 permits an explicitly declared
pre-release redesign of that baseline. Such a change updates the checksum and
release fixture, carries `BREAKING DEV DATABASE` in the commit body, and tells
developers to reset disposable local databases. Prefer a forward migration
when it produces an equally clear model with less disruption. Once a release
has actual installations, its migration files are immutable.

- **Installed release migrations are append-only.** Never edit, consolidate,
  split, renumber, or delete one. Every later schema change uses the next
  sequential file.
- **Every installed release supports forward upgrade from every earlier
  installed release.** The migration suite preserves sentinel data while
  migrating old baselines to `HEAD` and compares the result with a fresh
  `HEAD` schema. Add a release migration's checksum to the freeze test when
  its release is cut.
- **Upgrade, do not roll back schema.** Startup applies pending migrations before
  serving. Take and verify a backup before replacing a binary. If the upgrade
  fails, restore that backup and the previous binary; do not rely on Goose down
  migrations against production data. Operator steps live in `docs/upgrades.md`.
- **Migration numbers are renumbered before merge, not after.** Work lands
  directly on `main` (see *Branches And PRs* in `docs/developer-workflow.md`),
  so a collision now mostly means unpushed local work: pull first, and if your
  `00NN_` is taken, renumber **your own** file to the next free number before
  pushing. The same holds for the occasional branch — the one merged second
  renumbers itself. Never merge two files sharing a number, and never renumber a
  installed-release migration already on `main`.
- **Migration numbers do not encode product releases.** They are a single
  monotonic database ordering sequence. After `v0.1.0`, each schema change takes
  the next integer regardless of which release eventually contains it; release
  notes record the highest migration included in a release. This avoids
  renumbering an immutable migration when work moves between releases.

## API Conventions

- Real endpoints live under `/api/v1`.
- DTOs stay near HTTP handlers.
- Transaction-returning services enrich posting display fields through the
  shared batch owner before DTO conversion, including inverse/replacement and
  reconciliation batches. Use one account and one commodity lookup per batch.
  Internal import receipts that consume only durable IDs do not fetch optional
  response display data. A response lookup failure after commit returns the
  sanitized internal error without replaying the write; refresh canonical reads
  before deciding whether to submit another mutation (#370).
- Business rules belong in application services.
- Database access belongs behind repository-style functions or methods.
- JSON request consumers use the shared strict decoder with a 1 MiB
  (1,048,576-byte) body cap, including trailing whitespace; applicable operations
  declare HTTP 413 / `REQUEST_TOO_LARGE` through the shared OpenAPI response.
  Optional bodies use the optional decoder. Bodyless operations have no implied
  JSON cap. Multipart imports retain their separate 50 MiB file / 51 MiB request
  limits and `IMPORT_UPLOAD_TOO_LARGE`; `/imports` documents both branches.
- Error responses should be structured, consistent, and safe for user display.
- Error response shape is a JSON envelope: `{"error": {"code": "STABLE_CODE", "message": "human-readable detail"}}`. The `code` field is a stable uppercase string that the frontend translation layer keys off of. Never return raw Go error strings to the client.
- Initial API error codes are `VALIDATION_FAILED`, `UNAUTHENTICATED`, `FORBIDDEN`, `NOT_FOUND`, `CONFLICT`, `CSRF_INVALID`, `RATE_LIMITED`, `RESOURCE_BUSY`, `LEDGER_OVERFLOW`, `SETUP_REQUIRED`, `SETUP_ALREADY_COMPLETE`, and `INTERNAL_ERROR`.
- Public request and response shapes should be documented in OpenAPI as endpoints become real.
- The checked contract is OpenAPI 3.1. Express null with a type union such as
  `type: [string, 'null']`, not the obsolete `nullable` annotation. Requiredness
  and nullability are separate: an optional property may still reject null.
- Stable `/api/v1` endpoints are OpenAPI-first. `api/openapi/openapi.yaml` is the checked source of truth, handler changes and OpenAPI changes land together, and frontend API types must be generated from the checked OpenAPI artifact once frontend API client code lands.
- Page-level and reusable-component read models should be backend-composed when a screen would otherwise issue repeated per-row/per-card requests. Prefer one request per page, or at most one request per shareable component, with the backend preparing the JSON shape needed by that surface.
- Date fields in request and response bodies use ISO 8601 (`YYYY-MM-DD`) for calendar dates and RFC 3339 for timestamps, consistent with the data layer convention.
- List endpoints that can return large result sets must use **cursor-based pagination**, not offset pagination. Cursors must be stable under concurrent inserts. Return `next_cursor` in the response; a missing or null `next_cursor` means the last page.
- Transaction list endpoints must support a `search` query parameter backed by **SQLite FTS5** on the backend. Do not implement client-side full-text search over server-fetched pages.
- API code must treat `SQLITE_BUSY` as possible even with `busy_timeout`. If the timeout is exhausted, return a retryable server error instead of hiding or partially applying the operation.

## Authentication And Session Security

- Personal-access tokens follow [ADR 0018](adrs/0018-personal-access-token-boundary.md)
  and the [bounded contract](plans/personal-access-tokens-plan.md) (#334).
  Authorize exact method/registered-route pairs, never prefixes or wildcard
  grants. Supplied Authorization always selects token authentication without
  cookie fallback. The first grants are read-only `ledger:read` and
  `reports:read`; token administration is session-only. Creation shares MFA's
  password-confirmation budget and rechecks session/hash in its transaction.
  Sample token/management-session expiry after acquiring the pool connection
  and SQLite write admission; deferred `BeginTx` alone does not eliminate
  writer-lock waits (#359).
  Recovery revokes tokens atomically with sessions. GET/HEAD token-use events
  and last-use timestamps are the explicit ADR 0018 security-bookkeeping
  exception to durable read purity. They never change financial data or grants.

- Username validation requires: non-empty after trimming, at most 64 bytes, no control characters (Unicode < U+0020 or U+007F). These constraints apply at both owner creation and login. OpenAPI schemas for `CreateOwnerRequest` and `LoginRequest` must document the same limits.
- Any feature that changes the owner password must revoke all existing sessions
  and personal-access tokens atomically in the same database transaction. The
  `recover-owner` command demonstrates the required pattern.
- Expiry admission (#360): browser sessions and approved-device throttle selection
  check persisted deadlines against a fresh clock after their reads. These reads
  remain read-only and usable alongside a WAL writer. An already admitted request
  may finish after session expiry/logout, and an admitted device attempt retains
  its selected throttle scopes while it finishes. MFA challenge consumption and
  TOTP use/activation check deadlines after acquiring SQLite write admission;
  deferred `BeginTx` alone is insufficient. Challenge expiry is exclusive at
  consumption; a consumed challenge may finish after expiry. The matched TOTP
  step must still lie within the configured skew window at its consuming write.
  Throttle reservation and failure decisions sample their clocks after writer
  admission too. Each scope reservation pins its persisted random generation
  (#366). An exhausted in-flight window or expired block starts a fresh
  generation; deletion/recreation never reuses an identity, even at the same
  timestamp. Refunds, failures and successful resets affect only the admitted
  generation. A superseded attempt may finish, retaining its normal verdict
  and event, but never debits, refunds, resets or recreates a replacement or
  missing row. A late failure cannot restart an expired block. Counts and
  deadlines survive restart and migration 0020. The bound is on admissions
  within a generation: old guesses may finish after rollover, and intentional
  successful resets admit a fresh budget. It is not a rolling bound on verdict
  timestamps across generations.
- Revocation cutoff (#267/#365): MFA disable deletes challenges, and the final
  session insertion checks that its consumed challenge still exists with an active
  enrollment. Disable therefore cuts off attempts whose session has not committed,
  even after factor/challenge consumption. `recover-owner` holds the server's
  database lock and refuses while the server runs (#286); recovery deletes the
  enrollment/challenges and atomically revokes sessions and PATs. Regenerating
  recovery codes invalidates the old unspent set; a factor already consumed before
  regeneration remains proved. Logout/device revocation does not cancel a request
  or device attempt already admitted, and revoked devices are never renewed.
  Enabling MFA does not revoke sessions; a password-only sign-in that read the
  inactive state just before activation may finish.
- A sign-in writes its session, throttle settlement and device approval in one transaction; a rejected attempt leaves no session (#287). The authentication-event row is written afterwards and best-effort, so a failed log write never turns into a lockout.
- Account-security changes are attributed authentication events (#288): MFA enrollment start, activation, disable, recovery-code regeneration, approved-device revocation and `recover-owner` each append one `authentication_events` row naming the user, the session, the proxy-aware client IP and the request id (the CLI has none of the last three and records `origin = cli_recovery`). They live there rather than in `audit_events`, which is book-scoped ledger provenance with no client address, so one timeline shows the sign-in that created a session next to the changes made from it. Each is written in the same transaction as the change: unlike a sign-in, a change that cannot be recorded is simply retried, and recovery's event shares the reset's database and commit, so it adds no lockout. A sign-in records the second factor that completed it (`totp` or `recovery_code`). Events never carry passwords, tokens, MFA secrets or recovery codes; non-secret context goes in `metadata_json`. Retention prunes sign-in attempts, logouts and token-use/refusal events after 90 days; account-security lifecycle events are kept. A new security-relevant transition adds an event type here, not a log line.
- Login throttle scopes use `username` and `client_ip` as independent scope keys. The admitted `username` generation is cleared on successful login; the `client_ip` throttle is intentionally retained (a success only gives back its own place). Every throttle scope, including approved-device and `reauth_session`, admits each attempt atomically before any verifier runs and counts attempts still in flight against the budget, so parallel requests cannot admit more guesses than that generation permits (#285/#366). Never check the counter and record the failure as separate statements. A failure keeps its place; a success or an attempt with no verdict (password correct but MFA owed, internal error) gives it back; a place never settled by a crashed process expires with the window. When multi-user support is added, confirm that throttle scope keys remain globally unique and cannot be confused across user classes.
- MFA-settings reauthentication (enroll, activate, disable, regenerate recovery codes) and personal-access-token creation spend a `reauth_session` throttle scope keyed by the authenticated session id: five wrong passwords or activation codes, shared across these operations, block that session for fifteen minutes (#262/#334). The place is reserved before any verifier runs, so correct input is refused while blocked and nothing is mutated. Success clears the admitted session generation. The scope is never owner- or IP-wide, so a stolen session cannot lock the owner out of their settings; the owner signs in again for a fresh session. It never spends or clears the sign-in scopes.
- `429 Too Many Requests` responses from the login endpoint must include a `Retry-After` header with the remaining wait in seconds. The blocked-until time is known at the point the error is returned; derive `Retry-After` from it rather than from a fixed constant.
- Login-created session lifetime is configurable with `SESSION_LIFETIME_HOURS` (default `720`, must be a positive integer number of hours). Password and completed-MFA login cookies must use the same lifetime as the persisted session. Successful approved-device reuse renews its browser cookie as well as its database expiry, without creating a new device identity. Owner setup sessions use the product default unless setup is deliberately made configurable too.
- Expired and revoked `auth_sessions` rows are deleted at server startup and then once every 24 hours where `revoked_at IS NOT NULL OR expires_at <= now`. The index `auth_sessions_expires_revoked_idx` exists to support this.
- The `script-src 'unsafe-inline'` directive in the Content-Security-Policy header exists because SvelteKit `adapter-static` injects an inline bootstrapping script into `index.html` at build time. The correct fix is to compute a SHA-256 hash of that script during the frontend build and embed it in the Go middleware's CSP. This is deferred; when implemented, the build script must output the hash and the Go source must consume it at build time. Do not remove `'unsafe-inline'` until that mechanism is in place.
- `style-src` stays `'self'` with no `'unsafe-inline'` and no `'unsafe-hashes'`. Because `style-src` is the fallback for `style-src-attr`, any *static* `style="..."` attribute in Svelte markup is refused when the browser parses the compiled template, and reports a violation on every load. Style such elements from CSS instead: put the rules in `frontend/src/app.css` or a component `<style>` block. Dynamic `style={...}` bindings are unaffected — Svelte applies those through the CSSOM, which CSP does not police. SvelteKit's own `#svelte-announcer` is the one static case the app does not own; the `rekenraam:svelte-announcer-csp` Vite plugin (`frontend/vite/svelte-announcer-csp.js`) strips its inline attribute at build time so the directive can stay strict.
- `Strict-Transport-Security` (HSTS) is emitted only when the request is served over HTTPS (direct TLS or trusted proxy signalling `X-Forwarded-Proto: https`). Do not emit HSTS on plain HTTP responses.

## Frontend Conventions

- SvelteKit builds static output with `@sveltejs/adapter-static`; production does not run a SvelteKit server.
- Do not add `+page.server.ts`, `+layout.server.ts`, `+server.ts`, SvelteKit form actions, or production server hooks without first accepting an ADR that changes the runtime shape.
- Frontend routes may rely on the Go static handler's SPA fallback for extensionless browser paths. Missing asset-like paths with file extensions should 404.
- All production data access goes through Go API endpoints under `/api/v1`; unknown `/api/` paths must not fall back to the frontend shell.
- The frontend foundation stack uses **SvelteKit**, **Tailwind CSS**, **Bits UI**, and **shadcn-svelte**. Bits UI provides accessible primitives; shadcn-svelte is a component scaffolding source, not the product design system.
- The frontend i18n library is **Paraglide JS** (Inlang). Use its compile-time message functions for all user-facing copy.
- Inlang plugins in `frontend/project.inlang/settings.json` resolve from local `./node_modules/...` paths, not `https://cdn.jsdelivr.net/...`. CDN modules are invisible to the lockfile and to pnpm's supply-chain check, and they make `paraglide:compile` — and therefore every build — fail on a network-restricted machine. Add the plugin as a `devDependency` and reference its `dist` entry by path.
- Add the Paraglide/Inlang message catalog structure before the first non-placeholder user-facing screen.
- Backend translation (export file headers, server-generated content) is deferred to Phase 3. When needed, use `nicksnyder/go-i18n` with JSON message files.
- All user-facing copy goes through a translation boundary.
- English is the initial implementation language.
- Message catalogs live in `frontend/messages/{app,settings,budgets,pricing}/{locale}.json`. All
  directories feed the single `m` namespace, so a key must be unique across
  them; which file a key sits in is a grouping decision, not a namespace.
- Shipping locales are `en, es, fr, nl, de, ru`. Locale resolution is
  `localStorage → browser preference → English`, configured identically in
  `vite.config.ts` and the `paraglide:compile` script — change both together or
  a build silently ships English only.
- **Terminology is decided in `docs/localization-glossary.md` before strings are
  written.** Do not translate this app term-by-term: half its vocabulary is
  accounting vocabulary with an established word per language, and half is
  consumer-banking vocabulary with a different established word. Where a term
  exists in GnuCash or a localized MS Money/Quicken, that term wins over a more
  literal one. Add a row to the glossary before introducing a new domain term.
- A missing translation falls back to English per message rather than rendering
  blank. Configured shipping catalogs must match the base locale in keys and
  placeholders, contain nonempty text, and have unique IDs across domains.
  `catalogs:check` enforces this before frontend dev/check/build and in CI.
  Key parity is separate from native terminology review; the language settings
  page retains the draft notice until review is recorded on #102.
- UI code and built-in app-defined data must stay ready for additional languages without route, component, or schema rewrites.
- Do not concatenate translated fragments to form sentences.
- Formatting of numbers, dates, percentages, and money must be locale-aware and separate from message translation.
- All frontend money **parsing and exact arithmetic** goes through
  `frontend/src/lib/money/amount.ts` — one module, string/BigInt arithmetic over the
  ledger's `{ value, scale }` pair, never a JS `Number`. Do not add a second parser or
  a private copy of a helper to a `.svelte` file; this is the frontend counterpart of
  the backend's `exact.ScaledInt` rule above, and every copy that existed had drifted.
- Editable inputs use `parseDecimalAmount` and `formatEditableAmount` from that
  module, defaulting to Paraglide’s active locale. Machine-readable decimal
  output (including CSV) uses `formatLedgerAmount`; never feed that unlocalized
  string directly into a localized editable field (#100).
- Every `Intl` formatter, collator and display-names instance takes Paraglide's
  `getLocale()`, never `undefined`: the browser locale may differ from the app
  language the user picked (#252). Browser languages may still seed a *guess*,
  such as the setup screen's suggested currency.
- All frontend money **display** goes through `frontend/src/lib/money/format.ts`
  (`formatQuantity`) — locale-aware presentation via `Intl.NumberFormat` over a
  `BigInt`, for read-only money only. **No money-formatting dependency**: G-08 was
  settled against Dinero.js on 2026-08-08 and the package was removed. Dinero
  formats through `Intl` anyway, its default calculator is JS numbers, and it models
  a fixed exponent per currency, which is the wrong shape for a ledger that stores a
  24-scale crypto quantity beside a 2-scale euro. The reasoning is in `format.ts`.
- Investment gains summaries display monetary values at the cost currency's
  `standard_scale`, labelled with its currency code. Round to the nearest display
  unit, ties away from zero, only after exact backend aggregation. This is not an
  allocation or posting rule: stored amounts, fractional share quantities, and
  detailed lot values retain their precision. Use `formatMoney` for these summary
  amounts and `formatQuantity` for exact quantities.
- A displayed rate implied by a price/quantity pair (an FX rate) is divided
  exactly and rounded **half-up** (ties away from zero) at 8 decimals, the same
  rule as the backend's `scaledDivision`, then trailing zeros are dropped. Use
  `formatRatio`; never truncate or route the quotient through `Number` (#106).
- The two halves of `#lib/money` are not interchangeable, and a report or export must
  pick the right one rather than growing inline math: `format.ts` for anything a user
  only reads, `amount.ts` (`formatLedgerAmount`) for anything that must round-trip
  back into an editable input unchanged. `format.test.ts` pins the difference.
- Backend money arithmetic uses **`shopspring/decimal`**. All canonical balance and report calculations happen in Go, not in the browser.
- Report calculations must be backend-composed read models with explicit filter
  contracts. Reuse the same report semantics across full report pages,
  account-detail summaries, category/payee drill-downs, and future
  multi-currency/investment views instead of reimplementing route-local
  financial calculations in the frontend.
- Use Tailwind for implementation ergonomics, layout composition, and utility authoring, but keep semantic design tokens in CSS custom properties as the source of truth for theme roles. Tailwind utilities do not replace the token system or the app's product-specific visual language.
- Build app screens from shared shell and surface primitives (`PageHeader`, `Panel`, `StatePanel`, status badges, toolbar/list-row patterns) before adding route-local visual styling. Route files may compose layout, but token roles and reusable financial-app chrome should stay centralized.
- Themes must use semantic design tokens for color, spacing, typography, elevation, and motion.
- Themes start with light and dark only.
- Theme names and token roles must stay stable even if visual styling evolves.
- Define semantic theme tokens as CSS custom properties before the first non-placeholder user-facing screen. Persist theme preference only after a real user-facing theme control exists; until then, respect system preference where practical.
- Appearance color preferences may be stored as local browser preferences while
  runtime scope remains single-user and no durable user-settings API exists.
  Keep these preferences expressed as semantic token variants rather than
  route-local color values; move them behind a backend settings model only when
  broader user preferences are introduced.
- New screens must define loading, empty, error, and success states.
- New interactions must be keyboard-usable and accessible.
- Every modal dialog is `#lib/components/modal-dialog.svelte` (#327): mount it to open, unmount it to close. It is a native `<dialog>` opened with `showModal()`, so the page behind is inert; it wraps Tab inside, routes Escape to `onCancel` unless `pending` (or a form inside reports `aria-busy`), and returns focus to the opener. Destructive confirmations keep initial focus off the destructive button (put Cancel first, or mark a field `data-initial-focus`). Do not hand-roll `fixed inset-0` overlays with `aria-modal`; `e2e/playwright/dialog-focus.spec.ts` and `expectFocusTrapped` in `support/a11y.ts` check the behaviour.
- A side panel that covers the page only below `lg` (`fixed inset-0 … lg:relative`) attaches `phoneModal` from `#lib/components/modal-focus.ts` (#342), which applies the same contract below `lg` (inert page, initial focus, Tab wrap, Escape unless pending, focus back to the opener) and nothing from `lg` up. A keyboard handler that opens such a panel on Enter or Space calls `preventDefault()`. `e2e/playwright/panel-focus.spec.ts` checks both widths.
- A new route gets a row in `e2e/playwright/route-sweep.spec.ts`, and a new dialog, form, panel or review state reached by an action gets a row in `e2e/playwright/subview-sweep.spec.ts` (#331). Together they run axe and the sideways-scroll checks in both themes, at desktop and phone width and in long-copy locales.
- Responsive behavior must be deliberate on both desktop and mobile.
- Prefer shared frontend helpers and API seams over route-local ad hoc logic.
- Use **`openapi-typescript`** (type generation) + **`openapi-fetch`** (typed HTTP client) for all frontend API calls. The OpenAPI spec is the single source of type truth.
- Use **`@tanstack/svelte-query`** as the data layer for all server state. Use `createInfiniteQuery` for paginated lists; use query key composition to cache search results per search string.
- Dense ledger tables are built on **`@tanstack/svelte-table` v9** (Svelte 5 runes native — no v8 compatibility wrapper). Register features explicitly through the shared `transactionTableFeatures` in `frontend/src/lib/transactions/transaction-table-types.ts`; sorting, filtering, and pagination stay server-side, so those features are deliberately not registered. Responsive column hiding stays CSS-driven (column `meta.priority`) rather than `columnVisibilityFeature`, because the static adapter prerenders without `matchMedia`. Always supply a `rowId` so rows keep identity across refetches. Add TanStack Virtual only when a screen actually needs virtualized rendering.
- Use **`minisearch`** for client-side fuzzy filtering of small in-memory sets (account name dropdowns, payee autocomplete). Do not use client-side search for full transaction lists.
- Use **`date-fns` v4** for frontend date manipulation (parsing, arithmetic, formatting helpers). Use `Intl.DateTimeFormat` for final locale-aware display output. Do not use `luxon`, `moment`, or the browser `Date` constructor for financial date logic.
- All new frontend files must be **TypeScript** (`.ts`, `.svelte` with `<script lang="ts">`). No JavaScript-only files in `frontend/src`.
- Component state uses **Svelte 5 runes** (`$state`, `$derived`, `$effect`, `$props`). Cross-component and cross-route shared state uses `$state` in `.svelte.ts` module files. No Svelte 4 stores (`writable`, `readable`, `derived` from `svelte/store`) in new code.
- Frontend modules import shared code through the `#lib/*` subpath import (declared in `frontend/package.json` `imports`) with an explicit file extension: `#lib/money/amount.ts`, `#lib/components/panel.svelte`, `#lib/paraglide/messages.js`; the generated declaration-only `schema.d.ts` is imported as `#lib/api/schema.js`. Route state comes from `$app/state` (`page.url`, `page.params`), whose URL types are read-only — helpers that read query parameters take `ReadonlyURLSearchParams` and copy before writing. SvelteKit and adapter options live in the `sveltekit({...})` call in `frontend/vite.config.ts`; SvelteKit 3 refuses a `svelte.config.js`.
- URL-backed selections updated through shallow navigation (`replaceState`)
  read `page.shallow?.url ?? page.url`: SvelteKit 3 keeps the shallow URL
  separate from the last full navigation. Replace the current history entry
  when editing these selections, preserve unrelated query parameters, and
  verify reload, browser Back and shared links through the real screen (#326).
  Transaction lists share `transaction-url-filters.ts`; impossible calendar
  dates are dropped using `isCalendarDate`, rather than checking shape alone
  (#377). A route's account/category/instrument identity remains authoritative.

## Design Conventions

- Aim for calm, trustworthy financial software rather than generic dashboard styling.
- Use clear information hierarchy, explicit totals, and obvious destructive-action warnings.
- Preserve readability for dense ledger and reporting screens.
- Charts and color usage must remain understandable in all supported themes.
- **Charts summarize an accessible table; they never replace it.** Render a chart
  only when it can be honest: a magnitude chart over unlike commodities is a
  fabricated comparison, so it appears only once a single commodity is in range.
  A single-series chart uses one semantic hue (`--color-accent`) and needs no
  legend — the heading names the series. Categorical hues are assigned in a fixed
  order, never cycled, and never a rainbow. Any state a chart encodes with colour
  (a negative total, a warning) also carries a non-colour cue such as texture
  plus explanatory text. Chart geometry may use floating point; displayed amounts
  never do — they come from the exact coefficient strings.
- Prefer inline SVG built from the token system over a charting dependency until
  a screen genuinely needs interaction a hand-rolled chart cannot provide.
- Theme support should include a non-color cue strategy for critical financial states such as positive, negative, warning, reconciled, and locked.

## Security And Deployment Conventions

- The codebase is destined for public open-source release (AGPL-3.0, per `docs/product-requirements.md`). Treat the repository and its full git history as public: never commit secrets, real credentials, personal data, or real financial records — including in test fixtures, docs examples, and `.archive/`. Test values must be obviously fake.
- Security must not depend on source secrecy. Any mechanism that would break if an attacker read the code is wrong by design.
- Treat local-network deployment as safer than public deployment, but never as fully trusted.
- Local authentication must exist before real data entry.
- First-run setup is browser-based and guided. The first implementation creates the single owner with a username and password; later setup steps add the default book, default currency preference, system accounts, default categories, and optional additional categories as those domains are implemented.
- First-run setup uses `GET /api/v1/setup/status` and `POST /api/v1/setup/owner` for the first slice. Setup progress is persisted as named steps, not as a single boolean.
- Production owner creation requires a one-time `SETUP_TOKEN` in `X-Setup-Token`. Generate and log a cryptographically random token at boot only when the operator has not configured one; an operator-configured token must be at least 32 characters. The browser must not persist it.
- Early password recovery has no unauthenticated browser or email reset flow. Use an operator-controlled local recovery path requiring server or database access, and invalidate existing sessions after password reset.
- Session management uses **HTTP-only secure cookies** backed by a **SQLite session table**. Sessions are revocable by deleting the row. Do not use JWTs for session tokens.
- Session tokens are opaque random values. Store only token hashes in SQLite.
- Session cookies use `HttpOnly`, `SameSite=Strict`, `Path=/`, and no `Domain`. Use `Secure` whenever the app is served over HTTPS. HTTPS deployments should use a `__Host-` prefixed cookie name once auth implementation lands.
- CSRF protection for mutating API requests uses SameSite cookies as a baseline plus server-side `Origin` validation and a session-bound or signed CSRF token sent in a custom header such as `X-CSRF-Token`.
- `GET`, `HEAD`, and `OPTIONS` endpoints must not mutate durable application
  state, except ADR 0018’s token authentication-event and last-use bookkeeping.
- Password hashing uses **Argon2id** via `golang.org/x/crypto/argon2`. Store hashes in a self-describing format, such as PHC string format, with algorithm and parameters. Start from the OWASP minimum profile of 19 MiB memory, 2 iterations, and parallelism 1, then tune if local hardware requires it. Never store plaintext or reversibly encrypted passwords.
- Owner passwords must be at least 12 Unicode characters and at most 1024 bytes. Apply the same policy to owner setup and local owner recovery; login may accept existing shorter passwords only if such hashes were created by an older build.
- `REKENRAAM_SECRET_KEY` encrypts reusable online provider credentials **and the MFA shared secret** at rest. It must be base64 for exactly 32 random bytes, must live in operator configuration or a deployment secret manager, must never be committed, and must be backed up with the SQLite database.
- Losing `REKENRAAM_SECRET_KEY` makes stored online provider credentials and any MFA enrolment unreadable but does not invalidate imported ledger data. A login can still be completed with a recovery code, which is hashed rather than sealed — the MFA verification path deliberately falls through to recovery codes when the secret cannot be opened, because that is the situation those codes exist for. Until an in-place rotation command exists, the documented recovery and intentional-rotation path is backup-first deletion and re-creation of affected online import connections with fresh provider credentials.
- Mutating browser API routes must reject non-JSON request bodies unless a feature explicitly documents another content type. Mutating browser API routes must validate same-origin requests before business logic runs; authenticated mutations must also validate the session-bound CSRF token.
- Public deployment requires HTTPS and the reverse-proxy/operator guidance in `docs/deployment-security.md`; the app HTTP listener must not be exposed directly to the internet.
- MFA (TOTP + single-use recovery codes) shipped 2026-08-07. Public deployment with real financial data requires the owner account to be **enrolled**, not merely that the feature exists. Second-factor failures spend the same login-throttle budget as password failures, a code cannot be replayed within its own step, and enrolling, disabling, or regenerating recovery codes each re-confirm the password so a stolen session cannot change what protects the account.
- MFA enrollment writes must not demote an active enrollment. Activation pins the exact pending secret it verified inside the same transaction as recovery-code creation; an enrollment write that finds active state, or activation whose pending secret changed, refuses without altering the current enrollment or codes (#261).
- Localhost development may use HTTP.
- LAN/private-network deployments should use HTTPS through a reverse proxy; the app does not terminate TLS or accept certificate/key configuration. Browser-warning-free LAN/private HTTPS requires either a real trusted certificate for a domain or installing a trusted local certificate authority on every client device.
- Local-network use may run without MFA enrolled if authentication and operator guidance are clear.
- SQLite database encryption is deferred for early local use, but docs must explain when encrypted-at-rest storage may be needed.
- SQLite database, WAL, SHM, and generated backup files must be mode `0600`; keep their containing data directory private to the service account.
- Docker Compose must package the same app shape as the single binary.
- The Docker production runtime image uses the official Debian 13 slim base: **`debian:trixie-slim`**. The app runs as a non-root numeric user.
- SQLite data must live in persistent storage outside the container image or binary.
- Backup and restore instructions are part of product documentation, not only operator folklore.
- Database restore holds the server's process lock throughout backup verification,
  preservation and replacement, and releases it on every outcome. Keep the
  lock file in place across releases and handoffs; its existence or recorded PID
  is diagnostic metadata, not proof of a current owner. Never unlink a live
  deployment's lock file to bypass contention (#227).
- Operator backups do not replace user-facing export features.
- Live SQLite backups must use SQLite-aware flows: the online backup API as the preferred in-app approach, or `VACUUM INTO` when a compact operator-triggered copy is acceptable. Do not recommend raw live file copy as the normal backup path.

## Observability Conventions

- Use **`log/slog`** (Go stdlib) for all structured logging. Output JSON in production, text in development. No third-party logging library.
- Log at `Info` for normal request lifecycle events, `Warn` for recoverable anomalies, `Error` for failures that need operator attention.
- Do not log financial record content (amounts, payees, account names) at any level.
- The application mode is controlled by the **`APP_ENV`** environment variable. Accepted values: `development`, `production`. Default to `production` when unset. Mode gates log format (text vs JSON) and any dev-only middleware.
- A development-only override that could redirect a stored credential — such as `TRADING212_BASE_URL` or `MARKET_DATA_BASE_URL` for the e2e provider stub — is a startup **error** outside `APP_ENV=development`, never silently ignored, and the runtime re-checks it for configs not built by `config.Load`.
- HTTP middleware layers run in a consistent order per request: request ID injection → panic recovery when added → request logging → auth check → CSRF validation for mutating authenticated requests → handler. Each layer is a plain `http.Handler` wrapper; no framework-specific middleware interface.
- A server-generated request-scoped UUID is generated for every inbound request, included in all log entries for that request, and echoed in the `X-Request-ID` response header. Incoming `X-Request-ID` may be logged as an external/caller request ID, but it does not replace the server-generated ID.
- The HTTP server must handle `SIGTERM` and `SIGINT` with `http.Server.Shutdown(ctx)` and a short grace period before exiting.

## Infrastructure And Port Conventions

- Backend dev server listens on **`:16888`** (via `HTTP_ADDR` env var).
- Frontend dev server (SvelteKit + Vite) listens on **`:1888`** and proxies `/api` to `http://localhost:16888`.
- Production single binary and Docker container serve on **`:16888`**.
- Production static frontend files are copied from `frontend/build/` into `backend/internal/web/dist/` before compiling the Go binary.
- Entity IDs in SQLite use **auto-increment integers** (`INTEGER PRIMARY KEY`). External-facing identifiers in API responses may use integers directly; do not convert to UUIDs unless a later ADR introduces distributed ID requirements.

## Scope Conventions

- User-facing export support must include core ledger CSV and QIF in the first export milestone.
- Text-file imports must normalize content to UTF-8 before parsing. Unicode BOMs
  and valid UTF-8 are deterministic; legacy encoding detection must use a
  confidence floor and expose an explicit user override rather than silently
  accepting an uncertain locale. Persist the encoding decision in import source
  metadata so a preview remains explainable.
- Attachments are out of scope until explicitly brought in by a later requirement or ADR.
- Mobile support must cover full core workflows responsively, including transaction entry.
- User and permission naming may stay explicitly single-user until shared workflows become active scope.

## Testing Conventions

- Backend behavior gets Go tests.
- Backend integration fixtures use `backend/internal/testdb.Open`: it migrates
  one template per Go test process and copies that closed database into each
  test's temporary directory. Tests still receive isolated files opened through
  `db.Open`, including its production PRAGMA and permission checks. Tests whose
  subject is migration, restore, or fresh-database startup behavior must keep
  constructing and migrating their own database instead of using the template.
- Construct SQLite file URIs from filesystem paths with `db.SQLiteFileURL`
  (`SQLiteFileURL` inside the db package), then append any connection query
  options. This also applies to fresh migration, recovery, restore and read-only
  test connections; raw `file:` concatenation misreads temporary path punctuation.
- ADR 0016 sets the shared native backend gate to two concurrent packages and
  a five-minute deadline per package in both race and coverage modes. Normal
  commands use available Go CPU resources; do not impose the historical
  four-core/GOMAXPROCS limit. CI's integrated build runs independently of
  other checks, with all validation jobs retained.
- Every top-level test in `internal/app` and `internal/db` calls `t.Parallel()`
  first; serial tests still use only one core within their package (T-125).
  A test that genuinely needs process-wide state
  (`t.Setenv`, working directory, a package variable) instead carries a
  `// serial: <reason>` comment directly above it.
  `TestIntegrationSuitesRunTestsInParallel` enforces this. Never share a live
  database, file or worker between parallel tests.
- Use **`testify`** (`testify/assert` for non-fatal checks, `testify/require` for fatal checks) in all Go tests. Do not write verbose `if got != want` assertion blocks.
- Frontend logic gets Svelte checks and focused component or unit tests when introduced.
- Bruno covers important API workflows.
- Playwright covers critical user journeys.
- Playwright specs must never hard-code a ledger date. Accounts created during a run get `effective_from` set to the run date, and `PostingAccountRule` resolves the account version as-of the posting's entry date, so a fixed posting date starts failing with a `VALIDATION_FAILED` date rejection (`posting date is before account opened date`, or `posting account has no version effective on …`) the moment wall-clock time passes it. Derive dates from `todayISO()` / `todayQIF()` / `monthStartISO()` in `e2e/playwright/support/dates.ts`.
- Financial invariants, reconciliation behavior, imports, and calculations require named backend test cases.
- Financial calculation and report suites must include independent expected
  amounts as well as conservation identities. A balanced journal does not prove
  gross report classification, cost-basis selection, or report serialization is
  correct. Exercise equivalent
  decimal representations, mixed commodities and account scopes, and both gross
  and net amounts; use exact test oracles without floating point.
- Journal/subledger sequence tests must compare both against the entered facts
  after each operation, and inspect durable state after previews and rejected
  writes. Keep deterministic seeds reproducible. For financial regressions,
  verify the named test fails when the relevant defect is restored in an isolated
  checkout; a compile failure does not count as detecting the defect.

## Archive Translation Rules

- Do not port desktop-only features without re-justifying them for web and self-hosted deployment.
- Do not copy Python, Rust, or Tauri implementation details directly into Go or SvelteKit without confirming the underlying requirement still matters.
- When archive documents conflict with active docs, active docs win.
- If an archive idea is adopted, rewrite it in current-stack terms and add it to active docs.

## Work Tracking

- GitHub issue numbers (`#nn`) are the canonical IDs for active work. Do not
  allocate new `T-nn`, `G-nn` or `S-nn` codes. Existing codes remain historical
  aliases in `docs/backlog.md`; never renumber them or rewrite dated reviews,
  accepted ADR headings or old code references just to match GitHub numbers.
  `Rnn` codes identify roadmap initiatives, not a second issue-number system.
- The roadmap owns execution order and current focus. GitHub priority labels
  express urgency, not sequence. Record actual prerequisites and the reason
  they are required on the issue; a later roadmap slot alone is not a blocker.
  A `blocked` issue names the missing prerequisite or evidence and what would
  unblock it. Recheck active blockers when the initiative or evidence changes.
- A provider evidence/availability blocker applies only to that adapter's
  automation. Separate it from core/manual feature acceptance and unrelated
  providers. Do not label later roadmap placement as a technical blocker.
  Keep app-owned partial-data disclosure and repair defects actionable without
  an upstream prerequisite; never describe a still-missing workflow as shipped.
- Future initiative issues may stay brief. Before implementation starts, the
  issue must state the problem/evidence, bounded delivery scope, measurable
  acceptance, dependencies and relevant validation. Use the work-item template;
  link the governing plan/ADR rather than copying its design contract.
- GitHub issues own delivery checklists. Feature plans and ADRs own durable
  behavior and scope decisions, including why a follow-up is retained or
  deferred. A shipped/deferred design disposition is not a second live task
  checklist; link to the issue that owns the remaining work.
- Update tracking when work starts, scope changes, a blocker appears or is
  resolved, and delivery completes. Progress goes on the relevant issue;
  roadmap and capability changes are recorded in the same delivery change.
  Do not periodically copy the overall queue into another issue or report.
- Close a completed issue with a short record of what shipped, commit links,
  validation (including any pending CI), retained limits and linked follow-ups.
  Bounded umbrellas close when their agreed delivery is met. A new independent
  gap gets its own issue; an unmet original acceptance item keeps its issue
  open. Retired or duplicate work is closed with that reason and its successor
  linked, rather than described as implemented.

## Documentation Conventions

- Requirements belong in `docs/product-requirements.md`.
- Long-lived architectural constraints belong in `docs/early-architecture-decisions.md`.
- Active sequencing and current focus belong only in `docs/roadmap.md`;
  durable phase boundaries belong in `docs/product-requirements.md`. Plans,
  issues and other docs link to the roadmap rather than restating order.
- Shipped scope belongs in `docs/implemented.md`; live actionable defects and
  debt belong in GitHub Issues. `docs/backlog.md` maps historical aliases to
  issues but does not duplicate status or priority. Completed plans and dated reviews are
  historical references, not competing trackers.
- Navigation belongs in `docs/README.md` and the repository README. Retired
  index issues stay closed as historical references; progress updates belong
  on the relevant feature issue. Update the current-state documents in the
  same delivery or sequencing change rather than maintaining another overall
  queue in issue comments.
- When roadmap priorities change, cross-check `docs/competitor-comparison.md` and
  record intentional parity gaps or gains in the roadmap's parity section.
- **A durable scope/behavior acceptance decision is recorded in the governing
  plan, in the same commit that flips the status elsewhere.** `roadmap.md` and
  `implemented.md` say *that* an initiative was accepted; the feature plan is
  where each retained follow-up gets its yes/no and its reason, linked to its
  GitHub delivery checklist, because the plan is what the next initiative is
  checked against. R2 shipped with the status
  flipped in three trackers and no decision recorded in `reports-plan.md`, which
  left the three saying "closed" and the plan saying "open" for four days.
- Decision records belong in `docs/adrs/`.
- Repo-wide agent guidance belongs in `AGENTS.md`.
- Developer workflow notes belong in `README.md` and `docs/developer-workflow.md` unless a tooling area needs substantial local guidance.
- Placeholder README files are acceptable for ignored or generated directories that must remain present in Git.
