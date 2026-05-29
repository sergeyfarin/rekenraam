# Copilot Instructions

This repository builds a self-hosted personal finance app with a Go backend, SvelteKit frontend, SQLite persistence, and a single-binary production shape.

## Read First

Before making durable product, architecture, or workflow changes, read:

- `docs/product-requirements.md`
- `docs/conventions.md`
- `docs/early-architecture-decisions.md`
- `docs/adrs/`
- `docs/feature-roadmap.md`
- `docs/developer-workflow.md`
- `AGENTS.md`

In case of conflict, earlier items in the list take precedence over later items. Accepted ADRs supersede all other documents.

## Default Working Rules

- Deliver incrementally, one feature slice at a time.
- Keep the app runnable after each change.
- Treat `.archive/` as historical input only.
- Do not copy archived Python, Rust, or Tauri implementation details forward without translating them into current-stack requirements.
- Use `pnpm` for JavaScript tasks.
- Prefer minimal durable changes over speculative abstractions.
- When a change introduces a durable new rule, update the relevant docs in the same change.

## Financial And Product Invariants

- Preserve a double-entry-capable ledger model.
- Never store financial values as floating point.
- Do not hard-delete posted financial records.
- Prefer void, archive, and corrective-entry workflows for durable changes.
- Keep the production shape as one Go app serving the static frontend.
- Keep the product explicitly single-user until an ADR changes that scope.
- First-run setup creates the single owner with a username and password.
- Built-in database labels must use stable keys/codes and resolve localized display text outside durable financial data.
- First export milestone must include core ledger CSV and QIF.
- Attachments are out of scope until the active requirements say otherwise.

## Validation Defaults

- After code changes, run the narrowest relevant validation first.
- Backend changes usually validate with `./scripts/test-backend.sh`.
- Frontend changes usually validate with `./scripts/test-frontend.sh`.
- Integrated build changes usually validate with `pnpm build`.
- E2E and workflow changes should prefer the smallest reproducible verification available.

## Collaboration Defaults

- Use Conventional Commits.
- Lightweight feature branches and PRs are encouraged, not mandatory.
- Keep commit scope focused; do not bundle unrelated refactors.
