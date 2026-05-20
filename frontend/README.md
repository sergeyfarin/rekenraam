# Frontend

SvelteKit frontend area.

## Responsibilities

- SvelteKit application source
- Static assets
- Frontend-only tests
- Static production build

## Development

Run the SvelteKit dev server from this folder. In development, configure the dev server to proxy API calls from `/api` to the Go backend.

## Production Build

Use SvelteKit static output. The production build should be copied into:

```text
backend/internal/web/dist/
```

The Go backend can then embed and serve those files from the final single binary.
