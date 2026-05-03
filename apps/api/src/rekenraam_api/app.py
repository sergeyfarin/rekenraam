from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from rekenraam_api.api.router import api_router
from rekenraam_api.config.settings import get_settings
from rekenraam_api.workers.pricing import pricing_refresh_worker


@asynccontextmanager
async def lifespan(app: FastAPI):
	app.state.pricing_worker = pricing_refresh_worker
	await pricing_refresh_worker.start()
	try:
		yield
	finally:
		await pricing_refresh_worker.stop()


app = FastAPI(title="Rekenraam API", version="0.1.0", lifespan=lifespan)
settings = get_settings()
app.add_middleware(
	CORSMiddleware,
	allow_origins=settings.cors_allowed_origins_list,
	allow_credentials=False,
	allow_methods=["*"],
	allow_headers=["*"],
)
app.include_router(api_router, prefix="/api/v1")