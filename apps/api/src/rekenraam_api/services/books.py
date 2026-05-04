from rekenraam_api.repositories.books import BookRepository
from rekenraam_api.schemas.books import BookSummary
from rekenraam_api.services.access import AccessPolicy


class BookService:
    def __init__(self, repository: BookRepository, access_policy: AccessPolicy | None = None) -> None:
        self._repository = repository
        self._access_policy = access_policy

    async def list_books(self) -> list[BookSummary]:
        book_ids = await self._access_policy.list_readable_book_ids() if self._access_policy is not None else None
        books = await self._repository.list_books(book_ids)
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
        if self._access_policy is not None:
            await self._access_policy.require_book_read(book.id)

        return BookSummary(
            id=book.id,
            slug=book.slug,
            name=book.name,
            base_currency_code=book.base_currency_code,
        )
