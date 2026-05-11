"""Regression tests for the v1 accounting-correctness invariants added 2026-05-12.

Each test covers one specific invariant that was identified as an H-severity
gap and explicitly fixed in this pass:

- Partial unique index on ``accounts(book_id, system_role)`` so each book has
  at most one of each system-role account.
- Allow-list CHECK on ``accounts.system_role`` to keep the set of permitted
  role strings honest.
- ``price_sources`` 'manual' seed row exists.
- ``UNIQUE(split_id, lot_id)`` constraint on ``split_lot_allocations``.

The audit-log listener has its own test file.
"""

from __future__ import annotations

from datetime import date

import pytest
from sqlalchemy import select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.accounts import Account
from rekenraam_api.db.models.investments import Lot, SplitLotAllocation
from rekenraam_api.db.models.pricing import PriceSource
from rekenraam_api.db.models.transactions import Split, Transaction


@pytest.mark.asyncio
async def test_system_role_per_book_is_unique(repository_session: AsyncSession) -> None:
    """Each (book_id, system_role) pair must be unique. The partial unique
    index on system_role IS NOT NULL prevents duplicate system accounts."""
    base = Account(
        book_id=1,
        parent_id=None,
        account_type="income",
        name="Income Summary One",
        commodity_id=1,
        is_system=True,
        system_role="income_summary",
        effective_at=date(2026, 5, 1),
    )
    repository_session.add(base)
    await repository_session.commit()

    dup = Account(
        book_id=1,
        parent_id=None,
        account_type="income",
        name="Income Summary Two",
        commodity_id=1,
        is_system=True,
        system_role="income_summary",
        effective_at=date(2026, 5, 1),
    )
    repository_session.add(dup)
    with pytest.raises(IntegrityError):
        await repository_session.commit()
    await repository_session.rollback()


@pytest.mark.asyncio
async def test_system_role_allow_list_rejects_bad_values(
    repository_session: AsyncSession,
) -> None:
    """An unknown system_role string must be rejected by the CHECK constraint."""
    bogus = Account(
        book_id=1,
        parent_id=None,
        account_type="equity",
        name="Bogus System Role",
        commodity_id=1,
        is_system=True,
        system_role="not_a_real_role",
        effective_at=date(2026, 5, 1),
    )
    repository_session.add(bogus)
    with pytest.raises(IntegrityError):
        await repository_session.commit()
    await repository_session.rollback()


@pytest.mark.asyncio
async def test_non_system_accounts_can_share_null_system_role(
    repository_session: AsyncSession,
) -> None:
    """The partial index only applies WHERE system_role IS NOT NULL, so two
    ordinary accounts with system_role NULL must coexist freely."""
    a = Account(
        book_id=1,
        parent_id=None,
        account_type="asset",
        name="Checking",
        commodity_id=1,
        is_system=False,
        system_role=None,
        effective_at=date(2026, 5, 1),
    )
    b = Account(
        book_id=1,
        parent_id=None,
        account_type="asset",
        name="Savings",
        commodity_id=1,
        is_system=False,
        system_role=None,
        effective_at=date(2026, 5, 1),
    )
    repository_session.add_all([a, b])
    await repository_session.commit()
    assert a.id != b.id


@pytest.mark.asyncio
async def test_price_sources_manual_row_is_seeded(repository_session: AsyncSession) -> None:
    """Manual price-entry routes assume a 'manual' price_source row exists."""
    row = (
        await repository_session.execute(select(PriceSource).where(PriceSource.kind == "manual"))
    ).scalar_one_or_none()
    assert row is not None, "baseline migration must seed a manual price source"
    assert row.name == "Manual"


@pytest.mark.asyncio
async def test_split_lot_allocation_rejects_duplicate_split_lot_pair(
    repository_session: AsyncSession,
) -> None:
    """A given (split_id, lot_id) must not allocate twice. Enforced by the
    composite UNIQUE on split_lot_allocations."""
    tx = Transaction(
        book_id=1,
        occurred_date=date(2026, 5, 3),
        posted_date=date(2026, 5, 3),
        memo="lot test",
        status="uncleared",
    )
    repository_session.add(tx)
    await repository_session.flush()

    split = Split(
        tx_id=tx.id,
        account_id=2,
        commodity_id=1,
        amount_minor=1000,
    )
    repository_session.add(split)
    await repository_session.flush()

    lot = Lot(
        book_id=1,
        account_id=2,
        commodity_id=1,
        opened_date=date(2026, 5, 3),
        cost_basis_minor=1000,
    )
    repository_session.add(lot)
    await repository_session.flush()

    first = SplitLotAllocation(split_id=split.id, lot_id=lot.id, quantity_minor=100)
    repository_session.add(first)
    await repository_session.commit()

    dup = SplitLotAllocation(split_id=split.id, lot_id=lot.id, quantity_minor=50)
    repository_session.add(dup)
    with pytest.raises(IntegrityError):
        await repository_session.commit()
    await repository_session.rollback()
