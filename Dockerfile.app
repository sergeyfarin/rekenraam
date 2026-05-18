FROM node:22-alpine AS frontend-build

WORKDIR /web

COPY package.json package-lock.json ./
RUN npm ci

COPY src ./src
COPY static ./static
COPY components.json ./components.json
COPY svelte.config.js ./svelte.config.js
COPY tsconfig.json ./tsconfig.json
COPY vite.config.js ./vite.config.js

ARG PUBLIC_API_BASE_URL=/api/v1
ENV PUBLIC_API_BASE_URL=${PUBLIC_API_BASE_URL}

RUN npm run build

FROM python:3.14-slim-bookworm AS runtime

ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1
ENV APP_HOST=0.0.0.0
ENV APP_PORT=16888
ENV FRONTEND_STATIC_DIR=/app/static

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends curl build-essential \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /data

COPY apps/api/pyproject.toml /app/pyproject.toml
COPY apps/api/alembic.ini /app/alembic.ini
COPY apps/api/alembic /app/alembic
COPY apps/api/src /app/src
COPY --from=frontend-build /web/build /app/static

RUN pip install --no-cache-dir .

EXPOSE 16888

CMD ["sh", "-c", "alembic upgrade head && uvicorn rekenraam_api.app:app --host ${APP_HOST} --port ${APP_PORT}"]
