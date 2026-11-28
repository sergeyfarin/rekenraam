# Navigation and page design: open decisions after the Home redesign

Review, 2026-10-29, revised the same day after owner feedback. It follows the
owner-approved [Home and navigation design](../design/home-and-navigation.md)
(commit `457dd74`, scheduled on the [roadmap](../roadmap.md) in `e76b01f6`).
That design settles Home and the shell's general shape. This review covers
what it leaves open: how every existing path is reached, how navigation
copes with long lists such as 20+ accounts or 60 instruments, and what else
in the app has to change. The owner accepted the recommendations of D1–D12
on 2026-10-29, with the switcher built as scoped search (D3 option A); they
are recorded in the design document's
[Navigation beyond the header](../design/home-and-navigation.md#navigation-beyond-the-header).
Not chosen: moving FX rates to Investments (D8) and an account panel (O3),
which stays a later option.

**What the revision changed.** The first draft proposed tabs for the pages
inside a section without asking how many things each path holds. A tab row
works for five fixed pages; it cannot hold 25 accounts. This version maps
every path, estimates how many entries each can have, and organizes
navigation in three levels: sections, pages and objects.

## Three levels of navigation

| Level | What it is | How many | Size over time | Navigation tool |
|---|---|---|---|---|
| **Section** | Home, Ledger, Investments, Planning, Reports, plus the settings menu | 5 + 1 | Fixed | Header (approved) |
| **Page** | A screen with a fixed purpose: Transactions, Accounts, Returns, Budgets, Spending… | About 25 | Grows only when features ship | Section menu, plus a tab row inside the section (D1) |
| **Object** | One account, category, instrument, payee, import batch, saved run | Tens to thousands | Grows with the owner's data | List page, object switcher, pins and search (D3–D6), never tabs |

Within a page there is a fourth kind of control that is easy to confuse with
navigation: **views and filters** (Due / Templates, spending by category or
payee, a month, a date range, active / closed). These stay in the page as
segmented controls, steppers or filters, never in the header or a section
tab row.

## Path map

Every route and URL sub-view in the app today, with the estimated number of
entries and where it lands in the new navigation. Estimates are for the
product's target owner: an expat household with accounts in two or three
countries and a modest self-managed portfolio. "Heavy" is a long-running book
with several brokers. They are estimates, not measurements (see D11).

### Ledger

| Path today | Kind | Entries (typical / heavy) | Active at any time | Proposed place |
|---|---|---|---|---|
| `/app/transactions` with `q`, `account_id`, `category_id`, `payee_id`, `status`, `kind`, dates | Page + filters | 1 page; 2k–5k transactions a year | Recent weeks | Ledger tab **Transactions**; filters stay in the page |
| `/app/accounts` | Page (tree) | 1 | — | Ledger tab **Accounts**: the complete account index |
| `/app/accounts/{id}/register` | Object | 15–30 money accounts; 40–120 including holdings and closed | 3–5 | Object switcher, pins in the Ledger menu, search (D3–D6) |
| `/app/categories` | Page (tree) | 1 | — | Ledger tab **Categories** |
| `/app/categories/{id}` | Object | 40–80 / 150+, two or three levels deep | 10–15 | Reached from the tree, spending report and budgets; switcher with tree search; no pins by default |
| Payees | Object, **no page today** (API has list, archive, restore) | 200–500 / 1,500+ | 20–40 | Search and the Transactions payee filter; payee page decided in D9 |
| `/app/import` (upload, mapping, review, commit stages; rules; payee resolution) | Page | 1 | — | Ledger tab **Import** |
| `/app/import?batch_id=` | Object | 2–8 batches a month, hundreds over years | 0–3 in review | List inside Import, newest first; batches in review counted in the menu |
| `/app/import/dividends` | Page (review queue) | 1 | Items in review | Inside Import as a view (**Files · Dividends**), counted in the menu |
| `/app/reconcile` (account picker, session, checkpoints) | Page | 1; reconcilable accounts 5–15 | 1 session | Ledger tab **Reconcile**; account chosen with the same switcher, filtered |

### Investments

| Path today | Kind | Entries (typical / heavy) | Active | Proposed place |
|---|---|---|---|---|
| `/app/investments` tab Portfolio | Page | 1 | — | Investments tab **Overview** |
| `/app/investments?tab=instruments` | Page (list) | 1 | — | Investments tab **Instruments** |
| `/app/investments/instruments/{id}` | Object | 5–25 held; 40–80 ever held | 5–15 | Instrument switcher (held first), search; holding registers link here |
| `?tab=gains` (realized gains, saved statements) | Page; saved statements are objects | 1; 1–3 statements a year | Tax season | Investments tab **Gains**, also linked from Reports |
| `?tab=splits` | Page (history) | 1; a few splits a year | Rare | Section on the instrument page plus a filter in Overview activity; no tab |
| `?tab=suggestions` | Page (review queue) | 0–20 pending | Pending only | Review items: count in the menu and Home's review list; page reached from Overview |
| `/app/investments/returns` (+ saved runs) | Page; runs are objects | 1; tens of runs | Latest | Investments tab **Returns**; runs listed in the page |
| `/app/investments/income` | Page | 1 | — | Investments tab **Income** |
| `/app/pricing` with `base_commodity_id`, `quote_commodity_id` | Page | 1; one series per instrument | — | Investments tab **Prices** |
| Holding accounts (`security_holding`, `fund_holding`) | Object (accounts) | 1 per instrument per broker: 10–60 | Same as instruments | Under their broker in Accounts; left out of the general account switcher by default (D7) |

### Planning

| Path today | Kind | Entries | Proposed place |
|---|---|---|---|
| `/app/budgets?month=` | Page + period | 1; one month at a time | Planning tab **Budgets**; month is a stepper in the page |
| `/app/recurring?tab=due\|templates` | Page + view | 1; 10–40 templates | Planning tab **Recurring**; Due / Templates is a view switch, not a tab; due count in the menu |
| `/app/forecast` | Page | 1 | Planning tab **Forecast** |

### Reports

| Path today | Kind | Entries | Proposed place |
|---|---|---|---|
| `/app/reports?view=spending` with group, mode, range, filters | Page + view | 1 | Reports tab **Spending** |
| `?view=cashflow` | Page | 1 | Reports tab **Cashflow** |
| `?view=net-worth` | Page | 1 | Reports tab **Net worth** |
| Ledger CSV / QIF export (now on `/app/settings/data`) | Page section | 1 | Reports tab **Export** (D8) |
| Realized gains | — | — | Reports tab **Gains** linking to the Investments page (one canonical page) |
| Saved report views | Not built | — | Later, as Home shortcuts; no extra navigation |

### Settings

The settings area already has its own layout with a side list
(`routes/app/settings/+layout.svelte`): Appearance, Language, Currencies,
Data, Trash, Security, eight fixed pages. That vertical list is the right
local navigation for settings and stays. The header's settings menu repeats
the most-used entries (D10). Currencies holds 2–8 enabled currencies and the
FX-rate history, 3–20 pairs (D8).

### Other

| Path today | Proposed place |
|---|---|
| `/app` | Home (approved design) |
| `/` sign-in, setup, recovery | Minimal public header (D10) |
| Institutions (edited inside Accounts) | Grouping in the Accounts tree and switcher; no page |
| Personal access tokens (inside Security) | Unchanged |

## What the counts imply

1. **Only pages get tabs.** Every section has 3–6 fixed pages, so a tab row
   fits on desktop and scrolls horizontally on phones without hiding
   anything important. Objects never appear as tabs.
2. **Accounts are the one long list people move through constantly.** With
   15–30 money accounts, of which 3–5 are used weekly, the busy ones must be
   one or two clicks from anywhere, and every other account must be reachable
   in three steps or fewer without leaving the page you are on.
3. **Holdings inflate the account list.** Counting holdings, a heavy book has
   over 100 accounts, most of them one per instrument. People think of those
   by instrument, not by account, so they belong on the Investments side and
   under their broker, not mixed into the everyday account list.
4. **Categories, payees and instruments are reached by context more than by
   browsing.** You get to a category from a report bar, a budget row or a
   transaction; to a payee from a transaction or search. A type-ahead switcher
   and search matter more for them than menus or pins.
5. **Review queues are counts, not destinations.** Import batches, dividend
   proposals, investment suggestions and due recurring items are small,
   changing sets. They show as counts in the menus and in Home's review list
   and link into their page.
6. **Archives must not crowd the lists.** Closed accounts, archived
   categories and payees, and instruments no longer held are hidden by
   default and always one control away.

## What comparable products do

- **Finance apps put accounts in a sidebar.** Actual, YNAB, Quicken and
  Monarch list accounts with balances in a left rail, grouped by type
  (budget / tracking, cash / credit / investments), with closed accounts
  collapsed. That is why they keep a sidebar: it is an account switcher as
  much as navigation. Monarch narrowed its sidebar in December 2024 and kept
  it usable when collapsed.
- **Products with a top header use an object switcher.** GitHub, Vercel,
  Linear and Stripe put a type-ahead picker beside the current object's name
  (repository, project, team, account), listing recent and pinned items first
  and searching everything.
- **Visible beats hidden.** Research summarized by Nielsen Norman Group and
  others finds menus hidden behind icons are used less than visible links.
  The GOV.UK Design System split its header in 2024: a thin global header and
  a separate row for the service's own pages.
- **Menus in a site header are disclosures.** The WAI-ARIA Authoring Practices
  "disclosure navigation" example (a button with `aria-expanded` and a list of
  links) is the pattern for section menus; `role="menu"` is for command menus.
  Object switchers and the command bar are comboboxes: input keeps focus,
  listbox results, arrows move, Enter opens, Escape closes.

Sources: [GOV.UK service navigation](https://design-system.service.gov.uk/components/service-navigation/),
[GOV.UK: navigate a service](https://design-system.service.gov.uk/patterns/navigate-a-service),
[Monarch navigation refresh](https://www.monarchmoney.com/ipad-support-budget-enhancements-and-more),
[PocketSmith new look (2025)](https://www.pocketsmith.com/news/2025-11-23-new-look-for-pocketsmith-web/),
[Cmd+K pattern overview](https://www.chameleon.io/patterns/cmd-k-search),
[Infinum: dropdown navigation test](https://infinum.com/?p=48869),
[Uxcel: common navigation patterns](https://uxcel.com/lessons/common-navigation-patterns-046).

## Options for reaching accounts and other objects

Four ways to make 20+ accounts usable when only a few are busy. They can be
combined; the recommendation combines three.

**O1. Object switcher in the page header.** On a register the account name is
a button: *Checking ▾*. It opens a type-ahead list: pinned accounts first,
then every active account grouped by class and institution, each with its
balance and currency; *Show closed* at the bottom. Typing two letters
narrows it. The same component serves categories (as a tree), instruments
(held first) and the Reconcile account picker. On a phone it opens full
screen. Every account is three steps away from any register: open, type,
Enter.

**O2. Pinned accounts in the Ledger menu.** The Ledger menu shows its pages
on one side and up to six pinned accounts with balances on the other, plus
*All accounts*. The busy accounts are two clicks from any page. On a phone
the menu sheet shows the pins under Ledger.

**O3. Account panel on Ledger pages.** A collapsible rail on Transactions and
registers listing accounts grouped by class with balances, as Actual and YNAB
do. Desktop only (about 1280px and wider); phones use O1. It brings back a
sidebar, but only where accounts are the subject.

**O4. Search only.** The command bar finds any account by name, code, last
four digits or institution. Fast for keyboard users, but nothing is visible.

| | Busy accounts | Any other account | Phone | Fits approved design |
|---|---|---|---|---|
| O1 Switcher | 2 steps on a register | 3 steps | Good (full-screen picker) | Yes |
| O2 Pins in menu | 2 clicks from anywhere | — | Good | Yes, extends the menu |
| O3 Account panel | 1 click on Ledger pages | 1 click (scroll) | Not shown | Partly; the sidebar was rejected as global navigation |
| O4 Search | 3+ keystrokes | 3+ keystrokes | Awkward | Yes |

**Recommendation: O1 + O2, with O4 as it already comes with the command bar.**
Together they meet the target (busy accounts two clicks from anywhere, every
account three steps from any register, no lists in the header) and work the
same on phones. Keep **O3** as a later option, opened as its own issue only if
moving between registers still feels slow after O1 and O2 ship; it would sit
on Ledger pages only, never as global navigation.

## Decisions

### D1. Tabs for pages within a section

- **A. Header menus only** (as approved). Clean, but sibling pages are hidden.
- **B. Header menus plus a tab row** of the section's fixed pages: Ledger
  *Transactions · Accounts · Categories · Import · Reconcile*; Investments
  *Overview · Instruments · Returns · Income · Gains · Prices*; Planning
  *Budgets · Recurring · Forecast*; Reports *Spending · Cashflow · Net worth ·
  Gains · Export*. On phones the row scrolls horizontally. Objects never
  appear in it, and views or filters stay in the page.
- **C. Section landing pages** instead of menus.

**Recommendation: B.** The counts above show every section has 3–6 fixed
pages, which suits a tab row. It also replaces today's two different in-page
tab bars (Investments, Reports) with one component.

### D2. Section menus

**Recommendation:** open on click or Enter, built as WAI-ARIA disclosure
navigation, one open at a time, Escape returns focus, current section marked.
The Ledger menu has two columns: pages, and pinned accounts with balances
(O2). Other menus list their pages with review counts. Hover opening was
considered and rejected because menus open by accident and touch laptops
behave differently.

**Revised later the same day (owner, after the prototype):** sections are
plain links, each opening its *Overview* page, and the tab row is the only
list of pages. Reasons: one click lands on a useful page; the pages are not
listed twice (menu and tab row); nothing is hidden behind a layer that touch
users miss; GitHub, Vercel, Stripe and Lunch Money work this way; and there
is no menu focus handling to build and test. Pinned accounts move from the
Ledger menu to the top of Ledger overview (which replaces the Accounts page)
and Home's Accounts widget; counts move to tab badges.

### D3. Object switcher

**Recommendation:** adopt O1, built as a scoped mode of the command bar
rather than a second picker, for accounts, categories, instruments and the
Reconcile picker.

**Why not let the header search do it alone?** The header search can find any
account, so it can already switch accounts. What it does not do is keep
your place. The two answer different questions:

| | Header search | Switcher (*Checking ▾*) |
|---|---|---|
| Question | "Take me to anything" | "Show me this same page for another one" |
| Scope | Everything: pages, objects, commands, transactions | One kind: the page's object type |
| Result opens | The object's default page | The current page, keeping its period, filters, view and sort |
| Empty state | Suggestions and pages | The whole grouped list with balances, for people who recognize an account more easily than they recall its name |

Example: in the Checking register filtered to March, *Savings* from the
switcher opens the Savings register for March. *Savings* from the search
opens the Savings register as it normally starts. On Reconcile the switcher
changes the account being reconciled; the search would leave the page.

Options:

- **A. One component, two ways in.** The switcher is the command bar opened
  with a scope chip, *Accounts ×*, already applied, the way GitHub's command
  palette scopes to repositories and VS Code's Ctrl+P switches modes. The
  chip changes the results (one kind, grouped, with balances) and what Enter
  does (open here, keeping context). Removing the chip with × or Backspace
  turns it into the global search. In the global search, the empty state
  offers the same chips (*Accounts*, *Categories*, *Instruments*, *Payees*)
  to narrow down.
- **B. Two separate components**: a picker beside the name and the search in
  the header. Simpler to build first, but two lists that rank, group and
  behave differently on the keyboard.
- **C. Search only, no switcher.** One less control, but context is lost on
  every switch and there is nothing to browse.

**Recommendation: A.** One index, one ranking, one keyboard model and one
phone sheet, so the owner learns it once. The scope decides only two things,
which results show and what Enter does, and the chip states both visibly.
Typed scope prefixes (VS Code's `>` or `@`) are not needed at first, and
words would need translating in six locales; chips are clearer.

Rules that keep it predictable:

- Search results open the object's default page. Only a scoped search
  (the switcher) keeps the current page's context. If the current page cannot
  keep a filter for the new object (a category filter that does not apply to
  the new account, say), the filter is dropped and the scope line shows it.
- Both read from the same data: pages from the route registry; accounts,
  categories and instruments from the reads the page already has, matched in
  the browser (the app already uses MiniSearch for payees); payees and
  transactions from the server's search.
- `/` and Ctrl/⌘ K open the global search. The switcher opens from its button;
  a keyboard shortcut for it can wait.

Details of the switcher:

- Groups: pinned, then by class (assets, liabilities) and institution for
  accounts; by tree for categories; held, then no longer held, for
  instruments.
- Each row shows the name, currency and balance (respecting hide amounts);
  institution and last four digits are searchable.
- Closed and archived items are behind *Show closed*, never mixed in.
- Accessible as a combobox; full-screen on phones; keyboard: open with the
  name button, type to filter.
- The data comes from one read per object type that the page already loads
  (accounts and instruments are loaded today by each detail page), not per
  row.

### D4. Pins: explicit or automatic?

- **A. Explicit pins.** A pin button on each account row, in the switcher and
  in the register header. Up to six. The pin list is stored with the Home
  layout (#438) or as its own preference if pins ship first.
- **B. Automatic "recent".** The last five opened accounts, per browser.
- **C. Explicit pins, suggested once.** On first use the menu offers *Pin your
  busiest accounts*, proposing the three to five accounts with the most
  postings in the last 90 days. The owner accepts, edits or dismisses; after
  that the list only changes when the owner changes it.

**Recommendation: C.** The approved design rejected usage-based placement on
Home because things that move by themselves are hard to find again. C keeps
the list stable and still saves the owner the setup. The switcher may add a
small, labelled *Recent* group (B) below the pins, since a list you search
in is not a place you remember.

### D5. Breadcrumbs and links between related objects

**Recommendation:** detail pages show a breadcrumb built from the hierarchy:
*Accounts › DEGIRO › VWRL* for a holding, *Categories › Housing › Energy* for a
category. A holding's register links to its instrument page and back. A
category page links to its budget row and to the spending report filtered to
it. These links replace the "Back to accounts" buttons on the three detail
pages today.

### D6. Accounts page as the full index

**Recommendation:** Accounts stays the one complete list, grouped by class and
institution, with balances, a type-ahead filter, and active / closed / all.
Holdings collapse under their broker by default with a count (*DEGIRO · 14
holdings*), so the page stays scannable with 100+ accounts. It is the target
of *All accounts* in the Ledger menu and the switcher.

### D7. Holdings in account lists

- **A. Leave holdings out** of the account switcher and pins by default; they
  are reached through Instruments, the instrument switcher and their broker
  in Accounts. The switcher's search still finds them when typed.
- **B. Show holdings** in the switcher under their broker.

**Recommendation: A.** One holding account per instrument per broker can
outnumber the money accounts three to one. People look for "VWRL", not
"DEGIRO VWRL holding", and the instrument page already shows a holding's
activity.

### D8. Export, gains and FX rates

- **Export:** move ledger CSV and QIF export to a real *Reports → Export*
  page; *Settings → Data* keeps backup and restore and links to it.
  **Recommended.** The alternative is a link into the data settings page.
- **Gains:** one canonical page under Investments, also in the Reports tab
  row. **Recommended.**
- **FX rates:** keep them in *Settings → Currencies* for now and name the
  Investments tab *Prices*. Moving rates to Investments is cleaner for a
  multi-currency owner but means splitting an 886-line screen; give it its
  own issue if wanted.

### D9. A payees page?

Payees can number in the thousands, the API already supports list, archive
and restore, and no screen uses archive or restore.

- **A. Add *Ledger → Payees*:** a searchable list with transaction counts and
  last-used dates, rename, merge (later) and archive. Each payee opens
  Transactions filtered by that payee.
- **B. No page.** Payees stay reachable through search and the Transactions
  filter; archiving stays unavailable in the UI.

**Recommendation: A, as its own issue after slice 1.** Imports create many
near-duplicate payees, and without a page the owner has no way to tidy them.

### D10. Header details

- **Settings menu:** Settings, Security, Backups and data, Language and
  appearance, Trash, then Sign out. Theme becomes an inline Light / Dark /
  System choice in this menu. The GitHub link moves to an *About* line in
  Settings.
- **Public pages** (sign-in, setup, recovery) lose the root `TopBar` and get a
  minimal header with name, language and theme.
- **Hide amounts** is applied in `$lib/money/format.ts`, covers chart labels,
  does not touch exports or edit fields, and is remembered per browser.
- **Menu counts** come from one small attention read that Home's review line
  also uses, replacing the shell's recurring-summary query. Loading counts
  per menu entry would break the repository's no-fan-out rule.

### D11. Check the estimates against a real book

**Recommendation:** before slice 1 fixes the switcher's grouping and the pin
limit, count the owner's real book: postable accounts by class and kind,
holdings per broker, closed accounts, categories by depth, payees, and
postings per account over 90 days. A short read-only script against a backup
copy is enough. Record the result here.

**Script (#449, 2026-11-28):** `python3 scripts/count-book.py <copy of a
backup>` prints these counts and nothing identifying; see [Real-book count
(D11)](../developer-workflow.md#real-book-count-d11). Pins are offered from
`postings_in_window.pin_eligible_account_rows`: active account-index rows
other than holdings (D7), where a brokerage counts the postings of its cash
and holdings as one row.

**Result:** waiting for the owner's run. Record the output here, and note on
#442 and #448 any change it forces to the pin limit, the switcher grouping or
the Ledger overview grouping.

### D12. Page shell

- **Page header:** breadcrumb (detail pages), title or object switcher, a
  scope line where the page has one, and a slot on the right for the page's
  main action. The shell's description paragraph is dropped; help text moves
  into empty states.
- **Adding transactions on phones:** the action slot shows *Add* on
  Transactions, registers and category pages, prefilled for that page. This
  replaces the header button the design removed and keeps entry usable on
  phones, where there is no N key.
- **Real routes for tabs:** `/app/reports/spending`,
  `/app/investments/instruments`, `/app/investments/gains` and so on, with
  redirects from the old `?view=` and `?tab=` links. Today the Investments
  page reads its tab only once, so a header link to another tab of the same
  page would not switch it.
- **Route registry:** one typed list of pages (route, section, title key,
  icon, search keywords, attention source, width) drives the menus, tabs,
  document titles, breadcrumbs and the command bar's page results. A unit
  test fails when a `+page.svelte` route is missing from it.
- **Page widths:** narrow (about 720px) for Home's top area and forms;
  standard (about 1200px) for reports, budgets and investments; fluid (up to
  about 1600px) for registers, transactions and import review.
- **Keyboard help:** `?` lists `/`, Ctrl/⌘ K and N.
- **Returning to Home** restores its scroll position and expanded state, as
  the design review requires.

## Other parts of the application affected

- **Translations:** the `app_shell_*` sidebar and overview keys and the
  `top_bar_*` keys retire; menus, tabs, the switcher and counts need all six
  locales (#102).
- **Browser tests:** `route-sweep.spec.ts` and `subview-sweep.spec.ts` click
  through the sidebar today; they should iterate the route registry. Axe
  checks must cover open menus and switchers in both themes, at 320px and
  with long translations. Add a register-to-register journey through the
  switcher on a phone.
- **Print:** the header, tab row and switcher button need `data-print-hide`
  behaviour like the sidebar has.
- **Documents:** `docs/conventions.md` (page, tab, view and object rules;
  route registry; attention counts) and the `frontend-screen` skill.
- **Screenshots (#335):** take README and website screenshots after the
  header and Home ship.
- **First run:** with no pins and one account, the Ledger menu and switcher
  must still make sense; a new book shows setup prompts on Home, not
  failures.

## Issues

- #441 tracks the overhaul.
- #435 owner answers to D1–D12, recorded in the design document.
- #436 header and search, #437 Home, #438 personalization, #439 search depth,
  #440 widget depth.
- #442 object switcher, pins and breadcrumbs (D3–D7).
- #443 Payees page (D9).
- Moving FX rates (D8) and an account panel (O3) get their own issues only if
  later chosen.
- After the prototype's second review (same day): #446 settings in five
  pages with Trash and export moved, #447 Import views, #448 accounts by
  institution and brokerages as one account, #444 Tags view, #449 the D11
  count. The design's [Delivery slices](../design/home-and-navigation.md#delivery-slices)
  lists them all.

## Coverage review after the prototype (2026-10-29)

Owner feedback on the navigation prototype raised ten points (widget density,
holding accounts, institutions, the *Add* label, Home duplicated by the brand,
Trash, Home settings in Settings). This section checks every capability the
app ships today, from [implemented.md](../implemented.md) and the screens
under `frontend/src`, against the new navigation, and records what else
the check found. Accepted answers are in the
[design](../design/home-and-navigation.md#navigation-beyond-the-header).

### Where every capability lives

| Capability (today's place) | New place |
|---|---|
| Transactions list, filters, search, editor, splits, posting dates, reorder (`/app/transactions`) | Ledger › Transactions (*All*) |
| Drafts (recurring and imported) | Ledger › Transactions › *Drafts*; Home review line |
| Soft-deleted transactions and restore (`/app/settings/trash`, "Trash") | Ledger › Transactions › *Deleted*. Only transactions are ever deleted; voided ones stay in the ledger under *All* |
| Void, unvoid, correct, correction chain, audit history | Transaction detail panel (unchanged) |
| Account list, lifecycle (close, reopen, archive), account editor (`/app/accounts`) | Ledger › Accounts, grouped by institution; closed behind *Show closed* |
| Institutions CRUD and archive (only inside the account editor; "standalone UI minimal") | Ledger › Accounts › *Institutions* view |
| Account register, reconcile from register | Account page, switcher, pins |
| Security holding accounts (one per instrument per broker, listed as accounts) | Not listed as accounts. Positions inside the brokerage account page and on the instrument page; the ledger model is unchanged |
| Categories tree, editor, category activity | Ledger › Categories |
| Tags: CRUD and archive in the API; used only in the editor and import rules, no screen | Ledger › Categories › *Tags* view (#444) |
| Payees: API list, archive, restore; no screen | Ledger › Payees (#443) |
| File import (QIF, CSV), staging, review, commit, resume | Ledger › Import › *Files* |
| Trading 212 connection, auto-refresh (inside the import page) | Ledger › Import › *Connections*; the institution row says how data arrives |
| CSV profiles and import rules (inside the import page) | Ledger › Import › *Rules and profiles* |
| Grouped unknown-payee resolution | Inside a batch review (unchanged) |
| Dividend import review (`/app/import/dividends`) | Ledger › Import › *Dividends*, with a count |
| Reconcile sessions, checkpoints, void checkpoint, re-reconcile | Ledger › Reconcile |
| Positions and lots (`/app/investments` Portfolio) | Investments › Overview › *Positions* |
| Buy, sell, dividend, reinvest, write-off, transfers, short sale, corporate actions, corrections | *Record trade or dividend* on Overview, the instrument page and a brokerage account; commands in search |
| Split history and correction | Instrument page, and Overview › *Splits* |
| Provider dividend suggestions and the read-only automation rules list | Overview › *Suggestions* with a count; Home review list |
| Instruments | Investments › Instruments |
| Realized gains, saved gains runs, alternative-method comparison | Investments › Gains (also in the Reports menu) |
| Returns, allocation, saved returns runs | Investments › Returns |
| Dividend income coverage | Investments › Income |
| Security prices, coverage, history, manual price, void (`/app/pricing`) | Investments › Prices |
| Currencies, FX rates, refresh schedule, failed refresh work (`/app/settings/currencies`) | Settings › Currencies and FX rates |
| Budgets, recurring templates and due inbox, forecast | Planning › Budgets, Recurring (*Due*, *Templates*), Forecast |
| Spending/income, cashflow, net worth incl. unsettled trades, report CSV, print | Reports › Spending, Cashflow, Net worth |
| Ledger CSV, QIF, archive bundle, preview (inside `/app/settings/data`) | Reports › Export |
| Scheduled backups, retention, status; ledger self-check and history | Settings › Data and backups; failures in Home's review line |
| Language, theme | Settings › Preferences (one page) |
| Home layout | Settings › Preferences › *Home page*, and the Customize panel on Home |
| Default currency, time zone, version, last backup (settings overview) | Settings › General |
| Two-step sign-in, sessions, personal access tokens | Settings › Security |
| Sign out, theme quick switch | Settings menu |
| Restore and verify-backup | Command line only (unchanged); Data and backups explains it |

Every current route is either kept or redirected: `/app/settings/language`
and `/app/settings/appearance` go to Preferences, `/app/settings/trash` to
Transactions › Deleted, `/app/import/dividends` to Import › Dividends, and
the old `?view=` and `?tab=` addresses to their pages.

### Duplications found

- **Brand and Home link** both opened Home. The brand is now the Home link
  (marked current on Home); phones keep *Home* at the top of the menu.
- **Ledger menu**: *Accounts* in the pages column and *All accounts* under the
  pins. The pins column no longer repeats it.
- **Settings overview** (`/app/settings`) repeats the side list as cards. It
  becomes *General*, which shows facts (default currency, time zone,
  version), not links.
- **Language and Appearance** are two thin pages under one menu entry. They
  merge into *Preferences*, which also holds the Home page settings.
- **Customize** opens from four places (header on Home, the headline menu,
  the More tile, Preferences). Kept: all open the same panel, each where the
  owner happens to be.
- **Gains** in Investments and the Reports menu: one page, the Reports entry
  is marked *in Investments*.

### Gaps and defects found in the current app

- `/app/settings/data` is missing from the settings side list
  (`routes/app/settings/+layout.svelte` lists Overview, Appearance, Language,
  Currencies, Security, Trash). Backups and export are reachable only from
  the overview cards.
- "Trash" sounds like a general bin but holds only soft-deleted
  transactions.
- Holding accounts appear in the account list tree next to everyday
  accounts; nothing filters them out.
- Institutions, tags and payees have APIs but no screen to manage them.
- The import page carries five jobs (files, connections, rules, profiles,
  dividend review) in one long screen.
- *Add* on account and category pages does not say what it adds. It is now
  *Add transaction*; a brokerage also shows *Record trade or dividend*.

### How other apps place the same things

Monarch keeps institutions (connections), categories, merchants, rules and
tags under Settings and shows investment holdings inside their account.
Firefly III groups them as *Classification* (categories, tags) and
*Automation* (rules, recurring). Actual and YNAB list accounts in a sidebar
grouped by type. Quicken, like Monarch, lists a brokerage as one account with
its holdings inside. Rekenraam keeps weekly tasks (categories, payees, import,
reconcile) in Ledger and puts set-and-forget configuration (rules, profiles,
connections) beside the task that uses it (Import), rather than burying it in
Settings. Investment positions follow Monarch and Quicken: inside the brokerage
account, never as accounts of their own.
