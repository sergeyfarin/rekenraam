from __future__ import annotations

import rekenraam_api.db.models  # noqa: F401
from rekenraam_api.db.base import Base

from stage2_schema_contract import STAGE2_SCHEMA_CONTRACT, metadata_stage2_schema_contract


def test_sqlalchemy_metadata_matches_stage2_schema_contract() -> None:
    table_names = set(Base.metadata.tables)
    assert table_names == set(STAGE2_SCHEMA_CONTRACT)

    actual_schema = metadata_stage2_schema_contract(Base.metadata)
    assert actual_schema == STAGE2_SCHEMA_CONTRACT