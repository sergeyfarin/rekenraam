from sqlalchemy import Select, select
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.db.models.books import Book


class BookRepository:
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def list_books(self, book_ids: list[int] | None = None) -> list[Book]:
        statement: Select[tuple[Book]] = select(Book).order_by(Book.id)
        if book_ids is not None:
            if not book_ids:
                return []
            statement = statement.where(Book.id.in_(book_ids))
        result = await self._session.execute(statement)
        return list(result.scalars().all())

    async def get_book_by_slug(self, slug: str) -> Book | None:
        statement: Select[tuple[Book]] = select(Book).where(Book.slug == slug)
        result = await self._session.execute(statement)
        return result.scalar_one_or_none()
