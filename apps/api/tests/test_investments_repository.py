from datetime import date

import pytest
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.accounts import Account
from rekenraam_api.db.models.investments import Lot, PriceObservation, SplitLotAllocation
from rekenraam_api.db.models.metadata import Commodity
from rekenraam_api.db.models.transactions import Split, Transaction
from rekenraam_api.repositories.investments import InvestmentRepository


async def _seed_investment_data(session: AsyncSession) -> tuple[int, int]:
    security = Commodity(book_id=1, kind="stock", symbol="ACME", name="Acme Corp", scale=4)
    brokerage = Account(
        book_id=1,
        parent_id=1,
        previous_account_id=None,
        account_type="investment",
        name="Brokerage",
        commodity_id=1,
        booking_policy="fifo",
        number_last4=None,
        is_closed=False,
        is_hidden=False,
        is_system=False,
        system_role=None,
        effective_at=date(2026, 5, 1),
        lifecycle_event="open",
        lifecycle_note=None,
        lifecycle_metadata=None,
    )
    session.add_all([security, brokerage])
    await session.flush()

    buy_tx = Transaction(
        book_id=1,
        occurred_date=date(2026, 5, 10),
        posted_date=date(2026, 5, 10),
        payee_id=None,
        memo="Buy ACME",
        status="cleared",
        reference=None,
    )
    sell_tx = Transaction(
        book_id=1,
        occurred_date=date(2026, 6, 10),
        posted_date=date(2026, 6, 10),
        payee_id=None,
        memo="Sell ACME",
        status="cleared",
        reference=None,
    )
    session.add_all([buy_tx, sell_tx])
    await session.flush()

    buy_invest_split = Split(
        tx_id=buy_tx.id,
        account_id=brokerage.id,
        commodity_id=security.id,
        amount_minor=100000,
        category_id=None,
        tag_id=None,
        person_id=None,
        project_id=None,
        share_bps=None,
        memo="Buy shares",
    )
    buy_cash_split = Split(
        tx_id=buy_tx.id,
        account_id=2,
        commodity_id=1,
        amount_minor=-250000,
        category_id=None,
        tag_id=None,
        person_id=None,
        project_id=None,
        share_bps=None,
        memo="Pay cash",
    )
    sell_invest_split = Split(
        tx_id=sell_tx.id,
        account_id=brokerage.id,
        commodity_id=security.id,
        amount_minor=-40000,
        category_id=None,
        tag_id=None,
        person_id=None,
        project_id=None,
        share_bps=None,
        memo="Sell shares",
    )
    sell_cash_split = Split(
        tx_id=sell_tx.id,
        account_id=2,
        commodity_id=1,
        amount_minor=120000,
        category_id=None,
        tag_id=None,
        person_id=None,
        project_id=None,
        share_bps=None,
        memo="Sale proceeds",
    )
    session.add_all([buy_invest_split, buy_cash_split, sell_invest_split, sell_cash_split])
    await session.flush()

    lot = Lot(
        book_id=1,
        account_id=brokerage.id,
        commodity_id=security.id,
        opened_date=date(2026, 5, 10),
        notes=None,
        cost_basis_minor=250000,
    )
    session.add(lot)
    await session.flush()

    session.add_all(
        [
            SplitLotAllocation(split_id=buy_invest_split.id, lot_id=lot.id, quantity_minor=100000),
            SplitLotAllocation(split_id=sell_invest_split.id, lot_id=lot.id, quantity_minor=-40000),
            PriceObservation(
                book_id=1,
                commodity_id=security.id,
                quote_commodity_id=1,
                observation_kind="commodity_market",
                price_minor=30000,
                price_date=date(2026, 6, 15),
                source="manual",
            ),
        ]
    )
    await session.commit()
    return brokerage.id, security.id


@pytest.mark.asyncio
async def test_investment_repository_returns_positions_lots_and_gains(repository_session: AsyncSession) -> None:
    brokerage_id, security_id = await _seed_investment_data(repository_session)
    repository = InvestmentRepository(repository_session)

    positions = await repository.list_positions(book_id=1, as_of_date=date(2026, 6, 15))
    converted_positions = await repository.convert_positions(book_id=1, base_commodity_id=1, as_of_date=date(2026, 6, 15))
    lots = await repository.list_lots_with_holding_period(
        book_id=1,
        account_id=brokerage_id,
        commodity_id=security_id,
        as_of_date=date(2027, 6, 15),
    )
    realized = await repository.report_realized_gains(book_id=1, date_from=date(2026, 6, 1), date_to=date(2026, 6, 30))
    unrealized = await repository.report_unrealized_gains(book_id=1, base_commodity_id=1, as_of_date=date(2026, 6, 15))

    security_position = next(position for position in positions if position["commodity_id"] == security_id)
    converted_security_position = next(position for position in converted_positions if position["commodity_id"] == security_id)
    unrealized_security = next(position for position in unrealized if position["commodity_id"] == security_id)

    assert security_position["balance_minor"] == 60000
    assert security_position["lots"][0]["remaining_cost_basis_minor"] == 150000
    assert converted_security_position["value_minor"] == 180000
    assert converted_security_position["lots"][0]["converted_cost_basis_minor"] == 150000
    assert lots[0]["quantity_minor"] == 60000
    assert lots[0]["is_long_term"] is True
    assert realized == [
        {
            "tx_id": 3,
            "txn_date": date(2026, 6, 10),
            "commodity_id": security_id,
            "quantity_minor": 40000,
            "proceeds_minor": 120000,
            "quote_commodity_id": 1,
            "cost_basis_minor": 100000,
            "gain_loss_minor": 20000,
            "proceeds_missing": False,
        }
    ]
    assert unrealized_security["value_minor"] == 180000
    assert unrealized_security["cost_basis_minor"] == 150000
    assert unrealized_security["unrealized_gain_minor"] == 30000


@pytest.mark.asyncio
async def test_investment_repository_creates_buy_sell_and_dividend_transactions(repository_session: AsyncSession) -> None:
    brokerage_id, security_id = await _seed_investment_data(repository_session)
    repository = InvestmentRepository(repository_session)

    buy_result = await repository.create_buy(
        book_id=1,
        txn_date=date(2026, 7, 1),
        commodity_id=security_id,
        investment_account_id=brokerage_id,
        cash_account_id=2,
        quantity_minor=20000,
        cash_amount_minor=70000,
        memo="Buy more ACME",
        payee_id=None,
        status="cleared",
    )
    sell_result = await repository.create_sell(
        book_id=1,
        txn_date=date(2026, 7, 2),
        commodity_id=security_id,
        investment_account_id=brokerage_id,
        cash_account_id=2,
        quantity_minor=30000,
        cash_amount_minor=90000,
        lot_strategy="fifo",
        lot_allocations=None,
        allow_short=False,
        memo="Sell ACME",
        payee_id=None,
        status="cleared",
    )
    dividend_result = await repository.create_dividend(
        book_id=1,
        txn_date=date(2026, 7, 3),
        cash_account_id=2,
        income_account_id=3,
        amount_minor=5000,
        memo="Dividend",
        payee_id=None,
        status="cleared",
    )

    buy_tx = await repository._session.get(Transaction, buy_result["transaction_id"])
    sell_tx = await repository._session.get(Transaction, sell_result["transaction_id"])
    dividend_tx = await repository._session.get(Transaction, dividend_result["transaction_id"])

    buy_splits = (
        await repository._session.execute(select(Split).where(Split.tx_id == buy_result["transaction_id"]).order_by(Split.id))
    ).scalars().all()
    sell_splits = (
        await repository._session.execute(select(Split).where(Split.tx_id == sell_result["transaction_id"]).order_by(Split.id))
    ).scalars().all()
    dividend_splits = (
        await repository._session.execute(select(Split).where(Split.tx_id == dividend_result["transaction_id"]).order_by(Split.id))
    ).scalars().all()

    buy_lot = await repository._session.get(Lot, buy_result["lot_id"])
    sell_allocations = (
        await repository._session.execute(
            select(SplitLotAllocation).where(SplitLotAllocation.split_id == next(split.id for split in sell_splits if split.account_id == brokerage_id))
        )
    ).scalars().all()

    assert buy_tx is not None
    assert sell_tx is not None
    assert dividend_tx is not None
    assert sum(split.amount_minor for split in buy_splits) == -50000
    assert sum(split.amount_minor for split in sell_splits) == 60000
    assert sum(split.amount_minor for split in dividend_splits) == 0
    assert buy_lot is not None
    assert buy_lot.cost_basis_minor == 70000
    assert sell_allocations[0].quantity_minor == -30000
