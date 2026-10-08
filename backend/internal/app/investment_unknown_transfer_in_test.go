package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// unknownTransferInInput is the public command for two units with explicitly
// unknown basis, entering the book on effectiveOn.
func unknownTransferInInput(f *investmentsTestFixture, effectiveOn, originalOn string) ExternalTransferInInput {
	input := knownTransferInput(f)
	input.EffectiveOn, input.OriginalAcquiredOn = effectiveOn, originalOn
	input.CarriedBasisValue, input.CarriedBasisScale = 0, 0
	input.BasisKnowledge = db.InvestmentBasisUnknown
	return input
}

func TestExternalTransferInWithUnknownBasisPostsSecurityLegsOnly(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	result, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-06-01", "2020-03-01"))
	require.NoError(t, err)
	require.Len(t, result.Transaction.JournalEntries, 1)
	for _, posting := range result.Transaction.JournalEntries[0].Postings {
		require.Equal(t, f.stockCommodityID, posting.CommodityID, "no basis bridge for unknown basis")
	}
	var opening, projected, link string
	require.NoError(t, f.database.QueryRow(`SELECT l.opening_basis_knowledge, s.basis_knowledge, x.basis_knowledge
		FROM investment_lots l JOIN investment_lot_state s ON s.lot_id = l.id
		JOIN investment_transfer_lot_links x ON x.destination_lot_id = l.id WHERE l.id = ?`, *result.LotID).Scan(&opening, &projected, &link))
	require.Equal(t, []string{"unknown", "unknown", "unknown"}, []string{opening, projected, link})
	requireHealthyUnresolvedBook(t, f)

	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, result.Transaction.ID)
	require.NoError(t, err)
	require.NotNil(t, chain.EffectiveTransfer)
	require.Equal(t, db.InvestmentBasisUnknown, chain.EffectiveTransfer.BasisKnowledge,
		"a replacement form is prefilled with unknown, never a known zero")
}

func TestExternalTransferInRefusesAmountWithUnknownBasis(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	seedExternalTransferEquity(t, f.database)
	input := unknownTransferInInput(f, "2026-06-01", "")
	input.CarriedBasisValue, input.CarriedBasisScale = 0, 2
	_, err := f.investmentService.ExternalTransferIn(context.Background(), input)
	require.ErrorContains(t, err, "unknown carried basis cannot have an amount")
	input.CarriedBasisValue = 100
	_, err = f.investmentService.ExternalTransferIn(context.Background(), input)
	require.ErrorContains(t, err, "unknown carried basis cannot have an amount")
	input.BasisKnowledge = "maybe"
	_, err = f.investmentService.ExternalTransferIn(context.Background(), input)
	require.ErrorContains(t, err, "basis knowledge must be known or unknown")
}

// An unknown inbound dated behind a later known sale takes FIFO priority by
// its original date: replay leaves that sale unresolved, a disclosed change.
func TestBackdatedUnknownTransferInLeavesLaterSaleUnresolvedWithDisclosure(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	buyOn(t, f, "2026-01-01", 10, 10000)
	_, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)
	sale := latestDisposal(t, f)
	require.Equal(t, db.InvestmentBasisKnown, sale.knowledge)

	input := unknownTransferInInput(f, "2026-06-01", "2020-03-01")
	impact, err := f.investmentService.PreviewExternalTransferInReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, impact.GainImpact.Changes, 1)
	change := impact.GainImpact.Changes[0]
	require.Equal(t, db.InvestmentBasisKnown, change.Before.BasisKnowledge)
	require.Equal(t, db.InvestmentBasisUnknown, change.After.BasisKnowledge)
	require.Nil(t, change.After.Gain, "known to unresolved, never to a zero gain")
	_, err = f.investmentService.ExternalTransferIn(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	_, err = f.investmentService.ExternalTransferIn(ctx, input)
	require.NoError(t, err)
	require.Equal(t, db.InvestmentBasisUnknown, effectiveDisposal(t, f, sale.id).knowledge)
	requireHealthyUnresolvedBook(t, f)
}

// A later write-off has no unresolved-result contract, so a backdated
// unknown inbound it would have to consume is refused, naming it.
func TestBackdatedUnknownTransferInRefusesLaterWriteOffDependency(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	buyOn(t, f, "2026-01-01", 10, 10000)
	_, err := f.investmentService.WriteOff(ctx, backdatedWriteOff(f, "2026-07-01", 1))
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)
	input := unknownTransferInInput(f, "2026-06-01", "2020-03-01")
	input.ReconciliationOverride = true
	_, err = f.investmentService.ExternalTransferIn(ctx, input)
	var dependency InvestmentBuyDependencyError
	require.ErrorAs(t, err, &dependency, "the later write-off is named")
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "nothing is written")
}

// A split moves no basis: an unknown lot keeps its unknown basis across the
// new quantity, beside a known lot whose basis is unchanged.
func TestSplitConservesUnknownBasisKnowledge(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	known := buyOn(t, f, "2026-01-01", 3, 3000)
	unknown, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-02-01", ""))
	require.NoError(t, err)
	_, err = f.investmentService.Split(ctx, splitInput(f, "2026-03-01", 2, 1))
	require.NoError(t, err)
	quantity, basis, knowledge := lotStateByID(t, f, *unknown.LotID)
	require.Equal(t, "4", quantity)
	require.Equal(t, db.InvestmentBasisUnknown, knowledge)
	require.False(t, basis.Valid)
	quantity, basis, knowledge = lotStateByID(t, f, *known.LotID)
	require.Equal(t, "6", quantity)
	require.Equal(t, db.InvestmentBasisKnown, knowledge)
	require.True(t, basis.Valid)
	requireHealthyUnresolvedBook(t, f)
}

// Replacing an unknown inbound with sourced known basis posts the full bridge
// in the replacement and resolves the dependent sale through replay, as a
// disclosed and acknowledged gain change. The original link stays unknown.
func TestReplacingUnknownTransferInWithKnownBasisResolvesDependentSale(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	transfer, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-06-01", "2020-03-01"))
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)
	sale := latestDisposal(t, f)
	require.Equal(t, db.InvestmentBasisUnknown, sale.knowledge)

	replacement := knownTransferInput(f)
	replacement.OriginalAcquiredOn = "2020-03-01"
	input := ReplaceInvestmentTransferInInput{OwnerUserID: f.ownerUserID, TransactionID: transfer.Transaction.ID,
		Reason: "broker statement found", Replacement: replacement}
	impact, err := f.investmentService.ReplaceTransferInReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, impact.GainImpact.Changes, 1)
	require.Equal(t, db.InvestmentBasisUnknown, impact.GainImpact.Changes[0].Before.BasisKnowledge)
	require.Equal(t, db.InvestmentBasisKnown, impact.GainImpact.Changes[0].After.BasisKnowledge)
	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	replaced, err := f.investmentService.ReplaceTransferIn(ctx, input)
	require.NoError(t, err)

	bridged := false
	for _, posting := range replaced.Replacement.Transaction.JournalEntries[0].Postings {
		if posting.CommodityID == f.eurCommodityID {
			bridged = true
			amount := exact.ScaledIntFromCoefficient(posting.QuantityValue, posting.QuantityScale)
			if amount.Sign() < 0 {
				amount = exact.ScaledIntFromCoefficient(posting.QuantityValue.Negated(), posting.QuantityScale)
			}
			require.Zero(t, amount.Cmp(exact.ScaledIntFromInt64(8000, 2)))
		}
	}
	require.True(t, bridged, "the replacement posts the complete known bridge")
	var original string
	require.NoError(t, f.database.QueryRow(`SELECT basis_knowledge FROM investment_transfer_lot_links WHERE destination_lot_id = ?`,
		*transfer.LotID).Scan(&original))
	require.Equal(t, db.InvestmentBasisUnknown, original, "the original evidence is unchanged")
	resolved := latestDisposal(t, f)
	require.Equal(t, db.InvestmentBasisKnown, resolved.knowledge)
	requireHealthyUnresolvedBook(t, f)
}
