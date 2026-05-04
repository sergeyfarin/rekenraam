from __future__ import annotations

import csv
import io
from collections.abc import Iterable

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.accounts import Account
from rekenraam_api.db.models.metadata import Payee
from rekenraam_api.db.models.transactions import Split, Transaction
from rekenraam_api.schemas.exports import ReportCsvExportRequest
from rekenraam_api.schemas.investments import RealizedGainsQuery, UnrealizedGainsQuery
from rekenraam_api.schemas.reports import (
    CashflowReportInput,
    CategorySpendReportInput,
    PayeeTotalsReportInput,
)
from rekenraam_api.services.access import AccessPolicy
from rekenraam_api.services.investments import InvestmentService
from rekenraam_api.services.reports import ReportService


class ExportService:
    def __init__(
        self,
        session: AsyncSession,
        report_service: ReportService,
        investment_service: InvestmentService,
        access_policy: AccessPolicy | None = None,
    ) -> None:
        self._session = session
        self._report_service = report_service
        self._investment_service = investment_service
        self._access_policy = access_policy

    async def accounts_csv(self, book_id: int) -> str:
        await self._require_book_read(book_id)
        result = await self._session.execute(
            select(Account).where(Account.book_id == book_id).order_by(Account.id.asc())
        )
        return self._to_csv(
            [
                "id",
                "book_id",
                "parent_id",
                "account_type",
                "name",
                "commodity_id",
                "is_closed",
                "is_hidden",
            ],
            (
                [
                    account.id,
                    account.book_id,
                    account.parent_id,
                    account.account_type,
                    account.name,
                    account.commodity_id,
                    account.is_closed,
                    account.is_hidden,
                ]
                for account in result.scalars().all()
            ),
        )

    async def transactions_csv(self, book_id: int) -> str:
        await self._require_book_read(book_id)
        statement = (
            select(Transaction, Split, Payee)
            .join(Split, Split.tx_id == Transaction.id)
            .outerjoin(Payee, Payee.id == Transaction.payee_id)
            .where(Transaction.book_id == book_id)
            .order_by(Transaction.occurred_date.asc(), Transaction.id.asc(), Split.id.asc())
        )
        result = await self._session.execute(statement)
        return self._to_csv(
            [
                "transaction_id",
                "occurred_date",
                "posted_date",
                "status",
                "payee",
                "memo",
                "reference",
                "import_id",
                "split_id",
                "account_id",
                "commodity_id",
                "amount_minor",
                "category_id",
            ],
            (
                [
                    tx.id,
                    tx.occurred_date,
                    tx.posted_date,
                    tx.status,
                    payee.name if payee is not None else "",
                    tx.memo,
                    tx.reference,
                    tx.import_id,
                    split.id,
                    split.account_id,
                    split.commodity_id,
                    split.amount_minor,
                    split.category_id,
                ]
                for tx, split, payee in result.all()
            ),
        )

    async def register_qif(self, account_id: int) -> str:
        account = await self._session.get(Account, account_id)
        if account is None:
            raise ValueError("account not found")
        await self._require_book_read(account.book_id)
        statement = (
            select(Transaction, Split, Payee)
            .join(Split, Split.tx_id == Transaction.id)
            .outerjoin(Payee, Payee.id == Transaction.payee_id)
            .where(Split.account_id == account_id)
            .where(Transaction.status != "void")
            .order_by(Transaction.occurred_date.asc(), Transaction.id.asc())
        )
        result = await self._session.execute(statement)
        lines = ["!Type:Bank"]
        for tx, split, payee in result.all():
            lines.append(f"D{tx.occurred_date.strftime('%m/%d/%Y')}")
            lines.append(f"T{split.amount_minor / 100:.2f}")
            if payee is not None:
                lines.append(f"P{payee.name}")
            if tx.memo:
                lines.append(f"M{tx.memo}")
            if tx.reference:
                lines.append(f"N{tx.reference}")
            lines.append("^")
        return "\n".join(lines) + "\n"

    async def report_csv(self, kind: str, input: ReportCsvExportRequest) -> str:
        await self._require_book_read(input.book_id)
        if kind == "cashflow":
            rows = await self._report_service.report_cashflow(
                CashflowReportInput(
                    book_id=input.book_id, date_from=input.date_from, date_to=input.date_to
                )
            )
        elif kind == "category-spend":
            rows = await self._report_service.report_category_spend(
                CategorySpendReportInput(
                    book_id=input.book_id, date_from=input.date_from, date_to=input.date_to
                )
            )
        elif kind == "payee-totals":
            rows = await self._report_service.report_payee_totals(
                PayeeTotalsReportInput(
                    book_id=input.book_id, date_from=input.date_from, date_to=input.date_to
                )
            )
        elif kind == "realized-gains":
            rows = await self._investment_service.report_realized_gains(
                RealizedGainsQuery(
                    book_id=input.book_id, date_from=input.date_from, date_to=input.date_to
                )
            )
        elif kind == "unrealized-gains":
            if input.base_commodity_id is None:
                raise ValueError("base_commodity_id is required")
            rows = await self._investment_service.report_unrealized_gains(
                UnrealizedGainsQuery(
                    book_id=input.book_id,
                    base_commodity_id=input.base_commodity_id,
                    as_of_date=input.as_of_date,
                )
            )
        else:
            raise ValueError("unsupported report export")
        payloads = [row.model_dump(mode="json") for row in rows]
        headers = sorted({key for payload in payloads for key in payload})
        return self._to_csv(
            headers, ([payload.get(header, "") for header in headers] for payload in payloads)
        )

    def _to_csv(self, headers: list[str], rows: Iterable[Iterable[object]]) -> str:
        output = io.StringIO()
        writer = csv.writer(output)
        writer.writerow(headers)
        writer.writerows(rows)
        return output.getvalue()

    async def _require_book_read(self, book_id: int) -> None:
        if self._access_policy is not None:
            await self._access_policy.require_book_read(book_id)
