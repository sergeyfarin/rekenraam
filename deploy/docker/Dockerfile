FROM node:26-trixie-slim AS frontend
WORKDIR /src
RUN corepack enable
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY frontend/package.json ./frontend/package.json
RUN pnpm install --filter ./frontend... --frozen-lockfile
COPY frontend ./frontend
WORKDIR /src/frontend
RUN pnpm run build

FROM golang:1.27-trixie AS backend
WORKDIR /src
COPY backend ./backend
COPY --from=frontend /src/frontend/build ./backend/internal/web/dist
WORKDIR /src/backend
RUN CGO_ENABLED=0 go build -trimpath -o /out/rekenraam ./cmd/rekenraam

FROM debian:trixie-slim
WORKDIR /app
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /app/data \
    && chown -R 10001:10001 /app \
    && chmod 0700 /app/data
COPY --from=backend --chown=10001:10001 /out/rekenraam /app/rekenraam
USER 10001:10001
VOLUME ["/app/data"]
ENV HTTP_ADDR=:16888
ENV DATABASE_URL=file:/app/data/rekenraam.sqlite
EXPOSE 16888
ENTRYPOINT ["/app/rekenraam"]
