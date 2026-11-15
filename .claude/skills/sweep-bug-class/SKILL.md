---
name: sweep-bug-class
description: Hunt for every remaining instance of one known bug class across the whole Rekenraam tree - search for the pattern rather than the copies a ticket named, count and classify every hit, and prove the real ones. A check returns the findings only; a sweep also fixes, files issues, adds a guard and opens a pull request. Use when asked to sweep, hunt, audit or grep for a bug class or pattern, to check whether a bug that was just fixed exists elsewhere, or to work through an entry in the bug-class catalogue.
---

# Sweep One Bug Class

A fix is only as good as its sweep. The decimal-comma error shipped three
times here because each fix covered the copies its ticket named, and a survey
that reported two copies had missed five (catalogue entry 13). This task takes
one class and finds all of it.

## Outcome

The request decides how far this goes (`AGENTS.md` → Task Modes):

- **Check** ("does this exist elsewhere?", "find", "audit", "list"): steps 1–4
  and the report. No branch, no edits, no issues. End by asking whether to
  fix and file.
- **Sweep** (invoked by name, or asked to sweep, fix or clean up): all steps,
  run as a `work-issue` task — branch from the canonical `main`, fix small
  instances, file issues for the rest, finish with an open pull request.

An instance that could be a security vulnerability is never filed or
described publicly; it goes to the owner under `SECURITY.md`.

## 1. Pick the class and learn its shape

The class comes from the prompt: an entry number in
`.claude/skills/validate-and-ship/bug-classes/`, a bug that was just fixed, or
a pattern described in words. Read the entry and the fix that closed it. Write
down, before searching:

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

Update the catalogue entry with the new named tests and the guard. A pattern
that is not in the catalogue yet gets an entry
(`.claude/skills/README.md` → maintenance rules).

## 7. Report

- The pattern and the exact search commands.
- The count: hits, instances, safe, unsure.
- A table of every hit with its verdict and outcome (fixed, issue number,
  left with reason).
- The guard added, or why none is possible.
