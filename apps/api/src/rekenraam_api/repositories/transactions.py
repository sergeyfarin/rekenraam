from sqlalchemy import Select, asc, desc, func, or_, select
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.metadata import Payee
from rekenraam_api.db.models.transactions import Split, Transaction
from rekenraam_api.schemas.transactions import TransactionListFilters


class TransactionRepository:
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def list_transactions(self, filters: TransactionListFilters | None = None) -> list[Transaction]:
        active_filters = filters or TransactionListFilters()

        statement: Select[tuple[Transaction]] = select(Transaction).outerjoin(Payee, Payee.id == Transaction.payee_id)

        if active_filters.book_id is not None:
            statement = statement.where(Transaction.book_id == active_filters.book_id)

        status_filter = active_filters.status
        if status_filter is None or status_filter == "active":
            statement = statement.where(Transaction.status != "void")
        elif status_filter == "void":
            statement = statement.where(Transaction.status == "void")
        elif status_filter != "all":
            statement = statement.where(Transaction.status == status_filter)

        if active_filters.account_id is not None:
            account_tx_ids = select(Split.tx_id).where(Split.account_id == active_filters.account_id)
            statement = statement.where(Transaction.id.in_(account_tx_ids))

        if active_filters.payee_id is not None:
            statement = statement.where(Transaction.payee_id == active_filters.payee_id)

        if active_filters.occurred_from is not None:
            statement = statement.where(Transaction.occurred_date >= active_filters.occurred_from)

        if active_filters.occurred_to is not None:
            statement = statement.where(Transaction.occurred_date <= active_filters.occurred_to)

        if active_filters.search:
            like_pattern = f"%{active_filters.search}%"
            statement = statement.where(
                or_(
                    Transaction.memo.ilike(like_pattern),
                    Transaction.reference.ilike(like_pattern),
                    Payee.name.ilike(like_pattern),
                )
            )

        if active_filters.amount_min is not None:
            matching_tx_ids = select(Split.tx_id).where(func.abs(Split.amount_minor) >= active_filters.amount_min)
            statement = statement.where(Transaction.id.in_(matching_tx_ids))

        if active_filters.amount_max is not None:
            matching_tx_ids = select(Split.tx_id).where(func.abs(Split.amount_minor) <= active_filters.amount_max)
            statement = statement.where(Transaction.id.in_(matching_tx_ids))

        amount_sort_value = (
            select(func.max(func.abs(Split.amount_minor)))
            .where(Split.tx_id == Transaction.id)
            .scalar_subquery()
        )

        sort_by = active_filters.sort_by or "date"
        sort_dir = active_filters.sort_dir or "desc"
        primary_direction = asc if sort_dir == "asc" else desc
        tie_direction = asc if sort_dir == "asc" else desc

        if sort_by == "payee":
            statement = statement.order_by(primary_direction(Payee.name), desc(Transaction.occurred_date), desc(Transaction.id))
        elif sort_by == "status":
            statement = statement.order_by(primary_direction(Transaction.status), desc(Transaction.occurred_date), desc(Transaction.id))
        elif sort_by == "amount":
            statement = statement.order_by(primary_direction(amount_sort_value), desc(Transaction.occurred_date), desc(Transaction.id))
        else:
            statement = statement.order_by(primary_direction(Transaction.occurred_date), tie_direction(Transaction.id))

        if active_filters.limit is not None:
            statement = statement.limit(active_filters.limit)

        if active_filters.offset is not None:
            statement = statement.offset(active_filters.offset)

        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def get_transaction_by_id(self, transaction_id: int) -> Transaction | None:
        statement: Select[tuple[Transaction]] = select(Transaction).where(Transaction.id == transaction_id)
        result = await self._session.execute(statement)
        return result.scalar_one_or_none()

    async def list_splits_for_transaction_ids(self, transaction_ids: list[int]) -> list[Split]:
        if not transaction_ids:
            return []

        statement: Select[tuple[Split]] = select(Split).where(Split.tx_id.in_(transaction_ids)).order_by(Split.id)
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def list_account_register_splits(self, account_id: int) -> list[tuple[Transaction, Split]]:
        statement = (
            select(Transaction, Split)
            .join(Split, Split.tx_id == Transaction.id)
            .where(Split.account_id == account_id)
            .order_by(Transaction.occurred_date.asc(), Transaction.id.asc(), Split.id.asc())
        )
        result = await self._session.execute(statement)
        return list(result.all())