# TODO

Single dashboard for in-flight Rekenraam V1 work. Links back to the canonical
specs — does **not** duplicate them. Update this file when items move state.

Last updated: 2026-05-10

## Sources of truth

- [SELF_HOSTED_MIGRATION_PLAN.md](SELF_HOSTED_MIGRATION_PLAN.md) — milestone-level roadmap
- [docs/product/v1-scope.md](docs/product/v1-scope.md) — product scope and release gate
- [docs/product/v1-gap-plan.md](docs/product/v1-gap-plan.md) — gap analysis, phase status, fix plan, test-coverage gaps
- [docs/parity/desktop-to-python.md](docs/parity/desktop-to-python.md) — desktop-to-web parity matrix
- [docs/architecture/postgres-schema.md](docs/architecture/postgres-schema.md) — schema direction

## Active focus

**Stabilize-before-features pivot.** Steps 1 and 2 of the original active-focus list are done; the test signal is now trustworthy. CI runs the full suite with no quarantine flags, lint clean, pyright strict clean.

1. [x] **Phase 1 step 7** — Add CI API test job. Shipped 2026-05-09.
2. [x] **Phase 2 step 9** — Triage 8 pre-existing test failures. Shipped 2026-05-10. 6 fixed, 2 explicitly skipped with forward-pointers; CI no longer needs `--deselect` flags. Per-test outcomes: see [v1-gap-plan.md §Phase 2 step 9](docs/product/v1-gap-plan.md).
3. [ ] **Phase 2 step 10** — Rebuild [stage2_schema_contract.py](apps/api/tests/stage2_schema_contract.py) from `Base.metadata` so it can't drift silently. Acceptance: re-enable `test_alembic_head_matches_full_stage2_schema_contract` (currently `@pytest.mark.skip`); deleting any model attribute makes it fail.
4. [ ] **Phase 1 step 2** — User invite flow. Resume after step 3 lands so the schema-drift detector is back online before more migrations ship.

After (4) lands, return to the Phase 1 ordering in [v1-gap-plan.md §Phase 1](docs/product/v1-gap-plan.md).

## Phase status

| Phase | Status | Reference |
|---|---|---|
| Phase 0 — e2e test seam | **DONE** 2026-05-09 | [v1-gap-plan.md §Phase 0](docs/product/v1-gap-plan.md) |
| Phase 1 — release-blocker scope items | 1/8 done; pivoted to stabilize CI first | [v1-gap-plan.md §Phase 1](docs/product/v1-gap-plan.md) |
| Phase 2 — hardening of high-risk service code | not started | [v1-gap-plan.md §Phase 2](docs/product/v1-gap-plan.md) |
| Phase 3 — frontend tests | not started | [v1-gap-plan.md §Phase 3](docs/product/v1-gap-plan.md) |
| Phase 4 — nice-to-have scope items | not started | [v1-gap-plan.md §Phase 4](docs/product/v1-gap-plan.md) |

## Phase 1 — Release blockers

| # | Item | Status | Reference |
|---|---|---|---|
| 1 | Self-service password reset | **DONE** 2026-05-09 | gap-plan §Phase 1, item 1.1.1 |
| 2 | User invite flow | not started — gated on CI + triage | gap-plan §1.1.2, §Phase 1 |
| 3 | Cross-currency transfer endpoint | not started | gap-plan §1.2.2, §Phase 1 |
| 4 | Stock-split lot rewrite | not started | gap-plan §1.6.1, §Phase 1 |
| 5 | Cross-session OFX duplicate detection | not started | gap-plan §1.4.3, §Phase 1 |
| 6 | Reverse-proxy + TLS production example | not started | gap-plan §1.7.1, §Phase 1 |
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
| 10 | Rebuild `stage2_schema_contract.py` from metadata | **#3 in active focus** |
| 11 | Unauth `/api/v1/health` (remove transitive `require_request_context`) | not started |

## Open findings (severity B/H, not yet in a phase)

These came out of Phase 0 / Phase 1 step 1 / Phase 1 step 7 / Phase 2 step 9. Logged in detail in [v1-gap-plan.md §Findings](docs/product/v1-gap-plan.md).

- [ ] **`/api/v1/health` is auth-protected.** Severity H. Phase 2 step 11.
- [ ] **`/api/v1/accounts/balances` excludes zero-balance accounts.** Severity N. Decide: include zero rows or document the contract.
- [ ] **`stage2_schema_contract.py` is materially incomplete.** Severity B. Phase 2 step 10.
- [x] **8 pre-existing failing tests on `main`.** Resolved 2026-05-10 in Phase 2 step 9.
- [ ] **`ruff format` not enforced.** Severity N. 70+ files would need reformatting; bulk-format the repo as a single focused PR, then enable `ruff format --check` in [.github/workflows/api-tests.yml](.github/workflows/api-tests.yml).
- [ ] **`AuthorizationError` was masked as 400 in many routes.** Severity H, **fixed in step 9** by changing the base class from `ValueError` to `Exception`. Still worth a follow-up: audit every `except ValueError` in `apps/api/src/rekenraam_api/api/v1/` to confirm no other auth-shaped errors are similarly swallowed (Phase 2 step 7).
- [ ] **Service-level locked-range import test fails with `MissingGreenlet`.** Severity H, the underlying behavior is correct (verified at HTTP layer), the test wiring isn't. Skipped pending an e2e rewrite as part of Phase 2 step 1.

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

## Decision 2026-05-09 — stabilize before features

**Question:** continue Phase 1 feature work (user invite next) or pivot to fixing the findings?

**Pivot to stabilize** for three reasons:

1. **Silent red main.** The 8 failing tests pre-date Phase 0 and have been invisible because there is no CI for the API suite. Every new feature lands without a real "did this break anything?" check. The cost compounds.
2. **Schema-contract drift is a release-gate signal.** [stage2_schema_contract.py](apps/api/tests/stage2_schema_contract.py) is supposed to catch ORM↔Alembic drift. Right now it doesn't even know about migration-0004 or 0005 tables, so a real drift could ship undetected.
3. **CI + triage is small and unblocks everything.** Estimated 1-2 days for steps 1-3 in the active focus list. After that, every Phase 1 and Phase 2 item gets a proper CI verdict.

The user invite flow (Phase 1 step 2) mirrors password reset's shape, so when we resume, it should be fast.

## Maintenance

- When an item ships, move it to "Done since …" with a date and a one-line description, and link to where the work is documented.
- New findings go under "Open findings" until they're scheduled into a phase, then linked to the phase in the gap plan and removed from this list.
- Don't expand item descriptions here — change the canonical spec instead and let this dashboard stay short.
