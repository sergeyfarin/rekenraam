from __future__ import annotations

import pytest
from sqlalchemy import text
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.dialect import session_dialect_name

pytestmark = pytest.mark.asyncio


async def _require_sqlite(session: AsyncSession) -> None:
    if session_dialect_name(session) != "sqlite":
        pytest.skip("SQLite parity tests only run against SQLite")


async def test_sqlite_enforces_foreign_keys(repository_session: AsyncSession) -> None:
    await _require_sqlite(repository_session)

    assert await repository_session.scalar(text("PRAGMA foreign_keys")) == 1
    with pytest.raises(IntegrityError):
        await repository_session.execute(
            text("INSERT INTO book_state (book_id, change_seq) VALUES (999, 0)")
        )


async def test_sqlite_enforces_check_constraints(repository_session: AsyncSession) -> None:
    await _require_sqlite(repository_session)

    with pytest.raises(IntegrityError):
        await repository_session.execute(
            text(
                """
                INSERT INTO commodities (book_id, kind, symbol, name, scale)
                VALUES (1, 'currency', 'BAD', 'Bad Scale', 99)
                """
            )
        )


async def test_sqlite_enforces_partial_unique_indexes(repository_session: AsyncSession) -> None:
    await _require_sqlite(repository_session)

    with pytest.raises(IntegrityError):
        await repository_session.execute(
            text(
                """
                INSERT INTO accounts
                    (book_id, account_type, name, commodity_id, is_system, system_role)
                VALUES
                    (1, 'equity', 'Duplicate Opening Balance', 1, 1, 'opening_balance')
                """
            )
        )


async def test_sqlite_enforces_boolean_values(repository_session: AsyncSession) -> None:
    await _require_sqlite(repository_session)

    with pytest.raises(IntegrityError):
        await repository_session.execute(
            text(
                """
                INSERT INTO users
                    (email, display_name, is_admin, is_active, mfa_required)
                VALUES
                    ('bad-bool@example.test', 'Bad Boolean', 2, 1, 0)
                """
            )
        )


async def test_sqlite_enforces_string_lengths(repository_session: AsyncSession) -> None:
    await _require_sqlite(repository_session)

    with pytest.raises(IntegrityError):
        await repository_session.execute(
            text(
                """
                INSERT INTO users
                    (email, display_name, is_admin, is_active, mfa_required)
                VALUES
                    (:email, 'Too Long', 0, 1, 0)
                """
            ),
            {"email": "x" * 321},
        )
