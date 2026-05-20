# Backend

Go backend area.

## Responsibilities

- HTTP API routes
- Application services
- SQLite access
- Database migrations
- Backend-only tests
- Serving embedded static frontend assets in production

## Suggested Layout

```text
cmd/rekenraam/          Future main package for the server binary
internal/api/           HTTP handlers, middleware, request/response DTOs
internal/app/           Application/service layer
internal/config/        Configuration loading and validation
internal/db/            SQLite connection and repository code
internal/web/dist/      Copied SvelteKit static build for Go embed
migrations/             SQLite schema migrations
testdata/               Fixtures for Go tests
var/                    Local dev database files, ignored by Git
```

## Development

Backend development should work without starting the frontend. During full-stack development, run the backend API server and let the SvelteKit dev server proxy `/api` requests to it.

## Testing

Backend tests should use temporary SQLite databases where possible. Keep real local database files in `var/`, which is ignored by Git.
