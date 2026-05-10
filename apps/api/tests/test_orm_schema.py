"""Smoke checks that ORM models declare the columns each milestone needs.

The fuller schema contract is asserted by
`test_alembic_head_matches_full_stage2_schema_contract` in `test_migrations.py`,
which compares `Base.metadata` to a freshly-migrated Postgres. This file
keeps a small set of column-presence assertions per milestone so that a
removed-by-accident column shows up in a fast, DB-free unit test before the
heavier migration test runs in CI.
"""

from __future__ import annotations

import rekenraam_api.db.models  # noqa: F401  -- registers all ORM tables on Base.metadata
from rekenraam_api.db.base import Base


def test_milestone7_planning_tables_are_registered() -> None:
    metadata = Base.metadata

    assert {"book_id", "period", "commodity_id"} <= set(metadata.tables["budgets"].columns.keys())
    assert {"budget_id", "category_id", "rollover_enabled"} <= set(
        metadata.tables["budget_targets"].columns.keys()
    )
    assert {"frequency", "interval", "start_date", "enabled"} <= set(
        metadata.tables["scheduled_transactions"].columns.keys()
    )
    assert {"occurrence_date", "status", "posted_transaction_id"} <= set(
        metadata.tables["scheduled_transaction_occurrences"].columns.keys()
    )
    assert {"account_id", "principal_minor", "annual_rate_bps", "term_months"} <= set(
        metadata.tables["loans"].columns.keys()
    )
