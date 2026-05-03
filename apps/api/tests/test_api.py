from collections.abc import AsyncIterator
from datetime import UTC, datetime

import pytest
from httpx import ASGITransport, AsyncClient

from rekenraam_api.api.dependencies import get_account_service, get_book_service
from rekenraam_api.app import app
from rekenraam_api.schemas.accounts import AccountSummary
from rekenraam_api.schemas.books import BookSummary


class StubBookService:
    async def list_books(self) -> list[BookSummary]:
        return [
            BookSummary(
                id=1,
                slug="personal",
                name="Personal",
                base_currency_code="USD",
            )
        ]

    async def get_book_by_slug(self, slug: str) -> BookSummary | None:
        if slug == "personal":
            return BookSummary(
                id=1,
                slug="personal",
                name="Personal",
                base_currency_code="USD",
            )
        return None

    async def get_schema_version(self) -> str:
        return "0002_add_accounts"


class StubAccountService:
    _created_at = datetime(2026, 5, 3, tzinfo=UTC)

    async def list_accounts(self) -> list[AccountSummary]:
        return [
            AccountSummary(
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
            AccountSummary(
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

    async def get_account_by_id(self, account_id: int) -> AccountSummary | None:
        if account_id == 1:
            return AccountSummary(
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


@pytest.fixture(autouse=True)
def clear_dependency_overrides() -> AsyncIterator[None]:
    app.dependency_overrides.clear()
    yield
    app.dependency_overrides.clear()


@pytest.fixture()
async def client() -> AsyncIterator[AsyncClient]:
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://testserver") as async_client:
        yield async_client


@pytest.mark.asyncio
async def test_health_returns_expected_payload(client: AsyncClient) -> None:
    app.dependency_overrides[get_book_service] = StubBookService

    response = await client.get("/api/v1/health")

    assert response.status_code == 200
    assert response.json() == {
        "status": "ok",
        "service": "rekenraam-api",
        "database": "ok",
        "schema_version": "0002_add_accounts",
    }


@pytest.mark.asyncio
async def test_list_books_returns_seeded_book_shape(client: AsyncClient) -> None:
    app.dependency_overrides[get_book_service] = StubBookService

    response = await client.get("/api/v1/books")

    assert response.status_code == 200
    assert response.json() == [
        {"id": 1, "slug": "personal", "name": "Personal", "base_currency_code": "USD"}
    ]


@pytest.mark.asyncio
async def test_get_book_returns_404_for_missing_slug(client: AsyncClient) -> None:
    app.dependency_overrides[get_book_service] = StubBookService

    response = await client.get("/api/v1/books/missing")

    assert response.status_code == 404
    assert response.json() == {"detail": "book not found"}


@pytest.mark.asyncio
async def test_list_accounts_returns_flat_account_collection(client: AsyncClient) -> None:
    app.dependency_overrides[get_account_service] = StubAccountService

    response = await client.get("/api/v1/accounts")

    assert response.status_code == 200
    body = response.json()
    assert [item["id"] for item in body] == [1, 2]
    assert body[1]["parent_id"] == 1
    assert body[1]["name"] == "Cash"


@pytest.mark.asyncio
async def test_get_account_returns_404_for_missing_id(client: AsyncClient) -> None:
    app.dependency_overrides[get_account_service] = StubAccountService

    response = await client.get("/api/v1/accounts/999")

    assert response.status_code == 404
    assert response.json() == {"detail": "account not found"}


@pytest.mark.asyncio
async def test_get_account_returns_expected_payload(client: AsyncClient) -> None:
    app.dependency_overrides[get_account_service] = StubAccountService

    response = await client.get("/api/v1/accounts/1")

    assert response.status_code == 200
    assert response.json()["name"] == "Assets"
    assert response.json()["account_type"] == "asset"