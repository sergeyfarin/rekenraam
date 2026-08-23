# Developer Workflow

This document defines default development workflow conventions for contributors and coding agents.

## Source Of Truth

Before changing durable behavior, read:

- `docs/product-requirements.md`
- `docs/conventions.md`
- `docs/early-architecture-decisions.md`
- `docs/adrs/`
- `docs/localization-glossary.md` — before adding or translating a domain term.

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

The script resolves gofmt as `"$(go env GOROOT)/bin/gofmt"` rather than trusting
PATH. Run it that way by hand too: a version manager pinning an older Go leaves
its gofmt first on PATH even while the `go` command switches to the newer
toolchain `go.mod` requires, and gofmt's output is not stable across releases,
so a bare `gofmt` both misses drift CI will reject and flags files that are
correctly formatted for the toolchain actually compiling them.

```sh
./scripts/test-backend.sh
```

`COVERAGE=1 ./scripts/test-backend.sh` runs the same suite without `-race`,
writes `backend/coverage.out`, and prints the merged total. CI runs this in a
non-gating `backend-coverage` job and fails it below a soft floor
(`scripts/check-coverage-floor.sh`, `COVERAGE_FLOOR` default 73.0%) — raise
the floor deliberately when the level rises; never lower it to make a change
pass.

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

`REKENRAAM_SECRET_KEY` encrypts online provider credentials **and the two-factor
shared secret** at rest; without it, both connection creation and MFA enrolment
return `CONFIG_REQUIRED` rather than storing anything in the clear. It must be
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

Run just the fast journeys — everything except the serial release-preflight
suite. This is what CI runs on every push:

```sh
./scripts/test-e2e-smoke.sh
```

Run the release preflight when preparing a release:

```sh
pnpm test:release-preflight
```

- `./scripts/test-e2e.sh` now builds the integrated app, starts a fresh local instance on `127.0.0.1:16889`, and uses a dedicated SQLite file at `backend/var/e2e.sqlite` unless `E2E_BASE_URL` is set.
- The Playwright suite runs with one worker because the default harness shares one app instance and SQLite database.
- Set `E2E_PORT` when the self-managed e2e port needs to move.
- Set `E2E_BASE_URL` when you want Playwright to target an already-running app instead of booting its own fresh instance.
- The harness sets a throwaway `REKENRAAM_SECRET_KEY` for the instance it boots, because the MFA journey cannot enrol without one — enrolment returns `CONFIG_REQUIRED` rather than storing the shared secret in the clear. Export your own `REKENRAAM_SECRET_KEY` to override it.
- `mfa.spec.ts` shares the run's database with every other spec, so it turns MFA back off in `afterAll`. A spec that changes account-wide authentication state must clean up the same way.
- Set `PLAYWRIGHT_CHROMIUM_EXECUTABLE` to an existing Chromium binary when the sandbox or image cannot download the revision this Playwright release pins (`pnpm exec playwright install` fails, and the run dies with "Executable doesn't exist"). Container images that preinstall a browser usually expose one at `/opt/pw-browsers/chromium`. Prefer this over patching the browser cache by hand; leave it unset locally so Playwright uses its own pinned build.

## Area Notes

- Backend code lives in `backend/`; run `go test ./...` there directly only when you intentionally want to bypass the wrapper script.
- The backend binary also exposes a local maintenance command, `recover-owner`, for backup-first password recovery and session revocation.
- Frontend code lives in `frontend/`; SvelteKit builds static output that is copied into `backend/internal/web/dist/` for the single binary.
- Frontend foundation libraries are pinned in `frontend/package.json`: Tailwind CSS, Bits UI, shadcn-svelte, Paraglide JS, `@tanstack/svelte-query`, `@tanstack/svelte-table`, `openapi-typescript`, `openapi-fetch`, `date-fns`, and Dinero.js. Add TanStack Virtual only when a concrete screen needs it.
- The frontend stays on **TypeScript 6**. TypeScript 7 (the native port) drops the JS compiler API that `openapi-typescript` builds `src/lib/api/schema.d.ts` with — `openapi-typescript` still declares `peerDependencies.typescript: ^5.x`, and under TS 7 `pnpm run openapi:generate` dies with `Cannot read properties of undefined (reading 'createKeywordTypeNode')`. See backlog T-42.
- Frontend message catalogs live in `frontend/messages/`, split by domain as `frontend/messages/<domain>/<locale>.json`. Keep message IDs flat and prefixed by domain or screen intent instead of using one growing locale file or deep nested objects. Generated Paraglide output lives in `frontend/src/lib/paraglide/`. Regenerate the typed message layer with `pnpm --dir frontend run paraglide:compile` when changing catalog structure outside the normal `dev`, `check`, or `build` scripts.
- Shipping locales are `en, es, fr, nl, de, ru`, listed in `frontend/project.inlang/settings.json`. Both message directories compile into the single `m` namespace, so **a key must be unique across `messages/app/` and `messages/settings/`** — which file it lives in is grouping, not namespacing.
- Adding an English string is safe on its own: a locale missing that key falls back to English per message rather than rendering blank. Adding a *term* is not — put it in `docs/localization-glossary.md` first, because a plausible-but-wrong financial term is worse than English.
- **Locale resolution is configured in two places that must agree**: `strategy` in `frontend/vite.config.ts` and the `--strategy` flag on the `paraglide:compile` script in `frontend/package.json`. Both are `localStorage,preferredLanguage,baseLocale`. If they drift, the plugin's compile usually wins and hides the mistake — until something skips it and the app silently ships English only.
- A quick check that the catalogs are whole: compare each locale's key set and placeholder set against `en.json`. `e2e/playwright/language.spec.ts` covers the switcher end to end and deliberately asserts one string from *each* catalog, since a missing file would otherwise show up only as English on half a screen.
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
- `e2e/playwright/release-preflight.spec.ts` is the local release preflight for critical financial workflows. It is intentionally serial against one fresh SQLite database and is not part of fast CI yet. Because it is `describe.serial`, a failure in the first test skips the rest of the file — check "did not run" counts before concluding a journey passed.
- **Never hard-code a calendar date for a transaction, statement, or imported row in an e2e spec.** Everything a spec seeds is versioned as effective from *today*, and posting validation resolves account and commodity rules as of the entry date, so a date in the past relative to that seeded data matches no version and the posting is rejected. Use `todayISO()` / `todayQIF()` from `e2e/playwright/support/dates.ts`. A hard-coded `2026-07-08` silently broke every browser transaction-entry journey once the wall clock passed it (fixed 2026-08-06).
- **Assert a saved transaction with `expectSavedTransaction()`, not a bare `getByText(description)`.** Saving opens a detail panel, so the description can be on screen three times at once and a loose text locator fails Playwright strict mode — a failure that reads as "not saved" when the opposite is true. Note also that the list row shows the *payee* instead of the description when there is one, so asserting against the row alone is not portable either.
- Docker assets live in `deploy/docker/` and must preserve the same single-app production shape as the binary.
- `backend/var/`, `backend/internal/web/dist/`, and `dist/` contain local or generated files. Their README files are placeholders that keep those ignored directories present in Git.

## Default Change Workflow

1. Read the active requirements and conventions for the touched area.
2. Make the smallest durable change that satisfies the slice.
3. Run the narrowest relevant validation.
4. Update docs or ADRs if the change introduces a durable new rule.
5. Keep commits focused.

## Migrations And Resetting Your Database

The governing rule is **Project Lifecycle And Migration Immutability** in
`docs/conventions.md`; this section is only how to carry it out. Short version:
Rekenraam is pre-release, so migrations may still be rewritten and your local
database is disposable.

Adding schema:

```sh
# new sequential file, goose format (-- +goose Up / -- +goose Down)
$EDITOR backend/migrations/00NN_short_name.sql
./scripts/test-backend.sh
```

Resetting your database after someone rewrote a migration (or after your own
rewrite) — migrations run at startup, so deleting the file is the whole reset:

```sh
rm -f backend/var/rekenraam.sqlite backend/var/rekenraam.sqlite-shm backend/var/rekenraam.sqlite-wal
```

Then start the app and redo owner setup. The e2e database resets itself on every
run and needs nothing.

- Keep nothing in a local database you would be sorry to lose. Reproducible
  fixtures belong in the repo; `backend/var/*.sqlite` is scratch.
- Rewriting an already-committed migration requires `BREAKING DEV DATABASE` as
  the first line of the commit body, plus the reset instruction for other
  developers. Grep for past ones with `git log --grep='BREAKING DEV DATABASE'`.
- Two branches that both added `00NN_` collide. The one merged second renumbers
  **its own** file to the next free number while resolving the merge. Renumbering
  a migration already on `main` counts as a rewrite.
- CI validates the fresh-install path on every run, because every job migrates
  from an empty database. There is no historical-upgrade test yet; it arrives
  with the `v0.1.0` freeze.

## Commit Conventions

This repo uses Conventional Commits.

Preferred examples:

- `feat(backend): add opening balance endpoint`
- `feat(frontend): add account list empty state`
- `fix(api): validate unbalanced postings`
- `docs(requirements): lock export scope`
- `test(backend): cover reconciliation lock behavior`

Use the smallest honest scope. Avoid mixing unrelated concerns in one commit.

A commit that rewrites an already-committed migration must additionally start
its body with `BREAKING DEV DATABASE` — see *Migrations And Resetting Your
Database* above.

## Branches And PRs

**Commit directly to `main`.** This is the default for all work, including
changes to product rules, conventions, workflows, and CI. Do not open a feature
branch unless there is a specific reason to (owner decision, 2026-08-23).

Why: the project is pre-release with a single contributor, so there is no
release to protect and no second reviewer for a PR to reach. Branches bought
nothing and cost something — the 2026-08 merge of two long-diverged branches is
where the duplicated `docs/conventions.md` paragraph, the duplicated
`writePricingServiceError` arms, the duplicate CodeQL workflow, and the
colliding T-42–T-47 backlog IDs all came from.

What replaces the PR as the safety net:

- **`main` stays green.** Run the narrowest relevant validation before pushing
  (see *Validation Expectations By Area* below). CI runs the same scripts, so a
  local failure is a push that should not happen.
- **Commits stay focused and honest.** The commit message is now the only
  review artifact, so it carries the reasoning a PR summary would have. A commit
  that changes product requirements, conventions, or ADRs says so in its body.
- **Push often.** Long-lived local work recreates exactly the divergence this
  rule exists to avoid.

Branch when the work genuinely needs isolation — a spike you may throw away, or
a change you want CI to run before it reaches `main`. That is a judgement call,
not a default.

**This flips at the `v0.1.0` release**, the same milestone that freezes
migrations (see *Project Lifecycle And Migration Immutability* in
`docs/conventions.md`). From that tag, feature branches and PRs become the norm:
once a released version exists, `main` is something users can be running.

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
- CodeQL code scanning runs in `.github/workflows/codeql.yml`. It is an
  *advanced* setup — a committed workflow rather than GitHub's default setup —
  because default setup gives no control over the Go toolchain: it builds with
  whatever the runner preinstalls under `GOTOOLCHAIN=local`, so a `go.mod`
  above that version fails extraction and takes every other language in the run
  down with it. This workflow instead runs `actions/setup-go` with
  `go-version-file: backend/go.mod`. Reverting to default setup reintroduces
  that failure mode. Advanced setup was enabled in repository settings on
  2026-08-23; the two are mutually exclusive, so while default setup is on, an
  advanced workflow's results are rejected on upload with "CodeQL analyses from
  advanced configurations cannot be processed when the default setup is
  enabled". Keep exactly one CodeQL workflow file — enabling advanced setup
  through the GitHub UI offers to add its own `codeql.yml` and will create a
  second file (`codeql2.yml`) if that name is taken, which then runs duplicate
  analyses and uploads conflicting SARIF for the same categories.
- **CodeQL's `go` matrix entry is paused** (2026-08-23). The Go extractor
  bundled with CodeQL is itself built with Go 1.26 and cannot parse a 1.27
  module — every backend package fails with "package requires newer Go version
  go1.27 (application built with go1.26)", yielding a partial database and a
  red job. Go supports no `build-mode: none` fallback, and 2.26.3 was already
  the newest bundle. `govulncheck.yml` covers known dependency CVEs meanwhile.
  The upstream fix is already merged — github/codeql#22042 "Go: Update to 1.27"
  (2026-08-20), tracked by github/codeql#22394, where a CodeQL maintainer put it
  in "the next release". It missed the 2.26.3 bundle (2026-08-12), so it lands
  in 2.26.4; bundles ship roughly fortnightly, putting this around early
  September 2026. To restore: check `gh api
  repos/github/codeql-cli-binaries/releases --jq '.[0].tag_name'` for >= 2.26.4,
  then uncomment the two `go` matrix lines and confirm the job goes green.
- Dependabot version updates are configured in `.github/dependabot.yml` for
  GitHub Actions, the backend Go module, root/frontend pnpm packages, and the
  Docker runtime image.
- Browser journeys run in the `E2E Smoke` job of `ci.yml` on every push and pull
  request: `./scripts/test-e2e-smoke.sh`, which builds the single binary, boots
  a throwaway instance, and runs every spec except `release-preflight.spec.ts`.
  The Playwright report uploads as an artifact on failure. The preflight suite
  stays out of CI and is run deliberately before a release.
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
