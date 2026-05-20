# Scripts

Workflow scripts can live here once the concrete commands are known.

Root npm scripts now provide:

- `npm run dev`: run backend and frontend together with labeled logs
- `npm run test:backend`: run Go backend tests
- `npm run test:frontend`: run SvelteKit checks
- `npm run build`: build the single binary

Suggested future scripts:

- `dev-frontend`: run SvelteKit with `/api` proxying to the backend
- `build-frontend`: build SvelteKit static output
- `sync-frontend`: copy static frontend output into `backend/internal/web/dist/`
- `build-binary`: compile the final Go binary into `dist/`
- `test-backend`: run Go tests
- `test-api`: run Bruno CLI requests
- `test-e2e`: run browser e2e tests
