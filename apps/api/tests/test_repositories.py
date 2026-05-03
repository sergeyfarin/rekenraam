from datetime import UTC, date, datetime

import pytest
from sqlalchemy import delete
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.accounts import Account, AccountBalancing
from rekenraam_api.db.models.metadata import Payee
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
    payee_id: int | None = None,
    reference: str | None = None,
    splits: list[tuple[int, int, str]] = [],
) -> None:
    created_at = datetime(2026, 5, 3, tzinfo=UTC)
    session.add(
        Transaction(
            id=transaction_id,
            book_id=1,
            occurred_date=occurred_date,
            posted_date=posted_date,
            payee_id=payee_id,
            memo=memo,
            status=status,
            reference=reference,
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
                commodity_id=1,
                amount_minor=amount_minor,
                category_id=None,
                tag_id=None,
                person_id=None,
                project_id=None,
                share_bps=None,
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
async def test_account_repository_lists_latest_active_balancings(repository_session: AsyncSession) -> None:
    repository_session.add_all(
        [
            AccountBalancing(
                id=1,
                book_id=1,
                account_id=2,
                previous_account_balancing_id=None,
                as_of_date=date(2026, 5, 1),
                balance_minor=100000,
                memo="Older",
                created_at=datetime(2026, 5, 1, tzinfo=UTC),
                voided_at=None,
                void_reason=None,
            ),
            AccountBalancing(
                id=2,
                book_id=1,
                account_id=2,
                previous_account_balancing_id=1,
                as_of_date=date(2026, 5, 2),
                balance_minor=120000,
                memo="Latest",
                created_at=datetime(2026, 5, 2, tzinfo=UTC),
                voided_at=None,
                void_reason=None,
            ),
        ]
    )
    await repository_session.commit()

    balancings = await AccountRepository(repository_session).list_account_balancings(2)

    assert [balancing.id for balancing in balancings] == [2]
    assert balancings[0].balance_minor == 120000


@pytest.mark.asyncio
async def test_account_repository_lists_account_directives(repository_session: AsyncSession) -> None:
    directives = await AccountRepository(repository_session).list_account_directives(2)

    assert [directive.id for directive in directives] == [2]
    assert directives[0].lifecycle_event == "open"


@pytest.mark.asyncio
async def test_account_repository_returns_booking_policy(repository_session: AsyncSession) -> None:
    repository_session.add(
        Account(
            id=10,
            book_id=1,
            parent_id=None,
            previous_account_id=None,
            account_type="investment",
            name="Brokerage",
            commodity_id=1,
            booking_policy="average",
            number_last4=None,
            is_closed=False,
            is_hidden=False,
            is_system=False,
            system_role=None,
            effective_at=date(2026, 5, 3),
            lifecycle_event="open",
            lifecycle_note=None,
            lifecycle_metadata=None,
            created_at=datetime(2026, 5, 3, tzinfo=UTC),
            updated_at=datetime(2026, 5, 3, tzinfo=UTC),
        )
    )
    await repository_session.commit()

    account, policy = await AccountRepository(repository_session).get_account_booking_policy(10)

    assert account is not None
    assert policy == "average"


@pytest.mark.asyncio
async def test_transaction_repository_lists_seeded_transactions(repository_session: AsyncSession) -> None:
    repository = TransactionRepository(repository_session)

    transactions = await repository.list_transactions()

    assert len(transactions) == 1
    assert transactions[0].memo == "Initial opening balance"
    assert transactions[0].status == "cleared"
    assert transactions[0].payee_id is None
    assert transactions[0].reference is None


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
async def test_transaction_repository_applies_search_amount_and_pagination_filters(
    repository_session: AsyncSession,
) -> None:
    repository_session.add(Payee(id=1, book_id=1, name="Local Market", kind="business", metadata_text=None))
    await repository_session.commit()
    try:
        await _insert_test_transaction(
            repository_session,
            transaction_id=2,
            occurred_date=date(2026, 5, 2),
            posted_date=date(2026, 5, 2),
            memo="Pending groceries",
            status="uncleared",
            payee_id=1,
            reference="groceries-1",
            splits=[(2, -1250, "Groceries"), (3, 1250, "Offset")],
        )
        await _insert_test_transaction(
            repository_session,
            transaction_id=3,
            occurred_date=date(2026, 5, 3),
            posted_date=date(2026, 5, 3),
            memo="Big shop",
            status="uncleared",
            reference="bulk-1",
            splits=[(2, -4500, "Bulk"), (3, 4500, "Offset")],
        )

        filtered = await TransactionRepository(repository_session).list_transactions(
            TransactionListFilters(
                search="Local",
                amount_min=1200,
                status="all",
                sort_by="date",
                sort_dir="desc",
                limit=1,
                offset=0,
            )
        )

        assert [transaction.id for transaction in filtered] == [2]
    finally:
        await repository_session.execute(delete(Transaction).where(Transaction.id.in_([2, 3])))
        await repository_session.execute(delete(Payee).where(Payee.id == 1))
        await repository_session.commit()


@pytest.mark.asyncio
async def test_transaction_repository_lists_splits_for_transaction(repository_session: AsyncSession) -> None:
    repository = TransactionRepository(repository_session)

    splits = await repository.list_splits_for_transaction_ids([1])

    assert len(splits) == 2
    assert [split.amount_minor for split in splits] == [500000, -500000]
    assert [split.commodity_id for split in splits] == [1, 1]


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