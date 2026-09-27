# Competitor comparison

Reviewed 2026-09-27 against the vendors' own documentation and the current
[Rekenraam feature ledger](implemented.md). This is a decision aid, not a claim
that an unlisted feature is absent. It compares workflows relevant to a
single-user, self-hosted, multi-currency finance app. Prices, popularity,
security quality, and features behind particular paid tiers were not assessed.
The earlier broad survey remains in
[the July research record](reviews/competitive-analysis-2026-07.md).

## Verified comparison

| Product | Documented strengths relevant here | Boundary or implication for Rekenraam |
|---|---|---|
| **Rekenraam** | Exact multi-commodity ledger, reconciliation, CSV/QIF export and import, budgets, recurring drafts, per-currency forecasts, operational investment lots/gains and Trading 212 import. | Investment-native correction, transfers, basis actions, named short sales, price management UI, and reproducible gains projections remain open. See [implemented](implemented.md) and [roadmap](roadmap.md). |
| **GnuCash** | Its [investment guide](https://www.gnucash.org/docs/v5/C/gnucash-guide/chapter_invest.html) documents lots, dividends, return of capital, splits and mergers, and a price database. | Sets the accounting and transaction-type bar for R16. The guide is for a desktop application; this comparison does not equate its lot methods or tax results with Rekenraam's. |
| **Quicken Classic** | Its [investment action list](https://info.quicken.com/win/tell-me-about-the-investment-transaction-list-s-ac) names transfers, return of capital, splits, short sales and covers; its [placeholder guide](https://www.quicken.com/support/resolving-placeholders-and-usd0-00-cost-basis-in-quicken-for-mac/) explains missing-basis recovery. | A migration target needs basis-preserving transfers, correction and explicit unknown-basis handling before it can promise comparable investment history. Quicken's [short-cover help](https://info.quicken.com/win/how-do-i-cover-a-short-sale) reinforces naming shorts separately from ordinary sells. |
| **Portfolio Performance** | Its [security menu](https://help.portfolio-performance.info/en/reference/view/securities/context-menu/) documents security transfers and a split wizard; its [performance manual](https://help.portfolio-performance.info/en/reference/view/reports/performance/dashboard/) documents TTWROR and IRR. | R16's split/transfer gap and later R13 returns gap are concrete. Its default split path retroactively adjusts earlier transactions and quotes; Rekenraam's immutable-source design calls for a dated operation and separate adjusted views. |
| **Ghostfolio** | The project's [README](https://github.com/ghostfolio/ghostfolio/blob/main/README.md) documents self-hosting, activity import/export, multi-account holdings, portfolio charts, ROAI periods and a mobile-first PWA. | Portfolio analytics and mobile presentation are relevant R13 benchmarks. Its documented activity/API scope does not establish equivalent ledger or basis semantics, so no absence claim is made here. |
| **Actual Budget** | Its [rules](https://actualbudget.org/docs/budgeting/rules/) can transform imports, its [schedules](https://actualbudget.org/docs/tour/schedules/) support automatic or reviewed entry, and its [import guide](https://actualbudget.org/docs/transactions/importing/) lists CSV/QIF/OFX/QFX/CAMT. Its [multi-currency guide](https://actualbudget.org/docs/budgeting/multi-currency/) says native support is still absent and describes an experimental workaround. | Rekenraam's exact multi-currency model is a meaningful difference. Actual's broader import formats and rules are a later usability benchmark; its documented schedules reinforce a clear review-before-post workflow. |
| **Firefly III** | Its [rules](https://docs.firefly-iii.org/how-to/firefly-iii/features/rules/), [data importer](https://docs.firefly-iii.org/how-to/data-importer/import/csv/) and [budgets](https://docs.firefly-iii.org/how-to/firefly-iii/finances/budgets/) cover mature routine transaction management. | Keep R6 import depth and rules on the later list. This review did not verify comparable security-lot accounting, so it makes no claim about its presence or absence. |
| **PocketSmith** | Its [multi-currency guide](https://pocketsmith.helpkit.so/multi-currency/6a6X8SseDBfyRznV6vJTS3/multi-currency-an-overview/6a6X8SseDAA5o85oTGHWtJ) and [investment-account guide](https://learn.pocketsmith.com/net-worth-assets--debts/6a6X8SseDAohQfjgQVP5mj/managing-investment-accounts/6a6X8SseDCNBiAKmvJS9uG) document cross-currency planning and alternative ways to represent investments. | Rekenraam's R10 forecast is already a differentiated, exact per-currency base. No lot-level parity or pricing claim is inferred from these guides. |
| **Monarch Money** | Its [manual holdings guide](https://help.monarchmoney.com/hc/en-us/articles/10032888165140-Manual-investment-holdings) documents market-priced holdings and purchased cost; its [CSV import guide](https://help.monarchmoney.com/hc/en-us/articles/4409682789908-Import-data-manually-from-banks-or-other-finance-apps) limits that import to bank and card transactions and says imports cannot be undone. Its [manual transaction guide](https://help.monarchmoney.com/hc/en-us/articles/360058441811-Manual-transactions) says it lacks reconciliation. | Shows that a polished general finance app can have a different investment and trust boundary. Rekenraam should emphasize auditable import, reconciliation and investment history in migration examples without asserting feature-wide superiority. |

## Priority decisions from this review

1. **Complete R16 before more investment producers.** The competing investment
   workflows name correction, transfer, return of capital and splits as normal
   operations. Rekenraam currently cannot safely amend an old trade or carry
   basis between brokers. Preserve the accepted R16 order: correction, then
   transfers and basis actions, then named short sales and compound actions.
2. **Promote R11 price management UI after R16 and before R17.** Rekenraam has
   price storage and voiding, but no complete operator surface. A user needs to
   inspect provenance, correct a quote and see valuation coverage before
   additional quote adapters make price history denser. R17 still owns the
   shared `PriceProvider` registry and crypto instrument entry.
3. **Keep R18 gains projections after R17; keep R13 returns after R18.** R18
   needs a named quote/source and staleness policy. R13's TTWROR/IRR and
   allocation views need trustworthy operation history, dated valuation and
   transfer classification. Portfolio Performance and Ghostfolio provide a
   concrete UX benchmark; Rekenraam should specify its own equations and
   avoid promising tax compliance.
4. **Treat locale input and owner-local dates as trust work.** G-08, T-80 and
   T-87 affect the cross-border persona directly. Finish parsing and date
   defaults before calling the five drafted translations complete or using
   them in a migration demo.
5. **Keep R6 and broader feeds later.** Actual and Firefly III document
   richer import automation, but Rekenraam already has usable CSV/QIF entry.
   No official source in this review justifies moving broader integrations
   ahead of the unresolved investment and valuation lifecycle.

These priorities are reflected in the [roadmap](roadmap.md). Recheck source
pages before making market-wide uniqueness or release-parity claims.
