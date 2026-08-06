# Roadmap

This is the one active, forward-looking plan for Rekenraam. It answers
**what to build next**, in order. It is governed by
`docs/product-requirements.md`; shipped scope is recorded in
`docs/implemented.md`; live technical debt is in `docs/backlog.md`; the
short-horizon working queue is `docs/todo.md`.

Last reviewed: 2026-07-12. Structure updated 2026-07-19 (slice index and
pending-proposals pointer added; no priority changes). **Priorities updated
2026-08-05**: the 2026-07-19 review's seven proposals were all accepted —
see "Decisions adopted 2026-08-05" below.

## Slice index

Every R-number ever used, so references in other documents stay resolvable.
Statuses: ✅ shipped · ▶ current · ⏭ planned (ordered below) · ⏸ deliberately
later · — retired/unassigned.

| Slice | Name | Status | Primary document |
|---|---|---|---|
| R1 | Reconcile workflow screen (trust loop) | ✅ | `docs/implemented.md` (Reconciliation) |
| R2 | Reports users can act on | ▶ | `docs/plans/reports-plan.md` |
| R3 | Portable core data (CSV/QIF export) | ⏭ | this file |
| R3a | Accessibility regression coverage | ⏭ | this file |
| R4 | QIF import | ✅ | `docs/implemented.md` (Import Pipeline) |
| R5 | Ordinary-bank CSV import + profiles | ⏭ | `docs/plans/import-plan.md` |
| R6 | Import depth (XLSX/OFX, matching, rollback) | ⏸ | `docs/plans/import-plan.md` |
| R7 | Trading 212 online connections + lots | ✅ | `docs/plans/trading212-import-plan.md` |
| R7a | Daily-entry convenience | ⏸ | this file |
| R8 | Budgets | ⏭ | this file |
| R9 | Recurring transactions | ⏭ | this file |
| R10 | Projected balances / forecasting | ⏭ | this file |
| R11 | Pricing/FX management UI | ⏸ | this file |
| R12 | Investments UI + gains reporting | ✅ | `docs/plans/investments-plan.md` |
| R13 | Investment return analytics (TWR/MWR) | ⏸ | this file |
| R14 | Receipts & attachments (capture, OCR, inbox) | ⏸ | `docs/plans/receipts-plan.md` |
| R14a | Attachment storage + manual attach (after R5) | ⏭ | `docs/plans/receipts-plan.md` |
| R15 | Connections expansion (IBKR Flex → GoCardless → T-34 producer) | ⏸ | `docs/plans/connections-plan.md` |
| R16 | Investment lifecycle completeness (write-off, price void, return of capital, manual splits) | ⏭ | this file |
| R17 | Crypto instrument type + `PriceProvider` registry and quote adapters | ⏭ | this file |

## Decisions adopted 2026-08-05

The `docs/reviews/roadmap-review-2026-07-19.md` (§3, §4) proposals were
decided by the owner on 2026-08-05. **All were accepted**, each with a scope
fence recorded in the relevant section below:

| Proposal | Decision |
|---|---|
| §3a EU import correctness (T-35, T-36) | Accepted — fixed as defects *and* a hard pre-announcement gate |
| §3b R3 backups + trial-balance self-check | Accepted — self-check is read-only; a documented restore path ships with it |
| §3c Minimal import rules v1 in R5 | Accepted — contains-match, preview-time only, no retroactive apply |
| §3d R10 forecasting promotion | Accepted — planning loop is **R9 → R10 → R8** |
| §3e Investment lifecycle completeness | Accepted — new R16; manual splits gated behind a lot-mutation design note |
| §3f Personal-access tokens | Accepted — scoped, expiring, hashed, revocable; before announcement |
| §4.1 Crypto-holding expat persona | Accepted — new R17, after R16, with a standing guardrail in `product-requirements.md` |

The §2 documentation-accuracy fixes are separate and unconditional; the R12
slice-index row is fixed above.

The three remaining `plans/` questions were decided the same day:

| Question | Decision |
|---|---|
| Connections sequencing | Order adopted, **contents amended**: the quote-provider slice moves into R17 (it builds the `PriceProvider` registry once). R15 is now IBKR Flex → GoCardless → T-34 producer |
| Receipts R14a pull-forward | **No** — but R3 designs the backup/self-check with a documented attachments hook, and R14a ships after R5 |
| GoCardless / IBKR "verify" items | Not a decision — reclassified as blocking slice-start preconditions on GC-1 and IBKR-1 in `connections-plan.md` |

The R14/R15 plans (2026-07-19) are scoped and sliced but deliberately later:
their internal sequencing proposals are adopted or amended here when each
becomes current work (IBKR → quotes → GoCardless → T-34 producer; R14a
storage possibly pulled forward next to R3's backup work). The Yahoo Finance
quote-provider question inside R15 was decided 2026-08-05 (ship it, labeled
unofficial) — `docs/plans/connections-plan.md`.

## Direction

Build a polished, self-hosted personal-finance daily driver first. The order is:

1. Trust and visibility: reports and exports.
2. Lower manual effort: CSV import with reusable profiles.
3. Daily planning: budgets and recurring transactions.
4. Differentiate for multi-currency, cross-border households and investors.

The product is deliberately single-user and self-hosted. Connections are
bring-your-own-key adapters, never guaranteed coverage. Native apps,
small-business accounting, a hosted service, and broker trade execution are out
of current scope. The trade-execution decision and market analysis are retained
in `docs/reviews/competitive-analysis-2026-07.md`.

## Current plan

Do not start a new roadmap initiative until the current one has met its
acceptance criteria. Feature-specific design documents may clarify a slice, but
must not create a competing sequence.

### Now — R2: reports users can act on

Ship a reports route with net worth over time, spending by category/payee, and
cashflow. `docs/plans/reports-plan.md` is the implementation reference.

**Started:** `/app/reports` now presents the exact net-worth series with
URL-addressable date/bucket filters and an accessible per-commodity table.
Account/commodity filters, charts, and the spending/cashflow read models remain
to be delivered.

Deliver in this order:

1. Shared report-query contract and accessible report shell; net-worth and
   spending views over the existing read models.
2. Cashflow read model and view, explicitly separating inflow, outflow,
   transfers, and net movement.
3. Date/account/category/payee/commodity filters, CSV and print-friendly tables,
   and summary charts that do not replace the accessible data table.

The first implementation must be a coherent vertical slice, but this is a
**sequencing constraint, not a deleted design decision**. The fuller report
contract—including saved definitions/runs, cross-currency valuation, investment
dimensions, and later snapshots—remains in `docs/plans/reports-plan.md`. At the R2
acceptance review, explicitly decide which of those items is justified by real
use before moving to R3; do not let it disappear by omission.

### Next — R3: portable **and protected** core data

Ship core-ledger CSV and QIF export. These are mandatory product requirements,
not optional polish. The first release should favour a documented, stable export
shape over a broad structured-backup format.

Scope extended 2026-08-05 (review §3b) so the slice delivers the full trust
sentence — *"your data is exportable, backed up nightly, and provably
balanced"*:

1. **Scheduled backups.** The verified `VACUUM INTO` backup already exists but
   is reachable only from CLI recovery. Schedule it on the existing
   background-work queue with a retention policy and a visible last-backup
   status in the UI.
2. **A documented restore path.** Ships in this slice, not later. An untested
   backup is not a backup, and the claim above is dishonest without it.
3. **Trial-balance self-check.** Per-commodity posting sums ≡ 0, lots ↔
   holdings reconciliation, and SQLite `integrity_check`, surfaced in the UI.
   **Read-only and diagnostic**: it reports failures and never auto-repairs.
   No self-hosted competitor makes this claim.
4. **An attachments hook, designed but empty** (decided 2026-08-05). R14a
   will put files outside SQLite, where `VACUUM INTO` cannot reach them, so
   the backup procedure names "the database **and** the attachments
   directory" from day one and the self-check reserves a file-integrity pass
   slot. R14a itself ships after R5 — this hook exists so that "backed up
   nightly" never becomes a claim that has to be walked back.

Design the export shape so a ledger/beancount-format export is trivially
derivable later (review §4.2) — the plain-text-accounting audience is a
positioning group worth courting at announcement, at zero feature cost.

### R3a — core-workflow accessibility regression coverage

Add focused automated accessibility smoke checks for setup/auth, transaction
entry, reconciliation, reports, and import. Cover semantic controls, labels,
keyboard navigation, focus handling, and contrast violations; keep mobile
journeys in the broader Playwright suite.

### Then — R5: ordinary-bank CSV import

Ship CSV import plus saved mapping profiles. The user maps a bank statement once
and can reuse that profile for the next statement. Reuse the staged review and
commit pipeline; do not build another import path.

**Done ahead of R5:** the EU import-correctness defects T-35 (QIF `MM/DD`
parsed before `DD/MM`, profile override stubbed) and T-36 (decimal-comma
amounts 100× off) were fixed 2026-08-06 — `app/import_locale.go`. The
pre-announcement gate below is met for QIF; the CSV adapter must reuse
`canonicalDecimal` and `parseFlexibleDate` rather than re-deriving them, and
mapping profiles must expose `date_layout` and `decimal_separator`, which the
adapters already honor.

**Includes a minimal rules v1** (decided 2026-08-05, review §3c). Rules are
the retention feature for the import persona: CSV import without them means
manually categorizing every row of every statement, which is the week-two
abandonment path. The scope fence is deliberate and binding for v1:

- Ordered rules matching **contains** on payee/description, setting category,
  payee, or tags.
- Applied **inside the staged preview**, where the user already reviews rows.
- **No retroactive apply** to already-committed transactions, no regex, no
  amount predicates, no rule audit trail.

Everything outside that fence stays in the later import-rules slice.

### After that — planning loop

Order decided 2026-08-05 (review §3d): **R9 → R10 → R8**. Recurring
transactions are forecasting's data source, so R9 → R10 is a single coherent
arc that exercises the producer-owned draft machinery once instead of twice,
and it front-loads per-currency forecasting — the differentiator the parity
lens below commits to protecting. Budgets are independent of both and slot in
afterward with no rework.

1. **R9 Recurring transactions:** templates and due-entry generation into the
   reserved producer-owned draft workflow.
2. **R10 Projected balances:** per-currency projections first; converted totals
   only with explicit FX semantics. Loan helpers are optional follow-up work.
3. **R8 Budgets:** period budgets with actual-versus-budget reporting.

### R16 — investment lifecycle completeness

Decided 2026-08-05 (review §3e). `competitor-comparison.md` claims corporate
actions as shipped, but there is **no implementation** — not even manual
entry. This slice makes the moat claim honest. Sequenced after R5, and
split by risk:

1. **Now — independent and small:** zero-proceeds write-off (T-38), price
   observation voiding (T-37 — **done 2026-08-06**, backend endpoint only;
   the operator-facing surface still belongs to R11), return-of-capital as
   basis reduction. None needs a provider feed.
2. **Behind a design note — manual split / reverse-split entry.** Splits
   mutate historical lots, which is the unbuilt half of T-34 and touches the
   ledger code both 2026-07 audits certified as correct. Write the
   lot-mutation design before the code. A migrant's first AAPL split must not
   require deleting and re-entering lots.

### R17 — crypto instrument type

Decided 2026-08-05 (review §4.1): widen the persona to crypto-holding
expats, sequenced after R16. The audits found the lot engine is
commodity-kind-agnostic and already handles scale-24 crypto commodities end
to end via the API, so the work is an instrument type and a UI entry point.
Nobody self-hosted offers crypto with real cost-basis accounting.

**R17 also owns the `PriceProvider` registry** (moved here from R15 on
2026-08-05). Crypto needs prices, so the registry gets built either way —
building it once, here, avoids two slices both claiming it. Mirror the
existing FX registry and land three adapters against it: **Yahoo Finance**
(keyless, broadest EU coverage, the zero-setup default, labeled unofficial),
**CoinGecko** (BYO key, crypto), and one BYO-key equity provider (Twelve
Data or Alpha Vantage — pick on measured EU-exchange coverage, which is the
persona's actual requirement). Scheduled refresh reuses the pricing worker
and refresh-run bookkeeping wholesale. This also closes the unrealized-gains
staleness gap the 2026-07-19 audit flagged (§4), well before R15.

**Standing guardrail** (also recorded in `product-requirements.md`): scope is
manually/CSV-entered, priced holdings with lots. Exchange integrations, DeFi
positions, staking, and NFTs are **rejected, not deferred** — they are a
coverage promise the adapter rule forbids and a maintenance tarpit.

## Deliberately later

These are valuable, but they are not allowed to displace the current plan:

- R6: XLSX and OFX/QFX adapters, import matching, per-split mapping, batch
  rollback, and richer import history. `docs/plans/import-plan.md` retains the detailed
  data, lifecycle, and acceptance design.
- Import rules **beyond the R5 v1 fence**: regex and amount predicates,
  retroactive re-run over committed transactions, and a rule audit trail.
  Also duplicate payee merge, bulk recategorization, and the imported
  `needs_review` queue. (The minimal contains-match rules are in R5 as of
  2026-08-05.)
- R15 connections expansion, in this order (adopted 2026-08-05):
  **IBKR Flex Query → GoCardless EU/UK banks → the T-34 dividend/
  corporate-action event producer**, then SimpleFIN Bridge (US) later.
  Security quotes moved out of R15 into R17. CSV mapping-profile presets for
  API-less brokers (Trade Republic, DeGiro, Raisin, HL/AJ Bell/ii) are R5
  documentation-plus-fixtures tasks, not adapters. All bring-your-own-key;
  assessment matrix, free-vs-paid verdicts, and slice designs in
  `docs/plans/connections-plan.md`. Both first slices carry blocking
  provider-verification preconditions — see that plan.
- R7a daily-entry convenience: transaction templates, payee defaults, saved
  views, and keyboard-first entry.
- R11 pricing/FX management UI.
- R13 investment return analytics (TWR/MWR, allocation, benchmark comparison).
- R14 receipts & attachments: durable attachment storage (resolves the open
  attachment product decision), receipt capture with in-browser OCR, and a
  match-or-draft inbox using the reserved draft-producer workflow —
  `docs/plans/receipts-plan.md`. **R14a ships after R5**, not alongside R3
  (decided 2026-08-05): it is not an announcement gate, so it does not go
  between two slices that are. R3 carries the attachments hook instead.
- Multi-currency reporting, report snapshots, multi-user, and household
  features.

## Competitor and parity check

Keep `docs/competitor-comparison.md` as the maintained parity matrix and
`docs/reviews/competitive-analysis-2026-07.md` as the dated deep dive. Before declaring
each roadmap initiative complete, update the comparison's implication section
and record either the parity gained or the deliberate gap retained.

The current parity lens is:

- **Money/Quicken/Monarch:** reports, imports, exports, budgets, and recurring
  transactions determine whether the app feels like an everyday replacement.
- **Firefly III:** reusable import profiles and user-defined import rules drive
  import-heavy retention; bank coverage itself is not a promise.
- **PocketSmith:** per-currency cashflow forecasting is the differentiator to
  protect in R10, not an optional conversion afterthought.
- **Ghostfolio and Portfolio Performance:** returns analytics, allocation, and
  benchmark comparison remain the investment-expectation gap after gains.
- **Rekenraam's moat:** exact multi-currency double-entry and lot-level
  investments must stay coherent as parity features are added.

## Public-release gates

These are a parallel release-readiness track, not a reason to delay local
daily-driver work.

### Before making the repository public

1. Scan the complete Git history for secrets and remove scanner bait or real
   secrets before anyone clones it.
2. Keep AGPL-3.0, `SECURITY.md`, dependency scanning, and vulnerability scanning
   current; enable GitHub secret/push protection when available.

### Before public announcement or marketplace listings

1. Complete R2, R3, and R5 so a newcomer can migrate, inspect, and export data.
2. **EU import correctness: T-35 and T-36 fixed and regression-tested**
   (added 2026-08-05, review §3a) — done 2026-08-06 for the QIF path
   (`app/import_locale.go`); re-check when the CSV adapter lands in R5.
   The announcement's centerpiece is the
   migration story, the QIF parser targets MS Money exports, and the persona
   is European — so the demo the launch rests on currently corrupts dates and
   amounts for exactly the target audience. A correctness-branded finance app
   does not get a second first impression.
3. **Personal-access tokens shipped** (added 2026-08-05, review §3f). The
   typed OpenAPI surface is the foundation for an ecosystem, but session
   cookie + CSRF header auth means no script, tool, or community client can
   call it. Announcement is the moment of maximum developer attention.
   Required shape: hashed at rest, scoped, expiring by default, revocable,
   and emitting authentication events.
4. Produce signed release binaries with reproducibility notes.
5. Prepare adoption assets: seeded demo, README screenshots, and a short
   migration walkthrough — courting the plain-text-accounting audience
   explicitly by leading with the correctness architecture (append-only
   versions, exact decimals, trial balance).

Public internet deployment with real financial data remains blocked by the live
security gates in `docs/backlog.md`: lockout-safe login throttling, MFA, and
authentication-event visibility.

## Open product decisions

Resolve these only when their related slice becomes current work:

- Export scope beyond core ledger CSV/QIF, including a full structured JSON
  backup of settings and metadata.
- First non-English UI languages and the highest-priority mobile workflow.
- Attachment storage, retention, access-control, backup, and encryption model
  — a proposed resolution now exists in `docs/plans/receipts-plan.md` (R14a);
  decide when that slice is scheduled.
- I-03: whether gains reporting should offer read-only analytical methods in
  addition to the authoritative lot-disposal method.
- I-04: whether realized gains/losses should remain computed reporting values or
  become ledger postings with an explicit account convention.

## Completed milestones

Foundation, accounts, ledger transactions, reconciliation UI, QIF import,
Trading 212 ingestion (including investment lots), and investments UI/gains are
shipped. See `docs/implemented.md` for the capability ledger and the historical
design records for rationale.
