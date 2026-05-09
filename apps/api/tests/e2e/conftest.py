"""End-to-end fixtures: real FastAPI app, real services, real Postgres.

Unlike `test_api.py` (stub services) and `test_services.py` (fake repos),
these fixtures wire the full router -> service -> repository -> Postgres
stack and expose it through an `httpx.AsyncClient` so tests exercise the
same code path that production hits.

Lifespan is intentionally NOT executed:
- the bootstrap-admin seed in lifespan reads `FIRST_ADMIN_EMAIL`/`FIRST_ADMIN_PASSWORD`
  from settings; tests bootstrap explicitly via the API instead
- the pricing refresh worker would issue real outbound HTTP requests
"""

from __future__ import annotations

from collections.abc import AsyncIterator, Iterator
from dataclasses import dataclass

import pytest
import pytest_asyncio
from _postgres import temporary_database
from httpx import ASGITransport, AsyncClient
from sqlalchemy.ext.asyncio import (
    AsyncEngine,
    AsyncSession,
    async_sessionmaker,
    create_async_engine,
)

from rekenraam_api.api.dependencies import get_db_session
from rekenraam_api.app import app


@dataclass
class E2EApp:
    """Bundle of resources for an e2e test: the HTTP client plus the engine
    that backs it, so tests can poke the database directly when they need to
    inspect state that isn't reachable through the public API (e.g. issued
    password-reset tokens, audit rows)."""

    client: AsyncClient
    engine: AsyncEngine
    sessionmaker: async_sessionmaker[AsyncSession]


@pytest.fixture()
def e2e_database_url() -> Iterator[str]:
    try:
        with temporary_database() as url:
            yield url
    except Exception as exc:  # pragma: no cover
        pytest.skip(f"postgres not available for e2e tests: {exc}")


@pytest_asyncio.fixture()
async def e2e_app(e2e_database_url: str) -> AsyncIterator[E2EApp]:
    engine = create_async_engine(e2e_database_url, future=True)
    sessionmaker = async_sessionmaker(bind=engine, class_=AsyncSession, expire_on_commit=False)

    async def override_get_db_session() -> AsyncIterator[AsyncSession]:
        async with sessionmaker() as session:
            yield session

    app.dependency_overrides[get_db_session] = override_get_db_session
    try:
        transport = ASGITransport(app=app)
        async with AsyncClient(transport=transport, base_url="http://e2e") as client:
            yield E2EApp(client=client, engine=engine, sessionmaker=sessionmaker)
    finally:
        app.dependency_overrides.pop(get_db_session, None)
        await engine.dispose()


@pytest_asyncio.fixture()
async def e2e_client(e2e_app: E2EApp) -> AsyncIterator[AsyncClient]:
    """Convenience fixture for tests that only need the HTTP client."""

    yield e2e_app.client
