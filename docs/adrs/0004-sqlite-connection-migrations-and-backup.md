# ADR 0004: SQLite Connection, Migrations, And Backup

## Status

Accepted

## Date

2026-05-30

## Context

SQLite is the primary database for Rekenraam. The app stores financial data, so connection setup, migration behavior, lock handling, and backup semantics must be deliberate rather than relying on SQLite defaults.

SQLite foreign key enforcement is connection-local and historically defaults off. WAL mode improves reader/writer behavior, but applications still need busy handling and checkpoint policy. Live backups should use SQLite-aware flows rather than copying database files while the app is running.

## Decision

### Connection Setup

Open SQLite through `database/sql` with `modernc.org/sqlite`.

Install required PRAGMAs on every physical SQLite connection by putting them in the modernc SQLite DSN with repeated `_pragma` parameters:

```text
_pragma=busy_timeout(5000)
_pragma=foreign_keys(1)
_pragma=journal_mode(WAL)
_pragma=synchronous(NORMAL)
_pragma=wal_autocheckpoint(1000)
```

Rules:

1. Do not rely on SQLite defaults for foreign keys, journal mode, busy handling, or synchronous mode.
2. `foreign_keys` must be enabled on every connection before migrations or application queries run.
3. `busy_timeout` is **5000 milliseconds**.
4. `journal_mode=WAL` is the normal runtime mode for file-backed databases.
5. `synchronous=NORMAL` is the default runtime setting when WAL is enabled.
6. `wal_autocheckpoint=1000` is set explicitly, matching SQLite's default checkpoint threshold while making the app's intent visible.
7. Startup should validate the effective connection state with `PRAGMA foreign_keys`, `PRAGMA journal_mode`, `PRAGMA busy_timeout`, and `PRAGMA synchronous`.

Early implementation should use one application `*sql.DB` pool with `SetMaxOpenConns(1)` to avoid self-inflicted write contention while the app is single-user and feature-light. Revisit separate read/write pools or a higher connection limit only when real reporting, imports, or backup behavior demonstrates pressure.

### Migration Behavior

Migrations use embedded `pressly/goose` SQL migrations under `backend/migrations`.

Rules:

1. Normal app startup runs pending migrations automatically before the HTTP server starts listening.
2. If migrations fail, startup fails and the app must not serve requests against a partially upgraded schema.
3. Migrations run through the same SQLite connection configuration as normal runtime, including foreign keys, busy timeout, WAL, and synchronous mode.
4. Each SQL migration file runs inside goose's default transaction.
5. Do not use `-- +goose NO TRANSACTION` unless a migration has a documented SQLite requirement that cannot run inside a transaction.
6. Do not put connection-level PRAGMA setup in schema migrations.
7. Down migrations are useful for local development when they are straightforward, but production rollback guidance should prefer restoring a verified backup taken before upgrade.
8. Before the first beta database existed, the schema history was intentionally collapsed into `0001_initial_schema.sql`. From beta onward, that baseline is immutable: make all schema changes as new forward migrations and preserve a verified backup before upgrading a populated database.

### Busy Handling

The first line of defense for `SQLITE_BUSY` is the connection-level `busy_timeout=5000`.

Rules:

1. Application code must still treat `SQLITE_BUSY` as possible.
2. API requests that still hit `SQLITE_BUSY` after the timeout should return a retryable server error rather than silently dropping or partially applying work.
3. Long-running imports, reports, backups, and maintenance tasks must use contexts with deadlines or cancellation.
4. Do not add ad hoc spin loops around database writes. If retry behavior is needed, define it in a shared database helper with context awareness.

### WAL Checkpoints And Backup

Use WAL mode for normal runtime. Keep automatic checkpoints enabled at 1000 pages initially.

Backup rules:

1. Do not document or implement raw copying of the live `.sqlite`, `-wal`, and `-shm` files as the normal backup path.
2. In-app live backup should use SQLite's online backup API through `modernc.org/sqlite` when backup tooling lands.
3. `VACUUM INTO` is an acceptable alternative for an operator-triggered compact backup when its higher CPU/I/O cost is acceptable.
4. A stopped-app file copy is acceptable as an operator fallback if all SQLite database files are copied together and the app is not running.
5. Backup tooling must verify the created backup with `PRAGMA integrity_check` and `PRAGMA foreign_key_check` because the schema contains real foreign keys.
6. Restore instructions must require stopping the app before replacing the active database.

## Consequences

### Positive

- Foreign key enforcement is explicit and cannot silently depend on SQLite defaults.
- Startup migrations keep single-binary and Docker deployments easy to operate.
- WAL, busy timeout, and a conservative connection pool reduce early lock surprises.
- Backup behavior is safe for live financial data and leaves room for a productized backup UI later.

### Negative

- Automatic startup migrations require strong backup guidance before real deployments.
- `SetMaxOpenConns(1)` is conservative and may need revisiting once long reads, reports, imports, or backups become common.
- In-app backup through the online backup API is more code than copying files, but it avoids unsafe live-copy behavior.

## References

- SQLite foreign key support: https://www.sqlite.org/foreignkeys.html
- SQLite PRAGMA documentation: https://www.sqlite.org/pragma.html
- SQLite WAL documentation: https://www.sqlite.org/wal.html
- SQLite online backup API: https://www.sqlite.org/backup.html
- SQLite `VACUUM INTO`: https://www.sqlite.org/lang_vacuum.html
- pressly/goose SQL annotations: https://pressly.github.io/goose/documentation/annotations/
