from rekenraam_api.db.models.accounts import Account
from rekenraam_api.repositories.accounts import AccountRepository
from rekenraam_api.schemas.accounts import AccountSummary


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

    @staticmethod
    def _to_summary(account: Account) -> AccountSummary:
        return AccountSummary(
            id=account.id,
            book_id=account.book_id,
            parent_id=account.parent_id,
            account_type=account.account_type,
            name=account.name,
            is_closed=account.is_closed,
            is_hidden=account.is_hidden,
            is_system=account.is_system,
            system_role=account.system_role,
            created_at=account.created_at,
        )