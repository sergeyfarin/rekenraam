from sqlalchemy import Select, select
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.transactions import Split, Transaction
from rekenraam_api.schemas.transactions import TransactionListFilters


class TransactionRepository:
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def list_transactions(self, filters: TransactionListFilters | None = None) -> list[Transaction]:
        active_filters = filters or TransactionListFilters()

        statement: Select[tuple[Transaction]] = select(Transaction)

        if active_filters.book_id is not None:
            statement = statement.where(Transaction.book_id == active_filters.book_id)

        if active_filters.account_id is not None:
            account_tx_ids = select(Split.tx_id).where(Split.account_id == active_filters.account_id)
            statement = statement.where(Transaction.id.in_(account_tx_ids))

        if active_filters.status is not None:
            statement = statement.where(Transaction.status == active_filters.status)

        if active_filters.occurred_from is not None:
            statement = statement.where(Transaction.occurred_date >= active_filters.occurred_from)

        if active_filters.occurred_to is not None:
            statement = statement.where(Transaction.occurred_date <= active_filters.occurred_to)

        statement = statement.order_by(Transaction.occurred_date.desc(), Transaction.id.desc())
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