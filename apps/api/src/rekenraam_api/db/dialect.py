from __future__ import annotations

from pathlib import Path
from typing import Any

from sqlalchemy import Engine, event
from sqlalchemy.ext.asyncio import AsyncEngine, AsyncSession, create_async_engine
from sqlalchemy.pool import NullPool


def database_kind_from_url(url: str) -> str:
    if url.startswith("sqlite"):
        return "sqlite"
    if url.startswith("postgresql"):
        return "postgresql"
    return "unknown"


def session_dialect_name(session: AsyncSession) -> str:
    bind = session.get_bind()
    return bind.dialect.name


def is_sqlite_session(session: AsyncSession) -> bool:
    return session_dialect_name(session) == "sqlite"


def sqlite_path_from_url(url: str) -> str | None:
    if not url.startswith("sqlite"):
        return None
    prefix = "sqlite+aiosqlite:///"
    if url.startswith(prefix):
        raw_path = url.removeprefix(prefix)
        return raw_path if raw_path.startswith("/") else f"/{raw_path}"
    prefix = "sqlite:///"
    if url.startswith(prefix):
        raw_path = url.removeprefix(prefix)
        return raw_path if raw_path.startswith("/") else f"/{raw_path}"
    return None


def create_database_engine(database_url: str) -> AsyncEngine:
    kind = database_kind_from_url(database_url)
    kwargs: dict[str, object] = {"future": True}
    if kind == "sqlite":
        path = sqlite_path_from_url(database_url)
        if path and path != ":memory:":
            Path(path).parent.mkdir(parents=True, exist_ok=True)
        kwargs["poolclass"] = NullPool
    engine = create_async_engine(database_url, **kwargs)
    if kind == "sqlite":
        install_sqlite_pragmas(engine)
    return engine


def install_sqlite_pragmas(engine: AsyncEngine) -> None:
    def set_sqlite_pragmas(dbapi_connection: Any, _connection_record: object) -> None:
        cursor = dbapi_connection.cursor()
        try:
            cursor.execute("PRAGMA foreign_keys=ON")
            cursor.execute("PRAGMA busy_timeout=5000")
            cursor.execute("PRAGMA journal_mode=WAL")
        finally:
            cursor.close()

    event.listen(engine.sync_engine, "connect", set_sqlite_pragmas)


def sync_engine_kind(connection_or_engine: Engine) -> str:
    return connection_or_engine.dialect.name
