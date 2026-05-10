from collections.abc import AsyncIterator, Iterator
from datetime import UTC, datetime

import pytest
from httpx import ASGITransport, AsyncClient

from rekenraam_api.api.dependencies import (
    get_investment_service,
    get_report_service,
    require_request_context,
)
from rekenraam_api.app import app
from rekenraam_api.schemas.investments import RealizedGainEntry, UnrealizedGainEntry
from rekenraam_api.schemas.reports import ReportDefinitionSummary, ReportRunSummary
from rekenraam_api.services.request_context import RequestContext


class StubReportService:
    _created_at = datetime(2026, 5, 3, tzinfo=UTC)

    async def list_report_definitions(self, book_id: int) -> list[ReportDefinitionSummary]:
        return [
            ReportDefinitionSummary(
                id=1,
                book_id=book_id,
                name="Cashflow",
                kind="custom",
                query_type="sql",
                query_text="SELECT 1",
                params_schema=None,
                created_at=self._created_at,
            )
        ]

    async def get_report_definition(self, definition_id: int) -> ReportDefinitionSummary | None:
        if definition_id != 1:
            return None
        return ReportDefinitionSummary(
            id=1,
            book_id=1,
            name="Cashflow",
            kind="custom",
            query_type="sql",
            query_text="SELECT 1",
            params_schema=None,
            created_at=self._created_at,
        )

    async def create_report_definition(self, input: object) -> ReportDefinitionSummary:
        return ReportDefinitionSummary(
            id=2,
            book_id=1,
            name="Cashflow v2",
            kind="custom",
            query_type="template",
            query_text='{"sql":"SELECT 1"}',
            params_schema=None,
            created_at=self._created_at,
        )

    async def update_report_definition(self, input: object) -> ReportDefinitionSummary | None:
        data = getattr(input, "id", None)
        if data != 1:
            return None
        return ReportDefinitionSummary(
            id=3,
            book_id=1,
            name="Cashflow Updated",
            kind="custom",
            query_type="sql",
            query_text="SELECT 2",
            params_schema=None,
            created_at=self._created_at,
        )

    async def delete_report_definition(self, definition_id: int) -> bool:
        return definition_id == 1

    async def list_report_runs(
        self, book_id: int, definition_id: int | None = None
    ) -> list[ReportRunSummary]:
        return [
            ReportRunSummary(
                id=4,
                book_id=book_id,
                definition_id=definition_id or 1,
                params_hash="abc123",
                as_of_seq=0,
                pricing_mode="latest_corrected",
                pricing_policy_id=None,
                pricing_policy_version=None,
                valuation_snapshot_id=None,
                pricing_resolved_at=None,
                result_json="[]",
                created_at=self._created_at,
            )
        ]

    async def create_report_run(self, input: object) -> ReportRunSummary:
        return ReportRunSummary(
            id=5,
            book_id=1,
            definition_id=1,
            params_hash="abc123",
            as_of_seq=0,
            pricing_mode="latest_corrected",
            pricing_policy_id=None,
            pricing_policy_version=None,
            valuation_snapshot_id=None,
            pricing_resolved_at=None,
            result_json="[]",
            created_at=self._created_at,
        )

    async def report_cashflow(self, input: object) -> list[object]:
        return []

    async def report_category_spend(self, input: object) -> list[object]:
        return []

    async def report_payee_totals(self, input: object) -> list[object]:
        return []


class StubInvestmentService:
    async def report_realized_gains(self, input: object) -> list[RealizedGainEntry]:
        return []

    async def report_unrealized_gains(self, input: object) -> list[UnrealizedGainEntry]:
        return []


@pytest.fixture(autouse=True)
def clear_dependency_overrides() -> Iterator[None]:
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
async def test_report_metadata_endpoints(client: AsyncClient) -> None:
    app.dependency_overrides[get_report_service] = lambda: StubReportService()
    app.dependency_overrides[get_investment_service] = lambda: StubInvestmentService()

    list_definitions_response = await client.get("/api/v1/reports/definitions")
    get_definition_response = await client.get("/api/v1/reports/definitions/1")
    create_definition_response = await client.post(
        "/api/v1/reports/definitions",
        json={
            "book_id": 1,
            "name": "Cashflow v2",
            "kind": "custom",
            "query_type": "template",
            "query_text": '{"sql":"SELECT 1"}',
            "params_schema": None,
        },
    )
    update_definition_response = await client.put(
        "/api/v1/reports/definitions/1",
        json={
            "id": 1,
            "book_id": 1,
            "name": "Cashflow Updated",
            "kind": "custom",
            "query_type": "sql",
            "query_text": "SELECT 2",
            "params_schema": None,
        },
    )
    delete_definition_response = await client.delete("/api/v1/reports/definitions/1")
    list_runs_response = await client.get("/api/v1/reports/runs")
    create_run_response = await client.post(
        "/api/v1/reports/runs",
        json={
            "book_id": 1,
            "definition_id": 1,
            "params_hash": "abc123",
            "as_of_seq": 0,
            "pricing_mode": "latest_corrected",
            "pricing_policy_id": None,
            "pricing_policy_version": None,
            "valuation_snapshot_id": None,
            "pricing_resolved_at": None,
            "result_json": "[]",
        },
    )

    assert list_definitions_response.status_code == 200
    assert list_definitions_response.json()[0]["name"] == "Cashflow"
    assert get_definition_response.status_code == 200
    assert get_definition_response.json()["id"] == 1
    assert create_definition_response.status_code == 200
    assert create_definition_response.json()["id"] == 2
    assert update_definition_response.status_code == 200
    assert update_definition_response.json()["id"] == 3
    assert delete_definition_response.status_code == 204
    assert list_runs_response.status_code == 200
    assert list_runs_response.json()[0]["id"] == 4
    assert create_run_response.status_code == 200
    assert create_run_response.json()["id"] == 5
