# Home and Navigation Design

Owner-approved design direction, 2026-10-27, for the new Home page and the
application shell around it. It records what was agreed after three mockup
rounds; it does not set delivery order. Order lives in the
[roadmap](../roadmap.md) and acceptance in GitHub issues under tracking
#441 (listed in [Delivery slices](#delivery-slices)). The
navigation decisions of 2026-10-29 (#435) are recorded in
[Navigation beyond the header](#navigation-beyond-the-header); their options
and reasoning are in the
[navigation and pages review](../reviews/navigation-and-pages-options-2026-10-29.md).

Background and reasoning:

- [Design review](../reviews/focus-first-ui-review-2026-10-27.md): verified
  inventory of existing contracts, financial semantics, data-loading rules
  and acceptance checks. Those rules still apply.
- [Revised proposal](../reviews/focus-first-ui-proposal-revision-2026-10-27.md):
  options considered and the owner's first decisions. Where it differs from
  this document, this document wins.
- [Mockup](home-mockup/README.md): the interactive v3 canvas, with sample data.
- [Navigation prototype](navigation-prototype/README.md): browser-openable
  implementation-preparation companion (2026-10-29), with registers, report
  drill-down, import review and reconciliation. Synthetic data only; it does
  not deliver the shell, Home or object-navigation slices.

## Principles

- Home answers three questions: where do I stand, does anything need me,
  and what do I do next. Breadth comes from scrolling; direct access comes
  from search and navigation.
- Calm first screen: two lines of text, a search field and a row of
  shortcuts, with generous empty space. Widgets start below.
- Desktop is the priority. Phones (360–390px) must stay fully usable.
- Every figure states its scope and completeness. A missing price or rate
  makes a total unavailable, never silently partial, and never shown as a
  labelled partial either (decided 2026-10-29).
- When the net worth headline is unavailable, Home shows **Cash** instead
  (open checking, savings, cash and e-money accounts in the reporting
  currency), which needs no price, and the status line says why: "Net worth
  needs 3 prices", linking to Prices filtered to the missing ones. If cash
  itself lacks an FX rate, only the status line shows. A chosen headline that
  needs no price is never replaced; the line then reads "3 prices missing".
- An out-of-date price still values its position at that price and never
  blocks a total; Home counts them ("180 prices out of date", linking to
  Prices filtered to them), the Review page adds the oldest date, and the
  review list holds one grouped item, never one per instrument (proposed
  2026-10-29). Several missing prices are likewise counted and grouped.
- Prices are one kind of value among several. Net worth also depends on FX
  rates and on values the owner enters, such as a house, a car or a pension
  statement. The same two rules apply to all of them: a **missing** value
  (an asset with no value yet, an instrument with no price, a currency with
  no rate) makes net worth unavailable; an **out-of-date** value is used at
  its last amount and counted. Out of date is relative to how often each
  value is expected to change: prices and rates after 7 days, and for values
  the owner enters, an update interval set per account (proposed defaults:
  pension or other statement-based accounts 3 months, property and vehicles
  12 months, or never). Wording on Home names one item, counts several of
  one kind, and counts mixed kinds as values: "Net worth needs a value for
  House", "Net worth needs 3 prices", "Net worth needs 4 values"; "Pension
  out of date", "180 prices out of date", "3 values out of date". Missing
  outranks out of date; whatever does not lead joins "N more". A named item
  may wrap on a phone; "N more →" never splits. Recording entered values and
  their intervals is #454.
- Home names nothing that grows with the book: accounts, instruments and
  review items appear as counts, never as lists.
- Customization never changes financial data.

## Application shell (whole app)

The left sidebar and the separate dark top bar are replaced by one light
header on every page.

- **Surface (revised 2026-11-01):** the two header rows carry two olive
  tints from the app's one neutral hue (108), so the shell reads as two
  levels without a heavy coloured band. The pages row uses the old sidebar
  tone (`#f2f2e5`, about `oklch(0.953 0.019 108)`), the sections row one step
  deeper (about `oklch(0.918 0.026 108)`), the page the old page tone
  (`#f7f7ef`) and cards near white. The first version used the page colour
  for the header and felt flat next to the old dark bar and tinted sidebar
  (owner, 2026-11-01). The dark theme uses the same steps. See the
  [shell look mockup](shell-look-mockup/README.md).
- **Sections:** Home · Ledger · Investments · Planning · Reports, in
  sentence case. Each section is a plain link to its *Overview* page; the
  tab row under the header lists the section's pages, so there are no
  section menus (owner decision 2026-10-29, revising D2; reasoning in the
  [review](../reviews/navigation-and-pages-options-2026-10-29.md#d2-section-menus)).
  The current section is a tab in the pages-row tint, joined to the row
  under it (as MS Money did), so a section visibly opens into its pages; the
  current page has a green underline. Green otherwise marks only count
  badges and selected chips.
  A page that needs attention shows a number badge on its tab (*Import 1*),
  with the full sentence for screen readers. A section is navigation only: it
  never changes account scope, currency or book.

  | Section | Pages |
  |---|---|
  | Ledger | Overview, Transactions, Categories, Payees, Import, Reconcile |
  | Investments | Overview, Instruments, Returns, Income, Gains, Prices |
  | Planning | Overview, Budgets, Recurring, Forecast |
  | Reports | Overview, Spending, Cashflow, Net worth, Export |

  Overviews hold their section's state and actions, never a copy of Home:
  *Ledger overview* is the accounts list (pinned accounts first, then every
  account by institution, with an *Institutions* view) and the ledger's
  actions (New transaction, Transfer, Import file, Reconcile, Add account);
  it replaces the separate Accounts page. *Planning overview* shows the
  month's budget status, recurring items due and the forecast's low point.
  *Reports overview* lists every report with its latest figure and saved
  reports; it links Gains, which lives in Investments. Investments already
  has its Overview.

- **Left side:** the word *Home* is the first item of the row, styled like
  the sections and marked current on Home. The header shows no logo or app
  name; a mark alone was too small a target and too unclear, and beside
  *Home* it was redundant (owner, 2026-10-29).
- **Look:** header 56px, tab row about 42px, sticky; each row may stay
  slightly translucent in its own tint, solid under
  `prefers-reduced-transparency`. Content (cards, widgets, menus, dialogs) is
  never translucent. Cards stand clearly whiter than the page.
- **Right side:** a *Review* button with the count of items needing action
  (the total behind Home's status line, `!` when the check failed), hide
  amounts (eye), Customize (Home only), and a settings
  menu listing the settings pages (General, Preferences, Currencies and FX
  rates, Security, Data and backups), an inline Light / Dark / System theme
  choice and Sign out.
- **Search (revised 2026-11-01):** on every page except Home the header
  has a rounded *Search Ctrl K* button (⌘ K on macOS), not a field. It opens
  the same large, centred, rounded search as Home's command bar (below), as
  a dialog over the page, with room for grouped results and a scope chip.
  On Home the central bar is the search. Keyboard: `/` and Ctrl/⌘ K open it.
  The earlier compact field was square-cornered, truncated its placeholder
  and dropped a narrow list under itself.
- **Phone:** sections collapse into a menu button; search becomes an icon.
  No bottom navigation bar. The tab row scrolls horizontally, keeps the
  current tab in view and fades at the edge when more tabs follow.
- **No navigation sidebar:** the owner rejected an account list beside
  registers (2026-10-29): other accounts distract from the one in hand, and
  Ledger overview is one click away. Settings keeps its own side list. The
  right column is kept for the current selection's details and running
  summaries ([side panels](#side-panels)), never commands or navigation.
- **Alignment:** the header, tab row and page content share one left edge
  at every width.
- Every existing route stays reachable. The shell keeps the current
  layout's responsibilities: setup and session gates, owner-local date
  context, shared recurring summary, logout and page titles.

## Home layout

Top to bottom, centred in a narrow column (about 720px) above a full-width
widget area:

1. **Briefing**: two lines, same type size, regular weight with amounts in
   medium weight.
   1. Headline: net worth (default), cash, one account, or a named set of
      accounts, with absolute change since the start of the month (“+€2,140
      in Oct”). ▾ switches the headline; the menu gives each choice's scope as
      a count (“All 5 accounts”, “3 cash accounts”), never an account list. The change is coloured by its sign in a dark, quiet green
      or red (never bright), and stays neutral while amounts are hidden; the
      sign itself still carries the meaning.
   2. Status: “2 drafts and 1 import to review →”, a link to the
      [Review page](#review-page); nothing expands above the search (owner,
      2026-10-29: the list distracted, and the line already signals the
      work). A price problem leads the line because it explains the
      headline, and the rest becomes a count: “Net worth needs 3 prices ·
      2 more →”. The header's Review button carries the total, so the line
      never repeats it. “All clear, nothing to review” only after a successful
      check with no actionable items. An unchecked or failed source has its
      own visible state.

   Nothing else sits in the briefing (owner, 2026-10-29: four lines were too
   many). The headline's label is its scope; account lists, the reporting
   currency and data freshness live in the headline menu, the Review page's
   Status section and the reports. Both lines fit one line each on a 390px
   phone in the sample books.
2. **Command bar**: large, centred search and command field.
3. **Shortcuts**: a centred row of tiles.
4. **My Home**: the owner's widgets, full width. It has no visible heading;
   the widgets follow the shortcuts directly (owner, 2026-10-29).
5. **More to explore**: every other widget, full width, by category.

On phones the command bar opens the same search full screen when tapped, so
results are not squeezed under a keyboard.

### Briefing contract (#476)

`GET /api/v1/home` is session-only and starts no work. For owner-local today
it returns two headlines, `net_worth` and `cash`, each read through the Net
worth report's series for the same accounts in the book's default currency:
`value` at today, `start_value` at the end of the previous month and `change`
between them. A headline is complete-only; `unavailable` names why it has no
value (`valuation_gaps`, `unclassified_short`, `no_reporting_currency`,
`no_accounts`). `missing` lists each input without a usable observation today
(a chained conversion's missing currency leg is that currency's `rate`);
`stale` lists inputs valued from an observation older than the pricing
policy's staleness days (3 by default) and never repeats a missing one. That
is the threshold Prices and Review use, so the three agree on what is out of
date; each leg of a chained conversion is judged on its own observation date
and reported against its own commodity, so a stale rate is never blamed on a
current price. Inputs beyond the report's 7-day window are missing, as in the
report, so an old price does block the headline; the report's window, not
Home, decides that. Prices never calls usable a price valuation refuses: it
applies the shorter of the two limits (and returns both), judges a recent
quote with the report's rate table (a quote across conflicting splits is
`missing`), and lists a balance in an instrument nothing prices yet, so the
`?status=unusable` link always shows what Home named.
Cash is open checking, savings, cash and other money accounts; brokerage cash
stays with its brokerage. `account_count` counts open asset and liability
accounts without system or per-instrument holding accounts. Closed accounts
still count in the net-worth figure, so a book whose accounts are all closed
still has one (scope "Closed accounts only"); only an empty scope is
`no_accounts`. `empty_book` means no owner-created asset or liability account
exists. Both headlines total one ledger read and fold.

The status line's order: a failed or unreadable attention source; then the
net-worth valuation problem (no default currency, an unclassified short,
missing inputs, then out-of-date ones), linking to Prices filtered by
`?status=unusable` (every pair without a usable price, which covers what Home
calls missing) or `?status=stale`, or to Currencies when only rates are
involved; then
the work, naming two kinds at most. What does not lead is one "N more" link to
Review; price and rate attention units are not counted again behind a
valuation lead. A failed attention refresh replaces the line even while
earlier counts are cached, so an old "All clear" never outlives it. Values the owner enters join through #454.

### What belongs in the briefing

A briefing line is one sentence, chosen by the app, and either a position
or something to act on. Anything needing a chart, a list, a period or
settings is a widget. Notable moves (a holding's value change, a budget
overrun) were tried as a third line and rejected: they belong in the
Holdings and Budgets widgets. Notes such as unassigned dividends appear only
on the Review page; failures (backup, self-check, price refresh errors)
always count as items to review.

### Review page

`/app/review`, reached from Home's status line, the header's Review button
and search; not a section. One read (the attention read) feeds it, the
header count and Home's line.

- **Needs action:** drafts, import batches, missing or out-of-date prices,
  rates and entered values (#454), dividend suggestions, failed backups, self-check or refresh runs. Each is one
  grouped line with a count and a link to the page that resolves it, never one
  line per instrument or row.
- **Notes:** informational items that need no action now.
- **Status:** the state of background work, whether or not anything is
  wrong: last backup and retention, ledger self-check, price and FX refresh,
  each online connection's last download, staged file imports, and entered
  values once #454 lands. Each row links
  to where it is configured.
- The page states when it was checked; a failed check says so with Retry and
  never shows "All clear".

## Command bar

One component for Home's bar and the centred search dialog opened from the
header's *Search Ctrl K* button.

- Empty: suggested commands and pages.
- Typing matches, in groups:
  1. **Payee**, when the text matches one: *Add expense at Albert Heijn*
     (opens the transaction editor prefilled with the payee and its
     last-used category) and *Transactions with Albert Heijn* (Transactions
     page filtered by payee).
  2. **Pages, accounts, categories, instruments and commands.**
  3. **Everything**: *Search all transactions for “albert”*, covering
     payees, descriptions, references and notes.
- Quick entry never posts. Any typed amount or date only prefills the
  editor, which the owner confirms; parsing must respect locale decimal
  separators and dates.

Existing support: the `transaction_search` FTS5 index (payee, description,
external reference, notes), the transactions API `q` and `payee_id`
parameters with matching Transactions URL filters, and MiniSearch-based
payee matching in the frontend. New work: a last-used-category read per
payee and payee/Everything result grouping (#439). The page/object/command
grouping foundation is delivered in #457; the Home placement and the shared
entry dialog in #477.

Delivered in #477 (2026-10-31): Home renders the shared search with
`variant="home"` below the briefing, as wide as its column, and the header
omits its instance there, so Home owns `/` and Ctrl/⌘ K. On phones it is a
field-shaped trigger that opens the same full-screen search. The command bar
and shortcuts show in every briefing state, including the empty book and a
failed Home read, because neither needs Home data.
Searching account, category or instrument notes is later work.

### Shared search foundation (#457)

The search is one reusable combobox (delivered as a compact header field;
the header now opens it as a centred dialog, 2026-11-01). `scope` restricts its result
kind and `onSelect` lets a future switcher keep the current page's context;
`variant="home"` supports Home placement, with the header instance omitted
on Home (#477). The mounting owner registers the global
shortcuts once. Page-specific filter fields remain on their own pages.

Pages come from the route registry, including localized search aliases, with
legacy and parameterized routes excluded. One compact, cursor-paged catalog
composes current owner-local account, category and instrument names/codes;
seeded category labels are localized before indexing. It contains no balances,
transaction history or notes. The field fetches on opening and follows every
cursor, showing loading and partial-error coverage rather than an unsupported
empty result. Page links and browser commands remain usable when that read
fails. The read is refreshed on reopening; reference names are held only in the
query cache, never browser storage.

Within fixed groups (Pages, Accounts, Categories, Instruments, Commands), exact
names precede prefix and fuzzy matches; multiword queries require all terms.
Each group's initial six results can be expanded, with shown/total counts.
Closed and archived objects are hidden until explicitly included. Account and
category results open their register/detail; instruments use their commodity
page identity. Scope changes keep keyboard focus in the input. Arrow keys move
an active descendant, Enter chooses, Escape closes, and Tab reaches scope,
coverage and expansion controls. `/` ignores editable fields; Ctrl/⌘ K also
works from ordinary fields, while existing modal workflows retain their focus.
Phones open the same field in the shared full-screen modal.

Search belongs to the current page's interaction (#472). Leaving for a different
pathname resets it when navigation starts. Completion of an earlier navigation
must not close or clear search reopened on the destination. Same-page query or
fragment updates, including a report normalizing its default date range, keep
the query, scope and result selection. Choosing a result releases search before
the destination workflow takes focus, with no deferred close after arrival.

Commands open an unsaved transaction editor or toggle browser presentation
(amount visibility/theme); they never post or infer an amount/date. The entry
URL trigger is consumed so refresh does not reopen a dismissed working copy;
existing Transactions filters stay in place.
Payee actions/last-used category and the Everything transaction search result
remain #439. Recent objects, pins, hierarchy/institution grouping and switching
while preserving context remain #442. Those existing issues own the follow-up
acceptance; this foundation does not claim their behavior.


## Shortcuts

- Tiles with an icon and a label of up to two lines, Firefox New Tab style.
- **New transaction** is the first tile by default, in the accent colour
  (named *Add transaction* until #506; see [Transaction entry](#transaction-entry)).
  The Transfer tile opens the same entry on its Transfer tab.
  It can be moved but not removed. On other pages it is reachable through
  search (“add”), the keyboard shortcut N and the Transactions page.
- Other tiles are actions (Transfer, Import file, Reconcile an account) or
  links to a page or saved view (an account register, a filtered report).
  Defaults: New transaction, Transfer, Import file, Transactions, Spending by
  category, Checking.
- Each tile has a ⋯ menu (Move left/right, Rename, Remove); a final
  “+ Add shortcut” tile lists the remaining options. Phones show the first
  three tiles plus More.

Delivered in #477 (2026-10-31) as a fixed set: Add transaction, Import file,
Transactions and Spending by category. **Transfer** joined second with its
transfer entry (#481, 2026-11-01: different posting days, cross-currency
transfers with exact sent and received amounts, and a fee recorded on Bank
fees rather than in the transfer), so phones show Add transaction, Transfer
and Import file. The account-register tile (“Checking”) waits for pins (#442)
or the layout document (#438), since nothing yet says which account it
opens. ⋯ menus, *Add shortcut* and editing are #438. Phones show three tiles
and a More disclosure listing the rest in order, so a tile moved past the
third (Add included, once #438 allows moving) stays reachable.

### Shared entry dialog (#477)

This dialog hosts the one [transaction entry](#transaction-entry) workflow
on pages without their own editor (#506, delivered 2026-11-05); the record
below is what #477 delivered, under its old name *Add transaction*.

*Add transaction* on Home, `N` and search's *Add transaction* command on a
page without its own editor open one shared dialog: the existing transaction
editor (locale parsing, financial guards, reconciliation checks, error
recovery) in `ModalDialog`, a full-screen sheet below 768px. A page that
offers *Add transaction* in its header keeps its own side-panel editor,
prefilled for the page (#459); the dialog takes the same account/category
prefill when a caller supplies it. The shell mounts the dialog once, outside
every page and outside the shell's own loading and error states, so a
background refresh of Home, any page or the session never touches the working
copy (a failed session refresh shows the shell error behind it); the editor
reads its prefill once (#433). Signing out closes it. Opening it never
replaces an open working copy. Cancel and Escape discard it without asking;
backdrop clicks do nothing, and nothing in flight can be dismissed: the whole
save chain, including a payee the owner confirmed creating first, keeps the
editor busy, and a dismissed editor never continues to a save. Focus returns
to the opener, or, when a shell error or its recovery re-rendered the page
behind the dialog, to the page's main content (the shell error's Retry while
it shows). Payee and category suggestions are comboboxes (arrows, Enter,
Escape closes the list, not the dialog) whose options are not Tab stops, so
entry works by keyboard alone. Saving closes the dialog, refreshes every read
a transaction can change — transaction lists, registers, balances, reports,
budgets, forecast, reconciliation, brokerage cash reads and Prices coverage
(a posting in a new currency needs its exchange rate), so the page
beneath updates in place, plus Home and the attention count — and announces
*Transaction added* with a link to the transaction. Nothing posts without the
editor's own save.

## Transaction entry

Owner decision 2026-11-05 (#491). This supersedes the Investments-only
*New transaction* and *Change type* text of the same day. Delivery slices are
listed [at the end of this section](#entry-delivery).

### One workflow, one button

There is one primary *New transaction* button. Every entry point opens the same entry workflow, prefilled from where it started:
- Home's first tile
- `N`
- search's *New transaction* command
- account registers and category pages
- instrument pages and Investments

Where the workflow is hosted:
- Pages with an in-place editor (registers, Transactions) host it in their side panel.
- Everywhere else, the [shared entry dialog](#shared-entry-dialog-477) hosts it.

How defaults behave:
- The starting page's defaults apply once, when the entry opens. They never overwrite a choice the owner makes afterwards.
- The starting account stays editable until save.

What stays the same:
- Specialized form components, typed backend commands and their financial guards stay.
- The workflow organizes them; it does not replace them.
- There are no separate launchers for ordinary and investment entry.

### Modes by account context

**Four stable tabs (owner, 2026-11-05).** Every entry shows the same four
tabs in the same places, so position never shifts with the account. Labels
follow the account, and each mode has a restrained accent:

| Mode | Most accounts | Credit card | Accent |
|---|---|---|---|
| Money out | Withdrawal | Charge | red |
| Money in | Deposit | Credit | green |
| Between own accounts | Transfer | Transfer | blue |
| Investment activity | Investment | Investment | purple |

**Credit cards**
- *Charge* and *Credit* apply to credit cards only. Loans and mortgages
  keep *Withdrawal* and *Deposit*.
- A merchant refund or an adjustment is a *Credit*.
- Paying the card from another of the owner's accounts is a *Transfer*,
  which records both sides together. A short note under *Credit* says so,
  so a card repayment is never entered as a separate deposit that
  duplicates the checking movement.

**Tabs outside their natural context**
- *Investment* from a bank account is **muted but selectable**, never
  disabled. A bank account can fund an investment; what's missing is the
  holding. Selecting it:
  - keeps the bank account as the **settlement account**, labelled by the
    activity: *Pay from* for a purchase, *Pay into* for a sale or cash
    dividend. Activities that move no cash (split, reinvestment) neither
    show nor require it.
  - asks for the holding account, the instrument and the Activity
  - keeps the date and memo already entered

  Muted means more account information is needed, not unavailable. When
  active, the tab looks selected like any other: same selected indicator and
  accent, never the disabled style. The explanation, "Choose a holding
  account to record an investment", sits beside the holding-account picker,
  not on the tab.
- Starting from a holding, a cash mode asks for the cash account
  explicitly. It never posts cash to the holding (T-96).

**Accents**
- Red and green mark **entry direction**, never good, bad, profit or loss.
- Each mode has its own semantic tokens, separate from the error colour
  (`--color-danger`) and from the gain and loss colours. A Withdrawal must
  never look like an error.
- The accent shows only as the selected tab's underline or soft background
  and a small icon. Inactive tabs and the form surface stay neutral, and
  amounts keep the normal text colour.
- Label, icon and the selected indicator carry the mode without colour.
  Both themes meet contrast.

| Context | Starting mode |
|---|---|
| Ordinary account (checking, savings, cash, e-money, credit card, loan) | Withdrawal (Charge on a card); Investment muted |
| Brokerage (the container holding cash and positions) | Withdrawal; all four fully available |
| A holding (from an instrument page or a position) | Investment; cash modes ask for a cash account |
| None (Home, search, `N` without an account page) | Withdrawal, so a grocery payment stays quick |

**Withdrawal and Deposit**
- The owner enters a positive amount. The mode and the account's posting
  rules decide the direction, replacing today's signed amount.
- They describe direction relative to the account, not expense or income: a
  refund is a Deposit (Credit) against a category, and liability movements
  and transfers keep their accounting treatment.
- Split entry and the existing advanced journal paths (today's split and
  locked editor tiers) stay reachable. Tabs organize entry; they never
  reduce the shapes the ledger can already hold.

**Transfer** is today's transfer entry (#481): one operation between two of
the owner's accounts.

**Investment** has an **Activity** selector:
- grouped as Trades, Income, Shares and Corporate actions
- arrow keys and localized type-ahead
- share movements named *Shares in*, *Move shares between holdings* and
  *Shares out*, so they never read as the cash Transfer tab

**Changing account**
- It changes labels and which roles need choosing, never the tab positions.
  If the current activity no longer fits, the entry says what to choose and
  keeps the unfinished work.
- It never turns a Withdrawal into a Buy, never picks an activity, and never
  changes which account receives a posting without the owner's choice.

### Account context is not a posting role

A transaction involves accounts in roles:
- A Buy adds shares to a holding and pays from a settlement account.
- A dividend names a stock and may arrive in an ordinary bank account.
- Brokerage cash has ordinary fees, deposits and withdrawals.
- A split or reinvestment may move no cash.

Starting from a bank account never rules out an investment: the muted
*Investment* tab is the route. Selecting it keeps the bank account as a
settlement candidate (*Pay from* or *Pay into*, by activity) and asks for the
holding. Valid
participants come from the backend's existing role and capability rules.

### Settlement account defaults

Today nothing links a holding to its settlement account. The explicit relations that do exist are:
- the account hierarchy: a brokerage container's `brokerage_cash` children
- an import connection's cash account

The rule:
- **The owner's choice wins.** An account the owner selected stays
  selected. Automatic selection applies only while no explicit selection
  exists. A query refresh or a newly found candidate never overwrites it.
- **Candidates** come from explicit relations only:
  - the brokerage container's `brokerage_cash` children
  - the cash account the entry started from

  They are combined and deduplicated, then checked for role, currency and
  validity at the posting date.
- **Preselect** only when exactly one valid candidate remains, and show its
  name. A single eligible cash child is a suggestion, not proof that every
  operation settles there.
- **The owner chooses** when two different valid candidates remain (for
  example the starting bank account and a brokerage cash account), and in
  every other case:
  - several cash currencies
  - no candidate
  - a margin or other permitted liability account
  - a dividend paid elsewhere
- **No cash account** is requested for activities that move no cash.
- **Not defaults:**
  - an import connection's cash mapping, which is evidence for that
    connection's workflow, not a manual-entry default
  - anything guessed from institution names
- **A stored per-holding default** is a separate decision, made only if
  this rule proves insufficient.

### Shared context and working copies

One browser-only entry context holds, by stable ID:
- the latest **main date**
- the holding or brokerage, and settlement-account selections
- the instrument, where it plays the same role
- currency, where applicable
- memo and reference

A value changed in one mode is the value every other mode sees. Restoring Buy's unfinished fields never brings back a date older than one entered in Sell.

Main and secondary dates:
- Each activity declares its **main date**: trade date for trades and write-offs, payment date for income, effective date for transfers and corporate actions. It is shown under that activity's own label.
- Secondary dates stay in their own working copies and are revalidated, never mapped from another role: settlement, ex-dividend, payable, original acquisition and transfer-side posting dates.

Working copies:
- Fields specific to a mode or activity stay in its working copy for as long as the entry is open, so switching back restores them.
- These are unsaved browser state, not `draft` transactions.
- A selection that becomes invalid stays identified, with an explanation and a request to correct it. Nothing is silently substituted.

### Amounts by meaning

- Withdrawal ↔ Deposit keep the positive magnitude.
- Compatible activities keep the share quantity and an explicitly entered unit price (Buy ↔ Sell, Short sale ↔ Short cover).
- Entered charges are financial facts. Each activity keeps its own entered
  charges in its working copy. Derived totals and the cash movement are
  recomputed from the active activity's inputs with its exact arithmetic,
  and the resulting cash movement is shown. Another activity's fee is never
  invented, overwritten or carried without a defined compatibility rule, and
  a net cash figure is never reused as if fees and direction were unchanged.
- An ordinary cash amount never becomes an investment's gross amount, net settlement, fee or cost basis.
- Lot allocations, basis elections, corporate-action ratios and other declarations stay with their own activity.
- Entered facts are never replaced by market prices or inferred values.
- A transfer between the owner's accounts is one operation, and a share purchase is one operation with its cash effect. One movement is never entered twice.

### Switching never posts

- Switching mode, activity or account creates no ledger entries, lot effects or background work.
- Save runs only the active mode's command and validation.
- Any change to financial meaning invalidates a shown preview or approval. A Buy review never authorizes a Sell, and a review for one account never authorizes another.
- While a save is in flight, account, mode and activity cannot change.
- A discard confirmation appears only when work would actually be lost.
- Changing the type of a *posted* operation stays with its correction workflows.

### Move to account (posted transactions)

A register row's *Move to account…* moves the selected posting to another account:
- **Counterparts are kept:** the category and other counterpart postings stay, unless a supported rebuild is required.
- **Ambiguous rows ask:** a row with several postings, or an unclear side, names the affected postings and asks the owner to choose.
- **Transfers:** the owner picks the From or To side; an invalid result, such as a same-account transfer, is refused.
- **Atomic and audited:** the original stays intact until one audited change commits, through the existing edit or correction lifecycle. A failed or stale attempt leaves it untouched.

Guardrails:
- **Reconciliation:** checkpoints are reviewed on both the source and destination accounts, with the existing override.
- **Currency:** the recorded commodity and amount are kept. A destination with a different currency never converts or relabels silently.
- **Eligibility:** the destination must be valid at the posting date.
- **Statement evidence:** it is kept and any disagreement is disclosed (#485).
- **Investment operations:** they move only through investment correction commands. Correcting a wrong brokerage is not a share transfer.
- **Concurrency:** state is rechecked atomically at commit.
- **Type:** a move never changes the operation's financial type.

### Investment-entry hierarchy verification (#507)

The account model permits a holding outside a brokerage container and does not enforce brokerage membership. Manual-entry suggestions therefore use only a holding’s direct brokerage parent (or the starting brokerage container) and that container’s direct `brokerage_cash` children, plus the starting cash account. Nested arbitrary account groups and matching institution names do not establish settlement relationships. Missing hierarchy requires a choice, not a stored default. The dated entry-account read shares investment-command role validation; trade-date holdings and settlement-date cash choices may use different snapshots. Reinvestment selects its cost currency directly because it has no cash leg. See the [implementation contract](../plans/transaction-entry-investment-plan.md); #508 delivers full activity working-copy preservation.


### Entry delivery

| Slice | Issue |
|---|---|
| Entry shell and routing: one *New transaction*, four stable tabs with labels and accents, shared entry context with basic preservation and preview invalidation, editable account | #506, delivered 2026-11-05 |
| Investment mode: brokerage and holding contexts, Activity selector, settlement candidates | #507, delivered 2026-11-07 |
| Activity-specific restoration across activities and accounts, full acceptance coverage | #508, implemented in PR #521, awaiting merge |
| Move to account for posted transactions | #509 |

#491 tracks the first three slices. Move to account is its own issue.

**Delivered in #506 (2026-11-05).** Home's tile, `N`, search's *New
transaction* and *Transfer* commands, Transactions, account registers,
category pages and instrument pages open the one entry; a page with a side
panel hosts it there (search's Transfer included, with the page's account as
From), elsewhere the shared dialog does. Registers lost their separate
Transfer button. A holding register and an instrument page start on
Investment; a category page starts on Deposit for an income category,
Withdrawal otherwise. The tabs are a keyboard tab list (arrows, Home, End;
selection follows focus, which is safe because switching loses nothing),
four equal columns that fit a 390 px sheet, and the editor stays mounted
behind every tab, so its fields are the shared context and working copies.
Withdrawal ↔ Deposit keep the amount; Transfer keeps the date, payee, memo
and the account as From, starts an empty amount from a Withdrawal's, and
keeps its own To, fee and days; the cash tabs come back in the shape they
were left in (simple or split). Mode accents are their own tokens
(`--color-entry-{out,in,transfer,investment}` and `-soft`, pinned at 3:1 for
the indicator and AA for the label in every theme × palette); forced colours
keep a Highlight underline on the selected tab and a dashed one on a muted
tab. Interim choices until the later slices:
- Resolved by #507 (delivered 2026-11-07): the Investment tab shows the
  grouped Activity selector, which opens the existing record forms with the
  page's holding or instrument (#186). Its "Choose a holding account to
  record an investment" note sits at the top of that panel, above the
  selector. The Investments page's record buttons are gone; its header opens
  New transaction on the Investment tab.
- Editing and correcting show no tabs (a posted transaction's type stays
  with its correction workflows). The simple form shows the same positive
  amount with a Withdrawal/Deposit (or Charge/Credit) choice, and keeps its
  Transfer and Split links.
- The approvals this slice has are the payee confirmation and the
  reconciliation override; a tab, account, direction or shape change closes
  either, and the override also rechecks the exact request it reviewed.
  Resolved by #508: investment previews and reviews live in the open
  activity form and close with it, so none outlives a switch.

**Implemented for #508 (2026-11-07, PR #521, awaiting merge).** The entry's shared
context lives in the editor: its date, description and reference are the
main date, memo and reference every tab and activity reads and writes. The
holding, the subject instrument, the currency and an explicitly chosen
settlement account are shared by stable ID (an automatic suggestion is
never shared). The Activity selector remembers its choice across tabs. Each
activity keeps a working copy of its own fields for the life of the entry:
amounts, charges, secondary dates, lot choices, ratios and declarations.
Reopening it restores that copy under the current shared context. An emptied
picker keeps the last shared choice for the others, which validate it
themselves (invalid choices stay identified).
- **Unit price (owner decision 2026-11-07).** The trade forms had no
  unit-price input, so Buy, Sell, Short sale and Short cover gained an
  optional *Unit price*. With it the total, and in exact mode the gross, is
  derived and shown read-only. Gross is quantity × unit price. Net adds
  (buy, cover) or deducts (sell, short sale) each charge in the cash currency
  that is not paid separately, with a rebate the other way: the backend's
  net-settlement rule. A product finer than the currency's standard scale is
  refused with its exact value, never rounded. The price is an entry aid and
  is not stored.
- **Carry pairs.** Buy ↔ Sell and Short sale ↔ Short cover keep the quantity
  and unit price; each keeps its own charges, exact-mode choice, settlement
  date and basis method. A total typed by hand is dropped when the pair's
  quantity or price changed elsewhere, with a note to enter it again.
- **Locking and discarding.** While an investment save is in flight, the
  tabs, the Activity selector and the active form's fields are disabled.
  These all ask *Discard this transaction?*:
  - Cancel and the close button
  - Escape, in the shared dialog or on a phone's panel
  - the phone panel's backdrop
  - a register row opened into the panel
  - leaving the page for another path

  They ask only when entered work would be lost: a field the owner changed
  and did not change back. That includes posting dates, an explicitly
  chosen income account, an explicit Pay from, a holding and a position's
  currency. A default or a picker that fills itself never counts. A
  same-path URL change keeps the entry. Closing the browser tab is left to
  the browser.
- **Cost currency** is the shared currency, used by positions, Shares in's
  basis, a reinvestment's cost currency and a cash-and-stock merger's cash
  currency. A position picker with the same
  instrument in two cost currencies restores the one chosen; a currency the
  holding does not carry selects nothing. The merger's cash currency is
  different: a shared currency the holding has no
  lots in stays chosen, is named as the mismatch, and blocks the preview
  until the owner picks one the holding carries. Changing the holding
  never relabels the cash either: a chosen currency, or the default an
  amount was typed in, stays and is checked against the new holding. A position chosen in an earlier
  activity stays selected when its form reopens. If a new date makes it
  unavailable, it shows as a disabled *earlier choice* with an
  explanation, never a blank picker. Lot quantities entered for lots the
  position does not hold on the new date are named. Nothing is previewed
  until the owner removes them, so a transfer never quietly moves fewer
  units. Only allocations the command would use count: with Return of
  capital's *Specific lot quantities* off, they wait in the working copy.
- **Stacked dialogs.** When a browser closes the entry dialog without a
  cancelable Escape, the dialog reopens before asking, so the discard
  question opens above it and keeps focus.

## Widgets

- A widget is in exactly one place: **My Home** or **More to explore**.
  Adding moves it up; removing returns it to More to explore, so nothing
  disappears and every feature stays discoverable by scrolling.
- More to explore shows widgets at **real size**, exactly as they would look
  on My Home, with a dashed outline, muted text and half their colour
  saturation (full colour on hover or focus) and *Add to Home*. The section's subtitle reads “Add any of
  the widgets below to the home page”. The section can be
  collapsed, not removed. Categories: Everyday, Wealth & investments,
  Planning, Ledger & data.
- My Home is fully reorderable. Sizes: standard, or wide (two columns) where
  a widget supports it. Grid: three columns at desktop width in reading
  order, two on tablets, one on phones. No masonry or free placement.
- Each widget has a ⋯ menu: Move up/down, size, Settings… (period, accounts
  and similar where relevant), Remove from My Home, Open the full page. In
  More to explore: Add to My Home, Add to top, Settings…, Open.
- Home summarises: in the briefing and widgets an amount of 100 or more in
  its currency shows no minor units (€56,750, €612 of €700), rounded half up
  for display only; smaller amounts keep them (€96.40). Registers, reports,
  menus, editors and exports always show the exact amount.
- Widgets are compact: small title, one key figure, at most three or four
  rows or a short chart, and a single *Open* link. White space belongs to the
  page around them, not inside them. Section headings carry no counts.
- Default My Home: Spending this month, Budgets, Accounts, Recurring, Net
  worth trend (wide).
- The Accounts widget lists the owner's pinned accounts, the same list as the
  top of Ledger overview, with a count of the rest; it falls back to the largest balances
  only while nothing is pinned. One owner-chosen list is easier to predict than
  two rankings (2026-10-29).
- In an empty book a single *Start with an account* card sits above My Home and
  widgets show their *no data* state, rather than a grid of empty figures.
- States: loading, ready, zero, no data, not set up (doubles as onboarding,
  for example “Set up backups”), partial, stale, failed. Estimates (forecast)
  are labelled; market-value changes are never called returns.
- Unbuilt capabilities (goals, insights, available-to-spend) do not appear.
- Widgets introduce no rules of their own (owner, 2026-10-31). A threshold,
  ranking, window or setting a widget needs belongs to the feature it
  summarises and applies wherever that feature shows the same thing; when one
  does not exist yet, it is built there first (configurable where the owner
  would tune it), and the widget reads it. Example: *reconciled within 60
  days* is a reconciliation setting that the account list and Review use too,
  not a Home rule.

The capability inventory, which widget uses which existing contract, and the
composed Home read model are specified in the design review.

### Default widgets contract (#478)

`GET /api/v1/home` also returns `widgets`, one bounded summary per default
widget, each `{status, summary}`: `failed` (summary null) when that summary
could not be read, which never fails the briefing or another widget; a
cancelled request is not a widget failure. The trend and the accounts'
balances are the exception: they come from the briefing's own fold, so they
share its snapshot and fail with it. Spending, budgets and recurring are
separate reads, so Home takes a book change marker (the highest audit event,
transaction version, price observation and recurring occurrence) before and
after composing; if a write committed in between, it reads again, and after
three tries returns `consistent: false`, which the page discloses above the
widgets while it reads again. Two cards never show different states of the
book without saying so. Spending bars compare rows only when every row is in
one currency; otherwise rows are listed without bars, as in the report. `commodities` labels every amount
any summary names, so Home needs no second read. Each summary is its canonical
read for the same dates, scope and currency, and Home adds no arithmetic
beyond choosing and ordering rows (`HomeService`, `home_widgets.go`):

| Widget | Canonical read | Summary |
|---|---|---|
| Spending this month | Spending report, by category, 1st of the month to owner-local today, every account, default currency | Converted total (null rather than partial), exact per-currency totals, the report's first three rows in its order, category count |
| Budgets | Budget month | Expense total in one currency (the default currency when it has an expense target, else the lowest that does) and the count of other currencies; three targeted categories, over budget first (most over first), then by share of target used, compared exactly. No target: *not set up* |
| Accounts | Account balances at today, posted, taken from the briefing's fold (the same running balances, one pass over the ledger) | Open asset and liability accounts at top level (holdings and nested accounts roll into their parent), largest value in the default currency first; an account without a value ranks after those with one. A balance in one currency shows in that currency. Pins replace the ranking with #442 |
| Recurring | Recurring templates and the shared review counts | Active (enabled, unarchived) count, drafts and blocked (both shown, and either keeps the widget set up), three schedules by the next due date the Recurring page shows, each with its debit total per commodity |
| Net worth trend (wide) | Net worth series, monthly, from the end of the month twelve months ago to today, folded in the same ledger read as the headlines | Points (null where incomplete; the last two are the headline's start and value) and the change from first to last, present only when both are. One snapshot with the briefing, so it fails with it |

Each widget shows the designed states: *loading* (placeholders holding the
grid's places), *ready*, *zero* (nothing spent), *no data* (empty book),
*not set up* (no budget, no schedule, no default currency), *partial*
(per-currency totals when spending cannot convert, an account without a
value, today's net worth unavailable), *stale* (the trend counts the headline's
out-of-date inputs) and *failed* (in its card, with Retry). Drill-downs are
plain links that keep the canonical page's URL filters: spending rows open
Transactions filtered as the Spending report's drill-down, Open links carry
the period, bucket and reporting currency or the budget month and currency.
Home keeps the read cached across navigation, so Back restores its scroll
without late content moving it.

### More to explore contract (#479)

More to explore lists every widget not on My Home, by category, from the same
`GET /api/v1/home` and the shell's attention read: mounting or expanding it
starts no request and no returns or forecast calculation. Cards are the My
Home cards at real size with a dashed outline, muted text and half
saturation (full colour on hover or focus); the section collapses (the state
is remembered with #438) and *Add to Home* arrives with #438.

| Category | Widget | Source | Summary |
|---|---|---|---|
| Everyday | Cashflow | Cashflow report, 1st of the month to owner-local today, its default liquid-cash scope, one bucket, default currency | Money in, money out, transfers (net) and the operating net, each converted only when every posting it sums converts, else exact per currency; transfers that do not convert keep their exact amounts and the card says so (*partial*), never a silent zero |
| Wealth & investments | Holdings | Holding accounts from the briefing's fold, valued with the accounts widget's rate table (net worth's valuation) | Open positions with a balance, the largest three by value with instrument and brokerage; an unpriced position has no value and ranks last. The summary's own `missing` and `stale` inputs, the prices and the rates behind foreign holdings, judged as the headline judges its own, make it *partial* or *stale* |
| | Returns | The latest saved returns run, frozen | Time-weighted return (and annualized when the run has one) for the run's period, labelled *Saved run* with its save date; *Open* links `?run_id=` and Returns opens and focuses that run (its period alone would start a live read over another scope). No run is *not set up* with a link to calculate. Home never calculates a return; live figures are #440 |
| | Allocation | The same saved run's allocation | By asset type, largest value first, weights as the run froze them; `known_only` is *partial* with the count of balances without a value |
| | Dividend income | Income coverage summary (`SummaryOnly`) | Instruments with confirmed dividends; unassigned investment cash per currency is cash, never income, and leads as payments to assign |
| Planning | Forecast | None (the recurring summary Home already has) | Labelled *Estimate*, names the 90-day horizon and the active schedules it projects, calculated when opened; no schedule is *not set up* |
| Ledger & data | Imports | The shared attention read's `imports` source | Batches to review, the same count Review shows; its own Retry refetches the attention read |
| | Prices | The net-worth headline's `missing` and `stale` inputs | Named as the status line names them, linking to the same filtered Prices list. The headline judges its inputs even when an unclassified short leaves it unavailable, so empty lists always mean checked and current |
| | Backups | The Data screen's backup status | Off is *not set up*; a failed latest run says whether it will retry; otherwise the last backup and the next run. Shown in an empty book too |

Reconciliation joins Ledger & data with the reconciliation out-of-date rule
(#482), which the account list uses as well.

## Customization

- One Customize panel (sliders icon on Home): headline choice (net worth,
  cash, an account or a named account set); whether review notes are mentioned;
  shortcut list with reorder and remove; My Home order with reorder and
  remove; More to explore expanded or collapsed; Reset Home.
- Item-level changes from ⋯ menus save immediately and offer Undo.
- Reordering works with buttons and keyboard everywhere; drag-and-drop is a
  desktop supplement. Nothing depends on hover.

## Persistence

Server-side, per owner and book, in its own table by migration (not the
`user_preferences` time-zone row): one versioned JSON document with headline
choice and account set, shortcuts, My Home order and sizes, widget settings
and section state. It is written whole with a revision number so two tabs
cannot silently overwrite each other, validated on the server, and tolerant
of unknown widgets, archived accounts or removed routes, falling back to
defaults. Because it lives in the database it is included in backups.

## Accessibility and responsiveness

- Real buttons and links (sections are plain links), disclosure buttons
  for the phone menu, settings menu and headline menu, and
  combobox semantics for search and the switcher, visible focus, focus
  kept on a moved item with a live-region announcement, about 44px touch
  targets, reduced-motion support, both themes at WCAG AA contrast.
- One vertical page scroll; no nested card scrollers, sticky hero or
  horizontal carousels. Check 320px reflow and long translations in all six
  locales.

## Navigation beyond the header

Accepted 2026-10-29 (#435). Options and the path map with estimated entry
counts are in the
[navigation and pages review](../reviews/navigation-and-pages-options-2026-10-29.md).

### Three levels

| Level | Examples | Tool |
|---|---|---|
| Section | Ledger, Investments, Planning, Reports, settings | Header menus |
| Page | Transactions, Accounts, Returns, Budgets, Spending | A tab row of the section's fixed pages, under the header |
| Object | Accounts, categories, instruments, payees, import batches | List page, scoped search (switcher), pins, breadcrumbs. Never tabs. |

Views and filters inside a page (Due / Templates, budget month, spending by
category or payee, active / closed) stay in the page as segmented controls,
steppers or filters. On phones the tab row scrolls horizontally.

### Pages and routes

- Every tab is a real route (`/app/reports/spending`,
  `/app/investments/instruments`, `/app/investments/gains`…); old `?view=` and
  `?tab=` links redirect.
- One typed route registry (route, section, title key, icon, search keywords,
  attention source, width) drives menus, tabs, document titles, breadcrumbs
  and the search's page results. A unit test fails when a `+page.svelte`
  route is missing from it.
- Views inside pages: Transactions *All · Drafts · Deleted* (deleted
  transactions replace the Settings "Trash", which only ever held
  transactions); Ledger overview *Accounts · Institutions*; Categories *Categories ·
  Tags*; Import *Files · Connections · Rules and profiles · Dividends*;
  Investments Overview *Positions · Splits · Suggestions*.
- Settings has five pages in one side list, the same list as the settings
  menu: *General* (default currency, time zone, version), *Preferences*
  (language, theme and the Home page: the Customize panel and Reset),
  *Currencies and FX rates*, *Security*, *Data and backups* (backups,
  self-check, a link to Export). The settings overview's link cards go.
- Placement of today's sub-views: dividend review is a view inside Import;
  split history is a section on the instrument page; investment suggestions
  are review items (menu count and Home's review list), reached from
  Overview; ledger CSV and QIF export move to *Reports → Export*, while
  *Settings → Data* keeps backup and restore and links there; FX rates stay
  in *Settings → Currencies*. Settings keeps its own side list.
- Tab counts and Home's status line come from one attention read, replacing
  the shell's recurring-summary query.

### Section-owned paths (#460, 2026-10-30)

Canonical page paths identify the section that owns them. Move the remaining
Ledger and Planning families together, including their detail pages:

| Previous path | Canonical path | Decision and reason |
|---|---|---|
| `/app/accounts` | `/app/ledger/accounts` | Keep the existing list as a temporary Ledger tab. #448 replaces it with Ledger Overview; both list aliases must then lead directly to `/app/ledger`. |
| `/app/accounts/[id]/register` | `/app/ledger/accounts/[id]/register` | The account's ledger activity belongs to Ledger; this detail path survives #448's list replacement. |
| `/app/transactions` | `/app/ledger/transactions` | Ledger owns ordinary entries, drafts and their review filters. |
| `/app/categories` and `/app/categories/[id]` | `/app/ledger/categories` and `/app/ledger/categories/[id]` | Keep the category list and category activity in one family; the Tags view (#444) stays inside it. |
| `/app/import` and `/app/import/dividends` | `/app/ledger/import` and `/app/ledger/import/dividends` | Keep imports and dividend repair together; #447 still owns their view consolidation. |
| `/app/reconcile` | `/app/ledger/reconcile` | Reconciliation verifies ledger account activity. |
| `/app/budgets` | `/app/planning/budgets` | Budgets are Planning, even when linked from spending reports. |
| `/app/recurring` | `/app/planning/recurring` | Due items and templates are views of the same Planning page. |
| `/app/forecast` | `/app/planning/forecast` | Projected balances belong to Planning; their ledger drill-downs remain links. |

Keep `/app`, `/app/review`, all Investments and Reports paths, and the current
Settings paths: these already name their owner or are intentionally standalone.
Settings consolidation remains #446. Section overview paths also stay unchanged.
Do not change `/api/v1` paths or financial contracts when moving browser pages.

The extra path segment makes copied links agree with the header and tab row.
The bounded cost is redirect stubs and mechanical link/test updates; there is
no duplicate screen implementation. Keep every previous path as a registered
legacy route, redirect before rendering, and replace that history entry rather
than adding a Back stop. Keep **every** query parameter (including duplicates,
empty values and unknown keys) and the fragment. Internal links, search and
attention destinations use canonical paths directly. Registry tests check all
aliases; route and subview sweeps exercise the canonical screens, while browser
coverage checks old links, filter consumption, reload, sharing and Back/forward.

### Page header

Breadcrumb on detail pages, the title or the object switcher, a scope line
where the page has one, and a slot on the right for the page's main action.
The shell's description paragraph is dropped; help text moves into empty
states. Transactions, account registers and category pages put *Add* in the
action slot, prefilled for that page, so entry works on phones without a
keyboard shortcut. Widths: narrow (about 720px) for Home's top and forms,
standard (about 1200px) for reports, budgets and investments, fluid (up to
about 1600px) for registers, transactions and import review. `?` opens
keyboard help.

Delivered in #459 (2026-10-30): the page column narrows from the shell's
left edge instead of re-centring, because header, tab row and page share one
left edge at every width (#436); Home may still centre its own top block
inside that column. *N* and search's *Add transaction* (now *New
transaction*, #506) open the current page's editor in place, prefilled, when
the page has one, never replace an
editor that is already open, and otherwise open the
[shared entry dialog](#shared-entry-dialog-477) (#477; until then the
Transactions editor).
Register and category titles name the object, with the parent tab as the
breadcrumb; the *Accounts › Brokerage* and category-tree trails and the
switcher in the title slot remain #442.

### Side panels

**Investment entry** is part of the one entry workflow; see
[Transaction entry](#transaction-entry).

**The task rail is withdrawn (2026-11-05).** On 2026-11-01 a right-hand rail
listing commands and report filters was chosen ([mockup](shell-look-mockup/README.md)).
On review, the owner found one button enough for Investments, and a second
review found the rail's benefit unproven:
- It redistributed 17 types into a list that ran below the fold.
- It hid them again behind *Types…* on narrower screens.
- It would have taken the right column that Investments already uses for a
  position's lots.

No rail is built. What follows records where a right-hand panel does earn
its place, for a later decision.

**Where right-hand panels already work.** Three pages have one. Each holds
what the owner needs *while* working the main list, tied to the current
selection or to the work in progress:
- the transaction detail and editor on Transactions and registers
  (22–26rem, sticky)
- the selected position's lots on Investments (24rem)
- the reconciliation summary (statement balance, cleared, difference,
  finish) on Reconcile

On desktop each sticks below the shell bar, or at its own bottom when it is
taller than the window, so its last control stays reachable with one page
scroll (`side-panel-sticky`, #515).

The first two open as a full-screen modal on phones. Reconcile places its
summary below the list instead, which suits a summary you read after
ticking rows. Both are valid; each page chooses.

**Candidates, deferred** (each needs its own case):
- **Import batch review:** the selected row's details, its link or match
  candidates (#485) and pairing (#487), and the rule that matched. This is
  the strongest candidate: the review table is wide, and row decisions need
  context.
- **Report filters** (#492): a wide-screen filter panel, against a compact
  summary bar that expands in place. Decide with a task check on Spending,
  Cashflow and Net worth.
- **Budgets:** the month's totals (left to assign, overspent) kept in view
  while editing categories.

**What a panel is for:** the current selection or task, *including* the
actions that decide it. Transaction detail has *Edit* and *Correct*;
Reconcile's summary has *Finish* and *Void*, beside the numbers they depend
on.

**Not for:**
- a directory of the page's commands (the withdrawn rail), which belong in
  the page header's action slot, a chooser, or a row's ⋯ menu
- navigation (the account list was rejected 2026-10-29)
- help or promotion

**Rules if one is adopted:**
- One right column per page, shared by selection detail and summary; never
  two panels.
- One width token in place of today's 22, 24 and 26rem.
- Sticky.
- On phones, a full-screen modal or a section below the list, whichever
  fits the task.
- Adoption needs a task check first: the page's common tasks with mouse,
  keyboard and phone, and nothing important left below the fold.

### Objects

- **Switcher = scoped search.** On a detail page the object's name is a
  button (*Checking ▾*) that opens the shared search with a scope chip
  (*Accounts ×*). Scoped, it lists one kind, grouped (accounts by class and
  institution with balances, categories as a tree, instruments held first),
  and Enter reopens the current page for the chosen object, keeping period,
  filters, view and sort where they apply; a filter that cannot apply is
  dropped and shown in the scope line. Removing the chip makes it the global
  search, whose results open an object's default page. The global search's
  empty state offers the same chips. Reconcile uses it to pick the account.
- **Pins.** Up to six accounts, chosen by the owner, shown first on Ledger
  overview and in Home's Accounts widget, with balances. On first use the app
  offers once to pin the three to five accounts with the most postings in the
  last 90 days; afterwards only the owner changes the list.
- **Recent.** Search's empty state and the switcher show the last five
  objects opened (accounts, categories, instruments, payees, reports), below
  the pins, as GitHub, Linear and Raycast do. Kept per browser.
- **Ledger overview** is the complete account index, grouped by institution,
  with a filter and active / closed / all.
- **Holdings** are left out of the account switcher and pins by default and
  reached through Instruments and their broker; typing still finds them.
- **Closed and archived** objects are hidden by default and one control away.
- **Breadcrumbs** follow the hierarchy (*Ledger › Brokerage*,
  *Categories › Housing › Energy*). A holding's register and its instrument
  page link to each other.
- **Payees** get a *Ledger → Payees* page (#443); tags a *Tags* view on Categories (#444).
- **Accounts are grouped by institution**, each group headed by the
  institution and its country (#448). Ledger overview has an *Institutions* view:
  name, country, its accounts and how data arrives (file, profile or online
  connection), with add, edit and archive.
- **A brokerage is one account** with cash and positions. Per-instrument
  holding accounts stay in the ledger model but are never listed as
  accounts, pinned or offered in the account switcher; their positions show
  on the brokerage account page and on the instrument page.
- **Page actions:** every page that offers entry shows the one *New
  transaction* button, opening the [one entry workflow](#transaction-entry)
  prefilled from the page. A brokerage offers the Investment mode beside
  Withdrawal, Deposit and Transfer, so ordinary cash movements and fees stay
  in the same entry.
- Before #442 fixes grouping and the pin limit, count a real book (#449) (accounts
  by kind, holdings per broker, closed accounts, categories by depth, payees,
  postings per account over 90 days) and record the result in the review.

### Other shell details

- **Density:** registers, Transactions and import review offer
  *Comfortable* and *Compact* rows, remembered per browser, as Gmail and
  Linear do; compact keeps 24px minimum targets.
- **Two-key shortcuts:** `g` then a letter goes to a page (`g h` Home, `g l`
  Ledger overview, `g t` Transactions, `g i` Investments, `g p` Planning,
  `g r` Reports, `g s` Settings), listed in `?` help and never active in
  editable fields.
- Hide amounts is applied in `$lib/money/format.ts`, covers chart labels,
  leaves exports, edit fields and stored data unchanged, and is remembered
  per browser. Read-only security quantities, prices and FX ratios are masked
  too, using the same fixed four bullets regardless of magnitude. Dimensionless
  percentage returns, allocation/spending shares, dates, counts, identifiers and
  chart geometry remain visible. This is a presentation control: editable fields
  and raw source files are visible when deliberately opened; exports retain exact
  data. Masking is read before the first render and synchronized across tabs.
- Sign-in, setup and recovery lose the root top bar and get a minimal header
  with name, language and theme.
- Back to Home restores its scroll position.

## Delivery slices

Sequencing belongs in the roadmap; acceptance in the issues.

| Slice | Issue |
|---|---|
| Shell: header, section links and settings menu, tab row of real routes, route registry, hide amounts, shared search, attention read; retires the sidebar and the duplicate top bar | #436 (header, registry and tab routes #455; hide amounts #456; search #457; attention read #458; page header, widths and `?` help #459) |
| Route paths of pages outside their section | #460 |
| Home: briefing, command bar, shortcuts with the shared Add transaction dialog, default My Home and More to explore, composed Home read | #437 (read and briefing #476; command bar, shortcuts and entry #477; My Home widgets #478, delivered; More to explore #479, delivered) |
| Review page: everything needing attention and background status | #453 |
| Settings in five pages; Trash becomes Transactions › Deleted; ledger export moves to Reports › Export | #446 |
| Import views: Files, Connections, Rules and profiles, Dividends | #447 |
| Ledger overview: accounts by institution, Institutions view, brokerage as one account, *New transaction* in brokerage context | #448 |
| Planning and Reports overviews | #450 |
| Payees page | #443 |
| Tags view | #444 |
| Real-book count (D11), owner-run | #449 |
| Object switcher, pins and breadcrumbs | #442 |
| Personalization: layout migration and API, headline choices, Customize panel (also in Settings › Preferences), ⋯ actions with Undo, sizes and settings | #438 |
| Search depth: payee actions with prefilled editor and last-used category, full-text transaction search | #439 |
| Widget depth: lazy expanded returns, allocation and forecast charts; further widgets | #440 |
| Density: compact and comfortable rows | #451 |
| Shell look: olive two-tint rows, joined section tab, green place markers, card contrast | #489 |
| Search button with centred search dialog | #490 |
| One transaction entry: shell and ordinary modes (#506, delivered), Investment mode (#507, delivered), preservation (#508, implemented in PR #521) | #491 |
| Move to account for posted transactions | #509 |
| Report filters take less space above the figures (later: filter bar or side panel, after a task check) | #492 |
| Two-key navigation shortcuts | #452 |

## Considered and dropped

- A slot for widgets beside the headline (Focus): added noise; widgets live
  in My Home.
- A third briefing line for notable moves or spending pace.
- *Add transaction* as a header button: it is the first shortcut instead.
- The dark green top bar and the left navigation sidebar. A dark brand bar
  was reconsidered on 2026-11-01 and set aside for the olive tints.
- A header in the page colour (2026-10-29 to 2026-11-01): too flat.
- A compact search field in the header: replaced by the *Search Ctrl K*
  button.
- A wall of record buttons, and folding them into a small dropdown. *Record
  buy* as the Investments main action. Separate launchers for ordinary and
  investment entry. All replaced by the one *New transaction* entry.
- A task rail listing commands (chosen 2026-11-01, withdrawn 2026-11-05):
  see [side panels](#side-panels).
- Discarding everything on a type change, and carrying a Buy's net cash or
  charges into a Sell (both proposed and withdrawn on 2026-11-05): replaced by
  the preservation rules in [Transaction entry](#transaction-entry).
- A fixed-order map with a separate shortlist: replaced by My Home plus More
  to explore.
- Compact previews in More to explore: real-size widgets instead.
- Hover-opened section menus and `role="menu"` for site navigation; later
  section menus altogether, for plain section links and Overview tabs.
- Capitals for section labels, and an app-name wordmark inside the app.
- A left account rail beside registers (O3).
- A separate object picker beside the search: the switcher is the search
  with a scope chip.
- Tabs for objects such as accounts or instruments.
- Usage-ordered pins that rearrange themselves.
- Workspaces with separate layouts, bottom navigation on phones, usage-based
  automatic placement.

## Attention foundation (#458)

`GET /api/v1/attention` is the session-only composed read. The shell owns one
`['api', 'attention']` query and provides its reactive result to the header,
registry-driven tab badges, Home's status line (#476) and the bounded
`/app/review` landing page. It refreshes every 30 seconds while visible, on
focus, after successful typed-client writes and on explicit Retry. Home's
briefing follows the [briefing contract](#briefing-contract-476); Review's Needs action / Notes / Status design, per-source
last-run details and retention remain #453. Entered values join this read when
#454 supplies the shared coverage; their absence is not a claim about asset
valuation completeness. FX pair discovery includes nonzero posted currency
balances on asset/liability accounts, including closed accounts, alongside
account defaults and configured pairs (#464). The
[pricing contract](../plans/pricing-management-plan.md#posted-currency-discovery-464)
defines exact zero, lifecycle and date scope. Zero attention is not an assurance
that every historical or reporting-currency conversion is available; Home's
reporting consumer must retain its own dated coverage.

Each response samples independent sources; it is not a transactionally frozen
book snapshot. `checked_at` is the UTC start time of that read. Source units are:

| Source | Unit and deduplication |
|---|---|
| Drafts | Each undeleted current draft transaction, once regardless of posting count. |
| Transaction review | Each undeleted current posted transaction with `needs_review`, once. Drafts are excluded. |
| Blocked recurring | Each blocked occurrence identity; generated and skipped occurrences are excluded. |
| Imports | Each previewing/partially committed batch with actionable pending/failed rows, or failed batch (including parse failure without rows), once. Duplicate/excluded-only previews and committed/discarded batches are excluded. Dividend rows pending in a batch are included here, not counted again as suggestions. |
| Suggestions | Each provider event with a suggested/failed investment event suggestion, once; accepted, linked, ignored, auto-posted and superseded suggestions are excluded. |
| Connections | Each connection whose latest fetch failed, once. |
| Refresh | Each market-data source with an unresolved refresh-state error or a failed/partial latest ingest run, once across pairs and runs. |
| Backup / self-check | The latest recorded run of each kind, not historical failures. A failed/errored run contributes one; self-check warning/incomplete contributes one. Pending/running is unknown, never zero. No recorded run has the explicit `never` status. A later successful run clears its predecessor. |
| Prices / rates | Each distinct non-usable base/quote pair in the current owner-local Pricing workspace, separated by whether the base is a currency. The shared `live_raw_v1` observation owner and pricing staleness policy decide coverage, bounded by the reports' window and judged by the reports' rate table, so a quote valuation refuses (conflicting splits) counts. Posted balances are discovered for currencies and for instruments nothing prices yet; there is no separate attention valuation calculation. |

The total counts **action units**, not unique objects across sources: a failed
refresh and a missing price can both need action. Read failures retain successful
sources, set `state=error` with a sanitized code and `count=null`; pending work
uses `state=loading`. Running connection/refresh aggregates retain known failures
as a partial count, or null when no result is known yet. `total_known` is a lower
bound unless `complete=true`. Header and affected tab badges show `!` for failed reads or
operations and `…` while unknown. Accessible labels state the known count and
coverage. Failures have no mute setting, and cached successful data must not
produce “All clear” after a request fails. The empty message is scoped to the
available checks, and is shown only for a complete, successful zero result.
GET never contacts providers, posts, generates drafts or starts checks.
