FROM node:22-alpine AS frontend
WORKDIR /src
RUN corepack enable
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY frontend/package.json ./frontend/package.json
RUN pnpm install --filter ./frontend... --frozen-lockfile
COPY frontend ./frontend
WORKDIR /src/frontend
RUN pnpm run build

FROM golang:1.22-alpine AS backend
WORKDIR /src
COPY backend ./backend
COPY --from=frontend /src/frontend/build ./backend/internal/web/dist
WORKDIR /src/backend
RUN go build -o /out/rekenraam ./cmd/rekenraam

FROM alpine:3.20
WORKDIR /app
COPY --from=backend /out/rekenraam /app/rekenraam
VOLUME ["/app/data"]
ENV HTTP_ADDR=:16888
ENV DATABASE_URL=file:/app/data/rekenraam.sqlite
EXPOSE 16888
ENTRYPOINT ["/app/rekenraam"]
