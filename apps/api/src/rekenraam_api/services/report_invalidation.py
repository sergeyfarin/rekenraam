from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.repositories.reports import ReportRepository


async def bump_report_state(session: AsyncSession | None, book_id: int) -> int | None:
    if session is None:
        return None
    repository = ReportRepository(session)
    return await repository.bump_book_change_seq(book_id)
