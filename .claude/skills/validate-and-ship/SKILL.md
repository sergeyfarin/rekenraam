---
name: validate-and-ship
description: How to run, debug, validate, review, commit, and document changes in Rekenraam - command matrix, dev environment, docs-update rules, review checklist of this repo's recurring bug classes. Use before committing any change, when running the app, or when deciding which doc to update.
---

# Validate And Ship

## Run the app

```sh
pnpm install                 # once, repo root
pnpm dev                     # backend :16888 + frontend :1888 (proxies /api)
pnpm dev:backend             # go run, APP_ENV=development
pnpm dev:frontend
```
Open http://localhost:1888. Dev SQLite lives at `backend/var/dev.sqlite`
(git-ignored). Key env vars: `HTTP_ADDR`, `DATABASE_URL` (`file:...`),
`APP_ENV` (`development`|`production`, defaults production),
`REKENRAAM_SECRET_KEY` (base64 32 bytes; required for import connections),
`TRUST_PROXY_HEADERS` + `TRUSTED_PROXY_CIDRS`, `OPEN_EXCHANGE_RATES_APP_ID`.
Owner password reset: `recover-owner` command (see README § Local Owner
Recovery) — it backs up and revokes sessions; never edit the users table.

## Validation matrix (narrowest first — this is the contract; CI runs the same scripts)

| Changed | Run |
|---|---|
| `backend/**` | `./scripts/test-backend.sh` (= `go test -race ./...`), plus `go vet ./...`, `gofmt -l .` in `backend/` |
| Backend coverage check (optional, same script) | `COVERAGE=1 ./scripts/test-backend.sh` — non-race coverage pass, prints the merged total; CI enforces a soft floor (`scripts/check-coverage-floor.sh`) |
| `frontend/**` | `./scripts/test-frontend.sh` (openapi:generate + paraglide:compile + svelte-check); `pnpm --dir frontend run test` for unit-tested logic |
| OpenAPI | both scripts above |
| Integrated shape / static serving / embed | `pnpm build` (builds frontend, copies into `backend/internal/web/dist/`, runs embed test, compiles `dist/rekenraam`) |
| User journeys | `./scripts/test-e2e.sh` (self-contained: builds app, boots on 127.0.0.1:16889, fresh `backend/var/e2e.sqlite`) |

Never invent parallel validation commands; if a command must change, change
the script and CI together and update `docs/developer-workflow.md`.

## Review checklist — this repo's recurring bug classes

Every one of these shipped as a real bug here at least once. Check them on any
non-trivial diff (yours or reviewed):

1. **Silent limit clamping** — internal full-set reads using a paginated repo
   method whose limit gets clamped (import commit processed only 200 of 201
   rows). Internal reads use explicit `ListAll*` methods.
2. **PATCH omission overwrites** — optional update fields must be pointer
   types end-to-end; a plain `bool` made every rename silently disable
   auto-refresh.
3. **TOCTOU guard races** — check + insert in separate statements/transactions.
4. **Split-transaction crash holes** — a row and its idempotency marker written
   in different transactions (open item T-06).
5. **Cursor boundaries** — `<=` vs `<`, resume token vs incremental boundary
   (see `background-work`).
6. **Unconsumed pagination** — frontend fetching page one and ignoring
   `next_cursor` (T-05).
7. **Reconciliation guard bypass** — any new mutation path over postings must
   be guarded (see `ledger-invariants`).
8. **Error-envelope drift** — new error codes not added to the OpenAPI enum;
   raw Go errors leaking to clients.
9. **i18n bypass** — hard-coded English in UI or in `lib/api/`.
10. **Logging financial content** — forbidden at every level.
11. **Builder-output tests that never reach the real consumer** — a test that
    checks a spec-building function's *return value* in isolation (e.g.
    `buildTransactionSpec`) is not the same as proving it survives the
    consumer's real validation. `EntryKind: "main"` sat in
    `buildTransactionSpec` since the import feature's first commit — invalid
    per `entryKinds`, so every `CommitImportBatch` call failed the instant it
    reached `TransactionService.CreateTransaction` for real — undetected
    because no test drove a staged row through the *actual* commit path
    against a real account (T-22). When testing a function that produces
    input for another service, add at least one test that calls the
    consumer for real, not just asserts on the producer's output shape.

Fix workflow for any bug: failing named test first, then the fix, then the
full relevant suite.

## Docs to update in the same change (the docs ARE the product memory)

| What changed | Update |
|---|---|
| Feature shipped / status changed | `docs/implemented.md` (feature ledger) and `docs/roadmap.md` status line |
| Tech debt found or paid | `docs/backlog.md` — numbered item (T-NN), exact file/line, and when closing: what fixed it + which test proves it |
| Durable product behavior/scope | `docs/product-requirements.md` |
| Repo-wide rule/convention | `docs/conventions.md` |
| Long-lived tradeoff decision | new ADR in `docs/adrs/` (ADRs supersede everything once accepted) |
| Commands/layout/workflow | `README.md` + `docs/developer-workflow.md` (keep both consistent) |

Precedence when docs conflict: product-requirements → conventions →
early-architecture-decisions → ADRs govern all → developer-workflow.
`AGENTS.md` and skills are guidance, not product sources of truth.
`.archive/` is historical reference only — never port from it directly.

## Commits

Conventional Commits, smallest honest scope:
`feat(backend): ...`, `fix(api): ...`, `docs(requirements): ...`,
`test(backend): ...`. Don't mix unrelated refactors. Feature branches and PRs
encouraged, not mandatory; direct small fixes are fine when obvious and low
risk. If a change alters product rules, conventions, or ADRs, say so
explicitly in the PR/commit summary.

## Definition of done for a slice

1. App still runnable end-to-end (`pnpm dev` or `pnpm build`).
2. Narrowest relevant validation green; new behavior has named tests.
3. OpenAPI + generated types in sync (if API touched).
4. Docs updated per the table above.
5. No violation of `ledger-invariants` (if money/ledger touched).
6. Focused conventional commit.
