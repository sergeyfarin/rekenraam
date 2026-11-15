# AGENTS.md

This file gives repo-wide guidance to coding agents and automation working in this repository. It is the one instruction file every harness used here loads (Claude Code, Codex, OpenCode); do not add harness-specific copies of it.

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

## Task Skills

`.claude/skills/` holds six task-shaped skills distilled from this repo's conventions and bug history. Claude Code and OpenCode discover them there; Codex discovers the same files through `.agents/skills`, a symlink to that directory. This file is only the floor: load the matching skills before starting work.

| Work | Skill |
|---|---|
| Money, transactions, postings, reconciliation, investments, prices, FX | `ledger-invariants`, before writing code |
| Go code, migrations, backend tests | `backend-slice` |
| Any `/api/v1` endpoint, schema or error code | `api-contract` |
| Anything under `frontend/src` | `frontend-screen` |
| Workers, schedulers, provider fetching, FX refresh | `background-work` |
| Before every commit, reviewing a diff, running the app, editing CI workflows, choosing which doc to update | `validate-and-ship` |

- A typical task uses two or three skills, and anything financial includes `ledger-invariants`.
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
- Where a commit goes (`main` or a branch and PR) follows *Branches And PRs* in `docs/developer-workflow.md`.
