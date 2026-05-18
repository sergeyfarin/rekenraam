from __future__ import annotations

import os
import sqlite3
from datetime import UTC, datetime, timedelta
from pathlib import Path


def _integrity_check(path: Path) -> None:
    with sqlite3.connect(path) as connection:
        result = connection.execute("PRAGMA integrity_check").fetchone()
        if not result or result[0] != "ok":
            raise RuntimeError(f"SQLite integrity_check failed for {path}: {result!r}")
        connection.execute("SELECT version_num FROM alembic_version LIMIT 1").fetchone()
        connection.execute("SELECT count(*) FROM books").fetchone()


def _prune_old_backups(backup_dir: Path, retention_days: int) -> None:
    if retention_days <= 0:
        return
    cutoff = datetime.now(UTC) - timedelta(days=retention_days)
    for backup in backup_dir.glob("rekenraam-*.sqlite3"):
        modified = datetime.fromtimestamp(backup.stat().st_mtime, UTC)
        if modified < cutoff:
            backup.unlink()


def create_backup(
    *,
    sqlite_path: Path,
    backup_dir: Path,
    retention_days: int,
) -> Path:
    if not sqlite_path.exists():
        raise FileNotFoundError(f"SQLite database not found: {sqlite_path}")

    backup_dir.mkdir(parents=True, exist_ok=True)
    timestamp = datetime.now(UTC).strftime("%Y%m%d-%H%M%S")
    backup_path = backup_dir / f"rekenraam-{timestamp}.sqlite3"

    source_uri = f"file:{sqlite_path}?mode=ro"
    with sqlite3.connect(source_uri, uri=True) as source:
        with sqlite3.connect(backup_path) as target:
            source.backup(target)
            target.execute("PRAGMA journal_mode=DELETE")

    _integrity_check(backup_path)
    _prune_old_backups(backup_dir, retention_days)
    return backup_path


def main() -> None:
    sqlite_path = Path(os.environ.get("SQLITE_PATH", "/data/rekenraam.sqlite3"))
    backup_dir = Path(os.environ.get("BACKUP_DIR", "/backups"))
    retention_days = int(os.environ.get("BACKUP_RETENTION_DAYS", "30"))
    backup_path = create_backup(
        sqlite_path=sqlite_path,
        backup_dir=backup_dir,
        retention_days=retention_days,
    )
    print(backup_path)


if __name__ == "__main__":
    main()
