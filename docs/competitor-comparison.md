# Competitor Comparison

Durable reference for how Rekenraam positions against commercial and
open-source personal finance software. The roadmap
(`docs/roadmap.md`) links here instead of carrying its own gap table; update
this file when the landscape shifts. Point-in-time deep dives:
`docs/reviews/competitive-analysis-2026-07.md`. Last full revision: 2026-07-07.

## Positioning

> The self-hosted Quicken Premier / PocketSmith for people whose money lives
> in more than one country — exact double-entry ledger, lot-level
> investments, real multi-currency.

No product, commercial or open-source, currently combines all three of:
(1) a correct double-entry multi-currency ledger, (2) lot-level investment
cost basis with dividends and gains, (3) self-hosted web deployment. Users in
Rekenraam's target persona today run "Firefly III + Ghostfolio + a
spreadsheet."

## Migration waves (where new users come from)

- **Microsoft Money** (discontinued 2009) — users still limp along on
  sunset builds; QIF import (shipped, R4) is their path in.
- **Mint** (Intuit, shut down March 2024, folded into Credit Karma with no
  budgets) — the largest single displacement event in the category;
  its users scattered to Monarch, YNAB, Actual, Copilot. Still arriving.
- **Quicken subscription fatigue** — Classic went subscription in 2018;
  each price rise produces a migration wave of exactly the
  investment-literate users Rekenraam serves.
- **Maybe Finance** (open-sourced 2024, company shut down June 2025) —
  demand for a general OSS finance app persists; the community fork (Sure)
  inherited the repo but not the momentum.

## Feature matrix — open source / self-hosted

✅ = solid, 🟦 = partial/backend-only, ⬜ = missing.

| Capability | Rekenraam | GnuCash | Firefly III | Actual | Ghostfolio | Portfolio Perf. | Money Mgr Ex | Beancount/hledger |
|---|---|---|---|---|---|---|---|---|
| Double-entry ledger | ✅ | ✅ | ✅ | ⬜ (envelope) | ⬜ | partial | partial | ✅ |
| Multi-currency accounts | ✅ | ✅ | ✅ | limited | display only | ✅ | ✅ | ✅ |
| Reconciliation workflow | ✅ | ✅ | ✅ | ⬜ | ⬜ | ⬜ | ✅ | 🟦 (assert) |
| Core reports UI | 🟦 (R2) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ (Fava) |
| CSV import + profiles | ⬜ (R5) | ✅ | ✅ (importer) | ✅ | ✅ | ✅ | ✅ | ✅ |
| QIF/OFX import | ✅ QIF | ✅ | partial | ⬜ | ⬜ | partial | ✅ | via tools |
| Import rules engine | ⬜ | partial | ✅ (strongest) | ✅ | ⬜ | ⬜ | partial | ✅ (code) |
| Budgets | ⬜ (R8) | ✅ | ✅ | ✅ (core) | ⬜ | ⬜ | ✅ | 🟦 |
| Recurring/scheduled txns | ⬜ (R9) | ✅ | ✅ | ✅ | ⬜ | ⬜ | ✅ | ⬜ |
| Cashflow forecasting | ⬜ (R10) | partial | partial | ⬜ | ⬜ | ⬜ | ⬜ | 🟦 |
| Investment lots & cost basis | ✅ (4 methods) | ✅ | ⬜ | ⬜ | ⬜ | basic FIFO/avg | partial | ✅ |
| Dividends (incl. withholding, reinvest) | ✅ | ✅ | ⬜ | ⬜ | partial | ✅ | partial | ✅ |
| Corporate actions (splits/mergers/delist) | ⬜ (T-34; no manual entry either) | ✅ | ⬜ | ⬜ | partial | ✅ | partial | ✅ (manual) |
| Realized/unrealized gains | ✅ | ✅ | ⬜ | ⬜ | ⬜ | ✅ | partial | ✅ |
| Returns analytics (TWR/MWR) | ⬜ (R13) | ⬜ | ⬜ | ⬜ | ✅ | ✅ (strongest) | ⬜ | via tools |
| Price/FX history + refresh | 🟦 (R11) | ✅ | ✅ | ⬜ | ✅ | ✅ | partial | ✅ |
| Broker/bank online feeds | ✅ T212 (BYO-key) | partial | via importer | via SimpleFIN | partial | partial | ⬜ | via tools |
| Self-hosted web UI | ✅ | ⬜ desktop | ✅ | ✅ | ✅ | ⬜ desktop | 🟦 | ✅ (Fava) |
| Single-binary deploy | ✅ | n/a | ⬜ (2–3 containers) | ✅ | ⬜ (3 containers) | n/a | n/a | ✅ |
| Typed public API | ✅ OpenAPI | ⬜ | ✅ | partial | ✅ | ⬜ | ⬜ | ⬜ |

## Feature matrix — commercial

| Capability | Rekenraam | Quicken Classic | Simplifi | Mint (†2024) | Monarch | YNAB | PocketSmith | Lunch Money |
|---|---|---|---|---|---|---|---|---|
| Price | free, self-hosted | ~$60–120/yr | ~$48/yr | free (ads) | ~$100/yr | ~$110/yr | tiered ~$0–265/yr | ~$100/yr |
| Data ownership | ✅ local SQLite | partial (local file + cloud sync) | ⬜ cloud | ⬜ cloud | ⬜ cloud | ⬜ cloud | ⬜ cloud | ⬜ cloud |
| Double-entry correctness | ✅ | partial | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ |
| Bank sync coverage | BYO-key adapters only | ✅ US-centric | ✅ US | ✅ US/CA | ✅ US-centric | ✅ US | ✅ intl | ✅ (Plaid + intl) |
| Multi-currency | ✅ exact | clunky, US-centric | ⬜ | ⬜ | weak | limited | ✅ (best commercial) | ✅ |
| Reconciliation | ✅ | ✅ | ⬜ | ⬜ | ⬜ | partial | partial | ⬜ |
| Budgets | ⬜ (R8) | ✅ | ✅ | ✅ | ✅ | ✅ (core) | ✅ | ✅ |
| Forecasting | ⬜ (R10) | ✅ | partial | ⬜ | partial | partial | ✅ (30-yr calendar) | ⬜ |
| Investment lots & gains | ✅ | ✅ (Premier; only mainstream tool with full lot detail) | ⬜ | ⬜ | ⬜ (no lot detail) | ⬜ | partial | ⬜ |
| Dividends | ✅ | ✅ | ⬜ | ⬜ | ⬜ | ⬜ | partial | ⬜ |
| Corporate actions (splits/mergers) | ⬜ (T-34) | ✅ | ⬜ | ⬜ | ⬜ | ⬜ | partial | ⬜ |
| API for users | ✅ OpenAPI | ⬜ | ⬜ | ⬜ | ⬜ | ✅ | ✅ | ✅ (dev-friendly) |
| Longevity risk | AGPL, forkable | Quicken Inc. | Quicken Inc. | dead | VC-backed | stable | indie, stable | solo dev |

Notes on commercial players not in the matrix: **Empower Personal
Dashboard** (free US investment dashboard, advisory upsell, no ledger),
**Tiller** (spreadsheet automation, $79/yr), **Kubera** (net-worth/alt-asset
tracking for HNW/expat users, no ledger), **Banktivity** (Mac-only, decent
multi-currency), **Copilot** (iOS-first Mint successor, US).

## Per-competitor read

### Commercial

- **Quicken Classic (Deluxe/Premier)** — the feature ceiling: lots,
  reconcile, reports, forecasting, loans. Aging desktop codebase,
  subscription, cloud-sync trust issues, poor multi-currency. *Rekenraam is
  explicitly a successor candidate; QIF import + reconcile demo targets its
  users.*
- **Quicken Simplifi** — Quicken's cloud-lite product; no lots, no
  reconcile. Not a feature competitor; proof Quicken Inc. is moving
  down-market, leaving Premier users stranded long-term.
- **Mint (dead)** — defined the free bank-sync-first category, monetized by
  ads/upsell, died when that model did. Lessons: bank-sync-first without
  ownership is fragile; its shutdown remains the category's largest source
  of migrating users.
- **Monarch Money** — the Mint successor category leader. Polished,
  multi-user, connection-first; no lot-level gains, weak multi-currency,
  ~$100/yr, VC-backed. *Its gap is exactly Rekenraam's strength.*
- **YNAB** — envelope budgeting with religious following; limited currency,
  no investments. Actual Budget already serves its self-hosted refugees.
- **PocketSmith** — closest commercial analog to Rekenraam's niche:
  real multi-currency, international bank feeds, calendar cashflow
  forecasting to 30 years, premium-priced tiers. *Proof the
  expat/multi-currency segment pays. Its forecasting is the bar for R10.*
- **Lunch Money** — indie web app popular with developers and expats
  (multi-currency, crypto, open API). Cloud-only, solo-dev longevity risk,
  no investments depth. *Competes for the same self-reliant persona;
  Rekenraam's answer is ownership + investments.*

### Open source

- **GnuCash** — the correctness benchmark (full double-entry, lots,
  business features) but desktop-era UX, no web/mobile, XML/SQL files.
  *Rekenraam should match its accounting rigor with a modern web UX.*
- **Firefly III** — the default self-hosted all-rounder; strongest at
  rule-based import automation and its ecosystem (data importer, mobile
  apps). Weak investments by design ("not for investment tracking"),
  heavier deployment. *Its rules engine is the stickiest feature to match
  (roadmap R7); its investment gap is the wedge.*
- **Actual Budget** — best-in-class envelope budgeting, local-first sync,
  huge community. Not a ledger/investments competitor; sets the bar for
  onboarding speed (working budget in 15 minutes).
- **Ghostfolio** — default OSS portfolio tracker (8k+ stars); clean UX,
  broad asset classes, but **no cost basis, no taxes, no ledger** by
  design, and needs Postgres+Redis. *Its returns analytics (TWR, allocation)
  define user expectations Rekenraam R13 must meet; its demo instance is a
  proven adoption asset.*
- **Portfolio Performance** — free Java desktop, dominant among DACH retail
  investors; strongest performance attribution (TWR/MWR), basic FIFO/avg
  cost basis, no country-specific tax reports, no self-host/web story.
  *The analytics benchmark; its rebalancing view is the model for a future
  "trade planner" (see roadmap "Beyond").*
- **Money Manager Ex / KMyMoney / HomeBank** — desktop Money/Quicken clones;
  feature-broad, shallow investments, aging UX. Source of migrating users
  more than competition.
- **Beancount / hledger / Ledger (+ Fava)** — plain-text accounting:
  arbitrary precision, real multi-currency, lots, unmatched auditability —
  for people who write code. *Rekenraam's exact-precision ledger brings
  that correctness to users who want a UI instead of a text editor.*
- **Sure (Maybe fork)** — community-run general finance app on the
  abandoned Maybe codebase; energetic, unproven stewardship.
- **ezBookkeeping** — lightweight Go+SQLite bookkeeping (closest
  architectural cousin); simple cash ledger, no investments, no
  reconciliation depth.

## What the comparison implies (kept in sync with roadmap)

1. **Reports UI (R2) — closed 2026-08-19.** This was the perceived-completeness
   gap; every comparison review leads with dashboards. `/app/reports` now ships
   net worth over time, spending by category or payee, and cashflow, each with
   URL-addressable filters, CSV export, a print layout, and chart summaries
   alongside accessible tables. Parity and differentiation as delivered:

   - **Money / Quicken / Monarch parity** — visible net worth, spending,
     cashflow, and export-ready reports: **met**.
   - **Firefly III parity** — category and payee insight without compromising
     ledger semantics: **met**, and arguably exceeded: spending is built from
     category postings rather than an inferred bank-statement classification,
     so a transfer cannot be counted as spending by construction, and every row
     drills through to exactly the transactions it was summed from.
   - **PocketSmith differentiation groundwork** — exact per-currency cashflow:
     **met**. Cashflow reports per commodity with no fabricated base-currency
     number, and `net_movement = operating_net + transfer_net` holds as an
     identity, which is the property R10 forecasting will need.
   - **Ghostfolio / Portfolio Performance gap retained, deliberately** —
     returns, allocation, and benchmarks stay R13. R2 makes no accidental
     partial promise about them.

   The one honest caveat against the commercial tools: they show a single
   blended base-currency figure and Rekenraam still refuses to. A
   reporting-currency selector was approved on 2026-08-19 and is sequenced after
   R3; until it lands, a multi-currency user sees per-commodity totals rather
   than one number.
2. **Import rules engine** — Firefly's stickiest feature; belongs in R7's
   scope as persistent user-defined rules over the staged pipeline.
3. **Returns analytics (TWR/MWR, allocation, benchmark)** — expected by
   Ghostfolio/Portfolio Performance users; Rekenraam has better underlying
   data (exact lots + FX). Roadmap R13.
4. **Multi-currency cashflow forecasting** — PocketSmith's moat; no OSS
   equivalent; the niche-defining feature for R10.
5. **BYO-key feed adapters** — GoCardless Bank Account Data (EU) and
   SimpleFIN Bridge (US) close the manual-entry objection without coverage
   promises; both follow the Trading 212 pattern.
6. **Jurisdiction-aware capital-gains reporting** — no competitor, commercial
   or OSS, ships it; the long-term moat. Gated on the I-03/I-04 research task
   (escalated 2026-08-19 from a decision to research): realized versus
   unrealized answer different questions, countries differ in which they tax,
   and unrealized figures flip-flop with every price refresh. See
   `roadmap.md` § Open product decisions.
7. **Adoption assets** — public demo instance with seeded data (Ghostfolio's
   playbook), README screenshots, migration screencast.
