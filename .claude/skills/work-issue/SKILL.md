---
name: work-issue
description: End-to-end procedure for implementing a GitHub issue, bug fix or feature slice in Rekenraam - branch, build, validate, sweep the surrounding code for defects, then open a pull request without being asked. Use when asked to work on, implement, fix, build or deliver an issue, bug, feature or slice. Not for reviewing a pull request; use review-pr for that.
---

# Work An Issue

An implementation task ends with a pushed branch and an open, ready-for-review
pull request. Do not stop to ask whether to commit, push or open it: that is
the standing instruction for this repository, and it overrides any
machine-wide default to wait for a request. Never merge and never enable
auto-merge; the owner merges. Governing rule: *Branches And PRs* in
`docs/developer-workflow.md`.

An explicit check, report, proposal or dry run takes precedence over invoking
this skill. Read the issue and return findings or an implementation plan in
the session for those requests.

## 1. Understand before changing

- Resolve `$remote` and `$repo` using [GitHub repository
  selection](../README.md#github-repository-selection).
- Read the issue with its comments (`gh issue view <n> --repo "$repo" --comments`),
  the plan or ADR it links, and its parent issue if it is a slice.
- Implementation needs actionable acceptance (`docs/conventions.md` → Work
  Tracking): problem and evidence, bounded scope, measurable acceptance,
  dependencies, validation. Stop and ask one question, with your
  recommendation, when acceptance is missing, when the issue contradicts a
  governing document, or when it depends on an owner decision that is still
  open. Anything smaller you decide yourself and record in the pull request.
- Load the skills for the area from the table in `AGENTS.md`.

## 2. Branch

```sh
git fetch "$remote" && git switch -c <type>/<short-name> "$remote/main"
```

Always cut from the latest canonical `main`. In a fork that is
`upstream/main`, because the fork's own `main` can be behind; in a direct
clone it is `origin/main`. Never cut from another feature branch, and never
onto a branch that already has an open pull request for other work. If the
working tree holds changes that are not yours, stop and ask.

## 3. Build

- Make the smallest durable change that meets the acceptance. One slice.
- For a bug: write the failing named test, see it fail, then fix.
- Keep the app runnable.

## 4. Look around — required, not optional

While you are in the code, look for defects beyond the task: bugs, tests that
cannot fail, missing tests on a risky path, duplicated helpers, code smells,
stale documentation, an easier design. Look at what you read, not only at what
you change. Then give every finding one of two outcomes.

**Fix it in this pull request** when all of these hold:

- It is in, or directly beside, the code this task already touches.
- It is small: a reviewer can verify it at a glance, roughly 30 changed lines.
- It needs no migration, no API contract change, no new dependency and no
  owner decision.
- The validation you are already running covers it, with a named test added
  when it changes behaviour.
- It does not change a financial result, reconciliation behaviour or stored
  data.

Commit it separately from the task (`fix(...)`, `refactor(...)`, `test(...)`)
and list it under *Also fixed* in the pull request.

**File a GitHub issue** for everything else:

- Search first (`gh issue list --repo "$repo" --search "<terms>" --state all
  --limit 100`). Add new evidence to the existing issue when it covers the
  finding. Pass `--repo "$repo"` when filing or commenting.
- Write it with the sections of `.github/ISSUE_TEMPLATE/work-item.md`:
  evidence with `file:line` and a reproduction, bounded scope, measurable
  acceptance.
- Label it `bug` or `enhancement` and a `priority: P0`–`P3` for urgency. Do
  not set a milestone: creating an issue never changes release scope.
- Link it under *Follow-up issues* in the pull request.

A suspected security vulnerability is the exception to both outcomes: do not
fix it in a public pull request and do not file it. Report it to the owner in
the session and follow `SECURITY.md`.

A finding that changes a financial result, reconciliation or stored data
always gets an issue, even when the fix is short; fix it in this pull request
only when the task cannot be completed without it. Report any P0 or P1 finding
to the owner in your final message as well.

Nothing stays only in the conversation: every finding ends as a commit in this
pull request or as an issue. Do not start a second feature or a broad refactor.

## 5. Validate and document

Run the `validate-and-ship` skill: the validation matrix for what changed, the
`bug-classes/<area>.md` file for each area touched, the docs table, the
definition of done. A bug that is a new class gets its entry there in this
change.

## 6. Ship

- If the maintenance task changed only GitHub state, finish with the
  affected links. For tracked changes, commit with Conventional Commits.
- Push to `origin` and open a ready-for-review PR on the canonical repository.
  Prepare `$title` and `$body_file` using `.github/pull_request_template.md`:

  ```sh
  branch=$(git branch --show-current)
  origin_url=$(git remote get-url --push origin)
  head_repo=$(gh repo view "$origin_url" --json nameWithOwner --jq .nameWithOwner)
  head="$branch"
  if [ "$head_repo" != "$repo" ]; then
    head="${head_repo%%/*}:$branch"
  fi
  if [ -n "$branch" ] && git push -u origin "$branch"; then
    gh pr create --repo "$repo" --base main --head "$head" \
      --title "$title" --body-file "$body_file"
  else
    printf '%s\n' 'Branch missing or push failed' >&2
    false
  fi
  ```

  The body states:
  - what changed and why;
  - `Closes #<n>` only when every acceptance item is met, otherwise
    `Refs #<n>` with what remains;
  - the validation you ran with its result, and what you did not run;
  - *Also fixed* and *Follow-up issues* from step 4;
  - each decision you made on the owner's behalf.
- When the issue is a slice of a parent, add a short progress note on the
  parent with `--repo "$repo"` and link the pull request.
- Finish by reporting the pull request URL, the validation result and the
  list of findings with their outcome.

## Review rounds

Review findings come back through the owner. Fix them on the same branch,
push, and add one pull request comment with `--repo "$repo"` explaining each
change and its validation. Disagree in that comment, with evidence, rather
than silently skipping a finding.
