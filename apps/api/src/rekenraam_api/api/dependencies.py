from collections.abc import AsyncIterator
from datetime import UTC, datetime
from uuid import uuid4

from fastapi import Cookie, Depends, HTTPException, Request, status
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.config.settings import get_settings
from rekenraam_api.db.session import session_factory
from rekenraam_api.repositories.access import AccessRepository
from rekenraam_api.repositories.accounts import AccountRepository
from rekenraam_api.repositories.books import BookRepository
from rekenraam_api.repositories.investments import InvestmentRepository
from rekenraam_api.repositories.imports import ImportRepository
from rekenraam_api.repositories.metadata import MetadataRepository
from rekenraam_api.repositories.pricing import PricingRepository
from rekenraam_api.repositories.reconciliation import ReconciliationRepository
from rekenraam_api.repositories.reports import ReportRepository
from rekenraam_api.repositories.transactions import TransactionRepository
from rekenraam_api.services.access import AccessPolicy
from rekenraam_api.services.accounts import AccountService
from rekenraam_api.services.admin import AdminService
from rekenraam_api.services.auth import SESSION_COOKIE_NAME, AuthService
from rekenraam_api.services.books import BookService
from rekenraam_api.services.exports import ExportService
from rekenraam_api.services.investments import InvestmentService
from rekenraam_api.services.imports import ImportService
from rekenraam_api.services.metadata import MetadataService
from rekenraam_api.services.pricing import PricingService
from rekenraam_api.services.pricing_execution import PricingExecutionService
from rekenraam_api.services.reconciliation import ReconciliationService
from rekenraam_api.services.reports import ReportService
from rekenraam_api.services.request_context import RequestContext, set_request_context
from rekenraam_api.services.transactions import TransactionService
from rekenraam_api.workers.pricing import PricingRefreshWorker, pricing_refresh_worker


async def get_db_session() -> AsyncIterator[AsyncSession]:
    async with session_factory() as session:
        yield session


def get_access_repository(session: AsyncSession = Depends(get_db_session)) -> AccessRepository:
    return AccessRepository(session)


def get_auth_service(repository: AccessRepository = Depends(get_access_repository)) -> AuthService:
    return AuthService(repository)


async def require_request_context(
    request: Request,
    auth_service: AuthService = Depends(get_auth_service),
    session_token: str | None = Cookie(default=None, alias=SESSION_COOKIE_NAME),
) -> RequestContext:
    if session_token is None:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="authentication required")
    authenticated = await auth_service.authenticate_token(session_token)
    if authenticated is None:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="authentication required")
    _user, auth_session = authenticated
    context = RequestContext(
        user_id=auth_session.user_id,
        session_id=auth_session.id,
        device_id=auth_session.device_id,
        request_id=request.headers.get("x-request-id") or uuid4().hex,
        request_timestamp=datetime.now(UTC),
    )
    request.state.request_context = context
    set_request_context(context)
    return context


def get_access_policy(
    context: RequestContext = Depends(require_request_context),
    repository: AccessRepository = Depends(get_access_repository),
) -> AccessPolicy:
    return AccessPolicy(repository, context)


def get_book_service(
    session: AsyncSession = Depends(get_db_session),
    access_policy: AccessPolicy = Depends(get_access_policy),
) -> BookService:
    repository = BookRepository(session)
    return BookService(repository, access_policy)


def get_account_service(
    session: AsyncSession = Depends(get_db_session),
    access_policy: AccessPolicy = Depends(get_access_policy),
) -> AccountService:
    repository = AccountRepository(session)
    return AccountService(repository, access_policy)


def get_admin_service(session: AsyncSession = Depends(get_db_session)) -> AdminService:
    return AdminService(session, get_settings())


def get_transaction_service(
    session: AsyncSession = Depends(get_db_session),
    access_policy: AccessPolicy = Depends(get_access_policy),
) -> TransactionService:
    repository = TransactionRepository(session)
    return TransactionService(repository, access_policy)


def get_metadata_service(session: AsyncSession = Depends(get_db_session)) -> MetadataService:
    repository = MetadataRepository(session)
    return MetadataService(repository)


def get_investment_service(session: AsyncSession = Depends(get_db_session)) -> InvestmentService:
    repository = InvestmentRepository(session)
    return InvestmentService(repository)


def get_pricing_service(session: AsyncSession = Depends(get_db_session)) -> PricingService:
    repository = PricingRepository(session)
    return PricingService(repository)


def get_pricing_execution_service(session: AsyncSession = Depends(get_db_session)) -> PricingExecutionService:
    repository = PricingRepository(session)
    return PricingExecutionService(repository)


def get_reconciliation_service(
    session: AsyncSession = Depends(get_db_session),
    access_policy: AccessPolicy = Depends(get_access_policy),
) -> ReconciliationService:
    repository = ReconciliationRepository(session)
    return ReconciliationService(repository, access_policy)


def get_pricing_worker(request: Request) -> PricingRefreshWorker:
    return getattr(request.app.state, "pricing_worker", pricing_refresh_worker)


def get_report_service(
    session: AsyncSession = Depends(get_db_session),
    access_policy: AccessPolicy = Depends(get_access_policy),
) -> ReportService:
    repository = ReportRepository(session)
    return ReportService(repository, access_policy)


def get_import_service(
    session: AsyncSession = Depends(get_db_session),
    access_policy: AccessPolicy = Depends(get_access_policy),
) -> ImportService:
    repository = ImportRepository(session)
    return ImportService(repository, access_policy)


def get_export_service(
    session: AsyncSession = Depends(get_db_session),
    access_policy: AccessPolicy = Depends(get_access_policy),
) -> ExportService:
    report_repository = ReportRepository(session)
    investment_repository = InvestmentRepository(session)
    return ExportService(
        session,
        ReportService(report_repository, access_policy),
        InvestmentService(investment_repository),
        access_policy,
    )
