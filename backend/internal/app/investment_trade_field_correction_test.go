package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// T-116: a buy or sale replacement may change its trade date, holding
// account, instrument or cost currency. The inverse keeps every original
// journal date; the replacement posts at the corrected dates; the source and
// the new position replay in the same transaction or nothing is written.

func seedTestSecurityCommodity(t *testing.T, f *investmentsTestFixture, code string) int64 {
	t.Helper()
	result, err := f.database.Exec(`INSERT INTO commodities (book_id, code, kind, is_builtin, created_at, created_by_user_id)
		VALUES (?, ?, 'security', 0, '2026-01-01T00:00:00Z', 1)`, BookID, code)
	require.NoError(t, err)
	commodityID, err := result.LastInsertId()
	require.NoError(t, err)
	_, err = f.database.Exec(`INSERT INTO commodity_versions (
			commodity_id, version_seq, effective_from, recorded_at, changed_by_user_id,
			change_reason, status, symbol, display_symbol, name, standard_scale, max_quantity_scale
		) VALUES (?, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1, 'test fixture', 'active', ?, ?, ?, 0, 6)`,
		commodityID, code, code, code)
	require.NoError(t, err)
	return commodityID
}

func tradeOn(f *investmentsTestFixture, account int64, date string, quantity, cashValue int64) InvestmentTradeInput {
	return InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: date, CommodityID: f.stockCommodityID,
		HoldingAccountID: account, CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(quantity), CashAmountValue: cashValue, CashAmountScale: 2,
	}
}

func journalDates(t *testing.T, f *investmentsTestFixture, transactionID int64) []string {
	t.Helper()
	transaction, err := f.transactionService.Transaction(context.Background(), transactionID)
	require.NoError(t, err)
	dates := make([]string, 0, len(transaction.JournalEntries))
	for _, entry := range transaction.JournalEntries {
		dates = append(dates, entry.EntryDate)
	}
	return dates
}

func positionQuantity(t *testing.T, f *investmentsTestFixture, account, commodity int64) string {
	t.Helper()
	lots, err := f.investmentService.ListLots(context.Background(), account, commodity)
	require.NoError(t, err)
	total := exact.ScaledIntFromInt64(0, 0)
	for _, lot := range lots {
		if lot.Status == "open" {
			total.AddCoefficient(lot.RemainingQuantityValue, lot.RemainingQuantityScale)
		}
	}
	value, err := total.Coefficient()
	require.NoError(t, err)
	return value.String()
}

// gainChangeFor returns the disclosed change for one dependent transaction.
func gainChangeFor(t *testing.T, impact ReconciliationImpact, transactionID int64) db.InvestmentGainChange {
	t.Helper()
	require.NotNil(t, impact.GainImpact)
	for _, change := range impact.GainImpact.Changes {
		if change.TransactionID == transactionID {
			return change
		}
	}
	t.Fatalf("no gain change for transaction %d in %+v", transactionID, impact.GainImpact.Changes)
	return db.InvestmentGainChange{}
}

func correctionCount(t *testing.T, f *investmentsTestFixture) int {
	t.Helper()
	var count int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_operations
		WHERE correction_of_operation_id IS NOT NULL`).Scan(&count))
	return count
}

// Moving a later buy before an earlier one changes which lot FIFO consumes
// for a later sale. The preview discloses that sale's restated gain, the
// inverse stays on the original date and the replacement on the new one.
func TestReplaceBuyMovesTradeDateEarlierAndRestatesLaterSale(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	_, err := f.investmentService.Buy(ctx, tradeOn(f, f.holdingAccountID, "2026-02-01", 10, 100000))
	require.NoError(t, err)
	late, err := f.investmentService.Buy(ctx, tradeOn(f, f.holdingAccountID, "2026-03-01", 10, 300000))
	require.NoError(t, err)
	sale := tradeOn(f, f.holdingAccountID, "2026-04-01", 5, 250000)
	sale.CostBasisMethod = "fifo"
	sold, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)

	input := ReplaceInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: late.Transaction.ID,
		Reason: "broker executed in January", Replacement: tradeOn(f, f.holdingAccountID, "2026-01-15", 10, 300000)}
	impact, err := f.investmentService.ReplaceBuyReconciliationImpact(ctx, input)
	require.NoError(t, err)
	change := gainChangeFor(t, impact, sold.Transaction.ID)
	require.Equal(t, db.GainImpactRevised, change.Kind)

	_, err = f.investmentService.ReplaceBuy(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	require.Zero(t, correctionCount(t, f))

	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	result, err := f.investmentService.ReplaceBuy(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-03-01"}, journalDates(t, f, result.Inverse.ID))
	assert.Equal(t, []string{"2026-01-15"}, journalDates(t, f, result.Replacement.Transaction.ID))
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Len(t, gains, 1)
	// FIFO now sells five of the January lot at 300 each: basis 1500.
	assert.Zero(t, exact.ScaledIntFromInt64(gains[0].DisposedBasisValue, gains[0].DisposedBasisScale).
		Cmp(exact.ScaledIntFromInt64(-150000, 2)))
	assert.Equal(t, "15", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	requireSelfCheckPasses(t, f)
}

// Moving a buy after the sale that consumed it leaves the sale without
// shares on its date: the sale is named and nothing is written.
func TestReplaceBuyDateAfterDependentSaleNamesSale(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	original, err := f.investmentService.Buy(ctx, tradeOn(f, f.holdingAccountID, "2026-01-01", 10, 100000))
	require.NoError(t, err)
	sold, err := f.investmentService.Sell(ctx, tradeOn(f, f.holdingAccountID, "2026-02-01", 5, 60000))
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)

	_, err = acknowledgedReplaceBuy(ctx, f.investmentService, ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID, Reason: "settled in March",
		Replacement: tradeOn(f, f.holdingAccountID, "2026-03-01", 10, 100000)})
	require.ErrorIs(t, err, ErrInvestmentBuyDependency)
	require.ErrorContains(t, err, fmt.Sprint(saleOperationIDForTest(t, f, sold.Transaction.ID)))
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	assert.Zero(t, correctionCount(t, f))
}

// A buy booked to the wrong holding account moves: the source position's
// FIFO sale now consumes the remaining lot there, the new account holds the
// shares, and both positions replay under one audit event.
func TestReplaceBuyMovesHoldingAccountAndReplaysBothPositions(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	other := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	wrong, err := f.investmentService.Buy(ctx, tradeOn(f, f.holdingAccountID, "2026-01-01", 10, 100000))
	require.NoError(t, err)
	_, err = f.investmentService.Buy(ctx, tradeOn(f, f.holdingAccountID, "2026-02-01", 10, 200000))
	require.NoError(t, err)
	sale := tradeOn(f, f.holdingAccountID, "2026-03-01", 5, 150000)
	sale.CostBasisMethod = "fifo"
	sold, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)

	input := ReplaceInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: wrong.Transaction.ID,
		Reason: "booked to the wrong broker", Replacement: tradeOn(f, other, "2026-01-01", 10, 100000)}
	impact, err := f.investmentService.ReplaceBuyReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Equal(t, db.GainImpactRevised, gainChangeFor(t, impact, sold.Transaction.ID).Kind)
	result, err := acknowledgedReplaceBuy(ctx, f.investmentService, input)
	require.NoError(t, err)

	assert.Equal(t, "5", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	assert.Equal(t, "10", positionQuantity(t, f, other, f.stockCommodityID))
	var lotAccount int64
	require.NoError(t, f.database.QueryRow(`SELECT account_id FROM investment_lots WHERE id = ?`,
		*result.Replacement.LotID).Scan(&lotAccount))
	assert.Equal(t, other, lotAccount)
	var audits int
	require.NoError(t, f.database.QueryRow(`SELECT count(DISTINCT created_audit_event_id) FROM investment_disposal_revisions`).Scan(&audits))
	assert.Equal(t, 1, audits)
	requireSelfCheckPasses(t, f)
}

// Instrument and cost currency both change: the source position empties and
// the replacement opens a lot in the new instrument at its new cost currency.
func TestReplaceBuyChangesInstrumentAndCostCurrency(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	usd := seedTestCurrencyCommodity(t, f.database, "USD")
	corrected := seedTestSecurityCommodity(t, f, "RIGHT")
	original, err := f.investmentService.Buy(ctx, tradeOn(f, f.holdingAccountID, "2026-01-01", 10, 100000))
	require.NoError(t, err)

	replacement := tradeOn(f, f.holdingAccountID, "2026-01-01", 10, 110000)
	replacement.CommodityID, replacement.CashCommodityID = corrected, usd
	result, err := acknowledgedReplaceBuy(ctx, f.investmentService, ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID,
		Reason: "wrong ticker and currency", Replacement: replacement})
	require.NoError(t, err)

	assert.Equal(t, "0", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	assert.Equal(t, "10", positionQuantity(t, f, f.holdingAccountID, corrected))
	var lotCommodity, lotCost int64
	require.NoError(t, f.database.QueryRow(`SELECT commodity_id, cost_commodity_id FROM investment_lots WHERE id = ?`,
		*result.Replacement.LotID).Scan(&lotCommodity, &lotCost))
	assert.Equal(t, corrected, lotCommodity)
	assert.Equal(t, usd, lotCost)
	requireSelfCheckPasses(t, f)
}

// A later sale elected the acquisition by specific lot. Moving that
// acquisition to another account leaves the election without its lot, so the
// sale is named and nothing is written.
func TestReplaceBuyRefusesInvalidatedSpecificLotElection(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	other := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	elected, err := f.investmentService.Buy(ctx, tradeOn(f, f.holdingAccountID, "2026-01-01", 10, 100000))
	require.NoError(t, err)
	_, err = f.investmentService.Buy(ctx, tradeOn(f, f.holdingAccountID, "2026-02-01", 10, 200000))
	require.NoError(t, err)
	sale := tradeOn(f, f.holdingAccountID, "2026-03-01", 4, 120000)
	sale.CostBasisMethod = "specific_lot"
	sale.LotAllocations = []InvestmentLotAllocationInput{{LotID: *elected.LotID, QuantityValue: exact.New(4)}}
	sold, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)

	input := ReplaceInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: elected.Transaction.ID,
		Reason: "wrong broker", Replacement: tradeOn(f, other, "2026-01-01", 10, 100000)}
	_, err = f.investmentService.ReplaceBuyReconciliationImpact(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentBuyDependency)
	_, err = acknowledgedReplaceBuy(ctx, f.investmentService, input)
	var dependency InvestmentBuyDependencyError
	require.ErrorAs(t, err, &dependency)
	assert.Equal(t, saleOperationIDForTest(t, f, sold.Transaction.ID), dependency.OperationID)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

// A transferred acquisition keeps its date, holding and instrument: the
// transfer link's lot and original date order the destination, so moving the
// acquisition names the transfer (pooled lineage is T-135).
func TestReplaceTransferredBuyFieldChangeNamesTransfer(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	destination := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	other := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	transfer := chainTransfer(t, f, f.holdingAccountID, destination, *buy.LotID, "2026-06-01")
	transferOperation := transferOperationID(t, f, transfer.Transaction.ID)
	before := buyReplacementPreviewSnapshot(t, f.database)
	for name, replacement := range map[string]InvestmentTradeInput{
		"date":    tradeOn(f, f.holdingAccountID, "2026-04-01", 3, 3000),
		"account": tradeOn(f, other, "2026-05-01", 3, 3000),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := acknowledgedReplaceBuy(ctx, f.investmentService, ReplaceInvestmentBuyInput{
				OwnerUserID: f.ownerUserID, TransactionID: buy.Transaction.ID, Reason: "move it", Replacement: replacement})
			var dependency InvestmentBuyDependencyError
			require.ErrorAs(t, err, &dependency)
			assert.Equal(t, transferOperation, dependency.OperationID)
			assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
		})
	}
}

// A buy that settles later and paid a commission on yet another day spans
// three journal dates. The inverse keeps all three; the replacement posts its
// moved trade and settlement dates and the charge's own payment date.
func TestReplaceBuyDateKeepsMultiJournalFeeDates(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	fees := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	withFee := func(date, settles string) InvestmentTradeInput {
		trade := tradeOn(f, f.holdingAccountID, date, 10, 0)
		trade.SettlementDate = settles
		trade.GrossAmountValue, trade.GrossAmountScale = tradeMoney(-100000), 2
		trade.NetSettlementValue, trade.NetSettlementScale = tradeMoney(-100000), 2
		trade.Charges = []InvestmentTradeChargeInput{{Kind: "commission", AmountValue: -250, AmountScale: 2,
			CommodityID: f.eurCommodityID, Treatment: "separately_expensed", ChargeAccountID: &fees,
			CashAccountID: &f.cashAccountID, PaidOn: "2026-03-10"}}
		return trade
	}
	original, err := f.investmentService.Buy(ctx, withFee("2026-03-01", "2026-03-03"))
	require.NoError(t, err)

	result, err := acknowledgedReplaceBuy(ctx, f.investmentService, ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID, Reason: "executed in February",
		Replacement: withFee("2026-02-01", "2026-02-03")})
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-03-01", "2026-03-03", "2026-03-10"}, journalDates(t, f, result.Inverse.ID))
	assert.Equal(t, []string{"2026-02-01", "2026-02-03", "2026-03-10"}, journalDates(t, f, result.Replacement.Transaction.ID))
	requireSelfCheckPasses(t, f)
}

// The checkpoint covers the original date's cash but not the corrected one,
// then the reverse: either crossing needs the explicit override, the preview
// names the checkpoint, and a refusal writes nothing.
func TestReplaceBuyDateCrossingCheckpointRequiresOverride(t *testing.T) {
	for name, dates := range map[string][2]string{
		"into reconciled period":   {"2026-04-01", "2026-02-01"},
		"out of reconciled period": {"2026-02-01", "2026-04-01"},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			f := newInvestmentsTestFixture(t)
			_, err := f.investmentService.Buy(ctx, tradeOn(f, f.holdingAccountID, "2026-01-01", 1, 10000))
			require.NoError(t, err)
			var original InvestmentTradeResult
			var checkpointID int64
			if dates[0] < "2026-03-01" {
				original, err = f.investmentService.Buy(ctx, tradeOn(f, f.holdingAccountID, dates[0], 10, 100000))
				require.NoError(t, err)
				checkpointID = reconcileCash(t, f, "2026-03-01", -1100)
			} else {
				checkpointID = reconcileCash(t, f, "2026-03-01", -100)
				original, err = f.investmentService.Buy(ctx, tradeOn(f, f.holdingAccountID, dates[0], 10, 100000))
				require.NoError(t, err)
			}
			input := ReplaceInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID,
				Reason: "trade date was wrong", Replacement: tradeOn(f, f.holdingAccountID, dates[1], 10, 100000)}
			impact, err := f.investmentService.ReplaceBuyReconciliationImpact(ctx, input)
			require.NoError(t, err)
			require.Len(t, impact.AffectedCheckpoints, 1)
			assert.Equal(t, checkpointID, impact.AffectedCheckpoints[0].CheckpointID)
			_, err = acknowledgedReplaceBuy(ctx, f.investmentService, input)
			require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
			assert.Zero(t, correctionCount(t, f))
			assert.Equal(t, []int64{checkpointID}, activeCheckpointIDs(t, f))
			input.ReconciliationOverride = true
			_, err = acknowledgedReplaceBuy(ctx, f.investmentService, input)
			require.NoError(t, err)
			assert.Empty(t, activeCheckpointIDs(t, f))
			requireSelfCheckPasses(t, f)
		})
	}
}

// A late failure after the second position replays — here the source
// acceptance hook — rolls back the inverse, the replacement, both positions'
// replay revisions and the retired price.
func TestReplaceBuyAcrossPositionsRollsBackLateFailure(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	other := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	wrong, err := f.investmentService.Buy(ctx, tradeOn(f, f.holdingAccountID, "2026-01-01", 10, 100000))
	require.NoError(t, err)
	_, err = f.investmentService.Buy(ctx, tradeOn(f, f.holdingAccountID, "2026-02-01", 10, 200000))
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, tradeOn(f, f.holdingAccountID, "2026-03-01", 5, 150000))
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)
	injected := errors.New("injected after both positions replayed")

	input := ReplaceInvestmentBuyInput{OwnerUserID: f.ownerUserID, TransactionID: wrong.Transaction.ID,
		Reason: "wrong broker", Replacement: tradeOn(f, other, "2026-01-01", 10, 100000)}
	if impact, err := f.investmentService.ReplaceBuyReconciliationImpact(ctx, input); err == nil && impact.GainImpact != nil {
		input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	}
	_, err = f.investmentService.replaceBuyWithPostWrite(ctx, input, func(tx *sql.Tx, _, _ int64) error {
		var lots int
		require.NoError(t, tx.QueryRow(`SELECT count(*) FROM investment_lots WHERE account_id = ?`, other).Scan(&lots))
		require.Equal(t, 1, lots, "the new position has replayed when the hook runs")
		return injected
	})
	require.ErrorIs(t, err, injected)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	assert.Equal(t, "15", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	assert.Equal(t, "0", positionQuantity(t, f, other, f.stockCommodityID))
	requireSelfCheckPasses(t, f)
}

// A sale moves to its correct holding account and an earlier date. The
// source position gets its shares back, the new position's later sale is
// restated, and the inverse keeps the original date.
func TestReplaceSaleMovesAccountAndDateReplaysBothPositions(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	other := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	_, err := f.investmentService.Buy(ctx, tradeOn(f, f.holdingAccountID, "2026-01-01", 10, 100000))
	require.NoError(t, err)
	_, err = f.investmentService.Buy(ctx, tradeOn(f, other, "2026-01-01", 5, 50000))
	require.NoError(t, err)
	_, err = f.investmentService.Buy(ctx, tradeOn(f, other, "2026-02-01", 5, 100000))
	require.NoError(t, err)
	wrong := tradeOn(f, f.holdingAccountID, "2026-05-01", 4, 80000)
	wrong.CostBasisMethod = "fifo"
	misbooked, err := f.investmentService.Sell(ctx, wrong)
	require.NoError(t, err)
	later := tradeOn(f, other, "2026-04-01", 3, 60000)
	later.CostBasisMethod = "fifo"
	dependent, err := f.investmentService.Sell(ctx, later)
	require.NoError(t, err)

	corrected := tradeOn(f, other, "2026-03-01", 4, 80000)
	corrected.CostBasisMethod = "fifo"
	input := ReplaceInvestmentSaleInput{OwnerUserID: f.ownerUserID, TransactionID: misbooked.Transaction.ID,
		Reason: "sold from the other broker in March", Replacement: corrected}
	impact, err := f.investmentService.ReplaceSaleReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Equal(t, db.GainImpactRevised, gainChangeFor(t, impact, dependent.Transaction.ID).Kind)
	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	result, err := f.investmentService.ReplaceSale(ctx, input)
	require.NoError(t, err)

	assert.Equal(t, []string{"2026-05-01"}, journalDates(t, f, result.Inverse.ID))
	assert.Equal(t, []string{"2026-03-01"}, journalDates(t, f, result.Replacement.Transaction.ID))
	assert.Equal(t, "10", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	assert.Equal(t, "3", positionQuantity(t, f, other, f.stockCommodityID))
	// The corrected sale takes the four oldest January shares at 100 each.
	require.NotNil(t, result.Replacement.DisposalDecision)
	basis := exact.ScaledIntFromInt64(0, 0)
	for _, allocation := range result.Replacement.Allocations {
		basis.AddScaled(exact.ScaledIntFromInt64(allocation.CostBasisValue, allocation.CostBasisScale))
	}
	assert.Zero(t, basis.Cmp(exact.ScaledIntFromInt64(40000, 2)))
	requireSelfCheckPasses(t, f)
}

// A sale moved to a position that cannot cover it on the new date is refused
// as itself, not as a dependency, and nothing is written.
func TestReplaceSaleIntoPositionWithoutSharesIsRefused(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	other := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	_, err := f.investmentService.Buy(ctx, tradeOn(f, f.holdingAccountID, "2026-01-01", 10, 100000))
	require.NoError(t, err)
	_, err = f.investmentService.Buy(ctx, tradeOn(f, other, "2026-04-01", 10, 100000))
	require.NoError(t, err)
	sale := tradeOn(f, f.holdingAccountID, "2026-05-01", 4, 80000)
	sale.CostBasisMethod = "fifo"
	sold, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)

	corrected := tradeOn(f, other, "2026-03-01", 4, 80000)
	corrected.CostBasisMethod = "fifo"
	_, err = acknowledgedReplaceSale(ctx, f.investmentService, ReplaceInvestmentSaleInput{OwnerUserID: f.ownerUserID,
		TransactionID: sold.Transaction.ID, Reason: "other broker", Replacement: corrected})
	require.ErrorIs(t, err, ErrInvestmentLotsInsufficient)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

// A moved sale keeps its original entry-order slot on the new date. It was
// entered before the other sale now on the same day, so it still allocates
// first: FIFO gives it January's lot, not February's.
func TestReplaceSaleDateKeepsOriginalSameDaySlot(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	_, err := f.investmentService.Buy(ctx, tradeOn(f, f.holdingAccountID, "2026-01-01", 5, 50000))
	require.NoError(t, err)
	_, err = f.investmentService.Buy(ctx, tradeOn(f, f.holdingAccountID, "2026-02-01", 5, 100000))
	require.NoError(t, err)
	moving := tradeOn(f, f.holdingAccountID, "2026-04-01", 5, 100000)
	moving.CostBasisMethod = "fifo"
	sold, err := f.investmentService.Sell(ctx, moving)
	require.NoError(t, err)
	// Entered later but dated earlier: a backdated sale behind the April one.
	other := tradeOn(f, f.holdingAccountID, "2026-03-01", 5, 100000)
	other.CostBasisMethod = "fifo"
	otherImpact, err := f.investmentService.TradeReconciliationImpact(ctx, InvestmentImpactSell, other)
	require.NoError(t, err)
	other.GainImpactAcknowledgement = otherImpact.GainImpact.Acknowledgement
	otherSale, err := f.investmentService.Sell(ctx, other)
	require.NoError(t, err)

	corrected := tradeOn(f, f.holdingAccountID, "2026-03-01", 5, 100000)
	corrected.CostBasisMethod = "fifo"
	input := ReplaceInvestmentSaleInput{OwnerUserID: f.ownerUserID,
		TransactionID: sold.Transaction.ID, Reason: "same day as the other sale", Replacement: corrected}
	impact, err := f.investmentService.ReplaceSaleReconciliationImpact(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, db.GainImpactRevised, gainChangeFor(t, impact, otherSale.Transaction.ID).Kind)
	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	result, err := f.investmentService.ReplaceSale(ctx, input)
	require.NoError(t, err)
	basis := exact.ScaledIntFromInt64(0, 0)
	for _, allocation := range result.Replacement.Allocations {
		basis.AddScaled(exact.ScaledIntFromInt64(allocation.CostBasisValue, allocation.CostBasisScale))
	}
	assert.Zero(t, basis.Cmp(exact.ScaledIntFromInt64(50000, 2)))
	requireSelfCheckPasses(t, f)
}

// An imported buy moved to another date keeps its committed source identity
// on the original operation: a re-fetch of the same fill stays a duplicate,
// and a later provider quantity revision no longer matches the effective
// trade, so it is refused for review instead of moving the trade back.
func TestReplaceImportedBuyDatePreservesDedupeIdentity(t *testing.T) {
	ctx := context.Background()
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	fill := trading212OrderFill{
		FillType: "TRADE", FillID: "moved-fill", OrderID: "moved-order", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
		Side: "BUY", Quantity: "2", Price: "150.00", Currency: "EUR",
		FilledAt: "2026-06-01T10:00:00Z", NetValue: "-300.00", NetValueCurrency: "EUR",
	}
	batch, _ := f.stageOrderFillRow(t, conn.ID, fill)
	_, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batch})
	require.NoError(t, err)
	rows, err := f.importRepo.ListAllImportStagedRows(ctx, batch)
	require.NoError(t, err)
	originalID := rows[0].CommittedTransactionID.Int64
	source, err := f.investmentSvc.TradeCorrectionContext(ctx, f.ownerUserID, originalID)
	require.NoError(t, err)

	result, err := acknowledgedReplaceBuy(ctx, f.investmentSvc, ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: originalID, Reason: "executed the evening before",
		Replacement: InvestmentTradeInput{TransactionDate: "2026-05-31", CommodityID: source.CommodityID,
			HoldingAccountID: source.HoldingAccountID, CashAccountID: source.CashAccountID,
			CashCommodityID: source.CostCommodityID, QuantityValue: exact.New(2),
			CashAmountValue: 30000, CashAmountScale: 2}})
	require.NoError(t, err)
	var identityTransaction int64
	require.NoError(t, f.database.QueryRow(`SELECT transaction_id FROM import_commit_identity_effects
		WHERE identity_id = ?`, rows[0].CommittedIdentityID.Int64).Scan(&identityTransaction))
	assert.Equal(t, originalID, identityTransaction, "the identity stays on the original buy")

	before := transactionCount(t, f)
	again, _ := f.stageOrderFillRow(t, conn.ID, fill)
	againRows, err := f.importRepo.ListAllImportStagedRows(ctx, again)
	require.NoError(t, err)
	assert.Equal(t, "duplicate", againRows[0].DedupeStatus)
	_, err = f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: again})
	require.NoError(t, err)
	assert.Equal(t, before, transactionCount(t, f), "a re-fetched fill never posts again")

	revised := fill
	revised.Quantity, revised.NetValue = "3", "-450.00"
	revisedBatch, revisedRow := f.stageOrderFillRow(t, conn.ID, revised)
	_, err = acknowledgedCorrectTrading212Buy(ctx, f.importService, CorrectTrading212BuyInput{
		OwnerUserID: f.ownerUserID, BatchID: revisedBatch, RowID: revisedRow, Reason: "broker revised fill"})
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
	chain, err := f.investmentSvc.CorrectionChain(ctx, f.ownerUserID, originalID)
	require.NoError(t, err)
	assert.Equal(t, result.Replacement.Transaction.ID, *chain.EffectiveTransactionID)
}

// The new position's own transfers are dependencies too. Moving a buy into an
// average-cost position before its pooled transfer would change which lots
// the transfer depleted, so the transfer is named (pooled lineage, T-135);
// moving it in after the transfer leaves the transfer untouched and commits.
func TestReplaceBuyIntoPositionWithLaterPooledTransfer(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	wrong := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	destination := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	setHoldingCostBasisMethod(t, f, "average_cost")
	buyOn(t, f, "2026-02-01", 2, 2000)
	transfer, err := f.investmentService.InternalTransfer(ctx,
		pooledTransferInput(f, destination, "2026-03-01", exact.New(1), 0))
	require.NoError(t, err)
	misbooked, err := f.investmentService.Buy(ctx, tradeOn(f, wrong, "2026-01-01", 2, 4000))
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)

	_, err = acknowledgedReplaceBuy(ctx, f.investmentService, ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: misbooked.Transaction.ID, Reason: "other broker",
		Replacement: tradeOn(f, f.holdingAccountID, "2026-01-01", 2, 4000)})
	var dependency InvestmentBuyDependencyError
	require.ErrorAs(t, err, &dependency)
	assert.Equal(t, transferOperationID(t, f, transfer.Transaction.ID), dependency.OperationID)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))

	_, err = acknowledgedReplaceBuy(ctx, f.investmentService, ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: misbooked.Transaction.ID, Reason: "other broker, in April",
		Replacement: tradeOn(f, f.holdingAccountID, "2026-04-01", 2, 4000)})
	require.NoError(t, err)
	assert.Equal(t, "3", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	assert.Equal(t, "1", positionQuantity(t, f, destination, f.stockCommodityID))
	assert.Equal(t, "0", positionQuantity(t, f, wrong, f.stockCommodityID))
	requireSelfCheckPasses(t, f)
}
