"""user invites

Revision ID: 0006_user_invites
Revises: 0005_password_reset_tokens
Create Date: 2026-05-11 00:00:00
"""

from __future__ import annotations

import sqlalchemy as sa

from alembic import op

revision = "0006_user_invites"
down_revision = "0005_password_reset_tokens"
branch_labels = None
depends_on = None


def upgrade() -> None:
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
    op.create_index(
        "ix_user_invites_token_hash", "user_invites", ["token_hash"], unique=True
    )
    op.create_index("ix_user_invites_user_id", "user_invites", ["user_id"], unique=False)
    op.create_index(
        "ix_user_invites_expires_at", "user_invites", ["expires_at"], unique=False
    )


def downgrade() -> None:
    op.drop_index("ix_user_invites_expires_at", table_name="user_invites")
    op.drop_index("ix_user_invites_user_id", table_name="user_invites")
    op.drop_index("ix_user_invites_token_hash", table_name="user_invites")
    op.drop_table("user_invites")
