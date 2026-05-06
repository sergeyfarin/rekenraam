"""milestone 8 investments and valuation

Revision ID: 0003_milestone8_investments
Revises: 0002_planning_schema
Create Date: 2026-05-06 00:00:00
"""

from __future__ import annotations

import sqlalchemy as sa

from alembic import op

revision = "0003_milestone8_investments"
down_revision = "0002_planning_schema"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.create_table(
        "investment_instruments",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("book_id", sa.BigInteger(), sa.ForeignKey("books.id", ondelete="CASCADE"), nullable=False),
        sa.Column("commodity_id", sa.BigInteger(), sa.ForeignKey("commodities.id", ondelete="CASCADE"), nullable=False),
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
        sa.Column("quote_commodity_id", sa.BigInteger(), sa.ForeignKey("commodities.id", ondelete="SET NULL"), nullable=True),
        sa.Column("trading_commodity_id", sa.BigInteger(), sa.ForeignKey("commodities.id", ondelete="SET NULL"), nullable=True),
        sa.Column("quantity_scale", sa.Integer(), nullable=False, server_default=sa.text("4")),
        sa.Column("price_scale", sa.Integer(), nullable=False, server_default=sa.text("4")),
        sa.Column("is_active", sa.Boolean(), nullable=False, server_default=sa.text("true")),
        sa.Column("metadata_json", sa.Text(), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.CheckConstraint(
            "instrument_type IN ('stock', 'etf', 'mutual_fund', 'private_fund', 'bond', 'option', 'future', 'crypto', 'private_investment', 'generic')",
            name="ck_investment_instruments_type",
        ),
        sa.UniqueConstraint("book_id", "commodity_id", name="uq_investment_instruments_book_commodity"),
    )
    op.create_index("ix_investment_instruments_book_id", "investment_instruments", ["book_id"], unique=False)
    op.create_index("ix_investment_instruments_symbol", "investment_instruments", ["book_id", "symbol"], unique=False)

    op.create_table(
        "cost_basis_profiles",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("book_id", sa.BigInteger(), sa.ForeignKey("books.id", ondelete="CASCADE"), nullable=False),
        sa.Column("name", sa.String(length=120), nullable=False),
        sa.Column("method", sa.String(length=32), nullable=False, server_default=sa.text("'fifo'")),
        sa.Column("description", sa.Text(), nullable=True),
        sa.Column("is_default", sa.Boolean(), nullable=False, server_default=sa.text("false")),
        sa.Column("metadata_json", sa.Text(), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.CheckConstraint("method IN ('fifo', 'lifo', 'average_cost', 'specific_lot')", name="ck_cost_basis_profiles_method"),
        sa.UniqueConstraint("book_id", "name", name="uq_cost_basis_profiles_book_name"),
    )
    op.create_index("ix_cost_basis_profiles_book_id", "cost_basis_profiles", ["book_id"], unique=False)

    op.create_table(
        "corporate_actions",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column("book_id", sa.BigInteger(), sa.ForeignKey("books.id", ondelete="CASCADE"), nullable=False),
        sa.Column("action_type", sa.String(length=32), nullable=False),
        sa.Column("old_instrument_id", sa.BigInteger(), sa.ForeignKey("investment_instruments.id", ondelete="SET NULL"), nullable=True),
        sa.Column("new_instrument_id", sa.BigInteger(), sa.ForeignKey("investment_instruments.id", ondelete="SET NULL"), nullable=True),
        sa.Column("effective_date", sa.Date(), nullable=False),
        sa.Column("ratio_numerator", sa.BigInteger(), nullable=True),
        sa.Column("ratio_denominator", sa.BigInteger(), nullable=True),
        sa.Column("cash_in_lieu_minor", sa.BigInteger(), nullable=True),
        sa.Column("cash_commodity_id", sa.BigInteger(), sa.ForeignKey("commodities.id", ondelete="SET NULL"), nullable=True),
        sa.Column("source_reference", sa.Text(), nullable=True),
        sa.Column("memo", sa.Text(), nullable=True),
        sa.Column("generated_transaction_id", sa.BigInteger(), sa.ForeignKey("transactions.id", ondelete="SET NULL"), nullable=True),
        sa.Column("metadata_json", sa.Text(), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.text("now()")),
        sa.CheckConstraint(
            "action_type IN ('split', 'reverse_split', 'spin_off', 'merger', 'acquisition', 'conversion', 'return_of_capital', 'delisting', 'write_off', 'derivative_lifecycle', 'private_investment_event')",
            name="ck_corporate_actions_type",
        ),
    )
    op.create_index("ix_corporate_actions_book_effective", "corporate_actions", ["book_id", "effective_date"], unique=False)
    op.create_index("ix_corporate_actions_instrument", "corporate_actions", ["old_instrument_id", "new_instrument_id"], unique=False)

    op.execute(
        sa.text(
            """
            INSERT INTO cost_basis_profiles (book_id, name, method, description, is_default)
            SELECT id, 'Default FIFO', 'fifo', 'Default personal accounting cost-basis profile', TRUE
            FROM books
            """
        )
    )


def downgrade() -> None:
    op.drop_index("ix_corporate_actions_instrument", table_name="corporate_actions")
    op.drop_index("ix_corporate_actions_book_effective", table_name="corporate_actions")
    op.drop_table("corporate_actions")
    op.drop_index("ix_cost_basis_profiles_book_id", table_name="cost_basis_profiles")
    op.drop_table("cost_basis_profiles")
    op.drop_index("ix_investment_instruments_symbol", table_name="investment_instruments")
    op.drop_index("ix_investment_instruments_book_id", table_name="investment_instruments")
    op.drop_table("investment_instruments")
