# Roadmap

This is the forward-looking, prioritized roadmap. It is the single source for
"what's next." It is governed by `docs/product-requirements.md` (durable product
intent and phase boundaries) and refines it with concrete, ordered work plus a
competitor-driven feature gap analysis.

- **What's already done:** `docs/implemented.md`.
- **Technical debt / polish:** `docs/backlog.md`.
- **Long-lived decisions:** `docs/adrs/`, `docs/conventions.md`,
  `docs/early-architecture-decisions.md`.

Status as of 2026-06-27: Phases 0–2 ship end-to-end. Phase 3 has a complete
backend (reconciliation engine, ledger read models) but no reconcile/reports UI.
Phases 4–6 are mostly unstarted; FX/pricing and investments have backend
foundations only.

---

## Competitor Feature Gap Analysis

Reference apps for a self-hosted Microsoft Money / Quicken successor. ✅ = solid,
🟦 = partial/backend-only, ⬜ = missing.

| Capability | Rekenraam | Quicken | GnuCash | YNAB | Firefly III | Money Manager Ex |
|---|---|---|---|---|---|---|
| Double-entry ledger | ✅ | partial | ✅ | ⬜ (envelope) | ✅ | partial |
| Multi-currency accounts | ✅ | ✅ | ✅ | limited | ✅ | ✅ |
| Account register + same-day order | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Reconciliation workflow | 🟦 (no UI) | ✅ | ✅ | ⬜ | ✅ | ✅ |
| Core reports (net worth/cashflow/spending) | 🟦 (no UI) | ✅ | ✅ | ✅ | ✅ | ✅ |
| Saved/custom reports | ⬜ | ✅ | ✅ | ⬜ | ✅ | ✅ |
| CSV import | ⬜ | ✅ | ✅ | ✅ | ✅ | ✅ |
| OFX/QFX/QIF import | ⬜ | ✅ | ✅ | ⬜ | partial | ✅ |
| Duplicate detection + import rules | ⬜ | ✅ | partial | ✅ | ✅ | partial |
| CSV/QIF export | ⬜ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Budgets | ⬜ | ✅ | ✅ | ✅ (core) | ✅ | ✅ |
| Scheduled / recurring transactions | ⬜ | ✅ | ✅ | ⬜ | ✅ | ✅ |
| Projected balances / forecasting | ⬜ | ✅ | partial | ✅ | partial | ⬜ |
| Loans / amortization helpers | ⬜ | ✅ | ✅ | ⬜ | ⬜ | ✅ |
| Investment lots & cost basis | 🟦 | ✅ | ✅ | ⬜ | partial | ✅ |
| Dividends / corporate actions | 🟦 | ✅ | ✅ | ⬜ | ⬜ | partial |
| Price/FX history + scheduled refresh | 🟦 | ✅ | ✅ | ⬜ | ✅ | partial |
| Realized/unrealized gains reporting | ⬜ | ✅ | ✅ | ⬜ | ⬜ | partial |
| Payee/category cleanup & merge | ⬜ | ✅ | partial | ✅ | ✅ | partial |
| Self-hosted, single binary, no telemetry | ✅ | ⬜ | ✅ | ⬜ | ✅ | ✅ |
| Mobile-responsive web | ✅ | app | ⬜ | ✅ | ✅ | ⬜ |

**Reading of the gap:** Rekenraam's ledger core is competitive with GnuCash and
Firefly and ahead of YNAB on accounting correctness. The decisive missing pieces
for an everyday Money/Quicken replacement are, in order: (1) reconcile + reports
UI to expose the engine that already exists, (2) import — the single biggest
manual-entry reducer every competitor ships, (3) budgets and recurring
transactions, the daily-driver planning loop, and (4) surfacing the
investment/FX backends. Exports are table stakes and cheap.

---

## Now — close the trust loop (finish Phase 3)

The backend already computes reconciliation and reports; the gap is purely UI.
This is the highest-leverage work because it exposes shipped engines.

### R1. Reconciliation workflow UI
Competitors (Quicken, GnuCash, MMEX) all gate "is my balance real?" on a
reconcile screen. The engine, guard, and impact previews exist (`app/reconciliation.go`,
`/reconciliations/*`); only the UI is missing.
- Start-reconciliation flow (statement date + ending balance) per account.
- Clear/unclear postings against a running cleared balance; live difference.
- Finish (asserts difference = 0) and void-checkpoint controls.
- Surface the period-scoped guard warnings already returned by the API.

### R2. Reports UI
Read models exist (`/ledger/net-worth`, `/account-balances`, `/category-totals`).
- Net worth over time, spending by category/payee, cashflow.
- Date-range + account filters; CSV/print-friendly view.
- Add a cashflow read model (the one core report without an endpoint).

### R3. Core exports (CSV + QIF)
Locked as the first mandatory export set (product-requirements). Cheap, expected
by every competitor, and unblocks user trust/portability.
- CSV export of core ledger (transactions + postings) and QIF export.

---

## Next — reduce manual entry (Phase 4: Import)

Every reference app treats import as the core manual-entry reducer. Built as one
**unified, modular ingestion pipeline** (source → stage → normalize → dedupe →
review → commit) where file uploads and future online feeds are both "sources",
and each format/provider is a swappable adapter + saved profile. Nothing touches
the ledger unreviewed. **Full design: `docs/import-plan.md`.**

### R4. Pipeline + QIF import (MS Money migration) — first value
The first slice lands the parser interface + staging/preview/commit pipeline and
ships the **QIF adapter** — which doubles as the MS Money migration path
(MS Money exports per-account loose QIF; its `.mny` has no clean export).
- Modular `SourceAdapter` interface + registry; QIF implemented, others later.
- Preview → account/currency mapping → commit via the transaction service
  (`OriginType=import`, `needs_review=true`), reusing the existing review queue.
- Source-metadata retention; `source_fingerprint` dedupe makes re-imports a no-op.

### R5. CSV + provider profiles
CSV/XLSX differ wildly per bank, so column mapping is **saved profile data, not
code**. Adds the CSV adapter + the column-mapping profile engine/UI.
- Save a per-bank profile once; the next statement is one click.

### R6. XLSX, OFX/QFX, duplicate-review depth
- XLSX adapter (reuses the CSV row pipeline); OFX/QFX (FITID dedupe).
- Richer duplicate review, import audit-trail/history, batch rollback (via void).

### R7. Online ingestion + payee/category cleanup
- Online sources implement the same adapter contract, driven by the durable work
  queue (ADR 0010) so refreshes are restart-safe — same model as FX coverage.
- Merge duplicate payees; bulk recategorize; the `needs_review` queue UI for
  imported-but-unreviewed transactions (flag + endpoint already exist).

---

## Then — planning loop (Phase 5)

The forward-looking layer that makes the app a daily driver rather than a
record-keeper.

### R8. Budgets
Treated as a planning axis over income/expense accounts, not an account kind
(per product-requirements). Period budgets with actual-vs-budget reporting.

### R9. Scheduled / recurring transactions
Templates + schedule; generate due entries (a future producer of persisted
`draft` for review, which the lifecycle already reserves).

### R10. Projected balances & loan helpers
- Projected cash balance from scheduled transactions.
- Simple loan/liability + amortization helpers that fit the existing ledger.

---

## Later — surface advanced finance (Phase 6 UI)

Backends exist; this is about exposing and completing them.

### R11. Pricing / FX management UI
Source assignment, manual price/FX entry, refresh history, source health — all
have APIs (`/pricing/*`) and no screens yet.

### R12. Investments UI + gains reporting
- Holdings, lots, buy/sell/dividend entry over the existing investment service.
- Verify FIFO lot-matching is enforced server-side before exposing sell flows
  (gains must be computed, not free-form input).
- Realized/unrealized gains reports; multi-currency reporting; report snapshots
  where reproducibility matters.
- Corporate actions: provider events → reviewable suggestions; explicit
  automation rules required before auto-posting.

---

## Beyond — explicitly deferred

Kept out of the near/mid roadmap by product decision: multi-user/household,
bank/open-banking integrations, attachments (statements/receipts), plugin
architecture, small-business AR/AP & invoicing, native mobile apps, hosted
service operations. Compatibility guardrails (typed `/api/v1`, semantic theme
tokens, reserved `/plugins` `/themes` namespaces) keep these additive later.

---

## Open product decisions (resolve before the related slice)

- Export scope beyond core ledger CSV/QIF (full structured JSON of settings?).
- First UI languages to ship after the English boundary.
- Attachment storage target (SQLite / filesystem / object storage) — when in scope.
- Which mobile workflows get dedicated optimization first.
- Public-deployment MFA timing (gates real-data internet exposure).
