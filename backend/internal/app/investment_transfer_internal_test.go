package app

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

func internalTransferFromLot(f *investmentsTestFixture, destinationID, lotID int64, quantity exact.Coefficient, scale int) InternalTransferInput {
	return InternalTransferInput{
		OwnerUserID: f.ownerUserID, EffectiveOn: "2026-06-01",
		SourceAccountID: f.holdingAccountID, DestinationAccountID: destinationID,
		CommodityID: f.stockCommodityID, CostCommodityID: f.eurCommodityID,
		Allocations:        []InvestmentLotAllocationInput{{LotID: lotID, QuantityValue: quantity, QuantityScale: scale}},
		SourceEvidenceJSON: `{"statement":"moving account"}`, Memo: "move holdings",
	}
}

func TestInternalTransferPartialLotConservesQuantityBasisAndJournal(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	ctx := context.Background()
	buy := buyOn(t, f, "2026-05-01", 3, 1000)
	var auditsBefore int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditsBefore))
	result, err := f.investmentService.InternalTransfer(ctx,
		internalTransferFromLot(f, destinationID, *buy.LotID, exact.New(1), 0))
	require.NoError(t, err)
	require.Len(t, result.DestinationLotIDs, 1)
	require.Len(t, result.Transaction.JournalEntries, 1)
	postings := result.Transaction.JournalEntries[0].Postings
	require.Len(t, postings, 2)
	assert.Equal(t, f.holdingAccountID, postings[0].AccountID)
	assert.Equal(t, "-1", postings[0].QuantityValue.String())
	assert.Equal(t, destinationID, postings[1].AccountID)
	assert.Equal(t, "1", postings[1].QuantityValue.String())
	var auditsAfter int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditsAfter))
	assert.Equal(t, auditsBefore+1, auditsAfter)

	var sourceQty, destQty string
	var sourceBasis, destBasis int64
	var sourceScale, destScale int
	require.NoError(t, f.database.QueryRow(`SELECT remaining_quantity_value,
		remaining_cost_basis_value, remaining_cost_basis_scale FROM current_investment_lots WHERE id = ?`,
		*buy.LotID).Scan(&sourceQty, &sourceBasis, &sourceScale))
	require.NoError(t, f.database.QueryRow(`SELECT remaining_quantity_value,
		remaining_cost_basis_value, remaining_cost_basis_scale FROM current_investment_lots WHERE id = ?`,
		result.DestinationLotIDs[0]).Scan(&destQty, &destBasis, &destScale))
	assert.Equal(t, "2", sourceQty)
	assert.Equal(t, "1", destQty)
	combined := exact.NewScaledInt()
	combined.AddInt64(sourceBasis, sourceScale)
	combined.AddInt64(destBasis, destScale)
	assert.Zero(t, combined.Cmp(exact.ScaledIntFromInt64(1000, 2)))

	var originalDate, sourceKind, destinationKind, linkedBasis string
	require.NoError(t, f.database.QueryRow(`SELECT x.original_acquired_on, src.event_kind,
		dst.event_kind, x.carried_basis_value FROM investment_transfer_lot_links x
		JOIN investment_operations o ON o.id = x.operation_id
		JOIN investment_lot_events src ON src.lot_id = x.source_lot_id AND src.transaction_id = (SELECT linked_version.transaction_id FROM investment_operation_journal_links journal_link
        JOIN transaction_versions linked_version ON linked_version.id = journal_link.transaction_version_id
        WHERE journal_link.operation_id = o.id AND journal_link.role = 'primary' ORDER BY journal_link.link_seq LIMIT 1)
		JOIN investment_lot_events dst ON dst.lot_id = x.destination_lot_id AND dst.transaction_id = (SELECT linked_version.transaction_id FROM investment_operation_journal_links journal_link
        JOIN transaction_versions linked_version ON linked_version.id = journal_link.transaction_version_id
        WHERE journal_link.operation_id = o.id AND journal_link.role = 'primary' ORDER BY journal_link.link_seq LIMIT 1)
		WHERE (SELECT linked_version.transaction_id FROM investment_operation_journal_links journal_link
        JOIN transaction_versions linked_version ON linked_version.id = journal_link.transaction_version_id
        WHERE journal_link.operation_id = o.id AND journal_link.role = 'primary' ORDER BY journal_link.link_seq LIMIT 1) = ?`, result.Transaction.ID).Scan(&originalDate, &sourceKind, &destinationKind, &linkedBasis))
	assert.Equal(t, "2026-05-01", originalDate)
	assert.Equal(t, "transfer_out", sourceKind)
	assert.Equal(t, "transfer_in", destinationKind)
	assert.Equal(t, exact.New(destBasis).String(), linkedBasis)

	// A later sale must consume the destination lot, not the moved source units.
	sale := sellInput(f, "2026-07-01", 1)
	sale.HoldingAccountID = destinationID
	_, err = f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	run, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	assert.Equal(t, SelfCheckPassed, resultFor(t, run, CheckInvestmentFoundation).Status)
}

func TestInternalTransferPreservesUnknownOriginalAcquisitionDate(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	seedExternalTransferEquity(t, f.database)
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	inbound := knownTransferInput(f)
	inbound.EffectiveOn = "2026-05-01"
	inbound.OriginalAcquiredOn = ""
	opened, err := f.investmentService.ExternalTransferIn(context.Background(), inbound)
	require.NoError(t, err)
	_, err = f.investmentService.InternalTransfer(context.Background(),
		internalTransferFromLot(f, destinationID, *opened.LotID, exact.New(2), 0))
	require.NoError(t, err)
	var knowledge string
	var original sql.NullString
	require.NoError(t, f.database.QueryRow(`SELECT original_date_knowledge, original_acquired_on
		FROM investment_transfer_lot_links WHERE source_lot_id = ?`, *opened.LotID).Scan(&knowledge, &original))
	assert.Equal(t, "unknown", knowledge)
	assert.False(t, original.Valid)
}

func TestInternalTransferKnownZeroBasisKeepsZeroWithoutGain(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	seedExternalTransferEquity(t, f.database)
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	inbound := knownTransferInput(f)
	inbound.EffectiveOn = "2026-05-01"
	inbound.CarriedBasisValue = 0
	opened, err := f.investmentService.ExternalTransferIn(context.Background(), inbound)
	require.NoError(t, err)
	transfer, err := f.investmentService.InternalTransfer(context.Background(),
		internalTransferFromLot(f, destinationID, *opened.LotID, exact.New(2), 0))
	require.NoError(t, err)
	require.Len(t, transfer.DestinationLotIDs, 1)
	var sourceEventBasis, destinationBasis, linkedBasis string
	require.NoError(t, f.database.QueryRow(`SELECT e.cost_basis_value, l.cost_basis_value, x.carried_basis_value
		FROM investment_transfer_lot_links x
		JOIN current_investment_lots l ON l.id = x.destination_lot_id
		JOIN investment_operations o ON o.id = x.operation_id
		JOIN investment_lot_events e ON e.lot_id = x.source_lot_id
			AND e.transaction_id = (SELECT linked_version.transaction_id FROM investment_operation_journal_links journal_link
        JOIN transaction_versions linked_version ON linked_version.id = journal_link.transaction_version_id
        WHERE journal_link.operation_id = o.id AND journal_link.role = 'primary' ORDER BY journal_link.link_seq LIMIT 1) AND e.event_kind = 'transfer_out'
		WHERE x.destination_lot_id = ?`, transfer.DestinationLotIDs[0]).Scan(
		&sourceEventBasis, &destinationBasis, &linkedBasis))
	assert.Equal(t, "0", sourceEventBasis)
	assert.Equal(t, "0", destinationBasis)
	assert.Equal(t, "0", linkedBasis)
	run, err := selfCheckOver(t, f.database).RunSelfCheck(context.Background(), "manual")
	require.NoError(t, err)
	assert.Equal(t, SelfCheckPassed, resultFor(t, run, CheckInvestmentFoundation).Status)
}

func TestInternalTransferRefusesUnavailableSourceWithoutPartialWrite(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 1, 1000)
	before := f.transactionCount(t)
	input := internalTransferFromLot(f, destinationID, *buy.LotID, exact.New(2), 0)
	_, err := f.investmentService.PreviewInternalTransferReconciliationImpact(context.Background(), input)
	require.ErrorIs(t, err, ErrInvestmentLotsInsufficient)
	assert.Equal(t, before, f.transactionCount(t))
	_, err = f.investmentService.InternalTransfer(context.Background(), input)
	require.ErrorIs(t, err, ErrInvestmentLotsInsufficient)
	assert.Equal(t, before, f.transactionCount(t))
	var facts int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_transfer_facts`).Scan(&facts))
	assert.Zero(t, facts)
}

func TestInternalTransferRefusesLotBasisFromOpenAverageCostPool(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buyOn(t, f, "2026-01-01", 2, 2000)
	sale := sellInput(f, "2026-02-01", 1)
	sale.CostBasisMethod = "average_cost"
	_, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	laterBuy := buyOn(t, f, "2026-03-01", 1, 2000)
	input := internalTransferFromLot(f, destinationID, *laterBuy.LotID, exact.New(1), 0)
	before := f.transactionCount(t)
	_, err = f.investmentService.PreviewInternalTransferReconciliationImpact(ctx, input)
	require.ErrorContains(t, err, "requires pooled basis allocation")
	_, err = f.investmentService.InternalTransfer(ctx, input)
	require.ErrorContains(t, err, "requires pooled basis allocation")
	assert.Equal(t, before, f.transactionCount(t))
	var count int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_transfer_facts
		WHERE transfer_kind = 'internal'`).Scan(&count))
	assert.Zero(t, count)
}

func TestInternalTransferRefusesAverageCostDefaultBeforeFirstSale(t *testing.T) {
	for _, tier := range []string{"account", "global"} {
		t.Run(tier, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
			buyOn(t, f, "2026-01-01", 1, 1000)
			later := buyOn(t, f, "2026-01-02", 1, 2000)
			if tier == "account" {
				setHoldingCostBasisMethod(t, f, "average_cost")
			} else {
				_, err := f.investmentService.SaveCostBasisProfile(ctx, CostBasisProfileInput{
					OwnerUserID: f.ownerUserID, Name: "Book average cost", Method: "average_cost", IsDefault: true, Status: "active",
				})
				require.NoError(t, err)
			}
			input := internalTransferFromLot(f, destinationID, *later.LotID, exact.New(1), 0)
			before := f.transactionCount(t)
			_, err := f.investmentService.PreviewInternalTransferReconciliationImpact(ctx, input)
			require.ErrorContains(t, err, "requires pooled basis allocation")
			_, err = f.investmentService.InternalTransfer(ctx, input)
			require.ErrorContains(t, err, "requires pooled basis allocation")
			require.Equal(t, before, f.transactionCount(t))
			var transfers int
			require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_transfer_facts`).Scan(&transfers))
			require.Zero(t, transfers)
		})
	}
}

func TestInternalTransferLocksIndividualLotMethodBeforeFirstSale(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buyOn(t, f, "2026-01-01", 1, 1000)
	later := buyOn(t, f, "2026-01-02", 1, 2000)
	_, err := f.investmentService.InternalTransfer(ctx,
		internalTransferFromLot(f, destinationID, *later.LotID, exact.New(1), 0))
	require.NoError(t, err)
	var family string
	require.NoError(t, f.database.QueryRow(`SELECT method_family FROM investment_position_basis_state
		WHERE account_id = ? AND commodity_id = ? AND cost_commodity_id = ? AND position_side = 'long'`,
		f.holdingAccountID, f.stockCommodityID, f.eurCommodityID).Scan(&family))
	require.Equal(t, "individual_lot", family)
	sale := sellInput(f, "2026-07-01", 1)
	sale.CostBasisMethod = "average_cost"
	_, err = f.investmentService.Sell(ctx, sale)
	require.ErrorContains(t, err, "cannot switch into or out of average_cost")
}

func TestInternalTransferDepletionSurvivesLaterSaleReversalReplay(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	_, err := f.investmentService.InternalTransfer(context.Background(),
		internalTransferFromLot(f, destinationID, *buy.LotID, exact.New(1), 0))
	require.NoError(t, err)
	sale, err := f.investmentService.Sell(context.Background(), sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)
	_, err = acknowledgedReverseSale(context.Background(), f.investmentService, ReverseInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, TransactionID: sale.Transaction.ID,
		Reason: "duplicate sale",
	})
	require.NoError(t, err)
	var sourceQty, destinationQty string
	require.NoError(t, f.database.QueryRow(`SELECT remaining_quantity_value FROM current_investment_lots WHERE id = ?`,
		*buy.LotID).Scan(&sourceQty))
	require.NoError(t, f.database.QueryRow(`SELECT remaining_quantity_value FROM current_investment_lots
		WHERE source_transaction_id IN (SELECT (SELECT linked_version.transaction_id FROM investment_operation_journal_links journal_link
        JOIN transaction_versions linked_version ON linked_version.id = journal_link.transaction_version_id
        WHERE journal_link.operation_id = o.id AND journal_link.role = 'primary' ORDER BY journal_link.link_seq LIMIT 1) FROM investment_operations o
			JOIN investment_transfer_facts f ON f.operation_id = o.id
			WHERE f.transfer_kind = 'internal')`).Scan(&destinationQty))
	assert.Equal(t, "2", sourceQty)
	assert.Equal(t, "1", destinationQty)
}

func TestReplayTransferDepletionUsesEffectLinkWithoutLegacyOperationTransactionID(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	transfer, err := f.investmentService.InternalTransfer(ctx,
		internalTransferFromLot(f, destinationID, *buy.LotID, exact.New(1), 0))
	require.NoError(t, err)
	// The retired-header schema resolves depletion through its immutable lot-effect link.
	requireInvestmentHeaderRetired(t, f)
	intents, err := f.investmentService.repository.ListInvestmentReplayIntents(ctx,
		BookID, f.holdingAccountID, f.stockCommodityID, f.eurCommodityID, "long")
	require.NoError(t, err)
	var found bool
	for _, intent := range intents {
		if intent.Kind == "transfer_out" {
			found = true
			require.Equal(t, transfer.Transaction.ID, intent.TransactionID)
			require.Equal(t, *buy.LotID, intent.LotID)
		}
	}
	require.True(t, found)
	require.Equal(t, SelfCheckPassed,
		resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation).Status)
}
