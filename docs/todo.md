# TODO — short-horizon working queue

The distilled "what next" view. Items here are pointers — detail lives in the
roadmap (initiatives), backlog (defect registry), or the linked review docs.
Delete items when done; promote items when they grow. This file is allowed to
be edited freely and is never the source of truth for a decision.

Last updated: 2026-08-28 (reporting currency and R3a complete; R5 CSV import is
current, with profiles and grouped unknown-payee resolution shipped).

## Where things stand

Everything below is merged. To pick this up on a local machine:

```sh
pnpm install
pnpm dev                     # backend :16888, frontend :1888
./scripts/test-backend.sh    # gofmt + vet + go test -race
./scripts/test-frontend.sh   # svelte-check + vitest
./scripts/test-e2e.sh        # builds the binary, boots a fresh SQLite, Playwright
```

Leave `PLAYWRIGHT_CHROMIUM_EXECUTABLE` unset locally — it exists only for images
that cannot download Playwright's pinned browser. Full commands and environment
variables: `docs/developer-workflow.md`.

The open items are decisions and new work, listed below and in `roadmap.md`.
**A note on ticket IDs:** `docs/backlog.md` explains a numbering collision —
T-42 through T-47 were used for two different sets of items across the two
branches that merged here. This file uses the renumbered IDs (T-48–T-53) for
the branch that lost the collision; see `backlog.md`'s header note for the
full mapping.

## Decisions — owner answers of 2026-08-19

Recorded here as pointers; the source of truth is `roadmap.md` and
`product-requirements.md`.

- **Reporting-currency selector: build it** (one reporting currency, named
  valuation method), sequenced after R3. Per-commodity exact totals stay in
  every response — conversion is additive, never replacing. **Shipped
  2026-08-26**, backend and UI; see `implemented.md` under Reports.
- **Free-text payees (backlog T-50): resolve on entry, but never silently.**
  Typing a new payee name prompts for confirmation, offering existing payees
  through a fuzzy search before creating a record.
- **Zero-proceeds write-off (T-38): a disposal at zero proceeds**, booking
  through the existing realized gain/loss treatment rather than a dedicated
  expense category. Shipped as a dedicated `InvestmentWriteOffInput` type with
  its own required `reason` and preview endpoint — see `backlog.md` and
  `implemented.md`.
- **Cashflow keeps its reconciliation guarantee.** Category and payee filters
  are not added to it; the filtered question is answered by the spending
  report, reached by drill-down from a cashflow row. Shipped 2026-08-19,
  including the entry-level tightening of spending's account filter.
- **Public-deployment gates (S-04, S-06, S-07): parked.** Self-hosted locally
  for now. The "R3 at the earliest" half of that condition passed on
  2026-08-24; what remains is the other half, which is an owner decision:
  unpark when an internet-exposed deployment is actually planned. The gates
  themselves are closed — throttle, auth events, and MFA all shipped — so
  unparking means enrolling the owner account and re-reading
  `docs/deployment-security.md`, not building anything.
- **Gains reporting (I-03 / I-04): research task, not a decision.** See
  `roadmap.md` — realized vs unrealized, per-country tax treatment, and
  presentation stability all need study first. Does not block R16 slice 1.
- **First non-English languages: Spanish, French, Dutch, German, Russian.**

## Decisions — none otherwise pending

All seven `reviews/roadmap-review-2026-07-19.md` §3/§4 proposals were decided
2026-08-05 — **all accepted**, each with a scope fence. Recorded in
`roadmap.md` ("Decisions adopted 2026-08-05"); do not re-litigate here.

The three `plans/connections-plan.md` and `plans/receipts-plan.md` questions
were also decided 2026-08-05: sequencing adopted with the quote-provider
slice moved into R17; R14a stays after R5 with an attachments hook in R3;
the provider "verify" items reclassified as blocking slice-start
preconditions. Also in `roadmap.md`.

**Awaiting an owner decision:** who reviews the drafted translations for the
five target languages (they are drafted, not natively reviewed — see the
localization item below).

**Also open (from the G-02/G-09 sweep, 2026-08-08):** whether a displayed FX
rate in `settings/currencies/+page.svelte` should truncate or round half-up.
See `backlog.md` G-09.

## Current initiative — R2 reports — **complete, 2026-08-19**

- [x] Shared report filter contract (`account_id`, `include_descendants`,
      `commodity_id`) on the backend, with a `query.filters` echo carrying the
      resolved account expansion — done 2026-08-17.
- [x] Spending view — done 2026-08-17/18 (`GET /api/v1/reports/spending`,
      `group_by=category|payee`, `direction=expense|income`; its own read model
      rather than an overload of `ledger/category-totals`). View switch, dense
      table, category/payee and spending/income switches, single-commodity bar
      chart, all screen states.
- [x] Cashflow read model + view — done 2026-08-18
      (`GET /api/v1/reports/cashflow`; classification derived from the
      per-journal-entry balancing identity, so `net_movement` reconciles to
      the cash balance change by construction and a many-counterpart split
      needs no allocation rule).
- [x] Filters — **date, bucket, group_by, direction, and the repeated-ID
      filters (`account_id`/`category_id`/`payee_id`/`commodity_id`) all
      shipped** by 2026-08-18, closing what was briefly the one R2 delivery
      gap.
- [x] Drill-down — done 2026-08-18. A cashflow row's in/out figures link into
      the spending report for that bucket's dates, resolved accounts,
      commodity, and matching direction; a spending row links into
      `/app/transactions` (`date_basis=entry`, `category_type` added to
      `GET /transactions`). Spending's account filter was tightened from
      transaction level to journal-entry level so the two reports mean the
      same thing by "spent from this account."
- [x] CSV + print-friendly tables; summary charts alongside (not replacing) the
      accessible data table — done 2026-08-18. CSV uses `formatLedgerAmount`
      from `$lib/money/amount.ts`, not the display formatter, because a
      spreadsheet cannot parse locale group separators. Charts are
      `aria-hidden`, single-commodity only.
- [x] R2 acceptance review — done 2026-08-19. Every preserved follow-up is
      answered yes or no with reasoning at the end of
      `docs/plans/reports-plan.md`. Next: the named reporting-currency
      valuation method, after R3. Saved definitions, snapshots,
      tax/jurisdiction dimensions, and a report builder are all deferred with
      stated reasons.

## Previous initiative — R3 portable and protected core data — **complete, accepted 2026-08-24**

Plan: `docs/plans/data-portability-plan.md`. Eight slices, in order; the first
three are the mandatory export requirement, the next three the protection half.

- [x] 1. **Done 2026-08-23.** Dedicated read-only connection (`db.OpenReadOnly`)
      + one-snapshot export read model + `GET /exports/ledger.csv` (unfiltered,
      streamed) + `GET /exports/preview` + OpenAPI + `EXPORT_SCOPE_UNSUPPORTED`
      in six locales + `docs/adrs/0011-ledger-export-contract.md`, which now
      carries the column schema, the entry-complete filter semantics, the
      `in_scope` definition, and the trial-balance identities. Nine named tests,
      including balance per journal entry, system accounts present, stored scale
      without float, and voided/soft-deleted exclusion.
- [x] 2. **Done 2026-08-23.** `GET /exports/bundle.zip`: entry-complete
      filtering (`from`/`to`/`date_basis`/`account_id`/`include_descendants`/
      `commodity_id`), the seven-column `trial-balance.csv` with its three
      identities, eight reference files, `manifest.json` with a SHA-256 per
      file written last, `README.txt`, and `fixtures/decimal-rendering.json`
      pinning `exact.Decimal` to `formatLedgerAmount` across both languages.
      Ten named tests, including the two shapes earlier revisions got wrong:
      a scoped export whose counterpart account has activity of its own, and a
      transaction straddling the range boundary under both date bases.
- [x] 3. **Done 2026-08-23.** `GET /exports/qif`: one file per cash-like
      account (typed from `base_kind`), transfers as `[Account]`, categories by
      path, splits that sum to their record, a foreign counterpart stated in the
      memo rather than converted, `qif_date_layout` in every filename, and the
      bare-versus-archive rule (bare only for one account, nothing omitted, and
      a decisive date). Investment accounts refuse with
      `QIF_ACCOUNT_UNSUPPORTED` until `allow_partial=true`. Thirteen named
      tests, including round-trips through the app's own importer under both
      the declared layout and auto-detection.
- [x] 4. **Done 2026-08-24.** Scheduled verified backups via SQLite's online
      backup API (ADR 0004's in-app path), sourced from the read-only pool so a
      copy never holds the writer. `backup_policies` and
      `backup_runs`; the run row and its work item are created in one
      transaction, and the run's occurrence key is what makes a *completed*
      night idempotent where the queue's own uniqueness stops. Retention prunes
      only paths that are recorded, correctly named, and resolve through
      symlinks to a regular file inside `BACKUP_DIR`. Fifteen named tests,
      including the four crash points and a planted symlink.
- [x] 5. **Done 2026-08-24.** `rekenraam verify-backup` and `rekenraam restore`.
      The server holds an advisory `flock` for its whole life, so restore
      *proves* it is stopped rather than inferring it; the previous database is
      preserved as a whole file set after a checkpoint, and the new one is
      installed atomically with an fsync before success is reported. Refuses
      source==destination (through symlinks) and a backup whose schema is newer
      than the build. `verify-backup` reports whether sealed rows decrypt under
      the configured `REKENRAAM_SECRET_KEY` — the question a restore otherwise
      raises too late. Eleven named tests, including a drill that compares the
      restored trial balance to the source's.
- [x] 6. **Done 2026-08-24.** Trial-balance self-check: nine checks over one
      snapshot on the read-only pool, read-only and diagnostic, chained onto
      every successful backup. `self_check_runs` and
      `self_check_results` — a table rather than a JSON blob, since "which check
      failed, how often, since when" deserves a query (the mistake T-54
      records). `account_version_coverage` is slice 1's promised counter.
      Finding: the schema's own triggers already refuse most of these
      corruptions, so the tests drop the guarding trigger to produce each one
      and the check is documented as the second line of defence.
- [x] 7. **Done 2026-08-24.** `/app/settings/data`: export (format, range, QIF
      layout, preview line, exclusion list, confirm flow), backups (status, key
      notice, policy form, back-up-now, history with retry), and the health
      check (run, per-check results with explanations). 53 messages in six
      locales, all four screen states, two `[acceptance]`-tagged browser cases.
      Shipped with T-61 (acceptance-mapped browser subset) and T-62 (commodity
      symbol spacing).
- [x] 8. **Done 2026-08-24.** Acceptance review closed in
      `docs/plans/data-portability-plan.md`: every commitment checked, every
      deferred item answered with a reason, and the four planning claims that
      testing disproved corrected in place. It found one real gap — the
      attachments hook was in the manifest, the self-check, and the restore
      output, but the backup documentation never named the attachments
      directory — now fixed in `README.md` and `docs/deployment-security.md`.

The plan was reviewed the same day it was written; eight contract/safety
findings are folded into revisions 2-5 (see its header). The **five owner
questions** (QIF default date layout, backup retention default, `BACKUP_DIR` in
Docker, automatic nightly self-check, which export is the primary button) were
all settled by shipping the recommendation; the acceptance review records each
as a default rather than a frozen decision.

## Before R3a — verification review of what R3 shipped

`docs/reviews/r3-verification-review-2026-08-24.md`, opened because a gap turned
up at almost every slice and the pattern was worth measuring rather than
reassuring. A first, coverage-directed pass found seven items; **all five
backlog IDs are closed as of 2026-08-24**, and resolving them found one further
defect (a 404 about a run that exists, fixed with T-68):

- [x] **T-65** a restore test asserts its premise, not its name — the WAL
      preservation behaviour is unverified (11.8% coverage on the checkpoint
      path). The most safety-critical gap of the five.
- [x] **T-66** the nightly schedule and the backup's queue path have no tests
      at all (0%).
- [x] **T-67** `lots.csv` and `prices.csv` have never been written with a row,
      so two money-bearing files are unverified.
- [x] **T-68** the two recovery paths — retry-after-cap and the sealed-data
      report — are at 20%.
- [x] **T-69** a backup that will retry is displayed identically to one that
      gave up.

**Six further review passes remain**, listed in the review in value order:
claim audit, money-path coverage, failure-branch walk, contract-vs-code diff,
concurrency, and the Data screen's states. They are not scheduled — R3a starts
next — but they are the queue if another verification pass is wanted.

## Previous initiative — R3a accessibility coverage — **complete 2026-08-24**

Eight browser checks over setup/auth, transaction entry, the transactions list,
reconciliation, reports, import, and settings — axe for the machine-checkable
rules, plus keyboard-reachability and focus assertions that axe cannot make.

They found four real defects on the first run, all fixed: no page in the app had
a `<title>`; the light theme's accent family missed AA, including every primary
button at 4.20:1; a clickable table row used `role="button"` with `aria-selected`
and nested buttons inside it; and `auth.spec.ts`'s need to run first was held up
only by alphabetical luck, now stated as a project dependency.

**Current initiative:** R5 ordinary-bank CSV import. Profile creation/reuse,
maintenance, safe header/filename auto-suggestion, and grouped unknown-payee
resolution now ship through the existing preview and ledger-commit pipeline.
The next R5 cut is minimal preview-time rules v1.

## Ready to start — unblocked by the 2026-08-19 decisions

1. **Payee resolution (backlog T-50) — done 2026-08-19.**
   - [x] Exact-name linking on every write path, so manual entry *and* import
         commit both resolve a typed name to the record that carries it.
   - [x] Manual entry confirm-and-search: a name no payee carries stops the save
         and offers near matches (fuzzy, via `minisearch`) before creating a
         record. Skipped when the name was not edited, so older unlinked data
         never blocks an unrelated edit.
   - [x] **History tool dropped** — the owner confirmed 2026-08-19 that the app
         carries no real data yet, so there is nothing to backfill.
   - Imports still commit unrecognized names as free text by design; resolving
     them in import review is follow-up work under import (R5/R6), not this
     item.
2. **Localization of the five target languages — drafted 2026-08-19.** The
   catalog is far larger than the earlier "roughly 250 messages" estimate: it is
   **1,170 messages** — 283 in `frontend/messages/app/` and 887 in
   `frontend/messages/settings/`.
   - Terminology is decided first and written down in
     `docs/localization-glossary.md`, anchored to GnuCash, localized MS Money
     and Quicken, and per-market banking language. **Review that file, not the
     strings.** Two deliberate departures from the literal are recorded there
     (*commodity* → *instrument*; *cleared* and *reconciled* must not collapse).
   - **All 1,170 messages are translated in all five languages**:
     `frontend/messages/app/{es,fr,nl,de,ru}.json` at 283/283 each and
     `frontend/messages/settings/{es,fr,nl,de,ru}.json` at 887/887 each. Key
     sets and placeholder sets are verified against `en.json` for every file.
   - A missing translation still falls back to English per message rather than
     going blank, so adding a new English string never blanks a screen.
   - A language picker lives at `/app/settings/language`; the locale resolves
     `localStorage → browser preference → English` and is covered end-to-end by
     `e2e/playwright/language.spec.ts`.
   - Still open: **native review** of each language — the translations are
     drafted from the glossary, not reviewed by native speakers, and the
     language settings page says so to anyone using a non-English locale. Three
     items to check first, in order: Spanish *Punteado* for "cleared", Russian
     *партия* for a tax lot, and Dutch *Instelling* for a financial institution
     (it collides with *instellingen*, "settings"). This is also why `backlog.md`
     G-08's input-parsing gap (decimal-comma locales) is no longer purely
     hypothetical — es/fr/nl/de all use a comma decimal separator.

## Bug-fix queue (from backlog — schedule independent of R2)

- [x] T-35 QIF EU date misparse — fixed 2026-08-06 (`app/import_locale.go`:
      profile `date_layout` honored, otherwise the layout is detected across
      the whole file).
- [x] T-36 decimal-comma amounts 100× off — fixed 2026-08-06, same file
      (`canonicalDecimal`, profile `decimal_separator`).
- [x] T-39 background work no longer retries forever — fixed 2026-08-06
      (`app/pricing_worker.go` `maxFXCoverageAttempts`; failed items listed in
      pricing source health with a manual re-enqueue endpoint).
- [x] T-40 `triangulation_max_hops` now honored — done 2026-08-06
      (`app/pricing_refresh.go`: multi-hop chain search, shortest route wins).
- [x] T-37 price observations can be voided — done 2026-08-06
      (`POST /pricing/prices/{id}/void`; **cascades** to rates triangulated
      from the voided leg — `TestVoidPrice_CascadesToRatesTriangulatedFromTheVoidedLeg`;
      R11 still owns the UI).
- [x] T-42 commodity enable date no longer blocks earlier history — done
      2026-08-06 (`db.CommodityGenesisDate`; retired the Trading 212
      instrument-backdating workaround). Follow-ups T-43 and T-44 both
      done 2026-08-07.
- [x] T-43 user-created categories open at the genesis date — done
      2026-08-07 (`app.categoryGenesisDate`; `opened_on` removed from the
      category create/update API and the editor).
- [x] T-44 a later import carrying an earlier trade commits — done
      2026-08-07 (import-created holding accounts open at the genesis date;
      `docs/design/holding-account-opened-date.md`).
- [x] S-04 lockout-safe login throttle — done 2026-08-06 (approved-device
      cookie moves a known device onto its own throttle scope; the cookie is
      not a credential). Public-deployment gate closed, then **parked
      2026-08-19** (see Decisions above).
- [x] S-07 authentication-event visibility — done 2026-08-06
      (`authentication_events` + `GET /auth/events` + structured logs;
      90-day retention). Public-deployment gate closed, then **parked
      2026-08-19**.
- [x] T-38 zero-proceeds write-off — done 2026-08-06, extended 2026-08-20
      (`POST /investments/write-off` + `/preview`; dedicated
      `InvestmentWriteOffInput` type, not a zero-amount sell; loss stays a
      computed gains value, see I-04; reconciliation-override support added
      2026-08-20 as T-53).
- [x] T-41 scaled-integer arithmetic consolidated — done 2026-08-06
      (`internal/exact/scaled.go`: `exact.ScaledInt` + `exact.Pow10` replace
      `scaledAmount`/`scaledInteger`/`pow10DB`).
- [x] S-06 multi-factor authentication — done 2026-08-07 (TOTP + recovery
      codes, `internal/totp/` + Settings → Security). The
      last public-deployment gate before it was **parked 2026-08-19**; what
      remains is enrolling the owner account before an internet deployment.
- [x] G-07 CI coverage signal — done 2026-08-07 (`COVERAGE=1
      scripts/test-backend.sh` + non-gating `backend-coverage` job + soft floor
      `scripts/check-coverage-floor.sh`; merged total 75.2%, floor 73.0%).
      Closes the last open item of `plans/backend-test-coverage-plan.md`
      besides Workstream 6, whose harnesses wait for their first consumer.
- [x] T-49 (was remote's T-43) gofmt drift cleared — done 2026-08-17; `gofmt -l`
      and `go vet` moved into `scripts/test-backend.sh` so CI enforces them.
- [x] T-51 (was remote's T-45) net-worth series re-reads the ledger per bucket
      — done 2026-08-18; one ledger read folded forward across buckets, with
      account versions replayed in one pass instead of a snapshot query per
      bucket. `bucket=day` over a year drops from ~1.4 s to ~9 ms, and response
      time is now flat across bucket granularity.
- [x] T-52 (was remote's T-46) CSP-blocked inline style cleared — done
      2026-08-18; it was a `style` *attribute* (`style-src-attr`), not a
      stylesheet: SvelteKit's generated `#svelte-announcer` hardcodes its
      visually-hidden rules inline. A Vite plugin strips the attribute and
      `app.css` styles the announcer, so `style-src` stays `'self'` with no
      `'unsafe-inline'` or `'unsafe-hashes'`.
- [x] T-53 (was remote's T-47) investment trades can override a reconciliation
      lock — done 2026-08-20. `reconciliation_override` accepted on buy, sell,
      dividend, reinvested-dividend, and write-off; five preview routes name
      which checkpoints a write would invalidate before it happens.

Still open, listed under "Open, unscheduled" below rather than here: T-48 (was
remote's T-42, TypeScript 7) and T-34.

## Open, unscheduled (from the doc sweep of 2026-08-07)

- [x] G-02 frontend money logic untested — **done 2026-08-08**. The editor and
      reconcile form landed first (turning up two decimal-comma/zero-posting
      bugs, see `docs/reviews/resolved-backlog-2026-07.md`); the three
      investment forms followed, turning up the same decimal-comma 100× error,
      still live in every one of them. They held seven copies of three
      helpers, not the two the survey counted. Validation is now in
      `lib/investments/form-amounts.ts` behind 35 named tests, because with no
      component-test harness a per-form behaviour change cannot be pinned in
      place. `toSafeInt` became `toInt64Coefficient` in `lib/api/investments.ts`
      — it adapts an API contract quirk and is not money arithmetic, so it
      stayed out of `$lib/money`.
- [~] G-09 `.svelte` files carrying private money math — opened and mostly
      closed 2026-08-08. `category-transactions.svelte` retired onto a new
      `sumByCommodity` in `$lib/money/amount.ts` (it held a third copy of the
      ledger's normal-sign rule). `gains-report.svelte` turned out **not** to be
      a defect on closer reading — its lossy conversion feeds only a sign test
      that picks a CSS colour. Remaining: `settings/currencies/+page.svelte`,
      which is larger than first recorded — it does ad-hoc *truncating* division
      on an implied FX rate where `ledger-invariants` requires half-up via a
      shared helper, and then drops the exact result through `Number()` before
      formatting. **Needs a decision** (truncate or round a displayed rate?) so
      it was not folded in silently. See `backlog.md` G-09.
- [~] G-08 amount input is not locale-aware — **display half settled
      2026-08-08**: `Intl.NumberFormat` wins over Dinero.js, the unused
      `dinero.js` dependency is gone, and `formatQuantity` now lives in
      `frontend/src/lib/money/format.ts` as the read-only display half of
      `$lib/money`. The input half (resolving the separator from the active
      locale when parsing and when refilling a form field) stays open, and is
      no longer purely hypothetical now that es/fr/nl/de (all decimal-comma
      locales) shipped 2026-08-19. See `backlog.md` G-08.
- [ ] T-63 a posting refused for an account-version gap reports the wrong
      reason ("posting account is invalid"). Cosmetic, but the rejection itself
      is load-bearing for R3's export — see `backlog.md`.
- [x] T-64 **done 2026-08-24** — seven migration files folded into one after
      slice 6 added the last of R3's schema. All 207 schema objects verified
      identical to the old chain; `TestMigrationsProduceTheExpectedSchema` keeps
      checking the shape. Development databases must be recreated.
- [ ] T-34 investment provider-event producer — `[blocked]` on R15's third
      slice and an unmade provider choice. See `backlog.md`.
- [ ] T-48 TypeScript 7 upgrade — `[blocked]` on `openapi-typescript` shipping
      TS 7 support. See `backlog.md`.

## Hygiene

- [ ] Keep `docs/README.md` accurate when adding or moving documentation.
