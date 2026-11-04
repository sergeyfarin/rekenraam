# ADR 0003: Password Hashing And First-Run Setup

## Status

Accepted

## Date

2026-05-30; amended 2026-11-04 for bounded verification parameters (#504).

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
8. Treat parameters read from a stored hash as bounded input before verification (see the 2026-11-04 amendment).

First-run setup is a guided workflow whose complete target state is:

1. Create the single owner user.
2. Create the default book.
3. Choose the default currency preference.
4. Additional currencies become available when accounts using them are created.
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

## Amendment — bounded verification parameters, 2026-11-04

Stored hashes are self-describing (rule 2), so verification would otherwise
spend whatever memory and time a malformed or tampered row requests. Before
deriving a key, verification rejects a stored hash unless:

- memory is 1 KiB–256 MiB (the total argon2 allocation, not per lane);
- iterations are 1–8 and parallelism is 1–16;
- the decoded salt is 8–64 bytes and the decoded hash is 16–64 bytes.

Memory × iterations is then at most about 54× the configured 19 MiB × 2
profile, which bounds one login attempt on a small self-hosted container while
still admitting the RFC 9106 second recommended profile (64 MiB, t=3, p=4) and
the OWASP Argon2id profiles for future rehash upgrades (rule 6). Adopting a
profile beyond these caps requires amending this ADR and the caps together.
