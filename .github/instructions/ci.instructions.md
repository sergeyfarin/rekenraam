---
applyTo: ".github/workflows/**/*.yml"
description: "Use when editing GitHub Actions workflows, job structure, CI triggers, or workflow-level validation commands."
---

# GitHub Actions Instructions

- GitHub Actions workflows are intentionally absent for now.
- Do not add workflow files unless the project owner explicitly reintroduces them.
- Prefer official setup actions when available.
- Keep workflow names and job names readable.
- Use the repo's current toolchain versions: Go from `backend/go.mod`, Node 22, and `pnpm` 11.5.0 unless the repo updates them.
- If workflows are reintroduced, start with manual validation helpers.
- If automatic CI is enabled later, fast CI should cover backend tests, frontend checks, and integrated build confidence.
- Heavier e2e execution should stay in a separate workflow.
- Avoid matrix complexity until there is a real need.
- Use `--frozen-lockfile` for CI installs.
- Upload artifacts only when they provide debugging value.
