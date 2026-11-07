# Household contracts plan (#520)

Status: **proposal, 2026-11-07. Not accepted and not on the roadmap.**
[`docs/roadmap.md`](../roadmap.md) owns sequencing; the current UI-overhaul
initiative takes precedence. Delivery tracking lives on
[#520](https://github.com/sergeyfarin/rekenraam/issues/520).

The proposed promise:

> *Know when each household contract stops being a good deal, and see it in
> the money you already track.*

## Problem

Energy, internet, mobile, insurance, health insurance and streaming are paid
through the ledger. Rekenraam already knows the payee, the amount and often a
recurring template for each. It does not know the terms behind the payment:

- when the fixed term or introductory price ends, after which the price rises
  or switching becomes free;
- how much notice cancellation needs, and therefore the last day to act;
- what the price is supposed to be, so an increase in a posted charge can be
  noticed.

The costly misses are an introductory price that quietly ends, a fixed term
that rolls into a more expensive rate, and a yearly switching window (health
insurance) that passes unnoticed.

## Why in Rekenraam rather than a separate app

Self-hosted subscription trackers such as
[Wallos](https://github.com/ellite/Wallos) and
[Renewlet](https://github.com/zhiyingzzhou/renewlet) offer reminders and
spending statistics from manually entered prices (according to their project
pages; not evaluated hands-on here). What they cannot see is what was actually
charged. Rekenraam already has:

| Needed | Already present |
| --- | --- |
| Provider identity | Payees (`payees`, versioned) |
| Expected schedule and amount | Recurring templates (R9), which reference a payee |
| Actual charges | Posted transactions from manual entry, CSV/QIF import and connections |
| A place for deadlines | The shared attention read and Review count (#458) |
| A glanceable summary | Home widgets (#478), including Recurring |
| Use by other tools | Read-only personal-access tokens (#334) |

A standalone tracker would have to rebuild all of this, or settle for typed-in
prices. The contract feature is therefore metadata layered on existing
financial records, not a new financial producer.

A Veduta dashboard card, a Home Assistant sensor or a notification script can
read deadlines through a read-only token once an API exists. That covers use
outside Rekenraam without a second application.

## Scope

### In scope

1. **Contract records.** A contract belongs to one payee and may link to one
   recurring template. Fields:
   - kind: energy, internet, mobile, insurance, health insurance, streaming or
     software, other (stable codes, localized labels);
   - start date; fixed-term end date (optional); minimum term (optional);
   - notice period, in days or months, and what happens at term end: it ends,
     it continues indefinitely with the stated notice, or it renews for a
     further fixed term;
   - expected price, as an exact decimal amount with commodity and interval;
     an optional introductory price with its end date;
   - customer or contract reference, notes, and an optional document link
     (for example a Paperless-ngx URL). Attachments stay out of scope.

   Contracts are archived, never hard-deleted, and are included in backups and
   restore. They create no ledger entries and cannot change any.

2. **Deadline attention.** For each active contract, Rekenraam derives:
   - the last day to give notice before the fixed term ends;
   - the end of the introductory price;
   - for health insurance, the yearly switching window as entered by the
     owner (see *Jurisdiction* below).

   A deadline inside a configurable window (default 45 days) is counted in the
   attention read and appears in Review and as a Home widget. It can be
   acknowledged, or snoozed until a date. Dates are computed in the owner's
   time zone (`user_preferences.time_zone`), as recurring schedules are.

3. **Price-change detection.** A posted charge to the contract's payee (or
   from its recurring template) whose amount differs from the expected price by
   more than a tolerance appears in Review once, with these choices:
   - **adopt it as the new price,** which keeps a dated price history;
   - **mark it as a one-off;**
   - **ignore it.**

   Charges in a different currency are compared only when they are in the
   contract's commodity; there is no implicit conversion.

4. **Alternative quotes, entered by hand.** The owner records offers from
   other providers: provider name, estimated annual cost, date, source URL and
   notes. Each contract shows them beside its current annual cost, computed
   from the expected price or from the last twelve months of posted charges.
   Quotes age visibly; an old quote is never presented as current.

### Out of scope

- **Tariff modelling and consumption data.** Comparing energy offers needs
  real import, export and gas consumption (for example P1 readings through
  Home Assistant), fixed charges, feed-in compensation and fees, and changes
  materially when Dutch net metering ends on 1 January 2027. That is a
  different domain, with different data and a different rate of change, from
  a ledger app's. If it is built, it should be a separate service. Its result
  can be entered as an alternative quote today, and could later be read from
  it through a narrowly scoped integration.
- **Offer feeds and deals APIs.** Public tariff datasets and comparison-site
  APIs carry coverage, freshness and terms-of-use questions. Under
  [ADR 0017](../adrs/0017-provider-independent-features-and-incomplete-data.md)
  none of them may become a prerequisite; any such adapter would need its own
  evidence gate.
- **Automatic switching or cancellation.** Switching stays manual, because
  coverage, service quality, activation fees and cancellation conditions make
  automated mistakes expensive.
- **Push notifications and email.** These remain an open product decision
  (*Notification policy* in `docs/product-requirements.md`). Version 1 shows
  deadlines in the app and exposes them through the read-only API, so external
  tools such as ntfy scripts or Home Assistant can notify.
- **Document storage.** Attachments stay out of scope (Current Scope Locks); a
  link field covers Paperless-ngx and similar tools.

## Jurisdiction

Rekenraam does not encode jurisdiction-specific rules. This is consistent with
its stance on tax. The owner enters the notice period and what happens at the
end of the term. Kind-specific presets may *suggest* values, never apply them
silently.

For the Netherlands, the expected common cases (to be verified against
current law and each contract before any preset text ships) are:

- Many consumer subscriptions may continue only indefinitely after the initial
  term, with at most one month's notice. The key date is therefore the end of
  the fixed term or introductory price, not a renewal trap.
- Fixed-term energy contracts can carry an early-termination fee before the
  term ends.
- Basic health insurance is switched at the year end, with a short window
  around 1 January.

This is why the end of the fixed term and the end of the introductory price,
paired with price-change detection on real charges, matter more than generic
renewal reminders.

## Proposed slices

Each slice is independently useful and keeps the app runnable.

1. **Contract records.** Migration, service, `/api/v1` CRUD with OpenAPI,
   the archive lifecycle, backup/restore coverage, and a screen under Planning
   with loading, empty, error and success states, usable on a phone and
   translated into six locales.
2. **Deadline attention.** Deadline derivation, the attention-read source,
   Review entries with acknowledge and snooze, and a Home widget; deadlines
   available through the read-only API.
3. **Price-change detection.** Matching posted charges to a contract,
   the tolerance, the Review entry and its three outcomes, and a dated price
   history.
4. **Alternative quotes.** Manual quotes and a current-annual-cost comparison
   on the contract screen.

## Open decisions

1. **Naming.** "Contract" is already used throughout these docs for design and
   API contracts. Options: keep *contracts* in the UI and use
   `service_agreement` in code and docs, or use *subscriptions*, which
   undersells insurance and energy.
2. **Link to recurring templates.** Should a contract require a recurring
   template, allow one, or own its schedule independently? This proposal
   assumes it *may* link to one, because some contracts are paid yearly or
   irregularly.
3. **Price-change matching.** Payee alone, recurring-template occurrences
   alone, or both? Payee matching catches charges entered or imported without a
   template, but can confuse two contracts with the same provider.
4. **Notification policy.** Whether a later slice should send ntfy or webhook
   messages itself, or permanently leave delivery to external tools.
5. **Roadmap placement.** Whether this becomes an R-initiative, and after
   which current work.

## Acceptance outline

The detailed acceptance criteria live on #520 before implementation starts.
The durable boundaries this plan sets:

- No contract operation creates, edits or voids a ledger entry.
- Every amount is an exact decimal with a commodity; there are no floats and
  no implicit currency conversion.
- Deadlines are derived from recorded fields in the owner's time zone. Tests
  cover month ends, leap days and notice periods expressed in months.
- Contracts, price history and quotes survive backup and restore.
- Nothing requires a network provider.
