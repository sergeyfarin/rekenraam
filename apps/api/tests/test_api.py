from collections.abc import AsyncIterator
from datetime import UTC, datetime

import pytest
from httpx import ASGITransport, AsyncClient

from rekenraam_api.api.dependencies import get_account_service, get_book_service
from rekenraam_api.api.dependencies import get_metadata_service
from rekenraam_api.api.dependencies import get_transaction_service
from rekenraam_api.app import app
from rekenraam_api.schemas.accounts import (
    AccountBalanceSummary,
    AccountBalancingSummary,
    AccountDirectiveSummary,
    AccountSummary,
    AccountTreeNode,
)
from rekenraam_api.schemas.books import BookSummary
from rekenraam_api.schemas.metadata import (
    CategorySummary,
    CommoditySummary,
    CountrySummary,
    InstitutionSummary,
    PayeeSummary,
    PersonSummary,
    ProjectSummary,
    TagSummary,
)
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

    async def list_account_balances(self) -> list[AccountBalanceSummary]:
        return [
            AccountBalanceSummary(account_id=2, balance_minor=500000),
            AccountBalanceSummary(account_id=3, balance_minor=-500000),
        ]

    async def list_account_balancings(self, account_id: int) -> list[AccountBalancingSummary]:
        return [
            AccountBalancingSummary(
                id=7,
                account_id=account_id,
                as_of_date=datetime(2026, 5, 2, tzinfo=UTC).date(),
                balance_minor=500000,
                memo="Checkpoint",
            )
        ]

    async def list_account_directives(self, account_id: int) -> list[AccountDirectiveSummary]:
        return [
            AccountDirectiveSummary(
                id=2,
                book_id=1,
                account_id=account_id,
                directive_type="open",
                directive_date=datetime(2026, 5, 1, tzinfo=UTC).date(),
                note="Opened",
                metadata='{"source":"seed"}',
                created_at=self._created_at,
            )
        ]

    async def get_account_booking_policy(self, account_id: int) -> str | None:
        if account_id == 99:
            return None
        if account_id == 2:
            raise ValueError("booking policy only applies to investment accounts")
        return "average"


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
                payee_id=None,
                memo="Initial opening balance",
                status="cleared",
                reference=None,
                created_at=self._created_at,
                splits=(
                    SplitEntry(
                        id=1,
                        tx_id=1,
                        account_id=2,
                        commodity_id=1,
                        amount_minor=500000,
                        category_id=None,
                        tag_id=None,
                        person_id=None,
                        project_id=None,
                        share_bps=None,
                        memo="Opening cash",
                        created_at=self._created_at,
                    ),
                    SplitEntry(
                        id=2,
                        tx_id=1,
                        account_id=3,
                        commodity_id=1,
                        amount_minor=-500000,
                        category_id=None,
                        tag_id=None,
                        person_id=None,
                        project_id=None,
                        share_bps=None,
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
                payee_id=1,
                memo="Pending groceries",
                status="uncleared",
                reference="groceries-1",
                created_at=self._created_at,
                splits=(
                    SplitEntry(
                        id=3,
                        tx_id=2,
                        account_id=2,
                        commodity_id=1,
                        amount_minor=-1250,
                        category_id=1,
                        tag_id=None,
                        person_id=None,
                        project_id=None,
                        share_bps=None,
                        memo="Groceries",
                        created_at=self._created_at,
                    ),
                    SplitEntry(
                        id=4,
                        tx_id=2,
                        account_id=4,
                        commodity_id=1,
                        amount_minor=1250,
                        category_id=None,
                        tag_id=None,
                        person_id=None,
                        project_id=None,
                        share_bps=None,
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
                payee_id=None,
                memo="Initial opening balance",
                status="cleared",
                reference=None,
                commodity_id=1,
                category_id=None,
                amount_minor=500000,
                running_balance_minor=500000,
                created_at=self._created_at,
            )
        ]


class StubMetadataService:
    _created_at = datetime(2026, 5, 3, tzinfo=UTC)

    async def list_commodities(self) -> list[CommoditySummary]:
        return [
            CommoditySummary(
                id=1,
                book_id=1,
                kind="currency",
                symbol="USD",
                name="US Dollar",
                scale=2,
                metadata=None,
                created_at=self._created_at,
                updated_at=self._created_at,
            )
        ]

    async def list_countries(self) -> list[CountrySummary]:
        return []

    async def list_institutions(self) -> list[InstitutionSummary]:
        return []

    async def list_categories(self) -> list[CategorySummary]:
        return [
            CategorySummary(
                id=1,
                book_id=1,
                parent_id=None,
                name="Groceries",
                kind="expense",
                color="#00aa00",
                created_at=self._created_at,
                updated_at=self._created_at,
            )
        ]

    async def list_payees(self) -> list[PayeeSummary]:
        return [
            PayeeSummary(
                id=1,
                book_id=1,
                name="Local Market",
                kind="business",
                metadata=None,
                created_at=self._created_at,
                updated_at=self._created_at,
            )
        ]

    async def list_tags(self) -> list[TagSummary]:
        return []

    async def list_people(self) -> list[PersonSummary]:
        return []

    async def list_projects(self) -> list[ProjectSummary]:
        return []


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
async def test_list_account_balances_returns_balance_rows(client: AsyncClient) -> None:
    app.dependency_overrides[get_account_service] = StubAccountService

    response = await client.get("/api/v1/accounts/balances")

    assert response.status_code == 200
    assert response.json() == [
        {"account_id": 2, "balance_minor": 500000},
        {"account_id": 3, "balance_minor": -500000},
    ]


@pytest.mark.asyncio
async def test_list_account_balancings_returns_rows(client: AsyncClient) -> None:
    app.dependency_overrides[get_account_service] = StubAccountService

    response = await client.get("/api/v1/accounts/1/balancings")

    assert response.status_code == 200
    assert response.json() == [
        {
            "id": 7,
            "account_id": 1,
            "as_of_date": "2026-05-02",
            "balance_minor": 500000,
            "memo": "Checkpoint",
        }
    ]


@pytest.mark.asyncio
async def test_list_account_directives_returns_rows(client: AsyncClient) -> None:
    app.dependency_overrides[get_account_service] = StubAccountService

    response = await client.get("/api/v1/accounts/1/directives")

    assert response.status_code == 200
    assert response.json() == [
        {
            "id": 2,
            "book_id": 1,
            "account_id": 1,
            "directive_type": "open",
            "directive_date": "2026-05-01",
            "note": "Opened",
            "metadata": '{"source":"seed"}',
            "created_at": "2026-05-03T00:00:00Z",
        }
    ]


@pytest.mark.asyncio
async def test_get_account_booking_policy_returns_policy(client: AsyncClient) -> None:
    app.dependency_overrides[get_account_service] = StubAccountService

    response = await client.get("/api/v1/accounts/9/booking-policy")

    assert response.status_code == 200
    assert response.json() == "average"


@pytest.mark.asyncio
async def test_get_account_booking_policy_rejects_non_investment_account(client: AsyncClient) -> None:
    app.dependency_overrides[get_account_service] = StubAccountService

    response = await client.get("/api/v1/accounts/2/booking-policy")

    assert response.status_code == 400
    assert response.json() == {"detail": "booking policy only applies to investment accounts"}


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
    assert body[1]["payee_id"] == 1
    assert body[1]["reference"] == "groceries-1"
    assert body[1]["splits"][0]["commodity_id"] == 1


@pytest.mark.asyncio
async def test_metadata_endpoints_return_reference_shapes(client: AsyncClient) -> None:
    app.dependency_overrides[get_metadata_service] = StubMetadataService

    commodities_response = await client.get("/api/v1/commodities")
    countries_response = await client.get("/api/v1/countries")
    institutions_response = await client.get("/api/v1/institutions")
    categories_response = await client.get("/api/v1/categories")
    payees_response = await client.get("/api/v1/payees")
    tags_response = await client.get("/api/v1/tags")
    people_response = await client.get("/api/v1/people")
    projects_response = await client.get("/api/v1/projects")

    assert commodities_response.status_code == 200
    assert commodities_response.json()[0]["name"] == "US Dollar"
    assert countries_response.status_code == 200
    assert countries_response.json() == []
    assert institutions_response.status_code == 200
    assert institutions_response.json() == []
    assert categories_response.status_code == 200
    assert categories_response.json()[0]["name"] == "Groceries"
    assert payees_response.status_code == 200
    assert payees_response.json()[0]["name"] == "Local Market"
    assert tags_response.status_code == 200
    assert tags_response.json() == []
    assert people_response.status_code == 200
    assert people_response.json() == []
    assert projects_response.status_code == 200
    assert projects_response.json() == []


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
    assert body[0]["payee_id"] is None
    assert body[0]["reference"] is None
    assert body[0]["commodity_id"] == 1
    assert body[0]["category_id"] is None


@pytest.mark.asyncio
async def test_get_account_register_returns_404_for_missing_account(client: AsyncClient) -> None:
    app.dependency_overrides[get_account_service] = StubAccountService
    app.dependency_overrides[get_transaction_service] = StubTransactionService

    response = await client.get("/api/v1/accounts/999/register")

    assert response.status_code == 404
    assert response.json() == {"detail": "account not found"}