package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// #173: named short sale and cover. A short lot records its exact opening
// proceeds; a cover consumes short lots only and realizes allocated opening
// proceeds plus its signed (negative) covering amount. Entry is in date order,
// and one holding is never long and short of an instrument over overlapping
// dates. The operation plan's *Short positions (#103)* section governs.

func shortInput(f *investmentsTestFixture, date string, quantity int64, cash int64) InvestmentTradeInput {
	return InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: date, CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(quantity), CashAmountValue: cash, CashAmountScale: 2,
		CashCommodityID: f.eurCommodityID,
	}
}

func shortSaleOn(t *testing.T, f *investmentsTestFixture, date string, quantity int64, cash int64) InvestmentTradeResult {
	t.Helper()
	result, err := f.investmentService.ShortSale(context.Background(), shortInput(f, date, quantity, cash))
	require.NoError(t, err)
	require.NotNil(t, result.LotID)
	return result
}

type shortPosting struct {
	accountID, commodityID int64
	value                  string
	scale                  int
}

func shortTransactionPostings(t *testing.T, f *investmentsTestFixture, transactionID int64) []shortPosting {
	t.Helper()
	rows, err := f.database.Query(`
		SELECT pv.account_id, pv.commodity_id, pv.quantity_value, pv.quantity_scale
		FROM current_transaction_versions tv
		JOIN journal_entries je ON je.transaction_version_id = tv.id
		JOIN posting_versions pv ON pv.journal_entry_id = je.id
		WHERE tv.transaction_id = ? ORDER BY pv.commodity_id, pv.account_id`, transactionID)
	require.NoError(t, err)
	defer rows.Close()
	var postings []shortPosting
	for rows.Next() {
		var posting shortPosting
		require.NoError(t, rows.Scan(&posting.accountID, &posting.commodityID, &posting.value, &posting.scale))
		postings = append(postings, posting)
	}
	require.NoError(t, rows.Err())
	return postings
}

func commodityTradingAccountIDForTest(t *testing.T, f *investmentsTestFixture) int64 {
	t.Helper()
	id, err := db.NewInvestmentRepository(f.database).CommodityTradingAccountID(context.Background(), BookID)
	require.NoError(t, err)
	return id
}

func shortSelfCheckPasses(t *testing.T, f *investmentsTestFixture) {
	t.Helper()
	run := mustRunInvestmentSelfCheck(t, f)
	for _, check := range []string{CheckInvestmentFoundation, CheckLotReconciliation, CheckCommodityPositionSign, CheckInvestmentReplay} {
		result := resultFor(t, run, check)
		assert.Equal(t, SelfCheckPassed, result.Status, "%s: %s", check, result.Summary)
	}
}

func TestShortSaleOpensShortLotWithExactOpeningProceeds(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	result := shortSaleOn(t, f, "2026-03-02", 10, 10000)
	trading := commodityTradingAccountIDForTest(t, f)

	assert.ElementsMatch(t, []shortPosting{
		{f.holdingAccountID, f.stockCommodityID, "-10", 0},
		{trading, f.stockCommodityID, "10", 0},
		{f.cashAccountID, f.eurCommodityID, "10000", 2},
		{trading, f.eurCommodityID, "-10000", 2},
	}, shortTransactionPostings(t, f, result.Transaction.ID), "the security leaves the holding; cash arrives")

	var side, quantity, basis, knowledge, kind string
	var basisScale int
	require.NoError(t, f.database.QueryRow(`
		SELECT l.position_side, l.quantity_value, l.cost_basis_value, l.cost_basis_scale, l.opening_basis_knowledge, o.operation_kind
		FROM investment_lots l JOIN investment_operations o ON o.id = l.operation_id WHERE l.id = ?`, *result.LotID).
		Scan(&side, &quantity, &basis, &basisScale, &knowledge, &kind))
	assert.Equal(t, []any{"short", "10", "10000", 2, "known", "short_sale"}, []any{side, quantity, basis, basisScale, knowledge, kind})

	// Reads carry the side (#174); see investment_short_reads_test.go.
	positions, err := f.investmentService.Positions(context.Background())
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assert.Equal(t, "short", positions[0].PositionSide)
	shortSelfCheckPasses(t, f)
}

func TestShortCoverRealizesOpeningProceedsLessCoveringCost(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	opening := shortSaleOn(t, f, "2026-03-02", 10, 10000)

	cover := shortInput(f, "2026-04-01", 10, 7000)
	before := buyReplacementPreviewSnapshot(t, f.database)
	preview, err := f.investmentService.PreviewShortCover(ctx, cover)
	require.NoError(t, err)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "preview writes nothing")
	assertMoneyValue(t, 3000, 2, preview.RealizedGain, preview.RealizedGainScale, "100.00 opened − 70.00 covered")
	require.Len(t, preview.Allocations, 1)
	assert.Equal(t, *opening.LotID, preview.Allocations[0].LotID)

	result, err := f.investmentService.ShortCover(ctx, cover)
	require.NoError(t, err)
	require.NotNil(t, result.DisposalDecision)
	decision := *result.DisposalDecision
	requireScaled(t, 10000, 2, exact.ScaledIntFromCoefficient(decision.DisposedBasisValue, decision.DisposedBasisScale), "allocated opening proceeds")
	assert.Equal(t, int64(-7000), decision.ProceedsValue, "signed covering amount")
	trading := commodityTradingAccountIDForTest(t, f)
	assert.ElementsMatch(t, []shortPosting{
		{f.holdingAccountID, f.stockCommodityID, "10", 0},
		{trading, f.stockCommodityID, "-10", 0},
		{f.cashAccountID, f.eurCommodityID, "-7000", 2},
		{trading, f.eurCommodityID, "7000", 2},
	}, shortTransactionPostings(t, f, result.Transaction.ID))

	var side, status, remaining string
	require.NoError(t, f.database.QueryRow(`SELECT d.position_side, l.status, l.remaining_quantity_value
		FROM investment_disposal_decisions d JOIN current_investment_lots l ON l.id = ?
		WHERE d.id = ?`, *opening.LotID, *decision.ID).Scan(&side, &status, &remaining))
	assert.Equal(t, []string{"short", "closed", "0"}, []string{side, status, remaining})

	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Len(t, gains, 1)
	assert.Equal(t, "short", gains[0].PositionSide)
	assertMoneyValue(t, 3000, 2, gains[0].RealizedGainValue, gains[0].RealizedGainScale, "report result")
	assertMoneyValue(t, 10000, 2, gains[0].DisposedBasisValue, gains[0].DisposedBasisScale, "opening proceeds")
	assertMoneyValue(t, -7000, 2, gains[0].ProceedsValue, gains[0].ProceedsScale, "covering cost")
	shortSelfCheckPasses(t, f)
}

func TestShortCoverResultIsExactAcrossChargesAndScales(t *testing.T) {
	t.Parallel()
	gross := func(value int64) *int64 { return &value }
	for _, test := range []struct {
		name    string
		opening func(f *investmentsTestFixture) InvestmentTradeInput
		cover   func(f *investmentsTestFixture, expenseAccountID int64) InvestmentTradeInput
		want    int64
		scale   int
	}{
		{
			name:    "net only at mixed scales",
			opening: func(f *investmentsTestFixture) InvestmentTradeInput { return shortInput(f, "2026-03-02", 4, 10000) },
			cover: func(f *investmentsTestFixture, expenseAccountID int64) InvestmentTradeInput {
				input := shortInput(f, "2026-04-01", 4, 70001)
				input.CashAmountScale = 3
				return input
			},
			// 100.00 − 70.001 = 29.999 at scale 3.
			want: 29999, scale: 3,
		},
		{
			name: "equivalent decimal spellings",
			opening: func(f *investmentsTestFixture) InvestmentTradeInput {
				input := shortInput(f, "2026-03-02", 4, 100)
				input.CashAmountScale = 0
				return input
			},
			cover: func(f *investmentsTestFixture, expenseAccountID int64) InvestmentTradeInput {
				input := shortInput(f, "2026-04-01", 4, 700000)
				input.CashAmountScale = 4
				return input
			},
			want: 300000, scale: 4,
		},
		{
			name: "clearing-included commissions on both sides",
			opening: func(f *investmentsTestFixture) InvestmentTradeInput {
				input := shortInput(f, "2026-03-02", 4, 0)
				input.GrossAmountValue, input.GrossAmountScale = gross(10000), 2
				input.NetSettlementValue, input.NetSettlementScale = gross(9900), 2
				input.Charges = []InvestmentTradeChargeInput{{Kind: "commission", AmountValue: -100, AmountScale: 2, CommodityID: f.eurCommodityID}}
				return input
			},
			cover: func(f *investmentsTestFixture, expenseAccountID int64) InvestmentTradeInput {
				input := shortInput(f, "2026-04-01", 4, 0)
				input.GrossAmountValue, input.GrossAmountScale = gross(-7000), 2
				input.NetSettlementValue, input.NetSettlementScale = gross(-7050), 2
				input.Charges = []InvestmentTradeChargeInput{{Kind: "commission", AmountValue: -50, AmountScale: 2, CommodityID: f.eurCommodityID}}
				return input
			},
			// Opening proceeds 99.00; covering cost 70.50; result 28.50.
			want: 2850, scale: 2,
		},
		{
			name:    "separately expensed charge never enters the result",
			opening: func(f *investmentsTestFixture) InvestmentTradeInput { return shortInput(f, "2026-03-02", 4, 10000) },
			cover: func(f *investmentsTestFixture, expenseAccountID int64) InvestmentTradeInput {
				input := shortInput(f, "2026-04-01", 4, 0)
				input.GrossAmountValue, input.GrossAmountScale = gross(-7000), 2
				input.NetSettlementValue, input.NetSettlementScale = gross(-7200), 2
				input.Charges = []InvestmentTradeChargeInput{{Kind: "other_fee", AmountValue: -200, AmountScale: 2,
					CommodityID: f.eurCommodityID, Treatment: "separately_expensed", ChargeAccountID: &expenseAccountID}}
				return input
			},
			want: 3000, scale: 2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			expense := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
			ctx := context.Background()
			_, err := f.investmentService.ShortSale(ctx, test.opening(f))
			require.NoError(t, err)
			preview, err := f.investmentService.PreviewShortCover(ctx, test.cover(f, expense))
			require.NoError(t, err)
			assertMoneyValue(t, test.want, test.scale, preview.RealizedGain, preview.RealizedGainScale, "preview")
			_, err = f.investmentService.ShortCover(ctx, test.cover(f, expense))
			require.NoError(t, err)
			gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
			require.NoError(t, err)
			require.Len(t, gains, 1)
			assertMoneyValue(t, test.want, test.scale, gains[0].RealizedGainValue, gains[0].RealizedGainScale, "committed")
			shortSelfCheckPasses(t, f)
		})
	}
}

func TestPartialCoversConserveOpeningProceedsUnderEveryMethod(t *testing.T) {
	t.Parallel()
	// Two short lots: 10 opened for 100.00 and 10 for 130.00, so 230.00 of
	// opening proceeds across 20 owed units. Two covers of 7 then 13 close
	// the position; whatever the method, their allocated opening proceeds
	// sum to exactly 230.00.
	for _, test := range []struct {
		method string
		first  int64 // first cover's allocated opening proceeds at scale 2
	}{
		{method: "fifo", first: 7000},
		{method: "lifo", first: 9100},
		// 230.00 / 20 × 7 = 80.50.
		{method: "average_cost", first: 8050},
		{method: "specific_lot", first: 9100},
	} {
		t.Run(test.method, func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			march := shortSaleOn(t, f, "2026-03-02", 10, 10000)
			may := shortSaleOn(t, f, "2026-05-04", 10, 13000)
			first := shortInput(f, "2026-06-01", 7, 7700)
			first.CostBasisMethod = test.method
			second := shortInput(f, "2026-07-01", 13, 12000)
			second.CostBasisMethod = test.method
			if test.method == "specific_lot" {
				first.LotAllocations = []InvestmentLotAllocationInput{{LotID: *may.LotID, QuantityValue: exact.New(7)}}
				second.LotAllocations = []InvestmentLotAllocationInput{
					{LotID: *march.LotID, QuantityValue: exact.New(10)}, {LotID: *may.LotID, QuantityValue: exact.New(3)}}
			}
			one, err := f.investmentService.ShortCover(ctx, first)
			require.NoError(t, err)
			two, err := f.investmentService.ShortCover(ctx, second)
			require.NoError(t, err)

			total := exact.NewScaledInt()
			for _, decision := range []*DisposalDecision{one.DisposalDecision, two.DisposalDecision} {
				total.AddCoefficient(decision.DisposedBasisValue, decision.DisposedBasisScale)
			}
			requireScaled(t, 23000, 2, total, "opening proceeds conserved across covers")
			requireScaled(t, test.first, 2, exact.ScaledIntFromCoefficient(one.DisposalDecision.DisposedBasisValue,
				one.DisposalDecision.DisposedBasisScale), "first cover's allocation")
			var open int
			require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM current_investment_lots WHERE status = 'open'`).Scan(&open))
			assert.Zero(t, open)
			var lock int
			require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_position_basis_state`).Scan(&lock))
			assert.Zero(t, lock, "closing the short position releases its method lock")
			shortSelfCheckPasses(t, f)
		})
	}
}

func TestShortCoverRefusesOverCoverAndOtherSideLots(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	shortSaleOn(t, f, "2026-03-02", 10, 10000)
	before := buyReplacementPreviewSnapshot(t, f.database)

	_, err := f.investmentService.ShortCover(ctx, shortInput(f, "2026-04-01", 11, 7000))
	require.ErrorIs(t, err, ErrInvestmentLotsInsufficient, "covering more than is owed")
	// An ordinary sale never consumes the short lot, and does not silently
	// deepen the short either: there is nothing long to sell.
	_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-04-01", 1))
	require.ErrorIs(t, err, ErrInvestmentLotsInsufficient)
	// A buy cannot cross zero by netting against the short.
	_, err = f.investmentService.Buy(ctx, shortInput(f, "2026-04-01", 1, 1000))
	require.ErrorIs(t, err, ErrInvestmentPositionSideConflict)

	// A specific-lot election naming a long lot in another holding refuses.
	otherHolding := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := shortInput(f, "2026-01-02", 1, 1000)
	buy.HoldingAccountID = otherHolding
	long, err := f.investmentService.Buy(ctx, buy)
	require.NoError(t, err)
	before = buyReplacementPreviewSnapshot(t, f.database)
	election := shortInput(f, "2026-04-01", 1, 700)
	election.CostBasisMethod = "specific_lot"
	election.LotAllocations = []InvestmentLotAllocationInput{{LotID: *long.LotID, QuantityValue: exact.New(1)}}
	_, err = f.investmentService.ShortCover(ctx, election)
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "refusals write nothing")

	opening := shortInput(f, "2026-04-01", 1, 1000)
	opening.CostBasisMethod = "fifo"
	_, err = f.investmentService.ShortSale(ctx, opening)
	require.ErrorAs(t, err, &validation, "an opening takes no disposal method")
}

func TestPositionSideConflictRefusesOverlappingSides(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("short sale while long lots are open", func(t *testing.T) {
		t.Parallel()
		f := newInvestmentsTestFixture(t)
		buyOn(t, f, "2026-01-02", 5, 5000)
		before := buyReplacementPreviewSnapshot(t, f.database)
		_, err := f.investmentService.ShortSale(ctx, shortInput(f, "2026-03-02", 1, 1000))
		require.ErrorIs(t, err, ErrInvestmentPositionSideConflict)
		require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	})
	t.Run("same-day flip: close the long, then open the short", func(t *testing.T) {
		t.Parallel()
		f := newInvestmentsTestFixture(t)
		buyOn(t, f, "2026-01-02", 5, 5000)
		_, err := f.investmentService.Sell(ctx, sellInput(f, "2026-03-02", 5))
		require.NoError(t, err)
		shortSaleOn(t, f, "2026-03-02", 2, 2000)
		shortSelfCheckPasses(t, f)
	})
	t.Run("buy dated before later short activity", func(t *testing.T) {
		t.Parallel()
		f := newInvestmentsTestFixture(t)
		shortSaleOn(t, f, "2026-03-02", 2, 2000)
		_, err := f.investmentService.ShortCover(ctx, shortInput(f, "2026-04-01", 2, 1500))
		require.NoError(t, err)
		before := buyReplacementPreviewSnapshot(t, f.database)
		_, err = f.investmentService.Buy(ctx, shortInput(f, "2026-03-15", 1, 1000))
		require.ErrorIs(t, err, ErrInvestmentPositionSideConflict, "the holding was short then")
		require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
		buyOn(t, f, "2026-04-01", 1, 1000)
		shortSelfCheckPasses(t, f)
	})
	t.Run("other holdings may take the other side", func(t *testing.T) {
		t.Parallel()
		f := newInvestmentsTestFixture(t)
		shortSaleOn(t, f, "2026-03-02", 2, 2000)
		buy := shortInput(f, "2026-03-02", 1, 1000)
		buy.HoldingAccountID = seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
		_, err := f.investmentService.Buy(ctx, buy)
		require.NoError(t, err)
		shortSelfCheckPasses(t, f)
	})
}

func TestShortEntriesRefuseBackdatingUntilReplayExists(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	shortSaleOn(t, f, "2026-03-02", 10, 10000)
	_, err := f.investmentService.ShortCover(ctx, shortInput(f, "2026-05-01", 4, 3000))
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)

	_, err = f.investmentService.ShortSale(ctx, shortInput(f, "2026-04-01", 1, 1000))
	require.ErrorIs(t, err, ErrInvestmentEventOutOfOrder, "an opening behind a later cover")
	_, err = f.investmentService.ShortCover(ctx, shortInput(f, "2026-04-01", 1, 700))
	require.ErrorIs(t, err, ErrInvestmentEventOutOfOrder, "a cover behind a later cover")
	_, err = f.investmentService.PreviewShortCover(ctx, shortInput(f, "2026-04-01", 1, 700))
	require.ErrorIs(t, err, ErrInvestmentEventOutOfOrder, "the preview runs the same writer")
	_, err = f.investmentService.TradeReconciliationImpact(ctx, InvestmentImpactShortSale, shortInput(f, "2026-04-01", 1, 1000))
	require.ErrorIs(t, err, ErrInvestmentEventOutOfOrder)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))

	// Same-day entries stay legal in the order they are entered.
	_, err = f.investmentService.ShortCover(ctx, shortInput(f, "2026-05-01", 6, 4000))
	require.NoError(t, err)
	shortSelfCheckPasses(t, f)
}

func TestShortSaleRefusesChargesThatLeaveNoOpeningProceeds(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	gross, net := int64(100), int64(-50)
	input := shortInput(f, "2026-03-02", 1, 0)
	input.GrossAmountValue, input.GrossAmountScale = &gross, 2
	input.NetSettlementValue, input.NetSettlementScale = &net, 2
	input.Charges = []InvestmentTradeChargeInput{{Kind: "commission", AmountValue: -150, AmountScale: 2, CommodityID: f.eurCommodityID}}
	_, err := f.investmentService.ShortSale(context.Background(), input)
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
	assert.Contains(t, validation.Message, "opening proceeds")
}

func TestShortReconciliationImpactNamesCheckpointsWithoutWriting(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	shortSaleOn(t, f, "2026-03-02", 10, 10000)
	reconcileCash(t, f, "2026-04-01", 100)

	cover := shortInput(f, "2026-03-31", 10, 7000)
	before := buyReplacementPreviewSnapshot(t, f.database)
	impact, err := f.investmentService.TradeReconciliationImpact(ctx, InvestmentImpactShortCover, cover)
	require.NoError(t, err)
	require.Len(t, impact.AffectedCheckpoints, 1)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	_, err = f.investmentService.ShortCover(ctx, cover)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	cover.ReconciliationOverride = true
	_, err = f.investmentService.ShortCover(ctx, cover)
	require.NoError(t, err)
}

func TestNamedShortExplainsNegativeHoldingOnlyUpToOwedUnits(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	shortSaleOn(t, f, "2026-03-02", 10, 10000)
	worth, err := f.transactionService.NetWorth(ctx, NetWorthInput{AsOf: "2026-03-31"})
	require.NoError(t, err)
	assert.Empty(t, worth.UnclassifiedShorts, "ten owed units explain a −10 holding")
	shortSelfCheckPasses(t, f)

	// Damage the subledger so it explains only 6 of the 10 units: the excess
	// is an unclassified negative again, in net worth and in the self-check.
	_, err = f.database.Exec(`DROP TRIGGER investment_lot_events_no_update`)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_lot_events SET quantity_value = '6'
		WHERE lot_id IN (SELECT id FROM investment_lots WHERE position_side = 'short')`)
	require.NoError(t, err)
	worth, err = f.transactionService.NetWorth(ctx, NetWorthInput{AsOf: "2026-03-31"})
	require.NoError(t, err)
	assert.Equal(t, []UnclassifiedShortPosition{{AccountID: f.holdingAccountID, CommodityID: f.stockCommodityID}}, worth.UnclassifiedShorts)
	sign := resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckCommodityPositionSign)
	assert.Equal(t, SelfCheckFailed, sign.Status)
}

func TestSelfCheckDetectsShortLotAndSideDamage(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		damage string
		check  string
	}{
		{"remaining owed units disagree with the holding",
			`UPDATE investment_lot_state SET remaining_quantity_value = '9', status = 'open'
				WHERE lot_id IN (SELECT id FROM investment_lots WHERE position_side = 'short')`, CheckLotReconciliation},
		{"cover decision relabelled long",
			`DROP TRIGGER investment_disposal_decisions_no_update;
				UPDATE investment_disposal_decisions SET position_side = 'long'`, CheckInvestmentFoundation},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			shortSaleOn(t, f, "2026-03-02", 10, 10000)
			_, err := f.investmentService.ShortCover(context.Background(), shortInput(f, "2026-04-01", 10, 7000))
			require.NoError(t, err)
			shortSelfCheckPasses(t, f)
			_, err = f.database.Exec(test.damage)
			require.NoError(t, err)
			assert.Equal(t, SelfCheckFailed, resultFor(t, mustRunInvestmentSelfCheck(t, f), test.check).Status)
		})
	}
}

func TestShortOperationsExportTheirSide(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	shortSaleOn(t, f, "2026-03-02", 10, 10000)
	_, err := f.investmentService.ShortCover(context.Background(), shortInput(f, "2026-04-01", 4, 3000))
	require.NoError(t, err)
	var out bytes.Buffer
	require.NoError(t, NewExportService(db.NewExportRepository(f.database)).WriteBundle(context.Background(), &out, ExportFilter{}))
	archive, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	require.NoError(t, err)
	found := map[string][]string{}
	for _, file := range archive.File {
		switch file.Name {
		case "lots.csv", "disposal-decisions.csv":
		default:
			continue
		}
		reader, err := file.Open()
		require.NoError(t, err)
		rows, err := csv.NewReader(reader).ReadAll()
		require.NoError(t, err)
		require.NoError(t, reader.Close())
		require.Len(t, rows, 2, file.Name)
		index := slices.Index(rows[0], "position_side")
		require.GreaterOrEqual(t, index, 0, file.Name)
		found[file.Name] = append(found[file.Name], rows[1][index])
	}
	assert.Equal(t, map[string][]string{"lots.csv": {"short"}, "disposal-decisions.csv": {"short"}}, found)
}
