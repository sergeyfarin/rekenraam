from sqlalchemy import Select, select
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.transactions import Split, Transaction


class TransactionRepository:
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def list_transactions(self) -> list[Transaction]:
        statement: Select[tuple[Transaction]] = select(Transaction).order_by(
            Transaction.occurred_date.desc(), Transaction.id.desc()
        )
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