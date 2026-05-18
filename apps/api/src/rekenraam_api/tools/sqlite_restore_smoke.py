from __future__ import annotations

import sqlite3
import sys
from pathlib import Path


def validate_backup(path: Path) -> tuple[str, int]:
    if not path.exists():
        raise FileNotFoundError(f"Backup not found: {path}")
    with sqlite3.connect(f"file:{path}?mode=ro", uri=True) as connection:
        integrity = connection.execute("PRAGMA integrity_check").fetchone()
        if not integrity or integrity[0] != "ok":
            raise RuntimeError(f"SQLite integrity_check failed: {integrity!r}")
        version = connection.execute("SELECT version_num FROM alembic_version LIMIT 1").fetchone()
        if not version:
            raise RuntimeError("Backup does not contain an alembic_version row")
        book_count = connection.execute("SELECT count(*) FROM books").fetchone()
        if book_count is None:
            raise RuntimeError("Backup does not contain a readable books table")
        return str(version[0]), int(book_count[0])


def main(argv: list[str] | None = None) -> None:
    args = argv if argv is not None else sys.argv[1:]
    if len(args) != 1:
        raise SystemExit(
            "Usage: python -m rekenraam_api.tools.sqlite_restore_smoke /backups/file.sqlite3"
        )
    version, book_count = validate_backup(Path(args[0]))
    print(f"ok version={version} books={book_count}")


if __name__ == "__main__":
    main()
