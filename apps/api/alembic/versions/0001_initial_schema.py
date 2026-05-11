"""initial schema

Revision ID: 0001_initial_schema
Revises: None
Create Date: 2026-05-03 00:00:00
"""

from __future__ import annotations

import sqlalchemy as sa

from alembic import op

revision = "0001_initial_schema"
down_revision = None
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.create_table(
        "users",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("email", sa.String(length=320), nullable=False),
        sa.Column("password_hash", sa.Text(), nullable=True),
        sa.Column("display_name", sa.String(length=200), nullable=False),
        sa.Column("is_admin", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column("is_active", sa.Boolean(), nullable=False, server_default=sa.text("true")),
        sa.Column("mfa_required", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
    )
    op.create_index("ix_users_email", "users", ["email"], unique=True)

    op.create_table(
        "user_devices",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("device_fingerprint", sa.String(length=128), nullable=False),
        sa.Column("label", sa.String(length=200), nullable=True),
        sa.Column("user_agent", sa.Text(), nullable=True),
        sa.Column(
            "first_seen_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "last_seen_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.UniqueConstraint(
            "user_id", "device_fingerprint", name="uq_user_devices_user_fingerprint"
        ),
    )
    op.create_index("ix_user_devices_user_id", "user_devices", ["user_id"], unique=False)

    op.create_table(
        "auth_sessions",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "device_id",
            sa.BigInteger(),
            sa.ForeignKey("user_devices.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("token_hash", sa.String(length=128), nullable=False),
        sa.Column("user_agent", sa.Text(), nullable=True),
        sa.Column("ip_address", sa.String(length=64), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "last_seen_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column("expires_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("revoked_at", sa.DateTime(timezone=True), nullable=True),
    )
    op.create_index("ix_auth_sessions_user_id", "auth_sessions", ["user_id"], unique=False)
    op.create_index("ix_auth_sessions_token_hash", "auth_sessions", ["token_hash"], unique=True)
    op.create_index("ix_auth_sessions_expires_at", "auth_sessions", ["expires_at"], unique=False)

    op.create_table(
        "books",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("slug", sa.String(length=120), nullable=False),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("base_currency_code", sa.String(length=3), nullable=False),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
    )
    op.create_index("ix_books_slug", "books", ["slug"], unique=True)

    op.create_table(
        "book_state",
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            primary_key=True,
        ),
        sa.Column("change_seq", sa.Integer(), nullable=False, server_default=sa.text("0")),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
    )

    op.create_table(
        "report_cache",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("report_type", sa.String(length=100), nullable=False),
        sa.Column("params_hash", sa.String(length=128), nullable=False),
        sa.Column("params_json", sa.Text(), nullable=True),
        sa.Column("as_of_seq", sa.Integer(), nullable=False),
        sa.Column("payload_json", sa.Text(), nullable=False),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.UniqueConstraint(
            "book_id", "report_type", "params_hash", "as_of_seq", name="uq_report_cache_entry"
        ),
    )
    op.create_index(
        "ix_report_cache_book_type", "report_cache", ["book_id", "report_type"], unique=False
    )
    op.create_index(
        "ix_report_cache_book_seq", "report_cache", ["book_id", "as_of_seq"], unique=False
    )

    op.create_table(
        "report_definitions",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "previous_report_definition_id",
            sa.BigInteger(),
            sa.ForeignKey("report_definitions.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("session_id", sa.String(length=255), nullable=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("kind", sa.String(length=20), nullable=False, server_default=sa.text("'custom'")),
        sa.Column(
            "query_type", sa.String(length=20), nullable=False, server_default=sa.text("'sql'")
        ),
        sa.Column("query_text", sa.Text(), nullable=False),
        sa.Column("params_schema", sa.Text(), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "created_by_user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_session_id",
            sa.BigInteger(),
            sa.ForeignKey("auth_sessions.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_device_id",
            sa.BigInteger(),
            sa.ForeignKey("user_devices.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("created_request_id", sa.String(length=64), nullable=True),
        sa.CheckConstraint("kind IN ('builtin', 'custom')", name="ck_report_definitions_kind"),
        sa.CheckConstraint(
            "query_type IN ('sql', 'template')", name="ck_report_definitions_query_type"
        ),
        sa.UniqueConstraint("previous_report_definition_id", name="uq_report_definitions_previous"),
    )
    op.create_index("ix_report_definitions_book", "report_definitions", ["book_id"], unique=False)
    op.create_index(
        "ix_report_definitions_kind", "report_definitions", ["book_id", "kind"], unique=False
    )
    op.create_index(
        "ix_report_definitions_previous",
        "report_definitions",
        ["previous_report_definition_id"],
        unique=False,
    )

    op.create_table(
        "commodities",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("kind", sa.String(length=32), nullable=False),
        sa.Column("symbol", sa.String(length=32), nullable=True),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("scale", sa.Integer(), nullable=False),
        sa.Column("metadata", sa.Text(), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.CheckConstraint("scale >= 0 AND scale <= 12", name="ck_commodities_scale_range"),
    )
    op.create_index("ix_commodities_book_id", "commodities", ["book_id"], unique=False)

    op.create_table(
        "countries",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("code", sa.String(length=3), nullable=False),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
    )
    op.create_index("ix_countries_book_id", "countries", ["book_id"], unique=False)

    op.create_table(
        "institutions",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("kind", sa.String(length=64), nullable=True),
        sa.Column("routing", sa.String(length=64), nullable=True),
        sa.Column("website", sa.String(length=512), nullable=True),
        sa.Column("metadata", sa.Text(), nullable=True),
        sa.Column(
            "country_id",
            sa.BigInteger(),
            sa.ForeignKey("countries.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
    )
    op.create_index("ix_institutions_book_id", "institutions", ["book_id"], unique=False)

    op.create_table(
        "categories",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "parent_id",
            sa.BigInteger(),
            sa.ForeignKey("categories.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("kind", sa.String(length=32), nullable=False),
        sa.Column("color", sa.String(length=32), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
    )
    op.create_index("ix_categories_book_id", "categories", ["book_id"], unique=False)
    op.create_index("ix_categories_parent_id", "categories", ["parent_id"], unique=False)

    op.create_table(
        "payees",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("kind", sa.String(length=32), nullable=False),
        sa.Column("metadata", sa.Text(), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
    )
    op.create_index("ix_payees_book_id", "payees", ["book_id"], unique=False)

    op.create_table(
        "tags",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("color", sa.String(length=32), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
    )
    op.create_index("ix_tags_book_id", "tags", ["book_id"], unique=False)

    op.create_table(
        "people",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("role", sa.String(length=64), nullable=False),
        sa.Column("metadata", sa.Text(), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
    )
    op.create_index("ix_people_book_id", "people", ["book_id"], unique=False)

    op.create_table(
        "projects",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("status", sa.String(length=64), nullable=False),
        sa.Column("metadata", sa.Text(), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
    )
    op.create_index("ix_projects_book_id", "projects", ["book_id"], unique=False)

    op.create_table(
        "accounts",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "parent_id",
            sa.BigInteger(),
            sa.ForeignKey("accounts.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "previous_account_id",
            sa.BigInteger(),
            sa.ForeignKey("accounts.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("account_type", sa.String(length=32), nullable=False),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column(
            "commodity_id",
            sa.BigInteger(),
            sa.ForeignKey("commodities.id", ondelete="RESTRICT"),
            nullable=False,
        ),
        sa.Column(
            "booking_policy", sa.String(length=16), nullable=False, server_default=sa.text("'fifo'")
        ),
        sa.Column("number_last4", sa.String(length=4), nullable=True),
        sa.Column("is_closed", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column("is_hidden", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column("is_system", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column("system_role", sa.String(length=64), nullable=True),
        sa.Column(
            "effective_at", sa.Date(), nullable=False, server_default=sa.text("CURRENT_DATE")
        ),
        sa.Column(
            "lifecycle_event",
            sa.String(length=16),
            nullable=False,
            server_default=sa.text("'open'"),
        ),
        sa.Column("lifecycle_note", sa.Text(), nullable=True),
        sa.Column("lifecycle_metadata", sa.Text(), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.CheckConstraint(
            "account_type IN ('cash', 'checking', 'savings', 'credit', 'loan', 'investment', 'asset', 'liability', 'income', 'expense', 'equity')",
            name="ck_accounts_account_type",
        ),
        sa.CheckConstraint(
            "booking_policy IN ('fifo', 'lifo', 'strict', 'average')",
            name="ck_accounts_booking_policy",
        ),
        sa.CheckConstraint(
            "lifecycle_event IN ('open', 'close', 'reopen', 'update')",
            name="ck_accounts_lifecycle_event",
        ),
        sa.CheckConstraint(
            "(system_role IS NULL) OR is_system", name="ck_accounts_system_role_requires_system"
        ),
        sa.UniqueConstraint("previous_account_id", name="uq_accounts_previous_account_id"),
    )
    op.create_index("ix_accounts_book_id", "accounts", ["book_id"], unique=False)
    op.create_index("ix_accounts_parent_id", "accounts", ["parent_id"], unique=False)
    op.create_index(
        "ix_accounts_previous_account_id", "accounts", ["previous_account_id"], unique=False
    )

    op.create_table(
        "account_balancings",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "account_id",
            sa.BigInteger(),
            sa.ForeignKey("accounts.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "previous_account_balancing_id",
            sa.BigInteger(),
            sa.ForeignKey("account_balancings.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("as_of_date", sa.Date(), nullable=False),
        sa.Column("balance_minor", sa.BigInteger(), nullable=False),
        sa.Column("memo", sa.Text(), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "created_by_user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_session_id",
            sa.BigInteger(),
            sa.ForeignKey("auth_sessions.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_device_id",
            sa.BigInteger(),
            sa.ForeignKey("user_devices.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("created_request_id", sa.String(length=64), nullable=True),
        sa.Column("voided_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("void_reason", sa.Text(), nullable=True),
    )
    op.create_index(
        "ix_account_balancings_book_id", "account_balancings", ["book_id"], unique=False
    )
    op.create_index(
        "ix_account_balancings_account_date",
        "account_balancings",
        ["account_id", "as_of_date"],
        unique=False,
    )

    op.create_table(
        "balance_checks",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "account_id",
            sa.BigInteger(),
            sa.ForeignKey("accounts.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("as_of_date", sa.Date(), nullable=False),
        sa.Column("balance_minor", sa.BigInteger(), nullable=False),
        sa.Column("memo", sa.Text(), nullable=True),
        sa.Column(
            "status", sa.String(length=20), nullable=False, server_default=sa.text("'recorded'")
        ),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "created_by_user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_session_id",
            sa.BigInteger(),
            sa.ForeignKey("auth_sessions.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_device_id",
            sa.BigInteger(),
            sa.ForeignKey("user_devices.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("created_request_id", sa.String(length=64), nullable=True),
        sa.CheckConstraint(
            "status IN ('recorded', 'matched', 'failed', 'unbalanced', 'balanced')",
            name="ck_balance_checks_status",
        ),
    )
    op.create_index(
        "ix_balance_checks_account_date",
        "balance_checks",
        ["account_id", "as_of_date"],
        unique=False,
    )

    op.create_table(
        "balance_constraints",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "account_id",
            sa.BigInteger(),
            sa.ForeignKey("accounts.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("min_balance_minor", sa.BigInteger(), nullable=True),
        sa.Column("max_balance_minor", sa.BigInteger(), nullable=True),
        sa.Column(
            "sign_rule", sa.String(length=20), nullable=False, server_default=sa.text("'any'")
        ),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.CheckConstraint(
            "sign_rule IN ('any', 'nonnegative', 'nonpositive')",
            name="ck_balance_constraints_sign_rule",
        ),
    )
    op.create_index(
        "ix_balance_constraints_account", "balance_constraints", ["account_id"], unique=False
    )

    op.create_table(
        "reconciliation_preferences",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "account_id",
            sa.BigInteger(),
            sa.ForeignKey("accounts.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "offset_account_id",
            sa.BigInteger(),
            sa.ForeignKey("accounts.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.UniqueConstraint(
            "user_id", "account_id", name="uq_reconciliation_preferences_user_account"
        ),
    )
    op.create_index(
        "ix_reconciliation_preferences_account",
        "reconciliation_preferences",
        ["account_id"],
        unique=False,
    )

    op.create_table(
        "import_sessions",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("source", sa.String(length=120), nullable=True),
        sa.Column("file_name", sa.Text(), nullable=True),
        sa.Column("file_hash", sa.String(length=128), nullable=True),
        sa.Column("file_size", sa.BigInteger(), nullable=True),
        sa.Column(
            "status", sa.String(length=20), nullable=False, server_default=sa.text("'started'")
        ),
        sa.Column(
            "started_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column("committed_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("notes", sa.Text(), nullable=True),
        sa.Column(
            "created_by_user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_session_id",
            sa.BigInteger(),
            sa.ForeignKey("auth_sessions.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_device_id",
            sa.BigInteger(),
            sa.ForeignKey("user_devices.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("created_request_id", sa.String(length=64), nullable=True),
        sa.CheckConstraint(
            "status IN ('started', 'committed', 'abandoned')", name="ck_import_sessions_status"
        ),
    )
    op.create_index(
        "ix_import_sessions_book_status", "import_sessions", ["book_id", "status"], unique=False
    )
    op.create_index("ix_import_sessions_file_hash", "import_sessions", ["file_hash"], unique=False)

    op.create_table(
        "import_rules",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "previous_import_rule_id",
            sa.BigInteger(),
            sa.ForeignKey("import_rules.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("rule_kind", sa.String(length=20), nullable=False),
        sa.Column(
            "match_type", sa.String(length=20), nullable=False, server_default=sa.text("'contains'")
        ),
        sa.Column("match_text", sa.Text(), nullable=False),
        sa.Column("priority", sa.BigInteger(), nullable=False, server_default=sa.text("100")),
        sa.Column("amount_min_minor", sa.BigInteger(), nullable=True),
        sa.Column("amount_max_minor", sa.BigInteger(), nullable=True),
        sa.Column("date_from", sa.Date(), nullable=True),
        sa.Column("date_to", sa.Date(), nullable=True),
        sa.Column(
            "match_account_id",
            sa.BigInteger(),
            sa.ForeignKey("accounts.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "target_account_id",
            sa.BigInteger(),
            sa.ForeignKey("accounts.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "target_category_id",
            sa.BigInteger(),
            sa.ForeignKey("categories.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "target_payee_id",
            sa.BigInteger(),
            sa.ForeignKey("payees.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("deleted_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "created_by_user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_session_id",
            sa.BigInteger(),
            sa.ForeignKey("auth_sessions.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_device_id",
            sa.BigInteger(),
            sa.ForeignKey("user_devices.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("created_request_id", sa.String(length=64), nullable=True),
        sa.CheckConstraint(
            "rule_kind IN ('payee', 'memo', 'amount', 'date', 'account')",
            name="ck_import_rules_kind",
        ),
        sa.CheckConstraint(
            "match_type IN ('contains', 'equals')", name="ck_import_rules_match_type"
        ),
        sa.UniqueConstraint("previous_import_rule_id", name="uq_import_rules_previous"),
    )
    op.create_index(
        "ix_import_rules_book_kind", "import_rules", ["book_id", "rule_kind"], unique=False
    )
    op.create_index("ix_import_rules_priority", "import_rules", ["priority", "id"], unique=False)
    op.create_index(
        "ix_import_rules_previous", "import_rules", ["previous_import_rule_id"], unique=False
    )

    op.create_table(
        "transactions",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "previous_tx_id",
            sa.BigInteger(),
            sa.ForeignKey("transactions.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("occurred_date", sa.Date(), nullable=False),
        sa.Column("occurred_at_utc", sa.DateTime(timezone=True), nullable=True),
        sa.Column("occurred_tz", sa.String(length=64), nullable=True),
        sa.Column("posted_date", sa.Date(), nullable=False),
        sa.Column("posted_at_utc", sa.DateTime(timezone=True), nullable=True),
        sa.Column("posted_tz", sa.String(length=64), nullable=True),
        sa.Column(
            "payee_id",
            sa.BigInteger(),
            sa.ForeignKey("payees.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("memo", sa.Text(), nullable=True),
        sa.Column(
            "status", sa.String(length=20), nullable=False, server_default=sa.text("'uncleared'")
        ),
        sa.Column("reference", sa.Text(), nullable=True),
        sa.Column("import_id", sa.String(length=255), nullable=True),
        sa.Column(
            "import_session_id",
            sa.BigInteger(),
            sa.ForeignKey("import_sessions.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "created_by_user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_session_id",
            sa.BigInteger(),
            sa.ForeignKey("auth_sessions.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_device_id",
            sa.BigInteger(),
            sa.ForeignKey("user_devices.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("created_request_id", sa.String(length=64), nullable=True),
        sa.CheckConstraint(
            "status IN ('uncleared', 'cleared', 'reconciled', 'void')",
            name="ck_transactions_status",
        ),
        sa.UniqueConstraint("previous_tx_id", name="uq_transactions_previous_tx_id"),
    )
    op.create_index(
        "ix_transactions_book_occurred_date", "transactions", ["book_id", "occurred_date"]
    )
    op.create_index("ix_transactions_previous_tx_id", "transactions", ["previous_tx_id"])
    op.create_index("ix_transactions_import_id", "transactions", ["import_id"])
    op.create_index("ix_transactions_import_session_id", "transactions", ["import_session_id"])

    op.create_table(
        "balance_adjustments",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "balance_check_id",
            sa.BigInteger(),
            sa.ForeignKey("balance_checks.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "account_id",
            sa.BigInteger(),
            sa.ForeignKey("accounts.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "offset_account_id",
            sa.BigInteger(),
            sa.ForeignKey("accounts.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("as_of_date", sa.Date(), nullable=False),
        sa.Column("target_balance_minor", sa.BigInteger(), nullable=False),
        sa.Column("actual_balance_minor", sa.BigInteger(), nullable=False),
        sa.Column("adjustment_minor", sa.BigInteger(), nullable=False),
        sa.Column(
            "tx_id",
            sa.BigInteger(),
            sa.ForeignKey("transactions.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "mark",
            sa.String(length=64),
            nullable=False,
            server_default=sa.text("'balance_adjustment'"),
        ),
        sa.Column("memo", sa.Text(), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "created_by_user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_session_id",
            sa.BigInteger(),
            sa.ForeignKey("auth_sessions.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_device_id",
            sa.BigInteger(),
            sa.ForeignKey("user_devices.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("created_request_id", sa.String(length=64), nullable=True),
    )
    op.create_index(
        "ix_balance_adjustments_account_date",
        "balance_adjustments",
        ["account_id", "as_of_date"],
        unique=False,
    )
    op.create_index(
        "ix_balance_adjustments_check", "balance_adjustments", ["balance_check_id"], unique=False
    )

    op.create_table(
        "splits",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "tx_id",
            sa.BigInteger(),
            sa.ForeignKey("transactions.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "account_id",
            sa.BigInteger(),
            sa.ForeignKey("accounts.id", ondelete="RESTRICT"),
            nullable=False,
        ),
        sa.Column(
            "commodity_id",
            sa.BigInteger(),
            sa.ForeignKey("commodities.id", ondelete="RESTRICT"),
            nullable=False,
        ),
        sa.Column("amount_minor", sa.BigInteger(), nullable=False),
        sa.Column(
            "category_id",
            sa.BigInteger(),
            sa.ForeignKey("categories.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "tag_id", sa.BigInteger(), sa.ForeignKey("tags.id", ondelete="SET NULL"), nullable=True
        ),
        sa.Column(
            "person_id",
            sa.BigInteger(),
            sa.ForeignKey("people.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "project_id",
            sa.BigInteger(),
            sa.ForeignKey("projects.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("share_bps", sa.BigInteger(), nullable=True),
        sa.Column("memo", sa.Text(), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "created_by_user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_session_id",
            sa.BigInteger(),
            sa.ForeignKey("auth_sessions.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_device_id",
            sa.BigInteger(),
            sa.ForeignKey("user_devices.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("created_request_id", sa.String(length=64), nullable=True),
    )
    op.create_index("ix_splits_tx_id", "splits", ["tx_id"])
    op.create_index("ix_splits_account_id", "splits", ["account_id"])

    op.create_table(
        "import_session_transactions",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "session_id",
            sa.BigInteger(),
            sa.ForeignKey("import_sessions.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "tx_id",
            sa.BigInteger(),
            sa.ForeignKey("transactions.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("action", sa.String(length=20), nullable=False),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.CheckConstraint(
            "action IN ('created', 'updated', 'validated')",
            name="ck_import_session_transactions_action",
        ),
        sa.UniqueConstraint(
            "session_id", "tx_id", "action", name="uq_import_session_transaction_action"
        ),
    )
    op.create_index(
        "ix_import_session_transactions_session",
        "import_session_transactions",
        ["session_id"],
        unique=False,
    )
    op.create_index(
        "ix_import_session_transactions_tx", "import_session_transactions", ["tx_id"], unique=False
    )

    op.create_table(
        "import_transaction_keys",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "account_id",
            sa.BigInteger(),
            sa.ForeignKey("accounts.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "tx_id",
            sa.BigInteger(),
            sa.ForeignKey("transactions.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("import_id", sa.String(length=255), nullable=False),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.CheckConstraint(
            "length(trim(import_id)) > 0", name="ck_import_transaction_keys_import_id"
        ),
        sa.UniqueConstraint(
            "account_id", "import_id", name="uq_import_transaction_keys_account_id"
        ),
    )
    op.create_index(
        "ix_import_transaction_keys_book", "import_transaction_keys", ["book_id"], unique=False
    )
    op.create_index(
        "ix_import_transaction_keys_tx", "import_transaction_keys", ["tx_id"], unique=False
    )

    op.create_table(
        "lots",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "account_id",
            sa.BigInteger(),
            sa.ForeignKey("accounts.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "commodity_id",
            sa.BigInteger(),
            sa.ForeignKey("commodities.id", ondelete="RESTRICT"),
            nullable=False,
        ),
        sa.Column("opened_date", sa.Date(), nullable=True),
        sa.Column("notes", sa.Text(), nullable=True),
        sa.Column("cost_basis_minor", sa.BigInteger(), nullable=False),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
    )
    op.create_index("ix_lots_book_id", "lots", ["book_id"], unique=False)
    op.create_index(
        "ix_lots_account_commodity_opened",
        "lots",
        ["account_id", "commodity_id", "opened_date"],
        unique=False,
    )

    op.create_table(
        "split_lot_allocations",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "split_id",
            sa.BigInteger(),
            sa.ForeignKey("splits.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "lot_id", sa.BigInteger(), sa.ForeignKey("lots.id", ondelete="CASCADE"), nullable=False
        ),
        sa.Column("quantity_minor", sa.BigInteger(), nullable=False),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
    )
    op.create_index(
        "ix_split_lot_allocations_split_id", "split_lot_allocations", ["split_id"], unique=False
    )
    op.create_index(
        "ix_split_lot_allocations_lot_id", "split_lot_allocations", ["lot_id"], unique=False
    )

    op.create_table(
        "price_observations",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "commodity_id",
            sa.BigInteger(),
            sa.ForeignKey("commodities.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "quote_commodity_id",
            sa.BigInteger(),
            sa.ForeignKey("commodities.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "observation_kind",
            sa.String(length=32),
            nullable=False,
            server_default=sa.text("'commodity_market'"),
        ),
        sa.Column("price_minor", sa.BigInteger(), nullable=False),
        sa.Column("price_date", sa.Date(), nullable=False),
        sa.Column("source", sa.String(length=128), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.CheckConstraint(
            "observation_kind IN ('commodity_market', 'fx_daily', 'fx_manual', 'valuation_override')",
            name="ck_price_observations_observation_kind",
        ),
    )
    op.create_index(
        "ix_price_observations_book_id", "price_observations", ["book_id"], unique=False
    )
    op.create_index(
        "ix_price_observations_lookup",
        "price_observations",
        ["commodity_id", "quote_commodity_id", "observation_kind", "price_date"],
        unique=False,
    )

    op.create_table(
        "price_sources",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("name", sa.String(length=100), nullable=False),
        sa.Column(
            "kind", sa.String(length=32), nullable=False, server_default=sa.text("'provider'")
        ),
        sa.Column("provider", sa.String(length=100), nullable=True),
        sa.Column("base_url", sa.String(length=255), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.UniqueConstraint("name", name="uq_price_sources_name"),
    )
    op.create_index("ix_price_sources_kind", "price_sources", ["kind"], unique=False)

    op.create_table(
        "pricing_policies",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "base_commodity_id",
            sa.BigInteger(),
            sa.ForeignKey("commodities.id", ondelete="RESTRICT"),
            nullable=False,
        ),
        sa.Column("refresh_enabled", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column("refresh_hour_utc", sa.Integer(), nullable=False, server_default=sa.text("4")),
        sa.Column("refresh_minute_utc", sa.Integer(), nullable=False, server_default=sa.text("0")),
        sa.Column("max_backfill_days", sa.Integer(), nullable=False, server_default=sa.text("370")),
        sa.Column(
            "weekend_policy", sa.String(length=32), nullable=False, server_default=sa.text("'skip'")
        ),
        sa.Column(
            "default_source_id",
            sa.BigInteger(),
            sa.ForeignKey("price_sources.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.CheckConstraint(
            "refresh_hour_utc BETWEEN 0 AND 23", name="ck_pricing_policies_refresh_hour"
        ),
        sa.CheckConstraint(
            "refresh_minute_utc BETWEEN 0 AND 59", name="ck_pricing_policies_refresh_minute"
        ),
        sa.CheckConstraint("max_backfill_days >= 1", name="ck_pricing_policies_backfill_days"),
        sa.CheckConstraint(
            "weekend_policy IN ('skip', 'fill_previous', 'download')",
            name="ck_pricing_policies_weekend_policy",
        ),
        sa.UniqueConstraint("book_id", name="uq_pricing_policies_book_id"),
    )
    op.create_index("ix_pricing_policies_book_id", "pricing_policies", ["book_id"], unique=False)

    op.create_table(
        "pricing_source_assignments",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "commodity_id",
            sa.BigInteger(),
            sa.ForeignKey("commodities.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "quote_commodity_id",
            sa.BigInteger(),
            sa.ForeignKey("commodities.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "source_id",
            sa.BigInteger(),
            sa.ForeignKey("price_sources.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("priority", sa.Integer(), nullable=False, server_default=sa.text("100")),
        sa.Column("effective_from", sa.Date(), nullable=False),
        sa.Column("effective_to", sa.Date(), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.CheckConstraint("priority >= 0", name="ck_pricing_source_assignments_priority"),
    )
    op.create_index(
        "ix_pricing_source_assignments_book_id",
        "pricing_source_assignments",
        ["book_id"],
        unique=False,
    )

    op.create_table(
        "pricing_refresh_state",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "commodity_id",
            sa.BigInteger(),
            sa.ForeignKey("commodities.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "quote_commodity_id",
            sa.BigInteger(),
            sa.ForeignKey("commodities.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "source_id",
            sa.BigInteger(),
            sa.ForeignKey("price_sources.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("last_success_date", sa.Date(), nullable=True),
        sa.Column("last_attempt_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("last_error", sa.Text(), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.UniqueConstraint(
            "book_id",
            "commodity_id",
            "quote_commodity_id",
            "source_id",
            name="uq_pricing_refresh_state_pair_source",
        ),
    )
    op.create_index(
        "ix_pricing_refresh_state_book_id", "pricing_refresh_state", ["book_id"], unique=False
    )

    op.create_table(
        "pricing_refresh_runs",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("trigger", sa.String(length=32), nullable=False),
        sa.Column("started_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("finished_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("pairs_total", sa.Integer(), nullable=False, server_default=sa.text("0")),
        sa.Column("pairs_success", sa.Integer(), nullable=False, server_default=sa.text("0")),
        sa.Column("pairs_failed", sa.Integer(), nullable=False, server_default=sa.text("0")),
        sa.Column("rates_inserted", sa.Integer(), nullable=False, server_default=sa.text("0")),
        sa.Column("derived_inserted", sa.Integer(), nullable=False, server_default=sa.text("0")),
        sa.Column("last_error", sa.Text(), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
    )
    op.create_index(
        "ix_pricing_refresh_runs_book_id", "pricing_refresh_runs", ["book_id"], unique=False
    )
    op.create_index(
        "ix_pricing_refresh_runs_finished_at", "pricing_refresh_runs", ["finished_at"], unique=False
    )

    op.create_table(
        "report_runs",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "definition_id",
            sa.BigInteger(),
            sa.ForeignKey("report_definitions.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("params_hash", sa.String(length=128), nullable=False),
        sa.Column("as_of_seq", sa.Integer(), nullable=False),
        sa.Column(
            "pricing_mode",
            sa.String(length=32),
            nullable=False,
            server_default=sa.text("'latest_corrected'"),
        ),
        sa.Column(
            "pricing_policy_id",
            sa.BigInteger(),
            sa.ForeignKey("pricing_policies.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("pricing_policy_version", sa.String(length=255), nullable=True),
        sa.Column("valuation_snapshot_id", sa.BigInteger(), nullable=True),
        sa.Column("pricing_resolved_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("result_json", sa.Text(), nullable=False),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "created_by_user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_session_id",
            sa.BigInteger(),
            sa.ForeignKey("auth_sessions.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "created_device_id",
            sa.BigInteger(),
            sa.ForeignKey("user_devices.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("created_request_id", sa.String(length=64), nullable=True),
        sa.CheckConstraint(
            "pricing_mode IN ('frozen', 'latest_corrected')", name="ck_report_runs_pricing_mode"
        ),
        sa.UniqueConstraint(
            "book_id", "definition_id", "params_hash", "as_of_seq", name="uq_report_runs_entry"
        ),
    )
    op.create_index(
        "ix_report_runs_book_def", "report_runs", ["book_id", "definition_id"], unique=False
    )
    op.create_index(
        "ix_report_runs_book_seq", "report_runs", ["book_id", "as_of_seq"], unique=False
    )

    op.create_table(
        "user_preferences",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "default_book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("locale", sa.String(length=64), nullable=True),
        sa.Column(
            "date_format", sa.String(length=32), nullable=False, server_default=sa.text("'iso'")
        ),
        sa.Column(
            "number_format",
            sa.String(length=32),
            nullable=False,
            server_default=sa.text("'system'"),
        ),
        sa.Column(
            "theme", sa.String(length=64), nullable=False, server_default=sa.text("'system'")
        ),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.UniqueConstraint("user_id", name="uq_user_preferences_user_id"),
    )

    op.create_table(
        "audit_events",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "actor_user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "actor_session_id",
            sa.BigInteger(),
            sa.ForeignKey("auth_sessions.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "actor_device_id",
            sa.BigInteger(),
            sa.ForeignKey("user_devices.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("actor_request_id", sa.String(length=64), nullable=True),
        sa.Column("event_type", sa.String(length=100), nullable=False),
        sa.Column("target_type", sa.String(length=64), nullable=True),
        sa.Column("target_id", sa.BigInteger(), nullable=True),
        sa.Column("summary", sa.Text(), nullable=False),
        sa.Column("metadata_json", sa.Text(), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
    )
    op.create_index(
        "ix_audit_events_book_created", "audit_events", ["book_id", "created_at"], unique=False
    )
    op.create_index(
        "ix_audit_events_actor_created",
        "audit_events",
        ["actor_user_id", "created_at"],
        unique=False,
    )
    op.create_index("ix_audit_events_event_type", "audit_events", ["event_type"], unique=False)

    op.create_table(
        "transaction_saved_views",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("name", sa.String(length=120), nullable=False),
        sa.Column("filters_json", sa.Text(), nullable=False),
        sa.Column("is_shared", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.UniqueConstraint(
            "user_id", "book_id", "name", name="uq_transaction_saved_views_user_book_name"
        ),
    )
    op.create_index(
        "ix_transaction_saved_views_user_book",
        "transaction_saved_views",
        ["user_id", "book_id"],
        unique=False,
    )

    op.create_table(
        "transaction_templates",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("name", sa.String(length=120), nullable=False),
        sa.Column(
            "payee_id",
            sa.BigInteger(),
            sa.ForeignKey("payees.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("memo", sa.Text(), nullable=True),
        sa.Column(
            "status", sa.String(length=20), nullable=False, server_default=sa.text("'uncleared'")
        ),
        sa.Column("reference", sa.Text(), nullable=True),
        sa.Column("is_shared", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.UniqueConstraint(
            "user_id", "book_id", "name", name="uq_transaction_templates_user_book_name"
        ),
    )
    op.create_index(
        "ix_transaction_templates_user_book",
        "transaction_templates",
        ["user_id", "book_id"],
        unique=False,
    )

    op.create_table(
        "transaction_template_splits",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "template_id",
            sa.BigInteger(),
            sa.ForeignKey("transaction_templates.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("line_order", sa.Integer(), nullable=False),
        sa.Column(
            "account_id",
            sa.BigInteger(),
            sa.ForeignKey("accounts.id", ondelete="RESTRICT"),
            nullable=False,
        ),
        sa.Column(
            "commodity_id",
            sa.BigInteger(),
            sa.ForeignKey("commodities.id", ondelete="RESTRICT"),
            nullable=False,
        ),
        sa.Column("amount_minor", sa.BigInteger(), nullable=False),
        sa.Column(
            "category_id",
            sa.BigInteger(),
            sa.ForeignKey("categories.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "tag_id", sa.BigInteger(), sa.ForeignKey("tags.id", ondelete="SET NULL"), nullable=True
        ),
        sa.Column(
            "person_id",
            sa.BigInteger(),
            sa.ForeignKey("people.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "project_id",
            sa.BigInteger(),
            sa.ForeignKey("projects.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("share_bps", sa.BigInteger(), nullable=True),
        sa.Column("memo", sa.Text(), nullable=True),
    )
    op.create_index(
        "ix_transaction_template_splits_template",
        "transaction_template_splits",
        ["template_id", "line_order"],
        unique=False,
    )

    op.create_table(
        "payee_defaults",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "payee_id",
            sa.BigInteger(),
            sa.ForeignKey("payees.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "account_id",
            sa.BigInteger(),
            sa.ForeignKey("accounts.id", ondelete="CASCADE"),
            nullable=True,
        ),
        sa.Column(
            "category_id",
            sa.BigInteger(),
            sa.ForeignKey("categories.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("memo", sa.Text(), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.UniqueConstraint(
            "user_id", "book_id", "payee_id", "account_id", name="uq_payee_defaults_scope"
        ),
    )
    op.create_index(
        "ix_payee_defaults_user_book", "payee_defaults", ["user_id", "book_id"], unique=False
    )

    op.create_table(
        "markdown_notes",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("target_type", sa.String(length=20), nullable=False),
        sa.Column(
            "account_id",
            sa.BigInteger(),
            sa.ForeignKey("accounts.id", ondelete="CASCADE"),
            nullable=True,
        ),
        sa.Column(
            "transaction_id",
            sa.BigInteger(),
            sa.ForeignKey("transactions.id", ondelete="CASCADE"),
            nullable=True,
        ),
        sa.Column("title", sa.String(length=200), nullable=False),
        sa.Column("body_markdown", sa.Text(), nullable=False),
        sa.Column("pinned", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "created_by_user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "updated_by_user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.CheckConstraint(
            "target_type IN ('account', 'transaction')", name="ck_markdown_notes_target_type"
        ),
        sa.CheckConstraint(
            "(target_type = 'account' AND account_id IS NOT NULL AND transaction_id IS NULL) OR "
            "(target_type = 'transaction' AND transaction_id IS NOT NULL AND account_id IS NULL)",
            name="ck_markdown_notes_single_target",
        ),
    )
    op.create_index(
        "ix_markdown_notes_account",
        "markdown_notes",
        ["account_id", "pinned", "updated_at"],
        unique=False,
    )
    op.create_index(
        "ix_markdown_notes_transaction",
        "markdown_notes",
        ["transaction_id", "pinned", "updated_at"],
        unique=False,
    )

    op.create_table(
        "book_memberships",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("role", sa.String(length=20), nullable=False),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.CheckConstraint(
            "role IN ('owner', 'editor', 'viewer')", name="ck_book_memberships_role"
        ),
        sa.UniqueConstraint("user_id", "book_id", name="uq_book_memberships_user_book"),
    )

    op.execute(
        sa.text(
            "INSERT INTO books (slug, name, base_currency_code) VALUES ('personal', 'Personal', 'USD')"
        )
    )
    op.execute(sa.text("INSERT INTO book_state (book_id, change_seq) VALUES (1, 0)"))
    op.execute(
        sa.text(
            "INSERT INTO commodities (book_id, kind, symbol, name, scale) VALUES (1, 'currency', 'USD', 'US Dollar', 2)"
        )
    )
    op.execute(
        sa.text(
            """
            INSERT INTO price_sources (id, name, kind, provider, base_url)
            VALUES
                (1001, 'ECB', 'provider', 'ECB', 'https://www.ecb.europa.eu/stats/exchange/eurofxref/'),
                (1002, 'IRS', 'provider', 'IRS', 'https://www.irs.gov/individuals/international-taxpayers/yearly-average-currency-exchange-rates'),
                (1003, 'HMRC', 'provider', 'HMRC', 'https://www.gov.uk/government/collections/exchange-rates-for-customs-and-vat'),
                (1004, 'Belastingdienst', 'provider', 'Belastingdienst', 'https://www.belastingdienst.nl/wps/wcm/connect/nl/koerslijst/'),
                (1005, 'Federal Reserve', 'provider', 'Federal Reserve', 'https://www.federalreserve.gov/releases/h10/current/'),
                (1006, 'Bank of Canada', 'provider', 'Bank of Canada', 'https://www.bankofcanada.ca/rates/exchange/'),
                (1007, 'RBA', 'provider', 'RBA', 'https://www.rba.gov.au/statistics/frequency/exchange-rates.html'),
                (1008, 'SNB', 'provider', 'SNB', 'https://www.snb.ch/en/iabout/stat/statrep/id/current_interest_exchange_rates'),
                (1009, 'ExchangeRate.host', 'provider', 'ExchangeRate.host', 'https://api.exchangerate.host/'),
                (1010, 'Yahoo Finance', 'provider', 'Yahoo Finance', 'https://finance.yahoo.com/')
            """
        )
    )
    op.execute(
        sa.text(
            """
                        INSERT INTO accounts (book_id, parent_id, account_type, name, commodity_id, is_system, system_role)
            VALUES
                            (1, NULL, 'asset', 'Assets', 1, FALSE, NULL),
                            (1, 1, 'asset', 'Cash', 1, FALSE, NULL),
                            (1, NULL, 'equity', 'Opening Balances', 1, TRUE, 'opening_balance')
            """
        )
    )
    op.execute(
        sa.text(
            """
                        INSERT INTO transactions (book_id, occurred_date, posted_date, payee_id, memo, status, reference)
                        VALUES (1, DATE '2026-05-01', DATE '2026-05-01', NULL, 'Initial opening balance', 'cleared', NULL)
            """
        )
    )
    op.execute(
        sa.text(
            """
                        INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, category_id, tag_id, person_id, project_id, share_bps, memo)
            VALUES
                            (1, 2, 1, 500000, NULL, NULL, NULL, NULL, NULL, 'Opening cash balance'),
                            (1, 3, 1, -500000, NULL, NULL, NULL, NULL, NULL, 'Opening equity offset')
            """
        )
    )
    op.execute(
        sa.text(
            """
            INSERT INTO pricing_policies
                (book_id, base_commodity_id, refresh_enabled, refresh_hour_utc, refresh_minute_utc, max_backfill_days, weekend_policy, default_source_id)
            VALUES
                (1, 1, FALSE, 4, 0, 370, 'skip', NULL)
            """
        )
    )

    op.create_table(
        "budgets",
        sa.Column("id", sa.BigInteger(), autoincrement=True, nullable=False),
        sa.Column("book_id", sa.BigInteger(), nullable=False),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("period", sa.String(length=20), nullable=False),
        sa.Column("starts_on", sa.Date(), nullable=False),
        sa.Column("ends_on", sa.Date(), nullable=True),
        sa.Column("commodity_id", sa.BigInteger(), nullable=False),
        sa.Column("is_active", sa.Boolean(), server_default=sa.text("true"), nullable=False),
        sa.Column(
            "created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False
        ),
        sa.Column(
            "updated_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False
        ),
        sa.CheckConstraint("period IN ('monthly', 'annual')", name="ck_budgets_period"),
        sa.ForeignKeyConstraint(["book_id"], ["books.id"], ondelete="CASCADE"),
        sa.ForeignKeyConstraint(["commodity_id"], ["commodities.id"], ondelete="RESTRICT"),
        sa.PrimaryKeyConstraint("id"),
    )
    op.create_index("ix_budgets_book_id", "budgets", ["book_id"])

    op.create_table(
        "budget_targets",
        sa.Column("id", sa.BigInteger(), autoincrement=True, nullable=False),
        sa.Column("budget_id", sa.BigInteger(), nullable=False),
        sa.Column("book_id", sa.BigInteger(), nullable=False),
        sa.Column("category_id", sa.BigInteger(), nullable=False),
        sa.Column("amount_minor", sa.BigInteger(), nullable=False),
        sa.Column(
            "rollover_enabled", sa.Boolean(), server_default=sa.text("false"), nullable=False
        ),
        sa.Column(
            "created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False
        ),
        sa.Column(
            "updated_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False
        ),
        sa.ForeignKeyConstraint(["book_id"], ["books.id"], ondelete="CASCADE"),
        sa.ForeignKeyConstraint(["budget_id"], ["budgets.id"], ondelete="CASCADE"),
        sa.ForeignKeyConstraint(["category_id"], ["categories.id"], ondelete="CASCADE"),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("budget_id", "category_id", name="uq_budget_targets_budget_category"),
    )
    op.create_index("ix_budget_targets_budget", "budget_targets", ["budget_id"])

    op.create_table(
        "scheduled_transactions",
        sa.Column("id", sa.BigInteger(), autoincrement=True, nullable=False),
        sa.Column("book_id", sa.BigInteger(), nullable=False),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("payee_id", sa.BigInteger(), nullable=True),
        sa.Column("memo", sa.Text(), nullable=True),
        sa.Column("status", sa.String(length=20), server_default="uncleared", nullable=False),
        sa.Column("reference", sa.Text(), nullable=True),
        sa.Column("frequency", sa.String(length=20), nullable=False),
        sa.Column("interval", sa.BigInteger(), server_default=sa.text("1"), nullable=False),
        sa.Column("start_date", sa.Date(), nullable=False),
        sa.Column("end_date", sa.Date(), nullable=True),
        sa.Column("reminder_days", sa.BigInteger(), server_default=sa.text("0"), nullable=False),
        sa.Column("enabled", sa.Boolean(), server_default=sa.text("true"), nullable=False),
        sa.Column(
            "created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False
        ),
        sa.Column(
            "updated_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False
        ),
        sa.CheckConstraint(
            "frequency IN ('daily', 'weekly', 'monthly', 'yearly')",
            name="ck_scheduled_transactions_frequency",
        ),
        sa.ForeignKeyConstraint(["book_id"], ["books.id"], ondelete="CASCADE"),
        sa.ForeignKeyConstraint(["payee_id"], ["payees.id"], ondelete="SET NULL"),
        sa.PrimaryKeyConstraint("id"),
    )
    op.create_index("ix_scheduled_transactions_book", "scheduled_transactions", ["book_id"])

    op.create_table(
        "scheduled_transaction_splits",
        sa.Column("id", sa.BigInteger(), autoincrement=True, nullable=False),
        sa.Column("scheduled_transaction_id", sa.BigInteger(), nullable=False),
        sa.Column("account_id", sa.BigInteger(), nullable=False),
        sa.Column("commodity_id", sa.BigInteger(), nullable=False),
        sa.Column("amount_minor", sa.BigInteger(), nullable=False),
        sa.Column("category_id", sa.BigInteger(), nullable=True),
        sa.Column("memo", sa.Text(), nullable=True),
        sa.ForeignKeyConstraint(["account_id"], ["accounts.id"], ondelete="RESTRICT"),
        sa.ForeignKeyConstraint(["category_id"], ["categories.id"], ondelete="SET NULL"),
        sa.ForeignKeyConstraint(["commodity_id"], ["commodities.id"], ondelete="RESTRICT"),
        sa.ForeignKeyConstraint(
            ["scheduled_transaction_id"], ["scheduled_transactions.id"], ondelete="CASCADE"
        ),
        sa.PrimaryKeyConstraint("id"),
    )
    op.create_index(
        "ix_scheduled_transaction_splits_schedule",
        "scheduled_transaction_splits",
        ["scheduled_transaction_id"],
    )

    op.create_table(
        "scheduled_transaction_occurrences",
        sa.Column("id", sa.BigInteger(), autoincrement=True, nullable=False),
        sa.Column("scheduled_transaction_id", sa.BigInteger(), nullable=False),
        sa.Column("occurrence_date", sa.Date(), nullable=False),
        sa.Column("status", sa.String(length=20), nullable=False),
        sa.Column("posted_transaction_id", sa.BigInteger(), nullable=True),
        sa.Column("note", sa.Text(), nullable=True),
        sa.Column(
            "created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False
        ),
        sa.Column(
            "updated_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False
        ),
        sa.CheckConstraint(
            "status IN ('pending', 'skipped', 'posted')", name="ck_scheduled_occurrences_status"
        ),
        sa.ForeignKeyConstraint(
            ["posted_transaction_id"], ["transactions.id"], ondelete="SET NULL"
        ),
        sa.ForeignKeyConstraint(
            ["scheduled_transaction_id"], ["scheduled_transactions.id"], ondelete="CASCADE"
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "scheduled_transaction_id", "occurrence_date", name="uq_scheduled_occurrence_date"
        ),
    )

    op.create_table(
        "loans",
        sa.Column("id", sa.BigInteger(), autoincrement=True, nullable=False),
        sa.Column("book_id", sa.BigInteger(), nullable=False),
        sa.Column("account_id", sa.BigInteger(), nullable=False),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("principal_minor", sa.BigInteger(), nullable=False),
        sa.Column("annual_rate_bps", sa.BigInteger(), nullable=False),
        sa.Column("term_months", sa.BigInteger(), nullable=False),
        sa.Column(
            "payment_frequency", sa.String(length=20), server_default="monthly", nullable=False
        ),
        sa.Column("start_date", sa.Date(), nullable=False),
        sa.Column("payment_account_id", sa.BigInteger(), nullable=True),
        sa.Column("interest_category_id", sa.BigInteger(), nullable=True),
        sa.Column(
            "created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False
        ),
        sa.Column(
            "updated_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False
        ),
        sa.ForeignKeyConstraint(["account_id"], ["accounts.id"], ondelete="CASCADE"),
        sa.ForeignKeyConstraint(["book_id"], ["books.id"], ondelete="CASCADE"),
        sa.ForeignKeyConstraint(["interest_category_id"], ["categories.id"], ondelete="SET NULL"),
        sa.ForeignKeyConstraint(["payment_account_id"], ["accounts.id"], ondelete="SET NULL"),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("account_id", name="uq_loans_account"),
    )
    op.create_index("ix_loans_book", "loans", ["book_id"])

    op.create_table(
        "loan_terms",
        sa.Column("id", sa.BigInteger(), autoincrement=True, nullable=False),
        sa.Column("loan_id", sa.BigInteger(), nullable=False),
        sa.Column("effective_date", sa.Date(), nullable=False),
        sa.Column("annual_rate_bps", sa.BigInteger(), nullable=False),
        sa.Column("payment_minor", sa.BigInteger(), nullable=True),
        sa.Column(
            "extra_principal_minor", sa.BigInteger(), server_default=sa.text("0"), nullable=False
        ),
        sa.Column("metadata_json", sa.Text(), nullable=True),
        sa.Column(
            "created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False
        ),
        sa.ForeignKeyConstraint(["loan_id"], ["loans.id"], ondelete="CASCADE"),
        sa.PrimaryKeyConstraint("id"),
    )
    op.create_index("ix_loan_terms_loan", "loan_terms", ["loan_id"])

    op.create_table(
        "investment_instruments",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "commodity_id",
            sa.BigInteger(),
            sa.ForeignKey("commodities.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("instrument_type", sa.String(length=32), nullable=False),
        sa.Column("display_name", sa.String(length=200), nullable=False),
        sa.Column("symbol", sa.String(length=64), nullable=True),
        sa.Column("isin", sa.String(length=32), nullable=True),
        sa.Column("cusip", sa.String(length=32), nullable=True),
        sa.Column("figi", sa.String(length=32), nullable=True),
        sa.Column("exchange", sa.String(length=64), nullable=True),
        sa.Column("venue", sa.String(length=64), nullable=True),
        sa.Column("issuer", sa.String(length=200), nullable=True),
        sa.Column("country_code", sa.String(length=3), nullable=True),
        sa.Column(
            "quote_commodity_id",
            sa.BigInteger(),
            sa.ForeignKey("commodities.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "trading_commodity_id",
            sa.BigInteger(),
            sa.ForeignKey("commodities.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("quantity_scale", sa.Integer(), nullable=False, server_default=sa.text("4")),
        sa.Column("price_scale", sa.Integer(), nullable=False, server_default=sa.text("4")),
        sa.Column("is_active", sa.Boolean(), nullable=False, server_default=sa.text("true")),
        sa.Column("metadata_json", sa.Text(), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.CheckConstraint(
            "instrument_type IN ('stock', 'etf', 'mutual_fund', 'private_fund', 'bond', 'option', 'future', 'crypto', 'private_investment', 'generic')",
            name="ck_investment_instruments_type",
        ),
        sa.CheckConstraint(
            "quantity_scale >= 0 AND quantity_scale <= 12",
            name="ck_investment_instruments_quantity_scale_range",
        ),
        sa.CheckConstraint(
            "price_scale >= 0 AND price_scale <= 12",
            name="ck_investment_instruments_price_scale_range",
        ),
        sa.UniqueConstraint(
            "book_id", "commodity_id", name="uq_investment_instruments_book_commodity"
        ),
    )
    op.create_index(
        "ix_investment_instruments_book_id", "investment_instruments", ["book_id"], unique=False
    )
    op.create_index(
        "ix_investment_instruments_symbol",
        "investment_instruments",
        ["book_id", "symbol"],
        unique=False,
    )

    op.create_table(
        "cost_basis_profiles",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("name", sa.String(length=120), nullable=False),
        sa.Column("method", sa.String(length=32), nullable=False, server_default=sa.text("'fifo'")),
        sa.Column("description", sa.Text(), nullable=True),
        sa.Column("is_default", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column("metadata_json", sa.Text(), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.CheckConstraint(
            "method IN ('fifo', 'lifo', 'average_cost', 'specific_lot')",
            name="ck_cost_basis_profiles_method",
        ),
        sa.UniqueConstraint("book_id", "name", name="uq_cost_basis_profiles_book_name"),
    )
    op.create_index(
        "ix_cost_basis_profiles_book_id", "cost_basis_profiles", ["book_id"], unique=False
    )

    op.create_table(
        "corporate_actions",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "book_id",
            sa.BigInteger(),
            sa.ForeignKey("books.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("action_type", sa.String(length=32), nullable=False),
        sa.Column(
            "old_instrument_id",
            sa.BigInteger(),
            sa.ForeignKey("investment_instruments.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column(
            "new_instrument_id",
            sa.BigInteger(),
            sa.ForeignKey("investment_instruments.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("effective_date", sa.Date(), nullable=False),
        sa.Column("ratio_numerator", sa.BigInteger(), nullable=True),
        sa.Column("ratio_denominator", sa.BigInteger(), nullable=True),
        sa.Column("cash_in_lieu_minor", sa.BigInteger(), nullable=True),
        sa.Column(
            "cash_commodity_id",
            sa.BigInteger(),
            sa.ForeignKey("commodities.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("source_reference", sa.Text(), nullable=True),
        sa.Column("memo", sa.Text(), nullable=True),
        sa.Column(
            "generated_transaction_id",
            sa.BigInteger(),
            sa.ForeignKey("transactions.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("metadata_json", sa.Text(), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.CheckConstraint(
            "action_type IN ('split', 'reverse_split', 'spin_off', 'merger', 'acquisition', 'conversion', 'return_of_capital', 'delisting', 'write_off', 'derivative_lifecycle', 'private_investment_event')",
            name="ck_corporate_actions_type",
        ),
    )
    op.create_index(
        "ix_corporate_actions_book_effective",
        "corporate_actions",
        ["book_id", "effective_date"],
        unique=False,
    )
    op.create_index(
        "ix_corporate_actions_instrument",
        "corporate_actions",
        ["old_instrument_id", "new_instrument_id"],
        unique=False,
    )

    op.execute(
        sa.text(
            """
            INSERT INTO cost_basis_profiles (book_id, name, method, description, is_default)
            SELECT id, 'Default FIFO', 'fifo', 'Default personal accounting cost-basis profile', TRUE
            FROM books
            """
        )
    )

    op.create_table(
        "user_mfa_totp",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("secret_ciphertext", sa.Text(), nullable=False),
        sa.Column("confirmed_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.UniqueConstraint("user_id", name="uq_user_mfa_totp_user_id"),
    )
    op.create_table(
        "mfa_recovery_codes",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("code_hash", sa.String(length=128), nullable=False),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column("used_at", sa.DateTime(timezone=True), nullable=True),
    )
    op.create_index(
        "ix_mfa_recovery_codes_user_id", "mfa_recovery_codes", ["user_id"], unique=False
    )
    op.create_index(
        "ix_mfa_recovery_codes_code_hash", "mfa_recovery_codes", ["code_hash"], unique=True
    )

    op.create_table(
        "mfa_challenges",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("token_hash", sa.String(length=128), nullable=False),
        sa.Column("user_agent", sa.Text(), nullable=True),
        sa.Column("ip_address", sa.String(length=64), nullable=True),
        sa.Column("attempts", sa.BigInteger(), nullable=False, server_default=sa.text("0")),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column("expires_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("used_at", sa.DateTime(timezone=True), nullable=True),
    )
    op.create_index("ix_mfa_challenges_token_hash", "mfa_challenges", ["token_hash"], unique=True)
    op.create_index("ix_mfa_challenges_expires_at", "mfa_challenges", ["expires_at"], unique=False)

    op.create_table(
        "password_reset_tokens",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("token_hash", sa.String(length=128), nullable=False),
        sa.Column("user_agent", sa.Text(), nullable=True),
        sa.Column("ip_address", sa.String(length=64), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column("expires_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("used_at", sa.DateTime(timezone=True), nullable=True),
    )
    op.create_index(
        "ix_password_reset_tokens_token_hash", "password_reset_tokens", ["token_hash"], unique=True
    )
    op.create_index(
        "ix_password_reset_tokens_user_id", "password_reset_tokens", ["user_id"], unique=False
    )
    op.create_index(
        "ix_password_reset_tokens_expires_at", "password_reset_tokens", ["expires_at"], unique=False
    )

    op.create_table(
        "user_invites",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "invited_by_user_id",
            sa.BigInteger(),
            sa.ForeignKey("users.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("token_hash", sa.String(length=128), nullable=False),
        sa.Column("user_agent", sa.Text(), nullable=True),
        sa.Column("ip_address", sa.String(length=64), nullable=True),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column("expires_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("used_at", sa.DateTime(timezone=True), nullable=True),
    )
    op.create_index("ix_user_invites_token_hash", "user_invites", ["token_hash"], unique=True)
    op.create_index("ix_user_invites_user_id", "user_invites", ["user_id"], unique=False)
    op.create_index("ix_user_invites_expires_at", "user_invites", ["expires_at"], unique=False)


def downgrade() -> None:
    op.drop_index("ix_user_invites_expires_at", table_name="user_invites")
    op.drop_index("ix_user_invites_user_id", table_name="user_invites")
    op.drop_index("ix_user_invites_token_hash", table_name="user_invites")
    op.drop_table("user_invites")
    op.drop_index("ix_password_reset_tokens_expires_at", table_name="password_reset_tokens")
    op.drop_index("ix_password_reset_tokens_user_id", table_name="password_reset_tokens")
    op.drop_index("ix_password_reset_tokens_token_hash", table_name="password_reset_tokens")
    op.drop_table("password_reset_tokens")
    op.drop_index("ix_mfa_challenges_expires_at", table_name="mfa_challenges")
    op.drop_index("ix_mfa_challenges_token_hash", table_name="mfa_challenges")
    op.drop_table("mfa_challenges")
    op.drop_index("ix_mfa_recovery_codes_code_hash", table_name="mfa_recovery_codes")
    op.drop_index("ix_mfa_recovery_codes_user_id", table_name="mfa_recovery_codes")
    op.drop_table("mfa_recovery_codes")
    op.drop_table("user_mfa_totp")
    op.drop_index("ix_corporate_actions_instrument", table_name="corporate_actions")
    op.drop_index("ix_corporate_actions_book_effective", table_name="corporate_actions")
    op.drop_table("corporate_actions")
    op.drop_index("ix_cost_basis_profiles_book_id", table_name="cost_basis_profiles")
    op.drop_table("cost_basis_profiles")
    op.drop_index("ix_investment_instruments_symbol", table_name="investment_instruments")
    op.drop_index("ix_investment_instruments_book_id", table_name="investment_instruments")
    op.drop_table("investment_instruments")
    op.drop_index("ix_loan_terms_loan", table_name="loan_terms")
    op.drop_table("loan_terms")
    op.drop_index("ix_loans_book", table_name="loans")
    op.drop_table("loans")
    op.drop_table("scheduled_transaction_occurrences")
    op.drop_index(
        "ix_scheduled_transaction_splits_schedule", table_name="scheduled_transaction_splits"
    )
    op.drop_table("scheduled_transaction_splits")
    op.drop_index("ix_scheduled_transactions_book", table_name="scheduled_transactions")
    op.drop_table("scheduled_transactions")
    op.drop_index("ix_budget_targets_budget", table_name="budget_targets")
    op.drop_table("budget_targets")
    op.drop_index("ix_budgets_book_id", table_name="budgets")
    op.drop_table("budgets")
    op.drop_index("ix_markdown_notes_transaction", table_name="markdown_notes")
    op.drop_index("ix_markdown_notes_account", table_name="markdown_notes")
    op.drop_table("markdown_notes")
    op.drop_index("ix_payee_defaults_user_book", table_name="payee_defaults")
    op.drop_table("payee_defaults")
    op.drop_index(
        "ix_transaction_template_splits_template", table_name="transaction_template_splits"
    )
    op.drop_table("transaction_template_splits")
    op.drop_index("ix_transaction_templates_user_book", table_name="transaction_templates")
    op.drop_table("transaction_templates")
    op.drop_index("ix_transaction_saved_views_user_book", table_name="transaction_saved_views")
    op.drop_table("transaction_saved_views")
    op.drop_index("ix_audit_events_event_type", table_name="audit_events")
    op.drop_index("ix_audit_events_actor_created", table_name="audit_events")
    op.drop_index("ix_audit_events_book_created", table_name="audit_events")
    op.drop_table("audit_events")
    op.drop_table("user_preferences")
    op.drop_index("ix_report_runs_book_seq", table_name="report_runs")
    op.drop_index("ix_report_runs_book_def", table_name="report_runs")
    op.drop_table("report_runs")
    op.drop_index("ix_report_definitions_previous", table_name="report_definitions")
    op.drop_index("ix_report_definitions_kind", table_name="report_definitions")
    op.drop_index("ix_report_definitions_book", table_name="report_definitions")
    op.drop_table("report_definitions")
    op.drop_index("ix_report_cache_book_seq", table_name="report_cache")
    op.drop_index("ix_report_cache_book_type", table_name="report_cache")
    op.drop_table("report_cache")
    op.drop_table("book_state")
    op.drop_index("ix_balance_adjustments_check", table_name="balance_adjustments")
    op.drop_index("ix_balance_adjustments_account_date", table_name="balance_adjustments")
    op.drop_table("balance_adjustments")
    op.drop_index("ix_reconciliation_preferences_account", table_name="reconciliation_preferences")
    op.drop_table("reconciliation_preferences")
    op.drop_index("ix_balance_constraints_account", table_name="balance_constraints")
    op.drop_table("balance_constraints")
    op.drop_index("ix_balance_checks_account_date", table_name="balance_checks")
    op.drop_table("balance_checks")
    op.drop_index("ix_pricing_refresh_runs_finished_at", table_name="pricing_refresh_runs")
    op.drop_index("ix_pricing_refresh_runs_book_id", table_name="pricing_refresh_runs")
    op.drop_table("pricing_refresh_runs")
    op.drop_index("ix_pricing_refresh_state_book_id", table_name="pricing_refresh_state")
    op.drop_table("pricing_refresh_state")
    op.drop_index("ix_pricing_source_assignments_book_id", table_name="pricing_source_assignments")
    op.drop_table("pricing_source_assignments")
    op.drop_index("ix_pricing_policies_book_id", table_name="pricing_policies")
    op.drop_table("pricing_policies")
    op.drop_index("ix_price_sources_kind", table_name="price_sources")
    op.drop_table("price_sources")
    op.drop_index("ix_price_observations_lookup", table_name="price_observations")
    op.drop_index("ix_price_observations_book_id", table_name="price_observations")
    op.drop_table("price_observations")
    op.drop_index("ix_split_lot_allocations_lot_id", table_name="split_lot_allocations")
    op.drop_index("ix_split_lot_allocations_split_id", table_name="split_lot_allocations")
    op.drop_table("split_lot_allocations")
    op.drop_index("ix_lots_account_commodity_opened", table_name="lots")
    op.drop_index("ix_lots_book_id", table_name="lots")
    op.drop_table("lots")
    op.drop_index("ix_account_balancings_account_date", table_name="account_balancings")
    op.drop_index("ix_account_balancings_book_id", table_name="account_balancings")
    op.drop_table("account_balancings")
    op.drop_index("ix_splits_account_id", table_name="splits")
    op.drop_index("ix_splits_tx_id", table_name="splits")
    op.drop_table("splits")
    op.drop_index("ix_import_session_transactions_tx", table_name="import_session_transactions")
    op.drop_index(
        "ix_import_session_transactions_session", table_name="import_session_transactions"
    )
    op.drop_table("import_session_transactions")
    op.drop_index("ix_import_transaction_keys_tx", table_name="import_transaction_keys")
    op.drop_index("ix_import_transaction_keys_book", table_name="import_transaction_keys")
    op.drop_table("import_transaction_keys")
    op.drop_index("ix_transactions_import_session_id", table_name="transactions")
    op.drop_index("ix_transactions_import_id", table_name="transactions")
    op.drop_index("ix_transactions_book_occurred_date", table_name="transactions")
    op.drop_index("ix_transactions_previous_tx_id", table_name="transactions")
    op.drop_table("transactions")
    op.drop_index("ix_import_rules_previous", table_name="import_rules")
    op.drop_index("ix_import_rules_priority", table_name="import_rules")
    op.drop_index("ix_import_rules_book_kind", table_name="import_rules")
    op.drop_table("import_rules")
    op.drop_index("ix_import_sessions_file_hash", table_name="import_sessions")
    op.drop_index("ix_import_sessions_book_status", table_name="import_sessions")
    op.drop_table("import_sessions")
    op.drop_index("ix_projects_book_id", table_name="projects")
    op.drop_table("projects")
    op.drop_index("ix_people_book_id", table_name="people")
    op.drop_table("people")
    op.drop_index("ix_tags_book_id", table_name="tags")
    op.drop_table("tags")
    op.drop_index("ix_payees_book_id", table_name="payees")
    op.drop_table("payees")
    op.drop_index("ix_categories_parent_id", table_name="categories")
    op.drop_index("ix_categories_book_id", table_name="categories")
    op.drop_table("categories")
    op.drop_index("ix_accounts_parent_id", table_name="accounts")
    op.drop_index("ix_accounts_previous_account_id", table_name="accounts")
    op.drop_index("ix_accounts_book_id", table_name="accounts")
    op.drop_table("accounts")
    op.drop_index("ix_institutions_book_id", table_name="institutions")
    op.drop_table("institutions")
    op.drop_index("ix_countries_book_id", table_name="countries")
    op.drop_table("countries")
    op.drop_index("ix_commodities_book_id", table_name="commodities")
    op.drop_table("commodities")
    op.drop_table("book_memberships")
    op.drop_index("ix_books_slug", table_name="books")
    op.drop_table("books")
    op.drop_index("ix_auth_sessions_expires_at", table_name="auth_sessions")
    op.drop_index("ix_auth_sessions_token_hash", table_name="auth_sessions")
    op.drop_index("ix_auth_sessions_user_id", table_name="auth_sessions")
    op.drop_table("auth_sessions")
    op.drop_index("ix_user_devices_user_id", table_name="user_devices")
    op.drop_table("user_devices")
    op.drop_index("ix_users_email", table_name="users")
    op.drop_table("users")
