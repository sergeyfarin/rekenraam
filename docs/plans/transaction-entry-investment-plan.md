# Investment transaction entry (#507)

Replace the 17 launch buttons with one localized, grouped native Activity selector in New transaction. Keep existing investment commands, corrections, exact amounts and review guards. Portfolio entry uses the shared New transaction opener.

## Account contract

The model permits direct brokerage_cash children of a brokerage but does not require a holding to have a brokerage parent. Suggestions use the selected holding’s direct brokerage parent (or starting brokerage container), its direct brokerage_cash children, and the starting cash account. No institution inference, import mapping or stored default. A sole eligible asset candidate is suggested; liabilities and ambiguous sets require an explicit choice. A manual choice survives refresh and context changes; an invalid choice is explained and cannot save.

One composed dated account read shares the investment command role predicate and supplies eligibility and versioned currency/hierarchy metadata. No per-account requests. Status notifications reach both observers when holding and settlement share one query, including failures with no data; the retry path is covered in the browser. Writes still revalidate their dependencies. General account-catalog pagination and direct-register context beyond the legacy 2,000-account cap are tracked separately in #518.

Carry the ordinary entry’s date and memo into the chosen activity, never its amount. No-cash activities expose no settlement picker; reinvestment chooses its cost currency directly. Successful investment saves close the entry and retain a visible, announced confirmation on the containing screen. A write-off retains its specific confirmation. Full restoration and semantic carry between activities remains #508.

## Validation

Cover account-role parity and historical eligibility; candidate none/one/many/liability/dedup and sticky manual choices; grouped localized activities; bank-to-investment context, no-cash entry, existing investment flows, keyboard, phone, themes and locales. Run scoped checks before the full frontend, integrated build and backend gates.
