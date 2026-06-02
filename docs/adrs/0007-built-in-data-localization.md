# ADR 0007: Built-In Data Localization

## Status

Accepted

## Date

2026-06-02

## Context

Rekenraam seeds and offers app-defined records such as currencies, commodities,
account types, system accounts, and default categories. These records need
localized labels in the UI, but they also become part of long-lived financial
data, setup choices, imports, exports, reports, and audit trails.

Storing localized built-in names as canonical database values would make the
database depend on the locale active during setup. A later locale change,
translation fix, or label wording change would leave old rows with stale or
mixed-language built-in names.

## Decision

Built-in app-defined records use stable codes or keys as their durable
identity. The frontend resolves those stable identifiers to localized labels at
render time through Paraglide message functions, `Intl` formatting APIs, or
another approved frontend localization boundary.

The database and API must not require translated built-in names as canonical
input. Built-in setup requests should submit stable identifiers, such as
currency codes or category keys. API responses should expose stable identifiers
that the frontend can translate. English fallback labels may exist only as
non-canonical fallback metadata for development, diagnostics, exports before
backend localization exists, or clients that cannot localize.

User-created names are different: user-entered category, account, institution,
payee, and similar names are data and should be stored and displayed as entered
unless the user renames them.

## Consequences

- A user can change UI locale without rewriting built-in database rows.
- Translation improvements can update built-in labels without data migrations.
- Two installs in different locales keep the same durable identifiers for the
  same built-in records.
- Frontend code must provide localized labels for every built-in key it renders.
- Tests or generation scripts should check that backend built-in keys and
  frontend localization keys do not drift once seeded categories and system
  accounts are implemented.
