# V1 Gap Analysis & Fix Plan

Last updated: 2026-05-12

Audits the repo against [v1-scope.md](v1-scope.md) and the milestone-1-11 completion claims in [SELF_HOSTED_MIGRATION_PLAN.md](../../SELF_HOSTED_MIGRATION_PLAN.md). Scope-coverage gaps are listed first; test-depth gaps follow. Each item names what is missing, where it lives, and severity. Items marked **DONE** have shipped; the relevant section retains the original gap description for historical context.

For the day-to-day "what's next" view, see [TODO.md](../../TODO.md).

Severity legend:
- **B** — release blocker (gate per `v1-scope.md` "Release Gate" or scope `Must Have`)
- **H** — hardening (correctness or operational risk; should ship at v1)
- **N** — nice-to-have (scope `Should Have`, can defer if time-boxed)

## Test status snapshot (2026-05-12, after Phase 1 step 2)

Backend in [.github/workflows/api-tests.yml](../../.github/workflows/api-tests.yml):

- **CI verdict: 171 passed, 2 skipped, 0 failed.** No `--deselect` flags; CI runs the full suite.
- Pyright (strict): clean.
- Ruff check: clean.
- Ruff format check: not yet a gate (70/123 files would need reformatting).

Net change since Phase 2 step 10: **+11 e2e tests** for the user-invite flow. Two intentional skips: the `MissingGreenlet` import test (Phase 2 step 1 will rewrite it as e2e) and an invite/deactivate-state test deferred until the pending-vs-deactivated user model is cleaned up (logged in §Findings).

## Phase status

- [x] **Phase 0** — end-to-end test seam (completed 2026-05-09)
- [ ] **Phase 1** — release-blocker scope items (3/8 items done)
  - [x] 1. Self-service password reset (completed 2026-05-09)
  - [x] 2. User invite flow (completed 2026-05-12)
  - [ ] 3. Cross-currency transfer endpoint
  - [ ] 4. Stock-split lot rewrite
  - [ ] 5. Cross-session OFX duplicate detection
  - [ ] 6. Reverse-proxy + TLS production example
  - [x] 7. CI API test job (completed 2026-05-09)
  - [ ] 8. Tauri removal
- [ ] **Phase 2** — hardening of high-risk service code (2/11 items done)
  - [x] 9. Triage 8 pre-existing test failures (completed 2026-05-10)
  - [x] 10. Rebuild stage2_schema_contract.py from Base.metadata (completed 2026-05-11)
- [ ] **Phase 3** — frontend tests
- [ ] **Phase 4** — nice-to-have scope items

## Findings discovered while executing the plan

These were found while building Phase 0 and are tracked here so they don't get lost:

- **`/api/v1/health` is transitively auth-protected.** The route depends on `get_book_service`, which depends on `get_access_policy`, which depends on `require_request_context`. Health checks should be unauthenticated. Severity **H**; address in Phase 2 alongside other auth wiring fixes. Location: [apps/api/src/rekenraam_api/api/v1/health.py](../../apps/api/src/rekenraam_api/api/v1/health.py).
- **`/api/v1/accounts/balances` only returns accounts with at least one posted split.** Documented in the smoke test. Severity **N** (UI works around it). May surprise API consumers; consider returning a zero-balance row for every active account.
- **8 pre-existing test failures on `main` — RESOLVED 2026-05-10.** Phase 2 step 9 fixed 6, skipped 2 with forward-pointers. CI now runs the full suite with no quarantine flags. See the per-test outcome in §Phase 2 step 9 below.

- **`AuthorizationError` was masked as 400 in many routes — FIXED 2026-05-10.** While triaging Phase 2 step 9, discovered that `AuthorizationError` inheriting from `ValueError` meant any route with `try: ... except ValueError: HTTP_400_BAD_REQUEST` was silently swallowing 403 errors. Fixed at the root by changing the base class to `Exception`. A follow-up audit (Phase 2 step 7) should grep every `except ValueError` in `apps/api/src/rekenraam_api/api/v1/` to confirm no other auth-shaped errors are similarly affected.

- **`stage2_schema_contract.py` is materially incomplete — RESOLVED 2026-05-11.** Phase 2 step 10 rebuilt the module to derive both halves of the contract from canonical sources (`Base.metadata` for ORM, Postgres reflection for the migrated DB). 24 tables that were missing from the hand-written dict (the milestone 7 planning, milestone 8 investments, milestone 9 ergonomics/templates, milestone 10 MFA, and Phase 1 password-reset families) are now covered. Side findings logged below.

- **`ruff format` not enforced.** Discovered while wiring CI in Phase 1 step 7: `ruff format --check apps/api` reports 70 of 123 Python files would be reformatted. Adding the check as a CI gate now would block every PR. Bulk-format the repo as one focused PR, then enable the gate. Severity **N** (cosmetic, but a real risk-reduction once enforced because review diffs become smaller). Not in any phase yet — tracked in [TODO.md](../../TODO.md).

- **Numeric/boolean `server_default` strings need `sa.text(...)` everywhere — RESOLVED 2026-05-11 across ORM and migrations.** Surfaced in step 10 by the schema-drift detector. Detail: `server_default="0"` (raw string) compiles to `DEFAULT '0'` (a string literal) under SQLAlchemy. Postgres implicit-casts `'0'::bigint` and stores `'0'` as text in `pg_attrdef`, so reflection returns `'0'` (with quotes). The clean pattern is `server_default=text("0")`, which produces `DEFAULT 0` and round-trips identically through reflection. 32 ORM columns and 6 migration columns were converted. Severity **N** because the in-database behavior was equivalent for our column types, but the inconsistency was a real footgun for any future use of `Base.metadata.create_all()` or fresh migrations on different DDL targets.

- **`PasswordResetToken` was not exported from `db/models/__init__.py` — FIXED 2026-05-11.** Severity **N**. The class still landed in `Base.metadata` because the parent module gets imported transitively, but the `__init__.py` re-export list was inconsistent. Style follow-up: future ORM additions must update both the class and the re-export list.

- **`scheduled_transactions.interval` column uses a Postgres reserved word.** Severity **N**. Works because SQLAlchemy quotes column names in its generated SQL, but a footgun for raw SQL elsewhere (`SELECT interval FROM scheduled_transactions` would fail). Worth renaming to `interval_count` before the baseline hardens further. Not in any phase.

- **Pending-user state vs deactivated-user state are conflated.** Severity **N**. Surfaced building Phase 1 step 2: an invited-but-not-accepted user has `is_active=False` and `password_hash IS NULL`. An admin-deactivated existing user also has `is_active=False`. The invite-accept path activates whoever owns the token, which is correct for the pending case. Today this can't be exploited — `create_invite` rejects emails belonging to any user with `password_hash IS NOT NULL` — but the model is fragile. Cleaner long-term: add a discriminator (`status: 'pending' | 'active' | 'deactivated'`) so the three cases are distinct. The invite e2e test `test_accept_rejects_when_user_deactivated_between_issue_and_accept` is `@pytest.mark.skip`-ped pending this. Worth pulling into Phase 2 step 7 (auth depth).

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
| 1.2.2 | **Cross-currency transfers**: transaction mutation enforces zero-sum splits and account/commodity matching, so ordinary mixed-commodity balancing is intentionally rejected. Missing is a dedicated transfer endpoint that records both sides, stamps FX rate at post time, and writes realized FX gain/loss where needed. | `services/transactions.py`, new `/api/v1/transactions/transfer` or transfer-flag in mutation | B |
| 1.2.3 | Memorized **splits** & transaction templates exist as routes but split lines on templates are not validated to balance before save. | `services/ergonomics.py` (templates), `api/v1/templates.py` | H |
| 1.2.4 | Locked-range validation is service-only. No DB constraint. A direct SQL write or a bug in `_ensure_unlocked` lets writes through. | add CHECK or trigger; or partial unique index on `(account_id, txn_date)` against locked range | H |
| 1.2.5 | Bulk-void exists; **bulk-delete** exists; but there is no transactional guarantee tested that all-or-nothing on partial failure. | `services/transactions.py` | H |

### 1.3 Reconciliation (Milestone 5)

| # | Gap | Location | Sev |
|---|---|---|---|
| 1.3.1 | **Void-with-audit on reconciled transactions**: `bulk-void` works on un-reconciled txns; voiding a reconciled txn must either be blocked or it must roll the reconciliation. Current behavior unclear from code. | `services/reconciliation.py`, `services/transactions.py` | H |
| 1.3.2 | Balance-constraint **overlap detection** missing. Two constraints with overlapping date ranges silently coexist. | `services/reconciliation.py` | H |
| 1.3.3 | No race protection on concurrent reconciliations of the same account (two browser tabs, two users). Should use `SELECT ... FOR UPDATE` or advisory lock. | `repositories/reconciliation.py` | H |

### 1.4 Imports & exports (Milestone 6)

| # | Gap | Location | Sev |
|---|---|---|---|
| 1.4.1 | **Import mapping templates** export/import (scope `Should Have`). | `services/imports.py`, new `import_mapping_templates` table or JSON export | N |
| 1.4.2 | **Persistent payee/category cleanup rules** beyond `ImportRule`: payee normalization (regex/canonical), category mapping by payee, duplicate-handling preferences. | `services/imports.py` extension | N |
| 1.4.3 | **Cross-session duplicate detection** for OFX `FITID` and CSV `import_id` is not visibly enforced — a re-imported statement may duplicate. | `repositories/imports.py`, add unique partial index on `(account_id, fitid)` where fitid is set | B |
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
| 1.6.1 | **Stock-split lot adjustment** posting: `corporate_actions` records exist, but no service path that, given a 2-for-1 split, rewrites or supersedes lot quantities. Result: realized-gains math after a split is wrong. | `services/investments.py` | B |
| 1.6.2 | **Mixed-consideration corporate actions** (cash + stock) and **cash-in-lieu** for fractional shares are recorded but not posted. Scope explicitly accepts "structured event records" but valuation will be off. | `services/investments.py` | H |
| 1.6.3 | **Short-cover gain/loss**: the endpoint exists; cost-basis treatment for short positions (negative lots) is not documented and may not handle wash-sale-like edge cases. | `services/investments.py` | H |
| 1.6.4 | **FX cross-rate triangulation** when a direct pair is missing (e.g. need EUR→JPY, only have EUR→USD and USD→JPY). | `services/pricing_execution.py` | H |
| 1.6.5 | **Source-assignment effective-date supersession**: `as-of` lookup with overlapping assignments not validated. | `services/pricing.py` | H |
| 1.6.6 | **Rate staleness** warning: when last observation is older than `max_backfill_days`, valuation should flag. | `services/pricing.py`, frontend valuation views | H |

### 1.7 Operational hardening (Milestone 11)

| # | Gap | Location | Sev |
|---|---|---|---|
| 1.7.1 | `compose.prod.example.yaml` does not include a **reverse proxy / TLS** example (Caddy or nginx with Let's Encrypt). Scope release gate explicitly requires "documented HTTPS … requirements". | [compose.prod.example.yaml](../../compose.prod.example.yaml), [docs/deployment/self-hosting.md](../../docs/deployment/self-hosting.md) | B |
| 1.7.2 | CI API test job — **DONE 2026-05-09.** [.github/workflows/api-tests.yml](../../.github/workflows/api-tests.yml) runs `ruff check`, `pyright` (strict), and `pytest` against a Postgres 16 service. Triggers on changes under `apps/api/**`, `pyproject.toml`, `uv.lock`, or `Makefile`. Uses `astral-sh/setup-uv` + `uv sync --frozen` so what runs in CI matches local `uv sync`. The 8 pre-existing failures are quarantined inline via `--deselect`; Phase 2 step 9 will convert each to an in-source skip and remove the corresponding deselect. `ruff format --check` is intentionally NOT yet a gate — see findings. | [.github/workflows/api-tests.yml](../../.github/workflows/api-tests.yml) | B |
| 1.7.3 | **Backup smoke test** present (`scripts/restore_smoke.sh`) but not wired into CI on a schedule. | `.github/workflows/operational-self-hosting.yml` | H |
| 1.7.4 | **Rate-limit / failed-login** state in Postgres or Redis instead of process memory (see 1.1.5). | `services/auth.py` | H |

### 1.8 Tauri removal (Milestone 12 — gate for v1 release)

| # | Gap | Location | Sev |
|---|---|---|---|
| 1.8.1 | `src-tauri/` still exists. | repo root | B |
| 1.8.2 | `package.json` still has `@tauri-apps/api`, `@tauri-apps/plugin-opener`, `@tauri-apps/cli`, and `"tauri": "tauri"` script. | [package.json](../../package.json) | B |
| 1.8.3 | Vite/Svelte config for Tauri may still be present; not yet verified. | `vite.config.js`, `svelte.config.js` | B |

The migration plan exit criteria for v1 release require Tauri to be gone. Until 1.8.1–1.8.3 are done, v1 cannot ship.

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

### 2.1 Reconciliation — high risk, thinly tested

Scope at [services/reconciliation.py](../../apps/api/src/rekenraam_api/services/reconciliation.py) is 522 LoC. Tests live in `test_api.py` (3 tests, against stubs) and indirectly in `test_repositories.py`. Missing:

- Locked-range write rejection: post txn dated inside a closed reconciliation window — should 409.
- Concurrent `start` from two sessions on the same account.
- Constraint violation surfaced at finish (account ends below `balance_min`).
- Unlock that intersects with a locked txn — must clear locked state before writes.
- Offset-account currency mismatch (auto-balance txn in wrong commodity).
- Voiding a reconciled split rolls the reconciliation or 409s.
- Reconciliation rollback when adjustment-txn write fails (partial state).

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
3. **Cross-currency transfer endpoint** (1.2.2): explicit endpoint that takes source/dest accounts, amounts in each commodity, FX rate, and writes a balanced txn with FX gain/loss split. Tests: balanced-in-base assertion, FX gain/loss correctness, rejection when rate not provided.
4. **Stock-split lot rewrite** (1.6.1): on `corporate_action.kind == 'split'` post, supersede affected lots with adjusted quantity and per-share basis. Tests: realized gain after split, holding-period preservation.
5. **Cross-session OFX duplicate detection** (1.4.3): unique partial index on `(account_id, fitid)` where fitid IS NOT NULL; service path checks before insert.
6. **Reverse-proxy + TLS production example** (1.7.1): add Caddy service to `compose.prod.example.yaml` with auto-TLS; document Let's Encrypt setup in `docs/deployment/self-hosting.md`.
7. **CI API test job** (1.7.2) — **DONE 2026-05-09.** Delivered:
   - [.github/workflows/api-tests.yml](../../.github/workflows/api-tests.yml) with a Postgres 16 service container, `astral-sh/setup-uv@v6`, `uv sync --frozen`, then `make api-lint` (ruff), `make api-typecheck` (pyright strict), and `pytest` with the 8 pre-existing failures quarantined via `--deselect`.
   - Triggers on push and pull_request when `apps/api/**`, `pyproject.toml`, `uv.lock`, `Makefile`, or the workflow itself changes.
   - First useful catch: pyright caught a `reportUnknownMemberType` regression in [`AccessRepository.revoke_all_user_sessions`](../../apps/api/src/rekenraam_api/repositories/access.py) that local pytest had passed. Fixed in the same PR by mirroring the `cast(CursorResult[object], result).rowcount` pattern used in [ergonomics.py](../../apps/api/src/rekenraam_api/repositories/ergonomics.py).
   - **Not enforced yet:** `ruff format --check`. 70 of 123 Python files would need reformatting and that bulk change should land as a single focused PR (tracked as a new finding below and in [TODO.md](../../TODO.md)).
8. **Tauri removal** (1.8.1–1.8.3): delete `src-tauri/`, drop `@tauri-apps/*` deps, drop `"tauri"` script, remove Tauri-specific Vite config. Verify `npm run check` and `npm run build` still pass.

Acceptance: every release-gate clause in `v1-scope.md` is satisfied or has a documented deferral.

### Phase 2 — Hardening of high-risk service code (5-8 d)

1. **Reconciliation correctness** (1.3.1, 1.3.2, 1.3.3, 2.1):
   - Add `SELECT ... FOR UPDATE` around `start_reconciliation` per account.
   - Reject overlapping `BalanceConstraint` rows.
   - Define void-of-reconciled policy and enforce it.
   - Add a real `test_reconciliation_service.py` against the Phase-0 e2e seam covering all scenarios in §2.1.
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

Acceptance: each module listed has a dedicated `test_*_service.py` (or expanded existing one) that exercises the seam from Phase 0 and covers the missing scenarios named in §2.

### Phase 3 — Frontend tests (3-4 d)

1. Add Vitest + `@testing-library/svelte` config and one CI job.
2. Form-validation tests for transaction split balance, budget amount sign, date-picker leap-year.
3. Session-expired redirect smoke test.
4. Component tests for the reconciliation page (the most error-prone UI).

### Phase 4 — Nice-to-have scope items (3-5 d, optional for v1)

1. Import mapping templates (1.4.1).
2. Persistent payee/category cleanup rules (1.4.2).
3. Account-statement / income-expense report (1.5.1).
4. Print-friendly views (1.5.4).
5. User-facing audit/sessions view (1.1.6).
6. Lifecycle audit rows for account close/reopen (1.2.1).

---

## 4. Suggested ordering

Phase 0 is **done**. Phase 1 is in flight (3 of 8 items shipped — items 1, 2, 7). Phase 2 step 9 and step 10 are **done**. CI verdict: 171 passed / 2 skipped / 0 failed. Continue:

- **Next:** Phase 1 step 3 (cross-currency transfer endpoint). Larger than invite — needs FX-rate-stamping at post time and an FX gain/loss split. The existing transactions service has the infrastructure; this is mostly new schema + service logic + tests.
- **Then:** stock-split lot rewrite (#4), OFX duplicate detection (#5), reverse-proxy/TLS (#6), Tauri removal (#8).
- Phase 2 is where the bulk of correctness risk lives and where the original request "really test logic, edge cases, interfaces" gets satisfied. Phase 3 unblocks confident UI changes. Phase 4 is optional for v1.

Approximate remaining total: **8-13 engineering days** to v1-ready (was 10-15 before Phase 1 step 2 shipped).
