from rekenraam_api.db.models.transactions import Split, Transaction
from rekenraam_api.repositories.transactions import TransactionRepository
from rekenraam_api.services.report_invalidation import bump_report_state
from rekenraam_api.schemas.register import RegisterEntry
from rekenraam_api.schemas.transactions import (
    PayeeDefaults,
    SplitEntry,
    TransactionListFilters,
    TransactionMutationInput,
    TransactionSummary,
)


class TransactionService:
    def __init__(self, repository: TransactionRepository) -> None:
        self._repository = repository

    async def list_transactions(self, filters: TransactionListFilters | None = None) -> list[TransactionSummary]:
        transactions = await self._repository.list_transactions(filters)
        splits = await self._repository.list_splits_for_transaction_ids([transaction.id for transaction in transactions])
        return self._map_transactions(transactions, splits)

    async def get_transaction_by_id(self, transaction_id: int) -> TransactionSummary | None:
        transaction = await self._repository.get_transaction_by_id(transaction_id)
        if transaction is None:
            return None

        splits = await self._repository.list_splits_for_transaction_ids([transaction.id])
        return self._map_transactions([transaction], splits)[0]

    async def create_transaction(self, input: TransactionMutationInput) -> TransactionSummary:
        self._validate_mutation_input(input)
        transaction = await self._repository.create_transaction(
            book_id=input.book_id,
            txn_date=input.txn_date,
            payee_id=input.payee_id,
            memo=input.memo,
            status=input.status,
            reference=input.reference,
        )
        await self._repository.replace_transaction_splits(
            transaction.id,
            [split.model_dump() for split in input.splits],
        )
        await self._repository._session.commit()
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        refreshed = await self.get_transaction_by_id(transaction.id)
        if refreshed is None:
            raise ValueError("transaction not found after create")
        return refreshed

    async def update_transaction(self, transaction_id: int, input: TransactionMutationInput) -> TransactionSummary | None:
        self._validate_mutation_input(input)
        transaction = await self._repository.update_transaction(
            transaction_id=transaction_id,
            txn_date=input.txn_date,
            payee_id=input.payee_id,
            memo=input.memo,
            status=input.status,
            reference=input.reference,
        )
        if transaction is None:
            return None
        await self._repository.replace_transaction_splits(
            transaction.id,
            [split.model_dump() for split in input.splits],
        )
        await self._repository._session.commit()
        await bump_report_state(getattr(self._repository, "_session", None), input.book_id)
        return await self.get_transaction_by_id(transaction.id)

    async def delete_transaction(self, transaction_id: int) -> bool:
        transaction = await self._repository.get_transaction_by_id(transaction_id)
        if transaction is None:
            return False
        deleted = await self._repository.delete_transaction(transaction_id)
        if deleted:
            await bump_report_state(getattr(self._repository, "_session", None), transaction.book_id)
        return deleted

    async def duplicate_transaction(self, transaction_id: int, today) -> TransactionSummary | None:
        transaction = await self._repository.duplicate_transaction(transaction_id, today)
        if transaction is None:
            return None
        await bump_report_state(getattr(self._repository, "_session", None), transaction.book_id)
        return await self.get_transaction_by_id(transaction.id)

    async def bulk_void_transactions(self, transaction_ids: list[int]) -> int:
        transactions = [
            transaction
            for transaction in [await self._repository.get_transaction_by_id(transaction_id) for transaction_id in transaction_ids]
            if transaction is not None
        ]
        count = await self._repository.bulk_void_transactions(transaction_ids)
        for book_id in {transaction.book_id for transaction in transactions}:
            await bump_report_state(getattr(self._repository, "_session", None), book_id)
        return count

    async def bulk_delete_transactions(self, transaction_ids: list[int]) -> int:
        transactions = [
            transaction
            for transaction in [await self._repository.get_transaction_by_id(transaction_id) for transaction_id in transaction_ids]
            if transaction is not None
        ]
        count = await self._repository.bulk_delete_transactions(transaction_ids)
        for book_id in {transaction.book_id for transaction in transactions}:
            await bump_report_state(getattr(self._repository, "_session", None), book_id)
        return count

    async def list_account_register(self, account_id: int) -> list[RegisterEntry]:
        rows = await self._repository.list_account_register_splits(account_id)
        running_balance_minor = 0
        entries: list[RegisterEntry] = []

        for transaction, split in rows:
            running_balance_minor += split.amount_minor
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

        return entries

    async def get_payee_defaults(self, payee_id: int, account_id: int | None = None) -> PayeeDefaults:
        category_id, memo = await self._repository.get_payee_defaults(payee_id, account_id)
        return PayeeDefaults(category_id=category_id, memo=memo)

    @staticmethod
    def _validate_mutation_input(input: TransactionMutationInput) -> None:
        if input.status not in {"uncleared", "cleared", "reconciled", "void"}:
            raise ValueError("transaction status is invalid")
        if len(input.splits) < 2:
            raise ValueError("transaction requires at least two splits")
        if sum(split.amount_minor for split in input.splits) != 0:
            raise ValueError("transaction splits must balance to zero")

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
                occurred_date=transaction.occurred_date,
                posted_date=transaction.posted_date,
                payee_id=transaction.payee_id,
                memo=transaction.memo,
                status=transaction.status,
                reference=transaction.reference,
                created_at=transaction.created_at,
                splits=tuple(splits_by_tx.get(transaction.id, [])),
            )
            for transaction in transactions
        ]