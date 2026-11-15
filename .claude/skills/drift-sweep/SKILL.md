---
name: drift-sweep
description: Maintenance sweep for things in Rekenraam that have gone stale - blocked GitHub issues whose blocker is gone, roadmap and tracking entries that no longer match reality, docs and agent guidance that disagree with the code, and tests that are skipped, vacuous or cite closed work. Use when asked to check for stale or drifted docs, roadmap, comments, tests or blocked issues, or to run a maintenance or hygiene sweep.
---

# Drift Sweep

Compare what the repository claims with what is true, and close the gap. Run
the scopes named in the prompt (`blocked`, `roadmap`, `docs`, `tests`); with
none named, run all four in that order.

For an explicit check, report, read-only request or dry run, return findings
and describe what a full sweep would do (`AGENTS.md` → Task Modes). Run any
probe that needs code changes in a scratch worktree under `.claude/worktrees/`.
Keep the probe uncommitted and remove the worktree afterward.
That scope takes precedence over the skill name. A named
invocation with no explicit limit or a request to fix the drift runs the
full workflow below.

Resolve `$repo` using [GitHub repository selection](../README.md#github-repository-selection)
before surveying. Use `--repo "$repo"` for issue reads, comments, labels,
milestones and workflow runs.

Two rules hold for every scope of a full sweep:

- **Correct facts, never decide.** A sweep fixes a status, a link, a path, a
  number. It does not reorder the roadmap, change scope, or pick between a
  document and the code when they disagree about behaviour. Those go to the
  owner as an issue or a question.
- **Open a pull request when tracked files change.** Use `work-issue` to
  branch from canonical `main` before the first repository edit. A
  GitHub-only sweep, including a `blocked` run, finishes in the session with
  links to its comments, label changes and issues. A sweep with no changes
  reports what it checked.

## `blocked` — blocked issues

```sh
gh issue list --repo "$repo" --label blocked --state open --limit 500 --json number,title,updatedAt
```

Every `gh ... list` defaults to 30 results. Pass `--limit` (or `--paginate`
for `gh api`) on every survey, and compare the count you got with the limit.

For each one, read the prerequisite it names and check it: is the upstream
release published, has the peer range changed, has the evidence arrived, did
the issue it waits on close?

- **Unblocked, verifiably** (a published version, a merged pull request,
  recorded evidence): comment with the evidence and its link, remove the
  `blocked` label, and list it in your report. Do not start the work.
- **Probably unblocked, a judgement call**: comment with what changed, leave
  the label, and ask the owner in your report.
- **Still blocked**: no comment. Silence is not noise.
- **Names no prerequisite**: comment asking for one; the label requires it.

## `roadmap` — tracking against reality

- `docs/roadmap.md` current focus: entries whose issue is closed or whose
  pull request merged; "next" items already delivered; dated statements that
  have expired.
- `docs/implemented.md`: features merged since the last sweep that are not
  recorded (`gh pr list --repo "$repo" --state merged --limit 500 --search "merged:>=<date>"`).
- Open issues whose acceptance is fully met, and closed issues with an unmet
  acceptance item.
- The `v0.1.0` milestone against the roadmap's accepted gates. Report the
  difference; changing membership is the owner's.
- Issue numbers cited in active docs that point at closed or retitled issues.

Fix statuses and links. Report anything that implies a change of order.

## `docs` — documents and guidance against the code

- Commands in `README.md` and `docs/developer-workflow.md`: every script and
  `pnpm` script they name exists and does what the text says.
- File paths, function names and test names cited in `AGENTS.md` and under
  `.claude/skills/` (reference files included) still exist. This is
  mechanical: extract the backticked names and look each one up.
- Toolchain versions stated in prose match `backend/go.mod`, the root
  `packageManager` and the workflows.
- A rule a skill restates still matches `docs/conventions.md`.
- `AGENTS.md` scope locks and "currently" statements that have since shipped.
- Each `SKILL.md` is still lean (see `.claude/skills/README.md`).

Leave completed plans, dated reviews and anything under `docs/archive/` or
`docs/reviews/` alone: they are historical records, and being out of date is
their job.

## `tests` — tests that no longer earn their place

- Skipped or disabled tests (`t.Skip`, `test.skip`, `test.fixme`, a
  known-defect marker): is the reason still true, is the issue it cites still
  open? Re-enable when the reason is gone; file an issue when the test now
  fails.
- Tests that cannot fail: an assertion that is true of an empty result, a
  check on a builder's output that never reaches the real consumer, a browser
  check that runs on the sign-in screen
  (`.claude/skills/validate-and-ship/bug-classes/general.md` and `e2e.md`).
- `// serial:` opt-outs whose reason no longer applies.
- Tests that fail intermittently in CI:
  `gh run list --repo "$repo" --workflow ci.yml --status failure --limit 100`,
  then group by failing test.
- Comments and test names that cite an issue as pending when it is closed.

A test you believe is vacuous is proved so before you change it: break the
behaviour it claims to cover and watch it still pass.

## Report

One table per scope with what was checked, links to repository or GitHub
changes, and what needs the owner. "Nothing found" is a valid result for a
scope when the report says what was checked.
