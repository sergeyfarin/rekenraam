# Roadmap

This is the one active, forward-looking plan for Rekenraam. It answers
**what to build next**, in order. It is governed by
`docs/product-requirements.md`; shipped scope is recorded in
`docs/implemented.md`; actionable work is in
[GitHub Issues](https://github.com/sergeyfarin/rekenraam/issues), with local
IDs mapped in `docs/backlog.md`; the short-horizon queue is `docs/todo.md`.
The [GitHub roadmap index](https://github.com/sergeyfarin/rekenraam/issues/120)
links current slices to their actionable tickets; this file remains the
ordered product plan.

Last reviewed: 2026-10-02. The current order is R16,
R11 price management, R17 quotes and crypto, R18 gains projections, then R13
returns analytics. Cross-border input and date correctness run in parallel.
Prior roadmap detail is retained in the
[completed roadmap record](reviews/completed-roadmap-2026-09-27.md).

## Slice index

Every R-number ever used, so references in other documents stay resolvable.
Statuses: ✅ shipped · ◐ partly shipped ahead of its slice · ▶ current ·
⏭ planned (ordered below) · ⏸ deliberately later · — retired/unassigned.

| Slice | Name | Status | Primary document |
|---|---|---|---|
| R1 | Reconcile workflow screen (trust loop) | ✅ | `docs/implemented.md` (Reconciliation) |
| R2 | Reports users can act on | ✅ | `docs/plans/reports-plan.md` |
| R3 | Portable **and protected** core data (CSV/QIF export, backups, restore, self-check) | ✅ | `docs/plans/data-portability-plan.md` |
| R3a | Accessibility regression coverage | ✅ | this file |
| R4 | QIF import | ✅ | `docs/implemented.md` (Import Pipeline) |
| R5 | Ordinary-bank CSV import + profiles | ✅ | `docs/plans/import-plan.md` |
| R6 | Import depth (XLSX/OFX, matching, rollback) | ⏸ | `docs/plans/import-plan.md` |
| R7 | Trading 212 online connections + lots | ✅ | `docs/plans/trading212-import-plan.md` |
| R7a | Daily-entry convenience | ⏸ | this file |
| R8 | Budgets | ✅ | `docs/plans/budgets-plan.md` |
| R9 | Recurring transactions | ✅ | `docs/plans/recurring-transactions-plan.md` |
| R10 | Projected balances / forecasting | ✅ | `docs/plans/projected-balances-plan.md` |
| R11 | Pricing/FX management UI | ⏭ | this file |
| R12 | Investments UI + gains reporting | ✅ | `docs/plans/investments-plan.md` |
| R12a | Investment journal/subledger integrity correction | ✅ | `docs/plans/investment-integrity-plan.md`, ADR 0012 |
| R13 | Investment return analytics (TWR/MWR) | ⏭ | this file |
| R14 | Receipts & attachments (capture, OCR, inbox) | ⏸ | `docs/plans/receipts-plan.md` |
| R14a | Attachment storage + manual attach (after R5) | ⏭ | `docs/plans/receipts-plan.md` |
| R15 | Connections expansion (IBKR Flex → GoCardless → T-34 producer) | ⏸ | `docs/plans/connections-plan.md` |
| R16 | Investment lifecycle completeness (correction, transfers, basis actions, splits, short sales) | ▶ | `docs/plans/investment-operation-refactor-plan.md`; ADR 0013 |
| R17 | Crypto instrument type + `PriceProvider` registry and quote adapters | ⏭ | this file |
| R18 | Reproducible investment basis + gains projections | ⏭ | ADR 0012; plan required after R16/R17 |

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
| §3e Investment lifecycle completeness | Accepted — R16 framework in `docs/plans/investment-operation-refactor-plan.md`; each structural action still needs its posting, basis, and date specification before implementation |
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

The R14/R15 plans (2026-07-19) remain deliberately later. Their sequencing
was decided on 2026-08-05: quotes belong to R17; R15 is IBKR → GoCardless →
T-34 producer. R14a stays after R5 and was not pulled forward alongside R3. The Yahoo Finance
quote-provider question inside R15 was decided 2026-08-05 (ship it, labeled
unofficial) — `docs/plans/connections-plan.md`.

## Direction

Build a polished, self-hosted personal-finance daily driver first. The order is:

1. Trust and visibility: reports and exports.
2. Lower manual effort: CSV import with reusable profiles.
3. Repair any trust-boundary defect before adding more financial producers.
4. Daily planning: budgets and recurring transactions.
5. Differentiate for multi-currency, cross-border households and investors.

The product is deliberately single-user and self-hosted. Connections are
bring-your-own-key adapters, never guaranteed coverage. Native apps,
small-business accounting, a hosted service, and broker trade execution are out
of current scope. The trade-execution decision and market analysis are retained
in `docs/reviews/competitive-analysis-2026-07.md`.

## Current plan

Do not start a new roadmap initiative until the current one has met its
acceptance criteria. Feature-specific design documents may clarify a slice, but
must not create a competing sequence.

The reusable application runtime [#113](https://github.com/sergeyfarin/rekenraam/issues/113)
and operation integrity [#125](https://github.com/sergeyfarin/rekenraam/issues/125)
are complete. Long buy/sale correction has shipped within #99's now-bounded
scope; remaining correction families have separate acceptance issues.
The [2026-10-02 review](reviews/investment-review-2026-10-02.md) explains the
revised order and qualified findings.

### Completed initiatives through R10

R2 reports, R3 portability and backups, R3a accessibility, the reporting
currency selector, and the R9 → R10 → R8 planning loop are shipped. See the
[capability ledger](implemented.md) for current behavior and the
[dated completed-roadmap record](reviews/completed-roadmap-2026-09-27.md)
for the original scope and acceptance notes.

### R16 — investment lifecycle completeness

ADR 0013 defines the named operation/side foundation. The
[active operation plan](plans/investment-operation-refactor-plan.md) contains
the contract and gates; [implemented](implemented.md) records shipped commands.
The foundation, trade economics, long-buy/sale reversal/replacement, backdated
buy replay and Trading 212 quantity/net source corrections ship. Their writer
convergence and operation-integrity gates are complete. Buy and reinvestment
checkpoint previews prove replay feasibility through the rolled-back writer
and report its actual invalidation set. Shared reconciliation resolution now
also reports the full write set for generic transactions and other previews
(T-127 #142); per-boundary date/sequence and net-delta semantics remain #135.

The shared gain-impact mechanism and its manual-buy pilot (T-114 #129) ship;
manual buys now disclose and require acknowledgement of revised committed gains.
Near-term focus is **#137 now**, then #138 → #132 → #139.
Run #141’s existing-path safety rollout in parallel, starting
with reinvestment/imported acquisitions and continuing with native corrections
and source revisions. Deliver bounded slices alongside new commands; completion
of the entire rollout is not a prerequisite for splits or pooled transfers.
Closed #99/#125/#113/#127/#142 are historical evidence, not active gates.
Priority labels describe urgency; this sequence does not add hard dependencies.

Remaining work, in order:

1. **Existing-path gain disclosure rollout [T-126 #141](https://github.com/sergeyfarin/rekenraam/issues/141)** (parallel P1).
   The shared mechanism and manual-buy pilot shipped under [T-114 #129](https://github.com/sergeyfarin/rekenraam/issues/129).
   Reinvestment, imported acquisitions, native corrections and source revisions
   still replay without disclosure. New commands opt in themselves and do not
   wait for that entire matrix. R18 retains historical reporting and tax
   profiles; no permanent filed-through date has been adopted.
2. **Manual split/reverse split plus verified Trading 212 mapping [T-122 #137](https://github.com/sergeyfarin/rekenraam/issues/137).**
   Missing split effects can block later sales in instrument migration. Follow
   the existing exact-ratio/basis-conservation contract; hold insufficient
   provider evidence in review.
3. **Pooled average-cost internal transfer [T-123 #138](https://github.com/sergeyfarin/rekenraam/issues/138).** Explicit-lot
   internal transfers and known-basis external inbound already ship. Average
   pools remain refused until exact carried-basis allocation is implemented.
4. **Broader backdated replay [T-117 #132](https://github.com/sergeyfarin/rekenraam/issues/132).** Transfer-in behind
   later sales and earlier disposal replay. Reinvestment already admits earlier
   openings and now proves preview feasibility; gain disclosure belongs to T-126.
   Current earlier sales without a later disposal remain supported.
5. **Cross-position replay decision and effective-reader consolidation [T-124 #139](https://github.com/sergeyfarin/rekenraam/issues/139).**
   Settle before outbound transfers and compound actions. Full rebuild and
   affected dependency closure both need durable elections, dated bridges and
   atomic rollback; rebuilding alone cannot eliminate legitimate refusals.
6. **Remaining lifecycle families.** Dividend/reinvestment correction
   [T-115 #130](https://github.com/sergeyfarin/rekenraam/issues/130); trade field correction [T-116 #131](https://github.com/sergeyfarin/rekenraam/issues/131); write-off correction
   [T-118 #133](https://github.com/sergeyfarin/rekenraam/issues/133); transfer correction [T-119 #134](https://github.com/sergeyfarin/rekenraam/issues/134); correction-chain register
   and net checkpoint impact [T-120 #135](https://github.com/sergeyfarin/rekenraam/issues/135). Provider cancellation/wider source
   revision [T-121 #136](https://github.com/sergeyfarin/rekenraam/issues/136) is evidence-blocked, not an active P0 gate.
7. **Outbound/unknown transfers, return of capital and cash in lieu**, under
   [#114](https://github.com/sergeyfarin/rekenraam/issues/114) and the
   [slice 5 contract](plans/investment-operation-slice-5-contract.md), then
   **short sale/cover [T-108 #103](https://github.com/sergeyfarin/rekenraam/issues/103)**,
   then **compound actions [#115](https://github.com/sergeyfarin/rekenraam/issues/115)**.
   Keep each command independently runnable and audited. Negative holdings
   without named short events remain unclassified warnings.

Zero-proceeds write-off and price observation voiding ship as backend commands;
write-off UI and the R11 price operator surface remain follow-ups. Provider
return-of-capital/cash-in-lieu suggestions stay review-only until supported.
The exact monetary JSON boundary and bundle schema 5 are shipped; changes to
baseline/export contracts require fresh and seeded validation under ADR 0013.
Race-gate performance improvement [T-125 #140](https://github.com/sergeyfarin/rekenraam/issues/140) is independent trust work;
retain all financial and race coverage while measuring safe scheduling changes.

### R11 — price and FX management UI, promoted after R16

Give the existing price/FX repository and price-void operation an operator
surface before R17 adds more quote providers. The screen must show observation
source, as-of date, trust/approximation and age; permit manual entry,
correction/void with a dependency preview; and explain when no usable price
exists. Keep historical source observations auditable. Write the detailed
acceptance plan when R11 becomes current. This is a valuation-trust gate, not a
new automatic provider or a change to ledger postings.

### Parallel trust work for cross-border entry

Before presenting the five drafted non-English catalogs as complete or using
them in a migration demo, close G-08 (locale-aware amount input), T-87
(owner-local default dates), and T-80 (catalog parity and native review).
These are independently shippable correctness and communication fixes; they do
not require waiting for R16 or R11. The actionable tickets live in GitHub
Issues; `docs/backlog.md` retains the ID mapping.

### R17 — crypto instrument type

Decided 2026-08-05 (review §4.1): widen the persona to crypto-holding
expats, sequenced after R16 and the R11 operator surface. The audits found the lot engine is
commodity-kind-agnostic and already handles scale-24 crypto commodities end
to end via the API, so the work is an instrument type and a UI entry point.
Do not claim that this combination is unique without a separate market-wide
verification.

**R17 also owns the `PriceProvider` registry** (moved here from R15 on
2026-08-05). Crypto needs prices, so the registry gets built either way —
building it once, here, avoids two slices both claiming it. Mirror the
existing FX registry and evaluate three candidate adapters: **Yahoo Finance**
(keyless and labeled unofficial), **CoinGecko** (crypto), and one BYO-key equity
provider (Twelve Data or Alpha Vantage). Verify current API terms, EU-exchange
coverage, rate limits and key requirements before implementation. Scheduled
refresh reuses the pricing worker
and refresh-run bookkeeping wholesale. This also closes the unrealized-gains
staleness gap the 2026-07-19 audit flagged (§4), well before R15.

**Standing guardrail** (also recorded in `product-requirements.md`): scope is
manually/CSV-entered, priced holdings with lots. Exchange integrations, DeFi
positions, staking, and NFTs are **rejected, not deferred** — they are a
coverage promise the adapter rule forbids and a maintenance tarpit.

### R18 — reproducible investment basis and gains projections

Build after R12a has trustworthy immutable events, T-76 preserves disposal
policy provenance, R16 supplies the missing
corporate-action/basis-adjustment lifecycle, and R17 supplies explicit quote
provider and staleness policy. Write a dedicated plan before implementation.

The plan must specify named basis profiles rather than a tax-engine promise. A
profile states purpose/scope, cost-basis method, effective period/date basis,
valuation date and recorded-at knowledge cutoff, quote/source policy, FX method,
staleness, reporting currency, and rounding. It produces read-only operational,
alternative-method, broker-comparison, and as-of realized/unrealized projections
without mutating operational lots. Persist a report definition/run or snapshot
only where immutable inputs plus versioned policy cannot cheaply reproduce it.

Research inside the planning slice must answer which measures belong on the
portfolio dashboard versus a period report, how a user labels a jurisdiction or
accounting purpose without Rekenraam claiming tax compliance, and how unrealized
movement is presented without price-refresh flip-flop. I-04 is narrowed by ADR
0012: reports never silently post gains; if formal realized-gain, revaluation, or
tax-liability entries are wanted, scope an explicit linked accounting workflow
and decide separately whether it belongs in R18.

### R13 — investment returns and allocation analytics

Follow R18's reproducible valuation and basis projections. Plan explicit
TTWROR/TWR and money-weighted/IRR equations, external-flow classification,
portfolio versus security scope, missing-price behavior, FX policy, benchmark
selection, and period reproducibility before implementing charts. Use
[Portfolio Performance's documented methods](https://help.portfolio-performance.info/en/concepts/performance/time-weighted/)
as a comparison, not an implicit formula choice. Keep the figures read-only;
reports do not create postings.

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
  views, and keyboard-first entry plus general entry review/edit helpers such
  as previous/next entry, open/edit, save-and-next, duplicate, split, and
  audit-safe bulk changes. Add user-customizable action bindings and selectable
  shortcut presets modelled on Rekenraam, Quicken, Microsoft Money, and other
  documented schemes (for example, an MS Money-style `Ctrl+M` reconciled
  action). Presets are starting points rather than claims of full compatibility:
  conflicts and platform differences must be visible, commands must remain
  discoverable and have mouse/touch equivalents, and no shortcut may bypass
  reconciliation guards, confirmations, or audit history.
- R14 receipts & attachments: durable attachment storage (resolves the open
  attachment product decision), receipt capture with in-browser OCR, and a
  match-or-draft inbox using the reserved draft-producer workflow —
  `docs/plans/receipts-plan.md`. **R14a ships after R5**, not alongside R3
  (decided 2026-08-05): it is not an announcement gate, so it does not go
  between two slices that are. R3 carries the attachments hook instead.
- Report snapshots, multi-user, and household features. Core
  reporting-currency conversion is already shipped.

## Competitor and parity check

Keep `docs/competitor-comparison.md` as the maintained parity matrix and
`docs/reviews/competitive-analysis-2026-07.md` as the dated deep dive. Before declaring
each roadmap initiative complete, update the comparison's implication section
and record either the parity gained or the deliberate gap retained.

The 2026-09-27 primary-source recheck in
`docs/competitor-comparison.md` supports the following order: complete R16's
correction, transfer, basis and split workflows; provide the R11 price/FX
operator surface; add the R17 quote registry and crypto entry; then build
R18 reproducible gains and R13 returns. G-08 locale input and T-87 owner-local
date defaults are parallel correctness work. This ordering is a product
inference from verified competitor workflows and the present code boundary,
not a claim of feature uniqueness.

## Public-release gates

**Fourth-pass review (2026-09-19):**
`docs/reviews/ledger-investments-fourth-pass-2026-09-19.md` verifies that the
previous six regression tests pass, and found an earlier investment-role
validation window (reopened T-100, P1), precision-dependent realized gains
(T-101, P2), and non-cash legs counted in cashflow transfers (T-102, P2).
**All three are fixed as of 2026-09-19**, each with named regression tests that
fail without their fix and with the reviewer's own probes passing, so the
previous reproductions are closed. Subsequent test hardening found **T-103
(P2): partial-disposal basis depends on purchase text precision**, **fixed
2026-09-20** by fixing the allocation precision to the cost commodity's own
maximum scale, one scale per position, with explicit range backoff — so the
original code-side reproduction is closed. Follow-up verification found T-104
(later acquisitions could exceed already widened projection range), fixed by
atomic range admission checks with active sequence regressions. Gains-summary
display now uses standard currency precision; see
`docs/reviews/allocation-and-gains-verification-2026-09-20.md`. See
`docs/reviews/financial-test-hardening-2026-09-19.md`. T-94's draft-promotion
correction remains verified. The household dry run and investment feature
limits still remain required; the candidate baseline must be validated after
ADR 0013's pre-release redesign.

T-106's dated negative-position detection and net-worth warning shipped
2026-09-26. Out-of-order imports remain accepted; a genuine short sale needs
an explicit named workflow (T-108). T-107's investment money JSON precision
boundary also closed on 2026-09-26.

These are a parallel release-readiness track, not a reason to delay local
daily-driver work.

The earlier v0.1 migration freeze and reliability dispositions are recorded in
`docs/reviews/v0.1-release-triage-2026-09-11.md`. ADR 0013 supersedes the
unused candidate's freeze before installation. The first-pass fixes close
their original seven reproductions and the second-pass fixes close its four, so
the latest review above now governs the code release gate; the non-code gates
also still stand.
Public-announcement work below is a separate, later gate.

### Public repository hygiene

The repository is already public (verified 2026-09-27). `LICENSE`,
`SECURITY.md`, Dependabot and the `govulncheck` workflow exist. Verify the
complete Git history for secrets and confirm the repository's GitHub secret
scanning and push-protection settings; the current checkout alone cannot
establish those remote settings. Maintain the disclosure and dependency
scanning paths before an announcement.

### Before public announcement or marketplace listings

1. ✅ Complete R2, R3, and R5 so a newcomer can migrate, inspect, and export
   data. All three shipped.
2. ✅ **EU import correctness: T-35 and T-36 fixed and regression-tested**
   (added 2026-08-05, review §3a) — done 2026-08-06 for the QIF path
   (`app/import_locale.go`). **CSV re-check done 2026-09-12** now that R5 has
   landed, and it found one more (T-91): both adapters resolved the *date*
   layout across the whole file but the *decimal separator* one value at a
   time, so a German export's "1.234" read as 1.234 rather than 1234 — a
   silent 1000x error on the exact migration path the announcement leads with.
   Now detected file-wide for both adapters, the way dates already were.
   The announcement's centerpiece is the
   migration story, the QIF parser targets MS Money exports, and the persona
   is European — so the demo the launch rests on must not corrupt dates and
   amounts for exactly the target audience. A correctness-branded finance app
   does not get a second first impression.
3. **Cross-border entry trust — open.** Close G-08's locale-aware amount input
   and T-87's owner-local default dates before a multilingual migration demo.
   Fill T-80's 65 missing message keys per non-English locale and arrange
   native review before describing those catalogs as complete.
4. **Personal-access tokens — open** (added 2026-08-05, review §3f). The
   typed OpenAPI surface is the foundation for an ecosystem, but session
   cookie + CSRF header auth means no script, tool, or community client can
   call it. Announcement is the moment of maximum developer attention.
   Required shape: hashed at rest, scoped, expiring by default, revocable,
   and emitting authentication events.
5. ◐ Produce signed release binaries with reproducibility notes.
   **Reproducibility done 2026-09-12** (T-92): the build is `-trimpath` +
   `CGO_ENABLED=0`, so it is static, carries none of the builder's paths, and
   two builds of a commit hash identically — see `docs/developer-workflow.md`
   § Reproducibility. **Signing is still open** and needs an owner decision:
   a signing identity (Sigstore keyless via GitHub OIDC, or a held key) and a
   release workflow, neither of which exists yet — there is no release job in
   `.github/workflows/`, only `ci.yml` and `govulncheck.yml`.
6. Prepare adoption assets: seeded demo, README screenshots, and a short
   migration walkthrough — courting the plain-text-accounting audience
   explicitly by leading with the correctness architecture (append-only
   versions, exact decimals, trial balance).

**Every public-deployment security gate is closed, and parked pending an
internet-exposed deployment.** MFA (S-06) shipped 2026-08-07 as TOTP plus
single-use recovery codes, with a Settings → Security screen and the second
step in the login flow; lockout-safe login throttling (S-04) and
authentication-event visibility (S-07) closed 2026-08-06. **Parked 2026-08-19
(owner decision):** the app is self-hosted locally for now, so none of S-04,
S-06, or S-07 is scheduled work; the trigger to unpark is the first time an
internet-exposed deployment is planned, R3 at the earliest. What is left
before that point is an operator action rather than product work: the owner
account must be **enrolled** before real financial data goes on the internet,
and `REKENRAAM_SECRET_KEY` must be configured for enrolment to be possible.
See `docs/deployment-security.md`.

## Open product decisions

Resolve these only when their related slice becomes current work:

- Export scope beyond core ledger CSV/QIF, including a full structured JSON
  backup of settings and metadata.
- The highest-priority mobile workflow. (First non-English UI languages were
  decided 2026-08-19: Spanish, French, Dutch, German, Russian — recorded in
  `product-requirements.md`. Native review of the drafted translations still
  needs an owner to pick a reviewer per language.)
- Attachment storage, retention, access-control, backup, and encryption model
  — a proposed resolution now exists in `docs/plans/receipts-plan.md` (R14a);
  decide when that slice is scheduled.
- I-03 / I-04 (gains reporting): **architectural boundary decided 2026-08-29
  in ADR 0012; product research is scheduled inside R18 after R12a/R16/R17.**
  The remaining work is not a yes/no choice because:
  - realized and unrealized gains answer different questions, and which one a
    user should see depends on what they are trying to learn;
  - tax treatment differs by country — some tax realized gains, some tax
    unrealized — so a single hard-coded presentation cannot serve the persona;
  - unrealized figures move with every price refresh, so a naive presentation
    flip-flops and reads as instability rather than information.

  The R18 plan must produce a recommendation for: which measure is authoritative
  where, when each is shown, how the jurisdiction difference is expressed
  without turning the app into a tax engine, and how to present unrealized
  movement without flip-flopping. I-03 is a named read-side projection. ADR 0012
  answers I-04's boundary now: viewing a report never posts; any accounting entry
  is a separate explicit linked workflow whose inclusion remains an R18 scope choice.

  **This does not block R12a, R16 slice 1, or R17.** Zero-proceeds write-off (T-38), price
  observation voiding (T-37), and return of capital are lot-lifecycle work; they
  use whatever gains treatment is current and do not depend on this outcome.

## Completed milestones

Foundation, accounts, ledger transactions, reconciliation UI, QIF import,
Trading 212 ingestion (including investment lots), reports (net worth,
spending, cashflow, with filters and drill-down), and investments UI/gains are
shipped. See `docs/implemented.md` for the capability ledger and the historical
design records for rationale.
