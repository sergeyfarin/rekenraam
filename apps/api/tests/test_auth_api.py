from collections.abc import AsyncIterator

import pytest
from httpx import ASGITransport, AsyncClient
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from rekenraam_api.api.dependencies import get_db_session
from rekenraam_api.app import app
from rekenraam_api.db.models.access import BookMembership, User
from rekenraam_api.db.models.accounts import AccountBalancing
from rekenraam_api.db.models.transactions import Transaction


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


async def _bootstrap_admin(client: AsyncClient) -> dict[str, object]:
    response = await client.post(
        "/api/v1/auth/bootstrap/admin",
        json={
            "email": "admin@example.test",
            "password": "correct horse battery staple",
            "display_name": "Admin",
        },
    )
    assert response.status_code == 200
    return response.json()


@pytest.mark.asyncio
async def test_bootstrap_login_logout_and_protected_routes(client: AsyncClient) -> None:
    unauthenticated = await client.get("/api/v1/books")
    assert unauthenticated.status_code == 401

    status_before = await client.get("/api/v1/auth/bootstrap/status")
    assert status_before.status_code == 200
    assert status_before.json() == {"bootstrap_required": True}

    created = await _bootstrap_admin(client)
    assert created["user"]["email"] == "admin@example.test"
    assert "rekenraam_session" in client.cookies

    status_after = await client.get("/api/v1/auth/bootstrap/status")
    assert status_after.json() == {"bootstrap_required": False}

    duplicate = await client.post(
        "/api/v1/auth/bootstrap/admin",
        json={
            "email": "second@example.test",
            "password": "correct horse battery staple",
            "display_name": "Second",
        },
    )
    assert duplicate.status_code == 409

    books = await client.get("/api/v1/books")
    assert books.status_code == 200
    assert [book["slug"] for book in books.json()] == ["personal"]

    logout = await client.post("/api/v1/auth/logout")
    assert logout.status_code == 204
    assert "rekenraam_session" not in client.cookies

    logged_out = await client.get("/api/v1/books")
    assert logged_out.status_code == 401

    bad_login = await client.post(
        "/api/v1/auth/login",
        json={"email": "admin@example.test", "password": "wrong"},
    )
    assert bad_login.status_code == 401

    good_login = await client.post(
        "/api/v1/auth/login",
        json={"email": "admin@example.test", "password": "correct horse battery staple"},
    )
    assert good_login.status_code == 200
    assert good_login.json()["user"]["display_name"] == "Admin"
    assert good_login.json()["mfa_required"] is False


@pytest.mark.asyncio
async def test_book_membership_roles_gate_writes(
    client: AsyncClient,
    repository_session: AsyncSession,
) -> None:
    await _bootstrap_admin(client)
    await client.post("/api/v1/auth/logout")

    repository_session.add(
        User(
            email="viewer@example.test",
            password_hash=None,
            display_name="Viewer",
            is_admin=False,
        )
    )
    await repository_session.flush()
    viewer_id = int((await repository_session.execute(select(User.id).where(User.email == "viewer@example.test"))).scalar_one())
    repository_session.add(BookMembership(user_id=viewer_id, book_id=1, role="viewer"))
    await repository_session.commit()

    viewer = await repository_session.get(User, viewer_id)
    assert viewer is not None
    from rekenraam_api.services.auth import _password_hasher

    viewer.password_hash = _password_hasher.hash("correct horse battery staple")
    await repository_session.commit()

    login = await client.post(
        "/api/v1/auth/login",
        json={"email": "viewer@example.test", "password": "correct horse battery staple"},
    )
    assert login.status_code == 200

    read = await client.get("/api/v1/books")
    assert read.status_code == 200

    write = await client.post(
        "/api/v1/accounts/2/balancings",
        json={
            "book_id": 1,
            "account_id": 2,
            "as_of_date": "2026-05-02",
            "balance_minor": 500000,
            "memo": "viewer should not write",
        },
    )
    assert write.status_code == 403


@pytest.mark.asyncio
async def test_user_writes_stamp_audit_context(
    client: AsyncClient,
    repository_session: AsyncSession,
) -> None:
    await _bootstrap_admin(client)

    # Account 3 is the seeded "Opening Balances" system account; the transaction
    # service refuses splits against protected system accounts. Create a regular
    # expense account to use as the offset side instead.
    expense = await client.post(
        "/api/v1/accounts",
        json={
            "book_id": 1,
            "parent_id": None,
            "account_type": "expense",
            "name": "Groceries",
            "commodity_id": 1,
            "institution_id": None,
            "country_id": None,
            "number_last4": None,
            "is_closed": False,
        },
    )
    assert expense.status_code == 200
    expense_id = expense.json()["id"]

    transaction = await client.post(
        "/api/v1/transactions",
        json={
            "book_id": 1,
            "txn_date": "2026-05-04",
            "payee_id": None,
            "memo": "Audited",
            "status": "cleared",
            "reference": None,
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
                    "memo": "cash",
                },
                {
                    "account_id": expense_id,
                    "commodity_id": 1,
                    "amount_minor": 1000,
                    "category_id": None,
                    "tag_id": None,
                    "person_id": None,
                    "project_id": None,
                    "share_bps": None,
                    "memo": "offset",
                },
            ],
        },
    )
    assert transaction.status_code == 200
    transaction_id = transaction.json()["id"]

    row = await repository_session.get(Transaction, transaction_id)
    assert row is not None
    assert row.created_by_user_id == 1
    assert row.created_session_id is not None
    assert row.created_device_id is not None
    assert row.created_request_id is not None

    balancing = await client.post(
        "/api/v1/accounts/2/balancings",
        json={
            "book_id": 1,
            "account_id": 2,
            "as_of_date": "2026-05-02",
            "balance_minor": 499000,
            "memo": "Audited checkpoint",
        },
    )
    assert balancing.status_code == 200
    balancing_row = await repository_session.get(AccountBalancing, balancing.json()["id"])
    assert balancing_row is not None
    assert balancing_row.created_by_user_id == 1
