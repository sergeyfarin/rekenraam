from sqlalchemy import Select, select
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.accounts import Account


class AccountRepository:
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def list_accounts(self) -> list[Account]:
        statement: Select[tuple[Account]] = select(Account).order_by(Account.parent_id.nullsfirst(), Account.id)
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def get_account_by_id(self, account_id: int) -> Account | None:
        statement: Select[tuple[Account]] = select(Account).where(Account.id == account_id)
        result = await self._session.execute(statement)
        return result.scalar_one_or_none()