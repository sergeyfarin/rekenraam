from fastapi import APIRouter, Depends, HTTPException, status

from rekenraam_api.api.dependencies import get_account_service
from rekenraam_api.api.dependencies import get_transaction_service
from rekenraam_api.schemas.register import RegisterEntry
from rekenraam_api.schemas.accounts import (
    AccountBalanceSummary,
    AccountBalancingCreateInput,
    AccountBalancingSummary,
    AccountClosingValidationResult,
    AccountBalancingUnlockInput,
    AccountBookingPolicyUpdate,
    AccountCreateInput,
    AccountDirectiveSummary,
    AccountSummary,
    AccountTreeNode,
    AccountUpdateInput,
)
from rekenraam_api.services.accounts import AccountService
from rekenraam_api.services.transactions import TransactionService


router = APIRouter(prefix="/accounts", tags=["accounts"])


@router.get("", response_model=list[AccountSummary])
async def list_accounts(account_service: AccountService = Depends(get_account_service)) -> list[AccountSummary]:
    return await account_service.list_accounts()


@router.post("", response_model=AccountSummary)
async def create_account(
    input: AccountCreateInput,
    account_service: AccountService = Depends(get_account_service),
) -> AccountSummary:
    try:
        return await account_service.create_account(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.put("/{account_id}", response_model=AccountSummary)
async def update_account(
    account_id: int,
    input: AccountUpdateInput,
    account_service: AccountService = Depends(get_account_service),
) -> AccountSummary:
    try:
        account = await account_service.update_account(account_id, input)
    except ValueError as error:
        message = str(error)
        status_code = status.HTTP_409_CONFLICT if message == "system accounts cannot be updated" else status.HTTP_400_BAD_REQUEST
        raise HTTPException(status_code=status_code, detail=message) from error

    if account is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="account not found")
    return account


@router.delete("/{account_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_account(
    account_id: int,
    account_service: AccountService = Depends(get_account_service),
) -> None:
    try:
        deleted = await account_service.delete_account(account_id)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail=str(error)) from error

    if not deleted:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="account not found")


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


@router.post("/{account_id}/balancings", response_model=AccountBalancingSummary)
async def create_account_balancing(
    account_id: int,
    input: AccountBalancingCreateInput,
    account_service: AccountService = Depends(get_account_service),
) -> AccountBalancingSummary:
    if input.account_id != account_id:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="account id mismatch")

    try:
        balancing = await account_service.create_account_balancing(input)
    except ValueError as error:
        message = str(error)
        status_code = status.HTTP_409_CONFLICT if message == "account does not belong to book" else status.HTTP_400_BAD_REQUEST
        raise HTTPException(status_code=status_code, detail=message) from error

    if balancing is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="account not found")
    return balancing


@router.post("/{account_id}/balancings/unlock", response_model=int)
async def unlock_account_balancings(
    account_id: int,
    input: AccountBalancingUnlockInput,
    account_service: AccountService = Depends(get_account_service),
) -> int:
    account = await account_service.get_account_by_id(account_id)
    if account is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="account not found")
    try:
        return await account_service.unlock_account_balancings(account_id, input.from_date, input.reason, input.confirm)
    except ValueError as error:
        message = str(error)
        status_code = status.HTTP_409_CONFLICT if message == "unlock not confirmed" else status.HTTP_400_BAD_REQUEST
        raise HTTPException(status_code=status_code, detail=message) from error


@router.get("/{account_id}/directives", response_model=list[AccountDirectiveSummary])
async def list_account_directives(
    account_id: int,
    account_service: AccountService = Depends(get_account_service),
) -> list[AccountDirectiveSummary]:
    account = await account_service.get_account_by_id(account_id)
    if account is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="account not found")
    return await account_service.list_account_directives(account_id)


@router.get("/{account_id}/closing-validation", response_model=AccountClosingValidationResult)
async def validate_account_closing(
    account_id: int,
    account_service: AccountService = Depends(get_account_service),
) -> AccountClosingValidationResult:
    result = await account_service.validate_account_closing(account_id)
    if result is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="account not found")
    return result


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