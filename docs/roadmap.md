# Roadmap

This is the forward-looking, prioritized roadmap. It is the single source for
"what's next." It is governed by `docs/product-requirements.md` (durable product
intent and phase boundaries) and refines it with concrete, ordered work plus a
competitor-driven feature gap analysis.

- **What's already done:** `docs/implemented.md`.
- **Technical debt / polish:** `docs/backlog.md`.
- **Long-lived decisions:** `docs/adrs/`, `docs/conventions.md`,
  `docs/early-architecture-decisions.md`.

Status as of 2026-06-28: Phases 0–2 ship end-to-end. Phase 3's reconcile workflow
UI now ships (R1); the reports UI (R2) is still pending over a complete read-model
backend. Phase 4 import has its core pipeline + QIF adapter shipped (Slice 1). Phase 6
**investments shipped end-to-end** (R12: all four cost-basis methods, sell preview,
gains reporting, provider-event review UI); FX/pricing still has backend foundations
only. Phase 5 (budgets, recurring) is unstarted.

---

## Product Direction (decided 2026-07-02)

Governed by `docs/product-requirements.md` (Working Decisions); recorded here
because it orders the roadmap:

1. **Finish the personal daily-driver core** (R2 reports UI, R3 exports, R5 CSV
   import, then budgets and recurring). No direction succeeds without it; this
   is the Money/Quicken-successor market that keeps arriving as those tools die.
2. **Differentiate into the multi-currency / cross-border personal niche**
   (expats, multi-jurisdiction investors). The exact-precision multi-currency
   ledger, FX history, and lot-level investment engine are the durable
   differentiator — Firefly III is weak on investments, Actual is
   budgeting-only, Ghostfolio is portfolio-only. Concretely: multi-currency
   reporting done right, IBKR Flex Query as the likely second online provider
   (token-based, same bring-your-own-key pattern as Trading 212), import
   profiles for common cross-border brokers and banks.
3. **Distribution before hosting.** Self-host marketplace listings (PikaPods,
   Cloudron, Elestio, Umbrel) are the low-cost demand experiment. A
   first-party hosted service is the only meaningful revenue path in this
   category, but it is a company-sized commitment (GDPR posture, MFA, backups,
   incident response) — deferred until demand evidence exists.

Explicitly rejected: EU small-business pivot, native desktop/mobile apps, and
promised bank-connection coverage. **Connections are adapters users bring keys
for, never coverage we promise** — every promised connection is a permanent
maintenance liability (the Trading 212 hardening history, T-14…T-17, is the
proof at n=1).

---

## Open-Sourcing Track (decided 2026-07-02)

License: **AGPL-3.0**, sole copyright retained (rationale and durable rules in
`docs/product-requirements.md`). Repo-public and public announcement are two
separate events with separate gates.

### Gates before flipping the repo public (do soon, quietly)

1. **Full git-history secrets scan** — run `gitleaks` (or `trufflehog`) over
   the entire history (375+ commits including the archived Rust/Python/Tauri
   experiments), not just HEAD. Known scanner bait already found:
   `.archive/secrets/first_admin_password.txt` (a dummy CI value, but the path
   alone fails reviews) — remove it, and decide whether `.archive/` belongs in
   public history at all. History rewrite via `git filter-repo` is cheapest
   **before** anyone has cloned.
2. Add `LICENSE` (AGPL-3.0) and `SECURITY.md` with a private disclosure
   channel (GitHub private vulnerability reporting or a dedicated email).
3. Enable GitHub secret scanning + push protection and Dependabot; add
   `govulncheck` for the Go module (in CI or a scheduled workflow).

### Gates before the public announcement (one-shot first impression)

4. Daily-driver core shipped: R2 reports UI, R3 exports (CSV + QIF), R5 CSV
   import — the "migrate from MS Money/Mint and watch it reconcile" demo.
5. Close backlog T-19 (`REKENRAAM_SECRET_KEY` loss/rotation documentation) —
   the first serious reviewer will find it.
6. Signed release binaries (with reproducibility notes) once binaries are
   published.
7. Marketplace listings (PikaPods, Cloudron, Elestio, Umbrel) and the
   announcement (r/selfhosted, awesome-selfhosted PR) land together.

Between the two events, development continues in the open — a repo with
visible history and activity reads better at announcement time than one that
appeared yesterday.

---

## Competitor Feature Gap Analysis

Reference apps for a self-hosted Microsoft Money / Quicken successor. ✅ = solid,
🟦 = partial/backend-only, ⬜ = missing.

| Capability | Rekenraam | Quicken | GnuCash | YNAB | Firefly III | Money Manager Ex |
|---|---|---|---|---|---|---|
| Double-entry ledger | ✅ | partial | ✅ | ⬜ (envelope) | ✅ | partial |
| Multi-currency accounts | ✅ | ✅ | ✅ | limited | ✅ | ✅ |
| Account register + same-day order | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Reconciliation workflow | ✅ | ✅ | ✅ | ⬜ | ✅ | ✅ |
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
| Investment lots & cost basis | ✅ | ✅ | ✅ | ⬜ | partial | ✅ |
| Dividends / corporate actions | ✅ | ✅ | ✅ | ⬜ | ⬜ | partial |
| Price/FX history + scheduled refresh | 🟦 | ✅ | ✅ | ⬜ | ✅ | partial |
| Realized/unrealized gains reporting | ✅ | ✅ | ✅ | ⬜ | ⬜ | partial |
| Payee/category cleanup & merge | ⬜ | ✅ | partial | ✅ | ✅ | partial |
| Self-hosted, single binary, no telemetry | ✅ | ⬜ | ✅ | ⬜ | ✅ | ✅ |
| Mobile-responsive web | ✅ | app | ⬜ | ✅ | ✅ | ⬜ |

**Reading of the gap:** Rekenraam's ledger core is competitive with GnuCash and
Firefly and ahead of YNAB on accounting correctness. The decisive missing pieces
for an everyday Money/Quicken replacement are, in order: (1) reconcile + reports
UI to expose the engine that already exists, (2) import — the single biggest
manual-entry reducer every competitor ships, (3) budgets and recurring
transactions, the daily-driver planning loop, and (4) surfacing the remaining
FX/pricing backend (investments now ship end-to-end with UI). Exports are table
stakes and cheap.

---

## Now — close the trust loop (finish Phase 3)

The backend already computes reconciliation and reports; the gap is purely UI.
This is the highest-leverage work because it exposes shipped engines.

### R1. Reconciliation workflow UI — ✅ shipped
The core reconcile loop ships at `routes/app/reconcile` (`implemented.md`):
- Start-reconciliation flow (statement date + ending balance) per account.
- Clear/unclear postings against a server-authoritative live difference.
- Finish enabled only when the difference is zero (the backend asserts it); discard
  to abandon.
- Prior active checkpoints shown read-only as reconciled-through context.
- **Deferred (follow-up):** void-checkpoint controls and out-of-session
  mark-cleared UI (APIs exist); these sit outside the R1 trust loop.

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

### R4. Pipeline + QIF import (MS Money migration) — ✅ Slice 1 shipped
The first slice landed the parser interface + staging/preview/commit pipeline
and the **QIF adapter** — the MS Money migration path (MS Money exports
per-account loose QIF; its `.mny` has no clean export).

Shipped:
- `SourceAdapter` interface + registry; `QIFAdapter` handling all standard field
  codes including splits, transfers (`[Account]` syntax), and investment warnings.
- SHA-256 fingerprint dedupe: within-batch + ledger-level (`import_commit_identities`).
- 5 DB tables (`import_batches`, `import_batch_events`, `import_staged_rows`,
  `import_commit_identities`, `import_profiles`) with goose migration `0004_import_core.sql`.
- 7 REST endpoints: `POST /imports`, `GET /imports`, `GET /imports/{id}`,
  `PATCH /imports/{id}`, `POST /imports/{id}/preview-commit`,
  `POST /imports/{id}/commit`, `POST /imports/{id}/discard`.
- Upload → preview (per-row account / currency / category / transfer-account
  assignment) → commit → result UI at `routes/app/import`.
- Transfer detection: rows with a QIF `[Account]` transfer hint show an account
  picker in the preview and route to `transfer_account_id` in the resolution.
- Partial-commit semantics: each row commits in its own DB transaction; failures
  don't block the rest.
- Known gaps carried to backlog: crash-consistency hole (T-06), OpenAPI spec (T-07),
  per-split category routing deferred to R6.

### R5. CSV + provider profiles
CSV/XLSX differ wildly per bank, so column mapping is **saved profile data, not
code**. Adds the CSV adapter + the column-mapping profile engine/UI.
- Save a per-bank profile once; the next statement is one click.

### R6. XLSX, OFX/QFX, duplicate-review depth
- XLSX adapter (reuses the CSV row pipeline); OFX/QFX (FITID dedupe).
- Richer duplicate review, import audit-trail/history, batch rollback (via void).
- Per-split category mapping: currently all splits post to a single user-selected
  category; R6 should show per-split category selectors in the preview UI and route
  each split to its own `CategoryID` derived from `category_hint` in normalized JSON.

### R7. Online ingestion + payee/category cleanup — 🟦 Slices 1–3, 4a shipped
- Online sources implement the same adapter contract, driven by the durable work
  queue (ADR 0010) so refreshes are restart-safe — same model as FX coverage.
- **First provider: Trading 212** (token-based public API, no OAuth/PSD2). Proves
  the online model end-to-end: encrypted credential storage, durable polling,
  provider-id dedupe, native-currency money. **Full design:
  `docs/trading212-import-plan.md`.**
- **Slice 1 shipped (2026-06-28):** `internal/secretbox` credential store, `import_connections`
  table (migration `0007`), `ImportConnectionService` with probe-before-store + key masking,
  4 REST endpoints, OpenAPI, TS types, connections UI (masked list + add + delete).
- **Slice 2 shipped (2026-06-30):** `internal/onlinesource/trading212` HTTP fetcher
  (paging, 429 backoff, cursor), `Trading212Adapter`, real `Trading212Prober`.
- **Slice 3 shipped (2026-06-30, hardened 2026-07-01):** durable
  `import.fetch.trading212` worker (`app/import_fetch_worker.go`),
  content-negotiated `POST /imports` (JSON → online fetch, multipart →
  unchanged file upload), `POST /import-connections/{id}/refresh`
  (incremental), double-fetch guard, terminal-vs-retryable failure handling,
  per-connection Import/Refresh UI with polling, pagination beyond 50 pages
  via continuation work items (T-14), same-timestamp cursor + absolute
  `nextPagePath` hardening (T-17). Trading 212 cash-movement import is now
  end-to-end functional.
- **Slice 4a shipped (2026-07-01):** scheduled auto-refresh
  (B-T212-SCHED) — a per-connection toggle drives a 24h-since-last-fetch
  scheduler (`ImportService.StartScheduler`, `app/import_scheduler.go`)
  reusing the existing manual-refresh path and its in-flight guard; no new
  fetch logic.
- **Remaining (Slice 4b, planned but not built):** investment lot import
  (B-T212-INVST), blocked on a new prerequisite design doc,
  `docs/import-connection-accounts-plan.md` (import connections have no
  relationship to accounts today, which BUY/SELL lot mapping needs and plain
  cash-movement rows didn't). See `docs/trading212-import-plan.md` Slice 4b
  for the concrete plan.
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

### R12. Investments UI + gains reporting — **complete** (unblocks Trading 212 import)
- **Slice 1 (correctness gate) complete**: all four cost-basis methods (fifo/lifo/average_cost/specific_lot); I-01 and I-02 closed; 3-tier method resolution; account-level `cost_basis_method`; sell-preview endpoint. Full test coverage.
- **Slice 2 (read-only UI) complete**: OpenAPI spec for all 24 investment endpoints; generated TS types; positions page with lot drill-down; nav link.
- **Slice 3 (buy/sell/dividend entry UI) complete**: buy form; sell form with server-computed preview + specific-lot picker; dividend + reinvested-dividend forms.
- **Slice 4 (gains reporting + provider-event review UI) complete**: `GET /investments/gains` (realized from `investment_lot_events` + cash proceeds join; unrealized from positions × latest price; sign: disposed_basis_value stored negative, gain = proceeds + disposed_basis); Portfolio/Gains/Suggestions tabs; suggestions accept/ignore cards; `GET /investments/automation-rules`; 4 DB-layer tests.
- Report snapshots deferred (immutable event log makes past-date queries reproducible; explicit snapshot store deferred).
- **Next**: online import (R7 / Trading 212), lifting `B-T212-INVST`. Full design: `docs/investments-plan.md`.

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
