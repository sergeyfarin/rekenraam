package app

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// T-103: intentionally exposes an unresolved allocation-policy defect.
// Keep this regression active: conservation and full-closure checks miss it.
// Economic equality only: this does not prescribe a new allocation precision.
// All three acquisitions cost exactly 10 EUR and must follow the same policy.
func TestFinancialPartialDisposalGainMustNotDependOnPurchaseTextPrecision(t *testing.T) {
	for _, method := range []string{"fifo", "lifo", "average_cost", "specific_lot"} {
		t.Run(method, func(t *testing.T) {
			var reference *big.Rat
			for _, cash := range []struct {
				v int64
				s int
			}{{10, 0}, {1000, 2}, {100000, 4}} {
				f := newInvestmentsTestFixture(t)
				ctx := context.Background()
				buy := sellInput(f, "2026-01-01", 3)
				buy.CashAmountValue = cash.v
				buy.CashAmountScale = cash.s
				bought, err := f.investmentService.Buy(ctx, buy)
				require.NoError(t, err)
				sale := sellInput(f, "2026-02-01", 1)
				sale.CashAmountValue = 500
				sale.CashAmountScale = 2
				sale.CostBasisMethod = method
				if method == "specific_lot" {
					sale.LotAllocations = []InvestmentLotAllocationInput{{LotID: *bought.LotID, QuantityValue: exact.New(1)}}
				}
				_, err = f.investmentService.Sell(ctx, sale)
				require.NoError(t, err)
				gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
				require.NoError(t, err)
				require.Len(t, gains, 1)
				gain, ok := new(big.Rat).SetString(fmt.Sprintf("%de-%d", gains[0].RealizedGainValue, gains[0].RealizedGainScale))
				require.True(t, ok)
				t.Logf("method=%s purchase scale=%d partial gain=%s", method, cash.s, gain.FloatString(4))
				if reference == nil {
					reference = gain
				} else {
					assert.Zero(t, gain.Cmp(reference), "the same 10 EUR acquisition must allocate the same basis under one fixed rounding policy")
				}
			}
		})
	}
}
