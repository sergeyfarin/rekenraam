---
applyTo: ".github/workflows/**/*.yml"
description: "Use when editing GitHub Actions workflows, job structure, CI triggers, or workflow-level validation commands."
---

# GitHub Actions Instructions

- `.github/workflows/ci.yml` runs backend race tests, backend coverage,
  frontend checks, integrated build and the app/audit browser smoke matrix (Node
  24) once per validation context: pushes to `main`, pull requests and
  `workflow_dispatch`. Other branch pushes do not trigger it (#383). Concurrency
  is keyed by event and ref. Never add path filters or job-level `if:` to it:
  `Backend Tests`, `Frontend Check` and `Integrated Build` are required checks
  on `main` and a skipped job leaves them pending. Keep
  `scripts/test-ci-events.py` in step with any trigger, concurrency or job-name
  change.
- Browser smoke uses disjoint app/audit matrix jobs with three workers each.
  Their union must retain the complete default smoke selection. Record the
  repeated build cost and combined runner minutes when changing this split.
- Browser smoke retains timing artifacts on success and failure; release
  preflight remains a separate weekly/manual workflow.
- Prefer official setup actions when available.
- Keep workflow names and job names readable.
- Use the repo's current toolchain versions: Go from `backend/go.mod`, Node 26 (matching `deploy/docker/Dockerfile`), and the `pnpm` version pinned in the root `package.json` `packageManager` field. Do not hardcode a pnpm version in workflows or docs.
- Every workflow that runs Go must set up the toolchain with `go-version-file: backend/go.mod`, never a literal version. A workflow that skips this builds with whatever Go the runner preinstalls under `GOTOOLCHAIN=local` and breaks the moment `go.mod` moves ahead of it — which is exactly how CodeQL's default setup broke on the Go 1.27 bump.
- After bumping Go in `backend/go.mod`, re-run `gofmt` with the *new* toolchain before pushing. The formatting gate in `scripts/test-backend.sh` runs CI's gofmt, and a new release can reformat files nobody touched (1.27 reindents multi-value composite literals in `return` statements). A stale local `gofmt` reports clean and the gate still fails.
- Push/PR Go jobs use `.github/actions/setup-go-cache`, with a distinct writer
  key for race, coverage, build and each E2E suite; explicit successful save
  steps retain source-only refreshes. Do not re-enable setup-go caching beside
  it or give matrix jobs the same primary key. E2E inherits Go cache paths.
- A setup step must not fail or stall a job on runner I/O or network (#415).
  Keep cache keys out of `hashFiles` over large trees (the Go source key uses
  git object IDs and fails open), and install Chromium through
  `scripts/ci-install-chromium.sh`, which bounds and retries each phase.
- Keep shared validation wrappers and CI commands aligned. Optimize measured
  browser cost before changing coverage or validation frequency.
- Avoid matrix complexity until there is a real need.
- Use `--frozen-lockfile` for CI installs.
- Upload artifacts only when they provide debugging value.
