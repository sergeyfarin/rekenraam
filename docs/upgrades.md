# Upgrade Policy And Operator Guide

The first supported household installation uses baseline generation `20261122`
and the frozen migration chain `0001`–`0016` (#576). Build it from the commit
that lands the freeze on `main` or a later one. Later builds preserve the
baseline generation and upgrade this installation in place.

Startup and restore refuse databases or backups from earlier baseline
generations. To recover data from one of those development files, export it
with its original build. Follow the
[developer reset procedure](developer-workflow.md#migrations-and-resetting-your-database)
only for disposable data. Rekenraam applies pending Goose migrations before
the HTTP server starts and stops startup if migration fails.

## Before upgrading

Use a **backup**, not an export, as your safety copy. Only a backup restores the
whole installation; the Settings → Data exports cannot be imported back.

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

   It exits `0` only when this build can restore that file, and `3` when the
   baseline does not match or the schema is newer than this binary; do not
   continue with a backup that exits `3`. The other statuses are `1` for a file
   that cannot be inspected and `2` for a usage error. The full meanings are in
   [Restore](deployment-security.md#restore).

   Copy that key somewhere outside the backup directory when encrypted
   connection or MFA secrets are in use. It is deliberately not in the backup,
   and without it a restored database keeps its ledger but loses multi-factor
   enrolment and connection credentials.
3. Stop the running Rekenraam process or container. Everything below, and the
   rollback path, needs it stopped: `restore` refuses while the server holds
   its lock. Restore holds that same lock throughout replacement, so startup
   and another restore refuse until it completes. Leave `<database>.lock` in
   place between operations; its saved PID or existence is not a live lock.
4. Keep the existing binary or image tag available for rollback.
5. Record the release currently running and read the target release notes. Each
   release note must state the highest included migration.

If the Data screen cannot be reached, stop the app and create a compact
SQLite-aware operator backup. Replace the example paths with your actual
`DATABASE_URL` path and a new backup path on separate storage:

```sh
umask 077
sqlite3 data/rekenraam.sqlite "VACUUM INTO '/mnt/backup/rekenraam-pre-upgrade.sqlite'"
sqlite3 /mnt/backup/rekenraam-pre-upgrade.sqlite "PRAGMA integrity_check"
sqlite3 /mnt/backup/rekenraam-pre-upgrade.sqlite "PRAGMA foreign_key_check"
chmod 600 /mnt/backup/rekenraam-pre-upgrade.sqlite
```

The integrity check must print `ok`; the foreign-key check must return no rows.
Keep the original `REKENRAAM_SECRET_KEY` separately. Do not copy a live
WAL-mode database file.

## Upgrade

Start the new binary against the existing database. It applies every pending
migration in order before listening. After startup, run Settings → Data →
Self-check and retain the pre-upgrade backup until the result is `passed`, or
`incomplete` with no failed check (a book holding unknown investment basis
stays `incomplete`; the unavailable count should match what it was before the
upgrade). Keep the backup after any `failed` result.

Later builds, including the public `v0.1.0` release, must accept databases from
the supported household preview and earlier installed releases.
`TestMigrateSupportedHouseholdDatabase` advances the frozen synthetic book to
the household schema at migration 0016, applies forward migrations including
a test-only append, and checks that the book survives and the upgraded schema
matches a fresh install. The
[2026-10-18 deployment audit](reviews/deployment-audit-2026-10-18.md) separately
records an actual candidate-image upgrade from migrations 14 to 18 and rollback
by verified pre-upgrade backup. That synthetic drill does not establish support
for an earlier installed release or for every intermediate development schema.

The [historical migration notes](archive/pre-release-baseline-rewrites.md#historical-forward-migrations-folded-into-405)
describe the previous candidate. Their capabilities remain in baseline 1;
those intermediate migration numbers are no longer upgrade targets.

## Failed upgrade or rollback

Do not run down migrations on a populated database. Stop the new binary,
preserve the failed database for diagnosis, restore the verified pre-upgrade
backup with the documented `rekenraam restore --from <path>` workflow, and
restart the previous binary. Restore refuses a mismatched baseline generation
or a schema newer than the binary understands.

## Maintainer release checklist

- Add the next sequential migration and its SHA-256 to
  `backend/migrations/freeze_test.go` in the same PR.
- Run `./scripts/test-backend.sh`; checksum and historical upgrade tests must
  pass. Preserve the household baseline generation and frozen fixtures.
- State the highest migration included in the release notes.
- Walk *Before upgrading* against the release build (`pnpm build`, then
  `dist/rekenraam`) on a throwaway database: back up from the running app,
  verify it, stop, restore, restart, and self-check (`passed`, or `incomplete`
  with the same unavailable coverage as before). `restore_test.go` covers
  the commands in isolation; this is the only thing that exercises the sequence
  an operator is told to follow, and it is how the sequence above was found to
  be impossible as written.
