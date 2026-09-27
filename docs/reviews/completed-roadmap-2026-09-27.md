# Completed roadmap detail through R10

Historical implementation narrative moved from `docs/roadmap.md` on
2026-09-27. The current capability boundary is in `docs/implemented.md`;
this dated record preserves the earlier acceptance and scope notes.

### Done — R2: reports users can act on

Shipped a reports route with net worth over time, spending by category/payee,
and cashflow. `docs/plans/reports-plan.md` is the implementation reference and
now records what shipped and what was deliberately left out.

**Delivered 2026-08-19, acceptance review closed the same day.** `/app/reports`
presents all three reports as URL-addressable views: the net-worth series,
spending/income ranked by category or payee, and cashflow classified into
inflow, outflow, operating net, transfer/financing movement, and net movement.
Each has an accessible per-commodity table, a single-commodity summary chart
that adds no information the table lacks, CSV export carrying its own query and
exclusion policy, and print output. Repeated-ID filters
(`account_id`/`category_id`/`payee_id`/`commodity_id`) and drill-down from a
spending or cashflow row into the transactions list — the one item the earlier
slice cut — shipped 2026-08-18, closing what had briefly been the one
recorded gap.

Cashflow's counterpart classification is derived from the balancing identity
per journal entry rather than from an allocation heuristic, so `net_movement`
reconciles to the cash balance change by construction — see
`docs/plans/reports-plan.md` slice 4.

Two gaps are deliberate and recorded, not oversights: the net-worth series
response carries no asset/liability split, and cashflow takes no category or
payee filter, because such a filter would remove counterpart postings from the
basis and break the `net_movement` reconciliation guarantee — the filtered
question is answered by drilling from a cashflow row into the spending report
instead. The fuller report contract — saved definitions/runs, cross-currency
valuation, investment dimensions, and snapshots — remains recorded in
`docs/plans/reports-plan.md`'s acceptance review, each with a yes/no and a
reason; the reporting-currency valuation method is the only one approved to
build, sequenced after R3.

### Done — R3: portable **and protected** core data

**Delivered 2026-08-23/24, acceptance review closed 2026-08-24.** The trust
sentence holds end to end: a user can export the ledger as a flat CSV, a
checksummed archive, or QIF; the app backs itself up nightly with SQLite's
online backup API, verifies each copy before naming it, and prunes only what it
recorded; `rekenraam restore` installs a backup without destroying what it
replaces, behind a lock that proves the server is stopped; and a nine-check
read-only self-check runs after every successful backup. All of it is reachable
from `/app/settings/data` in six languages.

Two decisions worth carrying forward: the export contract is an ADR
(`docs/adrs/0011-ledger-export-contract.md`), not just a plan, because a
consumer builds against it; and the product still does not use the word
"protected", because that would need deployment guidance covering a separate
storage device and the retention of `REKENRAAM_SECRET_KEY`.

The four commitments below were the non-negotiables; every one is met, and
`docs/plans/data-portability-plan.md` records each deferred item with a yes/no
and a reason.

<details>
<summary>The scope as it was set on 2026-08-05</summary>

### R3 as planned: portable **and protected** core data

Ship core-ledger CSV and QIF export. These are mandatory product requirements,
not optional polish. The first release should favour a documented, stable export
shape over a broad structured-backup format.

Scope extended 2026-08-05 (review §3b) so the slice delivers the full trust
sentence — *"your data is exportable, backed up nightly, and provably
balanced"*:

1. **Scheduled backups.** The verified `VACUUM INTO` backup already exists but
   is reachable only from CLI recovery. Schedule it on the existing
   background-work queue with a retention policy and a visible last-backup
   status in the UI.
2. **A documented restore path.** Ships in this slice, not later. An untested
   backup is not a backup, and the claim above is dishonest without it.
3. **Trial-balance self-check.** Per-commodity posting sums ≡ 0, lots ↔
   holdings reconciliation, and SQLite `integrity_check`, surfaced in the UI.
   **Read-only and diagnostic**: it reports failures and never auto-repairs.
   No self-hosted competitor makes this claim.
4. **An attachments hook, designed but empty** (decided 2026-08-05). R14a
   will put files outside SQLite, where `VACUUM INTO` cannot reach them, so
   the backup procedure names "the database **and** the attachments
   directory" from day one and the self-check reserves a file-integrity pass
   slot. R14a itself ships after R5 — this hook exists so that "backed up
   nightly" never becomes a claim that has to be walked back.

Design the export shape so a ledger/beancount-format export is trivially
derivable later (review §4.2) — the plain-text-accounting audience is a
positioning group worth courting at announcement, at zero feature cost.

**The implementation reference is `docs/plans/data-portability-plan.md`**
(written 2026-08-23). It carries the export grain and column contract, the QIF
limitations, the backup policy and worker shape, the restore commands, the
seven self-check checks, the eight delivery slices with cut lines, and the
five open owner questions. The four numbered commitments above are its
non-negotiables; the plan may not quietly narrow them.

</details>

### Done — reporting-currency selector

**Delivered 2026-08-26** (approved 2026-08-19, sequenced after R3). One
reporting currency, a named valuation method (`observed_on_or_before`), on all
three reports: a stock converts at the date it is measured, a flow at the date
it happened. Per-commodity exact totals stay in every response — the conversion
is additive, never replacing what R2 shipped, and the UI renders it as one more
row after the rows it restates rather than as a substitution. A figure that
cannot be fully converted is omitted and named in the response's `valuation`
block rather than shown short. Cashflow's identities survive conversion exactly,
because the converted nets are derived from the converted parts. See
`implemented.md` for the full contract.

### Done — R3a: core-workflow accessibility regression coverage

**Delivered 2026-08-24.** Eight browser checks over the journeys named below,
combining axe (contrast, labels, ARIA, heading order) with keyboard-reachability
and focus assertions that axe cannot make — a screen can satisfy every axe rule
and still be unusable without a mouse.

They found real defects on their first run, all now fixed: **no page in the app
had a `<title>`**, so every tab and every screen-reader announcement was
unnamed; the light theme's accent family missed AA, including white-on-accent at
4.20:1, which is every primary button in the app; and a clickable table row
carried `role="button"` with `aria-selected`, which is invalid on that role and
also nested the row's own buttons inside a button. The palette values were
solved against the 4.5:1 threshold rather than nudged by eye, and the dark theme
already cleared it.

The suite also surfaced an ordering dependency that had been held up by luck:
`auth.spec.ts` is the bootstrap journey and needs a database with no owner, and
it only ran first because "auth" sorted before every other filename. A Playwright
project dependency now states that requirement.

**Subsequent delivery:** the reporting-currency selector shipped 2026-08-26.
R10's coherent snapshot, exact projection, authenticated balances/events API,
optional constant-as-of FX, forecast screen and opt-in learned-spending
extension are accepted; R8 budget planning is current below.

<details>
<summary>R3a as planned</summary>

### R3a as planned: core-workflow accessibility regression coverage

Add focused automated accessibility smoke checks for setup/auth, transaction
entry, reconciliation, reports, and import. Cover semantic controls, labels,
keyboard navigation, focus handling, and contrast violations; keep mobile
journeys in the broader Playwright suite.

</details>

### Shipped — R5: ordinary-bank CSV import

Ship CSV import plus saved mapping profiles. The user maps a bank statement once
and can reuse that profile for the next statement. Reuse the staged review and
commit pipeline; do not build another import path.

**First vertical slice delivered 2026-08-28.** Ordinary CSV files now use a
saved, book-scoped mapping profile for delimiter, named date/payee/memo/category/
external-id columns, one signed amount or separate debit/credit columns, date
layout, decimal separator, and sign inversion. The profile is created and reused
from the upload screen; parsing feeds the existing preview/dedupe/commit path.
The acceptance journey imports an EU semicolon/decimal-comma statement and
reuses its saved mapping for the next file. Named backend cases also exercise a
US debit/credit layout and reject missing or ambiguous mappings. Profile
editing/deletion and safe header/filename auto-suggestion followed on 2026-08-28:
only a uniquely best compatible mapping is selected automatically, while ties
remain explicit. Grouped unknown-payee resolution followed the same day: one
explicit link-or-create choice applies to every staged row carrying that name,
with fuzzy near matches offered before a new record is created. Minimal
preview-time rules v1 completed R5 on 2026-08-29: ordered literal contains
matches on payee or description visibly snapshot category, payee, and/or tags
into new staged rows before the normal review and commit path.

**Done ahead of R5:** the EU import-correctness defects T-35 (QIF `MM/DD`
parsed before `DD/MM`, profile override stubbed) and T-36 (decimal-comma
amounts 100× off) were fixed 2026-08-06 — `app/import_locale.go`. The
pre-announcement gate below is met for QIF; the CSV adapter must reuse
`canonicalDecimal` and `parseFlexibleDate` rather than re-deriving them, and
mapping profiles must expose `date_layout` and `decimal_separator`, which the
adapters already honor.

**Free-text payees (backlog T-50): resolve on entry, but never silently —
decided and shipped 2026-08-19.** Typing a new payee name prompts for
confirmation, offering existing payees through a fuzzy search (`minisearch`)
before creating a record. Import still commits an unrecognized name as free
text by design; resolving those in import review is separate follow-up work
that belongs with R5/R6, not with this item.

**Includes a minimal rules v1** (decided 2026-08-05, review §3c). Rules are
the retention feature for the import persona: CSV import without them means
manually categorizing every row of every statement, which is the week-two
abandonment path. The scope fence is deliberate and binding for v1:

- Ordered rules matching **contains** on payee/description, setting category,
  payee, or tags.
- Applied **inside the staged preview**, where the user already reviews rows.
- **No retroactive apply** to already-committed transactions, no regex, no
  amount predicates, no rule audit trail.

Everything outside that fence stays in the later import-rules slice.

### Completed 2026-08-30 — R12a investment integrity correction

The 2026-08-29 review found two reachable financial-correctness defects and one
provenance gap in a feature previously marked complete. The two defects closed
2026-08-30 before R9 resumed. ADR 0012 fixes the durable
boundary:
canonical journal, immutable investment subledger, named read-side projections,
and optional explicit accounting postings.

The corrections landed in the order specified by
`docs/plans/investment-integrity-plan.md`; do not expand this gate into R18 reporting:

1. **T-75a — lifecycle fence and symmetric self-check.** Generic investment
   mutation and non-posted investment creation are refused before writes;
   diagnostics cover basis and journal-only as well as lot-only positions.
2. **T-74 — average-cost conservation.** Disposed plus remaining basis now
   conserves exactly through sequential partial sales and final close while
   original acquisition basis remains immutable.

**Exit gate passed 2026-08-30:** the named T-75a/T-74 cases, full backend race
suite, frontend suite, integrated build, and 37-case browser suite are green. No
local development database existed to reset or assess. Average cost is restored
to ✅; the generic
lifecycle fence is shipped while native correction remains deliberately open.

**Required follow-up completed 2026-09-10:** T-76 now snapshots and exports
disposal method, resolution tier, versioned policy source, exact basis totals,
allocations, transaction version, and audit linkage. T-75b is the remaining
investment-native correction lifecycle in R16; the generic fence remains in
force until it ships.

### Done — planning loop

Order decided 2026-08-05 (review §3d): **R9 → R10 → R8**. Recurring
transactions are forecasting's data source, so R9 → R10 is a single coherent
arc that exercises the producer-owned draft machinery once instead of twice,
and it front-loads per-currency forecasting — the differentiator the parity
lens below commits to protecting. Budgets are independent of both and slot in
afterward with no rework. R9 is complete, including acceptance on 2026-08-31
(`docs/reviews/r9-acceptance-review-2026-08-31.md`); R10 planning is complete
and its snapshot, exact projection, authenticated API, constant-as-of FX,
forecast screen, basis-safe event details and cross-system acceptance have
landed. The eight-slice core is accepted in
`docs/reviews/r10-core-acceptance-review-2026-09-07.md`; the learned-spending
extension is accepted in `docs/reviews/r10-learning-acceptance-review-2026-09-09.md`.
The localized templates and due-inbox screens now expose
create/edit, skip/blocked retry, explicit post/discard and bulk review with
reconciliation checks. Startup/minute generation and public run-now are active;
every generated entry stays a draft until posted. Edited-draft discard retains
the T-77/T-78/T-81 lifecycle guards. Read-only unsaved schedule preview and
unpaginated summary counts support the editor and navigation badge.

1. **R9 Recurring transactions:** templates and due-entry generation into the
   reserved producer-owned draft workflow. Planned 2026-08-29 in
   `docs/plans/recurring-transactions-plan.md`: all six slices accepted, a pure
   `internal/recur` enumerator that R10 reuses for projections, drafts-only
   generation with a dedicated review inbox, and the `status="draft"` origin
   guard the ledger-core plan deferred until a real producer existed.
2. **R10 Projected balances — complete, planned 2026-08-31 and accepted 2026-09-09:**
   `docs/plans/projected-balances-plan.md` is the detailed eight-slice core
   execution contract. Read-only owner-local daily balances combine posted facts
   with
   separately labeled recurring drafts/computed dates, using occurrence identity
   to prevent double counting. Exact per-account/per-currency series come first;
   optional combined totals use explicit constant-as-of FX and complete coverage.
   Overdue assumptions carry to tomorrow visibly. Coherent snapshots, event
   explanations, bounds, named tests and core acceptance are required. Then M1–M4
   in `docs/plans/forecast-learning-plan.md` add opt-in local CPU spending models:
   daily/weekly fluctuations, monthly costs and annual calendar peaks, with
   confirmed history, no overlap with recurring bills, chronological evaluation
   and measured hardware budgets. M1's internal complete-history reader,
   classification/cadence baselines and exact residual allocation, M2's
   chronological selection and resource gates, and M3's opt-in surface are all
   complete. The extended recipe returns a nullable
   learned_spending response, estimated day-detail events and a separate,
   clearly labelled estimated-spending view in all six locales. M4's dated
   review accepts the extension. All eight core slices are accepted:
   the coherent read-only snapshot loader feeds an exact
   per-account/per-currency projection with recurring occurrence precedence,
   bounds and diagnostics, exposed through authenticated balances and
   cursor-paged event-detail endpoints. Optional combined curves use direct
   stored constant-as-of rates, complete coverage, explicit provenance and
   component-level exact rounding without changing source-currency results. A
   responsive six-locale screen now exposes strict URL filters, exact summaries
   and tables, an accessible two-curve chart, diagnostics and FX provenance from
   one composed response. Cursor-paged day details explain exact contributing
   events and safely clear/refetch when their basis changes; producer mutation
   paths invalidate the shared forecast cache without polling. Cross-system
   isolation, concurrent materialization, adversarial bounds/precision and the
   browser acceptance matrix are covered, and the dated slice-8 review maps the
   shipped core contract. Learning remains a separate opt-in overlay and does
   not change the core response.
   Learning M1–M4 are accepted; loan helpers remain optional later work.
3. **R8 Budgets — complete 2026-09-09:** exact per-currency monthly category
   targets, posted actuals, and effective-dated account budget treatment. The
   execution contract is `docs/plans/budgets-plan.md`; it deliberately excludes
   rollover/envelopes and keeps forecasts and learned estimates out of budget
   facts.

   The composed authenticated month read model, audited target upserts/removal,
   effective-dated `on_budget`/`off_budget`/`excluded` account axis, exact
   posted-only actuals, per-currency income/expense summaries, and responsive
   six-locale `/app/budgets` workflow ship together. Leap-month, lifecycle,
   sign, exact remaining, treatment-as-of and non-netted income/expense cases
   are covered by named backend acceptance tests.
