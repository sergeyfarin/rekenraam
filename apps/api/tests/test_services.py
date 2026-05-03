from datetime import UTC, datetime

import pytest

from rekenraam_api.db.models.accounts import Account
from rekenraam_api.db.models.books import Book
from rekenraam_api.services.accounts import AccountService
from rekenraam_api.services.books import BookService


class StubBookRepository:
    async def list_books(self) -> list[Book]:
        return [Book(id=1, slug="personal", name="Personal", base_currency_code="USD")]

    async def get_book_by_slug(self, slug: str) -> Book | None:
        if slug == "personal":
            return Book(id=1, slug="personal", name="Personal", base_currency_code="USD")
        return None

    async def get_schema_version(self) -> str:
        return "0002_add_accounts"


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

    assert result == "0002_add_accounts"


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