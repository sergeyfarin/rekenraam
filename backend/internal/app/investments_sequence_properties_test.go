package app

import (
	"context"
	"fmt"
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

type financialSnapshot struct {
	Lots     []InvestmentLot
	Postings []db.LedgerPostingRecord
	Counts   map[string]int
}

func snapshotFinancialState(t *testing.T, f *investmentsTestFixture) financialSnapshot {
	t.Helper()
	ctx := context.Background()
	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	postings, err := f.transactionService.repository.LedgerPostingsThrough(ctx, BookID, "2026-12-31", "posted")
	require.NoError(t, err)
	counts := map[string]int{}
	// Static identifiers only. Observe audits and child records as well as the
	// returned projections, so rollback cannot quietly leave orphan evidence.
	for _, table := range []string{"transactions", "transaction_versions", "journal_entries", "posting_versions", "investment_lots", "investment_lot_events", "investment_disposal_decisions", "investment_disposal_allocations", "audit_events"} {
		var n int
		require.NoError(t, f.database.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n))
		counts[table] = n
	}
	return financialSnapshot{lots, postings, counts}
}

func TestFinancialTradeSequencesPreserveJournalLotsBasisAndGains(t *testing.T) {
	for _, method := range []string{"fifo", "lifo", "average_cost", "specific_lot"} {
		for _, seed := range []uint64{7, 19, 43} {
			t.Run(fmt.Sprintf("%s/seed_%d", method, seed), func(t *testing.T) {
				f := newInvestmentsTestFixture(t)
				ctx := context.Background()
				rng := rand.New(rand.NewPCG(seed, 11))
				acquired, proceeds := new(big.Rat), new(big.Rat)
				remaining := int64(0)
				originals := map[int64]*big.Rat{}
				check := func() {
					t.Helper()
					state := snapshotFinancialState(t, f)
					journalQty, cash, lotQty, remainingBasis, disposedBasis, gainTotal := new(big.Rat), new(big.Rat), new(big.Rat), new(big.Rat), new(big.Rat), new(big.Rat)
					entryTotals := map[[2]int64]*big.Rat{}
					for _, p := range state.Postings {
						v := financialRat(p.QuantityValue.String(), p.QuantityScale)
						key := [2]int64{p.JournalEntryID, p.CommodityID}
						if entryTotals[key] == nil {
							entryTotals[key] = new(big.Rat)
						}
						entryTotals[key].Add(entryTotals[key], v)
						if p.AccountID == f.holdingAccountID {
							journalQty.Add(journalQty, v)
						}
						if p.AccountID == f.cashAccountID {
							cash.Add(cash, v)
						}
					}
					for key, total := range entryTotals {
						require.Zero(t, total.Sign(), "unbalanced entry/commodity %v", key)
					}
					wantQty := big.NewRat(remaining, 100)
					require.Zero(t, journalQty.Cmp(wantQty), "journal versus independently tracked shares")
					require.Zero(t, cash.Cmp(new(big.Rat).Sub(proceeds, acquired)), "cash versus entered buy/sell amounts")
					for _, lot := range state.Lots {
						qty := financialRat(lot.RemainingQuantityValue.String(), lot.RemainingQuantityScale)
						basis := financialRat(fmt.Sprint(lot.RemainingCostBasisValue), lot.RemainingCostBasisScale)
						require.GreaterOrEqual(t, qty.Sign(), 0)
						require.GreaterOrEqual(t, basis.Sign(), 0)
						lotQty.Add(lotQty, qty)
						remainingBasis.Add(remainingBasis, basis)
						require.Zero(t, financialRat(fmt.Sprint(lot.CostBasisValue), lot.CostBasisScale).Cmp(originals[lot.ID]), "immutable acquisition basis")
						if qty.Sign() == 0 {
							require.Equal(t, "closed", lot.Status)
							require.Zero(t, basis.Sign())
						} else {
							require.Equal(t, "open", lot.Status)
						}
					}
					require.Zero(t, lotQty.Cmp(wantQty), "lots versus independently tracked shares")
					gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
					require.NoError(t, err)
					for _, g := range gains {
						basis := financialRat(fmt.Sprint(g.DisposedBasisValue), g.DisposedBasisScale)
						got := financialRat(fmt.Sprint(g.RealizedGainValue), g.RealizedGainScale)
						expected := new(big.Rat).Add(financialRat(fmt.Sprint(g.ProceedsValue), g.ProceedsScale), basis)
						require.Zero(t, got.Cmp(expected), "each realized gain = exact proceeds minus basis")
						disposedBasis.Sub(disposedBasis, basis)
						gainTotal.Add(gainTotal, got)
					}
					require.Zero(t, new(big.Rat).Add(remainingBasis, disposedBasis).Cmp(acquired), "acquired basis must be conserved after every command")
					// This cross-model equation catches report-only errors that balanced
					// postings and internally consistent lots cannot see.
					wantGain := new(big.Rat).Add(new(big.Rat).Sub(proceeds, acquired), remainingBasis)
					require.Zero(t, gainTotal.Cmp(wantGain), "realized gain + remaining investment must explain net cash")
				}
				for i, b := range []struct {
					q    int64
					qs   int
					cost int64
					cs   int
				}{{1, 0, 1099, 2}, {25, 1, 20001, 3}, {175, 2, 13, 0}, {750, 3, 3007, 2}} {
					input := sellInput(f, fmt.Sprintf("2026-01-%02d", i+1), b.q)
					input.QuantityScale = b.qs
					input.CashAmountValue = b.cost
					input.CashAmountScale = b.cs
					result, err := f.investmentService.Buy(ctx, input)
					require.NoError(t, err)
					require.NotNil(t, result.LotID)
					cost := financialRat(fmt.Sprint(b.cost), b.cs)
					acquired.Add(acquired, cost)
					originals[*result.LotID] = cost
					qty := new(big.Rat).Mul(financialRat(fmt.Sprint(b.q), b.qs), big.NewRat(100, 1))
					require.True(t, qty.IsInt())
					require.True(t, qty.Num().IsInt64())
					remaining += qty.Num().Int64()
					check()
				}
				for step := 0; remaining > 0; step++ {
					take := remaining
					if step < 7 {
						take = 1 + rng.Int64N(remaining)
					}
					input := sellInput(f, fmt.Sprintf("2026-02-%02d", step+1), take)
					input.QuantityScale = 2
					input.CostBasisMethod = method
					input.CashAmountValue = 1 + rng.Int64N(10000)
					input.CashAmountScale = step % 4
					if method == "specific_lot" {
						lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
						require.NoError(t, err)
						rng.Shuffle(len(lots), func(i, j int) { lots[i], lots[j] = lots[j], lots[i] })
						left := take
						for _, lot := range lots {
							qty := new(big.Rat).Mul(financialRat(lot.RemainingQuantityValue.String(), lot.RemainingQuantityScale), big.NewRat(100, 1))
							require.True(t, qty.IsInt())
							require.True(t, qty.Num().IsInt64())
							n := min(left, qty.Num().Int64())
							if n > 0 {
								input.LotAllocations = append(input.LotAllocations, InvestmentLotAllocationInput{LotID: lot.ID, QuantityValue: exact.New(n), QuantityScale: 2})
								left -= n
							}
						}
						require.Zero(t, left)
					}
					before := snapshotFinancialState(t, f)
					preview, err := f.investmentService.PreviewSell(ctx, input)
					require.NoError(t, err)
					require.Equal(t, before, snapshotFinancialState(t, f), "preview must not change journal, lots, allocations or audit")
					// Try an oversell through the actual journal+lot command first. It may
					// fail after a journal has been inserted, so inspect rollback, not just err.
					invalid := input
					invalid.QuantityValue = exact.New(remaining + 1)
					if method == "specific_lot" {
						invalid.LotAllocations = []InvestmentLotAllocationInput{{LotID: input.LotAllocations[0].LotID, QuantityValue: invalid.QuantityValue, QuantityScale: 2}}
					}
					_, err = f.investmentService.Sell(ctx, invalid)
					require.Error(t, err)
					require.Equal(t, before, snapshotFinancialState(t, f), "rejected sale must be atomic including audit and disposal evidence")
					result, err := f.investmentService.Sell(ctx, input)
					require.NoError(t, err)
					require.Equal(t, preview.Allocations, result.Allocations, "preview must predict the actual lot allocations")
					remaining -= take
					proceeds.Add(proceeds, financialRat(fmt.Sprint(input.CashAmountValue), input.CashAmountScale))
					check()
				}
				// Reading either report repeatedly is a read, not an operational basis rewrite.
				before := snapshotFinancialState(t, f)
				for i := 0; i < 2; i++ {
					_, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
					require.NoError(t, err)
					_, err = f.investmentService.ListUnrealizedGains(ctx)
					require.NoError(t, err)
				}
				require.Equal(t, before, snapshotFinancialState(t, f))
			})
		}
	}
}

// Trailing zeros change representation, never the economic result of closing
// a position. Cross both scale directions, all methods and gain/loss/zero.
func TestFinancialClosedPositionGainIgnoresDecimalRepresentation(t *testing.T) {
	for _, method := range []string{"fifo", "lifo", "average_cost", "specific_lot"} {
		for variant := 0; variant < 4; variant++ {
			for _, delta := range []int64{-1, 0, 1} {
				t.Run(fmt.Sprintf("%s/representation_%d/gain_cents_%d", method, variant, delta), func(t *testing.T) {
					f := newInvestmentsTestFixture(t)
					ctx := context.Background()
					var allocations []InvestmentLotAllocationInput
					for i, cost := range []int64{1100 - delta, 2000} {
						input := sellInput(f, fmt.Sprintf("2026-01-%02d", i+1), 1)
						input.CashAmountValue = cost
						input.CashAmountScale = 2
						if variant%2 == 1 {
							input.QuantityValue = exact.New(100)
							input.QuantityScale = 2
							input.CashAmountValue *= 100
							input.CashAmountScale = 4
						}
						bought, err := f.investmentService.Buy(ctx, input)
						require.NoError(t, err)
						allocations = append(allocations, InvestmentLotAllocationInput{LotID: *bought.LotID, QuantityValue: exact.New(1)})
					}
					sale := sellInput(f, "2026-02-01", 2)
					sale.CostBasisMethod = method
					sale.CashAmountValue = 31
					sale.CashAmountScale = 0
					if variant >= 2 {
						sale.QuantityValue = exact.New(200)
						sale.QuantityScale = 2
						sale.CashAmountValue = 310000
						sale.CashAmountScale = 4
						for i := range allocations {
							allocations[i].QuantityValue = exact.New(100)
							allocations[i].QuantityScale = 2
						}
					}
					if method == "specific_lot" {
						sale.LotAllocations = allocations
					}
					preview, err := f.investmentService.PreviewSell(ctx, sale)
					require.NoError(t, err)
					_, err = f.investmentService.Sell(ctx, sale)
					require.NoError(t, err)
					gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
					require.NoError(t, err)
					require.Len(t, gains, 1)
					expected := big.NewRat(delta, 100)
					require.Zero(t, financialRat(fmt.Sprint(preview.RealizedGain), preview.RealizedGainScale).Cmp(expected))
					require.Zero(t, financialRat(fmt.Sprint(gains[0].RealizedGainValue), gains[0].RealizedGainScale).Cmp(expected))
				})
			}
		}
	}
}

// A method that conserves basis can still allocate the wrong basis to today's
// sale. These hand-calculated partial disposals distinguish all four policies.
// Acquisition: 3 shares for 1.01 EUR, then 2 for 2.03 EUR. Sell 2 for 2 EUR;
// whatever the split cannot divide evenly remains in the position until the
// final close.
//
// The expected bases are stated at the allocation scale the policy fixes —
// the cost commodity's own maximum, six places for this fixture's EUR (T-103).
// They are not stated to two places because the split is not taken to two
// places; before that policy existed these same figures came out as 0.67,
// 2.03, 1.21 and 1.34, and would have come out differently again had the
// purchases been typed with a different number of decimals.
//
//   - fifo:         2 of lot A's 3 shares, 1.01 × 2/3 truncated
//   - lifo:         all of lot B, exactly its 2.03
//   - average_cost: the 3.04 pool over 5 shares, × 2
//   - specific_lot: one share from each, 1.01 × 1/3 plus 2.03 × 1/2
func TestFinancialPartialDisposalsUseChosenMethodAndRetainResidual(t *testing.T) {
	for _, tc := range []struct {
		method     string
		basisValue int64
		basisScale int
	}{
		{method: "fifo", basisValue: 673333, basisScale: 6},
		{method: "lifo", basisValue: 2030000, basisScale: 6},
		{method: "average_cost", basisValue: 1216000, basisScale: 6},
		{method: "specific_lot", basisValue: 1351666, basisScale: 6},
	} {
		t.Run(tc.method, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			a := buyOn(t, f, "2026-01-01", 3, 101)
			b := buyOn(t, f, "2026-01-02", 2, 203)
			sale := sellInput(f, "2026-02-01", 20)
			sale.QuantityScale = 1
			sale.CashAmountValue = 2
			sale.CashAmountScale = 0
			sale.CostBasisMethod = tc.method
			if tc.method == "specific_lot" {
				sale.LotAllocations = []InvestmentLotAllocationInput{{LotID: *a.LotID, QuantityValue: exact.New(10), QuantityScale: 1}, {LotID: *b.LotID, QuantityValue: exact.New(10), QuantityScale: 1}}
			}
			preview, err := f.investmentService.PreviewSell(ctx, sale)
			require.NoError(t, err)
			_, err = f.investmentService.Sell(ctx, sale)
			require.NoError(t, err)
			gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
			require.NoError(t, err)
			require.Len(t, gains, 1)
			wantBasis := financialRat(fmt.Sprint(tc.basisValue), tc.basisScale)
			require.Zero(t, financialRat(fmt.Sprint(gains[0].DisposedBasisValue), gains[0].DisposedBasisScale).Cmp(new(big.Rat).Neg(wantBasis)))
			wantGain := new(big.Rat).Sub(big.NewRat(200, 100), wantBasis)
			require.Zero(t, financialRat(fmt.Sprint(gains[0].RealizedGainValue), gains[0].RealizedGainScale).Cmp(wantGain))
			require.Zero(t, financialRat(fmt.Sprint(preview.RealizedGain), preview.RealizedGainScale).Cmp(wantGain))
			// Close everything: no residual basis may disappear, whatever the earlier
			// method and truncation did to its distribution across surviving lots.
			closeSale := sellInput(f, "2026-03-01", 3)
			closeSale.CashAmountValue = 3
			closeSale.CashAmountScale = 2
			closeSale.CostBasisMethod = tc.method
			if tc.method == "specific_lot" {
				lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
				require.NoError(t, err)
				for _, lot := range lots {
					if lot.RemainingQuantityValue.Sign() > 0 {
						qty := financialRat(lot.RemainingQuantityValue.String(), lot.RemainingQuantityScale)
						require.True(t, qty.IsInt())
						require.True(t, qty.Num().IsInt64())
						closeSale.LotAllocations = append(closeSale.LotAllocations, InvestmentLotAllocationInput{LotID: lot.ID, QuantityValue: exact.New(qty.Num().Int64())})
					}
				}
			}
			_, err = f.investmentService.Sell(ctx, closeSale)
			require.NoError(t, err)
			gains, err = f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
			require.NoError(t, err)
			require.Len(t, gains, 2)
			total := new(big.Rat)
			for _, g := range gains {
				total.Add(total, financialRat(fmt.Sprint(g.RealizedGainValue), g.RealizedGainScale))
			}
			require.Zero(t, total.Cmp(big.NewRat(-101, 100)), "2.03 of total proceeds less 3.04 acquisition cost")
			lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
			require.NoError(t, err)
			for _, lot := range lots {
				require.Equal(t, "closed", lot.Status)
				require.Zero(t, lot.RemainingQuantityValue.Sign())
				require.Zero(t, lot.RemainingCostBasisValue)
			}
		})
	}
}
