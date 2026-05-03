"""add transactions

Revision ID: 0003_add_transactions
Revises: 0002_add_accounts
Create Date: 2026-05-03 01:10:00
"""

from __future__ import annotations

from alembic import op
import sqlalchemy as sa


revision = "0003_add_transactions"
down_revision = "0002_add_accounts"
branch_labels = None
depends_on = None


def upgrade() -> None:
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
    op.execute(sa.text("UPDATE app_schema_state SET schema_version = '0003_add_transactions' WHERE id = TRUE"))


def downgrade() -> None:
    op.execute(sa.text("UPDATE app_schema_state SET schema_version = '0002_add_accounts' WHERE id = TRUE"))
    op.drop_index("ix_splits_account_id", table_name="splits")
    op.drop_index("ix_splits_tx_id", table_name="splits")
    op.drop_table("splits")
    op.drop_index("ix_transactions_book_occurred_date", table_name="transactions")
    op.drop_table("transactions")