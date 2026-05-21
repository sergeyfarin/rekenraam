# Rust Backend

Experimental Axum backend for comparing the Go and Rust implementation paths.

## Layout

```text
src/api/        HTTP routes, handlers, and response DTOs
src/app/        Application services for finance features
src/config.rs   Environment-backed configuration
src/db/         SQLite connection, migrations, and repository support
src/web.rs      Static frontend file serving
migrations/     SQLite schema migrations
var/            Local dev database files, ignored by Git
dist/           Static files served by the Rust backend
```

## Development

From the repository root:

```sh
npm run dev:backend-rust
```

The server reads `HTTP_ADDR` and `DATABASE_URL`.

Defaults:

```text
HTTP_ADDR=:18888
DATABASE_URL=sqlite://var/dev.sqlite
```
