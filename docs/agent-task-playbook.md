# Agent Task Playbook

This is the owner's guide to directing coding agents in this repository: which
kinds of task exist, how to start each one, and what comes back. Agents do not
need this file. Their rules are in `AGENTS.md` and the skills under
`.claude/skills/`; where this guide and a skill differ, the skill is what the
agent follows.

## Starting a task

Every harness used here loads `AGENTS.md` (Claude Code through the one-line
`CLAUDE.md` that imports it) and the same twelve skills. How to check each
harness is in `.claude/skills/README.md`. Start the prompt with the task skill
and the issue or pull request number:

| Harness | Prefix | Example |
|---|---|---|
| Claude Code | `/` | `/work-issue 592` |
| Codex | `$` | `$work-issue 592` |
| OpenCode | `@` | `@work-issue 592` |

- One task per session. A session that implements should not also review.
- Add a sentence after the number when the task needs steering: the slice to
  take, a constraint, a decision already made.
- Name an area skill too when the area is not obvious from the issue, for
  example `/work-issue 592 with ledger-invariants`.
- An agent that was not given a task skill still follows `AGENTS.md` → Task
  Modes, so a plain "fix #592" works. The prefix makes it certain.
- Your wording sets what the agent may change. "Check", "find", "list" or
  "assess" gets a report and changes nothing. "Draft" gets text. "Fix",
  "build", "update", "sweep" or "file" — or the skill invoked by name — runs
  the full workflow: branch, commits, pull request, issues.
- Suspected security vulnerabilities come back to you in the session, never
  as a public issue or pull request text. You choose the private route in
  `SECURITY.md`.

## Task skills

These six say how a session runs from start to finish.

| Skill | Start it when | It ends with |
|---|---|---|
| `work-issue` | An issue, bug or feature slice is ready to build | An open pull request, plus issues for what it found |
| `review-pr` | A pull request needs a second opinion | A verdict and ranked findings; nothing changed |
| `shape-issue` | An idea, bug report or finding is not yet buildable | A ready GitHub issue and the decisions it needs |
| `update-dependencies` | Dependabot or security alerts have piled up | One pull request with every update |
| `drift-sweep` | Docs, roadmap, tests or blocked issues may be stale | A small pull request, issues, and a report |
| `sweep-bug-class` | A fixed bug could exist elsewhere | Every instance found, fixed or filed, and a guard |

### `work-issue` — implement

```
/work-issue 592
```

The agent reads the issue, branches from the latest canonical `main`, builds
the slice, validates it and opens a ready-for-review pull request without
asking. It never merges.

While it works it looks for other defects in the code it reads. A small, local
one is fixed in the same pull request as its own commit and listed under
*Also fixed*. Anything larger, and anything that changes a financial result,
becomes a GitHub issue listed under *Follow-up issues*.

- It stops and asks when the issue has no usable acceptance or depends on a
  decision you have not made. Run `shape-issue` first to avoid that.
- Review findings go back to the same session. It fixes on the same branch and
  answers each finding in one pull request comment.

### `review-pr` — review only

```
/review-pr 585
```

The agent reviews direction, your own decisions, approach, implementation,
tests and docs, in that order. It reports a verdict (*merge*, *merge after
fixes*, *rework*, *do not build*) and verified findings ranked `P0`–`P3` with
`file:line`. It edits nothing, commits nothing, and files nothing.

- Use a different model from the one that wrote the pull request, in a fresh
  session. The value is a second opinion with no memory of the implementation.
- Paste the report into the implementing session.
- Ask for a re-review after fixes: `/review-pr 585 re-review`. It checks each
  earlier finding, then reviews the new changes as fresh code.
- To have the review posted on GitHub, say so in the prompt. The default is a
  report in the session.

### `shape-issue` — make work buildable

```
/shape-issue transfers between two foreign-currency accounts lose the fee
/shape-issue 583
```

The agent searches for duplicates, checks the roadmap, requirements and scope
locks, gathers evidence, bounds the work to one reviewable pull request (or a
parent with slices), and files the issue with measurable acceptance. Ask for
a *draft* to get the text back without anything being filed. It writes
no code. It sets no milestone and does not touch roadmap order; both are
yours.

- The report lists **decisions for the owner**, each with a recommendation.
  Answer them on the issue before running `work-issue`.
- Give it an existing issue number to turn a brief issue into a ready one.

### `update-dependencies` — upgrades and security

```
/update-dependencies
```

The agent surveys open Dependabot pull requests and GitHub's Dependabot,
code-scanning and secret-scanning alerts, updates npm, Go modules, GitHub
Actions and the Docker image in one branch, runs every gate including the
full browser run, and opens one pull request.

- Anything it cannot move is held with the reason and tracked on an issue.
- A new version that replaces a local workaround is adopted when the change is
  small, and filed as an issue when it is not.
- Every published advisory ends as fixed, not fixable yet (with an issue) or
  believed not applicable. Code-scanning findings on our own code, and whether
  a vulnerable dependency is reachable here, come to you in the session, not
  into the pull request. It does not dismiss alerts; that is yours.
- It links the Dependabot pull requests it covers and leaves them open;
  Dependabot closes each once its update is on `main`.
- "Check the security warnings" gets the survey only, with no updates.
- A SQLite driver bump is treated as a driver admission review, not a routine
  update.

### `drift-sweep` — find what went stale

```
/drift-sweep
/drift-sweep blocked
/drift-sweep docs tests
```

Four scopes; name the ones you want or run all four.

| Scope | Checks |
|---|---|
| `blocked` | Each `blocked` issue's named prerequisite. Verifiably unblocked: it comments with the evidence, removes the label and tells you. It does not start the work. |
| `roadmap` | Roadmap focus, `docs/implemented.md`, issue status and the release milestone against what actually merged. |
| `docs` | Commands, paths, test names and versions in the docs, `AGENTS.md` and the skills against the code. |
| `tests` | Skipped tests whose reason is gone, tests that cannot fail, flaky tests, tests citing closed work. |

It corrects facts and never decides: it will not reorder the roadmap, change
scope, or choose between a document and the code when they disagree about
behaviour. Those come back to you.

### `sweep-bug-class` — find every copy

```
/sweep-bug-class 13
/sweep-bug-class the bug fixed in #577
check whether the bug fixed in #577 exists anywhere else
```

The first two run the full sweep. The third is a check: it reports every hit
with a verdict and changes nothing.

Give it an entry number from the bug-class catalogue
(`.claude/skills/validate-and-ship/bug-classes/`), a pull request that just
fixed something, or a pattern in words. It searches the whole tree for the
pattern, counts and classifies every hit, proves the real ones with a failing
test, fixes the small ones, files the rest, and adds a guard test where the
pattern can be detected mechanically.

- Run it after any bug whose cause was a copyable shape: a duplicated helper,
  a query written the same way in several readers, a repeated form pattern.
- The report includes the exact search commands, so the sweep can be rerun.

## Area skills

These six hold the rules for a part of the codebase. Agents load them from the
routing table in `AGENTS.md`; you rarely need to name them.

| Skill | Covers |
|---|---|
| `ledger-invariants` | Money, postings, reconciliation, audit, dates; `investments.md` beside it for the investment subledger |
| `backend-slice` | Go layering, SQLite, migrations, error envelope, backend tests |
| `api-contract` | OpenAPI-first workflow, generated types, wire conventions |
| `frontend-screen` | SvelteKit constraints, i18n, tokens, shell, dialogs, data layer |
| `background-work` | Work queue, schedulers, leases, provider adapters |
| `validate-and-ship` | Commands, CI rules, docs to update, and the catalogue of bugs this repository has shipped |

## Tasks worth practising that have no skill

These are prompts, not procedures. Each is one session.

- **Challenge a decision before it is built.** "Here is my plan for X. Argue
  against it, name the cheapest alternative, and say what evidence would
  change your mind. Do not implement." Use a different model from the one
  that will build it.
- **Ask which tests cannot fail.** "For `<package>`, list the tests that would
  still pass if the behaviour they name were broken. Prove each one by
  breaking it. File issues." This is the `tests` scope of `drift-sweep` aimed
  at one package.
- **Verify in the running app.** "Start the app, do `<workflow>` in the
  browser as a user would, and report what you saw, with screenshots."
  Tests passing and the feature working are different claims.
- **Rehearse the release.** "Do a fresh install, an upgrade from the previous
  build, a backup and a restore, following `docs/upgrades.md` exactly. File
  every step that fails or is unclear."
- **Hold a retrospective after a hard review.** "This pull request took
  several review rounds. What class of bug was each finding? Add any new
  class to the catalogue." This is how the catalogue grows.

## A workable rhythm

A suggestion, not a rule.

- **Per feature:** `shape-issue` → answer its decisions → `work-issue` →
  `review-pr` with another model → fixes → merge.
- **After a bug fix with a copyable cause:** `sweep-bug-class`.
- **Weekly:** `update-dependencies` (Dependabot opens its pull requests on
  Mondays) and `drift-sweep blocked`.
- **Monthly, and before a release:** a full `drift-sweep`, then the release
  rehearsal.

## When the project changes phase

Agent guidance follows decisions; it does not anticipate them.

- **`v0.1.0` is tagged.** Migrations become immutable, compatibility of stored
  and exported data starts to matter, and several skills restate pre-release
  rules. The changes are listed on #586 and are made when the tag exists.
- **A desktop edition is accepted.** `AGENTS.md` currently locks the product
  shape to one Go binary serving a static frontend and forbids desktop-only
  assumptions. A spike does not change that. An accepted ADR does, and the
  same change then updates `AGENTS.md`, `frontend-screen`, `backend-slice`
  and the validation matrix.
- **A new harness or model is adopted.** Check that it loads `AGENTS.md` and
  one of `.claude/skills/` or `.agents/skills/`, and add it to the table in
  `.claude/skills/README.md` with how to verify it. Do not add a
  harness-specific instruction file.
