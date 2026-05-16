"""Shared helpers for tests that need a real Postgres database.

Used by both the repository-level fixtures in `tests/conftest.py` and the
end-to-end fixtures in `tests/e2e/conftest.py`. Creates a fresh empty
database, runs Alembic migrations to head, and tears it down afterwards.
"""

from __future__ import annotations

import asyncio
import os
import uuid
from collections.abc import Iterator
from contextlib import contextmanager
from pathlib import Path

import asyncpg
from alembic.config import Config

from alembic import command
from rekenraam_api.config.settings import get_settings


def admin_connection_kwargs() -> dict[str, str | int]:
    return {
        "user": os.environ.get("TEST_POSTGRES_USER", os.environ.get("POSTGRES_USER", "rekenraam")),
        "password": os.environ.get(
            "TEST_POSTGRES_PASSWORD",
            os.environ.get("POSTGRES_PASSWORD", "rekenraam"),
        ),
        "host": os.environ.get("TEST_POSTGRES_HOST", "127.0.0.1"),
        "port": int(os.environ.get("TEST_POSTGRES_PORT", os.environ.get("POSTGRES_PORT", "5432"))),
        "database": os.environ.get("TEST_POSTGRES_ADMIN_DB", "postgres"),
    }


def database_url_for(database_name: str) -> str:
    admin = admin_connection_kwargs()
    return (
        "postgresql+asyncpg://"
        f"{admin['user']}:{admin['password']}"
        f"@{admin['host']}:{admin['port']}/{database_name}"
    )


async def _create_database(database_name: str) -> None:
    connection = await asyncpg.connect(**admin_connection_kwargs())
    try:
        await connection.execute(f'CREATE DATABASE "{database_name}"')
    finally:
        await connection.close()


async def _drop_database(database_name: str) -> None:
    connection = await asyncpg.connect(**admin_connection_kwargs())
    try:
        await connection.execute(
            """
            SELECT pg_terminate_backend(pid)
            FROM pg_stat_activity
            WHERE datname = $1 AND pid <> pg_backend_pid()
            """,
            database_name,
        )
        await connection.execute(f'DROP DATABASE IF EXISTS "{database_name}"')
    finally:
        await connection.close()


def _alembic_root() -> Path:
    return Path(__file__).resolve().parents[1]


def _run_migrations(database_name: str) -> None:
    root = _alembic_root()
    config = Config(str(root / "alembic.ini"))
    config.set_main_option("script_location", str(root / "alembic"))

    original_env = {
        "DATABASE_URL": os.environ.get("DATABASE_URL"),
        "POSTGRES_DB": os.environ.get("POSTGRES_DB"),
        "POSTGRES_USER": os.environ.get("POSTGRES_USER"),
        "POSTGRES_PASSWORD": os.environ.get("POSTGRES_PASSWORD"),
        "POSTGRES_HOST": os.environ.get("POSTGRES_HOST"),
        "POSTGRES_PORT": os.environ.get("POSTGRES_PORT"),
    }
    admin = admin_connection_kwargs()
    os.environ["DATABASE_URL"] = database_url_for(database_name)
    os.environ["POSTGRES_DB"] = database_name
    os.environ["POSTGRES_USER"] = str(admin["user"])
    os.environ["POSTGRES_PASSWORD"] = str(admin["password"])
    os.environ["POSTGRES_HOST"] = str(admin["host"])
    os.environ["POSTGRES_PORT"] = str(admin["port"])
    get_settings.cache_clear()
    try:
        command.upgrade(config, "head")
    finally:
        for key, value in original_env.items():
            if value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = value
        get_settings.cache_clear()


@contextmanager
def temporary_database() -> Iterator[str]:
    """Create a one-shot Postgres database with the schema migrated to head.

    Yields the SQLAlchemy/asyncpg URL for the database. The database is
    dropped on exit even if the caller raises.
    """

    database_name = f"rekenraam_test_{uuid.uuid4().hex}"
    asyncio.run(_create_database(database_name))
    try:
        _run_migrations(database_name)
        yield database_url_for(database_name)
    finally:
        asyncio.run(_drop_database(database_name))
