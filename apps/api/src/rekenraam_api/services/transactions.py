from rekenraam_api.db.models.transactions import Split, Transaction
from rekenraam_api.repositories.transactions import TransactionRepository
from rekenraam_api.schemas.transactions import SplitEntry, TransactionSummary


class TransactionService:
    def __init__(self, repository: TransactionRepository) -> None:
        self._repository = repository

    async def list_transactions(self) -> list[TransactionSummary]:
        transactions = await self._repository.list_transactions()
        splits = await self._repository.list_splits_for_transaction_ids([transaction.id for transaction in transactions])
        return self._map_transactions(transactions, splits)

    async def get_transaction_by_id(self, transaction_id: int) -> TransactionSummary | None:
        transaction = await self._repository.get_transaction_by_id(transaction_id)
        if transaction is None:
            return None

        splits = await self._repository.list_splits_for_transaction_ids([transaction.id])
        return self._map_transactions([transaction], splits)[0]

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
                    amount_minor=split.amount_minor,
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
                memo=transaction.memo,
                status=transaction.status,
                created_at=transaction.created_at,
                splits=tuple(splits_by_tx.get(transaction.id, [])),
            )
            for transaction in transactions
        ]