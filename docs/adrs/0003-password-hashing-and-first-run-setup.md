# ADR 0003: Password Hashing And First-Run Setup

## Status

Accepted

## Date

2026-05-30

## Context

Rekenraam needs local authentication before real financial data entry ships. It also needs an initial setup experience that creates enough durable accounting structure for a new user to start safely, without forcing every setup-related feature into the first auth slice.

Current password-storage guidance favors Argon2id for new systems because it is memory-hard and resists both GPU-based and side-channel attacks better than older default choices.

## Decision

Password hashing uses Argon2id, not bcrypt.

Implementation rules:

1. Use `golang.org/x/crypto/argon2` unless a later ADR selects a higher-level maintained library.
2. Store password hashes in a self-describing format such as PHC string format, including algorithm, memory, iterations, parallelism, salt, and hash.
3. Start from the OWASP minimum Argon2id profile: 19 MiB memory, 2 iterations, and parallelism 1.
4. Generate a unique cryptographically random salt per password.
5. Store no plaintext passwords and no reversible password encryption.
6. Keep parameters upgradeable so future logins can rehash with stronger settings when needed.
7. Owner setup and local owner recovery require passwords between 12 Unicode characters and 1024 bytes.

First-run setup is a guided workflow whose complete target state is:

1. Create the single owner user.
2. Create the default book.
3. Choose the default currency preference.
4. Optionally choose additional currencies.
5. Create required system accounts.
6. Choose default categories.
7. Optionally choose additional categories.

This workflow is implemented incrementally:

1. The first auth slice creates only the owner user.
2. Book, default-currency-preference, additional-currency, and system-account setup are added when books, commodities, and accounts are implemented.
3. Default and additional category selection is added when category functionality is implemented.
4. The setup state must be resumable so partially completed deployments can continue through later setup steps after upgrades.

## Consequences

### Positive

- Argon2id is a stronger default for a new password-auth system than bcrypt.
- Self-describing hashes make parameter changes and future migrations practical.
- First-run setup can grow into a complete guided onboarding flow without blocking early owner-auth implementation.
- Default books, currencies, system accounts, and categories are introduced by the feature slices that own their domain rules.

### Negative

- Argon2id requires explicit parameter handling and careful memory-cost tuning.
- Tests need deterministic boundaries around random salt generation and hash verification.
- Setup state needs a small amount of workflow modeling rather than a single boolean flag.

### Follow-Up

- Revisit password reset and failed-login throttling parameters before auth is exposed beyond localhost development.
- Define the exact system account roles before account setup lands.
- Define the initial default category set before category setup lands.
