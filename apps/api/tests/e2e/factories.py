"""Small factory helpers for end-to-end tests.

These wrap common bootstrap/auth/account flows so tests stay focused on the
behavior under test instead of repeating setup. Every helper makes real HTTP
calls through `httpx.AsyncClient` against the running FastAPI app.
"""

from __future__ import annotations

from datetime import date

from httpx import AsyncClient

DEFAULT_ADMIN_EMAIL = "admin@example.com"
DEFAULT_ADMIN_PASSWORD = "correct-horse-battery-staple"
DEFAULT_ADMIN_NAME = "Admin"


async def bootstrap_admin(
    client: AsyncClient,
    *,
    email: str = DEFAULT_ADMIN_EMAIL,
    password: str = DEFAULT_ADMIN_PASSWORD,
    display_name: str = DEFAULT_ADMIN_NAME,
) -> dict:
    """Create the first admin and leave the client authenticated."""

    response = await client.post(
        "/api/v1/auth/bootstrap/admin",
        json={"email": email, "password": password, "display_name": display_name},
    )
    response.raise_for_status()
    return response.json()


async def login(
    client: AsyncClient,
    *,
    email: str = DEFAULT_ADMIN_EMAIL,
    password: str = DEFAULT_ADMIN_PASSWORD,
) -> dict:
    """Log in an existing user and leave the client authenticated."""

    response = await client.post(
        "/api/v1/auth/login",
        json={"email": email, "password": password},
    )
    response.raise_for_status()
    return response.json()


async def create_account(
    client: AsyncClient,
    *,
    name: str,
    account_type: str = "asset",
    book_id: int = 1,
    parent_id: int | None = None,
    commodity_id: int = 1,
) -> dict:
    """Create an account in the seeded `personal` book and return its summary."""

    response = await client.post(
        "/api/v1/accounts",
        json={
            "book_id": book_id,
            "parent_id": parent_id,
            "account_type": account_type,
            "name": name,
            "commodity_id": commodity_id,
            "institution_id": None,
            "country_id": None,
            "number_last4": None,
            "is_closed": False,
        },
    )
    response.raise_for_status()
    return response.json()


async def post_transaction(
    client: AsyncClient,
    *,
    txn_date: date,
    splits: list[dict],
    book_id: int = 1,
    payee_id: int | None = None,
    memo: str | None = None,
    status_value: str = "cleared",
    reference: str | None = None,
) -> dict:
    """Post a balanced transaction with the given splits and return its summary.

    Each split dict requires `account_id`, `commodity_id`, `amount_minor`; other
    fields default to None.
    """

    full_splits = [
        {
            "account_id": split["account_id"],
            "commodity_id": split["commodity_id"],
            "amount_minor": split["amount_minor"],
            "category_id": split.get("category_id"),
            "tag_id": split.get("tag_id"),
            "person_id": split.get("person_id"),
            "project_id": split.get("project_id"),
            "share_bps": split.get("share_bps"),
            "memo": split.get("memo"),
        }
        for split in splits
    ]
    response = await client.post(
        "/api/v1/transactions",
        json={
            "book_id": book_id,
            "txn_date": txn_date.isoformat(),
            "payee_id": payee_id,
            "memo": memo,
            "status": status_value,
            "reference": reference,
            "splits": full_splits,
        },
    )
    response.raise_for_status()
    return response.json()
