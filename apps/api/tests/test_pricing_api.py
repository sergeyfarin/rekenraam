from collections.abc import AsyncIterator
from datetime import UTC, date, datetime

import pytest
from httpx import ASGITransport, AsyncClient

from rekenraam_api.api.dependencies import get_pricing_service, get_pricing_worker
from rekenraam_api.app import app
from rekenraam_api.schemas.pricing import (
    PricingExecutionStatusSummary,
    PriceSourceSummary,
    PricingPolicySummary,
    PricingRefreshRunSummary,
    PricingRefreshStateSummary,
    PricingSourceAssignmentSummary,
)


class StubPricingService:
    _created_at = datetime(2026, 5, 3, tzinfo=UTC)

    async def list_price_sources(self) -> list[PriceSourceSummary]:
        return [PriceSourceSummary(id=1001, name="ECB", kind="provider", country_code=None, website_url="https://ecb.test", notes="ECB", created_at=self._created_at)]

    async def get_pricing_policy(self, book_id: int) -> PricingPolicySummary | None:
        if book_id != 1:
            return None
        return PricingPolicySummary(
            book_id=1,
            base_currency_id=1,
            base_currency_symbol="USD",
            default_source_id=1001,
            default_source_name="ECB",
            refresh_enabled=True,
            refresh_hour_utc=4,
            refresh_minute_utc=0,
            max_backfill_days=30,
            weekend_policy="skip",
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def update_pricing_policy(self, input: object) -> PricingPolicySummary | None:
        return PricingPolicySummary(
            book_id=1,
            base_currency_id=2,
            base_currency_symbol="EUR",
            default_source_id=1001,
            default_source_name="ECB",
            refresh_enabled=True,
            refresh_hour_utc=6,
            refresh_minute_utc=30,
            max_backfill_days=45,
            weekend_policy="fill_previous",
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def list_pricing_source_assignments(self, book_id: int) -> list[PricingSourceAssignmentSummary]:
        return [
            PricingSourceAssignmentSummary(
                id=10,
                book_id=book_id,
                from_currency_id=2,
                from_currency_symbol="EUR",
                to_currency_id=1,
                to_currency_symbol="USD",
                source_id=1001,
                source_name="ECB",
                effective_from=date(2026, 5, 1),
                effective_to=None,
                created_at=self._created_at,
                updated_at=self._created_at,
            )
        ]

    async def create_pricing_source_assignment(self, input: object) -> PricingSourceAssignmentSummary:
        return PricingSourceAssignmentSummary(
            id=11,
            book_id=1,
            from_currency_id=2,
            from_currency_symbol="EUR",
            to_currency_id=1,
            to_currency_symbol="USD",
            source_id=1001,
            source_name="ECB",
            effective_from=date(2026, 5, 1),
            effective_to=None,
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def update_pricing_source_assignment(self, assignment_id: int, input: object) -> PricingSourceAssignmentSummary | None:
        if assignment_id != 10:
            return None
        return PricingSourceAssignmentSummary(
            id=10,
            book_id=1,
            from_currency_id=2,
            from_currency_symbol="EUR",
            to_currency_id=1,
            to_currency_symbol="USD",
            source_id=1005,
            source_name="Federal Reserve",
            effective_from=date(2026, 5, 15),
            effective_to=date(2026, 6, 30),
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def delete_pricing_source_assignment(self, assignment_id: int) -> bool:
        return assignment_id == 10

    async def list_pricing_refresh_state(self, book_id: int) -> list[PricingRefreshStateSummary]:
        return [
            PricingRefreshStateSummary(
                id=5,
                book_id=book_id,
                from_currency_id=2,
                from_currency_symbol="EUR",
                to_currency_id=1,
                to_currency_symbol="USD",
                source_id=1001,
                source_name="ECB",
                last_success_date=date(2026, 5, 20),
                last_attempt_at=self._created_at,
                last_error=None,
                created_at=self._created_at,
                updated_at=self._created_at,
            )
        ]


class StubPricingWorker:
    _created_at = datetime(2026, 5, 3, tzinfo=UTC)

    async def run_book(self, book_id: int, *, trigger: str, force: bool) -> PricingRefreshRunSummary | None:
        if book_id != 1:
            raise ValueError("pricing policy not found")
        return PricingRefreshRunSummary(
            book_id=1,
            trigger=trigger,
            started_at=self._created_at,
            finished_at=self._created_at,
            pairs_total=1,
            pairs_success=1,
            pairs_failed=0,
            rates_inserted=2,
            derived_inserted=0,
            last_error=None,
        )

    async def get_status(self, book_id: int) -> PricingExecutionStatusSummary:
        if book_id != 1:
            raise ValueError("pricing policy not found")
        return PricingExecutionStatusSummary(
            book_id=1,
            scheduler_enabled=True,
            scheduler_poll_seconds=60,
            worker_started_at=self._created_at,
            is_running=False,
            active_book_ids=[],
            next_scheduled_at=datetime(2026, 5, 4, 4, 0, tzinfo=UTC),
            last_run=PricingRefreshRunSummary(
                book_id=1,
                trigger="scheduled",
                started_at=self._created_at,
                finished_at=self._created_at,
                pairs_total=1,
                pairs_success=1,
                pairs_failed=0,
                rates_inserted=2,
                derived_inserted=0,
                last_error=None,
            ),
        )

    async def get_run_history(self, book_id: int, limit: int = 10) -> list[PricingRefreshRunSummary]:
        if book_id != 1:
            raise ValueError("pricing policy not found")
        return [
            PricingRefreshRunSummary(
                book_id=1,
                trigger="manual",
                started_at=self._created_at,
                finished_at=self._created_at,
                pairs_total=1,
                pairs_success=1,
                pairs_failed=0,
                rates_inserted=2,
                derived_inserted=0,
                last_error=None,
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
async def test_pricing_endpoints_return_policy_sources_assignments_and_refresh_state(client: AsyncClient) -> None:
    app.dependency_overrides[get_pricing_service] = lambda: StubPricingService()
    app.dependency_overrides[get_pricing_worker] = lambda: StubPricingWorker()

    sources_response = await client.get("/api/v1/pricing/sources")
    policy_response = await client.get("/api/v1/pricing/policy")
    update_policy_response = await client.put(
        "/api/v1/pricing/policy",
        json={
            "book_id": 1,
            "base_currency_id": 2,
            "default_source_id": 1001,
            "refresh_enabled": True,
            "refresh_hour_utc": 6,
            "refresh_minute_utc": 30,
            "max_backfill_days": 45,
            "weekend_policy": "fill_previous",
        },
    )
    assignments_response = await client.get("/api/v1/pricing/source-assignments")
    create_assignment_response = await client.post(
        "/api/v1/pricing/source-assignments",
        json={
            "book_id": 1,
            "from_currency_id": 2,
            "to_currency_id": 1,
            "source_id": 1001,
            "effective_from": "2026-05-01",
            "effective_to": None,
        },
    )
    update_assignment_response = await client.put(
        "/api/v1/pricing/source-assignments/10",
        json={
            "book_id": 1,
            "from_currency_id": 2,
            "to_currency_id": 1,
            "source_id": 1005,
            "effective_from": "2026-05-15",
            "effective_to": "2026-06-30",
        },
    )
    delete_assignment_response = await client.delete("/api/v1/pricing/source-assignments/10")
    refresh_state_response = await client.get("/api/v1/pricing/refresh-state")
    run_response = await client.post("/api/v1/pricing/refresh/run", json={"book_id": 1})
    execution_status_response = await client.get("/api/v1/pricing/refresh/execution-status")
    history_response = await client.get("/api/v1/pricing/refresh/history")

    assert sources_response.status_code == 200
    assert sources_response.json()[0]["name"] == "ECB"
    assert policy_response.status_code == 200
    assert policy_response.json()["base_currency_symbol"] == "USD"
    assert update_policy_response.status_code == 200
    assert update_policy_response.json()["base_currency_symbol"] == "EUR"
    assert assignments_response.status_code == 200
    assert assignments_response.json()[0]["from_currency_symbol"] == "EUR"
    assert create_assignment_response.status_code == 200
    assert create_assignment_response.json()["id"] == 11
    assert update_assignment_response.status_code == 200
    assert update_assignment_response.json()["source_name"] == "Federal Reserve"
    assert delete_assignment_response.status_code == 204
    assert refresh_state_response.status_code == 200
    assert refresh_state_response.json()[0]["last_success_date"] == "2026-05-20"
    assert run_response.status_code == 200
    assert run_response.json()["pairs_success"] == 1
    assert execution_status_response.status_code == 200
    assert execution_status_response.json()["scheduler_enabled"] is True
    assert history_response.status_code == 200
    assert history_response.json()[0]["trigger"] == "manual"