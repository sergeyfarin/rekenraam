---
name: validate-and-ship
description: How to run, debug, validate, review, commit, and document changes in Rekenraam - command matrix, dev environment, CI workflow rules, docs-update rules, review checklist of this repo's recurring bug classes. Use before committing any change, when reviewing a diff, when running the app, when editing CI workflows or scripts, or when deciding which doc to update.
---

# Validate And Ship

## Run the app

Run `pnpm install` from the root, then `pnpm dev`. The frontend serves port
1888 and proxies `/api` to port 16888. See [Development](../../../docs/developer-workflow.md#development)
for service and environment settings, and [Local Owner Recovery](../../../docs/developer-workflow.md#local-owner-recovery)
for the `recover-owner` command.

## Validation matrix (narrowest first — this is the contract; CI runs the same scripts)

Frontend commands must work after a clean install, without generated state from
an earlier dev/build run. `catalogs:check` first runs `svelte-kit sync` to create
SvelteKit's `$app/tsconfig` before Vitest transforms catalog tests (#207). Keep
that bootstrap in the shared gate used by dev/check/build and direct catalog
checks; CI must not need a separate warm-up or skip catalog validation.

| Changed | Run |
|---|---|
| `website/**` | `pnpm website:check` and `pnpm website:build`; `pnpm website:check-browser` for presentation/navigation changes (separate static site; no app server). See `website/README.md`. |
| `backend/**` | `./scripts/test-backend.sh` (= `go test -race -p 2 -timeout=5m ./...`), plus `./scripts/go-backend.sh vet ./...`, `gofmt -l .` in `backend/` |
| Backend coverage check (optional, same script) | `COVERAGE=1 ./scripts/test-backend.sh` — non-race coverage pass, prints the merged total; CI enforces a soft floor (`scripts/check-coverage-floor.sh`) |
| `frontend/**` | `./scripts/test-frontend.sh` (openapi:generate + paraglide:compile + svelte-check); `pnpm --dir frontend run test` for unit-tested logic |
| OpenAPI | both scripts above |
| Integrated shape / static serving / embed | `pnpm build` (builds frontend, copies into `backend/internal/web/dist/`, runs embed test, compiles `dist/rekenraam`) |
| After a non-trivial merge | `./scripts/check-post-merge.sh` — migration/ADR number collisions, OpenAPI client drift, duplicate doc paragraphs, duplicate workflows, colliding IDs, tests lost by the merge (*Post-Merge Audit* in `docs/developer-workflow.md`) |
| Release packaging or candidate workflow | `python3 scripts/test-release-artifacts.py`; actual package/repeat/smoke and signature/tamper controls in `docs/developer-workflow.md` → Release candidates (#333). ADR 0020 requires exact workflow/ref identity verification; local key-pair tests are not hosted OIDC evidence. Candidates never create tags or publish releases. |
| Changed the bundle export, the self-check, or a whole-book read | `./scripts/measure-large-book.sh` — 250k-posting book, bounded wall time and peak heap (*Large-book measurement* in `docs/developer-workflow.md`); weekly in CI, never on push |
| Changed the investment writer admission or payment-in-lieu entitlement checks | `REKENRAAM_PIL_DENSITIES=0,10,100 ./scripts/measure-payment-in-lieu-density.sh` — compare against the #379 figures (*Payment-in-lieu density measurement* in `docs/developer-workflow.md`); never on push |
| User journeys | `./scripts/test-e2e.sh` (self-contained: builds app once, one instance + fresh SQLite per worker under `backend/var/e2e/`, ports from 127.0.0.1:16900; `E2E_SKIP_BUILD=1` to reuse the binary). Playwright covers critical journeys, not every UI branch; pure logic gets a Go or Vitest test. |
| `.github/workflows/**`, CI scripts | Read *GitHub Actions* in `docs/developer-workflow.md` before editing, then `python3 scripts/test-ci-events.py`. `Backend Tests`, `Frontend Check` and `Integrated Build` are required checks on `main`: never add path filters or a job-level `if:` to `ci.yml`, because a skipped job leaves them pending. Go comes from `go-version-file: backend/go.mod`, Node and pnpm from the versions the repo pins; never a literal. |

For a full local browser trace, set `E2E_FULL_TRACE=1` on a focused test.
See [E2E](../../../docs/developer-workflow.md#e2e) for the light defaults and
diagnostic commands.

Native SQLite requires CGO and a C compiler with development headers. Supported
commands source `scripts/native-sqlite-env.sh`, which enables `fts5,netgo,osusergo`
and builds the engine with `-DSQLITE_ENABLE_QPSG` (ADR 0014).
Use `./scripts/go-backend.sh test ...` for focused checks. Go race detection
covers application and driver-wrapper Go code; it does not instrument the
bundled SQLite C engine.

Finish `pnpm build` before running backend gates: the build replaces embedded
assets under `backend/internal/web/dist`, so overlapping compilation can read
files that have just been removed. Reuse the completed current browser binary
with `E2E_SKIP_BUILD=1`; do not reuse it after changing runtime/frontend code.

ADR 0016 sets two concurrent packages and a five-minute per-package deadline
for both race and coverage modes. Normal commands use available Go CPU
resources; the old four-core tuning advice applies only to historical modernc
measurements. CI's integrated build runs independently, and every check remains.

Never invent parallel validation commands; if a command must change, change
the script and CI together and update `docs/developer-workflow.md`.

See [Go caches and measurements](../../../docs/developer-workflow.md#go-caches-and-measurements)
for CI cache inheritance and `GO_PHASE_TIMINGS=1` diagnostics.

## Review checklist — this repo's recurring bug classes

Every title below shipped as a real bug here at least once. The titles are the
checklist; each full entry (what shipped, the rule, the named regression tests
to keep green) lives in a per-area file in `bug-classes/` beside this file.

On any non-trivial diff, yours or one you review:

1. Read `bug-classes/general.md` — it applies to every bug fix, helper
   consolidation and new test.
2. Read the file for each area the diff touches and check the diff against
   every entry in it. A diff that touches two areas reads two files.
3. Skim the remaining titles; open an entry when its title could describe
   your change.

Numbers are stable IDs cited from code comments and docs ("validate-and-ship
item 31"); they are not an order and are never reused. To read one entry:
`grep -n -A40 '^## 31\.' .claude/skills/validate-and-ship/bug-classes/*.md`.

**Any bug fix, refactor or test** — `bug-classes/general.md`

- 11 — Builder-output tests that never reach the real consumer
- 13 — A duplicated helper is only as fixed as its least-visited copy
- 14 — Consolidation that silently widens what is accepted

**Backend, SQLite and API contract** — `bug-classes/backend.md`

- 1 — Silent limit clamping
- 2 — PATCH omission overwrites
- 4 — Split-transaction crash holes
- 8 — Error-envelope drift
- 10 — Logging financial content
- 36 — Canonical text CHECKs that inspect only a valid prefix
- 40 — Checking a process lock without holding it through replacement
- 41 — Upload limits that silently return a prefix
- 45 — Filesystem paths treated as SQLite URI syntax
- 56 — Empty data bypassing a native parser negative control
- 58 — Generated types mistaken for runtime schema evidence
- 59 — Mutation journals bypassing display enrichment
- 61 — JSON decoder limits drifting from operations and EOF checks
- 66 — A fresh connection per call hiding connection-lifetime state
- 82 — A reference read before the owner's day is pinned

**Auth, sessions and secrets** — `bug-classes/auth-security.md`

- 3 — TOCTOU guard races
- 47 — Security state checked before a later independent write
- 48 — A budget checked before a verifier and recorded after it
- 51 — Transport errors copying operator secrets into history
- 62 — Previous-step TOTP fixtures crossing a real clock boundary

**Ledger, reconciliation, dates and pricing** — `bug-classes/ledger.md`

- 7 — Reconciliation guard bypass
- 12 — Creation dates masquerading as financial facts
- 15 — Producer drafts mistaken for posted ledger changes
- 27 — A self-check that reports damage on healthy books
- 39 — Browser “today” and refreshes rewriting working copies
- 74 — Account defaults mistaken for currencies actually held
- 81 — A raw debit-positive balance in a class-normalized field

**Investments** — `bug-classes/investments.md`

- 17 — A sound current projection hiding damaged disposal snapshots
- 18 — Offsetting disposal errors hiding behind sound group totals
- 19 — Unknown basis converted to a numeric zero
- 20 — Source identity lost on an imported correction descendant
- 22 — A preview reporting checkpoints for an impossible replay
- 23 — Replay silently restating committed gains
- 24 — A new replay intent kind that moves quantity without pinning its journal
- 25 — A replay branch that omits a side effect its writer performs
- 26 — Revision rows read without asking whether their operation is still effective
- 30 — Position identity dropping side
- 31 — Transfer-link quantity read as the destination's quantity, or its instrument read as the destination's
- 32 — A transfer link's source side assumed to be a `transfer_out` that moves units
- 33 — Every disposal decision assumed to dispose units
- 34 — Reading `effective_investment_lot_events` as if it were complete
- 35 — An operation kind added without its capabilities
- 37 — Validating a different representation from the one readers consume
- 38 — Treating source corrections as amount corrections
- 43 — One response assembled from different effective histories
- 44 — A complete formula mistaken for a complete return
- 80 — A valuation quote chosen by sorting evidence by recording time (#203)

**Imports, providers and background work** — `bug-classes/imports-background.md`

- 5 — Cursor boundaries
- 21 — Order side mistaken for provider event type
- 46 — Missing defaults silently discarding known financial attribution
- 50 — Continuation deduplicated against itself
- 75 — A loaded empty demand treated as uninitialized
- 76 — Completing future demand before it becomes due

**Frontend** — `bug-classes/frontend.md`

- 6 — Unconsumed pagination
- 9 — i18n bypass
- 16 — Animating a control's disabled fade
- 28 — Contrast measured in one palette only
- 29 — Position identity dropping cost currency
- 52 — Colour utility naming no token
- 54 — Hand-rolled modal overlay
- 55 — Layout and hover states no sweep looked at
- 63 — Date-shaped URL values mistaken for calendar dates
- 69 — New version token attached to old form fields
- 70 — A new event source falling through to an old meaning
- 71 — Long identifiers setting a grid's intrinsic width
- 72 — Translated accessible names splitting their visible label
- 73 — An active tab geometrically visible beneath an overflow fade
- 77 — Navigation completion clearing a newer interaction
- 78 — A dismissal guard that sees only part of a write chain
- 79 — An editor that restates more than the owner changed

**Browser journeys and test fixtures** — `bug-classes/e2e.md`

- 42 — Browser journeys that depend on live providers or late handlers
- 49 — Partial form labels matching data inside another control
- 53 — Sweep that audits the sign-in screen
- 57 — CSP-blocked browser measurement
- 60 — Test harness exception disclosure
- 64 — A generic heading certifying the wrong route fixture
- 65 — Cold compilation inside a browser startup deadline
- 67 — A spec finding its own entry by scanning a shared list page
- 68 — A book-wide self-check read in a shared worker book

A new bug class takes the next free number and lands in the same change as its
fix: the entry goes in the matching area file (what shipped, the rule, the
named regression), and its title goes in the list above.

Fix workflow for any bug: failing named test first, then the fix, then the
full relevant suite.

## Docs to update in the same change (the docs ARE the product memory)

| What changed | Update |
|---|---|
| Feature shipped / status changed | `docs/implemented.md` (feature ledger); remove the item from the roadmap's current focus |
| Tech debt found or paid | GitHub Issue (`#number`) with evidence, bounded scope, measurable acceptance, dependencies and validation; retain existing local aliases in `docs/backlog.md`, never allocate new ones |
| Durable product behavior/scope | `docs/product-requirements.md` |
| Repo-wide rule/convention | `docs/conventions.md` |
| Long-lived tradeoff decision | new ADR in `docs/adrs/` (ADRs supersede everything once accepted) |
| Commands/layout/workflow | `README.md` + `docs/developer-workflow.md` (keep both consistent) |
| A convention a skill restates | the skill under `.claude/skills/`, in the same change as `docs/conventions.md` |
| Repo-wide agent guidance | `AGENTS.md` only; it is the one file every agent harness loads |

Precedence when docs conflict: product-requirements → conventions →
early-architecture-decisions → ADRs govern all → developer-workflow.
`AGENTS.md` and skills are guidance, not product sources of truth.
`.archive/` is historical reference only — never port from it directly.

Follow [Work Tracking](../../../docs/conventions.md#work-tracking) for progress,
acceptance, closure evidence and follow-ups.

## Commits

Conventional Commits, smallest honest scope:
`feat(backend): ...`, `fix(api): ...`, `docs(requirements): ...`,
`test(backend): ...`. Don't mix unrelated refactors.

Follow [Branches And PRs](../../../docs/developer-workflow.md#branches-and-prs)
and `work-issue` for branch selection, direct commits and PR delivery.
Record changes to product rules, conventions or ADRs in the commit and PR body.

## Provider-independent delivery (ADR 0017)

Review core/manual acceptance separately from verified provider automation.
Missing API data, credentials or live samples block only the adapter mapping.
Check explicit partial attribution, affected report coverage, manual repair and
reimport without duplicate effects or lost owner corrections. A real app-owned
recovery gap stays actionable; do not call it implemented through a policy change.
Use deterministic core fixtures without live provider access; synthetic examples
do not establish unverified wire semantics. Keep the roadmap and issue dependency
claims consistent with this boundary.

## Definition of done for a slice

1. App still runnable end-to-end (`pnpm dev` or `pnpm build`).
2. Narrowest relevant validation green; new behavior has named tests.
3. OpenAPI + generated types in sync (if API touched).
4. Docs updated per the table above.
5. No violation of `ledger-invariants` (if money/ledger touched).
6. Focused conventional commits on a branch, and an open pull request.
7. Every defect noticed on the way is a commit in that pull request or a
   GitHub issue (`work-issue` skill, step 4).
