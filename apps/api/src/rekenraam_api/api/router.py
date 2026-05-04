from fastapi import APIRouter, Depends

from rekenraam_api.api.v1.accounts import router as accounts_router
from rekenraam_api.api.v1.admin import router as admin_router
from rekenraam_api.api.v1.auth import router as auth_router
from rekenraam_api.api.v1.books import router as books_router
from rekenraam_api.api.v1.health import router as health_router
from rekenraam_api.api.v1.investments import router as investments_router
from rekenraam_api.api.v1.metadata import router as metadata_router
from rekenraam_api.api.v1.pricing import router as pricing_router
from rekenraam_api.api.v1.reports import router as reports_router
from rekenraam_api.api.v1.transactions import router as transactions_router
from rekenraam_api.api.dependencies import require_request_context


api_router = APIRouter()
api_router.include_router(health_router)
api_router.include_router(auth_router)
protected_dependencies = [Depends(require_request_context)]
api_router.include_router(admin_router, dependencies=protected_dependencies)
api_router.include_router(accounts_router, dependencies=protected_dependencies)
api_router.include_router(books_router, dependencies=protected_dependencies)
api_router.include_router(investments_router, dependencies=protected_dependencies)
api_router.include_router(metadata_router, dependencies=protected_dependencies)
api_router.include_router(pricing_router, dependencies=protected_dependencies)
api_router.include_router(reports_router, dependencies=protected_dependencies)
api_router.include_router(transactions_router, dependencies=protected_dependencies)
