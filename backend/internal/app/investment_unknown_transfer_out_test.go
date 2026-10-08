package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func linkKnowledge(t *testing.T, f *investmentsTestFixture, operationTransactionID int64) []string {
	t.Helper()
	rows, err := f.database.Query(`SELECT x.basis_knowledge FROM effective_investment_transfer_links x
		JOIN investment_operations o ON o.id = x.operation_id
		JOIN investment_operation_journal_links link ON link.operation_id = o.id AND link.role = 'primary'
		JOIN transaction_versions v ON v.id = link.transaction_version_id
		WHERE v.transaction_id = ? ORDER BY x.link_seq`, operationTransactionID)
	require.NoError(t, err)
	defer rows.Close()
	var knowledge []string
	for rows.Next() {
		var value string
		require.NoError(t, rows.Scan(&value))
		knowledge = append(knowledge, value)
	}
	require.NoError(t, rows.Err())
	return knowledge
}

// An unknown lot moves internally with its unknown basis: the destination lot
// opens unknown, the link records unknown, and a later destination sale stays
// unresolved. Quantity is conserved exactly.
func TestInternalTransferCarriesUnknownBasisToDestination(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	source, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-05-01", "2020-03-01"))
	require.NoError(t, err)
	result, err := f.investmentService.InternalTransfer(ctx, internalTransferFromLot(f, destinationID, *source.LotID, exact.New(1), 0))
	require.NoError(t, err)
	require.Len(t, result.DestinationLotIDs, 1)
	require.Equal(t, db.InvestmentBasisUnknown, result.Plan.Links[0].BasisKnowledge)
	destination := result.DestinationLotIDs[0]
	var opening string
	require.NoError(t, f.database.QueryRow(`SELECT opening_basis_knowledge FROM investment_lots WHERE id = ?`, destination).Scan(&opening))
	require.Equal(t, db.InvestmentBasisUnknown, opening)
	quantity, basis, knowledge := lotStateByID(t, f, destination)
	require.Equal(t, "1", quantity)
	require.Equal(t, db.InvestmentBasisUnknown, knowledge)
	require.False(t, basis.Valid)
	quantity, _, knowledge = lotStateByID(t, f, *source.LotID)
	require.Equal(t, "1", quantity)
	require.Equal(t, db.InvestmentBasisUnknown, knowledge)
	require.Equal(t, []string{"unknown"}, linkKnowledge(t, f, result.Transaction.ID))

	sale := sellInput(f, "2026-07-01", 1)
	sale.HoldingAccountID = destinationID
	_, err = f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	require.Equal(t, db.InvestmentBasisUnknown, latestDisposal(t, f).knowledge)
	requireHealthyUnresolvedBook(t, f)
}

// An average-cost pool holding unknown basis moves out as one unknown pooled
// lot by default, or as unknown source lots on request; the source pool keeps
// an unknown remainder.
func TestPooledTransferOfUnknownPoolCarriesUnknownInEitherLineage(t *testing.T) {
	t.Parallel()
	for _, lineage := range []string{db.InternalTransferPooledLot, db.InternalTransferSourceLots} {
		t.Run(lineage, func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			seedExternalTransferEquity(t, f.database)
			destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
			setHoldingCostBasisMethod(t, f, "average_cost")
			known := buyOn(t, f, "2026-01-01", 2, 2000)
			_, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-01-02", ""))
			require.NoError(t, err)
			input := pooledTransferInput(f, destinationID, "2026-02-01", exact.New(3), 0)
			input.DestinationLineage = lineage
			preview, err := f.investmentService.PreviewInternalTransfer(ctx, input)
			require.NoError(t, err)
			for _, link := range preview.Plan.Links {
				require.Equal(t, db.InvestmentBasisUnknown, link.BasisKnowledge, "a pool with unknown basis has no rate")
			}
			result, err := f.investmentService.InternalTransfer(ctx, input)
			require.NoError(t, err)
			for _, link := range linkKnowledge(t, f, result.Transaction.ID) {
				require.Equal(t, db.InvestmentBasisUnknown, link)
			}
			moved := exact.NewScaledInt()
			for _, lotID := range result.DestinationLotIDs {
				quantity, basis, knowledge := lotStateByID(t, f, lotID)
				require.Equal(t, db.InvestmentBasisUnknown, knowledge)
				require.False(t, basis.Valid)
				parsed, err := exact.Parse(quantity)
				require.NoError(t, err)
				moved.AddCoefficient(parsed, 0)
			}
			requireScaled(t, 3, 0, moved, "moved units")
			_, _, knowledge := lotStateByID(t, f, *known.LotID)
			require.Equal(t, db.InvestmentBasisUnknown, knowledge, "the pooled remainder stays unknown")
			requireHealthyUnresolvedBook(t, f)
		})
	}
}

// An outbound transfer of unknown basis posts security legs only. A transfer
// that mixes known and unknown lots has no known total, so it posts no
// partial bridge either; the complete bridge waits for resolution.
func TestOutboundTransferOfUnknownBasisPostsNoBridge(t *testing.T) {
	t.Parallel()
	for _, mixed := range []bool{false, true} {
		name := "unknown only"
		if mixed {
			name = "known and unknown lots"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newTransferOutFixture(t)
			ctx := context.Background()
			known := buyOn(t, f, "2026-05-01", 3, 3000)
			unknown, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-05-02", ""))
			require.NoError(t, err)
			input := transferOutOfLot(f, "2026-06-01", *unknown.LotID, 1)
			if mixed {
				input.Allocations = append(input.Allocations, InvestmentLotAllocationInput{LotID: *known.LotID, QuantityValue: exact.New(2)})
			}
			preview, err := f.investmentService.PreviewExternalTransferOut(ctx, input)
			require.NoError(t, err)
			require.Equal(t, db.InvestmentBasisUnknown, preview.Plan.BasisKnowledge)
			result, err := f.investmentService.ExternalTransferOut(ctx, input)
			require.NoError(t, err)
			require.Equal(t, db.InvestmentBasisUnknown, result.Plan.BasisKnowledge)
			require.Empty(t, bridgePostings(t, f, result.Transaction.ID), "no bridge until the basis is resolved")
			for _, posting := range result.Transaction.JournalEntries[0].Postings {
				require.Equal(t, f.stockCommodityID, posting.CommodityID, "security legs only")
			}
			wantLinks := []string{"unknown"}
			if mixed {
				wantLinks = append(wantLinks, "known")
			}
			require.Equal(t, wantLinks, linkKnowledge(t, f, result.Transaction.ID), "each link keeps its own knowledge")
			requireHealthyUnresolvedBook(t, f)
		})
	}
}

// History may not turn a recorded transfer's basis known or unknown. A
// backdated unknown inbound that would make an earlier known pooled outbound
// unknown is refused with the transfer named, and nothing is written.
func TestReplayRefusesToChangeAKnownOutboundToUnknown(t *testing.T) {
	t.Parallel()
	f := newTransferOutFixture(t)
	ctx := context.Background()
	setHoldingCostBasisMethod(t, f, "average_cost")
	buyOn(t, f, "2026-01-01", 4, 4000)
	pooled := ExternalTransferOutInput{OwnerUserID: f.ownerUserID, EffectiveOn: "2026-03-01",
		SourceAccountID: f.holdingAccountID, CommodityID: f.stockCommodityID, CostCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(2), SourceEvidenceJSON: `{}`, Memo: "moved out"}
	outbound, err := f.investmentService.ExternalTransferOut(ctx, pooled)
	require.NoError(t, err)
	require.Equal(t, db.InvestmentBasisKnown, outbound.Plan.BasisKnowledge)
	require.NotEmpty(t, bridgePostings(t, f, outbound.Transaction.ID))

	before := buyReplacementPreviewSnapshot(t, f.database)
	input := unknownTransferInInput(f, "2026-02-01", "")
	input.ReconciliationOverride = true
	_, err = f.investmentService.ExternalTransferIn(ctx, input)
	var dependency InvestmentBuyDependencyError
	require.ErrorAs(t, err, &dependency)
	var outboundOperation int64
	require.NoError(t, f.database.QueryRow(`SELECT link.operation_id FROM investment_operation_journal_links link
		JOIN transaction_versions v ON v.id = link.transaction_version_id
		WHERE v.transaction_id = ? AND link.role = 'primary'`, outbound.Transaction.ID).Scan(&outboundOperation))
	require.Equal(t, outboundOperation, dependency.OperationID, "the outbound transfer is named as the dependency")
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

// Reversing an internal transfer of unknown basis removes the destination lot
// and restores the source quantity with its unknown basis intact.
func TestReversingUnknownInternalTransferRestoresUnknownSource(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	source, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-05-01", ""))
	require.NoError(t, err)
	moved, err := f.investmentService.InternalTransfer(ctx, internalTransferFromLot(f, destinationID, *source.LotID, exact.New(2), 0))
	require.NoError(t, err)
	_, err = f.investmentService.ReverseTransfer(ctx, ReverseInvestmentTransferInput{OwnerUserID: f.ownerUserID,
		TransactionID: moved.Transaction.ID, Reason: "wrong destination"})
	require.NoError(t, err)
	quantity, basis, knowledge := lotStateByID(t, f, *source.LotID)
	require.Equal(t, "2", quantity)
	require.Equal(t, db.InvestmentBasisUnknown, knowledge)
	require.False(t, basis.Valid)
	quantity, _, _ = lotStateByID(t, f, moved.DestinationLotIDs[0])
	require.Equal(t, "0", quantity)
	requireHealthyUnresolvedBook(t, f)
}

// Replacing the unknown inbound that an internal transfer moved from revises
// that transfer's link to the successor lot without changing its knowledge:
// the revision stores NULL amounts and the destination stays unknown.
func TestUnknownTransferLinkRevisesLineageWithoutChangingKnowledge(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	source, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-05-01", ""))
	require.NoError(t, err)
	moved, err := f.investmentService.InternalTransfer(ctx, internalTransferFromLot(f, destinationID, *source.LotID, exact.New(1), 0))
	require.NoError(t, err)

	replacement := unknownTransferInInput(f, "2026-05-01", "2019-01-01")
	input := ReplaceInvestmentTransferInInput{OwnerUserID: f.ownerUserID, TransactionID: source.Transaction.ID,
		Reason: "original date found", Replacement: replacement}
	impact, err := f.investmentService.ReplaceTransferInReconciliationImpact(ctx, input)
	require.NoError(t, err)
	if impact.GainImpact != nil {
		input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	}
	_, err = f.investmentService.ReplaceTransferIn(ctx, input)
	require.NoError(t, err)

	var revisions int
	var value *string
	var knowledge string
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_transfer_link_revisions`).Scan(&revisions))
	require.Equal(t, 1, revisions, "the moved lot's lineage follows the successor acquisition")
	require.NoError(t, f.database.QueryRow(`SELECT carried_basis_value, basis_knowledge FROM investment_transfer_link_revisions`).Scan(&value, &knowledge))
	require.Nil(t, value)
	require.Equal(t, db.InvestmentBasisUnknown, knowledge)
	_, basis, destinationKnowledge := lotStateByID(t, f, moved.DestinationLotIDs[0])
	require.Equal(t, db.InvestmentBasisUnknown, destinationKnowledge)
	require.False(t, basis.Valid)
	requireHealthyUnresolvedBook(t, f)
}
