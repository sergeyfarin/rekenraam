from sqlalchemy import Select, select
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.books import AppSchemaState, Book


class BookRepository:
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def list_books(self) -> list[Book]:
        statement: Select[tuple[Book]] = select(Book).order_by(Book.id)
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def get_book_by_slug(self, slug: str) -> Book | None:
        statement: Select[tuple[Book]] = select(Book).where(Book.slug == slug)
        result = await self._session.execute(statement)
        return result.scalar_one_or_none()

    async def get_schema_version(self) -> str:
        statement: Select[tuple[AppSchemaState]] = select(AppSchemaState).where(AppSchemaState.id.is_(True))
        result = await self._session.execute(statement)
        schema_state = result.scalar_one()
        return schema_state.schema_version