# Scripts

Workflow scripts can live here once the concrete commands are known.

Root pnpm scripts now provide:

- `pnpm dev`: run backend and frontend together with labeled logs
- `pnpm test:backend`: run Go backend tests
- `pnpm test:frontend`: run SvelteKit checks
- `pnpm test:e2e`: run Playwright tests
- `pnpm build`: build the single binary

Stable script wrappers now provide:

- `./scripts/test-backend.sh`: backend test entrypoint used by local validation and CI
- `./scripts/test-frontend.sh`: frontend check entrypoint used by local validation and CI
- `./scripts/test-e2e.sh`: e2e entrypoint used by local validation and CI

Suggested future scripts:

- `dev-frontend`: run SvelteKit with `/api` proxying to the backend
- `build-frontend`: build SvelteKit static output
- `sync-frontend`: copy static frontend output into `backend/internal/web/dist/`
- `build-binary`: compile the final Go binary into `dist/`
- `test-api`: run Bruno CLI requests
