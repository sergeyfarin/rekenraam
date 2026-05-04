from collections import defaultdict
from datetime import UTC, datetime, date

from sqlalchemy import Select, exists, func, literal, select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import aliased

from rekenraam_api.db.models.accounts import Account, AccountBalancing
from rekenraam_api.db.models.books import Book
from rekenraam_api.db.models.transactions import Split, Transaction


class AccountRepository:
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def list_accounts(self, book_ids: list[int] | None = None) -> list[Account]:
        statement: Select[tuple[Account]] = select(Account).order_by(Account.parent_id.nullsfirst(), Account.id)
        if book_ids is not None:
            if not book_ids:
                return []
            statement = statement.where(Account.book_id.in_(book_ids))
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def create_account(
        self,
        *,
        book_id: int,
        parent_id: int | None,
        account_type: str,
        name: str,
        commodity_id: int,
        institution_id: int | None,
        country_id: int | None,
        number_last4: str | None,
        is_closed: bool,
    ) -> Account:
        account = Account(
            book_id=book_id,
            parent_id=parent_id,
            previous_account_id=None,
            account_type=account_type,
            name=name,
            commodity_id=commodity_id,
            booking_policy="fifo",
            number_last4=number_last4,
            is_closed=is_closed,
            is_hidden=False,
            is_system=False,
            system_role=None,
            lifecycle_event="close" if is_closed else "open",
        )
        self._session.add(account)
        await self._session.commit()
        await self._session.refresh(account)
        return account

    async def get_account_by_id(self, account_id: int) -> Account | None:
        statement: Select[tuple[Account]] = select(Account).where(Account.id == account_id)
        result = await self._session.execute(statement)
        return result.scalar_one_or_none()

    async def update_account(
        self,
        *,
        account_id: int,
        parent_id: int | None,
        account_type: str,
        name: str,
        commodity_id: int,
        institution_id: int | None,
        country_id: int | None,
        number_last4: str | None,
        is_closed: bool,
    ) -> Account | None:
        account = await self.get_account_by_id(account_id)
        if account is None:
            return None

        previous_closed = account.is_closed
        account.parent_id = parent_id
        account.account_type = account_type
        account.name = name
        account.commodity_id = commodity_id
        account.number_last4 = number_last4
        account.is_closed = is_closed
        account.updated_at = datetime.now(UTC)
        account.effective_at = date.today()
        if not previous_closed and is_closed:
            account.lifecycle_event = "close"
        elif previous_closed and not is_closed:
            account.lifecycle_event = "reopen"
        else:
            account.lifecycle_event = "update"

        await self._session.commit()
        await self._session.refresh(account)
        return account

    async def delete_account(self, account_id: int) -> bool:
        account = await self.get_account_by_id(account_id)
        if account is None:
            return False

        await self._session.delete(account)
        try:
            await self._session.commit()
        except IntegrityError:
            await self._session.rollback()
            raise
        return True

    async def get_book_base_currency_code(self, book_id: int) -> str | None:
        statement: Select[tuple[str]] = select(Book.base_currency_code).where(Book.id == book_id)
        result = await self._session.execute(statement)
        return result.scalar_one_or_none()

    async def get_account_balances(self, book_ids: list[int] | None = None) -> dict[int, int]:
        statement = (
            select(Split.account_id, func.coalesce(func.sum(Split.amount_minor), 0))
            .join(Transaction, Transaction.id == Split.tx_id)
            .where(Transaction.status != "void")
            .group_by(Split.account_id)
        )
        if book_ids is not None:
            if not book_ids:
                return {}
            statement = statement.where(Transaction.book_id.in_(book_ids))
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

    async def set_account_booking_policy(self, account_id: int, booking_policy: str) -> Account | None:
        account = await self.get_account_by_id(account_id)
        if account is None:
            return None

        account.booking_policy = booking_policy
        await self._session.commit()
        await self._session.refresh(account)
        return account

    async def unlock_account_balancings(self, account_id: int, from_date: date, reason: str | None) -> int:
        newer_balancing = aliased(AccountBalancing)
        statement: Select[tuple[AccountBalancing]] = (
            select(AccountBalancing)
            .where(AccountBalancing.account_id == account_id)
            .where(AccountBalancing.voided_at.is_(None))
            .where(AccountBalancing.as_of_date >= from_date)
            .where(
                ~exists(select(literal(1)).where(newer_balancing.previous_account_balancing_id == AccountBalancing.id))
            )
            .order_by(AccountBalancing.id)
        )
        result = await self._session.execute(statement)
        balancings = list(result.scalars().all())
        if not balancings:
            return 0

        now = datetime.now(UTC)
        inserted = 0
        for balancing in balancings:
            self._session.add(
                AccountBalancing(
                    book_id=balancing.book_id,
                    account_id=balancing.account_id,
                    previous_account_balancing_id=balancing.id,
                    as_of_date=balancing.as_of_date,
                    balance_minor=balancing.balance_minor,
                    memo=balancing.memo,
                    created_at=now,
                    voided_at=now,
                    void_reason=reason,
                )
            )
            inserted += 1

        await self._session.commit()
        return inserted

    async def create_account_balancing(
        self,
        *,
        book_id: int,
        account_id: int,
        as_of_date: date,
        balance_minor: int,
        memo: str | None,
        created_by_user_id: int | None = None,
        created_session_id: int | None = None,
        created_device_id: int | None = None,
        created_request_id: str | None = None,
    ) -> AccountBalancing | None:
        account = await self.get_account_by_id(account_id)
        if account is None:
            return None
        if account.book_id != book_id:
            raise ValueError("account does not belong to book")

        latest_active_balancings = await self.list_account_balancings(account_id)
        if latest_active_balancings and as_of_date <= latest_active_balancings[0].as_of_date:
            raise ValueError("balancing date must be after last active balancing")

        balancing = AccountBalancing(
            book_id=book_id,
            account_id=account_id,
            previous_account_balancing_id=None,
            as_of_date=as_of_date,
            balance_minor=balance_minor,
            memo=memo,
            created_at=datetime.now(UTC),
            created_by_user_id=created_by_user_id,
            created_session_id=created_session_id,
            created_device_id=created_device_id,
            created_request_id=created_request_id,
            voided_at=None,
            void_reason=None,
        )
        self._session.add(balancing)
        await self._session.commit()
        await self._session.refresh(balancing)
        return balancing
