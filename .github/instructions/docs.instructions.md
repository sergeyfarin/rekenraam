---
applyTo: "docs/**/*.md,README.md,AGENTS.md,.github/**/*.md"
description: "Use when editing requirements, ADRs, workflow docs, agent instructions, README files, or other repository documentation."
---

# Documentation Instructions

- Keep active docs aligned with current-stack reality, not archived experiments.
- Requirements belong in `docs/product-requirements.md`.
- Architecture constraints belong in `docs/early-architecture-decisions.md`.
- Sequencing belongs in `docs/feature-roadmap.md`.
- Archive requirement review belongs in `docs/archive-requirements-review.md`.
- Accepted decisions belong in `docs/adrs/`.
- Developer process and commands belong in `docs/developer-workflow.md` or the relevant README.
- Repo-wide agent guidance belongs in `AGENTS.md` and `.github/copilot-instructions.md`.
- When changing commands or workflow expectations, update all affected docs in the same change.
- Keep docs concise, declarative, and source-of-truth oriented.
