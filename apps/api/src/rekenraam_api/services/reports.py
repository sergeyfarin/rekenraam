from datetime import date

from rekenraam_api.repositories.reports import ReportRepository
from rekenraam_api.schemas.reports import (
    CashflowReportInput,
    CashflowRow,
    CategorySpendReportInput,
    CategorySpendRow,
    PayeeTotalsReportInput,
    PayeeTotalRow,
)


class ReportService:
    def __init__(self, repository: ReportRepository) -> None:
        self._repository = repository

    async def report_cashflow(self, input: CashflowReportInput) -> list[CashflowRow]:
        group_by = (input.group_by or "month").strip().lower()
        if group_by not in {"day", "month", "quarter", "year"}:
            raise ValueError("group_by must be day, month, quarter, or year")

        rows = await self._repository.report_cashflow(
            book_id=input.book_id,
            date_from=input.date_from,
            date_to=input.date_to,
            group_by=group_by,
        )
        return [
            CashflowRow(
                period_start=date.fromisoformat(period_start),
                inflow_minor=inflow_minor,
                outflow_minor=outflow_minor,
                net_minor=net_minor,
            )
            for period_start, inflow_minor, outflow_minor, net_minor in rows
        ]

    async def report_category_spend(self, input: CategorySpendReportInput) -> list[CategorySpendRow]:
        rows = await self._repository.report_category_spend(
            book_id=input.book_id,
            date_from=input.date_from,
            date_to=input.date_to,
            category_ids=input.category_ids,
        )
        return [
            CategorySpendRow(category_id=category_id, category_name=category_name, total_minor=total_minor)
            for category_id, category_name, total_minor in rows
        ]

    async def report_payee_totals(self, input: PayeeTotalsReportInput) -> list[PayeeTotalRow]:
        rows = await self._repository.report_payee_totals(
            book_id=input.book_id,
            date_from=input.date_from,
            date_to=input.date_to,
            payee_ids=input.payee_ids,
        )
        return [PayeeTotalRow(payee_id=payee_id, payee_name=payee_name, total_minor=total_minor) for payee_id, payee_name, total_minor in rows]