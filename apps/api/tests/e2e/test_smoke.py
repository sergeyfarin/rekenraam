"""End-to-end smoke test.

Exercises the full router -> service -> repository -> SQLite path against a
freshly-migrated database. This is the canonical example for the e2e seam: if
this test passes, the seam is wired correctly and other tests can build on the
same fixtures.

The seeded `personal` book ships with a $5,000 opening balance into the Cash
account (account_id=2, see migration 0001). This test bootstraps an admin,
creates a new "Checking" account, posts a balanced transfer of $1,234.56 from
Cash to Checking, and asserts the running balances on both registers reflect
that movement.
"""

from __future__ import annotations

from datetime import date

import pytest
from httpx import AsyncClient

from e2e.factories import (
    DEFAULT_ADMIN_PASSWORD,
    bootstrap_admin,
    create_account,
    login,
    post_transaction,
)

# Seeded by Alembic migration 0001 — see apps/api/alembic/versions/0001_initial_schema.py.
SEEDED_CASH_ACCOUNT_ID = 2
SEEDED_CASH_OPENING_BALANCE_MINOR = 500_000
SEEDED_USD_COMMODITY_ID = 1


@pytest.mark.asyncio
async def test_full_stack_round_trip(e2e_client: AsyncClient) -> None:
    # Bootstrap status reports a fresh database and is reachable without auth.
    bootstrap_status = await e2e_client.get("/api/v1/auth/bootstrap/status")
    assert bootstrap_status.status_code == 200
    assert bootstrap_status.json() == {"bootstrap_required": True}

    # Bootstrap leaves the AsyncClient holding the session cookie automatically.
    me = await bootstrap_admin(e2e_client)
    assert me["user"]["is_admin"] is True

    # Bootstrap is single-shot: a second attempt must be rejected.
    second_bootstrap = await e2e_client.post(
        "/api/v1/auth/bootstrap/admin",
        json={
            "email": "other@example.com",
            "password": DEFAULT_ADMIN_PASSWORD,
            "display_name": "Other",
        },
    )
    assert second_bootstrap.status_code == 409

    # The bootstrapped admin is auto-granted owner membership on the seeded
    # `personal` book, so a list request reflects it without further setup.
    books = await e2e_client.get("/api/v1/books")
    assert books.status_code == 200
    book_slugs = [book["slug"] for book in books.json()]
    assert "personal" in book_slugs

    # Create a new asset account in the seeded book.
    checking = await create_account(e2e_client, name="Checking")
    checking_id = checking["id"]
    assert checking["name"] == "Checking"

    # Confirm the opening balance posted by the migration is visible through
    # the same code path the UI uses. Accounts without any postings may be
    # absent from the result, so we check Cash explicitly.
    balances_before = await e2e_client.get("/api/v1/accounts/balances")
    assert balances_before.status_code == 200
    balance_map = {row["account_id"]: row["balance_minor"] for row in balances_before.json()}
    assert balance_map[SEEDED_CASH_ACCOUNT_ID] == SEEDED_CASH_OPENING_BALANCE_MINOR
    assert balance_map.get(checking_id, 0) == 0

    # Move $1,234.56 from Cash to the new Checking account.
    transfer_minor = 123_456
    txn = await post_transaction(
        e2e_client,
        txn_date=date(2026, 5, 9),
        memo="opening transfer",
        splits=[
            {
                "account_id": SEEDED_CASH_ACCOUNT_ID,
                "commodity_id": SEEDED_USD_COMMODITY_ID,
                "amount_minor": -transfer_minor,
                "memo": "from cash",
            },
            {
                "account_id": checking_id,
                "commodity_id": SEEDED_USD_COMMODITY_ID,
                "amount_minor": transfer_minor,
                "memo": "to checking",
            },
        ],
    )
    assert sum(split["amount_minor"] for split in txn["splits"]) == 0

    # Balances must reflect the transfer for both sides.
    balances_after = await e2e_client.get("/api/v1/accounts/balances")
    balance_map = {row["account_id"]: row["balance_minor"] for row in balances_after.json()}
    assert balance_map[SEEDED_CASH_ACCOUNT_ID] == (
        SEEDED_CASH_OPENING_BALANCE_MINOR - transfer_minor
    )
    assert balance_map[checking_id] == transfer_minor

    # The register endpoint computes a running balance per split — verify it
    # matches the post-transfer balance for the Checking account.
    register = await e2e_client.get(f"/api/v1/accounts/{checking_id}/register")
    assert register.status_code == 200
    items = register.json()["items"]
    assert len(items) == 1
    assert items[0]["amount_minor"] == transfer_minor
    assert items[0]["running_balance_minor"] == transfer_minor

    # Logout revokes the server-side session. Even with the cookie still set,
    # the next protected call must 401 because the token is no longer active.
    logout = await e2e_client.post("/api/v1/auth/logout")
    assert logout.status_code in (200, 204)
    after_logout = await e2e_client.get("/api/v1/accounts/balances")
    assert after_logout.status_code == 401

    # Logging back in issues a fresh session and protected calls work again.
    relogin = await login(e2e_client)
    assert relogin["user"]["email"] == me["user"]["email"]
    rebalance = await e2e_client.get("/api/v1/accounts/balances")
    assert rebalance.status_code == 200
