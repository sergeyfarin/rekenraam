from rekenraam_api.db.models.transactions import Split, Transaction
from rekenraam_api.db.models.ergonomics import AuditEvent
from rekenraam_api.repositories.transactions import TransactionRepository
from rekenraam_api.services.report_invalidation import bump_report_state
from rekenraam_api.services.access import AccessPolicy
from rekenraam_api.schemas.register import RegisterEntry
from rekenraam_api.schemas.register import RegisterPage
from rekenraam_api.schemas.transactions import (
    PayeeDefaults,
    SplitEntry,
    TransactionListFilters,
    TransactionMutationInput,
    TransactionPage,
    TransactionSummary,
)


class TransactionService:
    def __init__(
        self, repository: TransactionRepository, access_policy: AccessPolicy | None = None
    ) -> None:
        self._repository = repository
        self._access_policy = access_policy

    async def list_transactions(
        self, filters: TransactionListFilters | None = None
    ) -> TransactionPage:
        allowed_book_ids = None
        if self._access_policy is not None:
            if filters is not None and filters.book_id is not None:
                await self._access_policy.require_book_read(filters.book_id)
            allowed_book_ids = await self._access_policy.list_readable_book_ids()
        transactions, next_cursor = await self._repository.list_transactions(
            filters, allowed_book_ids
        )
        splits = await self._repository.list_splits_for_transaction_ids(
            [transaction.id for transaction in transactions]
        )
        return TransactionPage(
            items=tuple(self._map_transactions(transactions, splits)), next_cursor=next_cursor
        )

    async def get_transaction_by_id(self, transaction_id: int) -> TransactionSummary | None:
        transaction = await self._repository.get_transaction_by_id(transaction_id)
        if transaction is None:
            return None
        if self._access_policy is not None:
            await self._access_policy.require_book_read(transaction.book_id)

        splits = await self._repository.list_splits_for_transaction_ids([transaction.id])
        return self._map_transactions([transaction], splits)[0]

    async def create_transaction(self, input: TransactionMutationInput) -> TransactionSummary:
        await self._validate_mutation_input(input)
        if self._access_policy is not None:
            await self._access_policy.require_book_write(input.book_id)
        await self._ensure_unlocked({split.account_id for split in input.splits}, input.txn_date)
        audit = self._access_policy.audit_stamp() if self._access_policy is not None else None
        transaction = await self._repository.create_transaction(
            book_id=input.book_id,
            txn_date=input.txn_date,
            payee_id=input.payee_id,
            memo=input.memo,
            status=input.status,
            reference=input.reference,
            import_id=input.import_id,
            created_by_user_id=audit.created_by_user_id if audit is not None else None,
            created_session_id=audit.created_session_id if audit is not None else None,
            created_device_id=audit.created_device_id if audit is not None else None,
            created_request_id=audit.created_request_id if audit is not None else None,
        )
        await self._repository.replace_transaction_splits(
            transaction.id,
            [split.model_dump() for split in input.splits],
            created_by_user_id=audit.created_by_user_id if audit is not None else None,
            created_session_id=audit.created_session_id if audit is not None else None,
            created_device_id=audit.created_device_id if audit is not None else None,
            created_request_id=audit.created_request_id if audit is not None else None,
        )
        session = getattr(self._repository, "_session", None)
        if session is not None:
            session.add(
                AuditEvent(
                    book_id=input.book_id,
                    actor_user_id=audit.created_by_user_id if audit is not None else None,
                    actor_session_id=audit.created_session_id if audit is not None else None,
                    actor_device_id=audit.created_device_id if audit is not None else None,
                    actor_request_id=audit.created_request_id if audit is not None else None,
                    event_type="transaction.created",
                    target_type="transaction",
                    target_id=transaction.id,
                    summary=f"Created transaction {transaction.id}",
                    metadata_json=None,
                )
            )
            await session.commit()
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        refreshed = await self.get_transaction_by_id(transaction.id)
        if refreshed is None:
            raise ValueError("transaction not found after create")
        return refreshed

    async def update_transaction(
        self, transaction_id: int, input: TransactionMutationInput
    ) -> TransactionSummary | None:
        await self._validate_mutation_input(input)
        current = await self._repository.get_transaction_by_id(transaction_id)
        if current is not None and self._access_policy is not None:
            await self._access_policy.require_book_write(current.book_id)
            if input.book_id != current.book_id:
                raise ValueError("transaction book cannot be changed")
        if current is not None:
            current_splits = await self._repository.list_splits_for_transaction_ids([current.id])
            await self._ensure_unlocked(
                {split.account_id for split in current_splits}, current.occurred_date
            )
            await self._ensure_unlocked(
                {split.account_id for split in input.splits}, input.txn_date
            )
        audit = self._access_policy.audit_stamp() if self._access_policy is not None else None
        transaction = await self._repository.update_transaction(
            transaction_id=transaction_id,
            txn_date=input.txn_date,
            payee_id=input.payee_id,
            memo=input.memo,
            status=input.status,
            reference=input.reference,
            created_by_user_id=audit.created_by_user_id if audit is not None else None,
            created_session_id=audit.created_session_id if audit is not None else None,
            created_device_id=audit.created_device_id if audit is not None else None,
            created_request_id=audit.created_request_id if audit is not None else None,
        )
        if transaction is None:
            return None
        await self._repository.replace_transaction_splits(
            transaction.id,
            [split.model_dump() for split in input.splits],
            created_by_user_id=audit.created_by_user_id if audit is not None else None,
            created_session_id=audit.created_session_id if audit is not None else None,
            created_device_id=audit.created_device_id if audit is not None else None,
            created_request_id=audit.created_request_id if audit is not None else None,
        )
        session = getattr(self._repository, "_session", None)
        if session is not None:
            session.add(
                AuditEvent(
                    book_id=input.book_id,
                    actor_user_id=audit.created_by_user_id if audit is not None else None,
                    actor_session_id=audit.created_session_id if audit is not None else None,
                    actor_device_id=audit.created_device_id if audit is not None else None,
                    actor_request_id=audit.created_request_id if audit is not None else None,
                    event_type="transaction.updated",
                    target_type="transaction",
                    target_id=transaction.id,
                    summary=f"Updated transaction {transaction_id}",
                    metadata_json=None,
                )
            )
            await session.commit()
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        return await self.get_transaction_by_id(transaction.id)

    async def delete_transaction(self, transaction_id: int) -> bool:
        transaction = await self._repository.get_transaction_by_id(transaction_id)
        if transaction is None:
            return False
        if self._access_policy is not None:
            await self._access_policy.require_book_write(transaction.book_id)
        splits = await self._repository.list_splits_for_transaction_ids([transaction.id])
        await self._ensure_unlocked(
            {split.account_id for split in splits}, transaction.occurred_date
        )
        audit = self._access_policy.audit_stamp() if self._access_policy is not None else None
        deleted = await self._repository.delete_transaction(
            transaction_id,
            created_by_user_id=audit.created_by_user_id if audit is not None else None,
            created_session_id=audit.created_session_id if audit is not None else None,
            created_device_id=audit.created_device_id if audit is not None else None,
            created_request_id=audit.created_request_id if audit is not None else None,
        )
        if deleted:
            await bump_report_state(
                getattr(self._repository, "_session", None), transaction.book_id
            )
        return deleted

    async def duplicate_transaction(self, transaction_id: int, today) -> TransactionSummary | None:
        source = await self._repository.get_transaction_by_id(transaction_id)
        if source is not None and self._access_policy is not None:
            await self._access_policy.require_book_write(source.book_id)
        if source is not None:
            source_splits = await self._repository.list_splits_for_transaction_ids([source.id])
            await self._ensure_unlocked({split.account_id for split in source_splits}, today)
        audit = self._access_policy.audit_stamp() if self._access_policy is not None else None
        transaction = await self._repository.duplicate_transaction(
            transaction_id,
            today,
            created_by_user_id=audit.created_by_user_id if audit is not None else None,
            created_session_id=audit.created_session_id if audit is not None else None,
            created_device_id=audit.created_device_id if audit is not None else None,
            created_request_id=audit.created_request_id if audit is not None else None,
        )
        if transaction is None:
            return None
        await bump_report_state(getattr(self._repository, "_session", None), transaction.book_id)
        return await self.get_transaction_by_id(transaction.id)

    async def bulk_void_transactions(self, transaction_ids: list[int]) -> int:
        transactions = [
            transaction
            for transaction in [
                await self._repository.get_transaction_by_id(transaction_id)
                for transaction_id in transaction_ids
            ]
            if transaction is not None
        ]
        if self._access_policy is not None:
            for transaction in transactions:
                await self._access_policy.require_book_write(transaction.book_id)
        eligible_ids: list[int] = []
        for transaction in transactions:
            splits = await self._repository.list_splits_for_transaction_ids([transaction.id])
            try:
                await self._ensure_unlocked(
                    {split.account_id for split in splits}, transaction.occurred_date
                )
            except ValueError:
                continue
            eligible_ids.append(transaction.id)
        audit = self._access_policy.audit_stamp() if self._access_policy is not None else None
        count = await self._repository.bulk_void_transactions(
            eligible_ids,
            created_by_user_id=audit.created_by_user_id if audit is not None else None,
            created_session_id=audit.created_session_id if audit is not None else None,
            created_device_id=audit.created_device_id if audit is not None else None,
            created_request_id=audit.created_request_id if audit is not None else None,
        )
        for book_id in {transaction.book_id for transaction in transactions}:
            await bump_report_state(getattr(self._repository, "_session", None), book_id)
        return count

    async def bulk_delete_transactions(self, transaction_ids: list[int]) -> int:
        transactions = [
            transaction
            for transaction in [
                await self._repository.get_transaction_by_id(transaction_id)
                for transaction_id in transaction_ids
            ]
            if transaction is not None
        ]
        if self._access_policy is not None:
            for transaction in transactions:
                await self._access_policy.require_book_write(transaction.book_id)
        eligible_ids: list[int] = []
        for transaction in transactions:
            splits = await self._repository.list_splits_for_transaction_ids([transaction.id])
            try:
                await self._ensure_unlocked(
                    {split.account_id for split in splits}, transaction.occurred_date
                )
            except ValueError:
                continue
            eligible_ids.append(transaction.id)
        audit = self._access_policy.audit_stamp() if self._access_policy is not None else None
        count = await self._repository.bulk_delete_transactions(
            eligible_ids,
            created_by_user_id=audit.created_by_user_id if audit is not None else None,
            created_session_id=audit.created_session_id if audit is not None else None,
            created_device_id=audit.created_device_id if audit is not None else None,
            created_request_id=audit.created_request_id if audit is not None else None,
        )
        for book_id in {transaction.book_id for transaction in transactions}:
            await bump_report_state(getattr(self._repository, "_session", None), book_id)
        return count

    async def list_account_register(
        self, account_id: int, *, limit: int = 100, cursor: str | None = None
    ) -> RegisterPage:
        rows, next_cursor = await self._repository.list_account_register_splits(
            account_id, limit=limit, cursor=cursor
        )
        if rows and self._access_policy is not None:
            await self._access_policy.require_book_read(rows[0][0].book_id)
        entries: list[RegisterEntry] = []

        for transaction, split, running_balance_minor in rows:
            entries.append(
                RegisterEntry(
                    tx_id=transaction.id,
                    split_id=split.id,
                    account_id=split.account_id,
                    occurred_date=transaction.occurred_date,
                    posted_date=transaction.posted_date,
                    payee_id=transaction.payee_id,
                    memo=transaction.memo,
                    status=transaction.status,
                    reference=transaction.reference,
                    commodity_id=split.commodity_id,
                    category_id=split.category_id,
                    amount_minor=split.amount_minor,
                    running_balance_minor=running_balance_minor,
                    created_at=split.created_at,
                )
            )

        return RegisterPage(items=tuple(entries), next_cursor=next_cursor)

    async def get_payee_defaults(
        self, payee_id: int, account_id: int | None = None
    ) -> PayeeDefaults:
        category_id, memo = await self._repository.get_payee_defaults(payee_id, account_id)
        return PayeeDefaults(category_id=category_id, memo=memo)

    async def _validate_mutation_input(self, input: TransactionMutationInput) -> None:
        if input.status not in {"uncleared", "cleared", "reconciled", "void"}:
            raise ValueError("transaction status is invalid")
        if len(input.splits) < 2:
            raise ValueError("transaction requires at least two splits")
        if sum(split.amount_minor for split in input.splits) != 0:
            raise ValueError("transaction splits must balance to zero")
        accounts = await self._repository.list_accounts_by_ids(
            {split.account_id for split in input.splits}
        )
        if len(accounts) != len({split.account_id for split in input.splits}):
            raise ValueError("transaction split account not found")
        for account in accounts:
            if account.book_id != input.book_id:
                raise ValueError("transaction split account does not belong to book")
            if account.is_closed:
                raise ValueError("transaction cannot use closed accounts")
            if account.is_system and account.system_role is not None:
                raise ValueError("transaction cannot use protected system accounts")
        commodity_by_account_id = {account.id: account.commodity_id for account in accounts}
        for split in input.splits:
            if commodity_by_account_id.get(split.account_id) != split.commodity_id:
                raise ValueError("transaction split commodity must match account commodity")
        refs_valid = await self._repository.metadata_refs_belong_to_book(
            book_id=input.book_id,
            payee_id=input.payee_id,
            category_ids={
                split.category_id for split in input.splits if split.category_id is not None
            },
            tag_ids={split.tag_id for split in input.splits if split.tag_id is not None},
            person_ids={split.person_id for split in input.splits if split.person_id is not None},
            project_ids={
                split.project_id for split in input.splits if split.project_id is not None
            },
        )
        if not refs_valid:
            raise ValueError("transaction metadata references must belong to book")

    async def _ensure_unlocked(self, account_ids: set[int], occurred_date) -> None:
        locked = await self._repository.get_locked_account_ids(account_ids, occurred_date)
        if locked:
            details = ", ".join(
                f"{account_id} (balanced through {lock_date})" for account_id, lock_date in locked
            )
            raise ValueError(
                f"cannot post transaction dated {occurred_date} to locked accounts: {details}"
            )

    def _map_transactions(
        self,
        transactions: list[Transaction],
        splits: list[Split],
    ) -> list[TransactionSummary]:
        splits_by_tx: dict[int, list[SplitEntry]] = {}
        for split in splits:
            splits_by_tx.setdefault(split.tx_id, []).append(
                SplitEntry(
                    id=split.id,
                    tx_id=split.tx_id,
                    account_id=split.account_id,
                    commodity_id=split.commodity_id,
                    amount_minor=split.amount_minor,
                    category_id=split.category_id,
                    tag_id=split.tag_id,
                    person_id=split.person_id,
                    project_id=split.project_id,
                    share_bps=split.share_bps,
                    memo=split.memo,
                    created_at=split.created_at,
                )
            )

        return [
            TransactionSummary(
                id=transaction.id,
                book_id=transaction.book_id,
                previous_tx_id=transaction.previous_tx_id,
                occurred_date=transaction.occurred_date,
                posted_date=transaction.posted_date,
                payee_id=transaction.payee_id,
                memo=transaction.memo,
                status=transaction.status,
                reference=transaction.reference,
                import_id=transaction.import_id,
                created_at=transaction.created_at,
                splits=tuple(splits_by_tx.get(transaction.id, [])),
            )
            for transaction in transactions
        ]
