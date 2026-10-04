package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// T-126 #141: every existing replaying command discloses committed-disposal
// gain changes through its own rolled-back writer and requires that exact
// acknowledgement at commit. These tests call commands directly so missing
// and stale acknowledgements stay pinned per family.

func TestReinvestmentGainImpactRequiresAcknowledgement(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-02-01", 10, 20000)
	sale := sellInput(f, "2026-03-01", 5)
	sale.CashAmountValue = 15000
	_, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)

	// A backdated reinvestment opens a cheaper earlier lot: FIFO 50.00 → 140.00.
	input := ReinvestedDividendInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		IncomeAccountID: &f.incomeAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), AmountValue: 2000, AmountScale: 2}
	before := buyReplacementPreviewSnapshot(t, f.database)
	impact, err := f.investmentService.ReinvestedDividendReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	require.Len(t, impact.GainImpact.Changes, 1)
	change := impact.GainImpact.Changes[0]
	require.Equal(t, db.GainImpactRevised, change.Kind)
	requireScaled(t, 5000, 2, change.Before.Gain, "gain before reinvestment replay")
	requireScaled(t, 14000, 2, change.After.Gain, "gain after reinvestment replay")

	_, err = f.investmentService.ReinvestedDividend(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	_, err = f.investmentService.ReinvestedDividend(ctx, input)
	require.NoError(t, err)
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	assertMoneyValue(t, 14000, 2, gains[0].RealizedGainValue, gains[0].RealizedGainScale, "acknowledged reinvestment gain")
}

// Reversing the cheaper January buy changes a later sale under FIFO and
// average cost, but not under LIFO or an explicit February election, where
// the empty change set needs no acknowledgement.
func TestBuyReversalGainImpactUnderEveryBasisMethod(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		method                  string
		basisBefore, basisAfter int64
	}{
		{method: "fifo", basisBefore: 5000, basisAfter: 15000},
		{method: "lifo"},
		{method: "average_cost", basisBefore: 10000, basisAfter: 15000},
		{method: "specific_lot"},
	} {
		t.Run(test.method, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			f := newInvestmentsTestFixture(t)
			january := buyOn(t, f, "2026-01-01", 10, 10000)
			february := buyOn(t, f, "2026-02-01", 10, 30000)
			sale := sellInput(f, "2026-03-01", 5)
			sale.CashAmountValue = 25000
			sale.CostBasisMethod = test.method
			if test.method == "specific_lot" {
				sale.LotAllocations = []InvestmentLotAllocationInput{{LotID: *february.LotID, QuantityValue: exact.New(5)}}
			}
			_, err := f.investmentService.Sell(ctx, sale)
			require.NoError(t, err)

			input := ReverseInvestmentBuyInput{OwnerUserID: f.ownerUserID,
				TransactionID: january.Transaction.ID, Reason: "duplicate entry"}
			impact, err := f.investmentService.ReverseBuyReconciliationImpact(ctx, input)
			require.NoError(t, err)
			require.NotNil(t, impact.GainImpact, "buy reversal is opted into disclosure")
			if test.basisAfter == 0 {
				require.Empty(t, impact.GainImpact.Changes, "the sale's recorded method keeps the February lot")
				_, err = f.investmentService.ReverseBuy(ctx, input)
				require.NoError(t, err, "an empty change set needs no acknowledgement")
				return
			}
			require.Len(t, impact.GainImpact.Changes, 1)
			change := impact.GainImpact.Changes[0]
			require.Equal(t, test.method, change.Before.CostBasisMethod)
			requireScaled(t, test.basisBefore, 2, change.Before.DisposedBasis, "basis before reversal")
			requireScaled(t, test.basisAfter, 2, change.After.DisposedBasis, "basis after reversal")
			before := buyReplacementPreviewSnapshot(t, f.database)
			_, err = f.investmentService.ReverseBuy(ctx, input)
			require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
			require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
			input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
			_, err = f.investmentService.ReverseBuy(ctx, input)
			require.NoError(t, err)
		})
	}
}

// Reversing the first of three sales removes it and revises both later
// sales, which now consume the shares it returned.
func TestSaleReversalDisclosesRemovalAndEveryDependentSale(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-01-01", 10, 10000) // 10.00 per share
	buyOn(t, f, "2026-02-01", 10, 30000) // 30.00 per share
	first, err := f.investmentService.Sell(ctx, sellInput(f, "2026-03-01", 8))
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-04-01", 4))
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-05-01", 6))
	require.NoError(t, err)

	input := ReverseInvestmentSaleInput{OwnerUserID: f.ownerUserID,
		TransactionID: first.Transaction.ID, Reason: "entered twice"}
	before := buyReplacementPreviewSnapshot(t, f.database)
	impact, err := f.investmentService.ReverseSaleReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "the writer preview rolls back")
	require.Len(t, impact.GainImpact.Changes, 3)
	byDate := map[string]db.InvestmentGainChange{}
	for _, change := range impact.GainImpact.Changes {
		byDate[change.Before.DisposalDate] = change
	}
	require.Equal(t, db.GainImpactRemoved, byDate["2026-03-01"].Kind)
	requireScaled(t, 8000, 2, byDate["2026-03-01"].Before.DisposedBasis, "removed March basis")
	require.Equal(t, db.GainImpactRevised, byDate["2026-04-01"].Kind)
	requireScaled(t, 8000, 2, byDate["2026-04-01"].Before.DisposedBasis, "April basis before")
	requireScaled(t, 4000, 2, byDate["2026-04-01"].After.DisposedBasis, "April basis after")
	require.Equal(t, db.GainImpactRevised, byDate["2026-05-01"].Kind)
	requireScaled(t, 18000, 2, byDate["2026-05-01"].Before.DisposedBasis, "May basis before")
	requireScaled(t, 6000, 2, byDate["2026-05-01"].After.DisposedBasis, "May basis after")

	_, err = f.investmentService.ReverseSale(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	_, err = f.investmentService.ReverseSale(ctx, input)
	require.NoError(t, err)
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Len(t, gains, 2)
}

// A replacement with negative proceeds is disclosed with its signed loss, and
// the later sale that now consumes different shares is revised with it.
func TestSaleReplacementDisclosesNegativeProceedsAndDependentSale(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-01-01", 2, 2000) // 10.00 per share
	buyOn(t, f, "2026-02-01", 2, 6000) // 30.00 per share
	original, err := f.investmentService.Sell(ctx, sellInput(f, "2026-03-01", 2))
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-04-01", 2))
	require.NoError(t, err)

	input := ReplaceInvestmentSaleInput{OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID,
		Reason: "minimum commission exceeded gross", Replacement: negativeProceedsSale(f, "2026-03-01")}
	impact, err := f.investmentService.ReplaceSaleReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, impact.GainImpact.Changes, 2)
	replaced, later := impact.GainImpact.Changes[0], impact.GainImpact.Changes[1]
	require.Equal(t, db.GainImpactReplaced, replaced.Kind)
	requireScaled(t, -50, 2, replaced.After.Proceeds, "negative replacement proceeds")
	requireScaled(t, -1050, 2, replaced.After.Gain, "replacement loss")
	require.Equal(t, db.GainImpactRevised, later.Kind)
	requireScaled(t, 6000, 2, later.Before.DisposedBasis, "April basis before")
	requireScaled(t, 4000, 2, later.After.DisposedBasis, "April basis after")

	_, err = f.investmentService.ReplaceSale(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	_, err = f.investmentService.ReplaceSale(ctx, input)
	require.NoError(t, err)
}

// An acknowledgement taken before an intervening economic change is refused
// with nothing written; a fresh preview of the current set commits.
func TestBuyReplacementRejectsStaleGainAcknowledgement(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	target := buyOn(t, f, "2026-02-01", 10, 20000)
	sale := sellInput(f, "2026-03-01", 5)
	sale.CashAmountValue = 15000
	_, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)

	input := ReplaceInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: target.Transaction.ID,
		Reason: "broker corrected price", Replacement: InvestmentTradeInput{TransactionDate: "2026-02-01",
			HoldingAccountID: f.holdingAccountID, CommodityID: f.stockCommodityID,
			CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
			QuantityValue: exact.New(10), CashAmountValue: 10000, CashAmountScale: 2}}
	stale, err := f.investmentService.ReplaceBuyReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, stale.GainImpact.Changes, 1)
	requireScaled(t, 10000, 2, stale.GainImpact.Changes[0].Before.DisposedBasis, "previewed basis before")
	requireScaled(t, 5000, 2, stale.GainImpact.Changes[0].After.DisposedBasis, "previewed basis after")
	input.GainImpactAcknowledgement = stale.GainImpact.Acknowledgement

	acknowledgedBuy(t, f, backdatedBuy(f, "2026-01-01", 2, 200))
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.investmentService.ReplaceBuy(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementStale)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))

	input.GainImpactAcknowledgement = ""
	fresh, err := f.investmentService.ReplaceBuyReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.NotEqual(t, stale.GainImpact.Acknowledgement, fresh.GainImpact.Acknowledgement)
	input.GainImpactAcknowledgement = fresh.GainImpact.Acknowledgement
	_, err = f.investmentService.ReplaceBuy(ctx, input)
	require.NoError(t, err)
}

// An imported acquisition that would change a committed sale's gain is held
// pending — not skipped — until the current preview-commit set is
// acknowledged. A stale acknowledgement holds it again; the retry commits.
func TestImportedAcquisitionGainReviewHoldsRowUntilAcknowledged(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	fill := func(id, side, quantity, net, filledAt string) trading212OrderFill {
		return trading212OrderFill{FillType: "TRADE", FillID: id, OrderID: "order-" + id, Ticker: "AAPL_US_EQ",
			ISIN: "US0378331005", Side: side, Quantity: quantity, Price: "1.00", Currency: "EUR",
			FilledAt: filledAt, NetValue: net, NetValueCurrency: "EUR"}
	}
	commit := func(batchID int64, acknowledgements map[int64]string) CommitImportBatchResult {
		t.Helper()
		result, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{
			OwnerUserID: f.ownerUserID, BatchID: batchID, GainImpactAcknowledgements: acknowledgements})
		require.NoError(t, err)
		return result
	}
	february, _ := f.stageOrderFillRow(t, conn.ID, fill("feb-buy", "BUY", "10", "-200.00", "2026-02-01T10:00:00Z"))
	require.Equal(t, 1, commit(february, nil).CommittedCount)
	march, _ := f.stageOrderFillRow(t, conn.ID, fill("mar-sell", "SELL", "5", "150.00", "2026-03-01T10:00:00Z"))
	require.Equal(t, 1, commit(march, nil).CommittedCount)

	// A buy after the latest sale opens a lot without replay: nothing to review.
	april, _ := f.stageOrderFillRow(t, conn.ID, fill("apr-buy", "BUY", "1", "-1.00", "2026-04-01T10:00:00Z"))
	laterPreview, err := f.importService.PreviewCommit(ctx, PreviewCommitInput{OwnerUserID: f.ownerUserID, BatchID: april})
	require.NoError(t, err)
	require.Empty(t, laterPreview.GainImpacts)
	require.Equal(t, 1, commit(april, nil).CommittedCount)

	january, rowID := f.stageOrderFillRow(t, conn.ID, fill("jan-buy", "BUY", "10", "-20.00", "2026-01-01T10:00:00Z"))
	preview, err := f.importService.PreviewCommit(ctx, PreviewCommitInput{OwnerUserID: f.ownerUserID, BatchID: january})
	require.NoError(t, err)
	require.Len(t, preview.GainImpacts, 1)
	require.Equal(t, rowID, preview.GainImpacts[0].RowID)
	change := preview.GainImpacts[0].GainImpact.Changes[0]
	requireScaled(t, 5000, 2, change.Before.Gain, "imported sale gain before")
	requireScaled(t, 14000, 2, change.After.Gain, "imported sale gain after")

	held := commit(january, nil)
	require.Equal(t, []int64{rowID}, held.GainReviewRowIDs)
	require.Zero(t, held.CommittedCount)
	require.Equal(t, "partially_committed", held.Status)
	row, err := f.importRepo.ImportStagedRowByID(ctx, rowID)
	require.NoError(t, err)
	require.Equal(t, "pending", row.CommitStatus, "a held acquisition is never skipped")

	stale := commit(january, map[int64]string{rowID: "stale"})
	require.Equal(t, []int64{rowID}, stale.GainReviewRowIDs)

	preview, err = f.importService.PreviewCommit(ctx, PreviewCommitInput{OwnerUserID: f.ownerUserID, BatchID: january})
	require.NoError(t, err)
	require.Len(t, preview.GainImpacts, 1, "a partially committed batch can be previewed again")
	retried := commit(january, map[int64]string{rowID: preview.GainImpacts[0].GainImpact.Acknowledgement})
	require.Equal(t, 1, retried.CommittedCount)
	require.Empty(t, retried.GainReviewRowIDs)
	require.Equal(t, "committed", retried.Status)
	gains, err := f.investmentSvc.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Len(t, gains, 1)
	assertMoneyValue(t, 14000, 2, gains[0].RealizedGainValue, gains[0].RealizedGainScale, "acknowledged imported acquisition")
}

// A Trading 212 buy source correction changes a dependent imported sale's
// gain; the correction preview discloses it and commit requires it.
func TestCorrectTrading212BuyDisclosesDependentSaleGain(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	buy := trading212OrderFill{FillType: "TRADE", FillID: "src-buy", OrderID: "src-buy-order", Ticker: "AAPL_US_EQ",
		ISIN: "US0378331005", Side: "BUY", Quantity: "10", Price: "20.00", Currency: "EUR",
		FilledAt: "2026-02-01T10:00:00Z", NetValue: "-200.00", NetValueCurrency: "EUR"}
	original, _ := f.stageOrderFillRow(t, conn.ID, buy)
	_, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: original})
	require.NoError(t, err)
	sale, _ := f.stageOrderFillRow(t, conn.ID, trading212OrderFill{FillType: "TRADE", FillID: "src-sell",
		OrderID: "src-sell-order", Ticker: "AAPL_US_EQ", ISIN: "US0378331005", Side: "SELL", Quantity: "5",
		Price: "30.00", Currency: "EUR", FilledAt: "2026-03-01T10:00:00Z", NetValue: "150.00", NetValueCurrency: "EUR"})
	_, err = f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: sale})
	require.NoError(t, err)

	revised := buy
	revised.NetValue = "-100.00"
	revisedBatch, revisedRow := f.stageOrderFillRow(t, conn.ID, revised)
	input := CorrectTrading212BuyInput{OwnerUserID: f.ownerUserID, BatchID: revisedBatch, RowID: revisedRow,
		Reason: "broker revised net value"}
	impact, err := f.importService.Trading212BuyCorrectionReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, impact.GainImpact.Changes, 1)
	requireScaled(t, 10000, 2, impact.GainImpact.Changes[0].Before.DisposedBasis, "imported sale basis before")
	requireScaled(t, 5000, 2, impact.GainImpact.Changes[0].After.DisposedBasis, "imported sale basis after")

	_, err = f.importService.CorrectTrading212Buy(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	row, err := f.importRepo.ImportStagedRowByID(ctx, revisedRow)
	require.NoError(t, err)
	require.NotEqual(t, "committed", row.CommitStatus, "source acceptance rolled back with the refusal")
	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	_, err = f.importService.CorrectTrading212Buy(ctx, input)
	require.NoError(t, err)
	row, err = f.importRepo.ImportStagedRowByID(ctx, revisedRow)
	require.NoError(t, err)
	require.Equal(t, "committed", row.CommitStatus)
}

// A Trading 212 sale source revision replaces the imported sale's gain; the
// correction preview discloses it and commit requires that acknowledgement.
func TestCorrectTrading212SaleDisclosesReplacedGain(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	buy, _ := f.stageOrderFillRow(t, conn.ID, trading212OrderFill{FillType: "TRADE", FillID: "src-buy",
		OrderID: "src-buy-order", Ticker: "AAPL_US_EQ", ISIN: "US0378331005", Side: "BUY", Quantity: "10",
		Price: "20.00", Currency: "EUR", FilledAt: "2026-02-01T10:00:00Z", NetValue: "-200.00", NetValueCurrency: "EUR"})
	_, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: buy})
	require.NoError(t, err)
	sale := trading212OrderFill{FillType: "TRADE", FillID: "src-sell", OrderID: "src-sell-order", Ticker: "AAPL_US_EQ",
		ISIN: "US0378331005", Side: "SELL", Quantity: "5", Price: "30.00", Currency: "EUR",
		FilledAt: "2026-03-01T10:00:00Z", NetValue: "150.00", NetValueCurrency: "EUR"}
	saleBatch, _ := f.stageOrderFillRow(t, conn.ID, sale)
	_, err = f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: saleBatch})
	require.NoError(t, err)

	revised := sale
	revised.NetValue = "250.00"
	revisedBatch, revisedRow := f.stageOrderFillRow(t, conn.ID, revised)
	input := CorrectTrading212SaleInput{OwnerUserID: f.ownerUserID, BatchID: revisedBatch, RowID: revisedRow,
		Reason: "broker revised proceeds"}
	impact, err := f.importService.Trading212SaleCorrectionReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, impact.GainImpact.Changes, 1)
	change := impact.GainImpact.Changes[0]
	require.Equal(t, db.GainImpactReplaced, change.Kind)
	requireScaled(t, 5000, 2, change.Before.Gain, "imported sale gain before")
	requireScaled(t, 15000, 2, change.After.Gain, "imported sale gain after")

	_, err = f.importService.CorrectTrading212Sale(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	input.GainImpactAcknowledgement = "stale"
	_, err = f.importService.CorrectTrading212Sale(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementStale)
	row, err := f.importRepo.ImportStagedRowByID(ctx, revisedRow)
	require.NoError(t, err)
	require.NotEqual(t, "committed", row.CommitStatus, "source acceptance rolled back with the refusal")
	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	_, err = f.importService.CorrectTrading212Sale(ctx, input)
	require.NoError(t, err)
}
