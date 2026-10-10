package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func correctResolutionInput(f *investmentsTestFixture, transactionID, basis int64) CorrectTransferBasisResolutionInput {
	return CorrectTransferBasisResolutionInput{OwnerUserID: f.ownerUserID, TransactionID: transactionID,
		BasisValue: basis, BasisScale: 2, SourceEvidenceJSON: `{"statement":"corrected broker statement"}`,
		Reason: "statement total was mistyped"}
}

// replaceResolutionAcknowledged previews a resolution replacement and commits
// it with the preview's gain acknowledgement.
func replaceResolutionAcknowledged(t *testing.T, f *investmentsTestFixture, input CorrectTransferBasisResolutionInput) ReplaceTransferBasisResolutionResult {
	t.Helper()
	impact, err := f.investmentService.ReplaceTransferBasisResolutionImpact(context.Background(), input)
	require.NoError(t, err)
	if impact.GainImpact != nil {
		input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	}
	result, err := f.investmentService.ReplaceTransferBasisResolution(context.Background(), input)
	require.NoError(t, err)
	return result
}

func reverseResolutionAcknowledged(t *testing.T, f *investmentsTestFixture, input CorrectTransferBasisResolutionInput) Transaction {
	t.Helper()
	impact, err := f.investmentService.ReverseTransferBasisResolutionImpact(context.Background(), input)
	require.NoError(t, err)
	if impact.GainImpact != nil {
		input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	}
	reversed, err := f.investmentService.ReverseTransferBasisResolution(context.Background(), input)
	require.NoError(t, err)
	return reversed
}

// journalByRole sums one transaction's postings by account system role.
func journalByRole(t *testing.T, f *investmentsTestFixture, transaction Transaction) map[string]*exact.ScaledInt {
	t.Helper()
	sums := map[string]*exact.ScaledInt{}
	for _, entry := range transaction.JournalEntries {
		for _, posting := range entry.Postings {
			var role string
			require.NoError(t, f.database.QueryRow(`SELECT COALESCE(system_role, '') FROM accounts WHERE id = ?`, posting.AccountID).Scan(&role))
			if sums[role] == nil {
				sums[role] = exact.ScaledIntFromCoefficient(exact.New(0), 0)
			}
			sums[role].AddScaled(exact.ScaledIntFromCoefficient(posting.QuantityValue, posting.QuantityScale))
		}
	}
	return sums
}

// outboundBridgeTotal sums every bridge journal an outbound transfer posted,
// its first bridge and later adjustments, by account system role.
func outboundBridgeTotal(t *testing.T, f *investmentsTestFixture, transactionID int64) map[string]*exact.ScaledInt {
	t.Helper()
	rows, err := f.database.Query(`SELECT a.system_role, pv.quantity_value, pv.quantity_scale
		FROM investment_operation_journal_links primary_link
		JOIN transaction_versions primary_version ON primary_version.id = primary_link.transaction_version_id
		JOIN investment_operation_journal_links bridge ON bridge.operation_id = primary_link.operation_id
			AND bridge.role <> 'primary'
		JOIN posting_versions pv ON pv.transaction_version_id = bridge.transaction_version_id
		JOIN accounts a ON a.id = pv.account_id
		WHERE primary_link.role = 'primary' AND primary_version.transaction_id = ?`, transactionID)
	require.NoError(t, err)
	defer rows.Close()
	sums := map[string]*exact.ScaledInt{}
	for rows.Next() {
		var role, value string
		var scale int
		require.NoError(t, rows.Scan(&role, &value, &scale))
		if sums[role] == nil {
			sums[role] = exact.ScaledIntFromCoefficient(exact.New(0), 0)
		}
		sums[role].AddScaled(exact.ScaledIntFromCoefficient(exact.Coefficient(value), scale))
	}
	require.NoError(t, rows.Err())
	return sums
}

// Replacing a wrong resolution after a partial sale inverts the old bridge,
// posts the successor's complete bridge at the transfer date, and revises the
// sold and held units. The superseded fact, the link and the opening all stay
// as evidence, and only the successor offers a further correction.
func TestReplacingResolutionRevisesPartialSaleAndKeepsEvidence(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	transfer, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-06-01", "2020-03-01"))
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)
	sale := latestDisposal(t, f)
	resolved := resolveAcknowledged(t, f, resolveInput(f, transfer.Transaction.ID, 8000))

	input := correctResolutionInput(f, transfer.Transaction.ID, 6000)
	impact, err := f.investmentService.ReplaceTransferBasisResolutionImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, impact.GainImpact.Changes, 1)
	change := impact.GainImpact.Changes[0]
	requireScaled(t, 4000, 2, change.Before.DisposedBasis, "the sale carried half the wrong basis")
	requireScaled(t, 3000, 2, change.After.DisposedBasis, "and now half the corrected one")
	_, err = f.investmentService.ReplaceTransferBasisResolution(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	result := replaceResolutionAcknowledged(t, f, input)

	require.Equal(t, resolved.ID, result.CorrectedTransactionID)
	require.Equal(t, transfer.Transaction.ID, result.TransferTransactionID)
	for _, journal := range []Transaction{result.Inverse, result.Replacement} {
		require.Equal(t, "2026-06-01", journal.TransactionDate, "both journals are dated to the transfer")
	}
	inverse := journalByRole(t, f, result.Inverse)
	requireScaled(t, 8000, 2, inverse["external_investment_transfer_equity"], "the inverse cancels the old bridge's equity side")
	requireScaled(t, -8000, 2, inverse["commodity_trading"], "and its trading side")
	replacement := journalByRole(t, f, result.Replacement)
	requireScaled(t, 6000, 2, replacement["commodity_trading"], "the successor posts its complete bridge")
	requireScaled(t, -6000, 2, replacement["external_investment_transfer_equity"], "with the equity side")

	requireScaled(t, 3000, 2, effectiveDisposal(t, f, sale.id).basisAmount(t), "the sold unit is revised")
	var remaining string
	var remainingScale int
	require.NoError(t, f.database.QueryRow(`SELECT remaining_cost_basis_value, remaining_cost_basis_scale FROM investment_lot_state
		WHERE lot_id = ?`, *transfer.LotID).Scan(&remaining, &remainingScale))
	requireScaled(t, 3000, 2, scaledCoefficient(t, remaining, remainingScale), "the held unit is revised")

	var facts int
	var original string
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*), MIN(basis_value) FROM investment_basis_resolutions`).Scan(&facts, &original))
	require.Equal(t, 2, facts, "the superseded resolution stays as evidence")
	var linkKnowledge, openingKnowledge string
	require.NoError(t, f.database.QueryRow(`SELECT x.basis_knowledge, l.opening_basis_knowledge FROM investment_transfer_lot_links x
		JOIN investment_lots l ON l.id = x.destination_lot_id WHERE l.id = ?`, *transfer.LotID).Scan(&linkKnowledge, &openingKnowledge))
	require.Equal(t, []string{"unknown", "unknown"}, []string{linkKnowledge, openingKnowledge})

	lineage, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, resolved.ID)
	require.NoError(t, err)
	require.Equal(t, result.Replacement.ID, *lineage.EffectiveTransactionID, "the lineage names its effective resolution")
	require.Len(t, lineage.Operations, 2)
	require.Equal(t, "replace", lineage.Operations[1].CorrectionMode)
	transferChain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, transfer.Transaction.ID)
	require.NoError(t, err)
	require.True(t, transferChain.CanCorrectBasisResolution, "the transfer offers its resolution's correction")
	require.Equal(t, result.Replacement.ID, transferChain.EffectiveBasisResolution.TransactionID)
	requireScaled(t, 6000, 2, exact.ScaledIntFromCoefficient(transferChain.EffectiveBasisResolution.BasisValue,
		transferChain.EffectiveBasisResolution.BasisScale), "the replacement form pre-fills the effective basis")
	require.False(t, transferChain.CanReverseTransfer)
	requireHealthyUnresolvedBook(t, f)
}

// A replacement reaches every position the lot did: through a split, units
// moved internally and sold at the destination, and an outbound that the
// first resolution already bridged, which posts a dated adjustment so its
// bridges total the corrected basis.
func TestReplacingResolutionRevisesSplitInternalDestinationAndBridgedOutbound(t *testing.T) {
	t.Parallel()
	f := newTransferOutFixture(t)
	ctx := context.Background()
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	transfer, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-05-01", ""))
	require.NoError(t, err)
	_, err = acknowledgedSplit(ctx, f.investmentService, splitInput(f, "2026-05-10", 2, 1))
	require.NoError(t, err)
	_, err = f.investmentService.InternalTransfer(ctx, internalTransferFromLot(f, destinationID, *transfer.LotID, exact.New(2), 0))
	require.NoError(t, err)
	sale := sellInput(f, "2026-07-01", 2)
	sale.HoldingAccountID = destinationID
	_, err = f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	destinationSale := latestDisposal(t, f)
	outbound, err := f.investmentService.ExternalTransferOut(ctx, transferOutOfLot(f, "2026-08-01", *transfer.LotID, 1))
	require.NoError(t, err)

	resolveAcknowledged(t, f, resolveInput(f, transfer.Transaction.ID, 8000))
	bridge := outboundBridgeTotal(t, f, outbound.Transaction.ID)
	requireScaled(t, 2000, 2, bridge["external_investment_transfer_equity"], "a quarter of the first resolution left")

	replaceResolutionAcknowledged(t, f, correctResolutionInput(f, transfer.Transaction.ID, 12000))
	bridge = outboundBridgeTotal(t, f, outbound.Transaction.ID)
	requireScaled(t, 3000, 2, bridge["external_investment_transfer_equity"], "bridges total a quarter of the corrected basis")
	requireScaled(t, -3000, 2, bridge["commodity_trading"], "on both sides")
	revised := effectiveDisposal(t, f, destinationSale.id)
	require.Equal(t, db.InvestmentBasisKnown, revised.knowledge)
	requireScaled(t, 6000, 2, revised.basisAmount(t), "half the split units moved and were sold")
	requireHealthyUnresolvedBook(t, f)
}

// Corrections chain: each replacement supersedes exactly the effective fact,
// a repeated or zero correction is refused, and the resolution journals
// always net to the effective basis.
func TestRepeatedResolutionCorrectionKeepsOneEffectiveFact(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	transfer, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-06-01", ""))
	require.NoError(t, err)
	first := resolveAcknowledged(t, f, resolveInput(f, transfer.Transaction.ID, 100))
	second := replaceResolutionAcknowledged(t, f, correctResolutionInput(f, transfer.Transaction.ID, 200))
	third := replaceResolutionAcknowledged(t, f, correctResolutionInput(f, transfer.Transaction.ID, 300))

	lineage, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, first.ID)
	require.NoError(t, err)
	require.Len(t, lineage.Operations, 3, "each replacement corrects the one before it")
	require.Equal(t, third.Replacement.ID, *lineage.EffectiveTransactionID)
	same := correctResolutionInput(f, transfer.Transaction.ID, 3)
	same.BasisScale = 0
	_, err = f.investmentService.ReplaceTransferBasisResolution(ctx, same)
	require.ErrorContains(t, err, "repeats the effective resolution", "3 at scale 0 equals 3.00")
	_, err = f.investmentService.ReplaceTransferBasisResolution(ctx, correctResolutionInput(f, transfer.Transaction.ID, 0))
	require.ErrorContains(t, err, "zero basis")
	_, err = f.investmentService.ResolveTransferBasis(ctx, resolveInput(f, transfer.Transaction.ID, 500))
	require.ErrorIs(t, err, ErrInvestmentTransferBasisNotUnknown, "a resolved transfer is corrected, not resolved again")

	// The schema refuses a second effective resolution of the same link.
	tx, err := f.database.Begin()
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec(`INSERT INTO investment_operations (book_id, operation_kind, event_date, created_at, created_audit_event_id)
		SELECT o.book_id, 'basis_resolution', o.event_date, o.created_at, o.created_audit_event_id
		FROM investment_operations o JOIN investment_basis_resolutions r ON r.operation_id = o.id
		WHERE EXISTS (SELECT 1 FROM effective_investment_operations e WHERE e.id = o.id)`)
	require.NoError(t, err)
	_, err = tx.Exec(`INSERT INTO investment_basis_resolutions (operation_id, book_id, transfer_operation_id, link_seq, lot_id,
		quantity_value, quantity_scale, cost_commodity_id, basis_value, basis_scale, created_audit_event_id)
		SELECT (SELECT MAX(id) FROM investment_operations), r.book_id, r.transfer_operation_id, r.link_seq, r.lot_id,
			r.quantity_value, r.quantity_scale, r.cost_commodity_id, '999', 2, o.created_audit_event_id
		FROM investment_basis_resolutions r JOIN investment_operations o ON o.id = r.operation_id
		WHERE EXISTS (SELECT 1 FROM effective_investment_operations e WHERE e.id = o.id)`)
	require.ErrorContains(t, err, "outside its unknown inbound link")
	require.NoError(t, tx.Rollback())

	var facts, effective int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*), SUM(EXISTS (SELECT 1 FROM effective_investment_operations o
		WHERE o.id = r.operation_id)) FROM investment_basis_resolutions r`).Scan(&facts, &effective))
	require.Equal(t, []int{3, 1}, []int{facts, effective})
	var carried string
	require.NoError(t, f.database.QueryRow(`SELECT carried_basis_value FROM effective_investment_transfer_links x
		JOIN investment_transfer_facts tf ON tf.operation_id = x.operation_id WHERE tf.transfer_kind = 'external_in'`).Scan(&carried))
	require.Equal(t, "300", carried)
	net := exact.ScaledIntFromCoefficient(exact.New(0), 0)
	for _, journal := range []Transaction{first, second.Inverse, second.Replacement, third.Inverse, third.Replacement} {
		net.AddScaled(journalByRole(t, f, journal)["commodity_trading"])
	}
	requireScaled(t, 300, 2, net, "the resolution journals net to the effective basis")
	requireHealthyUnresolvedBook(t, f)
}

// Reversing a resolution returns the transfer to unknown basis: its bridge is
// inverted, the sale it resolved becomes unresolved again under the preview's
// acknowledgement, and the transfer is resolvable and correctable again.
func TestReversingResolutionRestoresUnknownBasisAndUnresolvedSale(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	transfer, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-06-01", ""))
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)
	sale := latestDisposal(t, f)
	resolved := resolveAcknowledged(t, f, resolveInput(f, transfer.Transaction.ID, 8000))
	require.Equal(t, db.InvestmentBasisKnown, effectiveDisposal(t, f, sale.id).knowledge)

	input := correctResolutionInput(f, transfer.Transaction.ID, 0)
	impact, err := f.investmentService.ReverseTransferBasisResolutionImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, impact.GainImpact.Changes, 1)
	require.Equal(t, db.InvestmentBasisKnown, impact.GainImpact.Changes[0].Before.BasisKnowledge)
	require.Equal(t, db.InvestmentBasisUnknown, impact.GainImpact.Changes[0].After.BasisKnowledge)
	_, err = f.investmentService.ReverseTransferBasisResolution(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	reversed := reverseResolutionAcknowledged(t, f, input)
	require.Equal(t, "2026-06-01", reversed.TransactionDate)
	requireScaled(t, -8000, 2, journalByRole(t, f, reversed)["commodity_trading"], "the bridge is inverted")

	require.Equal(t, db.InvestmentBasisUnknown, effectiveDisposal(t, f, sale.id).knowledge)
	_, basis, knowledge := lotStateByID(t, f, *transfer.LotID)
	require.Equal(t, db.InvestmentBasisUnknown, knowledge)
	require.False(t, basis.Valid)
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, transfer.Transaction.ID)
	require.NoError(t, err)
	require.True(t, chain.CanResolveBasis)
	require.True(t, chain.CanReplaceTransfer, "the named dependency is gone")
	require.False(t, chain.CanCorrectBasisResolution)
	require.Nil(t, chain.EffectiveBasisResolution)
	requireHealthyUnresolvedBook(t, f)

	again := resolveAcknowledged(t, f, resolveInput(f, transfer.Transaction.ID, 7000))
	requireScaled(t, 3500, 2, effectiveDisposal(t, f, sale.id).basisAmount(t), "a fresh resolution follows the reversal")
	var facts int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_basis_resolutions`).Scan(&facts))
	require.Equal(t, 2, facts)
	require.NotEqual(t, resolved.ID, again.ID)
	requireHealthyUnresolvedBook(t, f)
}

// After a reversal the transfer itself can be reversed; while the resolution
// is effective the transfer refuses with the resolution named.
func TestTransferCorrectionAfterResolutionReversal(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	transfer, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-06-01", ""))
	require.NoError(t, err)
	resolveAcknowledged(t, f, resolveInput(f, transfer.Transaction.ID, 8000))
	reverseTransfer := ReverseInvestmentTransferInput{OwnerUserID: f.ownerUserID, TransactionID: transfer.Transaction.ID, Reason: "wrong broker"}
	_, err = f.investmentService.ReverseTransfer(ctx, reverseTransfer)
	require.ErrorIs(t, err, ErrInvestmentTransferBasisResolved)
	require.ErrorContains(t, err, "reverse the resolution")

	reverseResolutionAcknowledged(t, f, correctResolutionInput(f, transfer.Transaction.ID, 0))
	_, err = f.investmentService.ReverseTransfer(ctx, reverseTransfer)
	require.NoError(t, err)
	quantity, _, _ := lotStateByID(t, f, *transfer.LotID)
	require.Equal(t, "0", quantity)
	_, err = f.investmentService.ReverseTransferBasisResolution(ctx, correctResolutionInput(f, transfer.Transaction.ID, 0))
	require.ErrorIs(t, err, ErrInvestmentTransferBasisNotResolved, "a reversed transfer has nothing to correct")
	requireHealthyUnresolvedBook(t, f)
}

// A reversal never lets a known outbound or onward link silently become
// unknown: replay refuses with the dependent operation named and nothing is
// written. Replacement of the same resolution stays available.
func TestReversingResolutionRefusesKnownDependentWithOperationNamed(t *testing.T) {
	t.Parallel()
	for _, onward := range []string{"outbound", "internal"} {
		t.Run(onward, func(t *testing.T) {
			t.Parallel()
			f := newTransferOutFixture(t)
			ctx := context.Background()
			transfer, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-05-01", ""))
			require.NoError(t, err)
			var dependentTransactionID int64
			if onward == "outbound" {
				outbound, err := f.investmentService.ExternalTransferOut(ctx, transferOutOfLot(f, "2026-06-01", *transfer.LotID, 1))
				require.NoError(t, err)
				dependentTransactionID = outbound.Transaction.ID
			} else {
				destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
				moved, err := f.investmentService.InternalTransfer(ctx, internalTransferFromLot(f, destinationID, *transfer.LotID, exact.New(1), 0))
				require.NoError(t, err)
				dependentTransactionID = moved.Transaction.ID
			}
			resolveAcknowledged(t, f, resolveInput(f, transfer.Transaction.ID, 8000))
			before := buyReplacementPreviewSnapshot(t, f.database)

			input := correctResolutionInput(f, transfer.Transaction.ID, 0)
			_, err = f.investmentService.ReverseTransferBasisResolutionImpact(ctx, input)
			var dependency InvestmentTransferDependencyError
			require.ErrorAs(t, err, &dependency)
			var dependentOperation int64
			require.NoError(t, f.database.QueryRow(`SELECT link.operation_id FROM investment_operation_journal_links link
				JOIN transaction_versions v ON v.id = link.transaction_version_id
				WHERE v.transaction_id = ? AND link.role = 'primary'`, dependentTransactionID).Scan(&dependentOperation))
			require.Equal(t, dependentOperation, dependency.OperationID, "the known dependent is named")
			_, err = f.investmentService.ReverseTransferBasisResolution(ctx, input)
			require.ErrorAs(t, err, &dependency)
			require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))

			replaceResolutionAcknowledged(t, f, correctResolutionInput(f, transfer.Transaction.ID, 4000))
			requireHealthyUnresolvedBook(t, f)
		})
	}
}

// A replacement that fails after its writes (a stale acknowledgement, checked
// once replay has run) leaves nothing behind.
func TestResolutionCorrectionLateRefusalsRollBackEverything(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	transfer, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-06-01", ""))
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)
	resolveAcknowledged(t, f, resolveInput(f, transfer.Transaction.ID, 8000))
	before := buyReplacementPreviewSnapshot(t, f.database)

	stale := correctResolutionInput(f, transfer.Transaction.ID, 6000)
	impact, err := f.investmentService.ReplaceTransferBasisResolutionImpact(ctx, stale)
	require.NoError(t, err)
	stale.BasisValue = 5000
	stale.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	_, err = f.investmentService.ReplaceTransferBasisResolution(ctx, stale)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementStale)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))

	reversal := correctResolutionInput(f, transfer.Transaction.ID, 0)
	reversal.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	_, err = f.investmentService.ReverseTransferBasisResolution(ctx, reversal)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementStale, "a replacement's token does not acknowledge a reversal")
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	requireHealthyUnresolvedBook(t, f)
}
