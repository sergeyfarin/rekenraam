# R8 Budgets Plan

Status: accepted for implementation on 2026-09-09.

R8 adds a deliberately small, exact, per-currency monthly planning loop. A
user chooses targets for income and expense categories, sees posted actuals for
the month, and controls which asset/liability accounts make those actuals part
of the day-to-day budget. It is not an envelope allocator and does not reuse a
forecast or learned estimate as a target or an actual.

## Decisions

1. **Period.** A budget period is one owner-local Gregorian calendar month,
   identified by its first date (`YYYY-MM-01`) and ending on the month's last
   date, inclusive. Targets are independent month facts; there is no implicit
   copy-forward.
2. **Scope and mapping.** Budgets are book-wide. A target maps directly to one
   active, posting-enabled, non-system income or expense account (the category
   model) and one currency commodity. Parent/container categories cannot have a
   target. A category may have one target per currency and month.
3. **Exact values.** Target, actual, and remaining values use the ledger's
   coefficient-string plus scale representation. Targets are non-negative.
   Expense actuals use the category posting's debit-positive value; income
   actuals negate its credit-positive ledger sign. Refunds/reversals can make a
   net actual negative. Arithmetic happens in Go through `exact.ScaledInt`.
4. **Actual basis.** Only current posted, non-voided, non-deleted category
   postings dated inside the month count. The journal entry must also touch an
   asset or liability account whose treatment on that date is `on_budget`.
   Drafts, recurring assumptions, forecast events, learned estimates,
   transfers without a category posting, equity, investment lots and price/FX
   observations never count.
5. **Account treatment.** Treatment is an effective-dated, versioned account
   axis with `on_budget`, `off_budget`, and `excluded`. It is valid only for
   non-system asset/liability accounts. Until explicitly set, liquid cash,
   checking, savings and consumer-credit kinds default to `on_budget`; other
   asset/liability kinds default to `off_budget`. `excluded` stays out of the
   ordinary page entirely except in the account-settings list.
6. **Rollover.** R8 has no rollover. `remaining = target - actual` is confined
   to the selected month and never mutates the next month. Copying targets and
   envelope/available-to-budget allocation require a later product decision.
7. **Currency.** Unlike commodities are never summed. Rows and totals remain
   per currency; no FX conversion is performed in R8. This is native
   multi-currency planning rather than an experimental converted total.
8. **History and deletion.** Setting a target is an audited upsert. Setting it
   to zero removes the target fact with an audit event; posted ledger data is
   untouched. Account-treatment changes append versions and never rewrite old
   classifications, so a historical month is reproducible.

## API and read model

`GET /api/v1/budgets/month?period_start=YYYY-MM-01` returns one page-composed
snapshot: period boundaries, category rows (identity/type, target, actual and
remaining per commodity), per-commodity totals, selectable currency metadata,
and budget-account treatments. It is one request for the screen and has no
pagination because the complete category/account set is the calculation basis;
the service rejects an input over its explicit 5,000-row safety cap rather than
returning a prefix. Omitting `period_start` selects the current month in the
owner's saved IANA time zone, so a month boundary never follows server UTC by
accident.

`PUT /api/v1/budgets/targets/{category_id}/{commodity_id}` accepts
`period_start`, exact `quantity_value`, `quantity_scale`, and `change_reason`.
Zero deletes the target. `PUT /api/v1/budgets/accounts/{account_id}` accepts an
effective date, treatment, and change reason. Browser mutations use the normal
session-bound CSRF boundary and emit `budget.target.set`,
`budget.target.remove`, or `budget.account-treatment.set` audit operations.

Validation failures use the existing `VALIDATION_FAILED`; absent category,
commodity, or account uses `NOT_FOUND`; a category/commodity/account of the
wrong kind is a validation failure; exact overflow uses `LEDGER_OVERFLOW`.

## Screen contract

`/app/budgets` has a month picker and a currency selector, exact target inputs,
actual and remaining values, per-currency summaries, and an account-treatment
editor. Mobile uses stacked category cards rather than requiring horizontal
scrolling. Loading, empty, error, saving, and success states are explicit.
Every input has a persistent label; progress is expressed in text as well as
colour; focus remains on the edited row after saving. All copy is translated in
the six existing locales and all formatting goes through the shared money/date
boundaries.

## Delivery slices and acceptance fixtures

1. Migration, repository, exact aggregation and account-treatment history.
2. Authenticated composed read API plus target/treatment mutations and OpenAPI.
3. Responsive localized screen and navigation.
4. Cross-system acceptance, docs and integrated build.

Named fixtures:

- **Monthly boundaries:** 2028-02 includes leap day; adjacent dates do not count.
- **Signs and scale:** EUR expense `12.34`, refund `-2.00`, target `20` yields
  actual `10.34`, remaining `9.66`; income credits present as positive actuals.
- **Lifecycle:** drafts, voided and soft-deleted records contribute zero.
- **Treatment as-of:** a 15th-of-month switch classifies entries on either side
  using the version effective on each entry date.
- **Split entry:** two category postings sharing one on-budget counterpart are
  each counted exactly once.
- **Currency isolation:** EUR and USD remain separate even when a stored FX rate
  exists; a security commodity cannot be targeted.
- **No rollover:** January remaining never changes February.
- **Atomic audit:** target/treatment write and its audit event commit or roll
  back together; concurrent target upserts leave one valid current fact.
- **Accessibility/mobile:** keyboard-only target edit and treatment change,
  named inputs, non-colour remaining status, 390 px layout, all four page states.

## Cut line

No envelopes, rollover, annual/custom periods, household sharing, target
recommendations, automatic forecast-to-target conversion, payee/tag budgets,
FX-converted totals, or bank-balance allocation ships in R8. Those are explicit
later decisions, not hidden partial schema.
