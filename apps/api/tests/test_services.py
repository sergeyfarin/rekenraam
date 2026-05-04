from datetime import UTC, datetime

import pytest
from pydantic_core import ValidationError

from rekenraam_api.db.models.accounts import Account, AccountBalancing
from rekenraam_api.db.models.books import Book
from rekenraam_api.db.models.transactions import Split, Transaction
from rekenraam_api.schemas.accounts import (
    AccountBalanceSummary,
    AccountBalancingCreateInput,
    AccountBalancingSummary,
    AccountClosingValidationResult,
    AccountCreateInput,
    AccountDirectiveSummary,
    AccountTreeNode,
    AccountUpdateInput,
)
from rekenraam_api.schemas.register import RegisterEntry, RegisterPage
from rekenraam_api.schemas.transactions import PayeeDefaults, TransactionListFilters, TransactionMutationInput, TransactionPage
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

    async def update_account(
        self,
        *,
        account_id: int,
        parent_id: int | None,
        account_type: str,
        name: str,
        commodity_id: int,
        institution_id: int | None,
        country_id: int | None,
        number_last4: str | None,
        is_closed: bool,
    ) -> Account | None:
        if account_id != 1:
            return None
        return Account(
            id=account_id,
            book_id=1,
            parent_id=parent_id,
            previous_account_id=None,
            account_type=account_type,
            name=name,
            commodity_id=commodity_id,
            booking_policy="fifo",
            number_last4=number_last4,
            is_closed=is_closed,
            is_hidden=False,
            is_system=False,
            system_role=None,
            effective_at=datetime(2026, 5, 3, tzinfo=UTC).date(),
            lifecycle_event="update",
            lifecycle_note=None,
            lifecycle_metadata=None,
            created_at=self._created_at,
            updated_at=self._created_at,
        )

    async def delete_account(self, account_id: int) -> bool:
        return account_id == 1

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

    async def create_account_balancing(
        self,
        *,
        book_id: int,
        account_id: int,
        as_of_date: datetime.date,
        balance_minor: int,
        memo: str | None,
        created_by_user_id: int | None = None,
        created_session_id: int | None = None,
        created_device_id: int | None = None,
        created_request_id: str | None = None,
    ) -> AccountBalancing | None:
        if account_id != 1:
            return None
        return AccountBalancing(
            id=9,
            book_id=book_id,
            account_id=account_id,
            previous_account_balancing_id=None,
            as_of_date=as_of_date,
            balance_minor=balance_minor,
            memo=memo,
            created_at=self._created_at,
            voided_at=None,
            void_reason=None,
        )


class StubTransactionRepository:
    _created_at = datetime(2026, 5, 3, tzinfo=UTC)
    last_filters: TransactionListFilters | None = None

    async def list_transactions(
        self,
        filters: TransactionListFilters | None = None,
        allowed_book_ids: list[int] | None = None,
    ) -> tuple[list[Transaction], str | None]:
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
            return transactions, None
        return [
            transaction
            for transaction in transactions
            if (filters.status is None or transaction.status == filters.status)
            and (filters.occurred_from is None or transaction.occurred_date >= filters.occurred_from)
            and (filters.occurred_to is None or transaction.occurred_date <= filters.occurred_to)
        ], None

    async def get_transaction_by_id(self, transaction_id: int) -> Transaction | None:
        if transaction_id == 3:
            return Transaction(
                id=3,
                book_id=1,
                occurred_date=datetime(2026, 5, 3, tzinfo=UTC).date(),
                posted_date=datetime(2026, 5, 3, tzinfo=UTC).date(),
                payee_id=None,
                memo="Created",
                status="uncleared",
                reference=None,
                created_at=self._created_at,
            )
        if transaction_id == 4:
            return Transaction(
                id=4,
                book_id=1,
                occurred_date=datetime(2026, 5, 4, tzinfo=UTC).date(),
                posted_date=datetime(2026, 5, 4, tzinfo=UTC).date(),
                payee_id=None,
                memo="Initial opening balance",
                status="uncleared",
                reference=None,
                created_at=self._created_at,
            )
        for transaction in (await self.list_transactions())[0]:
            if transaction.id == transaction_id:
                return transaction
        return None

    async def create_transaction(
        self,
        *,
        book_id: int,
        txn_date: datetime.date,
        payee_id: int | None,
        memo: str | None,
        status: str,
        reference: str | None,
        previous_tx_id: int | None = None,
        created_by_user_id: int | None = None,
        created_session_id: int | None = None,
        created_device_id: int | None = None,
        created_request_id: str | None = None,
    ) -> Transaction:
        return Transaction(
            id=3,
            book_id=book_id,
            occurred_date=txn_date,
            posted_date=txn_date,
            payee_id=payee_id,
            memo=memo,
            status=status,
            reference=reference,
            created_at=self._created_at,
        )

    async def update_transaction(
        self,
        *,
        transaction_id: int,
        txn_date: datetime.date,
        payee_id: int | None,
        memo: str | None,
        status: str,
        reference: str | None,
        created_by_user_id: int | None = None,
        created_session_id: int | None = None,
        created_device_id: int | None = None,
        created_request_id: str | None = None,
    ) -> Transaction | None:
        if transaction_id != 1:
            return None
        return Transaction(
            id=1,
            book_id=1,
            occurred_date=txn_date,
            posted_date=txn_date,
            payee_id=payee_id,
            memo=memo,
            status=status,
            reference=reference,
            created_at=self._created_at,
        )

    async def delete_transaction(
        self,
        transaction_id: int,
        *,
        created_by_user_id: int | None = None,
        created_session_id: int | None = None,
        created_device_id: int | None = None,
        created_request_id: str | None = None,
    ) -> bool:
        return transaction_id == 1

    async def list_splits_for_transaction_ids(self, transaction_ids: list[int]) -> list[Split]:
        if 1 not in transaction_ids and 3 not in transaction_ids:
            return []
        tx_id = 3 if 3 in transaction_ids else 1
        return [
            Split(
                id=1,
                tx_id=tx_id,
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
                tx_id=tx_id,
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

    async def replace_transaction_splits(
        self,
        transaction_id: int,
        splits: list[dict[str, int | str | None]],
        created_by_user_id: int | None = None,
        created_session_id: int | None = None,
        created_device_id: int | None = None,
        created_request_id: str | None = None,
    ) -> list[Split]:
        return await self.list_splits_for_transaction_ids([transaction_id])

    async def duplicate_transaction(
        self,
        transaction_id: int,
        today: datetime.date,
        *,
        created_by_user_id: int | None = None,
        created_session_id: int | None = None,
        created_device_id: int | None = None,
        created_request_id: str | None = None,
    ) -> Transaction | None:
        if transaction_id != 1:
            return None
        return Transaction(
            id=4,
            book_id=1,
            occurred_date=today,
            posted_date=today,
            payee_id=None,
            memo="Initial opening balance",
            status="uncleared",
            reference=None,
            created_at=self._created_at,
        )

    async def bulk_void_transactions(
        self,
        transaction_ids: list[int],
        *,
        created_by_user_id: int | None = None,
        created_session_id: int | None = None,
        created_device_id: int | None = None,
        created_request_id: str | None = None,
    ) -> int:
        return len([transaction_id for transaction_id in transaction_ids if transaction_id in {1, 2}])

    async def bulk_delete_transactions(
        self,
        transaction_ids: list[int],
        *,
        created_by_user_id: int | None = None,
        created_session_id: int | None = None,
        created_device_id: int | None = None,
        created_request_id: str | None = None,
    ) -> int:
        return len([transaction_id for transaction_id in transaction_ids if transaction_id in {1, 2}])

    async def list_account_register_splits(
        self,
        account_id: int,
        *,
        limit: int = 100,
        cursor: str | None = None,
    ) -> tuple[list[tuple[Transaction, Split, int]], str | None]:
        if account_id != 2:
            return [], None
        transaction = next(
            transaction
            for transaction in (await self.list_transactions())[0]
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
        return [(transaction, split, 500000)], None

    async def list_accounts_by_ids(self, account_ids: set[int]) -> list[Account]:
        return [
            Account(
                id=account_id,
                book_id=1,
                parent_id=None,
                account_type="asset",
                name=f"Account {account_id}",
                commodity_id=1,
                number_last4=None,
                is_closed=False,
                is_hidden=False,
                is_system=False,
                system_role=None,
                created_at=self._created_at,
                updated_at=self._created_at,
            )
            for account_id in sorted(account_ids)
        ]

    async def get_locked_account_ids(self, account_ids: set[int], occurred_date: datetime.date) -> list[tuple[int, datetime.date]]:
        return []

    async def metadata_refs_belong_to_book(
        self,
        *,
        book_id: int,
        payee_id: int | None,
        category_ids: set[int],
        tag_ids: set[int],
        person_ids: set[int],
        project_ids: set[int],
    ) -> bool:
        return True

    async def get_payee_defaults(self, payee_id: int, account_id: int | None = None) -> tuple[int | None, str | None]:
        if payee_id == 1 and account_id in {None, 2}:
            return 1, "Pending groceries"
        return None, None


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
async def test_account_service_updates_and_deletes_accounts() -> None:
    service = AccountService(StubAccountRepository())

    updated = await service.update_account(
        1,
        AccountUpdateInput(
            book_id=1,
            parent_id=None,
            account_type="asset",
            name="Renamed Assets",
            commodity_id=1,
            institution_id=None,
            country_id=None,
            number_last4=None,
            is_closed=False,
        ),
    )

    assert updated is not None
    assert updated.name == "Renamed Assets"
    assert await service.delete_account(1) is True


@pytest.mark.asyncio
async def test_account_service_validates_closing_and_creates_balancing() -> None:
    service = AccountService(StubAccountRepository())

    validation = await service.validate_account_closing(1)
    balancing = await service.create_account_balancing(
        AccountBalancingCreateInput(
            book_id=1,
            account_id=1,
            as_of_date=datetime(2026, 5, 4, tzinfo=UTC).date(),
            balance_minor=100,
            memo=None,
        )
    )

    assert validation == AccountClosingValidationResult(valid=False, issues=("account has locked balancing history",))
    assert balancing is not None
    assert balancing.account_id == 1


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

    assert isinstance(result, TransactionPage)
    assert len(result.items) == 2
    assert result.items[0].memo == "Pending groceries"
    assert result.items[0].payee_id == 1
    assert result.items[0].reference == "groceries-1"
    assert len(result.items[1].splits) == 2
    assert result.items[1].splits[0].amount_minor == 500000
    assert result.items[1].splits[0].commodity_id == 1


@pytest.mark.asyncio
async def test_transaction_service_passes_filters_to_repository() -> None:
    repository = StubTransactionRepository()
    service = TransactionService(repository)
    filters = TransactionListFilters(status="cleared", occurred_to=datetime(2026, 5, 1, tzinfo=UTC).date())

    result = await service.list_transactions(filters)

    assert repository.last_filters == filters
    assert [transaction.id for transaction in result.items] == [1]


@pytest.mark.asyncio
async def test_transaction_service_returns_none_for_missing_transaction() -> None:
    service = TransactionService(StubTransactionRepository())

    result = await service.get_transaction_by_id(999)

    assert result is None


@pytest.mark.asyncio
async def test_transaction_service_creates_updates_and_deletes_transactions() -> None:
    service = TransactionService(StubTransactionRepository())
    input = TransactionMutationInput(
        book_id=1,
        txn_date=datetime(2026, 5, 3, tzinfo=UTC).date(),
        payee_id=None,
        memo="Created",
        status="uncleared",
        reference=None,
        splits=(
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
        ),
    )

    created = await service.create_transaction(input)
    updated = await service.update_transaction(1, input)
    deleted = await service.delete_transaction(1)

    assert created.id == 3
    assert updated is not None
    assert updated.id == 1
    assert deleted is True


@pytest.mark.asyncio
async def test_transaction_service_builds_account_register_running_balance() -> None:
    service = TransactionService(StubTransactionRepository())

    result = await service.list_account_register(2)

    assert result == RegisterPage(items=(
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
        ),
    ), next_cursor=None)


@pytest.mark.asyncio
async def test_transaction_service_returns_payee_defaults() -> None:
    service = TransactionService(StubTransactionRepository())

    result = await service.get_payee_defaults(1, 2)

    assert result == PayeeDefaults(category_id=1, memo="Pending groceries")


@pytest.mark.asyncio
async def test_transaction_service_duplicates_and_bulk_mutates_transactions() -> None:
    service = TransactionService(StubTransactionRepository())

    duplicated = await service.duplicate_transaction(1, datetime(2026, 5, 4, tzinfo=UTC).date())
    voided = await service.bulk_void_transactions([1, 2, 999])
    deleted = await service.bulk_delete_transactions([1, 2, 999])

    assert duplicated is not None
    assert duplicated.id == 4
    assert voided == 2
    assert deleted == 2
