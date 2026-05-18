FROM python:3.14-slim-bookworm AS runtime

ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends curl build-essential \
    && rm -rf /var/lib/apt/lists/*

COPY apps/api/pyproject.toml /app/pyproject.toml
COPY apps/api/alembic.ini /app/alembic.ini
COPY apps/api/alembic /app/alembic
COPY apps/api/src /app/src

RUN pip install --no-cache-dir .

ENV APP_HOST=0.0.0.0
ENV APP_PORT=16888

EXPOSE 16888

CMD ["sh", "-c", "alembic upgrade head && uvicorn rekenraam_api.app:app --host ${APP_HOST} --port ${APP_PORT}"]
