from __future__ import annotations

import asyncio
import os
import uuid
from pathlib import Path

import asyncpg
import pytest
from alembic.config import Config
from sqlalchemy.ext.asyncio import create_async_engine
from stage2_schema_contract import (
    STAGE2_SCHEMA_CONTRACT,
    TAURI_RUNTIME_TABLES,
    database_stage2_schema_contract,
    without_check_sqltext,
)

from alembic import command
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


async def _fetchval(database_name: str, query: str) -> object:
    connection_kwargs = dict(_admin_connection_kwargs())
    connection_kwargs["database"] = database_name
    connection = await asyncpg.connect(**connection_kwargs)
    try:
        return await connection.fetchval(query)
    finally:
        await connection.close()


def _run_migrations(database_name: str, revision: str) -> None:
    root_dir = Path(__file__).resolve().parents[1]
    config = Config(str(root_dir / "alembic.ini"))
    # Resolve script_location explicitly so the test passes regardless of pytest cwd.
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
        if revision == "base":
            command.downgrade(config, revision)
        else:
            command.upgrade(config, revision)
    finally:
        for key, value in original_env.items():
            if value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = value
        get_settings.cache_clear()


def test_alembic_can_upgrade_downgrade_and_reupgrade_clean_database() -> None:
    database_name = f"rekenraam_migration_test_{uuid.uuid4().hex}"
    try:
        asyncio.run(_create_database(database_name))
    except Exception as exc:  # pragma: no cover
        pytest.skip(f"postgres not available for migration tests: {exc}")

    try:
        _run_migrations(database_name, "head")
        assert asyncio.run(_fetchval(database_name, "SELECT to_regclass('public.users')")) == "users"
        assert asyncio.run(_fetchval(database_name, "SELECT to_regclass('public.user_devices')")) == "user_devices"
        assert asyncio.run(_fetchval(database_name, "SELECT to_regclass('public.auth_sessions')")) == "auth_sessions"
        assert (
            asyncio.run(_fetchval(database_name, "SELECT to_regclass('public.book_memberships')"))
            == "book_memberships"
        )
        assert asyncio.run(_fetchval(database_name, "SELECT to_regclass('public.books')")) == "books"
        assert asyncio.run(_fetchval(database_name, "SELECT to_regclass('public.book_state')")) == "book_state"
        assert asyncio.run(_fetchval(database_name, "SELECT to_regclass('public.report_cache')")) == "report_cache"
        assert asyncio.run(_fetchval(database_name, "SELECT to_regclass('public.report_definitions')")) == "report_definitions"
        assert asyncio.run(_fetchval(database_name, "SELECT to_regclass('public.report_runs')")) == "report_runs"
        assert asyncio.run(_fetchval(database_name, "SELECT to_regclass('public.pricing_refresh_runs')")) == "pricing_refresh_runs"
        assert asyncio.run(_fetchval(database_name, "SELECT COUNT(*) FROM books")) == 1
        assert asyncio.run(_fetchval(database_name, "SELECT COUNT(*) FROM book_state")) == 1
        assert asyncio.run(_fetchval(database_name, "SELECT COUNT(*) FROM commodities")) == 1
        assert asyncio.run(_fetchval(database_name, "SELECT COUNT(*) FROM price_sources")) == 10

        _run_migrations(database_name, "base")
        assert asyncio.run(_fetchval(database_name, "SELECT to_regclass('public.users')")) is None
        assert asyncio.run(_fetchval(database_name, "SELECT to_regclass('public.auth_sessions')")) is None
        assert asyncio.run(_fetchval(database_name, "SELECT to_regclass('public.book_memberships')")) is None

        _run_migrations(database_name, "head")
        assert asyncio.run(_fetchval(database_name, "SELECT to_regclass('public.users')")) == "users"
        assert asyncio.run(_fetchval(database_name, "SELECT COUNT(*) FROM books")) == 1
    finally:
        asyncio.run(_drop_database(database_name))


@pytest.mark.skip(
    reason=(
        "stage2_schema_contract.py is materially incomplete (missing migration-0004 MFA tables and "
        "migration-0005 password_reset_tokens, plus drift on the users table). Tracked as Phase 2 "
        "step 10 in docs/product/v1-gap-plan.md: rebuild the contract from Base.metadata so it "
        "can't drift silently. Re-enable this test when that lands."
    )
)
@pytest.mark.asyncio
async def test_alembic_head_matches_full_stage2_schema_contract(repository_database_url: str) -> None:
    engine = create_async_engine(repository_database_url, future=True)
    try:
        async with engine.connect() as connection:
            table_names = set(
                await connection.run_sync(
                    lambda sync_connection: set(database_stage2_schema_contract(sync_connection))
                )
            )
            assert table_names == set(STAGE2_SCHEMA_CONTRACT)

            actual_schema = await connection.run_sync(database_stage2_schema_contract)
            assert without_check_sqltext(actual_schema) == without_check_sqltext(STAGE2_SCHEMA_CONTRACT)

            actual_check_names = {
                table_name: tuple(check.name for check in contract.check_constraints)
                for table_name, contract in actual_schema.items()
            }
            expected_check_names = {
                table_name: tuple(check.name for check in contract.check_constraints)
                for table_name, contract in STAGE2_SCHEMA_CONTRACT.items()
            }
            assert actual_check_names == expected_check_names
            assert all(
                check.sqltext
                for contract in actual_schema.values()
                for check in contract.check_constraints
            )

            reflected_table_names = set(
                await connection.run_sync(
                    lambda sync_connection: set(sync_connection.dialect.get_table_names(sync_connection))
                )
            )
            assert "alembic_version" in reflected_table_names
            assert not (set(TAURI_RUNTIME_TABLES) & reflected_table_names)
    finally:
        await engine.dispose()
