from datetime import UTC, date, datetime
from types import SimpleNamespace

import pytest

from rekenraam_api.schemas.investments import (
    BuyCommodityInput,
    ConvertedPositionsQuery,
    CorporateActionCreateInput,
    DividendIncomeCategoryCreateInput,
    DividendInput,
    LotsHoldingQuery,
    PositionsQuery,
    RealizedGainsQuery,
    SellCommodityInput,
    UnrealizedGainsQuery,
)
from rekenraam_api.services.investments import InvestmentService


class StubInvestmentRepository:
    def __init__(self) -> None:
        self.stock_split_calls = 0
        self.record_only_calls = 0

    async def create_stock_split_corporate_action(self, **values: object) -> object:
        self.stock_split_calls += 1
        return SimpleNamespace(
            id=15,
            generated_transaction_id=99,
            created_at=datetime(2026, 1, 1, tzinfo=UTC),
            **values,
        )

    async def create_corporate_action(self, **values: object) -> object:
        self.record_only_calls += 1
        return SimpleNamespace(
            id=16,
            generated_transaction_id=None,
            created_at=datetime(2026, 1, 1, tzinfo=UTC),
            **values,
        )

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

    async def list_dividend_income_categories(
        self, *, book_id: int, commodity_id: int | None = None
    ) -> list[SimpleNamespace]:
        return [
            SimpleNamespace(
                id=31,
                previous_dividend_income_category_id=None,
                book_id=book_id,
                commodity_id=commodity_id,
                income_account_id=5,
                category_id=7,
                tax_withheld_account_id=6,
                tax_withheld_category_id=8,
                default_tax_withheld_minor=150,
                withholding_rate_bps=1500,
                tax_country_code="USA",
                tax_treatment="foreign_tax_credit_candidate",
                notes="Treaty withholding",
                metadata_json=None,
                effective_from=None,
                effective_to=None,
                created_at=datetime(2026, 1, 1, tzinfo=UTC),
                updated_at=datetime(2026, 1, 1, tzinfo=UTC),
            )
        ]

    async def create_dividend_income_category(self, **values: object) -> SimpleNamespace:
        return SimpleNamespace(
            id=32,
            previous_dividend_income_category_id=None,
            created_at=datetime(2026, 1, 1, tzinfo=UTC),
            updated_at=datetime(2026, 1, 1, tzinfo=UTC),
            **values,
        )

    async def update_dividend_income_category(
        self, dividend_income_category_id: int, **values: object
    ) -> SimpleNamespace | None:
        if dividend_income_category_id != 31:
            return None
        return SimpleNamespace(
            id=33,
            previous_dividend_income_category_id=31,
            created_at=datetime(2026, 1, 1, tzinfo=UTC),
            updated_at=datetime(2026, 1, 1, tzinfo=UTC),
            **values,
        )

    async def get_dividend_income_category(
        self, dividend_income_category_id: int
    ) -> SimpleNamespace | None:
        if dividend_income_category_id != 31:
            return None
        return SimpleNamespace(id=31, book_id=1)

    async def delete_dividend_income_category(self, dividend_income_category_id: int) -> bool:
        return dividend_income_category_id == 31

    async def list_positions(
        self, *, book_id: int, as_of_date: date | None
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
    converted = await service.convert_positions(
        ConvertedPositionsQuery(book_id=1, base_commodity_id=1)
    )
    lots = await service.list_lots_with_holding_period(LotsHoldingQuery(book_id=1))
    realized = await service.report_realized_gains(RealizedGainsQuery(book_id=1))
    unrealized = await service.report_unrealized_gains(
        UnrealizedGainsQuery(book_id=1, base_commodity_id=1)
    )

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


@pytest.mark.asyncio
async def test_investment_service_manages_dividend_income_categories() -> None:
    service = InvestmentService(StubInvestmentRepository())

    rows = await service.list_dividend_income_categories(1, commodity_id=2)
    created = await service.create_dividend_income_category(
        DividendIncomeCategoryCreateInput(
            book_id=1,
            commodity_id=2,
            income_account_id=5,
            category_id=7,
            tax_withheld_account_id=6,
            tax_withheld_category_id=8,
            default_tax_withheld_minor=150,
            withholding_rate_bps=1500,
            tax_country_code="USA",
            tax_treatment="foreign_tax_credit_candidate",
        )
    )
    deleted = await service.delete_dividend_income_category(31)

    assert rows[0].category_id == 7
    assert created.id == 32
    assert deleted is True

    with pytest.raises(ValueError, match="withholding rate must be between 0 and 10000 bps"):
        await service.create_dividend_income_category(
            DividendIncomeCategoryCreateInput(
                book_id=1,
                income_account_id=5,
                category_id=7,
                withholding_rate_bps=10001,
            )
        )


@pytest.mark.asyncio
async def test_investment_service_dispatches_stock_splits_to_rewrite_path() -> None:
    repository = StubInvestmentRepository()
    service = InvestmentService(repository)

    split = await service.create_corporate_action(
        CorporateActionCreateInput(
            action_type="split",
            old_instrument_id=7,
            effective_date=date(2026, 2, 1),
            ratio_numerator=2,
            ratio_denominator=1,
        )
    )
    spin_off = await service.create_corporate_action(
        CorporateActionCreateInput(
            action_type="spin_off",
            old_instrument_id=7,
            effective_date=date(2026, 3, 1),
            ratio_numerator=1,
            ratio_denominator=1,
        )
    )

    assert split.generated_transaction_id == 99
    assert spin_off.generated_transaction_id is None
    assert repository.stock_split_calls == 1
    assert repository.record_only_calls == 1


@pytest.mark.asyncio
async def test_investment_service_rejects_invalid_stock_split_ratios() -> None:
    service = InvestmentService(StubInvestmentRepository())

    with pytest.raises(ValueError, match="split ratio must not be 1:1"):
        await service.create_corporate_action(
            CorporateActionCreateInput(
                action_type="split",
                old_instrument_id=7,
                effective_date=date(2026, 2, 1),
                ratio_numerator=1,
                ratio_denominator=1,
            )
        )

    with pytest.raises(ValueError, match="cash-in-lieu is not supported"):
        await service.create_corporate_action(
            CorporateActionCreateInput(
                action_type="split",
                old_instrument_id=7,
                effective_date=date(2026, 2, 1),
                ratio_numerator=2,
                ratio_denominator=1,
                cash_in_lieu_minor=50,
            )
        )
