from __future__ import annotations

from collections.abc import Iterator

import pytest
import pytest_asyncio
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker, create_async_engine

from _postgres import temporary_database


@pytest.fixture()
def repository_database_url() -> Iterator[str]:
    try:
        with temporary_database() as url:
            yield url
    except Exception as exc:  # pragma: no cover
        pytest.skip(f"postgres not available for repository tests: {exc}")


@pytest_asyncio.fixture()
async def repository_session(repository_database_url: str) -> AsyncSession:
    engine = create_async_engine(repository_database_url, future=True)
    session_factory = async_sessionmaker(bind=engine, class_=AsyncSession, expire_on_commit=False)

    async with session_factory() as session:
        yield session

    await engine.dispose()
