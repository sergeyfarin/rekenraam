---
applyTo: "backend/**/*_test.go,e2e/**/*.ts,e2e/playwright.config.ts,api/bruno/**,openapi/**,.github/workflows/**/*.yml"
description: "Use when editing automated tests, API test assets, Playwright config, or CI workflows."
---

# Testing And CI Instructions

- Prefer the narrowest useful validation for the changed slice.
- Backend behavior should be covered with Go tests.
- Frontend logic should at least pass `pnpm --dir frontend run check`.
- Playwright is for critical user journeys, not every tiny UI branch.
- API behavior should stay consistent with OpenAPI and Bruno assets as they become real.
- CI should reuse the repo's real commands rather than invent parallel scripts.
- Do not add GitHub Actions workflows unless the project owner explicitly reintroduces them.
- If automatic checks are introduced later, keep them simple, fast, and incremental.
- Heavy workflows such as e2e should be separate from fast CI.
- If CI changes introduce new durable workflow rules, update `docs/developer-workflow.md`.
