package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func resolveInput(f *investmentsTestFixture, transactionID, basis int64) ResolveTransferBasisInput {
	return ResolveTransferBasisInput{OwnerUserID: f.ownerUserID, TransactionID: transactionID,
		BasisValue: basis, BasisScale: 2, SourceEvidenceJSON: `{"statement":"old broker 2020"}`,
		Reason: "old broker statement found"}
}

// resolveAcknowledged previews a resolution and commits it with the preview's
// gain acknowledgement, as the resolution form does after confirmation.
func resolveAcknowledged(t *testing.T, f *investmentsTestFixture, input ResolveTransferBasisInput) *Transaction {
	t.Helper()
	impact, err := f.investmentService.ResolveTransferBasisImpact(context.Background(), input)
	require.NoError(t, err)
	if impact.GainImpact != nil {
		input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	}
	resolved, err := f.investmentService.ResolveTransferBasis(context.Background(), input)
	require.NoError(t, err)
	return resolved
}

func scaledCoefficient(t *testing.T, value string, scale int) *exact.ScaledInt {
	t.Helper()
	parsed, err := exact.Parse(value)
	require.NoError(t, err)
	return exact.ScaledIntFromCoefficient(parsed, scale)
}

// Resolving an inbound after a partial sale posts the complete omitted bridge
// dated to the transfer, resolves the sold units and the units still held,
// and leaves the original link and opening as unknown evidence.
func TestResolvingUnknownInboundAfterPartialSalePostsFullBridgeAndResolvesBoth(t *testing.T) {
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

	input := resolveInput(f, transfer.Transaction.ID, 8000)
	impact, err := f.investmentService.ResolveTransferBasisImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, impact.GainImpact.Changes, 1)
	change := impact.GainImpact.Changes[0]
	require.Equal(t, db.InvestmentBasisUnknown, change.Before.BasisKnowledge)
	require.Equal(t, db.InvestmentBasisKnown, change.After.BasisKnowledge)
	requireScaled(t, 4000, 2, change.After.DisposedBasis, "half the resolved basis left with the sale")
	again, err := f.investmentService.ResolveTransferBasisImpact(ctx, input)
	require.NoError(t, err)
	require.Equal(t, impact.GainImpact.Acknowledgement, again.GainImpact.Acknowledgement, "previews leave nothing behind")
	_, err = f.investmentService.ResolveTransferBasis(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	resolved := resolveAcknowledged(t, f, input)

	require.Equal(t, "2026-06-01", resolved.TransactionDate, "the bridge is dated to the transfer")
	require.Len(t, resolved.JournalEntries, 1)
	for _, posting := range resolved.JournalEntries[0].Postings {
		require.Equal(t, f.eurCommodityID, posting.CommodityID)
		amount := exact.ScaledIntFromCoefficient(posting.QuantityValue, posting.QuantityScale)
		if amount.Sign() < 0 {
			requireScaled(t, -8000, 2, amount, "equity side of the inbound bridge")
		} else {
			requireScaled(t, 8000, 2, amount, "trading side of the inbound bridge")
		}
	}
	require.Equal(t, db.InvestmentBasisKnown, effectiveDisposal(t, f, sale.id).knowledge)
	_, _, knowledge := lotStateByID(t, f, *transfer.LotID)
	require.Equal(t, db.InvestmentBasisKnown, knowledge)
	var remaining string
	var remainingScale int
	require.NoError(t, f.database.QueryRow(`SELECT remaining_cost_basis_value, remaining_cost_basis_scale FROM investment_lot_state
		WHERE lot_id = ?`, *transfer.LotID).Scan(&remaining, &remainingScale))
	requireScaled(t, 4000, 2, scaledCoefficient(t, remaining, remainingScale), "the held unit keeps the other half")
	var opening, link string
	require.NoError(t, f.database.QueryRow(`SELECT l.opening_basis_knowledge, x.basis_knowledge FROM investment_lots l
		JOIN investment_transfer_lot_links x ON x.destination_lot_id = l.id WHERE l.id = ?`, *transfer.LotID).Scan(&opening, &link))
	require.Equal(t, []string{"unknown", "unknown"}, []string{opening, link}, "original evidence is unchanged")
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Equal(t, db.InvestmentBasisKnown, gains[0].BasisKnowledge)
	requireHealthyUnresolvedBook(t, f)
}

// After a full sale nothing is held, yet resolution still revises the sale.
func TestResolvingUnknownInboundAfterFullSaleResolvesTheClosedLot(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	transfer, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-06-01", ""))
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 2))
	require.NoError(t, err)
	sale := latestDisposal(t, f)
	resolveAcknowledged(t, f, resolveInput(f, transfer.Transaction.ID, 8000))
	resolvedSale := effectiveDisposal(t, f, sale.id)
	require.Equal(t, db.InvestmentBasisKnown, resolvedSale.knowledge)
	requireScaled(t, 8000, 2, resolvedSale.basisAmount(t), "the closed lot's full resolved basis")
	quantity, _, _ := lotStateByID(t, f, *transfer.LotID)
	require.Equal(t, "0", quantity)
	requireHealthyUnresolvedBook(t, f)
}

// Resolution reaches units moved on internally: the destination link revises
// from unknown to known and the destination sale is resolved.
func TestResolvingUnknownInboundReachesInternalTransferDestination(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	transfer, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-05-01", ""))
	require.NoError(t, err)
	moved, err := f.investmentService.InternalTransfer(ctx, internalTransferFromLot(f, destinationID, *transfer.LotID, exact.New(1), 0))
	require.NoError(t, err)
	sale := sellInput(f, "2026-07-01", 1)
	sale.HoldingAccountID = destinationID
	_, err = f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	destinationSale := latestDisposal(t, f)

	resolveAcknowledged(t, f, resolveInput(f, transfer.Transaction.ID, 9000))
	require.Equal(t, []string{"known"}, linkKnowledge(t, f, moved.Transaction.ID))
	resolved := effectiveDisposal(t, f, destinationSale.id)
	require.Equal(t, db.InvestmentBasisKnown, resolved.knowledge)
	requireScaled(t, 4500, 2, resolved.basisAmount(t), "half the resolved basis moved and was sold")
	requireHealthyUnresolvedBook(t, f)
}

// An unknown outbound becomes known through its source's resolution and posts
// its complete omitted bridge at its own date, though its source has closed.
// A mixed outbound bridges its known and resolved links together.
func TestResolvingUnknownSourcePostsOmittedOutboundBridge(t *testing.T) {
	t.Parallel()
	for _, mixed := range []bool{false, true} {
		name := "unknown only"
		if mixed {
			name = "with a known lot"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newTransferOutFixture(t)
			ctx := context.Background()
			known := buyOn(t, f, "2026-05-01", 3, 3000)
			transfer, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-05-02", ""))
			require.NoError(t, err)
			input := transferOutOfLot(f, "2026-06-01", *transfer.LotID, 2)
			want := int64(6000)
			if mixed {
				input.Allocations = append(input.Allocations, InvestmentLotAllocationInput{LotID: *known.LotID, QuantityValue: exact.New(1)})
				want += 1000
			}
			outbound, err := f.investmentService.ExternalTransferOut(ctx, input)
			require.NoError(t, err)
			require.Empty(t, bridgePostings(t, f, outbound.Transaction.ID))

			resolveAcknowledged(t, f, resolveInput(f, transfer.Transaction.ID, 6000))
			bridge := bridgePostings(t, f, outbound.Transaction.ID)
			requireScaled(t, want, 2, bridge["external_investment_transfer_equity"], "complete omitted bridge, equity side")
			requireScaled(t, -want, 2, bridge["commodity_trading"], "complete omitted bridge, trading side")
			var bridgeDate string
			require.NoError(t, f.database.QueryRow(`SELECT v.transaction_date FROM investment_operation_journal_links link
				JOIN transaction_versions v ON v.id = link.transaction_version_id WHERE link.role = 'transfer_bridge'`).Scan(&bridgeDate))
			require.Equal(t, "2026-06-01", bridgeDate, "dated to the outbound transfer")
			links := linkKnowledge(t, f, outbound.Transaction.ID)
			for _, knowledge := range links {
				require.Equal(t, db.InvestmentBasisKnown, knowledge)
			}
			requireHealthyUnresolvedBook(t, f)
		})
	}
}

// An average pool holding unknown basis becomes definitive when the unknown
// opening resolves; the unresolved pool disposals are revised.
func TestResolvingUnknownLotResolvesAveragePoolDisposals(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	setHoldingCostBasisMethod(t, f, "average_cost")
	buyOn(t, f, "2026-01-01", 2, 2000)
	transfer, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-01-02", ""))
	require.NoError(t, err)
	input := sellInput(f, "2026-03-01", 2)
	input.CostBasisMethod = "average_cost"
	_, err = f.investmentService.Sell(ctx, input)
	require.NoError(t, err)
	sale := latestDisposal(t, f)
	require.Equal(t, db.InvestmentBasisUnknown, sale.knowledge)
	resolveAcknowledged(t, f, resolveInput(f, transfer.Transaction.ID, 6000))
	resolved := effectiveDisposal(t, f, sale.id)
	require.Equal(t, db.InvestmentBasisKnown, resolved.knowledge)
	// Pool of 4 units costing 20.00 + 60.00: two units take 40.00.
	requireScaled(t, 4000, 2, resolved.basisAmount(t), "two of four pooled units")
	requireHealthyUnresolvedBook(t, f)
}

func TestTransferBasisResolutionRefusals(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	known, err := f.investmentService.ExternalTransferIn(ctx, knownTransferInput(f))
	require.NoError(t, err)
	_, err = f.investmentService.ResolveTransferBasis(ctx, resolveInput(f, known.Transaction.ID, 100))
	require.ErrorIs(t, err, ErrInvestmentTransferBasisNotUnknown, "known basis has nothing to resolve")

	unknownInput := unknownTransferInInput(f, "2026-06-02", "")
	unknown, err := f.investmentService.ExternalTransferIn(ctx, unknownInput)
	require.NoError(t, err)
	negative := resolveInput(f, unknown.Transaction.ID, -100)
	_, err = f.investmentService.ResolveTransferBasis(ctx, negative)
	require.ErrorContains(t, err, "resolved basis must be nonnegative")
	missingReason := resolveInput(f, unknown.Transaction.ID, 100)
	missingReason.Reason = " "
	_, err = f.investmentService.ResolveTransferBasis(ctx, missingReason)
	require.ErrorContains(t, err, "resolution reason is required")

	resolveAcknowledged(t, f, resolveInput(f, unknown.Transaction.ID, 100))
	_, err = f.investmentService.ResolveTransferBasis(ctx, resolveInput(f, unknown.Transaction.ID, 200))
	require.ErrorIs(t, err, ErrInvestmentTransferBasisNotUnknown, "one resolution per transfer")

	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.investmentService.ReverseTransfer(ctx, ReverseInvestmentTransferInput{OwnerUserID: f.ownerUserID,
		TransactionID: unknown.Transaction.ID, Reason: "wrong broker"})
	require.ErrorIs(t, err, ErrInvestmentTransferBasisResolved, "the resolution is named, never silently dropped")
	replacement := unknownInput
	replacement.EffectiveOn = "2026-06-03"
	_, err = f.investmentService.ReplaceTransferIn(ctx, ReplaceInvestmentTransferInInput{OwnerUserID: f.ownerUserID,
		TransactionID: unknown.Transaction.ID, Reason: "date was wrong", Replacement: replacement})
	require.ErrorIs(t, err, ErrInvestmentTransferBasisResolved)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	requireHealthyUnresolvedBook(t, f)
}

// A healthy resolution passes the foundation check; a resolution whose pinned
// basis no longer matches its bridge journal is damage.
func TestSelfCheckDetectsResolutionBridgeDamage(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	transfer, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-06-01", ""))
	require.NoError(t, err)
	resolveAcknowledged(t, f, resolveInput(f, transfer.Transaction.ID, 8000))
	requireHealthyUnresolvedBook(t, f)

	_, err = f.database.Exec(`DROP TRIGGER investment_basis_resolutions_no_update`)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_basis_resolutions SET basis_value = '7000'`)
	require.NoError(t, err)
	foundation := resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation)
	require.Equal(t, SelfCheckFailed, foundation.Status)
	require.Contains(t, foundation.Summary, "basis resolution disagrees with its bridge or pinned transfer")
}

// A resolution that fails after its writes (a stale acknowledgement, checked
// once replay has run) leaves nothing behind: no fact, journal, revision or
// projection change. The same holds for a basis that overflows the position.
func TestTransferBasisResolutionLateRefusalsRollBackEverything(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	buyOn(t, f, "2026-01-01", 1, 100)
	transfer, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-06-01", "2020-03-01"))
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)
	before := buyReplacementPreviewSnapshot(t, f.database)

	stale := resolveInput(f, transfer.Transaction.ID, 8000)
	impact, err := f.investmentService.ResolveTransferBasisImpact(ctx, stale)
	require.NoError(t, err)
	stale.BasisValue = 9000 // a different change set than the one acknowledged
	stale.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	_, err = f.investmentService.ResolveTransferBasis(ctx, stale)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementStale)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))

	overflow := resolveInput(f, transfer.Transaction.ID, 9223372036854775807)
	overflow.BasisScale = 0
	_, err = f.investmentService.ResolveTransferBasisImpact(ctx, overflow)
	var validation ValidationError
	require.ErrorAs(t, err, &validation, "a basis the position cannot represent is refused as invalid, never a 500")
	require.Contains(t, validation.Message, "exceeds supported exact range")
	_, err = f.investmentService.ResolveTransferBasis(ctx, overflow)
	require.Error(t, err)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}

// A resolution stays in its transfer's cost currency: a USD-cost inbound in a
// holding that also holds EUR-cost lots is resolved and bridged in USD, and
// the EUR position is untouched; nothing is converted.
func TestResolutionKeepsItsTransferCostCurrencyWithoutFX(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	usd := seedTestCurrencyCommodity(t, f.database, "USD")
	eurLot := buyOn(t, f, "2026-01-01", 1, 1000)
	input := unknownTransferInInput(f, "2026-06-01", "")
	input.CostCommodityID = usd
	transfer, err := f.investmentService.ExternalTransferIn(ctx, input)
	require.NoError(t, err)
	resolved := resolveAcknowledged(t, f, resolveInput(f, transfer.Transaction.ID, 7000))
	for _, posting := range resolved.JournalEntries[0].Postings {
		require.Equal(t, usd, posting.CommodityID, "the bridge is in the transfer's own currency")
	}
	var cost int64
	require.NoError(t, f.database.QueryRow(`SELECT cost_commodity_id FROM investment_basis_resolutions`).Scan(&cost))
	require.Equal(t, usd, cost)
	_, basis, knowledge := lotStateByID(t, f, *eurLot.LotID)
	require.Equal(t, db.InvestmentBasisKnown, knowledge)
	require.Equal(t, "1000", basis.String, "the EUR-cost lot is not revised")
	requireHealthyUnresolvedBook(t, f)
}
