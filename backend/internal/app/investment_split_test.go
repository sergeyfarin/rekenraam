package app

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func splitInput(f *investmentsTestFixture, date string, numerator, denominator int64) InvestmentSplitInput {
	return InvestmentSplitInput{
		OwnerUserID: f.ownerUserID, EffectiveOn: date, HoldingAccountID: f.holdingAccountID,
		CommodityID: f.stockCommodityID, RatioNumerator: numerator, RatioDenominator: denominator,
		SourceEvidenceJSON: `{"notice":"corporate action"}`, Memo: "split",
	}
}

// acknowledgedSplit previews and accepts exactly the gain changes disclosed.
func acknowledgedSplit(ctx context.Context, s *InvestmentService, input InvestmentSplitInput) (InvestmentSplitResult, error) {
	if input.GainImpactAcknowledgement == "" {
		if preview, err := s.PreviewSplit(ctx, input); err == nil && preview.Impact.GainImpact != nil {
			input.GainImpactAcknowledgement = preview.Impact.GainImpact.Acknowledgement
		}
	}
	return s.Split(ctx, input)
}

func holdingQuantity(t *testing.T, f *investmentsTestFixture) *exact.ScaledInt {
	t.Helper()
	lots, err := f.investmentService.ListLots(context.Background(), f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	total := exact.NewScaledInt()
	for _, lot := range lots {
		total.AddCoefficient(lot.RemainingQuantityValue, lot.RemainingQuantityScale)
	}
	return total
}

func holdingBasis(t *testing.T, f *investmentsTestFixture) *exact.ScaledInt {
	t.Helper()
	lots, err := f.investmentService.ListLots(context.Background(), f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	total := exact.NewScaledInt()
	for _, lot := range lots {
		if lot.Status == "open" {
			total.AddInt64(lot.RemainingCostBasisValue, lot.RemainingCostBasisScale)
		}
	}
	return total
}

func scaled(value int64, scale int) *exact.ScaledInt { return exact.ScaledIntFromInt64(value, scale) }

func requireSelfCheckPasses(t *testing.T, f *investmentsTestFixture) {
	t.Helper()
	run, err := selfCheckOver(t, f.database).RunSelfCheck(context.Background(), "manual")
	require.NoError(t, err)
	for _, check := range []string{CheckLotReconciliation, CheckInvestmentFoundation, CheckCommodityPositionSign} {
		result := resultFor(t, run, check)
		assert.Equal(t, SelfCheckPassed, result.Status, "%s: %s", check, result.Summary)
	}
}

// effectiveDisposedBasis is the decision's current basis: its latest replay
// revision, else its immutable snapshot.
func effectiveDisposedBasis(t *testing.T, database *sql.DB, transactionID int64) *exact.ScaledInt {
	t.Helper()
	var value exact.Coefficient
	var scale int
	require.NoError(t, database.QueryRow(`SELECT COALESCE(r.disposed_basis_value, d.disposed_basis_value),
		COALESCE(r.disposed_basis_scale, d.disposed_basis_scale)
		FROM investment_disposal_decisions d
		LEFT JOIN investment_disposal_revisions r ON r.decision_id = d.id AND r.revision_seq =
			(SELECT MAX(latest.revision_seq) FROM investment_disposal_revisions latest WHERE latest.decision_id = d.id)
		WHERE d.transaction_id = ?`, transactionID).Scan(&value, &scale))
	return exact.ScaledIntFromCoefficient(value, scale)
}

type splitRowCounts struct{ audits, transactions, facts, events, revisions, splitRevisions int }

func countSplitRows(t *testing.T, database *sql.DB) splitRowCounts {
	t.Helper()
	var counts splitRowCounts
	for query, target := range map[string]*int{
		`SELECT COUNT(*) FROM audit_events`:                  &counts.audits,
		`SELECT COUNT(*) FROM transactions`:                  &counts.transactions,
		`SELECT COUNT(*) FROM investment_split_facts`:        &counts.facts,
		`SELECT COUNT(*) FROM investment_lot_events`:         &counts.events,
		`SELECT COUNT(*) FROM investment_disposal_revisions`: &counts.revisions,
		`SELECT COUNT(*) FROM investment_split_revisions`:    &counts.splitRevisions,
	} {
		require.NoError(t, database.QueryRow(query).Scan(target))
	}
	return counts
}

func TestSplitTenForOneConservesBasisAndPostsQuantityDelta(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-02-01", 10, 10000)
	before := countSplitRows(t, f.database)

	result, err := f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 10, 1))
	require.NoError(t, err)

	postings := result.Transaction.JournalEntries[0].Postings
	require.Len(t, postings, 2)
	assert.Equal(t, f.holdingAccountID, postings[0].AccountID)
	assert.Zero(t, exact.ScaledIntFromCoefficient(postings[0].QuantityValue, postings[0].QuantityScale).Cmp(scaled(90, 0)))
	assert.Zero(t, exact.ScaledIntFromCoefficient(postings[1].QuantityValue, postings[1].QuantityScale).Cmp(scaled(-90, 0)))
	for _, posting := range postings {
		assert.Equal(t, f.stockCommodityID, posting.CommodityID, "a split never posts cash")
	}
	assert.Zero(t, holdingQuantity(t, f).Cmp(scaled(100, 0)))
	assert.Zero(t, holdingBasis(t, f).Cmp(scaled(10000, 2)), "aggregate basis is conserved")
	assert.False(t, result.Plan.Replayed)
	assert.Equal(t, int64(10), result.Plan.RatioNumerator)

	after := countSplitRows(t, f.database)
	assert.Equal(t, before.audits+1, after.audits, "one audit event per command")
	assert.Equal(t, before.facts+1, after.facts)
	assert.Equal(t, before.events+1, after.events)
	requireSelfCheckPasses(t, f)
}

func TestSplitThreeForOneAndLowestTerms(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-02-01", 4, 12000)
	// 6-for-2 is the same action as 3-for-1 and is stored in lowest terms.
	result, err := f.investmentService.Split(context.Background(), splitInput(f, "2026-03-01", 6, 2))
	require.NoError(t, err)
	assert.Equal(t, int64(3), result.Plan.RatioNumerator)
	assert.Equal(t, int64(1), result.Plan.RatioDenominator)
	assert.Zero(t, holdingQuantity(t, f).Cmp(scaled(12, 0)))
	var numerator, denominator int64
	require.NoError(t, f.database.QueryRow(`SELECT ratio_numerator, ratio_denominator FROM investment_split_facts`).Scan(&numerator, &denominator))
	assert.Equal(t, []int64{3, 1}, []int64{numerator, denominator})
	requireSelfCheckPasses(t, f)
}

func TestSplitThreeForTwoWidensScaleExactly(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-02-01", 5, 10000)
	result, err := f.investmentService.Split(context.Background(), splitInput(f, "2026-03-01", 3, 2))
	require.NoError(t, err)
	assert.Equal(t, "25", result.Plan.DeltaValue.String())
	assert.Equal(t, 1, result.Plan.DeltaScale)
	require.Len(t, result.Plan.Effects, 1)
	assert.Equal(t, "75", result.Plan.Effects[0].AfterValue.String())
	assert.Equal(t, 1, result.Plan.Effects[0].AfterScale)
	assert.Zero(t, holdingQuantity(t, f).Cmp(scaled(75, 1)))
	assert.Zero(t, holdingBasis(t, f).Cmp(scaled(10000, 2)))
	requireSelfCheckPasses(t, f)
}

func TestReverseSplitPostsNegativeDeltaAndKeepsBasis(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-02-01", 20, 10000)
	buyOn(t, f, "2026-02-15", 30, 30000)
	result, err := f.investmentService.Split(context.Background(), splitInput(f, "2026-03-01", 1, 10))
	require.NoError(t, err)
	assert.Equal(t, "-45", result.Plan.DeltaValue.String())
	require.Len(t, result.Plan.Effects, 2)
	assert.Zero(t, holdingQuantity(t, f).Cmp(scaled(5, 0)))
	assert.Zero(t, holdingBasis(t, f).Cmp(scaled(40000, 2)))
	requireSelfCheckPasses(t, f)
}

func TestSplitRefusesUnrepresentableFractionWithoutWriting(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-02-01", 10, 10000)
	before := countSplitRows(t, f.database)
	// 10 × 1/3 has no exact decimal at the security's six-place limit.
	_, err := f.investmentService.PreviewSplit(ctx, splitInput(f, "2026-03-01", 1, 3))
	require.ErrorIs(t, err, ErrInvestmentSplitFraction)
	_, err = f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 1, 3))
	require.ErrorIs(t, err, ErrInvestmentSplitFraction)
	assert.Equal(t, before, countSplitRows(t, f.database))
	assert.Zero(t, holdingQuantity(t, f).Cmp(scaled(10, 0)))
}

func TestSplitRefusesNoHoldingsAndOneForOne(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	_, err := f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.ErrorIs(t, err, ErrInvestmentSplitNoHoldings)
	// A lot opened after the effective date is not entitled.
	buyOn(t, f, "2026-04-01", 10, 10000)
	_, err = f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.ErrorIs(t, err, ErrInvestmentSplitNoHoldings)
	var validation ValidationError
	_, err = f.investmentService.Split(ctx, splitInput(f, "2026-05-01", 3, 3))
	require.ErrorAs(t, err, &validation)
}

func TestLaterSaleAfterSplitUsesSplitAdjustedPerShareBasis(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-02-01", 10, 10000)
	_, err := f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.NoError(t, err)
	// Selling 15 of the 20 post-split shares is possible only because of the
	// split, and takes three quarters of the conserved 100.00 basis.
	sale, err := f.investmentService.Sell(ctx, sellInput(f, "2026-04-01", 15))
	require.NoError(t, err)
	assert.Zero(t, effectiveDisposedBasis(t, f.database, sale.Transaction.ID).Cmp(scaled(7500, 2)))
	assert.Zero(t, holdingQuantity(t, f).Cmp(scaled(5, 0)))
	requireSelfCheckPasses(t, f)
}

func TestBackdatedSplitReplaysLaterSaleBehindGainAcknowledgement(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-10", 10, 10000)
	sale, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 5))
	require.NoError(t, err)
	assert.Zero(t, effectiveDisposedBasis(t, f.database, sale.Transaction.ID).Cmp(scaled(5000, 2)))

	input := splitInput(f, "2026-06-01", 2, 1)
	before := countSplitRows(t, f.database)
	_, err = f.investmentService.Split(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	input.GainImpactAcknowledgement = "stale"
	_, err = f.investmentService.Split(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementStale, "the gain check runs after every effect; a late refusal rolls back")
	assert.Equal(t, before, countSplitRows(t, f.database))

	preview, err := f.investmentService.PreviewSplit(ctx, splitInput(f, "2026-06-01", 2, 1))
	require.NoError(t, err)
	assert.Equal(t, before, countSplitRows(t, f.database), "preview leaves no durable rows")
	assert.True(t, preview.Plan.Replayed)
	assert.Equal(t, "10", preview.Plan.DeltaValue.String(), "ten shares were held on the split date")
	require.NotNil(t, preview.Impact.GainImpact)
	require.Len(t, preview.Impact.GainImpact.Changes, 1)
	change := preview.Impact.GainImpact.Changes[0]
	assert.Equal(t, "revised", change.Kind)
	assert.Zero(t, change.Before.DisposedBasis.Cmp(scaled(5000, 2)))
	assert.Zero(t, change.After.DisposedBasis.Cmp(scaled(2500, 2)))

	input.GainImpactAcknowledgement = preview.Impact.GainImpact.Acknowledgement
	result, err := f.investmentService.Split(ctx, input)
	require.NoError(t, err)
	assert.True(t, result.Plan.Replayed)
	assert.Zero(t, effectiveDisposedBasis(t, f.database, sale.Transaction.ID).Cmp(scaled(2500, 2)))
	assert.Zero(t, holdingQuantity(t, f).Cmp(scaled(15, 0)))
	assert.Zero(t, holdingBasis(t, f).Cmp(scaled(7500, 2)))
	requireSelfCheckPasses(t, f)
}

func TestBackdatedReverseSplitRefusesImpossibleLaterSaleAtomically(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-10", 10, 10000)
	sale, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 5))
	require.NoError(t, err)
	before := countSplitRows(t, f.database)
	_, err = acknowledgedSplit(ctx, f.investmentService, splitInput(f, "2026-06-01", 1, 10))
	var dependency InvestmentSplitDependencyError
	require.ErrorAs(t, err, &dependency)
	require.ErrorIs(t, err, ErrInvestmentSplitDependency)
	var saleOperation int64
	require.NoError(t, f.database.QueryRow(`SELECT operation_id FROM investment_disposal_decisions
		WHERE transaction_id = ?`, sale.Transaction.ID).Scan(&saleOperation))
	assert.Equal(t, saleOperation, dependency.OperationID, "the conflict names the later sale")
	assert.Equal(t, before, countSplitRows(t, f.database))
}

func TestEarlierAcquisitionBasisCorrectionReplaysThroughSplit(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buy := buyOn(t, f, "2026-01-10", 10, 10000)
	_, err := f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.NoError(t, err)
	sale, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 5))
	require.NoError(t, err)
	assert.Zero(t, effectiveDisposedBasis(t, f.database, sale.Transaction.ID).Cmp(scaled(2500, 2)))

	replacement := InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-10", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(10), CashAmountValue: 20000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	}
	_, err = acknowledgedReplaceBuy(ctx, f.investmentService, ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: buy.Transaction.ID, Reason: "broker price correction",
		Replacement: replacement,
	})
	require.NoError(t, err)
	// The split now multiplies the replacement lot; its effect is revised, not
	// rewritten, and the later sale takes a quarter of the corrected basis.
	var splitRevisions int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_split_revisions`).Scan(&splitRevisions))
	assert.Equal(t, 1, splitRevisions)
	assert.Zero(t, effectiveDisposedBasis(t, f.database, sale.Transaction.ID).Cmp(scaled(5000, 2)))
	assert.Zero(t, holdingQuantity(t, f).Cmp(scaled(15, 0)))
	assert.Zero(t, holdingBasis(t, f).Cmp(scaled(15000, 2)))
	requireSelfCheckPasses(t, f)
}

// splitAdjustment is one adjustment journal a split revision posted.
type splitAdjustment struct {
	date             string
	holding, trading string
	causedBy         int64
}

// splitAdjustments lists every split adjustment journal in posting order with
// its signed holding and trading quantities, read through its split revision
// and its 'split_adjustment' journal link.
func splitAdjustments(t *testing.T, f *investmentsTestFixture) []splitAdjustment {
	t.Helper()
	rows, err := f.database.Query(`SELECT v.transaction_date, h.quantity_value, h.quantity_scale,
			tr.quantity_value, tr.quantity_scale, r.caused_by_operation_id
		FROM investment_split_revisions r
		JOIN investment_operation_journal_links link ON link.transaction_version_id = r.adjustment_transaction_version_id
			AND link.operation_id = r.operation_id AND link.role = 'split_adjustment'
		JOIN transaction_versions v ON v.id = r.adjustment_transaction_version_id
		JOIN posting_versions h ON h.transaction_version_id = v.id AND h.account_id = ?
		JOIN posting_versions tr ON tr.transaction_version_id = v.id AND tr.account_id <> ?
		ORDER BY r.id`, f.holdingAccountID, f.holdingAccountID)
	require.NoError(t, err)
	defer rows.Close()
	var adjustments []splitAdjustment
	for rows.Next() {
		var adjustment splitAdjustment
		var holding, trading exact.Coefficient
		var holdingScale, tradingScale int
		require.NoError(t, rows.Scan(&adjustment.date, &holding, &holdingScale, &trading, &tradingScale, &adjustment.causedBy))
		adjustment.holding = wholeQuantity(t, holding, holdingScale)
		adjustment.trading = wholeQuantity(t, trading, tradingScale)
		adjustments = append(adjustments, adjustment)
	}
	require.NoError(t, rows.Err())
	return adjustments
}

// wholeQuantity renders a whole-unit quantity at any stored scale.
func wholeQuantity(t *testing.T, value exact.Coefficient, scale int) string {
	t.Helper()
	normalized := exact.ScaledIntFromCoefficient(value, scale).Normalized()
	require.Zero(t, normalized.Scale())
	coefficient, err := normalized.Coefficient()
	require.NoError(t, err)
	return coefficient.String()
}

func operationOf(t *testing.T, f *investmentsTestFixture, transactionID int64) int64 {
	t.Helper()
	var operationID int64
	require.NoError(t, f.database.QueryRow(`SELECT link.operation_id FROM investment_operation_journal_links link
		JOIN transaction_versions v ON v.id = link.transaction_version_id
		WHERE v.transaction_id = ? AND link.role = 'primary'`, transactionID).Scan(&operationID))
	return operationID
}

func TestQuantityCorrectionBeforeSplitPostsAdjustmentJournal(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buy := buyOn(t, f, "2026-01-10", 10, 10000)
	_, err := f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.NoError(t, err)

	// Twelve shares, not ten, were bought: the split multiplied two more.
	replacement := InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-10", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(12), CashAmountValue: 12000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	}
	corrected, err := acknowledgedReplaceBuy(ctx, f.investmentService, ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: buy.Transaction.ID, Reason: "quantity correction",
		Replacement: replacement,
	})
	require.NoError(t, err)
	assert.Equal(t, []splitAdjustment{{date: "2026-03-01", holding: "2", trading: "-2",
		causedBy: operationOf(t, f, corrected.Replacement.Transaction.ID)}}, splitAdjustments(t, f))
	assert.Zero(t, holdingQuantity(t, f).Cmp(scaled(24, 0)))
	assert.Zero(t, holdingBasis(t, f).Cmp(scaled(12000, 2)))
	requireSelfCheckPasses(t, f)
}

func TestBackdatedBuyBeforeSplitPostsAdjustmentAndRevisesLaterGain(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-10", 10, 10000)
	_, err := f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.NoError(t, err)
	saleInput := sellInput(f, "2026-07-01", 20)
	saleInput.CostBasisMethod = "lifo"
	sale, err := f.investmentService.Sell(ctx, saleInput)
	require.NoError(t, err)
	assert.Zero(t, effectiveDisposedBasis(t, f.database, sale.Transaction.ID).Cmp(scaled(10000, 2)))

	// Three backdated shares are entitled to the split and become six. The
	// later LIFO sale takes those six split February shares (60.00)
	// before fourteen January ones (70.00), so its committed gain is revised.
	backdated := InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(3), CashAmountValue: 6000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	}
	_, err = f.investmentService.Buy(ctx, backdated)
	require.Error(t, err, "a revised committed gain needs acknowledgement")
	preview, err := f.investmentService.TradeReconciliationImpact(ctx, InvestmentImpactBuy, backdated)
	require.NoError(t, err)
	require.NotNil(t, preview.GainImpact)
	backdated.GainImpactAcknowledgement = preview.GainImpact.Acknowledgement
	bought, err := f.investmentService.Buy(ctx, backdated)
	require.NoError(t, err)

	// Doubling three shares adds three to the split's delta.
	assert.Equal(t, []splitAdjustment{{date: "2026-03-01", holding: "3", trading: "-3",
		causedBy: operationOf(t, f, bought.Transaction.ID)}}, splitAdjustments(t, f))
	assert.Zero(t, effectiveDisposedBasis(t, f.database, sale.Transaction.ID).Cmp(scaled(13000, 2)))
	assert.Zero(t, holdingQuantity(t, f).Cmp(scaled(6, 0)))
	assert.Zero(t, holdingBasis(t, f).Cmp(scaled(3000, 2)))
	requireSelfCheckPasses(t, f)
}

func TestReversalsBeforeSplitPostAdjustmentJournals(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buy := buyOn(t, f, "2026-01-10", 10, 10000)
	sale, err := f.investmentService.Sell(ctx, sellInput(f, "2026-02-01", 4))
	require.NoError(t, err)
	_, err = f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.NoError(t, err)
	assert.Zero(t, holdingQuantity(t, f).Cmp(scaled(12, 0)))

	// Reversing the sale leaves ten shares, not six, for the split to double.
	_, err = acknowledgedReverseSale(ctx, f.investmentService, ReverseInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: sale.Transaction.ID, Reason: "entered twice"})
	require.NoError(t, err)
	assert.Zero(t, holdingQuantity(t, f).Cmp(scaled(20, 0)))
	requireSelfCheckPasses(t, f)

	// Reversing the purchase leaves nothing for the split to multiply: the
	// second adjustment cancels its remaining delta exactly.
	_, err = acknowledgedReverseBuy(ctx, f.investmentService, ReverseInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: buy.Transaction.ID, Reason: "entered twice"})
	require.NoError(t, err)
	adjustments := splitAdjustments(t, f)
	require.Len(t, adjustments, 2)
	assert.Equal(t, []string{"4", "-10"}, []string{adjustments[0].holding, adjustments[1].holding})
	assert.Equal(t, []string{"-4", "10"}, []string{adjustments[0].trading, adjustments[1].trading})
	assert.Zero(t, holdingQuantity(t, f).Cmp(scaled(0, 0)))
	requireSelfCheckPasses(t, f)
}

func TestSplitAdjustmentIntoReconciledPeriodNeedsOverride(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-10", 10, 10000)
	_, err := f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.NoError(t, err)
	reconcileHoldingThrough(t, f, "2026-04-30", 20)
	backdated := InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(3), CashAmountValue: 3000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	}
	preview, err := f.investmentService.TradeReconciliationImpact(ctx, InvestmentImpactBuy, backdated)
	require.NoError(t, err)
	require.Len(t, preview.AffectedCheckpoints, 1)
	assert.Equal(t, f.holdingAccountID, preview.AffectedCheckpoints[0].AccountID)
	before := countSplitRows(t, f.database)
	_, err = f.investmentService.Buy(ctx, backdated)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	assert.Equal(t, before, countSplitRows(t, f.database))
	backdated.ReconciliationOverride = true
	_, err = f.investmentService.Buy(ctx, backdated)
	require.NoError(t, err)
	assert.Len(t, splitAdjustments(t, f), 1)
	var status string
	require.NoError(t, f.database.QueryRow(`SELECT status FROM reconciliation_checkpoints`).Scan(&status))
	assert.NotEqual(t, "active", status)
	requireSelfCheckPasses(t, f)
}

// TestSplitAdjustmentJournalMeetsCheckpointGuardOnItsOwn isolates the
// writer's guard over adjustment journals. Every current command also posts
// its own journal earlier in the same holding, which would reach the
// checkpoint first; with those positions removed from the triggering journal,
// only the adjustment the replay posts on the split date can reach it.
func TestSplitAdjustmentJournalMeetsCheckpointGuardOnItsOwn(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-10", 10, 10000)
	_, err := f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.NoError(t, err)
	reconcileHoldingThrough(t, f, "2026-04-30", 20)
	journal, lot, err := f.investmentService.prepareBuyWrite(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(3), CashAmountValue: 3000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
		ReconciliationOverride: true,
	})
	require.NoError(t, err)
	journal.CheckpointCandidates = nil

	// The preview reports the checkpoint at the adjustment's date.
	journal.GainImpact = gainImpactPolicy("")
	simulated, err := f.investmentService.repository.SimulateLotOpening(ctx, journal, lot)
	require.NoError(t, err)
	require.Len(t, simulated.InvalidatedCheckpointRefs, 1)
	assert.Equal(t, f.holdingAccountID, simulated.InvalidatedCheckpointRefs[0].AccountID)
	assert.Equal(t, "2026-03-01", simulated.InvalidatedCheckpointRefs[0].EntryDate)

	// Without an override the commit is refused and leaves nothing behind.
	before := countSplitRows(t, f.database)
	journal.ReconciliationOverride = false
	_, _, err = f.investmentService.repository.CreateTransactionAndLot(ctx, journal, lot)
	require.ErrorIs(t, err, db.ErrReconciliationOverrideRequired)
	assert.Equal(t, before, countSplitRows(t, f.database))

	// With it, the checkpoint the adjustment reaches is invalidated.
	journal.ReconciliationOverride = true
	_, _, err = f.investmentService.repository.CreateTransactionAndLot(ctx, journal, lot)
	require.NoError(t, err)
	var status string
	require.NoError(t, f.database.QueryRow(`SELECT status FROM reconciliation_checkpoints`).Scan(&status))
	assert.NotEqual(t, "active", status)
	requireSelfCheckPasses(t, f)
}

func TestSplitAdjustmentRollsBackWithItsCommand(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-10", 10, 10000)
	_, err := f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.NoError(t, err)
	before := countSplitRows(t, f.database)
	beforeHolding := holdingQuantity(t, f)
	// Fail after the adjustment journal, its link and its revision are written.
	_, err = f.database.Exec(`CREATE TRIGGER reject_split_adjustment AFTER INSERT ON investment_split_revisions
		WHEN NEW.adjustment_transaction_version_id IS NOT NULL
		BEGIN SELECT RAISE(ABORT, 'forced split adjustment failure'); END`)
	require.NoError(t, err)
	backdated := InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(3), CashAmountValue: 3000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	}
	_, err = f.investmentService.Buy(ctx, backdated)
	require.ErrorContains(t, err, "forced split adjustment failure")
	assert.Equal(t, before, countSplitRows(t, f.database))
	assert.Zero(t, holdingQuantity(t, f).Cmp(beforeHolding))
	var links int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_operation_journal_links
		WHERE role = 'split_adjustment'`).Scan(&links))
	assert.Zero(t, links)
	requireSelfCheckPasses(t, f)

	_, err = f.database.Exec(`DROP TRIGGER reject_split_adjustment`)
	require.NoError(t, err)
	_, err = f.investmentService.Buy(ctx, backdated)
	require.NoError(t, err)
	assert.Zero(t, holdingQuantity(t, f).Cmp(scaled(26, 0)))
	requireSelfCheckPasses(t, f)
}

func TestSameDaySplitOrderingFollowsEntrySlot(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-10", 10, 10000)
	_, err := f.investmentService.Sell(ctx, sellInput(f, "2026-03-01", 4))
	require.NoError(t, err)
	buyOn(t, f, "2026-03-01", 2, 2000)
	// Entered after the same-day sale and purchase, the split sees 6 + 2.
	result, err := f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.NoError(t, err)
	assert.Equal(t, "8", result.Plan.DeltaValue.String())
	// A purchase entered on the split date afterwards was not entitled.
	buyOn(t, f, "2026-03-01", 5, 5000)
	assert.Zero(t, holdingQuantity(t, f).Cmp(scaled(21, 0)))
	requireSelfCheckPasses(t, f)
}

func TestSplitCommitRefusesPositionChangedAfterPlanning(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-10", 10, 10000)
	journal, split, _, err := f.investmentService.prepareSplitWrite(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.NoError(t, err)
	// Between planning and commit a same-day sale moves the entitled quantity.
	_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-03-01", 3))
	require.NoError(t, err)
	before := countSplitRows(t, f.database)
	_, _, err = f.investmentService.repository.CreateSplit(ctx, journal, split)
	require.ErrorIs(t, err, ErrInvestmentSplitChanged)
	assert.Equal(t, before, countSplitRows(t, f.database))
}

func TestSplitIntoReconciledPeriodNeedsOverride(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-10", 10, 10000)
	reconcileHoldingThrough(t, f, "2026-04-30", 10)
	input := splitInput(f, "2026-03-01", 2, 1)
	preview, err := f.investmentService.PreviewSplit(ctx, input)
	require.NoError(t, err)
	require.Len(t, preview.Impact.AffectedCheckpoints, 1)
	assert.Equal(t, f.holdingAccountID, preview.Impact.AffectedCheckpoints[0].AccountID)
	before := countSplitRows(t, f.database)
	_, err = f.investmentService.Split(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	assert.Equal(t, before, countSplitRows(t, f.database))
	input.ReconciliationOverride = true
	_, err = f.investmentService.Split(ctx, input)
	require.NoError(t, err)
	var status string
	require.NoError(t, f.database.QueryRow(`SELECT status FROM reconciliation_checkpoints`).Scan(&status))
	assert.NotEqual(t, "active", status)
}

// reconcileHoldingThrough reconciles every uncleared security posting of the
// holding account to a statement of the given unit balance.
func reconcileHoldingThrough(t *testing.T, f *investmentsTestFixture, statementDate string, units int64) {
	t.Helper()
	ctx := context.Background()
	session, err := f.transactionService.StartReconciliation(ctx, StartReconciliationInput{
		OriginType: "browser_api", OwnerUserID: f.ownerUserID, AccountID: f.holdingAccountID,
		CommodityID: f.stockCommodityID, StatementDate: statementDate, StatementBalanceValue: exact.New(units),
	})
	require.NoError(t, err)
	postingIDs := make([]int64, 0, len(session.Candidates))
	for _, candidate := range session.Candidates {
		postingIDs = append(postingIDs, candidate.PostingID)
	}
	_, err = f.transactionService.UpdateReconciliationSelection(ctx, ReconciliationSelectionInput{
		OriginType: "browser_api", OwnerUserID: f.ownerUserID, SessionID: session.ID, PostingVersionIDs: postingIDs,
	})
	require.NoError(t, err)
	_, err = f.transactionService.FinishReconciliation(ctx, FinishReconciliationInput{
		OriginType: "browser_api", OwnerUserID: f.ownerUserID, SessionID: session.ID, ChangeReason: "statement verified",
	})
	require.NoError(t, err)
}

func TestSplitThenAverageCostAndSpecificLotSalesUseConservedBasis(t *testing.T) {
	for _, method := range []string{"average_cost", "specific_lot"} {
		t.Run(method, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			first := buyOn(t, f, "2026-01-10", 10, 10000)
			buyOn(t, f, "2026-02-10", 10, 30000)
			_, err := f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
			require.NoError(t, err)
			sale := sellInput(f, "2026-04-01", 10)
			sale.CostBasisMethod = method
			want := scaled(10000, 2) // average: 400.00 × 10/40
			if method == "specific_lot" {
				sale.LotAllocations = []InvestmentLotAllocationInput{{LotID: *first.LotID, QuantityValue: exact.New(10)}}
				want = scaled(5000, 2) // half of the first lot's 100.00
			}
			result, err := f.investmentService.Sell(ctx, sale)
			require.NoError(t, err)
			assert.Zero(t, effectiveDisposedBasis(t, f.database, result.Transaction.ID).Cmp(want))
			assert.Zero(t, holdingQuantity(t, f).Cmp(scaled(30, 0)))
			requireSelfCheckPasses(t, f)
		})
	}
}
