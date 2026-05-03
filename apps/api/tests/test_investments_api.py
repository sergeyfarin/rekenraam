from collections.abc import AsyncIterator
from datetime import UTC, date, datetime

import pytest
from httpx import ASGITransport, AsyncClient

from rekenraam_api.api.dependencies import get_investment_service
from rekenraam_api.app import app
from rekenraam_api.schemas.investments import ConvertedPosition, LotHoldingPeriod, Position, PositionLot, RealizedGainEntry, UnrealizedGainEntry


class StubInvestmentService:
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
    app.dependency_overrides.clear()
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

    positions_response = await client.get("/api/v1/investments/positions")
    converted_response = await client.get("/api/v1/investments/positions/converted?base_commodity_id=1")
    lots_response = await client.get("/api/v1/investments/lots")
    realized_response = await client.get("/api/v1/reports/realized-gains")
    unrealized_response = await client.get("/api/v1/reports/unrealized-gains?base_commodity_id=1")

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
