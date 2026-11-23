# Bug classes — Frontend

Part of the `validate-and-ship` review checklist. Anything under
`frontend/src`: forms, editors, dialogs, tokens, i18n, layout. Every entry
shipped as a real bug in this repo. Numbers are stable IDs cited from code
comments and docs; they are not an order and are never reused.

## 6. Unconsumed pagination

Frontend fetching page one and ignoring `next_cursor` (T-05).

## 9. i18n bypass

Hard-coded English in UI or in `lib/api/`.

## 16. Animating a control's disabled fade

`disabled:opacity-60` is safe on its own: the contrast rules exempt an
inactive control, and axe skips disabled elements for `color-contrast`
outright. Pairing it with a bare `transition` is not, because `transition`
covers opacity: clearing `disabled` removes the exemption one to three frames
before the 150ms ramp off 0.6 finishes, and an axe run that lands in that gap
measures an operable control at 4.37:1 (T-93, an intermittent failure of
`[acceptance] every report view is accessible`). Use `transition-colors` on
any control whose disabled state clears *on its own* — a query settling rather
than a click — so opacity stays a step function. Synchronising the test
instead only moves the race, and costs the check its view of the loading
state.

## 28. Contrast measured in one palette only

axe checks the theme a browser case renders, and the R3a contrast pass
re-derived only the light default palette, so the warning and danger badges,
accent badges over row hover, and four alternative accent palettes sat below
4.5:1 unnoticed. Any token change runs `src/lib/theme-contrast.test.ts`, which
computes every theme × base × accent pairing from `app.css`; a new
text-on-tint pairing gets a line there. Named check: `theme-contrast.test.ts`
(30 combinations).

## 29. Position identity dropping cost currency

One holding and instrument can contain several separately denominated
positions. Svelte row keys, selection highlighting and lot-detail filters must
include cost currency; a two-field key crashes the whole overview after a
currency-changing correction. Browser regression: the currency-changing case
in `investments-cash-in-lieu.spec.ts` visits the overview afterward and opens
both positions, checking that each shows only its own lots. Entry requests
must carry that selection through the application and disposal command too,
including zero-proceeds write-offs with no cash currency. Named regression:
`TestWriteOffMixedCurrenciesEachMethodConsumesOnlySelectedCurrency` (#199);
historical entry and native correction retain the same pin.

## 52. Colour utility naming no token

Tailwind emits nothing for an unknown colour, so `text-destructive`,
`text-warning-foreground` or `bg-accent-soft` compile silently and leave the
element its inherited colour: 22 investment errors and negative gains lost
their danger colour, and the danger/warning confirmation buttons drew
foreground text on their own fill (1.6:1 dark) (#324). Use only `@theme
inline` tokens; text on a danger or warning fill is `text-background`. Named
check: `src/lib/theme-tokens.test.ts` parses every `.svelte`/`.ts` source and
reads markup attribute values (not `style`), `class:` directives and
string/template literals; CSS, comments and text are skipped, so
`border-radius: 0;` is not a colour (#462).

## 54. Hand-rolled modal overlay

A `fixed inset-0` div with `aria-modal` hides nothing from the keyboard: focus
stayed on the trigger, Tab walked the blurred page behind, Escape did nothing
and Cancel dropped focus on `<body>` (#327). Use `ModalDialog`; a native
`<dialog>` alone still lets Tab leave for the browser chrome, which the
component wraps. A window `keydown` Escape handler also fires under a nested
dialog — let the dialog's `cancel` own Escape. A panel that covers the page
only below `lg` attaches `phoneModal` (`modal-focus.ts`) instead (#342); a row
that opens it on Enter must `preventDefault`, or the key activates the panel
control that just took focus. Named checks: `dialog-focus.spec.ts`,
`panel-focus.spec.ts`.

## 55. Layout and hover states no sweep looked at

Categories scrolled 118px sideways at 1280px once its `xl:` toolbar row
applied, because the route sweep only measured 390/640px (#340).
`hover:opacity-90` on a warning fill read 4.2:1 (#341): contrast tests covered
the resting state only. Measure overflow at desktop too, and pin hovered
pairs. Interactive sub-views (dialogs, forms, panels, import review) are swept
by `subview-sweep.spec.ts`; add new ones to its list.

## 63. Date-shaped URL values mistaken for calendar dates

A YYYY-MM-DD regex admitted February 31 and month 13, causing shared
transaction links to fail API validation (#377). Reuse `isCalendarDate`, keep
valid leap days and exercise malformed links through the real list
(`url-selection.spec.ts`). URL selection journeys also check reload, Back and
sharing: SvelteKit 3 exposes shallow URL changes through `page.shallow.url`
(#326). Forecast history URL/form checks also share `isCalendarDate` (#380):
regex plus `Date.parse` normalizes impossible dates. Keep ordinary/leap round
trips and the no-forecast-request invalid-link control in
`forecast-learning.spec.ts` green.

Transactions, account registers, category activity and instrument activity
keep individually valid From and To values while a range is inverted (#628).
Their requests use `transactionRequestFilters` to omit the inverted pair.
The shared filter bar explains that the list is showing transactions without
a date range. `url-selection.spec.ts` covers keyboard entry, changing a valid
range to an inverted one, and reloading the resulting link.

## 69. New version token attached to old form fields

Imported-dividend repair captured its editable resolution once but read the
version token from reactive list props (#433). A reconnect preserved unsaved
input while silently accepting the newer version. Capture fields and token
together; reload both only deliberately. Exercise a concurrent category edit
followed by list refetch through the real browser/API, then require the stale
refusal and unchanged newer journal (`import-dividend-review.spec.ts`).

## 70. A new event source falling through to an old meaning

Forecast day details treated every non-posted/non-draft event as a recurring
template, including learned estimates with no saved record (#434). Map labels,
tone and navigation exhaustively over the API source union; an unknown source
must not silently acquire another source's meaning. Expand real learned events
in browser coverage, require an estimate label and no recurring link, and keep
the detail span aligned with visible columns (`forecast-learning.spec.ts`,
`forecast.spec.ts`).

## 71. Long identifiers setting a grid's intrinsic width

Pricing's pair button and native selects expanded the phone layout even when
cards used `min-w-0` (#387). Constrain controls and their grid labels, and use
`wrap-anywhere` for complete identifiers and exact price/unit text so their
minimum content width can shrink. Never mask it with page overflow hiding or
truncate financial evidence. `pricing-long-code.spec.ts` owns its book, seeds
long codes explicitly, and checks page, control and rendered-text bounds in
six locales and both themes, including correction review.

## 72. Translated accessible names splitting their visible label

The instruments list's Dutch and German activity links inserted the instrument
name between the words of the visible action, failing axe's `label-in-name`
rule (#455). Keep the visible phrase together in the accessible name in every
locale. Catalog parity cannot check this. Moving a view to its own route must
add it to the route sweep; the instruments list had previously lived behind
`?tab=instruments` and escaped that coverage. Named check:
`route-sweep.spec.ts` → `every route fits the phone and declares its language
in every locale`.

## 73. An active tab geometrically visible beneath an overflow fade

`scrollIntoView` placed a middle Investments tab at the scroller's edge,
beneath its 40px mask (#455). Match scroll padding to the fade width and keep
the active tab visible after resize and font changes. Named check:
`shell-navigation.spec.ts` → `the current tab stays visible when a desktop
page narrows to phone width` tests both a last and a middle tab against the
unmasked bounds.

## 77. Navigation completion clearing a newer interaction

Shared search's `afterNavigate` reset and post-`await goto` close erased a
query reopened on Spending while its default range was being repaired (#472).
Reset when leaving a pathname, preserve same-page query/fragment changes, and
never let an older selection's completion dismiss the next search. Control
input at actual history updates and check retained query/results after
rendering; also verify genuine page departure still dismisses search
(`shared-search.spec.ts`, `search entered during Spending range repair …
(#472)`).

In Reconcile, a late Finish or Discard response replaced the session opened
while the write was pending (#642). Refresh the affected reads after the
write and apply the view transition only when the responding session ID is
still active. `reconciliation-selection.spec.ts` holds both writes and checks
the newer session's selection, the saved outcome, and checkpoint history.
When the owner is still on the start form they returned to, show the earlier
statement's outcome without remounting the form (#647). A failed response
requires a fresh read of that session before offering to resume it. Status
retries only read. The same spec covers late success, failed status recovery,
and a committed Finish with a lost response.

## 78. A dismissal guard that sees only part of a write chain

The editor's busy state covered the save but not the payee it creates first,
so Escape closed the entry dialog mid-chain and the continuation still posted
it (#477). Busy (`aria-busy`, which `ModalDialog`/`phoneModal` honour) spans
every write that leads to the commit, and an unmounted form never continues to
its own save. Related in the same review: a write from a dialog over any page
refreshes every read it can change (`invalidateTransactionReads`, Prices
coverage included), and a shell error state must not unmount an open working
copy; a dialog that outlives its opener needs `fallbackFocus`. Suggestion-list
options are never Tab stops: one that closes on blur drops focus out of the
dialog. Named checks: `home-entry.spec.ts` (payee chain, report refresh,
failed session refresh, keyboard category), `invalidate.test.ts`,
`suggestion-keys.test.ts`.

## 79. An editor that restates more than the owner changed

#481 took three
reviews: a journal the server could rebuild exactly was still unsafe for
the form (a refund read as a fee), the fallback split dropped system
postings, mode switches rebuilt postings without their line keys (a
cleared posting came back uncleared, a reconciled one asked for an
override), and metadata round-tripped through JavaScript objects (an
integer above 2^53 rounded). Acceptance for an editor is the browser's own
read → edit → save and every mode switch, keeping amounts, days,
accounts, currencies, memos, line keys, reconciliation state and recorded
choices; each transition applies the same eligibility and carry rules as
the initial load. Data the form does not show stays on the server: an
omitted field on an edit keeps the stored value exactly (metadata by line
key, the #311 pattern), never a client copy. Named checks:
`transfer-entry.spec.ts` (preservation gates),
`transaction-edit-preservation.spec.ts`,
`TestTransactionEditKeepsOmittedMetadataExactly`.

## 85. A scrollable combobox popup that becomes a Tab stop

The Category option buttons already had `tabindex="-1"`, but Chromium still
tabbed to the overflowing listbox. The input's blur timeout removed the popup
and focus fell to the document body during the Home entry journey (PR #634).
Shared Search also had an extra Tab stop when its results overflowed.
When the input owns focus through `aria-activedescendant`, set
`tabindex="-1"` on the scroller and on the option buttons. Exercise real
overflow in `home-entry.spec.ts` and `header-search.spec.ts`. The source guard
`listbox-focus.test.ts` covers scrollable listbox utilities and exempts the
instrument pickers whose option buttons participate in the Tab sequence.
Keep focus on the input during pointer interaction with a popup that closes on
blur (`category-scrollbar.spec.ts` exercises the native drag). When selecting
replaces the input, move focus to the replacement control for both pointer and
keyboard selection (`home-entry.spec.ts`).

## 86. A final action enabled by an older saved selection

Creates that duplicate on retry (#660) are guarded by request keys
(`docs/conventions.md` → *Create Request Keys*): a retry resends the exact
unresolved save under the same key. Open instance: a preview-first investment
create reopened after a lost response must preview again, and that preview
refuses what the lost save already moved (#677).

Reconcile could write a checkpoint while a posting deselection was still
being saved (#633). Use the same readiness check in the button and its
handler, and keep conflicting actions disabled through the write. A lost
response can follow a commit, so read the session again after a failed save
and bring the native checkbox back in sync before enabling Finish. The real
app cases in `reconciliation-selection.spec.ts` hold the PATCH, check that no
checkpoint is written, and cover failures before and after the server commits.
