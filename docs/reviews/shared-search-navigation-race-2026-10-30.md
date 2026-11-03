# Shared-search navigation race (#472)

The failure at `25a18fb1` in [CI run 41044450297](https://github.com/sergeyfarin/rekenraam/actions/runs/41044450297)
is separate from #468's duplicate-result selector. Search was reopened and
filled on Spending, then its combobox became closed and empty. Catalog reads
returned 200 in under 10 ms; the failed option was uniquely suffixed. The
original light trace cannot identify the exact callback that cleared it.

## Reproduced mechanism

Spending renders its default period and repairs a missing URL range through a
same-page `goto` with `replaceState: true` and `reset: false`. Shared search
previously reset its query, scope and open state in **every** `afterNavigate`
callback, including this repair. Its result-selection handler also closed
search after awaiting navigation. Either completion could act on an interaction
that belonged to the destination rather than the completed action.

`shared-search.spec.ts` now controls input at the real history replacement for
Spending's range repair. It invokes the existing keyboard shortcut, lets the
phone input mount in the rendering microtask, dispatches the input event and
records that the query was entered. It then waits for the normalized URL,
requests, fonts and rendered frames before checking the query and unique result.
The test uses the actual report effect, router, search component and catalog;
there is no production test hook, arbitrary sleep or copied navigation logic.

Both final tests fail against the saved **unfixed** production binary:

- At 1280px the query becomes empty and the combobox is closed.
- At 390px the search dialog disappears, so its combobox is absent.

The checkpoint confirms input was entered before either failure. An earlier
probe at the first history push ran before SvelteKit's normal arrival-focus
handling; that is not destination readiness and is not the retained regression.
The retained checkpoint targets same-page range repair after Spending renders.
This establishes the reset mechanism without claiming that the original light
CI trace identifies a particular callback invocation.

## Fix and retained behavior

Search resets in `beforeNavigate` only when leaving for another pathname.
Query and fragment changes on the current page preserve its independent search.
Every selection releases the search focus scope before its destination or
command runs; there is no post-navigation close that can dismiss the next search.

The regression chooses the retained instrument result, requires its actual
instrument URL, checks search is dismissed, and reopens it with an empty query.
The existing keyboard/scoped/Dutch matching journey now seeds two reference
books explicitly, keeps its unique suffix, and waits for Spending's normalized
range rather than asserting its transient bare URL. Phone focus trapping,
both-theme axe scans, commands, failure recovery and cursor consumption remain.
No report range, ledger, API, timeout or retry policy changes.

The first 60-case repetition passed every controlled race case but exposed five
late failures of the old total-request cap (`6` reads against `< 6`). A growing
book legitimately requires additional 500-item catalog pages. The journey now
waits for complete coverage at each opening, requires the composed catalog
endpoint, an initial cursor-free request and no repeated cursor within that
opening. It retains the protection against repeated/per-result reads while
allowing pagination. The separate final-cursor consumer test remains selected.

## Validation

- `pnpm build`: production frontend, embedded-assets test and Go binary pass.
- `./scripts/test-frontend.sh`: zero Svelte errors/warnings; 832 unit tests pass.
- Final controlled negative run: both desktop/phone cases fail on the unfixed
  binary. Binary restoration is guarded by a shell exit trap.
- Complete shared-search spec: all nine cases pass on the fixed binary.
- Both controlled cases and the populated-book keyboard/scoped/Dutch journey,
  20 repetitions each at three workers: **60/60 pass**, including the later
  repetitions on the growing shared worker books.

The documented `--trace=on` override was also found to retain the configured
light-trace DOM/network exclusions. That independent diagnostics/documentation
gap is [#473](https://github.com/sergeyfarin/rekenraam/issues/473), not part of
the application fix or an unmet reproduction requirement: the regression's
checkpoint and actual consumer assertions establish its ordering directly.

Repeated populated-book and application-smoke results, the pushed commit and
hosted acceptance are recorded on [#472](https://github.com/sergeyfarin/rekenraam/issues/472).
This local mechanism record does not stand in for hosted CI acceptance.
