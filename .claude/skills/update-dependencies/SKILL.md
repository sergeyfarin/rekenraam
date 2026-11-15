---
name: update-dependencies
description: Maintenance procedure for Rekenraam's dependencies - survey Dependabot pull requests and GitHub security alerts, update Go modules, npm packages, GitHub Actions and the Docker image in one branch, adopt what the new versions make possible, validate, and open one pull request. Use when asked to update, upgrade or bump dependencies, check security or vulnerability warnings, handle Dependabot, or modernize after an upgrade.
---

# Update Dependencies

One branch, one pull request, everything current. The owner prefers a single
bulk update over merging Dependabot pull requests one by one. This task runs
as a `work-issue` task: branch from the canonical `main`, finish with an open
pull request, fix small things, file issues for the rest.

When the request is only to check (security warnings, what is outdated, what
Dependabot has open), run step 1 and report in the session; update nothing
(`AGENTS.md` → Task Modes).

## 1. Survey first

```sh
gh pr list --author "app/dependabot" --state open --limit 200
gh api --paginate "repos/{owner}/{repo}/dependabot/alerts?state=open&per_page=100" --jq '.[] | "\(.security_advisory.severity) \(.dependency.package.name) \(.security_advisory.ghsa_id)"'
gh api --paginate "repos/{owner}/{repo}/code-scanning/alerts?state=open&per_page=100" --jq '.[] | "\(.rule.security_severity_level // "-") \(.rule.id) \(.most_recent_instance.location.path)"'
gh api --paginate "repos/{owner}/{repo}/secret-scanning/alerts?state=open&per_page=100" --jq '.[].number' | wc -l
gh issue list --label blocked --state open --limit 500
```

Paginate every survey: an unpaginated call returns the first page only and
the list silently looks complete. A feed that answers with an error (403 or
404: not enabled, or the token lacks access) is **unchecked**, not empty; say
so in the report.

Also read the latest `govulncheck` workflow run. Note which blocked issues are
held upgrades (a peer range, a missing upstream release): this run is when
they get rechecked.

## 2. Update

- **npm**: `pnpm -r --include-workspace-root update --latest`. Hold a package
  back with `'!<name>'`. Update pnpm itself with `corepack use pnpm@latest`,
  and keep the `packageManager` pin in `e2e/package.json` identical to the
  root one.
- **Go modules**: from `backend/`, `go get -u ./... && go mod tidy`.
- **Go toolchain**: when `backend/go.mod` moves to a new Go release, never
  judge formatting with a hand-run `gofmt`. Run `./scripts/test-backend.sh`;
  it resolves the right binary, and a new release can reformat files nobody
  touched.
- **GitHub Actions and the Docker image**: bump the action versions and
  `deploy/docker/Dockerfile`. Node and Go versions stay the ones the repo
  pins (`backend/go.mod`, the root `packageManager`, Node as named in
  `docs/developer-workflow.md` → GitHub Actions); change them everywhere or
  nowhere. Pushing a workflow file needs the `workflow` scope on the `gh`
  token.
- **The SQLite driver is not a routine bump.** If `go get -u` moves
  `github.com/mattn/go-sqlite3`, it changes the bundled engine. That is a
  driver admission review (`docs/developer-workflow.md`, the native SQLite
  section, and ADR 0016's race-gate thresholds): time `internal/app` against
  `main`, rerun the admission probes, decide the FTS5 workaround. Do it
  deliberately or pin the current version and file an issue.

## 3. Hold what cannot move

A major version that a peer dependency rejects, or one that fails validation
for a reason too large to fix here, is held. Say which and why in the commit
and the pull request, and record it on the issue that tracks it (create one
when none exists, labelled `blocked` with the prerequisite named). Never force
a peer range.

## 4. Modernize

An upgrade is the moment to stop carrying workarounds.

- Run `go fix ./...` until it changes nothing, then delete helpers it marked
  `//go:fix inline` once nothing uses them.
- Read the release notes of every major and notable minor bump. Look for an
  API that replaces a local helper or workaround, a deprecation the code now
  triggers, a default that changed, and renamed exports that still resolve
  through an alias (icon renames resolve silently).
- Small and local: do it in this pull request as its own commit. Anything
  else: an issue naming the new option and the code it would simplify.

## 5. Security alerts

What may be said in public depends on whether the vulnerability is already
public. `SECURITY.md` governs: nothing unpublished is disclosed before a fix.

**Published advisories** (Dependabot alerts and `govulncheck` findings, which
come from public GHSA/CVE records) may be tracked in the open. Each ends in
one of three states, listed in the pull request:

- **Fixed** by an update in this branch.
- **Not fixable yet** (no patched release, or the patch is a held major): an
  issue with the advisory ID and the dependency.
- **Believed not applicable**: say so.

Whether the vulnerable code is *reachable in this app*, and through which
path, is not published while the alert is open: that turns a public advisory
into a map of how to exploit this deployment. Give that analysis to the owner
in the session.

**First-party findings** (code-scanning alerts on this repository's own code,
and anything you notice yourself) are unpublished. Do not name, describe or
link them in the pull request, commit messages, an issue or a comment. Report
them to the owner in the session, ranked, with your recommendation, and leave
the route to the owner: a private security advisory per `SECURITY.md`, or a
fix that is reviewed privately first. A code-scanning rule that is about code
quality rather than security is ordinary work.

**Secret-scanning alerts** are reported to the owner at once and never quoted.

Dismissing any alert on GitHub is the owner's call; do not dismiss one
yourself.

## 6. Validate

Run the gates one after another, never concurrently; contention alone causes
race-gate timeouts.

```sh
./scripts/test-backend.sh
./scripts/test-frontend.sh
pnpm --dir frontend run test
pnpm build
./scripts/test-e2e.sh
```

A dependency update changes everything, so the full browser run is part of
it.

## 7. Ship

One pull request following `work-issue` step 6. Its body lists: what moved
(with majors called out), what is held and why, the state of each published
advisory, what was modernized, the follow-up issues, and the Dependabot pull
requests it covers (`Covers #612, #613`).

Do not close those Dependabot pull requests yourself. The owner may reject
this pull request or drop an upgrade in review, and a closed Dependabot pull
request would then claim an update that never landed. Dependabot closes its
own pull request once main has the same or a newer version; after this one
merges, list any that are still open for the owner.
