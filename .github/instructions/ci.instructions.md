---
applyTo: ".github/workflows/**/*.yml"
description: "Use when editing GitHub Actions workflows, job structure, CI triggers, or workflow-level validation commands."
---

# GitHub Actions Instructions

- The repo uses a fast CI workflow at `.github/workflows/ci.yml`.
- Fast CI covers three jobs: backend tests, frontend check, and integrated build.
- E2E execution belongs in a separate workflow, added when a real user journey exists.
- Prefer official setup actions when available.
- Keep workflow names and job names readable.
- Use the repo's current toolchain versions: Go from `backend/go.mod`, Node 22, and the `pnpm` version pinned in the root `package.json` `packageManager` field. Do not hardcode a pnpm version in workflows or docs.
- Every workflow that runs Go must set up the toolchain with `go-version-file: backend/go.mod`, never a literal version. A workflow that skips this builds with whatever Go the runner preinstalls under `GOTOOLCHAIN=local` and breaks the moment `go.mod` moves ahead of it — which is exactly how CodeQL's default setup broke on the Go 1.27 bump.
- After bumping Go in `backend/go.mod`, re-run `gofmt` with the *new* toolchain before pushing. The formatting gate in `scripts/test-backend.sh` runs CI's gofmt, and a new release can reformat files nobody touched (1.27 reindents multi-value composite literals in `return` statements). A stale local `gofmt` reports clean and the gate still fails.
- If workflows are reintroduced, start with manual validation helpers.
- If automatic CI is enabled later, fast CI should cover backend tests, frontend checks, and integrated build confidence.
- Heavier e2e execution should stay in a separate workflow.
- Avoid matrix complexity until there is a real need.
- Use `--frozen-lockfile` for CI installs.
- Upload artifacts only when they provide debugging value.
