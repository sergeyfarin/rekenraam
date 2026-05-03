from fastapi import APIRouter, Depends, HTTPException, status

from rekenraam_api.api.dependencies import get_transaction_service
from rekenraam_api.schemas.transactions import TransactionSummary
from rekenraam_api.services.transactions import TransactionService


router = APIRouter(prefix="/transactions", tags=["transactions"])


@router.get("", response_model=list[TransactionSummary])
async def list_transactions(
    transaction_service: TransactionService = Depends(get_transaction_service),
) -> list[TransactionSummary]:
    return await transaction_service.list_transactions()


@router.get("/{transaction_id}", response_model=TransactionSummary)
async def get_transaction(
    transaction_id: int,
    transaction_service: TransactionService = Depends(get_transaction_service),
) -> TransactionSummary:
    transaction = await transaction_service.get_transaction_by_id(transaction_id)
    if transaction is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="transaction not found")
    return transaction