package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"io"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// T-119: external transfer-in replacement. The inverse and the corrected
// transfer both post; the bridge difference is appended, never overwritten.

func acknowledgedReplaceTransferIn(ctx context.Context, s *InvestmentService, input ReplaceInvestmentTransferInInput) (ReplaceInvestmentTransferInResult, error) {
	if input.GainImpactAcknowledgement == "" {
		if impact, err := s.ReplaceTransferInReconciliationImpact(ctx, input); err == nil && impact.GainImpact != nil {
			input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
		}
	}
	return s.ReplaceTransferIn(ctx, input)
}

func replaceTransferInInput(f *investmentsTestFixture, transactionID int64, replacement ExternalTransferInInput) ReplaceInvestmentTransferInInput {
	return ReplaceInvestmentTransferInInput{OwnerUserID: f.ownerUserID, TransactionID: transactionID,
		Reason: "broker statement corrected", Replacement: replacement}
}

// transactionsAccountTotal sums one account's current postings in a commodity across
// the given transactions.
func transactionsAccountTotal(t *testing.T, f *investmentsTestFixture, accountID, commodityID int64, transactionIDs ...int64) *exact.ScaledInt {
	t.Helper()
	total := exact.NewScaledInt()
	for _, id := range transactionIDs {
		transaction, err := f.transactionService.Transaction(context.Background(), id)
		require.NoError(t, err)
		for _, entry := range transaction.JournalEntries {
			for _, posting := range entry.Postings {
				if posting.AccountID == accountID && posting.CommodityID == commodityID {
					total.AddCoefficient(posting.QuantityValue, posting.QuantityScale)
				}
			}
		}
	}
	return total
}

func linkOriginalDate(t *testing.T, f *investmentsTestFixture, lotID int64) (string, string) {
	t.Helper()
	var knowledge string
	var date *string
	require.NoError(t, f.database.QueryRow(`SELECT original_date_knowledge, original_acquired_on
		FROM effective_investment_transfer_links WHERE destination_lot_id = ?`, lotID).Scan(&knowledge, &date))
	if date == nil {
		return knowledge, ""
	}
	return knowledge, *date
}

// The carried basis was wrong. A later sale's basis is restated with
// acknowledgement, and the equity bridge nets to the corrected basis across
// the original, inverse and replacement journals, the original untouched.
func TestReplaceExternalTransferInCorrectsBasisAndAppendsBridge(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	equityID := seedExternalTransferEquity(t, f.database)
	ctx := context.Background()
	in, err := f.investmentService.ExternalTransferIn(ctx, knownTransferInput(f))
	require.NoError(t, err)
	original, err := f.transactionService.Transaction(ctx, in.Transaction.ID)
	require.NoError(t, err)
	sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)
	requireScaled(t, 4000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "half of 80.00")

	corrected := knownTransferInput(f)
	corrected.CarriedBasisValue = 10000
	input := replaceTransferInInput(f, in.Transaction.ID, corrected)
	_, err = f.investmentService.ReplaceTransferIn(ctx, input)
	require.ErrorIs(t, err, db.ErrGainImpactAcknowledgementRequired)
	result, err := acknowledgedReplaceTransferIn(ctx, f.investmentService, input)
	require.NoError(t, err)

	requireScaled(t, 5000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "half of 100.00")
	requireScaled(t, -10000, 2, transactionsAccountTotal(t, f, equityID, f.eurCommodityID,
		in.Transaction.ID, result.Inverse.ID, result.Replacement.Transaction.ID), "bridge nets to the corrected basis")
	unchanged, err := f.transactionService.Transaction(ctx, in.Transaction.ID)
	require.NoError(t, err)
	assert.Equal(t, original.VersionID, unchanged.VersionID, "the original journal is evidence")
	knowledge, date := linkOriginalDate(t, f, *result.Replacement.LotID)
	assert.Equal(t, "known", knowledge)
	assert.Equal(t, "2020-03-01", date)
	assert.Equal(t, "1", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	requireInvestmentSelfCheckPasses(t, f)

	_, err = f.investmentService.ReplaceTransferIn(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentTransferAlreadyCorrected)
}

// An unknown original date stays unknown unless the replacement supplies one,
// and a later replacement in the chain may supply it.
func TestReplaceExternalTransferInKeepsUnknownOriginalDateUnlessSupplied(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	seedExternalTransferEquity(t, f.database)
	ctx := context.Background()
	unknown := knownTransferInput(f)
	unknown.OriginalAcquiredOn = ""
	in, err := f.investmentService.ExternalTransferIn(ctx, unknown)
	require.NoError(t, err)

	more := unknown
	more.QuantityValue = exact.New(3)
	first, err := acknowledgedReplaceTransferIn(ctx, f.investmentService, replaceTransferInInput(f, in.Transaction.ID, more))
	require.NoError(t, err)
	knowledge, _ := linkOriginalDate(t, f, *first.Replacement.LotID)
	assert.Equal(t, "unknown", knowledge, "no date was supplied")

	dated := more
	dated.OriginalAcquiredOn = "2019-05-01"
	second, err := acknowledgedReplaceTransferIn(ctx, f.investmentService,
		replaceTransferInInput(f, first.Replacement.Transaction.ID, dated))
	require.NoError(t, err)
	knowledge, date := linkOriginalDate(t, f, *second.Replacement.LotID)
	assert.Equal(t, "known", knowledge)
	assert.Equal(t, "2019-05-01", date)
	assert.Equal(t, "3", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	requireInvestmentSelfCheckPasses(t, f)
}

// Fewer units than a later sale needs: the sale is named, nothing written.
func TestReplaceExternalTransferInBelowLaterSaleRefusedWithoutWriting(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	seedExternalTransferEquity(t, f.database)
	ctx := context.Background()
	in, err := f.investmentService.ExternalTransferIn(ctx, knownTransferInput(f))
	require.NoError(t, err)
	sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 2))
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)

	fewer := knownTransferInput(f)
	fewer.QuantityValue = exact.New(1)
	input := replaceTransferInInput(f, in.Transaction.ID, fewer)
	_, err = f.investmentService.ReplaceTransferInReconciliationImpact(ctx, input)
	var dependency InvestmentTransferDependencyError
	require.ErrorAs(t, err, &dependency)
	assert.Equal(t, transferOperationID(t, f, sold.Transaction.ID), dependency.OperationID)
	_, err = f.investmentService.ReplaceTransferIn(ctx, input)
	require.ErrorIs(t, err, ErrInvestmentTransferDependency)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

// Moved earlier than a FIFO sale, the transferred lot's 2020 original date
// makes it the oldest: the sale is restated onto it.
func TestReplaceExternalTransferInEarlierDateReordersFIFOSale(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	seedExternalTransferEquity(t, f.database)
	ctx := context.Background()
	buyOn(t, f, "2026-05-01", 1, 1000)
	sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)
	late := knownTransferInput(f)
	late.EffectiveOn, late.QuantityValue = "2026-08-01", exact.New(1)
	in, err := f.investmentService.ExternalTransferIn(ctx, late)
	require.NoError(t, err)
	requireScaled(t, 1000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "only the buy was held")

	early := late
	early.EffectiveOn = "2026-06-01"
	_, err = acknowledgedReplaceTransferIn(ctx, f.investmentService, replaceTransferInInput(f, in.Transaction.ID, early))
	require.NoError(t, err)

	requireScaled(t, 8000, 2, saleEffectiveBasis(t, f, sold.Transaction.ID), "FIFO takes the 2020 unit")
	assert.Equal(t, "1", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	requireInvestmentSelfCheckPasses(t, f)
}

// The reconciled holding balance changes; the guard refuses after the lot
// and replay are written and must roll everything back.
func TestReplaceExternalTransferInLateReconciliationRefusalRollsBackEverything(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	seedExternalTransferEquity(t, f.database)
	ctx := context.Background()
	in, err := f.investmentService.ExternalTransferIn(ctx, knownTransferInput(f))
	require.NoError(t, err)
	reconcileHolding(t, f, "2026-06-30", 2)
	more := knownTransferInput(f)
	more.QuantityValue = exact.New(3)
	input := replaceTransferInInput(f, in.Transaction.ID, more)
	impact, err := f.investmentService.ReplaceTransferInReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.NotEmpty(t, impact.AffectedCheckpoints)
	before := buyReplacementPreviewSnapshot(t, f.database)

	_, err = f.investmentService.ReplaceTransferIn(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))

	input.ReconciliationOverride = true
	_, err = acknowledgedReplaceTransferIn(ctx, f.investmentService, input)
	require.NoError(t, err)
	assert.Equal(t, "3", positionQuantity(t, f, f.holdingAccountID, f.stockCommodityID))
	requireInvestmentSelfCheckPasses(t, f)
}

// The transfer-in command does not replace an internal transfer.
func TestReplaceTransferInFencesKind(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	seedExternalTransferEquity(t, f.database)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	transfer, err := f.investmentService.InternalTransfer(ctx, selectedLotMove(f, destinationID, *buy.LotID, "2026-06-01", 1))
	require.NoError(t, err)
	_, err = f.investmentService.ReplaceTransferIn(ctx, replaceTransferInInput(f, transfer.Transaction.ID, knownTransferInput(f)))
	require.ErrorIs(t, err, ErrInvestmentTransferNotFound)
}

// T-119: every transfer correction exports. The bundle keeps the original
// facts beside their successors, the operations carry correction links and
// reasons, and each replacement links its inverse journal.
func TestTransferCorrectionsExportWithOriginalsAndCorrectionLinks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	seedExternalTransferEquity(t, f.database)
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 5, 5000)
	reversedTransfer, err := f.investmentService.InternalTransfer(ctx, selectedLotMove(f, destinationID, *buy.LotID, "2026-06-01", 1))
	require.NoError(t, err)
	_, err = acknowledgedReverseTransfer(ctx, f.investmentService, reverseTransferInput(f, reversedTransfer.Transaction.ID))
	require.NoError(t, err)
	movedTransfer, err := f.investmentService.InternalTransfer(ctx, selectedLotMove(f, destinationID, *buy.LotID, "2026-06-02", 1))
	require.NoError(t, err)
	internal, err := acknowledgedReplaceTransfer(ctx, f.investmentService, replaceTransferInput(f, movedTransfer.Transaction.ID,
		selectedLotMove(f, destinationID, *buy.LotID, "2026-06-02", 2)))
	require.NoError(t, err)
	in, err := f.investmentService.ExternalTransferIn(ctx, knownTransferInput(f))
	require.NoError(t, err)
	corrected := knownTransferInput(f)
	corrected.CarriedBasisValue = 9000
	external, err := acknowledgedReplaceTransferIn(ctx, f.investmentService, replaceTransferInInput(f, in.Transaction.ID, corrected))
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
	assert.Len(t, read("investment-transfer-facts.csv")[1:], 5, "three originals and two replacements")
	operations := map[string][]string{}
	for _, row := range read("investment-operations.csv")[1:] {
		operations[row[1]] = row
	}
	for _, replacement := range []struct {
		original, successor int64
	}{{movedTransfer.Transaction.ID, internal.Replacement.Transaction.ID},
		{in.Transaction.ID, external.Replacement.Transaction.ID}} {
		row := operations[strconv.FormatInt(replacement.successor, 10)]
		require.NotNil(t, row)
		assert.Equal(t, operations[strconv.FormatInt(replacement.original, 10)][0], row[5], "correction link")
		assert.Equal(t, "replace", row[6])
	}
	roles := map[string]int{}
	for _, row := range read("investment-operation-journal-links.csv")[1:] {
		roles[row[3]]++
	}
	assert.Equal(t, 2, roles["reversal"], "each replacement links its inverse journal")
	requireInvestmentSelfCheckPasses(t, f)
}
