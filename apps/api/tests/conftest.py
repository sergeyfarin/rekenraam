from __future__ import annotations

import asyncio
import os
import uuid
from collections.abc import Iterator
from pathlib import Path

import asyncpg
import pytest
import pytest_asyncio
from alembic import command
from alembic.config import Config
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker, create_async_engine

from rekenraam_api.config.settings import get_settings


def _admin_connection_kwargs() -> dict[str, str | int]:
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


async def _create_database(database_name: str) -> None:
    connection = await asyncpg.connect(**_admin_connection_kwargs())
    try:
        await connection.execute(f'CREATE DATABASE "{database_name}"')
    finally:
        await connection.close()


async def _drop_database(database_name: str) -> None:
    connection = await asyncpg.connect(**_admin_connection_kwargs())
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


def _run_migrations(database_name: str) -> None:
    root_dir = Path(__file__).resolve().parents[1]
    config = Config(str(root_dir / "alembic.ini"))
    config.set_main_option("script_location", str(root_dir / "alembic"))

    original_env = {
        "POSTGRES_DB": os.environ.get("POSTGRES_DB"),
        "POSTGRES_USER": os.environ.get("POSTGRES_USER"),
        "POSTGRES_PASSWORD": os.environ.get("POSTGRES_PASSWORD"),
        "POSTGRES_HOST": os.environ.get("POSTGRES_HOST"),
        "POSTGRES_PORT": os.environ.get("POSTGRES_PORT"),
    }
    admin_connection = _admin_connection_kwargs()
    os.environ["POSTGRES_DB"] = database_name
    os.environ["POSTGRES_USER"] = str(admin_connection["user"])
    os.environ["POSTGRES_PASSWORD"] = str(admin_connection["password"])
    os.environ["POSTGRES_HOST"] = str(admin_connection["host"])
    os.environ["POSTGRES_PORT"] = str(admin_connection["port"])
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


@pytest.fixture()
def repository_database_url() -> Iterator[str]:
    database_name = f"rekenraam_test_{uuid.uuid4().hex}"
    try:
        asyncio.run(_create_database(database_name))
    except Exception as exc:  # pragma: no cover
        pytest.skip(f"postgres not available for repository tests: {exc}")

    _run_migrations(database_name)
    admin_connection = _admin_connection_kwargs()
    database_url = (
        "postgresql+asyncpg://"
        f"{admin_connection['user']}:{admin_connection['password']}"
        f"@{admin_connection['host']}:{admin_connection['port']}/{database_name}"
    )

    try:
        yield database_url
    finally:
        asyncio.run(_drop_database(database_name))


@pytest_asyncio.fixture()
async def repository_session(repository_database_url: str) -> AsyncSession:
    engine = create_async_engine(repository_database_url, future=True)
    session_factory = async_sessionmaker(bind=engine, class_=AsyncSession, expire_on_commit=False)

    async with session_factory() as session:
        yield session

    await engine.dispose()
