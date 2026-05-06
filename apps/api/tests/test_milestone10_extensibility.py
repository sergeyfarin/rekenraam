from __future__ import annotations

from collections.abc import AsyncIterator, Iterator
from pathlib import Path

import pytest
from httpx import ASGITransport, AsyncClient
from sqlalchemy.ext.asyncio import AsyncSession

import rekenraam_api.db.models  # noqa: F401
from rekenraam_api.api.dependencies import get_db_session
from rekenraam_api.app import app
from rekenraam_api.db.base import Base

REPO_ROOT = Path(__file__).resolve().parents[3]
RESERVED_PREFIXES = ("/api/v1/plugins", "/api/v1/themes")


@pytest.fixture(autouse=True)
def clear_dependency_overrides() -> Iterator[None]:
    app.dependency_overrides.clear()
    yield
    app.dependency_overrides.clear()


@pytest.fixture()
async def client(repository_session: AsyncSession) -> AsyncIterator[AsyncClient]:
    async def override_session() -> AsyncIterator[AsyncSession]:
        yield repository_session

    app.dependency_overrides[get_db_session] = override_session
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://testserver") as async_client:
        yield async_client


async def _bootstrap_admin(client: AsyncClient) -> None:
    response = await client.post(
        "/api/v1/auth/bootstrap/admin",
        json={
            "email": "admin10@example.test",
            "password": "correct horse battery staple",
            "display_name": "Admin",
        },
    )
    assert response.status_code == 200


@pytest.mark.asyncio
async def test_plugin_and_theme_namespaces_are_reserved_but_not_implemented(
    client: AsyncClient,
) -> None:
    await _bootstrap_admin(client)

    for path in (
        "/api/v1/plugins",
        "/api/v1/plugins/example",
        "/api/v1/themes",
        "/api/v1/themes/example",
    ):
        response = await client.get(path)
        assert response.status_code == 404


def test_fastapi_route_table_has_no_b1_plugin_or_theme_routes() -> None:
    registered_paths = {getattr(route, "path", "") for route in app.routes}

    assert not {
        path
        for path in registered_paths
        if path.startswith(RESERVED_PREFIXES)
    }


@pytest.mark.asyncio
async def test_theme_preference_remains_future_compatible(client: AsyncClient) -> None:
    await _bootstrap_admin(client)

    for theme in ("system", "light", "dark", "post-b1-token-pack-preview"):
        response = await client.put(
            "/api/v1/preferences",
            json={
                "default_book_id": 1,
                "locale": "en-US",
                "date_format": "iso",
                "number_format": "system",
                "theme": theme,
            },
        )
        assert response.status_code == 200
        assert response.json()["theme"] == theme

        loaded = await client.get("/api/v1/preferences")
        assert loaded.status_code == 200
        assert loaded.json()["theme"] == theme


@pytest.mark.asyncio
async def test_theme_preference_rejects_overlong_values(client: AsyncClient) -> None:
    await _bootstrap_admin(client)

    response = await client.put("/api/v1/preferences", json={"theme": "x" * 65})

    assert response.status_code == 422


def test_b1_schema_has_no_plugin_or_theme_runtime_tables() -> None:
    table_names = set(Base.metadata.tables)

    assert not {
        table_name
        for table_name in table_names
        if table_name.startswith(("plugin", "plugins", "theme", "themes"))
    }


def test_b1_dependencies_do_not_include_plugin_runtimes() -> None:
    dependency_text = "\n".join(
        [
            (REPO_ROOT / "apps/api/pyproject.toml").read_text(),
            (REPO_ROOT / "package.json").read_text(),
        ]
    ).lower()

    forbidden_runtime_dependencies = (
        "extism",
        "wasmtime",
        "wasmer",
        "wasmedge",
        "open-policy-agent",
    )
    assert not any(name in dependency_text for name in forbidden_runtime_dependencies)


def test_docs_keep_plugin_theme_execution_deferred_and_permissioned() -> None:
    docs_text = "\n".join(
        [
            (REPO_ROOT / "SELF_HOSTED_MIGRATION_PLAN.md").read_text(),
            (REPO_ROOT / "README.md").read_text(),
            (REPO_ROOT / "docs/product/v1-scope.md").read_text(),
            (REPO_ROOT / "docs/architecture/postgres-schema.md").read_text(),
            (REPO_ROOT / "docs/architecture/post-b1-extensibility.md").read_text(),
            (REPO_ROOT / "docs/parity/desktop-to-python.md").read_text(),
        ]
    ).lower()

    required_phrases = (
        "plugin/theme runtime deferred",
        "no `/api/v1/plugins/*` or `/api/v1/themes/*` endpoints in b1",
        "semantic css tokens",
        "webassembly",
        "sidecar",
        "manifest-declared capabilities",
        "admin review",
        "disabled/failed-plugin isolation",
        "deterministic fallback",
        "no arbitrary remote css loading",
    )
    for phrase in required_phrases:
        assert phrase in docs_text
