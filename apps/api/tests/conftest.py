from __future__ import annotations

import os
from collections.abc import Iterator

os.environ.setdefault("SQLITE_PATH", "/tmp/rekenraam-api-test.sqlite3")

import pytest
import pytest_asyncio
from _sqlite import temporary_database as temporary_sqlite_database
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from rekenraam_api.db.dialect import create_database_engine


@pytest.fixture()
def repository_database_url() -> Iterator[str]:
    with temporary_sqlite_database() as url:
        yield url


@pytest_asyncio.fixture()
async def repository_session(repository_database_url: str) -> AsyncSession:
    engine = create_database_engine(repository_database_url)
    session_factory = async_sessionmaker(bind=engine, class_=AsyncSession, expire_on_commit=False)

    async with session_factory() as session:
        yield session

    await engine.dispose()
