# V1 Scope

V1 ships as a SQLite-only self-hosted web app in a single Docker container.

## In Scope

- Personal and small-business bookkeeping on the seeded book.
- Accounts, transactions, categories, tags, payees, notes, saved views, reports,
  reconciliation, imports/exports, investments, pricing, planning, auth, MFA,
  and admin runtime checks.
- SQLite database stored in the `rekenraam_data` volume.
- Online SQLite backup and restore smoke validation.
- Optional HTTPS proxy overlay.

## Out of Scope For V1

- Additional-book creation UX and associated access-management flows.
- Plugin/theme runtime endpoints. Plugin/theme runtime deferred: no
  `/api/v1/plugins/*` or `/api/v1/themes/*` endpoints in b1. Future extension
  work requires semantic css tokens, WebAssembly, sidecar isolation,
  manifest-declared capabilities, admin review, disabled/failed-plugin
  isolation, deterministic fallback, and no arbitrary remote CSS loading.
- Arbitrary remote CSS loading.
- Hosted service operations.
