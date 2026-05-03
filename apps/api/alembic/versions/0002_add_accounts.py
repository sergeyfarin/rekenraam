"""add accounts

Revision ID: 0002_add_accounts
Revises: 0001_initial_schema
Create Date: 2026-05-03 00:20:00
"""

from __future__ import annotations

from alembic import op
import sqlalchemy as sa


revision = "0002_add_accounts"
down_revision = "0001_initial_schema"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.create_table(
        "accounts",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("book_id", sa.BigInteger(), sa.ForeignKey("books.id", ondelete="CASCADE"), nullable=False),
        sa.Column("parent_id", sa.BigInteger(), sa.ForeignKey("accounts.id", ondelete="SET NULL"), nullable=True),
        sa.Column("account_type", sa.String(length=32), nullable=False),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("is_closed", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column("is_hidden", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column("is_system", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column("system_role", sa.String(length=64), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.CheckConstraint(
            "account_type IN ('asset', 'liability', 'equity', 'income', 'expense')",
            name="ck_accounts_account_type",
        ),
    )
    op.create_index("ix_accounts_book_id", "accounts", ["book_id"], unique=False)
    op.create_index("ix_accounts_parent_id", "accounts", ["parent_id"], unique=False)

    op.execute(
        sa.text(
            """
            INSERT INTO accounts (book_id, parent_id, account_type, name, is_system, system_role)
            VALUES
              (1, NULL, 'asset', 'Assets', FALSE, NULL),
              (1, 1, 'asset', 'Cash', FALSE, NULL),
              (1, NULL, 'equity', 'Opening Balances', TRUE, 'opening_balance')
            """
        )
    )
    op.execute(sa.text("UPDATE app_schema_state SET schema_version = '0002_add_accounts' WHERE id = TRUE"))


def downgrade() -> None:
    op.execute(sa.text("UPDATE app_schema_state SET schema_version = '0001_initial_schema' WHERE id = TRUE"))
    op.drop_index("ix_accounts_parent_id", table_name="accounts")
    op.drop_index("ix_accounts_book_id", table_name="accounts")
    op.drop_table("accounts")