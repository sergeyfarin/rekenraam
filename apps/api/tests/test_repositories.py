import pytest
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.repositories.accounts import AccountRepository
from rekenraam_api.repositories.books import BookRepository
from rekenraam_api.repositories.transactions import TransactionRepository


@pytest.mark.asyncio
async def test_book_repository_lists_seeded_books(repository_session: AsyncSession) -> None:
    repository = BookRepository(repository_session)

    books = await repository.list_books()

    assert [book.slug for book in books] == ["personal"]
    assert books[0].base_currency_code == "USD"


@pytest.mark.asyncio
async def test_book_repository_returns_schema_version_from_migrations(repository_session: AsyncSession) -> None:
    repository = BookRepository(repository_session)

    schema_version = await repository.get_schema_version()

    assert schema_version == "0003_add_transactions"


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