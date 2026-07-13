# Resolved Backlog Record — 2026-07

This is the historical record removed from the live `docs/backlog.md` during the
2026-07-12 documentation consolidation. The detailed original writeups are
preserved verbatim in `docs/reviews/backlog-pre-consolidation-2026-07-12.md`.
`docs/implemented.md`, tests, and the linked design records are the source for
current behaviour.

## Resolved

- **Security and operations:** S-01 setup ownership claim; S-02 slow-client
  protection; S-03 trusted forwarded-client parsing; S-05 external TLS posture;
  S-08 SQLite/WAL/SHM permissions; T-01 configurable session lifetime; T-18
  expired-session cleanup; T-19 secret-key loss/rotation guidance.
- **Import correctness and contracts:** T-05 pagination consumption; T-06
  atomic generic import commit; T-07 import OpenAPI coverage; T-08 encrypted
  connection secrets; T-11 real Trading 212 key probe; T-12 retained connection
  provenance; T-14 paginated-history continuation; T-15 retry re-staging signal;
  T-16 atomic start guard; T-17 same-timestamp cursor safety; T-21 verified
  provider endpoint/enums; T-22 import entry-kind validation; T-23 complete
  preview pagination; T-26 atomic investment-import commit; T-28 intraday fill
  ordering; T-29 unused routing-setup compensation; T-30/T-33 settlement-account
  eligibility; T-31 provider provenance; T-32 concurrent commit protection.
- **Product validation and UI coverage:** T-20 critical E2E expansion; T-24
  reconciliation guard for investment postings; T-25 pricing-history results
  signal; B-T212-SCHED scheduled refresh.

## Deliberate non-work

- T-02 single runtime book ID, T-03 CSRF-token rotation, and T-04 CSP
  `unsafe-inline` are recorded design choices with their rationale in the prior
  backlog history and governing security/architecture documents.
- T-13 file-import-wrapper service coverage and B-T212-INVST linking an existing
  manually tracked holding account are deferred design follow-ups, not current
  defects. Revisit only when their respective product work is scheduled.

## Moved product decisions

I-03 (multi-method analytical gains) and I-04 (posting realized gains) are not
technical debt. They are deferred accounting/reporting product decisions and now
live in the open decisions section of `docs/roadmap.md`.
