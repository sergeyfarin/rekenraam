# Reference history after v0.1 household testing

**Owner decision, 20 November 2026.** Finish the UI overhaul, prepare a v0.1 testing candidate and use the app before deciding how reference history should work. The candidate uses the shipped editors, API, version tables and audit conventions. The earlier five-slice proposal in PR #612 is withdrawn. Keeping the existing model is a valid outcome of the review. [Roadmap](../roadmap.md#deliberately-later) · [Dated-edit contract](../design/accounts-system-design.md) · [Tracker and initial evidence](https://github.com/sergeyfarin/rekenraam/issues/611#issuecomment-7022103335)

## Work that can proceed

| Work | Timing and scope |
|---|---|
| UI overhaul, including #443 Payees and #439 search | Ship their existing acceptance criteria. The Payees page can rename using changed fields at the current date. Search can use the existing query and payee ID. Historical editor controls and former-name search are separate decisions. |
| #568 historical editor and #569 historical name reuse | Hold implementation until the owner decides which dated edits and names people need. Keep the reproductions and existing regression tests. |
| #602 account edits with archived links | Fix the reproduced refusal when an edit keeps an existing link, including today’s rename, under the shipped contract. New rules for assigning a link across a date range can be decided separately. |
| #603, #606, #608, #609, #610 | A lost account update, disagreement over the owner day and a drifting valuation cutoff can affect the current model. Each has its own issue and dependencies. #565 (restore under an archived parent) and #584 (calendar validity) also keep their own decisions. |
| Candidate, household test and public release | Build the private testing candidate through the existing workflow. Use #336 for household evidence. The v0.1.0 milestone governs the public tag. #572/#576 separately govern the proposal to support upgrades from the household test database. A later storage change must respect the migration policy in force for that book. [Candidate workflow](../developer-workflow.md#release-candidates-333) · [Release gates](../roadmap.md#before-the-v010-tag) |

## What to learn from using the app

For any case encountered, record the task, which date the user meant, what the screen or export actually showed and what would have let them finish. Synthetic examples are sufficient to describe an observation publicly. These are prompts for the later decision. Completing the table is optional for the household run.

| Household task | Question the example can answer |
|---|---|
| Trace a December transfer that arrives in savings in January, after the bank renumbers the account. | Can the owner find the original account by its former last four digits and match each leg to its statement? What belongs on a year-end balance versus a list of movements? Use the existing recognition-hint limit. Two accounts can share those digits. |
| Reconcile a brokerage statement after a fund changes its name or ticker. | Can the owner recognize the original trade under the statement’s name and find the same holding under today’s name? Distinguish a rename from a merger or exchange that creates a different instrument or affects lots. |
| Rename a merchant, then change its suggested expense category. | Can the owner find last year’s payment by the printed or familiar name? Does the new suggestion help with the next payment while the original posting keeps its category? Try a second merchant that reuses the first name. |
| Move childcare from “Household” to “Family” after saving last year’s spending export. | Which grouping is wanted for a fresh comparison? Compare its subtotal with the retained export and check that the underlying transactions and total still agree. Decide separately whether the exported copy meets the filing need. |
| Correct a typo in a nickname or clear a private note. | Does any workflow require the old text? Distinguish a simple correction from a real change on a known date. Consider how long removed text would survive in versions, backups or a proposed audit event. |

Transaction responses resolve posting account names on the owner day. The ledger report account query selects name, code and structure at the supplied date. Spending can resolve a category through today’s catalog and exports the label it displayed. Compare a fresh report for a past period with a copy actually retained at filing. [Transaction enrichment](../../backend/internal/app/transactions_service.go) · [Ledger account lookup](../../backend/internal/db/ledger.go) · [Spending screen and CSV](../../frontend/src/lib/reports/spending-view.svelte)

## Decision after testing

Record the owner’s decision on #611, including a decision to leave things as they are. For each observed gap, name the field, the workflow that uses its previous value and the relevant date (posting, report end or time of generation). Try the smallest change that meets the need. Search by a former identifier, a more accurate report label, and a saved report each solve a different problem. A stable record ID still identifies the account, payee or instrument when names coincide.

Only propose changing storage if the existing versions or a change to the reader cannot meet a demonstrated need. Before doing so, trace affected writers, search and import matching, reports and exports, the account and institution versions endpoints where relevant, and the upgrade path for any supported book. If descriptive audit is proposed, the owner limited it to database investigations, without an app reader, new audit columns or indexes. Keep removed note contents out of any new audit payload. An investigation log is a separate decision from searchable identifiers and filed reports.
