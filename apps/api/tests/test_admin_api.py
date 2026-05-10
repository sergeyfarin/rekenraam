from datetime import date

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


async def _bootstrap_admin(client: AsyncClient) -> None:
    response = await client.post(
        "/api/v1/auth/bootstrap/admin",
        json={
            "email": "admin-api@example.test",
            "password": "correct horse battery staple",
            "display_name": "Admin",
        },
    )
    assert response.status_code == 200


@pytest.mark.asyncio
async def test_runtime_status_exposes_web_runtime(client: AsyncClient) -> None:
    await _bootstrap_admin(client)
    response = await client.get("/api/v1/admin/runtime")

    assert response.status_code == 200
    payload = response.json()
    assert payload["database_kind"] == "postgresql"
    assert payload["latest_version"] == "0001_initial_schema"
    assert payload["pending_migration_count"] == 0
    assert payload["writable"] is True
    assert "server-managed PostgreSQL" in payload["note"]


@pytest.mark.asyncio
async def test_integrity_check_returns_ok(client: AsyncClient) -> None:
    await _bootstrap_admin(client)
    response = await client.post("/api/v1/admin/integrity-check", json={})

    assert response.status_code == 200
    payload = response.json()
    assert payload["status"] == "ok"
    assert {check["name"] for check in payload["checks"]} >= {
        "database_connectivity",
        "migrations",
        "writable",
        "double_entry_balance",
    }


@pytest.mark.asyncio
async def test_fiscal_year_close_creates_balancing_transaction(
    client: AsyncClient,
    repository_session: AsyncSession,
) -> None:
    await _bootstrap_admin(client)
    commodity_id = await repository_session.scalar(
        select(Commodity.id).where(Commodity.book_id == 1).where(Commodity.symbol == "USD").limit(1)
    )
    assert commodity_id is not None

    asset_account_response = await client.post(
        "/api/v1/accounts",
        json={
            "book_id": 1,
            "parent_id": None,
            "account_type": "asset",
            "name": "FY Close Asset",
            "commodity_id": commodity_id,
            "institution_id": None,
            "country_id": None,
            "number_last4": None,
            "is_closed": False,
        },
    )
    income_account_response = await client.post(
        "/api/v1/accounts",
        json={
            "book_id": 1,
            "parent_id": None,
            "account_type": "income",
            "name": "FY Close Income",
            "commodity_id": commodity_id,
            "institution_id": None,
            "country_id": None,
            "number_last4": None,
            "is_closed": False,
        },
    )
    assert asset_account_response.status_code == 200
    assert income_account_response.status_code == 200
    asset_account_id = asset_account_response.json()["id"]
    income_account_id = income_account_response.json()["id"]

    transaction_payload = {
        "book_id": 1,
        "txn_date": "2026-01-15",
        "payee_id": None,
        "memo": "Year close seed",
        "status": "cleared",
        "reference": None,
        "import_id": None,
        "splits": [
            {
                "account_id": asset_account_id,
                "commodity_id": 1,
                "amount_minor": -5000,
                "category_id": None,
                "tag_id": None,
                "person_id": None,
                "project_id": None,
                "share_bps": None,
                "memo": None,
            },
            {
                "account_id": income_account_id,
                "commodity_id": 1,
                "amount_minor": 5000,
                "category_id": None,
                "tag_id": None,
                "person_id": None,
                "project_id": None,
                "share_bps": None,
                "memo": None,
            },
        ],
    }
    transaction_response = await client.post("/api/v1/transactions", json=transaction_payload)
    assert transaction_response.status_code == 200

    close_response = await client.post(
        "/api/v1/admin/fiscal-year-close",
        json={"close_date": date(2026, 1, 31).isoformat(), "memo": "Close FY 2025"},
    )

    assert close_response.status_code == 200
    payload = close_response.json()
    assert payload["tx_id"] is not None
    assert payload["closed_accounts_count"] == 1
    assert payload["retained_earnings_delta_minor"] == 5000

    duplicate_response = await client.post(
        "/api/v1/admin/fiscal-year-close",
        json={"close_date": date(2026, 1, 31).isoformat(), "memo": "Close FY 2025"},
    )
    assert duplicate_response.status_code == 409
