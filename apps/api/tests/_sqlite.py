"""Shared helpers for tests that need a real SQLite database."""

from __future__ import annotations

import os
import tempfile
from collections.abc import Iterator
from contextlib import contextmanager
from pathlib import Path

from alembic.config import Config

from alembic import command
from rekenraam_api.config.settings import get_settings


def database_url_for(path: Path) -> str:
    return f"sqlite+aiosqlite:///{path}"


def _alembic_root() -> Path:
    return Path(__file__).resolve().parents[1]


def _run_migrations(database_url: str, revision: str = "head") -> None:
    root = _alembic_root()
    config = Config(str(root / "alembic.ini"))
    config.set_main_option("script_location", str(root / "alembic"))

    original_database_url = os.environ.get("DATABASE_URL")
    os.environ["DATABASE_URL"] = database_url
    get_settings.cache_clear()
    try:
        if revision == "base":
            command.downgrade(config, revision)
        else:
            command.upgrade(config, revision)
    finally:
        if original_database_url is None:
            os.environ.pop("DATABASE_URL", None)
        else:
            os.environ["DATABASE_URL"] = original_database_url
        get_settings.cache_clear()


@contextmanager
def temporary_database() -> Iterator[str]:
    with tempfile.TemporaryDirectory(prefix="rekenraam_sqlite_test_") as directory:
        database_path = Path(directory) / "rekenraam.sqlite3"
        database_url = database_url_for(database_path)
        _run_migrations(database_url)
        yield database_url
