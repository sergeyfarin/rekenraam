from __future__ import annotations

from dataclasses import dataclass
from dataclasses import replace

from sqlalchemy import CheckConstraint, Index, MetaData, Table, UniqueConstraint, inspect
from sqlalchemy.dialects import postgresql
from sqlalchemy.engine import Connection


@dataclass(frozen=True, order=True)
class ColumnContract:
    name: str
    type_sql: str
    nullable: bool


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


def _normalize_type(type_sql: object, dialect: object) -> str:
    return str(type_sql.compile(dialect=dialect)).lower()


def _normalize_sql(sqltext: str | None) -> str:
    if not sqltext:
        return ""
    return " ".join(sqltext.replace('"', "").split()).lower()


def _table(
    *,
    columns: tuple[ColumnContract, ...],
    primary_key: tuple[str, ...],
    indexes: tuple[IndexContract, ...] = (),
    foreign_keys: tuple[ForeignKeyContract, ...] = (),
    check_constraints: tuple[CheckConstraintContract, ...] = (),
    unique_constraints: tuple[UniqueConstraintContract, ...] = (),
) -> TableContract:
    return TableContract(
        columns=tuple(sorted(columns)),
        primary_key=primary_key,
        indexes=tuple(sorted(indexes)),
        foreign_keys=tuple(sorted(foreign_keys)),
        check_constraints=tuple(sorted(check_constraints)),
        unique_constraints=tuple(sorted(unique_constraints)),
    )


STAGE2_SCHEMA_CONTRACT: dict[str, TableContract] = {
    "users": _table(
        columns=(
            ColumnContract("created_at", "timestamp with time zone", False),
            ColumnContract("display_name", "varchar(200)", False),
            ColumnContract("email", "varchar(320)", False),
            ColumnContract("id", "bigint", False),
            ColumnContract("is_admin", "boolean", False),
            ColumnContract("password_hash", "text", True),
            ColumnContract("updated_at", "timestamp with time zone", False),
        ),
        primary_key=("id",),
        indexes=(IndexContract("ix_users_email", ("email",), True),),
    ),
    "books": _table(
        columns=(
            ColumnContract("base_currency_code", "varchar(3)", False),
            ColumnContract("created_at", "timestamp with time zone", False),
            ColumnContract("id", "bigint", False),
            ColumnContract("name", "varchar(200)", False),
            ColumnContract("slug", "varchar(120)", False),
            ColumnContract("updated_at", "timestamp with time zone", False),
        ),
        primary_key=("id",),
        indexes=(IndexContract("ix_books_slug", ("slug",), True),),
    ),
    "commodities": _table(
        columns=(
            ColumnContract("book_id", "bigint", False),
            ColumnContract("created_at", "timestamp with time zone", False),
            ColumnContract("id", "bigint", False),
            ColumnContract("kind", "varchar(32)", False),
            ColumnContract("metadata", "text", True),
            ColumnContract("name", "varchar(200)", False),
            ColumnContract("scale", "integer", False),
            ColumnContract("symbol", "varchar(32)", True),
            ColumnContract("updated_at", "timestamp with time zone", False),
        ),
        primary_key=("id",),
        indexes=(IndexContract("ix_commodities_book_id", ("book_id",), False),),
        foreign_keys=(ForeignKeyContract(("book_id",), "books", ("id",), "CASCADE"),),
    ),
    "countries": _table(
        columns=(
            ColumnContract("book_id", "bigint", False),
            ColumnContract("code", "varchar(3)", False),
            ColumnContract("created_at", "timestamp with time zone", False),
            ColumnContract("id", "bigint", False),
            ColumnContract("name", "varchar(200)", False),
            ColumnContract("updated_at", "timestamp with time zone", False),
        ),
        primary_key=("id",),
        indexes=(IndexContract("ix_countries_book_id", ("book_id",), False),),
        foreign_keys=(ForeignKeyContract(("book_id",), "books", ("id",), "CASCADE"),),
    ),
    "institutions": _table(
        columns=(
            ColumnContract("book_id", "bigint", False),
            ColumnContract("country_id", "bigint", True),
            ColumnContract("created_at", "timestamp with time zone", False),
            ColumnContract("id", "bigint", False),
            ColumnContract("kind", "varchar(64)", True),
            ColumnContract("metadata", "text", True),
            ColumnContract("name", "varchar(200)", False),
            ColumnContract("routing", "varchar(64)", True),
            ColumnContract("updated_at", "timestamp with time zone", False),
            ColumnContract("website", "varchar(512)", True),
        ),
        primary_key=("id",),
        indexes=(IndexContract("ix_institutions_book_id", ("book_id",), False),),
        foreign_keys=(
            ForeignKeyContract(("book_id",), "books", ("id",), "CASCADE"),
            ForeignKeyContract(("country_id",), "countries", ("id",), "SET NULL"),
        ),
    ),
    "categories": _table(
        columns=(
            ColumnContract("book_id", "bigint", False),
            ColumnContract("color", "varchar(32)", True),
            ColumnContract("created_at", "timestamp with time zone", False),
            ColumnContract("id", "bigint", False),
            ColumnContract("kind", "varchar(32)", False),
            ColumnContract("name", "varchar(200)", False),
            ColumnContract("parent_id", "bigint", True),
            ColumnContract("updated_at", "timestamp with time zone", False),
        ),
        primary_key=("id",),
        indexes=(
            IndexContract("ix_categories_book_id", ("book_id",), False),
            IndexContract("ix_categories_parent_id", ("parent_id",), False),
        ),
        foreign_keys=(
            ForeignKeyContract(("book_id",), "books", ("id",), "CASCADE"),
            ForeignKeyContract(("parent_id",), "categories", ("id",), "SET NULL"),
        ),
    ),
    "payees": _table(
        columns=(
            ColumnContract("book_id", "bigint", False),
            ColumnContract("created_at", "timestamp with time zone", False),
            ColumnContract("id", "bigint", False),
            ColumnContract("kind", "varchar(32)", False),
            ColumnContract("metadata", "text", True),
            ColumnContract("name", "varchar(200)", False),
            ColumnContract("updated_at", "timestamp with time zone", False),
        ),
        primary_key=("id",),
        indexes=(IndexContract("ix_payees_book_id", ("book_id",), False),),
        foreign_keys=(ForeignKeyContract(("book_id",), "books", ("id",), "CASCADE"),),
    ),
    "tags": _table(
        columns=(
            ColumnContract("book_id", "bigint", False),
            ColumnContract("color", "varchar(32)", True),
            ColumnContract("created_at", "timestamp with time zone", False),
            ColumnContract("id", "bigint", False),
            ColumnContract("name", "varchar(200)", False),
            ColumnContract("updated_at", "timestamp with time zone", False),
        ),
        primary_key=("id",),
        indexes=(IndexContract("ix_tags_book_id", ("book_id",), False),),
        foreign_keys=(ForeignKeyContract(("book_id",), "books", ("id",), "CASCADE"),),
    ),
    "people": _table(
        columns=(
            ColumnContract("book_id", "bigint", False),
            ColumnContract("created_at", "timestamp with time zone", False),
            ColumnContract("id", "bigint", False),
            ColumnContract("metadata", "text", True),
            ColumnContract("name", "varchar(200)", False),
            ColumnContract("role", "varchar(64)", False),
            ColumnContract("updated_at", "timestamp with time zone", False),
        ),
        primary_key=("id",),
        indexes=(IndexContract("ix_people_book_id", ("book_id",), False),),
        foreign_keys=(ForeignKeyContract(("book_id",), "books", ("id",), "CASCADE"),),
    ),
    "projects": _table(
        columns=(
            ColumnContract("book_id", "bigint", False),
            ColumnContract("created_at", "timestamp with time zone", False),
            ColumnContract("id", "bigint", False),
            ColumnContract("metadata", "text", True),
            ColumnContract("name", "varchar(200)", False),
            ColumnContract("status", "varchar(64)", False),
            ColumnContract("updated_at", "timestamp with time zone", False),
        ),
        primary_key=("id",),
        indexes=(IndexContract("ix_projects_book_id", ("book_id",), False),),
        foreign_keys=(ForeignKeyContract(("book_id",), "books", ("id",), "CASCADE"),),
    ),
    "accounts": _table(
        columns=(
            ColumnContract("account_type", "varchar(32)", False),
            ColumnContract("book_id", "bigint", False),
            ColumnContract("booking_policy", "varchar(16)", False),
            ColumnContract("commodity_id", "bigint", False),
            ColumnContract("created_at", "timestamp with time zone", False),
            ColumnContract("effective_at", "date", False),
            ColumnContract("id", "bigint", False),
            ColumnContract("is_closed", "boolean", False),
            ColumnContract("is_hidden", "boolean", False),
            ColumnContract("is_system", "boolean", False),
            ColumnContract("lifecycle_event", "varchar(16)", False),
            ColumnContract("lifecycle_metadata", "text", True),
            ColumnContract("lifecycle_note", "text", True),
            ColumnContract("name", "varchar(200)", False),
            ColumnContract("number_last4", "varchar(4)", True),
            ColumnContract("parent_id", "bigint", True),
            ColumnContract("previous_account_id", "bigint", True),
            ColumnContract("system_role", "varchar(64)", True),
            ColumnContract("updated_at", "timestamp with time zone", False),
        ),
        primary_key=("id",),
        indexes=(
            IndexContract("ix_accounts_book_id", ("book_id",), False),
            IndexContract("ix_accounts_parent_id", ("parent_id",), False),
        ),
        foreign_keys=(
            ForeignKeyContract(("book_id",), "books", ("id",), "CASCADE"),
            ForeignKeyContract(("parent_id",), "accounts", ("id",), "SET NULL"),
            ForeignKeyContract(("previous_account_id",), "accounts", ("id",), "SET NULL"),
        ),
        check_constraints=(
            CheckConstraintContract(
                "ck_accounts_account_type",
                "account_type in ('cash', 'checking', 'savings', 'credit', 'loan', 'investment', 'asset', 'liability', 'income', 'expense', 'equity')",
            ),
            CheckConstraintContract(
                "ck_accounts_booking_policy",
                "booking_policy in ('fifo', 'lifo', 'strict', 'average')",
            ),
            CheckConstraintContract(
                "ck_accounts_lifecycle_event",
                "lifecycle_event in ('open', 'close', 'reopen', 'update')",
            ),
        ),
    ),
    "account_balancings": _table(
        columns=(
            ColumnContract("account_id", "bigint", False),
            ColumnContract("as_of_date", "date", False),
            ColumnContract("balance_minor", "bigint", False),
            ColumnContract("book_id", "bigint", False),
            ColumnContract("created_at", "timestamp with time zone", False),
            ColumnContract("id", "bigint", False),
            ColumnContract("memo", "text", True),
            ColumnContract("previous_account_balancing_id", "bigint", True),
            ColumnContract("void_reason", "text", True),
            ColumnContract("voided_at", "timestamp with time zone", True),
        ),
        primary_key=("id",),
        indexes=(
            IndexContract("ix_account_balancings_account_date", ("account_id", "as_of_date"), False),
            IndexContract("ix_account_balancings_book_id", ("book_id",), False),
        ),
        foreign_keys=(
            ForeignKeyContract(("account_id",), "accounts", ("id",), "CASCADE"),
            ForeignKeyContract(("book_id",), "books", ("id",), "CASCADE"),
            ForeignKeyContract(
                ("previous_account_balancing_id",),
                "account_balancings",
                ("id",),
                "SET NULL",
            ),
        ),
    ),
    "transactions": _table(
        columns=(
            ColumnContract("book_id", "bigint", False),
            ColumnContract("created_at", "timestamp with time zone", False),
            ColumnContract("id", "bigint", False),
            ColumnContract("memo", "text", True),
            ColumnContract("occurred_date", "date", False),
            ColumnContract("payee_id", "bigint", True),
            ColumnContract("posted_date", "date", False),
            ColumnContract("reference", "text", True),
            ColumnContract("status", "varchar(20)", False),
        ),
        primary_key=("id",),
        indexes=(IndexContract("ix_transactions_book_occurred_date", ("book_id", "occurred_date"), False),),
        foreign_keys=(
            ForeignKeyContract(("book_id",), "books", ("id",), "CASCADE"),
            ForeignKeyContract(("payee_id",), "payees", ("id",), "SET NULL"),
        ),
        check_constraints=(
            CheckConstraintContract(
                "ck_transactions_status",
                "status in ('uncleared', 'cleared', 'reconciled', 'void')",
            ),
        ),
    ),
    "splits": _table(
        columns=(
            ColumnContract("account_id", "bigint", False),
            ColumnContract("amount_minor", "bigint", False),
            ColumnContract("category_id", "bigint", True),
            ColumnContract("commodity_id", "bigint", False),
            ColumnContract("created_at", "timestamp with time zone", False),
            ColumnContract("id", "bigint", False),
            ColumnContract("memo", "text", True),
            ColumnContract("person_id", "bigint", True),
            ColumnContract("project_id", "bigint", True),
            ColumnContract("share_bps", "bigint", True),
            ColumnContract("tag_id", "bigint", True),
            ColumnContract("tx_id", "bigint", False),
        ),
        primary_key=("id",),
        indexes=(
            IndexContract("ix_splits_account_id", ("account_id",), False),
            IndexContract("ix_splits_tx_id", ("tx_id",), False),
        ),
        foreign_keys=(
            ForeignKeyContract(("account_id",), "accounts", ("id",), "RESTRICT"),
            ForeignKeyContract(("category_id",), "categories", ("id",), "SET NULL"),
            ForeignKeyContract(("commodity_id",), "commodities", ("id",), "RESTRICT"),
            ForeignKeyContract(("person_id",), "people", ("id",), "SET NULL"),
            ForeignKeyContract(("project_id",), "projects", ("id",), "SET NULL"),
            ForeignKeyContract(("tag_id",), "tags", ("id",), "SET NULL"),
            ForeignKeyContract(("tx_id",), "transactions", ("id",), "CASCADE"),
        ),
    ),
    "book_memberships": _table(
        columns=(
            ColumnContract("book_id", "bigint", False),
            ColumnContract("created_at", "timestamp with time zone", False),
            ColumnContract("id", "bigint", False),
            ColumnContract("role", "varchar(20)", False),
            ColumnContract("user_id", "bigint", False),
        ),
        primary_key=("id",),
        foreign_keys=(
            ForeignKeyContract(("book_id",), "books", ("id",), "CASCADE"),
            ForeignKeyContract(("user_id",), "users", ("id",), "CASCADE"),
        ),
        check_constraints=(
            CheckConstraintContract(
                "ck_book_memberships_role",
                "role in ('owner', 'editor', 'viewer')",
            ),
        ),
        unique_constraints=(
            UniqueConstraintContract("uq_book_memberships_user_book", ("user_id", "book_id")),
        ),
    ),
}

TAURI_RUNTIME_TABLES = (
    "app_runtime_session",
    "session_undo_stack",
    "session_redo_stack",
    "session_reverts",
)


def database_stage2_schema_contract(connection: Connection) -> dict[str, TableContract]:
    inspector = inspect(connection)
    schema: dict[str, TableContract] = {}

    for table_name in sorted(STAGE2_SCHEMA_CONTRACT):
        columns = tuple(
            sorted(
                ColumnContract(
                    name=column["name"],
                    type_sql=_normalize_type(column["type"], connection.dialect),
                    nullable=bool(column["nullable"]),
                )
                for column in inspector.get_columns(table_name)
            )
        )
        primary_key = tuple(inspector.get_pk_constraint(table_name).get("constrained_columns") or ())
        unique_constraints = tuple(
            sorted(
                UniqueConstraintContract(
                    name=str(unique["name"]),
                    columns=tuple(unique.get("column_names") or ()),
                )
                for unique in inspector.get_unique_constraints(table_name)
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
                for index in inspector.get_indexes(table_name)
                if not index.get("duplicates_constraint") and index["name"] not in unique_constraint_names
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
                for foreign_key in inspector.get_foreign_keys(table_name)
            )
        )
        check_constraints = tuple(
            sorted(
                CheckConstraintContract(
                    name=str(check["name"]),
                    sqltext=_normalize_sql(check.get("sqltext")),
                )
                for check in inspector.get_check_constraints(table_name)
            )
        )
        schema[table_name] = TableContract(
            columns=columns,
            primary_key=primary_key,
            indexes=indexes,
            foreign_keys=foreign_keys,
            check_constraints=check_constraints,
            unique_constraints=unique_constraints,
        )

    return schema


def metadata_stage2_schema_contract(metadata: MetaData) -> dict[str, TableContract]:
    dialect = postgresql.dialect()
    schema: dict[str, TableContract] = {}

    for table_name in sorted(STAGE2_SCHEMA_CONTRACT):
        table = metadata.tables[table_name]
        schema[table_name] = _table_contract_from_metadata(table, dialect)

    return schema


def without_check_sqltext(schema: dict[str, TableContract]) -> dict[str, TableContract]:
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


def _table_contract_from_metadata(table: Table, dialect: object) -> TableContract:
    columns = tuple(
        sorted(
            ColumnContract(
                name=column.name,
                type_sql=_normalize_type(column.type, dialect),
                nullable=bool(column.nullable),
            )
            for column in table.columns
        )
    )
    primary_key = tuple(column.name for column in table.primary_key.columns)
    indexes = tuple(
        sorted(
            IndexContract(
                name=index.name,
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
                sqltext=_normalize_sql(str(constraint.sqltext)),
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