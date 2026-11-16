---
name: sweep-bug-class
description: Hunt for every remaining instance of one known bug class across the whole Rekenraam tree - search for the pattern rather than the copies a ticket named, count and classify every hit, and prove the real ones. An explicit check or audit returns findings, even when invoked by name. A full sweep also fixes or files instances and opens a pull request for tracked changes. Use when asked to sweep, hunt, audit or grep for a bug class or pattern, to check whether a bug that was just fixed exists elsewhere, or to work through an entry in the bug-class catalogue.
---

# Sweep One Bug Class

A fix is only as good as its sweep. The decimal-comma error shipped three
times here because each fix covered the copies its ticket named, and a survey
that reported two copies had missed five (catalogue entry 13). This task takes
one class and finds all of it.

## Outcome

Select the outcome using [Task Modes](../../../AGENTS.md#task-modes):

- **Check** ("does this exist elsewhere?", "check", "audit", "only list"): steps 1–4
  and the report. No branch, no edits to the checkout, nothing written to
  GitHub. Any probe that needs code changed (a failing test, a broken
  behaviour) runs in a scratch worktree under `.claude/worktrees/`, is never
  committed, and the worktree is removed afterwards. End by asking whether to
  fix and file.
- **Sweep** (invoked by name without an explicit limit, or asked to fix
  and file): all steps. Use
  `work-issue` to branch from canonical `main` when making tracked changes
  and open a PR for those changes. If all instances are filed and there are
  no tracked changes, finish with the GitHub links.

An instance that could be a security vulnerability is never filed or
described publicly; it goes to the owner under `SECURITY.md`.

## 1. Pick the class and learn its shape

The class comes from the prompt: an entry number in
`.claude/skills/validate-and-ship/bug-classes/`, a bug that was just fixed, or
a pattern described in words. Read the entry and the fix that closed it.
Use [GitHub repository selection](../../../docs/developer-workflow.md#github-repository-selection)
for issue and PR lookups and filing. Write down before searching:

- the code shape that causes it;
- the shape of a correct version;
- how it shows up in tests, SQL, fixtures and docs, not only in source.

## 2. Search for the pattern, not the copies

- Search the whole tree: `backend/`, `frontend/src`, `e2e/`, `scripts/`,
  migrations, fixtures. A class found in Go may also live in TypeScript.
- Use several spellings of the pattern. A helper that was copied was usually
  also renamed, reordered or inlined.
- Search for the absence as well as the presence: a call site that should use
  the shared owner and does not.
- Keep the exact commands. They go in the report, so the sweep can be rerun.

## 3. Count and classify every hit

List every hit with `file:line` and give each one a verdict:

- **Instance** — the bug is there.
- **Safe** — with the reason: a guard, a different input domain, a test-only
  path.
- **Unsure** — say what you would need to know.

State the total. A sweep that reports "a few" has not counted.

## 4. Prove each instance

A named test that fails, or a reproduction with the input and the wrong
result. An instance you cannot make fail is "unsure", not "instance".

## 5. Fix or file

The `work-issue` rule applies. A small, local instance is fixed in this pull
request with its named test. A larger one, and any instance that changes a
financial result, reconciliation or stored data, becomes an issue with the
evidence from step 4.

When several copies of a helper are the cause, move them onto one owner. Then
ask what the old copies rejected by accident that the shared one accepts, and
make that rejection explicit and tested per call site (catalogue entry 14).

## 6. Make it stay fixed

When the pattern can be detected mechanically, add a guard test that fails on
a new copy, as `TestEffectiveHistoryReadersUseTheOwner` and
`src/lib/theme-tokens.test.ts` do. Give it an allow-list with a reason per
entry rather than a loose pattern. When it cannot be detected mechanically,
say so.

When the sweep adds tests or a guard, record them in the catalogue entry.
Add an entry when the sweep fixes a new bug class
(`.claude/skills/README.md` → maintenance rules).

## 7. Report

- The pattern and the exact search commands.
- The count: hits, instances, safe, unsure.
- A table of every hit with its verdict and outcome (fixed, issue number,
  left with reason).
- The guard added, or why none is possible.
