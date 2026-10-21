# Rekenraam

Rekenraam is a self-hosted, single-user personal finance app. It combines a
versioned double-entry ledger with accounts, imports, reconciliation, reports,
budgets, recurring transactions, investments, and backups. The Go backend serves
a static SvelteKit frontend from one binary; SQLite is the primary database.
Docker Compose runs the same app shape.

Scripts and tools can use read-only personal access tokens created under
Settings → Security. See the [token contract](docs/plans/personal-access-tokens-plan.md)
for grants, expiry and revocation, and the [deployment guidance](docs/deployment-security.md)
for HTTPS and credential handling.

The app is still a pre-release candidate. Investment buy, sell, dividend,
reinvestment, write-off, native correction, in-kind transfers, basis actions,
and named short sale/cover workflows ship within R16's agreed scope. Provider
import for additional operation kinds and other extensions remain tracked;
see the [R16 plan](docs/plans/investment-operation-refactor-plan.md) and the
[feature ledger](docs/implemented.md) for the exact boundary.
Gains and period statements can be saved as immutable operational reports and
reopened after corrections or price withdrawals; the [reporting policy](docs/plans/investment-reporting-plan.md#immutable-operational-runs-234) defines their scope.

## Run locally

Use Go 1.27.3 or newer in the 1.27 series, Node 24, and the pinned pnpm version in `package.json`.
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
./scripts/test-backend.sh      # Go formatting, vet, race tests (2 packages, 5m/package)
./scripts/test-frontend.sh     # catalog parity, generated API types, Svelte checks, unit tests
./scripts/test-e2e-smoke.sh    # provider stub HTTP tests + integrated browser smoke suite
pnpm build                     # static frontend + embedded Go binary
```

The build writes `dist/rekenraam`. Use `./scripts/test-e2e.sh` for the full
browser suite and `pnpm test:release-preflight` for the serial release journeys.
Run `pnpm --dir e2e run test:stubs` for just the provider stub HTTP regressions.
For the deliberate runtime OpenAPI audit, run
`python3 scripts/audit-api-contract.py --output /tmp/rekenraam-api-audit`;
[setup, sensitivity controls and retained findings](docs/developer-workflow.md#backend-validation)
are documented separately from the shared gates.
Commands, test scope, and environment settings are in the
[developer workflow](docs/developer-workflow.md).
The [runtime-image audit](docs/reviews/runtime-image-advisory-audit-2026-10-20.md)
and [bundled SQLite review](docs/reviews/native-sqlite-advisory-audit-2026-10-20.md)
record advisory evidence, retained findings and release-disposition limits.
The [SQLite upgrade prerequisite check](docs/reviews/sqlite-driver-upgrade-blocker-2026-10-20.md)
records the fixed-engine diagnostic and the still-missing released driver.

SQLite uses `github.com/mattn/go-sqlite3` with bundled C source (ADR 0014).
Financial writes use WAL with FULL commit durability consistently in runtime
and tests ([ADR 0015](docs/adrs/0015-sqlite-commit-durability.md)).
Backend commands require CGO and a C compiler with development headers:
`sudo apt install build-essential` on Debian/Ubuntu. The shared wrappers enable
FTS5, `netgo` and `osusergo`; use `./scripts/go-backend.sh test ...` for focused
Go checks. Standalone binaries need a compatible libc; Docker supplies the
Debian trixie runtime. See the [developer workflow](docs/developer-workflow.md).

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

The build excludes local data and environment files and updates Debian runtime
packages. Maintainers can check the build-context boundary with
`bash scripts/test-docker-context.sh`; the disposable install/upgrade/rollback
drill is documented in the [developer workflow](docs/developer-workflow.md#deployment-audit).

Optional quote adapters use operator environment keys `COINGECKO_DEMO_API_KEY`
and `TWELVE_DATA_API_KEY`. Configure instrument identifiers and quote assignments
in Investments → Instruments; the [quote/crypto contract](docs/plans/quote-providers-crypto-plan.md)
records provider entitlements, unofficial Yahoo limits and crypto CSV entry.
With a Twelve Data plan that includes dividends, Investments → Suggestions →
**Check provider dividends** stages held ex-dates for review; nothing posts until
you complete or link each one ([contract](docs/plans/connections-plan.md#provider-dividend-producer-111-delivered-2026-10-17)).

For LAN or internet access, put the app behind HTTPS and follow the
[deployment security guide](docs/deployment-security.md). Before exposing real
financial data to the public internet, enrol the owner in two-factor
authentication. Use the [upgrade and restore guide](docs/upgrades.md) before
changing a populated installation. Local password recovery commands are in the
[developer workflow](docs/developer-workflow.md#local-owner-recovery).

## Backups and exports

Rekenraam keeps two different kinds of copy, and they are not interchangeable:

- **Backups** (Settings → Data → Backups) are the only complete copy of your
  data: the ledger, investments, budgets, recurring transactions,
  reconciliations, settings and history. The app takes one nightly and on
  request. To recover, or to move to another server, restore one with
  `rekenraam restore --from <backup>`, following the
  [upgrade and restore guide](docs/upgrades.md). Keep `REKENRAAM_SECRET_KEY`
  separately; it is deliberately not in the backup.
- **Exports** (Settings → Data → Export: archive, ledger CSV or QIF) are plain
  files for reading your figures in spreadsheets and other finance or
  accounting tools. Every balance, lot and gain can be recomputed from the
  archive, and its manifest lists what it leaves out. An export **cannot be
  imported back** into Rekenraam, and it does not contain budgets, recurring
  transactions, saved reconciliations or settings. Use a backup for that.

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
