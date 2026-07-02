---
applyTo: "docs/**/*.md,README.md,AGENTS.md,.github/**/*.md"
description: "Use when editing requirements, ADRs, workflow docs, agent instructions, README files, or other repository documentation."
---

# Documentation Instructions

- Keep active docs aligned with current-stack reality, not archived experiments.
- Requirements belong in `docs/product-requirements.md`.
- Architecture constraints belong in `docs/early-architecture-decisions.md`.
- Sequencing belongs in `docs/product-requirements.md`.
- Archive requirement review belongs in `docs/archive-requirements-review.md`.
- Accepted decisions belong in `docs/adrs/`.
- Developer process and commands belong in `README.md` and `docs/developer-workflow.md`.
- Repo-wide agent guidance belongs in `AGENTS.md` and `.github/copilot-instructions.md`; task-shaped agent skill guides belong in `.claude/skills/`.
- When a rule in `docs/conventions.md` changes, update any `.claude/skills/` file that restates it in the same change.
- When changing commands or workflow expectations, update all affected docs in the same change.
- Avoid adding area README files for brief command notes; keep local README files for substantial area-specific guidance or ignored/generated directory placeholders.
- Keep docs concise, declarative, and source-of-truth oriented.
