# Deployment and distribution choice review — 2026-11-03

**Status:** Analysis only. This document reviews self-hosted, desktop, managed
and SaaS directions; it does not change the current product decision. The
accepted direction remains self-hosted first, with desktop as a separately
gated optional host and a first-party hosted service deferred. ADR 0020 still
governs release signing and platforms.

## Question

Rekenraam is currently a self-hosted web app: one Go binary serves the API and
static frontend, stores data in a local SQLite file, and runs in Docker Compose
or on a home server/VPS. Why self-hosted, and what would desktop, managed
hosting, SaaS, hybrid sync, PWA or marketplace distribution add or take away?

This review separates:

- **from-scratch fit** — how well a direction fits the product's values,
  security model and operating shape; and
- **transition cost from here** — what moving the current app and its users
  would cost.

There is no single winner on every criterion. Self-hosted is not chosen because
it is cheapest for every user; it is chosen because data ownership, local
control and a small trusted deployment are the product's value proposition.
Desktop is a credible optional host. SaaS is a different business with a much
larger operational and compliance surface.

## Answer

**Keep self-hosted web as the canonical product.** It is the only direction that
fully matches the current promise: your records, backups and exports live on a
host you choose, with no required cloud account. It also matches the measured
deployment footprint: one binary, one SQLite file, no database server and no
external service required for core use.

**Keep desktop as a pursued optional direction, not a replacement.** The
[desktop feasibility review](desktop-application-feasibility-2026-09-19.md)
shows that a thin Wails host can reuse the same Go handler, SQLite file and
Svelte UI. A local-data desktop edition is technically credible, but it adds
per-OS packaging, signing, updates, WebView differences and support. It should
continue through the existing demand, stable-Wails-v3, spike and ADR gates; it
should not fork the ledger or create a second business API. Current product
requirements still say native desktop is out of scope, so shipping it would
require an ADR that changes that scope; "pursued" here means continuing the
gated path, not making a release promise.

**Treat hosted service as a separate business decision.** Managed single-tenant
hosting is the least risky hosted experiment and could reuse the same binary and
SQLite file per instance. Multi-tenant SaaS is a much larger step: tenant
isolation, compliance, billing, support, uptime, data residency and a different
security review. Neither is implied by the current architecture or demand.

**Do not pursue serverless/edge or a native mobile rewrite now.** They conflict
with local SQLite, durable background workers and the single-binary shape, and
they do not solve a measured problem.

## Current shape and measurements

| Item | Current observation |
|---|---|
| Production process | One Go binary serves `/api/v1` and the embedded SvelteKit build |
| Binary | About 33 MiB; links libc/libm and the ELF loader, no system `libsqlite3` |
| Database | One SQLite file, WAL + `synchronous=FULL` |
| Empty migrated database | About 1.58 MiB |
| Warm startup | About 76 ms, 23 MiB RSS |
| Cold startup including migrations | About 269 ms, 28 MiB RSS |
| Large-book workload | 250k postings: export 11.1 s, self-check 10.9 s; peak heap 12–41 MiB for those reads |
| Container shape | One app process; no database or queue service |
| Release targets | Linux amd64/arm64, signed candidate workflow; desktop not part of the current release claim |
| Required cloud account | None for core bookkeeping |
| Optional network | Provider connections use the owner's own keys and are not required |

The [evidence JSON](fixtures/database-deployment-profiles-2026-11-03.json)
retains the startup, schema, large-book and SQLite commit-probe results. The
startup probe used the existing binary built at `da2563b4`; later `main` commits
only patch frontend dependencies. These are observations from the current
source, not a service-level objective. A hosted product would add its own
availability, backup, monitoring and support requirements on top.

## Directions compared

Ratings describe this product. "Transition" is kept separate from the
from-scratch design.

| Criterion | Self-hosted web (current) | Managed single-tenant | Multi-tenant SaaS | Desktop local (Wails) | Hybrid desktop + sync | PWA / mobile web | Marketplace distribution | Serverless / edge |
|---|---|---|---|---|---|---|---|---|
| Data ownership by user | Strongest | Medium | Weakest | Strongest | Strong if local-first | Strongest | Strongest | Weakest |
| Onboarding ease | Low–medium | Strong | Strongest | Medium | Medium | Strong | Medium | Strong |
| Operator burden on us | Lowest | High | Highest | Low–medium | High | Lowest | Medium | Medium |
| Operator burden on user | High | Low | Lowest | Low | Medium | Lowest | Low | Lowest |
| Offline/local use | Strong on LAN/local | Weak | Weak | Strongest | Strong if local-first | Medium | Strong on LAN/local | Weak |
| Multi-device access | Via own server/VPN | Strong | Strongest | Weak without sync | Strong but complex | Strong | Via own server | Strong |
| Security/compliance surface | User-controlled | Shared responsibility | Highest | Medium | High | User-controlled | User-controlled | Shared |
| Support burden | Community/user | High | Highest | High per OS | Highest | Low | Medium | Medium |
| Infrastructure cost | User pays | We pay | We pay | User pays | Both | User pays | User pays | Usage-based |
| Desktop fit | Via browser | Via browser | Via browser | Native host | Native host | Via browser | Via browser | Via browser |

### Self-hosted web (current)

**From-scratch strengths**

- The user owns the database, backups and access. This is the product's trust
  proposition, not an implementation detail.
- No required cloud account, no telemetry, no vendor relationship and no
  first-party service that must stay online for core bookkeeping.
- One process and one file are easy to explain, back up and restore. A home-lab
  user can run it on a small server, a Raspberry Pi or a VPS.
- The same shape supports LAN-only, VPN, reverse-proxied HTTPS and local
  browser use without changing the app.
- It keeps optional providers optional: imports and market data are adapters,
  not prerequisites.

**From-scratch costs**

- The user must run a host, keep it updated, arrange HTTPS for remote access,
  protect storage and keep off-site backups. The app makes backups easier but
  cannot arrange a second device.
- Remote access requires networking knowledge; a hosted service is easier for
  non-technical users.
- Support is community/operator-led; there is no first-party uptime promise.
- A self-hosted instance has no built-in multi-device sync beyond reaching the
  same server.

A local-only browser launch is the same model: start the binary on the laptop
and open localhost. It is the cheapest desktop-like path because there is no
installer, native shell or second packaging matrix, but it is not an installed
application and still leaves update/backup/startup handling to the user.

**Transition cost from here:** None; this is the current product.

### Managed single-tenant hosting

One instance or one database per customer, operated by the project or a partner.
The user gets a URL and backups without running a server.

**From-scratch strengths**

- Much easier onboarding than self-hosting while preserving tenant isolation
  much more naturally than a shared multi-tenant database.
- The existing binary and SQLite file can be reused per instance, so the
  application architecture does not need a database rewrite.
- The project controls upgrades, backups, TLS and monitoring, reducing user
  support burden.
- A credible middle ground between self-hosted and full SaaS.

**From-scratch costs**

- We become an operator: provisioning, uptime, backups, restore drills,
  monitoring, incident response, abuse handling, billing and support.
- We hold or process other people's financial data; security, privacy, data
  deletion, retention and jurisdiction questions become first-party.
- Per-instance SQLite is simple but needs isolation, resource limits and a
  fleet upgrade/backup strategy.
- A paid service needs pricing, terms, tax/accounting and customer support
  capacity that the project does not currently have.

**Transition cost from here:** Medium–high. The app can be reused, but the
service wrapper, provisioning, observability, billing, support, backup fleet,
security review and incident process are new work. It still needs a hosting
business decision before an ADR.

### Multi-tenant SaaS

One shared service, many customers, with tenant isolation.

**From-scratch strengths**

- Best onboarding and potentially the strongest unit economics at scale.
- Central upgrades, monitoring, backups and support.
- Enables web/mobile access without the user running anything.

**From-scratch costs**

- Highest security and compliance surface: tenant isolation, authorization,
  audit, encryption, data residency, deletion, breach response, uptime and
  possibly financial-data regulation.
- The current single-user/single-book model is not a multi-tenant model. User,
  book, membership, billing and support identities would need a deliberate
  design.
- PostgreSQL or another server database becomes much more attractive, adding a
  database migration on top of the service build.
- Pricing, support, marketing, legal and operational staffing become part of
  the product.
- It weakens the self-hosted ownership promise unless offered as a separate
  product with explicit data terms.

**Transition cost from here:** Very high. It is not a deployment toggle: it
requires multi-tenancy, identity, isolation, billing, compliance, a server
database decision and a support organization. It should not be inferred from
the current app's ability to run in Docker.

### Desktop local (Wails)

An installed application that owns its local SQLite file and embeds the same Go
handler and Svelte UI.

**From-scratch strengths**

- Strong offline/local ownership and a familiar installed-app experience.
- Reuses the Go backend, SQLite schema, migrations, Svelte UI, localization and
  most tests.
- No server, no port exposure and no required cloud account.
- The
  [desktop review](desktop-application-feasibility-2026-09-19.md) estimates a
  credible one-OS prototype in 3–7 working days and a signed single-OS release
  in roughly 4–7 engineer-weeks including the spike.

**From-scratch costs**

- Per-OS packaging, signing/notarization, updates, WebView differences and
  clean-machine testing. A three-OS supported line is roughly 7–12
  engineer-weeks before ongoing maintenance, with an estimated 15–30% ongoing
  release/maintenance overhead.
- Data and secret locations, single-instance handling, backup/restore UX,
  file dialogs, downloads and background work while the window is closed all
  need explicit design.
- WebView cookie/origin behavior may differ from a normal browser; the desktop
  transport must pass a conformance spike.
- It creates a second deployment promise even when feature code is shared.
- Sync is not included. A desktop file is a separate local book unless the user
  moves a backup or a future sync product is built.

**Transition cost from here:** Medium–high. The shared runtime extraction,
Wails host, data/secret paths, packaging/signing, update path and per-OS test
matrix are new. The existing desktop review already defines the gates,
including waiting for a stable Wails v3 release before reconsidering
implementation; no desktop implementation is claimed by this review.

### Hybrid desktop + sync / remote server client

A local desktop book plus sync, or a desktop client for a remote Rekenraam
server.

**From-scratch strengths**

- Local responsiveness with multi-device access when it works.
- A remote-client-only variant is simpler than sync and reuses the existing API.

**From-scratch costs**

- Sync introduces identity, conflict resolution, encryption, tombstones,
  migrations across replicas, offline edits and data-loss recovery. For a
  financial ledger, silent merge errors are unacceptable.
- A remote client needs server selection, TLS, credentials, disconnection
  states and a clear trust boundary.
- Supporting both local and remote books creates two data-lifecycle stories.

**Transition cost from here:** Very high. It is a separate product design, not
a Wails packaging task. Do not add sync until a concrete user need and a
conflict/recovery contract exist.

### PWA and mobile web

**From-scratch strengths**

- The current responsive web UI already works on phones; PWA polish could add
  installability, offline shell caching and better mobile ergonomics.
- No app-store or per-OS native release pipeline.

**From-scratch costs**

- Offline writes are not free: the app is server-backed, and a service worker
  cache can show stale financial data or hide failed writes.
- Push notifications, background sync and file handling have browser-specific
  limits.
- A native mobile app would duplicate the UI and release matrix; it is not
  current scope.

**Transition cost from here:** Low–medium for PWA polish; very high for a native
mobile app. The current responsive UI is the mobile story.

### Marketplace / self-host distribution

Not a separate runtime model: PikaPods, Cloudron, Elestio, Umbrel and similar
services package the existing self-hosted app for easier installation.

**From-scratch strengths**

- Lowers installation friction without taking ownership of user data.
- Reaches users who want self-hosting but not manual Docker/Compose work.
- Keeps the same binary and backup/export story.

**From-scratch costs**

- Each marketplace has its own packaging, upgrade, health-check, permission and
  support contract.
- The project must keep those packages current and answer operator questions.
- Marketplace listing is a distribution/release gate, not a substitute for
  product readiness.

**Transition cost from here:** Medium. Mostly packaging, documentation,
health/upgrade checks and support, with no core architecture change.

### Serverless / edge

**From-scratch strengths**

- Elastic scale and low idle cost for request/response workloads.

**From-scratch costs**

- The current app needs a durable local SQLite file, a long-lived process,
  background workers, scheduled jobs, migrations and a lock file. A stateless
  function model does not provide those directly.
- Edge runtimes add storage, region, cold-start, egress and vendor constraints.
- It would require a database/service rewrite and a new security model for a
  single-user finance app that does not need elastic scale.

**Transition cost from here:** Very high and strategically mismatched.

## Transition cost from the current deployment

| Direction | Transition from here | Main work |
|---|---|---|
| Self-hosted web | None | Current product |
| Managed single-tenant | Medium–high | Provisioning, monitoring, backups, billing/support, security/ops, fleet upgrades |
| Multi-tenant SaaS | Very high | Tenant model, isolation, auth, server database, billing, compliance, support |
| Desktop local | Medium–high | Runtime extraction, Wails host, data/secret paths, packaging/signing/updates, per-OS tests |
| Hybrid sync / remote client | Very high | Identity, sync/conflict/recovery contract, encryption, remote trust boundary |
| PWA / mobile web | Low–medium for PWA; very high for native | Service-worker/offline policy; native app is a separate UI |
| Marketplace distribution | Medium | Packaging, upgrade/health checks, docs and support per marketplace |
| Serverless / edge | Very high | Replace local file/workers with managed storage and jobs; new security/ops model |

## Recommendation

1. **Self-hosted web stays canonical.** Preserve the one-binary, one-file,
   no-required-cloud shape and the existing security/backup guidance.
2. **Desktop stays a pursued optional host.** Continue the existing demand,
   one-OS spike and ADR gates. Reuse the same Go handler, SQLite schema and
   Svelte UI; do not create a second business API or a separate ledger.
3. **If hosting is tried, start with managed single-tenant.** It reuses the
   current architecture and keeps tenant isolation simpler. Multi-tenant SaaS
   needs a separate business/architecture decision and likely PostgreSQL.
4. **Keep PWA polish separate from native mobile.** Responsive web remains the
   mobile story until a measured need justifies more.
5. **Use marketplaces as distribution, not as a new runtime.** They reduce
   installation friction while keeping user ownership.
6. **Do not pursue serverless/edge or sync now.** They conflict with the current
   local-file/durable-worker shape and add disproportionate financial-data risk.

## Revisit triggers and gates

- **Desktop:** confirmed user demand; **stable Wails v3 available and pinned**
  (or an explicit owner decision accepting a pinned v2 plus a later v3
  migration); a successful one-OS Wails conformance spike; and an ADR that
  changes the current native-desktop scope.
- **Managed hosting:** marketplace/community demand evidence, operational
  readiness (MFA, backups, monitoring, incident response), support capacity and
  a clear data-processing/privacy position.
- **Multi-tenant SaaS:** a separate business decision with demand, pricing,
  legal/compliance review and a multi-tenant data model.
- **Hybrid sync:** a concrete multi-device requirement plus a conflict,
  encryption and recovery contract.
- **PWA:** a measured mobile/offline need that the responsive web UI cannot meet.
- **Serverless/edge:** only after a database/worker architecture change that
  makes the current local-file model impossible or clearly inferior.

Do not infer a hosted or SaaS direction from the app's ability to run in Docker.
Self-hosting is a product promise; hosting is a different operating business.

## Reproduction

```sh
pnpm build
rm -rf .tmp/deploy-profile
mkdir -p .tmp/deploy-profile
DB="$PWD/.tmp/deploy-profile/rekenraam.sqlite"
APP_ENV=development HTTP_ADDR=127.0.0.1:16999 \
  DATABASE_URL="file:$DB" ./dist/rekenraam serve >/tmp/rekenraam-deploy.log 2>&1 &
pid=$!
until curl -sf http://127.0.0.1:16999/healthz >/dev/null; do sleep 0.05; done
awk '/VmRSS|Threads/{print}' /proc/$pid/status
kill -TERM "$pid"; wait "$pid" || true
```

The first startup includes migrations; repeat with the same database for the
warm figure. `curl`, `awk` and procfs are Linux-specific. The desktop and
hosting cost estimates come from the linked desktop review and this analysis;
they are not measured service bills.

## Other architecture choices

The four choice reviews now cover the load-bearing stack decisions: backend
language, frontend framework, database and deployment/distribution. Other
decisions already have a home:

- REST/OpenAPI contract and generated TypeScript client: `docs/conventions.md`
  and `api/openapi/openapi.yaml`.
- Authentication, sessions, MFA and personal access tokens: ADR 0003, ADR 0018
  and the auth conventions.
- Durable background work, leases, cursors and provider polling: ADR 0010.
- Exact decimals, double-entry postings and ledger invariants: ADR 0009, the
  ledger conventions and `docs/plans/transaction-ledger-core-plan.md`.
- Release packaging, signing and platform targets: ADR 0020 and
  `docs/developer-workflow.md`.

Not separately re-reviewed here: API protocol alternatives (GraphQL/gRPC), an
ORM or query builder, an external job broker, observability/telemetry policy,
multi-user/household permissions, and sync/mobile strategy. None is implied by
these reviews; each needs its own bounded comparison if it becomes current work.

## References

- [Product requirements — Product Intent](../product-requirements.md#product-intent).
- [Self-hosting and operations requirements](../product-requirements.md#self-hosting-and-operations).
- [Roadmap direction](../roadmap.md#direction).
- [Desktop application feasibility review](desktop-application-feasibility-2026-09-19.md).
- [Deployment security guide](../deployment-security.md).
- [Database choice review — 2026-11-03](database-choice-review-2026-11-03.md).
- [Backend language choice review — 2026-11-03](language-choice-review-2026-11-03.md).
- [Frontend framework choice review — 2026-11-03](frontend-framework-choice-review-2026-11-03.md).
- [ADR 0020: Release Signing And Platform Targets](../adrs/0020-release-signing-and-platform-targets.md).
