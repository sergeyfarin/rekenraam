# Recurring Transactions Plan (R9)

Status: **planned, not started.** Written 2026-08-29, immediately after R5's
ordinary-bank CSV import closed. This is the implementation reference for the
roadmap slice "R9 — recurring transactions", the first leg of the planning loop
`R9 → R10 → R8` decided 2026-08-05 (roadmap review §3d). The roadmap keeps its
one-paragraph summary and points here.

Governed by `docs/conventions.md` (the schedule, draft, and lifecycle rules
quoted below are binding, not restated for flavour), sequenced by
`docs/roadmap.md`, and bound by the ledger invariants in
`docs/plans/transaction-ledger-core-plan.md` and the `ledger-invariants` skill.

R9 exists to deliver one sentence end to end:

> *the entries you make every month make themselves, and you still see each one
> before it counts.*

Both halves are load-bearing. The first is the daily-driver value — rent,
salary, subscriptions, standing transfers. The second is why R9 is allowed to
touch the ledger at all: recurring generation is the app's **first machine
producer of financial records**, and the conventions already say what such a
producer owes the user.

## What R9 inherits

Three pieces of the design were decided before this plan and are inputs, not
open questions.

**`draft` is reserved for exactly this.** `docs/conventions.md`: *"`draft` is a
reserved persisted lifecycle status … system-only, not a user-facing maturity
step … Future machine/async producers such as import commit awaiting review,
scheduled generation, or explicit crash-recovery autosave may activate it and
must own a dedicated review/discard surface."* R9 is the "scheduled generation"
named there. The review surface is therefore a deliverable of this slice, not
follow-up polish.

**The API-layer draft guard is R9's to close.**
`docs/plans/transaction-ledger-core-plan.md` (Transactions API):
*"Add an origin-based guard (reject `status="draft"` unless `OriginType !=
"browser_api"`) when R9 or import-review lands a real producer; until then this
is a deliberately deferred, low-severity gap."* R9 lands the real producer, so
R9 closes the gap. See slice 3.

**Schedules are local wall clock plus an IANA zone.**
`docs/conventions.md` § Data: *"Store recurring user-facing schedules as local
wall-clock time plus an IANA time zone, then record each actual run/attempt as
UTC timestamps."* `user_preferences.time_zone` already exists and is already the
zone both `app/pricing_scheduler.go` and `app/backup_scheduler.go` read. R9 uses
the same one and adds no per-template zone — see the decision below.

## Decisions

Each of these is a decision with a named alternative, not a default that fell
out of the first draft. They are binding for v1; the acceptance review at the
end is where they get re-examined against what shipping taught.

### D1 — Generated entries are drafts. There is no auto-post in v1.

A due occurrence materializes as a `draft` transaction that the user posts,
edits then posts, or discards.

The alternative — a per-template `auto_post` mode, which most competitors offer
— is deliberately not built. The reason is not caution about the feature but the
shape of this ledger: a posted transaction participates in reports, in
reconciliation, and in the export's trial balance. A rent template with a stale
amount that auto-posts for four months corrupts four reconciliations silently,
and the user's first signal is a checkpoint that will not balance. A draft that
the user posts with one click costs one click and cannot do that. The typing —
which is the actual complaint recurring transactions answer — is gone either
way.

The cost is honest and recorded: someone who wants a truly hands-off ledger does
not get one in v1. If the acceptance review finds the due inbox is being cleared
without being read, auto-post is the right answer and the model already supports
it (an occurrence knows what it produced). No column is shipped for it now — a
CHECK-constrained column with one legal value is a promise the code has not
made.

### D2 — Creating a template never backfills history.

A template's `starts_on` is the **phase anchor** for the recurrence, not an
instruction to create the past. Generation covers `[watermark, today +
lead_days]`, where the watermark is set once at template creation to
`max(starts_on, today)`.

This is what makes catch-up bounded. A user who enters "rent, monthly on the 1st,
starting 2019-01-01" so the schedule lands on the right day of the month gets
one upcoming draft, not eighty-eight historical ones. Downtime catch-up still
works, because the watermark is behind and today has moved: a server off for a
week generates the week it missed.

The alternative — backfilling from `starts_on` — has no bounded worst case and
manufactures ledger history the user never saw, which every other rule in this
codebase refuses to do.

### D3 — Occurrences are enumerated by a pure function; only materialized ones get rows.

`internal/recur` holds `Occurrences(spec ScheduleSpec, from, to string)
([]string, error)` over ISO `YYYY-MM-DD` dates — no database, no clock, no
book. `recurring_occurrences` rows
exist only for occurrences that were *acted on*: generated, skipped, or blocked.
The future is computed, never stored.

This is the R10 contract. Projected balances need the next twelve months of a
schedule, and they must not need twelve months of speculative rows kept in sync
with an edited template. One pure function, two consumers, one set of
month-clamping tests.

### D4 — Monthly recurrence clamps to month end and re-anchors from the nominal day.

`day_of_month` is 1–31, plus a separate `last_day_of_month` flag for "the last
day, whatever it is". A nominal day past the end of a short month clamps:
`min(day_of_month, days_in_month)`.

The bug this exists to prevent is drift. A schedule of "the 31st" that computes
each date from the *previous produced date* becomes the 28th forever after one
February. Enumeration therefore walks the anchor month by month and clamps at
render, so 31 Jan → 28 Feb → 31 Mar. Same rule for yearly on 29 February. This
is a named test in slice 1, not a comment.

### D5 — No weekend or holiday shifting.

A due date that lands on a Sunday stays on Sunday. Per-country banking-holiday
calendars are a maintenance treadmill of exactly the kind
`product-requirements.md` rejects elsewhere, and the app cannot know whether a
given direct debit is pulled early, late, or on the day. The user sees the draft
before it posts and can move the date; the bank statement is what settles the
question, through import and reconciliation.

### D6 — Fixed amounts. Variability is answered at review, not by the template.

Template postings carry an exact coefficient and scale like any posting. A
variable bill (electricity, a card payment) is entered as its usual amount and
corrected in the draft when the real figure is known — which is the same edit
the user would make anyway, minus retyping the accounts, payee, and category.

No "estimated" flag, no "same as last time", no amount ranges. R10 may want an
estimate marker for projections; if so it belongs to R10's own model, where the
question is "what will the balance be", not to the template, where the question
is "what should this entry say".

### D7 — Generation runs on the scheduler tick, not through the background work queue.

ADR 0010's queue exists for restart-sensitive, network-bound, retryable work:
provider downloads, FX backfill, online import fetches. Materializing a due
occurrence is a single local SQLite transaction with no network access and no
partial state. Its retry is the next tick, one minute later, and its idempotency
comes from `UNIQUE (template_id, occurrence_date)` rather than from queue
semantics.

The queue's real contribution — a bounded attempt count with a way back — is
kept without the queue: an occurrence that cannot be generated (its account was
archived, its commodity retired, its category deleted) is written as a `blocked`
row with an error summary, which stops the retry, surfaces on the template and
in the due inbox, and can be retried explicitly once the cause is fixed. That
is the T-39 lesson (retry forever with no cap) and its resolution (a cap is only
safe with a way back), applied without borrowing machinery that would otherwise
be dead weight.

### D8 — One book-level time zone, from `user_preferences`.

Templates carry no zone of their own. The zone answers one question only: *which
local date is it now*, so the generator knows what "due" means. Per-template
zones would be a real feature for someone whose rent is due in Amsterdam and
whose salary lands in Singapore — but the due *date* is a calendar date either
way, and only the moment of materialization moves. Deferred with the reason
stated; revisit if the expat persona asks for it.

## Data model

Four tables, all following the established shapes: `import_rules` for the
book-scoped configuration table with same-book triggers, `backup_runs` for
occurrence identity, `posting_versions` for the exact-amount columns.

### `recurring_templates`

```
id, book_id
name                 TEXT, trimmed, non-empty
enabled              INTEGER 0/1
archived_at          TEXT NULL          -- templates are archived, never deleted
transaction_kind     TEXT CHECK IN ('ordinary','transfer')
payee_id             INTEGER NULL REFERENCES payees
payee_name           TEXT NULL
description          TEXT NOT NULL DEFAULT ''
note_markdown        TEXT NOT NULL DEFAULT ''
frequency            TEXT CHECK IN ('daily','weekly','monthly','yearly')
interval_count       INTEGER CHECK (>= 1)
by_weekday           INTEGER NULL CHECK (0..6)      -- weekly
day_of_month         INTEGER NULL CHECK (1..31)     -- monthly, yearly
last_day_of_month    INTEGER NOT NULL DEFAULT 0     -- monthly, yearly
month_of_year        INTEGER NULL CHECK (1..12)     -- yearly
starts_on            TEXT  GLOB '????-??-??'
ends_on              TEXT NULL GLOB '????-??-??'
max_occurrences      INTEGER NULL CHECK (>= 1)
lead_days            INTEGER NOT NULL DEFAULT 5 CHECK (0..90)
generate_from        TEXT  GLOB '????-??-??'        -- the D2 watermark
created_at, updated_at, created_by_user_id, updated_by_user_id
```

`ends_on` and `max_occurrences` are both nullable and may both be set; whichever
ends the series first wins, and the enumeration decides that, not the schema.
A CHECK enforces the per-frequency field requirements (weekly needs
`by_weekday`; monthly needs `day_of_month` xor `last_day_of_month`; yearly needs
`month_of_year` and one of the two day rules) so an unusable template cannot
reach the generator.

### `recurring_template_postings`

Shaped like `posting_versions`, minus everything that only a real posting has
(no reconciliation status, no cleared date, no journal entry):

```
id, template_id, book_id, line_seq, line_key
account_id       REFERENCES accounts
quantity_value   TEXT  (exact coefficient, never a float)
quantity_scale   INTEGER 0..24
commodity_id     REFERENCES commodities
memo             TEXT NOT NULL DEFAULT ''
UNIQUE (template_id, line_seq)
```

Same-book triggers on `account_id` and `commodity_id`, matching
`import_rules_targets_same_book_insert`. Balance is **not** enforced by a
trigger: it is checked in the service on save and again at generation, because
the reason a template does not balance is a message the user needs, not an
`ABORT` string. `recurring_template_tags` mirrors `import_rule_tags` exactly.

### `recurring_occurrences`

```
id, book_id, template_id REFERENCES recurring_templates
occurrence_date  TEXT GLOB '????-??-??'
status           TEXT CHECK IN ('generated','skipped','blocked')
transaction_id   INTEGER NULL REFERENCES transactions
error_summary    TEXT NOT NULL DEFAULT ''
skip_reason      TEXT NOT NULL DEFAULT ''
materialized_at  TEXT              -- UTC, per the conventions' run/attempt rule
created_at, updated_at
UNIQUE (template_id, occurrence_date)
```

The unique constraint is the whole idempotency story, the same way
`backup_runs.occurrence_key` is: two schedulers, a restart mid-tick, or a clock
that steps backwards all converge on one row per template per date.

`status` has no `pending`: a row is written at the moment it is acted on.
`skipped` rows may be written *ahead* of the due date — that is how "not this
month" works, and it is also what R10 reads to leave a skipped occurrence out of
a projection.

Why a table rather than a JSON blob on the template: T-54's lesson, applied
before it bites. "Which occurrence failed, why, and since when" is a query.

## Enumeration — `internal/recur`

```go
type ScheduleSpec struct {
    Frequency      Frequency // daily | weekly | monthly | yearly
    IntervalCount  int
    ByWeekday      *time.Weekday
    DayOfMonth     int
    LastDayOfMonth bool
    MonthOfYear    int
    StartsOn       string // ISO YYYY-MM-DD
    EndsOn         string // ISO YYYY-MM-DD, empty for open-ended
    MaxOccurrences int    // 0 for unlimited
}

func Occurrences(spec ScheduleSpec, from, to string) ([]string, error)
func Next(spec ScheduleSpec, after string) (string, bool, error)
```

Dates are ISO `YYYY-MM-DD` strings on the boundary, because that is what every
other calendar date in this codebase is — `time.DateOnly` parsed at the edge,
`GLOB '????-??-??'` in the schema, ISO on the wire. Internally the arithmetic
runs on `time.Time` pinned to UTC, which is safe precisely because nothing here
is a wall-clock instant.

Pure. No `time.Now`, no context, no database. `MaxOccurrences` counts from
`StartsOn` — the anchor, not the window — so asking for a mid-series window
returns the right dates and the right end.

Named tests (slice 1):

- `TestMonthlyOnThe31stClampsAndReanchors` — Jan 31 → Feb 28 → Mar 31, and the
  same series in a leap year gives Feb 29.
- `TestYearlyOnLeapDayClampsInCommonYears`.
- `TestIntervalGreaterThanOneKeepsPhaseAcrossAClampedMonth` — every-2-months
  from Jan 31 stays on the odd months, not shifted by the February clamp.
- `TestWeeklyStartsOnTheFirstMatchingWeekdayOnOrAfterStart`.
- `TestMaxOccurrencesCountsFromTheAnchorNotTheWindow`.
- `TestEndsOnAndMaxOccurrencesTakeWhicheverEndsFirst`.
- `TestWindowIsInclusiveOfBothBounds`.
- `TestDailyAcrossADaylightSavingBoundaryProducesEveryDate` — a date-only series
  has no 23- or 25-hour day; the test exists to pin that the implementation
  never routes through wall-clock arithmetic.

## Generation

A once-a-minute ticker on `RecurringService`, started from `command.go`
alongside the pricing, import, and backup schedulers, following
`app/backup_scheduler.go`:

1. Read the owner's `user_preferences.time_zone`; `localToday` is today in it.
2. For each enabled, unarchived template: `window = [generate_from,
   localToday + lead_days]`, `due = recur.Occurrences(spec, window...)` minus
   the dates that already have an occurrence row.
3. For each due date, in one transaction: build the `TransactionSpec` from the
   template, create the transaction with `status="draft"`, `OriginType=
   "scheduled"`, and insert the `recurring_occurrences` row. One transaction, so
   a crash between the two is impossible — the T-14..T-17 lesson (guard, insert,
   and enqueue must be one repo-layer transaction) applies unchanged.
4. On a validation failure — archived account, closed account, retired
   commodity, unbalanced template — write a `blocked` row with the message and
   move on. No retry, no log line per minute.
5. Advance `generate_from` to `localToday + lead_days + 1` once every date in
   the window has a row — `generated` or `blocked` both count, because both are
   recorded. A date left unrecorded by a crash or a per-tick cap holds the
   watermark where it is, and the next tick picks it up.

The watermark and the occurrence rows are deliberate belt and braces. The rows
are the correctness mechanism; the watermark only keeps a tick from
re-enumerating a decade of history to discover it has nothing to do.

Per-tick materialization is capped (`maxOccurrencesPerTick`, 50) so a long
outage resolves over a few ticks instead of one long write transaction. Losing
the race on the unique constraint is a normal outcome, not an error — same
handling as `db.ErrBackupOccurrenceExists`.

**Drafts and FX coverage.** `fx_work_after_posting_version_insert` fires on
`posted` versions only, so a generated draft triggers no coverage work. That is
correct and is left alone: the due inbox shows each amount in its own commodity,
never a converted total, so there is nothing on that surface a missing rate
could make wrong. Coverage happens at post, as it does for every other
transaction.

**Reconciliation.** A draft is not in the ledger, so generating one can never
invalidate a checkpoint. *Posting* one runs the ordinary period-impact check —
a backdated occurrence inside an active checkpoint returns
reconciliation-override-required, and the inbox previews the named checkpoints
before the user confirms. No new machinery; the existing preview route answers
it.

## The draft origin guard

`POST /api/v1/transactions` currently accepts `status="draft"` from any caller.
With a real producer in place, it must reject `status="draft"` when
`OriginType == "browser_api"`, returning the standard error envelope with a new
`TRANSACTION_DRAFT_NOT_USER_CREATABLE` code in all six locales.

Two consequences to handle rather than discover:

- The backend suite uses the browser route to exercise the draft→post and
  draft→discard lifecycle. Those tests move onto the recurring producer (which
  is what they were always standing in for) or an explicitly internal origin;
  they do not get an exemption in the handler.
- `POST /transactions/{id}/post` and `DELETE /transactions/{id}` (never-posted
  draft hard delete, allowed by the conventions) both already exist and are
  unchanged. R9 adds no second way to promote or discard a draft.

## API surface

New paths under `/api/v1/recurring/`, each with its own file in
`api/openapi/paths/`:

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/recurring/templates` | list, with next-due date per template |
| `POST` | `/recurring/templates` | create |
| `GET` | `/recurring/templates/{id}` | read, with postings and tags |
| `PATCH` | `/recurring/templates/{id}` | update (PATCH omission semantics — see below) |
| `POST` | `/recurring/templates/{id}/archive` | archive; occurrences and their drafts survive |
| `GET` | `/recurring/templates/{id}/occurrences` | `from`/`to`; enumerated future merged with materialized rows |
| `POST` | `/recurring/templates/{id}/skip` | `{occurrence_date, reason}` — writes a `skipped` row, before or after the date |
| `POST` | `/recurring/templates/{id}/run-now` | materialize what is due now, the "back up now" analogue |
| `GET` | `/recurring/due` | the review inbox read model |

`GET /recurring/due` returns generated drafts that are still drafts, plus
blocked occurrences, each with template name, occurrence date, and the amounts
per commodity — one request, because the inbox is one screen.

**PATCH omission semantics.** This repo's recurring bug class (see
`import_connections_test.go`, and T-36/T-45/T-47 for the money variant) is a
plain `bool` or zero value in a PATCH struct overwriting a field the caller
never sent. Template PATCH uses pointer fields throughout, and the test for it
is named and required in slice 2: `TestUpdateRecurringTemplateLeavesOmittedFieldsAlone`.

Errors: `RECURRING_TEMPLATE_UNBALANCED`, `RECURRING_SCHEDULE_INVALID`,
`RECURRING_OCCURRENCE_ALREADY_MATERIALIZED`, `RECURRING_TEMPLATE_ARCHIVED`,
plus the `TRANSACTION_DRAFT_NOT_USER_CREATABLE` above. Six locales each.

## Frontend

`/app/recurring`, two views behind one nav entry:

- **Templates** — list with name, schedule in words ("Monthly on the 1st"),
  amount, next due, enabled toggle. Editor reuses the transaction entry form's
  posting rows, payee resolution (including the confirm-and-search path from
  T-50), category picker, and tag input; the schedule block is the only new UI.
  A live "next five occurrences" preview under the schedule block, fed by
  `GET /occurrences` — the fastest way to tell someone their monthly-on-the-31st
  rule does what they meant.
- **Due** — the review inbox. Each row: post, edit then post, skip, discard.
  Blocked rows show the reason and a retry. Bulk post for a checked set, because
  the first of the month produces several at once.

All four screen states on both views (loading, empty, error, populated), six
locales, and an `[acceptance]`-tagged browser case covering create → generate →
post, following the R3a pattern. Amounts render through `$lib/money/format.ts`;
any amount the user types goes through `$lib/money/amount.ts`. No third copy of
either.

## Slices

1. **`internal/recur` + schema + repository.** The pure enumerator with its
   eight named tests, the four tables in one migration (`0003_recurring.sql`),
   and the repository with same-book trigger coverage. No API, no scheduler.
   Ships alone because everything else depends on the date arithmetic being
   right, and the date arithmetic is testable with no ledger at all.
2. **Template CRUD.** Service, handlers, OpenAPI, typed frontend client, error
   codes in six locales, PATCH omission test. Balance validation on save.
3. **Generator, scheduler, and the origin guard.** Materialization in one
   transaction, blocked-occurrence handling, the per-tick cap, `run-now`, and
   the `status="draft"` rejection for `browser_api` with the existing draft
   lifecycle tests moved onto the real producer.
4. **Due inbox read model and review actions.** `GET /recurring/due`, skip, and
   the reconciliation-impact preview wired to the existing route.
5. **Frontend.** Both views, all states, six locales, the acceptance browser
   case.
6. **Acceptance review.** Every commitment above checked against the code, every
   deferred item answered yes or no with a reason, and any planning claim the
   implementation disproved corrected in place — the R2/R3 pattern.

## Validation and tests

Beyond the enumeration tests in slice 1, these are required and named:

- `TestGenerationIsIdempotentAcrossTicks` — two ticks over the same window
  produce one draft.
- `TestConcurrentGeneratorsProduceOneOccurrence` — N goroutines, exactly one
  winner, mirroring `import_fetch_worker_test.go`.
- `TestCreatingATemplateWithAPastStartDateBackfillsNothing` — D2, the one that
  keeps a phase anchor from becoming eighty-eight drafts.
- `TestDowntimeCatchUpGeneratesEveryMissedOccurrence` — the same watermark from
  the other side.
- `TestArchivedAccountBlocksTheOccurrenceWithoutRetrying` — a `blocked` row, one
  log line, no second attempt.
- `TestBlockedOccurrenceRetriesOnlyWhenAskedTo`.
- `TestGeneratedDraftIsExcludedFromLedgerReportsAndExport` — the invariant that
  makes D1 safe, asserted against the report read models and the export, not
  only against the transactions list.
- `TestPostingAGeneratedDraftInsideACheckpointRequiresOverride`.
- `TestBrowserApiCannotCreateADraft`.
- `TestSkippingAFutureOccurrenceStopsItGenerating`.
- `TestArchivingATemplateLeavesItsGeneratedDraftsAlone`.
- `TestUpdateRecurringTemplateLeavesOmittedFieldsAlone`.

## Out of scope, with reasons

- **Auto-post** — D1. Revisited at acceptance.
- **Weekend and holiday shifting** — D5.
- **Variable and estimated amounts** — D6; may return as an R10 concept.
- **Loan amortization schedules** — the roadmap already puts loan helpers in R10
  as optional follow-up work. A principal/interest split that changes every
  month is not a recurring template; it is a computed series.
- **Reminders and notifications** — nothing in the app notifies anyone yet, and
  R9 is not the place to invent a notification channel. The due inbox is a
  count in the nav.
- **Recurring investment transactions** (a monthly ETF purchase) — the template
  kinds are `ordinary` and `transfer` only. A recurring buy needs a lot, a
  price, and a fee model on the generation date; that belongs with the
  investment slices, and generating a wrong lot is much worse than generating a
  wrong expense.
- **iCalendar RRULE import/export** — the enumerator covers a deliberate subset.
  If a user ever needs `BYSETPOS`, the subset is where to extend, not the wire
  format to adopt.

## Owner questions

Answered by shipping the recommendation unless the owner says otherwise; each
is recorded so the acceptance review can revisit it as a default rather than a
frozen decision.

1. **Default `lead_days`** — recommend 5. Long enough that the first of the
   month is visible over a weekend, short enough that the inbox is not a
   forecast.
2. **Does the nav show a due count badge?** — recommend yes, it is the only
   thing that makes the inbox get read, and it is a count of drafts, not a
   notification.
3. **Should `run-now` be user-visible or a test seam?** — recommend visible, for
   the same reason "back up now" is: a schedule you cannot trigger is a schedule
   you cannot trust.
