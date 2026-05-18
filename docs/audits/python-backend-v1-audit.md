# Python Backend V1 Audit

Current V1 target: one FastAPI app container with SQLite.

## Covered Areas

- Auth, bootstrap, sessions, password reset, MFA, invites, and memberships.
- Accounts, transactions, imports/exports, reconciliation, reports, pricing,
  planning, investments, audit log, and admin runtime checks.
- Alembic migration smoke and ORM/schema contract checks against SQLite.
- E2E tests against the same one-container stack used for deployment.

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
