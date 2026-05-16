from datetime import datetime

from sqlalchemy import DateTime, String, func
from sqlalchemy.orm import Mapped, mapped_column

from rekenraam_api.db.base import Base


class DatabaseLock(Base):
    __tablename__ = "database_locks"

    lock_key: Mapped[str] = mapped_column(String(120), primary_key=True)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
