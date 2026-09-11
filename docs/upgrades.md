# Upgrade Policy And Operator Guide

The v0.1 schema baseline is migration `0001`. Rekenraam applies pending embedded
Goose migrations automatically before the HTTP server starts. A migration
failure stops startup; the app never serves against a partially upgraded schema.

## Before upgrading

1. Stop the running Rekenraam process or container.
2. Keep the existing binary or image tag available for rollback.
3. Create a verified backup with the existing version's Data screen or backup
   command, and copy `REKENRAAM_SECRET_KEY` with it when encrypted connection or
   MFA secrets are in use.
4. Record the release currently running and read the target release notes. Each
   release note must state the highest included migration.

## Upgrade

Start the new binary against the existing database. It applies every pending
migration in order before listening. After startup, run Settings → Data →
Self-check and retain the pre-upgrade backup until the result is `passed`.

The first release, v0.1.0, has no earlier supported release to upgrade from and
contains migration `0001`. Later releases must accept a database from any
earlier tagged release. CI constructs the frozen v0.1 state, inserts sentinel
user data, upgrades it to `HEAD`, and asserts that its schema matches a fresh
install.

## Failed upgrade or rollback

Do not run down migrations on a populated database. Stop the new binary,
preserve the failed database for diagnosis, restore the verified pre-upgrade
backup with the documented `rekenraam restore --from <path>` workflow, and
restart the previous binary. Restore refuses a database whose schema is newer
than the binary understands.

## Maintainer release checklist

- Add schema changes only as the next sequential migration.
- Run `./scripts/test-backend.sh`; both the immutable checksum and historical
  upgrade tests must pass.
- Add every newly released migration and its SHA-256 to
  `backend/migrations/freeze_test.go` in the release commit.
- State the highest migration included in the release notes.
