import pytest
import pytest_asyncio
from httpx import ASGITransport, AsyncClient
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.api.dependencies import get_db_session
from rekenraam_api.app import app
from rekenraam_api.db.models.metadata import Commodity


@pytest.fixture(autouse=True)
def clear_dependency_overrides() -> None:
    app.dependency_overrides.clear()
    yield
    app.dependency_overrides.clear()


@pytest_asyncio.fixture()
async def client(repository_session: AsyncSession) -> AsyncClient:
    async def override_db_session():
        yield repository_session

    app.dependency_overrides[get_db_session] = override_db_session
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://testserver") as async_client:
        yield async_client


@pytest.mark.asyncio
async def test_commodity_autocomplete_and_manual_fx_observations(
    client: AsyncClient,
    repository_session: AsyncSession,
) -> None:
    usd_id = await repository_session.scalar(
        select(Commodity.id).where(Commodity.book_id == 1).where(Commodity.symbol == "USD").limit(1)
    )
    if usd_id is None:
        usd = Commodity(book_id=1, kind="currency", symbol="USD", name="US Dollar", scale=2, metadata_text=None)
        repository_session.add(usd)
        await repository_session.commit()
        await repository_session.refresh(usd)
        usd_id = usd.id

    eur_id = await repository_session.scalar(
        select(Commodity.id).where(Commodity.book_id == 1).where(Commodity.symbol == "EUR").limit(1)
    )
    if eur_id is None:
        eur = Commodity(book_id=1, kind="currency", symbol="EUR", name="Euro", scale=2, metadata_text=None)
        repository_session.add(eur)
        await repository_session.commit()
        await repository_session.refresh(eur)
        eur_id = eur.id

    autocomplete_response = await client.get("/api/v1/commodities/autocomplete", params={"query": "us", "book_id": 1})
    assert autocomplete_response.status_code == 200
    assert any(option["symbol"] == "USD" for option in autocomplete_response.json())

    create_daily_response = await client.post(
        "/api/v1/pricing/rates/daily",
        json={
            "book_id": 1,
            "from_currency_id": eur_id,
            "to_currency_id": usd_id,
            "rate_date": "2026-05-01",
            "rate": 1.12,
            "source": "manual",
        },
    )
    assert create_daily_response.status_code == 200
    daily_id = create_daily_response.json()["id"]

    list_daily_response = await client.get("/api/v1/pricing/rates/daily")
    assert list_daily_response.status_code == 200
    assert any(rate["id"] == daily_id for rate in list_daily_response.json())

    create_official_response = await client.post(
        "/api/v1/pricing/rates/official",
        json={
            "book_id": 1,
            "from_currency_id": eur_id,
            "to_currency_id": usd_id,
            "period_type": "monthly",
            "period_year": 2026,
            "period_month": 5,
            "rate": 1.10,
            "source_name": "ECB",
            "source_url": None,
            "source_date": None,
            "notes": None,
        },
    )
    assert create_official_response.status_code == 200
    official_id = create_official_response.json()["id"]

    list_official_response = await client.get("/api/v1/pricing/rates/official")
    assert list_official_response.status_code == 200
    assert any(rate["id"] == official_id for rate in list_official_response.json())

    delete_daily_response = await client.delete(f"/api/v1/pricing/rates/daily/{daily_id}")
    delete_official_response = await client.delete(f"/api/v1/pricing/rates/official/{official_id}")
    assert delete_daily_response.status_code == 204
    assert delete_official_response.status_code == 204