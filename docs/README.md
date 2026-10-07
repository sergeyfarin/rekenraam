# Documentation map

This folder is organized around one question: **"where do I look, and where
does a new document go?"** The current-state files and GitHub Issues answer
"what is happening"; everything else is reference material sorted by kind.

## Current state

| File | Answers | Update discipline |
|---|---|---|
| [roadmap.md](roadmap.md) | What are we building next, in what order, and what is the current focus? | The **only** place order lives. Remove focus items when they ship; governed by `product-requirements.md` |
| [GitHub Issues](https://github.com/sergeyfarin/rekenraam/issues) | What does each ticket require, and is it open? | Acceptance, state, priority, discussion and PR links. Issues never restate the overall order |
| [backlog.md](backlog.md) | Which GitHub issue owns a local ID? | ID-to-issue table only; no status, priority or order |
| [implemented.md](implemented.md) | What ships today, backend vs UI? | Reconcile with the codebase when a slice lands |

The boundary between them: **roadmap** holds order and current focus,
**GitHub Issues** holds each ticket's acceptance and state, **backlog** maps
local IDs to issues, and **implemented** is the capability ledger. Feature
plans hold design only. Each fact has exactly one home; elsewhere, link to it.
GitHub issue [#120](https://github.com/sergeyfarin/rekenraam/issues/120) only
points to the roadmap.

## Governance (root)

- [product-requirements.md](product-requirements.md) — product intent, locked
  decisions, cross-cutting requirements. Governs the roadmap.
- [conventions.md](conventions.md) — repo-wide engineering and financial
  conventions. The `.claude/skills/` library must stay in sync with it.

## Durable references (root)

- [competitor-comparison.md](competitor-comparison.md) — maintained parity
  comparison, rechecked against primary product sources on 2026-09-27. Dated
  deep dives live in `reviews/`.
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

## Current execution plan

R16 investment lifecycle completeness is the current initiative. Its
[operation plan](plans/investment-operation-refactor-plan.md) and slice
contracts govern design; the [roadmap](roadmap.md) holds its current focus.

The [2026-09-27 work-tracking audit](reviews/github-work-tracking-audit-2026-09-27.md)
records the open-ID triage used for the migration to GitHub Issues.
The [pre-migration backlog snapshot](reviews/open-backlog-before-github-2026-09-27.md)
preserves their acceptance context.

Latest strategic review:
[2026-09-09 product direction, AI and privacy](reviews/product-direction-ai-privacy-2026-09-09.md).
It challenges the positioning and evaluates optional AI/MCP, data ownership,
release readiness and adoption experiments. Its recommendations are proposals,
not changes to the accepted roadmap or ADRs.

Desktop distribution feasibility:
[2026-09-19 desktop application review](reviews/desktop-application-feasibility-2026-09-19.md).
It evaluates a thin Wails host, required architecture and release work, ongoing
cost, and repository strategy. It is analysis only; native desktop remains out
of scope until an accepted ADR changes the product decision.

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
It records findings at that date; T-79 has since closed and T-80 remains open.

Recent change and issue-closure audits:
[2026-10-07 review since f2bd160f](reviews/changes-since-f2bd160f-2026-10-07.md)
and [2026-10-04 review since a0487f9d](reviews/changes-since-a0487f9d-2026-10-04.md).
They distinguish reproduced defects from R16 work still tracked in open issues.

## Rules of thumb

1. Dated snapshot → `reviews/`. Feature design → `plans/`. Everything else
   probably updates an existing file instead of creating a new one.
2. When a plan's slice ships: record capabilities in `implemented.md`, keep
   the plan in `plans/` as the design record, and delete any per-step
   checkbox tracking from it (that job belongs to `implemented.md`).
3. Audit/review findings that need action get a backlog ID; the review file
   itself is not a tracker.
4. Code and docs reference these files by full path (`docs/plans/...`) — when
   moving or renaming, update references repo-wide (README.md, AGENTS.md,
   `.claude/`, backend and frontend source comments).
