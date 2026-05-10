from datetime import date
from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException, Query, status
from pydantic import ValidationError

from rekenraam_api.api.dependencies import get_ergonomics_service, get_transaction_service
from rekenraam_api.schemas.ergonomics import PayeeDefaultInput, PayeeDefaultSummary
from rekenraam_api.schemas.transactions import (
    CrossCurrencyTransferInput,
    PayeeDefaults,
    TransactionListFilters,
    TransactionMutationInput,
    TransactionPage,
    TransactionSummary,
)
from rekenraam_api.services.access import AuthorizationError
from rekenraam_api.services.ergonomics import ErgonomicsService
from rekenraam_api.services.transactions import TransactionService

router = APIRouter(prefix="/transactions", tags=["transactions"])


def _mutation_error(error: ValueError) -> HTTPException:
    message = str(error)
    conflict_prefixes = (
        "cannot post transaction dated",
        "transaction cannot use closed accounts",
        "transaction cannot use protected system accounts",
    )
    status_code = (
        status.HTTP_409_CONFLICT
        if message.startswith(conflict_prefixes)
        else status.HTTP_400_BAD_REQUEST
    )
    return HTTPException(status_code=status_code, detail=message)


def get_transaction_list_filters(
    book_id: int | None = Query(default=None),
    account_id: int | None = Query(default=None),
    payee_id: int | None = Query(default=None),
    status_filter: str | None = Query(default=None, alias="status"),
    occurred_from: date | None = Query(default=None),
    occurred_to: date | None = Query(default=None),
    search: str | None = Query(default=None),
    amount_min: int | None = Query(default=None),
    amount_max: int | None = Query(default=None),
    sort_by: str | None = Query(default=None),
    sort_dir: str | None = Query(default=None),
    limit: int | None = Query(default=None),
    offset: int | None = Query(default=None),
    cursor: str | None = Query(default=None),
) -> TransactionListFilters:
    try:
        return TransactionListFilters(
            book_id=book_id,
            account_id=account_id,
            payee_id=payee_id,
            status=status_filter,
            occurred_from=occurred_from,
            occurred_to=occurred_to,
            search=search,
            amount_min=amount_min,
            amount_max=amount_max,
            sort_by=sort_by,
            sort_dir=sort_dir,
            limit=limit,
            offset=offset,
            cursor=cursor,
        )
    except ValidationError as exc:
        raise HTTPException(
            status_code=status.HTTP_422_UNPROCESSABLE_CONTENT,
            detail=exc.errors(include_context=False, include_input=False),
        ) from exc


@router.get("", response_model=TransactionPage)
async def list_transactions(
    filters: Annotated[TransactionListFilters, Depends(get_transaction_list_filters)],
    transaction_service: TransactionService = Depends(get_transaction_service),
) -> TransactionPage:
    return await transaction_service.list_transactions(filters)


@router.get("/payee-defaults", response_model=PayeeDefaults)
async def get_payee_defaults(
    payee_id: int = Query(...),
    account_id: int | None = Query(default=None),
    book_id: int | None = Query(default=None),
    transaction_service: TransactionService = Depends(get_transaction_service),
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> PayeeDefaults:
    if book_id is not None:
        try:
            explicit = await ergonomics_service.get_explicit_payee_default(
                book_id=book_id, payee_id=payee_id, account_id=account_id
            )
        except AuthorizationError as error:
            raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail=str(error)) from error
        if explicit is not None:
            return PayeeDefaults(category_id=explicit.category_id, memo=explicit.memo)
    return await transaction_service.get_payee_defaults(payee_id, account_id)


@router.put("/payee-defaults", response_model=PayeeDefaultSummary)
async def set_payee_default(
    input: PayeeDefaultInput,
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> PayeeDefaultSummary:
    try:
        return await ergonomics_service.upsert_payee_default(input)
    except AuthorizationError as error:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail=str(error)) from error
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.get("/{transaction_id}", response_model=TransactionSummary)
async def get_transaction(
    transaction_id: int,
    transaction_service: TransactionService = Depends(get_transaction_service),
) -> TransactionSummary:
    transaction = await transaction_service.get_transaction_by_id(transaction_id)
    if transaction is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="transaction not found")
    return transaction


@router.post("", response_model=TransactionSummary)
async def create_transaction(
    input: TransactionMutationInput,
    transaction_service: TransactionService = Depends(get_transaction_service),
) -> TransactionSummary:
    try:
        return await transaction_service.create_transaction(input)
    except ValueError as error:
        raise _mutation_error(error) from error


@router.post("/transfer", response_model=TransactionSummary)
async def create_cross_currency_transfer(
    input: CrossCurrencyTransferInput,
    transaction_service: TransactionService = Depends(get_transaction_service),
) -> TransactionSummary:
    try:
        return await transaction_service.create_cross_currency_transfer(input)
    except ValueError as error:
        raise _mutation_error(error) from error


@router.put("/{transaction_id}", response_model=TransactionSummary)
async def update_transaction(
    transaction_id: int,
    input: TransactionMutationInput,
    transaction_service: TransactionService = Depends(get_transaction_service),
) -> TransactionSummary:
    try:
        transaction = await transaction_service.update_transaction(transaction_id, input)
    except ValueError as error:
        raise _mutation_error(error) from error

    if transaction is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="transaction not found")
    return transaction


@router.delete("/{transaction_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_transaction(
    transaction_id: int,
    transaction_service: TransactionService = Depends(get_transaction_service),
) -> None:
    try:
        deleted = await transaction_service.delete_transaction(transaction_id)
    except ValueError as error:
        raise _mutation_error(error) from error
    if not deleted:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="transaction not found")


@router.post("/{transaction_id}/duplicate", response_model=TransactionSummary)
async def duplicate_transaction(
    transaction_id: int,
    today: date,
    transaction_service: TransactionService = Depends(get_transaction_service),
) -> TransactionSummary:
    transaction = await transaction_service.duplicate_transaction(transaction_id, today)
    if transaction is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="transaction not found")
    return transaction


@router.post("/bulk-void", response_model=int)
async def bulk_void_transactions(
    ids: list[int],
    transaction_service: TransactionService = Depends(get_transaction_service),
) -> int:
    return await transaction_service.bulk_void_transactions(ids)


@router.post("/bulk-delete", response_model=int)
async def bulk_delete_transactions(
    ids: list[int],
    transaction_service: TransactionService = Depends(get_transaction_service),
) -> int:
    return await transaction_service.bulk_delete_transactions(ids)
