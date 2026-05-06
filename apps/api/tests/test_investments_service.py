from datetime import date

import pytest

from rekenraam_api.schemas.investments import (
    BuyCommodityInput,
    ConvertedPositionsQuery,
    DividendInput,
    LotsHoldingQuery,
    PositionsQuery,
    RealizedGainsQuery,
    SellCommodityInput,
    UnrealizedGainsQuery,
)
from rekenraam_api.services.investments import InvestmentService


class StubInvestmentRepository:
    async def create_buy(self, **_: object) -> dict[str, object]:
        return {
            "transaction_id": 10,
            "allocations": [{"lot_id": 8, "quantity_minor": 60000}],
            "lot_id": 8,
        }

    async def create_sell(self, **_: object) -> dict[str, object]:
        return {
            "transaction_id": 11,
            "allocations": [{"lot_id": 1, "quantity_minor": 40000}],
            "lot_id": None,
        }

    async def create_dividend(self, **_: object) -> dict[str, object]:
        return {"transaction_id": 12}

    async def list_positions(self, *, book_id: int, as_of_date: date | None) -> list[dict[str, object]]:
        return [
            {
                "account_id": 4,
                "account_name": "Brokerage",
                "account_type": "investment",
                "commodity_id": 2,
                "commodity_name": "Acme Corp",
                "commodity_scale": 4,
                "balance_minor": 60000,
                "lots": [
                    {
                        "lot_id": 1,
                        "opened_date": date(2026, 5, 10),
                        "quantity_minor": 60000,
                        "cost_basis_minor": 250000,
                        "remaining_cost_basis_minor": 150000,
                        "converted_value_minor": None,
                        "converted_cost_basis_minor": None,
                        "price_missing": None,
                    }
                ],
            }
        ]

    async def convert_positions(
        self,
        *,
        book_id: int,
        base_commodity_id: int,
        as_of_date: date | None,
    ) -> list[dict[str, object]]:
        return [
            {
                "account_id": 4,
                "account_name": "Brokerage",
                "account_type": "investment",
                "commodity_id": 2,
                "commodity_name": "Acme Corp",
                "commodity_scale": 4,
                "balance_minor": 60000,
                "value_minor": 180000,
                "price_missing": False,
                "lots": [
                    {
                        "lot_id": 1,
                        "opened_date": date(2026, 5, 10),
                        "quantity_minor": 60000,
                        "cost_basis_minor": 250000,
                        "remaining_cost_basis_minor": 150000,
                        "converted_value_minor": 180000,
                        "converted_cost_basis_minor": 150000,
                        "price_missing": False,
                    }
                ],
            }
        ]

    async def list_lots_with_holding_period(
        self,
        *,
        book_id: int,
        account_id: int | None,
        commodity_id: int | None,
        as_of_date: date | None,
    ) -> list[dict[str, object]]:
        return [
            {
                "lot_id": 1,
                "account_id": 4,
                "account_name": "Brokerage",
                "commodity_id": 2,
                "commodity_name": "Acme Corp",
                "opened_date": date(2026, 5, 10),
                "quantity_minor": 60000,
                "cost_basis_minor": 250000,
                "commodity_scale": 4,
                "holding_days": 401,
                "is_long_term": True,
            }
        ]

    async def report_realized_gains(
        self,
        *,
        book_id: int,
        date_from: date | None,
        date_to: date | None,
        cost_basis_profile_id: int | None,
    ) -> list[dict[str, object]]:
        return [
            {
                "tx_id": 3,
                "txn_date": date(2026, 6, 10),
                "commodity_id": 2,
                "quantity_minor": 40000,
                "proceeds_minor": 120000,
                "quote_commodity_id": 1,
                "cost_basis_minor": 100000,
                "gain_loss_minor": 20000,
                "proceeds_missing": False,
            }
        ]

    async def report_unrealized_gains(
        self,
        *,
        book_id: int,
        base_commodity_id: int,
        as_of_date: date | None,
        cost_basis_profile_id: int | None,
    ) -> list[dict[str, object]]:
        return [
            {
                "account_id": 4,
                "account_name": "Brokerage",
                "account_type": "investment",
                "commodity_id": 2,
                "commodity_name": "Acme Corp",
                "value_minor": 180000,
                "cost_basis_minor": 150000,
                "unrealized_gain_minor": 30000,
                "price_missing": False,
            }
        ]


@pytest.mark.asyncio
async def test_investment_service_maps_positions_lots_and_gains() -> None:
    service = InvestmentService(StubInvestmentRepository())

    buy = await service.create_buy(
        BuyCommodityInput(
            txn_date=date(2026, 5, 10),
            commodity_id=2,
            investment_account_id=4,
            cash_account_id=2,
            quantity_minor=60000,
            cash_amount_minor=250000,
        )
    )
    sell = await service.create_sell(
        SellCommodityInput(
            txn_date=date(2026, 6, 10),
            commodity_id=2,
            investment_account_id=4,
            cash_account_id=2,
            quantity_minor=40000,
            cash_amount_minor=120000,
            lot_strategy="fifo",
        )
    )
    dividend = await service.create_dividend(
        DividendInput(
            txn_date=date(2026, 6, 20),
            cash_account_id=2,
            income_account_id=5,
            amount_minor=5000,
        )
    )
    positions = await service.list_positions(PositionsQuery(book_id=1))
    converted = await service.convert_positions(ConvertedPositionsQuery(book_id=1, base_commodity_id=1))
    lots = await service.list_lots_with_holding_period(LotsHoldingQuery(book_id=1))
    realized = await service.report_realized_gains(RealizedGainsQuery(book_id=1))
    unrealized = await service.report_unrealized_gains(UnrealizedGainsQuery(book_id=1, base_commodity_id=1))

    assert buy.transaction_id == 10
    assert buy.lot_id == 8
    assert sell.allocations[0].lot_id == 1
    assert dividend.transaction_id == 12
    assert positions[0].commodity_name == "Acme Corp"
    assert positions[0].lots[0].remaining_cost_basis_minor == 150000
    assert converted[0].value_minor == 180000
    assert lots[0].is_long_term is True
    assert realized[0].gain_loss_minor == 20000
    assert unrealized[0].unrealized_gain_minor == 30000
