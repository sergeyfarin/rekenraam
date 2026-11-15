---
name: review-pr
description: Review-only procedure for a Rekenraam pull request - judge direction, owner decisions, approach, implementation, tests and docs, and report ranked findings without changing anything. Use when asked to review, re-review, check, assess or give an opinion on a pull request, branch or diff. Never fixes, commits or pushes; use work-issue to implement.
---

# Review A Pull Request

Review only. You change nothing: no edits to the branch, no commits, no
pushes, no pull request comments or approvals, no new issues. Do one of those
only when the owner asks for that specific thing in this session. When a fix
is obvious, describe it; do not make it. The owner passes your findings to the
agent that wrote the change.

The reason is independence. A reviewer that fixes stops being a second
opinion, and its fixes reach `main` unreviewed.

## Prepare

- Read the pull request and what it claims:
  `gh pr view <n> --json title,body,commits,files,closingIssuesReferences`,
  `gh pr diff <n>`, `gh pr checks <n>`.
- Read the linked issue with its acceptance, the governing plan or ADR, and
  where the work sits in `docs/roadmap.md`.
- Get the code without disturbing the working tree:
  ```sh
  git fetch origin pull/<n>/head
  git worktree add --detach <scratch-dir> FETCH_HEAD
  ```
  Run tests and builds there. A throwaway probe test that confirms a suspected
  bug is allowed there and is never committed. Remove the worktree when done.
- Read the whole changed files and their callers, not just the hunks. Review
  the entire pull request, every commit, not only the latest push.
- On a re-review, first check each earlier finding against the new commits
  (fixed, not fixed, fixed differently), then review the new changes as fresh
  code.

## What to judge, in this order

1. **Direction.** Should this be built, now, in this shape? Check it against
   `docs/product-requirements.md`, the roadmap's current focus, the scope
   locks in `AGENTS.md`, accepted ADRs and the issue's acceptance. Owner
   decisions are in scope: when a decision recorded in the issue, plan or pull
   request looks wrong or costly, say so once, plainly, with the evidence and
   the alternative. Do not defer to a decision because the owner made it.
2. **Approach.** Is this the right design for the goal? Layering, the smallest
   durable change, reuse of an existing owner or helper, a simpler
   alternative, what the migration or API contract commits the project to,
   what this makes harder later.
3. **Implementation.** Find the bugs. Trace real inputs through the changed
   code: empty and boundary values, dates at midnight and year end, several
   currencies, unknown basis, concurrent writers, a crash between two
   transactions, retries, an omitted PATCH field, a second page of results.
   Apply `ledger-invariants` to anything touching money. Check the diff
   against `.claude/skills/validate-and-ship/bug-classes/<area>.md` for each
   area it touches; every entry there is a bug this repository already
   shipped.
4. **Tests.** Would each new test fail without the change? Does it reach the
   real consumer or only a builder's output? Is there a negative control?
   What risky path has no test? Verify the pull request's validation claims by
   running the narrowest relevant command yourself, and say what you ran.
5. **Docs and tracking.** OpenAPI and generated types, translations, the docs
   table in `validate-and-ship`, `docs/implemented.md`, and whether `Closes`
   is honest about the issue's acceptance.
6. **Scope.** Unrelated changes, leftover debugging, commits that mix
   concerns.

## What counts as a finding

- It is verified. You read the code path or ran it. Anything you could not
  verify is a question, labelled as one.
- It states a priority, `file:line`, the concrete failure (this input gives
  that wrong result), why it matters, and the direction of a fix in words or
  a few lines.
- Priorities match the repository's labels:
  - **P0** — data loss, a wrong financial result, broken reconciliation
    trust, a security hole.
  - **P1** — wrong behaviour in a core workflow; a violated invariant,
    convention or API contract.
  - **P2** — a real defect with limited reach; a risky path without a test; a
    design problem that will cost soon.
  - **P3** — minor: naming, polish, a simplification.
- No padding. No praise, no summary of the diff, no style points a formatter
  or linter owns. "No findings" is a valid review when you say what you
  checked.

## Report

1. **Verdict** — one of *merge*, *merge after fixes* (name the priorities),
   *rework*, *do not build* — with the reason in a sentence or two.
2. **Direction and decisions** — including any owner decision you would
   change.
3. **Findings** — most severe first.
4. **Questions for the owner** — decisions only the owner can make.
5. **Noticed outside this pull request** — candidate follow-up issues,
   described, not filed.
6. **What I checked and ran, and what I did not.**
