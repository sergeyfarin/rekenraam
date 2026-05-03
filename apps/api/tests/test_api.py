from collections.abc import AsyncIterator
from datetime import UTC, datetime

import pytest
from httpx import ASGITransport, AsyncClient

from rekenraam_api.api.dependencies import get_account_service, get_book_service
from rekenraam_api.api.dependencies import get_metadata_service
from rekenraam_api.api.dependencies import get_report_service
from rekenraam_api.api.dependencies import get_transaction_service
from rekenraam_api.app import app
from rekenraam_api.schemas.accounts import (
    AccountBalanceSummary,
    AccountBalancingCreateInput,
    AccountBalancingSummary,
    AccountClosingValidationResult,
    AccountDirectiveSummary,
    AccountSummary,
    AccountTreeNode,
    AccountUpdateInput,
)
from rekenraam_api.schemas.books import BookSummary
from rekenraam_api.schemas.metadata import (
    CategorySummary,
    CommoditySummary,
    CountrySummary,
    CurrencyActivationInput,
    CurrencySummary,
    InstitutionSummary,
    PayeeSummary,
    PersonSummary,
    ProjectSummary,
    TagSummary,
)
from rekenraam_api.schemas.reports import CashflowRow, CategorySpendRow, PayeeTotalRow
from rekenraam_api.schemas.register import RegisterEntry
from rekenraam_api.schemas.transactions import PayeeDefaults, SplitEntry, TransactionListFilters, TransactionMutationInput, TransactionSummary


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

    async def update_account(self, account_id: int, input: AccountUpdateInput) -> AccountSummary | None:
        if account_id != 1:
            return None
        return AccountSummary(
            id=account_id,
            book_id=input.book_id,
            parent_id=input.parent_id,
            account_type=input.account_type,
            name=input.name,
            commodity_id=input.commodity_id,
            institution_id=None,
            institution_name=None,
            country_id=None,
            country_name=None,
            number_last4=input.number_last4,
            is_closed=input.is_closed,
            is_hidden=False,
            is_system=False,
            system_role=None,
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def delete_account(self, account_id: int) -> bool:
        return account_id == 1

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

    async def create_account_balancing(self, input: AccountBalancingCreateInput) -> AccountBalancingSummary | None:
        if input.account_id == 99:
            return None
        return AccountBalancingSummary(
            id=8,
            account_id=input.account_id,
            as_of_date=input.as_of_date,
            balance_minor=input.balance_minor,
            memo=input.memo,
        )

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

    async def validate_account_closing(self, account_id: int) -> AccountClosingValidationResult | None:
        if account_id == 99:
            return None
        if account_id == 2:
            return AccountClosingValidationResult(valid=False, issues=("account balance is not zero",))
        return AccountClosingValidationResult(valid=True, issues=())

    async def get_account_booking_policy(self, account_id: int) -> str | None:
        if account_id == 99:
            return None
        if account_id == 2:
            raise ValueError("booking policy only applies to investment accounts")
        return "average"

    async def set_account_booking_policy(self, account_id: int, booking_policy: str) -> str | None:
        if account_id == 99:
            return None
        if account_id == 2:
            raise ValueError("booking policy only applies to investment accounts")
        if booking_policy == "bad-policy":
            raise ValueError("booking policy must be fifo, lifo, strict, or average")
        return booking_policy

    async def unlock_account_balancings(self, account_id: int, from_date: datetime.date, reason: str | None, confirm: bool) -> int:
        if account_id == 99:
            return 0
        if not confirm:
            raise ValueError("unlock not confirmed")
        return 2


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

    async def create_transaction(self, input: TransactionMutationInput) -> TransactionSummary:
        return TransactionSummary(
            id=3,
            book_id=input.book_id,
            occurred_date=input.txn_date,
            posted_date=input.txn_date,
            payee_id=input.payee_id,
            memo=input.memo,
            status=input.status,
            reference=input.reference,
            created_at=self._created_at,
            splits=(
                SplitEntry(
                    id=10,
                    tx_id=3,
                    account_id=input.splits[0].account_id,
                    commodity_id=input.splits[0].commodity_id,
                    amount_minor=input.splits[0].amount_minor,
                    category_id=input.splits[0].category_id,
                    tag_id=input.splits[0].tag_id,
                    person_id=input.splits[0].person_id,
                    project_id=input.splits[0].project_id,
                    share_bps=input.splits[0].share_bps,
                    memo=input.splits[0].memo,
                    created_at=self._created_at,
                ),
                SplitEntry(
                    id=11,
                    tx_id=3,
                    account_id=input.splits[1].account_id,
                    commodity_id=input.splits[1].commodity_id,
                    amount_minor=input.splits[1].amount_minor,
                    category_id=input.splits[1].category_id,
                    tag_id=input.splits[1].tag_id,
                    person_id=input.splits[1].person_id,
                    project_id=input.splits[1].project_id,
                    share_bps=input.splits[1].share_bps,
                    memo=input.splits[1].memo,
                    created_at=self._created_at,
                ),
            ),
        )

    async def update_transaction(self, transaction_id: int, input: TransactionMutationInput) -> TransactionSummary | None:
        if transaction_id != 1:
            return None
        return TransactionSummary(
            id=1,
            book_id=input.book_id,
            occurred_date=input.txn_date,
            posted_date=input.txn_date,
            payee_id=input.payee_id,
            memo=input.memo,
            status=input.status,
            reference=input.reference,
            created_at=self._created_at,
            splits=(
                SplitEntry(
                    id=1,
                    tx_id=1,
                    account_id=input.splits[0].account_id,
                    commodity_id=input.splits[0].commodity_id,
                    amount_minor=input.splits[0].amount_minor,
                    category_id=input.splits[0].category_id,
                    tag_id=input.splits[0].tag_id,
                    person_id=input.splits[0].person_id,
                    project_id=input.splits[0].project_id,
                    share_bps=input.splits[0].share_bps,
                    memo=input.splits[0].memo,
                    created_at=self._created_at,
                ),
                SplitEntry(
                    id=2,
                    tx_id=1,
                    account_id=input.splits[1].account_id,
                    commodity_id=input.splits[1].commodity_id,
                    amount_minor=input.splits[1].amount_minor,
                    category_id=input.splits[1].category_id,
                    tag_id=input.splits[1].tag_id,
                    person_id=input.splits[1].person_id,
                    project_id=input.splits[1].project_id,
                    share_bps=input.splits[1].share_bps,
                    memo=input.splits[1].memo,
                    created_at=self._created_at,
                ),
            ),
        )

    async def delete_transaction(self, transaction_id: int) -> bool:
        return transaction_id == 1

    async def duplicate_transaction(self, transaction_id: int, today: datetime.date) -> TransactionSummary | None:
        if transaction_id != 1:
            return None
        return TransactionSummary(
            id=4,
            book_id=1,
            occurred_date=today,
            posted_date=today,
            payee_id=None,
            memo="Initial opening balance",
            status="uncleared",
            reference=None,
            created_at=self._created_at,
            splits=(
                SplitEntry(
                    id=12,
                    tx_id=4,
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
                    id=13,
                    tx_id=4,
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
        )

    async def bulk_void_transactions(self, transaction_ids: list[int]) -> int:
        return len([transaction_id for transaction_id in transaction_ids if transaction_id in {1, 2}])

    async def bulk_delete_transactions(self, transaction_ids: list[int]) -> int:
        return len([transaction_id for transaction_id in transaction_ids if transaction_id in {1, 2}])

    async def get_payee_defaults(self, payee_id: int, account_id: int | None = None) -> PayeeDefaults:
        if payee_id == 1 and account_id in {None, 2}:
            return PayeeDefaults(category_id=1, memo="Pending groceries")
        return PayeeDefaults(category_id=None, memo=None)

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

    async def update_commodity(self, commodity_id: int, input: object) -> CommoditySummary | None:
        if commodity_id != 1:
            return None
        return CommoditySummary(
            id=1,
            book_id=1,
            kind="currency",
            symbol="USDX",
            name="US Dollar Updated",
            scale=2,
            metadata="Primary currency",
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def list_currencies(self, book_id: int) -> list[CurrencySummary]:
        return [
            CurrencySummary(
                id=1,
                book_id=book_id,
                symbol="USD",
                display_symbol="USD",
                name="US Dollar",
                scale=2,
                is_active=True,
                is_default=True,
                created_at=self._created_at,
                updated_at=self._created_at,
            )
        ]

    async def create_currency(self, input: object) -> CurrencySummary:
        return CurrencySummary(
            id=2,
            book_id=1,
            symbol="EUR",
            display_symbol="EUR",
            name="Euro",
            scale=2,
            is_active=True,
            is_default=False,
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def update_currency(self, currency_id: int, input: object) -> CurrencySummary | None:
        if currency_id != 1:
            return None
        return CurrencySummary(
            id=1,
            book_id=1,
            symbol="USD",
            display_symbol="USD",
            name="US Dollar",
            scale=2,
            is_active=True,
            is_default=True,
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def set_default_currency(self, *, book_id: int, currency_id: int) -> CurrencySummary | None:
        if currency_id != 1:
            return None
        return CurrencySummary(
            id=1,
            book_id=book_id,
            symbol="USD",
            display_symbol="USD",
            name="US Dollar",
            scale=2,
            is_active=True,
            is_default=True,
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def set_currency_active(self, *, currency_id: int, input: CurrencyActivationInput) -> CurrencySummary | None:
        if currency_id != 1:
            return None
        return CurrencySummary(
            id=1,
            book_id=input.book_id,
            symbol="USD",
            display_symbol="USD",
            name="US Dollar",
            scale=2,
            is_active=input.is_active,
            is_default=True,
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def list_countries(self) -> list[CountrySummary]:
        return []

    async def list_institutions(self) -> list[InstitutionSummary]:
        return [
            InstitutionSummary(
                id=1,
                book_id=1,
                name="Example Bank",
                kind="bank",
                routing="123456789",
                website="https://example.test",
                metadata="Primary",
                country_id=None,
                country_name=None,
                created_at=self._created_at,
                updated_at=self._created_at,
            )
        ]

    async def create_institution(self, input: object) -> InstitutionSummary:
        return InstitutionSummary(
            id=2,
            book_id=1,
            name="New Bank",
            kind="bank",
            routing="111222333",
            website="https://newbank.test",
            metadata="Note",
            country_id=None,
            country_name=None,
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def update_institution(self, institution_id: int, input: object) -> InstitutionSummary | None:
        if institution_id != 1:
            return None
        return InstitutionSummary(
            id=1,
            book_id=1,
            name="Updated Bank",
            kind="brokerage",
            routing="333222111",
            website="https://updated.test",
            metadata="Updated",
            country_id=None,
            country_name=None,
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def delete_institution(self, institution_id: int) -> bool:
        return institution_id == 1

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

    async def update_category(self, category_id: int, input: object) -> CategorySummary | None:
        if category_id != 1:
            return None
        return CategorySummary(
            id=category_id,
            book_id=1,
            parent_id=None,
            name="Food",
            kind="expense",
            color="#111111",
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def delete_category(self, category_id: int) -> bool:
        return category_id == 1

    async def update_payee(self, payee_id: int, input: object) -> PayeeSummary | None:
        if payee_id != 1:
            return None
        return PayeeSummary(
            id=payee_id,
            book_id=1,
            name="Corner Shop",
            kind="business",
            metadata=None,
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def delete_payee(self, payee_id: int) -> bool:
        return payee_id == 1

    async def update_tag(self, tag_id: int, input: object) -> TagSummary | None:
        if tag_id != 1:
            return None
        return TagSummary(
            id=tag_id,
            book_id=1,
            name="Family",
            color="#222222",
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def delete_tag(self, tag_id: int) -> bool:
        return tag_id == 1


class StubReportService:
    async def report_cashflow(self, input: object) -> list[CashflowRow]:
        return [CashflowRow(period_start=datetime(2026, 5, 1, tzinfo=UTC).date(), inflow_minor=500000, outflow_minor=500000, net_minor=0)]

    async def report_category_spend(self, input: object) -> list[CategorySpendRow]:
        return [CategorySpendRow(category_id=1, category_name="Groceries", total_minor=-1250)]

    async def report_payee_totals(self, input: object) -> list[PayeeTotalRow]:
        return [PayeeTotalRow(payee_id=1, payee_name="Local Market", total_minor=-1250)]


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
async def test_health_preflight_allows_local_frontend_origin(client: AsyncClient) -> None:
    app.dependency_overrides[get_book_service] = StubBookService

    response = await client.options(
        "/api/v1/health",
        headers={
            "Origin": "http://localhost:3000",
            "Access-Control-Request-Method": "GET",
        },
    )

    assert response.status_code == 200
    assert response.headers["access-control-allow-origin"] == "http://localhost:3000"


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
async def test_account_write_endpoints_return_expected_shapes(client: AsyncClient) -> None:
    app.dependency_overrides[get_account_service] = StubAccountService

    update_response = await client.put(
        "/api/v1/accounts/1",
        json={
            "book_id": 1,
            "parent_id": None,
            "account_type": "asset",
            "name": "Renamed Assets",
            "commodity_id": 1,
            "institution_id": None,
            "country_id": None,
            "number_last4": None,
            "is_closed": False,
        },
    )
    validation_response = await client.get("/api/v1/accounts/2/closing-validation")
    balancing_response = await client.post(
        "/api/v1/accounts/1/balancings",
        json={
            "book_id": 1,
            "account_id": 1,
            "as_of_date": "2026-05-03",
            "balance_minor": 500000,
            "memo": "Checkpoint",
        },
    )
    delete_response = await client.delete("/api/v1/accounts/1")

    assert update_response.status_code == 200
    assert update_response.json()["name"] == "Renamed Assets"
    assert validation_response.status_code == 200
    assert validation_response.json() == {"valid": False, "issues": ["account balance is not zero"]}
    assert balancing_response.status_code == 200
    assert balancing_response.json()["account_id"] == 1
    assert delete_response.status_code == 204


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
async def test_set_account_booking_policy_updates_policy(client: AsyncClient) -> None:
    app.dependency_overrides[get_account_service] = StubAccountService

    response = await client.put("/api/v1/accounts/9/booking-policy", json={"booking_policy": "lifo"})

    assert response.status_code == 200
    assert response.json() == "lifo"


@pytest.mark.asyncio
async def test_set_account_booking_policy_rejects_invalid_value(client: AsyncClient) -> None:
    app.dependency_overrides[get_account_service] = StubAccountService

    response = await client.put("/api/v1/accounts/9/booking-policy", json={"booking_policy": "bad-policy"})

    assert response.status_code == 400
    assert response.json() == {"detail": "booking policy must be fifo, lifo, strict, or average"}


@pytest.mark.asyncio
async def test_unlock_account_balancings_returns_count(client: AsyncClient) -> None:
    app.dependency_overrides[get_account_service] = StubAccountService

    response = await client.post(
        "/api/v1/accounts/1/balancings/unlock",
        json={"from_date": "2026-05-02", "reason": "retry", "confirm": True},
    )

    assert response.status_code == 200
    assert response.json() == 2


@pytest.mark.asyncio
async def test_unlock_account_balancings_requires_confirmation(client: AsyncClient) -> None:
    app.dependency_overrides[get_account_service] = StubAccountService

    response = await client.post(
        "/api/v1/accounts/1/balancings/unlock",
        json={"from_date": "2026-05-02", "reason": None, "confirm": False},
    )

    assert response.status_code == 409
    assert response.json() == {"detail": "unlock not confirmed"}


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
    currencies_response = await client.get("/api/v1/currencies")
    countries_response = await client.get("/api/v1/countries")
    institutions_response = await client.get("/api/v1/institutions")
    categories_response = await client.get("/api/v1/categories")
    payees_response = await client.get("/api/v1/payees")
    tags_response = await client.get("/api/v1/tags")
    people_response = await client.get("/api/v1/people")
    projects_response = await client.get("/api/v1/projects")

    assert commodities_response.status_code == 200
    assert commodities_response.json()[0]["name"] == "US Dollar"
    assert currencies_response.status_code == 200
    assert currencies_response.json()[0]["is_default"] is True
    assert countries_response.status_code == 200
    assert countries_response.json() == []
    assert institutions_response.status_code == 200
    assert institutions_response.json()[0]["routing"] == "123456789"
    assert institutions_response.json()[0]["website"] == "https://example.test"
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
async def test_metadata_write_endpoints_update_and_delete_categories_payees_tags_and_institutions(client: AsyncClient) -> None:
    app.dependency_overrides[get_metadata_service] = StubMetadataService

    commodity_response = await client.put(
        "/api/v1/commodities/1",
        json={"book_id": 1, "symbol": "USDX", "name": "US Dollar Updated", "metadata": "Primary currency"},
    )
    list_currencies_response = await client.get("/api/v1/currencies")
    create_currency_response = await client.post(
        "/api/v1/currencies",
        json={"book_id": 1, "symbol": "EUR", "display_symbol": "EUR", "name": "Euro", "scale": 2},
    )
    update_currency_response = await client.put(
        "/api/v1/currencies/1",
        json={"book_id": 1, "symbol": "USD", "display_symbol": "USD", "name": "US Dollar", "scale": 2},
    )
    default_currency_response = await client.post("/api/v1/currencies/1/default")
    activate_currency_response = await client.post(
        "/api/v1/currencies/1/activation",
        json={"book_id": 1, "is_active": True},
    )
    create_institution_response = await client.post(
        "/api/v1/institutions",
        json={
            "book_id": 1,
            "name": "New Bank",
            "kind": "bank",
            "routing": "111222333",
            "website": "https://newbank.test",
            "metadata": "Note",
            "country_id": None,
        },
    )
    update_institution_response = await client.put(
        "/api/v1/institutions/1",
        json={
            "book_id": 1,
            "name": "Updated Bank",
            "kind": "brokerage",
            "routing": "333222111",
            "website": "https://updated.test",
            "metadata": "Updated",
            "country_id": None,
        },
    )
    category_response = await client.put(
        "/api/v1/categories/1",
        json={"book_id": 1, "parent_id": None, "name": "Food", "kind": "expense", "color": "#111111"},
    )
    payee_response = await client.put(
        "/api/v1/payees/1",
        json={"book_id": 1, "name": "Corner Shop", "kind": "business", "metadata": None},
    )
    tag_response = await client.put(
        "/api/v1/tags/1",
        json={"book_id": 1, "name": "Family", "color": "#222222"},
    )
    delete_category_response = await client.delete("/api/v1/categories/1")
    delete_payee_response = await client.delete("/api/v1/payees/1")
    delete_tag_response = await client.delete("/api/v1/tags/1")
    delete_institution_response = await client.delete("/api/v1/institutions/1")

    assert commodity_response.status_code == 200
    assert commodity_response.json()["symbol"] == "USDX"
    assert list_currencies_response.status_code == 200
    assert list_currencies_response.json()[0]["symbol"] == "USD"
    assert create_currency_response.status_code == 200
    assert create_currency_response.json()["symbol"] == "EUR"
    assert update_currency_response.status_code == 200
    assert update_currency_response.json()["symbol"] == "USD"
    assert default_currency_response.status_code == 200
    assert default_currency_response.json()["is_default"] is True
    assert activate_currency_response.status_code == 200
    assert activate_currency_response.json()["is_active"] is True
    assert create_institution_response.status_code == 200
    assert create_institution_response.json()["routing"] == "111222333"
    assert update_institution_response.status_code == 200
    assert update_institution_response.json()["kind"] == "brokerage"
    assert category_response.status_code == 200
    assert category_response.json()["name"] == "Food"
    assert payee_response.status_code == 200
    assert payee_response.json()["name"] == "Corner Shop"
    assert tag_response.status_code == 200
    assert tag_response.json()["name"] == "Family"
    assert delete_category_response.status_code == 204
    assert delete_payee_response.status_code == 204
    assert delete_tag_response.status_code == 204
    assert delete_institution_response.status_code == 204


@pytest.mark.asyncio
async def test_report_endpoints_return_expected_shapes(client: AsyncClient) -> None:
    app.dependency_overrides[get_report_service] = StubReportService

    cashflow_response = await client.post("/api/v1/reports/cashflow", json={"book_id": 1, "date_from": None, "date_to": None, "group_by": "month"})
    category_response = await client.post("/api/v1/reports/category-spend", json={"book_id": 1, "date_from": None, "date_to": None, "category_ids": None})
    payee_response = await client.post("/api/v1/reports/payee-totals", json={"book_id": 1, "date_from": None, "date_to": None, "payee_ids": None})

    assert cashflow_response.status_code == 200
    assert cashflow_response.json()[0]["period_start"] == "2026-05-01"
    assert category_response.status_code == 200
    assert category_response.json()[0]["category_name"] == "Groceries"
    assert payee_response.status_code == 200
    assert payee_response.json()[0]["payee_name"] == "Local Market"


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
async def test_transaction_write_endpoints_return_expected_shapes(client: AsyncClient) -> None:
    app.dependency_overrides[get_transaction_service] = StubTransactionService
    payload = {
        "book_id": 1,
        "txn_date": "2026-05-03",
        "payee_id": None,
        "memo": "Created",
        "status": "uncleared",
        "reference": None,
        "import_id": None,
        "splits": [
            {
                "account_id": 2,
                "commodity_id": 1,
                "amount_minor": 100,
                "category_id": None,
                "tag_id": None,
                "person_id": None,
                "project_id": None,
                "share_bps": None,
                "memo": None,
            },
            {
                "account_id": 3,
                "commodity_id": 1,
                "amount_minor": -100,
                "category_id": None,
                "tag_id": None,
                "person_id": None,
                "project_id": None,
                "share_bps": None,
                "memo": None,
            },
        ],
    }

    create_response = await client.post("/api/v1/transactions", json=payload)
    update_response = await client.put("/api/v1/transactions/1", json=payload)
    delete_response = await client.delete("/api/v1/transactions/1")

    assert create_response.status_code == 200
    assert create_response.json()["id"] == 3
    assert update_response.status_code == 200
    assert update_response.json()["id"] == 1
    assert delete_response.status_code == 204


@pytest.mark.asyncio
async def test_transaction_duplicate_and_bulk_endpoints_return_expected_shapes(client: AsyncClient) -> None:
    app.dependency_overrides[get_transaction_service] = StubTransactionService

    duplicate_response = await client.post("/api/v1/transactions/1/duplicate", params={"today": "2026-05-04"})
    bulk_void_response = await client.post("/api/v1/transactions/bulk-void", json=[1, 2, 999])
    bulk_delete_response = await client.post("/api/v1/transactions/bulk-delete", json=[1, 2, 999])

    assert duplicate_response.status_code == 200
    assert duplicate_response.json()["id"] == 4
    assert bulk_void_response.status_code == 200
    assert bulk_void_response.json() == 2
    assert bulk_delete_response.status_code == 200
    assert bulk_delete_response.json() == 2


@pytest.mark.asyncio
async def test_get_payee_defaults_returns_expected_shape(client: AsyncClient) -> None:
    app.dependency_overrides[get_transaction_service] = StubTransactionService

    response = await client.get("/api/v1/transactions/payee-defaults", params={"payee_id": 1, "account_id": 2})

    assert response.status_code == 200
    assert response.json() == {"category_id": 1, "memo": "Pending groceries"}


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