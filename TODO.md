# TODO

Single dashboard for in-flight Rekenraam V1 work. Links back to the canonical
specs — does **not** duplicate them. Update this file when items move state.

Last updated: 2026-05-12

## Sources of truth

- [SELF_HOSTED_MIGRATION_PLAN.md](SELF_HOSTED_MIGRATION_PLAN.md) — milestone-level roadmap
- [docs/product/v1-scope.md](docs/product/v1-scope.md) — product scope and release gate
- [docs/product/v1-gap-plan.md](docs/product/v1-gap-plan.md) — gap analysis, phase status, fix plan, test-coverage gaps
- [docs/parity/desktop-to-python.md](docs/parity/desktop-to-python.md) — desktop-to-web parity matrix
- [docs/architecture/postgres-schema.md](docs/architecture/postgres-schema.md) — schema direction
- [docs/architecture/accounting-foundations.md](docs/architecture/accounting-foundations.md) — **pending decision**: proposed shift to small-business accounting + investments, with new Phase 1.5/1.6/1.7 inserted before remaining Phase 2 work

## Pending decision (2026-05-12)

The product target is up for review: **personal-finance web app** (current
[v1-scope.md](docs/product/v1-scope.md)) vs **small-business accounting
with full investment support**
([docs/architecture/accounting-foundations.md](docs/architecture/accounting-foundations.md)).

Until decided, the plan below stands as written. If the accounting-
foundations doc is adopted, the active focus changes to:

1. Finish Phase 1 step 8 (Tauri removal) — unblocked either way.
2. Start Phase 1.5: F1 (DB-enforced audit log + no hard deletes) → F3
   (reconciled immutability + corrective entries) → F2 (master-data version
   chains) → F4 (report input snapshots). ~3 weeks.
3. Phase 1.6 (trial balance + balance sheet + income statement). ~1 week.
4. Phase 1.7 (period close + year-end roll-forward). ~3–5 days.
5. Slimmed Phase 2 (current steps 1, 3, 6, 7, 8, 11 remain; 2 and 12 are
   subsumed; 5 simplifies).

See §9 of the foundations doc for the full merge.

## Active focus

**Phase 1 feature work resumed.** Stabilization is done; invite flow shipped.

1. [x] **Phase 1 step 7** — Add CI API test job. Shipped 2026-05-09.
2. [x] **Phase 2 step 9** — Triage 8 pre-existing test failures. Shipped 2026-05-10.
3. [x] **Phase 2 step 10** — Rebuild [stage2_schema_contract.py](apps/api/tests/stage2_schema_contract.py) from `Base.metadata`. Shipped 2026-05-11.
4. [x] **Phase 1 step 2** — User invite flow. Shipped 2026-05-12. Detail: [v1-gap-plan.md §Phase 1 step 2](docs/product/v1-gap-plan.md).
5. [x] **Phase 1 step 3** — Cross-currency transfer endpoint. Shipped 2026-05-10. Detail: [v1-gap-plan.md §Phase 1 step 3](docs/product/v1-gap-plan.md).
6. [x] **Phase 1 step 4** — Stock-split lot rewrite. Shipped 2026-05-10. Detail: [v1-gap-plan.md §Phase 1 step 4](docs/product/v1-gap-plan.md).
7. [x] **Phase 1 step 5** — Cross-session OFX duplicate detection. Shipped 2026-05-11. Detail: [v1-gap-plan.md §Phase 1 step 5](docs/product/v1-gap-plan.md).
8. [x] **Phase 1 step 6** — Reverse-proxy + TLS production example. Shipped 2026-05-11. Detail: [v1-gap-plan.md §Phase 1 step 6](docs/product/v1-gap-plan.md).

Next, return to the Phase 1 ordering in [v1-gap-plan.md §Phase 1](docs/product/v1-gap-plan.md): Tauri removal (#8).

## Phase status

| Phase | Status | Reference |
|---|---|---|
| Phase 0 — e2e test seam | **DONE** 2026-05-09 | [v1-gap-plan.md §Phase 0](docs/product/v1-gap-plan.md) |
| Phase 1 — release-blocker scope items | 7/8 done | [v1-gap-plan.md §Phase 1](docs/product/v1-gap-plan.md) |
| Phase 2 — hardening of high-risk service code | not started | [v1-gap-plan.md §Phase 2](docs/product/v1-gap-plan.md) |
| Phase 3 — frontend tests | not started | [v1-gap-plan.md §Phase 3](docs/product/v1-gap-plan.md) |
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
| 8 | Tauri removal | not started — requires Phase 0 e2e + parity sign-off | gap-plan §1.8, migration-plan Milestone 12 |

## Phase 2 — Hardening (high-risk correctness)

Reference for full detail: [v1-gap-plan.md §Phase 2](docs/product/v1-gap-plan.md).

| # | Item | Status |
|---|---|---|
| 1 | Reconciliation correctness (locks, constraints, void-of-reconciled, race) | not started |
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
| 12 | Append-only audit trail for reference tables (payees, categories, commodities, price_observations, lots, corporate_actions) | partial — `import_rules` chain fixed 2026-05-12; splits versioning verified intact; remainder open |

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
