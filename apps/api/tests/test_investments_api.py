from collections.abc import AsyncIterator
from datetime import UTC, date, datetime

import pytest
from httpx import ASGITransport, AsyncClient

from rekenraam_api.api.dependencies import get_investment_service, require_request_context
from rekenraam_api.app import app
from rekenraam_api.schemas.investments import (
    ConvertedPosition,
    CorporateActionSummary,
    DividendIncomeCategorySummary,
    DividendResult,
    LotHoldingPeriod,
    Position,
    PositionLot,
    RealizedGainEntry,
    TradeAllocation,
    TradeResult,
    UnrealizedGainEntry,
)
from rekenraam_api.services.request_context import RequestContext


class StubInvestmentService:
    async def create_corporate_action(self, input: object) -> CorporateActionSummary:
        if getattr(input, "ratio_numerator", None) == getattr(input, "ratio_denominator", None):
            raise ValueError("split ratio must not be 1:1")
        return CorporateActionSummary(
            id=20,
            book_id=1,
            action_type="split",
            old_instrument_id=2,
            new_instrument_id=None,
            effective_date=date(2026, 2, 1),
            ratio_numerator=2,
            ratio_denominator=1,
            cash_in_lieu_minor=None,
            cash_commodity_id=None,
            source_reference=None,
            memo="2-for-1 split",
            metadata_json=None,
            generated_transaction_id=99,
            created_at=datetime(2026, 2, 1, tzinfo=UTC),
        )

    async def create_buy(self, input: object) -> TradeResult:
        return TradeResult(
            transaction_id=10,
            allocations=(TradeAllocation(lot_id=8, quantity_minor=60000),),
            lot_id=8,
        )

    async def create_sell(self, input: object) -> TradeResult:
        return TradeResult(
            transaction_id=11,
            allocations=(TradeAllocation(lot_id=1, quantity_minor=40000),),
            lot_id=None,
        )

    async def create_dividend(self, input: object) -> DividendResult:
        return DividendResult(transaction_id=12)

    async def list_dividend_income_categories(
        self, book_id: int, commodity_id: int | None = None
    ) -> list[DividendIncomeCategorySummary]:
        return [
            DividendIncomeCategorySummary(
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

    async def create_dividend_income_category(self, input: object) -> DividendIncomeCategorySummary:
        return DividendIncomeCategorySummary(
            id=32,
            previous_dividend_income_category_id=None,
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
            notes=None,
            metadata_json=None,
            effective_from=None,
            effective_to=None,
            created_at=datetime(2026, 1, 1, tzinfo=UTC),
            updated_at=datetime(2026, 1, 1, tzinfo=UTC),
        )

    async def update_dividend_income_category(
        self, dividend_income_category_id: int, input: object
    ) -> DividendIncomeCategorySummary | None:
        if dividend_income_category_id != 31:
            return None
        return DividendIncomeCategorySummary(
            id=33,
            previous_dividend_income_category_id=31,
            book_id=1,
            commodity_id=2,
            income_account_id=5,
            category_id=7,
            tax_withheld_account_id=None,
            tax_withheld_category_id=None,
            default_tax_withheld_minor=None,
            withholding_rate_bps=None,
            tax_country_code=None,
            tax_treatment=None,
            notes="Updated",
            metadata_json=None,
            effective_from=None,
            effective_to=None,
            created_at=datetime(2026, 1, 1, tzinfo=UTC),
            updated_at=datetime(2026, 1, 1, tzinfo=UTC),
        )

    async def delete_dividend_income_category(self, dividend_income_category_id: int) -> bool:
        return dividend_income_category_id == 31

    async def list_positions(self, input: object) -> list[Position]:
        return [
            Position(
                account_id=4,
                account_name="Brokerage",
                account_type="investment",
                commodity_id=2,
                commodity_name="Acme Corp",
                commodity_scale=4,
                balance_minor=60000,
                lots=(
                    PositionLot(
                        lot_id=1,
                        opened_date=date(2026, 5, 10),
                        quantity_minor=60000,
                        cost_basis_minor=250000,
                        remaining_cost_basis_minor=150000,
                    ),
                ),
            )
        ]

    async def convert_positions(self, input: object) -> list[ConvertedPosition]:
        return [
            ConvertedPosition(
                account_id=4,
                account_name="Brokerage",
                account_type="investment",
                commodity_id=2,
                commodity_name="Acme Corp",
                commodity_scale=4,
                balance_minor=60000,
                value_minor=180000,
                price_missing=False,
                lots=(
                    PositionLot(
                        lot_id=1,
                        opened_date=date(2026, 5, 10),
                        quantity_minor=60000,
                        cost_basis_minor=250000,
                        remaining_cost_basis_minor=150000,
                        converted_value_minor=180000,
                        converted_cost_basis_minor=150000,
                        price_missing=False,
                    ),
                ),
            )
        ]

    async def list_lots_with_holding_period(self, input: object) -> list[LotHoldingPeriod]:
        return [
            LotHoldingPeriod(
                lot_id=1,
                account_id=4,
                account_name="Brokerage",
                commodity_id=2,
                commodity_name="Acme Corp",
                opened_date=date(2026, 5, 10),
                quantity_minor=60000,
                cost_basis_minor=250000,
                commodity_scale=4,
                holding_days=401,
                is_long_term=True,
            )
        ]

    async def report_realized_gains(self, input: object) -> list[RealizedGainEntry]:
        return [
            RealizedGainEntry(
                tx_id=3,
                txn_date=date(2026, 6, 10),
                commodity_id=2,
                quantity_minor=40000,
                proceeds_minor=120000,
                quote_commodity_id=1,
                cost_basis_minor=100000,
                gain_loss_minor=20000,
                proceeds_missing=False,
            )
        ]

    async def report_unrealized_gains(self, input: object) -> list[UnrealizedGainEntry]:
        return [
            UnrealizedGainEntry(
                account_id=4,
                account_name="Brokerage",
                account_type="investment",
                commodity_id=2,
                commodity_name="Acme Corp",
                value_minor=180000,
                cost_basis_minor=150000,
                unrealized_gain_minor=30000,
                price_missing=False,
            )
        ]


@pytest.fixture(autouse=True)
def clear_dependency_overrides() -> AsyncIterator[None]:
    async def stub_request_context() -> RequestContext:
        return RequestContext(
            user_id=1,
            session_id=1,
            device_id=1,
            request_id="test-request",
            request_timestamp=datetime(2026, 5, 3, tzinfo=UTC),
        )

    app.dependency_overrides.clear()
    app.dependency_overrides[require_request_context] = stub_request_context
    yield
    app.dependency_overrides.clear()


@pytest.fixture()
async def client() -> AsyncIterator[AsyncClient]:
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://testserver") as async_client:
        yield async_client


@pytest.mark.asyncio
async def test_investment_endpoints_return_positions_lots_and_gains(client: AsyncClient) -> None:
    app.dependency_overrides[get_investment_service] = lambda: StubInvestmentService()

    buy_response = await client.post(
        "/api/v1/investments/buy",
        json={
            "txn_date": "2026-05-10",
            "commodity_id": 2,
            "investment_account_id": 4,
            "cash_account_id": 2,
            "quantity_minor": 60000,
            "cash_amount_minor": 250000,
        },
    )
    sell_response = await client.post(
        "/api/v1/investments/sell",
        json={
            "txn_date": "2026-06-10",
            "commodity_id": 2,
            "investment_account_id": 4,
            "cash_account_id": 2,
            "quantity_minor": 40000,
            "cash_amount_minor": 120000,
            "lot_strategy": "fifo",
        },
    )
    dividend_response = await client.post(
        "/api/v1/investments/dividend",
        json={
            "txn_date": "2026-06-20",
            "cash_account_id": 2,
            "income_account_id": 5,
            "amount_minor": 5000,
        },
    )
    dividend_defaults_response = await client.get(
        "/api/v1/investments/dividend-income-categories?commodity_id=2"
    )
    create_dividend_default_response = await client.post(
        "/api/v1/investments/dividend-income-categories",
        json={
            "book_id": 1,
            "commodity_id": 2,
            "income_account_id": 5,
            "category_id": 7,
            "tax_withheld_account_id": 6,
            "tax_withheld_category_id": 8,
            "default_tax_withheld_minor": 150,
            "withholding_rate_bps": 1500,
            "tax_country_code": "USA",
            "tax_treatment": "foreign_tax_credit_candidate",
        },
    )
    update_dividend_default_response = await client.put(
        "/api/v1/investments/dividend-income-categories/31",
        json={
            "book_id": 1,
            "commodity_id": 2,
            "income_account_id": 5,
            "category_id": 7,
            "notes": "Updated",
        },
    )
    delete_dividend_default_response = await client.delete(
        "/api/v1/investments/dividend-income-categories/31"
    )
    positions_response = await client.get("/api/v1/investments/positions")
    converted_response = await client.get(
        "/api/v1/investments/positions/converted?base_commodity_id=1"
    )
    lots_response = await client.get("/api/v1/investments/lots")
    realized_response = await client.get("/api/v1/reports/realized-gains")
    unrealized_response = await client.get("/api/v1/reports/unrealized-gains?base_commodity_id=1")

    assert buy_response.status_code == 200
    assert buy_response.json()["lot_id"] == 8
    assert sell_response.status_code == 200
    assert sell_response.json()["allocations"][0]["lot_id"] == 1
    assert dividend_response.status_code == 200
    assert dividend_response.json()["transaction_id"] == 12
    assert dividend_defaults_response.status_code == 200
    assert dividend_defaults_response.json()[0]["category_id"] == 7
    assert create_dividend_default_response.status_code == 200
    assert create_dividend_default_response.json()["id"] == 32
    assert update_dividend_default_response.status_code == 200
    assert update_dividend_default_response.json()["previous_dividend_income_category_id"] == 31
    assert delete_dividend_default_response.status_code == 204
    assert positions_response.status_code == 200
    assert positions_response.json()[0]["commodity_name"] == "Acme Corp"
    assert converted_response.status_code == 200
    assert converted_response.json()[0]["value_minor"] == 180000
    assert lots_response.status_code == 200
    assert lots_response.json()[0]["is_long_term"] is True
    assert realized_response.status_code == 200
    assert realized_response.json()[0]["gain_loss_minor"] == 20000
    assert unrealized_response.status_code == 200
    assert unrealized_response.json()[0]["unrealized_gain_minor"] == 30000


@pytest.mark.asyncio
async def test_corporate_action_endpoint_returns_generated_split_transaction(
    client: AsyncClient,
) -> None:
    app.dependency_overrides[get_investment_service] = lambda: StubInvestmentService()

    response = await client.post(
        "/api/v1/investments/corporate-actions",
        json={
            "action_type": "split",
            "old_instrument_id": 2,
            "effective_date": "2026-02-01",
            "ratio_numerator": 2,
            "ratio_denominator": 1,
        },
    )

    assert response.status_code == 200
    assert response.json()["generated_transaction_id"] == 99


@pytest.mark.asyncio
async def test_corporate_action_endpoint_returns_400_for_invalid_split(
    client: AsyncClient,
) -> None:
    app.dependency_overrides[get_investment_service] = lambda: StubInvestmentService()

    response = await client.post(
        "/api/v1/investments/corporate-actions",
        json={
            "action_type": "split",
            "old_instrument_id": 2,
            "effective_date": "2026-02-01",
            "ratio_numerator": 1,
            "ratio_denominator": 1,
        },
    )

    assert response.status_code == 400
    assert response.json() == {"detail": "split ratio must not be 1:1"}
