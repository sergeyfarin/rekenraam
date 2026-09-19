# Ledger and investments: fourth release review — 2026-09-19

Reviewed clean HEAD `ad59d416`, focusing on financial fix `7c31e2ad` and the
current journal, investment and accounting read paths. Reproductions ran in an
isolated archive of that revision with migrated test databases; no household
records were used or changed.

## Resolution (added 2026-09-19, after the review)

All three findings are fixed.

- **T-100.** `accountInRole` records the version each role check read, the plan
  carries that collector, and journal preparation adds to the same one.
  `accountRuleDependencies.observe` now keeps the *first* sighting of an
  account rather than the last, so the write is checked against the version the
  role was decided against instead of against a later read of itself. Swept
  across sell, write-off, dividend and reinvestment.
- **T-101.** The gain is subtracted at whichever scale is deeper and carries
  its own `realized_gain_scale`, matching the shape `unrealized_gain_scale`
  already had; realized totals are one exactly-summed row per cost commodity.
  The UI formats the gain with that scale.
- **T-102.** "Touches selected cash" is asked per commodity group within an
  entry rather than once for the whole entry — strictly narrowing, and net
  movement is unchanged.

Named regression tests are in `investments_plan_binding_test.go`,
`investments_gain_precision_test.go` and `cashflow_commodity_scope_test.go`;
each fails with its own fix stubbed out. Two lines of the retained probe source
were updated for the two interfaces the fixes changed — see the note at the top
of the fixture — and all four probes pass. Full detail is in `docs/backlog.md`
T-100, T-101 and T-102.

## Result

**Hold the v0.1 financial correctness gate.** The two previous reproductions
are fixed, and all six named regression tests in
`transactions_prepared_write_race_test.go` pass in a fresh run. This pass found
one remaining P1 account-role interleaving (T-100), a P2 realized-gain
precision defect (T-101), and P2 non-cash legs counted in cashflow transfers
(T-102). The draft-promotion finding T-94 stays closed.

## P1 / T-100 — investment planning checks a version the write never remembers

Locations: `backend/internal/app/investments.go:1048–1053`,
`backend/internal/app/investments_roles.go:136`, and
`backend/internal/app/transactions_write.go:73`.

`buyPlan` validates the holding/settlement roles. Later,
`prepareInvestmentTransactionForWrite` reads the posting accounts again and
collects version dependencies from those later reads. Its subledger exemption
permits holding postings but does not require the planned holding leg still to
be a holding account. An account change between these phases therefore passes
the new database version guard: the guard receives the *new* version, although
the role check used the old version.

Deterministic reproduction, following the actual `buy` phases:

1. Build a buy plan for 10 shares against an unused security holding account.
2. Through `AccountService.UpdateAccount`, change that account to `other_asset`,
   effective on the trade date. No postings exist yet, so the change is valid.
3. Prepare the captured plan and call the same `CreateTransactionAndLot` sink
   used by `buy`, with the plan's original lot parameters.
4. The transaction and lot commit. A fresh `buyPlan` correctly rejects the
   same input because its holding account is no longer a holding account.
5. An ordinary balanced transaction removing 10 shares from the now-ordinary
   account is accepted. Its journal balance becomes **zero**, while its lot
   still contains **10 shares**.

No direct SQL mutation, invalid account fixture, or probabilistic goroutine
race is used. Splitting the command at its existing service phases schedules
an account edit in a window that overlapping requests can reach. The new
`TestPreparedInvestmentPostingIsRefusedAfterTheHoldingAccountChanges` tests a
later window (after journal preparation) and correctly passes; it cannot
cover this earlier one.

Required correction: bind all role-dependent planning decisions to the account
versions originally read, preserving those dependencies through journal
preparation and the database write, or validate the roles in the same database
transaction that commits the journal and lot. Audit the analogous reinvestment,
sell, write-off and dividend planning boundaries. Keep the already-working
post-preparation dependency guard and reciprocal structural-edit guard.

Probe: `TestFourthPassInvestmentRoleReadMustBindToWrite` fails because commit
returns no error. The probe also demonstrates the subsequent journal/lot
mismatch through an ordinary application-service write.

## P2 / T-101 — realized gain depends on the textual precision of proceeds

Location: `backend/internal/db/investments.go:2979–2984`.

Buy one share for 10.99 EUR, then sell it for 11 EUR (`CashAmountValue=11`,
`CashAmountScale=0`). Both commands are valid. `PreviewSell` returns a gain of
**0.01 EUR**. After saving, `ListRealizedGains` returns a gain of **1 EUR**,
because it truncates the negative 10.99 EUR disposed basis to -10 EUR before
adding proceeds. Entering the identical proceeds as `1100` at scale `2`
correctly returns 0.01 EUR. The lot records preserve the correct basis; the
error is in the report calculation.

The UI formats `realized_gain_value` using `proceeds_scale` in
`frontend/src/lib/investments/gains-report.svelte:204`, so this is a visible
incorrect gain, not merely a different internal representation. Rounding the
disposed basis before subtraction can also hide a loss. It is not the named
residual policy used for partial lot allocation: this reproduction disposes an
entire lot and has no allocation residual.

Required correction: subtract at a common exact scale, as the preview already
does. Expose a gain scale or consistently restate both proceeds and gain to a
shared lossless scale through the read model, API, totals and UI. Test both
scale directions and verify preview/report equality, including gains, losses,
full closures and mixed-scale multi-lot sales.

Probe: `TestFourthPassGainPreservesBasisPrecision/0` fails;
`TestFourthPassGainPreservesBasisPrecision/2` passes as the economically
identical control.

## P2 / T-102 — cashflow counts the non-cash commodity legs of a trade

Location: `backend/internal/app/cashflow.go:258–276`.

Buy 10 shares for 100 EUR through the normal investment service, then request
January cashflow with the default liquid-cash scope. That scope contains only
the cash account. The report returns:

- Transfer in: **10 shares**.
- Transfer out: **100 EUR and 10 shares**.
- Net movement: **-100 EUR**.

The EUR purchase is financing movement out of cash; neither security posting
moves anything into or out of the selected cash scope. `classifyCashflowEntry`
checks whether *any* posting in the journal entry touches cash and then
classifies every other posting, including the separately balanced security
pair between the holding and commodity-trading accounts. Net movement remains
correct, so the existing arithmetic identity cannot detect these inflated gross
flows. With EUR reporting enabled and the buy-implied price, the same probe
reports **100 EUR transfer-in and 200 EUR transfer-out**, instead of zero in
and 100 EUR out. This conflicts with the cash-scope semantics in
`docs/plans/reports-plan.md`, rules 2–4.

Required correction: classify counterparts within the commodity groups that
actually touch selected cash. Preserve the EUR trading-account counterpart,
which explains the real cash outflow, but exclude balanced commodity groups
that never touch selected cash. Test buy, sell, cross-currency transfers and
reporting-currency totals; do not just assert that net inflow equals net cash.

Probe: `TestFourthPassCashflowDoesNotCountNonCashSecurityLegs` uses the public
buy and cashflow services and fails on the spurious security commodity.

## Broader checks and coverage

The additional `TestFourthPassJournalAndLotsConserveAcrossTradeSequences`
passes under FIFO, LIFO, average cost and specific lot. Each method executes
three buys with mixed quantity/basis scales and nine fractional sales through
the public application commands, draining the position exactly. After **each
of the 48 commands**, it independently checks:

- Each journal entry balances separately for each commodity.
- The journal holding quantity equals the sum of remaining lot quantities.
- Remaining quantities and cost bases never become negative.
- Remaining total basis equals the sum of immutable acquisition/disposal
  basis events.

The current suite also covers method-specific allocation, oversell rollback,
future acquisitions, out-of-order disposals, fractional precision, write-offs,
dividends, transaction lifecycle restrictions, reconciliation boundaries,
report date/filter behavior, net-worth series versus point-in-time totals,
refund signs, running balances, overflow and exact arithmetic. Inspected the
current ledger aggregation, report selection, lot disposal and valuation paths;
apart from the three findings above, no additional confirmed defect emerged from that inspection or the sequence
probe. This is not a proof that every possible entry or interleaving is safe.

Validation:

- `COVERAGE=1 ./scripts/test-backend.sh`: passed (formatting, vet, all backend
  packages); merged statement coverage **79.1%**.
- Fresh targeted run of all six previous-fix regression tests: **6/6 passed**.
- Isolated review probes: three expected failing defect assertions; the
  equivalent-proceeds control and all four conservation sequences passed.
- `./scripts/test-backend.sh`: passed, including the full backend race suite.
- No frontend or browser suite rerun for this backend review; earlier browser
  results are not presented as fresh evidence.

Selected current function coverage illustrates why line coverage alone is not
an accounting oracle:

| Function | Statement coverage |
| --- | ---: |
| `validateBalanced` | 100.0% |
| `validateTradeRoles` | 100.0% |
| `requireAccountRuleDependenciesTx` | 81.2% |
| `disposeFIFOOrLIFOTx` | 87.8% |
| `disposeAverageCostTx` | 86.4% |
| DB `ListRealizedGains` | 87.4% |
| `aggregateSpending` | 96.7% |
| `NetWorthSeries` | 94.9% |

The missing assertions are cross-phase consistency and invariance under
financially equivalent decimal representations, plus cash-scope membership
for every classified commodity. Add them to the normal suite
when fixing these findings. Retain the journal-versus-lot sequence check as a
regression invariant as well.

## Reproducing the probes

The source is retained as
`docs/reviews/fixtures/v0.1-ledger-investment-fourth-pass-probes.go.txt`, outside
the compiled suite because three assertions intentionally expose unresolved
bugs. In an isolated checkout of the reviewed revision, copy it to
`backend/internal/app/fourth_pass_review_test.go`, then run:

```sh
cd backend
go test ./internal/app -run TestFourthPass -count=1 -v
```

The helper functions come from existing application tests. No additional
packages, credentials, network providers or household database are needed.
