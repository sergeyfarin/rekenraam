from datetime import UTC, datetime

from sqlalchemy import Select, case, func, select
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.report_metadata import ReportDefinition, ReportRun
from rekenraam_api.db.models.report_state import BookState, ReportCache
from rekenraam_api.db.models.metadata import Category, Payee
from rekenraam_api.db.models.transactions import Split, Transaction
from rekenraam_api.services.request_context import get_request_context


def _audit_values() -> dict[str, int | str | None]:
    context = get_request_context()
    if context is None:
        return {
            "created_by_user_id": None,
            "created_session_id": None,
            "created_device_id": None,
            "created_request_id": None,
        }
    return {
        "created_by_user_id": context.user_id,
        "created_session_id": context.session_id,
        "created_device_id": context.device_id,
        "created_request_id": context.request_id,
    }


class ReportRepository:
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def get_book_change_seq(self, book_id: int) -> int:
        row = await self._session.get(BookState, book_id)
        if row is None:
            row = BookState(book_id=book_id, change_seq=0)
            self._session.add(row)
            await self._session.commit()
            await self._session.refresh(row)
        return row.change_seq

    async def bump_book_change_seq(self, book_id: int) -> int:
        row = await self._session.get(BookState, book_id)
        if row is None:
            row = BookState(book_id=book_id, change_seq=1)
            self._session.add(row)
        else:
            row.change_seq += 1
            row.updated_at = datetime.now(UTC)
        await self._session.commit()
        await self._session.refresh(row)
        return row.change_seq

    async def get_cached_report(self, *, book_id: int, report_type: str, params_hash: str, as_of_seq: int) -> ReportCache | None:
        statement: Select[tuple[ReportCache]] = (
            select(ReportCache)
            .where(ReportCache.book_id == book_id)
            .where(ReportCache.report_type == report_type)
            .where(ReportCache.params_hash == params_hash)
            .where(ReportCache.as_of_seq == as_of_seq)
            .order_by(ReportCache.id.desc())
            .limit(1)
        )
        return await self._session.scalar(statement)

    async def save_cached_report(
        self,
        *,
        book_id: int,
        report_type: str,
        params_hash: str,
        params_json: str,
        as_of_seq: int,
        payload_json: str,
    ) -> ReportCache:
        existing = await self.get_cached_report(
            book_id=book_id,
            report_type=report_type,
            params_hash=params_hash,
            as_of_seq=as_of_seq,
        )
        if existing is not None:
            return existing

        row = ReportCache(
            book_id=book_id,
            report_type=report_type,
            params_hash=params_hash,
            params_json=params_json,
            as_of_seq=as_of_seq,
            payload_json=payload_json,
        )
        self._session.add(row)
        await self._session.commit()
        await self._session.refresh(row)
        return row

    async def list_report_definitions(self, book_id: int) -> list[ReportDefinition]:
        newer = ReportDefinition.__table__.alias("newer")
        statement: Select[tuple[ReportDefinition]] = (
            select(ReportDefinition)
            .where(ReportDefinition.book_id == book_id)
            .where((ReportDefinition.session_id.is_(None)) | (~ReportDefinition.session_id.like("void:%")))
            .where(
                ~select(newer.c.id)
                .where(newer.c.previous_report_definition_id == ReportDefinition.id)
                .exists()
            )
            .order_by(ReportDefinition.name.asc(), ReportDefinition.id.asc())
        )
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def get_report_definition(self, definition_id: int) -> ReportDefinition | None:
        current_items = await self.list_report_definitions_for_chain(definition_id)
        return current_items[0] if current_items else None

    async def list_report_definitions_for_chain(self, definition_id: int) -> list[ReportDefinition]:
        all_rows = await self._session.execute(select(ReportDefinition).order_by(ReportDefinition.id.asc()))
        rows = list(all_rows.scalars().all())
        row_by_id = {row.id: row for row in rows}
        chain_ids = {definition_id}
        changed = True
        while changed:
            changed = False
            for row in rows:
                if row.id in chain_ids and row.previous_report_definition_id is not None and row.previous_report_definition_id not in chain_ids:
                    chain_ids.add(row.previous_report_definition_id)
                    changed = True
                if row.previous_report_definition_id in chain_ids and row.id not in chain_ids:
                    chain_ids.add(row.id)
                    changed = True
        candidates = [row_by_id[row_id] for row_id in chain_ids if row_id in row_by_id]
        active = [
            row
            for row in candidates
            if (row.session_id is None or not row.session_id.startswith("void:"))
            and not any(other.previous_report_definition_id == row.id for other in candidates)
        ]
        active.sort(key=lambda row: row.id)
        return active[:1]

    async def create_report_definition(
        self,
        *,
        book_id: int,
        name: str,
        kind: str,
        query_type: str,
        query_text: str,
        params_schema: str | None,
    ) -> ReportDefinition:
        row = ReportDefinition(
            book_id=book_id,
            name=name,
            kind=kind,
            query_type=query_type,
            query_text=query_text,
            params_schema=params_schema,
            **_audit_values(),
        )
        self._session.add(row)
        await self._session.commit()
        await self._session.refresh(row)
        return row

    async def update_report_definition(
        self,
        *,
        definition_id: int,
        book_id: int,
        name: str,
        kind: str,
        query_type: str,
        query_text: str,
        params_schema: str | None,
    ) -> ReportDefinition | None:
        current = await self.get_report_definition(definition_id)
        if current is None:
            return None
        row = ReportDefinition(
            previous_report_definition_id=current.id,
            book_id=book_id,
            name=name,
            kind=kind,
            query_type=query_type,
            query_text=query_text,
            params_schema=params_schema,
            **_audit_values(),
        )
        self._session.add(row)
        await self._session.commit()
        await self._session.refresh(row)
        return row

    async def delete_report_definition(self, definition_id: int) -> bool:
        current = await self.get_report_definition(definition_id)
        if current is None:
            return False
        row = ReportDefinition(
            previous_report_definition_id=current.id,
            session_id="void:delete",
            book_id=current.book_id,
            name=current.name,
            kind=current.kind,
            query_type=current.query_type,
            query_text=current.query_text,
            params_schema=current.params_schema,
            **_audit_values(),
        )
        self._session.add(row)
        await self._session.commit()
        return True

    async def list_report_runs(self, book_id: int, definition_id: int | None = None) -> list[ReportRun]:
        statement: Select[tuple[ReportRun]] = (
            select(ReportRun)
            .where(ReportRun.book_id == book_id)
            .order_by(ReportRun.created_at.desc(), ReportRun.id.desc())
        )
        if definition_id is not None:
            statement = statement.where(ReportRun.definition_id == definition_id)
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def create_report_run(
        self,
        *,
        book_id: int,
        definition_id: int,
        params_hash: str,
        as_of_seq: int,
        pricing_mode: str,
        pricing_policy_id: int | None,
        pricing_policy_version: str | None,
        valuation_snapshot_id: int | None,
        pricing_resolved_at,
        result_json: str,
    ) -> ReportRun:
        row = ReportRun(
            book_id=book_id,
            definition_id=definition_id,
            params_hash=params_hash,
            as_of_seq=as_of_seq,
            pricing_mode=pricing_mode,
            pricing_policy_id=pricing_policy_id,
            pricing_policy_version=pricing_policy_version,
            valuation_snapshot_id=valuation_snapshot_id,
            pricing_resolved_at=pricing_resolved_at,
            result_json=result_json,
            **_audit_values(),
        )
        self._session.add(row)
        await self._session.commit()
        await self._session.refresh(row)
        return row

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
