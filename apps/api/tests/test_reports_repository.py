from datetime import date

import pytest
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.repositories.reports import ReportRepository


@pytest.mark.asyncio
async def test_report_repository_returns_cashflow_category_spend_and_payee_totals(repository_session: AsyncSession) -> None:
    repository = ReportRepository(repository_session)

    cashflow = await repository.report_cashflow(book_id=1, date_from=None, date_to=None, group_by="month")
    category_spend = await repository.report_category_spend(book_id=1, date_from=None, date_to=None, category_ids=None)
    payee_totals = await repository.report_payee_totals(book_id=1, date_from=None, date_to=None, payee_ids=None)

    assert cashflow[0][0] == "2026-05-01"
    assert cashflow[0][3] == 0
    assert category_spend == []
    assert payee_totals == []