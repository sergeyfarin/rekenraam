"""Schema contract used by `test_alembic_head_matches_full_stage2_schema_contract`.

Two callers can produce a `dict[str, TableContract]`:

- `contract_from_metadata(Base.metadata)` — derived from the SQLAlchemy ORM
  models. This is the canonical specification of what the schema *should* be.
- `contract_from_database(connection)` — derived by reflecting a live
  Postgres database that has been migrated to head.

The schema-drift test compares the two. Any difference is real drift and
must be resolved by either a migration or an ORM change. There is no
hand-maintained dictionary to keep in sync.

`TAURI_RUNTIME_TABLES` are tables the legacy desktop app used that are
intentionally not present in the Postgres baseline. The drift test asserts
they don't reappear by accident.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, replace

from sqlalchemy import CheckConstraint, MetaData, Table, UniqueConstraint, inspect
from sqlalchemy.dialects import postgresql
from sqlalchemy.engine import Connection, Dialect


@dataclass(frozen=True, order=True)
class ColumnContract:
    name: str
    type_sql: str
    nullable: bool
    server_default: str


@dataclass(frozen=True, order=True)
class IndexContract:
    name: str
    columns: tuple[str, ...]
    unique: bool


@dataclass(frozen=True, order=True)
class ForeignKeyContract:
    columns: tuple[str, ...]
    referred_table: str
    referred_columns: tuple[str, ...]
    ondelete: str | None


@dataclass(frozen=True, order=True)
class CheckConstraintContract:
    name: str
    sqltext: str


@dataclass(frozen=True, order=True)
class UniqueConstraintContract:
    name: str
    columns: tuple[str, ...]


@dataclass(frozen=True)
class TableContract:
    columns: tuple[ColumnContract, ...]
    primary_key: tuple[str, ...]
    indexes: tuple[IndexContract, ...]
    foreign_keys: tuple[ForeignKeyContract, ...]
    check_constraints: tuple[CheckConstraintContract, ...]
    unique_constraints: tuple[UniqueConstraintContract, ...]


# Tables the legacy Tauri/SQLite runtime used. These must not be created by
# any migration in the Postgres baseline. Asserted by the schema-drift test.
TAURI_RUNTIME_TABLES = (
    "app_runtime_session",
    "session_undo_stack",
    "session_redo_stack",
    "session_reverts",
)


# ---------- Normalization helpers ----------

_SEQUENCE_DEFAULT = re.compile(r"^nextval\(.*_seq.*\)$", re.IGNORECASE)


def _normalize_type(type_sql: object, dialect: Dialect) -> str:
    """Render a SQLAlchemy type to a stable lowercase SQL string."""

    return str(type_sql.compile(dialect=dialect)).lower()  # type: ignore[attr-defined]


def _normalize_check_sql(sqltext: str | None) -> str:
    """Reduce a CHECK constraint's SQL text to a comparable form.

    Used only for human-readable diff output — the test asserts on names plus
    `sqltext != ""`, not on textual equality, because Postgres reflection and
    SQLAlchemy's render emit slightly different formatting.
    """

    if not sqltext:
        return ""
    return " ".join(sqltext.replace('"', "").split()).lower()


def _normalize_orm_server_default(default: object | None, dialect: Dialect) -> str:
    """Render an ORM `Column.server_default` to a comparable SQL fragment.

    `Column.server_default` is a `DefaultClause` whose `.arg` is either:

    - a raw `str` — SQLAlchemy auto-quotes this as a SQL literal at DDL time,
      so we mirror that and emit `'value'`.
    - a SQL element such as `func.now()` or `sa.text("'fifo'")` — render via
      the Postgres dialect to match the form Postgres reports back through
      reflection.
    """

    if default is None:
        return ""
    arg = getattr(default, "arg", default)
    if isinstance(arg, str):
        rendered = f"'{arg}'"
    elif hasattr(arg, "compile"):
        rendered = str(arg.compile(dialect=dialect))
    else:
        rendered = str(arg)
    return _strip_default_artifacts(rendered)


def _normalize_db_server_default(default: str | None) -> str:
    """Reduce a Postgres-reflected default fragment to its comparable form.

    Reflection returns the SQL that appears after `DEFAULT` in `pg_attrdef`
    (e.g. ``'fifo'``, ``false``, ``now()``, ``nextval('foo_id_seq'::regclass)``).
    Drop autoincrement sequence defaults — the ORM doesn't model them
    explicitly — and strip Postgres' `::type` casts.
    """

    if default is None:
        return ""
    return _strip_default_artifacts(default)


def _strip_default_artifacts(rendered: str) -> str:
    rendered = rendered.strip().lower()
    if _SEQUENCE_DEFAULT.match(rendered):
        return ""
    rendered = re.sub(r"::[a-z][a-z _\d]*", "", rendered).strip()
    return rendered


# ---------- Contract builders ----------


def contract_from_metadata(
    metadata: MetaData, dialect: Dialect | None = None
) -> dict[str, TableContract]:
    """Derive the schema contract from SQLAlchemy ORM models.

    Returns one `TableContract` per `metadata.tables` entry. Skips the
    Alembic version table because it is operational, not part of the domain
    contract.
    """

    dialect = dialect or postgresql.dialect()
    return {
        table_name: _table_contract_from_metadata(table, dialect)
        for table_name, table in metadata.tables.items()
        if table_name != "alembic_version"
    }


def contract_from_database(connection: Connection) -> dict[str, TableContract]:
    """Reflect the schema contract from a live Postgres connection.

    Returns one `TableContract` per user table (excluding `alembic_version`).
    Tables that exist only in the database, but not in `Base.metadata`, will
    surface here and trigger the drift test.
    """

    inspector = inspect(connection)
    return {
        table_name: _table_contract_from_database(inspector, connection.dialect, table_name)
        for table_name in sorted(inspector.get_table_names())
        if table_name != "alembic_version"
    }


def normalize_for_comparison(
    schema: dict[str, TableContract],
) -> dict[str, TableContract]:
    """Strip fields that legitimately differ between ORM and Postgres reflection.

    Specifically:
    - CHECK constraint `sqltext` is replaced with the empty string. Postgres
      stores constraint expressions in canonical form (e.g. parens,
      requalified column refs) that SQLAlchemy's `str(constraint.sqltext)`
      doesn't reproduce. The constraint *names* are still compared, and the
      drift test separately asserts that every CHECK has a non-empty
      `sqltext` on the database side.

    Apply this to *both* sides before equality-checking.
    """

    return {
        table_name: replace(
            contract,
            check_constraints=tuple(
                sorted(
                    CheckConstraintContract(name=check.name, sqltext="")
                    for check in contract.check_constraints
                )
            ),
        )
        for table_name, contract in schema.items()
    }


# ---------- Internals ----------


def _table_contract_from_metadata(table: Table, dialect: Dialect) -> TableContract:
    columns = tuple(
        sorted(
            ColumnContract(
                name=column.name,
                type_sql=_normalize_type(column.type, dialect),
                nullable=bool(column.nullable),
                server_default=_normalize_orm_server_default(column.server_default, dialect),
            )
            for column in table.columns
        )
    )
    primary_key = tuple(column.name for column in table.primary_key.columns)
    indexes = tuple(
        sorted(
            IndexContract(
                name=index.name or "",
                columns=tuple(column.name for column in index.columns),
                unique=bool(index.unique),
            )
            for index in table.indexes
        )
    )
    foreign_keys = tuple(
        sorted(
            ForeignKeyContract(
                columns=tuple(element.parent.name for element in constraint.elements),
                referred_table=next(iter(constraint.elements)).column.table.name,
                referred_columns=tuple(element.column.name for element in constraint.elements),
                ondelete=next(iter(constraint.elements)).ondelete,
            )
            for constraint in table.foreign_key_constraints
        )
    )
    check_constraints = tuple(
        sorted(
            CheckConstraintContract(
                name=str(constraint.name),
                sqltext=_normalize_check_sql(str(constraint.sqltext)),
            )
            for constraint in table.constraints
            if isinstance(constraint, CheckConstraint)
        )
    )
    unique_constraints = tuple(
        sorted(
            UniqueConstraintContract(
                name=str(constraint.name),
                columns=tuple(column.name for column in constraint.columns),
            )
            for constraint in table.constraints
            if isinstance(constraint, UniqueConstraint)
        )
    )
    return TableContract(
        columns=columns,
        primary_key=primary_key,
        indexes=indexes,
        foreign_keys=foreign_keys,
        check_constraints=check_constraints,
        unique_constraints=unique_constraints,
    )


def _table_contract_from_database(
    inspector: object, dialect: Dialect, table_name: str
) -> TableContract:
    columns = tuple(
        sorted(
            ColumnContract(
                name=column["name"],
                type_sql=_normalize_type(column["type"], dialect),
                nullable=bool(column["nullable"]),
                server_default=_normalize_db_server_default(column.get("default")),
            )
            for column in inspector.get_columns(table_name)  # type: ignore[attr-defined]
        )
    )
    primary_key = tuple(
        inspector.get_pk_constraint(table_name).get("constrained_columns") or ()  # type: ignore[attr-defined]
    )
    unique_constraints = tuple(
        sorted(
            UniqueConstraintContract(
                name=str(unique["name"]),
                columns=tuple(unique.get("column_names") or ()),
            )
            for unique in inspector.get_unique_constraints(table_name)  # type: ignore[attr-defined]
        )
    )
    unique_constraint_names = {unique.name for unique in unique_constraints}
    indexes = tuple(
        sorted(
            IndexContract(
                name=index["name"],
                columns=tuple(index.get("column_names") or ()),
                unique=bool(index.get("unique")),
            )
            for index in inspector.get_indexes(table_name)  # type: ignore[attr-defined]
            if not index.get("duplicates_constraint")
            and index["name"] not in unique_constraint_names
        )
    )
    foreign_keys = tuple(
        sorted(
            ForeignKeyContract(
                columns=tuple(foreign_key.get("constrained_columns") or ()),
                referred_table=str(foreign_key["referred_table"]),
                referred_columns=tuple(foreign_key.get("referred_columns") or ()),
                ondelete=(foreign_key.get("options") or {}).get("ondelete"),
            )
            for foreign_key in inspector.get_foreign_keys(table_name)  # type: ignore[attr-defined]
        )
    )
    check_constraints = tuple(
        sorted(
            CheckConstraintContract(
                name=str(check["name"]),
                sqltext=_normalize_check_sql(check.get("sqltext")),
            )
            for check in inspector.get_check_constraints(table_name)  # type: ignore[attr-defined]
        )
    )
    return TableContract(
        columns=columns,
        primary_key=primary_key,
        indexes=indexes,
        foreign_keys=foreign_keys,
        check_constraints=check_constraints,
        unique_constraints=unique_constraints,
    )
