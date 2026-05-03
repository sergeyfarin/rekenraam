from datetime import UTC, date, datetime

import pytest
from sqlalchemy import delete
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.transactions import Split, Transaction
from rekenraam_api.repositories.accounts import AccountRepository
from rekenraam_api.repositories.books import BookRepository
from rekenraam_api.repositories.transactions import TransactionRepository
from rekenraam_api.schemas.transactions import TransactionListFilters


async def _insert_test_transaction(
    session: AsyncSession,
    *,
    transaction_id: int,
    occurred_date: date,
    posted_date: date,
    memo: str,
    status: str,
    splits: list[tuple[int, int, str]],
) -> None:
    created_at = datetime(2026, 5, 3, tzinfo=UTC)
    session.add(
        Transaction(
            id=transaction_id,
            book_id=1,
            occurred_date=occurred_date,
            posted_date=posted_date,
            memo=memo,
            status=status,
            created_at=created_at,
        )
    )
    await session.flush()
    for offset, (account_id, amount_minor, split_memo) in enumerate(splits, start=1):
        session.add(
            Split(
                id=transaction_id * 10 + offset,
                tx_id=transaction_id,
                account_id=account_id,
                amount_minor=amount_minor,
                memo=split_memo,
                created_at=created_at,
            )
        )
    await session.commit()


@pytest.mark.asyncio
async def test_book_repository_lists_seeded_books(repository_session: AsyncSession) -> None:
    repository = BookRepository(repository_session)

    books = await repository.list_books()

    assert [book.slug for book in books] == ["personal"]
    assert books[0].base_currency_code == "USD"


@pytest.mark.asyncio
async def test_book_repository_returns_none_for_unknown_slug(repository_session: AsyncSession) -> None:
    repository = BookRepository(repository_session)

    book = await repository.get_book_by_slug("missing")

    assert book is None


@pytest.mark.asyncio
async def test_account_repository_lists_seeded_accounts_in_parent_first_order(
    repository_session: AsyncSession,
) -> None:
    repository = AccountRepository(repository_session)

    accounts = await repository.list_accounts()

    assert [account.name for account in accounts] == ["Assets", "Opening Balances", "Cash"]
    assert accounts[2].parent_id == accounts[0].id


@pytest.mark.asyncio
async def test_account_repository_returns_expected_account_by_id(repository_session: AsyncSession) -> None:
    repository = AccountRepository(repository_session)

    account = await repository.get_account_by_id(2)

    assert account is not None
    assert account.name == "Cash"
    assert account.account_type == "asset"
    assert account.commodity_id == 1
    assert account.number_last4 is None


@pytest.mark.asyncio
async def test_account_repository_returns_none_for_missing_account(repository_session: AsyncSession) -> None:
    repository = AccountRepository(repository_session)

    account = await repository.get_account_by_id(999)

    assert account is None


@pytest.mark.asyncio
async def test_account_repository_returns_book_base_currency_code(repository_session: AsyncSession) -> None:
    repository = AccountRepository(repository_session)

    base_currency_code = await repository.get_book_base_currency_code(1)

    assert base_currency_code == "USD"


@pytest.mark.asyncio
async def test_account_repository_returns_transaction_backed_balances(repository_session: AsyncSession) -> None:
    repository = AccountRepository(repository_session)

    balances = await repository.get_account_balances()

    assert balances[2] == 500000
    assert balances[3] == -500000


@pytest.mark.asyncio
async def test_transaction_repository_lists_seeded_transactions(repository_session: AsyncSession) -> None:
    repository = TransactionRepository(repository_session)

    transactions = await repository.list_transactions()

    assert len(transactions) == 1
    assert transactions[0].memo == "Initial opening balance"
    assert transactions[0].status == "cleared"


@pytest.mark.asyncio
async def test_transaction_repository_applies_status_account_and_date_filters(
    repository_session: AsyncSession,
) -> None:
    try:
        await _insert_test_transaction(
            repository_session,
            transaction_id=2,
            occurred_date=date(2026, 5, 2),
            posted_date=date(2026, 5, 2),
            memo="Pending groceries",
            status="uncleared",
            splits=[(2, -1250, "Groceries"), (3, 1250, "Offset")],
        )
        await _insert_test_transaction(
            repository_session,
            transaction_id=3,
            occurred_date=date(2026, 5, 3),
            posted_date=date(2026, 5, 3),
            memo="Voided test",
            status="void",
            splits=[(2, -2500, "Voided cash"), (3, 2500, "Voided offset")],
        )
        repository = TransactionRepository(repository_session)

        filtered = await repository.list_transactions(
            TransactionListFilters(
                account_id=2,
                status="uncleared",
                occurred_from=date(2026, 5, 2),
                occurred_to=date(2026, 5, 2),
            )
        )

        assert [transaction.id for transaction in filtered] == [2]
    finally:
        await repository_session.execute(delete(Transaction).where(Transaction.id.in_([2, 3])))
        await repository_session.commit()


@pytest.mark.asyncio
async def test_transaction_repository_lists_splits_for_transaction(repository_session: AsyncSession) -> None:
    repository = TransactionRepository(repository_session)

    splits = await repository.list_splits_for_transaction_ids([1])

    assert len(splits) == 2
    assert [split.amount_minor for split in splits] == [500000, -500000]


@pytest.mark.asyncio
async def test_transaction_repository_returns_none_for_missing_transaction(repository_session: AsyncSession) -> None:
    repository = TransactionRepository(repository_session)

    transaction = await repository.get_transaction_by_id(999)

    assert transaction is None


@pytest.mark.asyncio
async def test_transaction_repository_lists_account_register_rows(repository_session: AsyncSession) -> None:
    repository = TransactionRepository(repository_session)

    rows = await repository.list_account_register_splits(2)

    assert len(rows) == 1
    transaction, split = rows[0]
    assert transaction.memo == "Initial opening balance"
    assert split.amount_minor == 500000