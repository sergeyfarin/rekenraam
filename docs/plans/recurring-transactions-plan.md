# Recurring Transactions Plan (R9)

Status: **complete; all six slices accepted 2026-08-31. Production generation
and public run-now are active. R10 planning is next.**
Acceptance evidence: `docs/reviews/r9-acceptance-review-2026-08-31.md`.
`docs/roadmap.md` owns that sequence. Written 2026-08-29,
immediately after R5's ordinary-bank CSV import closed. Slice 1 delivered
`internal/recur`, `backend/migrations/0003_recurring.sql`, and
`db.RecurringRepository` behind 26 named tests. This is the implementation reference for the
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
producer of persisted draft transactions**, and the conventions already say what such a
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

R9 v1 templates contain only `ordinary` and `transfer` transactions (enforced by
the schema). They do not call investment services and cannot create lot effects,
so T-76 provenance and T-75b investment correction do not block R9. This is a
deliberate boundary, not accidental compatibility: a future recurring-investment
producer must first specify that drafts have no lot effects, promotion activates
journal and subledger atomically, and discard leaves neither behind.

Slice 2 enforces the boundary on postings as well as the transaction-kind label:
security commodities and security-holding accounts are rejected even in an
`ordinary` template. Currency-only `commodity_trading` postings remain allowed
for balanced FX exchanges: the system account is not exclusively investment
infrastructure. A kind label alone was insufficient;
`TestRecurringTemplateRejectsInvestmentPostingsDespiteOrdinaryKind` proves it.

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
revision             INTEGER, starts at 1; incremented by edits/archive/watermark
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
last_audit_event_id INTEGER NULL REFERENCES audit_events
UNIQUE (template_id, occurrence_date)
```

Discarding a never-posted generated draft atomically changes its occurrence to
`skipped`, sets a discard reason, clears `transaction_id`, and links the discard
audit through `last_audit_event_id`. Generation and blocked attempts also set
this audit link. The original generation audit survives draft deletion; the
discard audit includes the former transaction ID. No schema rewrite is needed:
migration `0006_recurring_occurrence_audit.sql` adds the nullable audit link.

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

**Activation gate:** slice 3 builds/tests generation and the scheduler but does
not start it in production or expose public `run-now` yet. Slice 5 enables those
entry points only once the dedicated review/discard surface exists. Otherwise
this slice order would violate the producer-owned draft rule above. The gate
was satisfied in slice 5 on 2026-08-31: startup and minute ticks now run from
`command.go`, and generated drafts are reachable at `/app/recurring`.

1. Read the owner's `user_preferences.time_zone`; `localToday` is today in it.
2. For each enabled, unarchived template: `window = [generate_from,
   min(localToday + lead_days, generate_from + 3999 days)]`,
   `due = recur.Occurrences(spec, window...)` minus dates with an occurrence row.
3. For each due date, in one transaction: build the `TransactionSpec` from the
   template, create the transaction with `status="draft"`, `OriginType=
   "scheduled"`, and insert the `recurring_occurrences` row. One transaction, so
   a crash between the two is impossible — the T-14..T-17 lesson (guard, insert,
   and enqueue must be one repo-layer transaction) applies unchanged.
4. On a validation failure — archived account, closed account, retired
   commodity, unbalanced template — write a `blocked` row with the message and
   move on. No retry, no log line per minute.
5. Advance `generate_from` to the bounded window end plus one day once every
   date in the window has a row — `generated` or `blocked` both count, because both are
   recorded. A date left unrecorded by a crash or a per-tick cap holds the
   watermark where it is, and the next tick picks it up.

The watermark and the occurrence rows are deliberate belt and braces. The rows
are the correctness mechanism; the watermark only keeps a tick from
re-enumerating a decade of history to discover it has nothing to do.

Per-tick materialization is capped (`maxOccurrencesPerTick`, 50) so a long
outage resolves over multiple ticks rather than unbounded work in one tick.
Enumeration is also bounded to 4,000 calendar days, with at most one occurrence
per day; an outage beyond the pure enumerator cap still makes progress (T-82).
Losing the race on the unique constraint is a normal outcome, not an error — same
handling as `db.ErrBackupOccurrenceExists`.

**Template-edit races:** materialization and watermark advancement must verify
the template's captured revision, enabled/archive state, and schedule inside the
same write transaction. The slice-2 revision column exists for this purpose as
well as PATCH safety. A tick based on an old schedule must not advance a newly
edited template's watermark past dates that have never been handled; re-read on
conflict at the next tick. Both writes acquire the SQLite writer lock before
taking a read snapshot; independent database pools therefore serialize without
attempting to upgrade a stale WAL snapshot. Named tests cover stale schedule,
disable/archive, and independent-pool generation races.

**Drafts and FX coverage.** `fx_work_after_posting_version_insert` fires on
`posted` versions only, so a generated draft triggers no coverage work. That is
correct and is left alone: the due inbox shows each amount in its own commodity,
never a converted total, so there is nothing on that surface a missing rate
could make wrong. Coverage happens at post, as it does for every other
transaction. The refresh planner also excludes draft dates. ADR 0010 was
explicitly amended at acceptance to replace its earlier speculative draft-FX
policy with this implemented rule (T-83).

**Reconciliation.** A draft is not in the ledger, so generating one can never
invalidate a checkpoint. *Posting* one runs the ordinary period-impact check —
a backdated occurrence inside an active checkpoint returns
reconciliation-override-required, and the inbox previews the named checkpoints
before the user confirms. The existing checkpoint machinery is reusable, but
slice 3 verified that the current update-preview route deliberately preserves
draft status and therefore previews an edit, not promotion. Slice 4 now supplies
`GET /transactions/{id}/post/reconciliation-impact`, validating the saved draft
as posted and using its stored posting positions. The empty draft-edit impact
is never permission to post; posting rechecks the live checkpoint guard.

## The draft origin guard

**Shipped in slice 3.** `POST /api/v1/transactions` rejects `status="draft"`
when the service input has `OriginType == "browser_api"`, returning HTTP 400
with `TRANSACTION_DRAFT_NOT_USER_CREATABLE` in the standard error envelope.
The message is translated in all six locales; the origin is set by the handler,
never chosen by request JSON.

Two consequences addressed in slice 3:

- The API draft→post and draft→discard lifecycle tests now use the recurring
  producer; they do not get an exemption in the handler.
- `POST /transactions/{id}/post` and `DELETE /transactions/{id}` remain the
  public promotion/discard routes. **Their implementation is not unchanged:**
  the occurrence FK is `ON DELETE RESTRICT` and a generated occurrence requires
  a non-null transaction ID. **T-77 is closed:** discard atomically preserves
  an audited skipped occurrence, removes its live transaction link, and deletes
  the never-posted draft with the existing audit/lifecycle checks. It cannot
  regenerate on the next tick or leave a dangling generated row. The existing
  route remains the single discard path.

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
| `POST` | `/recurring/templates/{id}/retry` | explicitly retry one blocked occurrence with `{occurrence_date}` |
| `POST` | `/recurring/templates/{id}/run-now` | materialize what is due now, the "back up now" analogue |
| `GET` | `/recurring/due` | the review inbox read model |
| `GET` | `/recurring/summary` | unpaginated draft/blocked counts and owner-local today |
| `POST` | `/recurring/preview` | read-only next five dates for an unsaved schedule |

**Slice-4 review contract (shipped 2026-08-31):**

- `GET /recurring/due` returns generated transactions still in draft plus blocked
  occurrences, ordered by `(occurrence_date, id)`. Each item includes template
  name/enabled/archive state, current description/payee, transaction ID, error
  detail, and exact per-commodity debit and signed credit totals. Generated rows
  use the edited draft, not the template; blocked rows use the current template.
  Separate debit/credit totals make incomplete draft edits visible. No FX total.
- One composed request per page, default 50 and maximum 200. Follow `next_cursor`
  until null; refresh from page one to discover newly added earlier occurrences.
  Posted/voided/deleted/skipped rows leave the inbox. Disabling or archiving a
  template never hides its outstanding review items.
- `GET /templates/{id}/occurrences` requires inclusive `from`/`to` dates, at most
  367 dates. Computed `scheduled` dates start at the watermark and merge with
  persisted identities, including old dates preserved through schedule edits.
  Disabled/archived templates can still preview their configured schedule;
  reads do not generate drafts or persist speculative rows.
- Skip requires a non-empty reason. A new identity must be a current schedule
  date at/after the watermark on an unarchived template. An existing blocked
  identity can be skipped even after archive or schedule change. Generated
  drafts use the existing transaction DELETE route; duplicate skips conflict.
- Retry requires an enabled, unarchived template and an existing blocked
  identity. It uses the current template at the original occurrence date even
  if the schedule has since changed; it never generates other dates or advances
  the watermark. Another validation failure returns `blocked=1`; success returns
  `generated=1`. The original occurrence ID and prior audit events survive.
  Captured template revision and blocked-attempt audit ID guard the atomic write
  against concurrent edits, skips, and retries, including independent pools.
- Posting/discard keep the existing transaction routes. The new read-only
  `GET /transactions/{id}/post/reconciliation-impact` validates the saved draft
  as posted and names the affected checkpoints using stored posting positions.
  It never posts or invalidates anything; posting always rechecks current state.
- The new occurrence-conflict error is translated in all six locales. The
  typed client supplies cursor continuation. Slice 5 adds screens and public
  run-now, with the same authentication/origin/CSRF guards as other mutations.
- `POST /recurring/preview` requires authentication but makes no writes. It
  applies the same strict schedule validation and enumerator as saved templates,
  returning up to five dates from owner-local today (or the later anchor). It
  counts occurrence limits from the anchor. Non-schedule patch fields are ignored.
- `GET /recurring/summary` counts all outstanding drafts and blocked occurrences
  independently of inbox pagination, including archived templates. The shell
  badge counts drafts only; the response also supplies owner-local today.
- Public run-now accepts an empty JSON object and generates at most 50 due
  occurrences for one template. It never posts or implicitly retries blocked
  identities. Disabled or archived templates return zero changes.

**PATCH omission semantics.** This repo's recurring bug class (see
`import_connections_test.go`, and T-36/T-45/T-47 for the money variant) is a
plain `bool` or zero value in a PATCH struct overwriting a field the caller
never sent. Template PATCH uses pointer fields throughout, and the test for it
is named and required in slice 2: `TestUpdateRecurringTemplateLeavesOmittedFieldsAlone`.

Implemented slice-2 contract:

- Omitted fields preserve current values. Explicit `null` clears `payee_id`,
  `by_weekday`, `day_of_month`, `month_of_year`, `ends_on`, or `max_occurrences`.
  Empty arrays replace postings/tags (empty postings are rejected); false and
  zero are real values. `max_occurrences=0` is invalid, not an alias for null.
- A changed frequency does not silently reinterpret leftover fields: clear
  fields that no longer apply in the same PATCH. `last_day_of_month=true`
  likewise requires `day_of_month=null` if a nominal day was previously set.
- Supplying `payee_name` replaces the old link unless an ID is also supplied.
  Known active names resolve through the existing payee matcher; unknown names
  remain unlinked. `payee_id=null` alone clears both the link and its name.
- A schedule-field edit resets `generate_from` to `max(owner-local today,
  starts_on)`; descriptive, enabled, lead-day, and posting edits preserve it.
  Existing occurrences/drafts are never rewritten. Re-enabling therefore keeps
  the catch-up watermark; the per-tick cap in slice 3 bounds the work.
- A read/merge/write PATCH uses a revision check in the repository transaction;
  a concurrent edit/archive/watermark move returns conflict without child or
  audit changes. `revision` is returned for inspection, not required as input.
- List returns the complete template configuration set, with postings and tags;
  there is no hidden limit or per-row HTTP fetch. `next_due_on` is the first
  scheduled date at/after the watermark, including overdue dates, or null for
  disabled/archived/exhausted templates. Slice 4 excludes every already-materialized
  future identity, so skipped dates cannot appear as next due.
- Save validates a balanced prospective entry at `max(local today, starts_on)`
  through the real transaction validator without writing a transaction. Actual
  generation must revalidate at each occurrence date. Investment postings are
  outside this producer's scope even if the entry's kind says `ordinary`.

Errors: `RECURRING_TEMPLATE_UNBALANCED`, `RECURRING_SCHEDULE_INVALID`,
`RECURRING_OCCURRENCE_ALREADY_MATERIALIZED`, `RECURRING_TEMPLATE_ARCHIVED`,
plus the `TRANSACTION_DRAFT_NOT_USER_CREATABLE` above. Six locales each.

## Frontend

`/app/recurring`, two views behind one nav entry:

- **Templates** — list with name, schedule in words ("Monthly on the 1st"),
  amount, next due, enabled toggle. Editor reuses the transaction entry form's
  posting rows, payee resolution (including the confirm-and-search path from
  T-50), and category picker. Slice 5 adds the missing shared tag input and
  explicit currency selection for recurring split/clearing legs. Template mode
  saves through template CRUD without creating a transaction. Unchanged schedule
  fields are omitted from edits so amount/name changes preserve catch-up.
  A live "next five scheduled dates" preview under the schedule block uses
  `POST /recurring/preview`. The planned `GET /occurrences` was insufficient:
  it requires a saved template and its 367-day range cannot preview five yearly
  dates. Saved occurrence history still uses that GET endpoint.
- **Due** — the review inbox. Each row: post, edit then post, skip, discard.
  Blocked rows show diagnostic details and a retry; they can also be skipped
  with a reason. Generated drafts are skipped by discarding them, preserving
  the audited occurrence identity. Bulk post confirms a checked set, names
  reconciliation impacts, and rechecks before each post. Posts commit separately;
  after a later failure only the remaining drafts stay in the confirmation.
  Archived templates never hide outstanding drafts. The nav badge polls the
  summary every 30 seconds; review actions invalidate the relevant read models.

All four screen states on both views (loading, empty, error, populated), six
locales, and an `[acceptance]`-tagged browser case covering create → generate →
post, following the R3a pattern. Amounts render through `$lib/money/format.ts`;
any amount the user types goes through `$lib/money/amount.ts`. No third copy of
either.

## Slices

1. **`internal/recur` + schema + repository — done 2026-08-29.** The pure
   enumerator (14 named tests), the four tables in one migration
   (`0003_recurring.sql`), and `db.RecurringRepository` (12 named tests,
   including same-book trigger coverage). No API, no scheduler. It shipped
   alone because everything else depends on the date arithmetic being right,
   and the date arithmetic is testable with no ledger at all — which is how
   the duplicate-date defect below was found before any caller existed.

   Two things the plan did not anticipate, corrected in place:

   - **The month-skip rule needed a series origin, not a special case.**
     Resolving "the anchor's month has already passed its nominal day" per
     occurrence shifted index 0 and left index 1 on the same date. The origin
     is now computed once, so shifting it keeps every later index one interval
     apart.
   - **A second book cannot be created**, because `books.id` carries
     `CHECK (id = 1)`. The same-book triggers are exercised through the arm a
     cross-book row would hit anyway — the target is not in this book — the
     way `TestMigrationsEnforceTransactionAndVersionIntegrity` already does.
2. **Template CRUD — done 2026-08-30.** `app/recurring.go`, five authenticated
   template routes, OpenAPI, typed frontend client, three error codes in six
   locales, and exact per-commodity balance validation on save. Migration
   `0005_recurring_template_revision.sql` protects stale merges. Service/API
   tests cover omission/null/false/zero, payee resolution, owner-local date,
   immutable occurrence identity on schedule edit, no ledger side effects, and
   a real draft transaction consumer. Repository tests cover edit/archive/tick
   revision conflicts without partial writes. The now-public enumerator also
   rejects year zero and ends at 9999 rather than returning five-digit dates.

   **Validation:** full backend race suite (including formatting/vet), frontend
   type check and all 349 unit tests, and the integrated production build passed
   on 2026-08-30. The new HTTP journeys are API integration tests; browser
   acceptance remains in slice 5 because this slice adds no recurring screen.
3. **Generator, scheduler, and the origin guard — done 2026-08-31.**
   `app/recurring_generation.go` and `db/recurring_generation.go` create each
   draft, occurrence identity, and audit in one transaction. Validation failures
   become blocked occurrences; infrastructure failures roll back and leave the
   watermark for retry. The cap is 50 materializations across all templates per
   tick, and the watermark advances only after the whole window is recorded.
   Captured revisions guard generation and watermark writes against edits,
   disable/archive, and concurrent generators, including independent pools.
   T-77 is closed: the existing DELETE route preserves an audited skipped
   tombstone before removing a never-posted draft. Posted records still refuse
   hard discard. Browser draft creation now returns the translated origin error;
   existing API draft-lifecycle cases use the real recurring producer.

   Testing found another prerequisite, T-78: create/edit reconciliation checks
   treated persisted drafts as ledger changes. They now exclude draft-only
   changes, with promotion still requiring its ordinary checkpoint override.
   The named API test creates and edits a backdated draft *after* reconciliation,
   proves checkpoints remain unchanged, then exercises guarded promotion.

   **At slice 3 close, activation remained off:** scheduler startup was not wired
   in `command.go`, and run-now existed only as a service input. Slice 5 enables both after the
   review/discard UI exists. Blocked retry and its read model belong to slice 4.

   **Validation:** the full backend race suite, formatting/vet, frontend type
   check and all 349 unit tests, and the production single-binary build passed
   on 2026-08-31. Sixteen new named service/API tests cover the generation and
   discard boundaries; existing API draft-lifecycle cases now use the producer.
4. **Due inbox read model and review actions — done 2026-08-31.**
   `app/recurring_review.go`, `db/recurring_review.go`, `api/recurring_review.go`,
   OpenAPI and the typed client deliver the review contract above. Next-due
   reads exclude materialized identities; skip/retry preserve audited identity
   atomically. Posting preview uses actual stored positions, including same-day
   checkpoint boundaries, and agrees with the checkpoints invalidated on post.

   Review testing found and fixed **T-81**: an edited never-posted draft could
   not be discarded because an unordered bulk delete hit its predecessor FK.
   Delete now walks newest revision first without weakening constraints, in the
   existing occurrence-tombstone/audit transaction. The named API regression
   first failed with HTTP 500, then passed after two draft edits and discard.

   At slice 4 close, public run-now and scheduler activation remained gated
   to slice 5. Named tests
   cover blocked revalidation, stale attempts, independent-pool retry races,
   rollback, archived cleanup, date validation, mixed-scale exact totals and
   overflow, cursor continuation beyond 200 items, and draft edit/post/discard.

   **Validation:** full backend formatting/vet/race suite, frontend type check
   (zero errors/warnings) and all 352 unit tests, and the production single-binary
   build passed. Seventeen new named backend tests and three client tests cover
   this slice. Browser acceptance remains in slice 5; no recurring screen was
   added or claimed here.
5. **Frontend — done 2026-08-31.** `/app/recurring` supplies templates and a
   cursor-paginated due inbox, loading/empty/error/success states, six locales,
   accessible native review dialogs, mobile layout, and a draft-count nav badge.
   The shared transaction editor preserves tags, line keys, exact quantities
   and clearing legs. Template saves never create transactions; draft edits
   remain drafts. Template pause/resume/archive, scheduled skip, blocked retry,
   edit/post/discard and partial-failure-safe bulk post are reachable.

   Production startup/minute scheduling and authenticated public run-now are
   now enabled after the create/generate/edit/post and archived mobile discard
   browser journeys passed. Preview/summary read models are documented above.
   The missing shared tag input and unsaved preview endpoint are small additions
   to the original frontend plan, not a second transaction editor or calendar.

   Validation covers strict no-write month-end/yearly preview, separate summary
   counts, route security, exact editor payloads and omission/null semantics,
   real browser posting/discard/retry and partial bulk failure. Changed
   reconciliation impacts stop posting and show a translated request for a
   fresh review (the named browser regression first caught a generic error).
   Full backend formatting/vet/race checks, frontend type checks and 355 unit
   tests, and the production build with 18 browser tests passed at slice 5 close.
   Slice 6 below records the broader commitment-by-commitment review.
6. **Acceptance review — done 2026-08-31.**
   `docs/reviews/r9-acceptance-review-2026-08-31.md` maps commitments, tests,
   decisions and exclusions. Closed T-82 (long-outage enumeration), T-83 (FX
   draft demand and contradictory policy), and T-84 (template payee edits
   resetting the form). Added real-producer report/CSV/QIF isolation evidence.
   No automatic posting, forecasts or other deferred scope was added.

## Validation and tests

Beyond the enumeration tests in slice 1, these are required and named:

- `TestGenerationIsIdempotentAcrossTicks` — two ticks over the same window
  produce one draft.
- `TestConcurrentGeneratorsProduceOneOccurrence` — N goroutines, exactly one
  winner, mirroring `import_fetch_worker_test.go`.
- `TestCreatingATemplateWithAPastStartDateBackfillsNothing` — D2, the one that
  keeps a phase anchor from becoming eighty-eight drafts.
- `TestRecurringCatchUpCapHoldsWatermarkUntilWindowComplete` — 64 missed dates
  across two ticks, with no loss or premature watermark advancement.
- `TestDowntimeCatchUpBeyondEnumerationCapMakesProgress` — an outage beyond
  4,000 dates makes bounded progress instead of failing forever.
- `TestRecurringGenerationRevalidatesAccountsAtOccurrenceDate` and
  `TestRecurringValidationFailureIsBlockedOnceAndDoesNotStopOtherTemplates` —
  a blocked row with an audit event, no repeated attempt or per-minute log.
- `TestBlockedOccurrenceRetriesOnlyWhenAskedTo`.
- `TestGeneratedDraftIsExcludedFromLedgerReportsAndExport` — the invariant that
  makes D1 safe, asserted against the report read models and the export, not
  only against the transactions list.
- `TestPostDraftTransactionIntoReconciledPeriodRequiresOverride` and
  `TestRecurringPostPreviewUsesStoredSameDayPositionsAndNamesCheckpoints`.
- `TestBrowserApiCannotCreateADraft`.
- `TestSkippingAFutureOccurrenceStopsItGenerating`.
- `TestArchivingATemplateLeavesItsGeneratedDraftsAlone`.
- `TestUpdateRecurringTemplateLeavesOmittedFieldsAlone`.
- `TestDiscardingGeneratedDraftPreservesOccurrenceAndCannotRegenerate` — T-77,
  required in slice 3 before generation can ship.
- `TestConcurrentTemplateEditCannotAdvanceAStaleGenerationWatermark` — slice 3.

## Out of scope, with reasons

- **Auto-post** — D1. Retained outside v1 at acceptance; no usage evidence
  justifies bypassing explicit review.
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


Acceptance retained all three owner defaults: **5 lead days**, a **draft-only nav
badge** (blocked counts remain separate in the inbox), and visible **Generate due
drafts**. These are reviewed defaults, not claims from user-usage research.
