# Bug classes — Ledger, reconciliation, dates and pricing

Part of the `validate-and-ship` review checklist. Postings, checkpoints,
drafts, dated reference records, owner-local dates, FX exposure. Every entry
shipped as a real bug in this repo. Numbers are stable IDs cited from code
comments and docs; they are not an order and are never reused.

## 7. Reconciliation guard bypass

Any new mutation path over postings must be guarded (see `ledger-invariants`).

## 12. Creation dates masquerading as financial facts

Stamping a record's `effective_from`/`opened_on` with "today" makes every
earlier posting fail, so installing the app now and importing years of history
breaks. Shipped three times: commodities (T-42), user-created categories
(T-43), import-created holding accounts (T-44), each fixed by opening the
record at the genesis date `0001-01-01`. Ask of any new dated container: is
this date a real financial fact (account `opened_on` — keep it, it should
reject earlier postings) or app bookkeeping (everything above — genesis)?

## 15. Producer drafts mistaken for posted ledger changes

R9 generation initially inherited a create/edit reconciliation guard that
checked dates without checking draft status (T-78). A test that creates the
draft before reconciling cannot catch this: create and edit a producer draft
after a checkpoint exists, prove no invalidation, then prove posting still
requires its override. Draft-edit preview is not a promotion preview. Also
test discard through the existing DELETE route: producer occurrence identity,
its audit, and deletion must commit or roll back together (T-77).

## 27. A self-check that reports damage on healthy books

`checkpoint_integrity` summed only the postings a session cleared, ignoring
the starting balance it carried from the previous checkpoint, so every second
reconciliation failed; and it called a snapshot posting superseded whenever
its transaction gained a version, so an unguarded description edit failed it
too. A diagnostic is only trusted if it passes on the ordinary workflows: test
each new check over chained and non-financially-edited data, and keep a
damaged-data case beside it. Judge staleness by financial facts and position,
never by version identity. Named regressions:
`TestCheckpointIntegrityPassesAcrossChainedReconciliations`,
`TestCheckpointIntegrityIgnoresNonFinancialEdits`,
`TestCheckpointIntegrityStillFlagsChangedReconciledFacts`.

## 39. Browser “today” and refreshes rewriting working copies

Financial defaults come from session `owner_today` (#101), including report
years, investment discovery and provider-assignment dates. Search the whole
frontend for private current-date helpers. Editable initializer effects must
not track the shared date. Keep dialogs outside loading branches and retain
their opening snapshot, so a refreshed workspace date cannot remount them and
discard selected dates/amounts. Check an opposite-midnight browser, year
rollover and refresh followed by reopening; see `owner-local-dates.spec.ts`
and `owner-local-date-contract.spec.ts`. A valuation's date must also
constrain holding events: current lot state can include recorded future buys,
sales or covers. Omitting a gains date must match explicit owner-local today.
Named regressions: `TestDatedValuationDefaultExcludesFutureInvestmentEvents`,
`TestInvestmentGainsDefaultTodayKeepsFutureSaleOutOfValuation`. The backend
has the same trap: grep `UTC().Format(time.DateOnly)` and bound/default
owner-entered dates through `SettingsService.OwnerTodayAt` (#339,
`TestOwnerEnteredDatesFollowOwnerDay`). Current reference reads must use that
owner day too (#385): pass the application's reference-date context before
repository transactions, preserve explicit historical dates, and date schema
guards by source effective date. The UTC current-version views can hide a
valid owner-today write or expose tomorrow's version. Keep
`TestCurrentReference*`,
`TestReferenceDateRefreshesPreferencesWithoutLeakingAcrossOperations`,
`TestReferenceDateBaselineGuardsUseSourceDates`,
`TestOwnerCurrentReferenceHTTPCreateListAndFutureExclusion` and
`owner-current-reference.spec.ts` green. Carry the authenticated owner into
blank transaction and ledger-read defaults too (#343); undated FX source
assignments use the owner's day at refresh start
(`TestBlankTransactionAndLedgerDatesFollowOwnerDay`,
`TestUndatedFXRefreshSelectsAssignmentsOnOwnerDay`).

## 74. Account defaults mistaken for currencies actually held

A multi-currency account can hold a posted currency with no default or price
series (#464). Current Pricing/attention discovery streams exact posted
balances per account/currency at the selected date; closed nonzero balances
still count, while exact zero, drafts, voids, deleted and superseded versions
do not add exposure. Compare the real reporting consumer and manual-rate
recovery; counts are not a certificate of historical or reporting-currency
coverage. Named checks: `TestPricingPostedCurrency*` and `attention.spec.ts`.

## 81. A raw debit-positive balance in a class-normalized field

`BalanceQuantity.normal_quantity_value` is the account's normal sign:
liability, equity and income balances are negated, so a card owing 500 reads
500. The account register built its running balance with the raw value, and
its screen trusted the field, so a card owing 500 showed −500 in the danger
colour beside the Ledger screens' 500 (#598). An account-scoped balance gets
its normal value from `normalBalanceValue` (`app/ledger.go`) with the
account's class, and a row shown beside it (a statement balance) uses the same
field. Net, expense and valuation totals may keep the raw value. Reconciliation
responses still do (#600). Named checks:
`TestRegisterLiabilityBalancesAreClassNormalized`, `register-statements.test.ts`
(`registerBalanceView`).
