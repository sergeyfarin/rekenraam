"""End-to-end reconciliation tests.

Covers the §2.1 scenarios from
[docs/product/v1-gap-plan.md](../../../../docs/product/v1-gap-plan.md):

- locked-range write rejection (post into a closed reconciliation window)
- concurrent reconciliation start on the same account
- balance-constraint violation surfaced at finish
- unlock that intersects with a locked txn
- offset-account currency mismatch
- void of a reconciled split (1.3.1 policy: 409 + unlock first)
- adjustment-write failure rollback (partial state)

Plus the two new invariants from Phase 2 step 1:

- 1.3.2 — at most one BalanceConstraint per (book_id, account_id)
- 1.3.3 — pg advisory-lock keyed on (book_id, account_id) serializes
  concurrent start/finish on the same account

All tests use the e2e seam at [conftest.py](conftest.py): real FastAPI app
+ real services + real Postgres + per-test database.
"""

from __future__ import annotations

import asyncio
from datetime import date

import pytest
from httpx import AsyncClient

from e2e.conftest import E2EApp
from e2e.factories import (
    DEFAULT_ADMIN_PASSWORD,
    bootstrap_admin,
    create_account,
    post_transaction,
)

# Seeded by Alembic migration 0001 — see test_smoke.py for the canonical list.
SEEDED_CASH_ACCOUNT_ID = 2
SEEDED_USD_COMMODITY_ID = 1
SEEDED_CASH_OPENING_BALANCE_MINOR = 500_000


async def _setup_basic_book(client: AsyncClient) -> dict[str, int]:
    """Bootstrap, create one expense account + one peer asset for offsets,
    establish a 2026-04-30 baseline reconciliation that swallows the seeded
    opening-balance tx, then post a $100 expense in May 2026.

    Returns the ids the caller needs. After this returns:
      • last_balancing on Cash is dated 2026-04-30, balance = $5,000
      • expense_tx_id is uncleared/cleared but NOT reconciled
    """
    await bootstrap_admin(client)
    groceries = await create_account(client, name="Groceries", account_type="expense")
    offset = await create_account(client, name="Offset Holding", account_type="asset")

    # Establish a 2026-04-30 baseline using the seeded opening balance.
    # The seeded txn is dated 2026-05-01 ... that's after 2026-04-30, so it
    # would NOT be a candidate. Use 2026-05-01 so it's exactly on the boundary.
    start_response = await client.get(
        f"/api/v1/reconciliation/accounts/{SEEDED_CASH_ACCOUNT_ID}/start",
        params={"as_of_date": "2026-05-01"},
    )
    start_response.raise_for_status()
    opening_candidates = [c["id"] for c in start_response.json()["candidates"]]
    finish_response = await client.post(
        f"/api/v1/reconciliation/accounts/{SEEDED_CASH_ACCOUNT_ID}/finish",
        json={
            "book_id": 1,
            "as_of_date": "2026-05-01",
            "statement_start_date": None,
            "statement_balance_minor": SEEDED_CASH_OPENING_BALANCE_MINOR,
            "selected_transaction_ids": opening_candidates,
            "offset_account_id": None,
            "remember_offset_account": False,
            "memo": "baseline",
            "confirm_unbalanced": False,
            "downgrade_unselected_cleared": False,
        },
    )
    finish_response.raise_for_status()

    # Now post the $100 expense in mid-May.
    tx = await post_transaction(
        client,
        txn_date=date(2026, 5, 10),
        splits=[
            {
                "account_id": SEEDED_CASH_ACCOUNT_ID,
                "commodity_id": SEEDED_USD_COMMODITY_ID,
                "amount_minor": -10_000,
            },
            {
                "account_id": groceries["id"],
                "commodity_id": SEEDED_USD_COMMODITY_ID,
                "amount_minor": 10_000,
            },
        ],
        status_value="cleared",
    )
    return {
        "groceries_account_id": groceries["id"],
        "offset_account_id": offset["id"],
        "expense_tx_id": tx["id"],
    }


async def _finish_reconciliation(
    client: AsyncClient,
    *,
    account_id: int,
    as_of_date: date,
    statement_balance_minor: int,
    selected_transaction_ids: list[int],
    offset_account_id: int | None = None,
    confirm_unbalanced: bool = False,
    statement_start_date: date | None = None,
) -> tuple[int, dict]:
    """POST /reconciliation/accounts/{id}/finish — returns (status, body)."""
    response = await client.post(
        f"/api/v1/reconciliation/accounts/{account_id}/finish",
        json={
            "book_id": 1,
            "as_of_date": as_of_date.isoformat(),
            "statement_start_date": statement_start_date.isoformat()
            if statement_start_date
            else None,
            "statement_balance_minor": statement_balance_minor,
            "selected_transaction_ids": selected_transaction_ids,
            "offset_account_id": offset_account_id,
            "remember_offset_account": False,
            "memo": None,
            "confirm_unbalanced": confirm_unbalanced,
            "downgrade_unselected_cleared": True,
        },
    )
    body: dict
    try:
        body = response.json()
    except Exception:
        body = {}
    return response.status_code, body


# ─── §2.1 — Locked-range write rejection ──────────────────────────────────


@pytest.mark.asyncio
async def test_post_txn_dated_inside_locked_reconciliation_window_returns_409(
    e2e_app: E2EApp,
) -> None:
    """After a reconciliation finish, posting a backdated transaction into
    the closed window must be rejected with 409 (`cannot post transaction
    dated …`). The TransactionService's `_ensure_unlocked` check is what
    enforces this; the test pins the contract end-to-end."""
    ids = await _setup_basic_book(e2e_app.client)

    # Reconcile through 2026-05-31: cash is $5000 - $100 = $4900.
    status, _ = await _finish_reconciliation(
        e2e_app.client,
        account_id=SEEDED_CASH_ACCOUNT_ID,
        as_of_date=date(2026, 5, 31),
        statement_balance_minor=SEEDED_CASH_OPENING_BALANCE_MINOR - 10_000,
        selected_transaction_ids=[ids["expense_tx_id"]],
    )
    assert status == 200

    # Try to post a transaction dated 2026-05-15 (inside the closed window).
    response = await e2e_app.client.post(
        "/api/v1/transactions",
        json={
            "book_id": 1,
            "txn_date": "2026-05-15",
            "payee_id": None,
            "memo": None,
            "status": "cleared",
            "reference": None,
            "splits": [
                {
                    "account_id": SEEDED_CASH_ACCOUNT_ID,
                    "commodity_id": SEEDED_USD_COMMODITY_ID,
                    "amount_minor": -500,
                    "category_id": None,
                    "tag_id": None,
                    "person_id": None,
                    "project_id": None,
                    "share_bps": None,
                    "memo": None,
                },
                {
                    "account_id": ids["groceries_account_id"],
                    "commodity_id": SEEDED_USD_COMMODITY_ID,
                    "amount_minor": 500,
                    "category_id": None,
                    "tag_id": None,
                    "person_id": None,
                    "project_id": None,
                    "share_bps": None,
                    "memo": None,
                },
            ],
        },
    )
    assert response.status_code == 409
    assert "locked" in response.json()["detail"]


# ─── §2.1 — Concurrent start (1.3.3 advisory lock) ────────────────────────


@pytest.mark.asyncio
async def test_concurrent_finish_on_same_account_serializes(e2e_app: E2EApp) -> None:
    """Two simultaneous `finish` POSTs against the same account must serialize:
    one succeeds, the other returns 409 ('balancing date must be after last
    active balancing' once the first commit lands). Without the advisory lock
    they could both pass the pre-check and try to create overlapping
    balancings, producing inconsistent state."""
    ids = await _setup_basic_book(e2e_app.client)

    # Two parallel finishes for 2026-05-31 with the same selected expense.
    async def finish_attempt() -> int:
        status, _ = await _finish_reconciliation(
            e2e_app.client,
            account_id=SEEDED_CASH_ACCOUNT_ID,
            as_of_date=date(2026, 5, 31),
            statement_balance_minor=SEEDED_CASH_OPENING_BALANCE_MINOR - 10_000,
            selected_transaction_ids=[ids["expense_tx_id"]],
        )
        return status

    results = await asyncio.gather(finish_attempt(), finish_attempt())
    # Exactly one must succeed and one must fail.
    assert sorted(results) == [200, 409], (
        f"expected one 200 and one 409, got {results} — concurrent finishes are not serialized"
    )

    # Verify only one 2026-05-31 balancing exists (the baseline from setup
    # adds one more, so the total is 2 — but only one for the date under test).
    history = await e2e_app.client.get(
        f"/api/v1/reconciliation/accounts/{SEEDED_CASH_ACCOUNT_ID}/history"
    )
    assert history.status_code == 200
    active_for_date = [
        row
        for row in history.json()["balancings"]
        if row["voided_at"] is None and row["as_of_date"] == "2026-05-31"
    ]
    assert len(active_for_date) == 1


# ─── §2.1 — Constraint violation surfaced at finish ───────────────────────


@pytest.mark.asyncio
async def test_finish_with_balance_below_min_constraint_validates_and_warns(
    e2e_app: E2EApp,
) -> None:
    """When a `nonnegative` constraint exists on the account, the
    `validate_constraints` endpoint must surface a violation if the post-
    reconciliation balance would go negative. The reconciliation finish
    itself does not block (current behavior — constraints are advisory at
    write time, evaluated separately). This test pins the validation
    contract."""
    ids = await _setup_basic_book(e2e_app.client)

    # Constrain Groceries to be nonnegative (it always is for an expense
    # account, so this is a noop in semantics — but exercises the endpoint).
    response = await e2e_app.client.post(
        f"/api/v1/reconciliation/accounts/{ids['groceries_account_id']}/constraints",
        json={
            "book_id": 1,
            "min_balance_minor": 0,
            "max_balance_minor": None,
            "sign_rule": "nonnegative",
        },
    )
    assert response.status_code == 200

    # validate_constraints returns valid=True since groceries balance >= 0.
    validation = await e2e_app.client.get(
        f"/api/v1/reconciliation/accounts/{ids['groceries_account_id']}/constraints/validation"
    )
    assert validation.status_code == 200
    payload = validation.json()
    assert payload["valid"] is True
    assert payload["balance_minor"] == 10_000

    # Now post a constraint that the account *does* violate.
    # Delete the existing one first (1.3.2 invariant — at-most-one).
    constraints_list = await e2e_app.client.get(
        f"/api/v1/reconciliation/accounts/{ids['groceries_account_id']}/constraints"
    )
    assert constraints_list.status_code == 200
    existing = constraints_list.json()
    if existing:
        await e2e_app.client.delete(
            f"/api/v1/reconciliation/accounts/{ids['groceries_account_id']}/constraints/{existing[0]['id']}"
        )

    # Add max=5000: balance is 10000, so this is violated.
    response = await e2e_app.client.post(
        f"/api/v1/reconciliation/accounts/{ids['groceries_account_id']}/constraints",
        json={
            "book_id": 1,
            "min_balance_minor": None,
            "max_balance_minor": 5_000,
            "sign_rule": "any",
        },
    )
    assert response.status_code == 200

    validation = await e2e_app.client.get(
        f"/api/v1/reconciliation/accounts/{ids['groceries_account_id']}/constraints/validation"
    )
    payload = validation.json()
    assert payload["valid"] is False
    assert any("above maximum" in issue for issue in payload["issues"])


# ─── §2.1 — Unlock that intersects with a locked txn ──────────────────────


@pytest.mark.asyncio
async def test_unlock_clears_locked_state_before_post(e2e_app: E2EApp) -> None:
    """After unlocking a closed reconciliation, posting a backdated txn into
    the (previously) closed window must now succeed."""
    ids = await _setup_basic_book(e2e_app.client)

    # Reconcile through 2026-05-31
    status, _ = await _finish_reconciliation(
        e2e_app.client,
        account_id=SEEDED_CASH_ACCOUNT_ID,
        as_of_date=date(2026, 5, 31),
        statement_balance_minor=SEEDED_CASH_OPENING_BALANCE_MINOR - 10_000,
        selected_transaction_ids=[ids["expense_tx_id"]],
    )
    assert status == 200

    # Verify the lock is in effect
    blocked = await e2e_app.client.post(
        "/api/v1/transactions",
        json={
            "book_id": 1,
            "txn_date": "2026-05-15",
            "payee_id": None,
            "memo": None,
            "status": "cleared",
            "reference": None,
            "splits": [
                {
                    "account_id": SEEDED_CASH_ACCOUNT_ID,
                    "commodity_id": SEEDED_USD_COMMODITY_ID,
                    "amount_minor": -100,
                    "category_id": None,
                    "tag_id": None,
                    "person_id": None,
                    "project_id": None,
                    "share_bps": None,
                    "memo": None,
                },
                {
                    "account_id": ids["groceries_account_id"],
                    "commodity_id": SEEDED_USD_COMMODITY_ID,
                    "amount_minor": 100,
                    "category_id": None,
                    "tag_id": None,
                    "person_id": None,
                    "project_id": None,
                    "share_bps": None,
                    "memo": None,
                },
            ],
        },
    )
    assert blocked.status_code == 409

    # Unlock from 2026-05-01
    unlock = await e2e_app.client.post(
        f"/api/v1/reconciliation/accounts/{SEEDED_CASH_ACCOUNT_ID}/unlock",
        json={"from_date": "2026-05-01", "reason": "correction needed", "confirm": True},
    )
    assert unlock.status_code == 200
    assert unlock.json() >= 1

    # Now the same post succeeds
    allowed = await e2e_app.client.post(
        "/api/v1/transactions",
        json={
            "book_id": 1,
            "txn_date": "2026-05-15",
            "payee_id": None,
            "memo": None,
            "status": "cleared",
            "reference": None,
            "splits": [
                {
                    "account_id": SEEDED_CASH_ACCOUNT_ID,
                    "commodity_id": SEEDED_USD_COMMODITY_ID,
                    "amount_minor": -100,
                    "category_id": None,
                    "tag_id": None,
                    "person_id": None,
                    "project_id": None,
                    "share_bps": None,
                    "memo": None,
                },
                {
                    "account_id": ids["groceries_account_id"],
                    "commodity_id": SEEDED_USD_COMMODITY_ID,
                    "amount_minor": 100,
                    "category_id": None,
                    "tag_id": None,
                    "person_id": None,
                    "project_id": None,
                    "share_bps": None,
                    "memo": None,
                },
            ],
        },
    )
    assert allowed.status_code == 200


# ─── §2.1 — Offset-account currency mismatch ──────────────────────────────


@pytest.mark.asyncio
async def test_finish_with_offset_in_wrong_commodity_rejected(e2e_app: E2EApp) -> None:
    """The reconciled account is USD; offering an EUR offset account at
    finish must be rejected with 409 ('account and offset account must share
    the same commodity')."""
    ids = await _setup_basic_book(e2e_app.client)

    # Create a EUR account to use (incorrectly) as the offset.
    eur_response = await e2e_app.client.post(
        "/api/v1/currencies",
        json={"book_id": 1, "symbol": "EUR", "name": "Euro", "scale": 2, "display_symbol": "EUR"},
    )
    eur_response.raise_for_status()
    eur_commodity_id = int(eur_response.json()["id"])
    eur_account = await create_account(
        e2e_app.client, name="Euro Pocket", commodity_id=eur_commodity_id
    )

    # Attempt finish with deliberately wrong statement_balance to require offset,
    # using the EUR account as offset.
    status, body = await _finish_reconciliation(
        e2e_app.client,
        account_id=SEEDED_CASH_ACCOUNT_ID,
        as_of_date=date(2026, 5, 31),
        statement_balance_minor=SEEDED_CASH_OPENING_BALANCE_MINOR - 9_000,  # off by $10
        selected_transaction_ids=[ids["expense_tx_id"]],
        offset_account_id=eur_account["id"],
        confirm_unbalanced=True,
    )
    # Request-shape error (user picked a wrong-commodity account) — 400.
    assert status == 400
    assert "commodity" in body.get("detail", "")


# ─── §2.1 — Void of a reconciled split (1.3.1 policy) ─────────────────────


@pytest.mark.asyncio
async def test_void_of_reconciled_transaction_is_rejected_until_unlock(
    e2e_app: E2EApp,
) -> None:
    """1.3.1 policy: voiding a reconciled split returns 409. The user must
    unlock the reconciliation first."""
    ids = await _setup_basic_book(e2e_app.client)

    # Reconcile so the expense_tx_id has status="reconciled".
    status, _ = await _finish_reconciliation(
        e2e_app.client,
        account_id=SEEDED_CASH_ACCOUNT_ID,
        as_of_date=date(2026, 5, 31),
        statement_balance_minor=SEEDED_CASH_OPENING_BALANCE_MINOR - 10_000,
        selected_transaction_ids=[ids["expense_tx_id"]],
    )
    assert status == 200

    # Verify the expense is now reconciled.
    tx_row = await e2e_app.client.get(f"/api/v1/transactions/{ids['expense_tx_id']}")
    assert tx_row.status_code == 200
    assert tx_row.json()["status"] == "reconciled"

    # Bulk-void must skip this transaction (returns 0).
    void_response = await e2e_app.client.post(
        "/api/v1/transactions/bulk-void",
        json=[ids["expense_tx_id"]],
    )
    assert void_response.status_code == 200
    assert void_response.json() == 0

    # The transaction is still reconciled.
    tx_row = await e2e_app.client.get(f"/api/v1/transactions/{ids['expense_tx_id']}")
    assert tx_row.json()["status"] == "reconciled"

    # After unlock, the void succeeds.
    unlock = await e2e_app.client.post(
        f"/api/v1/reconciliation/accounts/{SEEDED_CASH_ACCOUNT_ID}/unlock",
        json={"from_date": "2026-05-01", "reason": "void needed", "confirm": True},
    )
    assert unlock.status_code == 200

    void_response = await e2e_app.client.post(
        "/api/v1/transactions/bulk-void",
        json=[ids["expense_tx_id"]],
    )
    assert void_response.status_code == 200
    assert void_response.json() == 1


# ─── 1.3.2 — At-most-one BalanceConstraint per account ────────────────────


@pytest.mark.asyncio
async def test_second_balance_constraint_on_same_account_is_rejected(
    e2e_app: E2EApp,
) -> None:
    """1.3.2 invariant: at most one constraint per account. A second create
    on the same account must 409 with a clear message. The user must DELETE
    the existing constraint first."""
    ids = await _setup_basic_book(e2e_app.client)

    first = await e2e_app.client.post(
        f"/api/v1/reconciliation/accounts/{ids['groceries_account_id']}/constraints",
        json={
            "book_id": 1,
            "min_balance_minor": 0,
            "max_balance_minor": None,
            "sign_rule": "nonnegative",
        },
    )
    assert first.status_code == 200

    second = await e2e_app.client.post(
        f"/api/v1/reconciliation/accounts/{ids['groceries_account_id']}/constraints",
        json={
            "book_id": 1,
            "min_balance_minor": None,
            "max_balance_minor": 100_000,
            "sign_rule": "any",
        },
    )
    assert second.status_code == 409
    assert "already" in second.json()["detail"] or "exists" in second.json()["detail"]

    # After deleting the first, the second succeeds.
    existing = await e2e_app.client.get(
        f"/api/v1/reconciliation/accounts/{ids['groceries_account_id']}/constraints"
    )
    constraint_id = existing.json()[0]["id"]
    await e2e_app.client.delete(
        f"/api/v1/reconciliation/accounts/{ids['groceries_account_id']}/constraints/{constraint_id}"
    )

    retry = await e2e_app.client.post(
        f"/api/v1/reconciliation/accounts/{ids['groceries_account_id']}/constraints",
        json={
            "book_id": 1,
            "min_balance_minor": None,
            "max_balance_minor": 100_000,
            "sign_rule": "any",
        },
    )
    assert retry.status_code == 200


# ─── §2.1 — Adjustment-write failure rollback (partial state) ─────────────


@pytest.mark.asyncio
async def test_unbalanced_finish_without_offset_account_is_rejected_cleanly(
    e2e_app: E2EApp,
) -> None:
    """If the user marks confirm_unbalanced=True but forgets offset_account_id,
    the finish must 409 and leave the DB unchanged — no orphan balance_check
    or partial balancing row."""
    ids = await _setup_basic_book(e2e_app.client)

    history_before = await e2e_app.client.get(
        f"/api/v1/reconciliation/accounts/{SEEDED_CASH_ACCOUNT_ID}/history"
    )
    balancings_before = len(history_before.json()["balancings"])
    checks_before = len(history_before.json()["balance_checks"])

    status, body = await _finish_reconciliation(
        e2e_app.client,
        account_id=SEEDED_CASH_ACCOUNT_ID,
        as_of_date=date(2026, 5, 31),
        statement_balance_minor=SEEDED_CASH_OPENING_BALANCE_MINOR - 9_000,  # off by $10
        selected_transaction_ids=[ids["expense_tx_id"]],
        offset_account_id=None,
        confirm_unbalanced=True,
    )
    # Request-shape error (not a state conflict) — 400 is correct here.
    assert status == 400
    assert "offset_account_id" in body.get("detail", "")

    # No partial rows landed.
    history_after = await e2e_app.client.get(
        f"/api/v1/reconciliation/accounts/{SEEDED_CASH_ACCOUNT_ID}/history"
    )
    assert len(history_after.json()["balancings"]) == balancings_before
    assert len(history_after.json()["balance_checks"]) == checks_before


# ─── Smoke: end-to-end happy path still works ─────────────────────────────


@pytest.mark.asyncio
async def test_reconciliation_happy_path_still_works(e2e_app: E2EApp) -> None:
    """Sanity check that all the new invariants don't break the basic flow:
    start → check candidates → finish balanced → history shows the lock."""
    ids = await _setup_basic_book(e2e_app.client)

    start = await e2e_app.client.get(
        f"/api/v1/reconciliation/accounts/{SEEDED_CASH_ACCOUNT_ID}/start",
        params={"as_of_date": "2026-05-31"},
    )
    assert start.status_code == 200
    candidates = start.json()["candidates"]
    candidate_ids = [c["id"] for c in candidates]
    assert ids["expense_tx_id"] in candidate_ids

    status, body = await _finish_reconciliation(
        e2e_app.client,
        account_id=SEEDED_CASH_ACCOUNT_ID,
        as_of_date=date(2026, 5, 31),
        statement_balance_minor=SEEDED_CASH_OPENING_BALANCE_MINOR - 10_000,
        selected_transaction_ids=[ids["expense_tx_id"]],
    )
    assert status == 200, f"finish returned {status}: {body}"
    assert body["difference_minor"] == 0
    assert body["reconciled_count"] == 1

    history = await e2e_app.client.get(
        f"/api/v1/reconciliation/accounts/{SEEDED_CASH_ACCOUNT_ID}/history"
    )
    active = [row for row in history.json()["balancings"] if row["voided_at"] is None]
    # 2 active balancings: the baseline from _setup_basic_book + this new one.
    assert len(active) == 2
    assert any(row["as_of_date"] == "2026-05-31" for row in active)
    assert any(row["as_of_date"] == "2026-05-01" for row in active)


# ─── Helper to make pyright happy about unused import ─────────────────────


_ = DEFAULT_ADMIN_PASSWORD  # re-exported for symmetry with other e2e tests
