from rekenraam_api.repositories.books import BookRepository
from rekenraam_api.schemas.books import BookSummary


class BookService:
    def __init__(self, repository: BookRepository) -> None:
        self._repository = repository

    async def list_books(self) -> list[BookSummary]:
        books = await self._repository.list_books()
        return [
            BookSummary(
                id=book.id,
                slug=book.slug,
                name=book.name,
                base_currency_code=book.base_currency_code,
            )
            for book in books
        ]

    async def get_book_by_slug(self, slug: str) -> BookSummary | None:
        book = await self._repository.get_book_by_slug(slug)
        if book is None:
            return None

        return BookSummary(
            id=book.id,
            slug=book.slug,
            name=book.name,
            base_currency_code=book.base_currency_code,
        )