from fastapi import APIRouter, Depends, HTTPException, status

from rekenraam_api.api.dependencies import get_book_service
from rekenraam_api.schemas.books import BookSummary
from rekenraam_api.services.books import BookService

router = APIRouter(prefix="/books", tags=["books"])


@router.get("", response_model=list[BookSummary])
async def list_books(book_service: BookService = Depends(get_book_service)) -> list[BookSummary]:
    return await book_service.list_books()


@router.get("/{slug}", response_model=BookSummary)
async def get_book(slug: str, book_service: BookService = Depends(get_book_service)) -> BookSummary:
    book = await book_service.get_book_by_slug(slug)
    if book is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="book not found")
    return book