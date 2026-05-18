# TODO

Single dashboard for in-flight Rekenraam V1 work. Links back to the canonical
specs — does **not** duplicate them. Update this file when items move state.

Last updated: 2026-05-18

## Sources of truth

- [SELF_HOSTED_MIGRATION_PLAN.md](SELF_HOSTED_MIGRATION_PLAN.md) — milestone-level roadmap
- [docs/product/v1-scope.md](docs/product/v1-scope.md) — product scope and release gate
- [docs/product/v1-gap-plan.md](docs/product/v1-gap-plan.md) — gap analysis, phase status, fix plan, test-coverage gaps
- [docs/product/frontend-parity-plan.md](docs/product/frontend-parity-plan.md) — live frontend parity/refactor tracker
- [docs/parity/desktop-to-python.md](docs/parity/desktop-to-python.md) — desktop-to-web parity matrix
- [docs/architecture/postgres-schema.md](docs/architecture/postgres-schema.md) — schema direction
- [docs/architecture/accounting-foundations.md](docs/architecture/accounting-foundations.md) — **pending decision**: proposed shift to small-business accounting + investments, with new Phase 1.5/1.6/1.7 inserted before remaining Phase 2 work

## Decision 2026-05-12 — v1 stays personal-finance, v2 is small-business

The product target stays the **personal-finance web app** defined in
[v1-scope.md](docs/product/v1-scope.md). Small-business accounting + full
investment bookkeeping is **v2**; the architectural roadmap is at
[docs/architecture/accounting-foundations.md](docs/architecture/accounting-foundations.md)
and is not a v1 gate.

Within v1 we still took the cheap, v2-aligned wins:

- Partial unique index + allow-list CHECK on `accounts.system_role`.
- `price_sources` "Manual" row seeded.
- `UNIQUE(split_id, lot_id)` on `split_lot_allocations`.
- Generic `audit_log` table + SQLAlchemy `before_flush` listener with
  before/after JSONB snapshots and request-context attribution on 21
  business-table classes.

Detail in [v1-gap-plan.md §"v1 accounting-correctness fixes (2026-05-12)"](docs/product/v1-gap-plan.md).
Tests: 197 passed / 2 skipped on real Postgres.

**Remaining v1 work** (existing gap-plan, in priority order):

1. Phase 1 step 8 — Tauri removal.
2. Phase 2 step 11 — unauth `/api/v1/health`.
3. Phase 2 step 5 — report cache invalidation on writes/currency-change.
4. ~~Phase 2 step 1 — reconciliation correctness.~~ **DONE 2026-05-16.**
5. Phase 2 step 3 — investments math test coverage.
6. Phase 2 step 6 — budget/schedule/loan edge cases.
7. Phase 2 step 7 — auth depth (deactivate-revokes-sessions, persistent
   throttling, MFA paths).
8. Phase 2 step 8 — CSV export escaping & locale.
9. ~~Phase 3 — frontend tests.~~ **DONE 2026-05-15.**
10. Phase 4 — nice-to-haves.

## Active focus

**Phase 3 (frontend tests) complete + Phase 2 step 1 (reconciliation correctness) complete.** Detail: [docs/product/phase-3-plan.md](docs/product/phase-3-plan.md), [docs/product/v1-gap-plan.md §Phase 2 step 1](docs/product/v1-gap-plan.md). 216 Vitest tests + 12 Playwright specs + 9 e2e reconciliation tests + 1 e2e imports test = 233 backend / 216 frontend, all green. Phase 1 #8 (Tauri removal) remains partial; full removal stays open.

1. [x] **Phase 1 step 7** — Add CI API test job. Shipped 2026-05-09.
2. [x] **Phase 2 step 9** — Triage 8 pre-existing test failures. Shipped 2026-05-10.
3. [x] **Phase 2 step 10** — Rebuild [stage2_schema_contract.py](apps/api/tests/stage2_schema_contract.py) from `Base.metadata`. Shipped 2026-05-11.
4. [x] **Phase 1 step 2** — User invite flow. Shipped 2026-05-12. Detail: [v1-gap-plan.md §Phase 1 step 2](docs/product/v1-gap-plan.md).
5. [x] **Phase 1 step 3** — Cross-currency transfer endpoint. Shipped 2026-05-10. Detail: [v1-gap-plan.md §Phase 1 step 3](docs/product/v1-gap-plan.md).
6. [x] **Phase 1 step 4** — Stock-split lot rewrite. Shipped 2026-05-10. Detail: [v1-gap-plan.md §Phase 1 step 4](docs/product/v1-gap-plan.md).
7. [x] **Phase 1 step 5** — Cross-session OFX duplicate detection. Shipped 2026-05-11. Detail: [v1-gap-plan.md §Phase 1 step 5](docs/product/v1-gap-plan.md).
8. [x] **Phase 1 step 6** — Reverse-proxy + TLS production example. Shipped 2026-05-11. Detail: [v1-gap-plan.md §Phase 1 step 6](docs/product/v1-gap-plan.md).
9. [x] **Phase 3 workstream F** — Root-level Tauri cleanup. Shipped 2026-05-12. Dropped `@tauri-apps/*` deps and `"tauri"` script from package.json, removed Tauri-specific blocks from vite.config.js. `src-tauri/` kept as parity-lookup reference. Detail: [phase-3-plan.md §Changelog](docs/product/phase-3-plan.md).
10. [x] **Phase 3 workstream A1** — Vitest + jsdom + `@testing-library/svelte` infra. Shipped 2026-05-12. `npm test` green (1 file, 2 tests, 1.4s). Vitest pinned to v4 to match Vite 8. Detail: [phase-3-plan.md §Changelog](docs/product/phase-3-plan.md).
11. [x] **Phase 3 workstream B1** — `$lib/money.ts` extracted with 19 unit tests. Shipped 2026-05-12. Canonical `formatMinorWithScale` + `parseAmountToMinor` deduplicated from two route files; reconcile's bespoke `parseAmount` documented as a known divergence. 21/21 tests green, `npm run check` clean, `npm run build` ✓. Detail: [phase-3-plan.md §Changelog](docs/product/phase-3-plan.md).
12. [x] **Phase 3 workstream B2** — `$lib/dates.ts` extracted with 29 unit tests. Shipped 2026-05-12. Smart-date parser (`t` / `today` / ISO / `M/D` / `M/D/YY` / `YYYY/M/D` / bare digits with day-of-month heuristics) moved to `$lib`. Tests pin previously-undocumented behaviors: 60-day prior-year guess cutoff, noon-anchored 2-day "near future" rule, day-31-in-30-day-month silently clears. 50/50 tests green, `npm run check` clean, `npm run build` ✓. Detail: [phase-3-plan.md §Changelog](docs/product/phase-3-plan.md).
13. [x] **Phase 3 workstream B3** — `$lib/transactions/split-balance.ts` extracted with 12 unit tests. Shipped 2026-05-12. Pure `sumSplitsInMinor` + `isSplitsBalanced` deduplicated from two route files; the page-state lookups (`account_id → commodity → scale`) stay in the pages. Known limitation pinned: helper sums raw minor units without commodity grouping (matches existing UI behavior; cross-currency goes through its own backend endpoint). 62/62 tests green, `npm run check` clean, `npm run build` ✓. Detail: [phase-3-plan.md §Changelog](docs/product/phase-3-plan.md).
14. [x] **Phase 3 workstream B4** — `$lib/search/fuzzy.ts` extracted with 21 unit tests. Shipped 2026-05-12. `normalizeName` / `fuzzyMatch` / `fuzzyOptions` / `exactMatchByName` deduplicated from two route files. Pinned semantics: normalization is case-only (no accent stripping); `fuzzyOptions` hard-caps at 30 results. 83/83 tests green, `npm run check` clean, `npm run build` ✓. Detail: [phase-3-plan.md §Changelog](docs/product/phase-3-plan.md).
15. [x] **Phase 3 workstream B5** — `$lib/reconciliation/state.ts` extracted with 13 unit tests. Shipped 2026-05-12. Pure `deriveReconciliationState` + `sumCheckedAmounts` cover the cleared/difference/needsOffset derivation that the reconcile page's `$:` block was inlining. `null` statement-balance now propagates explicitly through the helper (matches existing semantics). 96/96 tests green, `npm run check` clean, `npm run build` ✓. Detail: [phase-3-plan.md §Changelog](docs/product/phase-3-plan.md).
16. [x] **Phase 3 workstreams B6 + B7** — Shipped 2026-05-15. `$lib/transactions/saved-views.ts` (18 tests) — round-trippable `FilterFormState` ↔ `TransactionFilter`; `$lib/forms/validators.ts` (46 tests) — result-typed validators (`validateIsoDate`, `validatePositiveAmountMinor`, `validateIntegerInRange`, `combine`, etc.) wired into `transactions/+page.svelte`, `accounts/[id]/+page.svelte`, and `planning/+page.svelte`. 160/160 tests. Detail: [phase-3-plan.md §Changelog](docs/product/phase-3-plan.md).
17. [x] **Phase 3 workstreams D1–D3** — Shipped 2026-05-15. Carved 10 components out of the three largest route files (parent pages 3676 → 2611 LoC, **-29%**): `TransactionSplitEditor`, `TransactionFilters`, `SavedViewsBar`, `TransactionRow`, `AccountHeader`, `AccountRegister`, `ReportFilters`, `CashflowReport`, `CategorySpendReport`, `PayeeTotalsReport`, `InvestmentGainsReport`. Side fix: `bulkVoid` / `bulkDelete` race where `selectedIds.size` was read after `clearSelection()`. Detail: [phase-3-plan.md §Changelog](docs/product/phase-3-plan.md).
18. [x] **Phase 3 workstreams C1–C6** — Shipped 2026-05-15. 56 component tests across 5 files: `TransactionSplitEditor.test.ts` (C3, 10), `planning/page.test.ts` (C4, 12), `TransactionFilters.test.ts` (C5, 12), `layout.test.ts` (C1+C6, 9), `reconcile/page.test.ts` (C2, 13). Per-spec `vi.mock("$lib/api/...")` mocking strategy adopted (resolves plan's Open Question 2). 216/216 tests, runtime 7.2s. Detail: [phase-3-plan.md §Changelog](docs/product/phase-3-plan.md).
19. [x] **Phase 3 workstreams A2 + A3 + E1–E6** — Shipped 2026-05-15. Playwright harness in [e2e/](e2e/) with template-DB-based per-spec reset fixture; 12 specs across 7 files covering auth, transactions, reconcile, cross-currency transfer, OFX import, and reports. Two CI workflows: `web-unit.yml` (always-on, ~30s) and `web-e2e.yml` (label-gated via `run-e2e` label, ~5-10 min). Production-side additions to make E4/E5 drivable: minimal `CrossCurrencyTransferDialog.svelte` UI + canned `apps/api/tests/data/sample.ofx`. Detail: [phase-3-plan.md §Changelog](docs/product/phase-3-plan.md).
20. [x] **Phase 3 workstream G** — Shipped 2026-05-15. New [docs/architecture/frontend-testing.md](docs/architecture/frontend-testing.md) (test pyramid, fixture conventions, common patterns, when-to-use-which guidance). README "Development" section now lists `npm test` / `npm run e2e` commands; cross-currency transfer + Phase 3 test infra added to "Working today".
21. [x] **Phase 2 step 1 — Reconciliation correctness** — Shipped 2026-05-16. TDD-style: wrote 9 e2e tests in [apps/api/tests/e2e/test_reconciliation.py](apps/api/tests/e2e/test_reconciliation.py) covering §2.1 scenarios (locked-range writes, concurrent finishes, constraint validation, unlock-then-post, currency-mismatch offset, void-of-reconciled, partial-state rollback, happy path) plus the two new invariants (1.3.2 + 1.3.3). Fixes: **1.3.2** — `ReconciliationService.create_constraint` now rejects a second constraint per (book_id, account_id) with a 409 conflict (was silent coexistence). **1.3.3** — `pg_advisory_xact_lock` on (namespace, account_id) at the top of `start` and `finish` serializes concurrent reconciliations on the same account; released at txn commit/rollback. **1.3.1** — verified: existing `_ensure_unlocked` check already blocks void of a reconciled txn (the locked-range mechanism subsumes it; no policy change needed). Side fix: the deferred `test_import_commit_marks_session_abandoned_on_locked_account` un-skipped after fixing the underlying `MissingGreenlet` bug in `services/imports.py` — captured `session.id` to a local before the `db_session.commit()` so the post-rollback abandonment path no longer triggers a lazy reload. 233/1 passed/skipped (was 222/2 before). Detail: [v1-gap-plan.md §Phase 2](docs/product/v1-gap-plan.md).

Next: continue the frontend parity/refactor pass in [docs/product/frontend-parity-plan.md](docs/product/frontend-parity-plan.md), Phase 2 step 2 (transactions correctness — DB-level locked-range + bulk atomicity), Phase 2 step 5 (report cache invalidation on writes), or Phase 1 #8 (full Tauri removal).

Frontend parity/refactor started 2026-05-18:

- [x] Typed `ApiError` boundary + `formatApiError` convention.
- [x] Active-book context foundation.
- [x] Password reset and invite acceptance browser routes.
- [x] Admin invite issuance and prompt-free admin password reset UI.
- [x] Profile display-name/password controls.
- [x] First active-book/error-management adoption in planning and import/export.
- [x] Loan payment draft/post client + planning-page assistant.
- [ ] Split `CommoditySettings.svelte`, `investments/+page.svelte`, `reports/+page.svelte`, and `import-export/+page.svelte` before adding further surface area.
- [ ] Finish removing `book_id: 1` from remaining pages/helpers.

## Phase status

| Phase | Status | Reference |
|---|---|---|
| Phase 0 — e2e test seam | **DONE** 2026-05-09 | [v1-gap-plan.md §Phase 0](docs/product/v1-gap-plan.md) |
| Phase 1 — release-blocker scope items | 7/8 done | [v1-gap-plan.md §Phase 1](docs/product/v1-gap-plan.md) |
| Phase 2 — hardening of high-risk service code | not started | [v1-gap-plan.md §Phase 2](docs/product/v1-gap-plan.md) |
| Phase 3 — frontend tests | **DONE** 2026-05-15 — all workstreams (F + A1 + B1–B7 + C1–C6 + D1–D3 + A2 + A3 + E1–E6 + G) shipped. 216 Vitest tests + 12 Playwright specs; parent pages -29% LoC; 10 carved components in `$lib/components/`; `web-unit.yml` + `web-e2e.yml` workflows. Detail: [phase-3-plan.md](docs/product/phase-3-plan.md). | [v1-gap-plan.md §Phase 3](docs/product/v1-gap-plan.md) |
| Phase 4 — nice-to-have scope items | not started | [v1-gap-plan.md §Phase 4](docs/product/v1-gap-plan.md) |

## Phase 1 — Release blockers

| # | Item | Status | Reference |
|---|---|---|---|
| 1 | Self-service password reset | **DONE** 2026-05-09 | gap-plan §Phase 1, item 1.1.1 |
| 2 | User invite flow | **DONE 2026-05-12** | gap-plan §1.1.2, §Phase 1 |
| 3 | Cross-currency transfer endpoint | **DONE 2026-05-10** | gap-plan §1.2.2, §Phase 1 |
| 4 | Stock-split lot rewrite | **DONE 2026-05-10** | gap-plan §1.6.1, §Phase 1 |
| 5 | Cross-session OFX duplicate detection | **DONE 2026-05-11** | gap-plan §1.4.3, §Phase 1 |
| 6 | Reverse-proxy + TLS production example | **DONE 2026-05-11** | gap-plan §1.7.1, §Phase 1 |
| 7 | CI API test job | **DONE 2026-05-09** | gap-plan §1.7.2, §Phase 1 |
| 8 | Tauri removal | partial — root-level cleanup done 2026-05-12 (Phase 3 workstream F): dropped `@tauri-apps/*` deps, `"tauri"` script, and Tauri-specific blocks from [vite.config.js](vite.config.js). `src-tauri/` directory kept intentionally as parity-lookup reference; full removal remains open. | gap-plan §1.8, migration-plan Milestone 12 |

## Phase 2 — Hardening (high-risk correctness)

Reference for full detail: [v1-gap-plan.md §Phase 2](docs/product/v1-gap-plan.md).

| # | Item | Status |
|---|---|---|
| 1 | Reconciliation correctness (locks, constraints, void-of-reconciled, race) | **DONE 2026-05-16** — 1.3.2 (constraint singleton + 409), 1.3.3 (pg advisory lock per account), 1.3.1 verified (already enforced via `_ensure_unlocked`); 9 e2e tests in `test_reconciliation.py` covering §2.1; deferred `test_import_commit_marks_session_abandoned_on_locked_account` un-skipped after fixing the underlying `MissingGreenlet` bug in `services/imports.py`. |
| 2 | Transactions correctness (DB-level locked-range, bulk atomicity) | not started |
| 3 | Investments math (cost-basis, splits, DRIP, short cover, multi-currency) | not started |
| 4 | Pricing supersession & cross-rate triangulation | not started |
| 5 | Report cache invalidation on writes/currency-change | not started |
| 6 | Budget rollover, schedule recurrence, loan amortization edge cases | not started |
| 7 | Auth depth (deactivate-revokes-sessions, persistent throttling, MFA paths) | partial — `AuthorizationError` no longer inherits `ValueError` (fixed 2026-05-10 in step 9), so 403s aren't masked as 400s. Remaining work as listed in gap plan. |
| 8 | CSV export escaping & locale | not started |
| 9 | Triage 8 pre-existing test failures | **DONE 2026-05-10** |
| 10 | Rebuild `stage2_schema_contract.py` from metadata | **DONE 2026-05-11** |
| 11 | Unauth `/api/v1/health` (remove transitive `require_request_context`) | not started |
| 12 | Append-only audit trail for reference tables (payees, categories, commodities, price_observations, lots, corporate_actions) | **DONE 2026-05-12** via generic `audit_log` table + SQLAlchemy `before_flush` listener with JSONB before/after snapshots + request-context attribution; `import_rules` chain fixed earlier; splits versioning verified intact. Trigger-based DB-level enforcement deferred to v2 (see accounting-foundations.md). |

## Open findings (severity B/H, not yet in a phase)

These came out of Phase 0 / Phase 1 step 1 / Phase 1 step 7 / Phase 2 step 9 / Phase 2 step 10. Logged in detail in [v1-gap-plan.md §Findings](docs/product/v1-gap-plan.md).

- [ ] **`/api/v1/health` is auth-protected.** Severity H. Phase 2 step 11.
- [ ] **`/api/v1/accounts/balances` excludes zero-balance accounts.** Severity N. Decide: include zero rows or document the contract.
- [x] **`stage2_schema_contract.py` is materially incomplete.** Resolved 2026-05-11 in Phase 2 step 10.
- [x] **8 pre-existing failing tests on `main`.** Resolved 2026-05-10 in Phase 2 step 9.
- [x] **`ruff format` not enforced.** Resolved 2026-05-10: bulk-formatted `apps/api`, added `make api-format-check`, and enabled `ruff format --check` in [.github/workflows/api-tests.yml](.github/workflows/api-tests.yml).
- [ ] **`AuthorizationError` was masked as 400 in many routes.** Severity H, **fixed in step 9** by changing the base class from `ValueError` to `Exception`. Still worth a follow-up: audit every `except ValueError` in `apps/api/src/rekenraam_api/api/v1/` to confirm no other auth-shaped errors are similarly swallowed (Phase 2 step 7).
- [ ] **Service-level locked-range import test fails with `MissingGreenlet`.** Severity H, the underlying behavior is correct (verified at HTTP layer), the test wiring isn't. Skipped pending an e2e rewrite as part of Phase 2 step 1.
- [ ] **Pending-user state vs deactivated-user state are conflated.** Severity N. Surfaced building Phase 1 step 2: an invited-but-not-accepted user has `is_active=False` and `password_hash IS NULL`. An admin-deactivated existing user also has `is_active=False`. Right now the invite-accept path activates whoever owns the token, which is correct for the pending case but means a re-issued invite to a previously-active-then-deactivated user could be (mis)used to revive them. Today this can't actually happen — `create_invite` rejects accepted users (any user with `password_hash IS NOT NULL`) — but the model is fragile. Cleaner long-term: add a discriminator (`status: 'pending' | 'active' | 'deactivated'`) so the three cases are distinct. One invite test (`test_accept_rejects_when_user_deactivated_between_issue_and_accept`) is `@pytest.mark.skip`-ped pending this.
- [ ] **Numeric/boolean `server_default` strings need `sa.text(...)` everywhere.** Severity N. Fixed for all 32 ORM columns + 6 migration columns in step 10 (the schema-drift detector caught them). Future migrations and ORM changes must follow the same pattern; consider a CI lint or a code review checklist item. Background: SQLAlchemy auto-quotes `server_default="0"` to `DEFAULT '0'`, which Postgres implicit-casts and stores as the text `'0'`. The cleaner pattern is `server_default=text("0")` — generates `DEFAULT 0` directly.
- [ ] **`scheduled_transactions.interval` column is named after a Postgres reserved word.** Severity N. Works because SQLAlchemy quotes column names, but a footgun for raw SQL. Worth renaming to `interval_count` before the baseline hardens further.
- [ ] **`PasswordResetToken` was missing from `db/models/__init__.py` re-export list.** Severity N, fixed in step 10. The class still landed in `Base.metadata` because the parent module gets imported transitively, but the missing entry was inconsistent. Future ORM additions should always be added to the `__init__.py` re-exports too.

## Open hardening items from milestone roadmap not yet in the gap plan

Not all of [SELF_HOSTED_MIGRATION_PLAN.md](SELF_HOSTED_MIGRATION_PLAN.md)'s "Remaining hardening" entries are tracked in the gap plan. Pulling them through here so they aren't dropped:

- [ ] **Post-V1 database engine migration tooling** — build and test
  bidirectional PostgreSQL ↔ SQLite export/import for existing deployments.
  V1 supports fresh SQLite deployments and continued PostgreSQL deployments,
  but not switching live data between engines.
- [ ] **Milestone 4** — broaden parity fixtures for balances, splits, voided, revisions, locked ranges, high-volume registers; document append-only/versioned accounting model; advanced register search/saved views; memorized splits/templates UX. (Some overlap with Phase 2 step 2.)
- [ ] **Milestone 5** — broader statement fixtures and account-type coverage in reconciliation; expand unlock/void policy tests. (Overlaps Phase 2 step 1.)
- [ ] **Milestone 7** — recurrence coverage for end-of-month, yearly, edited future occurrences; richer non-USD commodity support in planning; loan workflows beyond fixed-rate monthly. (Overlaps Phase 2 step 6.)
- [ ] **Milestone 8** — multi-currency brokerage fixtures (quote/account/book all differ); derivative lifecycle posting; mixed-consideration mergers and cash-in-lieu; private-investment audit references; print-friendly investment exports. (Overlaps Phase 1 #4 and Phase 2 #3.)
- [ ] **Milestone 9** — saved-view/template polish; richer audit coverage for every write family. Mostly **N**.

## Done since 2026-05-07

- 2026-05-09 — Phase 0: end-to-end test seam ([v1-gap-plan.md §Phase 0](docs/product/v1-gap-plan.md)).
- 2026-05-09 — Phase 1 step 1: self-service password reset ([v1-gap-plan.md §Phase 1 step 1](docs/product/v1-gap-plan.md)).
- 2026-05-09 — Phase 1 step 7: CI API test job at [.github/workflows/api-tests.yml](.github/workflows/api-tests.yml). Pyright caught a strict-mode bug in `revoke_all_user_sessions`'s rowcount handling that the local pytest run missed; fixed in the same PR.
- 2026-05-10 — Phase 2 step 9: triaged 8 pre-existing test failures. 6 fixed, 2 explicitly skipped with forward-pointers; deselect list pruned from CI. Side fix: `AuthorizationError` no longer inherits `ValueError`, which was systemically masking 403s as 400s across many routes ([v1-gap-plan.md §Phase 2 step 9](docs/product/v1-gap-plan.md)).
- 2026-05-10 — Phase 1 step 3: shipped cross-currency transfer endpoint. New `POST /api/v1/transactions/transfer` takes source/destination accounts, source/destination amounts, and an explicit FX rate; posts both currency legs, stamps the transfer-date manual FX observation, and writes an optional realized FX gain/loss split when the paid source amount differs from the rate-implied amount ([v1-gap-plan.md §Phase 1 step 3](docs/product/v1-gap-plan.md)).
- 2026-05-10 — Phase 1 step 4: shipped stock-split lot rewrite. Split and reverse-split corporate actions now generate lot-closing/replacement transactions that preserve holding period and remaining cost basis; generated close allocations are excluded from realized gains ([v1-gap-plan.md §Phase 1 step 4](docs/product/v1-gap-plan.md)).
- 2026-05-10 — Resolved the `ruff format` finding: ran a focused `ruff format apps/api` pass, added `make api-format-check`, and enabled the new check in API CI.
- 2026-05-11 — Phase 1 step 5: shipped cross-session OFX duplicate detection. Import commits now consume account-scoped OFX `FITID`/CSV `import_id` keys in Postgres and validate re-imports against the existing transaction ([v1-gap-plan.md §Phase 1 step 5](docs/product/v1-gap-plan.md)).
- 2026-05-11 — Phase 1 step 6: shipped the reverse-proxy + TLS production example. `compose.prod.example.yaml` now includes Caddy with automatic HTTPS, persistent ACME state, public `80`/`443`, and local-only direct frontend binding by default ([v1-gap-plan.md §Phase 1 step 6](docs/product/v1-gap-plan.md)).
- 2026-05-11 — Phase 2 step 10: rebuilt `apps/api/tests/stage2_schema_contract.py` from `Base.metadata`. Dropped the 800-line hand-written `STAGE2_SCHEMA_CONTRACT` (was missing 24 of 57 tables and silently drifting). Added `server_default` as a tracked dimension; fixed 32 ORM columns and 6 migration columns to use `sa.text(...)` for non-string defaults. Added 12-test self-test suite in [test_schema_contract.py](apps/api/tests/test_schema_contract.py) so the detector itself can't silently regress. Re-enabled the migration-vs-ORM drift test ([v1-gap-plan.md §Phase 2 step 10](docs/product/v1-gap-plan.md)).
- 2026-05-12 — Phase 1 step 2: shipped admin-issued user invite flow. New `user_invites` table in the baseline schema, `UserInvite` ORM model, `ErgonomicsService.create_invite` (admin-only, idempotent for re-invites of pending users) and `AuthService.accept_invite` (public, single-use token, anti-enumeration error message), `POST /api/v1/admin/invites` and `POST /api/v1/auth/invite/accept` endpoints. 11 e2e tests cover the full lifecycle ([v1-gap-plan.md §Phase 1 step 2](docs/product/v1-gap-plan.md)).
- 2026-05-12 — v1 accounting-correctness fixes: partial unique index + allow-list CHECK on `accounts.system_role`; seeded `price_sources` "Manual" row; `UNIQUE(split_id, lot_id)` on `split_lot_allocations`; new `audit_log` table + SQLAlchemy `before_flush` listener emitting JSONB before/after snapshots for every mutation on 21 business-table classes; full attribution from `RequestContext`. Schema-contract test green; full suite 197 passed / 2 skipped on Postgres. Resolves Phase 2 step 12. Detail: [v1-gap-plan.md §"v1 accounting-correctness fixes (2026-05-12)"](docs/product/v1-gap-plan.md). v2 will replace the listener with Postgres triggers (see [accounting-foundations.md](docs/architecture/accounting-foundations.md)).

## Decision 2026-05-09 — stabilize before features

**Question:** continue Phase 1 feature work (user invite next) or pivot to fixing the findings?

**Pivot to stabilize** for three reasons:

1. **Silent red main.** The 8 failing tests pre-date Phase 0 and have been invisible because there is no CI for the API suite. Every new feature lands without a real "did this break anything?" check. The cost compounds.
2. **Schema-contract drift is a release-gate signal.** [stage2_schema_contract.py](apps/api/tests/stage2_schema_contract.py) catches ORM↔Alembic drift and should remain a required CI gate now that the baseline schema is consolidated.
3. **CI + triage is small and unblocks everything.** Estimated 1-2 days for steps 1-3 in the active focus list. After that, every Phase 1 and Phase 2 item gets a proper CI verdict.

The user invite flow (Phase 1 step 2) mirrors password reset's shape, so when we resume, it should be fast.

## Maintenance

- When an item ships, move it to "Done since …" with a date and a one-line description, and link to where the work is documented.
- New findings go under "Open findings" until they're scheduled into a phase, then linked to the phase in the gap plan and removed from this list.
- Don't expand item descriptions here — change the canonical spec instead and let this dashboard stay short.
