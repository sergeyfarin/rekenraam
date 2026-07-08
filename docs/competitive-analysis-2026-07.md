# Competitive Analysis & Trade-Execution Feasibility — 2026-07-07

> **Point-in-time deep dive.** The durable, maintained comparison (full
> feature matrices incl. Mint, Quicken, Monarch, PocketSmith, Lunch Money,
> and the OSS field) now lives in `docs/competitor-comparison.md`; the
> roadmap consequences (trade-execution rejection, R13, rules engine,
> forecasting promotion) are recorded in `docs/roadmap.md`.

Deep-dive follow-up to the roadmap's feature-gap table: backend approach vs
the field, open-source and commercial landscape as of July 2026, roadmap
cross-check, and a feasibility/risk assessment of trade execution
(Trading 212, IBKR, Wikifolio; scheduled and trigger-based trades).
Governed by `docs/product-requirements.md`; refines, does not relitigate, the
2026-07-02 product direction (daily-driver core → expat/multi-currency niche).

---

## 1. Backend approach vs the field

| | Rekenraam | Firefly III | Actual Budget | Ghostfolio | Maybe→Sure | Portfolio Performance |
|---|---|---|---|---|---|---|
| Stack | Go single binary, embedded SvelteKit, SQLite WAL | PHP/Laravel + Postgres/MySQL (+ separate data-importer app) | Node + SQLite, local-first CRDT sync | NestJS + Postgres + Redis | Rails + Postgres (community fork) | Java desktop |
| Deployment | 1 container / 1 binary | 2–3 containers | 1 container or fully local | 3 containers | 2+ containers | not self-hostable (desktop) |
| Accounting model | strict double-entry, exact decimal, immutable versions | double-entry | envelope budget | portfolio only | simple ledger | portfolio + cash |
| Investments | lot-level, 4 cost-basis methods, dividends, gains | weak (price history only) | none | no cost-basis/tax features | basic | basic FIFO/avg, strong performance analytics |
| Multi-currency | exact, FX history, per-commodity scale | good | limited | good (display) | basic | good |

**Assessment.** The architecture choices hold up well against everything in
the table:

- **Single Go binary + SQLite** is a real distribution advantage for the
  self-host marketplaces the roadmap targets (PikaPods, Umbrel, Cloudron).
  Firefly's multi-container Laravel stack and Ghostfolio's Postgres+Redis
  requirement are recurring complaints in comparison reviews. Nothing in the
  OSS web space matches "download one file, run it."
- **The exact-precision double-entry ledger with lot-level investments is
  unique in the self-hosted web category.** Firefly III is competent
  accounting but weak on investments; Ghostfolio is investments-only with
  *no cost-basis/tax features at all*; Actual is budgeting-only; Portfolio
  Performance has the analytics but is a desktop app with basic lot support.
  The niche the product direction picked genuinely has no incumbent.
- **Weak points vs the field** are ecosystem, not engine: Firefly III has a
  mature third-party ecosystem (data importer, mobile apps, integrations)
  and a decade of trust; Actual has a polished local-first UX and huge
  community. Rekenraam's OpenAPI-first typed `/api/v1` is the right
  foundation to grow the same, but the surface is unproven.
- **Cautionary tale worth internalizing:** Maybe Finance (Rails,
  VC-funded, open-sourced 2024) shut down in June 2025; the community fork
  *Sure* carries it on. Validates two roadmap decisions — AGPL with retained
  copyright, and "distribution before hosting" (Maybe died of
  company-sized hosting ambitions, not of product).

---

## 2. Market landscape 2026 — where the demand signals are

**Commercial:**

- **Quicken Classic** — still the only mainstream tool with full lot-level
  capital-gains detail; legacy desktop, subscription, US-centric. Its users
  are the perpetual migration wave the roadmap already targets.
- **Monarch Money** — the Mint-successor category leader (~$100/yr):
  connection-first, multi-user, good dashboards, but *no lot-level gains*
  and weak multi-currency. Its gap is exactly rekenraam's strength.
- **PocketSmith** — the closest commercial analog to the chosen niche:
  explicit multi-currency accounts, international coverage, and
  calendar-based cashflow forecasting up to 30 years. Charges premium tiers
  for it. **PocketSmith's existence is the strongest evidence the
  expat/multi-currency segment pays.**
- **Kubera** — net-worth/alt-assets tracker for HNW expat-ish users; no
  ledger. YNAB — envelope budgeting, limited currency support.

**Open source:**

- **Actual Budget** — envelope budgeting king; the default self-hosted YNAB
  replacement. Not a competitor on ledger/investments.
- **Firefly III** — the default self-hosted "track everything" tool
  (actively developed, v6.6.x mid-2026). Strengths: rules engine,
  bill tracking, data importer, huge API. Weaknesses: investments, running
  balances edge cases, deployment weight.
- **Ghostfolio** — the default self-hosted portfolio tracker (8k+ stars).
  Strengths: clean UX, broad asset classes, API/data feeds. Weakness by
  design: no cost basis, no taxes, no ledger.
- **Portfolio Performance** — free Java desktop, dominant in DACH investor
  communities; strong performance attribution (TWR/MWR), basic FIFO/avg
  cost basis, no web/self-host story.
- **Sure (Maybe fork)** — community-run general finance app; energetic but
  unproven stewardship.

**The stack users actually assemble today for rekenraam's target persona is
"Firefly III + Ghostfolio + a spreadsheet"** (ledger + portfolio + FX/tax
glue). Rekenraam's pitch is replacing that stack with one correct system.

---

## 3. Roadmap cross-check

The current ordering (R2 reports UI → R3 exports → R5 CSV import → budgets/
recurring → surface FX/pricing) survives contact with the market scan. Every
comparison review leads with dashboards/reports screenshots — R2 is
correctly first; perceived completeness *is* the report UI.

Adjustments and additions the scan suggests:

1. **Returns, not just gains (add to R11/R12 follow-up).** Ghostfolio and
   Portfolio Performance users expect TWR/MWR/IRR percentages, allocation
   breakdowns, and benchmark comparison — not only realized/unrealized gain
   amounts. This is read-side only over data that already exists (lots,
   prices, FX) and is the single biggest "investments feel complete" gap vs
   the portfolio trackers. Candidate: extend `GET /investments/gains` with a
   returns read model.
2. **Rules engine deserves a named slice.** Firefly III's most-loved feature
   is rule-based automation (auto-categorize/rename/tag on import). The
   roadmap has "payee/category cleanup" inside R7 but no persistent
   user-defined rules. For the import-heavy persona this is the retention
   feature; the staged-import pipeline is the natural place to run rules.
3. **Cashflow forecasting is the niche's killer feature, currently
   underweighted at R10.** PocketSmith monetizes calendar-based forecasting;
   for cross-border users, *projected balances per currency* (upcoming
   rent in EUR, salary in USD, tax bill in GBP) is the differentiating
   version nobody ships. Consider promoting R10 scope into R8/R9 planning
   work rather than after it.
4. **Bring-your-own-key bank feeds have two obvious candidates** that fit
   the "adapters users bring keys for, never coverage we promise" rule:
   **GoCardless Bank Account Data** (EU PSD2 aggregation, free personal
   tier, token-based — what Firefly/Actual communities actually use) and
   **SimpleFIN Bridge** (US, ~$1.5/mo, user-purchased token). Both match the
   Trading 212 pattern (user-owned credential, polling, staged review) and
   would close the "manual entry" objection for the EU persona without
   promising coverage. IBKR Flex Query stays the right second provider for
   the investor persona.
5. **Multi-jurisdiction tax reporting is the durable moat (post-I-03).**
   Portfolio Performance explicitly lacks country-specific tax reports;
   Ghostfolio lacks any. A capital-gains report parameterized by
   jurisdiction rules (method, holding-period, currency-of-tax) built on
   the multi-method analytical layer (I-03) has no OSS competitor. Long
   lead time; keep on the horizon deliberately.
6. **Adoption assets, not features:** a public demo instance with seeded
   data, screenshots in README, and a "migrate from MS Money/Quicken in 10
   minutes" screencast. Ghostfolio's traction is substantially its live
   demo. Cheap, high leverage at announcement time (Open-Sourcing gate 7).
7. **Positioning sentence for the announcement:** *"The self-hosted
   Quicken Premier / PocketSmith for people whose money lives in more than
   one country — exact double-entry ledger, lot-level investments, real
   multi-currency."* Every named product in that sentence has a migration
   wave and no self-hosted equivalent.

---

## 4. Trade execution feasibility (T212 / IBKR / Wikifolio; scheduled & trigger-based)

### What is technically possible today

- **Trading 212**: the public API supports **placing equity orders**
  (market, limit, stop, stop-limit) on live accounts, plus a practice-mode
  sandbox; auth is API key/secret (HTTP Basic). Caveats: the API is beta,
  and the order endpoint is **documented as non-idempotent — duplicate
  requests can produce duplicate orders**. Technically this would slot into
  the existing `internal/onlinesource/trading212` fetcher with modest work.
- **IBKR**: full order placement exists via the Client Portal Web API / TWS
  API, but for retail accounts authentication runs through the **Client
  Portal Gateway (a local Java process) with interactive 2FA login and
  session keep-alive** — a poor fit for an unattended self-hosted server.
  More importantly, IBKR states that **third-party vendors offering
  automated trading are expected to hold applicable regulatory registration
  and pass IBKR compliance review** in every region served. IBKR *reporting*
  via Flex Query (already the planned second import provider) has none of
  these problems.
- **Wikifolio**: an official trading API exists (client + user API keys),
  but **autonomous automation is against wikifolio's terms — accounts get
  blocked**; only user-initiated ("user clicks, software executes") flows
  are tolerated. Also structurally different: you'd be trading inside a
  wikifolio certificate you manage, not your own brokerage portfolio.
  **Not a fit.**

### Why execution changes what Rekenraam is

1. **Risk class inversion.** Every current bug class misreports the past;
   an execution bug spends real money prospectively. The T-22 lesson
   (severity-1 commit bug shipped, uncaught by tests) applied to a non-
   idempotent beta order endpoint means duplicate live orders. The current
   test-coverage review (0% on several service orchestration paths) is
   nowhere near the bar this feature needs.
2. **Security posture inversion.** Today's stored provider keys are
   read-only history scopes; execution keys are write-capable. A compromised
   instance goes from "privacy incident" to "theft vector." The security
   audit's S-01 (setup race) and S-06 (no 2FA) become hard blockers, plus
   new requirements (per-order confirmation, withdrawal-address-style
   allowlists, spending limits).
3. **Regulatory/ToS exposure.** Shipping software that executes scheduled or
   trigger-based trades for users edges toward regulated activity
   (automated investment services) and explicitly triggers broker
   compliance regimes (IBKR vendor review; wikifolio ToS). A personal
   script is fine; a distributed product doing it is a different legal
   object — especially under AGPL distribution with the project's name on it.
4. **Strategic dilution.** The differentiator is the correct ledger.
   Execution bots are a crowded, separate category (freqtrade et al.).
   Time spent there is time not spent on the uncontested niche.

### Recommendation

- **Keep Rekenraam read-only against brokers for the current roadmap
  horizon.** IBKR enters as Flex Query *import*, as planned. Wikifolio: no.
- **Capture 80% of the underlying user value with zero execution risk via a
  "trade planner" feature** (fits Phase 5/6 naturally): target allocations
  and DCA schedules produce a *proposed order list* (server-computed, using
  the pricing/FX engine that already exists) which the user executes
  manually in their broker app, then the next import reconciles plan vs
  actual. Portfolio Performance's rebalancing view proves the demand; no
  OSS web app does it with real multi-currency math.
- **If execution is ever attempted**, the only defensible shape is
  **human-confirmed order staging**: Rekenraam prepares the order, the user
  explicitly confirms *each* order in-session (fresh auth + 2FA), the API
  places it — never unattended, never scheduled, never trigger-fired.
  Gate it: off-by-default env flag, Trading 212 practice mode first,
  idempotency guard on our side (client order dedupe), spending caps, and
  only after S-01/S-06 (setup hardening, 2FA) ship. Scheduled/trigger-based
  autonomous execution should be treated as out of scope permanently unless
  the project deliberately decides to become a trading product.

---

## 5. Priority synthesis (what makes it competitive and desired)

1. **Ship the trust loop** (R2 reports, R3 exports) — unchanged, confirmed
   by every comparison review's emphasis on dashboards.
2. **Returns analytics (TWR/MWR, allocation)** — closes the gap to
   Ghostfolio/Portfolio Performance where rekenraam already has better data.
3. **Import rules engine** — Firefly's stickiest feature, natural extension
   of the staged pipeline.
4. **Multi-currency cashflow forecasting** — the PocketSmith-beating,
   niche-defining feature; promote within Phase 5.
5. **GoCardless + SimpleFIN as bring-your-own-key feed adapters** — closes
   manual entry for EU/US personas without coverage promises.
6. **Demo instance + migration screencast** at announcement.
7. **Trade planner (proposed orders, manual execution)** — the safe version
   of the execution idea, later.
8. **Jurisdiction-aware capital-gains reporting** — the long-term moat.
