"""pricing policy and observation metadata

Revision ID: 0002_pricing_policy_and_observation_metadata
Revises: 0001_initial_schema
Create Date: 2026-05-18 00:00:00
"""

from __future__ import annotations

import sqlalchemy as sa

from alembic import op

revision = "0002_pricing_policy_and_observation_metadata"
down_revision = "0001_initial_schema"
branch_labels = None
depends_on = None


def upgrade() -> None:
    with op.batch_alter_table("pricing_policies") as batch_op:
        batch_op.add_column(
            sa.Column(
                "staleness_max_days",
                sa.Integer(),
                nullable=False,
                server_default=sa.text("3"),
            )
        )
        batch_op.add_column(
            sa.Column(
                "triangulation_max_hops",
                sa.Integer(),
                nullable=False,
                server_default=sa.text("1"),
            )
        )
        batch_op.add_column(
            sa.Column(
                "rounding_mode",
                sa.String(length=32),
                nullable=False,
                server_default="half_up",
            )
        )
        batch_op.add_column(
            sa.Column(
                "prefer_official_fx",
                sa.Boolean(),
                nullable=False,
                server_default=sa.text("true"),
            )
        )
        batch_op.create_check_constraint(
            "ck_pricing_policies_staleness_days", "staleness_max_days >= 1"
        )
        batch_op.create_check_constraint(
            "ck_pricing_policies_triangulation_hops",
            "triangulation_max_hops >= 0 AND triangulation_max_hops <= 4",
        )
        batch_op.create_check_constraint(
            "ck_pricing_policies_rounding_mode",
            "rounding_mode IN ('half_up', 'half_even', 'down', 'up')",
        )

    with op.batch_alter_table("price_observations") as batch_op:
        batch_op.add_column(
            sa.Column("mode", sa.String(length=32), nullable=False, server_default="market")
        )
        batch_op.add_column(sa.Column("period_type", sa.String(length=16), nullable=True))
        batch_op.add_column(sa.Column("period_year", sa.Integer(), nullable=True))
        batch_op.add_column(sa.Column("period_month", sa.Integer(), nullable=True))
        batch_op.create_check_constraint(
            "ck_price_observations_mode",
            "mode IN ('market', 'daily', 'official', 'transaction_implied')",
        )
        batch_op.create_check_constraint(
            "ck_price_observations_period_type",
            "period_type IS NULL OR period_type IN ('daily', 'monthly', 'yearly')",
        )
        batch_op.create_check_constraint(
            "ck_price_observations_period_month",
            "period_month IS NULL OR (period_month >= 1 AND period_month <= 12)",
        )
        batch_op.create_check_constraint(
            "ck_price_observations_period_requires_type",
            "period_type IS NOT NULL OR (period_year IS NULL AND period_month IS NULL)",
        )

    op.execute(
        """
        UPDATE pricing_policies
        SET staleness_max_days = COALESCE(staleness_max_days, 3),
            triangulation_max_hops = COALESCE(triangulation_max_hops, 1),
            rounding_mode = COALESCE(rounding_mode, 'half_up'),
            prefer_official_fx = COALESCE(prefer_official_fx, 1)
        """
    )
    op.execute(
        """
        UPDATE price_observations
        SET mode = CASE
                WHEN observation_kind = 'fx_daily' THEN 'daily'
                WHEN observation_kind = 'fx_manual' THEN 'official'
                WHEN observation_kind = 'commodity_market' AND source = 'implicit' THEN 'transaction_implied'
                ELSE 'market'
            END,
            period_type = CASE
                WHEN observation_kind = 'fx_daily' THEN 'daily'
                WHEN observation_kind = 'fx_manual' AND strftime('%m-%d', price_date) = '01-01' THEN 'yearly'
                WHEN observation_kind = 'fx_manual' THEN 'monthly'
                ELSE NULL
            END,
            period_year = CASE
                WHEN observation_kind IN ('fx_daily', 'fx_manual') THEN CAST(strftime('%Y', price_date) AS INTEGER)
                ELSE NULL
            END,
            period_month = CASE
                WHEN observation_kind = 'fx_daily' THEN CAST(strftime('%m', price_date) AS INTEGER)
                WHEN observation_kind = 'fx_manual' AND strftime('%m-%d', price_date) != '01-01' THEN CAST(strftime('%m', price_date) AS INTEGER)
                ELSE NULL
            END
        """
    )


def downgrade() -> None:
    with op.batch_alter_table("price_observations") as batch_op:
        batch_op.drop_constraint("ck_price_observations_period_requires_type", type_="check")
        batch_op.drop_constraint("ck_price_observations_period_month", type_="check")
        batch_op.drop_constraint("ck_price_observations_period_type", type_="check")
        batch_op.drop_constraint("ck_price_observations_mode", type_="check")
        batch_op.drop_column("period_month")
        batch_op.drop_column("period_year")
        batch_op.drop_column("period_type")
        batch_op.drop_column("mode")

    with op.batch_alter_table("pricing_policies") as batch_op:
        batch_op.drop_constraint("ck_pricing_policies_rounding_mode", type_="check")
        batch_op.drop_constraint("ck_pricing_policies_triangulation_hops", type_="check")
        batch_op.drop_constraint("ck_pricing_policies_staleness_days", type_="check")
        batch_op.drop_column("prefer_official_fx")
        batch_op.drop_column("rounding_mode")
        batch_op.drop_column("triangulation_max_hops")
        batch_op.drop_column("staleness_max_days")
