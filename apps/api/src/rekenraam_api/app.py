from fastapi import FastAPI

from rekenraam_api.api.router import api_router


app = FastAPI(title="Rekenraam API", version="0.1.0")
app.include_router(api_router, prefix="/api/v1")