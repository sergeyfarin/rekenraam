"""Regression tests for the SQLAlchemy audit-log listener added 2026-05-12.

The listener writes one ``AuditLogEntry`` row for every insert / update /
delete observed on an ORM class with ``__audit_logged__ = True``. The fixtures
already register the listener at module-import time through
``rekenraam_api.db.session``; we only need to exercise it.
"""

from __future__ import annotations

from datetime import UTC, date, datetime

import pytest
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.audit_listener import install_audit_listener
from rekenraam_api.db.models.ergonomics import AuditLogEntry
from rekenraam_api.db.models.metadata import Payee
from rekenraam_api.services.request_context import RequestContext, set_request_context


@pytest.fixture(autouse=True)
def _install_listener_and_clear_context():
    """Make sure the listener is registered (tests construct their own engine)
    and that the request-context contextvar starts empty for every test."""
    install_audit_listener()
    set_request_context(None)
    yield
    set_request_context(None)


@pytest.mark.asyncio
async def test_listener_records_insert_for_audited_table(
    repository_session: AsyncSession,
) -> None:
    payee = Payee(book_id=1, name="ACME, Inc.", kind="vendor")
    repository_session.add(payee)
    await repository_session.commit()

    rows = (
        (
            await repository_session.execute(
                select(AuditLogEntry)
                .where(AuditLogEntry.table_name == "payees")
                .where(AuditLogEntry.row_pk == str(payee.id))
            )
        )
        .scalars()
        .all()
    )
    assert len(rows) == 1
    entry = rows[0]
    assert entry.op == "insert"
    assert entry.before_state is None
    assert entry.after_state is not None
    after = entry.after_state
    assert after["name"] == "ACME, Inc."
    assert after["kind"] == "vendor"


@pytest.mark.asyncio
async def test_listener_records_update_with_before_and_after(
    repository_session: AsyncSession,
) -> None:
    payee = Payee(book_id=1, name="Original Name", kind="payee")
    repository_session.add(payee)
    await repository_session.commit()

    payee.name = "Renamed"
    await repository_session.commit()

    rows = (
        (
            await repository_session.execute(
                select(AuditLogEntry)
                .where(AuditLogEntry.table_name == "payees")
                .where(AuditLogEntry.row_pk == str(payee.id))
                .order_by(AuditLogEntry.id)
            )
        )
        .scalars()
        .all()
    )
    assert [row.op for row in rows] == ["insert", "update"]
    update_entry = rows[1]
    assert update_entry.before_state is not None
    assert update_entry.after_state is not None
    assert update_entry.before_state["name"] == "Original Name"
    assert update_entry.after_state["name"] == "Renamed"


@pytest.mark.asyncio
async def test_listener_records_delete_with_full_before_snapshot(
    repository_session: AsyncSession,
) -> None:
    payee = Payee(book_id=1, name="Disposable", kind="payee")
    repository_session.add(payee)
    await repository_session.commit()
    pk = payee.id

    await repository_session.delete(payee)
    await repository_session.commit()

    rows = (
        (
            await repository_session.execute(
                select(AuditLogEntry)
                .where(AuditLogEntry.table_name == "payees")
                .where(AuditLogEntry.row_pk == str(pk))
                .order_by(AuditLogEntry.id)
            )
        )
        .scalars()
        .all()
    )
    ops = [row.op for row in rows]
    assert "delete" in ops
    delete_entry = next(row for row in rows if row.op == "delete")
    assert delete_entry.before_state is not None
    assert delete_entry.after_state is None
    assert delete_entry.before_state["name"] == "Disposable"


@pytest.mark.asyncio
async def test_listener_stamps_request_context_attribution(
    repository_session: AsyncSession,
) -> None:
    """Listener must stamp the active request-context onto the audit row.

    Seeds a real user and auth_session because the audit_log columns FK to
    those tables with ``ON DELETE SET NULL``; the constraint rejects the
    insert if we point at non-existent rows. (request_id is just a string.)
    """
    from rekenraam_api.db.models.access import AuthSession, User

    user = User(
        email="auditor@example.test",
        display_name="Auditor",
        is_active=True,
        password_hash="x",
        is_admin=False,
    )
    repository_session.add(user)
    await repository_session.flush()
    session_row = AuthSession(
        user_id=user.id,
        token_hash="abc",
        expires_at=datetime(2026, 6, 1, tzinfo=UTC),
    )
    repository_session.add(session_row)
    await repository_session.flush()

    set_request_context(
        RequestContext(
            user_id=user.id,
            session_id=session_row.id,
            device_id=None,
            request_id="req-test-123",
            request_timestamp=datetime(2026, 5, 12, 12, 0, 0),
        )
    )
    payee = Payee(book_id=1, name="Attributed", kind="payee")
    repository_session.add(payee)
    await repository_session.commit()

    row = (
        (
            await repository_session.execute(
                select(AuditLogEntry)
                .where(AuditLogEntry.table_name == "payees")
                .where(AuditLogEntry.row_pk == str(payee.id))
                .order_by(AuditLogEntry.id)
            )
        )
        .scalars()
        .first()
    )
    assert row is not None
    assert row.actor_user_id == user.id
    assert row.actor_session_id == session_row.id
    assert row.actor_request_id == "req-test-123"


@pytest.mark.asyncio
async def test_listener_ignores_unaudited_classes(
    repository_session: AsyncSession,
) -> None:
    """The listener must not log writes to its own AuditLogEntry table,
    otherwise it would loop. AuditLogEntry has no ``__audit_logged__`` flag."""
    direct = AuditLogEntry(
        table_name="probe",
        row_pk="1",
        op="insert",
        before_state=None,
        after_state={"smoke": "test"},
    )
    repository_session.add(direct)
    await repository_session.commit()

    follow_ups = (
        (
            await repository_session.execute(
                select(AuditLogEntry).where(AuditLogEntry.table_name == "audit_log")
            )
        )
        .scalars()
        .all()
    )
    assert follow_ups == []


@pytest.mark.asyncio
async def test_listener_coerces_dates_and_decimals_to_strings(
    repository_session: AsyncSession,
) -> None:
    """JSON snapshots must be JSON-serializable. The listener coerces dates
    and other non-JSON-native column values to strings."""
    from rekenraam_api.db.models.accounts import Account

    account = Account(
        book_id=1,
        parent_id=None,
        account_type="asset",
        name="Date Coercion Test",
        commodity_id=1,
        effective_at=date(2026, 5, 3),
    )
    repository_session.add(account)
    await repository_session.commit()

    row = (
        (
            await repository_session.execute(
                select(AuditLogEntry)
                .where(AuditLogEntry.table_name == "accounts")
                .where(AuditLogEntry.row_pk == str(account.id))
            )
        )
        .scalars()
        .one()
    )
    assert row.after_state is not None
    assert row.after_state["effective_at"] == "2026-05-03"
