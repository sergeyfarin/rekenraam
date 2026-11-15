---
name: shape-issue
description: Turn a rough idea, bug report or review finding into an implementation-ready GitHub issue for Rekenraam - evidence, bounded scope, measurable acceptance, real dependencies, and the decisions the owner must make. Explicit draft and check requests stay in the session. Publish when asked to file or when invoked without an explicit limit. Use when asked to shape, draft, write up, refine, split or groom an issue, or to make an existing issue ready for work. Writes no code; use work-issue to implement.
---

# Shape An Issue

The output is one GitHub issue (or a parent with slices) that another agent
could implement with the `work-issue` skill without asking a question. You
write no code, create no branch and open no pull request. Rules:
`docs/conventions.md` → Work Tracking.

Resolve `$repo` using [GitHub repository selection](../../../docs/developer-workflow.md#github-repository-selection).
Use `--repo "${repo:?resolve repo first}"` when reading issues or adding an authorized comment.

Follow [Task Modes](../../../AGENTS.md#task-modes) to select the outcome:

- **Draft or check** ("draft", "propose", "check", "read-only", "dry run"):
  steps 1–5, then return the draft text or findings in the session. Nothing is written to GitHub: no
  issue created or edited, no comment, no label. A duplicate found in step 1
  comes back as its link and the comment you would add.
- **Shape and file** (invoked by name with no explicit limit, or explicitly
  asked to publish or edit the issue): all steps, including step 6.

A suspected security vulnerability is never written up as a public issue. It
goes to the owner under `SECURITY.md`, whatever the request says.

## 1. Find out what already exists

- Search open and closed issues:
  `gh issue list --repo "${repo:?resolve repo first}" --search "<terms>" --state all --limit 100`.
  When one already covers it, the outcome is a comment with the new evidence
  on that issue, not a second issue. Write the comment in step 6.
- Check whether it already ships (`docs/implemented.md`), where it sits in
  `docs/roadmap.md`, and what the governing plan or ADR already decided.
- Check it against `docs/product-requirements.md` and the scope locks in
  `AGENTS.md`. An idea that crosses a scope lock or an accepted ADR is a
  proposal for the owner, not a work item; say so in the issue.

## 2. Gather evidence

- A bug: reproduce it, or trace it in the code. Record the input, the wrong
  result, the expected result and `file:line`. Say plainly when you could not
  reproduce it.
- A feature or improvement: the user-visible gap, who meets it, how the
  current code behaves, and the existing owner or helper the work would
  extend.
- Name the bug class from
  `.claude/skills/validate-and-ship/bug-classes/` when one fits.

## 3. Bound it

- One issue is one reviewable pull request. Work larger than that becomes a
  parent issue with slices; each slice leaves the app runnable and is useful
  by itself.
- State what is deliberately left out and why.
- A dependency is a real prerequisite with its reason. A later roadmap
  position is not a blocker. Use the `blocked` label only when you can name
  the missing prerequisite or evidence and what would unblock the work.
- Separate provider automation from the core or manual feature (ADR 0017):
  missing provider evidence blocks only the adapter.

## 4. Separate decisions from facts

List every decision that is the owner's: product scope, a trade-off with a
lasting cost, a change to a convention or ADR. Give each one your
recommendation and the reason. Do not settle one silently by wording the
acceptance around it. Decide the small things yourself and say what you
chose.

## 5. Write it

Use the sections of `.github/ISSUE_TEMPLATE/work-item.md`:

- **Problem and evidence** — from step 2.
- **Delivery scope** — the bounded outcome and what is deferred. Link the plan
  or ADR; do not copy its design.
- **Acceptance** — checkboxes, each observable and measurable. "Works
  correctly" is not acceptance; "a transfer between accounts in different
  currencies keeps both amounts after an edit" is.
- **Dependencies and blockers** — or "None".
- **Validation** — the named service, API or browser cases that will prove
  it, scaled to the change.
- **References** — related issues, plan, ADR, roadmap entry.

Add **Decisions for the owner** when step 4 found any.

## 6. File it (not for a draft)

- A duplicate of the proposed issue from step 1: add the evidence as a comment on that issue and
  stop; do not create a second one.
- For a new issue, use `gh issue create --repo "${repo:?resolve repo first}"` with the label `bug`,
  `enhancement` or `documentation` and a `priority: P0`–`P3` for urgency.
- No milestone, and no edit to the order in `docs/roadmap.md`: creating an
  issue never changes release scope or sequence. Both are the owner's.
- To make an existing brief issue ready, rewrite its body with
  `gh api -X PATCH "repos/${repo:?resolve repo first}/issues/$n" -F "body=@$body_file"` and add a
  comment saying what changed. Keep the original problem statement.
- Report the issue URL, the decisions waiting on the owner, and whether the
  issue is ready for `work-issue` now or only after those decisions.

## Ready means

A different agent with no access to this conversation can pick the issue up,
knows what to build, knows what not to build, and can tell when it is done.
