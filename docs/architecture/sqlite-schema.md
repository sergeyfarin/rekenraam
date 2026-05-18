# SQLite Schema Notes

SQLite is the only V1 runtime database. The schema keeps `book_id` columns and a
multi-book shape so later product work can enable additional books without a
schema reset, but V1 runtime access remains guarded to the seeded book.

## Runtime Pragmas

The API installs these pragmas on every SQLite connection:

- `foreign_keys=ON`
- `journal_mode=WAL`
- `busy_timeout=5000`
- `synchronous=NORMAL`

## Migrations

Alembic owns the schema. The baseline includes auth/session tables, books,
accounts, transactions, imports, reconciliation, reports, pricing, planning,
investments, audit log, and runtime lock rows.

SQLite-specific parity triggers remain in the baseline for cross-table
invariants that are hard to express with ordinary constraints.

## Backups

Backups use Python's SQLite online backup API. Backup smoke validates:

- `PRAGMA integrity_check`
- an `alembic_version` row
- readability of the `books` table

Live file copies are discouraged because WAL databases may have committed data
outside the main database file.
