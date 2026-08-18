# Developer Workflow

This document defines default development workflow conventions for contributors and coding agents.

## Source Of Truth

Before changing durable behavior, read:

- `docs/product-requirements.md`
- `docs/conventions.md`
- `docs/early-architecture-decisions.md`
- `docs/adrs/`

## Daily Commands

Run from repo root unless noted otherwise.

### Install

```sh
pnpm install
```

### Development

```sh
pnpm dev
pnpm dev:backend
pnpm dev:frontend
```

### Backend Validation

Checks formatting (`gofmt -l`), runs `go vet ./...`, then the full Go suite with
the race detector. The formatting and vet gates live inside the wrapper script
so CI enforces them too — before they were added, `gofmt` drift could sit in the
tree with a green pipeline (backlog T-43).

```sh
./scripts/test-backend.sh
```

### Local Owner Recovery

Reset the owner password locally with a verified SQLite backup first:

```sh
printf '%s\n' 'new-password' | DATABASE_URL=file:backend/var/dev.sqlite go run ./backend/cmd/rekenraam recover-owner --password-stdin
```

Write the verified backup to an explicit path when needed:

```sh
printf '%s\n' 'new-password' | DATABASE_URL=file:backend/var/dev.sqlite go run ./backend/cmd/rekenraam recover-owner --password-stdin --backup-path dist/recovery.sqlite
```

Only use the emergency override when backup creation or verification is impossible and you accept the risk of proceeding without a fresh verified backup:

```sh
printf '%s\n' 'new-password' | DATABASE_URL=file:backend/var/dev.sqlite go run ./backend/cmd/rekenraam recover-owner --password-stdin --allow-no-backup
```

### Online Provider Secret Key

`REKENRAAM_SECRET_KEY` encrypts online provider credentials at rest. It must be
base64 for exactly 32 random bytes:

```sh
openssl rand -base64 32
```

Keep the value outside Git in the service environment or deployment secret
manager, and back it up with the SQLite database. If the key is lost, existing
online connection secrets cannot be decrypted. Restore the old key from backup,
or stop the app, take and verify a SQLite backup, start with a new key, delete
the affected online import connections, and re-add them with fresh provider
credentials. Existing imported ledger data and import history remain durable.

There is no in-place secret-key rotation command yet. Intentional rotation uses
the same backup-first delete-and-re-add procedure for stored online import
connections.

### Frontend Validation

Runs SvelteKit checks and the Vitest unit suite.

```sh
./scripts/test-frontend.sh
```

### Integrated Build

```sh
pnpm build
```

### E2E

```sh
./scripts/test-e2e.sh
E2E_BASE_URL=http://localhost:16888 ./scripts/test-e2e.sh
```

Run the release preflight when preparing a release:

```sh
pnpm test:release-preflight
```

- `./scripts/test-e2e.sh` now builds the integrated app, starts a fresh local instance on `127.0.0.1:16889`, and uses a dedicated SQLite file at `backend/var/e2e.sqlite` unless `E2E_BASE_URL` is set.
- The Playwright suite runs with one worker because the default harness shares one app instance and SQLite database.
- Set `E2E_PORT` when the self-managed e2e port needs to move.
- Set `E2E_BASE_URL` when you want Playwright to target an already-running app instead of booting its own fresh instance.
- Set `PLAYWRIGHT_CHROMIUM_EXECUTABLE` to an existing Chromium binary when the sandbox or image cannot download the revision this Playwright release pins (`pnpm exec playwright install` fails, and the run dies with "Executable doesn't exist"). Container images that preinstall a browser usually expose one at `/opt/pw-browsers/chromium`. Prefer this over patching the browser cache by hand; leave it unset locally so Playwright uses its own pinned build.

## Area Notes

- Backend code lives in `backend/`; run `go test ./...` there directly only when you intentionally want to bypass the wrapper script.
- The backend binary also exposes a local maintenance command, `recover-owner`, for backup-first password recovery and session revocation.
- Frontend code lives in `frontend/`; SvelteKit builds static output that is copied into `backend/internal/web/dist/` for the single binary.
- Frontend foundation libraries are pinned in `frontend/package.json`: Tailwind CSS, Bits UI, shadcn-svelte, Paraglide JS, `@tanstack/svelte-query`, `@tanstack/svelte-table`, `openapi-typescript`, `openapi-fetch`, `date-fns`, and Dinero.js. Add TanStack Virtual only when a concrete screen needs it.
- The frontend stays on **TypeScript 6**. TypeScript 7 (the native port) drops the JS compiler API that `openapi-typescript` builds `src/lib/api/schema.d.ts` with — `openapi-typescript` still declares `peerDependencies.typescript: ^5.x`, and under TS 7 `pnpm run openapi:generate` dies with `Cannot read properties of undefined (reading 'createKeywordTypeNode')`. See backlog T-42.
- Frontend message catalogs live in `frontend/messages/`, split by domain as `frontend/messages/<domain>/<locale>.json`. Keep message IDs flat and prefixed by domain or screen intent instead of using one growing locale file or deep nested objects. Generated Paraglide output lives in `frontend/src/lib/paraglide/`. Regenerate the typed message layer with `pnpm --dir frontend run paraglide:compile` when changing catalog structure outside the normal `dev`, `check`, or `build` scripts.
- Multi-state Svelte routes should stay thin route entrypoints. Move reusable workflow UI and route-specific helpers into feature folders under `frontend/src/lib/`, as with `frontend/src/lib/install-gate/`.
- Frontend OpenAPI types are generated from `api/openapi/openapi.yaml` into `frontend/src/lib/api/schema.d.ts`. The root OpenAPI file is only the entrypoint: keep top-level metadata, global components, and `$ref` registrations there rather than inline path or schema bodies.
- Add new API endpoints by creating or extending a path item under `api/openapi/paths/`, then referencing that file from `api/openapi/openapi.yaml` under `paths`. Keep each path item focused on one route pattern such as `institutions.yaml` or `institution-versions.yaml`.
- Add or extend schema groups under `api/openapi/components/schemas/`, then expose individual schemas through the root `components.schemas` map. Path files should reference shared schemas through `../openapi.yaml#/components/schemas/...`; schema group files should reference siblings through `../../openapi.yaml#/components/schemas/...`.
- Regenerate frontend types with `pnpm --dir frontend run openapi:generate` when changing the contract outside the normal `dev`, `check`, or `build` scripts.
- Keep the OpenAPI client seam language-agnostic. Do not embed user-facing English fallbacks in `frontend/src/lib/api/`; translate API status and error presentation at the UI layer with Paraglide using stable error codes or screen-specific fallback copy.
- API examples and contract assets live in `api/`; use the Bruno `local` environment for the backend dev server and `app` for an integrated binary or Docker app.
- Login-created sessions default to 30 days. Set `SESSION_LIFETIME_HOURS` to a positive integer number of hours when testing shorter or longer session expiry behavior.
- Production generates a one-time setup token when `SETUP_TOKEN` is absent; operators should set a durable random token of at least 32 characters and enter it only for the first owner-creation request. See `docs/deployment-security.md`.
- Forwarded proxy headers are ignored by default. Set `TRUST_PROXY_HEADERS=1` only when the app is behind a trusted reverse proxy that rewrites those headers, and set `TRUSTED_PROXY_CIDRS` to the proxy source ranges that are allowed to supply them.
- Browser e2e tests live in `e2e/` and use Playwright. Keep them focused on user journeys that need a browser.
- `e2e/playwright/release-preflight.spec.ts` is the local release preflight for critical financial workflows. It is intentionally serial against one fresh SQLite database and is not part of fast CI yet.
- Docker assets live in `deploy/docker/` and must preserve the same single-app production shape as the binary.
- `backend/var/`, `backend/internal/web/dist/`, and `dist/` contain local or generated files. Their README files are placeholders that keep those ignored directories present in Git.

## Default Change Workflow

1. Read the active requirements and conventions for the touched area.
2. Make the smallest durable change that satisfies the slice.
3. Run the narrowest relevant validation.
4. Update docs or ADRs if the change introduces a durable new rule.
5. Keep commits focused.

## Commit Conventions

This repo uses Conventional Commits.

Preferred examples:

- `feat(backend): add opening balance endpoint`
- `feat(frontend): add account list empty state`
- `fix(api): validate unbalanced postings`
- `docs(requirements): lock export scope`
- `test(backend): cover reconciliation lock behavior`

Use the smallest honest scope. Avoid mixing unrelated concerns in one commit.

## Branches And PRs

- Feature branches are encouraged, not mandatory.
- PRs are encouraged for reviewable changes, especially when they affect product rules, workflows, or CI.
- Direct small fixes are acceptable when the change is obvious and low risk.
- If a PR changes product requirements, conventions, or ADRs, mention that explicitly in the PR summary.

## Validation Expectations By Area

### Backend

- Usually validate with `./scripts/test-backend.sh` — it gates on `gofmt -l`, then runs `go vet ./...` and `go test -race ./...`.
- If backend changes affect the integrated binary shape, also run `pnpm build`.

### Frontend

- Usually validate with `./scripts/test-frontend.sh`.
- If the change affects static build output, also run `pnpm build`.

### Docs And Requirements

- Keep cross-references and commands consistent.
- Run `git diff --check -- ...` for touched docs when no narrower executable validation exists.

### Testing

- Treat local validation scripts as the development contract.
- CI commands must match the local wrapper scripts exactly.

## GitHub Actions

The repo uses a fast CI workflow in `.github/workflows/ci.yml`.

Workflow conventions:

- Fast CI covers race-detected backend tests, frontend checks plus Vitest, and
  the integrated build.
- Go vulnerability scanning runs in `.github/workflows/govulncheck.yml` with
  `govulncheck ./...` from `backend/` on a weekly schedule, manually via
  `workflow_dispatch`, and on backend-affecting pull requests.
- Dependabot version updates are configured in `.github/dependabot.yml` for
  GitHub Actions, the backend Go module, root/frontend pnpm packages, and the
  Docker runtime image.
- E2E stays in a separate workflow and runs only when a real user journey exists.
- Use `pnpm install --frozen-lockfile` in CI.
- Keep CI commands aligned with the local wrapper scripts in `scripts/`.
- Use Node 22.
- Use the Go version declared in `backend/go.mod`.
- Keep job structure simple and readable.
- If workflows are reintroduced, start with manual triggers before considering automatic gates.

## Documentation Update Rules

- Update `docs/product-requirements.md` when scope or durable product behavior changes.
- Update `docs/conventions.md` when a repo-wide rule changes.
- Add or update an ADR in `docs/adrs/` when a decision locks a long-lived tradeoff.
- Update `README.md` and this document when developer commands, repo layout, or workflow expectations change. Avoid reintroducing area README files for short command notes; use them only when an area needs substantial local guidance.

## Archive Rules

- `.archive/` is reference only.
- Do not port archive implementation details directly.
- If an archive idea is adopted, rewrite it in current-stack terms and add it to active docs.
