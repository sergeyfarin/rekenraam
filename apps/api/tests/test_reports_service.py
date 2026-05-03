from datetime import date

import pytest

from rekenraam_api.schemas.reports import CashflowReportInput, CategorySpendReportInput, PayeeTotalsReportInput
from rekenraam_api.services.reports import ReportService


class StubReportRepository:
    async def report_cashflow(self, *, book_id: int, date_from: date | None, date_to: date | None, group_by: str) -> list[tuple[str, int, int, int]]:
        return [("2026-05-01", 500000, 500000, 0)]

    async def report_category_spend(self, *, book_id: int, date_from: date | None, date_to: date | None, category_ids: tuple[int, ...] | None) -> list[tuple[int, str, int]]:
        return [(1, "Groceries", -1250)]

    async def report_payee_totals(self, *, book_id: int, date_from: date | None, date_to: date | None, payee_ids: tuple[int, ...] | None) -> list[tuple[int, str, int]]:
        return [(1, "Local Market", -1250)]


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