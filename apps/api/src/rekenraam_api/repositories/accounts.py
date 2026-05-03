from sqlalchemy import Select, func, select
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.accounts import Account
from rekenraam_api.db.models.books import Book
from rekenraam_api.db.models.transactions import Split, Transaction


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

    async def get_book_base_currency_code(self, book_id: int) -> str | None:
        statement: Select[tuple[str]] = select(Book.base_currency_code).where(Book.id == book_id)
        result = await self._session.execute(statement)
        return result.scalar_one_or_none()

    async def get_account_balances(self) -> dict[int, int]:
        statement = (
            select(Split.account_id, func.coalesce(func.sum(Split.amount_minor), 0))
            .join(Transaction, Transaction.id == Split.tx_id)
            .where(Transaction.status != "void")
            .group_by(Split.account_id)
        )
        result = await self._session.execute(statement)
        return {account_id: balance_minor for account_id, balance_minor in result.all()}