# Documentation map

This folder is organized around one question: **"where do I look, and where
does a new document go?"** Four current-state files answer "what is happening";
everything else is reference material sorted by kind.

## Current state — the four working files

| File | Answers | Update discipline |
|---|---|---|
| [roadmap.md](roadmap.md) | What are we building next, in what order? | Only when priorities genuinely change; governed by `product-requirements.md` |
| [todo.md](todo.md) | What is the short-horizon working queue right now? | Freely; items are deleted when done or promoted to roadmap/backlog |
| [backlog.md](backlog.md) | What known defects and technical debt are tracked? | Add with an ID (`T-nn`, security `S-nn`, test-coverage gaps keep their review's `G-nn`); move resolved items to `reviews/resolved-backlog-2026-07.md` |
| [implemented.md](implemented.md) | What ships today, backend vs UI? | Reconcile with the codebase when a slice lands |

The boundary between them: **roadmap** holds ordered product initiatives,
**backlog** is the registry of known problems, **todo** is the distilled
"next actions" view that references both (it never carries detail of its
own), and **implemented** is the capability ledger. A piece of work should
have exactly one home.

## Governance (root)

- [product-requirements.md](product-requirements.md) — product intent, locked
  decisions, cross-cutting requirements. Governs the roadmap.
- [conventions.md](conventions.md) — repo-wide engineering and financial
  conventions. The `.claude/skills/` library must stay in sync with it.

## Durable references (root)

- [competitor-comparison.md](competitor-comparison.md) — maintained parity
  matrix and positioning. Dated deep dives live in `reviews/`.
- [developer-workflow.md](developer-workflow.md) — commands, environments,
  commit conventions.
- [deployment-security.md](deployment-security.md) — operator-facing
  deployment guidance.
- [early-architecture-decisions.md](early-architecture-decisions.md) — active
  architecture decisions predating the ADR series.
- [adrs/](adrs/) — accepted decision records (numbered, immutable).

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
