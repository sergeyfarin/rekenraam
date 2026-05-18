"""End-to-end fixtures: real FastAPI app, real services, real database.

Unlike `test_api.py` (stub services) and `test_services.py` (fake repos),
these fixtures wire the full router -> service -> repository -> SQLite
stack and expose it through an `httpx.AsyncClient` so tests exercise the
same code path that the v1 single-container runtime hits.

Lifespan is intentionally NOT executed:
- the bootstrap-admin seed in lifespan reads `FIRST_ADMIN_EMAIL`/`FIRST_ADMIN_PASSWORD`
  from settings; tests bootstrap explicitly via the API instead
- the pricing refresh worker would issue real outbound HTTP requests
"""

from __future__ import annotations

import asyncio
import os
import sys
import tempfile
from collections.abc import AsyncIterator
from dataclasses import dataclass
from pathlib import Path

import pytest_asyncio
from _sqlite import database_url_for
from httpx import ASGITransport, AsyncClient
from sqlalchemy.ext.asyncio import (
    AsyncEngine,
    AsyncSession,
    async_sessionmaker,
)

from rekenraam_api.api.dependencies import get_db_session
from rekenraam_api.app import app
from rekenraam_api.db.dialect import create_database_engine

E2E_API_ROOT = Path(__file__).resolve().parents[2]


@dataclass
class E2EApp:
    """Bundle of resources for an e2e test: the HTTP client plus the engine
    that backs it, so tests can poke the database directly when they need to
    inspect state that isn't reachable through the public API (e.g. issued
    password-reset tokens, audit rows)."""

    client: AsyncClient
    engine: AsyncEngine
    sessionmaker: async_sessionmaker[AsyncSession]


@pytest_asyncio.fixture()
async def e2e_database_url() -> AsyncIterator[str]:
    _ = os.environ.get("API_E2E_DB_BACKEND")
    with tempfile.TemporaryDirectory(prefix="rekenraam_sqlite_e2e_") as directory:
        database_url = database_url_for(Path(directory) / "rekenraam.sqlite3")
        await _run_sqlite_migrations_subprocess(database_url)
        yield database_url


async def _run_sqlite_migrations_subprocess(database_url: str) -> None:
    env = os.environ.copy()
    env["DATABASE_URL"] = database_url
    process = await asyncio.create_subprocess_exec(
        sys.executable,
        "-m",
        "alembic",
        "upgrade",
        "head",
        cwd=E2E_API_ROOT,
        env=env,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    stdout, stderr = await process.communicate()
    if process.returncode != 0:
        raise RuntimeError(
            f"SQLite e2e migration failed\nstdout:\n{stdout.decode()}\nstderr:\n{stderr.decode()}"
        )


@pytest_asyncio.fixture()
async def e2e_app(e2e_database_url: str) -> AsyncIterator[E2EApp]:
    engine = create_database_engine(e2e_database_url)
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
