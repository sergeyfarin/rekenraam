from collections.abc import AsyncIterator

from fastapi import Depends, Request
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.session import session_factory
from rekenraam_api.repositories.accounts import AccountRepository
from rekenraam_api.repositories.books import BookRepository
from rekenraam_api.repositories.investments import InvestmentRepository
from rekenraam_api.repositories.metadata import MetadataRepository
from rekenraam_api.repositories.pricing import PricingRepository
from rekenraam_api.repositories.reports import ReportRepository
from rekenraam_api.repositories.transactions import TransactionRepository
from rekenraam_api.services.accounts import AccountService
from rekenraam_api.services.books import BookService
from rekenraam_api.services.investments import InvestmentService
from rekenraam_api.services.metadata import MetadataService
from rekenraam_api.services.pricing import PricingService
from rekenraam_api.services.pricing_execution import PricingExecutionService
from rekenraam_api.services.reports import ReportService
from rekenraam_api.services.transactions import TransactionService
from rekenraam_api.workers.pricing import PricingRefreshWorker, pricing_refresh_worker


async def get_db_session() -> AsyncIterator[AsyncSession]:
    async with session_factory() as session:
        yield session


def get_book_service(session: AsyncSession = Depends(get_db_session)) -> BookService:
    repository = BookRepository(session)
    return BookService(repository)


def get_account_service(session: AsyncSession = Depends(get_db_session)) -> AccountService:
    repository = AccountRepository(session)
    return AccountService(repository)


def get_transaction_service(session: AsyncSession = Depends(get_db_session)) -> TransactionService:
    repository = TransactionRepository(session)
    return TransactionService(repository)


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


def get_pricing_worker(request: Request) -> PricingRefreshWorker:
    return getattr(request.app.state, "pricing_worker", pricing_refresh_worker)


def get_report_service(session: AsyncSession = Depends(get_db_session)) -> ReportService:
    repository = ReportRepository(session)
    return ReportService(repository)