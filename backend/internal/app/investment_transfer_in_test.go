package app

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func seedExternalTransferEquity(t *testing.T, database *sql.DB) int64 {
	t.Helper()
	result, err := database.Exec(`
		INSERT INTO accounts (book_id, system_role, created_at, created_by_user_id)
		VALUES (?, 'external_investment_transfer_equity', '2026-01-01T00:00:00Z', 1)
	`, BookID)
	require.NoError(t, err)
	id, err := result.LastInsertId()
	require.NoError(t, err)
	_, err = database.Exec(`
		INSERT INTO account_versions (account_id, version_seq, effective_from, recorded_at,
			changed_by_user_id, change_reason, status, opened_on, code, name,
			account_class, account_kind, allows_postings)
		VALUES (?, 1, '0001-01-01', '2026-01-01T00:00:00Z', 1, 'test fixture',
			'active', '0001-01-01', 'TRANSFER_EQUITY', 'External transfer equity', 'equity', 'equity', 1)
	`, id)
	require.NoError(t, err)
	return id
}

func knownTransferInput(f *investmentsTestFixture) ExternalTransferInInput {
	return ExternalTransferInInput{
		OwnerUserID: f.ownerUserID, EffectiveOn: "2026-06-01", HoldingAccountID: f.holdingAccountID,
		CommodityID: f.stockCommodityID, QuantityValue: exact.New(2), QuantityScale: 0,
		CarriedBasisValue: 8000, CarriedBasisScale: 2, CostCommodityID: f.eurCommodityID,
		OriginalAcquiredOn: "2020-03-01", SourceEvidenceJSON: `{"broker":"statement-42"}`,
		Memo: "in-kind transfer", ChangeReason: "migrated holding",
	}
}

func TestExternalTransferInKnownBasisPostsBookBridgeAndReplaysAsOpening(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	equityID := seedExternalTransferEquity(t, f.database)
	ctx := context.Background()
	tradingID, err := f.investmentService.repository.CommodityTradingAccountID(ctx, BookID)
	require.NoError(t, err)
	input := knownTransferInput(f)
	var auditsBefore int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditsBefore))
	result, err := f.investmentService.ExternalTransferIn(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, result.LotID)
	require.Len(t, result.Transaction.JournalEntries, 1)
	postings := result.Transaction.JournalEntries[0].Postings
	require.Len(t, postings, 4)
	want := map[int64]map[int64]string{
		f.holdingAccountID: {f.stockCommodityID: "2"},
		tradingID:          {f.stockCommodityID: "-2", f.eurCommodityID: "8000"},
		equityID:           {f.eurCommodityID: "-8000"},
	}
	for _, posting := range postings {
		assert.Equal(t, want[posting.AccountID][posting.CommodityID], posting.QuantityValue.String())
	}
	var auditsAfter int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditsAfter))
	assert.Equal(t, auditsBefore+1, auditsAfter)

	var kind, dateRole, originalDate, originalKnowledge, basisKnowledge, basis, evidence string
	err = f.database.QueryRow(`
		SELECT o.operation_kind, d.date_role, x.original_acquired_on, x.original_date_knowledge,
			x.basis_knowledge, x.carried_basis_value, x.source_evidence_json
		FROM investment_operations o JOIN investment_operation_dates d ON d.operation_id = o.id
		JOIN investment_transfer_lot_links x ON x.operation_id = o.id
		WHERE o.transaction_id = ?`, result.Transaction.ID).Scan(&kind, &dateRole, &originalDate,
		&originalKnowledge, &basisKnowledge, &basis, &evidence)
	require.NoError(t, err)
	assert.Equal(t, "external_transfer_in", kind)
	assert.Equal(t, "effective", dateRole)
	assert.Equal(t, "2020-03-01", originalDate)
	assert.Equal(t, "known", originalKnowledge)
	assert.Equal(t, "known", basisKnowledge)
	assert.Equal(t, "8000", basis)
	assert.JSONEq(t, `{"broker":"statement-42"}`, evidence)

	intents, err := f.investmentService.repository.ListInvestmentReplayIntents(ctx, BookID,
		f.holdingAccountID, f.stockCommodityID, f.eurCommodityID, "long")
	require.NoError(t, err)
	require.Len(t, intents, 1)
	assert.Equal(t, "opening", intents[0].Kind)
	assert.Equal(t, *result.LotID, intents[0].LotID)

	positions, err := f.investmentService.Positions(ctx)
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assert.Equal(t, int64(8000), positions[0].RemainingCostBasisValue)
	assert.Equal(t, "2", positions[0].QuantityValue.String())
	var archive bytes.Buffer
	require.NoError(t, NewExportService(db.NewExportRepository(f.database)).WriteBundle(ctx, &archive, ExportFilter{}))
	zipReader, err := zip.NewReader(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	require.NoError(t, err)
	for name, expected := range map[string]string{
		"investment-transfer-facts.csv":     "external_in",
		"investment-transfer-lot-links.csv": "2020-03-01",
	} {
		found := false
		for _, file := range zipReader.File {
			if file.Name != name {
				continue
			}
			reader, err := file.Open()
			require.NoError(t, err)
			contents, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.NoError(t, reader.Close())
			assert.Contains(t, string(contents), expected)
			found = true
		}
		require.True(t, found, "bundle omitted %s", name)
	}

	sale, err := f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-07-01",
		HoldingAccountID: f.holdingAccountID, CommodityID: f.stockCommodityID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(2), CashAmountValue: 10000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	require.Len(t, sale.Allocations, 1)
	assert.Zero(t, exact.ScaledIntFromInt64(sale.Allocations[0].CostBasisValue, sale.Allocations[0].CostBasisScale).Cmp(exact.ScaledIntFromInt64(8000, 2)))
	assert.Zero(t, exact.ScaledIntFromInt64(sale.Allocations[0].ProceedsValue, sale.Allocations[0].ProceedsScale).Cmp(exact.ScaledIntFromInt64(10000, 2)))
	run, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	assert.Equal(t, SelfCheckPassed, resultFor(t, run, CheckInvestmentFoundation).Status)
}

func TestExternalTransferInKnownZeroBasisAndUnknownOriginalDate(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	seedExternalTransferEquity(t, f.database)
	input := knownTransferInput(f)
	input.CarriedBasisValue = 0
	input.OriginalAcquiredOn = ""
	result, err := f.investmentService.ExternalTransferIn(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, result.Transaction.JournalEntries[0].Postings, 2)
	var basis, knowledge string
	var original sql.NullString
	require.NoError(t, f.database.QueryRow(`
		SELECT x.carried_basis_value, x.original_date_knowledge, x.original_acquired_on
		FROM investment_transfer_lot_links x JOIN investment_operations o ON o.id = x.operation_id
		WHERE o.transaction_id = ?`, result.Transaction.ID).Scan(&basis, &knowledge, &original))
	assert.Equal(t, "0", basis)
	assert.Equal(t, "unknown", knowledge)
	assert.False(t, original.Valid)
}

func TestExternalTransferInRefusesBackdatingBehindSaleWithoutPartialWrite(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	seedExternalTransferEquity(t, f.database)
	buyOn(t, f, "2026-05-01", 2, 8000)
	_, err := f.investmentService.Sell(context.Background(), sellInput(f, "2026-07-01", 2))
	require.NoError(t, err)
	before := f.transactionCount(t)
	input := knownTransferInput(f)
	_, err = f.investmentService.ExternalTransferIn(context.Background(), input)
	require.ErrorIs(t, err, ErrInvestmentEventOutOfOrder)
	assert.Equal(t, before, f.transactionCount(t))
	var facts int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_transfer_facts`).Scan(&facts))
	assert.Zero(t, facts)
}
