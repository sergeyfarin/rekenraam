from sqlalchemy import Select, case, func, select
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.metadata import Category, Payee
from rekenraam_api.db.models.transactions import Split, Transaction


class ReportRepository:
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def report_cashflow(
        self,
        *,
        book_id: int,
        date_from,
        date_to,
        group_by: str,
    ) -> list[tuple[str, int, int, int]]:
        if group_by == "year":
            period_expr = func.to_char(func.date_trunc("year", Transaction.occurred_date), "YYYY-MM-DD")
        elif group_by == "quarter":
            period_expr = func.to_char(func.date_trunc("quarter", Transaction.occurred_date), "YYYY-MM-DD")
        elif group_by == "day":
            period_expr = func.to_char(Transaction.occurred_date, "YYYY-MM-DD")
        else:
            period_expr = func.to_char(func.date_trunc("month", Transaction.occurred_date), "YYYY-MM-DD")

        statement: Select[tuple[str, int, int, int]] = (
            select(
                period_expr.label("period_start"),
                func.coalesce(func.sum(case((Split.amount_minor > 0, Split.amount_minor), else_=0)), 0).label("inflow_minor"),
                func.coalesce(func.sum(case((Split.amount_minor < 0, -Split.amount_minor), else_=0)), 0).label("outflow_minor"),
                func.coalesce(func.sum(Split.amount_minor), 0).label("net_minor"),
            )
            .join(Split, Split.tx_id == Transaction.id)
            .join(Category, Category.id == Split.category_id)
            .where(Transaction.book_id == book_id)
            .where(Category.kind.in_(["income", "expense"]))
            .group_by(period_expr)
            .order_by(period_expr.asc())
        )

        if date_from is not None:
            statement = statement.where(Transaction.occurred_date >= date_from)
        if date_to is not None:
            statement = statement.where(Transaction.occurred_date <= date_to)

        result = await self._session.execute(statement)
        return list(result.all())

    async def report_category_spend(
        self,
        *,
        book_id: int,
        date_from,
        date_to,
        category_ids: tuple[int, ...] | None,
    ) -> list[tuple[int, str, int]]:
        statement: Select[tuple[int, str, int]] = (
            select(
                Category.id,
                Category.name,
                func.coalesce(func.sum(Split.amount_minor), 0).label("total_minor"),
            )
            .outerjoin(Split, Split.category_id == Category.id)
            .outerjoin(Transaction, Transaction.id == Split.tx_id)
            .where(Category.book_id == book_id)
            .where(Category.kind == "expense")
            .group_by(Category.id, Category.name)
            .order_by(func.coalesce(func.sum(Split.amount_minor), 0).desc(), Category.name.asc())
        )

        if date_from is not None:
            statement = statement.where((Transaction.id.is_(None)) | (Transaction.occurred_date >= date_from))
        if date_to is not None:
            statement = statement.where((Transaction.id.is_(None)) | (Transaction.occurred_date <= date_to))
        if category_ids:
            statement = statement.where(Category.id.in_(category_ids))

        result = await self._session.execute(statement)
        return list(result.all())

    async def report_payee_totals(
        self,
        *,
        book_id: int,
        date_from,
        date_to,
        payee_ids: tuple[int, ...] | None,
    ) -> list[tuple[int, str, int]]:
        statement: Select[tuple[int, str, int]] = (
            select(
                Payee.id,
                Payee.name,
                func.coalesce(func.sum(Split.amount_minor), 0).label("total_minor"),
            )
            .join(Transaction, Transaction.payee_id == Payee.id)
            .join(Split, Split.tx_id == Transaction.id)
            .where(Payee.book_id == book_id)
            .group_by(Payee.id, Payee.name)
            .order_by(func.coalesce(func.sum(Split.amount_minor), 0).desc(), Payee.name.asc())
        )

        if date_from is not None:
            statement = statement.where(Transaction.occurred_date >= date_from)
        if date_to is not None:
            statement = statement.where(Transaction.occurred_date <= date_to)
        if payee_ids:
            statement = statement.where(Payee.id.in_(payee_ids))

        result = await self._session.execute(statement)
        return list(result.all())