# Setup And Auth Implementation Plan

This document maps the next setup and auth slices to the current repository shape. It is an implementation checklist, not a replacement for accepted ADRs or product requirements.

## Locked Decisions

- Keep `setup_steps` as durable backend progress state.
- Do not use a UI setup table as the primary setup experience.
- Early forgotten-password recovery is operator-controlled, local, and backup-first.
- Cookie session plus CSRF stays same-origin and follows ADR 0002.
- Owner creation stays Argon2id-based and upgradeable.

## Slice 1: Derived Install State

Goal: stop treating future seeded setup steps as immediate user blockers.

Backend APIs:

- Extend `GET /api/v1/setup/status` with a derived install state.
- Return `install_state`, `implemented_steps`, and `blocking_step` alongside raw `steps`.

Backend modules:

- Derive install state in `backend/internal/app/setup.go`.
- Add repository helpers in `backend/internal/db/setup.go` for owner and setup-state inspection.

Current runtime derivation:

- `fresh`: no owner exists and the owner step is not marked complete.
- `configured`: owner exists and the owner step is marked complete.
- `recovery_required`: owner rows and owner-step completion disagree.

Notes:

- `owner` is the only currently implemented setup step.
- Future pending steps must not block the app until their APIs and UI exist.

Tests:

- Empty database returns `fresh`.
- Owner created database returns `configured`.
- Inconsistent owner/setup-step state returns `recovery_required`.

## Slice 2: Login, Logout, Session Introspection

Goal: make returning-owner access coherent.

Backend APIs:

- Add `POST /api/v1/auth/login`.
- Add `POST /api/v1/auth/logout`.
- Add `GET /api/v1/auth/session`.

Backend modules:

- Add password verification and optional rehash-on-login.
- Move cookie issuance into shared auth helpers.
- Add session lookup, revocation, and expiration handling.

Migrations:

- Extend `auth_sessions` with expiration and revocation fields.
- Add session-bound CSRF material if needed for the chosen CSRF approach.

Frontend modules:

- Add auth query and mutation modules.
- Add a login form that reuses the shared translated API error surface.

Tests:

- Correct password login.
- Generic wrong-password failure.
- Logout revokes the session.
- Session introspection rejects revoked or expired cookies.

## Slice 3: CSRF Baseline

Goal: satisfy ADR 0002 before authenticated mutations expand.

Backend APIs:

- Add CSRF token issuance on session bootstrap or a dedicated endpoint.

Backend modules:

- Add Origin validation for authenticated mutating requests.
- Add CSRF validation middleware.

Frontend modules:

- Add shared CSRF-token handling in the typed API client seam.

Tests:

- Missing or mismatched Origin rejected.
- Missing or invalid CSRF token rejected.
- Valid token accepted.

## Slice 4: Operator Recovery

Goal: recover forgotten access without a browser reset flow.

Backend modules:

- Add a local maintenance command that creates a verified backup first, resets the owner password hash, and revokes all sessions.

Rules:

- Backup first by default.
- Abort reset if backup creation or verification fails unless an explicit emergency override is used.

Tests:

- Recovery command creates a backup artifact.
- Recovery command invalidates all sessions.

## Slice 5: Minimal Setup UI

Goal: ship the first real setup and login screens without overcommitting to future domains.

Frontend states:

- `fresh`: owner setup form.
- `configured`: app shell or auth gate, depending on session state once auth APIs exist.
- `recovery_required`: recovery instructions only, with no browser-side reset path.

Frontend modules:

- Setup status query.
- Owner creation mutation.
- Login mutation.
- Install gate component.
- Reuse translated API error helpers and form-error components.

## Testing Matrix

Backend:

- Setup status by install state.
- Owner bootstrap success and rejection paths.
- Login and logout behavior.
- Session revocation and CSRF enforcement.
- Recovery-command backup and reset behavior.

Frontend:

- Install gate state switching.
- Owner setup form states.
- Login form states.
- Shared translated API error rendering.

E2E:

- Fresh database bootstrap path.
- Existing owner login path.
- Recovery-required warning path.

## Build Order

1. Derived install state.
2. Login and session APIs.
3. CSRF baseline.
4. Minimal setup and login UI.
5. Operator recovery command.
6. Future setup-step slices.