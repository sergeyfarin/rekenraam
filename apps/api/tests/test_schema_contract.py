"""Self-tests for `stage2_schema_contract`.

These verify the drift detector itself works:

- A round-trip across `contract_from_metadata` produces a stable shape.
- Modifying a `MetaData` (add a column, change `nullable`, add an index, change
  a `server_default`) shows up as a difference.
- Server-default normalization handles the four real shapes that appear in
  the codebase: raw string literal, `sa.text("…")`, `func.now()`, and
  autoincrement sequence.

Without this, the migration drift test could silently degrade (e.g. a
normalization regression that masked all defaults) and we'd never know.
"""

from __future__ import annotations

import pytest
from sqlalchemy import (
    BigInteger,
    Boolean,
    Column,
    DateTime,
    Integer,
    MetaData,
    String,
    Table,
    func,
    text,
)
from stage2_schema_contract import (
    ColumnContract,
    contract_from_metadata,
    normalize_for_comparison,
)


def _build_metadata(server_default: object | None = None, nullable: bool = False) -> MetaData:
    metadata = MetaData()
    Table(
        "demo",
        metadata,
        Column("id", BigInteger, primary_key=True, autoincrement=True),
        Column("name", String(64), nullable=False),
        Column("flag", Boolean, nullable=nullable, server_default=server_default),
        Column("created_at", DateTime(timezone=True), server_default=func.now(), nullable=False),
    )
    return metadata


def test_contract_round_trips_for_a_minimal_metadata() -> None:
    metadata = _build_metadata(server_default=text("false"))
    contract = contract_from_metadata(metadata)

    assert "demo" in contract
    table = contract["demo"]
    assert table.primary_key == ("id",)

    columns_by_name = {column.name: column for column in table.columns}
    assert columns_by_name["id"].type_sql == "bigint"
    # Autoincrement PK sequence default is normalized away — ORM doesn't model
    # the implicit sequence and Postgres reports `nextval(...)` only when
    # actually reflected, so we suppress on both sides.
    assert columns_by_name["id"].server_default == ""
    assert columns_by_name["flag"].server_default == "false"
    assert columns_by_name["created_at"].server_default == "now()"


def test_drift_detected_when_column_added() -> None:
    base = contract_from_metadata(_build_metadata())
    drifted_metadata = _build_metadata()
    drifted_metadata.tables["demo"].append_column(Column("extra", Integer, nullable=True))
    drifted = contract_from_metadata(drifted_metadata)
    assert normalize_for_comparison(base) != normalize_for_comparison(drifted)


def test_drift_detected_when_nullable_flips() -> None:
    base = contract_from_metadata(_build_metadata(nullable=False))
    drifted = contract_from_metadata(_build_metadata(nullable=True))
    assert normalize_for_comparison(base) != normalize_for_comparison(drifted)


def test_drift_detected_when_server_default_flips() -> None:
    """The exact failure mode the rebuild was meant to catch."""

    base = contract_from_metadata(_build_metadata(server_default=text("false")))
    drifted = contract_from_metadata(_build_metadata(server_default=text("true")))
    assert normalize_for_comparison(base) != normalize_for_comparison(drifted)


def test_drift_detected_when_index_added() -> None:
    base_metadata = _build_metadata()
    drifted_metadata = _build_metadata()
    Table("demo", drifted_metadata, autoload_with=None, extend_existing=True)
    # Manually add an index to the drifted variant via core API.
    from sqlalchemy import Index

    Index("ix_demo_name", drifted_metadata.tables["demo"].c.name)

    base = contract_from_metadata(base_metadata)
    drifted = contract_from_metadata(drifted_metadata)
    assert normalize_for_comparison(base) != normalize_for_comparison(drifted)


@pytest.mark.parametrize(
    ("server_default", "expected"),
    [
        # Raw string -- SQLAlchemy auto-quotes this in DDL, so ORM reads as
        # 'value'. The drift test treats this as the canonical literal form.
        ("hello", "'hello'"),
        # sa.text(...) -- the migration-style form with explicit quoting.
        (text("'fifo'"), "'fifo'"),
        # sa.text("0") -- numeric default; remains unquoted.
        (text("0"), "0"),
        # sa.text("true") -- boolean default; remains unquoted.
        (text("true"), "true"),
        # func.now() -- expression default.
        (func.now(), "now()"),
        # No default at all.
        (None, ""),
    ],
)
def test_server_default_normalization(server_default: object | None, expected: str) -> None:
    metadata = MetaData()
    Table(
        "demo",
        metadata,
        Column("id", BigInteger, primary_key=True, autoincrement=True),
        Column("value", String(64), server_default=server_default),
    )
    contract = contract_from_metadata(metadata)
    columns_by_name = {column.name: column for column in contract["demo"].columns}
    assert columns_by_name["value"].server_default == expected


def test_column_contract_is_hashable_and_orderable() -> None:
    """ColumnContract is used in `tuple(sorted(...))`; it must be both
    comparable and hashable for the dataclass-frozen contract types to
    survive set operations in error reporting."""

    a = ColumnContract(name="x", type_sql="bigint", nullable=False, server_default="")
    b = ColumnContract(name="y", type_sql="bigint", nullable=False, server_default="")
    assert a < b
    assert hash(a) != hash(b)
    assert {a, b}
