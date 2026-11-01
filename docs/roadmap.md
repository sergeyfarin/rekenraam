# Roadmap

The single source of truth for **what to build next, in what order** —
initiatives and the current focus within them. Governed by
`docs/product-requirements.md`.

Other questions have exactly one home each:

- Ticket acceptance, state, priority and discussion: [GitHub Issues](https://github.com/sergeyfarin/rekenraam/issues).
- What ships today: [implemented features](implemented.md).
- All R-number references, including completed initiatives: [initiative index](implemented.md#roadmap-initiative-index).
- Feature design contracts: `docs/plans/` and `docs/design/`. They never define a sequence.
- Resolving a historical local code to its issue: `docs/backlog.md`.

Do not restate shipped detail here; when a focus item ships, delete it and
record the capability in `implemented.md`. Retain a short reference only when
it explains remaining scope, a dependency or an unmet gate.

Reviewed 2026-10-28 against `2f0a9fda`, delivery records and live GitHub issue
state. Completed maintenance, R13 and satisfied release prerequisites are in
[the capability ledger](implemented.md). This cleanup preserves the accepted
order of remaining work; it does not infer owner release acceptance.

## Direction

Build a polished, self-hosted personal-finance daily driver, with trustworthy
migration, daily planning and multi-currency support for cross-border households
and investors. Repair trust-boundary defects before adding financial producers.
The remaining execution queue is under Current focus.

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

The [UI overhaul](#current-product-initiative--ui-overhaul) is the current
product initiative (owner decision 2026-10-29), starting with #436. The items
below continue alongside it and do not block it.

1. **Resolve the remaining owner/release gates in parallel with maintenance.**
   [#260](https://github.com/sergeyfarin/rekenraam/issues/260) remains open for
   actual owner deployment/household evidence
   [#336](https://github.com/sergeyfarin/rekenraam/issues/336) and owner
   image-advisory disposition
   [#337](https://github.com/sergeyfarin/rekenraam/issues/337). Signing
   [#333](https://github.com/sergeyfarin/rekenraam/issues/333) (signing decision
   accepted; candidate validation remains) and the upstream
   driver-release prerequisite
   [#356](https://github.com/sergeyfarin/rekenraam/issues/356) retain separate
   acceptance. Release remains on hold; these gates do not block accepted
   maintenance. Adoption assets #335 stay P3 for later release preparation.
   See [Public-release gates](#public-release-gates) for the remaining evidence.
2. **Track the remaining bounded maintenance follow-ups.**

   - [#414](https://github.com/sergeyfarin/rekenraam/issues/414), P3:
     investigate the capital-return reversal browser stall on recurrence.
     [Fresh bounds and scoped diagnostics](reviews/capital-return-stall-2026-10-29.md)
     retain the unconfirmed cause; failure traces include DOM/network evidence
     for this spec and separate reversal-phase timings.
   - [#416](https://github.com/sergeyfarin/rekenraam/issues/416): confirm the
     dividend-entry contrast fix on hosted CI after publication. The
     [local fix and regression evidence](reviews/dividend-entry-contrast-2026-10-29.md)
     are delivered with all existing accessibility coverage retained.

   [#445's bounded query experiment](reviews/investment-read-preparation-2026-10-29.md)
   meets its preparation target. Both fresh proposed full gates pass below
   270 s (263.4 / 269.2 s), completing the measured action response. The slower
   run has only 0.8 s to that threshold; investigate and the unmet 30% headroom
   objective remain under ADR 0016 monitoring. No suite speedup or policy
   change is claimed.

3. **Continue independent readiness work.**
   [#102](https://github.com/sergeyfarin/rekenraam/issues/102) needs native
   terminology review. Assign a reviewer per drafted non-English language
   using the [review packet](reviews/localization-native-review-2026-10-13.md)
   and record sign-off before calling those catalogs complete.
   Recheck [#112](https://github.com/sergeyfarin/rekenraam/issues/112)'s upstream
   TypeScript/OpenAPI-generator compatibility when considering the upgrade;
   its old blocker text is not fresh compatibility evidence.

### Current product initiative — UI overhaul

Implement the owner-approved [Home and navigation design](design/home-and-navigation.md)
recorded in [commit `457dd74`](https://github.com/sergeyfarin/rekenraam/commit/457dd74a8344caa962bfdf29fe8dd18a69254102),
with its [v3 mockup](design/home-mockup/README.md). The design is approved;
its header, registry, tab-route, hide-amounts, shared-search and attention slices
#455–#459 and section-owned paths #460 are delivered, and Home #437 is delivered and closed; the remaining shell work (#436) and the pages below are not delivered. The owner made it the current
initiative on 2026-10-29, after the remaining maintenance items (#414, #416)
were judged not to block it; they, owner-dependent readiness and release
evidence continue in parallel. Implementation starts with #436.

Deliver incrementally, tracked in #441, in this order:

1. **Shell and search (#436):** replace the sidebar and duplicate top bar with
   a light header (Home, section links that open each section's Overview, a
   Review button, settings menu), a tab row of real page routes driven by one
   route registry, hide amounts, compact shared search and one attention read
   for counts. Header, registry and tab routes #455 are delivered (see the
   capability ledger). Hide amounts #456, search #457 and attention read/Review
   count #458 are delivered. Its posted-currency pricing-discovery follow-up
   #464, durable posted-only historical targets #465 and their shared demand
   read #466 and restart-safe future-demand deferral #471 are delivered;
   report-specific conversion coverage remains the
   consumer's responsibility.
   Section-owned Ledger and Planning paths #460 are delivered with legacy
   redirects; #448 still replaces the temporary Accounts list with Ledger
   Overview. Page actions, widths and `?` help #459 are delivered and closed;
   the shared Home → register → reconciliation journey is #442's acceptance.
2. **Home (#437):** two-line briefing, central command bar, shortcut tiles led
   by Add transaction, compact default My Home and real-size More to explore
   widgets, backed by a composed Home read and a shared transaction-entry
   dialog. Its status line links to the **Review page (#453)**, which lists
   everything needing attention and the status of background work. Split into
   #476 (composed read and briefing), #477 (command bar, shortcuts and entry
   dialog), #478 (My Home widgets) and #479 (More to explore), all delivered;
   #437 closed 2026-10-31. The Transfer tile and transfer entry (#481: same-
   and cross-currency, posting days and a fee on Bank fees) were delivered
   2026-11-01. Still open: the general reconciliation out-of-date rule with
   its account-list state and Home widget (#482, configurable window, default
   60 days). Pinned accounts on the Accounts widget arrive with #442
   and entered values in the briefing with #454.
3. **Pages in their new places**, in any order after #436: settings in five
   pages with Trash moved to Transactions › Deleted and export to Reports
   (#446) delivered 2026-10-31; Import views (#447); Ledger overview with
   accounts by institution, an Institutions view and brokerages as one account
   (#448); Planning and Reports overviews (#450); Payees page (#443); Tags view
   (#444).
4. **Object navigation (#442):** the switcher as scoped search, pinned
   accounts and breadcrumbs, after #448 and the owner-run real-book count
   (#449).
5. **Personalization (#438):** server-persisted layout and revision-checked
   API, headline choices, Customize panel (also in Settings › Preferences),
   item actions with Undo, sizes and settings.
6. **Search depth (#439):** payee actions with editor prefill and last-used
   category, payee filtering and full-text transaction search.
7. **Widget depth (#440):** lazy expanded returns, allocation and forecast
   charts, then further widgets for existing capabilities.
8. **Power use**, any time after #436: compact and comfortable density for
   registers and transactions (#451) and two-key navigation shortcuts (#452).

Navigation decisions (#435) are recorded in the design, and the
[navigation prototype](design/navigation-prototype/README.md) shows the
result after two owner reviews. Preserve every existing route (kept or
redirected) and financial contract, explicit scope/completeness,
localization, both themes and usable phone/keyboard flows. README
screenshots (#335) follow #436 and #437.
R7a's broader entry helpers and configurable shortcut presets remain separate.

## Deliberately later

These are valuable, but they are not allowed to displace the current plan:

- R6: XLSX and OFX/QFX adapters, import matching, per-split mapping, batch
  rollback, and richer import history. `docs/plans/import-plan.md` retains the detailed
  data, lifecycle, and acceptance design.
- Import rules **beyond the R5 v1 fence**: regex and amount predicates,
  retroactive re-run over committed transactions, and a rule audit trail.
  Also duplicate payee merge, bulk recategorization and a dedicated import
  review inbox beyond the existing `needs_review` filter/approval flow.
- R15 connections expansion, in this order (adopted 2026-08-05):
  **IBKR Flex Query → GoCardless EU/UK banks**, then SimpleFIN Bridge (US)
  later. Further event-family mappings extend the existing dividend producer;
  adapter evidence gates are listed below. CSV mapping-profile presets for
  API-less brokers (Trade Republic, DeGiro, Raisin, HL/AJ Bell/ii) are R5
  documentation-plus-fixtures tasks, not adapters. All bring-your-own-key;
  assessment matrix, free-vs-paid verdicts, and slice designs in
  `docs/plans/connections-plan.md`. Both first slices carry blocking
  provider-verification preconditions for their online adapters — see that plan.
  These do not block shared manual/file workflows or unrelated app features.
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
  `docs/plans/receipts-plan.md`. R14a remains outside announcement gates;
  its historical placement decision is recorded in the capability ledger.
- General report snapshots beyond R18’s saved operational investment
  gains/statement documents, multi-user, and household features. Core
  reporting-currency conversion remains part of the existing report surface.
- Target allocation/drift [#411](https://github.com/sergeyfarin/rekenraam/issues/411)
  and advisory rebalancing [#412](https://github.com/sergeyfarin/rekenraam/issues/412)
  are deferred P3 extensions. Each needs an accepted contract; rebalancing
  depends on the target model. No trade execution is implied.
- Large-investment-book returns measurement
  [#413](https://github.com/sergeyfarin/rekenraam/issues/413), P3: measure the
  live read (including `instruments=true`) and saved runs against the #405
  baseline. The current large-book harness seeds no investments; this is
  evidence work, not a gate for accepted R13 delivery.
- Values you enter [#454](https://github.com/sergeyfarin/rekenraam/issues/454):
  house, vehicle and pension values with a per-account update interval, so
  net worth reports them as missing or out of date like prices. Storage is
  decided ([ADR 0021](adrs/0021-entered-asset-values.md): dated values beside
  the ledger, never postings); ready in three slices. #437 and #453 do not
  wait for it.
- Original maintainability candidates 7–13 remain
  [explicitly deferred without issues](reviews/maintainability-audit-2026-10-24.md#explicitly-deferred-inventory-original-ranks-713).
  Revisit only on the recorded trigger and refreshed evidence; open a bounded
  issue then if justified.

### Adapter evidence gates

These follow-ups need verified provider semantics before automatic admission:

| Adapter-only follow-up | Required evidence |
|---|---|
| [#136](https://github.com/sergeyfarin/rekenraam/issues/136), Trading 212 execution cancellations | Execution identity, cancellation/revision relationship and financial effects |
| [#145](https://github.com/sergeyfarin/rekenraam/issues/145), Trading 212 splits | Ratio, effective date and entitlement payloads |
| [#176](https://github.com/sergeyfarin/rekenraam/issues/176), provider short/cover rows | Explicit provider side semantics and representative payloads |
| [#256](https://github.com/sergeyfarin/rekenraam/issues/256), Twelve Data splits | Unambiguous forward/reverse ratio and effective-date semantics |

These gates apply to those mappings alone. Manual operations, file workflows,
other adapters and existing reviewed suggestions retain their own acceptance.

## Competitor and parity check

Keep `docs/competitor-comparison.md` as the maintained parity matrix and
`docs/reviews/competitive-analysis-2026-07.md` as the dated deep dive. Before declaring
each roadmap initiative complete, update the comparison's implication section
and record either the parity gained or the deliberate gap retained.

The [comparison's 2026-10-26 local status update](competitor-comparison.md)
records accepted delivery boundaries. Targets/drift (#411), advisory
rebalancing (#412), R6 import depth and R7a convenience remain deliberate gaps.
The planned UI overhaul addresses Home discovery and navigation; those
presentation gaps remain until it ships, without changing financial metric or
tax parity claims.
Actual's import/rules and PocketSmith's planning workflows justify auditing
everyday migration and planning usability. Verify the core journeys before
expanding integrations; provider signup/coverage claims need fresh
adapter-specific evidence. These are product inferences from the dated review,
not market-wide uniqueness or full metric/tax parity claims.

## Public-release gates

Release readiness is a parallel track; announcement and marketplace listing
remain separate from local daily-driver delivery. Completed prerequisites are
recorded in [the capability ledger](implemented.md#completed-announcement-prerequisites).
Dated engineering reviews establish their recorded scenarios; a release
candidate needs fresh evidence for its actual baseline.

### Before the v0.1.0 tag

- Run the documented release preflight and record candidate/deployment identity.
- Complete the actual owner household dry run, including import, entry,
  reconciliation, reports, export, backup and tested restore (#336).
  Synthetic automated acceptance does not substitute for that dry run.
- Record an explicit list of investment limits and resolve the retained
  image-advisory disposition (#337); track the driver prerequisite (#356)
  separately with its admission evidence.
- Freeze installed-release migrations as append-only at the tag. The
  [baseline and reset contract](implemented.md) records the schema consolidation;
  later schema changes follow the release lifecycle in `docs/conventions.md`.

### Public repository hygiene

Maintain `LICENSE`, `SECURITY.md`, dependency scanning and private disclosure
paths. Confirm complete-history secret-scan evidence and GitHub secret-scanning
and push-protection settings for the candidate; a checkout alone cannot
establish remote settings. The
[dated deployment audit](reviews/deployment-audit-2026-10-18.md) supplies bounded
historical evidence; actual settings/custody acceptance remains #336 and
runtime-image disposition remains #337.

### Before public announcement or marketplace listings

1. Complete native terminology sign-off (#102); see Current focus and the
   [review packet](reviews/localization-native-review-2026-10-13.md).
2. Produce signed release artifacts with checksums and reproducibility notes
   [#333](https://github.com/sergeyfarin/rekenraam/issues/333). The owner accepted
   Sigstore keyless GitHub Actions OIDC/Cosign and initial Linux amd64/arm64
   targets on 2026-10-28 ([ADR 0020](adrs/0020-release-signing-and-platform-targets.md)).
   Complete the manual candidate workflow's native-platform, signature/tamper
   and preflight evidence; preserve the
   [reproducibility conditions](developer-workflow.md#reproducibility).
   Actual publication remains a separate owner-authorized release cut, with
   all migration checksums frozen and installed-release policies activated.
   Future Wails/Windows packaging can add Authenticode before final checksums;
   no desktop implementation or Windows signing-provider choice is implied.
3. Prepare adoption assets [#335](https://github.com/sergeyfarin/rekenraam/issues/335):
   seeded demo, README screenshots and a short migration walkthrough, leading
   with exact decimals, append-only versions and trial balance for the
   plain-text-accounting audience.

Deployment acceptance remains required (#336). An internet-exposed installation
requires HTTPS, retained secret-key configuration and actual owner MFA enrollment,
plus the current operator/security checks in
[deployment guidance](deployment-security.md). Trusted-device management and
authentication-event APIs lack management screens; record those limits rather
than inferring a deployment's safety from closed implementation issues.

## Open product decisions

Resolve these only when their related slice becomes current work:

- The highest-priority mobile workflow, beyond the required responsive core.
- Attachment storage, retention, access-control, backup and encryption model.
  A proposed resolution exists in [the receipts plan](plans/receipts-plan.md)
  (R14a); accepting it is required before changing the current attachment scope.
