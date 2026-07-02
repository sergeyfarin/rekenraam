---
name: api-contract
description: OpenAPI-first API workflow for Rekenraam - spec file layout, regeneration commands, typed frontend client, pagination/date/error conventions. Use when adding or changing any /api/v1 endpoint, request/response shape, or error code.
---

# API Contract Workflow

`api/openapi/openapi.yaml` is the checked source of truth for `/api/v1`.
**Handler changes and OpenAPI changes land in the same commit.** Frontend types
are generated, never hand-written.

## Spec file layout

- `api/openapi/openapi.yaml` — entrypoint only: metadata, global components,
  `$ref` registrations. No inline path or schema bodies here.
- `api/openapi/paths/<area>.yaml` — one file per route pattern
  (e.g. `institutions.yaml`, `institution-versions.yaml`). Reference shared
  schemas as `../openapi.yaml#/components/schemas/...`.
- `api/openapi/components/schemas/<area>.yaml` — schema groups; expose each
  schema through the root `components.schemas` map. Sibling refs go through
  `../../openapi.yaml#/components/schemas/...`.

## Adding an endpoint (checklist)

1. Add/extend the path file under `api/openapi/paths/`, register it in
   `openapi.yaml` under `paths`.
2. Add/extend schemas under `components/schemas/`, register in the root map.
3. New error code? Add it to the `ErrorBody.code` enum as well.
4. Regenerate frontend types:
   ```sh
   pnpm --dir frontend run openapi:generate
   ```
   (also runs automatically inside `dev`, `check`, `build`).
5. Implement the handler (see `backend-slice` skill).
6. Frontend consumes types from the generated file only:
   ```ts
   import type { components } from '$lib/api/schema';
   type Foo = components['schemas']['FooResponse'];
   ```
   Follow the client pattern in `frontend/src/lib/api/connections.ts`
   (`credentials: 'same-origin'`, parse the error envelope into
   `APIClientError`). No user-facing English in `frontend/src/lib/api/` —
   translate error presentation at the UI layer keyed on stable codes.
7. Consider a Bruno request in `api/bruno/` for important workflows
   (`local` env = dev backend on 16888; `app` env = integrated binary).

## Wire conventions (fixed)

- Endpoints live under `/api/v1`. Unknown `/api/` paths 404 — they must never
  fall back to the frontend shell.
- Errors: `{"error":{"code":"STABLE_CODE","message":"..."}}`, code is a stable
  uppercase string the frontend translation layer keys off.
- Calendar dates: ISO 8601 `YYYY-MM-DD` strings. Timestamps: RFC 3339 UTC.
  Money/quantity coefficients: **strings** (see `ledger-invariants`).
- Large list endpoints use **cursor pagination** (stable under concurrent
  inserts), returning `next_cursor`; null/absent = last page. Never offset
  pagination. Frontend list views must consume the cursor (infinite query) or
  show a hidden-results count — never silently truncate.
- Transaction-list search is backend FTS5 via a `search` query parameter;
  never client-side full-text search over fetched pages.
- Page data is backend-composed: one request per page (or per shareable
  component), never per-row/per-card fan-out. If the UI needs a new shape,
  add a read model endpoint.
- `GET`/`HEAD`/`OPTIONS` never mutate durable state.
- Mutating browser routes: JSON bodies only (unless explicitly documented,
  e.g. multipart import upload), same-origin validation, and session-bound
  CSRF token — the middleware enforces this; don't route around it.
- Optional-on-update fields in PATCH requests must be nullable/omittable and
  map to pointer types in Go so omission preserves the existing value.

## Validation

```sh
pnpm --dir frontend run openapi:generate   # must succeed (spec is parseable)
./scripts/test-frontend.sh                 # type check catches contract drift
./scripts/test-backend.sh
```
