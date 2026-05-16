from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from rekenraam_api.config.settings import get_settings
from rekenraam_api.db.audit_listener import install_audit_listener
from rekenraam_api.db.dialect import create_database_engine

settings = get_settings()
engine = create_database_engine(settings.database_url)
session_factory = async_sessionmaker(bind=engine, class_=AsyncSession, expire_on_commit=False)

install_audit_listener()
