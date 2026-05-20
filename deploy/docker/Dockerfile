FROM node:22-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm install
COPY frontend ./
RUN npm run build

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
