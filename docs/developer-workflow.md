# Developer Workflow

This document defines default development workflow conventions for contributors and coding agents.

## Source Of Truth

Before changing durable behavior, read:

- `docs/product-requirements.md`
- `docs/conventions.md`
- `docs/early-architecture-decisions.md`
- `docs/feature-roadmap.md`
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

```sh
./scripts/test-backend.sh
```

### Frontend Validation

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

- Usually validate with `./scripts/test-backend.sh`.
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

- Fast CI covers backend tests, frontend check, and integrated build.
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
- Update README or area-specific READMEs when developer commands or workflows change.

## Archive Rules

- `.archive/` is reference only.
- Do not port archive implementation details directly.
- If an archive idea is adopted, rewrite it in current-stack terms and add it to active docs.
