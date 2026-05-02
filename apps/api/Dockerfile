FROM rust:1.88-bookworm AS builder

WORKDIR /app

COPY apps/api /app

RUN cargo build --release --locked

FROM debian:bookworm-slim AS runtime

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/target/release/rekenraam-api /usr/local/bin/rekenraam-api

ENV APP_HOST=0.0.0.0
ENV APP_PORT=8080

EXPOSE 8080

CMD ["rekenraam-api"]