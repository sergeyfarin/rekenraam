from __future__ import annotations

import os
from collections.abc import Iterator

os.environ.setdefault("SQLITE_PATH", "/tmp/rekenraam-api-test.sqlite3")

import pytest
import pytest_asyncio
from _postgres import temporary_database as temporary_postgres_database
from _sqlite import temporary_database as temporary_sqlite_database
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from rekenraam_api.db.dialect import create_database_engine


def _repository_backends() -> tuple[str, ...]:
    configured = os.environ.get("REPOSITORY_DB_BACKENDS", "sqlite")
    backends = tuple(backend.strip() for backend in configured.split(",") if backend.strip())
    return backends or ("sqlite",)


@pytest.fixture(params=_repository_backends())
def repository_database_url(request: pytest.FixtureRequest) -> Iterator[str]:
    if request.param == "sqlite":
        with temporary_sqlite_database() as url:
            yield url
        return

    try:
        with temporary_postgres_database() as url:
            yield url
    except Exception as exc:  # pragma: no cover
        pytest.skip(f"postgres not available for repository tests: {exc}")


@pytest_asyncio.fixture()
async def repository_session(repository_database_url: str) -> AsyncSession:
    engine = create_database_engine(repository_database_url)
    session_factory = async_sessionmaker(bind=engine, class_=AsyncSession, expire_on_commit=False)

    async with session_factory() as session:
        yield session

    await engine.dispose()
