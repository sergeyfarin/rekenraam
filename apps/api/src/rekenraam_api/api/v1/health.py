from fastapi import APIRouter, Depends

from rekenraam_api.api.dependencies import get_book_service
from rekenraam_api.schemas.health import HealthResponse
from rekenraam_api.services.books import BookService


router = APIRouter(tags=["health"])


@router.get("/health", response_model=HealthResponse)
async def health(book_service: BookService = Depends(get_book_service)) -> HealthResponse:
    schema_version = await book_service.get_schema_version()
    return HealthResponse(
        status="ok",
        service="rekenraam-api",
        database="ok",
        schema_version=schema_version,
    )