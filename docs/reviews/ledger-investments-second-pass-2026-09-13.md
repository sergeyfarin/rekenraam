# Ledger and investments: second release review — 2026-09-13

Financial code reviewed: `738a6512`, including fixes `4c76eca9`, `630a7e1e`,
`ca0a6679`, and `c6501af8`. Later commit `0bdfbf77` changes workflow guidance
and includes the review's temporary probes; it does not change the financial
implementation. Concurrent migration-fixture work is outside this review.

## Recommendation

**Keep the release on hold.** The original seven reproductions now pass, as do
the focused permanent tests for the four fixes. The fixes are useful and close
their original cases. Four additional probes nevertheless fail: two demonstrate
remaining integrity gaps in T-94/T-95, one demonstrates invalid investment API
roles bypassing subledger ownership, and one exposes a valuation regression
along the newly enabled fractional-sale path.

This is a review, not a production-code fix. The claims below distinguish
supported UI operations from invalid inputs that the API must reject. No
production database was opened or modified.

## Findings

### P1 / T-94 reopened — stale transaction facts can bypass the new guard

Locations: `backend/internal/app/transactions_write.go:195` and
`backend/internal/db/transactions_write.go:158`.

Checkpoint lookup is now inside the write transaction, but its candidate list
is still computed against an earlier transaction version. The repository reads
the latest version without either rejecting the stale version or recomputing
the financial difference. An edit originally classified as metadata-only can
therefore become a financial edit with an empty candidate list.

Confirmed interleaving:

1. Create an unreconciled 10 EUR posting.
2. Prepare a description-only edit against that version; the real service
   cleaner and `reconciliationCandidates` produce zero candidates.
3. Another edit changes the posting to 20 EUR; reconcile it to a 20 EUR statement.
4. Commit the first prepared edit through the real repository update method.

Observed: success, posting restored to 10 EUR, checkpoint still active. No
reconciliation override was supplied. This is **not limited to a same-day
reorder**, contrary to the earlier residual assessment. It needs overlapping
requests in one process, not two servers or a Go data race.

Required correction: derive the diff/candidates and inherited reconciliation
state from the transaction version read inside the write, or require that the
version used during preparation still matches and reject/retry otherwise.
Inspect lifecycle candidates against concurrent date/account/status edits as
well as sequence changes. Add named interleaving tests for metadata-only edits,
stale drafts/promotions, and void/delete/restore. Do not solve this by making
all ordinary metadata edits require a reconciliation override.

### P1 / T-95 reopened — eligible old lots can carry future pooled basis

Location: `backend/internal/db/investments.go:1432`.

Filtering `opened_on` correctly excludes future acquisitions. It does not make
`remaining_cost_basis_value` historical: a later average-cost sale has already
redistributed that mutable projection across the surviving lots.

Confirmed sequence (all purchases entered before either sale):

1. Buy 10 shares in January for 100 EUR.
2. Buy 10 shares in June for 300 EUR.
3. Sell 5 shares in July with average cost. Basis is 100 EUR, and January's
   remaining 5 shares now carry 100 EUR of pooled basis.
4. Enter a sale of 5 shares dated March with average cost.

Observed: the March sale succeeds and takes 100 EUR of basis from the January
lot. Only January's 10-EUR-per-share acquisition existed in March: the basis
for five shares would be 50 EUR. June's price still influences a March sale,
even though the selected lot passes the new date filter.

This is a backdated **disposal**, not just the already acknowledged backdated
acquisition case. The normal sell form permits entering the date, and the
service neither refuses it nor explains a chronological-entry limitation.

Required correction: reject unsupported out-of-order position events before
preview/commit, or implement explicit replay/correction semantics that preserve
previous elections and journal/subledger consistency. A minimal chronological
write guard can close this without implementing the deferred multi-projection
reporting feature. Define same-day order and apply the rule to write-offs and
imports too; a paragraph in a developer backlog is not runtime enforcement.

### P1 / T-98 — investment exemption does not validate account roles

Locations: `backend/internal/app/transactions_write.go:43`,
`backend/internal/app/investments.go:1009`, and `:2188`.

The private investment preparation path permits **all** subledger-account
postings in its spec. `validateTradeInput` verifies positive IDs, amounts and
scales, but not holding/cash roles or settlement currency. Consequently the
investment API can create the same journal/subledger mismatch the generic
posting fence prevents.

Confirmed request to `InvestmentService.Buy`: use the same security-holding
account for `holding_account_id` and `cash_account_id`, the security as both
commodities, and quantity/cash amount 10 at scale 0. The service accepts it.
The holding's +10 and -10 journal postings cancel to zero, while it creates an
open lot containing 10 shares. Both journal balancing and atomicity pass; the
financial result is nevertheless inconsistent.

The standard buy form excludes holding accounts from its cash picker, so this
is an **API validation hole**, not a claim that the normal picker offers this
combination. The API is the write boundary and must reject malformed role
combinations. The fixture creates a real `security_holding` account through the
existing test helper, without modifying or disabling schema constraints.

Required correction: validate holding, settlement, income and withholding roles
and commodity kinds in the domain plan, and narrow the exemption to postings
whose subledger consequences the command actually writes. Merely requiring
different account IDs leaves the same hole between two different holdings.
Test rejected role combinations through HTTP, plus valid buy/sell/reinvestment
and preview/import paths. Cash dividends should not acquire a blanket lot-write
exemption simply because they are investment-related.

### P2 / T-99 — fractional sale makes an existing valuation disappear

Location: `backend/internal/db/investments.go:3065`, reached after T-97 widens
`remaining_quantity_scale`.

Confirmed with ordinary service inputs and trade-implied prices:

1. Buy 1,000 shares for 100,000 EUR (100 EUR per share). Market value is present.
2. Sell 0.000001 shares for 0.0001 EUR, within the fixture's six-decimal
   commodity precision, retaining the same 100-EUR price.
3. Read unrealized gains again.

Observed: quantity is correctly 999.999999 and a current price is present, but
market value and unrealized gain both become null. The quantity coefficient
999999999 multiplied by the price coefficient 10000000000 is
9999999990000000000, which does not fit in `int64`. The read model silently
omits the values instead of normalizing their redundant decimal zeros or
representing the exact coefficient. The actual market value is only
99,999.9999 EUR and is readily representable at scale 4.

The underlying read-model limit predates T-97, but the previously rejected sale
now reaches it after a whole-share acquisition. This is not a claim that T-97
corrupts lots: it is a newly enabled workflow losing its valuation. The earlier
rationale for avoiding normalization at acquisition does not prevent this
failure when widening happens during disposal.

Required correction: normalize exact result coefficients where possible, or
use the lossless coefficient representation for the read model. Distinguish an
unrepresentable result from an absent price. Test the whole
buy → fractional sale → positions/unrealized-gains consumer chain; conservation
assertions over lots alone cannot catch this.

## Reproductions and test assessment

`fixtures/v0.1-ledger-investment-second-pass-probes.go.txt` contains the four
executed probes. They use isolated migrated databases and existing fixture
helpers. The reconciliation probe deliberately executes the same preparation
and commit phases with an intervening service update/reconciliation, rather
than relying on a timing-sensitive goroutine race.

To reproduce on the reviewed code, first ensure the destination is absent:

```sh
cp docs/reviews/fixtures/v0.1-ledger-investment-second-pass-probes.go.txt backend/internal/app/release_second_pass_probe_test.go
(cd backend && go test ./internal/app -run '^TestSecondPass' -count=1 -v)
rm backend/internal/app/release_second_pass_probe_test.go
```

Expected on the reviewed revision: four failing safety/validity assertions;
setup succeeds. Keep these outside the default suite until fixes land, then
promote them to permanent regression tests. For the historical-sale probe the
assertion selects rejection as the minimal safe implementation; a future
explicit, correct replay workflow would require a different assertion.

Validation:

- Baseline `./scripts/test-backend.sh`: passed formatting, vet, and race tests;
  several packages were cached. Run before temporary probes were installed.
- Original seven probes and focused permanent tests for T-94–T-97:
  `go test ./internal/app -run <focused expressions> -count=1` passed. This fresh
  run covers all original reproduction scenarios and the fixes' main tests.
- Four new probes: fresh `-count=1` run, all four confirmed failures above.
- `./scripts/test-frontend.sh`: zero Svelte errors/warnings; 25 files and
  381 Vitest tests passed.
- Coverage: initial sandboxed attempt failed because local `httptest` TCP
  listeners were denied (`socket: operation not permitted`), not because of
  missing Go tooling. The rerun on an isolated `738a6512` snapshot with local
  sockets enabled passed the full coverage suite and reported **79.0% of
  backend statements**. No tooling defect is asserted.
- Browser preflight was not rerun in this pass. The earlier successful 7/7 run
  remains prior evidence, not a fresh validation claim.

Coverage priorities are now concrete: stale **transaction** state as well as
stale checkpoint state; event ordering across mutable basis projections;
invalid domain-role combinations; and downstream read models after precision
changes. The passing baseline and original probes establish closure of those
specific cases, not completeness of the financial state machine.
