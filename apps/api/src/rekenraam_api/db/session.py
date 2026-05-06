from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker, create_async_engine

from rekenraam_api.config.settings import get_settings

settings = get_settings()
engine = create_async_engine(settings.database_url, future=True)
session_factory = async_sessionmaker(bind=engine, class_=AsyncSession, expire_on_commit=False)