from sqlalchemy import Select, asc, desc, func, or_, select
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.metadata import Payee
from rekenraam_api.db.models.transactions import Split, Transaction
from rekenraam_api.schemas.transactions import TransactionListFilters


class TransactionRepository:
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def list_transactions(
        self,
        filters: TransactionListFilters | None = None,
        allowed_book_ids: list[int] | None = None,
    ) -> list[Transaction]:
        active_filters = filters or TransactionListFilters()

        statement: Select[tuple[Transaction]] = select(Transaction).outerjoin(Payee, Payee.id == Transaction.payee_id)

        if active_filters.book_id is not None:
            statement = statement.where(Transaction.book_id == active_filters.book_id)
        elif allowed_book_ids is not None:
            if not allowed_book_ids:
                return []
            statement = statement.where(Transaction.book_id.in_(allowed_book_ids))

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

    async def create_transaction(
        self,
        *,
        book_id: int,
        txn_date,
        payee_id: int | None,
        memo: str | None,
        status: str,
        reference: str | None,
        created_by_user_id: int | None = None,
        created_session_id: int | None = None,
        created_device_id: int | None = None,
        created_request_id: str | None = None,
    ) -> Transaction:
        transaction = Transaction(
            book_id=book_id,
            occurred_date=txn_date,
            posted_date=txn_date,
            payee_id=payee_id,
            memo=memo,
            status=status,
            reference=reference,
            created_by_user_id=created_by_user_id,
            created_session_id=created_session_id,
            created_device_id=created_device_id,
            created_request_id=created_request_id,
        )
        self._session.add(transaction)
        await self._session.flush()
        return transaction

    async def update_transaction(
        self,
        *,
        transaction_id: int,
        txn_date,
        payee_id: int | None,
        memo: str | None,
        status: str,
        reference: str | None,
    ) -> Transaction | None:
        transaction = await self.get_transaction_by_id(transaction_id)
        if transaction is None:
            return None

        transaction.occurred_date = txn_date
        transaction.posted_date = txn_date
        transaction.payee_id = payee_id
        transaction.memo = memo
        transaction.status = status
        transaction.reference = reference
        await self._session.flush()
        return transaction

    async def delete_transaction(self, transaction_id: int) -> bool:
        transaction = await self.get_transaction_by_id(transaction_id)
        if transaction is None:
            return False

        await self._session.delete(transaction)
        await self._session.commit()
        return True

    async def list_splits_for_transaction_ids(self, transaction_ids: list[int]) -> list[Split]:
        if not transaction_ids:
            return []

        statement: Select[tuple[Split]] = select(Split).where(Split.tx_id.in_(transaction_ids)).order_by(Split.id)
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def replace_transaction_splits(
        self,
        transaction_id: int,
        splits: list[dict[str, int | str | None]],
        created_by_user_id: int | None = None,
        created_session_id: int | None = None,
        created_device_id: int | None = None,
        created_request_id: str | None = None,
    ) -> list[Split]:
        existing = await self.list_splits_for_transaction_ids([transaction_id])
        for split in existing:
            await self._session.delete(split)
        await self._session.flush()

        created: list[Split] = []
        for split_input in splits:
            split = Split(
                tx_id=transaction_id,
                account_id=int(split_input["account_id"]),
                commodity_id=int(split_input["commodity_id"]),
                amount_minor=int(split_input["amount_minor"]),
                category_id=split_input["category_id"],
                tag_id=split_input["tag_id"],
                person_id=split_input["person_id"],
                project_id=split_input["project_id"],
                share_bps=split_input["share_bps"],
                memo=split_input["memo"],
                created_by_user_id=created_by_user_id,
                created_session_id=created_session_id,
                created_device_id=created_device_id,
                created_request_id=created_request_id,
            )
            self._session.add(split)
            created.append(split)

        await self._session.flush()
        return created

    async def list_account_register_splits(self, account_id: int) -> list[tuple[Transaction, Split]]:
        statement = (
            select(Transaction, Split)
            .join(Split, Split.tx_id == Transaction.id)
            .where(Split.account_id == account_id)
            .order_by(Transaction.occurred_date.asc(), Transaction.id.asc(), Split.id.asc())
        )
        result = await self._session.execute(statement)
        return list(result.all())

    async def get_payee_defaults(self, payee_id: int, account_id: int | None = None) -> tuple[int | None, str | None]:
        statement = (
            select(Split.category_id, Transaction.memo)
            .join(Transaction, Transaction.id == Split.tx_id)
            .where(Transaction.payee_id == payee_id)
            .where(Split.category_id.is_not(None))
            .where(Transaction.status != "void")
        )

        if account_id is not None:
            statement = statement.where(Split.account_id == account_id)

        statement = statement.order_by(Transaction.occurred_date.desc(), Transaction.id.desc()).limit(1)
        result = await self._session.execute(statement)
        row = result.first()
        if row is None:
            return None, None
        return row[0], row[1]

    async def duplicate_transaction(
        self,
        transaction_id: int,
        today,
        *,
        created_by_user_id: int | None = None,
        created_session_id: int | None = None,
        created_device_id: int | None = None,
        created_request_id: str | None = None,
    ) -> Transaction | None:
        source = await self.get_transaction_by_id(transaction_id)
        if source is None:
            return None

        duplicate = Transaction(
            book_id=source.book_id,
            occurred_date=today,
            posted_date=today,
            payee_id=source.payee_id,
            memo=source.memo,
            status="uncleared",
            reference=None,
            created_by_user_id=created_by_user_id,
            created_session_id=created_session_id,
            created_device_id=created_device_id,
            created_request_id=created_request_id,
        )
        self._session.add(duplicate)
        await self._session.flush()

        source_splits = await self.list_splits_for_transaction_ids([transaction_id])
        for source_split in source_splits:
            self._session.add(
                Split(
                    tx_id=duplicate.id,
                    account_id=source_split.account_id,
                    commodity_id=source_split.commodity_id,
                    amount_minor=source_split.amount_minor,
                    category_id=source_split.category_id,
                    tag_id=source_split.tag_id,
                    person_id=source_split.person_id,
                    project_id=source_split.project_id,
                    share_bps=source_split.share_bps,
                    memo=source_split.memo,
                    created_by_user_id=created_by_user_id,
                    created_session_id=created_session_id,
                    created_device_id=created_device_id,
                    created_request_id=created_request_id,
                )
            )

        await self._session.commit()
        await self._session.refresh(duplicate)
        return duplicate

    async def bulk_void_transactions(
        self,
        transaction_ids: list[int],
        *,
        created_by_user_id: int | None = None,
        created_session_id: int | None = None,
        created_device_id: int | None = None,
        created_request_id: str | None = None,
    ) -> int:
        if not transaction_ids:
            return 0

        voided = 0
        for transaction_id in transaction_ids:
            source = await self.get_transaction_by_id(transaction_id)
            if source is None or source.status == "void":
                continue

            replacement = Transaction(
                book_id=source.book_id,
                occurred_date=source.occurred_date,
                posted_date=source.posted_date,
                payee_id=source.payee_id,
                memo=source.memo,
                status="void",
                reference=source.reference,
                created_by_user_id=created_by_user_id,
                created_session_id=created_session_id,
                created_device_id=created_device_id,
                created_request_id=created_request_id,
            )
            self._session.add(replacement)
            await self._session.flush()

            source_splits = await self.list_splits_for_transaction_ids([transaction_id])
            for source_split in source_splits:
                self._session.add(
                    Split(
                        tx_id=replacement.id,
                        account_id=source_split.account_id,
                        commodity_id=source_split.commodity_id,
                        amount_minor=source_split.amount_minor,
                        category_id=source_split.category_id,
                        tag_id=source_split.tag_id,
                        person_id=source_split.person_id,
                        project_id=source_split.project_id,
                        share_bps=source_split.share_bps,
                        memo=source_split.memo,
                        created_by_user_id=created_by_user_id,
                        created_session_id=created_session_id,
                        created_device_id=created_device_id,
                        created_request_id=created_request_id,
                    )
                )
            voided += 1

        await self._session.commit()
        return voided

    async def bulk_delete_transactions(self, transaction_ids: list[int]) -> int:
        if not transaction_ids:
            return 0

        deleted = 0
        for transaction_id in transaction_ids:
            transaction = await self.get_transaction_by_id(transaction_id)
            if transaction is None:
                continue
            splits = await self.list_splits_for_transaction_ids([transaction_id])
            for split in splits:
                await self._session.delete(split)
            await self._session.delete(transaction)
            deleted += 1

        await self._session.commit()
        return deleted
