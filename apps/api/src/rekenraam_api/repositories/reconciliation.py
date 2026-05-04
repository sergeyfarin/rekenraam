from __future__ import annotations

from datetime import UTC, date, datetime

from sqlalchemy import Select, exists, func, literal, select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import aliased

from rekenraam_api.db.models.accounts import (
    Account,
    AccountBalancing,
    BalanceAdjustment,
    BalanceCheck,
    BalanceConstraint,
    ReconciliationPreference,
)
from rekenraam_api.db.models.metadata import Commodity
from rekenraam_api.db.models.transactions import Split, Transaction


class ReconciliationRepository:
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def get_account(self, account_id: int) -> Account | None:
        return await self._session.get(Account, account_id)

    async def get_commodity(self, commodity_id: int) -> Commodity | None:
        return await self._session.get(Commodity, commodity_id)

    async def list_account_chain_ids(self, account_id: int) -> list[int]:
        result = await self._session.execute(select(Account).order_by(Account.id.asc()))
        accounts = list(result.scalars().all())
        chain_ids = {account_id}
        changed = True
        while changed:
            changed = False
            for account in accounts:
                if (
                    account.id in chain_ids
                    and account.previous_account_id is not None
                    and account.previous_account_id not in chain_ids
                ):
                    chain_ids.add(account.previous_account_id)
                    changed = True
                if account.previous_account_id in chain_ids and account.id not in chain_ids:
                    chain_ids.add(account.id)
                    changed = True
        return sorted(chain_ids)

    async def get_last_active_balancing(self, account_id: int) -> AccountBalancing | None:
        newer = aliased(AccountBalancing)
        statement: Select[tuple[AccountBalancing]] = (
            select(AccountBalancing)
            .where(AccountBalancing.account_id == account_id)
            .where(AccountBalancing.voided_at.is_(None))
            .where(
                ~exists(
                    select(literal(1)).where(
                        newer.previous_account_balancing_id == AccountBalancing.id
                    )
                )
            )
            .order_by(AccountBalancing.as_of_date.desc(), AccountBalancing.id.desc())
            .limit(1)
        )
        return await self._session.scalar(statement)

    async def list_balancing_history(self, account_id: int) -> list[AccountBalancing]:
        newer = aliased(AccountBalancing)
        statement: Select[tuple[AccountBalancing]] = (
            select(AccountBalancing)
            .where(AccountBalancing.account_id == account_id)
            .where(
                ~exists(
                    select(literal(1)).where(
                        newer.previous_account_balancing_id == AccountBalancing.id
                    )
                )
            )
            .order_by(AccountBalancing.as_of_date.desc(), AccountBalancing.id.desc())
        )
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def list_candidates(
        self, account_id: int, as_of_date: date, start_date: date | None = None
    ) -> list[Transaction]:
        account_ids = await self.list_account_chain_ids(account_id)
        if not account_ids:
            return []
        newer = aliased(Transaction)
        statement: Select[tuple[Transaction]] = (
            select(Transaction)
            .join(Split, Split.tx_id == Transaction.id)
            .where(Split.account_id.in_(account_ids))
            .where(Transaction.occurred_date <= as_of_date)
            .where(Transaction.status.in_(("uncleared", "cleared")))
            .where(~exists(select(literal(1)).where(newer.previous_tx_id == Transaction.id)))
            .order_by(Transaction.occurred_date.asc(), Transaction.id.asc())
            .distinct()
        )
        if start_date is not None:
            statement = statement.where(Transaction.occurred_date >= start_date)
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def list_splits_for_transaction_ids(self, transaction_ids: list[int]) -> list[Split]:
        if not transaction_ids:
            return []
        result = await self._session.execute(
            select(Split).where(Split.tx_id.in_(transaction_ids)).order_by(Split.id)
        )
        return list(result.scalars().all())

    async def get_remembered_offset_account_id(self, user_id: int, account_id: int) -> int | None:
        statement = select(ReconciliationPreference.offset_account_id).where(
            ReconciliationPreference.user_id == user_id,
            ReconciliationPreference.account_id == account_id,
        )
        return await self._session.scalar(statement)

    async def remember_offset_account(
        self, *, user_id: int, book_id: int, account_id: int, offset_account_id: int
    ) -> None:
        statement = (
            insert(ReconciliationPreference)
            .values(
                user_id=user_id,
                book_id=book_id,
                account_id=account_id,
                offset_account_id=offset_account_id,
                updated_at=datetime.now(UTC),
            )
            .on_conflict_do_update(
                constraint="uq_reconciliation_preferences_user_account",
                set_={
                    "book_id": book_id,
                    "offset_account_id": offset_account_id,
                    "updated_at": datetime.now(UTC),
                },
            )
        )
        await self._session.execute(statement)

    async def create_balance_check(
        self,
        *,
        book_id: int,
        account_id: int,
        as_of_date: date,
        balance_minor: int,
        memo: str | None,
        status: str,
        created_by_user_id: int | None,
        created_session_id: int | None,
        created_device_id: int | None,
        created_request_id: str | None,
    ) -> BalanceCheck:
        check = BalanceCheck(
            book_id=book_id,
            account_id=account_id,
            as_of_date=as_of_date,
            balance_minor=balance_minor,
            memo=memo,
            status=status,
            created_by_user_id=created_by_user_id,
            created_session_id=created_session_id,
            created_device_id=created_device_id,
            created_request_id=created_request_id,
        )
        self._session.add(check)
        await self._session.flush()
        return check

    async def update_balance_check_status(self, check: BalanceCheck, status: str) -> None:
        check.status = status
        await self._session.flush()

    async def create_account_balancing(
        self,
        *,
        book_id: int,
        account_id: int,
        as_of_date: date,
        balance_minor: int,
        memo: str | None,
        created_by_user_id: int | None,
        created_session_id: int | None,
        created_device_id: int | None,
        created_request_id: str | None,
    ) -> AccountBalancing:
        balancing = AccountBalancing(
            book_id=book_id,
            account_id=account_id,
            as_of_date=as_of_date,
            balance_minor=balance_minor,
            memo=memo,
            created_by_user_id=created_by_user_id,
            created_session_id=created_session_id,
            created_device_id=created_device_id,
            created_request_id=created_request_id,
        )
        self._session.add(balancing)
        await self._session.flush()
        return balancing

    async def append_transaction_status(
        self,
        *,
        transaction: Transaction,
        status: str,
        created_by_user_id: int | None,
        created_session_id: int | None,
        created_device_id: int | None,
        created_request_id: str | None,
    ) -> Transaction:
        replacement = Transaction(
            book_id=transaction.book_id,
            previous_tx_id=transaction.id,
            occurred_date=transaction.occurred_date,
            posted_date=transaction.posted_date,
            payee_id=transaction.payee_id,
            memo=transaction.memo,
            status=status,
            reference=transaction.reference,
            created_by_user_id=created_by_user_id,
            created_session_id=created_session_id,
            created_device_id=created_device_id,
            created_request_id=created_request_id,
        )
        self._session.add(replacement)
        await self._session.flush()
        splits = await self.list_splits_for_transaction_ids([transaction.id])
        for split in splits:
            self._session.add(
                Split(
                    tx_id=replacement.id,
                    account_id=split.account_id,
                    commodity_id=split.commodity_id,
                    amount_minor=split.amount_minor,
                    category_id=split.category_id,
                    tag_id=split.tag_id,
                    person_id=split.person_id,
                    project_id=split.project_id,
                    share_bps=split.share_bps,
                    memo=split.memo,
                    created_by_user_id=created_by_user_id,
                    created_session_id=created_session_id,
                    created_device_id=created_device_id,
                    created_request_id=created_request_id,
                )
            )
        await self._session.flush()
        return replacement

    async def create_adjustment_transaction(
        self,
        *,
        book_id: int,
        account_id: int,
        account_commodity_id: int,
        offset_account_id: int,
        as_of_date: date,
        adjustment_minor: int,
        memo: str | None,
        created_by_user_id: int | None,
        created_session_id: int | None,
        created_device_id: int | None,
        created_request_id: str | None,
    ) -> Transaction:
        tx_memo = f"[BALANCING] {memo}" if memo else "[BALANCING] Balance adjustment"
        transaction = Transaction(
            book_id=book_id,
            occurred_date=as_of_date,
            posted_date=as_of_date,
            memo=tx_memo,
            status="cleared",
            reference="balance_adjustment",
            created_by_user_id=created_by_user_id,
            created_session_id=created_session_id,
            created_device_id=created_device_id,
            created_request_id=created_request_id,
        )
        self._session.add(transaction)
        await self._session.flush()
        self._session.add_all(
            [
                Split(
                    tx_id=transaction.id,
                    account_id=account_id,
                    commodity_id=account_commodity_id,
                    amount_minor=adjustment_minor,
                    created_by_user_id=created_by_user_id,
                    created_session_id=created_session_id,
                    created_device_id=created_device_id,
                    created_request_id=created_request_id,
                ),
                Split(
                    tx_id=transaction.id,
                    account_id=offset_account_id,
                    commodity_id=account_commodity_id,
                    amount_minor=-adjustment_minor,
                    created_by_user_id=created_by_user_id,
                    created_session_id=created_session_id,
                    created_device_id=created_device_id,
                    created_request_id=created_request_id,
                ),
            ]
        )
        await self._session.flush()
        return transaction

    async def create_balance_adjustment(
        self,
        *,
        book_id: int,
        balance_check_id: int,
        account_id: int,
        offset_account_id: int,
        as_of_date: date,
        target_balance_minor: int,
        actual_balance_minor: int,
        adjustment_minor: int,
        tx_id: int,
        memo: str | None,
        created_by_user_id: int | None,
        created_session_id: int | None,
        created_device_id: int | None,
        created_request_id: str | None,
    ) -> BalanceAdjustment:
        adjustment = BalanceAdjustment(
            book_id=book_id,
            balance_check_id=balance_check_id,
            account_id=account_id,
            offset_account_id=offset_account_id,
            as_of_date=as_of_date,
            target_balance_minor=target_balance_minor,
            actual_balance_minor=actual_balance_minor,
            adjustment_minor=adjustment_minor,
            tx_id=tx_id,
            mark="balance_adjustment",
            memo=memo,
            created_by_user_id=created_by_user_id,
            created_session_id=created_session_id,
            created_device_id=created_device_id,
            created_request_id=created_request_id,
        )
        self._session.add(adjustment)
        await self._session.flush()
        return adjustment

    async def list_balance_checks(self, account_id: int) -> list[BalanceCheck]:
        statement = (
            select(BalanceCheck)
            .where(BalanceCheck.account_id == account_id)
            .order_by(BalanceCheck.as_of_date.desc(), BalanceCheck.id.desc())
        )
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def list_balance_adjustments(self, account_id: int) -> list[BalanceAdjustment]:
        statement = (
            select(BalanceAdjustment)
            .where(BalanceAdjustment.account_id == account_id)
            .order_by(BalanceAdjustment.as_of_date.desc(), BalanceAdjustment.id.desc())
        )
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def list_constraints(self, account_id: int) -> list[BalanceConstraint]:
        result = await self._session.execute(
            select(BalanceConstraint)
            .where(BalanceConstraint.account_id == account_id)
            .order_by(BalanceConstraint.id.desc())
        )
        return list(result.scalars().all())

    async def create_constraint(
        self,
        *,
        book_id: int,
        account_id: int,
        min_balance_minor: int | None,
        max_balance_minor: int | None,
        sign_rule: str,
    ) -> BalanceConstraint:
        constraint = BalanceConstraint(
            book_id=book_id,
            account_id=account_id,
            min_balance_minor=min_balance_minor,
            max_balance_minor=max_balance_minor,
            sign_rule=sign_rule,
            updated_at=datetime.now(UTC),
        )
        self._session.add(constraint)
        await self._session.flush()
        return constraint

    async def delete_constraint(self, constraint_id: int, account_id: int) -> bool:
        constraint = await self._session.get(BalanceConstraint, constraint_id)
        if constraint is None or constraint.account_id != account_id:
            return False
        await self._session.delete(constraint)
        await self._session.flush()
        return True

    async def current_balance_minor(self, account_id: int) -> int:
        account_ids = await self.list_account_chain_ids(account_id)
        if not account_ids:
            return 0
        newer = aliased(Transaction)
        statement = (
            select(func.coalesce(func.sum(Split.amount_minor), 0))
            .join(Transaction, Transaction.id == Split.tx_id)
            .where(Split.account_id.in_(account_ids))
            .where(Transaction.status != "void")
            .where(~exists(select(literal(1)).where(newer.previous_tx_id == Transaction.id)))
        )
        return int(await self._session.scalar(statement) or 0)

    async def unlock_balancings(self, account_id: int, from_date: date, reason: str | None) -> int:
        newer = aliased(AccountBalancing)
        statement = (
            select(AccountBalancing)
            .where(AccountBalancing.account_id == account_id)
            .where(AccountBalancing.voided_at.is_(None))
            .where(AccountBalancing.as_of_date >= from_date)
            .where(
                ~exists(
                    select(literal(1)).where(
                        newer.previous_account_balancing_id == AccountBalancing.id
                    )
                )
            )
            .order_by(AccountBalancing.id)
        )
        result = await self._session.execute(statement)
        balancings = list(result.scalars().all())
        now = datetime.now(UTC)
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
        await self._session.flush()
        return len(balancings)
