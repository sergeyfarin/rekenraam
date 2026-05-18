# TODO

## V1 Cleanup Follow-Ups

- [ ] Add a small lint or schema-contract assertion for raw numeric/boolean
  `server_default` strings.
- [ ] Rename `scheduled_transactions.interval` to `interval_count` before the
  baseline is treated as externally stable.
- [ ] Expand create-books UX and authorization before enabling multi-book
  runtime access beyond the seeded book.
- [ ] Keep backup smoke in release checks: `make backup-smoke` and
  `make restore-smoke BACKUP=...`.

## Current Commands

```bash
make api-test-fast
make api-test
make api-migrate-smoke
docker compose up -d --build --wait
PLAYWRIGHT_BASE_URL=http://localhost:16888 npm run e2e
```
