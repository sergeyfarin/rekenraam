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
- Use the repo's current toolchain versions: Go from `backend/go.mod`, Node 22, and `pnpm` 11.5.1 unless the repo updates them.
- If workflows are reintroduced, start with manual validation helpers.
- If automatic CI is enabled later, fast CI should cover backend tests, frontend checks, and integrated build confidence.
- Heavier e2e execution should stay in a separate workflow.
- Avoid matrix complexity until there is a real need.
- Use `--frozen-lockfile` for CI installs.
- Upload artifacts only when they provide debugging value.
