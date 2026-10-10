package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// Share exchange reads (#178): the lots, the transaction and its correction
// chain explain an exchange with its instruments, ratio and carried basis.

func TestShareExchangeReadsExplainInstrumentsRatioAndBasis(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	first := buyOn(t, f, "2026-02-01", 60, 60000)
	second := buyOn(t, f, "2026-03-01", 40, 40000)
	exchanged, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 3, 2))
	require.NoError(t, err)

	// 100 old units at 1,000.00 become 150 new units at the same 1,000.00.
	require.Len(t, exchanged.Plan.BasisTotals, 1)
	total := exchanged.Plan.BasisTotals[0]
	assert.Equal(t, f.eurCommodityID, total.CostCommodityID)
	assert.Equal(t, "100", total.SourceQuantityValue.String())
	assert.Equal(t, "150", total.DestinationQuantityValue.String())
	assert.Equal(t, db.InvestmentBasisKnown, total.BasisKnowledge)
	assert.Zero(t, exact.ScaledIntFromCoefficient(total.CarriedBasisValue, total.CarriedBasisScale).Cmp(
		exact.ScaledIntFromInt64(100000, 2)), "basis %s", total.CarriedBasisValue)
	assert.Zero(t, total.UnknownLots)

	// Each new lot names the exchange, the old instrument and the ratio, and
	// keeps its source lot's original acquisition date.
	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, newID)
	require.NoError(t, err)
	require.Len(t, lots, 2)
	wantOriginal := map[int64]string{*first.LotID: "2026-02-01", *second.LotID: "2026-03-01"}
	for _, lot := range lots {
		assert.Equal(t, "2026-06-01", lot.OpenedOn, "the lot arrived on the exchange date")
		require.NotNil(t, lot.Origin, "lot %d", lot.ID)
		assert.Equal(t, "exchange", lot.Origin.TransferKind)
		assert.Equal(t, f.stockCommodityID, lot.Origin.SourceCommodityID)
		require.NotNil(t, lot.Origin.SourceLotID)
		require.NotNil(t, lot.Origin.RatioNumerator)
		require.NotNil(t, lot.Origin.RatioDenominator)
		assert.Equal(t, [2]int64{3, 2}, [2]int64{*lot.Origin.RatioNumerator, *lot.Origin.RatioDenominator})
		assert.Equal(t, "known", lot.Origin.OriginalDateKnowledge)
		assert.Equal(t, wantOriginal[*lot.Origin.SourceLotID], lot.Origin.OriginalAcquiredOn)
	}
	// A bought lot has no origin.
	oldLots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	require.Len(t, oldLots, 2)
	for _, lot := range oldLots {
		assert.Nil(t, lot.Origin)
	}

	// The journal is labelled for a title when it has no memo.
	transaction, err := f.transactionService.Transaction(ctx, exchanged.Transaction.ID)
	require.NoError(t, err)
	assert.Equal(t, "share_exchange", transaction.SystemLabel)

	// The chain explains the exchange and offers no correction yet (#179).
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, exchanged.Transaction.ID)
	require.NoError(t, err)
	require.NotNil(t, chain.EffectiveShareExchange)
	terms := chain.EffectiveShareExchange
	assert.Equal(t, f.stockCommodityID, terms.CommodityID)
	assert.Equal(t, newID, terms.DestinationCommodityID)
	assert.Equal(t, f.holdingAccountID, terms.HoldingAccountID)
	assert.Equal(t, f.holdingAccountID, terms.DestinationHoldingAccountID)
	assert.Equal(t, "2026-06-01", terms.EffectiveOn)
	assert.Equal(t, exchanged.Plan, terms.Plan, "the chain reads back exactly what was committed")
	assert.False(t, chain.CanReverseTransfer || chain.CanReplaceTransfer || chain.CanReverseBuy || chain.CanReverseSale ||
		chain.CanCorrectSplit, "an exchange is not yet correctable")
}

func TestShareExchangeReadsKeepUnknownBasisUnknownAndShowRevisedBasis(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	seedExternalTransferEquity(t, f.database)
	ctx := context.Background()
	newID := seedTestSecurityCommodity(t, f, "NEWCO")
	bought := buyOn(t, f, "2026-02-01", 10, 10000)
	_, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-03-01", "2020-03-01"))
	require.NoError(t, err)
	exchanged, err := f.investmentService.ShareExchange(ctx, shareExchangeInput(f, newID, "2026-06-01", 1, 1))
	require.NoError(t, err)

	// One unknown lot leaves the currency's carried basis unknown, never a
	// partial sum.
	require.Len(t, exchanged.Plan.BasisTotals, 1)
	assert.Equal(t, db.InvestmentBasisUnknown, exchanged.Plan.BasisTotals[0].BasisKnowledge)
	assert.Equal(t, 1, exchanged.Plan.BasisTotals[0].UnknownLots)
	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, newID)
	require.NoError(t, err)
	originals := map[string]bool{}
	for _, lot := range lots {
		require.NotNil(t, lot.Origin)
		originals[lot.Origin.OriginalAcquiredOn] = true
	}
	assert.Equal(t, map[string]bool{"2020-03-01": true, "2026-02-01": true}, originals,
		"the transferred lot's original date survives the exchange")

	// An upstream correction revises the carried basis the chain shows.
	_, err = f.investmentService.ReplaceBuy(ctx, buyReplacementForNetting(f, bought.Transaction.ID, "2026-02-01", 10, 15000))
	require.NoError(t, err)
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, exchanged.Transaction.ID)
	require.NoError(t, err)
	require.NotNil(t, chain.EffectiveShareExchange)
	// The revised link now names the replacement buy's lot.
	var revised *ShareExchangeLink
	for index, link := range chain.EffectiveShareExchange.Plan.Links {
		if link.BasisKnowledge == db.InvestmentBasisKnown {
			revised = &chain.EffectiveShareExchange.Plan.Links[index]
		}
	}
	require.NotNil(t, revised)
	assert.NotEqual(t, *bought.LotID, revised.SourceLotID)
	assert.Zero(t, exact.ScaledIntFromInt64(revised.CarriedBasisValue, revised.CarriedBasisScale).Cmp(
		exact.ScaledIntFromInt64(15000, 2)))
	assert.Equal(t, db.InvestmentBasisUnknown, chain.EffectiveShareExchange.Plan.BasisTotals[0].BasisKnowledge)
}
