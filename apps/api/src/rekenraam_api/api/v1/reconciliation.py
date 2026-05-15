from datetime import date
from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException, status

from rekenraam_api.api.dependencies import get_reconciliation_service
from rekenraam_api.schemas.reconciliation import (
    BalanceConstraintCreateInput,
    BalanceConstraintSummary,
    BalanceConstraintValidation,
    ReconciliationFinishInput,
    ReconciliationFinishResult,
    ReconciliationHistoryResponse,
    ReconciliationStartResponse,
    ReconciliationUnlockInput,
)
from rekenraam_api.services.reconciliation import ReconciliationService

router = APIRouter(prefix="/reconciliation", tags=["reconciliation"])
ReconciliationServiceDep = Annotated[ReconciliationService, Depends(get_reconciliation_service)]


@router.get("/accounts/{account_id}/start", response_model=ReconciliationStartResponse)
async def start_reconciliation(
    account_id: int,
    as_of_date: date,
    service: ReconciliationServiceDep,
    statement_start_date: date | None = None,
) -> ReconciliationStartResponse:
    try:
        result = await service.start(account_id, as_of_date, statement_start_date)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail=str(error)) from error
    if result is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="account not found")
    return result


@router.post("/accounts/{account_id}/finish", response_model=ReconciliationFinishResult)
async def finish_reconciliation(
    account_id: int,
    input: ReconciliationFinishInput,
    service: ReconciliationServiceDep,
) -> ReconciliationFinishResult:
    try:
        result = await service.finish(account_id, input)
    except ValueError as error:
        message = str(error)
        conflict_messages = {
            "balancing date must be after last active balancing",
            "statement_start_date is locked; unlock the affected reconciliation period first",
        }
        status_code = (
            status.HTTP_409_CONFLICT
            if message in conflict_messages
            else status.HTTP_400_BAD_REQUEST
        )
        raise HTTPException(status_code=status_code, detail=message) from error
    if result is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="account not found")
    return result


@router.get("/accounts/{account_id}/history", response_model=ReconciliationHistoryResponse)
async def reconciliation_history(
    account_id: int,
    service: ReconciliationServiceDep,
) -> ReconciliationHistoryResponse:
    result = await service.history(account_id)
    if result is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="account not found")
    return result


@router.post("/accounts/{account_id}/unlock", response_model=int)
async def unlock_reconciliation(
    account_id: int,
    input: ReconciliationUnlockInput,
    service: ReconciliationServiceDep,
) -> int:
    try:
        result = await service.unlock(account_id, input.from_date, input.reason, input.confirm)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail=str(error)) from error
    if result is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="account not found")
    return result


@router.get(
    "/accounts/{account_id}/constraints", response_model=tuple[BalanceConstraintSummary, ...]
)
async def list_balance_constraints(
    account_id: int,
    service: ReconciliationServiceDep,
) -> tuple[BalanceConstraintSummary, ...]:
    result = await service.list_constraints(account_id)
    if result is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="account not found")
    return result


@router.post("/accounts/{account_id}/constraints", response_model=BalanceConstraintSummary)
async def create_balance_constraint(
    account_id: int,
    input: BalanceConstraintCreateInput,
    service: ReconciliationServiceDep,
) -> BalanceConstraintSummary:
    try:
        result = await service.create_constraint(account_id, input)
    except ValueError as error:
        message = str(error)
        # 1.3.2 — "already exists" is a state conflict (409), not a request
        # shape error. Other ValueErrors (sign_rule validation, min > max,
        # cross-book) remain 400.
        status_code = (
            status.HTTP_409_CONFLICT
            if message.startswith("balance constraint already exists")
            else status.HTTP_400_BAD_REQUEST
        )
        raise HTTPException(status_code=status_code, detail=message) from error
    if result is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="account not found")
    return result


@router.delete(
    "/accounts/{account_id}/constraints/{constraint_id}", status_code=status.HTTP_204_NO_CONTENT
)
async def delete_balance_constraint(
    account_id: int,
    constraint_id: int,
    service: ReconciliationServiceDep,
) -> None:
    result = await service.delete_constraint(account_id, constraint_id)
    if result is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="account not found")
    if not result:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="constraint not found")


@router.get(
    "/accounts/{account_id}/constraints/validation", response_model=BalanceConstraintValidation
)
async def validate_balance_constraints(
    account_id: int,
    service: ReconciliationServiceDep,
) -> BalanceConstraintValidation:
    result = await service.validate_constraints(account_id)
    if result is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="account not found")
    return result
