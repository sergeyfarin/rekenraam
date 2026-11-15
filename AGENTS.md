# AGENTS.md

This file gives repo-wide guidance to coding agents and automation working in this repository. It is the one instruction file for every harness used here (Claude Code, Codex, OpenCode); do not add harness-specific copies of it. `CLAUDE.md` only imports this file, so Claude Code loads it in every setup.

## Project Shape

- Active stack: Go backend, SvelteKit frontend, SQLite database.
- Production shape: one Go binary serving the statically built frontend.
- Docker Compose must preserve the same app shape.
- `.archive/` contains previous Rust, Tauri, and Python experiments and is not source of truth.

## Source Of Truth Documents

Read these before making durable product or architecture changes:

- `docs/product-requirements.md`
- `docs/conventions.md`
- `docs/early-architecture-decisions.md`
- `docs/adrs/`
- `docs/developer-workflow.md`

In case of conflict, earlier items in the list take precedence over later items. ADRs supersede all other documents once accepted. Agent guidance (`AGENTS.md` and the skills under `.claude/skills/`) is not a product source of truth; the documents above govern.

For current execution state, read these (governed by the sources above):

- `docs/roadmap.md` — the only source of work order and current focus.
- `docs/implemented.md` — what already ships (the feature ledger).
- `docs/backlog.md` — historical local-ID aliases; issue status and
  priority live on GitHub.

If a change introduces a durable new rule, update the relevant document in the same change.

Use GitHub `#numbers` for active work; do not allocate new local issue IDs.
Follow `docs/conventions.md` → Work Tracking for issue readiness and closure.

## Task Modes

Most requests are one of two modes. Decide which one you are in before you start, and do not mix them.

**Work an issue** (implement an issue, bug fix or feature) — `work-issue` skill:

- Branch from the latest canonical `main` (`upstream/main` in a fork, otherwise `origin/main`), build the slice, validate it.
- Finish with a pushed branch and an open, ready-for-review pull request. Do this without asking; it overrides any machine-wide default to wait for a request. Never merge.
- While in the code, look for bugs, weak tests, code smells and improvement opportunities beyond the task. A finding that is small, local and needs no migration, API contract change or owner decision is fixed in the same pull request as its own commit. Anything else, and anything that changes a financial result, becomes a GitHub issue. Nothing is left only in the conversation.

**Review a pull request** (review, re-review, assess) — `review-pr` skill:

- Review everything: direction, owner decisions, approach, implementation, bugs, tests, docs. An owner decision that looks wrong is a finding like any other.
- Review only. No edits, commits, pushes, pull request comments or new issues unless the owner asks for that specific thing. Describe a fix; do not make it.
- Report a verdict, then verified findings ranked `P0`–`P3` with `file:line`.

Four recurring tasks have their own skill. The last three are run as *work an issue*: they end in a pull request, fix what is small and file issues for the rest.

- **Shape an issue** — `shape-issue`: turn an idea, bug report or finding into an implementation-ready issue. No code, no branch.
- **Update dependencies** — `update-dependencies`: Dependabot, security alerts, upgrades, and what the new versions make possible.
- **Sweep for drift** — `drift-sweep`: blocked issues whose blocker is gone, roadmap and docs that no longer match the code, tests that no longer earn their place.
- **Sweep one bug class** — `sweep-bug-class`: find every remaining instance of a known pattern and add a guard.

A request that fits none of these (a question, an investigation) follows the owner's wording; it still ends in a pull request when it changes the repository. `docs/agent-task-playbook.md` is the owner's guide to all of them.

Two rules bound every task:

- **The requested outcome sets the side effects.** A request to check, find, list or assess returns findings in the session. A request to draft or propose returns text. Only a request to fix, build, update, sweep or file — or invoking a task skill by name — runs that skill's write workflow (branch, commits, pull request, issues, labels, comments). When the wording is unclear, report first and ask.
- **A suspected security vulnerability is never made public before it is fixed.** Do not describe it in a public issue, pull request, commit message or comment. Report it to the owner in the session and follow `SECURITY.md` (GitHub private vulnerability reporting). An advisory GitHub has already published may be tracked publicly.

## Task Skills

`.claude/skills/` holds twelve skills distilled from this repo's conventions and bug history: the six task skills above and six area skills. Claude Code and OpenCode discover them there; Codex discovers the same files through `.agents/skills`, a symlink to that directory. This file is only the floor: load the matching skills before starting work.

| Work | Skill |
|---|---|
| Implementing an issue, bug fix or feature slice | `work-issue` |
| Reviewing a pull request, branch or diff | `review-pr` |
| Turning an idea or finding into a ready issue | `shape-issue` |
| Dependencies, Dependabot, security alerts | `update-dependencies` |
| Stale docs, roadmap, tests, blocked issues | `drift-sweep` |
| Finding every instance of one bug pattern | `sweep-bug-class` |
| Money, transactions, postings, reconciliation, investments, prices, FX | `ledger-invariants`, before writing code |
| Go code, migrations, backend tests | `backend-slice` |
| Any `/api/v1` endpoint, schema or error code | `api-contract` |
| Anything under `frontend/src` | `frontend-screen` |
| Workers, schedulers, provider fetching, FX refresh | `background-work` |
| Before every commit, reviewing a diff, running the app, editing CI workflows, choosing which doc to update | `validate-and-ship` |

- A typical task uses one task skill plus two or three area skills, and anything financial includes `ledger-invariants`.
- If your harness did not list these skills, read `.claude/skills/<name>/SKILL.md` directly.
- A `SKILL.md` names the reference files beside it (`ledger-invariants/investments.md`, `validate-and-ship/bug-classes/<area>.md`). Read the ones for the area you are changing; they are not loaded automatically.
- The `docs/` sources above govern on any conflict. When a convention changes, update the skill that restates it in the same change. `.claude/skills/README.md` covers maintaining the library.

## Working Rules

- Deliver incrementally, one feature slice at a time.
- Do not attempt broad archive-to-current-stack conversion in one change.
- Prefer minimal, durable implementations over speculative abstractions.
- Keep the app runnable after each slice.
- Use `pnpm` for workspace JavaScript tasks.
- Respect accepted ADRs unless the change explicitly updates or supersedes them.

## Financial Invariants

- Preserve double-entry-capable ledger design.
- Never store monetary values as floating point.
- Keep `book_id` in core financial tables even while runtime remains single-book.
- Do not hard-delete posted financial records.
- Prefer void, archive, or corrective-entry workflows for durable financial changes.
- Reconciliation-related edits must preserve auditability and trust.

## Backend Rules

- Real API endpoints belong under `/api/v1`.
- Business rules belong in application services, not directly in HTTP handlers.
- Avoid frontend fan-out requests for page data. When a page or shareable component would fetch once per row/card/item, add a backend-composed JSON read model so the frontend uses one request per page, or at most one request per shareable component.
- Schema changes require migrations under `backend/migrations`.
- SQLite remains the primary database unless an active architecture document is updated.

## Frontend Rules

- Route all user-facing copy through a translation boundary.
- Keep built-in database labels localization-ready with stable keys/codes, not English-only source text.
- Keep formatting of money, dates, and numbers locale-aware and separate from translation strings.
- Use semantic design tokens for theming; avoid one-off hard-coded visual values in new UI.
- New screens should define loading, empty, error, and success states.
- Accessibility and mobile usability are required, not optional polish.
- Core workflows must remain usable on mobile, including transaction entry.

## Archive Handling

- Treat `.archive/` documents as research input only.
- Do not port desktop-only assumptions such as native file pickers, local path selection UX, or Tauri-specific runtime features unless explicitly re-approved.
- When reusing an archive idea, rewrite it in current-stack terms and place it in active docs.

## Current Scope Locks

- Keep the product explicitly single-user until a later decision changes that scope.
- First export milestone must include core ledger CSV and QIF.
- Attachments are currently out of scope.

## Validation Expectations

- Run the narrowest relevant validation after edits.
- Prefer feature-scoped tests first, then broader checks.
- Backend: `./scripts/test-backend.sh`; focused Go commands go through `./scripts/go-backend.sh`, which sets the SQLite build tags a bare `go test` lacks.
- Frontend: `./scripts/test-frontend.sh`. Integrated shape: `pnpm build`. User journeys: `./scripts/test-e2e.sh`.
- The full command matrix and the review checklist are in the `validate-and-ship` skill.
- If docs or conventions change, keep references and commands consistent across the repo.
- Keep README guidance and developer commands aligned.

## Commits

- Use Conventional Commits with the smallest honest scope; do not bundle unrelated refactors.
- Implementation work lands through a branch and a pull request; a direct commit to `main` needs the owner's say-so. *Branches And PRs* in `docs/developer-workflow.md` governs.
