# Rekenraam

Rekenraam is a self-hosted, single-user personal finance app. It combines a
versioned double-entry ledger with accounts, imports, reconciliation, reports,
budgets, recurring transactions, investments, and backups. The Go backend serves
a static SvelteKit frontend from one binary; SQLite is the primary database.
Docker Compose runs the same app shape.

The app is still a pre-release candidate. Investment buy, sell, dividend,
reinvestment, and write-off workflows exist. Investment-native correction,
in-kind transfers, basis actions, and named short sales are still being built;
see the [R16 plan](docs/plans/investment-operation-refactor-plan.md) and the
[feature ledger](docs/implemented.md) for the exact boundary.

## Run locally

Use Go 1.27.2 or newer in the 1.27 series, Node 24, and the pinned pnpm version in `package.json`.
From the repository root:

```sh
pnpm install
pnpm dev
```

Open <http://localhost:1888>. The frontend development server proxies `/api`
to the backend on `127.0.0.1:16888`. The development server binds to localhost;
for a browser on another machine, use the SSH forward described in the
[developer workflow](docs/developer-workflow.md).

The unused v0.1 candidate schema has been redesigned for R16. If an older
**disposable development** database reports a migration checksum mismatch,
follow the [reset instructions](docs/developer-workflow.md#migrations-and-resetting-your-database).
Do not apply that reset to a database containing data you need to keep.

## Validate and build

```sh
./scripts/test-backend.sh      # Go formatting, vet, race tests (15m/package)
./scripts/test-frontend.sh     # generated API types, Svelte checks, unit tests
./scripts/test-e2e-smoke.sh    # integrated browser smoke suite
pnpm build                     # static frontend + embedded Go binary
```

The build writes `dist/rekenraam`. Use `./scripts/test-e2e.sh` for the full
browser suite and `pnpm test:release-preflight` for the serial release journeys.
Commands, test scope, and environment settings are in the
[developer workflow](docs/developer-workflow.md).

## Run the built app

Set a random `SETUP_TOKEN` of at least 32 characters before first production
startup. Set a durable
`REKENRAAM_SECRET_KEY` before using online connections or enrolling in two-factor
authentication; keep it outside the SQLite backup directory. The binary listens
on port `16888` by default. The provided Compose example binds only to
`127.0.0.1:16888`:

```sh
docker compose -f deploy/docker/compose.yaml up --build
```

For LAN or internet access, put the app behind HTTPS and follow the
[deployment security guide](docs/deployment-security.md). Before exposing real
financial data to the public internet, enrol the owner in two-factor
authentication. Use the [upgrade and restore guide](docs/upgrades.md) before
changing a populated installation. Local password recovery commands are in the
[developer workflow](docs/developer-workflow.md#local-owner-recovery).

## Repository layout

| Path | Purpose |
|---|---|
| `backend/` | Go server, application services, SQLite migrations, tests |
| `frontend/` | SvelteKit app compiled for Go embedding |
| `api/` | OpenAPI contract and Bruno requests |
| `e2e/` | Playwright browser journeys |
| `deploy/` | Docker build and Compose example |
| `scripts/` | Development, validation, and build commands |
| `docs/` | Product rules, decisions, plans, and current status |

Start with the [documentation map](docs/README.md). The
[roadmap](docs/roadmap.md) gives the work order and current focus,
[GitHub Issues](https://github.com/sergeyfarin/rekenraam/issues) tracks open
work by GitHub number, [backlog](docs/backlog.md) maps historical aliases, and
[implemented](docs/implemented.md) records what ships.
Product and architecture decisions live in the
[requirements](docs/product-requirements.md),
[conventions](docs/conventions.md), and [ADRs](docs/adrs/).
The [tracking policy](docs/conventions.md#work-tracking) defines issue readiness,
progress updates and closure; new work uses GitHub numbers without local IDs.

### Consolidated development schema

The T-148 return-of-capital quantity entitlement change (2026-10-07) revises
the pre-release baseline. Reset disposable development databases before using
it; see [database reset guidance](docs/developer-workflow.md#migrations-and-resetting-your-database).

No legacy databases exist. The pre-release schema is consolidated into
`0001_initial_schema.sql`, including source revisions, authoritative journal
links, separate immutable lot identity/mutable lot state, and immutable disposal
proceeds portions tied to clearing postings. Projected basis has explicit
known/unknown knowledge and nullable unknown amounts. The checksum and seeded equivalence
tests are updated together; see [the migration workflow](docs/developer-workflow.md#migrations-and-resetting-your-database).
