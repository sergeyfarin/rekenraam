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
	corrected, err := f.investmentService.ReplaceBuy(ctx, ReplaceInvestmentBuyInput{
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
