"""End-to-end import tests, covering paths the service-level tests couldn't
exercise cleanly (e.g. locked-account abandonment, which fails with
`MissingGreenlet` against a service-level repository fixture).

Originally tracked alongside Phase 2 step 1 (reconciliation correctness) per
[v1-gap-plan.md §Findings](../../../../docs/product/v1-gap-plan.md).
"""

from __future__ import annotations

import pytest
from sqlalchemy import select

from e2e.conftest import E2EApp
from e2e.factories import bootstrap_admin, create_account
from rekenraam_api.db.models.imports import ImportSession

SEEDED_CASH_ACCOUNT_ID = 2
SEEDED_USD_COMMODITY_ID = 1
SEEDED_CASH_OPENING_BALANCE_MINOR = 500_000


@pytest.mark.asyncio
async def test_import_commit_marks_session_abandoned_on_locked_account(
    e2e_app: E2EApp,
) -> None:
    """Committing an import into an account whose range is locked by a
    reconciliation must:

    1. Return 409 with the `cannot post transaction dated` message
       (locked-range check from TransactionService._ensure_unlocked).
    2. Mark the in-flight ImportSession row as `abandoned` so the user can
       see in /sessions that the attempt didn't silently succeed.

    The original repository-fixture version of this test failed with
    `sqlalchemy.exc.MissingGreenlet` because the abandonment path needs to
    read back the session row through an async-capable connection that the
    in-memory fake didn't provide. The e2e seam runs against a real
    Postgres connection so the path works end-to-end.
    """
    await bootstrap_admin(e2e_app.client)

    # Establish a baseline reconciliation on Cash through 2026-05-31 so any
    # transaction dated 2026-05-31 or earlier becomes locked.
    target = await create_account(e2e_app.client, name="Import Target", account_type="asset")

    # Reconcile Cash to lock the May window. Use the seeded opening tx.
    start_response = await e2e_app.client.get(
        f"/api/v1/reconciliation/accounts/{SEEDED_CASH_ACCOUNT_ID}/start",
        params={"as_of_date": "2026-05-31"},
    )
    start_response.raise_for_status()
    candidates = [c["id"] for c in start_response.json()["candidates"]]
    finish = await e2e_app.client.post(
        f"/api/v1/reconciliation/accounts/{SEEDED_CASH_ACCOUNT_ID}/finish",
        json={
            "book_id": 1,
            "as_of_date": "2026-05-31",
            "statement_start_date": None,
            "statement_balance_minor": SEEDED_CASH_OPENING_BALANCE_MINOR,
            "selected_transaction_ids": candidates,
            "offset_account_id": None,
            "remember_offset_account": False,
            "memo": None,
            "confirm_unbalanced": False,
            "downgrade_unselected_cleared": False,
        },
    )
    assert finish.status_code == 200, finish.text

    # Now reconcile the *target* account so it has its own lock for the
    # import-commit path to catch. Without a balancing row, the locked-range
    # check would not fire against Import Target.
    target_finish = await e2e_app.client.post(
        f"/api/v1/reconciliation/accounts/{target['id']}/finish",
        json={
            "book_id": 1,
            "as_of_date": "2026-05-31",
            "statement_start_date": None,
            "statement_balance_minor": 0,
            "selected_transaction_ids": [],
            "offset_account_id": None,
            "remember_offset_account": False,
            "memo": "baseline",
            "confirm_unbalanced": False,
            "downgrade_unselected_cleared": False,
        },
    )
    assert target_finish.status_code == 200, target_finish.text

    # Attempt to commit an import with a draft dated 2026-05-02 — inside
    # the locked window for Import Target.
    response = await e2e_app.client.post(
        "/api/v1/imports/commit",
        json={
            "book_id": 1,
            "account_id": target["id"],
            "drafts": [
                {
                    "txn_date": "2026-05-02",
                    "amount_minor": 100,
                    "payee_name": "Late",
                    "memo": None,
                    "category_name": None,
                    "import_id": None,
                    "reference": None,
                }
            ],
            "create_payees": True,
            "mode": "match_and_enrich",
            "source": None,
            "file_name": "locked.csv",
            "file_hash": None,
            "file_size": None,
            "notes": None,
        },
    )
    # 409 with the locked-range error.
    assert response.status_code == 409, response.text
    assert "cannot post transaction dated" in response.json()["detail"]

    # The post-error abandonment path must mark the in-flight ImportSession
    # row as "abandoned". This is the subtle bit the service-level fixture
    # couldn't exercise cleanly. Inspect via the engine the fixture exposes.
    async with e2e_app.sessionmaker() as sql_session:
        result = await sql_session.execute(
            select(ImportSession)
            .where(ImportSession.file_name == "locked.csv")
            .order_by(ImportSession.id.desc())
        )
        rows = list(result.scalars().all())

    assert len(rows) == 1, f"expected one ImportSession row, found {len(rows)}"
    assert rows[0].status == "abandoned"
