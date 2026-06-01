Items 9–12: Design Suggestions
9 — Session lifetime is hardcoded (30 days)
Add SESSION_LIFETIME_HOURS to config.go (default 720, must be > 0). The AuthService should receive it as a field and pass it to sessionExpiresAt(). This lets operators tighten sessions in higher-security environments without a recompile. Document the default and constraints in conventions.md under the new auth section.

10 — CSRF token doesn't rotate
Acceptable for now. The threat is mitigated by SameSite=Strict + same-origin constraints. True rotation requires either server-side nonce state (doubles session lookups) or a short-lived double-submit cookie separate from the session. Neither is justified until cross-origin requests are possible. Document this as a deliberate design choice in 0002-http-security-policy.md so future contributors don't reopen it without justification.

11 — passwordNeedsRehash silently swallows errors
Thread the *slog.Logger into AuthService and emit a WarnContext log when parsePasswordHash fails inside passwordNeedsRehash. The function must still return false on error (safe-fail — don't rehash what you can't parse), but silently discarding parse errors makes operational debugging hard. The logger is already passed to NewAPIServer; routing it to NewAuthService is a single-line change.

12 — Pre-domain readiness gaps
Three gaps to address before starting domain feature slices:

Money representation limits not yet decided — shopspring/decimal is named in conventions but the wire format (integer + scale + commodity code) hasn't been formalized into a schema convention or an OpenAPI type. An ADR or schema example should lock this down before the first account/transaction migration lands.

OpenAPI code generation command undocumented — openapi.yaml is declared the source of truth, but the command to regenerate frontend types (openapi-fetch + openapi-typescript) is not in developer-workflow.md. Add it so contributors know what to run after schema changes.

Domain lifecycle status taxonomy missing — Account, transaction, and posting records will each need an archived/void/draft/posted status. Defining the allowed states and valid transitions now (as a short ADR or conventions section) prevents incompatible patterns from landing in separate feature slices.


Issues to fix or track
setDefaultCurrency drops the actor — consider passing the owner ID through SetDefaultCurrencyInput and recording it in a books_audit_log or adding updated_by_user_id to books, or at minimum documenting this gap. Not a blocker but inconsistent with the other mutation handlers.
BookID constant — add a brief comment explaining it's a single-book simplification to aid future readers.
is_builtin semantic drift — consider renaming to is_catalog_sourced or documenting the intended distinction between catalog-sourced and truly system-pre-loaded currencies.