# Documentation map

This folder is organized around one question: **"where do I look, and where
does a new document go?"** The current-state files and GitHub Issues answer
"what is happening"; everything else is reference material sorted by kind.

## Current state

| File | Answers | Update discipline |
|---|---|---|
| [roadmap.md](roadmap.md) | What are we building next, in what order, and what is the current focus? | The **only** place order lives. Update when scope, order or focus changes; remove focus items when they ship; governed by `product-requirements.md` |
| [GitHub Issues](https://github.com/sergeyfarin/rekenraam/issues) | What does each ticket require, and is it open? | Acceptance, state, priority, discussion and PR links. Issues never restate the overall order |
| [backlog.md](backlog.md) | Which GitHub issue owns an old local code? | Historical aliases only; no new local IDs, status, priority or order |
| [implemented.md](implemented.md) | What ships today, backend vs UI? | Reconcile with the codebase when a slice lands |

The boundary between them: **roadmap** holds order and current focus,
**GitHub Issues** holds each ticket's acceptance and state, **backlog** maps
historical aliases to issues, and **implemented** is the capability ledger.
Feature plans and ADRs hold durable behavior and scope decisions; live delivery
checklists belong to issues. Each fact has exactly one home; elsewhere, link to it.
GitHub issue [#120](https://github.com/sergeyfarin/rekenraam/issues/120) was
retired on 2026-10-09. Its older progress comments remain history; use this
map and the roadmap for current navigation. Post progress on the relevant
feature issue and update the current-state documents in the same delivery
or sequencing change.

## Governance (root)

- [product-requirements.md](product-requirements.md) — product intent, locked
  decisions, cross-cutting requirements. Governs the roadmap.
- [conventions.md](conventions.md) — repo-wide engineering and financial
  conventions. The `.claude/skills/` library must stay in sync with it.

## Durable references (root)

- [competitor-comparison.md](competitor-comparison.md) — maintained parity
  comparison, rechecked against primary product sources on 2026-10-17
  (direct Firefly III retrieval unavailable; indexed official evidence retained).
  Dated deep dives live in `reviews/`.
- [developer-workflow.md](developer-workflow.md) — commands, environments,
  commit conventions.
- [deployment-security.md](deployment-security.md) — operator-facing
  deployment guidance.
- [upgrades.md](upgrades.md) — released migration policy and the operator
  upgrade/rollback checklist.
- [early-architecture-decisions.md](early-architecture-decisions.md) — active
  architecture decisions predating the ADR series.
- [localization-glossary.md](localization-glossary.md) — the terminology every
  translated catalog follows. Read it before adding a domain term or reviewing
  a translation; it is far shorter than the message catalogs it governs.
- [adrs/](adrs/) — accepted decision records (numbered, immutable).

## Initiative and design references

[ADR 0018](adrs/0018-personal-access-token-boundary.md) and the
[personal-access-token contract](plans/personal-access-tokens-plan.md) define
#334's read-only owner API grants, credential lifecycle and security events.
The [2026-10-20 delivery evidence](reviews/personal-access-tokens-delivery-2026-10-20.md)
records its service/API/browser acceptance. The [independent audit](reviews/personal-access-token-audit-2026-10-20.md)
completes #357 with queued-expiry finding #359 fixed and reproducible
HTTPS/client/native-restore/browser evidence; adjacent expiry audit #360 and
actual owner deployment #336 remain separate.

[ADR 0017](adrs/0017-provider-independent-features-and-incomplete-data.md) defines
provider-independent features, explicit incomplete attribution and manual repair.
The [2026-10-15 review](reviews/provider-independence-review-2026-10-15.md) audits
existing manual paths, provider-only gates and app-owned recovery gaps.

The [quote/crypto contract](plans/quote-providers-crypto-plan.md) records #117’s
instrument identity/precision, provider evidence and entitlements, durable quote
refresh/recovery, and the fixed crypto CSV interchange format.

The [roadmap](roadmap.md#current-focus) owns the current initiative and focus.
R16's agreed investment lifecycle gates are delivered; its
[operation plan](plans/investment-operation-refactor-plan.md) and slice
contracts remain design references for shipped behavior and related extensions.

The [native SQLite driver migration plan](plans/native-sqlite-driver-migration-plan.md)
and [ADR 0014](adrs/0014-native-sqlite-driver.md) record the accepted driver
direction, compatibility/build contracts and performance evidence for #195
and #164. #195 installs the bundled native driver; #164 records controlled
performance and development-feedback evidence.

[ADR 0015](adrs/0015-sqlite-commit-durability.md) strengthens the writer from
WAL/NORMAL to WAL/FULL after driver admission. The
[FULL durability review](reviews/sqlite-full-durability-2026-10-12.md) records
current-policy validation and the remaining connection/build settings audit.

[ADR 0016](adrs/0016-native-backend-test-resources.md) and the
[native gate review](reviews/native-test-gate-2026-10-12.md) replace the old
serial-package/20-minute test gate with two concurrent packages, five-minute
deadlines and normal available CPU resources after native scheduling evidence.

The [2026-09-27 work-tracking audit](reviews/github-work-tracking-audit-2026-09-27.md)
records the open-ID triage used for the migration to GitHub Issues.
The [pre-migration backlog snapshot](reviews/open-backlog-before-github-2026-09-27.md)
preserves their acceptance context.

Latest application-state reconciliation and audit proposal:
[2026-10-17 app state and audit brief](reviews/app-state-and-audit-plan-2026-10-17.md).
It reconciles recent closures, maps all feature/design references and all app
areas to twelve bounded audits, and distinguishes review completed from audit
execution pending. The roadmap retains order and R13 readiness;
[#260](https://github.com/sergeyfarin/rekenraam/issues/260) tracks audit execution.
The [first identity/setup/privacy audit](reviews/setup-auth-privacy-audit-2026-10-17.md)
is complete with fixes, including the #262 reauthentication budget. Subsequent
area dispositions and remaining work live in the roadmap and #260. The
[prices/FX/durable-work audit](reviews/pricing-durable-work-audit-2026-10-18.md)
records #274's real-worker recovery and browser evidence, corrected findings
#314/#316, and the publication-gap finding #315. Its
[follow-up delivery](reviews/fx-coverage-progress-fix-2026-10-18.md) corrects
starvation with durable scan progress and independent failed-date retries.

The [runtime API contract audit](reviews/runtime-api-contract-audit-2026-10-20.md)
records #349's bounded response/status matrix and schema sensitivity controls,
small fixes #367–#369 and the original retained findings #350/#370.
[#370's delivery](reviews/mutation-response-enrichment-2026-10-20.md) repairs
posting enrichment, fixes #371/#372 and expands the schema evidence to 165
operations. [#350's delivery](reviews/json-body-contract-2026-10-21.md) documents
and exercises all 215 capped JSON operations' 413 responses, fixes #374/#375
and the validation-fixture finding #376, and expands the strict schema sample
to 1,582 responses across 251 operations with nine sensitivity controls.
The audits' explicit unverified operations/statuses prevent treating the sample
as universal conformity.

Earlier strategic proposal:
[2026-09-09 product direction, AI and privacy](reviews/product-direction-ai-privacy-2026-09-09.md).
It challenges the positioning and evaluates optional AI/MCP, data ownership,
release readiness and adoption experiments. Its recommendations are proposals,
not changes to the accepted roadmap or ADRs.

Desktop distribution feasibility:
[2026-09-19 desktop application review](reviews/desktop-application-feasibility-2026-09-19.md).
It evaluates a thin Wails host, required architecture and release work, ongoing
cost, and repository strategy. It is analysis only; native desktop remains out
of scope until an accepted ADR changes the product decision.

- [R18 reporting policy](plans/investment-reporting-plan.md) — live observation selection (#203), named basis/valuation profiles, research recommendations and delivery slices (#118, #230–#235).
- [R18 completion audit](reviews/r18-completion-audit-2026-10-15.md) — delivery evidence, date-default fix and independent follow-ups #237/#238.

## Folders

- **[plans/](plans/)** — feature plans: design + acceptance criteria for one
  feature area. Some describe shipped features and are retained as their
  design record (status is stated in each header); some describe future
  slices. New feature design docs go here.
- **[design/](design/)** — durable design documents for shipped foundations
  (account hierarchy, accounts system, categories, the opened date of an
  import-created holding account) that are not tied to one roadmap slice.
- **[reviews/](reviews/)** — dated, point-in-time documents: audits, reviews,
  analyses, resolution records. Named `<topic>-<yyyy-mm[-dd]>.md`. Their
  **bodies** are never rewritten to stay current — supersede them with a newer
  dated file. The one allowed edit is a **status banner at the top** recording
  how the findings were resolved (see the 2026-07-13 and 2026-07-19 audits and
  the 2026-07-19 roadmap review), so a reader never has to re-derive whether a
  finding is still live. Resolution records such as
  `resolved-backlog-2026-07.md` are append-only by design. Anything with a date
  in its name belongs here.
- **[archive/](archive/)** — superseded documents kept for history: completed
  per-step implementation trackers (replaced by `implemented.md`) and
  reviews of the pre-Go experimental stacks. Never cite these as current.

Earlier cross-document code reconciliation:
[2026-08-31 documentation review](reviews/documentation-code-review-2026-08-31.md).
It records findings at that date; use GitHub Issues for their current status.

Provider evidence:
[2026-10-13 Trading 212 split recheck and collection guide](reviews/trading212-split-evidence-2026-10-13.md)
records the evidence still required by #145.

Recent change and issue-closure audits:
[2026-10-12 investment schema closure audit](reviews/investment-schema-closure-audit-2026-10-12.md),
[2026-10-07 review since f2bd160f](reviews/changes-since-f2bd160f-2026-10-07.md)
and [2026-10-04 review since a0487f9d](reviews/changes-since-a0487f9d-2026-10-04.md).
They distinguish reproduced defects from R16 work still tracked in open issues.

## Rules of thumb

1. Dated snapshot → `reviews/`. Feature design → `plans/`. Everything else
   probably updates an existing file instead of creating a new one.
2. When a plan's slice ships: record capabilities in `implemented.md`, keep
   the plan in `plans/` as the design record, and delete any per-step
   checkbox tracking from it (live checklists belong to GitHub issues).
3. Audit/review findings that need action get a GitHub issue, using its
   `#number`; preserve old local IDs only as historical aliases. The review
   itself is not a tracker. See [Work Tracking](conventions.md#work-tracking)
   for readiness, progress and closure.
4. Code and docs reference these files by full path (`docs/plans/...`) — when
   moving or renaming, update references repo-wide (README.md, AGENTS.md,
   `.claude/`, backend and frontend source comments).

Superseded sections extracted from active documents:
[pre-release baseline rewrite notices](archive/pre-release-baseline-rewrites.md)
and [the original investment ledger sketch](archive/investment-ledger-sketch.md).
Their historical wording is preserved; current migration guidance and investment
contracts remain in the workflow, plans and ADRs.
