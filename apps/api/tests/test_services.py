from datetime import UTC, datetime

import pytest
from pydantic_core import ValidationError

from rekenraam_api.db.models.accounts import Account
from rekenraam_api.db.models.books import Book
from rekenraam_api.db.models.transactions import Split, Transaction
from rekenraam_api.schemas.accounts import AccountTreeNode
from rekenraam_api.services.accounts import AccountService
from rekenraam_api.services.books import BookService
from rekenraam_api.services.transactions import TransactionService


class StubBookRepository:
    async def list_books(self) -> list[Book]:
        return [Book(id=1, slug="personal", name="Personal", base_currency_code="USD")]

    async def get_book_by_slug(self, slug: str) -> Book | None:
        if slug == "personal":
            return Book(id=1, slug="personal", name="Personal", base_currency_code="USD")
        return None

    async def get_schema_version(self) -> str:
        return "0003_add_transactions"


class StubAccountRepository:
    _created_at = datetime(2026, 5, 3, tzinfo=UTC)

    async def list_accounts(self) -> list[Account]:
        return [
            Account(
                id=1,
                book_id=1,
                parent_id=None,
                account_type="asset",
                name="Assets",
                is_closed=False,
                is_hidden=False,
                is_system=False,
                system_role=None,
                created_at=self._created_at,
            ),
            Account(
                id=2,
                book_id=1,
                parent_id=1,
                account_type="asset",
                name="Cash",
                is_closed=False,
                is_hidden=False,
                is_system=False,
                system_role=None,
                created_at=self._created_at,
            ),
        ]

    async def get_account_by_id(self, account_id: int) -> Account | None:
        if account_id == 1:
            return Account(
                id=1,
                book_id=1,
                parent_id=None,
                account_type="asset",
                name="Assets",
                is_closed=False,
                is_hidden=False,
                is_system=False,
                system_role=None,
                created_at=self._created_at,
            )
        return None

    async def get_book_base_currency_code(self, book_id: int) -> str | None:
        if book_id == 1:
            return "USD"
        return None

    async def get_account_balances(self) -> dict[int, int]:
        return {2: 500000, 3: -500000}


class StubTransactionRepository:
    _created_at = datetime(2026, 5, 3, tzinfo=UTC)

    async def list_transactions(self) -> list[Transaction]:
        return [
            Transaction(
                id=1,
                book_id=1,
                occurred_date=datetime(2026, 5, 1, tzinfo=UTC).date(),
                posted_date=datetime(2026, 5, 1, tzinfo=UTC).date(),
                memo="Initial opening balance",
                status="cleared",
                created_at=self._created_at,
            )
        ]

    async def get_transaction_by_id(self, transaction_id: int) -> Transaction | None:
        if transaction_id == 1:
            return (await self.list_transactions())[0]
        return None

    async def list_splits_for_transaction_ids(self, transaction_ids: list[int]) -> list[Split]:
        if 1 not in transaction_ids:
            return []
        return [
            Split(id=1, tx_id=1, account_id=2, amount_minor=500000, memo="Opening cash", created_at=self._created_at),
            Split(id=2, tx_id=1, account_id=3, amount_minor=-500000, memo="Offset", created_at=self._created_at),
        ]


@pytest.mark.asyncio
async def test_book_service_maps_books_to_frozen_summaries() -> None:
    service = BookService(StubBookRepository())

    result = await service.list_books()

    assert len(result) == 1
    assert result[0].slug == "personal"
    with pytest.raises(Exception):
        result[0].name = "Changed"


@pytest.mark.asyncio
async def test_book_service_returns_none_when_slug_is_missing() -> None:
    service = BookService(StubBookRepository())

    result = await service.get_book_by_slug("missing")

    assert result is None


@pytest.mark.asyncio
async def test_book_service_returns_schema_version_from_repository() -> None:
    service = BookService(StubBookRepository())

    result = await service.get_schema_version()

    assert result == "0003_add_transactions"


@pytest.mark.asyncio
async def test_account_service_maps_parent_and_system_flags() -> None:
    service = AccountService(StubAccountRepository())

    result = await service.list_accounts()

    assert [account.id for account in result] == [1, 2]
    assert result[1].parent_id == 1
    assert result[0].is_system is False


@pytest.mark.asyncio
async def test_account_service_returns_none_when_account_is_missing() -> None:
    service = AccountService(StubAccountRepository())

    result = await service.get_account_by_id(999)

    assert result is None


@pytest.mark.asyncio
async def test_account_service_returns_frozen_summary_for_detail() -> None:
    service = AccountService(StubAccountRepository())

    result = await service.get_account_by_id(1)

    assert result is not None
    assert result.name == "Assets"
    with pytest.raises(Exception):
        result.name = "Changed"


@pytest.mark.asyncio
async def test_account_service_builds_nested_tree_with_rollup_shape() -> None:
    service = AccountService(StubAccountRepository())

    result = await service.list_account_tree()

    assert [node.id for node in result] == [1, 2] or [node.id for node in result] == [1]
    assert result[0].commodity_name == "USD"
    assert result[0].commodity_scale == 2
    assert result[0].rollup_balance_minor == 500000
    assert len(result[0].children) == 1
    assert result[0].children[0].name == "Cash"
    assert result[0].children[0].balance_minor == 500000


def test_account_tree_response_is_frozen() -> None:
    node = AccountTreeNode(
        id=1,
        parent_id=None,
        name="Assets",
        account_type="asset",
        commodity_id=1,
        commodity_name="USD",
        commodity_scale=2,
        institution_name=None,
        country_name=None,
        balance_minor=0,
        rollup_balance_minor=0,
        children=(),
    )

    with pytest.raises(ValidationError):
        node.name = "Changed"


@pytest.mark.asyncio
async def test_transaction_service_returns_transactions_with_splits() -> None:
    service = TransactionService(StubTransactionRepository())

    result = await service.list_transactions()

    assert len(result) == 1
    assert result[0].memo == "Initial opening balance"
    assert len(result[0].splits) == 2
    assert result[0].splits[0].amount_minor == 500000


@pytest.mark.asyncio
async def test_transaction_service_returns_none_for_missing_transaction() -> None:
    service = TransactionService(StubTransactionRepository())

    result = await service.get_transaction_by_id(999)

    assert result is None