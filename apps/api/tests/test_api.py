from collections.abc import AsyncIterator
from datetime import UTC, datetime

import pytest
from httpx import ASGITransport, AsyncClient

from rekenraam_api.api.dependencies import get_account_service, get_book_service
from rekenraam_api.api.dependencies import get_transaction_service
from rekenraam_api.app import app
from rekenraam_api.schemas.accounts import AccountSummary, AccountTreeNode
from rekenraam_api.schemas.books import BookSummary
from rekenraam_api.schemas.register import RegisterEntry
from rekenraam_api.schemas.transactions import SplitEntry, TransactionListFilters, TransactionSummary


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
                commodity_id=1,
                institution_id=None,
                institution_name=None,
                country_id=None,
                country_name=None,
                number_last4=None,
                is_closed=False,
                is_hidden=False,
                is_system=False,
                system_role=None,
                created_at=self._created_at,
                updated_at=self._created_at,
            ),
            AccountSummary(
                id=2,
                book_id=1,
                parent_id=1,
                account_type="asset",
                name="Cash",
                commodity_id=1,
                institution_id=None,
                institution_name=None,
                country_id=None,
                country_name=None,
                number_last4="1234",
                is_closed=False,
                is_hidden=False,
                is_system=False,
                system_role=None,
                created_at=self._created_at,
                updated_at=self._created_at,
            ),
        ]

    async def get_account_by_id(self, account_id: int) -> AccountSummary | None:
        if account_id in {1, 2}:
            return AccountSummary(
                id=account_id,
                book_id=1,
                parent_id=None if account_id == 1 else 1,
                account_type="asset",
                name="Assets" if account_id == 1 else "Cash",
                commodity_id=1,
                institution_id=None,
                institution_name=None,
                country_id=None,
                country_name=None,
                number_last4=None if account_id == 1 else "1234",
                is_closed=False,
                is_hidden=False,
                is_system=False,
                system_role=None,
                created_at=self._created_at,
                updated_at=self._created_at,
            )
        return None

    async def list_account_tree(self) -> list[AccountTreeNode]:
        return [
            AccountTreeNode(
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
                rollup_balance_minor=500000,
                children=(
                    AccountTreeNode(
                        id=2,
                        parent_id=1,
                        name="Cash",
                        account_type="asset",
                        commodity_id=1,
                        commodity_name="USD",
                        commodity_scale=2,
                        institution_name=None,
                        country_name=None,
                        balance_minor=500000,
                        rollup_balance_minor=500000,
                        children=(),
                    ),
                ),
            ),
            AccountTreeNode(
                id=3,
                parent_id=None,
                name="Opening Balances",
                account_type="equity",
                commodity_id=1,
                commodity_name="USD",
                commodity_scale=2,
                institution_name=None,
                country_name=None,
                balance_minor=-500000,
                rollup_balance_minor=-500000,
                children=(),
            ),
        ]


class StubTransactionService:
    _created_at = datetime(2026, 5, 3, tzinfo=UTC)
    last_filters: TransactionListFilters | None = None

    async def list_transactions(self, filters: TransactionListFilters | None = None) -> list[TransactionSummary]:
        self.last_filters = filters
        transactions = [
            TransactionSummary(
                id=1,
                book_id=1,
                occurred_date=datetime(2026, 5, 1, tzinfo=UTC).date(),
                posted_date=datetime(2026, 5, 1, tzinfo=UTC).date(),
                memo="Initial opening balance",
                status="cleared",
                created_at=self._created_at,
                splits=(
                    SplitEntry(
                        id=1,
                        tx_id=1,
                        account_id=2,
                        amount_minor=500000,
                        memo="Opening cash",
                        created_at=self._created_at,
                    ),
                    SplitEntry(
                        id=2,
                        tx_id=1,
                        account_id=3,
                        amount_minor=-500000,
                        memo="Offset",
                        created_at=self._created_at,
                    ),
                ),
            ),
            TransactionSummary(
                id=2,
                book_id=1,
                occurred_date=datetime(2026, 5, 2, tzinfo=UTC).date(),
                posted_date=datetime(2026, 5, 2, tzinfo=UTC).date(),
                memo="Pending groceries",
                status="uncleared",
                created_at=self._created_at,
                splits=(
                    SplitEntry(
                        id=3,
                        tx_id=2,
                        account_id=2,
                        amount_minor=-1250,
                        memo="Groceries",
                        created_at=self._created_at,
                    ),
                    SplitEntry(
                        id=4,
                        tx_id=2,
                        account_id=4,
                        amount_minor=1250,
                        memo="Expense",
                        created_at=self._created_at,
                    ),
                ),
            ),
        ]
        if filters is None:
            return transactions
        return [
            transaction
            for transaction in transactions
            if (filters.status is None or transaction.status == filters.status)
            and (filters.account_id is None or any(split.account_id == filters.account_id for split in transaction.splits))
            and (filters.occurred_from is None or transaction.occurred_date >= filters.occurred_from)
            and (filters.occurred_to is None or transaction.occurred_date <= filters.occurred_to)
        ]

    async def get_transaction_by_id(self, transaction_id: int) -> TransactionSummary | None:
        if transaction_id == 1:
            return (await self.list_transactions())[0]
        return None

    async def list_account_register(self, account_id: int) -> list[RegisterEntry]:
        if account_id != 2:
            return []
        return [
            RegisterEntry(
                tx_id=1,
                split_id=1,
                account_id=2,
                occurred_date=datetime(2026, 5, 1, tzinfo=UTC).date(),
                posted_date=datetime(2026, 5, 1, tzinfo=UTC).date(),
                memo="Initial opening balance",
                status="cleared",
                amount_minor=500000,
                running_balance_minor=500000,
                created_at=self._created_at,
            )
        ]


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


@pytest.mark.asyncio
async def test_list_account_tree_returns_nested_balance_shape(client: AsyncClient) -> None:
    app.dependency_overrides[get_account_service] = StubAccountService

    response = await client.get("/api/v1/accounts/tree")

    assert response.status_code == 200
    body = response.json()
    assert body[0]["commodity_name"] == "USD"
    assert body[0]["rollup_balance_minor"] == 500000
    assert body[0]["children"][0]["name"] == "Cash"
    assert body[0]["children"][0]["balance_minor"] == 500000


@pytest.mark.asyncio
async def test_list_transactions_returns_nested_splits(client: AsyncClient) -> None:
    app.dependency_overrides[get_transaction_service] = StubTransactionService

    response = await client.get("/api/v1/transactions")

    assert response.status_code == 200
    body = response.json()
    assert body[0]["memo"] == "Initial opening balance"
    assert len(body) == 2
    assert len(body[0]["splits"]) == 2
    assert body[0]["splits"][0]["amount_minor"] == 500000


@pytest.mark.asyncio
async def test_list_transactions_applies_query_filters(client: AsyncClient) -> None:
    service = StubTransactionService()
    app.dependency_overrides[get_transaction_service] = lambda: service

    response = await client.get(
        "/api/v1/transactions",
        params={"account_id": 2, "status": "uncleared", "occurred_from": "2026-05-02"},
    )

    assert response.status_code == 200
    body = response.json()
    assert [item["id"] for item in body] == [2]
    assert service.last_filters == TransactionListFilters(
        account_id=2,
        status="uncleared",
        occurred_from=datetime(2026, 5, 2, tzinfo=UTC).date(),
    )


@pytest.mark.asyncio
async def test_list_transactions_returns_422_for_invalid_date_range(client: AsyncClient) -> None:
    app.dependency_overrides[get_transaction_service] = StubTransactionService

    response = await client.get(
        "/api/v1/transactions",
        params={"occurred_from": "2026-05-03", "occurred_to": "2026-05-02"},
    )

    assert response.status_code == 422


@pytest.mark.asyncio
async def test_get_transaction_returns_404_for_missing_id(client: AsyncClient) -> None:
    app.dependency_overrides[get_transaction_service] = StubTransactionService

    response = await client.get("/api/v1/transactions/999")

    assert response.status_code == 404
    assert response.json() == {"detail": "transaction not found"}


@pytest.mark.asyncio
async def test_get_account_register_returns_running_balance_entries(client: AsyncClient) -> None:
    app.dependency_overrides[get_account_service] = StubAccountService
    app.dependency_overrides[get_transaction_service] = StubTransactionService

    response = await client.get("/api/v1/accounts/2/register")

    assert response.status_code == 200
    body = response.json()
    assert body[0]["amount_minor"] == 500000
    assert body[0]["running_balance_minor"] == 500000


@pytest.mark.asyncio
async def test_get_account_register_returns_404_for_missing_account(client: AsyncClient) -> None:
    app.dependency_overrides[get_account_service] = StubAccountService
    app.dependency_overrides[get_transaction_service] = StubTransactionService

    response = await client.get("/api/v1/accounts/999/register")

    assert response.status_code == 404
    assert response.json() == {"detail": "account not found"}