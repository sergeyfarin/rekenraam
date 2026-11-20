# ADR 0022: Current reference data and descriptive audit

## Status

Accepted 2026-11-20 by the owner for [#611](https://github.com/sergeyfarin/rekenraam/issues/611). Implementation proceeds by reference kind. The existing dated editors, endpoints and schema still describe the shipped application until their respective changes land.

## Context

The earlier-dated account edits in the shipped design can correct descriptive
fields inside a past interval. Structural and lifecycle edits already have
to take effect on or after the latest version. Keeping names on version rows
still makes each lookup choose a date and lets an as-of join surface a label
the owner thought they had replaced. #608 showed this when a write returned a
label from a different owner day. The same dependence on date selection
complicates editor baselines, name reuse and updates with archived associations
(#568, #569, #602). The owner-day problems in Home and pricing (#606, #610)
also affect dated facts and need their own fixes.

The June 8 experiment (`d01680e8`) replaced version tables with a
centralized audit trail. That removed transaction versions and their
posting-version references in reconciliation. This decision keeps those
financial versions and references authoritative. Moving current
descriptions to their identity rows removes date selection from ordinary
edits. Direct database investigation can use the event metadata. An in-app
history view would add a read contract for data that serves investigations.
Use report-specific exports and saved investment runs for filing copies.
Revisit a general report archive when those outputs leave a filing need unmet.

## Decision

An ordinary descriptive edit takes effect on save. Each identity has one authoritative current value per descriptive field, independent of the owner’s time zone. Rename the same identity when the thing being described is the same. Create a new identity and archive the old one when the activities should remain separately identifiable.

| Data | Write and read policy |
|---|---|
| Financial facts | Preserve dates where they determine eligibility, balances, FX and prices, reconciliation, cost basis or structure. An earlier posting still uses the rules for its day. Saved investment runs retain their output bytes, inputs and fingerprints. |
| Reference-record notes, icons and websites | Keep the current value. Record the changed field names. A note may contain sensitive text the owner removes. Copying either value into an append-only event would retain it indefinitely. An overwritten value can be recovered only from a backup that contains it. |
| Account, category, institution, payee and instrument display names | Record the committed old and new names on a rename. Registers and live reports use the current name. |
| Category moves and payee defaults | Store the current link. Capture each old and new target’s ID and display name as values in the event. An absent parent or default is null. Deleting an otherwise unused former target leaves the event readable. |

The descriptive audit exists for direct database investigation. Use the existing `audit_events.metadata_json` for the edited record’s kind, ID and names of fields actually changed. Include values only for the selected changes above. One rename plus a note edit records both field names and both display names and omits all note text. Read the previous values, merge the submitted fields, snapshot target labels and write the event and edit in one transaction. A failed write commits neither. Descriptive PATCH requests send only changed fields. Edits to different fields compose. For competing edits of the same descriptive field, the last commit wins. Financial and lifecycle write guards still apply.

The app reads current records, dated facts and saved outputs directly. This audit has no in-app reader, public history API, new audit columns or indexes. A “Formerly” display remains an untracked idea.

## Field policy and consumer checks

For each slice, trace readers, writers, joins, constraints and tests before
moving fields. Record the chosen storage, callers and checks under #611.
Use the rows below to scope those traces.

| Kind | Current data and consumers | Dated facts, evidence and checks |
|---|---|---|
| Institutions | One identity row owns the name, kind, country, website, address, reference notes, descriptive metadata and current status. Country remains available for search, display and account entry. Create, archive and restore take effect at save. | Keep dated account-to-institution links. In the archive write, check non-archived accounts linked today or in an already-recorded later effective state. A time-zone change can make yesterday the new owner day. An exclusively past link permits archive. Check the reverse direction inside account creation, reassignment and restore. |
| Payees | One current record holds name, derived normalized name, defaults, metadata and status. Entry and recurring defaults use the current record. Forecast and Spending use the linked current name. Creation and archive take effect at save. | Keep transaction-version payee text and source import evidence. Search the selected version’s text and current linked name, including archived identities. Return each transaction once with normal filters and pagination. Preserve unlinked free text. Reusing a name never relinks old transactions. Current defaults protect targets from deletion. Former defaults alone permit deletion. |
| Accounts | Put the display name and reference notes on the identity. Classify code, country, last-four and external hints against their entry, export and import consumers before moving them. | Keep effective-dated account structure, including parent and institution, lifecycle, opening/closing dates, posting eligibility, commodity, precision and basis policy. Preserve version IDs used as investment evidence. |
| Categories | Income/expense accounts keep stable IDs. Use current display names, name overrides, icons and parents in live views. Classify the shared metadata by key. Preserve type, built-in/starter flags and stable built-in label keys. Audit a reset of a translated name override with its stable key. | A category move can change hierarchy-based budget or report subtotals. Direct postings keep their category and same-scope grand totals. Spending groups direct postings. Check sibling uniqueness, cycles, archive/restore and budgets when changing hierarchy. Trace `categoryHasAnyChildren`, the `account_versions_no_delete` trigger and the parent foreign key. Each currently sees historical child links. After regrouping moves the links, an otherwise unused former parent can be deleted when its only remaining category relationship is captured in audit metadata. A current child or a retained financial reference still protects its parent. |
| Instruments | Move display name to the identity. Review issuer, country and metadata by use. | Keep existing behavior for symbol, exchange/MIC, provider identifiers, instrument type, quote/trading commodities, scale and status until their import, quote, lot and valuation consumers are verified individually. Preserve matching evidence and basis inputs. |

Retire the public institution versions route together with its generated client,
unused browser wrapper, token-scope entry and access probe. Keep dated account
structure and lifecycle in storage. Neither versions endpoint has a screen.
The recommended account API disposition is to keep the route for
`ledger:read` token clients that need dated structure or lifecycle. In that
case, give it a structural response and remove the version-local display
name when account labels move. Confirm the disposition with the owner in the
account slice after tracing the documented consumers. Verify the OpenAPI
contract, HTTP callers, generated client, permissions and tests for each
route change.

## Consequences and acceptance

- Live reports for earlier periods use current labels. For Spending, verify that a post-period rename reaches the API, screen and newly generated CSV with the original figure. Test a delayed, failed and successfully capped category catalog, including built-in localization. Each report row must render independently of the catalog.
- Preserve filed wording through a retained report-specific export where required. A live report may regroup prior periods after a category move. A saved investment run retains its stored output bytes and hashes across a rename or price correction. Use saved labels where present and the documented current label or ID elsewhere. Defer a general archive until an unmet filing case exists.
- Read event metadata directly in tests. Cover all five rename kinds, separate and mixed field updates, rollback, clearing defaults, moves to/from the top level and deletion of an unused former target. Assert both target IDs and their captured display names after deletion. In the regrouping slice, test that a former child alone permits an otherwise unused category to be deleted, and that a current child and a retained financial reference each prevent deletion. Exercise same-field and disjoint edits from two tabs and both close/rename interleavings in #603.
- Retire earlier-date editors and their assertions per converted field. Preserve independent tests for dated financial facts, owner-day pinning, skipped occurrences and valuation knowledge cutoffs. Explicit effective dates on current-only commands receive a documented validation error.
- The first delivery targets a fresh book. Use the pre-freeze baseline procedure while available. A later installed-book migration takes the final effective state per identity ordered by effective date then version sequence descending, independent of the migration day. Test a late-recorded correction to an older interval, a zone shift and a same-day tie. Validate a seeded book, backup and restore, and all surviving financial references before dropping storage. The household dry run can proceed independently.
