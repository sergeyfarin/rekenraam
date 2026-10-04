package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func TestInvestmentOperationExportAndImportEffectsUsePrimaryJournalLinks(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	trade := InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(2), CashAmountValue: 20000, CashAmountScale: 2,
	}
	original, err := f.investmentService.Buy(ctx, trade)
	require.NoError(t, err)
	trade.CashAmountValue = 21000
	corrected, err := acknowledgedReplaceBuy(ctx, f.investmentService, ReplaceInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID,
		Reason: "correct source settlement", Replacement: trade,
	})
	require.NoError(t, err)
	exporter := NewExportService(db.NewExportRepository(f.database))
	var before bytes.Buffer
	snapshot, err := db.NewExportRepository(f.database).Snapshot(ctx)
	require.NoError(t, err)
	_, err = exporter.writeInvestmentOperationsCSV(ctx, &before, snapshot)
	require.NoError(t, err)
	require.NoError(t, snapshot.Rollback())
	requireInvestmentHeaderRetired(t, f)
	exported, err := csv.NewReader(bytes.NewReader(before.Bytes())).ReadAll()
	require.NoError(t, err)
	require.Len(t, exported, 3, "one row per operation even for a multi-journal replacement")
	require.Equal(t, strconv.FormatInt(original.Transaction.ID, 10), exported[1][1])
	require.Equal(t, strconv.FormatInt(corrected.Replacement.Transaction.ID, 10), exported[2][1])

	operation, err := f.investmentService.repository.BuyOperationByTransactionID(ctx, BookID, corrected.Replacement.Transaction.ID)
	require.NoError(t, err)
	repository := db.NewImportRepository(f.database)
	tx, err := f.database.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()
	identityID, err := repository.CreateCommitIdentityWithEffects(ctx, tx, db.CreateImportCommitIdentityParams{
		BookID: BookID, DedupeFingerprint: "primary-link-source", SourceKind: "trading212",
		AccountID: f.cashAccountID, CreatedAt: "2026-01-01T00:00:00Z",
	}, []db.CreateImportCommitEffectParams{
		{TransactionID: sql.NullInt64{Int64: corrected.Replacement.Transaction.ID, Valid: true}},
		{TransactionID: sql.NullInt64{Int64: corrected.Inverse.ID, Valid: true}},
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	effects, err := repository.ListCommitIdentityEffects(ctx, identityID)
	require.NoError(t, err)
	require.Len(t, effects, 2)
	require.Equal(t, operation.OperationID, effects[0].OperationID.Int64)
	require.True(t, effects[0].OperationID.Valid)
	require.False(t, effects[1].OperationID.Valid, "inverse journal must not be inferred as a primary source operation")
	require.Equal(t, corrected.Inverse.ID, effects[1].TransactionID.Int64)
}

func TestImportIdentityRefusesUnlinkedInvestmentJournal(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	bought, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(2), CashAmountValue: 20000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	_, err = f.database.ExecContext(ctx, "DROP TRIGGER investment_operation_journal_links_no_delete")
	require.NoError(t, err)
	_, err = f.database.ExecContext(ctx, "DELETE FROM investment_operation_journal_links WHERE transaction_version_id = ?", bought.Transaction.VersionID)
	require.NoError(t, err)
	repository := db.NewImportRepository(f.database)
	tx, err := f.database.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()
	_, err = repository.CreateCommitIdentityWithEffects(ctx, tx, db.CreateImportCommitIdentityParams{
		BookID: BookID, DedupeFingerprint: "missing-primary-link", SourceKind: "trading212",
		AccountID: f.cashAccountID, CreatedAt: "2026-01-01T00:00:00Z",
	}, []db.CreateImportCommitEffectParams{{TransactionID: sql.NullInt64{Int64: bought.Transaction.ID, Valid: true}}})
	require.Error(t, err, "an unlinked investment journal must not become a journal-only import effect")
	require.NoError(t, tx.Rollback())
	_, found, err := repository.FindCommitIdentity(ctx, BookID, "missing-primary-link")
	require.NoError(t, err)
	require.False(t, found)
}

// T-115: dividend and reinvestment corrections export one row per operation
// with their correction link, mode and reason, like trade corrections.
func TestDividendAndReinvestmentCorrectionsExportAsOperations(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	dividend, err := f.investmentService.Dividend(ctx, cashDividendInput(f, 0, 10000, 0))
	require.NoError(t, err)
	replaced, err := f.investmentService.ReplaceDividend(ctx, ReplaceInvestmentDividendInput{OwnerUserID: f.ownerUserID,
		TransactionID: dividend.ID, Reason: "wrong gross", Replacement: cashDividendInput(f, 0, 12000, 0)})
	require.NoError(t, err)
	reinvestment := reinvestOn(t, f, "2026-01-01", 2, 400)
	reversal, err := f.investmentService.ReverseReinvestedDividend(ctx, ReverseReinvestedDividendInput{
		OwnerUserID: f.ownerUserID, TransactionID: reinvestment.Transaction.ID, Reason: "paid as cash"})
	require.NoError(t, err)

	snapshot, err := db.NewExportRepository(f.database).Snapshot(ctx)
	require.NoError(t, err)
	var out bytes.Buffer
	_, err = NewExportService(db.NewExportRepository(f.database)).writeInvestmentOperationsCSV(ctx, &out, snapshot)
	require.NoError(t, err)
	require.NoError(t, snapshot.Rollback())
	rows, err := csv.NewReader(bytes.NewReader(out.Bytes())).ReadAll()
	require.NoError(t, err)
	byTransaction := make(map[string][]string)
	for _, row := range rows[1:] {
		byTransaction[row[1]] = row
	}
	require.Len(t, byTransaction, 4)
	original := byTransaction[strconv.FormatInt(dividend.ID, 10)]
	replacement := byTransaction[strconv.FormatInt(replaced.Replacement.ID, 10)]
	require.Equal(t, []string{"dividend", original[0], "replace", "wrong gross"},
		[]string{replacement[2], replacement[5], replacement[6], replacement[7]})
	reversed := byTransaction[strconv.FormatInt(reversal.ID, 10)]
	require.Equal(t, []string{"reversal", "reverse", "paid as cash"}, []string{reversed[2], reversed[6], reversed[7]})
	require.Equal(t, byTransaction[strconv.FormatInt(reinvestment.Transaction.ID, 10)][0], reversed[5])
}

// T-118: a replaced and a reversed write-off export as operations with their
// correction link, mode and reason.
func TestWriteOffCorrectionsExportAsOperations(t *testing.T) {
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-01-01", 10, 100000)
	first := writeOffOn(t, f, "2026-02-01", 4, "fifo")
	replaced, err := acknowledgedReplaceWriteOff(ctx, f.investmentService, ReplaceInvestmentWriteOffInput{
		OwnerUserID: f.ownerUserID, TransactionID: first.Transaction.ID, Reason: "three shares",
		Replacement: writeOffReplacement(f, "2026-02-01", 3, "fifo")})
	require.NoError(t, err)
	second := writeOffOn(t, f, "2026-03-01", 2, "fifo")
	reversal, err := acknowledgedReverseWriteOff(ctx, f.investmentService, ReverseInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: second.Transaction.ID, Reason: "not delisted"})
	require.NoError(t, err)

	snapshot, err := db.NewExportRepository(f.database).Snapshot(ctx)
	require.NoError(t, err)
	var out bytes.Buffer
	_, err = NewExportService(db.NewExportRepository(f.database)).writeInvestmentOperationsCSV(ctx, &out, snapshot)
	require.NoError(t, err)
	require.NoError(t, snapshot.Rollback())
	rows, err := csv.NewReader(bytes.NewReader(out.Bytes())).ReadAll()
	require.NoError(t, err)
	byTransaction := make(map[string][]string)
	for _, row := range rows[1:] {
		byTransaction[row[1]] = row
	}
	original := byTransaction[strconv.FormatInt(first.Transaction.ID, 10)]
	replacement := byTransaction[strconv.FormatInt(replaced.Replacement.Transaction.ID, 10)]
	require.Equal(t, []string{"write_off", original[0], "replace", "three shares"},
		[]string{replacement[2], replacement[5], replacement[6], replacement[7]})
	reversed := byTransaction[strconv.FormatInt(reversal.ID, 10)]
	require.Equal(t, []string{"reversal", "reverse", "not delisted"}, []string{reversed[2], reversed[6], reversed[7]})
	require.Equal(t, byTransaction[strconv.FormatInt(second.Transaction.ID, 10)][0], reversed[5])
	requireSelfCheckPasses(t, f)
}
