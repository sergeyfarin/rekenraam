from __future__ import annotations

import asyncio
import os
from pathlib import Path

import pytest
from _sqlite import database_url_for as sqlite_database_url_for
from alembic.config import Config
from sqlalchemy import inspect
from stage2_schema_contract import (
    TAURI_RUNTIME_TABLES,
    contract_from_database,
    contract_from_metadata,
    normalize_for_comparison,
)

import rekenraam_api.db.models  # noqa: F401  -- registers all ORM tables on Base.metadata
from alembic import command
from rekenraam_api.config.settings import get_settings
from rekenraam_api.db.base import Base
from rekenraam_api.db.dialect import create_database_engine


def _run_migrations(database_url: str, revision: str) -> None:
    root_dir = Path(__file__).resolve().parents[1]
    config = Config(str(root_dir / "alembic.ini"))
    # Resolve script_location explicitly so the test passes regardless of pytest cwd.
    config.set_main_option("script_location", str(root_dir / "alembic"))

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


def test_sqlite_alembic_can_upgrade_downgrade_and_reupgrade_clean_database(tmp_path: Path) -> None:
    database_url = sqlite_database_url_for(tmp_path / "migration.sqlite3")

    _run_migrations(database_url, "head")

    async def table_names() -> set[str]:
        engine = create_database_engine(database_url)
        try:
            async with engine.connect() as connection:
                return await connection.run_sync(
                    lambda sync_connection: set(inspect(sync_connection).get_table_names())
                )
        finally:
            await engine.dispose()

    names = asyncio.run(table_names())
    assert {"users", "database_locks", "books", "book_state"} <= names

    _run_migrations(database_url, "base")
    assert "users" not in asyncio.run(table_names())

    _run_migrations(database_url, "head")
    assert "users" in asyncio.run(table_names())


@pytest.mark.asyncio
async def test_alembic_head_matches_full_stage2_schema_contract(
    repository_database_url: str,
) -> None:
    """The migrated database must match what `Base.metadata` describes.

    Drift in either direction (a column added to ORM but not migrated, or a
    migration that creates something the ORM doesn't model) makes this test
    fail. Both halves of the contract are derived from canonical sources —
    no hand-maintained dictionary to keep in sync.
    """

    engine = create_database_engine(repository_database_url)
    try:
        async with engine.connect() as connection:
            expected = contract_from_metadata(Base.metadata, connection.dialect)
            # Sanity: forgetting to import the model modules before reading
            # `Base.metadata` would yield an empty contract that silently equals an
            # empty actual, masking real drift. Guard against that.
            assert len(expected) > 10, (
                "Base.metadata has fewer tables than expected. Did the model "
                "modules get imported before this test ran?"
            )
            actual = await connection.run_sync(contract_from_database)

            assert set(actual) == set(expected), (
                "Tables in the migrated DB don't match Base.metadata. "
                f"Only in DB: {sorted(set(actual) - set(expected))}; "
                f"only in ORM: {sorted(set(expected) - set(actual))}."
            )

            assert normalize_for_comparison(actual) == normalize_for_comparison(expected), (
                "Column/index/FK/unique drift between Base.metadata and the migrated DB."
            )

            # CHECK constraint names must match across both sides. Asserting
            # the DB has a non-empty sqltext for every CHECK keeps the
            # constraints meaningful.
            assert all(
                check.sqltext
                for contract in actual.values()
                for check in contract.check_constraints
            )

            reflected_table_names = set(
                await connection.run_sync(
                    lambda sync_connection: set(inspect(sync_connection).get_table_names())
                )
            )
            assert "alembic_version" in reflected_table_names
            assert not (set(TAURI_RUNTIME_TABLES) & reflected_table_names)
    finally:
        await engine.dispose()
