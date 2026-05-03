from collections.abc import AsyncIterator

from fastapi import Depends
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.session import session_factory
from rekenraam_api.repositories.accounts import AccountRepository
from rekenraam_api.repositories.books import BookRepository
from rekenraam_api.repositories.metadata import MetadataRepository
from rekenraam_api.repositories.transactions import TransactionRepository
from rekenraam_api.services.accounts import AccountService
from rekenraam_api.services.books import BookService
from rekenraam_api.services.metadata import MetadataService
from rekenraam_api.services.transactions import TransactionService


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