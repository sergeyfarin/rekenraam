package app

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/exact"
)

// T-104: a prior allocation introduces a six-place residual. Later commands
// must not accept a position that its reports cannot represent, and rejection
// must roll back the journal, projections, disposal evidence and audit together.
func TestFinancialLaterAcquisitionRejectsUnrepresentableBasisAtomically(t *testing.T) {
	for _, method := range []string{"fifo", "lifo", "average_cost", "specific_lot"} {
		for _, reinvest := range []bool{false, true} {
			name := method + "/buy"
			if reinvest {
				name = method + "/reinvest"
			}
			t.Run(name, func(t *testing.T) {
				f := newInvestmentsTestFixture(t)
				ctx := context.Background()
				bought := buyOn(t, f, "2026-01-01", 3, 1000)
				sale := sellInput(f, "2026-02-01", 1)
				sale.CashAmountValue = 500
				sale.CostBasisMethod = method
				if method == "specific_lot" {
					sale.LotAllocations = []InvestmentLotAllocationInput{{LotID: *bought.LotID, QuantityValue: exact.New(1)}}
				}
				_, err := f.investmentService.Sell(ctx, sale)
				require.NoError(t, err)
				before := snapshotFinancialState(t, f)
				if reinvest {
					_, err = f.investmentService.ReinvestedDividend(ctx, ReinvestedDividendInput{
						OwnerUserID: f.ownerUserID, TransactionDate: "2026-03-01", CommodityID: f.stockCommodityID,
						HoldingAccountID: f.holdingAccountID, IncomeAccountID: &f.incomeAccountID,
						QuantityValue: exact.New(1), AmountValue: 1_000_000_000_000_000, AmountScale: 2, CashCommodityID: f.eurCommodityID,
					})
				} else {
					buy := sellInput(f, "2026-03-01", 1)
					buy.CashAmountValue = 1_000_000_000_000_000
					_, err = f.investmentService.Buy(ctx, buy)
				}
				var validation ValidationError
				require.ErrorAs(t, err, &validation)
				require.Contains(t, err.Error(), "position basis exceeds supported exact range")
				require.Equal(t, before, snapshotFinancialState(t, f))
				positions, err := f.investmentService.Positions(ctx)
				require.NoError(t, err)
				require.Len(t, positions, 1)
				// Control: a smaller later acquisition remains usable, and the original
				// method can still sell. Rejection must not poison either workflow.
				buyOn(t, f, "2026-03-01", 1, 1000)
				sale.TransactionDate = "2026-04-01"
				_, err = f.investmentService.Sell(ctx, sale)
				require.NoError(t, err)
				_, err = f.investmentService.Positions(ctx)
				require.NoError(t, err)
			})
		}
	}
}

func TestFinancialBackdatedDisposalCannotOverflowFuturePosition(t *testing.T) {
	for _, method := range []string{"fifo", "lifo", "average_cost", "specific_lot"} {
		t.Run(method, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			bought := buyOn(t, f, "2026-01-01", 3, 1000)
			// Both lots initially fit at scale 2. The future one is ineligible for the
			// February sale but is included in the current position report.
			buyOn(t, f, "2026-03-01", 1, 1_000_000_000_000_000)
			_, err := f.investmentService.Positions(ctx)
			require.NoError(t, err)
			before := snapshotFinancialState(t, f)
			sale := sellInput(f, "2026-02-01", 1)
			sale.CashAmountValue = 500
			sale.CostBasisMethod = method
			if method == "specific_lot" {
				sale.LotAllocations = []InvestmentLotAllocationInput{{LotID: *bought.LotID, QuantityValue: exact.New(1)}}
			}
			_, err = f.investmentService.Sell(ctx, sale)
			var validation ValidationError
			require.ErrorAs(t, err, &validation)
			require.Contains(t, err.Error(), "position basis exceeds supported exact range")
			require.Equal(t, before, snapshotFinancialState(t, f))
			_, err = f.investmentService.Positions(ctx)
			require.NoError(t, err)
		})
	}
}

func TestFinancialLaterAcquisitionRangeBoundaryIsValueBased(t *testing.T) {
	// At scale 6, MaxInt64 is 9,223,372,036,854.775807. The old
	// residual is 6.666667: adding 9,223,372,036,848.10 fits, while
	// one more cent does not. Equivalent input spellings must agree.
	for _, scale := range []int{2, 4} {
		for _, over := range []bool{false, true} {
			t.Run(fmt.Sprintf("scale_%d/over_%t", scale, over), func(t *testing.T) {
				f := newInvestmentsTestFixture(t)
				ctx := context.Background()
				buyOn(t, f, "2026-01-01", 3, 1000)
				sale := sellInput(f, "2026-02-01", 1)
				sale.CashAmountValue = 500
				_, err := f.investmentService.Sell(ctx, sale)
				require.NoError(t, err)
				before := snapshotFinancialState(t, f)
				buy := sellInput(f, "2026-03-01", 1)
				buy.CashAmountValue = 922_337_203_684_810
				if over {
					buy.CashAmountValue++
				}
				if scale == 4 {
					buy.CashAmountValue *= 100
				}
				buy.CashAmountScale = scale
				_, err = f.investmentService.Buy(ctx, buy)
				if over {
					var validation ValidationError
					require.ErrorAs(t, err, &validation)
					require.Equal(t, before, snapshotFinancialState(t, f))
				} else {
					require.NoError(t, err)
				}
				_, err = f.investmentService.Positions(ctx)
				require.NoError(t, err)
			})
		}
	}
}
