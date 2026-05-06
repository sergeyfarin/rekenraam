from datetime import UTC, date, datetime

import pytest

from rekenraam_api.schemas.reports import (
    CashflowReportInput,
    CategorySpendReportInput,
    PayeeTotalsReportInput,
    ReportDefinitionCreateInput,
    ReportRunCreateInput,
)
from rekenraam_api.services.reports import ReportService


class StubReportRepository:
    _created_at = datetime(2026, 5, 3, tzinfo=UTC)

    def __init__(self) -> None:
        self.change_seq = 0
        self.cached_reports: dict[tuple[int, str, str, int], str] = {}
        self.cashflow_calls = 0
        self.category_calls = 0
        self.payee_calls = 0

    async def report_cashflow(self, *, book_id: int, date_from: date | None, date_to: date | None, group_by: str) -> list[tuple[str, int, int, int]]:
        self.cashflow_calls += 1
        return [("2026-05-01", 500000, 500000, 0)]

    async def report_category_spend(self, *, book_id: int, date_from: date | None, date_to: date | None, category_ids: tuple[int, ...] | None) -> list[tuple[int, str, int]]:
        self.category_calls += 1
        return [(1, "Groceries", -1250)]

    async def report_payee_totals(self, *, book_id: int, date_from: date | None, date_to: date | None, payee_ids: tuple[int, ...] | None) -> list[tuple[int, str, int]]:
        self.payee_calls += 1
        return [(1, "Local Market", -1250)]

    async def get_book_change_seq(self, book_id: int) -> int:
        return self.change_seq

    async def bump_book_change_seq(self, book_id: int) -> int:
        self.change_seq += 1
        return self.change_seq

    async def get_cached_report(self, *, book_id: int, report_type: str, params_hash: str, as_of_seq: int) -> object | None:
        payload_json = self.cached_reports.get((book_id, report_type, params_hash, as_of_seq))
        if payload_json is None:
            return None
        return type("CacheRow", (), {"payload_json": payload_json})()

    async def save_cached_report(self, *, book_id: int, report_type: str, params_hash: str, params_json: str, as_of_seq: int, payload_json: str) -> object:
        self.cached_reports[(book_id, report_type, params_hash, as_of_seq)] = payload_json
        return type("CacheRow", (), {"payload_json": payload_json})()

    async def list_report_definitions(self, book_id: int) -> list[object]:
        return [type("Row", (), {"id": 1, "book_id": 1, "name": "Cashflow", "kind": "custom", "query_type": "sql", "query_text": "SELECT 1", "params_schema": None, "created_at": self._created_at})()]

    async def get_report_definition(self, definition_id: int) -> object | None:
        if definition_id != 1:
            return None
        return type("Row", (), {"id": 1, "book_id": 1, "name": "Cashflow", "kind": "custom", "query_type": "sql", "query_text": "SELECT 1", "params_schema": None, "created_at": self._created_at})()

    async def create_report_definition(self, **kwargs: object) -> object:
        return type("Row", (), {"id": 2, "created_at": self._created_at, **kwargs})()

    async def update_report_definition(self, **kwargs: object) -> object | None:
        if kwargs["definition_id"] != 1:
            return None
        data = dict(kwargs)
        data.pop("definition_id")
        data["id"] = 3
        data["created_at"] = self._created_at
        return type("Row", (), data | {"previous_report_definition_id": 1})()

    async def delete_report_definition(self, definition_id: int) -> bool:
        return definition_id == 1

    async def list_report_runs(self, book_id: int, definition_id: int | None = None) -> list[object]:
        return [type("Row", (), {"id": 4, "book_id": 1, "definition_id": 1, "params_hash": "abc123", "as_of_seq": 0, "pricing_mode": "latest_corrected", "pricing_policy_id": None, "pricing_policy_version": None, "valuation_snapshot_id": None, "pricing_resolved_at": None, "result_json": "[]", "created_at": self._created_at})()]

    async def create_report_run(self, **kwargs: object) -> object:
        return type("Row", (), {"id": 5, "created_at": self._created_at, **kwargs})()


@pytest.mark.asyncio
async def test_report_service_maps_rows() -> None:
    service = ReportService(StubReportRepository())

    cashflow = await service.report_cashflow(CashflowReportInput(book_id=1, group_by="month"))
    category_spend = await service.report_category_spend(CategorySpendReportInput(book_id=1))
    payee_totals = await service.report_payee_totals(PayeeTotalsReportInput(book_id=1))

    assert cashflow[0].period_start == date(2026, 5, 1)
    assert cashflow[0].net_minor == 0
    assert category_spend[0].category_name == "Groceries"
    assert payee_totals[0].payee_name == "Local Market"


@pytest.mark.asyncio
async def test_report_service_uses_report_cache_until_change_seq_bumps() -> None:
    repository = StubReportRepository()
    service = ReportService(repository)
    input = CashflowReportInput(book_id=1, group_by="month")

    first = await service.report_cashflow(input)
    second = await service.report_cashflow(input)
    await service.bump_report_state(1)
    third = await service.report_cashflow(input)

    assert first[0].period_start == date(2026, 5, 1)
    assert second[0].period_start == date(2026, 5, 1)
    assert third[0].period_start == date(2026, 5, 1)
    assert repository.cashflow_calls == 2


@pytest.mark.asyncio
async def test_report_service_manages_definition_and_run_metadata() -> None:
    service = ReportService(StubReportRepository())

    created_definition = await service.create_report_definition(
        ReportDefinitionCreateInput(book_id=1, name="Cashflow", kind="custom", query_type="sql", query_text="SELECT 1")
    )
    definitions = await service.list_report_definitions(1)
    created_run = await service.create_report_run(
        ReportRunCreateInput(book_id=1, definition_id=1, params_hash="abc123", as_of_seq=0, result_json="[]")
    )

    assert created_definition.id == 2
    assert definitions[0].name == "Cashflow"
    assert created_run.id == 5


@pytest.mark.asyncio
async def test_report_service_rejects_invalid_report_metadata() -> None:
    service = ReportService(StubReportRepository())

    with pytest.raises(ValueError):
        await service.create_report_definition(
            ReportDefinitionCreateInput(book_id=1, name="Bad", kind="wrong", query_type="sql", query_text="SELECT 1")
        )

    with pytest.raises(ValueError):
        await service.create_report_run(
            ReportRunCreateInput(book_id=1, definition_id=1, params_hash="abc123", as_of_seq=0, pricing_mode="wrong", result_json="[]")
        )