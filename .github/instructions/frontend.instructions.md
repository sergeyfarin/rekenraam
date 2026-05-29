---
applyTo: "frontend/src/**/*.svelte,frontend/src/**/*.ts,frontend/src/**/*.js,frontend/svelte.config.js,frontend/vite.config.ts,frontend/tsconfig.json"
description: "Use when editing SvelteKit frontend routes, components, client-side helpers, styling, or frontend configuration."
---

# Frontend Instructions

- All user-facing copy must go through a translation boundary.
- English is the initial implementation language.
- UI code and built-in app-defined data must stay ready for additional languages without route, component, or schema rewrites.
- Keep formatting of money, dates, and numbers locale-aware and separate from translation strings.
- Use semantic design tokens for theming.
- Light and dark themes are the starting requirement.
- Core workflows must remain usable on mobile, including transaction entry.
- New screens must define loading, empty, error, and success states.
- Accessibility is required: keyboard paths, labels, focus handling, and readable contrast.
- Prefer shared frontend helpers and API seams over route-local ad hoc logic.
- Do not introduce desktop-only assumptions from `.archive/`.
- Use Svelte 5 runes (`$state`, `$derived`, `$effect`, `$props`) for all new component state. Do not use legacy Svelte 4 stores for new code.

## Frontend Validation

- Default validation is `pnpm --dir frontend run check`.
- If the change affects the production bundle or app shell, also run `pnpm build` from repo root.
