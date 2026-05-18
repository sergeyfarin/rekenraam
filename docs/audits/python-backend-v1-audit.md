# Python Backend V1 Audit

Current V1 target: one FastAPI app container with SQLite.

## Summary

This audit reviews the Python backend against the active personal-finance v1
scope, not the broader future accounting-foundations proposal.

Current backend shape:

- FastAPI API under `/api/v1`
- SQLAlchemy async repositories and services
- Alembic/SQLAlchemy sqlite baseline
- Pydantic request/response contracts
- One-container Docker deployment used by both ops smoke and Playwright e2e

The backend already covers the main v1 product surface. The remaining risks are
less about missing whole modules and more about history semantics, invariant
enforcement, route-depth coverage, and operational hardening.

## Covered Areas

- Auth, bootstrap, sessions, password reset, MFA, invites, and memberships.
- Accounts, transactions, imports/exports, reconciliation, reports, pricing,
  planning, investments, audit log, and admin runtime checks.
- Alembic migration smoke and ORM/schema contract checks against SQLite.
- E2E tests against the same one-container stack used for deployment.

## Module Matrix

| Area | Confidence | Notes |
| --- | --- | --- |
| config/app/dependencies/request context | Medium | Runtime wiring is stable; public-host warning depth can still improve. |
| auth/access/admin | Medium | Bootstrap, login, reset, invites, MFA, and memberships exist; negative-path and enforcement semantics still need depth. |
| books/accounts/transactions/reconciliation | Medium-high | Main ledger flows exist and are heavily exercised; locked-range, correction semantics, and load behavior remain the risk surface. |
| metadata/preferences/templates/notes/search | Medium | Main endpoints exist; history semantics and route-depth coverage are thinner than the ledger core. |
| imports/exports | Medium | Format coverage is broad; malformed-file, duplicate, and edge-case depth should keep expanding. |
| reports/report invalidation | Medium | Core reports exist; frozen reproducibility and richer invalidation semantics remain follow-up work. |
| investments/pricing | Medium | Main workflows exist; correction semantics and deeper investment-accounting helpers are still thinner than the mature ledger flows. |
| planning | Medium-low | Budgets, schedules, and loans exist, but validation and scenario depth still lag the ledger core. |
| models/migrations/audit | Medium | Migration smoke and schema-contract checks exist; direct-SQL limitations and recoverable-history semantics remain important caveats. |
| deployment/scripts/docs | Medium-high | One-container deployment and backup tooling are documented and smoke-checked; public-host discipline remains an operator risk. |

## Operational Checks

- `make api-check`
- `make api-lint`
- `make api-format-check`
- `make api-typecheck`
- `make api-test-fast`
- `make api-test`
- `make api-migrate-smoke`
- `make backup-smoke`
- `make restore-smoke BACKUP=...`

## Open Risks

- SQLite write contention should be watched as reconciliation/import workloads
  grow.
- Public deployments need operator discipline around secure cookies, MFA, DNS,
  proxy config, and backup retention.
- Multi-book schema is present, but create-books product workflows remain gated.

## Findings Backlog

### V1 Hardening

| ID | Severity | Finding | Recommendation |
| --- | --- | --- | --- |
| AUD-001 | High | Hard deletes and simpler mutable-reference semantics still exist in some non-core slices. | Keep converting user-visible financial/reference flows toward recoverable state changes or explicit history where product history matters. |
| AUD-002 | High | Audit enforcement is not fully database-enforced in every path. | Keep documenting the direct-SQL limitation and preserve schema-contract plus migration-smoke checks until stronger database-side guarantees exist. |
| AUD-003 | Medium | Route-depth coverage is uneven outside the ledger core. | Add more real-auth route tests for notes, planning, templates, search, exports, admin, pricing, and imports, including negative-path coverage. |
| AUD-004 | Medium | Runtime security guidance is documented more strongly than it is surfaced in-product. | Keep adding admin/runtime warnings for insecure public-host combinations instead of relying only on docs. |
| AUD-005 | Medium | Planning, pricing correction, and frozen-report reproducibility remain thinner than the core transaction flows. | Treat these as scenario-test and semantic-hardening priorities rather than assuming module presence is enough. |

### Longer-Horizon Gaps

| ID | Severity | Finding | Recommendation |
| --- | --- | --- | --- |
| AUD-006 | Medium | Frozen report valuation reproducibility is still weaker than the schema shape suggests. | Either snapshot the needed report inputs explicitly or simplify the exposed frozen-report contract until reproducibility is real. |
| AUD-007 | Medium | Price correction semantics are still less history-preserving than ideal. | Prefer supersession/correction chains over destructive replacement where report reproducibility depends on historical observations. |
| AUD-008 | Medium | Some advanced investment and planning helpers remain shallower than the broader architectural proposal. | Keep the active v1 audit honest about these as future-depth items rather than treating them as release blockers by default. |

## What Must Not Get Lost

- SQLite-first validation is a feature, not a downgrade: the ops smoke,
  migration smoke, schema-contract tests, and Playwright stack now exercise the
  actual supported deployment path.
- The existence of a module or endpoint is not enough confidence for v1; the
  important question is whether history semantics, auth boundaries, and error
  behavior are pinned by tests.
- The accounting-foundations document is still useful as a future-direction
  pressure test, but this audit should continue measuring the backend against
  the active personal-finance sqlite-only release target.
