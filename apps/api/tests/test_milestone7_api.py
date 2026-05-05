from collections.abc import AsyncIterator, Iterator

import pytest
from httpx import ASGITransport, AsyncClient
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.api.dependencies import get_db_session
from rekenraam_api.app import app


@pytest.fixture(autouse=True)
def clear_dependency_overrides() -> Iterator[None]:
    app.dependency_overrides.clear()
    yield
    app.dependency_overrides.clear()


@pytest.fixture()
async def client(repository_session: AsyncSession) -> AsyncIterator[AsyncClient]:
    async def override_session() -> AsyncIterator[AsyncSession]:
        yield repository_session

    app.dependency_overrides[get_db_session] = override_session
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://testserver") as async_client:
        yield async_client


async def _bootstrap_admin(client: AsyncClient) -> None:
    response = await client.post(
        "/api/v1/auth/bootstrap/admin",
        json={
            "email": "admin7@example.test",
            "password": "correct horse battery staple",
            "display_name": "Admin",
        },
    )
    assert response.status_code == 200


@pytest.mark.asyncio
async def test_budget_schedule_projection_and_loan_workflows(client: AsyncClient) -> None:
    await _bootstrap_admin(client)

    category = await client.post(
        "/api/v1/categories",
        json={
            "book_id": 1,
            "parent_id": None,
            "name": "Housing",
            "kind": "expense",
            "color": None,
        },
    )
    assert category.status_code == 200
    category_id = category.json()["id"]

    budget = await client.post(
        "/api/v1/budgets",
        json={
            "book_id": 1,
            "name": "Monthly plan",
            "period": "monthly",
            "starts_on": "2026-05-01",
            "ends_on": None,
            "commodity_id": 1,
            "is_active": True,
            "targets": [
                {
                    "category_id": category_id,
                    "amount_minor": 100000,
                    "rollover_enabled": True,
                }
            ],
        },
    )
    assert budget.status_code == 200
    budget_id = budget.json()["id"]
    variance = await client.get(f"/api/v1/budgets/{budget_id}/variance?period_start=2026-05-01")
    assert variance.status_code == 200
    assert variance.json()[0]["remaining_minor"] == 100000

    schedule = await client.post(
        "/api/v1/schedules",
        json={
            "book_id": 1,
            "name": "Monthly savings",
            "payee_id": None,
            "memo": "Move cash",
            "status": "uncleared",
            "reference": None,
            "frequency": "monthly",
            "interval": 1,
            "start_date": "2026-05-05",
            "end_date": None,
            "reminder_days": 3,
            "enabled": True,
            "splits": [
                {
                    "account_id": 2,
                    "commodity_id": 1,
                    "amount_minor": -25000,
                    "category_id": None,
                    "memo": "source",
                },
                {
                    "account_id": 3,
                    "commodity_id": 1,
                    "amount_minor": 25000,
                    "category_id": None,
                    "memo": "destination",
                },
            ],
        },
    )
    assert schedule.status_code == 200
    schedule_id = schedule.json()["id"]

    instances = await client.get(
        "/api/v1/schedules/instances?book_id=1&start=2026-05-01&end=2026-07-31"
    )
    assert instances.status_code == 200
    assert [row["occurrence_date"] for row in instances.json()] == [
        "2026-05-05",
        "2026-06-05",
        "2026-07-05",
    ]

    forecast = await client.get(
        "/api/v1/schedules/projected-cash?book_id=1&start=2026-05-01&end=2026-05-31"
    )
    assert forecast.status_code == 200
    assert any(row["scheduled_delta_minor"] == -25000 for row in forecast.json())

    posted = await client.post(f"/api/v1/schedules/{schedule_id}/post?occurrence_date=2026-05-05")
    assert posted.status_code == 200
    assert posted.json()["occurrence"]["status"] == "posted"
    assert posted.json()["transaction"]["memo"] == "Move cash"

    skipped = await client.post(f"/api/v1/schedules/{schedule_id}/skip?occurrence_date=2026-06-05")
    assert skipped.status_code == 200
    assert skipped.json()["status"] == "skipped"

    loan = await client.post(
        "/api/v1/loans",
        json={
            "book_id": 1,
            "account_id": 2,
            "name": "Fixed mortgage",
            "principal_minor": 25000000,
            "annual_rate_bps": 650,
            "term_months": 360,
            "payment_frequency": "monthly",
            "start_date": "2026-05-05",
            "payment_account_id": 2,
            "interest_category_id": category_id,
        },
    )
    assert loan.status_code == 200
    loan_id = loan.json()["id"]
    assert loan.json()["payment_minor"] > 0

    amortization = await client.get(f"/api/v1/loans/{loan_id}/amortization")
    assert amortization.status_code == 200
    assert amortization.json()[0]["interest_minor"] > 0

    draft = await client.post(
        f"/api/v1/loans/{loan_id}/payment-draft",
        json={
            "payment_date": "2026-06-05",
            "payment_account_id": 2,
            "interest_category_id": category_id,
            "memo": None,
        },
    )
    assert draft.status_code == 200
    splits = draft.json()["transaction"]["splits"]
    assert sum(split["amount_minor"] for split in splits) == 0
