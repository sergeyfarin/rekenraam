from __future__ import annotations

from stage2_schema_contract import STAGE2_SCHEMA_CONTRACT, metadata_stage2_schema_contract

import rekenraam_api.db.models  # noqa: F401
from rekenraam_api.db.base import Base


def test_sqlalchemy_metadata_matches_stage2_schema_contract() -> None:
    table_names = set(Base.metadata.tables)
    planning_tables = {
        "budgets",
        "budget_targets",
        "scheduled_transactions",
        "scheduled_transaction_splits",
        "scheduled_transaction_occurrences",
        "loans",
        "loan_terms",
    }
    assert set(STAGE2_SCHEMA_CONTRACT) | planning_tables <= table_names

    actual_schema = metadata_stage2_schema_contract(Base.metadata)
    for table_name, expected in STAGE2_SCHEMA_CONTRACT.items():
        actual = actual_schema[table_name]
        assert actual.primary_key == expected.primary_key
        assert set(expected.columns) <= set(actual.columns)
        assert set(expected.indexes) <= set(actual.indexes)
        assert set(expected.foreign_keys) <= set(actual.foreign_keys)
        assert set(expected.check_constraints) <= set(actual.check_constraints)
        assert set(expected.unique_constraints) <= set(actual.unique_constraints)


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
