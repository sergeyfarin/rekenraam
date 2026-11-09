package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An instrument's view lists its activity through the commodity filter
// (#229): postings and named cash-dividend facts both identify activity.
func TestListTransactionsFiltersByPostingCommodity(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buy := buyOn(t, f, "2026-01-10", 10, 10000)
	dividend, err := f.investmentService.Dividend(ctx, DividendInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
		CommodityID: NullableValue(f.stockCommodityID), CashAccountID: f.cashAccountID,
		CashCommodityID: f.eurCommodityID, IncomeAccountID: &f.incomeAccountID,
		AmountValue: 500, AmountScale: 2,
	})
	require.NoError(t, err)

	ids := func(commodityID int64) []int64 {
		t.Helper()
		listed, err := f.transactionService.ListTransactions(ctx, ListTransactionsInput{CommodityID: commodityID})
		require.NoError(t, err)
		out := []int64{}
		for _, transaction := range listed.Transactions {
			out = append(out, transaction.ID)
		}
		return out
	}

	assert.ElementsMatch(t, []int64{buy.Transaction.ID, dividend.ID}, ids(f.stockCommodityID))
	assert.ElementsMatch(t, []int64{buy.Transaction.ID, dividend.ID}, ids(f.eurCommodityID))
	assert.Empty(t, ids(f.stockCommodityID+f.eurCommodityID+1000))
}

func TestInstrumentActivityRetainsDividendWithholdingThroughReplacementAndReversal(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	withholding := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	input := cashDividendInput(f, withholding, 10000, 1500)
	input.CommodityID = NullableValue(f.stockCommodityID)
	original, err := f.investmentService.Dividend(ctx, input)
	require.NoError(t, err)
	replacement, err := f.investmentService.ReplaceDividend(ctx, ReplaceInvestmentDividendInput{
		OwnerUserID: f.ownerUserID, TransactionID: original.ID, Reason: "correct tax",
		Replacement: cashDividendInput(f, withholding, 10000, 2000),
	})
	require.NoError(t, err)
	wrongShape := cashDividendInput(f, withholding, 2000, 0)
	wrongShape.WithholdingOnly = true
	_, err = f.investmentService.ReplaceDividend(ctx, ReplaceInvestmentDividendInput{
		OwnerUserID: f.ownerUserID, TransactionID: replacement.Replacement.ID, Reason: "cannot change shape", Replacement: wrongShape,
	})
	var validation ValidationError
	require.ErrorAs(t, err, &validation)

	listed, err := f.transactionService.ListTransactions(ctx, ListTransactionsInput{CommodityID: f.stockCommodityID})
	require.NoError(t, err)
	ids := []int64{}
	for _, transaction := range listed.Transactions {
		ids = append(ids, transaction.ID)
	}
	require.ElementsMatch(t, []int64{original.ID, replacement.Inverse.ID, replacement.Replacement.ID}, ids)
	require.Equal(t, int64(8000), accountBalance(t, f, f.cashAccountID))
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, original.ID)
	require.NoError(t, err)
	require.NotNil(t, chain.EffectiveDividend)
	require.Equal(t, &f.stockCommodityID, chain.EffectiveDividend.CommodityID)
	reversal, err := f.investmentService.ReverseDividend(ctx, ReverseInvestmentDividendInput{
		OwnerUserID: f.ownerUserID, TransactionID: replacement.Replacement.ID, Reason: "duplicate",
	})
	require.NoError(t, err)
	listed, err = f.transactionService.ListTransactions(ctx, ListTransactionsInput{CommodityID: f.stockCommodityID})
	require.NoError(t, err)
	ids = nil
	for _, transaction := range listed.Transactions {
		ids = append(ids, transaction.ID)
	}
	require.ElementsMatch(t, []int64{original.ID, replacement.Inverse.ID, replacement.Replacement.ID, reversal.ID}, ids)
	require.Zero(t, accountBalance(t, f, f.cashAccountID))
	requireInvestmentSelfCheckPasses(t, f)
}

func TestDividendInstrumentReplacementKeepsInverseWithOriginalPayer(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	instrument, err := f.investmentService.CreateInstrument(ctx, InvestmentInstrumentInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", ChangeReason: "test payer", CommodityCode: "OTHER", InstrumentType: "stock", DisplayName: "Other payer",
		QuoteCommodityID: &f.eurCommodityID, TradingCommodityID: &f.eurCommodityID, QuantityScale: 0, PriceScale: 2,
	})
	require.NoError(t, err)
	originalInput := cashDividendInput(f, 0, 10000, 0)
	originalInput.CommodityID = NullableValue(f.stockCommodityID)
	original, err := f.investmentService.Dividend(ctx, originalInput)
	require.NoError(t, err)
	replacementInput := originalInput
	replacementInput.CommodityID = NullableValue(instrument.CommodityID)
	replacement, err := f.investmentService.ReplaceDividend(ctx, ReplaceInvestmentDividendInput{
		OwnerUserID: f.ownerUserID, TransactionID: original.ID, Reason: "wrong payer", Replacement: replacementInput,
	})
	require.NoError(t, err)
	oldActivity, err := f.transactionService.ListTransactions(ctx, ListTransactionsInput{CommodityID: f.stockCommodityID})
	require.NoError(t, err)
	oldIDs := []int64{}
	for _, row := range oldActivity.Transactions {
		oldIDs = append(oldIDs, row.ID)
	}
	require.ElementsMatch(t, []int64{original.ID, replacement.Inverse.ID}, oldIDs)
	newActivity, err := f.transactionService.ListTransactions(ctx, ListTransactionsInput{CommodityID: instrument.CommodityID})
	require.NoError(t, err)
	require.Len(t, newActivity.Transactions, 1)
	require.Equal(t, replacement.Replacement.ID, newActivity.Transactions[0].ID)
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, original.ID)
	require.NoError(t, err)
	require.Equal(t, &instrument.CommodityID, chain.EffectiveDividend.CommodityID)
	requireInvestmentSelfCheckPasses(t, f)
}

func TestDividendInstrumentFactsAreImmutableAndValidatePayer(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	input := cashDividendInput(f, 0, 10000, 0)
	input.CommodityID = NullableValue(f.eurCommodityID)
	_, err := f.investmentService.Dividend(ctx, input)
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
	input.CommodityID = NullableValue(f.stockCommodityID)
	dividend, err := f.investmentService.Dividend(ctx, input)
	require.NoError(t, err)
	operation, err := f.investmentService.repository.DividendOperationByTransactionID(ctx, BookID, dividend.ID)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_dividend_facts SET commodity_id = ? WHERE operation_id = ?`, f.eurCommodityID, operation.OperationID)
	require.ErrorContains(t, err, "immutable")
	_, err = f.database.Exec(`DELETE FROM investment_dividend_facts WHERE operation_id = ?`, operation.OperationID)
	require.ErrorContains(t, err, "immutable")
	// A cash-only historical/unassigned dividend remains valid and does not guess a payer.
	input.CommodityID = NullablePatch[int64]{}
	unassigned, err := f.investmentService.Dividend(ctx, input)
	require.NoError(t, err)
	operation, err = f.investmentService.repository.DividendOperationByTransactionID(ctx, BookID, unassigned.ID)
	require.NoError(t, err)
	require.False(t, operation.CommodityID.Valid)
	var auditID int64
	require.NoError(t, f.database.QueryRow(`SELECT created_audit_event_id FROM investment_operations WHERE id = ?`, operation.OperationID).Scan(&auditID))
	_, err = f.database.Exec(`INSERT INTO investment_dividend_facts (operation_id, book_id, commodity_id, created_audit_event_id) VALUES (?, ?, ?, ?)`, operation.OperationID, BookID, f.eurCommodityID, auditID)
	require.ErrorContains(t, err, "invalid dividend instrument fact")
	var count int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_dividend_facts`).Scan(&count))
	require.Equal(t, 2, count, "assigned and unassigned facts both retain payment shape")

}

func TestSeparateWithholdingIsInstrumentActivityWithoutGrossIncome(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	taxAccount := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	input := DividendInput{OwnerUserID: f.ownerUserID, WithholdingOnly: true, CommodityID: NullableValue(f.stockCommodityID),
		TransactionDate: "2026-03-01", CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		AmountValue: 1500, AmountScale: 2, WithholdingAccountID: &taxAccount, Memo: "separate tax"}
	tax, err := f.investmentService.Dividend(ctx, input)
	require.NoError(t, err)
	require.Equal(t, int64(-1500), accountBalance(t, f, f.cashAccountID))
	require.Zero(t, accountBalance(t, f, f.incomeAccountID))
	require.Len(t, tax.JournalEntries[0].Postings, 2)
	income, err := f.investmentService.repository.InvestmentIncome(ctx, BookID, "2026-01-01", "2026-12-31")
	require.NoError(t, err)
	require.Empty(t, income, "tax alone must never manufacture dividend income")
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, tax.ID)
	require.NoError(t, err)
	require.True(t, chain.CanCorrectDividend)
	require.True(t, chain.EffectiveDividend.WithholdingOnly)
	require.Equal(t, &f.stockCommodityID, chain.EffectiveDividend.CommodityID)
	input.AmountValue = 2000
	before := buyReplacementPreviewSnapshot(t, f.database)
	replacement := ReplaceInvestmentDividendInput{OwnerUserID: f.ownerUserID, TransactionID: tax.ID, Reason: "correct tax", Replacement: input}
	_, err = f.investmentService.ReplaceDividendReconciliationImpact(ctx, replacement)
	require.NoError(t, err)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	result, err := f.investmentService.ReplaceDividend(ctx, replacement)
	require.NoError(t, err)
	require.Equal(t, int64(-2000), accountBalance(t, f, f.cashAccountID))
	require.Equal(t, int64(2000), accountBalance(t, f, taxAccount))
	listed, err := f.transactionService.ListTransactions(ctx, ListTransactionsInput{CommodityID: f.stockCommodityID})
	require.NoError(t, err)
	require.Len(t, listed.Transactions, 3)
	requireInvestmentSelfCheckPasses(t, f)
	_, err = f.investmentService.ReverseDividend(ctx, ReverseInvestmentDividendInput{
		OwnerUserID: f.ownerUserID, TransactionID: result.Replacement.ID, Reason: "duplicate tax"})
	require.NoError(t, err)
	require.Zero(t, accountBalance(t, f, f.cashAccountID))
	require.Zero(t, accountBalance(t, f, taxAccount))
	requireInvestmentSelfCheckPasses(t, f)
}

func TestSeparateWithholdingRequiresTaxShapeAndValidSuppliedPayer(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	taxAccount := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	valid := DividendInput{OwnerUserID: f.ownerUserID, WithholdingOnly: true, CommodityID: NullableValue(f.stockCommodityID),
		TransactionDate: "2026-03-01", CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		AmountValue: 1500, AmountScale: 2, WithholdingAccountID: &taxAccount}
	for _, mutate := range []func(*DividendInput){
		func(i *DividendInput) { i.CommodityID = NullableValue(int64(0)) },
		func(i *DividendInput) { i.WithholdingAccountID = nil },
		func(i *DividendInput) { i.IncomeAccountID = &f.incomeAccountID },
		func(i *DividendInput) { i.WithholdingValue = &i.AmountValue },
		func(i *DividendInput) { i.WithholdingAccountID = &f.incomeAccountID },
	} {
		input := valid
		mutate(&input)
		before := buyReplacementPreviewSnapshot(t, f.database)
		_, err := f.investmentService.Dividend(ctx, input)
		var validation ValidationError
		require.ErrorAs(t, err, &validation)
		require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	}
}

func TestSeparateWithholdingReconciliationPreviewGuardsCreate(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	_, err := f.investmentService.Dividend(ctx, cashDividendInput(f, 0, 10000, 0))
	require.NoError(t, err)
	checkpoint := reconcileCash(t, f, "2026-04-01", 100)
	taxAccount := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	tax := DividendInput{OwnerUserID: f.ownerUserID, WithholdingOnly: true, CommodityID: NullableValue(f.stockCommodityID),
		TransactionDate: "2026-03-01", CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		AmountValue: 1500, AmountScale: 2, WithholdingAccountID: &taxAccount}
	before := buyReplacementPreviewSnapshot(t, f.database)
	impact, err := f.investmentService.DividendReconciliationImpact(ctx, tax)
	require.NoError(t, err)
	require.Len(t, impact.AffectedCheckpoints, 1)
	require.Equal(t, checkpoint, impact.AffectedCheckpoints[0].CheckpointID)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	_, err = f.investmentService.Dividend(ctx, tax)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	tax.ReconciliationOverride = unboundOverride
	recorded, err := f.investmentService.Dividend(ctx, tax)
	require.NoError(t, err)
	require.NotZero(t, recorded.ID)
	require.Empty(t, activeCheckpointIDs(t, f))
}

func TestSeparateWithholdingCorrectionRequiresReconciliationOverride(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	taxAccount := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	tax := DividendInput{OwnerUserID: f.ownerUserID, WithholdingOnly: true, CommodityID: NullableValue(f.stockCommodityID),
		TransactionDate: "2026-03-01", CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		AmountValue: 1500, AmountScale: 2, WithholdingAccountID: &taxAccount}
	recorded, err := f.investmentService.Dividend(ctx, tax)
	require.NoError(t, err)
	checkpoint := reconcileCash(t, f, "2026-04-01", -15)
	replacement := tax
	replacement.AmountValue = 2000
	correction := ReplaceInvestmentDividendInput{OwnerUserID: f.ownerUserID, TransactionID: recorded.ID,
		Reason: "correct tax", Replacement: replacement}
	impact, err := f.investmentService.ReplaceDividendReconciliationImpact(ctx, correction)
	require.NoError(t, err)
	require.Len(t, impact.AffectedCheckpoints, 1)
	require.Equal(t, checkpoint, impact.AffectedCheckpoints[0].CheckpointID)
	_, err = f.investmentService.ReplaceDividend(ctx, correction)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	correction.ReconciliationOverride = unboundOverride
	_, err = f.investmentService.ReplaceDividend(ctx, correction)
	require.NoError(t, err)
	require.Equal(t, int64(-2000), accountBalance(t, f, f.cashAccountID))
	requireInvestmentSelfCheckPasses(t, f)
}
