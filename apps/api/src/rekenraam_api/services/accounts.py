from dataclasses import dataclass, field
from datetime import date

from sqlalchemy.exc import IntegrityError

from rekenraam_api.db.models.accounts import Account
from rekenraam_api.repositories.accounts import AccountRepository
from rekenraam_api.schemas.accounts import (
    AccountBalanceSummary,
    AccountBalancingCreateInput,
    AccountBalancingSummary,
    AccountClosingValidationResult,
    AccountCreateInput,
    AccountDirectiveSummary,
    AccountSummary,
    AccountTreeNode,
    AccountUpdateInput,
)


@dataclass
class _TreeState:
    account: Account
    children: list[int] = field(default_factory=list)


class AccountService:
    def __init__(self, repository: AccountRepository) -> None:
        self._repository = repository

    async def list_accounts(self) -> list[AccountSummary]:
        accounts = await self._repository.list_accounts()
        return [self._to_summary(account) for account in accounts]

    async def create_account(self, input: AccountCreateInput) -> AccountSummary:
        name = input.name.strip()
        account_type = input.account_type.strip().lower()
        if not name:
            raise ValueError("name is required")
        if account_type not in {"cash", "checking", "savings", "credit", "loan", "investment", "asset", "liability", "income", "expense", "equity"}:
            raise ValueError("account type is invalid")
        account = await self._repository.create_account(
            book_id=input.book_id,
            parent_id=input.parent_id,
            account_type=account_type,
            name=name,
            commodity_id=input.commodity_id,
            institution_id=input.institution_id,
            country_id=input.country_id,
            number_last4=input.number_last4,
            is_closed=input.is_closed,
        )
        return self._to_summary(account)

    async def update_account(self, account_id: int, input: AccountUpdateInput) -> AccountSummary | None:
        name = input.name.strip()
        account_type = input.account_type.strip().lower()
        if not name:
            raise ValueError("name is required")
        if account_type not in {"cash", "checking", "savings", "credit", "loan", "investment", "asset", "liability", "income", "expense", "equity"}:
            raise ValueError("account type is invalid")

        current = await self._repository.get_account_by_id(account_id)
        if current is None:
            return None
        if current.is_system:
            raise ValueError("system accounts cannot be updated")

        account = await self._repository.update_account(
            account_id=account_id,
            parent_id=input.parent_id,
            account_type=account_type,
            name=name,
            commodity_id=input.commodity_id,
            institution_id=input.institution_id,
            country_id=input.country_id,
            number_last4=input.number_last4,
            is_closed=input.is_closed,
        )
        if account is None:
            return None
        return self._to_summary(account)

    async def delete_account(self, account_id: int) -> bool:
        current = await self._repository.get_account_by_id(account_id)
        if current is None:
            return False
        if current.is_system:
            raise ValueError("system accounts cannot be deleted")
        try:
            return await self._repository.delete_account(account_id)
        except IntegrityError as error:
            raise ValueError("account is still in use") from error

    async def get_account_by_id(self, account_id: int) -> AccountSummary | None:
        account = await self._repository.get_account_by_id(account_id)
        if account is None:
            return None
        return self._to_summary(account)

    async def list_account_balances(self) -> list[AccountBalanceSummary]:
        balances = await self._repository.get_account_balances()
        return [
            AccountBalanceSummary(account_id=account_id, balance_minor=balance_minor)
            for account_id, balance_minor in sorted(balances.items())
        ]

    async def list_account_balancings(self, account_id: int) -> list[AccountBalancingSummary]:
        balancings = await self._repository.list_account_balancings(account_id)
        return [
            AccountBalancingSummary(
                id=balancing.id,
                account_id=balancing.account_id,
                as_of_date=balancing.as_of_date,
                balance_minor=balancing.balance_minor,
                memo=balancing.memo,
            )
            for balancing in balancings
        ]

    async def list_account_directives(self, account_id: int) -> list[AccountDirectiveSummary]:
        directives = await self._repository.list_account_directives(account_id)
        return [
            AccountDirectiveSummary(
                id=directive.id,
                book_id=directive.book_id,
                account_id=account_id,
                directive_type=directive.lifecycle_event,
                directive_date=directive.effective_at,
                note=directive.lifecycle_note,
                metadata=directive.lifecycle_metadata,
                created_at=directive.created_at,
            )
            for directive in directives
        ]

    async def get_account_booking_policy(self, account_id: int) -> str | None:
        account, policy = await self._repository.get_account_booking_policy(account_id)
        if account is None:
            return None
        if account.account_type != "investment":
            raise ValueError("booking policy only applies to investment accounts")
        return policy or "fifo"

    async def set_account_booking_policy(self, account_id: int, booking_policy: str) -> str | None:
        normalized_policy = booking_policy.strip().lower()
        if normalized_policy not in {"fifo", "lifo", "strict", "average"}:
            raise ValueError("booking policy must be fifo, lifo, strict, or average")

        account = await self._repository.set_account_booking_policy(account_id, normalized_policy)
        if account is None:
            return None
        if account.is_system:
            raise ValueError("system accounts cannot be updated")
        if account.account_type != "investment":
            raise ValueError("booking policy only applies to investment accounts")
        return account.booking_policy or "fifo"

    async def unlock_account_balancings(self, account_id: int, from_date: date, reason: str | None, confirm: bool) -> int:
        if not confirm:
            raise ValueError("unlock not confirmed")
        return await self._repository.unlock_account_balancings(account_id, from_date, reason)

    async def validate_account_closing(self, account_id: int) -> AccountClosingValidationResult | None:
        account = await self._repository.get_account_by_id(account_id)
        if account is None:
            return None

        issues: list[str] = []
        if account.is_system:
            issues.append("system accounts cannot be closed")

        balances = await self._repository.get_account_balances()
        if balances.get(account_id, 0) != 0:
            issues.append("account balance is not zero")

        return AccountClosingValidationResult(valid=len(issues) == 0, issues=tuple(issues))

    async def create_account_balancing(self, input: AccountBalancingCreateInput) -> AccountBalancingSummary | None:
        try:
            balancing = await self._repository.create_account_balancing(
                book_id=input.book_id,
                account_id=input.account_id,
                as_of_date=input.as_of_date,
                balance_minor=input.balance_minor,
                memo=input.memo,
            )
        except ValueError:
            raise

        if balancing is None:
            return None

        return AccountBalancingSummary(
            id=balancing.id,
            account_id=balancing.account_id,
            as_of_date=balancing.as_of_date,
            balance_minor=balancing.balance_minor,
            memo=balancing.memo,
        )

    async def list_account_tree(self) -> list[AccountTreeNode]:
        accounts = await self._repository.list_accounts()
        if not accounts:
            return []

        base_currency_code = await self._repository.get_book_base_currency_code(accounts[0].book_id)
        commodity_name = base_currency_code or "USD"
        balances = await self._repository.get_account_balances()

        state_by_id = {account.id: _TreeState(account=account) for account in accounts}
        root_ids: list[int] = []
        for account in accounts:
            if account.parent_id is None or account.parent_id not in state_by_id:
                root_ids.append(account.id)
            else:
                state_by_id[account.parent_id].children.append(account.id)

        return [self._build_tree_node(state_by_id, root_id, commodity_name, balances) for root_id in root_ids]

    @staticmethod
    def _to_summary(account: Account) -> AccountSummary:
        return AccountSummary(
            id=account.id,
            book_id=account.book_id,
            parent_id=account.parent_id,
            account_type=account.account_type,
            name=account.name,
            commodity_id=account.commodity_id,
            institution_id=None,
            institution_name=None,
            country_id=None,
            country_name=None,
            number_last4=account.number_last4,
            is_closed=account.is_closed,
            is_hidden=account.is_hidden,
            is_system=account.is_system,
            system_role=account.system_role,
            created_at=account.created_at,
            updated_at=account.updated_at,
        )

    def _build_tree_node(
        self,
        state_by_id: dict[int, _TreeState],
        account_id: int,
        commodity_name: str,
        balances: dict[int, int],
    ) -> AccountTreeNode:
        state = state_by_id[account_id]
        child_nodes = tuple(
            self._build_tree_node(state_by_id, child_id, commodity_name, balances) for child_id in state.children
        )
        balance_minor = balances.get(account_id, 0)
        rollup_balance_minor = balance_minor + sum(child.rollup_balance_minor for child in child_nodes)

        return AccountTreeNode(
            id=state.account.id,
            parent_id=state.account.parent_id,
            name=state.account.name,
            account_type=state.account.account_type,
            commodity_id=state.account.commodity_id,
            commodity_name=commodity_name,
            commodity_scale=2,
            institution_name=None,
            country_name=None,
            balance_minor=balance_minor,
            rollup_balance_minor=rollup_balance_minor,
            children=child_nodes,
        )