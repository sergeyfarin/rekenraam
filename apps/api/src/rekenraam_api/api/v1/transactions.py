from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException, Query, status
from pydantic import ValidationError

from rekenraam_api.api.dependencies import get_transaction_service
from rekenraam_api.schemas.transactions import TransactionListFilters, TransactionSummary
from rekenraam_api.services.transactions import TransactionService


router = APIRouter(prefix="/transactions", tags=["transactions"])


def get_transaction_list_filters(
    book_id: int | None = Query(default=None),
    account_id: int | None = Query(default=None),
    payee_id: int | None = Query(default=None),
    status_filter: str | None = Query(default=None, alias="status"),
    occurred_from: str | None = Query(default=None),
    occurred_to: str | None = Query(default=None),
    search: str | None = Query(default=None),
    amount_min: int | None = Query(default=None),
    amount_max: int | None = Query(default=None),
    sort_by: str | None = Query(default=None),
    sort_dir: str | None = Query(default=None),
    limit: int | None = Query(default=None),
    offset: int | None = Query(default=None),
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
        )
    except ValidationError as exc:
        raise HTTPException(
            status_code=status.HTTP_422_UNPROCESSABLE_CONTENT,
            detail=exc.errors(include_context=False, include_input=False),
        ) from exc


@router.get("", response_model=list[TransactionSummary])
async def list_transactions(
    filters: Annotated[TransactionListFilters, Depends(get_transaction_list_filters)],
    transaction_service: TransactionService = Depends(get_transaction_service),
) -> list[TransactionSummary]:
    return await transaction_service.list_transactions(filters)


@router.get("/{transaction_id}", response_model=TransactionSummary)
async def get_transaction(
    transaction_id: int,
    transaction_service: TransactionService = Depends(get_transaction_service),
) -> TransactionSummary:
    transaction = await transaction_service.get_transaction_by_id(transaction_id)
    if transaction is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="transaction not found")
    return transaction