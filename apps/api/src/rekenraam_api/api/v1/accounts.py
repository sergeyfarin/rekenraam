from fastapi import APIRouter, Depends, HTTPException, status

from rekenraam_api.api.dependencies import get_account_service
from rekenraam_api.api.dependencies import get_transaction_service
from rekenraam_api.schemas.register import RegisterEntry
from rekenraam_api.schemas.accounts import (
    AccountBalanceSummary,
    AccountBalancingSummary,
    AccountBookingPolicyUpdate,
    AccountDirectiveSummary,
    AccountSummary,
    AccountTreeNode,
)
from rekenraam_api.services.accounts import AccountService
from rekenraam_api.services.transactions import TransactionService


router = APIRouter(prefix="/accounts", tags=["accounts"])


@router.get("", response_model=list[AccountSummary])
async def list_accounts(account_service: AccountService = Depends(get_account_service)) -> list[AccountSummary]:
    return await account_service.list_accounts()


@router.get("/balances", response_model=list[AccountBalanceSummary])
async def list_account_balances(
    account_service: AccountService = Depends(get_account_service),
) -> list[AccountBalanceSummary]:
    return await account_service.list_account_balances()


@router.get("/{account_id}/balancings", response_model=list[AccountBalancingSummary])
async def list_account_balancings(
    account_id: int,
    account_service: AccountService = Depends(get_account_service),
) -> list[AccountBalancingSummary]:
    account = await account_service.get_account_by_id(account_id)
    if account is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="account not found")
    return await account_service.list_account_balancings(account_id)


@router.get("/{account_id}/directives", response_model=list[AccountDirectiveSummary])
async def list_account_directives(
    account_id: int,
    account_service: AccountService = Depends(get_account_service),
) -> list[AccountDirectiveSummary]:
    account = await account_service.get_account_by_id(account_id)
    if account is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="account not found")
    return await account_service.list_account_directives(account_id)


@router.get("/{account_id}/booking-policy", response_model=str)
async def get_account_booking_policy(
    account_id: int,
    account_service: AccountService = Depends(get_account_service),
) -> str:
    try:
        policy = await account_service.get_account_booking_policy(account_id)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error

    if policy is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="account not found")
    return policy


@router.put("/{account_id}/booking-policy", response_model=str)
async def set_account_booking_policy(
    account_id: int,
    input: AccountBookingPolicyUpdate,
    account_service: AccountService = Depends(get_account_service),
) -> str:
    try:
        policy = await account_service.set_account_booking_policy(account_id, input.booking_policy)
    except ValueError as error:
        message = str(error)
        status_code = status.HTTP_400_BAD_REQUEST
        if message == "system accounts cannot be updated":
            status_code = status.HTTP_409_CONFLICT
        raise HTTPException(status_code=status_code, detail=message) from error

    if policy is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="account not found")
    return policy


@router.get("/tree", response_model=list[AccountTreeNode])
async def list_account_tree(
    account_service: AccountService = Depends(get_account_service),
) -> list[AccountTreeNode]:
    return await account_service.list_account_tree()


@router.get("/{account_id}", response_model=AccountSummary)
async def get_account(
    account_id: int,
    account_service: AccountService = Depends(get_account_service),
) -> AccountSummary:
    account = await account_service.get_account_by_id(account_id)
    if account is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="account not found")
    return account


@router.get("/{account_id}/register", response_model=list[RegisterEntry])
async def get_account_register(
    account_id: int,
    account_service: AccountService = Depends(get_account_service),
    transaction_service: TransactionService = Depends(get_transaction_service),
) -> list[RegisterEntry]:
    account = await account_service.get_account_by_id(account_id)
    if account is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="account not found")

    return await transaction_service.list_account_register(account_id)