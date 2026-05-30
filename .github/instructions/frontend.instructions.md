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
- Production uses `@sveltejs/adapter-static` and the Go binary serves the built frontend. Do not add `+page.server.ts`, `+layout.server.ts`, `+server.ts`, SvelteKit form actions, or production server hooks unless an accepted ADR changes the runtime shape.
- Route production data access through Go `/api/v1` endpoints and the typed API client.
- Do not introduce desktop-only assumptions from `.archive/`.
- Use Svelte 5 runes (`$state`, `$derived`, `$effect`, `$props`) for all new component state. Cross-component and cross-route shared state uses `$state` in `.svelte.ts` module files. Do not use Svelte 4 stores (`writable`, `readable`, `derived` from `svelte/store`) in any new code.
- All new frontend files must be TypeScript: `.ts` files or `.svelte` files with `<script lang="ts">`. Do not create `.js` source files under `frontend/src`.

## Frontend Validation

- Default validation is `./scripts/test-frontend.sh`.
- If the change affects the production bundle or app shell, also run `pnpm build` from repo root.
