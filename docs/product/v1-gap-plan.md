# V1 Gap Analysis & Fix Plan

Last updated: 2026-05-17

Audits the repo against [v1-scope.md](v1-scope.md) and the milestone-1-11 completion claims in [SELF_HOSTED_MIGRATION_PLAN.md](../../SELF_HOSTED_MIGRATION_PLAN.md). Scope-coverage gaps are listed first; test-depth gaps follow. Each item names what is missing, where it lives, and severity. Items marked **DONE** have shipped; the relevant section retains the original gap description for historical context.

For the day-to-day "what's next" view, see [TODO.md](../../TODO.md).

> **Decision recorded 2026-05-12:** v1 ships as a **personal-finance web app**
> per the existing `v1-scope.md`. Small-business accounting + full investment
> bookkeeping is **v2** scope; the architectural roadmap lives in
> [docs/architecture/accounting-foundations.md](../architecture/accounting-foundations.md)
> and is not a v1 gate. Within v1, we still take the cheap audit/correctness
> wins that align with v2 direction: see "v1 accounting-correctness fixes
> (2026-05-12)" below for what shipped in that pass.

> **Decision recorded 2026-05-17:** v1 ships and gates on the
> **single-container SQLite runtime**. PostgreSQL remains in the repository as
> a post-v1 compatibility target, but CI coverage must not shrink: repository,
> API e2e, migration, and browser e2e tests now run against SQLite by default.
> Explicit Postgres make targets remain for later hardening.

Severity legend:
- **B** — release blocker (gate per `v1-scope.md` "Release Gate" or scope `Must Have`)
- **H** — hardening (correctness or operational risk; should ship at v1)
- **N** — nice-to-have (scope `Should Have`, can defer if time-boxed)

## Test status snapshot (2026-05-17, SQLite-first pivot)

Backend in [.github/workflows/api-tests.yml](../../.github/workflows/api-tests.yml):

- CI sets `REPOSITORY_DB_BACKENDS=sqlite` and `API_E2E_DB_BACKEND=sqlite`.
  `make api-test-coverage` runs the full API suite on SQLite; Postgres skips
  are no longer treated as the default confidence signal.
- Pyright (strict): clean.
- Ruff check: clean.
- Ruff format check: enforced and clean.

Postgres compatibility is explicit via `make api-test-postgres`,
`make api-test-postgres-coverage`, and `make api-migrate-smoke-postgres`.

## Phase status

- [x] **Phase 0** — end-to-end test seam (completed 2026-05-09)
- [ ] **Phase 1** — release-blocker scope items (7/8 items done)
  - [x] 1. Self-service password reset (completed 2026-05-09)
  - [x] 2. User invite flow (completed 2026-05-12)
  - [x] 3. Cross-currency transfer endpoint (completed 2026-05-10)
  - [x] 4. Stock-split lot rewrite (completed 2026-05-10)
  - [x] 5. Cross-session OFX duplicate detection (completed 2026-05-11)
  - [x] 6. Reverse-proxy + TLS production example (completed 2026-05-11)
  - [x] 7. CI API test job (completed 2026-05-09)
  - [ ] 8. Tauri removal
- [ ] **Phase 2** — hardening of high-risk service code (2/11 items done)
  - [x] 9. Triage 8 pre-existing test failures (completed 2026-05-10)
  - [x] 10. Rebuild stage2_schema_contract.py from Base.metadata (completed 2026-05-11)
- [ ] **Phase 3** — frontend tests (in flight 2026-05-12 — detail plan: [phase-3-plan.md](phase-3-plan.md). Workstreams F + A1 shipped: root-level Tauri cleanup, Vitest + jsdom + @testing-library/svelte infra, sanity test green.)
- [ ] **Phase 4** — nice-to-have scope items

## Findings discovered while executing the plan

These were found while building Phase 0 and are tracked here so they don't get lost:

- **`/api/v1/health` is transitively auth-protected.** The route depends on `get_book_service`, which depends on `get_access_policy`, which depends on `require_request_context`. Health checks should be unauthenticated. Severity **H**; address in Phase 2 alongside other auth wiring fixes. Location: [apps/api/src/rekenraam_api/api/v1/health.py](../../apps/api/src/rekenraam_api/api/v1/health.py).
- **`/api/v1/accounts/balances` only returns accounts with at least one posted split.** Documented in the smoke test. Severity **N** (UI works around it). May surprise API consumers; consider returning a zero-balance row for every active account.
- **8 pre-existing test failures on `main` — RESOLVED 2026-05-10.** Phase 2 step 9 fixed 6, skipped 2 with forward-pointers. CI now runs the full suite with no quarantine flags. See the per-test outcome in §Phase 2 step 9 below.

- **`AuthorizationError` was masked as 400 in many routes — FIXED 2026-05-10.** While triaging Phase 2 step 9, discovered that `AuthorizationError` inheriting from `ValueError` meant any route with `try: ... except ValueError: HTTP_400_BAD_REQUEST` was silently swallowing 403 errors. Fixed at the root by changing the base class to `Exception`. A follow-up audit (Phase 2 step 7) should grep every `except ValueError` in `apps/api/src/rekenraam_api/api/v1/` to confirm no other auth-shaped errors are similarly affected.

- **`stage2_schema_contract.py` is materially incomplete — RESOLVED 2026-05-11.** Phase 2 step 10 rebuilt the module to derive both halves of the contract from canonical sources (`Base.metadata` for ORM, Postgres reflection for the migrated DB). 24 tables that were missing from the hand-written dict (the milestone 7 planning, milestone 8 investments, milestone 9 ergonomics/templates, milestone 10 MFA, and Phase 1 password-reset families) are now covered. Side findings logged below.

- **`ruff format` not enforced — RESOLVED 2026-05-10.** Bulk-formatted `apps/api` with `ruff format`, added `make api-format-check`, and enabled `ruff format --check` in the API CI workflow. Future Python formatting drift now fails CI.

- **Numeric/boolean `server_default` strings need `sa.text(...)` everywhere — RESOLVED 2026-05-11 across ORM and migrations.** Surfaced in step 10 by the schema-drift detector. Detail: `server_default="0"` (raw string) compiles to `DEFAULT '0'` (a string literal) under SQLAlchemy. Postgres implicit-casts `'0'::bigint` and stores `'0'` as text in `pg_attrdef`, so reflection returns `'0'` (with quotes). The clean pattern is `server_default=text("0")`, which produces `DEFAULT 0` and round-trips identically through reflection. 32 ORM columns and 6 migration columns were converted. Severity **N** because the in-database behavior was equivalent for our column types, but the inconsistency was a real footgun for any future use of `Base.metadata.create_all()` or fresh migrations on different DDL targets.

- **`PasswordResetToken` was not exported from `db/models/__init__.py` — FIXED 2026-05-11.** Severity **N**. The class still landed in `Base.metadata` because the parent module gets imported transitively, but the `__init__.py` re-export list was inconsistent. Style follow-up: future ORM additions must update both the class and the re-export list.

- **`scheduled_transactions.interval` column uses a Postgres reserved word.** Severity **N**. Works because SQLAlchemy quotes column names in its generated SQL, but a footgun for raw SQL elsewhere (`SELECT interval FROM scheduled_transactions` would fail). Worth renaming to `interval_count` before the baseline hardens further. Not in any phase.

- **Pending-user state vs deactivated-user state are conflated.** Severity **N**. Surfaced building Phase 1 step 2: an invited-but-not-accepted user has `is_active=False` and `password_hash IS NULL`. An admin-deactivated existing user also has `is_active=False`. The invite-accept path activates whoever owns the token, which is correct for the pending case. Today this can't be exploited — `create_invite` rejects emails belonging to any user with `password_hash IS NOT NULL` — but the model is fragile. Cleaner long-term: add a discriminator (`status: 'pending' | 'active' | 'deactivated'`) so the three cases are distinct. The invite e2e test `test_accept_rejects_when_user_deactivated_between_issue_and_accept` is `@pytest.mark.skip`-ped pending this. Worth pulling into Phase 2 step 7 (auth depth).

### v1 accounting-correctness fixes (2026-05-12)

After the small-business-vs-personal-finance decision (v1 stays personal-finance;
small-business is v2), the following H-severity correctness gaps were closed
in v1 as the cheap, scope-aligned wins. Each landed with a focused regression
test exercising real Postgres semantics.

- [x] **Partial unique index on `accounts(book_id, system_role) WHERE system_role
  IS NOT NULL`.** Eliminates the race window where two concurrent
  `_ensure_system_account` calls could insert duplicate `income_summary` /
  `expense_summary` / `retained_earnings` rows for a book. Index added in
  baseline schema and ORM model. Regression test:
  [test_system_role_per_book_is_unique](../../apps/api/tests/test_accounting_invariants.py).
- [x] **`accounts.system_role` allow-list `CHECK`.** Restricts the column to
  `{'opening_balance', 'imbalance_import', 'income_summary',
  'expense_summary', 'retained_earnings'}` — the canonical set used by
  `services/admin.py` and `repositories/imports.py`. CHECK added in baseline +
  ORM model. Regression test:
  [test_system_role_allow_list_rejects_bad_values](../../apps/api/tests/test_accounting_invariants.py).
- [x] **`price_sources` "Manual" row seeded.** The 10 provider rows were
  already seeded; the manual entry-point used by user-supplied prices was
  missing. Seeded as `id=1` so application code that expects a stable manual
  source id works.
- [x] **`UNIQUE(split_id, lot_id)` on `split_lot_allocations`.** The legacy
  SQLite schema enforced this via a composite primary key; the Postgres
  baseline used a surrogate `id` PK and dropped the natural-key uniqueness.
  Re-added as a UNIQUE constraint. Regression test:
  [test_split_lot_allocation_rejects_duplicate_split_lot_pair](../../apps/api/tests/test_accounting_invariants.py).
- [x] **Generic audit-log table + SQLAlchemy `before_flush` listener.** New
  `audit_log` table (`table_name`, `row_pk`, `op`, `before_state JSONB`,
  `after_state JSONB`, full attribution from `RequestContext`). New listener
  in [audit_listener.py](../../apps/api/src/rekenraam_api/db/audit_listener.py)
  observes every audit-logged ORM class via a marker (`__audit_logged__ = True`).
  21 business-table classes marked: accounts, transactions, splits, payees,
  categories, tags, people, projects, institutions, commodities,
  account_balancings, balance_checks, balance_adjustments, balance_constraints,
  lots, corporate_actions, split_lot_allocations, price_observations,
  investment_instruments, cost_basis_profiles, price_sources, pricing_policies,
  pricing_source_assignments, import_rules, report_definitions. Captures
  insert/update/delete with full before/after JSON snapshots; coerces dates,
  Decimals, and bytes for JSONB compatibility; stamps `actor_user_id`,
  `actor_session_id`, `actor_device_id`, `actor_request_id` from the request
  context. The listener is the v1 stand-in for the v2 trigger-based approach
  in [`accounting-foundations.md`](../architecture/accounting-foundations.md)
  §5 (F1); v2 will replace it with Postgres triggers for direct-SQL coverage.
  Regression tests in
  [test_audit_log_listener.py](../../apps/api/tests/test_audit_log_listener.py).
  Test verdict on real Postgres: **197 passed, 2 skipped, 0 failed.**

Schema-contract test (`test_alembic_head_matches_full_stage2_schema_contract`)
passes against the updated baseline, so ORM and migration stayed in sync.

### Append-only / audit-trail enforcement (2026-05-12)

The legacy SQLite schema versioned 24 tables via `previous_X_id` chains protected by `RAISE`-on-UPDATE/DELETE triggers — every "edit" inserted a new row pointing at the superseded one. The Postgres baseline kept the chain column on five tables only and treats the rest as mutable. With no Postgres triggers, enforcement lives entirely in Python repository code.

Audit verdict on the five chained tables:

| Table | Chain enforced? | Notes |
|---|---|---|
| `transactions` | **YES** | Every edit/void/duplicate/import path inserts a new row with `previous_tx_id`. No `session.delete`, no `UPDATE`. |
| `accounts` | **YES** | `update`, `delete`, and `set_booking_policy` all insert replacement rows; delete uses a hidden tombstone via `is_hidden=True, lifecycle_note="deleted"`. |
| `account_balancings` | **YES** | Voiding inserts a new row with `voided_at` set; never flips it on the existing row. |
| `report_definitions` | **YES** | Update + delete both insert new rows; delete marks `session_id="void:delete"`. |
| `import_rules` | **YES — fixed 2026-05-12** | Originally broken: `delete_rule` mutated `rule.deleted_at` in place. Now writes a tombstone row with `previous_import_rule_id=head.id, deleted_at=now()`, mirroring the other four chains. `list_rules` filters to chain-head and non-tombstone. Regression test in [test_imports_exports.py](../../apps/api/tests/test_imports_exports.py). |

Splits versioning specifically: the audit subagent flagged `replace_transaction_splits` as destroying old splits during update. After verification this is **not the case** — the service layer always calls `replace_transaction_splits(new_tx.id, …)` where `new_tx` is the new chain head; the existing-split delete loop runs against the new tx's empty split set. Old splits remain attached to the old `tx_id`. A regression test now locks in this invariant: [test_repositories.py::test_transaction_update_preserves_original_row_and_splits](../../apps/api/tests/test_repositories.py).

Findings on the 19 tables that lost the chain (sampled: payees, categories, commodities, splits, price_observations):

| Table | Edit behavior | Severity |
|---|---|---|
| `payees` | `update_payee` mutates in place; `delete_payee` is hard `session.delete`. No `audit_events` emitted from `repositories/metadata.py`. | **H** |
| `categories` | Same pattern as payees. | **H** |
| `commodities` | `update_commodity`, `update_currency`, `set_currency_active` all mutate in place. Currency edits may also mutate `Book.base_currency_code` in place. | **H** |
| `splits` | Versioned via the transaction chain (verified above). | OK |
| `price_observations` | `delete_market_price_observation`, `delete_fx_rate_daily_observation`, `delete_fx_rate_official_observation` are all hard deletes. No audit emissions from `repositories/pricing.py`. | **H** |
| `lots` | Mutable. (Not sampled in detail.) | **H** |
| `corporate_actions` | Mutable. (Not sampled.) | **H** |
| `pricing_policies`, `pricing_source_assignments`, `price_sources`, `commodity_price_sources` (this one is missing entirely), `book_base_currency_history` (missing), `notes`, `tags`, `people`, `projects`, `institutions`, `countries`, `currencies` (missing) | Mutable or not present. Some are intentional design simplifications; others lose audit history that the desktop schema preserved. | **N–H** depending on table |

The `audit_events` table exists but is only emitted from `services/transactions.py` and `services/imports.py`, and its payload (`summary: str + metadata_json: str | None`) is a free-text event log, not a before/after snapshot. It does not compensate for the lost chains.

**Decision (2026-05-12):** prefer append-only where it's reasonable. v1 must restore the chain — or at minimum a structured `audit_events` before/after snapshot — for reference data that has a real edit history: payees, categories, commodities (incl. currency edits), price_observations, lots, corporate_actions. Mutable-by-design is acceptable only where the column itself is the audit (e.g., a denormalized counter). Schema-level shape changes (re-adding `previous_X_id` + UNIQUE) need a follow-up migration; the repository-layer behavior change can ship in advance via `audit_events` snapshots.

Action items (Phase 2 step 12 — new):

- [ ] **`metadata.py` reference tables** — payees, categories, commodities, tags, people, projects, institutions, currencies. Either (a) re-introduce `previous_X_id` + UNIQUE columns and switch updates/deletes to "insert new version" + chain-head filtering in `list_*`, or (b) emit a structured `audit_events` row carrying full before/after JSON snapshots on every edit/delete. Option (a) matches the desktop intent and is what we want for tables the user can edit freely. Option (b) is the bare-minimum compatibility path.
- [ ] **`pricing.py` observations** — `delete_market_price_observation` and friends must either tombstone via a new column (`superseded_by_observation_id` or `voided_at` + replacement) or emit a structured `audit_events` snapshot. Price corrections are a real workflow; losing the history is worse than losing it for payees.
- [ ] **Lots and corporate_actions** — these touch tax-basis math; an audit trail is a v1 release-gate concern, not "nice to have."
- [ ] **`AuditEvent.metadata_json` shape** — define a canonical schema (`{"before": {...}, "after": {...}, "diff_keys": [...]}`) and document it so future readers can decode it without per-event_type spelunking.

### Findings from the Tauri parity audit (2026-05-12)

These came out of the function-by-function and table-by-table audit of `src-tauri/` ahead of Phase 1 step 8 (Tauri removal). Items already fixed in this pass are checked off; the rest are logged here so they don't get lost.

Fixed during the audit pass:

- [x] **LIKE wildcard escaping.** `Payee.name.ilike(f"%{value}%")` and `Transaction.memo.ilike(...)` in [repositories/imports.py](../../apps/api/src/rekenraam_api/repositories/imports.py) and [repositories/transactions.py](../../apps/api/src/rekenraam_api/repositories/transactions.py) passed user input unescaped — payees containing `%` or `_` would match more than intended. Now escaped via a local `_escape_like` helper using `escape="\\"`. Severity **N**.
- [x] **Commodity scale bound inconsistent.** Legacy SQLite enforced `0..9` for currencies and non-currencies via triggers; Python validators were inconsistent (`metadata.py` only required `>= 0`; `investments.py` allowed `0..12`; baseline had no CHECK). Standardized on `0..12` and enforced everywhere — added `CheckConstraint("scale >= 0 AND scale <= 12")` to [`Commodity`](../../apps/api/src/rekenraam_api/db/models/metadata.py), `quantity_scale`/`price_scale` checks to [`InvestmentInstrument`](../../apps/api/src/rekenraam_api/db/models/investments.py), mirrored in [baseline schema](../../apps/api/alembic/versions/0001_initial_schema.py), and tightened `CurrencyCreateInput`/`CurrencyUpdateInput` validators in [services/metadata.py](../../apps/api/src/rekenraam_api/services/metadata.py). Severity **N**.
- [x] **OFX timezone offset dropped.** Rust's `parse_ofx_datetime_utc` extracts the `[offset:tz]` bracket and normalizes wall-clock to UTC before taking the date; Python's `_parse_ofx_date` only read the first 8 digits. End-of-day OFX entries in non-UTC banks could land on the wrong calendar day. Rewrote [`_parse_ofx_date` in services/imports.py](../../apps/api/src/rekenraam_api/services/imports.py) to parse the optional `HHMMSS` segment and `[offset:tz]` bracket (incl. fractional offsets like `5.5`) and normalize to UTC. Severity **N**.

Logged for later:

- **Detailed Rust function/helper audit added.** The second-pass deletion
  review is now documented in
  [tauri-rust-function-audit-2026-05-18.md](../parity/tauri-rust-function-audit-2026-05-18.md).
  It covers private Rust helpers and tests, not only registered Tauri command
  names.
- **HBCI/MT940 import parser — DONE 2026-05-18.** Rust had
  `parse_hbci_mt940`, `parse_mt940_date`, `.hbci`/`.mt940` detection, and a
  parser test. Python now accepts first-class `hbci` and `mt940` import formats
  and improves the legacy behavior with MT940 field continuation handling,
  structured `:86:` payee/memo/reference extraction, debit/credit signs,
  locale-aware amounts, opening/closing-balance currency extraction, and stable
  fallback import IDs. Covered by parser and auto-detection tests in
  [test_imports_exports.py](../../apps/api/tests/test_imports_exports.py).
- **Transaction time-of-day/timezone fields are not exposed.** Severity **N**.
  Rust transaction create/update/list shapes exposed `occurred_at_utc`,
  `occurred_tz`, `posted_at_utc`, and `posted_tz`, normalized UTC timestamps,
  and required timezone co-presence. Python models have the columns but public
  schemas are date-only. Decide whether date-only is the v1 contract or expose
  these fields.
- **`bulk-delete` semantic change.** Severity **N/H depending on expected UX**.
  Rust `bulk_delete_transactions` physically deleted eligible transactions and
  splits. Python `bulk_delete_transactions` delegates to bulk void, preserving
  history. The safer behavior may be intentional, but the endpoint name now
  promises more destructive semantics than it performs.
- **Import reverse lookup missing.** Severity **N**. Rust
  `list_transaction_import_sessions(tx_id)` exposed transaction-to-import
  session provenance. Python exposes session-to-transactions only.
- **Commodity detail/default helpers missing.** Severity **N**. Rust had public
  `get_commodity`, `commodity_price_sources` CRUD for source-specific ticker
  overrides, and `dividend_income_categories` CRUD for commodity/category
  dividend defaults. Python has commodity list/autocomplete/update, pricing
  source assignments, and explicit dividend input, but no exact equivalents.
- **Import amount-rounding policy is per-account, not a global constant.** Severity **N**. Today `_parse_amount` in [services/imports.py](../../apps/api/src/rekenraam_api/services/imports.py) implicitly uses `Decimal.quantize(Decimal("1"))` (default `ROUND_HALF_EVEN`, banker's). Rust's `import.rs:280-289` truncated extra fractional cents. Real-world need is more nuanced than either: some accounts must carry fractional cents, some allow fractional minor units in transactions but round for balance, some need fractional intermediate currency. Plan: add an account/institution-level rounding policy (`half_even` | `half_up` | `truncate` | `keep_fractional`) with `half_even` as the default. Defer to post-v1.
- **Backend length/value guards lean on DB column limits.** Severity **N**. Rust enforced `MAX_NAME_LEN=512`, `MAX_MEMO_LEN=4096`, `MAX_AMOUNT_MINOR=10^13` in validation.rs. Python relies on SQLAlchemy column types (200/4096) and unbounded `int`. Acceptable for v1 — the frontend must reject overlong/over-magnitude inputs before they hit the API and produce a DB error. Action: add frontend-side validators to match the column constraints; document the API-error contract for boundary cases. No backend change needed.
- **Reports: category-spend zero-balance behavior flipped vs. legacy.** Severity **H**. Rust `report_category_spend` (db_reports.rs:1067) excluded categories with zero matching splits in the date range (because the LEFT JOIN's NULL t.occurred_date fails the `>= date_from` comparison). Python [repositories/reports.py:399-425](../../apps/api/src/rekenraam_api/repositories/reports.py) explicitly `OR`s in `Transaction.id.is_(None)` so categories with no splits in the window appear with `total_minor=0`. The Python output is arguably more useful for budgeting UIs but it does change report contents. Decide and lock the contract before users start saving definitions against the current shape. If we keep Python's behavior, add a test naming it; if we revert, remove the `is_(None)` clause. Phase 2 step 5 (report cache invalidation) is the natural home for this.
- **Reports: no generic `run_report` executor.** Severity **N** (likely OBSOLETE). Rust supported arbitrary SQL/template reports with parameter validation, caching, and pricing-context parameters. Python only implements `cashflow`, `category_spend`, `payee_totals`, `net_worth`, `account_trends` as hardcoded service methods. The `report_definitions` table still has `query_type IN ('sql','template','builtin')` and `report_runs.pricing_mode`, so the schema is set up for it, but no executor exists. Two options: (a) drop `query_type`/`pricing_mode` from the schema if custom reports were intentionally cut, or (b) port a minimal executor for `template` type only. Either is fine for v1; the current state is "schema implies a feature that has no code."
- **Reports: no prune for `report_runs`.** Severity **N**. Rust had `prune_report_runs(book_id, retain_per_definition)` to bound the cache. Python's `report_cache` will grow unbounded. Wire a periodic prune (or LRU eviction on insert) before users notice. Operational, not a release blocker.
- **`accounts(book_id, system_role)` lacks a partial unique index.** Severity **H**. SQLite had `idx_accounts_system_role_unique` with `WHERE system_role IS NOT NULL`, ensuring at most one `income_summary`/`expense_summary`/`retained_earnings` per book. Postgres has no such constraint, so a bug in lazy-creation logic could double-mint a system account. Add the partial unique index. The system-role allow-list is also unguarded: legacy CHECK accepted only the three above, but [repositories/imports.py:244](../../apps/api/src/rekenraam_api/repositories/imports.py) writes `imbalance_import`. Pick a canonical set, add a CHECK, document.
- **Three system accounts not seeded in baseline.** Severity **H**. The legacy schema seeded `system_income_summary`/`system_expense_summary`/`retained_earnings` rows; the Postgres baseline only seeds `Assets`/`Cash`/`Opening Balances`. `services/admin.py` references the three system roles by name (lines 305/312/319). Confirm a bootstrap or service path lazily creates them; if not, the close-the-books flow will fail at first use.
- **`split.account.book == tx.book` and `split.category.book == tx.book` invariants not DB-enforced.** Severity **H**. SQLite triggers `trg_splits_book_matches_txn_*` and `trg_splits_category_book_matches_txn_*` prevented cross-book contamination. Postgres has no equivalent and a grep didn't find an app-level check. Either add CHECKs (via composite-FK trick) or assert in `services/transactions.py` validation.
- **`split_lot_allocations` lost composite PK `(split_id, lot_id)`.** Severity **H**. Postgres uses a surrogate `id` PK, so the same split can allocate to the same lot multiple times. Add a `UNIQUE (split_id, lot_id)` constraint.
- **`valuation_snapshots` / `valuation_snapshot_items` tables missing.** Severity **H** if `pricing_mode='frozen'` is supposed to work. `report_runs.valuation_snapshot_id` is a dangling BIGINT with no FK and no target table. Decide: implement the tables, or drop the column.
- **`price_sources` "Manual" (id=1 in legacy) row not seeded.** Severity **H**. The 10 provider rows are inserted (IDs 1001–1010), but manual price entry routes assume a `manual`-kind source row exists. Confirm by tracing a manual price-observation insert path; seed if missing.
- **CHECK constraints lost on multiple `kind`/`status`/`role` columns.** Severity **N** but a footgun. `commodities.kind`, `payees.kind`, `categories.kind`, `people.role`, `projects.status`, `institutions.kind` accept any string. Add CHECKs to match the legacy allow-lists (or document why each was intentionally relaxed).
- **`book_state.change_seq` bump is now manual per write path.** Severity **H**. Legacy used triggers on every mutating table. Now `bump_book_change_seq` is called from service code. Easy to miss on a new table or a new write path → stale report cache. Audit every repository write method against the legacy trigger list; add a CI lint that fails if a new INSERT/UPDATE in `repositories/` doesn't end with a bump (or a comment justifying skip).
- **Missing indexes for common query shapes.** Severity **N**. Postgres lacks several legacy indexes — `(tx, payee)`, `splits(category|tag|person|project)`, `accounts(book_id, type|hidden|institution|country)`, `payees(book_id, name)`, `tags(book_id, name)`, `account_balancings(account_id, voided_at)`, `pricing_source_assignments(effective_from, effective_to, priority)`. Add as performance is measured; not blocking.
- **Pricing feature regressions vs. legacy schema.** Severity **N** if intentional. Postgres dropped: `pricing_policies.{staleness_max_days, triangulation_max_hops, rounding_mode, prefer_official_fx, mode}`; `price_observations.{is_derived, derived_via_commodity_id, triangulation_path_json, supersedes_observation_id, ingest_run_id, period_type, period_year, period_month}`; `commodity_price_sources` table (per-source ticker overrides like `VWRL.AS`); `book_base_currency_history` table. Each of these encodes a real product behavior (FX triangulation audit trail, IRS/HMRC annual fixings, source-specific tickers, historical base-currency timeline). Decide which are cut for v1 vs. need to come back before unfreezing pricing.
- **No CHECK on `transactions.occurred_at_utc IS NOT NULL ↔ occurred_tz IS NOT NULL`.** Severity **N**. Legacy trigger `trg_transactions_datetime_format_ins` enforced co-presence; Postgres has neither check nor confirmed app guard. Add either a CHECK or an assertion in `services/transactions.py`.
- **Investment `corporate_actions.commodity_id` link replaced by instrument-pair link.** Severity **N** (data-migration concern, not a v1 blocker since there's no legacy Postgres data to migrate). Just noting: any external scripts referencing the legacy column shape will break.

---

## 1. Scope coverage gaps

### 1.1 Auth & user lifecycle (Milestone 3 / Milestone 9)

| # | Gap | Location | Sev |
|---|---|---|---|
| 1.1.1 | **Self-service password reset** — **DONE 2026-05-09.** Two public endpoints (`/api/v1/auth/password-reset/request`, `/api/v1/auth/password-reset/confirm`); 24-hour single-use tokens; sessions revoked on confirm; request endpoint always returns 204 to avoid email enumeration; SMTP delivery deferred (audit log is the v1 hand-off mechanism). 9 e2e tests cover happy-path, token reuse, expiry, unknown email, inactive user, bootstrap-not-done, and session revocation. | [auth.py](../../apps/api/src/rekenraam_api/api/v1/auth.py), [services/auth.py](../../apps/api/src/rekenraam_api/services/auth.py), [baseline schema](../../apps/api/alembic/versions/0001_initial_schema.py) | B |
| 1.1.2 | **User invite flow** — **DONE 2026-05-12.** New table `user_invites` in the baseline schema, admin endpoint `POST /api/v1/admin/invites` (issues a 7-day single-use token, optionally with role memberships, idempotent for re-invites of pending users), public endpoint `POST /api/v1/auth/invite/accept` (sets password, activates user, creates session). Pending users sit at `is_active=False`/`password_hash=NULL`; accept flips both. 11 e2e tests cover happy-path, admin-gating, re-invite invalidation, expired token, reuse, short-password, audit rows, role honored. | [admin.py](../../apps/api/src/rekenraam_api/api/v1/admin.py), [services/ergonomics.py](../../apps/api/src/rekenraam_api/services/ergonomics.py), [services/auth.py](../../apps/api/src/rekenraam_api/services/auth.py), [baseline schema](../../apps/api/alembic/versions/0001_initial_schema.py) | B |
| 1.1.3 | **Explicit deactivate semantics** remain thin. PUT `/admin/users/{id}` accepts `is_active`, revokes sessions when set false, and writes a generic `admin.user.updated` audit row. Remaining work is a clearer deactivate/reactivate service action, explicit tests, and separation from pending-invite state. | `services/ergonomics.py`, `services/auth.py` | H |
| 1.1.4 | MFA recovery code regeneration without disabling MFA missing. `confirm_mfa_setup` is the only generator. | `services/auth.py:227-268` | H |
| 1.1.5 | Failed-login throttling is **in-memory** (`_login_failures` dict). Resets on restart and does not work across multiple API replicas. | `services/auth.py` | H |
| 1.1.6 | Audit-trail visibility for auth events: events are written but no user-facing "your sessions / your activity" view. | new `/api/v1/auth/me/sessions`, `/api/v1/auth/me/activity` | H |

### 1.2 Accounts & transactions (Milestone 4)

| # | Gap | Location | Sev |
|---|---|---|---|
| 1.2.1 | **Account close/reopen as first-class actions**: PUT `is_closed` works, records `lifecycle_event` (`close`/`reopen`), exposes directives, and close validation blocks invalid closes. Remaining work is a clearer dedicated close/reopen service/API shape and lifecycle-specific audit rows. | [accounts.py](../../apps/api/src/rekenraam_api/api/v1/accounts.py), `services/accounts.py:79` | H |
| 1.2.2 | **Cross-currency transfers** — **DONE 2026-05-10.** Dedicated `POST /api/v1/transactions/transfer` records both currency legs, requires an explicit transfer-date FX rate, stamps that rate as a manual FX observation, and writes a realized FX gain/loss split when the source amount differs from the rate-implied amount. Ordinary transaction mutation still rejects mixed-commodity balancing. | [api/v1/transactions.py](../../apps/api/src/rekenraam_api/api/v1/transactions.py), [services/transactions.py](../../apps/api/src/rekenraam_api/services/transactions.py), [repositories/transactions.py](../../apps/api/src/rekenraam_api/repositories/transactions.py) | B |
| 1.2.3 | Memorized **splits** & transaction templates exist as routes but split lines on templates are not validated to balance before save. | `services/ergonomics.py` (templates), `api/v1/templates.py` | H |
| 1.2.4 | Locked-range validation is service-only. No DB constraint. A direct SQL write or a bug in `_ensure_unlocked` lets writes through. | add CHECK or trigger; or partial unique index on `(account_id, txn_date)` against locked range | H |
| 1.2.5 | Bulk-void exists; **bulk-delete** exists; but there is no transactional guarantee tested that all-or-nothing on partial failure. | `services/transactions.py` | H |

### 1.3 Reconciliation (Milestone 5)

| # | Gap | Location | Sev |
|---|---|---|---|
| 1.3.1 | **Void-of-reconciled** — **DONE 2026-05-16.** Existing `_ensure_unlocked` already rejects writes (void, delete, status change) to a transaction whose date falls inside an active reconciliation lock. The 1.3.1 policy ("reject with 409 + require explicit unlock first") is therefore enforced implicitly through the locked-range check, since a reconciled transaction is necessarily inside its parent reconciliation's window. The user's recovery path is the existing `POST /api/v1/reconciliation/accounts/{id}/unlock` flow. Verified by `test_void_of_reconciled_transaction_is_rejected_until_unlock` in [test_reconciliation.py](../../apps/api/tests/e2e/test_reconciliation.py). | `services/reconciliation.py`, `services/transactions.py` | H |
| 1.3.2 | **Balance-constraint singleton** — **DONE 2026-05-16.** `ReconciliationService.create_constraint` now queries `list_constraints(account_id)` and raises `ValueError("balance constraint already exists for this account; delete it first to replace")` if any exist; the router maps that specific message to 409. The schema has no date columns, so "overlap" reduces to "more than one constraint per account" — the strict invariant from the user's decision matrix. Verified by `test_second_balance_constraint_on_same_account_is_rejected`. | `services/reconciliation.py`, `api/v1/reconciliation.py` | H |
| 1.3.3 | **Race protection on concurrent reconciliations** — **DONE 2026-05-16.** New `ReconciliationRepository.acquire_reconciliation_lock(account_id)` issues `pg_advisory_xact_lock(0x52454301, account_id)` keyed on the account. Called at the top of both `start` and `finish` before any reads. Released automatically at txn commit/rollback. Verified by `test_concurrent_finish_on_same_account_serializes` which fires two parallel finishes via `asyncio.gather` and asserts exactly one 200 + one 409. | `repositories/reconciliation.py`, `services/reconciliation.py` | H |

### 1.4 Imports & exports (Milestone 6)

| # | Gap | Location | Sev |
|---|---|---|---|
| 1.4.1 | **Import mapping templates** export/import (scope `Should Have`). | `services/imports.py`, new `import_mapping_templates` table or JSON export | N |
| 1.4.2 | **Persistent payee/category cleanup rules** beyond `ImportRule`: payee normalization (regex/canonical), category mapping by payee, duplicate-handling preferences. | `services/imports.py` extension | N |
| 1.4.3 | **Cross-session duplicate detection** — **DONE 2026-05-11.** Import commits now record consumed OFX `FITID`/CSV `import_id` values in an account-scoped `import_transaction_keys` table with a database unique constraint. Preview and commit consult that key before inserting, so even `always_create` re-imports validate against the existing transaction instead of posting a duplicate. | [models/imports.py](../../apps/api/src/rekenraam_api/db/models/imports.py), [repositories/imports.py](../../apps/api/src/rekenraam_api/repositories/imports.py), [services/imports.py](../../apps/api/src/rekenraam_api/services/imports.py), [baseline schema](../../apps/api/alembic/versions/0001_initial_schema.py) | B |
| 1.4.4 | CSV export does not test/escape special characters (commas, quotes, newlines in payee/memo) or honor user `number_format`/`date_format` preferences. | `services/exports.py` | H |

### 1.5 Reports (Milestone 8)

| # | Gap | Location | Sev |
|---|---|---|---|
| 1.5.1 | **Account-statement / income-expense reports** (scope `Should Have`) — present in scope as widening but not exposed. | `services/reports.py`, new endpoints | N |
| 1.5.2 | **Budget-variance** is a budget endpoint, not a report. Scope wants it as part of "core reports". Either add `/reports/budget-variance` alias or document the redirect. | `api/v1/reports.py` | N |
| 1.5.3 | Report-cache invalidation on **book base-currency change**: a base-currency switch should invalidate every cached report run. Currently `book_state.change_seq` only bumps on writes, not on currency-history changes. | `services/report_invalidation.py` | H |
| 1.5.4 | Print-friendly view (scope `Should Have`). | frontend reports routes | N |

### 1.6 Investments & pricing (Milestone 8)

| # | Gap | Location | Sev |
|---|---|---|---|
| 1.6.1 | **Stock-split lot adjustment** — **DONE 2026-05-10.** `split` and `reverse_split` corporate actions now rewrite positive open lots through a generated transaction: old lots are closed, replacement lots preserve opened date and remaining cost basis, fractional minor-unit outcomes are rejected, and generated close allocations are excluded from realized-gains reporting. | [services/investments.py](../../apps/api/src/rekenraam_api/services/investments.py), [repositories/investments.py](../../apps/api/src/rekenraam_api/repositories/investments.py) | B |
| 1.6.2 | **Mixed-consideration corporate actions** (cash + stock) and **cash-in-lieu** for fractional shares are recorded but not posted. Scope explicitly accepts "structured event records" but valuation will be off. | `services/investments.py` | H |
| 1.6.3 | **Short-cover gain/loss**: the endpoint exists; cost-basis treatment for short positions (negative lots) is not documented and may not handle wash-sale-like edge cases. | `services/investments.py` | H |
| 1.6.4 | **FX cross-rate triangulation** when a direct pair is missing (e.g. need EUR→JPY, only have EUR→USD and USD→JPY). | `services/pricing_execution.py` | H |
| 1.6.5 | **Source-assignment effective-date supersession**: `as-of` lookup with overlapping assignments not validated. | `services/pricing.py` | H |
| 1.6.6 | **Rate staleness** warning: when last observation is older than `max_backfill_days`, valuation should flag. | `services/pricing.py`, frontend valuation views | H |

### 1.7 Operational hardening (Milestone 11)

| # | Gap | Location | Sev |
|---|---|---|---|
| 1.7.1 | **Reverse proxy / TLS production example** — **DONE 2026-05-11; SQLite-first updated 2026-05-17.** Production Compose includes a Caddy service that terminates HTTPS with automatic ACME certificates, stores Caddy state in named volumes, exposes `80`/`443`, and proxies to the SQLite `app` container for v1. Deployment docs cover DNS, firewall ports, Caddy logs/validation, secure cookies, CORS origin, MFA, private SQLite volume, and LAN-only HTTP caveats. | [compose.sqlite.public.yaml](../../compose.sqlite.public.yaml), [Caddyfile](../../docker/caddy/Caddyfile), [docs/deployment/self-hosting.md](../../docs/deployment/self-hosting.md) | B |
| 1.7.2 | CI API test job — **DONE 2026-05-09; SQLite-first updated 2026-05-17.** [.github/workflows/api-tests.yml](../../.github/workflows/api-tests.yml) runs `ruff check`, `ruff format --check`, `pyright` (strict), and `make api-test-coverage` with SQLite as the backend. Triggers on changes under `apps/api/**`, `pyproject.toml`, `uv.lock`, or `Makefile`. Uses `astral-sh/setup-uv` + `uv sync --frozen` so what runs in CI matches local `uv sync`. Explicit Postgres targets remain post-v1. | [.github/workflows/api-tests.yml](../../.github/workflows/api-tests.yml) | B |
| 1.7.3 | **Backup smoke test** present (`scripts/restore_smoke.sh`) but not wired into CI on a schedule. | `.github/workflows/operational-self-hosting.yml` | H |
| 1.7.4 | **Rate-limit / failed-login** state in Postgres or Redis instead of process memory (see 1.1.5). | `services/auth.py` | H |

### 1.8 Tauri removal (Milestone 12 — gate for v1 release)

| # | Gap | Location | Sev |
|---|---|---|---|
| 1.8.1 | `src-tauri/` still exists. | repo root | B |
| 1.8.2 | **DONE 2026-05-12; reverified 2026-05-18.** `package.json` and `package-lock.json` no longer contain `@tauri-apps/*` packages or a `tauri` script. | [package.json](../../package.json), [package-lock.json](../../package-lock.json) | B |
| 1.8.3 | **DONE 2026-05-12; reverified 2026-05-18.** Vite/Svelte config is plain web config; no Tauri fixed-port/HMR/watch-ignore block remains. | [vite.config.js](../../vite.config.js), [svelte.config.js](../../svelte.config.js) | B |
| 1.8.4 | Stale non-runtime references remain after full deletion: `.vscode/extensions.json` still recommends `tauri-apps.tauri-vscode`, `.dockerignore` still ignores `src-tauri/target`, and unreferenced `static/tauri.svg` remains. | [.vscode/extensions.json](../../.vscode/extensions.json), [.dockerignore](../../.dockerignore), [static/tauri.svg](../../static/tauri.svg) | N |

Fresh deletion audit (2026-05-18):

- Active runtime is clean: no frontend `@tauri`, `invoke`, `__TAURI__`, or
  Tauri API imports outside docs/TODO and `src-tauri`; Docker and API entry
  points use `/api/v1`.
- Rust desktop surface still registers 209 commands: 182 through
  `commands::*`, 25 through `db_currencies::*`, and 2 through `fx_refresh::*`.
- Remaining unported or intentionally non-direct ports are desktop-only storage
  and file-picker commands, desktop undo/redo, event/document CRUD, country
  create/update/delete, transaction timestamp/timezone API fields,
  generic/custom `run_report`, `prune_report_runs`, commodity price-source
  override CRUD, dividend income category defaults, and legacy path-based import
  helpers.
  These are either dropped/deferred in the parity matrix or represented by web
  replacements rather than direct ports.

The migration plan exit criteria for v1 release require Tauri to be gone. As
of this pass, the physical `src-tauri/` tree and tiny non-runtime references
are the remaining Tauri-removal tasks; product parity sign-off still depends on
accepting the deferred/dropped command families above.

---

## 2. Test coverage gaps

The repo has 27 backend test files but real coverage is uneven:

| Layer | Style | Reality |
|---|---|---|
| Repositories | Real Postgres session via `repository_session` fixture | Good — 22+ functions in `test_repositories.py` |
| Services | In-memory **fake repositories** | Branch coverage but **does not exercise real SQL**; e.g. locked-range checks pass against a fake that never actually re-reads after write |
| API | **Stub services** in `test_api.py` | Schema/contract only; routers never touch real services |
| End-to-end | None | **No test wires router → real service → real repo → real DB** |

Frontend has **zero tests** (no `*.test.ts`, `*.spec.ts`, no Vitest config).

The biggest structural gap is the absence of an end-to-end seam. A single integration fixture that boots the FastAPI app against a real Postgres test database (via `testcontainers` or the existing compose dev DB) and exercises full flows would catch every category below.

### 2.1 Reconciliation — high risk, thinly tested — **DONE 2026-05-16**

Scope at [services/reconciliation.py](../../apps/api/src/rekenraam_api/services/reconciliation.py) is now 538 LoC (was 522 — added the advisory-lock call sites + the constraint-singleton check). 9 e2e tests in [apps/api/tests/e2e/test_reconciliation.py](../../apps/api/tests/e2e/test_reconciliation.py) cover the original gap items end-to-end against real Postgres:

- [x] **Locked-range write rejection** — `test_post_txn_dated_inside_locked_reconciliation_window_returns_409`
- [x] **Concurrent finish (same account)** — `test_concurrent_finish_on_same_account_serializes` (2 parallel `asyncio.gather` calls, exactly one 200 + one 409)
- [x] **Constraint validation surfaced** — `test_finish_with_balance_below_min_constraint_validates_and_warns` (covers both valid + violation cases via `validate_constraints` endpoint)
- [x] **Unlock clears locked state** — `test_unlock_clears_locked_state_before_post` (post 409 before unlock, post 200 after unlock)
- [x] **Offset-account currency mismatch** — `test_finish_with_offset_in_wrong_commodity_rejected` (rejected with `account and offset account must share the same commodity`)
- [x] **Void of a reconciled split** — `test_void_of_reconciled_transaction_is_rejected_until_unlock` (bulk-void returns 0 for reconciled; succeeds after unlock)
- [x] **Adjustment write failure rollback** — `test_unbalanced_finish_without_offset_account_is_rejected_cleanly` (no partial balance_check / balancing rows land)
- [x] **Happy path still works** — `test_reconciliation_happy_path_still_works` (smoke after all the new invariants)

Plus the deferred Phase 2 step 9 skip:

- [x] **`test_import_commit_marks_session_abandoned_on_locked_account`** — un-skipped after fixing the underlying `MissingGreenlet` bug in [services/imports.py](../../apps/api/src/rekenraam_api/services/imports.py). Capture `session.id` to a local before the first `db_session.commit()` so the post-rollback abandonment path doesn't trigger a lazy attribute reload outside a greenlet-safe context. Now passes both as a service-level test and as a new e2e test in [tests/e2e/test_imports.py](../../apps/api/tests/e2e/test_imports.py).

Test count: **233 passed / 1 skipped** (was 222/2 before this work). The remaining skip is the user-invite/deactivated-state test pending the pending-vs-deactivated user model cleanup (logged in §Findings).

### 2.2 Transactions — service tested with fakes only

Locked-range, split balancing, status transitions, duplicate-of-reconciled, and bulk atomicity all rely on `_ensure_unlocked` and split sum logic that the service tests use a fake repo to bypass. Missing:

- Post txn whose `txn_date` is before account effective-from → reject.
- Cross-currency split that does not balance in book base → reject.
- Status transition `reconciled → uncleared` is rejected unless via reconciliation unlock.
- `bulk_void` on N txns where one is reconciled: all-or-nothing rollback test.
- `duplicate` of a reconciled txn produces an *uncleared* copy (not reconciled).
- Pagination keyset stability when txns share `txn_date` and `id` ordering matters.

### 2.3 Investments — wide endpoint coverage, shallow math coverage

`test_investments_service.py` uses `StubInvestmentRepository`. Missing:

- FIFO vs LIFO vs avg-cost vs specific-lot under partial sale (sell 60 of 100 shares acquired across 3 lots).
- Cost basis after **stock split**: 100 sh @ $10 then 2:1 → lots show 200 sh @ $5 with realized gain on subsequent sale at $6 = $200 not $-400.
- Reinvested-dividend lot uses dividend payment date, not declaration date.
- Short sell → cover: gain = (short price − cover price) × qty; loss path; cover for more than shorted (wraps to long).
- Multi-currency holding: account in USD, instrument quoted in EUR, book in USD — performance computed against book.
- Corporate-action **conversion** (acquirer issues new ticker): old lots remap; cost basis preserved.
- Holding-period crossover (short-term to long-term boundary) for tax-aware reports.

### 2.4 Pricing & FX

Missing:

- Cross-rate triangulation when direct pair absent.
- Effective-date overlap on pricing source assignments: as-of lookup picks correct row.
- Weekend `skip` vs `fill_previous` policy actually applied on report dates that fall on Saturday.
- Rate staleness threshold surfaces as warning, not silent stale value.
- Provider failure: graceful fallback to next assignment, audit row written.
- Manual price entry for non-currency instruments must not collide with provider-supplied observation (supersession order).

### 2.5 Reports

Missing:

- Cache invalidation when a transaction is written, when a corporate action is added, when book base currency changes, when a pricing source assignment shifts.
- Net-worth: zero-balance account inclusion/exclusion, closed-account handling, as-of date exactly equal to a txn date (boundary).
- Realized-gains policy switch (FIFO ↔ LIFO) on the same book — produces different numbers; test that it does and that audit references the policy.
- Currency-exposure report excludes the book base by design — verify.
- Account-trends bucketing across DST boundary.

### 2.6 Budgets, schedules, loans

Missing:

- Budget rollover Jan 31 → Feb 28: monthly budget anchored on day 31 has 11 occurrences in a non-leap year, not 12. Test for Feb behavior.
- Edited future occurrence of a recurring schedule does not retroactively edit past posted txns.
- Skip then post idempotency: posting a skipped occurrence should be a no-op.
- Loan amortization: principal $250k @ 6.5% × 360 mo — final balance must be exactly zero (penny rounding distributed to last payment).
- Extra principal payment shortens schedule and recomputes interest.

### 2.7 Auth

Missing:

- Session expiry boundary (request 1ms after expiry → 401).
- Logging out one device does not log out others (current behavior unverified).
- MFA-required login that succeeds on TOTP code; that succeeds on recovery code; recovery code single-use enforced.
- Cross-book permission isolation: user is editor in book A, viewer in book B; PUT on book B txn → 403.
- Request-context propagation: nested service calls preserve `user_id`, `session_id`, `request_id` on all audit rows.

### 2.8 Imports & exports

Missing:

- Re-import same OFX file → 0 duplicates created (must rely on FITID).
- Re-import same CSV with different encoding (UTF-8 BOM, Latin-1) — same duplicate detection.
- Malformed row (bad date, NaN amount) skipped with line-level error in preview, not aborting the whole session.
- Import rule order priority: rule 1 matches, rule 2 also matches — only rule 1 applies.
- CSV export with payee `O'Reilly, Inc. "Books"` — round-trips correctly through CSV reader.
- QIF export of split transaction matches QIF spec format.

### 2.9 Frontend

Zero tests. Minimum to pull in:

- Vitest + @testing-library/svelte set up; one smoke test per critical route.
- Form validation tests: transaction split balance, budget amount, date pickers (Feb 29 in non-leap year).
- Session-expired global redirect.

---

## 3. Fix plan

Plan is grouped into phases ordered by release-blocker first. Each phase lists outputs and an acceptance check. Estimates are ranges in engineering-days assuming one engineer.

### Phase 0 — End-to-end test seam (prerequisite) — **DONE** 2026-05-09

Without this, every other test improvement layers more fakes onto the existing test pyramid that already doesn't catch real bugs.

Delivered:

- Shared `temporary_database()` helper in [apps/api/tests/_postgres.py](../../apps/api/tests/_postgres.py): creates a fresh Postgres database, runs Alembic to head, drops it on exit.
- Refactored [apps/api/tests/conftest.py](../../apps/api/tests/conftest.py) to use the shared helper. The 22 existing repository tests still pass.
- New e2e fixture in [apps/api/tests/e2e/conftest.py](../../apps/api/tests/e2e/conftest.py): per-test Postgres, real `async_sessionmaker`, `app.dependency_overrides[get_db_session]` swap, `httpx.AsyncClient` against the real FastAPI app. Lifespan deliberately not run (would otherwise attempt to read first-admin env vars and start the pricing-refresh worker).
- Factory helpers in [apps/api/tests/e2e/factories.py](../../apps/api/tests/e2e/factories.py): `bootstrap_admin`, `login`, `create_account`, `post_transaction`.
- Canonical smoke test [apps/api/tests/e2e/test_smoke.py](../../apps/api/tests/e2e/test_smoke.py): bootstrap status → bootstrap admin → 409 on second bootstrap → list books → create account → check seeded opening balance → post a balanced transfer → re-check balances → register reads with running balance → logout → 401 on protected route → re-login → 200 again. Runs in ~1.7s end-to-end through the real router/service/repo/Postgres stack.

Not yet delivered (deferred to Phase 1 alongside the API CI job):

- CI workflow that runs the e2e tests against a Postgres service container. Will be folded into the `api-tests.yml` workflow being added in Phase 1 step 7.
- Migrating the existing `test_api.py` stub tests onto the seam. This is intentional drip-work — new tests should use the seam; old stub tests can stay until the modules they cover get a real Phase 2 service-level rewrite.

How to run locally:

```bash
TEST_POSTGRES_HOST=127.0.0.1 \
TEST_POSTGRES_PORT=55432 \
TEST_POSTGRES_USER=rekenraam \
TEST_POSTGRES_PASSWORD=rekenraam \
TEST_POSTGRES_ADMIN_DB=rekenraam \
.venv/bin/pytest apps/api/tests/e2e/
```

The same env-var convention (`TEST_POSTGRES_*`) is honored by the existing repository tests, so a single Postgres instance covers both.

### Phase 1 — Release-blocker scope items (5-7 d)

In order:

1. **Self-service password reset** (1.1.1) — **DONE 2026-05-09.** Delivered:
   - Baseline schema [0001_initial_schema.py](../../apps/api/alembic/versions/0001_initial_schema.py) creates `password_reset_tokens` (token-hash unique index, expires-at index, FK to users with cascade).
   - `PasswordResetToken` ORM model in [db/models/access.py](../../apps/api/src/rekenraam_api/db/models/access.py) mirroring `MfaChallenge`.
   - Repository methods on [`AccessRepository`](../../apps/api/src/rekenraam_api/repositories/access.py): `create_password_reset_token`, `get_active_password_reset_token`, `mark_password_reset_token_used`, `update_user_password_hash`, plus a new `revoke_all_user_sessions` for the post-confirm session sweep.
   - Service methods on [`AuthService`](../../apps/api/src/rekenraam_api/services/auth.py): `request_password_reset` (emits 24h-TTL token, audit row for known/unknown/inactive paths, refuses before bootstrap) and `confirm_password_reset` (validates active token, hashes new password via argon2, marks token used, revokes all sessions for the user, emits audit row).
   - Endpoints `POST /api/v1/auth/password-reset/{request,confirm}` in [api/v1/auth.py](../../apps/api/src/rekenraam_api/api/v1/auth.py); request endpoint always 204 to prevent email enumeration; confirm returns 400 on bad/expired/used tokens, 422 on Pydantic validation, 204 on success.
   - 9 e2e tests in [tests/e2e/test_password_reset.py](../../apps/api/tests/e2e/test_password_reset.py): known-vs-unknown email both 204, no token row for unknown; bootstrap-required gate; full round-trip with new-password login and old-password rejection; single-use enforcement; expiry enforcement (DB-aged token); short-password rejection without consuming the token; bogus-token rejection; session revocation on confirm; inactive user treated as unknown.
   - SMTP delivery is deferred to post-v1; until then admins retrieve the issued token from the audit log to hand to the user. The wire contract supports adding email later without breaking changes.
2. **User invite** (1.1.2) — **DONE 2026-05-12.** Delivered:
   - Baseline schema [0001_initial_schema.py](../../apps/api/alembic/versions/0001_initial_schema.py) creates `user_invites` (token-hash unique index, expires-at index, `invited_by_user_id` FK with SET NULL).
   - `UserInvite` ORM model; `ErgonomicsRepository.create_user` generalized to accept `password_hash: str | None` and `is_active: bool` so the same helper handles bootstrap-active users and pending-inactive invitees.
   - Repository methods on [`AccessRepository`](../../apps/api/src/rekenraam_api/repositories/access.py): `create_user_invite`, `get_active_user_invite`, `mark_user_invite_used`, `invalidate_outstanding_user_invites`.
   - Service split: [`AuthService.issue_invite_token`](../../apps/api/src/rekenraam_api/services/auth.py) owns the token lifecycle; [`AuthService.accept_invite`](../../apps/api/src/rekenraam_api/services/auth.py) handles the public accept flow (password set, activate, single-use token mark, session creation, audit). [`ErgonomicsService.create_invite`](../../apps/api/src/rekenraam_api/services/ergonomics.py) handles the admin-side user/membership creation and asks `AuthService` to mint the token. The two services collaborate via `auth_service` injection on the ergonomics constructor (TYPE_CHECKING import to avoid the cycle); the wiring is centralized in `get_ergonomics_service`.
   - Endpoints `POST /api/v1/admin/invites` (admin-router-gated; returns `{user, token, expires_at}`) and `POST /api/v1/auth/invite/accept` (public; returns `AuthMe` and sets the session cookie).
   - 11 e2e tests in [tests/e2e/test_user_invite.py](../../apps/api/tests/e2e/test_user_invite.py): full round-trip with role memberships, login pre-acceptance is rejected, admin-gating, conflict on already-accepted email, idempotent re-invite that invalidates prior token, unknown/expired/reused tokens rejected, short password rejected without consuming token, audit rows written, is_admin honored both ways. One test deferred-skipped pending the pending-vs-deactivated state model cleanup (logged below).
3. **Cross-currency transfer endpoint** (1.2.2) — **DONE 2026-05-10.** Delivered:
   - Endpoint `POST /api/v1/transactions/transfer` in [api/v1/transactions.py](../../apps/api/src/rekenraam_api/api/v1/transactions.py) with a dedicated `CrossCurrencyTransferInput`.
   - [`TransactionService.create_cross_currency_transfer`](../../apps/api/src/rekenraam_api/services/transactions.py) validates write access, account ownership, non-system/open accounts, different source/destination currencies, an explicit positive FX rate, and locked ranges.
   - The write path posts the source leg in the source account currency, destination leg in the destination account currency, and an optional realized FX gain/loss split in the source currency when `source_amount_minor` differs from `destination_amount_minor * fx_rate` after half-up rounding.
   - [`TransactionRepository.create_manual_fx_observation`](../../apps/api/src/rekenraam_api/repositories/transactions.py) stamps the transfer-date rate as an `fx_manual` `price_observations` row (`source='transfer:{transaction_id}'`), preserving the post-time FX rate used by the transfer.
   - 3 e2e tests in [tests/e2e/test_cross_currency_transfer.py](../../apps/api/tests/e2e/test_cross_currency_transfer.py): two-sided mixed-currency transfer with FX observation, realized gain/loss split correctness, and 422 rejection when the required rate is omitted.
4. **Stock-split lot rewrite** (1.6.1) — **DONE 2026-05-10.** Delivered:
   - `POST /api/v1/investments/corporate-actions` keeps its existing wire shape, but `split` and `reverse_split` actions now require valid directional ratios and reject cash-in-lieu/fractional minor-unit outcomes.
   - [`InvestmentRepository.create_stock_split_corporate_action`](../../apps/api/src/rekenraam_api/repositories/investments.py) records the corporate action, creates a generated transaction, closes affected positive lots, creates replacement lots with preserved `opened_date` and remaining cost basis, and sets `generated_transaction_id`.
   - Realized-gains reporting ignores generated split/reverse-split close allocations so split mechanics are not treated as sales.
   - Focused Postgres tests cover full-sale after 2-for-1 split, partial-sale-before-split basis preservation, holding-period preservation, exact reverse split, fractional rollback, and generated-allocation exclusion.
5. **Cross-session OFX duplicate detection** (1.4.3) — **DONE 2026-05-11.** Delivered:
   - New `import_transaction_keys` table with `UNIQUE (account_id, import_id)` to consume OFX `FITID`/CSV `import_id` values once per account without fighting the versioned transaction-row model.
   - Import preview and commit both consult the persistent key; commit records the key for created and enriched transactions, and treats key hits as validated matches even when the request mode is `always_create`.
   - Focused Postgres regression test re-imports the same OFX `FITID` across sessions and verifies only one transaction/key is created.
6. **Reverse-proxy + TLS production example** (1.7.1) — **DONE 2026-05-11.** Delivered:
   - Caddy service in [`compose.prod.example.yaml`](../../compose.prod.example.yaml) with public `80`/`443`, named ACME/config volumes, and reverse proxying to the frontend container.
   - [`docker/caddy/Caddyfile`](../../docker/caddy/Caddyfile) using `REKENRAAM_PUBLIC_HOST` for automatic HTTPS.
   - Production docs for DNS, firewall ports, secure-cookie/CORS/MFA settings, Caddy logs/validation, private Postgres, and keeping the direct frontend port local-only.
7. **CI API test job** (1.7.2) — **DONE 2026-05-09.** Delivered:
   - [.github/workflows/api-tests.yml](../../.github/workflows/api-tests.yml) with a Postgres 16 service container, `astral-sh/setup-uv@v6`, `uv sync --frozen`, then `make api-lint` (ruff), `make api-format-check` (`ruff format --check`), `make api-typecheck` (pyright strict), and `pytest` with the 8 pre-existing failures quarantined via `--deselect`.
   - Triggers on push and pull_request when `apps/api/**`, `pyproject.toml`, `uv.lock`, `Makefile`, or the workflow itself changes.
   - First useful catch: pyright caught a `reportUnknownMemberType` regression in [`AccessRepository.revoke_all_user_sessions`](../../apps/api/src/rekenraam_api/repositories/access.py) that local pytest had passed. Fixed in the same PR by mirroring the `cast(CursorResult[object], result).rowcount` pattern used in [ergonomics.py](../../apps/api/src/rekenraam_api/repositories/ergonomics.py).
   - `ruff format --check` was enabled 2026-05-10 after a focused bulk-format pass over `apps/api`.
8. **Tauri removal** (1.8.1–1.8.3): delete `src-tauri/`, drop `@tauri-apps/*` deps, drop `"tauri"` script, remove Tauri-specific Vite config. Verify `npm run check` and `npm run build` still pass.

Acceptance: every release-gate clause in `v1-scope.md` is satisfied or has a documented deferral.

### Phase 2 — Hardening of high-risk service code (5-8 d)

1. **Reconciliation correctness** (1.3.1, 1.3.2, 1.3.3, 2.1) — **DONE 2026-05-16.** Delivered:
   - 1.3.3 — `pg_advisory_xact_lock(0x52454301, account_id)` in `ReconciliationRepository.acquire_reconciliation_lock`, called at the top of both `start` and `finish`. Transaction-scoped so it releases automatically at commit/rollback. Chose advisory-lock over `SELECT ... FOR UPDATE` (the plan's literal suggestion) because it doesn't block unrelated account-table updates (rename, close, etc.) for the duration of a reconciliation.
   - 1.3.2 — `ReconciliationService.create_constraint` rejects a second constraint per `(book_id, account_id)` with `ValueError("balance constraint already exists for this account; delete it first to replace")`. Router maps that specific message to 409 (other ValueErrors stay 400). The schema has no date columns, so "overlap" reduces to "more than one constraint per account"; the strict-singleton invariant is the right shape.
   - 1.3.1 — verified: voiding a reconciled split is already rejected via the existing `_ensure_unlocked` check in `TransactionService`. The locked-range mechanism subsumes 1.3.1 because every reconciled txn is necessarily inside its parent reconciliation's window. No new code; documented by test.
   - 9 new e2e tests in [test_reconciliation.py](../../apps/api/tests/e2e/test_reconciliation.py) covering §2.1 (see that section's checklist above).
   - Side fix: the deferred `test_import_commit_marks_session_abandoned_on_locked_account` was un-skipped after fixing the root-cause `MissingGreenlet` bug in `services/imports.py` (lazy reload of `session.id` after `db_session.rollback()` — capture to a local before the first commit).
2. **Transactions correctness** (1.2.4, 1.2.5, 2.2):
   - Add DB-level locked-range check (CHECK or partial index using `book_state.locked_through`).
   - All-or-nothing test for `bulk_void` and `bulk_delete`.
   - Cover §2.2 scenarios.
3. **Investments math** (1.6.2, 1.6.3, 2.3): cover FIFO/LIFO/avg/specific under partial sale, dividend reinvestment, short cover, multi-currency holding, conversion corporate action.
4. **Pricing supersession & triangulation** (1.6.4, 1.6.5, 1.6.6, 2.4).
5. **Report cache invalidation** (1.5.3, 2.5).
6. **Budget rollover, schedule recurrence, loan amortization** (2.6).
7. **Auth depth** (1.1.3, 1.1.5, 2.7): explicit deactivate/reactivate semantics and tests; persistent throttling state (Postgres table or Redis); session expiry tests; cross-book isolation tests.
8. **CSV export escaping & locale** (1.4.4, 2.8).
9. **Pre-existing test failures** (per the findings above) — **DONE 2026-05-10.** Per-test outcomes:
   - **Fixed (6):**
     - `test_alembic_can_upgrade_downgrade_and_reupgrade_clean_database` — set `script_location` explicitly so the test is cwd-independent.
     - `test_book_membership_roles_gate_writes` — surfaced a systemic bug: `AuthorizationError` extending `ValueError` was being swallowed by the `except ValueError` in many route handlers, returning 400 instead of 403. Fixed at the root by changing `AuthorizationError` to inherit from `Exception`. The global FastAPI exception handler still maps it to 403; routes that already explicitly catch `AuthorizationError` continue to work.
     - `test_user_writes_stamp_audit_context` — replaced the seeded system-account leg (Opening Balances) with a freshly-created Expense account. The transaction service correctly refuses splits on protected system accounts; the test was written before that guard.
     - `test_pricing_repository_lists_sources_updates_policy_and_manages_assignments` — `session.merge()` returns the merged persistent instance; the original `Commodity()` argument stays detached, so the subsequent `.refresh()` raised `InvalidRequestError`. Bound the merged instance back to `eur`.
     - `test_report_repository_returns_cashflow_category_spend_and_payee_totals` — the test asserted a non-empty cashflow row, but `report_cashflow` filters via `Category.kind in ('income','expense')` and the seeded transaction has no category. Updated assertions to the correct empty-shape contract.
     - `test_commodity_autocomplete_and_manual_fx_observations` — added a bootstrap-admin call so the now-protected commodities/pricing routes don't 401.
   - **Skipped with forward-pointer (2):**
     - `test_alembic_head_matches_full_stage2_schema_contract` — schema-contract drift (Phase 2 step 10).
     - `test_import_commit_marks_session_abandoned_on_locked_account` — `sqlalchemy.exc.MissingGreenlet` during the post-error abandonment path on the service-level test. Behavior is correct at the HTTP layer. To be rewritten as an e2e test alongside Phase 2 step 1 (reconciliation correctness).
   - CI workflow updated: deselect list pruned, `pytest -q` runs the full suite. 147 passed, 2 skipped, 0 failed.
10. **Schema-contract rebuild** — **DONE 2026-05-11.** Delivered:
    - Rewrote [apps/api/tests/stage2_schema_contract.py](../../apps/api/tests/stage2_schema_contract.py) (~250 LoC, down from ~1050) so the contract is derived from canonical sources rather than hand-written:
      - `contract_from_metadata(Base.metadata)` produces one `TableContract` per ORM-registered table.
      - `contract_from_database(connection)` reflects the same shape from a live Postgres connection.
      - `normalize_for_comparison(schema)` strips legitimately-different fields (CHECK `sqltext` formatting between Postgres canonicalization and SQLAlchemy's render).
      - Server defaults are now part of `ColumnContract`, with separate ORM and DB normalizers (`_normalize_orm_server_default`, `_normalize_db_server_default`) since the two sources have different shapes.
    - Diagnostic-script run while building: 0 hand-written drift, but **18 server_default mismatches** between ORM and Postgres that the old contract missed entirely. 32 ORM columns fixed (`server_default="0"` -> `server_default=text("0")` etc); 6 migration columns fixed (`sa.Column(..., server_default="0", ...)` -> `sa.Column(..., server_default=sa.text("0"), ...)`).
    - Side fix in the baseline migration mirrors the ORM correction; for fresh deployments going forward the `pg_attrdef` text matches the ORM exactly.
    - New self-test suite in [test_schema_contract.py](../../apps/api/tests/test_schema_contract.py) — 12 tests proving the detector itself works (column add, nullable flip, server_default flip, index add, normalization shapes for raw string / `sa.text(...)` / `func.now()` / autoincrement-PK-sequence). Without this, a future regression in the normalizer could silently mask all defaults.
    - Removed the now-redundant `test_sqlalchemy_metadata_matches_stage2_schema_contract` from `test_orm_schema.py` (subsumed by the migration drift test). Kept `test_milestone7_planning_tables_are_registered` as a fast DB-free sanity check.
    - Re-enabled `test_alembic_head_matches_full_stage2_schema_contract`. CI now runs it as part of the standard suite.
11. **Health endpoint deauth** (Phase 0 finding): remove the transitive `require_request_context` dependency from `/api/v1/health` so the deployment health probe works without a session cookie.
12. **Append-only audit trail for reference data** (see "Append-only / audit-trail enforcement" finding above). Restore versioned writes (or at minimum structured `audit_events` before/after snapshots) for the mutable reference tables: payees, categories, commodities (incl. currency edits and `set_currency_active`), price_observations, lots, corporate_actions. Decide tombstone-via-chain vs. snapshot-via-audit_events per table; schema changes (re-adding `previous_X_id`) go in a follow-up migration. Acceptance: every edit/delete on these tables leaves the prior state recoverable from the database alone.

Acceptance: each module listed has a dedicated `test_*_service.py` (or expanded existing one) that exercises the seam from Phase 0 and covers the missing scenarios named in §2.

### Phase 3 — Frontend tests — **DONE 2026-05-15**

Detailed plan: [phase-3-plan.md](phase-3-plan.md). Scope was extended on
2026-05-12 to include Playwright e2e plus a targeted component-extraction
refactor of the largest route files. Original four-item gap-plan minimum
(below) is now a subset of phase-3-plan.md workstream C.

Minimum scope (gap-plan baseline):

1. Add Vitest + `@testing-library/svelte` config and one CI job.
2. Form-validation tests for transaction split balance, budget amount sign, date-picker leap-year.
3. Session-expired redirect smoke test.
4. Component tests for the reconciliation page (the most error-prone UI).

Progress (2026-05-12):

- [x] **Workstream F** — root-level Tauri cleanup. Removed `@tauri-apps/*`
  deps and `"tauri"` script from [package.json](../../package.json); dropped
  Tauri-specific blocks from [vite.config.js](../../vite.config.js). Phase 1
  #8 row in [TODO.md](../../TODO.md) updated to "partial"; full Tauri removal
  remains open. `src-tauri/` directory kept as parity-lookup reference per
  scope decision.
- [x] **Workstream A1** — Vitest + jsdom + `@testing-library/svelte` infra.
  New [vitest.config.ts](../../vitest.config.ts), `src/test/setup.ts`,
  `$app/*` mock stubs, `src/test/sanity.test.ts`. `npm test` green
  (1 file, 2 tests, 1.4s). Vitest pinned to v4 (matches Vite 8); vitest 2
  was incompatible with `@sveltejs/vite-plugin-svelte@7`.
- [x] **Workstream B1** — `$lib/money.ts` extracted. Canonical
  `formatMinorWithScale` and `parseAmountToMinor` (the duplicates from
  `transactions/+page.svelte` and `accounts/[id]/+page.svelte`) moved to
  [src/lib/money.ts](../../src/lib/money.ts); 19 unit tests cover scales 0/2/4/8,
  trim/padding/truncation, explicit plus, rejection cases (commas, letters,
  exponent), and round-trip. Three pages migrated; reconcile's bespoke
  `parseAmount` (comma-stripping, null-on-empty, float-rounding) stays
  documented inline as a known divergence. 21/21 tests green, `npm run check`
  clean, `npm run build` ✓.
- [x] **Workstream B2** — `$lib/dates.ts` extracted. `parseSmartDate` moved
  to [src/lib/dates.ts](../../src/lib/dates.ts); 29 unit tests cover empty /
  `t` / `today` shorthand, ISO with leap-year semantics, separator-delimited
  two-part (M/D auto-swap, 60-day prior-year guess), three-part (2- and 4-
  digit year, `YYYY/MM/DD`), and bare digits (day-only with 2-day cutoff and
  January year-rollback, 3-digit `MDD`, 4-digit `MMDD`). 50/50 tests green
  across `$lib`, `npm run check` clean, `npm run build` ✓.
- [x] **Workstream B3** — `$lib/transactions/split-balance.ts` extracted.
  Pure `sumSplitsInMinor` + `isSplitsBalanced` moved to
  [src/lib/transactions/split-balance.ts](../../src/lib/transactions/split-balance.ts);
  12 unit tests cover empty / balanced / unbalanced / mixed-scale / null
  short-circuit / empty-amount-as-zero. Both copies of `splitsTotalMinor()`
  in `transactions/+page.svelte` and `accounts/[id]/+page.svelte` now resolve
  the account/commodity lookups page-side and delegate the parse-and-sum to
  the pure helper. 62/62 tests green, `npm run check` clean, `npm run build`
  ✓. Limitation pinned by tests: the helper sums raw minor units without
  commodity grouping, so a balanced cross-currency split is not detected —
  acceptable because the backend routes cross-currency through its own
  endpoint ([§1.2.2](v1-gap-plan.md)).
- [x] **Workstream B4** — `$lib/search/fuzzy.ts` extracted. `normalizeName`
  / `fuzzyMatch` / `fuzzyOptions` / `exactMatchByName` deduplicated from
  both `transactions/+page.svelte` and `accounts/[id]/+page.svelte`; 21
  unit tests cover normalization semantics (no accent stripping —
  intentional), empty-query semantics, subsequence matching (in-order
  required), the 30-result cap, and duplicate-name first-match order.
  83/83 tests green, `npm run check` clean, `npm run build` ✓.
- [x] **Workstream B5** — `$lib/reconciliation/state.ts` extracted. Pure
  `deriveReconciliationState` (clearedBalance / difference / needsOffset
  with `null` statement-balance propagation) and `sumCheckedAmounts`
  (generic over txId, sums only checked candidates) moved to
  [src/lib/reconciliation/state.ts](../../src/lib/reconciliation/state.ts);
  13 unit tests cover balanced / one-cent-over / one-cent-under / null
  statement / all-zeros / negative opening / partial check / unknown id /
  all-checked. The reconcile page's `$:` derivation block now goes through
  the pure helper; page-coupled lookups stay page-side. 96/96 tests green,
  `npm run check` clean, `npm run build` ✓.
- [x] **Workstreams B6 + B7** (shipped 2026-05-15) — saved-view filter
  serialization + form-validators module. `$lib/transactions/saved-views.ts`
  with `FilterFormState` ↔ `TransactionFilter` round-trip (18 tests);
  `$lib/forms/validators.ts` with result-typed `validateIsoDate`,
  `validatePositiveAmountMinor`, `validateIntegerInRange`, `combine`, etc.
  (46 tests). Validators wired into the transaction-form and planning-page
  submit paths (previously planning had zero client-side validation).
  160/160 tests, `npm run check` clean, `npm run build` ✓.
- [x] **Workstreams D1 + D2 + D3** (shipped 2026-05-15) — component
  extraction. 10 components carved out of the three largest route files,
  collapsing parent pages 3676 → 2611 LoC (**-29%**): `TransactionRow`,
  `TransactionFilters`, `SavedViewsBar`, `TransactionSplitEditor`,
  `AccountHeader`, `AccountRegister`, `ReportFilters`, `CashflowReport`,
  `CategorySpendReport`, `PayeeTotalsReport`, `InvestmentGainsReport`.
  Side fix: `bulkVoid` / `bulkDelete` race condition where
  `selectedIds.size` was read after `clearSelection()`. Accounts-page split
  editor visually unified onto the shared `<Dialog>` shell (was bespoke
  `dialog-backdrop` markup).
- [x] **Workstreams C1–C6** (shipped 2026-05-15) — 56 component tests
  across 5 files: `TransactionSplitEditor.test.ts` (C3, 10),
  `planning/page.test.ts` (C4, 12), `TransactionFilters.test.ts` (C5, 12),
  `layout.test.ts` (C1+C6, 9), `reconcile/page.test.ts` (C2, 13). Per-spec
  `vi.mock("$lib/api/*")` mocking strategy adopted (resolves the plan's
  Open Question 2). 216/216 tests green, runtime 7.2s.
- [x] **Workstreams A2 + A3 + E1–E6** (shipped 2026-05-15) — Playwright
  harness + CI + 12 e2e specs across 7 files. DB reset via Postgres
  template-DB (not the plan's `/api/v1/test/reset` endpoint — the user
  explicitly chose the snapshot approach to keep test code out of the
  production image). CI split into `web-unit.yml` (always-on,
  ~30s) and `web-e2e.yml` (label-gated, ~5-10 min). Two production-side
  additions to make E4/E5 drivable: a minimal `CrossCurrencyTransferDialog`
  UI for E4, and a canned `apps/api/tests/data/sample.ofx` for E5.
- [x] **Workstream G** (shipped 2026-05-15) — docs.
  [docs/architecture/frontend-testing.md](../architecture/frontend-testing.md)
  added with test pyramid, fixture conventions, common patterns, and the
  per-feature checklist for new components. README "Development" section
  lists `npm test` / `npm run e2e` commands.

**Phase 3 DONE 2026-05-15.** Tally:

| Layer | Files | Tests | Runtime |
|---|---:|---:|---:|
| Vitest unit | 8 | 144 | ~3s |
| Vitest component | 5 | 56 | ~5s |
| Playwright e2e | 7 | 12 | ~3 min on CI |

Parent route files: 3676 → 2611 LoC (-29%). 10 new components in
`$lib/components/`, 5 in `$lib/<feature>/`. Two CI workflows
([web-unit.yml](../../.github/workflows/web-unit.yml),
[web-e2e.yml](../../.github/workflows/web-e2e.yml)).

### Phase 4 — Nice-to-have scope items (3-5 d, optional for v1)

1. Import mapping templates (1.4.1).
2. Persistent payee/category cleanup rules (1.4.2).
3. Account-statement / income-expense report (1.5.1).
4. Print-friendly views (1.5.4).
5. User-facing audit/sessions view (1.1.6).
6. Lifecycle audit rows for account close/reopen (1.2.1).

---

## 4. Suggested ordering

Phase 0 is **done**. Phase 1 is in flight (7 of 8 items shipped — items 1, 2, 3, 4, 5, 6, 7). Phase 2 steps 1, 9, 10, 12 are **done**. Phase 3 is **DONE 2026-05-15** (full scope: 216 Vitest + 12 Playwright tests, parent pages -29% LoC, two CI workflows, frontend-testing.md docs). API CI verdict (post step 1): 233 passed / 1 skipped / 0 failed. Continue:

- **Next:** Phase 2 step 2 (transactions correctness — DB-level locked-range check, bulk atomicity) or Phase 2 step 5 (report cache invalidation on writes — the next-highest correctness risk). Phase 1 #8 (full Tauri removal) is also still on the board, cheap, and unblocks the next round of FE work.
- Phase 2 is where the bulk of correctness risk lives. Step 1 closure means the most-error-prone UI (reconciliation) now has both unit + e2e + frontend test coverage; Phase 3 frontend tests already unblocked confident UI changes. Phase 4 is optional for v1.

Approximate remaining total: **4-8 engineering days** to v1-ready (was 5-10 before Phase 2 step 1 shipped).
