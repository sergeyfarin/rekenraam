# V1 Gap Analysis & Fix Plan

Last updated: 2026-05-07

Audits the repo against [v1-scope.md](v1-scope.md) and the milestone-1-11 completion claims in [SELF_HOSTED_MIGRATION_PLAN.md](../../SELF_HOSTED_MIGRATION_PLAN.md). Scope-coverage gaps are listed first; test-depth gaps follow. Each item names what is missing, where it lives, and severity.

Severity legend:
- **B** — release blocker (gate per `v1-scope.md` "Release Gate" or scope `Must Have`)
- **H** — hardening (correctness or operational risk; should ship at v1)
- **N** — nice-to-have (scope `Should Have`, can defer if time-boxed)

---

## 1. Scope coverage gaps

### 1.1 Auth & user lifecycle (Milestone 3 / Milestone 9)

| # | Gap | Location | Sev |
|---|---|---|---|
| 1.1.1 | **Self-service password reset** missing. Only admin reset exists at [admin.py:96](../../apps/api/src/rekenraam_api/api/v1/admin.py#L96). No reset-token model, no `/api/v1/auth/password-reset/{request,confirm}`, no token expiry, no rate limiting. Email delivery is intentionally deferred — but the token-based flow must work via admin-issued or copy-paste link to satisfy "password change/reset path" in scope. | [auth.py](../../apps/api/src/rekenraam_api/api/v1/auth.py), [services/auth.py](../../apps/api/src/rekenraam_api/services/auth.py), new `password_reset_tokens` table | B |
| 1.1.2 | **User invite flow** missing. Only admin POST `/admin/users` with a chosen password exists. No invite token, no first-login password set, no expiry. Scope says "create/invite/deactivate users". | [admin.py](../../apps/api/src/rekenraam_api/api/v1/admin.py), `services/ergonomics.py`, new `user_invites` table | B |
| 1.1.3 | **Explicit deactivate** missing. PUT `/admin/users/{id}` accepts `is_active` but there is no audited deactivate path that revokes sessions. Sessions of a deactivated user keep working until expiry. | `services/ergonomics.py`, `services/auth.py` (session revocation) | H |
| 1.1.4 | MFA recovery code regeneration without disabling MFA missing. `confirm_mfa_setup` is the only generator. | `services/auth.py:227-268` | H |
| 1.1.5 | Failed-login throttling is **in-memory** (`_login_failures` dict). Resets on restart and does not work across multiple API replicas. | `services/auth.py` | H |
| 1.1.6 | Audit-trail visibility for auth events: events are written but no user-facing "your sessions / your activity" view. | new `/api/v1/auth/me/sessions`, `/api/v1/auth/me/activity` | H |

### 1.2 Accounts & transactions (Milestone 4)

| # | Gap | Location | Sev |
|---|---|---|---|
| 1.2.1 | **Account close/reopen as discrete events**: PUT `is_closed` works, but the DB carries `lifecycle_event` ('open' \| 'close' \| 'reopen' \| 'update') that is never emitted as its own audit row. Reopen has no service entry point. Closing-validation endpoint exists but does not block close on uncleared txns. | [accounts.py](../../apps/api/src/rekenraam_api/api/v1/accounts.py), `services/accounts.py:79` | H |
| 1.2.2 | **Cross-currency transfers**: schema fields exist on transactions, but no dedicated transfer endpoint, no FX-rate stamping at post time, no realized FX gain/loss split. Splits with mismatched commodities pass through without validation that book-base totals balance. | `services/transactions.py`, new `/api/v1/transactions/transfer` or transfer-flag in mutation | B |
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
| 1.7.2 | CI exists for migrations + ops smoke + frontend, but **no API test job** runs `pytest`, `ruff`, or `pyright`. | new `.github/workflows/api-tests.yml` | B |
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

### Phase 0 — End-to-end test seam (prerequisite, 2-3 d)

Without this, every other test improvement layers more fakes onto the existing test pyramid that already doesn't catch real bugs.

- Add a `test_e2e_*` fixture set in `apps/api/tests/` that:
  - Boots FastAPI app with **real** services and repositories.
  - Connects to a per-test Postgres schema (use `testcontainers-postgres` or reuse the dev compose DB with txn rollback).
  - Provides factories for users, books, accounts, currencies, instruments.
- Migrate the existing `test_api.py` stub tests into this seam over time; do not rewrite at once.
- Add a CI job that runs the e2e tests against a Postgres service container.

Acceptance: a single test that creates a user → logs in → creates an account → posts a txn → reads register → asserts balance, all through HTTP, all against real DB.

### Phase 1 — Release-blocker scope items (5-7 d)

In order:

1. **Self-service password reset** (1.1.1): token model, two endpoints (`request`, `confirm`), 24h expiry, single-use. Tests: full e2e flow + token reuse rejection + expiry.
2. **User invite** (1.1.2): admin issues invite token; `accept-invite` endpoint sets password and activates user. Tests: e2e + token reuse + expiry.
3. **Cross-currency transfer endpoint** (1.2.2): explicit endpoint that takes source/dest accounts, amounts in each commodity, FX rate, and writes a balanced txn with FX gain/loss split. Tests: balanced-in-base assertion, FX gain/loss correctness, rejection when rate not provided.
4. **Stock-split lot rewrite** (1.6.1): on `corporate_action.kind == 'split'` post, supersede affected lots with adjusted quantity and per-share basis. Tests: realized gain after split, holding-period preservation.
5. **Cross-session OFX duplicate detection** (1.4.3): unique partial index on `(account_id, fitid)` where fitid IS NOT NULL; service path checks before insert.
6. **Reverse-proxy + TLS production example** (1.7.1): add Caddy service to `compose.prod.example.yaml` with auto-TLS; document Let's Encrypt setup in `docs/deployment/self-hosting.md`.
7. **CI API test job** (1.7.2): new `.github/workflows/api-tests.yml` running `ruff check`, `ruff format --check`, `pyright`, `pytest` against a Postgres service.
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
7. **Auth depth** (1.1.3, 1.1.5, 2.7): session revocation on deactivate; persistent throttling state (Postgres table or Redis); session expiry tests; cross-book isolation tests.
8. **CSV export escaping & locale** (1.4.4, 2.8).

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

Run **Phase 0 first**. It is a small investment that makes every subsequent test write payoff much higher — and it is currently the single biggest reason real bugs could ship. Phase 1 is the release-blocker list. Phase 2 is where the bulk of correctness risk lives, and where the request "really test logic, edge cases, interfaces" gets satisfied. Phase 3 unblocks confident UI changes. Phase 4 is optional for v1.

Approximate total: **15-22 engineering days** to v1-ready, of which ~8 days are net-new feature work (Phase 1) and the rest is correctness and tests.
