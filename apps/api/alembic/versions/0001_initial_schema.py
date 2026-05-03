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
        "accounts",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("book_id", sa.BigInteger(), sa.ForeignKey("books.id", ondelete="CASCADE"), nullable=False),
        sa.Column("parent_id", sa.BigInteger(), sa.ForeignKey("accounts.id", ondelete="SET NULL"), nullable=True),
        sa.Column("account_type", sa.String(length=32), nullable=False),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("commodity_id", sa.BigInteger(), nullable=False),
        sa.Column("number_last4", sa.String(length=4), nullable=True),
        sa.Column("is_closed", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column("is_hidden", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column("is_system", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column("system_role", sa.String(length=64), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.CheckConstraint(
            "account_type IN ('asset', 'liability', 'equity', 'income', 'expense')",
            name="ck_accounts_account_type",
        ),
    )
    op.create_index("ix_accounts_book_id", "accounts", ["book_id"], unique=False)
    op.create_index("ix_accounts_parent_id", "accounts", ["parent_id"], unique=False)

    op.create_table(
        "transactions",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("book_id", sa.BigInteger(), sa.ForeignKey("books.id", ondelete="CASCADE"), nullable=False),
        sa.Column("occurred_date", sa.Date(), nullable=False),
        sa.Column("posted_date", sa.Date(), nullable=False),
        sa.Column("memo", sa.Text(), nullable=True),
        sa.Column("status", sa.String(length=20), nullable=False, server_default=sa.text("'uncleared'")),
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
        sa.Column("amount_minor", sa.BigInteger(), nullable=False),
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
            INSERT INTO transactions (book_id, occurred_date, posted_date, memo, status)
            VALUES (1, DATE '2026-05-01', DATE '2026-05-01', 'Initial opening balance', 'cleared')
            """
        )
    )
    op.execute(
        sa.text(
            """
            INSERT INTO splits (tx_id, account_id, amount_minor, memo)
            VALUES
              (1, 2, 500000, 'Opening cash balance'),
              (1, 3, -500000, 'Opening equity offset')
            """
        )
    )


def downgrade() -> None:
    op.drop_index("ix_splits_account_id", table_name="splits")
    op.drop_index("ix_splits_tx_id", table_name="splits")
    op.drop_table("splits")
    op.drop_index("ix_transactions_book_occurred_date", table_name="transactions")
    op.drop_table("transactions")
    op.drop_index("ix_accounts_parent_id", table_name="accounts")
    op.drop_index("ix_accounts_book_id", table_name="accounts")
    op.drop_table("accounts")
    op.drop_table("book_memberships")
    op.drop_index("ix_books_slug", table_name="books")
    op.drop_table("books")
    op.drop_index("ix_users_email", table_name="users")
    op.drop_table("users")