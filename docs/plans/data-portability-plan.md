# Data Portability & Protection Plan (R3)

Status: **slices 1-3 shipped 2026-08-23; slices 4-8 planned**. Written 2026-08-23,
immediately after R2's acceptance review closed. Slice 1 delivered the
dedicated read-only connection, the one-snapshot export read model,
`GET /api/v1/exports/ledger.csv`, `GET /api/v1/exports/preview`, and
`docs/adrs/0011-ledger-export-contract.md`, which is now the authority for the
column schema, the filter semantics, and the trial-balance definitions this
plan describes. Slice 2 delivered `GET /api/v1/exports/bundle.zip` — the
scoped export — with entry-complete filtering, the seven-column trial balance
and its three identities, the eight reference files, a checksummed manifest
written last, `README.txt`, and the shared decimal vectors that pin
`exact.Decimal` to `formatLedgerAmount` across two languages. Slice 3
delivered `GET /api/v1/exports/qif`: one file per cash-like account, transfers
and categories in QIF's own syntax, splits that sum to their record, a foreign
counterpart stated rather than converted, the bare-versus-archive rule, and the
confirm flow for accounts QIF cannot express. This plan is the implementation reference for the
roadmap slice "R3 — portable **and protected** core data"; it replaces the
roadmap's inline prose, which stays as the one-paragraph summary and now points
here.

Governed by `docs/product-requirements.md` (CSV and QIF export are mandatory,
not optional polish — §"Export must support CSV export of core ledger data and
QIF export"), sequenced by `docs/roadmap.md`, and bound by the ledger
invariants in `docs/plans/transaction-ledger-core-plan.md` and the
`ledger-invariants` skill. Scope was extended 2026-08-05 (roadmap review §3b) so
the slice delivers one trust sentence end to end:

> *your data is exportable, backed up nightly, and provably balanced.*

Each clause of that sentence is a deliverable below. None of them may ship as a
claim the code cannot keep.

**Revision 2, 2026-08-23** — a contract-and-safety review raised six P1 and two
P2 issues against revision 1. All eight are resolved in place below and each
resolution is marked *(rev 2)* where it changed a decision: the online backup
API replaces scheduled `VACUUM INTO`, filters became entry-complete, the trial
balance became a three-column period statement, restore preserves the WAL set
behind a process lock, the QIF round-trip test lost its self-contradiction,
`REKENRAAM_SECRET_KEY` became an explicit part of the protection story, backup
runs gained an occurrence identity, and the whole export now runs in one
snapshot on a dedicated read-only connection. The export-filter semantics and
the trial-balance definition are contract, not implementation detail: they
belong in the slice-1 ADR, and the ADR is what a later reader is entitled to
rely on.

**Revision 3, 2026-08-23** — a second review found that two of revision 2's
fixes were themselves wrong, and two contracts were still undefined. Marked
*(rev 3)* below: the period trial balance holds only for date scoping and broke
under account/commodity scoping, so its closing figure is now split into a
derived and an actual column with an explicit excluded-movement term (split
again in rev 4 below); the promise
to reject a mismatched QIF date layout was **impossible to keep** — an
ambiguous date is valid under both layouts and no importer can tell — so it is
withdrawn and replaced with what is actually decidable;
`date_basis=transaction` gained the selection semantics it never had; and the
QIF archive gained the manifest its own omission rule assumed.

**Revision 5, 2026-08-23** — two contract-precision fixes, marked *(rev 5)*:
`in_scope` is defined mechanically rather than in prose, and slice 3's
acceptance condition no longer demands archive metadata from a bare file it
also permits.

**Revision 4, 2026-08-23** — a third review caught that revision 3's six-column
trial balance was itself only correct under `date_basis=entry`: transaction-basis
selection exports entries outside the range, which double-counts against a real
opening balance and goes missing from a real closing one. Marked *(rev 4)*: the
trial balance now separates in-range from out-of-range exported movement and
states the rule that resolves the whole confusion — **selection basis decides
which rows are in the file; balances are always entry-date arithmetic**. Two
smaller contradictions are fixed with it: `transaction_complete` is now in the
authoritative column list, and an ambiguous-only QIF can no longer come back as
a bare file with nowhere to state its layout.

## Verified starting point (checked against the codebase 2026-08-23)

What exists:

- `db.BackupSQLiteDatabase` (`backend/internal/db/sqlite.go:206`) — `VACUUM
  INTO` with a `0700` parent directory, `0600` on the file, and a refusal when
  the target already exists.
- `db.VerifySQLiteBackup` (same file) — opens the copy read-only and runs
  `PRAGMA integrity_check`. It does **not** yet run `foreign_key_check`, which
  the documented operator procedure in `README.md` does run.
- Both are reachable from exactly one place: `RecoveryService.PrepareBackup`
  (`backend/internal/app/recovery.go:37`), i.e. the `rekenraam recover-owner`
  CLI path. Nothing schedules a backup; nothing shows one in the UI.
- A durable background-work queue (`background_work_items`, ADR 0010) with
  leasing, coalescing, bounded retries and manual re-enqueue, plus two
  reference workers (`app/pricing_worker.go`, `app/import_fetch_worker.go`) and
  two schedulers (`app/pricing_scheduler.go` wall-clock local,
  `app/import_scheduler.go` "older than N hours").
- A QIF **importer** (`app/import_qif.go`) with the locale hardening from T-35
  and T-36 (`app/import_locale.go`: `parseFlexibleDate`, `canonicalDecimal`).
  There is no writer.
- Client-side report CSV (`frontend/src/lib/reports/report-csv.ts`) — R2's
  export of a screen, built from an already-fetched JSON response. It is *not*
  the pattern for a ledger export; see "Why R3's CSV differs from R2's" below.
- The ledger tables the export reads: `transactions`,
  `transaction_versions` (+ the `current_transaction_versions` view),
  `journal_entries`, `posting_lines`, `posting_versions`, `transaction_tags`,
  `posting_tags`, `investment_lots`, `price_observations`.

Three facts from the review that shape the design (all verified 2026-08-23):

- **The driver does expose SQLite's online backup API.** `modernc.org/sqlite
  v1.57.0` ships `backup.go` (`Backup.Step/Finish/Remaining/PageCount`) with
  `(*conn).NewBackup(dstURI)` reachable through `sql.Conn.Raw`. ADR 0004's
  preferred in-app path is therefore available, not theoretical (rev 2).
- **The pool is one connection.** `db.Open` sets `SetMaxOpenConns(1)` with a
  documented rationale (single writer, no transaction across a network call).
  A long-held read transaction on that pool would stall every request, which
  the export and the self-check both need.
- **Sealed data depends on an external key.** `REKENRAAM_SECRET_KEY` seals MFA
  shared secrets (`authService.SetSecretKey`) and connection credentials. It
  lives in the environment, not in SQLite, so no database backup contains it.

What does not exist: any export endpoint, any archive writer, any backup
schedule or history, any self-check, any `Settings → Data` screen, and any
restore command. All of it is new.

## Outcomes and non-negotiables

1. A user can leave. Every posted ledger fact they entered comes back out in a
   documented, stable, non-proprietary shape without database access.
2. **The export balances, per journal entry, always** (rev 2). Grouped by
   `journal_entry_id` and commodity, the exported quantity column sums to
   zero — under every filter, because filters select whole entries and never
   individual postings. Per-**transaction** balance additionally holds whenever
   every entry of that transaction is in scope — reported per row by
   `ledger.csv`'s `transaction_complete` column and archive-wide by the
   manifest's `all_transactions_complete`, so a consumer knows exactly which
   guarantee it holds for which group (rev 3). The per-entry identity is the
   same one R2's cashflow classification rests on.
3. Money stays exact. Amounts are canonical decimal **strings** produced in Go
   (`exact.Decimal` over `exact.ScaledInt`), point decimal separator, no group separators, sign
   carried as a leading `-`. No float touches an export at any layer, and the
   frontend never computes or reformats an exported figure.
4. **Exports include system accounts.** Reports exclude `commodity_trading` and
   friends because a report answers a human question; an export that dropped
   them would emit unbalanced transactions and violate outcome 2. Reports
   exclude, exports never do — the manifest states this.
5. A backup that has never been restored is not a backup. The restore path
   ships in this slice and is exercised automatically.
6. The self-check is **read-only and diagnostic**. It reports, names, and
   explains; it never repairs, and it never writes to a ledger table.
7. Nothing claims coverage it does not have. The backup names *the database
   and the attachments directory* from day one (R14a will fill the second
   half), and any excluded data is listed in the export manifest by name.
8. Export column headers and file names are stable English identifiers, not
   localized strings — backend translation is deferred (`docs/conventions.md`
   §Localization) and, independently, a machine-readable contract must not
   change shape with the user's locale. Only the surrounding UI is translated.

## Scope fence

**In R3:** posting-level ledger CSV; a CSV bundle with the dimension tables and
a manifest; QIF export for cash-like accounts; scheduled verified backups with
retention and visible status; a restore command plus a rehearsed, documented
procedure; a trial-balance/integrity self-check; one `Settings → Data` screen;
an empty-but-designed attachments hook.

**Not in R3, and each has a home:** the reporting-currency selector (approved
2026-08-19, sequenced *after* R3); accessibility regression coverage (R3a);
CSV **import** (R5); attachment storage and files-in-backup (R14a); a full
structured JSON export of settings, connections and profiles (open PRD
question — R3 deliberately does not answer it); a native beancount/ledger
writer (derivability only, see below); off-machine backup targets (S3, rsync)
and notification on backup failure (no notification channel exists yet).

**Explicitly not exported in R3, and named in the manifest so nobody has to
guess:** credentials and secrets, auth sessions, MFA enrolment, audit events,
authentication events, import profiles/batches/staged rows, connection
configuration, background work items, and non-current (superseded) versions.
The SQLite backup is the audit-complete artifact; the export is the portable
one. Say this in the UI, not only here.

## Export contract

### Grain and policy

One row per **posting version** of the **current** version of each transaction
whose status is `posted`. Drafts and voided transactions are excluded (no
current workflow creates drafts; a voided transaction is a fact about the
ledger's history, which lives in the backup). Soft-deleted records never
appear. The manifest records this policy verbatim, the same discipline R2
established for report CSVs.

Two date columns, never one: `transaction_date` (the transaction version's
date) and `entry_date` (the journal entry's date). They differ for multi-leg
and investment transactions, and a consumer that conflates them will
misclassify periods. `from`/`to` filtering is on `entry_date` by default, with
`date_basis=transaction` available — matching the vocabulary
`GET /transactions` already uses.

### `ledger.csv` columns

```
transaction_id, transaction_version_id, transaction_kind, status,
transaction_date, correction_of_transaction_id, payee_id, payee, description,
external_ref_hint, needs_review, transaction_tags, transaction_complete,
journal_entry_id, entry_seq, entry_kind, entry_date, entry_memo,
posting_line_key, line_seq, account_id, account_path, account_class,
account_kind, quantity, commodity_id, commodity, commodity_kind, posting_memo,
reconciliation_status, cleared_on, posting_tags
```

- Three identifier columns were added when slice 1 shipped, so every dimension
  the export names can be joined rather than matched on text: `payee_id`,
  `commodity_id`, and `account_class` (the schema's own word — revision 1 called
  it `account_type`, which named nothing in this ledger).
- Amounts are rendered by `exact.Decimal` (`backend/internal/exact/decimal.go`),
  added in slice 1 because the backend had never rendered a decimal before — the
  API sends coefficient and scale as separate fields and the frontend formats
  them. It now has a **cross-language twin**: `formatLedgerAmount` in
  `frontend/src/lib/money/amount.ts` applies the same rule in TypeScript, and
  neither language can import the other. Slice 2 is where the second Go caller
  arrives, so slice 2 also adds **one shared fixture of test vectors**
  (`{coefficient, scale, expected}`) read by both the Go tests and Vitest. Two
  implementations of one rule is how this repo's recurring decimal bug class
  starts (T-36/T-45/T-47, G-02); a shared fixture turns them into one
  specification with two readers.
- `transaction_complete` is the lowercase token `true` or `false`, repeated on
  **every posting row** of the transaction (rev 4). It is a transaction-level
  fact carried at posting grain, like `payee` and `transaction_date` beside it,
  because a flat file has no other place to put it. It is part of the stable
  schema: the slice-1 format ADR and the beancount mapping quote *this* column
  list, not a variant of it.
- `quantity` is debit-positive, exactly as stored (coefficient + scale rendered
  as a decimal string). Trailing zeros reflect the stored scale, not a display
  choice.
- `account_path` is the full colon-separated hierarchy (`Assets:Bank:ING
  Current`), which is what every plain-text-accounting tool wants; `account_id`
  is the stable key.
- `commodity` is the commodity code (`EUR`, `IWDA`), `commodity_kind`
  distinguishes currency from security from crypto.
- Tag columns are semicolon-separated within the single CSV field.
- RFC 4180 quoting, `\r\n`? **No** — LF line endings, UTF-8 **with** a BOM
  (Excel misreads UTF-8 without one, and every other consumer tolerates it).
- Header row first, then data. No context block. See below.

**Why R3's CSV differs from R2's.** R2's report CSV carries a leading context
block, because it is a snapshot of a screen that must still be legible a year
later on someone's desktop. R3's `ledger.csv` is a machine-readable table whose
first row must be the header; its context lives in `manifest.json` (bundle) or
in the `Content-Disposition` filename and response headers (standalone file).
Recording the divergence here so a later reader does not "fix" one to match the
other.

### Filter semantics — entry-complete selection (rev 2)

A filter that removes individual postings destroys the balance guarantee: drop
the counterpart leg and the remaining rows do not sum to zero. R2 hit the same
wall and answered it by refusing category/payee filters on cashflow. R3 answers
it by making the **journal entry the unit of selection**:

Selection composes in a fixed order, and `date_basis` decides what the first
step even selects (rev 3):

1. **The date basis picks the candidate set.**
   - `date_basis=entry` (default): entries whose `entry_date` falls in range.
   - `date_basis=transaction`: current transactions whose `transaction_date`
     falls in range, then **every** entry of those transactions — including
     entries dated outside the range. The two bases genuinely differ for
     multi-entry transactions, which is the whole reason the parameter exists.
2. **Account and commodity filters narrow the candidate set**, entry by entry:
   an entry stays if it contains at least one posting in a resolved account
   (with `include_descendants`, following R2's shared filter contract) or, for
   the commodity filter, at least one posting in that commodity. Both filters
   applied means both must match, on the same entry, though not necessarily on
   the same posting.
3. **Every posting of a surviving entry is exported**, including postings in
   accounts, commodities, and dates no filter named.

A consequence worth stating because it surprises people: under
`date_basis=transaction` a transaction straddling the range boundary comes out
whole, and under `date_basis=entry` the same transaction comes out as its
in-range entries only. Both are correct; they answer different questions. A
fixture with exactly that shape is exported under both bases and asserted in
slice 1.

So a filter widens to entry boundaries; it never cuts inside one. The manifest
records the requested filter and the resolved effect: `selection_unit:
"journal_entry"`, **`all_transactions_complete: true|false`** — archive-wide,
named so it cannot be mistaken for a per-transaction fact (rev 3) —
`incomplete_transaction_count`, and `out_of_scope_postings_included`.
Completeness is also carried **per row**: `ledger.csv` has a
`transaction_complete` column, true when every entry of that row's transaction
is in the export. A consumer that wants to balance transaction groups can then
filter on the column instead of trusting an archive-wide flag or building its
own lookup. A filter that cuts a transaction in half is not an error — it is an
entry-complete export of part of it, labelled as such at both levels.

**Standalone `ledger.csv` takes no filters at all** (rev 2). A downloaded file
cannot carry response headers, and a scoped CSV with no manifest is an artifact
nobody can reproduce or audit six months later. The flat file is therefore the
whole posted ledger; every scoped export goes through `bundle.zip`, which
carries `manifest.json`. The UI states this where the format is chosen, and
`GET /exports/ledger.csv` rejects scope parameters with
`EXPORT_SCOPE_UNSUPPORTED` rather than silently ignoring them.

### One snapshot, one connection (rev 2)

A bundle is many files from many queries. Run apart, `ledger.csv` and
`trial-balance.csv` can straddle a write and disagree — the archive would then
fail its own verification. So:

- The whole export (every file in the bundle, and the self-check's whole run)
  executes inside **one read transaction**. In WAL mode a `BEGIN DEFERRED`
  read gives a stable snapshot for its lifetime.
- That transaction must not run on the main pool, which is
  `SetMaxOpenConns(1)`: holding it would stall every other request for the
  duration. Slice 1 therefore opens a **dedicated read-only `*sql.DB`**
  (`mode=ro`, its own small pool, same required pragmas) used by exports and
  the self-check only. WAL readers are concurrent with the single writer by
  design, so this adds no write contention and no new failure mode. The
  rationale belongs in the slice-1 ADR next to the export contract.

### `bundle.zip`

The recommended artifact, because a zip's central directory is written last:
a truncated download fails to open rather than looking complete. Contents:

| File | Contents |
|---|---|
| `manifest.json` | schema version, generated-at, app version, query and policy, per-file row counts and SHA-256, the exclusion list, the attachments block |
| `ledger.csv` | as above |
| `accounts.csv` | **every account of every class, income and expense included** — id, path, parent_id, class, kind, commodity, institution, opened_on, closed_on, status |
| `categories.csv` | the income/expense subset restated in category vocabulary, with the metadata only categories carry: builtin/starter flags, `builtin_key`, name override |
| `payees.csv` | id, name, status |
| `commodities.csv` | id, code, name, kind, display scale, max_quantity_scale |
| `tags.csv` | id, name |
| `lots.csv` | investment lots: account, commodity, opened_on, original and remaining quantity, remaining cost basis, cost commodity, status |
| `prices.csv` | non-voided price observations: base, quote, date, price, source |
| `trial-balance.csv` | per account per commodity: `in_scope`, `opening_balance`, `exported_in_range_movement`, `exported_out_of_range_movement`, `excluded_in_range_movement`, `derived_closing_balance`, `actual_closing_balance` |
| `README.txt` | what this archive is, what it excludes, how to read it |

**`trial-balance.csv` is a period statement, and it needs more than one closing
figure** (rev 3, corrected rev 4). Two independent things break a naive
`opening + exported = closing`:

- **Scope.** Filter on account A and the export pulls in A's counterpart
  postings from account B, but none of B's unrelated activity. B's opening
  balance plus the exported slice of B is not B's closing balance, and a column
  labelled `closing_balance` would be stating a falsehood about B.
- **Transaction-basis selection** (the one revision 3 missed). It deliberately
  exports *every* entry of a selected transaction, including entries dated
  before `from` or after `to`. A pre-`from` entry is already inside the real
  opening balance and would be counted twice; a post-`to` entry is in the file
  but not in the real closing balance. So any identity that adds the file's
  whole movement to a real opening balance is arithmetically wrong under that
  basis, however it is labelled.

**The rule that dissolves the confusion: the selection basis decides which rows
are in the file; balances are always entry-date arithmetic** (rev 4). A balance
is a fact about the book on a date, and `entry_date` is the date it is made of.
`date_basis` never changes what a balance means — only which rows travel.

Seven columns:

| Column | Meaning |
|---|---|
| `in_scope` | mechanically: `(no account filter OR this account is in the resolved account set) AND (no commodity filter OR this commodity was requested)`. True for every row of an unfiltered export; false marks a row present only as a counterpart (rev 5 — the definition belongs verbatim in the slice-1 ADR, since a consumer reading the column must not have to infer it from our implementation) |
| `opening_balance` | the account's real balance the day before the range starts, from all history, by entry date |
| `exported_in_range_movement` | sum of this archive's rows for this account/commodity whose `entry_date` is in range |
| `exported_out_of_range_movement` | sum of this archive's rows whose `entry_date` is outside it — always zero under `date_basis=entry`, non-zero only for straddling transactions under `date_basis=transaction` |
| `excluded_in_range_movement` | in-range movement the scope left out |
| `derived_closing_balance` | `opening_balance + exported_in_range_movement` — what this archive's in-range content alone can justify |
| `actual_closing_balance` | the account's real balance at the range end, by entry date |

Three identities, all exact, all **independent of the date basis** — which is
the point of splitting the movement column:

- **A.** `opening_balance + exported_in_range_movement` ≡ `derived_closing_balance`
- **B.** `derived_closing_balance + excluded_in_range_movement` ≡ `actual_closing_balance`
- **C.** `exported_in_range_movement + exported_out_of_range_movement` ≡ the sum
  of `ledger.csv` for that account and commodity

A reconciles the archive against itself, B reconciles it against the book, and C
is the bridge between the two — the term that revision 3 folded into the others
and thereby broke. For an unfiltered entry-basis export, columns 4 and 5 are
zero everywhere, `derived` equals `actual`, and all three collapse into the
obvious one; the simple case stays simple.

**The derived figure is never called the account's closing balance**, because
for a counterpart account, or under transaction basis, it is not one.
`opening_balance`, `excluded_in_range_movement`, and `actual_closing_balance`
are figures the archive does not itself justify; `README.txt` says so plainly,
because an unexplained number is worse than an absent one.

Fixtures that prove it, in the trial balance and not only in export selection
(rev 4): account-scoped and commodity-scoped books with **unrelated activity in
the counterpart account**, and a **multi-entry transaction with one entry before
`from` and one after `to`**, asserted under both date bases.

**Categories are accounts here** (`docs/design/categories-design.md`: a category
*is* an account with `account_class` income or expense). `accounts.csv` is
therefore the complete set and `categories.csv` is a deliberate second view of
part of it, carrying the metadata the account row does not. `README.txt` names
the overlap so nobody double-counts.

`lots.csv` is included because lot cost basis cannot be reconstructed from
postings alone. It is a flat statement of current lot state, not a replayable
event log — `README.txt` says so.

### Beancount derivability (review §4.2)

R3 does **not** ship a beancount writer. It ships a shape from which one is a
mechanical transform, and a documented mapping table proving it:

| beancount needs | comes from |
|---|---|
| directive date | `entry_date` |
| narration / payee | `description` / `payee` |
| account name | `account_path` (already hierarchical) |
| amount + currency | `quantity` + `commodity` |
| per-transaction balance | outcome 2, guaranteed |
| metadata | ids, memos, tags, reconciliation status |
| `balance` assertions | `trial-balance.csv` |

**Required versus optional** (rev 2). Only `entry_date`, `account_path`,
`quantity`, and `commodity` are required of every posting; payee, description,
memos, and tags are legitimately empty on ordinary rows and a test demanding
otherwise would be testing nothing but the fixture. The acceptance test asserts
the four required fields on every posting, the balance identity per entry, and
one narration rule: beancount narration derives from `description`, falling back
to `payee`, falling back to the empty string — a documented rule, not an
accident of whichever field happened to be filled. No beancount parser
dependency.

### QIF export

QIF is a lossy legacy format, and pretending otherwise is how EU users lost
100× amounts on import (T-35, T-36). Rules:

- One `.qif` per account. A **bare `.qif`** comes back only when all three of
  these hold (rev 4): a single account was requested, nothing was omitted, and
  the written file contains at least one decisive date to anchor its layout.
  Otherwise the response is a zip — including the case where partial approval
  leaves exactly one supported account, because the omission metadata has to
  travel with the file rather than live in a dialog the reader already
  dismissed (rev 3), and including an ambiguous-only file, which has no in-band
  way to state the layout it was written in (rev 4). The zip holds:
  `<account-slug>-<id>.qif` per supported account, `manifest.json` (generated
  at, app version, date layout as written, per-file SHA-256, the included
  accounts with their ids, and the **excluded accounts with a reason code
  each**), and `README.txt` restating the layout, the omissions, and QIF's
  limits in prose. Same manifest discipline as `bundle.zip`, on purpose: two
  archive formats with two different metadata stories would be one story too
  many.
- `!Type:` from the account kind — `Bank`, `Cash`, `CCard`, `Oth A`, `Oth L`.
- **Investment accounts and non-currency commodities are excluded in R3.**
  `!Type:Invst` semantics vary per reader and would silently misstate cost
  basis; the CSV bundle is the lossless investment path.
- **A mixed selection never downloads silently** (rev 2). `/exports/preview`
  lists every account that would be excluded and why. The download itself
  rejects a selection containing an unsupported account with `422
  QIF_ACCOUNT_UNSUPPORTED`, naming them, unless the caller passes
  `allow_partial=true` — which the UI sends only after the reader has seen the
  list and confirmed. Three states, no fourth: all supported → download;
  mixed → confirm, then a download whose manifest/README names the omissions;
  none supported → refuse with the reason.
- Fields: `D` date, `T` amount (plus an identical `U` for Quicken
  compatibility), `P` payee, `M` memo, `C` cleared (`*` cleared, `X`
  reconciled), `L` category or `[Account]` for a transfer counterpart,
  `S`/`E`/`$` split lines, `^` terminator.
- Amounts always use `.` as the decimal separator, always with **no group
  separators** — the absence of grouping is what keeps a written amount
  unambiguous to any detector, ours included.
- Dates: the format is chosen by the caller (`qif_date_layout=mdy|dmy`,
  default `mdy`, four-digit year) because QIF carries no layout declaration.
  The chosen layout is stated in the UI, in the filename, and in the bundle
  `README.txt`. This is the export-side mirror of T-35.
- A transaction with legs in two commodities (an exchange) exports the leg
  belonging to this account plus a transfer counterpart, with the other side's
  amount and commodity in the memo. Lossy by construction; documented.

**Round-trip test (corrected twice; rev 3).** Revision 1 asked for a round trip
through a *decimal-comma* profile, which contradicts a writer that always emits
`.`. Revision 2 fixed that but over-promised in the other direction: it claimed
a mismatched date layout would be *rejected*. **It cannot be.** QIF carries no
layout declaration, and `01/02/2026` is a valid date under both MDY and DMY —
no importer can tell from file content that the reader chose the opposite
layout from the writer. That promise is withdrawn.

What is actually decidable, and therefore what gets tested:

- **Matching explicit profile always round-trips exactly.** Export, re-import
  with the layout the export declares, assert every date and amount is
  unchanged. This is the supported path.
- **Auto-detection round-trips when the file contains at least one decisive
  date** — a day greater than 12 fixes the layout for the whole file, which is
  precisely what `parseFlexibleDate` already does per file. The fixture is
  built to contain one.
- **An ambiguous-only file is undecidable, so it is never handed over bare**
  (rev 4). Every date in `01/02/2026` shape and no decisive date anywhere means
  the layout must come from outside the file, and revision 3 promised it in
  three places — filename, manifest, and `README.txt` — while also allowing a
  bare `.qif`, which has only the first. The guarantee is now honest on both
  branches: **the filename always states the layout**, archives additionally
  state it in `manifest.json` and `README.txt`, and an ambiguous-only export is
  always archived so it gets all three. Two acceptance tests, one per branch:
  the bare-file branch asserts the filename carries the layout, and the
  ambiguous-only branch asserts the response is an archive at all. Neither
  asserts an importer performs an impossible detection.
- **No written amount contains a group separator**, so nothing invites a
  grouping misread.

On a mismatched *decimal* profile, the honest statement is narrower than
"rejected" (rev 3, refined in slice 3 against what the importer actually does).
Reading Rekenraam's `1234.56` under a comma profile, `stripGrouping` sees a
four-digit leading group and refuses to parse it — but the importer does not
discard the row: it keeps the raw text and attaches an "unrecognized amount"
warning, so the failure is visible and the number is never wrong by 100×. In a
three-decimal currency (KWD, TND — this ledger supports them) `1.234` *is*
well-formed grouping under that profile and becomes `1234`, with no warning at
all. So: flagged in the common case, silent in a real corner. The test pins both
behaviours rather than claiming the corner away, and the mitigation is the same
out-of-band statement of separator and layout that the date problem needs.

Whether Rekenraam should also *write* comma-decimal QIF for European tools is
deliberately deferred; the point is that a file has one true reading and the
archive says which.

### API shape

All under `/api/v1`, session-authenticated, `GET` only (nothing here mutates):

- `GET /exports/preview` → JSON. Row counts per file, resolved account list,
  date range, the exclusion policy, estimated size. The UI shows this before
  offering a download so the file is never a black box, and it is the place a
  bad request fails with a proper error envelope — a streamed body cannot
  change its status code after the first byte.
- `GET /exports/ledger.csv` → `text/csv`, streamed, `Content-Disposition:
  attachment; filename="rekenraam-ledger-YYYYMMDD.csv"`.
- `GET /exports/bundle.zip` → `application/zip`, streamed.
- `GET /exports/qif` → `application/qif` for a single supported account with
  nothing omitted, otherwise `application/zip` (see the QIF archive rules).

Shared query parameters: `from`, `to`, `date_basis=entry|transaction`,
repeated `account_id`, `include_descendants`, repeated `commodity_id`;
QIF adds `qif_date_layout` and `allow_partial`. Repeated-ID filters follow R2's
shared filter contract so the two features mean the same thing by "these
accounts", and `date_basis` composes with them in the fixed order given under
"Filter semantics".

Non-JSON responses still get OpenAPI entries with their content types and
`Content-Disposition` documented (`api-contract` skill: handler and spec land
in the same commit). Errors keep the standard envelope; new codes:
`EXPORT_RANGE_INVALID`, `EXPORT_SCOPE_UNSUPPORTED`, `EXPORT_TOO_LARGE` (if a
ceiling proves necessary), `QIF_ACCOUNT_UNSUPPORTED`.

Streaming rules: rows are written as the scan produces them, with context
cancellation honoured; nothing is materialized in memory or staged on disk; the
whole response reads inside one snapshot transaction on the dedicated read-only
connection (above), so it never holds the single write connection — keep each
file to one ordered scan, never a per-row lookup fan-out.

## Backups

**Policy** (stored in SQLite so the UI can change it, defaults in parentheses):
`enabled` (true), `hour_local`/`minute_local` (03:15), IANA zone from the book
owner's preferences, `retention_count` (14), `retention_max_age_days` (null).
Wall-clock local scheduling follows `pricing_scheduler.go`, not the
"older-than-N-hours" shape, because "nightly at 03:15" is a user-facing promise
(`background-work` skill: store local time + zone, record runs as UTC).

**Location:** `BACKUP_DIR`, default `<database dir>/backups`. The deployment
docs recommend a different mount from the database — a backup on the same disk
survives corruption, not disk loss, and the docs must not overstate it.

**Copy mechanism: SQLite's online backup API** (rev 2). ADR 0004 prefers the
online backup API for in-app backups and permits `VACUUM INTO` only for a
compact *operator-triggered* copy; a nightly backup the app schedules itself is
squarely the first case, so revision 1's plan to schedule the existing
`VACUUM INTO` helper conflicted with the accepted decision. The driver
supports it: `sql.Conn.Raw` → `(*conn).NewBackup(dstURI)` →
`Step`/`Finish`, which also yields `PageCount`/`Remaining` for a real progress
figure. `BackupSQLiteDatabase` stays exactly as it is for the `recover-owner`
CLI, which *is* operator-triggered; the new path lives beside it rather than
replacing it, and the ADR needs no amendment.

Three implementation constraints, recorded here because each is easy to get
wrong (rev 3):

- **The entire copy — `NewBackup`, every `Step`, and `Finish` — runs inside the
  `sql.Conn.Raw` callback.** The raw driver connection is only valid for that
  callback's lifetime; stashing it to drive the copy from outside is a
  use-after-free waiting to happen, and no test would reliably catch it.
- **Source the backup from the dedicated read-only pool**, not the single write
  connection, so a nightly copy of a large book never stalls a user's save.
- **Bound the copy.** The online backup API restarts its copy when the source
  is written mid-flight; on a busy database that can starve. Copy in page
  batches with an overall deadline, and treat exceeding it as a retryable
  failure with a legible reason rather than looping forever.

**Work identity and idempotency** (rev 2). The queue's coalescing index covers
only `pending`/`running`, so it says nothing about a run that already
completed — the same scheduled date could be enqueued and executed twice. And
there is a crash window between the final rename and the run record, which
would leave an untracked file that pruning must never touch. So:

- `backup_runs` carries an **occurrence key** — `(book_id, trigger,
  scheduled_local_date)` for scheduled runs, `(book_id, 'manual', run_id)` for
  a button press — with a unique index on it.
- The run row and the work item are created **in one repository transaction**
  (the guard-then-create TOCTOU lesson, `background-work` skill, T-14). An
  occurrence that already has a run row is never enqueued twice.
- The target filename is **deterministic from the occurrence key**, not from
  the clock at execution time, so a retry writes to the same path instead of
  scattering near-duplicates.
- On retry, the handler reconciles what it finds: a verified final file for
  this occurrence is adopted and recorded (the crash happened after rename); a
  `.part` file is deleted and rewritten; anything else starts clean.

Handler steps:

1. **Preflight free space on the filesystem holding `BACKUP_DIR`** — which is
   not necessarily the database's filesystem, and on a well-configured
   deployment is deliberately a different device. The check is advisory:
   ~1.2× the database size, and space can still disappear mid-copy, so
   `ENOSPC` during the copy is handled on its own terms — partial output
   removed, the failure recorded legibly.
2. **A space failure is retryable, not terminal** (rev 2). Nothing about the
   request is invalid; an operator freeing 200 MB makes the identical work
   succeed. It takes the normal bounded backoff, surfaces in the read model,
   and is manually re-enqueueable — the same treatment as a provider outage.
   Terminal is reserved for what cannot succeed on repetition.
3. Copy through the online backup API into
   `rekenraam-<occurrence>.sqlite.part`.
4. Verify: `integrity_check` **and** `foreign_key_check` (extend
   `VerifySQLiteBackup`; the README procedure already runs both, the code does
   not — closing that gap is part of this slice).
5. Rename to the final name only after verification passes, so a `.part` file
   is always a failed attempt and never mistaken for a backup. `fsync` the
   file and its parent directory before the rename is trusted.
6. Prune by policy, under three conditions that must all hold (rev 2): the
   path is recorded in `backup_runs`, it matches the app's own name pattern,
   and — checked at delete time — **`filepath.EvalSymlinks` resolves it to a
   regular file still beneath the configured `BACKUP_DIR`**. A name and a
   database row are not authority to unlink a path; a symlink planted in the
   backup directory would otherwise redirect a delete anywhere the process can
   reach. Anything failing a condition is left alone and reported.
7. Record the run: started/finished, path, byte size, verification result,
   error summary.

Attempt cap 5 (the queue requires a bounded cap — T-39), failures surfaced in
the read model the Data screen already polls, with a manual re-enqueue via
`RequeueBackgroundWork`.

**Endpoints:** `GET /maintenance/backups` (policy, next scheduled run,
directory, recent runs, last-success age, and the secret-key statement below),
`PUT /maintenance/backups/policy`, `POST /maintenance/backups` → `202` with the
enqueued work item. Per ADR 0010, the response reports *accepted work*, never
implies a completed backup.

## The key is part of the backup — and it is not in it (rev 2)

`REKENRAAM_SECRET_KEY` seals MFA shared secrets and connection credentials.
It lives in the environment, so **no database backup contains it, and it must
not** — a key stored beside the ciphertext it protects is not protecting
anything. The consequence is blunt and has to be said out loud everywhere it
matters: *restoring the database without the original key gives you a book with
unreadable MFA enrolment and unusable connection credentials.* The ledger is
intact; those two are not.

This slice therefore ships:

- **An operator workflow**, in `README.md` and `docs/deployment-security.md`:
  where the key comes from, that it must be retained in a password manager or
  secret store **separate from the backup directory**, that rotating it
  invalidates sealed data unless the app re-seals first, and what to do when it
  is lost (re-enrol MFA, re-enter connection credentials — the ledger survives).
- **A statement on the Data screen**, next to the backup status rather than
  buried in docs: what the nightly backup covers, what it cannot cover, and
  the one thing the operator must keep elsewhere.
- **A restore-time verification step**: `rekenraam verify-backup` reports
  whether sealed rows in the backup decrypt under the currently configured key,
  so a mismatch is discovered during a rehearsal instead of during an incident.
- **A drill test** proving sealed data is still decryptable after restore when
  the retained key is supplied, and that its absence fails loudly with an
  actionable message rather than a decode panic.

Until all four exist, the Data screen does not use the word "protected".



## Restore path — ships in this slice

Revision 1 got this dangerously wrong (rev 2): moving only the main database
file aside and then deleting `-wal`/`-shm` **destroys committed transactions
that live only in the WAL**, and it leaves the "safety copy" incomplete — the
copy an operator would reach for after a bad restore. A SQLite database in WAL
mode is a *set* of files, and every step below treats it as one.

- `rekenraam verify-backup --from <path>`: opens read-only, runs
  `integrity_check` and `foreign_key_check`, reports the schema/migration
  version, the row counts a human can sanity-check, and whether sealed rows
  decrypt under the currently configured `REKENRAAM_SECRET_KEY`.
- `rekenraam restore --from <path>`:
  1. **Proves the server is not running** via an explicit lifetime lock — an
     advisory `flock` on a lock file held by `serve` for the life of the
     process, with the PID recorded in it. An idle SQLite connection or an
     absent `-wal` is not proof of anything; only a lock a live process holds
     is.
  2. Rejects `source == destination` (resolved through symlinks) before
     touching anything.
  3. Rejects a backup whose migration version is newer than the binary.
  4. **Preserves the whole set** — `<db>`, `<db>-wal`, `<db>-shm` — into
     `<db>.before-restore-<timestamp>/`, after a SQLite-aware
     `PRAGMA wal_checkpoint(TRUNCATE)` on the stopped database so the aside
     copy is complete and self-contained rather than a torn main file.
  5. Installs atomically: copy into a temp file in the destination directory,
     `fsync` it, `chmod 0600`, rename into place, then `fsync` the parent
     directory. Only after that does it remove the now-stale `-wal`/`-shm`
     siblings of the replaced database.
  6. Prints the attachments-directory step (a documented no-op until R14a) and
     the secret-key requirement, and declares success only after the final
     `fsync` returns.
- `README.md` and `docs/deployment-security.md` are rewritten around these
  commands, keeping the manual `sqlite3` procedure as the fallback.
- **A restore drill runs automatically**: a Go test that seeds a book, takes a
  scheduled-path backup, restores it into a fresh location, and asserts the
  restored book's trial balance and row counts match the source — including a
  case where the source has **uncheckpointed WAL content at backup time**, and
  a case that proves sealed data still decrypts with the retained key. The
  claim in outcome 5 is only true if these tests exist.

## Trial-balance self-check

Read-only. Persists only its own run record. Each check returns
pass/fail/skipped, counts, a capped sample of offending ids, and a
plain-language explanation with what to do next — never an automatic repair.

| id | Check |
|---|---|
| `entry_balance` | every journal entry balances per commodity, scale-aware, summed in Go (`exact.ScaledInt`) |
| `transaction_balance` | every posted transaction balances per commodity |
| `book_balance` | book-wide per-commodity posting sum ≡ 0 |
| `version_integrity` | exactly one current version per transaction; every posting version's journal entry belongs to the same transaction version; posting line keys unique per transaction |
| `lot_reconciliation` | per account+commodity, open lots' remaining quantity ≡ the posted holding balance; no negative remaining; remaining ≤ original |
| `checkpoint_integrity` | each reconciliation checkpoint's recorded balance still equals the recomputed balance of its postings (catches T-53-class overrides) |
| `sqlite_integrity` | `PRAGMA integrity_check` + `PRAGMA foreign_key_check` |
| `attachments` | **reserved slot, reports `not_applicable`** until R14a fills it |
| `account_version_coverage` | every posting has an account version effective on its entry date. Slice 1 shipped a fallback for the miss (`db/exports.go`: the as-of join is LEFT, falling back to the account's earliest version) so that a gap can never drop a posting and unbalance an export. The write path rejects the case today (`transactions_validate.go`, T-63), so this check exists to say so out loud: a non-zero count means data arrived from outside the service layer, and the fallback has been quietly covering for it |

Never `SUM()` a coefficient column in SQL — they are strings
(`ledger-invariants`). Every total here is folded in Go.

**Why a fallback needs a counter.** Slice 1 chose a fallback over both
alternatives on purpose: an inner join would silently drop a posting and produce
a file whose entries do not balance, and failing the whole export on one bad row
would break the feature exactly when a user most needs their data out. But a
fallback that never announces itself turns corruption into a plausible-looking
number. `account_version_coverage` is the announcement, and the export's preview
carries the same count so it is visible without running a check.

`GET /maintenance/self-check` returns the last result; `POST` runs it now and
returns the fresh one. It also runs automatically after each successful
scheduled backup, so "provably balanced" is a nightly fact rather than a button
nobody presses. It reads through the same dedicated read-only connection and
the same one-snapshot rule as the export, so a check running against a book
being written to reports one coherent state rather than a torn one, and never
blocks a write. Synchronous execution is fine at personal-finance scale (one
ordered scan of `posting_versions`); if a book ever exceeds a documented
ceiling, it moves onto the work queue as a follow-up, not a redesign.

## Attachments hook (designed, empty — decided 2026-08-05)

R14a will put files under `ATTACHMENTS_DIR`, outside SQLite, where `VACUUM
INTO` cannot reach them (`docs/plans/receipts-plan.md`). So, from day one:

- `manifest.json` carries `"attachments": {"included": false, "directory":
  null, "reason": "not implemented"}`.
- The backup read model and the Data screen state that the backup covers *the
  database*, and name the attachments directory as a future component rather
  than implying coverage.
- The restore command and docs name both halves, with the second a documented
  no-op.
- The self-check reserves `attachments`, reporting `not_applicable`.

R14a then fills these four slots instead of renegotiating the promise.

## Frontend shape

One new route, `/app/settings/data`, three panels:

1. **Export** — format (CSV bundle / ledger CSV / QIF), date range, account
   scope, QIF date layout when relevant; a preview line from
   `/exports/preview` ("12,481 postings across 14 accounts, 2019-03-02 to
   today"); the excluded-data list; then the download. Investment accounts
   excluded from a QIF selection are named, not silently dropped.
2. **Backups** — last backup (when, size, verified), next scheduled run,
   directory, the policy form, "Back up now", and recent run history with any
   failure and a retry.
3. **Health check** — "Run check", per-check results with explanations, last
   run time, and the reserved attachments row shown as not applicable.

Standard rules apply (`frontend-screen` skill): Svelte 5 runes, TanStack Query,
all four screen states, semantic accessible tables, theme tokens, and every
string through Paraglide — six locales, and the glossary
(`docs/localization-glossary.md`) gets the new terms (*backup*, *restore*,
*integrity check*, *export*) before the strings are written, not after.
Downloads go through an authenticated `fetch` → blob, same-origin credentials,
consistent with the existing client.

## Delivery slices and acceptance

Ordered so the announcement-blocking product requirement (export) lands first
and the protection work builds on a verified base. Slices 1–3 and 4–6 are
independently shippable; if R3 has to be cut, it is cut after slice 3 or
after slice 6, never mid-slice.

| # | Slice | Size | Done when |
|---|---|---|---|
| 1 | Dedicated read-only connection; export read model in one snapshot; `ledger.csv` (unfiltered); `/exports/preview`; OpenAPI; **ADR carrying the format-stability guarantee, the entry-complete filter semantics, and the trial-balance definition** | M | A fixture book exports; every journal entry sums to zero per commodity; system accounts present; policy echoed in the preview; a concurrent write during an export cannot change its output |
| 2 | `bundle.zip` — dimension files, `trial-balance.csv`, `manifest.json` with SHA-256, `README.txt`; beancount mapping table + its test; **shared decimal test vectors** (below) | M | Bundle opens, checksums verify, trial balance reconciles to `ledger.csv` summed in a test, and one fixture pins `exact.Decimal` and `formatLedgerAmount` to the same answers |
| 3 | QIF writer + archive (per-account files, `manifest.json`, `README.txt`), unsupported-account confirm flow | M | Round-trips exactly under the declared layout and under auto-detect with a decisive date; **the declared layout appears in the filename of every response, and additionally in `manifest.json` and `README.txt` for every archive** (rev 5); an ambiguous-only export is archived and both metadata files carry the declared layout; any omission returns a zip whose manifest names the excluded accounts and why |
| 4 | **Online-backup-API copy path**, policy table + migration, `database_backup` work kind with occurrence identity, scheduler, retention with symlink-safe pruning, `backup_runs`, endpoints, `foreign_key_check` added to verification | L | A scheduled backup appears and verifies; pruning refuses a symlink and anything outside `BACKUP_DIR`; a crash at each of four points leaves no duplicate and no untracked file; a full disk fails retryably and recovers when space returns |
| 5 | `verify-backup` and `restore` CLI (process lock, WAL-set preservation, atomic install), the `REKENRAAM_SECRET_KEY` operator workflow, docs rewrite, automated restore drill | M | The drill restores into a fresh path and matches trial balance and row counts, including a source with uncheckpointed WAL; sealed data decrypts with the retained key and fails loudly without it; restore refuses while `serve` holds the lock |
| 6 | Self-check: eight checks (including `account_version_coverage`) + reserved attachments slot, run record, endpoints, post-backup chaining | M | Each check has a test that deliberately corrupts a fixture (raw SQL, bypassing services) and is caught; a healthy book passes clean; a posting written behind the service layer with no account version effective at its entry date is reported rather than silently absorbed by slice 1's fallback |
| 7 | `Settings → Data` screen, six locales, glossary terms, e2e smoke | M | Export downloads, backup status renders, self-check runs, all states present, keyboard-navigable |
| 8 | Acceptance review + docs reconciliation (`implemented.md`, `roadmap.md`, `README.md`, this plan's status header) | S | Every deferred item has a yes/no with a reason, in this file, the way R2 closed |

**Scope risk, stated plainly:** R3 as scoped is four features (CSV export, QIF
export, backup+restore, self-check), and it is the largest slice since R7. The
cut lines above are deliberate. Slice 2 (`bundle.zip`) is the one piece whose
absence would not break a promise — `ledger.csv` alone satisfies the PRD — so
it is the first thing to drop if the slice runs long. Note the coupling the
review surfaced: dropping slice 2 also drops the only artifact that can carry a
*scoped* export, since the flat CSV is unfiltered by design. That is an
acceptable cut, not a hidden one.

## Backlog items this plan touches

Registered defects and debt that land inside R3's path, each with the slice it
belongs to. The registry is `docs/backlog.md`; this table only says *where*.

| Item | Where it lands | Why it belongs there |
|---|---|---|
| **T-63** a posting refused for an account-version gap reports the wrong reason | Slice 6, or earlier as a standalone fix | It is the guard that keeps slice 1's export fallback defensive. Fixing the message is cosmetic; keeping the rejection is not. Slice 6's `account_version_coverage` check is only meaningful while the write path still refuses the case |
| **T-64** collapse the five migration files into one | **After slice 4**, before `v0.1.0` | Slice 4 adds the backup-policy and `backup_runs` tables. Collapsing before it means collapsing twice; collapsing after the tag is not permitted at all |
| **T-55** no historical-upgrade migration test `[blocked]` | Post-`v0.1.0` | T-64 removes the only multi-step upgrade path such a test could exercise. Pre-release that is fine — there are no legacy databases — but the two items must not be worked in the wrong order |
| **T-61** the browser suite has no acceptance-mapped subset | Slice 7 | Slice 7 adds the first browser cases for export, backup, and self-check. They are exactly the kind of case T-61 wants grouped against a plan's validation matrix, so the tag is cheapest to introduce while writing them |
| **T-62** a commodity symbol renders hard against its amount | Slice 7 | The Data screen shows sizes, counts, and commodity-labelled figures; it should not add a fourth call site to the three that already run symbol and amount together |

Nothing else in the backlog blocks a slice. T-34 and T-48 are unrelated and
stay where they are.

## Validation matrix

| Area | Test |
|---|---|
| Export balance | `TestLedgerExportBalancesPerJournalEntryAndCommodity` under every filter, plus `TestFilteredExportKeepsWholeEntriesAndFlagsIncompleteTransactions` |
| System accounts | `TestLedgerExportIncludesCommodityTradingPostings` — the reports/exports divergence, pinned |
| Exactness | `TestLedgerExportRendersStoredScaleWithoutFloat`, including a 24-scale crypto quantity |
| Policy | `TestLedgerExportExcludesVoidedDraftAndDeleted` |
| Filters | `TestLedgerExportHonoursDateBasisAndAccountDescendants`, `TestStandaloneLedgerCSVRejectsScopeParameters`, `TestStraddlingTransactionIsWholeUnderTransactionBasisAndPartialUnderEntryBasis`, `TestAccountAndCommodityFiltersBothMatchOnOneEntry`, `TestTransactionCompleteColumnMatchesManifestCount`, `TestTransactionCompleteTokenRepeatsOnEveryPostingRow` |
| Snapshot | `TestBundleFilesShareOneSnapshotUnderConcurrentWrites`, `TestExportDoesNotBlockTheWritePool` |
| Bundle | `TestExportBundleChecksumsVerify`, and identities A/B/C as `TestTrialBalanceOpeningPlusInRangeEqualsDerivedClosing`, `TestTrialBalanceDerivedPlusExcludedEqualsActualClosing`, `TestTrialBalanceInRangePlusOutOfRangeEqualsLedgerSum` — each over **account-scoped and commodity-scoped fixtures with unrelated counterpart activity** *and* a **straddling multi-entry transaction, under both date bases** |
| QIF | `TestQIFRoundTripsUnderTheDeclaredLayout`, `TestQIFAutoDetectionRoundTripsWithADecisiveDate`, `TestBareQIFFilenameStatesTheLayout`, `TestAmbiguousOnlyQIFIsAlwaysArchived`, `TestQIFExportNeverWritesGroupSeparators`, `TestCommaProfileFailsOnTwoDecimalMoneyAndIsUndetectableOnThreeDecimal` (pins both halves of the corner) |
| QIF limits | `TestQIFExportRefusesMixedSelectionWithoutAcknowledgement`, `TestSingleSupportedAccountAfterPartialApprovalStillReturnsAnArchive` |
| Backup | `TestBackupUsesOnlineBackupAPI`, `TestBackupRunsEntirelyInsideRawConnCallback`, `TestBackupSourcesFromReadOnlyPoolAndDoesNotBlockWrites`, `TestBackupDeadlineUnderContinuousWritesFailsRetryably`, `TestBackupWorkerVerifiesBeforePublishingFinalName`, `TestBackupWorkerPrunesOnlyItsOwnFiles`, `TestPruneRefusesSymlinkAndPathOutsideBackupDir`, `TestDiskShortageIsRetryableAndRecovers`, `TestENOSPCMidCopyLeavesNoPartialArtifact`, and one crash-point test each after **creation**, **verification**, **rename**, and **run-record persistence** |
| Restore | `TestRestoreDrillMatchesSourceTrialBalance`, `TestRestorePreservesUncheckpointedWALContent`, `TestRestoreRefusesWhileServeHoldsTheLock`, `TestRestoreRefusesSourceEqualsDestination`, `TestRestoreRefusesNewerSchema`, `TestSealedDataDecryptsAfterRestoreWithRetainedKey` |
| Self-check | one deliberate-corruption test per check, plus `TestSelfCheckPassesOnHealthyBook`, `TestSelfCheckWritesNothingToLedgerTables`, and `TestSelfCheckReportsPostingsWithNoEffectiveAccountVersion` (the counter behind slice 1's export fallback) |
| Frontend | Vitest for the export-options and backup-status view models; Playwright smoke for download, backup-now, run-check |

Full-suite discipline per `validate-and-ship`: `scripts/test-backend.sh`,
`scripts/test-frontend.sh`, `scripts/test-e2e.sh`, and the coverage floor.

## Decisions this plan makes (reversible, but made)

1. Posting-level grain, not transaction-level — the only grain that preserves
   double entry.
2. Server-side streaming export, not a background job producing a file — a
   personal ledger streams in well under a second, and a job adds a state
   machine, a retention policy, and a temp directory for nothing.
3. Exports include system accounts; reports do not.
4. `ledger.csv` has no context block, unlike R2's report CSVs (reason above).
5. QIF excludes investment accounts in R3.
6. Voided and superseded versions are out of the export and in the backup.
7. The self-check runs synchronously and chains after each nightly backup.
8. Filters select whole journal entries; the flat `ledger.csv` takes no filters
   at all, and every scoped export is a bundle (rev 2).
9. Scheduled and UI-triggered backups use the online backup API; `VACUUM INTO`
   stays the operator-CLI path, as ADR 0004 has it (rev 2).
10. `REKENRAAM_SECRET_KEY` is never written into a backup, and the gap that
    creates is stated in the product, not only in the docs (rev 2).
11. Exports and the self-check read through a dedicated read-only connection,
    in one snapshot (rev 2).
12. A scoped trial balance states a *derived* closing figure and an *actual*
    one, and never conflates them (rev 3); exported movement is split into
    in-range and out-of-range so the identities hold under both date bases,
    because balances are entry-date arithmetic whatever the selection basis
    is (rev 4).
13. QIF layout mismatch is stated out of band, never claimed as detectable
    (rev 3).

## Open for the owner (recommendation first, none blocking the start)

1. **QIF default date layout** — recommend `mdy` with a visible selector:
   maximum compatibility with legacy Quicken/Money readers, and the layout is
   stated everywhere it matters. Alternative: default to `dmy` given the
   product's European lean.
2. **Backup retention default** — recommend 14 dailies with no age cap; a
   personal database is a few MB, and 14 covers "I noticed a week later".
3. **Backup directory in Docker** — recommend documenting a *separate* mount
   for `BACKUP_DIR` and defaulting to `<data dir>/backups` when unset, with the
   docs plainly saying that same-disk backups do not survive disk loss.
4. **Automatic nightly self-check** — recommend yes (chained after the backup).
   With no notification channel, a failure is visible only on the Data screen
   until R-later; the alternative is not running it and calling it on demand.
5. **`bundle.zip` vs `ledger.csv` as the primary export button** — recommend
   the bundle. The review's condition for that recommendation is met in rev 2:
   the trial balance is now a period statement that reconciles under a range,
   and the manifest states the snapshot and scope contract. The flat CSV stays
   a secondary link and is unfiltered by design.

Two qualifications carried from the review, and adopted:

- The word **"protected"** is not used in the product until the deployment
  guidance covers a *separate storage device* and the retention of
  `REKENRAAM_SECRET_KEY`. A nightly copy on the same disk as the database, with
  the key lost, is a weaker thing than that word suggests.
- The roadmap may show R3 as ▶ current while planning is underway, but **this
  plan's status header stays "planned, not started" until slice-one code
  begins** — the status describes the code, not the document.

These are recorded here rather than in `todo.md`; only the answers move.
