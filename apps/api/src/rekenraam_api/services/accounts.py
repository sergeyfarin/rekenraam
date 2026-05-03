from dataclasses import dataclass, field

from rekenraam_api.db.models.accounts import Account
from rekenraam_api.repositories.accounts import AccountRepository
from rekenraam_api.schemas.accounts import AccountSummary, AccountTreeNode


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

    async def get_account_by_id(self, account_id: int) -> AccountSummary | None:
        account = await self._repository.get_account_by_id(account_id)
        if account is None:
            return None
        return self._to_summary(account)

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