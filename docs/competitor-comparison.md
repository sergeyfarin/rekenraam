# Competitor comparison

Reviewed 2026-10-17 against the vendors' own documentation and the current
[Rekenraam feature ledger](implemented.md). This is a decision aid, not a claim
that an unlisted feature is absent. It compares workflows relevant to a
single-user, self-hosted, multi-currency finance app. Prices, popularity,
security quality, and features behind particular paid tiers were not assessed.
The earlier broad survey remains in
[the July research record](reviews/competitive-analysis-2026-07.md).

Local status was reconciled at `f35cc723`; see the
[app-state review and proposed audits](reviews/app-state-and-audit-plan-2026-10-17.md).
Official pages for the products below were rechecked on 2026-10-17. Firefly III
was available through indexed official documentation; direct retrieval failed,
so current release/tier details are unverified. The export/backup section was
added on 2026-10-18 from the same kind of sources. Provider signup, feed availability
and licensing are separate checks. Documentation supports a workflow comparison,
not measured usability, equivalent financial semantics or market demand.

Rekenraam's R13 status was reconciled on 2026-10-26 at `b9c57ea3`; the
[accepted delivery record](plans/investment-returns-plan.md#accepted-delivery-and-follow-up-disposition-2026-10-26)
now replaces the earlier local returns gap. This updates local capability
status only; competitor sources retain their original review dates.

The [self-hosted newcomers section](#self-hosted-newcomers-2026-11-14) was
added on 2026-11-14. It is the one place this document records popularity,
and it reads repository schema files rather than vendor documentation.

## Verified comparison

| Product | Documented strengths relevant here | Boundary or implication for Rekenraam |
|---|---|---|
| **Rekenraam** | Exact multi-commodity ledger, reconciliation, CSV/QIF export and import, budgets, recurring drafts, per-currency forecasts, investment lots/gains and Trading 212 import. | Native correction, transfers, structural actions, historical entry, unknown-basis resolution/recovery, named shorts, price management and crypto quotes ship. R18 adds dated gains, period statements, alternative/person-pooled methods, operational FX and immutable operational runs. Live-read consistency, capital-return disclosure and unclassified imported dividend cash disclosure ship (#237/#238/#246); reviewed Twelve Data dividend suggestions and named short payment in lieu also ship; tax rules and TWR/IRR remain outside R18. See [implemented](implemented.md) and [roadmap](roadmap.md). |
| **GnuCash** | Its [investment guide](https://www.gnucash.org/docs/v5/C/gnucash-guide/chapter_invest.html) documents lots, dividends, return of capital, splits and mergers, and a price database. | Sets the accounting and transaction-type bar for R16. The guide is for a desktop application; this comparison does not equate its lot methods or tax results with Rekenraam's. |
| **Quicken Classic** | Its [investment action list](https://info.quicken.com/win/tell-me-about-the-investment-transaction-list-s-ac) names transfers, return of capital, splits, short sales and covers; its [placeholder guide](https://www.quicken.com/support/resolving-placeholders-and-usd0-00-cost-basis-in-quicken-for-mac/) explains missing-basis recovery. | A migration target needs basis-preserving transfers, correction and explicit unknown-basis handling before it can promise comparable investment history. Quicken's [short-cover help](https://info.quicken.com/win/how-do-i-cover-a-short-sale) reinforces naming shorts separately from ordinary sells. |
| **Portfolio Performance** | Its [security menu](https://help.portfolio-performance.info/en/reference/view/securities/context-menu/) documents security transfers and a split wizard; its [performance manual](https://help.portfolio-performance.info/en/reference/view/reports/performance/dashboard/) documents TTWROR and IRR. | R16's split/transfer and historical-entry/recovery workflows ship; R18 adds dated basis/gains and saved operational documents. R13 now adds named TTWROR/IRR contracts, allocation, benchmarks, instrument returns and saved returns runs. Its default split path retroactively adjusts earlier transactions and quotes; Rekenraam retains dated operations and immutable source history. No formula-by-formula equivalence is claimed. |
| **Ghostfolio** | The project's [README](https://github.com/ghostfolio/ghostfolio/blob/main/README.md) documents self-hosting, activity import/export, multi-account holdings, portfolio charts, ROAI periods and a mobile-first PWA. | Portfolio analytics and mobile presentation are relevant R13 benchmarks. Its documented activity/API scope does not establish equivalent ledger or basis semantics, so no absence claim is made here. |
| **Actual Budget** | Its [rules](https://actualbudget.org/docs/budgeting/rules/) can transform imports, its [schedules](https://actualbudget.org/docs/schedules/) support automatic or reviewed entry, and its [import guide](https://actualbudget.org/docs/transactions/importing/) lists CSV/QIF/OFX/QFX/CAMT. Its [multi-currency guide](https://actualbudget.org/docs/budgeting/multi-currency/) says native support is still absent and describes an experimental workaround. | Rekenraam's exact multi-currency model is a meaningful difference. Actual's broader import formats and rules are a later usability benchmark; its documented schedules reinforce a clear review-before-post workflow. |
| **Firefly III** | Its [rules](https://docs.firefly-iii.org/how-to/firefly-iii/features/rules/), [data importer](https://docs.firefly-iii.org/how-to/data-importer/import/csv/) and [budgets](https://docs.firefly-iii.org/how-to/firefly-iii/finances/budgets/) cover mature routine transaction management. | Keep R6 import depth and rules on the later list. This review did not verify comparable security-lot accounting, so it makes no claim about its presence or absence. |
| **PocketSmith** | Its [multi-currency guide](https://pocketsmith.helpkit.so/multi-currency/6a6X8SseDBfyRznV6vJTS3/multi-currency-an-overview/6a6X8SseDAA5o85oTGHWtJ) and [investment-account guide](https://learn.pocketsmith.com/net-worth-assets--debts/6a6X8SseDAohQfjgQVP5mj/managing-investment-accounts/6a6X8SseDCNBiAKmvJS9uG) document cross-currency planning and alternative ways to represent investments. | Rekenraam's R10 forecast is already a differentiated, exact per-currency base. No lot-level parity or pricing claim is inferred from these guides. |
| **Monarch Money** | Its [manual holdings guide](https://help.monarch.com/hc/en-us/articles/10032888165140-Manual-Investment-Holdings) documents priced holdings and separates quantity tracking from manually entered balances. Its [CSV guide](https://help.monarch.com/hc/en-us/articles/4409682789908-Importing-Transactions-Manually) now documents overlap choices and transaction-ID matching. Its [manual transaction guide](https://help.monarch.com/hc/en-us/articles/360058441811-Creating-Manual-Transactions) says it lacks reconciliation. | Shows that a polished general finance app can have a different investment and trust boundary. Rekenraam should emphasize auditable import, reconciliation and investment history in migration examples without asserting feature-wide superiority. |

## Export, backup and restore

Added 2026-10-18, after the [#277 recovery audit](reviews/recovery-audit-2026-10-18.md)
and the product decision that the native backup is Rekenraam's only
full-installation round trip (`product-requirements.md`). "Re-importable" means
the vendor documents loading its own export back into the same product.

| Product | Complete copy and restore | Portable export | Export re-importable? |
|---|---|---|---|
| **Rekenraam** | Nightly and on-demand SQLite backups made by the running app, verified before they are kept, with retention and a chained ledger self-check. `rekenraam verify-backup` checks one, including whether sealed data opens with the configured key. `rekenraam restore` installs one under the server lock and keeps the previous database. The #277 drill restores the reference book exactly. The encryption key is deliberately not in the backup. | Ledger CSV, QIF (cash accounts) and an archive with a manifest of checksums and named exclusions, a trial balance, lots and the investment evidence files. Every balance, lot and gain can be rebuilt from it. | **No, by decision.** The Data screen says so and points to backups. |
| **Actual Budget** | Desktop **Backup Now** ([backup guide](https://actualbudget.org/docs/backup-restore/backup/)). Settings → Export data saves the budget, and Import file restores it ([restore guide](https://actualbudget.org/docs/backup-restore/restore/)). | The same export. The API also has `exportBudget`/`importBudget` ([API reference](https://actualbudget.org/docs/api/reference/)). | **Yes:** one file serves as both backup and transfer. |
| **Firefly III** | No built-in backup routine; the operator backs up the database, `.env`/`APP_KEY` and uploads ([backup how-to](https://docs.firefly-iii.org/how-to/firefly-iii/advanced/backup/)). | CSV export from the web or the CLI ([exporting data](https://docs.firefly-iii.org/tutorials/firefly-iii/exporting-data/), [CLI](https://docs.firefly-iii.org/references/firefly-iii/cli/)). | **No.** Its docs say the export is not a backup and cannot be imported (indexed official text; direct retrieval failed). |
| **Ghostfolio** | The [README](https://github.com/ghostfolio/ghostfolio/blob/main/README.md) documents PostgreSQL and automatic migrations, but no backup routine. Database backup is the operator's job. | Activity import/export API (`/api/v1/import`, `/api/v1/export`). | The import takes activity JSON; whether the export is complete is not documented, so no round-trip claim is made. |
| **GnuCash** | An XML book keeps a timestamped copy on every save, plus change logs, deleted after 30 days by default. SQL backends make no backup files ([backup guide](https://www.gnucash.org/docs/v5/C/gnucash-guide/basics-backup1.html), [wiki](https://wiki.gnucash.org/wiki/Backup)). | CSV export of the account tree and of transactions; QIF is import-only ([FAQ](https://code.gnucash.org/wiki/FAQ)). | The book file itself is the round trip; the CSV export is not documented as one. |
| **Portfolio Performance** | Optional backup copy on every save, plus interval autosave ([preferences](https://help.portfolio-performance.info/en/reference/help/preferences/)). | CSV (transactions, securities, quotes) and Portfolio Performance XML ([File → Export](https://help.portfolio-performance.info/en/reference/file/export/)). | The XML is its own file format; CSV is for other tools. |
| **Quicken Classic** | Backups hold reports, passwords, accounts and attachments; restoring replaces the current file ([backup and restore](https://www.quicken.com/support/how-backup-or-restore-your-quicken-data)). | QXF moves non-investment accounts, scheduled transactions, categories and tags, but not investments, budgets, attachments, reports or settings ([QXF](https://info.quicken.com/win/how-do-i-export-data-to-a-qxf-file)). QIF for the rest. | **Partly:** QXF between Quicken files, within those limits. |
| **PocketSmith** | **Backup and restore** CSVs for categories, rules and transactions, restored in that order. Budgets, attachments and saved searches are excluded ([backup and restore](https://learn.pocketsmith.com/article/1455-backup-and-restore-your-pocketsmith-data)). | Transaction export as XLSX/CSV ([exporting](https://learn.pocketsmith.com/article/158-exporting-your-transactions)). | **Yes,** for the backed-up data types. |
| **Monarch Money** | Hosted service; no owner-held backup is documented. | Transaction and balance history as CSV, available even after a subscription ends ([downloading history](https://help.monarch.com/hc/en-us/articles/15526600975764-Downloading-Transaction-or-Account-History)). | **Partly:** re-importing with the transaction-ID column updates matching transactions ([CSV import](https://help.monarch.com/hc/en-us/articles/4409682789908-Importing-Transactions-Manually)). |

What this means for Rekenraam:

- **Backups are a strength among self-hosted peers.** Firefly III and Ghostfolio
  leave backup to the operator. Rekenraam schedules, verifies, prunes and
  self-checks its own backups, and it ships verify and restore commands with a
  proven drill. Desktop products back up their own file on save, which a server
  app cannot rely on.
- **The export carries accounting evidence** (an exact trial balance, a
  checksummed manifest, named exclusions, lot and gain facts) that none of the
  pages reviewed here document for their exports. This is not a claim about
  undocumented features. Unlike Actual, PocketSmith or Portfolio Performance's XML, it is not
  re-importable. That is a deliberate boundary. Owners coming from Actual
  expect one file to do both jobs, which is why the Data screen and README now
  say plainly that an export is not a backup, as Firefly III's documentation
  does.
- **Restore is operator-run, not a screen action.** Actual, Quicken and
  PocketSmith restore from inside the app. Rekenraam restores with a CLI command
  that holds the server lock and preserves the previous database, which is
  safer for a running server but less approachable. This is an observation for
  the household dry run, not an accepted gap; any in-app restore would need its
  own decision.
- **Key custody.** Firefly III likewise tells the operator to back up `APP_KEY`
  apart from the database. None of the pages reviewed document a check, like
  `verify-backup`'s, that a backup's sealed data opens with the configured key.

## Priority decisions from this review

1. **R16's agreed delivery bar is complete (2026-10-13).** #146 adds
   journal-free sourced splits and entitlement replay to the shipped long,
   short and compound-action workflows. This gains zero-delta split recording;
   inspecting/correcting/settling initially journal-free splits by operation
   identity also ships in #196, including mobile history and import dedupe. The competing
   investment workflows treat corrections, transfers, basis actions and splits
   as ordinary operations. Rekenraam's bounded #114 and #166–#168 gates, named
   shorts and compound actions are delivered; provider/tax-policy completeness
   remains separate.
2. **R11 and R17 now supply pricing operations and instrument entry (2026-10-14).**
   Users can inspect/correct observations and coverage, create crypto holdings
   with lots, import their trades through CSV, and opt into a shared quote
   registry with durable refresh and visible failures. This gains priced crypto
   entry parity; exchange/DeFi/staking/NFT coverage remains deliberately absent.
   Provider keys, EU market/display entitlements and unofficial Yahoo availability
   remain operator boundaries, not product coverage promises. See the
   [provider evidence and contract](plans/quote-providers-crypto-plan.md).
3. **R18's six gains-projection slices are delivered (2026-10-15).** Named
   quote/staleness rules, dated positions and period results, method comparisons,
   operational FX and immutable operational runs gain explicit valuation and
   basis inspection. Saved alternative/FX runs, broker-basis comparison and
   jurisdiction tax rules remain outside that delivery. #237/#238 are delivered;
   R13's accepted TTWROR/IRR, allocation, benchmark and per-instrument views
   followed by 2026-10-26, with immutable saved output. Targets/drift (#411)
   and advisory rebalancing (#412) remain deliberately deferred. This closes
   the local implementation gap without claiming identical vendor semantics.
4. **Treat locale input and owner-local dates as trust work.** Locale amount
   parsing (#100) and owner-local date defaults (#101) are delivered. Native
   terminology review (#102) remains before calling the five drafted
   translations complete or using them in a migration demo.
5. **Keep R6 and broader feeds later.** Actual and Firefly III document
   richer import automation, but Rekenraam already has usable CSV/QIF entry.
   No official source in this review justifies moving broader integrations
   ahead of validating the delivered core.

These priorities are reflected in the [roadmap](roadmap.md). Recheck source
pages before making market-wide uniqueness or release-parity claims.

## Consequences of the October recheck

The October comparison identified an R13 gap, since delivered locally under
named contracts; different products' metrics still require separate comparison:
Portfolio Performance documents [TTWROR](https://help.portfolio-performance.info/en/concepts/performance/time-weighted/),
while Ghostfolio's linked README calls its period measure ROAI. These are not
interchangeable labels for the same output. Rekenraam now has an accepted
formula, external-flow, allocation-denominator and missing-value contract with
independent expected cases. Allocation targets (#411) and advisory rebalancing
(#412) still require their own contracts; delivered returns do not imply them.

The July survey and September strategy review remain dated research. Their
claims of category leadership, uniqueness, demand, prices or willingness to
pay are not established by this workflow recheck. The September recommendation
to validate migration and daily use before broadening scope remains a proposal
worth testing through the household dry run; it does not authorize AI/MCP,
desktop distribution, tax compliance or a hosted product.

Actual's richer imports/rules, Firefly III's routine management and PocketSmith's
cross-currency planning remain reasons to test the whole daily workflow, even
though the recent delivery effort concentrated on investments. The roadmap's
audit coverage includes those areas. Market-wide parity is not a release gate.

## Self-hosted newcomers (2026-11-14)

Added after the owner asked whether newer self-hosted projects make Rekenraam
redundant, and whether joining one would be better than continuing. Because
the question is about the field, this section records stars and contributor
counts, which the rest of this document leaves out. Counts come from the
GitHub API on 2026-11-14.

Each row names its evidence. "Models" means the schema or model files were
read at the commit given. "README" means only the project's own description
was read. Nothing was installed or run. No claim is made about code quality,
security, or behaviour beyond the columns read, and an unread feature is not
an absent one.

Field size: GitHub search returned 6,010 repositories tagged
`personal-finance` created since 2026-01-01, and 288 tagged both
`self-hosted` and `finance`. Topic tags are self-assigned and include guides,
tax tools and chat bots, so these numbers bound the activity; they do not
count comparable apps. The descriptions of the most-starred 2026 entries lead
with an AI agent, an MCP server or bank sync.

| Project | Size and age | Evidence | What the evidence shows |
|---|---|---|---|
| **[Sure](https://github.com/we-promise/sure)** (community fork of Maybe) | 15,144 stars. Forked 2025-07-23. Several maintainers. 929 open issues and pull requests. Ruby, PostgreSQL, AGPL-3.0. | Models at `c9cc6d24` (`db/schema.rb`). | An `entries` row holds one account and one `decimal(19,4)` amount, with a reconciled status and a statement reference. A transfer pairs an inflow and an outflow transaction. `holdings` are dated rows with one `cost_basis` each; `trades` hold quantity, price and fee. No lot table was found in the tables read. |
| **[ezBookkeeping](https://github.com/mayswind/ezbookkeeping)** | 6,073 stars. Created 2020. One author has 3,627 of about 3,745 commits. Go with SQLite, MySQL or PostgreSQL. MIT. | README. | Imports CSV, Excel, OFX, QFX, QIF, IIF, CAMT.052/053, MT940, GnuCash, Firefly III and Beancount files. Scheduled transactions, receipt recognition, an MCP server, PWA. The README does not mention double entry, investments or reconciliation. The closest stack to Rekenraam's. |
| **[Securo](https://github.com/securo-finance/securo)** | 4,499 stars. Created 2026-03-08. Top contributor has 333 of 729 commits; about 99 contributors. 273 open issues and pull requests. Python, PostgreSQL, AGPL-3.0. | Models at `83301a15` (`backend/app/models`). | A `transactions` row holds one account, a `Numeric(15,2)` amount, a debit/credit type and an optional `transfer_pair_id`. `asset_transactions` are buys and sells with quantity, price and fee; no lot model is among the model files. Its `reconciliation_*` tables store rules and suggestions for matching transactions to expected items, not statement checkpoints. README: bank sync through Pluggy, Enable Banking and SimpleFIN; OFX, QIF, CAMT and CSV import; rules, budgets, goals, workspaces with roles, OIDC, passkeys, optional LLM agents and plugins. |
| **[open-net-worth](https://github.com/open-net-worth-ai/open-net-worth)** | 482 stars. Created 2026-10-15. One author has 1,025 of about 1,040 commits. Python, AGPL-3.0. | README. | Describes "a real multi-currency ledger" with transfers, splits, reconciliation and reviewed statement import, operated by an AI agent that can also act through MCP servers, including placing trades. None of this was checked against its code. |
| **[MoneyMatter](https://github.com/letehaha/moneymatter)** | 239 stars. Created 2025-02-19. One author has 2,776 of about 2,880 commits. TypeScript, PostgreSQL, Redis, AGPL-3.0. Its README offers a paid hosted plan. | Models at `906e7064` (`packages/backend/src/models`). | A transaction holds one account with `amount` and `refAmount` as `BIGINT` cents. A holding stores one `quantity`, `costBasis` and `refCostBasis` as `DECIMAL(20,10)`; a code comment derives the average price as cost basis divided by quantity. Its "reconciliation" endpoints remove, restore and merge duplicate transactions. README: bank sync through Monobank, SimpleFIN, LunchFlow and Walutomat; CSV, OFX, QFX, YNAB, Wallet and Microsoft Money import; AI statement parsing; splits, attachments, budgets, subscriptions, loans, sharing and an MCP server. |

Two further projects are noted without a row.
[Paisa](https://github.com/ananthakumaran/paisa) (3,236 stars) describes
itself as built on the ledger double-entry tool, so it belongs with the
plain-text-accounting audience that adoption asset #335 already addresses.
[FireFunds](https://github.com/nicolopozzato/firefunds) (created 2026-10-19,
18 commits, no stars) claims double entry with average-cost investments and
lists imports and reconciliation as planned; it is the only 2026 entry found
that leads with the ledger.

What this means for Rekenraam:

- **The ledger and lot model is not matched in the three schemas read.** Sure,
  Securo and MoneyMatter store a cash transaction as a row on one account and
  carry investment basis as one figure per holding. A balanced journal, lots,
  dated corrections and statement checkpoints would need a redesign there, not
  an added feature. This describes the files read. It is not a uniqueness claim.
- **Exact amounts are not a difference.** All three use integer cents or fixed
  decimals. Securo's two-decimal amount column is a boundary for currencies and
  commodities with other scales; whether that matters in use was not tested.
- **Adoption has not followed ledger depth.** Securo gathered its stars in
  eight months on a single-entry model. The fast-growing projects lead with
  bank sync, wide import format lists, rules and AI or MCP access. Those are
  the areas Rekenraam has placed deliberately later (R6, feeds) or has not
  authorized (AI/MCP).
- **Joining one is not a practical option.** The stacks differ, most of these
  projects are also one maintainer's work, and contributing would mean writing
  for a model that cannot carry this ledger. Cooperation is realistic only as
  interoperability, for example importing their exports.
- **Rekenraam has no adoption evidence yet.** The repository is public with one
  star and no tagged release. The
  [public-release gates](roadmap.md#public-release-gates), not feature parity,
  are what stand between the delivered core and any outside user.

Proposals raised by this review. None is accepted, and the roadmap is
unchanged:

1. Keep the `v0.1.0` and announcement gates ahead of further investment depth.
   This agrees with the roadmap's current order.
2. Consider importers for Firefly III, Actual and Sure exports as migration
   paths when R6 is scheduled.
3. Decide explicitly whether a read-only MCP server fits the read-only stance
   toward outside systems. The October recheck above did not authorize one;
   this review only shows that the 2026 field treats it as expected.
4. Set a checkpoint some months after announcement. If no outside use appears,
   weigh keeping Rekenraam as the owner's own tool against further
   public-facing work.
