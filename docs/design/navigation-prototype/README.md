# Navigation prototype — 2026-10-29

Browser-openable companion to the approved [Home and navigation design](../home-and-navigation.md).
It extends the [v3 Home canvas](../home-mockup/README.md) with destinations so
the header, scoped object search and everyday journeys can be reviewed together.
This is implementation preparation for the slices tracked in #441 (shell #436, Home #437,
moved pages #446–#448, object navigation #442), not delivery of any of them.

Home was brought back in line with the approved design and canvas on
2026-10-29 (see [Home in this prototype](#home-in-this-prototype)).

Open [index.html](index.html) directly in a browser. No server, canvas runtime,
credentials, network access or build is needed. All data is synthetic. Local
browser storage remembers only this prototype's theme and amount masking; entry,
import and reconciliation controls never write financial data. The original
approved canvas remains the reference for Home styling and personalization.

## Home in this prototype

Home follows the [approved design](../home-and-navigation.md#home-layout) and
the v3 canvas: two briefing lines (headline with ▾ switcher, and a status
line linking to the Review page); the command bar inline
under the briefing, with grouped results (*Payee*, *Pages, accounts and
categories*, *Everything*) and *Narrow to* chips; shortcut tiles led by Add
transaction with ⋯ menus and *Add shortcut*; My Home with the five default
widgets at their real content (bars, budget rows, lists, sparklines); More to
explore with every other widget at real size, grouped by category, each with
*Add to My Home*. Widget and shortcut ⋯ menus, Undo and the Customize panel
work in memory and reset on reload.

Deliberate differences from the canvas, recorded in the design document:

- The Accounts widget lists the pinned accounts (the Ledger menu's list), not
  a separate "largest balances" ranking.
- On phones, tapping the command bar opens the same search full screen.
- An empty book shows one *Start with an account* card above My Home, and
  widgets show *No data yet* instead of sample values.
- The Prices widget covers security prices only; FX rates stay in Settings.

## Second owner review (2026-10-29)

After the owner's ten comments: widgets are denser and section headings carry
no counts; the brand is the Home link; accounts are grouped by institution
with an *Institutions* view; a brokerage is one account showing its cash and
positions (no holding accounts in lists); page actions say *Add transaction*
(brokerages add *New transaction*, which opens the investment type chooser); "Trash" became Transactions ›
*Deleted*; Settings has five pages, and *Preferences* holds language, theme
and the Home page settings. The full capability map is in the
[navigation review](../../reviews/navigation-and-pages-options-2026-10-29.md#coverage-review-after-the-prototype-2026-10-29).

## What to review

- All approved section/settings menu entries lead to distinct destinations.
  Ledger includes Payees; Gains has one canonical Investments destination and
  a Reports menu entry marked *in Investments*; a section's tab row lists only
  its own pages. Settings pages use a side list, not a tab row. Prices and
  currency/FX settings remain separate.
- Checking and Savings registers retain applicable URL filters when switched
  using the title's scoped search. Removing the scope chip changes the same
  search into global navigation. Closed accounts are one checkbox away;
  holding accounts appear when searched. Pins are sample choices, not usage-ranked.
- Spending links to category details; Home review links to an import batch
  and recurring drafts; Reconcile has explicit preview and sample finish.
- Add is available on registers, Transactions and category pages, including
  phones. Escape with unsaved sample input asks whether to discard it; no
  search action saves an entry.
- The phone menu opens, local page tabs scroll, and scoped search becomes
  full screen. Header menus use disclosures; search keeps input focus and
  exposes the selected option to assistive technology.
- The sample-book selector exposes fresh-book setup, unavailable valuation,
  and failed attention checks. A failed check never says All clear.
- Legacy Reports/Investments URL selections and dividend-review links retain
  other filters. Native Back, forward, copied links and new tabs remain usable.
  Returning to Home restores its review/explore expansion and scroll.

## Bounded walkthrough

1. On Home, type `albert` in the command bar and choose *Add expense at
   Albert Heijn*; check the prefilled payee and category, then cancel. Remove a
   widget with its ⋯ menu, undo, add one from More to explore, and reset in
   Customize.
2. From Home, open Ledger (its Overview), then the pinned Checking account. Apply March and a search filter.
3. Open Checking ▾, type Savings and press Enter. Check that the applicable
   filters remain. Remove the scope chip and observe that results become global.
4. Open Add; check the selected account, type a payee, then press Escape.
   Retain the input or discard deliberately. Sample Save confirms the interaction
   without adding a journal or changing the displayed fixture balances.
5. Follow Reconcile this account, preview, then finish the sample. Switch the
   account before previewing to check context.
6. Return Home, follow the status line to Review and continue the import batch. Choose a category
   and review the sample changes; the batch remains uncommitted.
7. Open Spending, drill into Groceries, then use Back twice. Check Home's
   scroll position. Repeat on a 390px phone and with only a keyboard.
8. Try both themes, hide amounts, the alternate sample books and
   320px reflow. Open a pin or report link in a new tab.

Record task time, wrong turns, context loss and whether each figure's scope
was understood against today's app for the same tasks. No owner task timings,
real-book counts or usability acceptance have been recorded by building this
artifact. D11's real-book inventory (#449) remains #442's prerequisite for final grouping.

## Validation

Use the already installed repository browser dependencies; no new packages:

```sh
pnpm exec node docs/design/navigation-prototype/check.mjs
# Optional screenshots outside the source tree:
pnpm exec node docs/design/navigation-prototype/check.mjs /tmp/rekenraam-navigation-prototype
```

The focused check exercises links and context retention, editor cancellation,
review and reconciliation, legacy URLs, Back restoration, the Home command
bar and payee prefill, widget moves with Undo and Customize reset, amount
masking, 42 axe/reflow cases over seven representative pages at 1440/390/320px
in both themes, open switchers, browser errors and absence of remote requests.
This checks the design artifact only. It is not production integration,
financial-correctness validation or native terminology sign-off.

Passed locally on 2026-10-29 after the Home revision: 28 menu/pin
destinations, the walkthrough's automated journeys, all 36 page axe/reflow
cases (42 after the second review) and three open switchers,
masking and zero remote requests. JavaScript syntax, local Markdown targets
and `git diff --check` also passed. Acceptance was tightened in
[#436](https://github.com/sergeyfarin/rekenraam/issues/436) and
[#437](https://github.com/sergeyfarin/rekenraam/issues/437) for retained route
context, companion object navigation, attention failure/counting contracts,
masking, composed Home loading/refresh and unsaved editor snapshots. Both
issues remain open for application implementation and its acceptance evidence.

## Deliberate limits

Widget settings, sizes, drag reordering and saving the layout are not built
(#438). Most other destinations identify placement
and link to relevant objects rather than reproduce their full screens.
Transfer, posting recurring drafts, uploads, actual exports, security/session
gates, layout and pin persistence, and live API/cache behavior are not
implemented here. The sample editor is not the production
editor and does not parse or calculate money. English sample copy and simplified
account grouping are for this artifact only; production retains the six-locale
translation boundary, canonical exact arithmetic and all existing workflow guards.

The prototype uses `#/app/...` links to work from a local file. Production
uses actual `/app/...` routes served by the existing Go/static SvelteKit app.
The frontend route registry, dialog primitives, composed reads and cache
invalidation must be implemented in their slices, not copied from this artifact.

## Header revision (2026-10-29)

After the owner found the two header rows "not neat", the rows were given
distinct roles. The row starts with the word *Home* (no logo or app name: a mark
alone was too small and unclear, and beside *Home* redundant), then the sections in
sentence case as plain links without chevrons or dots; the current section is
a soft tinted pill. Each section opens its *Overview*; pages are underline
tabs on the header's hairline, with attention shown as a number badge
(*Import 1*) whose full sentence stays for screen readers. Header rows are
56px and 40px. The header is sticky with a translucent, blurred background,
solid under `prefers-reduced-transparency`; content stays opaque. The page
background is a light neutral off-white with white cards. Home has no *My
Home* heading; widgets follow the shortcuts directly.

*Ledger overview* (`/app/ledger`, with `/app/accounts` redirecting) replaces
the Accounts page: ledger actions, pinned accounts first, then every account
by institution, and the Institutions view. Planning and Reports gained sample
Overview pages. The prototype bar still offers the earlier *Menus* behaviour
for comparison. Capitals and a left account rail were tried and rejected.

Sample books for valuation: *Missing valuation* has three unpriced
instruments (Home shows Cash, and the status line reads "Net worth needs 3
prices · 2 more →"; one grouped review item); *Stale prices at scale* has
180 of 240 prices older than 7 days (the total stays, the status line reads
"180 prices out of date · 2 more →", and one grouped review item with the
oldest date links to Prices filtered to out-of-date prices); *House and
pension* has a house with no value yet and a pension last updated 30 Jun
against a 3-month interval (Home shows Cash, and the status line reads "Net
worth needs a value for House · 3 more →"; the pension is one of the 3 and
appears on the Review page with its interval).

Home's status line no longer expands into a list above the search: it links
to a *Review* page (`/app/review`), also reached from the header's Review
button with its count. The page has *Needs action*, *Notes* and *Status* (backups,
self-check, price and FX refresh, connections, staged imports).
