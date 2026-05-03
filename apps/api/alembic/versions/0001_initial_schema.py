"""initial schema

Revision ID: 0001_initial_schema
Revises: None
Create Date: 2026-05-03 00:00:00
"""

from __future__ import annotations

from alembic import op
import sqlalchemy as sa


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
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
    )
    op.create_index("ix_users_email", "users", ["email"], unique=True)

    op.create_table(
        "books",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("slug", sa.String(length=120), nullable=False),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("base_currency_code", sa.String(length=3), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
    )
    op.create_index("ix_books_slug", "books", ["slug"], unique=True)

    op.create_table(
        "commodities",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("book_id", sa.BigInteger(), sa.ForeignKey("books.id", ondelete="CASCADE"), nullable=False),
        sa.Column("kind", sa.String(length=32), nullable=False),
        sa.Column("symbol", sa.String(length=32), nullable=True),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("scale", sa.Integer(), nullable=False),
        sa.Column("metadata", sa.Text(), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
    )
    op.create_index("ix_commodities_book_id", "commodities", ["book_id"], unique=False)

    op.create_table(
        "countries",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("book_id", sa.BigInteger(), sa.ForeignKey("books.id", ondelete="CASCADE"), nullable=False),
        sa.Column("code", sa.String(length=3), nullable=False),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
    )
    op.create_index("ix_countries_book_id", "countries", ["book_id"], unique=False)

    op.create_table(
        "institutions",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("book_id", sa.BigInteger(), sa.ForeignKey("books.id", ondelete="CASCADE"), nullable=False),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("kind", sa.String(length=64), nullable=True),
        sa.Column("country_id", sa.BigInteger(), sa.ForeignKey("countries.id", ondelete="SET NULL"), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
    )
    op.create_index("ix_institutions_book_id", "institutions", ["book_id"], unique=False)

    op.create_table(
        "categories",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("book_id", sa.BigInteger(), sa.ForeignKey("books.id", ondelete="CASCADE"), nullable=False),
        sa.Column("parent_id", sa.BigInteger(), sa.ForeignKey("categories.id", ondelete="SET NULL"), nullable=True),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("kind", sa.String(length=32), nullable=False),
        sa.Column("color", sa.String(length=32), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
    )
    op.create_index("ix_categories_book_id", "categories", ["book_id"], unique=False)
    op.create_index("ix_categories_parent_id", "categories", ["parent_id"], unique=False)

    op.create_table(
        "payees",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("book_id", sa.BigInteger(), sa.ForeignKey("books.id", ondelete="CASCADE"), nullable=False),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("kind", sa.String(length=32), nullable=False),
        sa.Column("metadata", sa.Text(), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
    )
    op.create_index("ix_payees_book_id", "payees", ["book_id"], unique=False)

    op.create_table(
        "tags",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("book_id", sa.BigInteger(), sa.ForeignKey("books.id", ondelete="CASCADE"), nullable=False),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("color", sa.String(length=32), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
    )
    op.create_index("ix_tags_book_id", "tags", ["book_id"], unique=False)

    op.create_table(
        "people",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("book_id", sa.BigInteger(), sa.ForeignKey("books.id", ondelete="CASCADE"), nullable=False),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("role", sa.String(length=64), nullable=False),
        sa.Column("metadata", sa.Text(), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
    )
    op.create_index("ix_people_book_id", "people", ["book_id"], unique=False)

    op.create_table(
        "projects",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("book_id", sa.BigInteger(), sa.ForeignKey("books.id", ondelete="CASCADE"), nullable=False),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("status", sa.String(length=64), nullable=False),
        sa.Column("metadata", sa.Text(), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
    )
    op.create_index("ix_projects_book_id", "projects", ["book_id"], unique=False)

    op.create_table(
        "accounts",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("book_id", sa.BigInteger(), sa.ForeignKey("books.id", ondelete="CASCADE"), nullable=False),
        sa.Column("parent_id", sa.BigInteger(), sa.ForeignKey("accounts.id", ondelete="SET NULL"), nullable=True),
        sa.Column("previous_account_id", sa.BigInteger(), sa.ForeignKey("accounts.id", ondelete="SET NULL"), nullable=True),
        sa.Column("account_type", sa.String(length=32), nullable=False),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("commodity_id", sa.BigInteger(), nullable=False),
        sa.Column("booking_policy", sa.String(length=16), nullable=False, server_default=sa.text("'fifo'")),
        sa.Column("number_last4", sa.String(length=4), nullable=True),
        sa.Column("is_closed", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column("is_hidden", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column("is_system", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column("system_role", sa.String(length=64), nullable=True),
        sa.Column("effective_at", sa.Date(), nullable=False, server_default=sa.text("CURRENT_DATE")),
        sa.Column("lifecycle_event", sa.String(length=16), nullable=False, server_default=sa.text("'open'")),
        sa.Column("lifecycle_note", sa.Text(), nullable=True),
        sa.Column("lifecycle_metadata", sa.Text(), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.CheckConstraint(
            "account_type IN ('cash', 'checking', 'savings', 'credit', 'loan', 'investment', 'asset', 'liability', 'income', 'expense', 'equity')",
            name="ck_accounts_account_type",
        ),
        sa.CheckConstraint("booking_policy IN ('fifo', 'lifo', 'strict', 'average')", name="ck_accounts_booking_policy"),
        sa.CheckConstraint(
            "lifecycle_event IN ('open', 'close', 'reopen', 'update')",
            name="ck_accounts_lifecycle_event",
        ),
    )
    op.create_index("ix_accounts_book_id", "accounts", ["book_id"], unique=False)
    op.create_index("ix_accounts_parent_id", "accounts", ["parent_id"], unique=False)

    op.create_table(
        "account_balancings",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("book_id", sa.BigInteger(), sa.ForeignKey("books.id", ondelete="CASCADE"), nullable=False),
        sa.Column("account_id", sa.BigInteger(), sa.ForeignKey("accounts.id", ondelete="CASCADE"), nullable=False),
        sa.Column(
            "previous_account_balancing_id",
            sa.BigInteger(),
            sa.ForeignKey("account_balancings.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("as_of_date", sa.Date(), nullable=False),
        sa.Column("balance_minor", sa.BigInteger(), nullable=False),
        sa.Column("memo", sa.Text(), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.Column("voided_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("void_reason", sa.Text(), nullable=True),
    )
    op.create_index("ix_account_balancings_book_id", "account_balancings", ["book_id"], unique=False)
    op.create_index(
        "ix_account_balancings_account_date",
        "account_balancings",
        ["account_id", "as_of_date"],
        unique=False,
    )

    op.create_table(
        "transactions",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("book_id", sa.BigInteger(), sa.ForeignKey("books.id", ondelete="CASCADE"), nullable=False),
        sa.Column("occurred_date", sa.Date(), nullable=False),
        sa.Column("posted_date", sa.Date(), nullable=False),
        sa.Column("payee_id", sa.BigInteger(), sa.ForeignKey("payees.id", ondelete="SET NULL"), nullable=True),
        sa.Column("memo", sa.Text(), nullable=True),
        sa.Column("status", sa.String(length=20), nullable=False, server_default=sa.text("'uncleared'")),
        sa.Column("reference", sa.Text(), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.CheckConstraint(
            "status IN ('uncleared', 'cleared', 'reconciled', 'void')",
            name="ck_transactions_status",
        ),
    )
    op.create_index("ix_transactions_book_occurred_date", "transactions", ["book_id", "occurred_date"])

    op.create_table(
        "splits",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("tx_id", sa.BigInteger(), sa.ForeignKey("transactions.id", ondelete="CASCADE"), nullable=False),
        sa.Column("account_id", sa.BigInteger(), sa.ForeignKey("accounts.id", ondelete="RESTRICT"), nullable=False),
        sa.Column("commodity_id", sa.BigInteger(), sa.ForeignKey("commodities.id", ondelete="RESTRICT"), nullable=False),
        sa.Column("amount_minor", sa.BigInteger(), nullable=False),
        sa.Column("category_id", sa.BigInteger(), sa.ForeignKey("categories.id", ondelete="SET NULL"), nullable=True),
        sa.Column("tag_id", sa.BigInteger(), sa.ForeignKey("tags.id", ondelete="SET NULL"), nullable=True),
        sa.Column("person_id", sa.BigInteger(), sa.ForeignKey("people.id", ondelete="SET NULL"), nullable=True),
        sa.Column("project_id", sa.BigInteger(), sa.ForeignKey("projects.id", ondelete="SET NULL"), nullable=True),
        sa.Column("share_bps", sa.BigInteger(), nullable=True),
        sa.Column("memo", sa.Text(), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
    )
    op.create_index("ix_splits_tx_id", "splits", ["tx_id"])
    op.create_index("ix_splits_account_id", "splits", ["account_id"])

    op.create_table(
        "book_memberships",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("user_id", sa.BigInteger(), sa.ForeignKey("users.id", ondelete="CASCADE"), nullable=False),
        sa.Column("book_id", sa.BigInteger(), sa.ForeignKey("books.id", ondelete="CASCADE"), nullable=False),
        sa.Column("role", sa.String(length=20), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.CheckConstraint("role IN ('owner', 'editor', 'viewer')", name="ck_book_memberships_role"),
        sa.UniqueConstraint("user_id", "book_id", name="uq_book_memberships_user_book"),
    )

    op.execute(
        sa.text(
            "INSERT INTO books (slug, name, base_currency_code) VALUES ('personal', 'Personal', 'USD')"
        )
    )
    op.execute(
        sa.text(
            "INSERT INTO commodities (book_id, kind, symbol, name, scale) VALUES (1, 'currency', 'USD', 'US Dollar', 2)"
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


def downgrade() -> None:
    op.drop_index("ix_account_balancings_account_date", table_name="account_balancings")
    op.drop_index("ix_account_balancings_book_id", table_name="account_balancings")
    op.drop_table("account_balancings")
    op.drop_index("ix_splits_account_id", table_name="splits")
    op.drop_index("ix_splits_tx_id", table_name="splits")
    op.drop_table("splits")
    op.drop_index("ix_transactions_book_occurred_date", table_name="transactions")
    op.drop_table("transactions")
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
    op.drop_index("ix_users_email", table_name="users")
    op.drop_table("users")