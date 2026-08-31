# Documentation/code reconciliation — 2026-08-31

Reviewed against code at `4e24da16`. This is a documentation-only review:
runtime behavior, product requirements, accepted ADRs, and roadmap priorities
are unchanged. Dated reviews and archived experiments remain historical input.

## Corrections

| Area | Verified evidence | Documentation correction |
|---|---|---|
| Current sequence | `backend/internal/app/recurring_generation.go`, recurring routes in `backend/internal/api/health.go`, scheduler wiring in `backend/cmd/rekenraam/command.go` | R9 slices 1–3 implemented; due inbox/review actions next. Production scheduler and public run-now remain inactive until slice 5. |
| Recurring review | `backend/internal/app/transactions_reconciliation.go`, `backend/internal/app/transactions_types.go` | Draft-edit preview is not promotion preview. Slice 4 needs explicit posting impact and blocked retry; it does not activate public generation. |
| CSV import and rules | `backend/internal/app/import_csv.go`, `backend/internal/app/import_service.go`, `backend/internal/api/imports.go`, `e2e/playwright/csv-import.spec.ts` | R5 is shipped, not unstarted. The feature ledger's trailing summary and comparison column now agree with its detailed rows. |
| Reporting currency | `backend/internal/app/valuation.go`, `backend/internal/app/reports.go`, `frontend/src/lib/reports/` | Conversion shipped 2026-08-26. Rate dates, seven-day default window, coverage gaps, and retained exact totals are implemented, not unresolved proposals. Snapshots and R18 projections remain future work. |
| Investment integrity | `backend/migrations/0004_investment_integrity.sql`, `backend/internal/app/transactions_write.go`, investment and self-check tests | R12a is complete. T-76 is a later pre-freeze requirement; T-75b remains investment-native lifecycle work in R16. The old R12 banner no longer calls all three immediate blockers. |
| Schema | Six SQL migrations in `backend/migrations/` | The August baseline consolidation is historical; additive migrations now exist. Removed references to retired `0008`–`0011` migrations from the current feature ledger. |
| Localization | Direct key and placeholder comparison of all twelve catalog files | 1,358 English keys; each other locale misses 65 and retains one obsolete key. Shared-key placeholders match. Translation completeness and native review are no longer conflated. |
| Test plan | CSV parser/API/browser cases and recurring service/API cases | CSV shipped without the proposed shared parser golden harness (6a remains open). Draft-workflow coverage (6d) is implemented; provider/analytics harnesses still await consumers. |
| Workflow and navigation | Package scripts, wrapper scripts, route tree | Corrected README example routes and renumbered backlog references. Existing developer commands remain valid. |
| Later plans | Roadmap decisions and absence of corresponding adapters/UI | R17 quote providers and R14 attachments are planned, not shipped. R15 sequence remains IBKR → GoCardless → event producer; quotes belong to R17. |

## Actual gaps retained

- **T-79:** caller-supplied request IDs replace server-generated IDs, contrary
  to existing requirements. Recorded without changing the rule or code.
- **T-80:** missing translations in all five non-English locales. English
  fallback is functional coverage, not translation completion.
- The parser golden harness, backend localization, native translation review,
  recurring review UI, T-76, and T-75b remain incomplete; this review does not
  mark them delivered or silently reschedule them.

## Verification and limits

- Checked referenced routes, services, migration filenames, scripts, and named
  tests against the repository; validated local Markdown links and changed-file
  whitespace.
- Recomputed catalog key counts and shared-key placeholder parity for every
  locale directly from JSON. Counts exclude `$schema` and do not treat the
  obsolete key as translated coverage.
- No application tests were rerun for these documentation-only changes. The
  preceding implementation commit records backend race, frontend, and build
  validation; those results are not represented as a new run here.
- Competitor columns, prices, and external-provider details remain dated
  research. Only Rekenraam's code/status claims were refreshed; no market or
  provider API re-verification is claimed.
- This is a reconciliation of active summaries and plans, not a new exhaustive
  financial/security audit or a certification of every historical design claim.
