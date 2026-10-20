# Interactive sub-view audit: dialogs, record forms, panels, import review and editors — 2026-10-19

**Status:** [#331](https://github.com/sergeyfarin/rekenraam/issues/331), an
additional audit of area 10 of
[#260](https://github.com/sergeyfarin/rekenraam/issues/260), created by
[#278](https://github.com/sergeyfarin/rekenraam/issues/278). It ran after
[#327](https://github.com/sergeyfarin/rekenraam/issues/327) landed, so the
dialogs were checked in their final shared structure. Every listed sub-view
was opened and checked, or marked not reachable with a reason. Four findings
were filed. Two were fixed in this change, each with a regression:
[#340](https://github.com/sergeyfarin/rekenraam/issues/340) and
[#341](https://github.com/sergeyfarin/rekenraam/issues/341) (P3). Two remain
open, each with its impact stated:
[#342](https://github.com/sergeyfarin/rekenraam/issues/342) (P2, phone-width
side panels do not take, keep or return focus) and
[#339](https://github.com/sergeyfarin/rekenraam/issues/339) (P2, backend:
account and version dates are checked against the UTC day, not the owner's).
No finding affects financial data. This is bounded evidence on the states
each check reaches. It does not certify every state of every screen, a
native-language review (#102), or release readiness.

## Evidence limitation identified by #280

[#348](https://github.com/sergeyfarin/rekenraam/issues/348) demonstrated that the
original nested-dialog pixel override could accept low-contrast text when a border
or icon had higher contrast. A real Chromium negative control yielded one axe
violation but zero retained violations and a reported 21:1 border ratio. The
matrix's pixel-override rows (¹ below) therefore do **not** establish text
contrast; they remain the historical observation. Other axe/fit/close checks
retain their bounded evidence. #344's separate
shared-worker fixture/fit failures are repaired with explicit long-option and
24-place lot fixtures; the complete 352-journey gate passes at `af233abb`
([follow-up evidence](subview-shared-book-fixes-2026-10-19.md)).

**Resolved by #348 (2026-10-20).** The override now measures text only
(`e2e/playwright/support/rendered-contrast.ts`): each node is screenshotted
as rendered and again with its own text fill transparent, so the changed
pixels are its glyphs and the second shot is the background rendered under
them. Borders, icons and child elements are identical in both shots and
cannot stand in for text. The 4.5:1 threshold is unchanged, and a kept or
dropped finding keeps axe's own diagnostics with the measured text and
background colours. `rendered-contrast.spec.ts` holds the controls: `#999`
text with a black border and a black icon stays a failure (2.85:1), `#333`
text passes at 12.63:1, and a finding outside a nested dialog is never
re-measured. In the post-fix sweep (all eight configurations), axe raised no
nested-dialog contrast finding, so no cell relies on the override. The
`de 390` configuration failed one unrelated fit check (`add currency`, 8 px
page scroll). The unmodified `a7d125e` fails identically in that sandbox,
while CI at the same commit is green.

## Scope and baseline

- Baseline: `46e2db2e` (#327, every modal on the shared native
  `ModalDialog`), clean working tree.
- Governing sources: `AGENTS.md` frontend rules, the `frontend-screen` skill,
  [the shared audit brief](app-state-and-audit-plan-2026-10-17.md), the #278
  [route audit](frontend-audit-2026-10-18.md) and its evidence contract.
- Book: the provider-free #266 reference book (`seedReferenceBook`). The
  sweep adds the following on top of it, each on its own accounts or
  instruments so the book's own figures are not read:
  - a deleted spend under a fresh statement on the book's checking account
    (trash override);
  - a FIFO position with two buys and two sales, whose first sale reverses,
    and first buy corrects, into a gain review. Every buy in the book has a
    dependant, so its reversal is refused, not reviewed;
  - a Trading 212 stub connection that stages a synthetic short fill and a
    `STOCK_SPLIT` fill, each with a recorded operation to link to;
  - a committed stub dividend that waits in review.
- Independent expectations: WCAG 2.1 A/AA through axe 4.14 (rules, not app
  output); a page and a sub-view that never scroll sideways and stay inside
  the viewport; a dismissible sub-view that leaves no open dialog behind it,
  and a nested review that closes back into its parent. Contrast pairs are
  computed from the `app.css` tokens with the WCAG formula
  (`theme-contrast.test.ts`), never read from the page, except where noted
  under axe's nested-dialog limit below.
- New consumer: `e2e/playwright/subview-sweep.spec.ts`. It runs eight
  configurations as parallel tests, each on its own worker book: light and
  dark at 1280 px and 390 px, and German and Russian at both widths. In each,
  it opens every sub-view below from its own route, runs axe on the page,
  measures the page's and the sub-view's sideways scroll and the sub-view's
  box against the viewport, then closes it and checks that it is gone. Each
  configuration writes its matrix row to `e2e/test-results/subview-sweep/`.
  Its controls are found through the message catalogs of the active locale,
  so the German and Russian runs open the same sub-views by their translated
  names. Dialog focus behaviour is #327's (`dialog-focus.spec.ts`) and is not
  repeated here.
- Write guard: during the sweep every non-GET request is aborted unless it
  only previews (`impact`/`preview` paths) or the sub-view declares the write
  it needs to reach its state: a CSV upload, a provider fetch, or a void or
  edit the server refuses for review. A preview that would have committed
  shows an error instead of writing.

## Findings

| Finding | Severity | Expected versus actual | Disposition |
|---|---|---|---|
| [#342](https://github.com/sergeyfarin/rekenraam/issues/342): phone-width side panels | P2 | Below `lg`, the transaction workspace panel (detail, edit, correct, create) and the investments lot panel are full-screen overlays. Like a dialog, they should take focus, keep it and return it. A 390 px keyboard probe: focus **stayed on the opener behind the overlay**; 25 Tabs walked the top bar, the nav, *New transaction*, the filters and the position rows; Escape did nothing. axe passes because the covered page is still in the tree. | **Confirmed, fixed in #342.** At `lg` and above these panels are side columns and must stay non-modal, so the fix is a phone-only modal contract (reusing #327's), not a mechanical swap. Impact: a phone keyboard user operates hidden controls. Writes still need their own confirmation. |
| [#339](https://github.com/sergeyfarin/rekenraam/issues/339): UTC day for account and version dates | P2 | Financial Date Defaults: owner-entered dates follow `owner_today`. At 00:01 CEST the session said `2026-10-19`, but `POST /api/v1/accounts` refused `opened_on: 2026-10-19` as in the future. `cleanOptionalAccountDate` and the version effective-date check compare with `now.UTC()`. | **Confirmed, open** (backend, outside this frontend audit's fix scope). Impact: east of UTC, an owner cannot open an account or date a version "today" for the first hours of the day, and blanks default to the UTC day. The sweep opens its own accounts on a fixed past date. |
| [#340](https://github.com/sergeyfarin/rekenraam/issues/340): Categories scrolls sideways at desktop | P3 | No page scrolls sideways. At 1280 px `/app/categories` scrolled **118 px** (de 186, ru 250) once its toolbar became a row at `xl`: the filter grid's fixed tracks (about 34rem) were squeezed to 200 px. The sweep first saw it with the category editor open. The landing page has it too, which the #278 route sweep missed because it checked overflow only at 390 and 640 px. | **Confirmed, fixed.** The toolbar wraps the filters onto their own line when the row cannot hold them. Measured 0 px from 800 to 1920 px in six locales on Categories and Accounts. `route-sweep.spec.ts` now checks sideways scroll at desktop in both themes and in every locale. |
| [#341](https://github.com/sergeyfarin/rekenraam/issues/341): hovered warning confirmations | P3 | Text on a filled confirmation stays at 4.5:1. axe flagged *Accept changed gains* while the pointer rested on it. `hover:opacity-90` fades the button over the surface: warning **4.16–4.21:1** in light (danger 4.68–4.73). | **Confirmed, fixed.** Warning confirmations keep their fill on hover and show a warning ring. `theme-contrast.test.ts` pins the hovered danger pair, and `theme-tokens.test.ts` fails on any `bg-warning` control with `hover:opacity-*` (it lists `reconciliation-confirm.svelte:83` on the baseline). A `brightness()` darkening was tried first and dropped: axe measured it inconsistently under the pointer. |

### Accepted limit: axe and nested modal dialogs

The reconciliation and gain review opens inside the correction dialog that
asked for it: a modal `<dialog>` inside another. axe sometimes measured its
confirm button as if the outer modal's backdrop covered it. It reported
1.99:1 and 3.2:1 in some desktop runs, while a screenshot shows the opaque
button on top at about 5:1. For a `color-contrast` hit whose every node sits
in a nested open dialog, the sweep re-measures the rendered pixels. It
keeps the hit only below 4.5:1 and records the rendered ratio in the matrix
cell's `note`. The measurement originally compared the element's commonest
colour with the pixel that contrasts most with it; #348 replaced that
with a text-only measurement (above). In the final run of this audit, three desktop cells carry such a note:
5.39:1 in light and German and 9.24:1 in dark. The token computation gives
5.12:1 and 9.26:1 at rest. Hits outside nested dialogs are never
re-measured.

## Sub-view × check matrix

**V** verified by `subview-sweep.spec.ts` in this run: axe clean, no sideways
scroll of the page or the sub-view, inside the viewport, and closed cleanly.
Columns are the eight configurations. **n/a close**: an inline state with
nothing to dismiss (the budget validation error, the pending dividend review).
**—** not reachable, with the reason in the last column.

| Sub-view | Light 1280 | Light 390 | Dark 1280 | Dark 390 | de 1280 | de 390 | ru 1280 | ru 390 | Not reachable because |
|---|---|---|---|---|---|---|---|---|---|
| void reason | V | V | V | V | V | V | V | V | |
| unvoid reason | V | V | V | V | V | V | V | V | |
| delete reason | V | V | V | V | V | V | V | V | |
| restore reason (detail panel) | — | — | — | — | — | — | — | — | transaction-detail-panel keeps a `restore` reason branch that nothing opens: a deleted transaction is not served by the ordinary read, and restore lives in Settings → Trash |
| void reconciliation override | V | V | V | V | V | V | V | V | |
| edit reconciliation override | V | V | V | V | V | V | V | V | |
| checkpoint void | V | V | V | V | V | V | V | V | |
| trash restore override | V | V | V | V | V | V | V | V | |
| add currency | V | V | V | V | V | V | V | V | |
| account currency picker | V | V | V | V | V | V | V | V | |
| price form (add) | V | V | V | V | V | V | V | V | |
| price form (correct) | V | V | V | V | V | V | V | V | |
| price form (withdraw) | V | V | V | V | V | V | V | V | |
| recurring post review | V | V | V | V | V | V | V | V | |
| recurring draft edit | V | V | V | V | V | V | V | V | |
| recurring discard | V | V | V | V | V | V | V | V | |
| recurring template editor | V | V | V | V | V | V | V | V | |
| split activity: correct | V | V | V | V | V | V | V | V | |
| split activity: reverse | V | V | V | V | V | V | V | V | |
| split activity: reverse submitted | V | V | V | V | V | V | V | V | |
| split activity: cash in lieu | V | V | V | V | V | V | V | V | |
| record buy | V | V | V | V | V | V | V | V | |
| record sell | V | V | V | V | V | V | V | V | |
| record short sale | V | V | V | V | V | V | V | V | |
| record short cover | V | V | V | V | V | V | V | V | |
| record payment in lieu | V | V | V | V | V | V | V | V | |
| record dividend | V | V | V | V | V | V | V | V | |
| record withholding | V | V | V | V | V | V | V | V | |
| record reinvested dividend | V | V | V | V | V | V | V | V | |
| record transfer in | V | V | V | V | V | V | V | V | |
| record move between holdings | V | V | V | V | V | V | V | V | |
| record transfer out | V | V | V | V | V | V | V | V | |
| record split | V | V | V | V | V | V | V | V | |
| record share exchange | V | V | V | V | V | V | V | V | |
| record cash-and-stock merger | V | V | V | V | V | V | V | V | |
| record spin-off | V | V | V | V | V | V | V | V | |
| record return of capital | V | V | V | V | V | V | V | V | |
| record write-off | V | V | V | V | V | V | V | V | |
| correct buy | V | V | V | V | V | V | V | V | |
| reverse buy | V | V | V | V | V | V | V | V | |
| reverse sale: gain review | V | V | V | V | V | V | V | V | |
| correct buy: reconciliation confirm | V ¹ | V | V ¹ | V | V ¹ | V | V | V | |
| correct sale | V | V | V | V | V | V | V | V | |
| reverse sale | V | V | V | V | V | V | V | V | |
| correct dividend | V | V | V | V | V | V | V | V | |
| correct split | V | V | V | V | V | V | V | V | |
| correct transfer in | V | V | V | V | V | V | V | V | |
| resolve unknown basis | V | V | V | V | V | V | V | V | |
| correct short sale | V | V | V | V | V | V | V | V | |
| correct short cover | V | V | V | V | V | V | V | V | |
| correct return of capital | V | V | V | V | V | V | V | V | |
| correct payment in lieu | V | V | V | V | V | V | V | V | |
| correct write-off | — | — | — | — | — | — | — | — | no write-off in the reference book; its form is the record write-off form plus write-off-correction-form, checked by investments-write-off-correction.spec.ts |
| correct share exchange / spin-off / merger | — | — | — | — | — | — | — | — | no corporate action in the reference book; the correction dialogs render the same forms as the record forms swept above |
| correct cash in lieu | — | — | — | — | — | — | — | — | no settled cash in lieu in the reference book; the dialog renders the cash-in-lieu form swept under split activity |
| correct resolved basis | — | — | — | — | — | — | — | — | the reference book leaves G1 unresolved; resolving it would change every later configuration |
| transaction detail panel | V | V | V | V | V | V | V | V | |
| investment detail panel | V | V | V | V | V | V | V | V | |
| transaction editor | V | V | V | V | V | V | V | V | |
| lot panel | V | V | V | V | V | V | V | V | |
| account editor (new) | V | V | V | V | V | V | V | V | |
| institution editor (new) | V | V | V | V | V | V | V | V | |
| category editor (new) | V | V | V | V | V | V | V | V | |
| budget target: invalid amount | V (n/a close) | V (n/a close) | V (n/a close) | V (n/a close) | V (n/a close) | V (n/a close) | V (n/a close) | V (n/a close) | |
| staged CSV preview with payee resolution | V | V | V | V | V | V | V | V | |
| staged CSV preview with a rejected row | V | V | V | V | V | V | V | V | |
| failed-row review | V | V | V | V | V | V | V | V | |
| provider preview: short link | V | V | V | V | V | V | V | V | |
| provider preview: split link | V | V | V | V | V | V | V | V | |
| pending dividend review | V (n/a close) | V (n/a close) | V (n/a close) | V (n/a close) | V (n/a close) | V (n/a close) | V (n/a close) | V (n/a close) | |

¹ correct buy: reconciliation confirm, light-desktop: axe color-contrast in a nested dialog re-measured from pixels: 5.39:1.
¹ correct buy: reconciliation confirm, dark-desktop: axe color-contrast in a nested dialog re-measured from pixels: 9.24:1.
¹ correct buy: reconciliation confirm, de-desktop: axe color-contrast in a nested dialog re-measured from pixels: 5.39:1.

## Maintainability notes

1. **One dialog, one contract.** After #327, every modal is `ModalDialog`.
   Its Escape guard reads `pending` or an `aria-busy="true"` form inside, so a
   new form in a dialog must keep `aria-busy={pending}` on its `<form>`. Nine
   investment forms lacked it and gained it in #327. The phone-width panels
   (#342) now share the contract through `modal-focus.ts` (`phoneModal`).
2. **`investment-correction-section.svelte` (~1,050 lines)** holds fifteen
   replacement dialogs that differ only in which form they render and the
   condition that offers them, plus the reversal reason/review modal and a
   34-way title/copy ternary. A table of `{kind, offered, title, form}` would
   remove most of it. The sweep reaches ten of the fifteen on the reference
   book.
3. **`routes/app/import/+page.svelte` (~2,100 lines)** owns upload, mapping,
   payee resolution, preview, provider link panels, discard, commit and the
   result step. In the sweep's eyes, its states are one `main` region. Each
   check passed, but a failure would point at the whole page.
4. **Dead branch.** `transaction-detail-panel.svelte` keeps a `restore`
   reason modal and its four messages. Nothing opens it: a deleted transaction
   is not served by the ordinary read, and restore lives in Settings → Trash
   (`trash/+page.svelte`). Remove it with its messages, or wire it, in a later
   cleanup.
5. **Harness: calendar day.** Specs date their seeds with the browser's
   `todayISO()`. Between local midnight and the owner zone's UTC offset, that
   can disagree with the server. With #339 open, a spec that opens an account
   "today" then fails. The sweep reads `owner_today` from the session instead.
6. **Harness: same-day statements are positional.** A statement covers its
   day only up to its last selected posting. A transaction created later the
   same day is outside it, so its restore needs no override. Fixtures that
   want a guarded restore create the subject first (as
   `trash-restore-reconciliation.spec.ts` does).
7. **The #278 sweep's blind spot.** It skipped sideways scroll at desktop,
   where `xl:` row layouts first apply. That is how #340 shipped. Fixed in
   this change.

## Disposition of the delivery scope

| Bullet | Disposition |
|---|---|
| Open each modal dialog (void/unvoid/delete/restore, reconciliation overrides, checkpoint void, trash restore, currency rate, investment corrections) | **Done.** Restore *from the detail panel* is unreachable (dead branch, note 4); trash restore is covered. The issue's "currency rate" dialog is the add-currency picker: currencies has no rate dialog after #327. |
| About 20 investment record forms, the lot panel and the transaction detail panel | **Done:** all 17 activities of the investment entry, including write-off; the lot, transaction and investment detail panels; and the transaction editor. |
| Import review: payee resolution, staged CSV/QIF preview, dividend/short/split link panels, failed-row review | **Done for CSV and the provider panels.** A QIF preview renders the same preview component as CSV and is not opened separately. |
| Editors: account, institution, category, recurring template, budget target row, price form | **Done.** The budget target row is inline on its route (covered by #278); the sweep adds its validation-error state. |
| axe light/dark at 390 px and desktop; sideways scroll; de and ru | **Done**, at both widths for de and ru. |
| Closes cleanly | **Done.** A nested review must close back into its parent dialog. |
| Dated report with matrix and maintainability notes; findings filed and linked here and on #260 | **Done** (this report; comments on #331 and #260). |

## Validation

```sh
./scripts/test-frontend.sh
pnpm --dir frontend exec vitest run src/lib/theme-tokens.test.ts src/lib/theme-contrast.test.ts
pnpm --dir e2e exec playwright test subview-sweep route-sweep accessibility dialog-focus \
  reference-data-management csv-import
```

Results on the final tree: `./scripts/test-frontend.sh` passes: svelte-check
reports 0 errors and 0 warnings, **691** frontend unit tests pass (including
the new warning-hover scan), and catalog parity is 66/66. `subview-sweep`
passes in all **8** configurations: **65** sub-views opened and checked
in each (520 cells), plus **5** marked not reachable. Its
matrix above is from that final run. In the same tree, `route-sweep` (now
with desktop overflow), `accessibility`, `reference-data-management` and
`csv-import` pass. `dialog-focus` failed once, at 00:15 CEST, on #339 (its
seed opened an account on the browser's day, still tomorrow in UTC). Its seed
is now dated a day back, and it passes 3/3.

Baseline evidence: #340 measured 118 px (en), 186 px (de) and 250 px (ru)
at 1280 px before the toolbar fix, and 0 px after, at every width from 800 to
1920 px in six locales. #341's scan lists `reconciliation-confirm.svelte:83`
on the baseline. Its hovered ratios are computed from the tokens. #342 and
#339 were each reproduced by a temporary probe that was not committed; the
issues quote the output.
