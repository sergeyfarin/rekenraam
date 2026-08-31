# R9 acceptance review — 2026-08-31

R9 is accepted: all six slices of `docs/plans/recurring-transactions-plan.md`
are complete. Templates produce reviewable drafts; only explicit posting makes
them part of the ledger. R10 projected-balances planning is next. No forecasting
implementation or deferred recurring feature is included in this review.

Reviewed the implementation at `00e36082` plus the acceptance fixes recorded
below against the product requirements, conventions, accepted ADRs, and R9
plan. This is code and automated workflow acceptance, not a claim of long-term
owner usage or native-language review.

## Commitment audit

| Commitment | Result and evidence |
|---|---|
| D1: drafts only, explicit post/edit/discard | Accepted. Generation supplies scheduled origin and draft status. Public manual creation rejects drafts (`TestBrowserApiCannotCreateADraft`). Browser creation → generation → edit → post and archived mobile discard exercise the real routes. |
| D2: no history on creation; downtime catch-up | Accepted after T-82. Creation uses the later of the owner's local today and the anchor. `TestCreatingATemplateWithAPastStartDateBackfillsNothing`, the 64-date catch-up test, and the new >4,000-date regression cover both sides. Fifty materializations per tick; bounded enumeration; no lost dates. |
| D3: computed future, durable acted-on identities | Accepted. `internal/recur` is independent of the database and clock; only generated, skipped or blocked dates get occurrence rows. Preview makes no writes. The uniqueness constraint and transactional generation preserve one identity. |
| D4: phase, clamping and series limits | Accepted. Named tests cover the 31st, leap day, last day, intervals, the first matching weekday, an anchor after the nominal day, inclusive windows, maximum occurrences counted from the anchor, and end-date limits. Calendar validation rejects year zero and stops at year 9999. |
| D5/D6: calendar dates and fixed exact amounts | Accepted. No holiday shifting or amount estimation. Per-commodity exact balancing and scale validation reject invalid templates. Draft editing permits correction before posting. Currency clearing legs remain supported without permitting security holdings. |
| D7/D8: local scheduler and owner time zone | Accepted. Startup and minute ticks are active; materialization uses local SQLite transactions, not network work. Owner-local dates and UTC attribution are tested, including a date on the other side of UTC midnight. The enumerator also has a DST-date regression. No per-template time zone. |
| Template schema, CRUD and audit history | Accepted. Four recurring tables retain book/template references, versions, postings and occurrence identity. Create/read/list/PATCH/archive use authenticated routes and service validation. Nullable versus omitted PATCH fields, exact posting details, tags, enabled/archive state and revisions are tested. Template saves do not alter the ledger. |
| Schedule edits and concurrency | Accepted. Schedule-field updates reset the generation watermark; existing occurrence identities survive. Posting-only edits leave schedule fields omitted. Materialization and advancement check the captured revision under the writer lock. Tests use independent database pools and cover stale template edits, retries and generation races. |
| Atomic generation, blocking and retry | Accepted. Draft, occurrence and audit commit together; injected failures roll them all back. Validation failures create blocked occurrences once and do not stop valid templates. Infrastructure failure leaves dates retryable. Explicit retry revalidates the current template and uses prior-attempt/revision guards. There is no repeated financial-content log. |
| Draft lifecycle and reconciliation | Accepted. Generation and draft editing do not invalidate checkpoints. Promotion preview validates the saved draft and uses stored same-day positions; posting rechecks and requires override. `TestPostDraftTransactionIntoReconciledPeriodRequiresOverride` and `TestRecurringPostPreviewUsesStoredSameDayPositionsAndNamesCheckpoints` fulfill the plan's checkpoint commitment. |
| Discard, skip and archive | Accepted. Never-posted draft deletion atomically retains an audited skipped identity, including after multiple edits (T-81). Injected delete failures roll everything back. Skip requires a reason and suppresses generation. Archive leaves outstanding drafts reachable; archived blocked items can be skipped but cannot be retried. |
| Report, export and FX isolation | Accepted after T-83 and added evidence. `TestGeneratedDraftIsExcludedFromLedgerReportsAndExport` uses the real producer and existing posted activity: net worth, spending, cashflow, ledger CSV and QIF remain unchanged until posting, then change. `TestGeneratedDraftDoesNotExtendFXCoverageUntilPosted` covers both planned coverage and queued work. |
| Composed review API | Accepted. One composed query per due page; exact debit/credit amounts remain separate per commodity and reflect current draft edits. Cursor tests cover multiple pages, edited/discarded items and archived templates. Preview merges the schedule with durable identities; summary totals do not depend on pagination. |
| Screens, shared editor and review safety | Accepted after T-84. Templates and due inbox expose loading/empty/error/success states, create/edit/pause/resume/archive, scheduled skip, retry, edit/post/discard and bulk post. The shared editor preserves exact postings and tags. New browser coverage verifies payee typing does not reset other edits or posting details. Partial bulk failure keeps successful posts and only offers remaining drafts again. Changed checkpoint impact stops posting and requests a fresh review. |
| Accessibility, mobile and localization | Accepted within existing project limits. Browser checks cover keyboard/native dialogs, narrow mobile layout, dark theme and automated accessibility. R9 strings are present in all six catalogs. Native-language review remains outstanding, as documented; unrelated missing CSV/MFA messages remain T-80. |
| Activation and API contract | Accepted. Review screens shipped before startup scheduling and public run-now were enabled. Mutations require auth/CSRF; read-only unsaved preview requires auth and has no side effects. OpenAPI and generated client types agree. Browser acceptance runs against the single production binary and isolated SQLite database. |

## Findings resolved in this review

- **T-82:** the 50-write limit did not bound enumeration. An outage with more
  than 4,000 daily dates failed permanently. Generation now enumerates at most
  4,000 inclusive calendar days and advances only completed windows. The named
  regression first failed with `ErrWindowTooWide`; it now proves the oldest
  100 dates are produced across two ticks without advancing too soon.
- **T-83:** the FX outbox trigger was posted-only, but refresh planning included
  drafts. ADR 0010 and the requirements also retained a contrary speculative
  policy. The planner is now posted-only and ADR 0010 has an explicit dated
  amendment: R9's per-currency review needs no early FX download. The real EUR
  draft regression first exposed the extra coverage and now verifies demand
  and work start at posting. Active-account demand is unchanged.
- **T-84:** the shared editor's template initialization effect read reactive
  payee text, so typing reset the saved snapshot over other edits. Both initial
  payee values now come directly from input props. The browser regression
  exposed the reset and now verifies editing and saving preserve the form.

The promised report/export isolation test was previously missing; the older
export test's name mentioned drafts without creating one. This review adds the
real-producer integration proof rather than counting a transactions-list test
as report/export coverage. Plan test names now match the actual named tests;
the blocked-occurrence evidence is an audit event, not a promised log line.

## Deferred scope and owner defaults

| Item | Include in accepted R9? | Reason |
|---|---|---|
| Automatic posting | No | Explicit review remains the safety boundary. There is no usage evidence justifying a change. |
| Weekend/holiday movement | No | No banking-calendar policy or provider exists; users can correct the draft date. |
| Variable/estimated template amounts | No | Fixed postings plus review are sufficient for v1. Forecast estimate semantics belong in R10. |
| Loan amortization | No | A changing principal/interest series needs its own model; optional R10 follow-up. |
| Reminders/notifications | No | The app has no notification channel. The inbox and nav count provide discovery. |
| Recurring investments | No | Lot, price and fee effects require a separately designed atomic producer lifecycle. T-75b/T-76 remain separate work. |
| RRULE import/export | No | The supported calendar subset covers v1; accepting another format is not needed. |
| Per-template time zones | No | The owner's zone is the existing scheduling policy; per-template controls add unsupported semantics. |
| Five-day default lead | Yes | Keeps near-term items reviewable; configurable from zero through 90 days. |
| Navigation badge | Yes | Counts generated drafts only. Blocked items have a separate inbox count and remain visible. |
| User-visible run-now | Yes | “Generate due drafts” lets the owner check a schedule immediately; it never posts. |

## Validation and limits

Final validation passed:

- `./scripts/test-backend.sh`: formatting, vet and all backend race tests.
- `./scripts/test-frontend.sh`: zero type errors/warnings; 355 unit tests in
  21 files.
- `pnpm --dir e2e test recurring.spec.ts transactions.spec.ts accessibility.spec.ts`:
  production build and 19 browser cases (two bootstrap, seven recurring, two
  shared transaction-entry and eight accessibility cases).
- `git diff --check`: no whitespace errors; generated OpenAPI types unchanged.
- All 90 `recurring_*` messages and their placeholder sets match across six
  locale catalogs; referenced review documents exist.

Initial frontend and browser checks overlapped translation regeneration,
causing missing generated-file warnings and a failed bootstrap; they were
rerun sequentially. That failed attempt is not acceptance evidence.

Known unrelated debt remains explicitly open: T-79 request IDs, T-80 catalog
parity, T-76 disposal provenance and T-75b investment correction. No claim is
made that this review closes the whole backlog, validates deployment against
real financial data, or proves usability through longitudinal user research.

The next step is an R10 plan defining projection dates, per-currency totals,
and treatment of posted transactions, generated drafts and computed future
occurrences without double counting. Converted totals need explicit FX
semantics. R9's pure enumerator and occurrence identities are available for
that plan; forecasts are not shipped yet.
