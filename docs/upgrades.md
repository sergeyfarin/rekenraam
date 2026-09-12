# Upgrade Policy And Operator Guide

The v0.1 schema baseline is migration `0001`. Rekenraam applies pending embedded
Goose migrations automatically before the HTTP server starts. A migration
failure stops startup; the app never serves against a partially upgraded schema.

## Before upgrading

The first two steps need the old version **running**; the rest need it
**stopped**. Taking the backup is not something a stopped app can do — the copy
is made by the running process through SQLite's online backup API, and there is
no `backup` subcommand to reach for instead.

1. **With the current version still running**, create a backup: Settings → Data
   → **Back up now**, then wait for it to show up under **Last backup**. The
   button only queues the copy — the screen says so — and the backup worker
   picks it up within a minute, so stopping the app before it lands means no
   backup was taken. A recent nightly backup is just as good; the point is a
   known-good copy from *before* the upgrade.
2. **Still running**, verify that backup, with the same `REKENRAAM_SECRET_KEY`
   the app runs with so its sealed-data line checks the key a restore would
   really use:

   ```sh
   ./rekenraam verify-backup --from <backup path>
   ```

   Copy that key somewhere outside the backup directory when encrypted
   connection or MFA secrets are in use. It is deliberately not in the backup,
   and without it a restored database keeps its ledger but loses multi-factor
   enrolment and connection credentials.
3. Stop the running Rekenraam process or container. Everything below, and the
   rollback path, needs it stopped: `restore` refuses while the server holds
   its lock.
4. Keep the existing binary or image tag available for rollback.
5. Record the release currently running and read the target release notes. Each
   release note must state the highest included migration.

If the Data screen cannot be reached, take the operator backup documented under
*Backup And Restore* in `README.md` (`VACUUM INTO` against a stopped app)
instead. Do not copy a live WAL-mode database file.

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
- Walk *Before upgrading* against the release build (`pnpm build`, then
  `dist/rekenraam`) on a throwaway database: back up from the running app,
  verify it, stop, restore, restart, and self-check. `restore_test.go` covers
  the commands in isolation; this is the only thing that exercises the sequence
  an operator is told to follow, and it is how the sequence above was found to
  be impossible as written.
