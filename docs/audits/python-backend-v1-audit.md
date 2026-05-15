# Python Backend V1 Audit

Last updated: 2026-05-15

## Summary

This audit reviews the Python backend against the Personal V1 target in
`docs/product/v1-scope.md`. Small-business accounting and advanced investment
capabilities are treated as future-readiness gaps unless they directly threaten
Personal V1 correctness.

Current backend shape:

- FastAPI API under `/api/v1`
- SQLAlchemy async repositories and services
- PostgreSQL/Alembic schema baseline
- Pydantic request/response contracts
- Docker Compose deployment for local, LAN, and VPS use

Verification snapshot after this audit pass:

- `uv run pytest --collect-only -q`: 224 tests collected
- `uv run pytest -q --cov=rekenraam_api --cov-report=term-missing --cov-report=xml`:
  128 passed, 96 skipped, 45% package coverage in the no-Postgres local path
- `make api-test-postgres`: 23 passed
- `make api-migrate-smoke`: 2 passed
- `make api-test-postgres-coverage`: 25 passed, 85% coverage for the
  repository/migration slice under Docker
- `uv run ruff check .`: pass
- `uv run ruff format --check .`: pass
- `uv run pyright`: pass

## Module Matrix

| Area | Reviewed surface | Test confidence | Notes |
| --- | --- | --- | --- |
| config/app/dependencies/request context | `settings.py`, `app.py`, `dependencies.py`, `request_context.py` | Medium | Settings parsing now has extra coverage; app lifespan and dependency wiring remain lightly covered. |
| auth/access/admin | auth routes, session service, access repository, admin service | Medium | Core bootstrap/login/reset/invite tests exist; MFA enforcement semantics and admin runtime paths need deeper negative coverage. |
| books/accounts/transactions/reconciliation | routes, services, repositories, models | Medium-high | Core account/transaction/repository paths are covered; locked-period and validation tests improved in this pass. |
| metadata/preferences/templates/notes/search | metadata, ergonomics, preferences, saved views, templates, notes | Low-medium | Many API routes have low coverage and hard-delete or mutable-reference semantics. |
| imports/exports | import parsers/rules/commit, export service | Medium | Several parser and duplicate tests exist; malformed file, idempotency, and export edge cases need expansion. |
| reports/report invalidation | reports service/repository/state | Medium | Cache invalidation is tested, but frozen valuation snapshots are schema-implied and not implemented. |
| investments/pricing/pricing worker | investments, lots, corporate actions, price observations, scheduler | Medium | Lots of domain tests exist; repository coverage is low without Postgres and price correction semantics remain weak. |
| planning | budgets, schedules, loans | Low | Routes/services exist but coverage and audit semantics lag behind transaction/account paths. |
| database models/Alembic/audit listener | models, migration contract, audit listener | Medium | Migration smoke and schema contract tests exist; audit listener is SQLAlchemy-level, not database-enforced. |
| deployment/scripts/docs | Compose, prod override, backup/restore docs | Medium-high | Added no-host-port test override and deployment config tests; full restore smoke remains operational rather than unit-tested. |

## Findings Backlog

### V1 Blockers

No new critical Personal V1 blocker was proven in this pass. The items below
are V1 hardening or roadmap gaps unless product scope changes.

### V1 Hardening

| ID | Severity | Finding | Evidence | Recommendation | Acceptance criteria |
| --- | --- | --- | --- | --- | --- |
| AUD-001 | High | `MFA_ENFORCED=true` does not force MFA for users without confirmed TOTP. | `services/auth.py::_mfa_required_for_user` returns false when no confirmed TOTP exists, even if `mfa_enforced` is true. Public VPS docs recommend enforced MFA. | Add an enforced-MFA setup/lockout policy: either require setup before public deployment, force setup after password login, or deny login until an admin disables enforcement. | Tests cover enforced MFA with confirmed TOTP, no TOTP, recovery code, and admin reset. Docs explain the exact setup flow. |
| AUD-002 | High | Hard deletes remain for user-visible financial/reference data. | `session.delete(...)` appears in metadata, pricing, planning, reconciliation constraints, saved views/templates/notes, and split replacement paths. | Classify each delete as ephemeral cleanup, soft delete, version-chain mutation, or audit-log-only delete. Convert financial/reference delete paths to recoverable state changes where V1 history matters. | Every edit/delete on payees, categories, commodities, price observations, lots, corporate actions, budgets, schedules, and loans leaves recoverable state in the database. |
| AUD-003 | High | Audit capture is SQLAlchemy-session-level, not database-enforced. | `db/audit_listener.py` documents that direct SQL writes bypass the listener. | Keep SQLAlchemy listener for V1, but document direct-SQL limitation in admin docs and plan trigger/constraint enforcement for post-V1. | Admin docs warn against direct writes; future migration plan includes trigger-backed audit or DB constraints for accounting tables. |
| AUD-004 | Medium | Default local test path skips Postgres-heavy tests, which can hide repository regressions. | Full local coverage run showed 96 skips; `tests/conftest.py` skips when Postgres is unavailable. | Use the Docker-backed targets for repository/migration coverage in CI and release checks. Keep `compose.test.yaml` to avoid host port conflicts. | CI and release checklist include `make api-test-postgres` and `make api-migrate-smoke`; skipped local runs are treated as partial confidence only. |
| AUD-005 | Medium | Overall no-Postgres package coverage is low and uneven. | Coverage baseline is 45%; low areas include planning, admin, ergonomics, export, reconciliation, worker, and many route modules. | Raise coverage through scenario tests in weak modules before setting a high gate. Start with an informational CI artifact, then add a floor once stable. | Coverage report is published in CI; a documented floor is added after weak modules have meaningful scenario coverage. |
| AUD-006 | Medium | API route tests rely heavily on stubs and do not fully exercise dependency/auth/book boundaries for every route family. | Route coverage is low for notes, planning, templates, search, imports, exports, admin, and pricing. | Add ASGI tests with real auth/session fixtures for every route family, including 401/403/404/422 branches. | Each protected route family has at least one happy path, one unauthenticated test, one permission failure, and one invalid payload/not-found test. |
| AUD-007 | Medium | Production security defaults are documented but not fully enforced as runtime checks. | `compose.prod.example.yaml` sets secure defaults; `settings.py` accepts insecure combinations. | Add admin runtime warnings for public-host + insecure cookie/CORS/MFA settings rather than preventing LAN installs. | Runtime status reports warnings for public origin with insecure cookies, broad CORS, missing MFA secret, or published database. |

### Post-V1 Roadmap

| ID | Severity | Finding | Evidence | Recommendation | Acceptance criteria |
| --- | --- | --- | --- | --- | --- |
| AUD-009 | High | Frozen report valuation snapshots are schema-implied but absent. | `report_runs.valuation_snapshot_id` exists; docs note missing `valuation_snapshots` and `valuation_snapshot_items`. | Either implement valuation snapshot tables and report execution against them, or remove/defer the frozen pricing mode fields. | Frozen report reruns produce byte-identical output after later price corrections. |
| AUD-010 | Medium | Price corrections use delete-oriented semantics instead of supersession. | `repositories/pricing.py` has delete paths for market and FX observations. | Add `voided_at`, `superseded_by_observation_id`, or equivalent correction chain. | Correcting a bad price preserves the old observation and reports can choose latest-corrected or as-known pricing. |
| AUD-011 | Medium | Planning workflows are structurally present but less robust than core ledger paths. | Planning service/repository coverage remains low and delete paths are hard deletes. | Add scenario tests for rollover budgets, recurrence boundaries, skip/post flows, loan schedule recalculation, and locked-account interactions. | Planning tests cover invalid recurrence, boundary dates, deletes, and projected cash balance behavior. |
| AUD-012 | Medium | Advanced market and instrument adaptation is partial. | V1 scope names bonds, options, futures, crypto spot assets, private investments, corporate actions, and cost-basis profiles; current tests focus mainly on stock-like flows. | Keep Personal V1 focused, but add explicit instrument capability matrix and schema constraints before expanding workflows. | Each supported instrument kind has documented accounting behavior, valuation assumptions, and at least one integration test. |
| AUD-013 | Low | Public VPS deployment is documented but lacks automated end-to-end TLS/proxy verification. | Operational workflow checks compose config and local health, not real ACME/TLS. | Keep TLS validation manual for V1, but add a reverse-proxy smoke profile using local Caddy config validation. | CI validates Caddy config and docs include a VPS verification checklist. |

## Test Coverage Added In This Pass

New or expanded tests cover:

- Settings parsing for CORS, trusted proxy CIDRs, and file-backed first-admin secrets.
- Session cookie flags, invalid SameSite fallback, trusted proxy CIDR matching, and login rate limiting.
- Transaction schema and import rule edge-case validation.
- Production and test Compose security assumptions.
- Transaction service rejection of unbalanced, locked, cross-book, and closed-account splits.
- Pricing policy boundary validation.

New tooling:

- `pytest-cov` in backend dev dependencies.
- Coverage config with branch coverage.
- `api-test-fast`, `api-test-coverage`, and `api-test-postgres-coverage` Make targets.
- Make backend Python/tooling targets now run through `uv run` instead of
  requiring `.venv/bin` on PATH.
- `compose.test.yaml` to prevent Postgres test targets from binding host port 5432.
- API CI now runs compile check and coverage, and uploads coverage artifacts.

## Next Test Work

Prioritized next tests:

1. Real-auth ASGI route tests for notes, templates, search, planning, exports,
   admin, pricing, and imports.
2. Postgres-backed tests for metadata/pricing/planning hard-delete recovery
   semantics.
3. MFA-enforced login/setup behavior once product semantics are chosen.
4. Report frozen pricing and valuation snapshot behavior after the schema
   decision.
5. Import fuzz-style malformed CSV/QIF/OFX/XLSX fixtures and idempotent commit
   tests.
6. Investment lots/corporate-action tests for non-stock instruments and price
   correction workflows.
