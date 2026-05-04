from contextlib import asynccontextmanager

from fastapi import FastAPI, Request, status
from fastapi.responses import JSONResponse
from fastapi.middleware.cors import CORSMiddleware

from rekenraam_api.api.router import api_router
from rekenraam_api.config.settings import get_settings
from rekenraam_api.db.session import session_factory
from rekenraam_api.repositories.access import AccessRepository
from rekenraam_api.services.access import AuthorizationError
from rekenraam_api.services.auth import AuthService
from rekenraam_api.workers.pricing import pricing_refresh_worker


async def seed_first_admin_from_env() -> None:
	settings = get_settings()
	email = (settings.first_admin_email or "").strip()
	password = settings.first_admin_password or ""
	if not email and not password:
		return
	if not email or not password:
		raise RuntimeError("FIRST_ADMIN_EMAIL and FIRST_ADMIN_PASSWORD must be set together")
	if len(password) < 12:
		raise RuntimeError("FIRST_ADMIN_PASSWORD must be at least 12 characters")

	async with session_factory() as session:
		await AuthService(AccessRepository(session)).seed_first_admin(
			email=email,
			password=password,
			display_name=settings.first_admin_display_name,
		)


@asynccontextmanager
async def lifespan(app: FastAPI):
	await seed_first_admin_from_env()
	app.state.pricing_worker = pricing_refresh_worker
	await pricing_refresh_worker.start()
	try:
		yield
	finally:
		await pricing_refresh_worker.stop()


app = FastAPI(title="Rekenraam API", version="0.1.0", lifespan=lifespan)


@app.exception_handler(AuthorizationError)
async def authorization_error_handler(_request: Request, exc: AuthorizationError) -> JSONResponse:
	return JSONResponse(status_code=status.HTTP_403_FORBIDDEN, content={"detail": str(exc)})


settings = get_settings()
app.add_middleware(
	CORSMiddleware,
	allow_origins=settings.cors_allowed_origins_list,
	allow_credentials=True,
	allow_methods=["*"],
	allow_headers=["*"],
)
app.include_router(api_router, prefix="/api/v1")
