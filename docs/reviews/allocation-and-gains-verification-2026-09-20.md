# Allocation and gains verification — 2026-09-20

Reviewed `f45542eb`, with additional tests and the requested gains-summary
currency display policy. This is a focused verification, not certification of
all possible ledger histories or tax/broker accounting policies.

## T-103 calculation check

The revised partial-disposal oracle is correct. For lots of three shares
costing 1.01 and two shares costing 2.03, disposing two shares allocates:

| Method | Independent calculation at scale 6 | Basis |
|---|---|---|
| FIFO | floor(1.01 × 2 / 3) | 0.673333 |
| LIFO | whole second lot | 2.030000 |
| Average cost | 3.04 × 2 / 5 | 1.216000 |
| Specific lot | floor(1.01 / 3) + 2.03 / 2 | 1.351666 |

These are allocated basis amounts, not gains. Remaining basis stays in the
position. The existing closure oracle still expects total gain −1.01.
The targeted partial-disposal, method/conservation, allocation-policy and
trade-sequence tests pass. The original equivalent-input representation
regression passes under all four methods.

The maximum currency scale is a defensible allocation policy, but not the only
mathematically possible one. Preserving an already deeper basis is necessary;
using the ceiling for *new allocations* is a policy choice. Display precision
is independent. This review does not independently reproduce every fault
injection reported in the other agent's response.

## Range failure found and fixed: T-104 (P2)

At `f45542eb`, the backoff protected a single command only if some scale at or
above the deepest existing basis scale fit. It returned that deepest scale
without checking fit when none did. A previous disposal can introduce a genuinely six-place
residual; a later accepted acquisition can then exceed that scale's int64 range.

Using the real fixture's EUR maximum scale of 6, without modifying metadata:

1. Buy three shares for EUR 10.00 on January 1.
2. Sell one for EUR 5.00 on February 1: remaining basis 6.666667.
3. Buy one for EUR 10,000,000,000,000.00 on March 1 (accepted).
4. Read positions: `investment position cost basis: value exceeds int64 range`.
5. Sell one on April 1: LIFO fails restating lot 2, average cost fails aligning
   lot 2; FIFO can still dispose from the older small lot.

Reproduced in an isolated checkout of `f45542eb`, all three method cases failing
the positions assertion. No silent corruption was observed. This is far beyond
family use but contradicted an unrestricted range-safety claim.

**Fixed here after the user's follow-up.** Acquisitions check the entire open
position's exact basis inside the journal/lot/audit transaction; an impossible
position returns a validation error and rolls everything back. Reinvestment
and imported acquisitions use the same repository write path. Disposals check
their resulting position including future-dated lots. This covers the reverse
history: a large future lot already exists when a backdated partial disposal
would widen the smaller older lot. The allocation selector also explicitly
rejects a fallback scale that does not fit. No historical data is rewritten or
residual basis truncated.

This deliberately defines an admission limit for the existing int64 projection
format. It does not make all 38-digit journal values valid investment basis,
expand report range, or repair already-invalid databases. Such a representation
migration would be a separate change.

`backend/internal/app/investments_basis_range_test.go` is in the active suite:
all four methods; buy/reinvestment rejection; exact pre/post journal, lot,
event, disposal and audit snapshots; reports still readable; smaller subsequent
trades still usable; future-lot interactions; and a hand-derived one-cent
accept/reject boundary with equivalent scale-2/scale-4 purchase spellings.
The HTTP regression requires validation status 400 and identical gains before
and after rejection. Removing the acquisition guard makes all eight associated
method/command cases fail. Removing the post-disposal guard makes all four
future-lot cases fail. Both mutations compiled and failed assertions in an
isolated checkout, not the working tree.

## Display changes and regressions

- Gains-summary money uses the cost currency's `standard_scale` and code.
  Round to nearest display unit, ties away from zero. Fractional shares and
  detailed lot values remain exact; no journal, lot or event values are changed.
- The response includes currency metadata; no additional frontend fetch per row
  or currency is needed. Totals remain backend-computed before display rounding.
- Realized table keys formerly collided for two sales on the same date in the
  same account/instrument. These stateless rows now render without a key.
  Unrealized keys now include cost currency as well.
- The central formatter asked Intl for parts of integer zero and therefore never
  obtained a decimal separator. Its test checked only English substrings. It now
  explicitly requests a fractional part; Dutch and German exact-output tests
  catch the previous period-for-comma error.

New unit cases cover positive/negative ties, carry, sub-unit negative zero,
zero/two/three currency decimals, widening, coefficients beyond JS safe integer,
and exact share display. The browser fixture covers repeated same-day sales,
multiple cost currencies in one holding, and two half-cent gains whose exact
one-cent total must not become two cents. The API regression checks currency
metadata alongside exact mixed-scale totals.

## Coverage interpretation

Double entry constrains postings, not the correctness of downstream selection,
classification or allocation. Equal spurious gross inflows/outflows preserve a
net identity. Correct final closure does not prove correct interim gains.
Independent numerical oracles, equivalent representations and intermediate
sequence assertions address those blind spots; line coverage alone cannot.

Coverage is not complete. T-104 exposed a concrete gap now covered by active
regressions. The existing investment JSON-number contract also remains a documented precision limitation
(see backlog G-02 discussion); formatting with BigInt cannot recover precision
already lost while decoding a numeric coefficient. This change does not migrate
that contract. Longer acquisition/disposal histories and magnitude changes after
prior allocations remain useful next targets for generated sequence tests.

## Validation

- `./scripts/test-backend.sh`: exit 0; formatting, vet and full race suite.
  Application package 382.859s, database package 353.173s, API 193.089s.
- Additional final range-boundary and HTTP rejection tests passed with `-race`.
- `COVERAGE=1 ./scripts/test-backend.sh`: exit 0, merged backend statement
  coverage 79.2%. This is not a percentage of financial scenarios proved.
- `./scripts/test-frontend.sh`: 0 errors/warnings; 394/394 unit tests pass.
- Production build plus targeted gains-display Playwright run: 3/3 pass,
  including two authentication prerequisites. The gains fixture verifies
  filtered updates remove stale rows as well as initial display.
- Two independent range-guard mutations fail their expected assertions. Restoring
  the old gains-table keys also makes the browser regression fail (zero rendered
  rows instead of five); the fixed source was restored after the check.
