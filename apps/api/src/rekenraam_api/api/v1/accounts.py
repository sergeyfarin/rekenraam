from fastapi import APIRouter, Depends, HTTPException, status

from rekenraam_api.api.dependencies import get_account_service
from rekenraam_api.schemas.accounts import AccountSummary
from rekenraam_api.services.accounts import AccountService


router = APIRouter(prefix="/accounts", tags=["accounts"])


@router.get("", response_model=list[AccountSummary])
async def list_accounts(account_service: AccountService = Depends(get_account_service)) -> list[AccountSummary]:
    return await account_service.list_accounts()


@router.get("/{account_id}", response_model=AccountSummary)
async def get_account(
    account_id: int,
    account_service: AccountService = Depends(get_account_service),
) -> AccountSummary:
    account = await account_service.get_account_by_id(account_id)
    if account is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="account not found")
    return account