package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func reverseSplitInput(f *investmentsTestFixture, transactionID int64) ReverseInvestmentSplitInput {
	return ReverseInvestmentSplitInput{OwnerUserID: f.ownerUserID, TransactionID: transactionID, Reason: "entered in error"}
}

func replaceSplitInput(f *investmentsTestFixture, transactionID int64, date string, numerator, denominator int64) ReplaceInvestmentSplitInput {
	return ReplaceInvestmentSplitInput{OwnerUserID: f.ownerUserID, TransactionID: transactionID, Reason: "wrong terms",
		EffectiveOn: date, RatioNumerator: numerator, RatioDenominator: denominator, Memo: "split"}
}

// acknowledgedReverseSplit previews and accepts exactly the gain changes disclosed.
func acknowledgedReverseSplit(ctx context.Context, s *InvestmentService, input ReverseInvestmentSplitInput) (Transaction, error) {
	if impact, err := s.ReverseSplitReconciliationImpact(ctx, input); err == nil && impact.GainImpact != nil {
		input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	}
	return s.ReverseSplit(ctx, input)
}

func acknowledgedReplaceSplit(ctx context.Context, s *InvestmentService, input ReplaceInvestmentSplitInput) (ReplaceInvestmentSplitResult, error) {
	if preview, err := s.PreviewSplitReplacement(ctx, input); err == nil && preview.Impact.GainImpact != nil {
		input.GainImpactAcknowledgement = preview.Impact.GainImpact.Acknowledgement
	}
	return s.ReplaceSplit(ctx, input)
}

// holdingPosting is the signed security quantity a journal posts to the
// fixture's holding account.
func holdingPosting(t *testing.T, f *investmentsTestFixture, transaction Transaction) string {
	t.Helper()
	total := exact.NewScaledInt()
	for _, entry := range transaction.JournalEntries {
		for _, posting := range entry.Postings {
			if posting.AccountID == f.holdingAccountID && posting.CommodityID == f.stockCommodityID {
				total.AddCoefficient(posting.QuantityValue, posting.QuantityScale)
			}
		}
	}
	normalized := total.Normalized()
	value, err := normalized.Coefficient()
	require.NoError(t, err)
	require.Zero(t, normalized.Scale())
	return value.String()
}

func TestSplitReversalPostsExactInverseAndRestoresUnsplitHolding(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-10", 10, 10000)
	split, err := f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.NoError(t, err)

	inverse, err := acknowledgedReverseSplit(ctx, f.investmentService, reverseSplitInput(f, split.Transaction.ID))
	require.NoError(t, err)
	assert.Equal(t, "2026-03-01", inverse.TransactionDate)
	assert.Equal(t, "-10", holdingPosting(t, f, inverse))
	assert.Zero(t, holdingQuantity(t, f).Cmp(scaled(10, 0)))
	assert.Zero(t, holdingBasis(t, f).Cmp(scaled(10000, 2)))

	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, split.Transaction.ID)
	require.NoError(t, err)
	require.Len(t, chain.Operations, 2)
	assert.Equal(t, "reverse", chain.Operations[1].CorrectionMode)
	assert.False(t, chain.CanCorrectSplit)
	_, err = f.investmentService.ReverseSplit(ctx, reverseSplitInput(f, split.Transaction.ID))
	require.ErrorIs(t, err, ErrInvestmentSplitAlreadyCorrected)
	requireSelfCheckPasses(t, f)
}

func TestSplitReversalInvertsPrimaryPlusAdjustmentDelta(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-10", 10, 10000)
	split, err := f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.NoError(t, err)
	_, err = f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(3), CashAmountValue: 3000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)
	require.Len(t, splitAdjustments(t, f), 1)

	// The split's journals moved +10 and then +3; the inverse cancels both.
	inverse, err := acknowledgedReverseSplit(ctx, f.investmentService, reverseSplitInput(f, split.Transaction.ID))
	require.NoError(t, err)
	assert.Equal(t, "-13", holdingPosting(t, f, inverse))
	assert.Zero(t, holdingQuantity(t, f).Cmp(scaled(13, 0)))
	requireSelfCheckPasses(t, f)
}

func TestSplitReversalRefusesImpossibleLaterSaleAtomically(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-10", 10, 10000)
	split, err := f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.NoError(t, err)
	sale, err := f.investmentService.Sell(ctx, sellInput(f, "2026-05-01", 15))
	require.NoError(t, err)
	before := countSplitRows(t, f.database)

	// Without the split there were only ten shares to sell fifteen of.
	for name, attempt := range map[string]func() error{
		"preview": func() error {
			_, err := f.investmentService.ReverseSplitReconciliationImpact(ctx, reverseSplitInput(f, split.Transaction.ID))
			return err
		},
		"commit": func() error {
			_, err := f.investmentService.ReverseSplit(ctx, reverseSplitInput(f, split.Transaction.ID))
			return err
		},
	} {
		err := attempt()
		var dependency InvestmentSplitDependencyError
		require.ErrorAs(t, err, &dependency, name)
		assert.Equal(t, operationOf(t, f, sale.Transaction.ID), dependency.OperationID, name)
	}
	assert.Equal(t, before, countSplitRows(t, f.database))
	assert.Zero(t, holdingQuantity(t, f).Cmp(scaled(5, 0)))
	requireSelfCheckPasses(t, f)
}

func TestSplitReplacementCorrectsRatioAndRevisesLaterGain(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-10", 10, 12000)
	split, err := f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.NoError(t, err)
	sale, err := f.investmentService.Sell(ctx, sellInput(f, "2026-05-01", 10))
	require.NoError(t, err)
	assert.Zero(t, effectiveDisposedBasis(t, f.database, sale.Transaction.ID).Cmp(scaled(6000, 2)))

	// It was a 3-for-1: the later sale of ten took a third of the basis.
	input := replaceSplitInput(f, split.Transaction.ID, "2026-03-01", 3, 1)
	_, err = f.investmentService.ReplaceSplit(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	preview, err := f.investmentService.PreviewSplitReplacement(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, "20", preview.Plan.DeltaValue.String())
	require.NotNil(t, preview.Impact.GainImpact)
	input.GainImpactAcknowledgement = preview.Impact.GainImpact.Acknowledgement
	result, err := f.investmentService.ReplaceSplit(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, "-10", holdingPosting(t, f, result.Inverse))
	assert.Equal(t, "20", holdingPosting(t, f, result.Replacement))
	assert.Zero(t, effectiveDisposedBasis(t, f.database, sale.Transaction.ID).Cmp(scaled(4000, 2)))
	assert.Zero(t, holdingQuantity(t, f).Cmp(scaled(20, 0)))
	assert.Zero(t, holdingBasis(t, f).Cmp(scaled(8000, 2)))

	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, result.Replacement.ID)
	require.NoError(t, err)
	require.Len(t, chain.Operations, 2)
	assert.Equal(t, "replace", chain.Operations[1].CorrectionMode)
	assert.True(t, chain.CanCorrectSplit, "the replacement is itself correctable")
	_, err = f.investmentService.ReplaceSplit(ctx, replaceSplitInput(f, split.Transaction.ID, "2026-03-01", 4, 1))
	require.ErrorIs(t, err, ErrInvestmentSplitAlreadyCorrected)
	requireSelfCheckPasses(t, f)
}

func TestSplitReplacementMovesDateOverLaterPurchase(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-10", 10, 10000)
	buyOn(t, f, "2026-03-10", 5, 5000)
	split, err := f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.NoError(t, err)
	assert.Equal(t, "10", split.Plan.DeltaValue.String())

	// Effective on 15 March the split also doubles the 10 March purchase.
	result, err := acknowledgedReplaceSplit(ctx, f.investmentService, replaceSplitInput(f, split.Transaction.ID, "2026-03-15", 2, 1))
	require.NoError(t, err)
	assert.Equal(t, "2026-03-01", result.Inverse.TransactionDate)
	assert.Equal(t, "2026-03-15", result.Replacement.TransactionDate)
	assert.Equal(t, "15", holdingPosting(t, f, result.Replacement))
	assert.Zero(t, holdingQuantity(t, f).Cmp(scaled(30, 0)))
	requireSelfCheckPasses(t, f)
}

func TestSplitReplacementRefusesUnchangedTermsAndOtherKinds(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buy := buyOn(t, f, "2026-01-10", 10, 10000)
	split, err := f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.NoError(t, err)
	before := countSplitRows(t, f.database)
	_, err = f.investmentService.ReplaceSplit(ctx, replaceSplitInput(f, split.Transaction.ID, "2026-03-01", 4, 2))
	var validation ValidationError
	require.ErrorAs(t, err, &validation, "4-for-2 is the same split")
	_, err = f.investmentService.ReverseSplit(ctx, reverseSplitInput(f, buy.Transaction.ID))
	require.ErrorIs(t, err, ErrInvestmentSplitNotFound)
	_, err = f.investmentService.ReverseSplit(ctx, ReverseInvestmentSplitInput{OwnerUserID: f.ownerUserID, TransactionID: split.Transaction.ID})
	require.ErrorAs(t, err, &validation, "a reason is required")
	assert.Equal(t, before, countSplitRows(t, f.database))
}

func TestSplitReversalIntoReconciledPeriodNeedsOverride(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-10", 10, 10000)
	split, err := f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.NoError(t, err)
	reconcileHoldingThrough(t, f, "2026-04-30", 20)
	input := reverseSplitInput(f, split.Transaction.ID)
	impact, err := f.investmentService.ReverseSplitReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, impact.AffectedCheckpoints, 1)
	assert.Equal(t, f.holdingAccountID, impact.AffectedCheckpoints[0].AccountID)
	before := countSplitRows(t, f.database)
	_, err = f.investmentService.ReverseSplit(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	assert.Equal(t, before, countSplitRows(t, f.database))
	input.ReconciliationOverride = true
	_, err = f.investmentService.ReverseSplit(ctx, input)
	require.NoError(t, err)
	requireSelfCheckPasses(t, f)
}

func TestSplitCorrectionRollsBackLateFailure(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"reverse", "replace"} {
		t.Run(kind, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			buyOn(t, f, "2026-01-10", 10, 10000)
			split, err := f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
			require.NoError(t, err)
			_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-05-01", 5))
			require.NoError(t, err)
			reverse := reverseSplitInput(f, split.Transaction.ID)
			replace := replaceSplitInput(f, split.Transaction.ID, "2026-03-01", 3, 1)
			// Accept the disclosed gain changes before installing the failure.
			if kind == "reverse" {
				impact, err := f.investmentService.ReverseSplitReconciliationImpact(ctx, reverse)
				require.NoError(t, err)
				reverse.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
			} else {
				preview, err := f.investmentService.PreviewSplitReplacement(ctx, replace)
				require.NoError(t, err)
				replace.GainImpactAcknowledgement = preview.Impact.GainImpact.Acknowledgement
			}
			before := countSplitRows(t, f.database)
			beforeHolding := holdingQuantity(t, f)
			// Fail after the journals, operation and replayed disposal revision.
			_, err = f.database.Exec(`CREATE TRIGGER reject_split_correction AFTER INSERT ON investment_disposal_revisions
				BEGIN SELECT RAISE(ABORT, 'forced split correction failure'); END`)
			require.NoError(t, err)
			commit := func() error {
				if kind == "reverse" {
					_, err := f.investmentService.ReverseSplit(ctx, reverse)
					return err
				}
				_, err := f.investmentService.ReplaceSplit(ctx, replace)
				return err
			}
			require.ErrorContains(t, commit(), "forced split correction failure")
			assert.Equal(t, before, countSplitRows(t, f.database))
			assert.Zero(t, holdingQuantity(t, f).Cmp(beforeHolding))
			requireSelfCheckPasses(t, f)

			_, err = f.database.Exec(`DROP TRIGGER reject_split_correction`)
			require.NoError(t, err)
			require.NoError(t, commit())
			requireSelfCheckPasses(t, f)
		})
	}
}

func TestSplitCorrectionsAndAdjustmentsExportTheirLinks(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-10", 10, 10000)
	split, err := f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.NoError(t, err)
	_, err = f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(3), CashAmountValue: 3000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)
	_, err = acknowledgedReplaceSplit(ctx, f.investmentService, replaceSplitInput(f, split.Transaction.ID, "2026-03-01", 3, 1))
	require.NoError(t, err)

	var out bytes.Buffer
	require.NoError(t, NewExportService(db.NewExportRepository(f.database)).WriteBundle(ctx, &out, ExportFilter{}))
	archive, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	require.NoError(t, err)
	read := func(name string) [][]string {
		for _, entry := range archive.File {
			if entry.Name == name {
				reader, err := entry.Open()
				require.NoError(t, err)
				content, err := io.ReadAll(reader)
				require.NoError(t, err)
				records, err := csv.NewReader(bytes.NewReader(content)).ReadAll()
				require.NoError(t, err)
				return records
			}
		}
		t.Fatalf("bundle has no %s", name)
		return nil
	}
	roles := map[string]int{}
	for _, row := range read("investment-operation-journal-links.csv")[1:] {
		roles[row[3]]++
	}
	assert.Equal(t, 1, roles["split_adjustment"])
	assert.Equal(t, 1, roles["reversal"], "the replacement links the inverse journal")
	revisions := read("investment-split-revisions.csv")
	require.Len(t, revisions, 2)
	assert.Equal(t, "adjustment_transaction_version_id", revisions[0][8])
	assert.NotEmpty(t, revisions[1][8])
	requireSelfCheckPasses(t, f)
}

func TestSplitAdjustmentJournalsCarryASystemLabel(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-10", 10, 10000)
	split, err := f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.NoError(t, err)
	_, err = f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(3), CashAmountValue: 3000, CashAmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)

	listed, err := f.transactionService.ListTransactions(ctx, ListTransactionsInput{})
	require.NoError(t, err)
	transactions := listed.Transactions
	labels := map[string]int{}
	var adjustmentID int64
	for _, transaction := range transactions {
		labels[transaction.SystemLabel]++
		if transaction.SystemLabel == "split_adjustment" {
			adjustmentID = transaction.ID
		}
	}
	assert.Equal(t, 1, labels["split_adjustment"])
	assert.Equal(t, len(transactions)-1, labels[""], "only the adjustment journal is labelled")

	register, err := f.transactionService.Register(ctx, f.holdingAccountID, ListTransactionsInput{})
	require.NoError(t, err)
	registerLabels := 0
	for _, entry := range register.Entries {
		if entry.SystemLabel == "split_adjustment" {
			registerLabels++
			assert.Equal(t, adjustmentID, entry.TransactionID)
		}
	}
	assert.Equal(t, 1, registerLabels)

	// The adjustment's correction chain resolves the split it adjusts.
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, adjustmentID)
	require.NoError(t, err)
	require.NotNil(t, chain.EffectiveTransactionID)
	assert.Equal(t, split.Transaction.ID, *chain.EffectiveTransactionID)
	assert.True(t, chain.CanCorrectSplit)
}

func TestSplitReplacementKeepsTheReplacedSplitsSameDaySlot(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-10", 10, 10000)
	split, err := f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.NoError(t, err)
	// Entered after the split on its date, this purchase was not entitled.
	buyOn(t, f, "2026-03-01", 5, 5000)
	result, err := acknowledgedReplaceSplit(ctx, f.investmentService, replaceSplitInput(f, split.Transaction.ID, "2026-03-01", 3, 1))
	require.NoError(t, err)
	// The corrected ratio still applies only to the ten earlier shares.
	assert.Equal(t, "20", holdingPosting(t, f, result.Replacement))
	assert.Zero(t, holdingQuantity(t, f).Cmp(scaled(35, 0)))
	requireSelfCheckPasses(t, f)
}
