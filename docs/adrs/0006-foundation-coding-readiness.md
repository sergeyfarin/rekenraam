# ADR 0006: Foundation Coding Readiness

## Status

Accepted

## Date

2026-05-30

## Context

The project is ready to start implementation, but the first coding slices will create durable foundations for setup, authentication, API contracts, request middleware, frontend localization, and theme tokens. These decisions should be explicit before code grows around accidental defaults.

Domain-specific accounting decisions such as commodity precision, account tree behavior, posting schemas, reconciliation locks, report snapshots, and import formats remain tied to their feature slices.

## Decision

Phase 0 implementation may start once the following foundation rules are followed.

### Setup And Password Recovery

First-run setup uses versioned API endpoints:

1. `GET /api/v1/setup/status` returns coarse setup progress.
2. `POST /api/v1/setup/owner` creates the single owner and may only succeed when no owner exists.
3. Creating the owner issues a normal authenticated browser session.
4. Later setup steps add book, currency, system-account, and category setup without replacing the owner step.

Setup progress must be persisted as named steps, not as a single boolean. Stable setup step keys start as:

- `owner`
- `book`
- `currencies`
- `system_accounts`
- `categories`

Early password recovery has no unauthenticated browser reset flow and no email-based reset. A self-hosted single-owner deployment should use an operator-controlled local recovery path, such as a future CLI command or documented maintenance command, that requires server or database access and invalidates existing sessions after password reset.

Login and setup APIs must avoid user enumeration. Failed login behavior should return generic errors and include throttling before the app is recommended beyond localhost development.

### API Contract And Error Codes

The API contract workflow is OpenAPI-first for stable public API shapes:

1. `api/openapi/openapi.yaml` is the checked source of truth for stable `/api/v1` endpoints.
2. Handler changes and OpenAPI changes land together.
3. Bruno requests cover important API workflows as they become real.
4. Before frontend code uses generated API types, add a repo command that generates TypeScript types from the checked OpenAPI artifact and document it in developer workflow docs.

Initial structured API error codes are:

- `VALIDATION_FAILED`
- `UNAUTHENTICATED`
- `FORBIDDEN`
- `NOT_FOUND`
- `CONFLICT`
- `CSRF_INVALID`
- `RATE_LIMITED`
- `RESOURCE_BUSY`
- `SETUP_REQUIRED`
- `SETUP_ALREADY_COMPLETE`
- `INTERNAL_ERROR`

All API errors use the envelope shape documented in conventions:

```json
{"error": {"code": "STABLE_CODE", "message": "human-readable detail"}}
```

### Request IDs And Middleware

Every inbound HTTP request gets a server-generated request UUID.

Rules:

1. The server-generated ID is the authoritative request ID.
2. The response includes `X-Request-ID`.
3. Logs include the request ID.
4. If an incoming `X-Request-ID` is present, it may be logged as an external/caller request ID, but it does not replace the server-generated ID.
5. Middleware order is request ID, panic recovery when added, request logging, authentication, CSRF validation for mutating authenticated requests, handler.

### Frontend Foundation

Before the first non-placeholder user-facing screen lands:

1. Add the Paraglide/Inlang message catalog structure.
2. Route visible UI copy through generated message functions.
3. Define semantic theme tokens as CSS custom properties.
4. Support stable `light` and `dark` theme names.
5. Store theme preference only after a real user-facing theme control exists; until then, respect system preference where practical.
6. Keep money, date, and number formatting helpers separate from translated strings.

## Consequences

### Positive

- Phase 0 can start without waiting for full ledger-domain decisions.
- Setup can grow from owner creation into the later book/currency/account/category workflow.
- API tests, frontend types, and handlers have one contract source.
- Error handling and request tracing are stable before real writes exist.
- The first UI slice will not need to undo hard-coded copy or one-off colors.

### Negative

- OpenAPI-first requires discipline to update the spec with handlers.
- Local operator password recovery is less convenient than email reset, but it avoids adding mail delivery and external identity assumptions.
- Theme and i18n scaffolding add some early setup work before the UI is visually complex.
