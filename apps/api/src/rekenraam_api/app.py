from contextlib import asynccontextmanager
from pathlib import Path

from fastapi import FastAPI, HTTPException, Request, status
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import FileResponse, JSONResponse
from fastapi.staticfiles import StaticFiles

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
    password = settings.resolved_first_admin_password or ""
    if not email and not password:
        return
    if not email or not password:
        raise RuntimeError("FIRST_ADMIN_EMAIL and FIRST_ADMIN_PASSWORD must be set together")
    if len(password) < 12:
        raise RuntimeError("FIRST_ADMIN_PASSWORD must be at least 12 characters")

    async with session_factory() as session:
        await AuthService(AccessRepository(session), settings).seed_first_admin(
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


def mount_frontend_static(app: FastAPI) -> None:
    static_dir = Path(settings.frontend_static_dir)
    index_file = static_dir / "index.html"
    if not index_file.is_file():
        return

    assets_dir = static_dir / "assets"
    if assets_dir.is_dir():
        app.mount("/assets", StaticFiles(directory=assets_dir), name="assets")

    static_root = static_dir.resolve()

    @app.get("/", include_in_schema=False)
    @app.get("/{full_path:path}", include_in_schema=False)
    async def serve_frontend(full_path: str = "") -> FileResponse:  # pyright: ignore[reportUnusedFunction]
        if full_path.startswith("api/"):
            raise HTTPException(status_code=status.HTTP_404_NOT_FOUND)

        requested = (static_dir / full_path).resolve()
        if requested.is_relative_to(static_root) and requested.is_file():
            return FileResponse(requested)
        if "." in Path(full_path).name:
            raise HTTPException(status_code=status.HTTP_404_NOT_FOUND)
        return FileResponse(index_file)


mount_frontend_static(app)
