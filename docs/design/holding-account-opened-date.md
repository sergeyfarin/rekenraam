# Design Note: The Opened Date Of An Import-Created Holding Account

Status: **accepted 2026-08-07**, implemented the same day (T-44).
Governs `backend/internal/app/import_trading212_invest.go`
(`resolveTrading212HoldingAccount`) and every future provider that
auto-creates `security_holding` accounts.

Related: `docs/plans/import-connection-accounts-plan.md` (the linking design
this amends), `docs/design/accounts-system-design.md` (locked structural
fields), `db.CommodityGenesisDate` (T-42), `app.categoryGenesisDate` (T-43).

## The problem

When an import meets an instrument it has no holding account for, it creates
one and stamps `opened_on`/`effective_from` with **the date of the fill it
happens to be looking at** — necessarily so, because using "today" would open
the account *after* a back-dated fill and reject it (the Slice 4b bug).

That account is then never revisited. If a later sync, a backfill, or a
re-imported older statement carries an **earlier** trade for the same
instrument, the posting fails with `posting date is before account opened
date`, and the row is stuck: no amount of retrying helps, and the user has no
obvious repair. The account's opened date is a guess made from whichever row
arrived first, which is not a property of the user's finances at all.

## Options considered

1. **Widen `opened_on` backwards when an earlier trade arrives.** Financially
   safe in isolation (moving the boundary earlier can never invalidate an
   existing posting), but it fights the locked-structural-fields rule, and a
   new account version effective *before* version 1 breaks the monotonicity
   that `version_seq`/`effective_from` as-of resolution relies on. Real work,
   real risk, in code two audits certified.
2. **Reject the row with an actionable message.** Honest, but it makes a
   routine backfill a manual repair chore, and the repair — editing an account
   opened date — is exactly the operation the lock forbids once postings exist.
   The user is left holding a problem the app created.
3. **Open import-created holding accounts at the genesis date.** Chosen.

## Decision

An import-created `security_holding` account is stamped `opened_on` and first
version `effective_from` of `0001-01-01`.

The rationale is the one already settled for commodities (T-42) and categories
(T-43): the date is **app bookkeeping, not a financial fact**. A holding
account is a container the app materializes for one (connection, instrument)
pair so lots have somewhere to live. Nobody "opens" it; the real financial
facts are the lots and the postings, and the position's true start is
derivable from the earliest lot. Making a container's creation date constrain
which history may enter it is the same category error each time, and it has now
produced three separate defects.

This keeps the lock intact — the opened date is never *changed*, so no
structural field mutates after posting activity exists — and it needs no new
versioning machinery.

## Deliberate boundary

**Hand-created holding accounts are unchanged.** `POST /investments/holding-accounts`
still accepts `opened_on`, and the account editor still offers it. A date the
user typed is a statement they made; a date the importer scraped off the first
row it saw is a guess. Only the guess is replaced.

Ordinary asset and liability accounts are further still from this rule:
there `opened_on` is a genuine financial fact about a real account at a real
institution, and it must keep constraining postings.

## Consequence to accept

An import-created holding account displays an opened date of `0001-01-01` in
the account editor, as system accounts already do. That is cosmetic, and it is
the correct reading: this account has no opening date of its own. If the
account list ever wants to show something more useful for these accounts, the
earliest lot date is the value with actual meaning.
