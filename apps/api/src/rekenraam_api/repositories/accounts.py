from collections import defaultdict

from sqlalchemy import Select, exists, func, literal, select
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import aliased

from rekenraam_api.db.models.accounts import Account, AccountBalancing
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

    async def list_account_balancings(self, account_id: int) -> list[AccountBalancing]:
        newer_balancing = aliased(AccountBalancing)
        statement: Select[tuple[AccountBalancing]] = (
            select(AccountBalancing)
            .where(AccountBalancing.account_id == account_id)
            .where(AccountBalancing.voided_at.is_(None))
            .where(
                ~exists(select(literal(1)).where(newer_balancing.previous_account_balancing_id == AccountBalancing.id))
            )
            .order_by(AccountBalancing.as_of_date.desc(), AccountBalancing.id.desc())
        )
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def list_account_directives(self, account_id: int) -> list[Account]:
        statement: Select[tuple[Account]] = select(Account)
        result = await self._session.execute(statement)
        accounts = list(result.scalars().all())

        account_by_id = {account.id: account for account in accounts}
        if account_id not in account_by_id:
            return []

        child_ids_by_previous_id: dict[int, list[int]] = defaultdict(list)
        for account in accounts:
            if account.previous_account_id is not None:
                child_ids_by_previous_id[account.previous_account_id].append(account.id)

        chain_ids: set[int] = set()
        pending_ids = [account_id]
        while pending_ids:
            current_id = pending_ids.pop()
            if current_id in chain_ids:
                continue
            chain_ids.add(current_id)

            account = account_by_id.get(current_id)
            if account is None:
                continue
            if account.previous_account_id is not None:
                pending_ids.append(account.previous_account_id)
            pending_ids.extend(child_ids_by_previous_id.get(current_id, []))

        return sorted(
            [
                account
                for account_id_in_chain, account in account_by_id.items()
                if account_id_in_chain in chain_ids and account.lifecycle_event in {"open", "close", "reopen"}
            ],
            key=lambda account: (account.effective_at, account.id),
            reverse=True,
        )

    async def get_account_booking_policy(self, account_id: int) -> tuple[Account | None, str | None]:
        account = await self.get_account_by_id(account_id)
        if account is None:
            return None, None
        return account, account.booking_policy