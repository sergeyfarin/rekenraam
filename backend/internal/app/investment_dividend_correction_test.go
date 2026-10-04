package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// T-115: a cash dividend is corrected by an audited inverse journal, plus a
// replacement when the amounts were wrong. Nothing replays; the original
// journal stays posted history.

func cashDividendInput(f *investmentsTestFixture, withholdingAccountID int64, gross, withheld int64) DividendInput {
	scale := 2
	input := DividendInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-03-01",
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, IncomeAccountID: &f.incomeAccountID,
		AmountValue: gross, AmountScale: 2, Memo: "ACME dividend"}
	if withheld > 0 {
		input.WithholdingAccountID = &withholdingAccountID
		input.WithholdingValue = &withheld
		input.WithholdingScale = &scale
	}
	return input
}

// accountBalance sums an account's current postings in cents.
func accountBalance(t *testing.T, f *investmentsTestFixture, accountID int64) int64 {
	t.Helper()
	rows, err := f.database.Query(`SELECT pv.quantity_value, pv.quantity_scale FROM posting_versions pv
		JOIN current_transaction_versions current ON current.id = pv.transaction_version_id
		JOIN transactions tx ON tx.id = current.transaction_id
		WHERE pv.account_id = ? AND tx.deleted_at IS NULL`, accountID)
	require.NoError(t, err)
	defer rows.Close()
	total := exact.ScaledIntFromInt64(0, 2)
	for rows.Next() {
		var value exact.Coefficient
		var scale int
		require.NoError(t, rows.Scan(&value, &scale))
		total.AddScaled(exact.ScaledIntFromCoefficient(value, scale))
	}
	require.NoError(t, rows.Err())
	cents, err := total.TruncatedTo(2).Int64()
	require.NoError(t, err)
	return cents
}

func TestReplaceDividendCorrectsAmountAndWithholding(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	withholding := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	original, err := f.investmentService.Dividend(ctx, cashDividendInput(f, withholding, 10000, 1500))
	require.NoError(t, err)
	require.Equal(t, int64(8500), accountBalance(t, f, f.cashAccountID))

	input := ReplaceInvestmentDividendInput{OwnerUserID: f.ownerUserID, TransactionID: original.ID,
		Reason:      "statement shows 120.00 gross, 18.00 withheld",
		Replacement: cashDividendInput(f, withholding, 12000, 1800)}
	before := buyReplacementPreviewSnapshot(t, f.database)
	impact, err := f.investmentService.ReplaceDividendReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Empty(t, impact.AffectedCheckpoints)
	require.Nil(t, impact.GainImpact, "a cash dividend replays no disposal")
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "preview writes nothing")

	result, err := f.investmentService.ReplaceDividend(ctx, input)
	require.NoError(t, err)
	require.Equal(t, original.ID, result.CorrectedTransactionID)
	require.Equal(t, original.ID, *result.Inverse.CorrectionOfTransactionID)
	require.Equal(t, original.ID, *result.Replacement.CorrectionOfTransactionID)
	for entryIndex, entry := range original.JournalEntries {
		for postingIndex, posting := range entry.Postings {
			require.Equal(t, posting.QuantityValue.Negated(), result.Inverse.JournalEntries[entryIndex].Postings[postingIndex].QuantityValue)
		}
	}
	require.Equal(t, int64(10200), accountBalance(t, f, f.cashAccountID), "net cash is the corrected 102.00")
	require.Equal(t, int64(-12000), accountBalance(t, f, f.incomeAccountID))
	require.Equal(t, int64(1800), accountBalance(t, f, withholding))

	var components int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_operation_components c
		JOIN investment_operations o ON o.id = c.operation_id
		WHERE o.correction_of_operation_id IS NOT NULL`).Scan(&components))
	require.Equal(t, 2, components, "the replacement records its own gross and withholding")
	var operation string
	require.NoError(t, f.database.QueryRow(`SELECT audit.operation FROM transactions tx
		JOIN audit_events audit ON audit.id = tx.created_audit_event_id WHERE tx.id = ?`, result.Replacement.ID).Scan(&operation))
	require.Equal(t, "investment.dividend.replace", operation)
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, original.ID)
	require.NoError(t, err)
	require.Len(t, chain.Operations, 2)
	require.Equal(t, result.Replacement.ID, *chain.EffectiveTransactionID)
	requireInvestmentSelfCheckPasses(t, f)

	_, err = f.investmentService.ReplaceDividend(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentDividendAlreadyCorrected)
	// The replacement is itself correctable.
	_, err = f.investmentService.ReverseDividend(ctx, ReverseInvestmentDividendInput{
		OwnerUserID: f.ownerUserID, TransactionID: result.Replacement.ID, Reason: "paid in error"})
	require.NoError(t, err)
	require.Zero(t, accountBalance(t, f, f.cashAccountID))
	require.Zero(t, accountBalance(t, f, f.incomeAccountID))
	requireInvestmentSelfCheckPasses(t, f)
}

func TestReverseDividendRequiresReconciliationOverride(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	original, err := f.investmentService.Dividend(ctx, cashDividendInput(f, 0, 10000, 0))
	require.NoError(t, err)
	checkpointID := reconcileCash(t, f, "2026-04-01", 100)
	input := ReverseInvestmentDividendInput{OwnerUserID: f.ownerUserID, TransactionID: original.ID, Reason: "duplicate entry"}
	impact, err := f.investmentService.ReverseDividendReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, impact.AffectedCheckpoints, 1)
	require.Equal(t, checkpointID, impact.AffectedCheckpoints[0].CheckpointID)
	_, err = f.investmentService.ReverseDividend(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	require.Equal(t, []int64{checkpointID}, activeCheckpointIDs(t, f))

	input.ReconciliationOverride = true
	reversal, err := f.investmentService.ReverseDividend(ctx, input)
	require.NoError(t, err)
	require.Equal(t, original.ID, *reversal.CorrectionOfTransactionID)
	require.Empty(t, activeCheckpointIDs(t, f))
	require.Zero(t, accountBalance(t, f, f.cashAccountID))
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, original.ID)
	require.NoError(t, err)
	require.Nil(t, chain.EffectiveTransactionID)
	requireInvestmentSelfCheckPasses(t, f)
}

// A correction prepared against a dividend that another command corrected
// in between is refused inside the write transaction, not double-applied.
func TestDividendCorrectionRefusesStaleSource(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	original, err := f.investmentService.Dividend(ctx, cashDividendInput(f, 0, 10000, 0))
	require.NoError(t, err)
	reverse := ReverseInvestmentDividendInput{OwnerUserID: f.ownerUserID, TransactionID: original.ID, Reason: "duplicate"}
	operation, params, err := f.investmentService.prepareDividendReversalWrite(ctx, reverse)
	require.NoError(t, err)
	_, err = f.investmentService.ReplaceDividend(ctx, ReplaceInvestmentDividendInput{OwnerUserID: f.ownerUserID,
		TransactionID: original.ID, Reason: "wrong amount", Replacement: cashDividendInput(f, 0, 9000, 0)})
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.investmentService.repository.ReverseDividend(ctx, params, operation)
	require.ErrorIs(t, mapDividendCorrectionError(err), ErrInvestmentDividendAlreadyCorrected)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	require.Equal(t, int64(9000), accountBalance(t, f, f.cashAccountID))
}

func TestDividendReplacementRollsBackLateFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	original, err := f.investmentService.Dividend(ctx, cashDividendInput(f, 0, 10000, 0))
	require.NoError(t, err)
	reconcileCash(t, f, "2026-04-01", 100)
	input := ReplaceInvestmentDividendInput{OwnerUserID: f.ownerUserID, TransactionID: original.ID,
		Reason: "wrong amount", ReconciliationOverride: true, Replacement: cashDividendInput(f, 0, 11000, 0)}
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.database.Exec(`CREATE TRIGGER reject_dividend_checkpoint AFTER UPDATE ON reconciliation_checkpoints
		WHEN NEW.status = 'invalidated' BEGIN SELECT RAISE(ABORT, 'forced checkpoint failure'); END`)
	require.NoError(t, err)
	_, err = f.investmentService.ReplaceDividend(ctx, input)
	require.ErrorContains(t, err, "forced checkpoint failure")
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	_, err = f.database.Exec(`DROP TRIGGER reject_dividend_checkpoint`)
	require.NoError(t, err)
	_, err = f.investmentService.ReplaceDividend(ctx, input)
	require.NoError(t, err)
	require.Equal(t, int64(11000), accountBalance(t, f, f.cashAccountID))
	requireInvestmentSelfCheckPasses(t, f)
}

func TestDividendCorrectionsStayInTheirFamily(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	dividend, err := f.investmentService.Dividend(ctx, cashDividendInput(f, 0, 10000, 0))
	require.NoError(t, err)
	buy := buyOn(t, f, "2026-01-01", 10, 10000)
	reinvestment := reinvestOn(t, f, "2026-02-01", 1, 500)
	for _, id := range []int64{buy.Transaction.ID, reinvestment.Transaction.ID} {
		_, err = f.investmentService.ReverseDividendReconciliationImpact(ctx, ReverseInvestmentDividendInput{
			OwnerUserID: f.ownerUserID, TransactionID: id, Reason: "wrong command"})
		require.ErrorIs(t, err, ErrInvestmentDividendNotFound)
	}
	_, err = f.investmentService.ReverseReinvestedDividendReconciliationImpact(ctx, ReverseReinvestedDividendInput{
		OwnerUserID: f.ownerUserID, TransactionID: dividend.ID, Reason: "wrong command"})
	require.ErrorIs(t, err, ErrInvestmentReinvestmentNotFound)
	other := cashDividendInput(f, 0, 10000, 0)
	other.TransactionDate = "2026-03-02"
	_, err = f.investmentService.ReplaceDividendReconciliationImpact(ctx, ReplaceInvestmentDividendInput{
		OwnerUserID: f.ownerUserID, TransactionID: dividend.ID, Reason: "moved", Replacement: other})
	var validation ValidationError
	require.ErrorAs(t, err, &validation, "date changes are a separate correction family")
	_, err = f.investmentService.ReverseDividendReconciliationImpact(ctx, ReverseInvestmentDividendInput{
		OwnerUserID: f.ownerUserID, TransactionID: dividend.ID})
	require.ErrorAs(t, err, &validation, "a reason is required")
}

// A native correction of an imported dividend keeps the committed import
// identity on the original, so a re-fetch of the same dividend still dedupes.
func TestImportedDividendCorrectionPreservesCommittedIdentity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	buyBatch, _ := f.stageOrderFillRow(t, conn.ID, trading212OrderFill{
		FillType: "TRADE", FillID: "fill-1", OrderID: "order-1", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Side: "BUY", Quantity: "2", Price: "150.00", Currency: "EUR",
		FilledAt: "2026-06-01T10:00:00Z", NetValue: "-300.00", NetValueCurrency: "EUR",
	})
	_, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: buyBatch})
	require.NoError(t, err)
	instruments, err := f.investmentSvc.ListInstruments(ctx)
	require.NoError(t, err)
	commodityID := instruments[0].CommodityID
	_, err = f.investmentSvc.SaveDividendDefault(ctx, DividendDefaultInput{OwnerUserID: f.ownerUserID,
		CommodityID: &commodityID, IncomeAccountID: f.categoryAccountID, EffectiveFrom: "2026-01-01", ChangeReason: "fixture"})
	require.NoError(t, err)
	dividend := trading212Dividend{Reference: "div-1", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Quantity: "2", Amount: "4.32", Currency: "EUR", PaidOn: "2026-06-15T00:00:00Z", Type: "DIVIDEND"}
	divBatch, divRow := f.stageDividendRow(t, conn.ID, dividend)
	_, err = f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: divBatch})
	require.NoError(t, err)
	row, err := f.importRepo.ImportStagedRowByID(ctx, divRow)
	require.NoError(t, err)
	originalID := row.CommittedTransactionID.Int64

	replacement := DividendInput{TransactionDate: "2026-06-15", CommodityID: &commodityID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, IncomeAccountID: &f.categoryAccountID,
		AmountValue: 523, AmountScale: 2}
	result, err := f.investmentSvc.ReplaceDividend(ctx, ReplaceInvestmentDividendInput{OwnerUserID: f.ownerUserID,
		TransactionID: originalID, Reason: "broker statement shows 5.23", Replacement: replacement})
	require.NoError(t, err)

	var identityTransaction int64
	require.NoError(t, f.database.QueryRow(`SELECT effect.transaction_id FROM import_commit_identity_effects effect
		JOIN investment_operations o ON o.id = effect.operation_id WHERE o.operation_kind = 'dividend'
		AND o.correction_of_operation_id IS NULL`).Scan(&identityTransaction))
	require.Equal(t, originalID, identityTransaction, "the identity stays on the original dividend")
	before := transactionCount(t, f)
	again, againRow := f.stageDividendRow(t, conn.ID, dividend)
	row, err = f.importRepo.ImportStagedRowByID(ctx, againRow)
	require.NoError(t, err)
	require.Equal(t, "duplicate", row.DedupeStatus)
	_, err = f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: again})
	require.NoError(t, err)
	require.Equal(t, before, transactionCount(t, f), "a re-fetched dividend never posts again")
	_, err = f.investmentSvc.ReverseDividend(ctx, ReverseInvestmentDividendInput{OwnerUserID: f.ownerUserID,
		TransactionID: result.Replacement.ID, Reason: "the broker withdrew the payment"})
	require.NoError(t, err)
}

// The correction chain offers native correction with the effective
// dividend's posted terms, and stops offering it once the chain is reversed.
func TestCorrectionChainOffersDividendTerms(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	withholding := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	original, err := f.investmentService.Dividend(ctx, cashDividendInput(f, withholding, 10000, 1500))
	require.NoError(t, err)
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, original.ID)
	require.NoError(t, err)
	require.True(t, chain.CanCorrectDividend)
	require.False(t, chain.CanCorrectReinvestedDividend)
	terms := chain.EffectiveDividend
	require.NotNil(t, terms)
	require.Equal(t, InvestmentCorrectionDividendTerms{EventDate: "2026-03-01", CashAccountID: f.cashAccountID,
		CashCommodityID: f.eurCommodityID, IncomeAccountID: f.incomeAccountID, AmountValue: exact.New(10000), AmountScale: 2,
		WithholdingAccountID: &withholding, WithholdingValue: terms.WithholdingValue, WithholdingScale: terms.WithholdingScale,
		Memo: "ACME dividend"}, *terms)
	require.Equal(t, exact.New(1500), *terms.WithholdingValue)
	require.Equal(t, 2, *terms.WithholdingScale)

	_, err = f.investmentService.ReverseDividend(ctx, ReverseInvestmentDividendInput{OwnerUserID: f.ownerUserID,
		TransactionID: original.ID, Reason: "duplicate"})
	require.NoError(t, err)
	chain, err = f.investmentService.CorrectionChain(ctx, f.ownerUserID, original.ID)
	require.NoError(t, err)
	require.False(t, chain.CanCorrectDividend)
	require.Nil(t, chain.EffectiveDividend)

	reinvestment := reinvestOn(t, f, "2026-01-01", 3, 600)
	chain, err = f.investmentService.CorrectionChain(ctx, f.ownerUserID, reinvestment.Transaction.ID)
	require.NoError(t, err)
	require.True(t, chain.CanCorrectReinvestedDividend)
	require.Equal(t, InvestmentCorrectionReinvestmentTerms{EventDate: "2026-01-01", HoldingAccountID: f.holdingAccountID,
		CommodityID: f.stockCommodityID, CashCommodityID: f.eurCommodityID, IncomeAccountID: f.incomeAccountID,
		QuantityValue: exact.New(3), QuantityScale: 0, AmountValue: exact.New(600), AmountScale: 2}, *chain.EffectiveReinvestment)
}
