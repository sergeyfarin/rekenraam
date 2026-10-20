# Roadmap

The single source of truth for **what to build next, in what order** —
initiatives and the current focus within them. Governed by
`docs/product-requirements.md`.

Other questions have exactly one home each:

- Ticket acceptance, state, priority and discussion: [GitHub Issues](https://github.com/sergeyfarin/rekenraam/issues).
- What ships today: `docs/implemented.md`.
- Feature design contracts: `docs/plans/`. They never define a sequence.
- Resolving a historical local code to its issue: `docs/backlog.md`.

Do not restate shipped detail here; when a focus item ships, delete it and
record the capability in `implemented.md`.

Last reviewed: 2026-10-17 against local `f35cc723` and live GitHub issue
searches. The documentation and competitor review is recorded in the
[app-state review and audit brief](reviews/app-state-and-audit-plan-2026-10-17.md).
R16, R11, R17 and R18 have met their bounded delivery bars. **R13 remains the
next feature initiative; its calculation contract is not yet implementation-ready.**
The cross-app audit below is planned, not completed. Earlier milestone detail
lives in the [completed roadmap record](reviews/completed-roadmap-2026-09-27.md)
and the [capability ledger](implemented.md).

## Slice index

Every R-number ever used, so references in other documents stay resolvable.
Statuses: ✅ shipped · ◐ partly shipped ahead of its slice · ▶ current ·
⏭ planned (ordered below) · ⏸ deliberately later · — retired/unassigned.

| Slice | Name | Status | Primary document |
|---|---|---|---|
| R1 | Reconcile workflow screen (trust loop) | ✅ | `docs/implemented.md` (Reconciliation) |
| R2 | Reports users can act on | ✅ | `docs/plans/reports-plan.md` |
| R3 | Portable **and protected** core data (CSV/QIF export, backups, restore, self-check) | ✅ | `docs/plans/data-portability-plan.md` |
| R3a | Accessibility regression coverage | ✅ | `docs/implemented.md` (Accessibility) |
| R4 | QIF import | ✅ | `docs/implemented.md` (Import Pipeline) |
| R5 | Ordinary-bank CSV import + profiles | ✅ | `docs/plans/import-plan.md` |
| R6 | Import depth (XLSX/OFX, matching, rollback) | ⏸ | `docs/plans/import-plan.md` |
| R7 | Trading 212 online connections + lots | ✅ | `docs/plans/trading212-import-plan.md` |
| R7a | Daily-entry convenience | ⏸ | this file |
| R8 | Budgets | ✅ | `docs/plans/budgets-plan.md` |
| R9 | Recurring transactions | ✅ | `docs/plans/recurring-transactions-plan.md` |
| R10 | Projected balances / forecasting | ✅ | `docs/plans/projected-balances-plan.md` |
| R11 | Pricing/FX management UI | ✅ | `docs/plans/pricing-management-plan.md` |
| R12 | Investments UI + gains reporting | ✅ | `docs/plans/investments-plan.md` |
| R12a | Investment journal/subledger integrity correction | ✅ | `docs/plans/investment-integrity-plan.md`, ADR 0012 |
| R13 | Investment return analytics (TWR/MWR) | ⏭ | this file |
| R14 | Receipts & attachments (capture, OCR, inbox) | ⏸ | `docs/plans/receipts-plan.md` |
| R14a | Attachment storage + manual attach (scope decision still required) | ⏸ | `docs/plans/receipts-plan.md` |
| R15 | Connections expansion (IBKR Flex → GoCardless; the #111 dividend producer shipped early, 2026-10-17) | ⏸ | `docs/plans/connections-plan.md` |
| R16 | Investment lifecycle completeness (correction, transfers, basis actions, splits, short sales) | ✅ agreed gates; related work open | `docs/plans/investment-operation-refactor-plan.md`; ADR 0013 |
| R17 | Crypto instrument type + `PriceProvider` registry and quote adapters | ✅ | `docs/plans/quote-providers-crypto-plan.md` |
| R18 | Reproducible investment basis + gains projections | ✅ | ADR 0012; `docs/plans/investment-reporting-plan.md` |

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
| Connections sequencing | Order adopted, **contents amended**: the quote-provider slice moves into R17 (it builds the `PriceProvider` registry once). R15 is now IBKR Flex → GoCardless → #111 producer |
| Receipts R14a pull-forward | **No** — but R3 designs the backup/self-check with a documented attachments hook, and R14a ships after R5 |
| GoCardless / IBKR "verify" items | Not a decision — reclassified as blocking slice-start preconditions on GC-1 and IBKR-1 in `connections-plan.md` |

These are historical sequencing decisions, not the current delivery queue.
The R14/R15 plans (2026-07-19) remain deliberately later. Their sequencing
was decided on 2026-08-05: quotes belong to R17; R15 is IBKR → GoCardless →
#111 producer. R14a stays after R5 and was not pulled forward alongside R3. The Yahoo Finance
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

## Provider independence decision (2026-10-15)

[ADR 0017](adrs/0017-provider-independent-features-and-incomplete-data.md)
separates app delivery from online automation. Build manual/file-based workflows
and explicit partial attribution without waiting for Trading 212, another API,
or live provider samples. Missing required financial effects still need manual
input or a supported unknown-state workflow; optional enrichment may remain
unresolved with visible coverage. Provider evidence gates apply only to their
adapter mappings. Later R15 placement is sequencing, not a hard dependency for
core features or the existing event/suggestion infrastructure.

## Current plan

Do not start a new roadmap initiative until the current one has met its
acceptance criteria. Priority labels on issues express urgency; the order
below is execution order, not a chain of hard dependencies.

### Current focus

1. **Assess the combined app after the recent deliveries.** Execute the twelve
   bounded audits in the [audit brief](reviews/app-state-and-audit-plan-2026-10-17.md#proposed-audit-coverage).
   Start with ledger/reconciliation, investment replay/reporting and recovery;
   then planning/import integration, frontend journeys and deployment/tooling.
   Record fresh evidence per area and file scoped findings. Audit execution is
   tracked in [#260](https://github.com/sergeyfarin/rekenraam/issues/260);
   the first identity/setup/privacy audit is complete with findings in the
   [2026-10-17 evidence](reviews/setup-auth-privacy-audit-2026-10-17.md).
   #261/#262/#263/#264 are corrected. The identity-concurrency follow-up #267
   is complete ([evidence](reviews/identity-concurrency-audit-2026-10-17.md));
   its findings #285/#286/#287 are fixed and attribution gap #288 is closed.
   The proxy/shared-browser follow-up #268 is complete
   ([evidence](reviews/proxy-browser-privacy-audit-2026-10-17.md)); its
   key-loss sign-in finding #290 (P3) is fixed.
   The reference-data audit #269 is complete
   ([evidence](reviews/reference-data-audit-2026-10-17.md)); its findings
   #291/#292 (P2) and #293/#294/#295 (P3) are fixed.
   The ledger/reconciliation audit #270 is complete
   ([evidence](reviews/ledger-reconciliation-audit-2026-10-17.md)); its
   finding #296 (P2), its follow-ups #304 (P2) and #305 (P3), and #297/#298
   (P3) are fixed.
   The import audit #271 is complete
   ([evidence](reviews/import-audit-2026-10-17.md)); all its findings,
   #299 (P2) and #300–#303 (P3), are fixed.
   Investment part 1 #272 has been executed
   ([evidence](reviews/investment-operations-1-audit-2026-10-18.md)); the
   operation matrix and exact-figure checks pass with no new financial defect.
   #308's diagnostic audit is complete
   ([evidence](reviews/unresolved-basis-diagnostics-audit-2026-10-18.md)) and
   #313 shipped its incomplete self-check outcome, so supported unknown basis
   is now disclosed as incomplete coverage rather than a failure. Its
   bundle acceptance is met by #277's independent reconstruction, and #272 is
   closed: the literal import round trip is retired (no importer exists),
   replaced by figure reconstruction plus the exact native-backup round trip.
   Investment part 2 #273 is complete
   ([evidence](reviews/investment-operations-2-audit-2026-10-18.md)); its
   independent transfer/corporate-action/correction register and replay pass,
   and every sequence ends in a whole-run self-check assertion.
   #307/#309/#310/#311/#312 are fixed. Acceptance disposition for #272/#273's
   "self-check clean" item under #313's contract: clean means **no failed
   check**. A book whose only gap is supported unknown basis reports
   `incomplete` with that scope named — not an all-passed run, which such a
   book cannot honestly produce — and resolved controls pass outright. #308
   verified exact knowledge transitions, all six deliberate-damage controls
   and 24 native restore comparisons; #313 re-ran them under the new contract.
   The prices/FX/durable-work audit #274 is complete
   ([evidence](reviews/pricing-durable-work-audit-2026-10-18.md)); #314's FX
   continuation loss and #316's FX transport-error key disclosure are fixed.
   #315's publication-gap starvation is also fixed
   ([delivery evidence](reviews/fx-coverage-progress-fix-2026-10-18.md)): durable
   scan cursors and independent bounded date retries preserve progress and fresh
   demand. Missing/stale coverage disclosure remains; provider availability is
   not a completeness guarantee.
   The reports/R18 audit #275 is complete
   ([evidence](reviews/reports-r18-audit-2026-10-18.md)); exact reference figures,
   shipped exports, saved schemas and R13 input readiness are verified. #317 is
   fixed; #318 resolves the original export-acceptance mismatch with separate
   checks for shipped downloads and live-only API/browser figures. The
   original unmet criterion is retained in the audit record; comparison/FX
   downloads remain outside the saved-run contract. R13 can use the inventory.
   The planning audit #276 is complete
   ([evidence](reviews/planning-audit-2026-10-18.md)); #319/#320 are fixed.
   The recovery audit #277 is complete
   ([evidence](reviews/recovery-audit-2026-10-18.md)): the #266 restore drill
   matches the register exactly on a fresh instance, every corruption class is
   detected, and the bundle reconstructs every ledger and investment figure.
   That completes #272's bundle evidence. #321/#323 are fixed, and #322 is
   fixed in bundle schema 15: every bundle id now resolves inside the archive.
   The frontend audit #278 is complete
   ([evidence](reviews/frontend-audit-2026-10-18.md)):
   #324/#325/#328/#329/#330 are fixed, and #327 is fixed: every modal is the
   shared native `ModalDialog`. #326 (URL state, P3) remains open. Its
   additional sub-view audit #331 is complete
   ([evidence](reviews/subview-audit-2026-10-19.md)): 65 dialogs, record
   forms, panels, import review states and editors pass axe, fit and close in
   both themes and in de/ru at 1280 and 390 px (five more are unreachable on
   the book, with reasons). #340 (Categories scrolled
   sideways at desktop) and #341 (hovered warning buttons under AA) are fixed.
   #342 is fixed: below `lg` the transaction and lot panels hold focus like a
   dialog. #339 is fixed: account, version and reconciliation dates are
   bounded and defaulted by the owner's day. #343 is fixed: blank transaction
   dates, ledger read dates and undated FX source-assignment lookups also
   use the owner's day.
   #344 repairs the shared-book transaction lookup and import/lot overflow;
   explicit long-option and 24-place quantity fixtures pass before/after checks
   and the complete browser gate ([evidence](reviews/subview-shared-book-fixes-2026-10-19.md)).
   Its published main CI / E2E Smoke acceptance is recorded on the issue.
   The deployment audit #279 is complete
   ([evidence](reviews/deployment-audit-2026-10-18.md)): fresh native/Compose
   builds, candidate migration 14→18, verified-backup rollback and the focused
   release preflight pass. #332's private build-context exposure and #338's
   stale upgrade-evidence references are fixed. Available Debian updates are
   applied; residual image-advisory review is additional audit #337. Owner
   deployment/household evidence is additional audit #336, blocked on actual
   owner evidence. Release gaps now have issues: signing #333 (owner decision),
   PATs #334 and adoption assets #335. These remain release gates, not a claim
   that the synthetic deployment is ready to announce.
   Architecture/quality #280 has been executed
   ([evidence](reviews/architecture-quality-audit-2026-10-19.md)); #344 resolves
   the initial browser failures. All four shared gates pass at `af233abb`:
   backend race, frontend, structural audit and all 352 browser journeys.
   #260 can cite that final-gate SHA with the report's explicit retained limits.
   #345–#347
   (P3 contract/tooling gaps) and #351 (P3 direct-step duplicate detection)
   are fixed. #352 (P2 full-suite FX test isolation) is fixed in its own
   worker project, and its full-suite acceptance now passes. #348 (P2) is fixed: the
   nested-dialog contrast fallback now measures rendered text against its own
   background, and a black border or icon can no longer hide low-contrast text. Additional audit #349 covers runtime API
   response-schema conformance beyond verified route/client parity; #350 (P3)
   records missing HTTP 413 responses on capped JSON operations. The shared
   reference register #266 is delivered, with
   [evidence](reviews/reference-book-audit-2026-10-17.md); #281 is fixed.
   Fix confirmed integrity/security defects before affected feature slices.
   This is a bounded review, not a recurring full-suite gate. Each open
   sub-audit also records maintainability notes, without fixing them.
2. **Rank evidence-gated refactoring candidates in
   [#284](https://github.com/sergeyfarin/rekenraam/issues/284).** After the
   #260 sub-audits report, measure duplication, change amplification and
   invariants enforced in more than one place. Publish a ranked list with
   the rejected candidates; each accepted refactor gets its own issue and
   preserves behavior. Refactoring only to change style or file length is
   out of scope. R13 specification can proceed in parallel, but R13
   implementation waits for #284's investment/reporting dispositions.
3. **Specify R13 [#119](https://github.com/sergeyfarin/rekenraam/issues/119).**
   Use the delivered R18 valuation contracts; resolve portfolio scope,
   external-flow classification, return equations, numerical failure states,
   coverage and reproducibility before implementation. The reporting/investment
   audit informs this contract; unrelated provider evidence is not a prerequisite.
4. **Continue independent readiness work.**
   [#102](https://github.com/sergeyfarin/rekenraam/issues/102) needs native
   terminology review, not another catalog-parity implementation. Recheck
   [#112](https://github.com/sergeyfarin/rekenraam/issues/112)'s upstream
   TypeScript/OpenAPI-generator compatibility when considering the upgrade;
   its old blocker text is not fresh compatibility evidence.

### Completed initiatives through R10

R2/R3/R3a, R4/R5 and the R9 → R10 → R8 planning loop are accepted.
[Implemented features](implemented.md) records capabilities and retained limits;
the [completed record](reviews/completed-roadmap-2026-09-27.md) preserves earlier
scope decisions. Audit their present interactions rather than restarting them.

### R16 — investment lifecycle completeness

The agreed gates are delivered. The [operation contract](plans/investment-operation-refactor-plan.md),
[slice 5 contract](plans/investment-operation-slice-5-contract.md) and ADR 0013
record named operations, corrections, dated replay and explicit refusals.
Provider automation remains separate from native/manual acceptance (ADR 0017).

| Adapter-only follow-up | Evidence needed before automatic admission |
|---|---|
| [#136](https://github.com/sergeyfarin/rekenraam/issues/136), Trading 212 execution cancellations | Verified execution identity, cancellation/revision relationship and financial effects |
| [#145](https://github.com/sergeyfarin/rekenraam/issues/145), Trading 212 splits | Verified ratio, effective date and entitlement payloads |
| [#176](https://github.com/sergeyfarin/rekenraam/issues/176), provider short/cover rows | Explicit provider side semantics and representative payloads |
| [#256](https://github.com/sergeyfarin/rekenraam/issues/256), Twelve Data splits | Unambiguous forward/reverse ratio and effective-date semantics |

None blocks manual operations, file workflows, R13, another adapter or the
existing reviewed dividend producer. Other event-family mappings are deferred
extensions, not implied by the completion of #111.

### R11 — price and FX management UI, delivered

The [price-management contract](plans/pricing-management-plan.md) records the
bounded operator surface. Consumer-specific observation selection and age rules
live in the [R18 policy](plans/investment-reporting-plan.md).

### Parallel trust work for cross-border entry

Locale-aware amounts and owner-local financial dates ship. Native terminology
sign-off remains [#102](https://github.com/sergeyfarin/rekenraam/issues/102);
use the [review packet](reviews/localization-native-review-2026-10-13.md) before
claiming the five drafted non-English catalogs are complete.

### R17 — delivered reference

The [quote/crypto contract](plans/quote-providers-crypto-plan.md) records priced
manual/CSV crypto holdings, quote adapters and entitlements. Exchange APIs,
DeFi, staking and NFT scope remains rejected.

### R18 — reproducible investment basis and gains projections

All six bounded slices and the live-consistency/capital-return disclosure
follow-ups are delivered. The [reporting policy](plans/investment-reporting-plan.md)
records supported projections, frozen operational documents and exclusions.
TWR/IRR, saved alternative/FX runs, broker comparisons, tax policy and automatic
accounting postings are not implied by R18 acceptance.

### R13 — investment returns and allocation analytics

Follow R18's reproducible valuation and basis projections. Plan explicit
TTWROR/TWR and money-weighted/IRR equations, external-flow classification,
portfolio versus security scope, missing-price behavior, FX policy, benchmark
selection, and period reproducibility before implementing charts. Use
[Portfolio Performance's documented methods](https://help.portfolio-performance.info/en/concepts/performance/time-weighted/)
as a comparison, not an implicit formula choice. Keep the figures read-only;
reports do not create postings. Core implementation uses existing manual/file
ledger and price/FX inputs with explicit incomplete coverage; Trading 212 history,
a broker-comparison feed and a live benchmark are optional integration work,
not start or completion gates. Manual benchmark observations are a valid input
once their series contract is specified. Missing calculation inputs suppress
only the affected figures under the chosen policy.

## Deliberately later

These are valuable, but they are not allowed to displace the current plan:

- R6: XLSX and OFX/QFX adapters, import matching, per-split mapping, batch
  rollback, and richer import history. `docs/plans/import-plan.md` retains the detailed
  data, lifecycle, and acceptance design.
- Import rules **beyond the R5 v1 fence**: regex and amount predicates,
  retroactive re-run over committed transactions, and a rule audit trail.
  Also duplicate payee merge, bulk recategorization and a dedicated import
  review inbox beyond the delivered `needs_review` filter/approval flow. (The minimal contains-match rules are in R5 as of
  2026-08-05.)
- R15 connections expansion, in this order (adopted 2026-08-05):
  **IBKR Flex Query → GoCardless EU/UK banks**, then SimpleFIN Bridge (US)
  later. The #111 dividend event producer (Twelve Data, dividends only) was
  delivered ahead of this order on 2026-10-17 at the owner's request. The
  evidence-blocked Trading 212 mappings stay separate (split fills #145,
  execution cancellation #136, short/cover rows #176). The payment-in-lieu
  operation short dividends need (#255) was delivered on 2026-10-17, and the
  #111 producer now proposes it for a short-only entitlement.
  Security quotes moved out of R15 into R17. CSV mapping-profile presets for
  API-less brokers (Trade Republic, DeGiro, Raisin, HL/AJ Bell/ii) are R5
  documentation-plus-fixtures tasks, not adapters. All bring-your-own-key;
  assessment matrix, free-vs-paid verdicts, and slice designs in
  `docs/plans/connections-plan.md`. Both first slices carry blocking
  provider-verification preconditions for their online adapters — see that plan.
  These do not block shared manual/file workflows or unrelated app features;
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
- R14 receipts & attachments: proposed durable attachment storage (requires
  accepting the open attachment product decision), receipt capture with in-browser OCR, and a
  match-or-draft inbox using the reserved draft-producer workflow —
  `docs/plans/receipts-plan.md`. **R14a ships after R5**, not alongside R3
  (decided 2026-08-05): it is not an announcement gate, so it does not go
  between two slices that are. R3 carries the attachments hook instead.
- General report snapshots beyond R18’s saved operational investment
  gains/statement documents, multi-user, and household features. Core
  reporting-currency conversion is already shipped.

## Competitor and parity check

Keep `docs/competitor-comparison.md` as the maintained parity matrix and
`docs/reviews/competitive-analysis-2026-07.md` as the dated deep dive. Before declaring
each roadmap initiative complete, update the comparison's implication section
and record either the parity gained or the deliberate gap retained.

The [2026-10-17 source recheck](competitor-comparison.md) supports keeping
R13 returns/allocation as the next feature gap against Portfolio Performance
and Ghostfolio. R16 lifecycle, R11 pricing, R17 crypto and R18 gains inspection
are delivered gains; they are not claims of full product or tax parity.
Actual's import/rules and PocketSmith's planning workflows also justify auditing
everyday migration and planning usability now. R6 depth and R7a convenience
remain deliberate gaps, not automatically promoted features. Verify the core
journeys before expanding integrations; provider signup/coverage claims need
fresh adapter-specific evidence. These are product inferences from documented
workflows, not market-wide uniqueness claims.

## Public-release gates

Historical financial reviews and their fixed reproductions remain in
[the September release triage](reviews/v0.1-release-triage-2026-09-11.md),
[the fourth pass](reviews/ledger-investments-fourth-pass-2026-09-19.md) and
[subsequent verification](reviews/allocation-and-gains-verification-2026-09-20.md).
They do not certify the October operation, driver and reporting changes.
The [new audit brief](reviews/app-state-and-audit-plan-2026-10-17.md) covers the
present combined baseline, failure/recovery paths and release evidence.
A candidate still needs the documented release preflight, an owner household
dry run, tested restore/export and an explicit list of investment limits.
Synthetic automated acceptance does not substitute for that dry run.

Release readiness is a parallel track; announcement and marketplace listing
remain separate from local daily-driver delivery.

### Public repository hygiene

The repository is already public (verified 2026-09-27). `LICENSE`,
`SECURITY.md`, Dependabot and the `govulncheck` workflow exist. Verify the
complete Git history for secrets and confirm the repository's GitHub secret
scanning and push-protection settings; the current checkout alone cannot
establish those remote settings. Maintain the disclosure and dependency
scanning paths before an announcement.

The [2026-10-18 deployment audit](reviews/deployment-audit-2026-10-18.md)
records a complete local-history scan with five inspected synthetic-test
detections, no reachable Go vulnerability and no JavaScript advisory. Remote
settings and the real operator/household acceptance remain #336; residual
runtime-image advisory triage remains #337. These are dated, bounded checks.

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
3. **Cross-border entry trust — open.** #100’s locale-aware amount input and
   #101’s owner-local default dates are delivered.
   #102 catalog parity and its regression gate are delivered. Assign native
   reviewers and record terminology sign-off before describing those catalogs
   as complete; see the [review packet](reviews/localization-native-review-2026-10-13.md).
4. **Personal-access tokens — open, #334** (added 2026-08-05, review §3f). The
   typed OpenAPI surface is the foundation for an ecosystem, but session
   cookie + CSRF header auth supplies no dedicated scoped credential for
   scripts, tools or community clients. Announcement is the moment of maximum developer attention.
   Required shape: hashed at rest, scoped, expiring by default, revocable,
   and emitting authentication events.
5. ◐ Produce signed release binaries with reproducibility notes (#333).
   **Reproducibility rechecked 2026-10-12** (#195): native SQLite uses
   `-trimpath`, CGO and a pinned bundled engine. Identical Go/C toolchain,
   source and embedded frontend inputs produce matching hashes. The Linux
   artifact links libc/libm; fully static linking is separate work. See
   `docs/developer-workflow.md` § Reproducibility. **Signing is still open** and needs an owner decision:
   a signing identity (Sigstore keyless via GitHub OIDC, or a held key) and a
   release workflow, neither of which exists yet — there is no release job in
   `.github/workflows/`. The existing `ci.yml`, `govulncheck.yml` and scheduled
   `large-book.yml` and `release-preflight.yml` are validation workflows.
6. Prepare adoption assets (#335): seeded demo, README screenshots, and a short
   migration walkthrough — courting the plain-text-accounting audience
   explicitly by leading with the correctness architecture (append-only
   versions, exact decimals, trial balance).

**Authentication mechanisms ship; deployment acceptance is still required.**
TOTP/recovery codes, lockout-safe throttling and authentication-event recording
are delivered; trusted-device management and authentication-event APIs lack
management screens. Local deployment does not require reimplementing these
mechanisms. An internet-exposed installation requires HTTPS, retained secret-key
configuration and actual owner MFA enrollment, plus current operator/security
checks in [deployment guidance](deployment-security.md). An older closed
security issue alone does not prove a deployment safe.

## Open product decisions

Resolve these only when their related slice becomes current work:

- (Decided 2026-10-18: no settings/metadata interchange or bundle importer. The
  native backup is the only full-installation round trip, and the bundle stays a
  portable ledger export. Recorded in `product-requirements.md` and decision 14
  of the [data portability plan](plans/data-portability-plan.md).)
- The highest-priority mobile workflow. (First non-English UI languages were
  decided 2026-08-19: Spanish, French, Dutch, German, Russian — recorded in
  `product-requirements.md`. Native review of the drafted translations still
  needs an owner to pick a reviewer per language.)
- Attachment storage, retention, access-control, backup, and encryption model
  — a proposed resolution now exists in `docs/plans/receipts-plan.md` (R14a);
  decide when that slice is scheduled.

## Completed milestones

Foundation, accounts, ledger transactions, reconciliation UI, QIF import,
Trading 212 ingestion (including investment lots), reports (net worth,
spending, cashflow, with filters and drill-down), and investments UI/gains are
shipped. See `docs/implemented.md` for the capability ledger and the historical
design records for rationale.
