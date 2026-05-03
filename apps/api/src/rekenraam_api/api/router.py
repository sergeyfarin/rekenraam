from fastapi import APIRouter

from rekenraam_api.api.v1.accounts import router as accounts_router
from rekenraam_api.api.v1.books import router as books_router
from rekenraam_api.api.v1.health import router as health_router
from rekenraam_api.api.v1.transactions import router as transactions_router


api_router = APIRouter()
api_router.include_router(health_router)
api_router.include_router(accounts_router)
api_router.include_router(books_router)
api_router.include_router(transactions_router)