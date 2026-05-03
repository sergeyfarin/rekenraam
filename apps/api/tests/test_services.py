from datetime import UTC, datetime

import pytest
from pydantic_core import ValidationError

from rekenraam_api.db.models.accounts import Account, AccountBalancing
from rekenraam_api.db.models.books import Book
from rekenraam_api.db.models.transactions import Split, Transaction
from rekenraam_api.schemas.accounts import (
    AccountBalanceSummary,
    AccountBalancingSummary,
    AccountDirectiveSummary,
    AccountTreeNode,
)
from rekenraam_api.schemas.register import RegisterEntry
from rekenraam_api.schemas.transactions import TransactionListFilters
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
                commodity_id=1,
                number_last4=None,
                is_closed=False,
                is_hidden=False,
                is_system=False,
                system_role=None,
                created_at=self._created_at,
                updated_at=self._created_at,
            ),
            Account(
                id=2,
                book_id=1,
                parent_id=1,
                account_type="asset",
                name="Cash",
                commodity_id=1,
                number_last4="1234",
                is_closed=False,
                is_hidden=False,
                is_system=False,
                system_role=None,
                created_at=self._created_at,
                updated_at=self._created_at,
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
                commodity_id=1,
                number_last4=None,
                is_closed=False,
                is_hidden=False,
                is_system=False,
                system_role=None,
                created_at=self._created_at,
                updated_at=self._created_at,
            )
        if account_id == 9:
            return Account(
                id=9,
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
                effective_at=datetime(2026, 5, 3, tzinfo=UTC).date(),
                lifecycle_event="open",
                lifecycle_note=None,
                lifecycle_metadata=None,
                created_at=self._created_at,
                updated_at=self._created_at,
            )
        return None

    async def get_book_base_currency_code(self, book_id: int) -> str | None:
        if book_id == 1:
            return "USD"
        return None

    async def get_account_balances(self) -> dict[int, int]:
        return {2: 500000, 3: -500000}

    async def list_account_balancings(self, account_id: int) -> list[AccountBalancing]:
        if account_id != 1:
            return []
        return [
            AccountBalancing(
                id=7,
                book_id=1,
                account_id=1,
                previous_account_balancing_id=None,
                as_of_date=datetime(2026, 5, 2, tzinfo=UTC).date(),
                balance_minor=500000,
                memo="Checkpoint",
                created_at=self._created_at,
                voided_at=None,
                void_reason=None,
            )
        ]

    async def list_account_directives(self, account_id: int) -> list[Account]:
        if account_id != 1:
            return []
        return [
            Account(
                id=1,
                book_id=1,
                parent_id=None,
                previous_account_id=None,
                account_type="asset",
                name="Assets",
                commodity_id=1,
                booking_policy="fifo",
                number_last4=None,
                is_closed=False,
                is_hidden=False,
                is_system=False,
                system_role=None,
                effective_at=datetime(2026, 5, 1, tzinfo=UTC).date(),
                lifecycle_event="open",
                lifecycle_note="Opened",
                lifecycle_metadata='{"source":"seed"}',
                created_at=self._created_at,
                updated_at=self._created_at,
            )
        ]

    async def get_account_booking_policy(self, account_id: int) -> tuple[Account | None, str | None]:
        account = await self.get_account_by_id(account_id)
        if account is None:
            return None, None
        return account, account.booking_policy

    async def set_account_booking_policy(self, account_id: int, booking_policy: str) -> Account | None:
        account = await self.get_account_by_id(account_id)
        if account is None:
            return None
        account.booking_policy = booking_policy
        return account

    async def unlock_account_balancings(self, account_id: int, from_date: datetime.date, reason: str | None) -> int:
        if account_id != 1:
            return 0
        return 2


class StubTransactionRepository:
    _created_at = datetime(2026, 5, 3, tzinfo=UTC)
    last_filters: TransactionListFilters | None = None

    async def list_transactions(self, filters: TransactionListFilters | None = None) -> list[Transaction]:
        self.last_filters = filters
        transactions = [
            Transaction(
                id=2,
                book_id=1,
                occurred_date=datetime(2026, 5, 2, tzinfo=UTC).date(),
                posted_date=datetime(2026, 5, 2, tzinfo=UTC).date(),
                payee_id=1,
                memo="Pending groceries",
                status="uncleared",
                reference="groceries-1",
                created_at=self._created_at,
            ),
            Transaction(
                id=1,
                book_id=1,
                occurred_date=datetime(2026, 5, 1, tzinfo=UTC).date(),
                posted_date=datetime(2026, 5, 1, tzinfo=UTC).date(),
                payee_id=None,
                memo="Initial opening balance",
                status="cleared",
                reference=None,
                created_at=self._created_at,
            ),
        ]
        if filters is None:
            return transactions
        return [
            transaction
            for transaction in transactions
            if (filters.status is None or transaction.status == filters.status)
            and (filters.occurred_from is None or transaction.occurred_date >= filters.occurred_from)
            and (filters.occurred_to is None or transaction.occurred_date <= filters.occurred_to)
        ]

    async def get_transaction_by_id(self, transaction_id: int) -> Transaction | None:
        if transaction_id == 1:
            return (await self.list_transactions())[0]
        return None

    async def list_splits_for_transaction_ids(self, transaction_ids: list[int]) -> list[Split]:
        if 1 not in transaction_ids:
            return []
        return [
            Split(
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
            Split(
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
        ]

    async def list_account_register_splits(self, account_id: int) -> list[tuple[Transaction, Split]]:
        if account_id != 2:
            return []
        transaction = next(
            transaction
            for transaction in await self.list_transactions()
            if transaction.id == 1
        )
        split = Split(
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
        )
        return [(transaction, split)]


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
async def test_account_service_maps_parent_and_system_flags() -> None:
    service = AccountService(StubAccountRepository())

    result = await service.list_accounts()

    assert [account.id for account in result] == [1, 2]
    assert result[1].parent_id == 1
    assert result[1].commodity_id == 1
    assert result[1].number_last4 == "1234"
    assert result[0].is_system is False


@pytest.mark.asyncio
async def test_account_service_returns_none_when_account_is_missing() -> None:
    service = AccountService(StubAccountRepository())

    result = await service.get_account_by_id(999)

    assert result is None


@pytest.mark.asyncio
async def test_account_service_lists_account_balances() -> None:
    service = AccountService(StubAccountRepository())

    result = await service.list_account_balances()

    assert result == [
        AccountBalanceSummary(account_id=2, balance_minor=500000),
        AccountBalanceSummary(account_id=3, balance_minor=-500000),
    ]


@pytest.mark.asyncio
async def test_account_service_lists_account_balancings() -> None:
    service = AccountService(StubAccountRepository())

    result = await service.list_account_balancings(1)

    assert result == [
        AccountBalancingSummary(
            id=7,
            account_id=1,
            as_of_date=datetime(2026, 5, 2, tzinfo=UTC).date(),
            balance_minor=500000,
            memo="Checkpoint",
        )
    ]


@pytest.mark.asyncio
async def test_account_service_lists_account_directives() -> None:
    service = AccountService(StubAccountRepository())

    result = await service.list_account_directives(1)

    assert result == [
        AccountDirectiveSummary(
            id=1,
            book_id=1,
            account_id=1,
            directive_type="open",
            directive_date=datetime(2026, 5, 1, tzinfo=UTC).date(),
            note="Opened",
            metadata='{"source":"seed"}',
            created_at=datetime(2026, 5, 3, tzinfo=UTC),
        )
    ]


@pytest.mark.asyncio
async def test_account_service_returns_booking_policy_for_investment_account() -> None:
    service = AccountService(StubAccountRepository())

    result = await service.get_account_booking_policy(9)

    assert result == "average"


@pytest.mark.asyncio
async def test_account_service_rejects_booking_policy_for_non_investment_account() -> None:
    service = AccountService(StubAccountRepository())

    with pytest.raises(ValueError, match="booking policy only applies"):
        await service.get_account_booking_policy(1)


@pytest.mark.asyncio
async def test_account_service_updates_booking_policy_for_investment_account() -> None:
    service = AccountService(StubAccountRepository())

    result = await service.set_account_booking_policy(9, "lifo")

    assert result == "lifo"


@pytest.mark.asyncio
async def test_account_service_rejects_invalid_booking_policy_updates() -> None:
    service = AccountService(StubAccountRepository())

    with pytest.raises(ValueError, match="booking policy must be fifo, lifo, strict, or average"):
        await service.set_account_booking_policy(9, "bad-policy")


@pytest.mark.asyncio
async def test_account_service_unlocks_balancings_when_confirmed() -> None:
    service = AccountService(StubAccountRepository())

    result = await service.unlock_account_balancings(1, datetime(2026, 5, 2, tzinfo=UTC).date(), "retry", True)

    assert result == 2


@pytest.mark.asyncio
async def test_account_service_rejects_unlock_without_confirmation() -> None:
    service = AccountService(StubAccountRepository())

    with pytest.raises(ValueError, match="unlock not confirmed"):
        await service.unlock_account_balancings(1, datetime(2026, 5, 2, tzinfo=UTC).date(), None, False)


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

    assert len(result) == 2
    assert result[0].memo == "Pending groceries"
    assert result[0].payee_id == 1
    assert result[0].reference == "groceries-1"
    assert len(result[1].splits) == 2
    assert result[1].splits[0].amount_minor == 500000
    assert result[1].splits[0].commodity_id == 1


@pytest.mark.asyncio
async def test_transaction_service_passes_filters_to_repository() -> None:
    repository = StubTransactionRepository()
    service = TransactionService(repository)
    filters = TransactionListFilters(status="cleared", occurred_to=datetime(2026, 5, 1, tzinfo=UTC).date())

    result = await service.list_transactions(filters)

    assert repository.last_filters == filters
    assert [transaction.id for transaction in result] == [1]


@pytest.mark.asyncio
async def test_transaction_service_returns_none_for_missing_transaction() -> None:
    service = TransactionService(StubTransactionRepository())

    result = await service.get_transaction_by_id(999)

    assert result is None


@pytest.mark.asyncio
async def test_transaction_service_builds_account_register_running_balance() -> None:
    service = TransactionService(StubTransactionRepository())

    result = await service.list_account_register(2)

    assert result == [
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
            created_at=StubTransactionRepository._created_at,
        )
    ]