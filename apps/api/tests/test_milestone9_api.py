from collections.abc import AsyncIterator

import pytest
from httpx import ASGITransport, AsyncClient
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.api.dependencies import get_db_session
from rekenraam_api.app import app


@pytest.fixture(autouse=True)
def clear_dependency_overrides() -> AsyncIterator[None]:
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
            "email": "admin9@example.test",
            "password": "correct horse battery staple",
            "display_name": "Admin",
        },
    )
    assert response.status_code == 200


@pytest.mark.asyncio
async def test_admin_user_lifecycle_roles_audit_and_deactivation(client: AsyncClient) -> None:
    await _bootstrap_admin(client)

    created = await client.post(
        "/api/v1/admin/users",
        json={
            "email": "editor@example.test",
            "display_name": "Editor",
            "password": "temporary password 123",
            "is_admin": False,
            "memberships": [[1, "editor"]],
        },
    )
    assert created.status_code == 200
    user_id = created.json()["id"]
    assert created.json()["memberships"][0]["role"] == "editor"

    reset = await client.post(
        f"/api/v1/admin/users/{user_id}/password",
        json={"password": "new temporary password 123"},
    )
    assert reset.status_code == 200

    revoke = await client.post(f"/api/v1/admin/users/{user_id}/sessions/revoke")
    assert revoke.status_code == 200

    role = await client.put(
        f"/api/v1/admin/users/{user_id}/memberships",
        json={"book_id": 1, "role": "viewer"},
    )
    assert role.status_code == 200
    assert role.json()["memberships"][0]["role"] == "viewer"

    audit = await client.get("/api/v1/admin/audit-events")
    assert audit.status_code == 200
    assert {event["event_type"] for event in audit.json()} >= {
        "admin.user.created",
        "admin.user.password_reset",
        "admin.book_role.updated",
    }

    deactivate = await client.put(f"/api/v1/admin/users/{user_id}", json={"is_active": False})
    assert deactivate.status_code == 200
    assert deactivate.json()["is_active"] is False

    await client.post("/api/v1/auth/logout")
    blocked = await client.post(
        "/api/v1/auth/login",
        json={"email": "editor@example.test", "password": "new temporary password 123"},
    )
    assert blocked.status_code == 401


@pytest.mark.asyncio
async def test_preferences_profile_password_saved_views_templates_defaults_and_notes(
    client: AsyncClient,
) -> None:
    await _bootstrap_admin(client)

    profile = await client.put("/api/v1/auth/me/profile", json={"display_name": "Primary Admin"})
    assert profile.status_code == 200
    assert profile.json()["user"]["display_name"] == "Primary Admin"

    preferences = await client.put(
        "/api/v1/preferences",
        json={
            "default_book_id": 1,
            "locale": "en-US",
            "date_format": "iso",
            "number_format": "system",
            "theme": "system",
        },
    )
    assert preferences.status_code == 200
    assert preferences.json()["default_book_id"] == 1

    saved = await client.post(
        "/api/v1/search/transaction-views",
        json={
            "book_id": 1,
            "name": "Cleared groceries",
            "filters": {"book_id": 1, "status": "cleared", "search": "groceries"},
            "is_shared": False,
        },
    )
    assert saved.status_code == 200
    view_id = saved.json()["id"]
    listed = await client.get("/api/v1/search/transaction-views?book_id=1")
    assert listed.status_code == 200
    assert [view["id"] for view in listed.json()] == [view_id]
    offset_account = await client.post(
        "/api/v1/accounts",
        json={
            "book_id": 1,
            "parent_id": 1,
            "account_type": "asset",
            "name": "Template offset",
            "commodity_id": 1,
            "institution_id": None,
            "country_id": None,
            "number_last4": None,
            "is_closed": False,
        },
    )
    assert offset_account.status_code == 200
    offset_account_id = offset_account.json()["id"]

    template = await client.post(
        "/api/v1/templates/transactions",
        json={
            "book_id": 1,
            "name": "Transfer template",
            "payee_id": None,
            "memo": "Template memo",
            "status": "cleared",
            "reference": None,
            "is_shared": False,
            "splits": [
                {
                    "account_id": 2,
                    "commodity_id": 1,
                    "amount_minor": -1000,
                    "category_id": None,
                    "tag_id": None,
                    "person_id": None,
                    "project_id": None,
                    "share_bps": None,
                    "memo": "source",
                    },
                    {
                        "account_id": offset_account_id,
                        "commodity_id": 1,
                        "amount_minor": 1000,
                    "category_id": None,
                    "tag_id": None,
                    "person_id": None,
                    "project_id": None,
                    "share_bps": None,
                    "memo": "target",
                },
            ],
        },
    )
    assert template.status_code == 200
    posted = await client.post(
        f"/api/v1/templates/transactions/{template.json()['id']}/post",
        json={"txn_date": "2026-05-05"},
    )
    assert posted.status_code == 200
    assert posted.json()["memo"] == "Template memo"

    account_note = await client.post(
        "/api/v1/notes/accounts/2",
        json={"title": "Account note", "body_markdown": "**Remember** this", "pinned": True},
    )
    assert account_note.status_code == 200
    notes = await client.get("/api/v1/notes/accounts/2")
    assert notes.status_code == 200
    assert notes.json()[0]["title"] == "Account note"

    password = await client.post(
        "/api/v1/auth/me/password",
        json={
            "current_password": "correct horse battery staple",
            "new_password": "replacement password 123",
        },
    )
    assert password.status_code == 204

    await client.post("/api/v1/auth/logout")
    login = await client.post(
        "/api/v1/auth/login",
        json={"email": "admin9@example.test", "password": "replacement password 123"},
    )
    assert login.status_code == 200
