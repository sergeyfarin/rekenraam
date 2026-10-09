package app

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// seedUnknownOpeningAcquiredOn opens two units with unknown basis, entering the
// book on 2026-06-01. The sourced original date decides FIFO/LIFO priority.
func seedUnknownOpeningAcquiredOn(t *testing.T, f *investmentsTestFixture, originalOn string) int64 {
	t.Helper()
	seedExternalTransferEquity(t, f.database)
	input := knownTransferInput(f)
	input.CarriedBasisValue, input.CarriedBasisScale = 0, 0
	input.OriginalAcquiredOn = originalOn
	journal, transfer, err := f.investmentService.externalTransferInWrite(context.Background(), input)
	require.NoError(t, err)
	transfer.Lot.OpeningBasisKnowledge = db.InvestmentBasisUnknown
	_, lot, err := f.investmentService.repository.CreateExternalTransferIn(context.Background(), journal, transfer)
	require.NoError(t, err)
	return lot.ID
}

type storedDisposalAllocation struct {
	lotID     int64
	quantity  string
	basis     sql.NullString
	knowledge string
}

type storedDisposal struct {
	id          int64
	knowledge   string
	basis       sql.NullString
	basisScale  sql.NullInt64
	allocations map[int64]storedDisposalAllocation
}

// basisAmount is a known stored total at its own scale.
func (d storedDisposal) basisAmount(t *testing.T) *exact.ScaledInt {
	t.Helper()
	require.True(t, d.basis.Valid && d.basisScale.Valid, "disposal %d basis is unknown", d.id)
	parsed, err := exact.Parse(d.basis.String)
	require.NoError(t, err)
	return exact.ScaledIntFromCoefficient(parsed, int(d.basisScale.Int64))
}

// latestDisposal reads the newest decision's effective snapshot: its latest
// replay revision when one exists, else its original allocations.
func latestDisposal(t *testing.T, f *investmentsTestFixture) storedDisposal {
	t.Helper()
	var decision storedDisposal
	require.NoError(t, f.database.QueryRow(`SELECT id FROM investment_disposal_decisions ORDER BY id DESC LIMIT 1`).Scan(&decision.id))
	return effectiveDisposal(t, f, decision.id)
}

func effectiveDisposal(t *testing.T, f *investmentsTestFixture, decisionID int64) storedDisposal {
	t.Helper()
	decision := storedDisposal{id: decisionID, allocations: map[int64]storedDisposalAllocation{}}
	var revisionID sql.NullInt64
	require.NoError(t, f.database.QueryRow(`SELECT d.basis_knowledge, d.disposed_basis_value, d.disposed_basis_scale, r.id
		FROM investment_disposal_decisions d LEFT JOIN latest_investment_disposal_revisions r ON r.decision_id = d.id
		WHERE d.id = ?`, decisionID).Scan(&decision.knowledge, &decision.basis, &decision.basisScale, &revisionID))
	query := `SELECT lot_id, quantity_value, cost_basis_value, basis_knowledge FROM investment_disposal_allocations WHERE decision_id = ?`
	key := decisionID
	if revisionID.Valid {
		require.NoError(t, f.database.QueryRow(`SELECT basis_knowledge, disposed_basis_value, disposed_basis_scale FROM investment_disposal_revisions WHERE id = ?`,
			revisionID.Int64).Scan(&decision.knowledge, &decision.basis, &decision.basisScale))
		query = `SELECT lot_id, quantity_value, cost_basis_value, basis_knowledge FROM investment_disposal_revision_allocations WHERE revision_id = ?`
		key = revisionID.Int64
	}
	rows, err := f.database.Query(query, key)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var allocation storedDisposalAllocation
		require.NoError(t, rows.Scan(&allocation.lotID, &allocation.quantity, &allocation.basis, &allocation.knowledge))
		decision.allocations[allocation.lotID] = allocation
	}
	require.NoError(t, rows.Err())
	return decision
}

func lotStateByID(t *testing.T, f *investmentsTestFixture, lotID int64) (quantity string, basis sql.NullString, knowledge string) {
	t.Helper()
	require.NoError(t, f.database.QueryRow(`SELECT remaining_quantity_value, remaining_cost_basis_value, basis_knowledge
		FROM investment_lot_state WHERE lot_id = ?`, lotID).Scan(&quantity, &basis, &knowledge))
	return quantity, basis, knowledge
}

// requireHealthyUnresolvedBook checks that an unresolved book still replays
// exactly and conserves quantity and proceeds: unknown is information, not damage.
func requireHealthyUnresolvedBook(t *testing.T, f *investmentsTestFixture) {
	t.Helper()
	check := mustRunInvestmentSelfCheck(t, f)
	require.Equal(t, SelfCheckPassed, resultFor(t, check, CheckInvestmentReplay).Status, resultFor(t, check, CheckInvestmentReplay).Summary)
	require.Equal(t, SelfCheckPassed, resultFor(t, check, CheckInvestmentFoundation).Status, resultFor(t, check, CheckInvestmentFoundation).Summary)
	lots := resultFor(t, check, CheckLotReconciliation)
	require.NotContains(t, lots.Summary, "disposal allocation sets disagree")
	require.NotContains(t, lots.Summary, "holdings disagree")
	require.NotContains(t, lots.Summary, "disagree with their quantity events")
}

func TestSaleOfMixedKnownAndUnknownLotsLeavesGainUnresolvedInEitherOrder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, method, unknownOriginalOn string
	}{
		{"fifo unknown first", "fifo", "2020-03-01"},
		{"fifo known first", "fifo", "2026-05-01"},
		{"lifo unknown first", "lifo", "2026-05-01"},
		{"lifo known first", "lifo", "2020-03-01"},
		{"specific lot", "specific_lot", "2020-03-01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			known := buyOn(t, f, "2026-01-01", 10, 10000)
			unknownLotID := seedUnknownOpeningAcquiredOn(t, f, tc.unknownOriginalOn)
			input := sellInput(f, "2026-07-01", 11)
			input.CostBasisMethod = tc.method
			if tc.method == "specific_lot" {
				input.LotAllocations = []InvestmentLotAllocationInput{
					{LotID: unknownLotID, QuantityValue: exact.New(1)}, {LotID: *known.LotID, QuantityValue: exact.New(10)}}
			}

			preview, err := f.investmentService.PreviewSell(ctx, input)
			require.NoError(t, err)
			require.Equal(t, db.InvestmentBasisUnknown, preview.BasisKnowledge, "no gain against a partial basis")
			require.Equal(t, db.InvestmentBasisUnknown, preview.DisposalDecision.BasisKnowledge)
			_, err = f.investmentService.Sell(ctx, input)
			require.NoError(t, err)

			sale := latestDisposal(t, f)
			require.Equal(t, db.InvestmentBasisUnknown, sale.knowledge)
			require.False(t, sale.basis.Valid, "unknown total is NULL, never the known part")
			require.Len(t, sale.allocations, 2, "both lots take part whichever comes first")
			require.Equal(t, db.InvestmentBasisUnknown, sale.allocations[unknownLotID].knowledge)
			require.False(t, sale.allocations[unknownLotID].basis.Valid)
			knownAllocation := sale.allocations[*known.LotID]
			require.Equal(t, db.InvestmentBasisKnown, knownAllocation.knowledge, "a known lot keeps its own amount")
			require.True(t, knownAllocation.basis.Valid)

			// Quantities are exact: 12 held, 11 sold, whichever lot keeps one.
			unknownQuantity, unknownBasis, unknownKnowledge := lotStateByID(t, f, unknownLotID)
			knownQuantity, _, knownKnowledge := lotStateByID(t, f, *known.LotID)
			remaining := exact.NewScaledInt()
			for _, quantity := range []string{unknownQuantity, knownQuantity} {
				parsed, err := exact.Parse(quantity)
				require.NoError(t, err)
				remaining.AddCoefficient(parsed, 0)
			}
			requireScaled(t, 1, 0, remaining, "remaining units")
			require.Equal(t, db.InvestmentBasisUnknown, unknownKnowledge)
			require.False(t, unknownBasis.Valid)
			require.Equal(t, db.InvestmentBasisKnown, knownKnowledge, "individual lots keep their own knowledge")

			gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
			require.NoError(t, err)
			require.Len(t, gains, 1)
			require.Equal(t, db.InvestmentBasisUnknown, gains[0].BasisKnowledge)
			require.Equal(t, "11", gains[0].QuantityValue.String())
			require.NotZero(t, gains[0].ProceedsValue, "proceeds stay known")
			requireHealthyUnresolvedBook(t, f)
		})
	}
}

func TestSaleOfOnlyKnownLotsStaysKnownBesideAnUnknownLot(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	known := buyOn(t, f, "2026-01-01", 10, 10000)
	seedUnknownOpeningAcquiredOn(t, f, "2026-05-01")
	preview, err := f.investmentService.PreviewSell(ctx, sellInput(f, "2026-07-01", 5))
	require.NoError(t, err)
	require.Equal(t, db.InvestmentBasisKnown, preview.BasisKnowledge)
	requireScaled(t, 5000, 2, exact.ScaledIntFromInt64(preview.RealizedGain, preview.RealizedGainScale), "gain")
	_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 5))
	require.NoError(t, err)
	sale := latestDisposal(t, f)
	require.Equal(t, db.InvestmentBasisKnown, sale.knowledge)
	require.Len(t, sale.allocations, 1)
	require.Contains(t, sale.allocations, *known.LotID)
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Equal(t, db.InvestmentBasisKnown, gains[0].BasisKnowledge)
	requireScaled(t, 5000, 2, exact.ScaledIntFromInt64(gains[0].RealizedGainValue, gains[0].RealizedGainScale), "realized gain")
	requireHealthyUnresolvedBook(t, f)
}

// Selling every unknown unit closes the lot without manufacturing a known
// historical gain or a known zero remainder.
func TestClosingUnknownQuantityDoesNotManufactureKnownGain(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	unknownLotID := seedUnknownOpeningAcquiredOn(t, f, "2020-03-01")
	_, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 2))
	require.NoError(t, err)
	quantity, basis, knowledge := lotStateByID(t, f, unknownLotID)
	require.Equal(t, "0", quantity)
	require.Equal(t, db.InvestmentBasisUnknown, knowledge)
	require.False(t, basis.Valid)
	sale := latestDisposal(t, f)
	require.Equal(t, db.InvestmentBasisUnknown, sale.knowledge)
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Equal(t, db.InvestmentBasisUnknown, gains[0].BasisKnowledge)
	require.Zero(t, gains[0].RealizedGainValue, "the numeric field is unused, not a reported zero")
	requireHealthyUnresolvedBook(t, f)
}

// An average pool holding unknown basis has no definitive rate: every
// disposal from it stays unresolved. Once the pool is exhausted, a new known
// acquisition opens a known pool and earlier disposals keep their labels.
func TestAverageCostPoolWithUnknownBasisStaysUnresolvedUntilExhausted(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	known := buyOn(t, f, "2026-01-01", 10, 10000)
	unknownLotID := seedUnknownOpeningAcquiredOn(t, f, "2020-03-01")
	sell := func(date string, quantity int64) storedDisposal {
		input := sellInput(f, date, quantity)
		input.CostBasisMethod = "average_cost"
		_, err := f.investmentService.Sell(ctx, input)
		require.NoError(t, err)
		return latestDisposal(t, f)
	}

	first := sell("2026-07-01", 6)
	require.Equal(t, db.InvestmentBasisUnknown, first.knowledge)
	for lotID, allocation := range first.allocations {
		require.Equalf(t, db.InvestmentBasisUnknown, allocation.knowledge, "lot %d: a pool share has no known rate", lotID)
		require.False(t, allocation.basis.Valid)
	}
	_, basis, knowledge := lotStateByID(t, f, *known.LotID)
	require.Equal(t, db.InvestmentBasisUnknown, knowledge, "pooled remainder is unknown, even on a known-opening lot")
	require.False(t, basis.Valid)
	var original string
	require.NoError(t, f.database.QueryRow(`SELECT opening_basis_knowledge FROM investment_lots WHERE id = ?`, *known.LotID).Scan(&original))
	require.Equal(t, db.InvestmentBasisKnown, original, "immutable opening evidence is unchanged")

	second := sell("2026-07-02", 6)
	require.Equal(t, db.InvestmentBasisUnknown, second.knowledge)
	for _, lotID := range []int64{*known.LotID, unknownLotID} {
		quantity, _, _ := lotStateByID(t, f, lotID)
		require.Equal(t, "0", quantity)
	}

	buyOn(t, f, "2026-07-03", 3, 3000)
	third := sell("2026-07-04", 1)
	require.Equal(t, db.InvestmentBasisKnown, third.knowledge, "a new pool after exhaustion is known")
	require.True(t, third.basis.Valid)
	require.Zero(t, exact.ScaledIntFromCoefficient(exact.Coefficient(third.basis.String), 6).Cmp(exact.ScaledIntFromInt64(1000, 2)),
		"the new pool rate is 10.00 per unit")
	require.Equal(t, db.InvestmentBasisUnknown, effectiveDisposal(t, f, first.id).knowledge, "earlier disposals are not relabeled")
	require.Equal(t, db.InvestmentBasisUnknown, effectiveDisposal(t, f, second.id).knowledge)

	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Len(t, gains, 3)
	unresolved := 0
	for _, gain := range gains {
		if gain.BasisKnowledge == db.InvestmentBasisUnknown {
			unresolved++
		}
	}
	require.Equal(t, 2, unresolved)
	requireHealthyUnresolvedBook(t, f)
}

// A backdated known purchase can take an unresolved sale's place in FIFO.
// Replay then resolves that sale; the transition is a disclosed gain change
// that needs acknowledgement, and the original unknown evidence stays.
func TestBackdatedKnownBuyResolvingAnUnknownSaleIsDisclosed(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-02-01", 10, 10000)
	unknownLotID := seedUnknownOpeningAcquiredOn(t, f, "2026-05-01")
	_, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 11))
	require.NoError(t, err)
	sale := latestDisposal(t, f)
	require.Equal(t, db.InvestmentBasisUnknown, sale.knowledge)

	backdated := backdatedBuy(f, "2026-01-10", 5, 2500)
	impact := previewBuyGainImpact(t, f, backdated)
	require.Len(t, impact.Changes, 1)
	change := impact.Changes[0]
	require.Equal(t, db.GainImpactRevised, change.Kind)
	require.Equal(t, db.InvestmentBasisUnknown, change.Before.BasisKnowledge)
	require.Nil(t, change.Before.Gain)
	require.Equal(t, db.InvestmentBasisKnown, change.After.BasisKnowledge)
	require.NotNil(t, change.After.Gain)
	_, err = f.investmentService.Buy(ctx, backdated)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	acknowledgedBuy(t, f, backdated)

	resolved := effectiveDisposal(t, f, sale.id)
	require.Equal(t, db.InvestmentBasisKnown, resolved.knowledge)
	require.NotContains(t, resolved.allocations, unknownLotID, "FIFO now takes the earlier known lots")
	var originalKnowledge string
	require.NoError(t, f.database.QueryRow(`SELECT basis_knowledge FROM investment_disposal_decisions WHERE id = ?`, sale.id).Scan(&originalKnowledge))
	require.Equal(t, db.InvestmentBasisUnknown, originalKnowledge, "the original decision stays unknown evidence")
	quantity, _, knowledge := lotStateByID(t, f, unknownLotID)
	require.Equal(t, "2", quantity)
	require.Equal(t, db.InvestmentBasisUnknown, knowledge)
	requireHealthyUnresolvedBook(t, f)
}

// A sale dated behind a later sale is admitted through replay; it may take
// unknown basis, and the later sale's revision keeps its own knowledge.
func TestBackdatedSaleThroughReplayAdmitsUnknownBasis(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-01", 10, 10000)
	seedUnknownOpeningAcquiredOn(t, f, "2020-03-01")
	_, err := f.investmentService.Sell(ctx, sellInput(f, "2026-08-01", 1))
	require.NoError(t, err)
	later := latestDisposal(t, f)
	require.Equal(t, db.InvestmentBasisUnknown, later.knowledge, "FIFO takes the older unknown lot first")

	backdated := sellInput(f, "2026-07-01", 2)
	preview, err := f.investmentService.PreviewSell(ctx, backdated)
	require.NoError(t, err)
	require.Equal(t, db.InvestmentBasisUnknown, preview.BasisKnowledge)
	impact, err := f.investmentService.TradeReconciliationImpact(ctx, InvestmentImpactSell, backdated)
	require.NoError(t, err)
	backdated.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	_, err = f.investmentService.Sell(ctx, backdated)
	require.NoError(t, err)
	require.Equal(t, db.InvestmentBasisUnknown, latestDisposal(t, f).knowledge)
	revised := effectiveDisposal(t, f, later.id)
	require.Equal(t, db.InvestmentBasisKnown, revised.knowledge, "the unknown units went to the earlier sale")
	requireHealthyUnresolvedBook(t, f)
}

// Reversing an unresolved sale restores the exact quantity through replay and
// leaves the lot's basis unknown; nothing is resolved as a side effect.
func TestReversingAnUnresolvedSaleRestoresQuantityAndKeepsBasisUnknown(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	unknownLotID := seedUnknownOpeningAcquiredOn(t, f, "2020-03-01")
	sold, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)
	reversal := ReverseInvestmentSaleInput{OwnerUserID: f.ownerUserID,
		TransactionID: sold.Transaction.ID, Reason: "entered on the wrong holding"}
	impact, err := f.investmentService.ReverseSaleReconciliationImpact(ctx, reversal)
	require.NoError(t, err)
	require.Len(t, impact.GainImpact.Changes, 1)
	removed := impact.GainImpact.Changes[0]
	require.Equal(t, db.GainImpactRemoved, removed.Kind)
	require.Equal(t, db.InvestmentBasisUnknown, removed.Before.BasisKnowledge, "the removed gain was unresolved, not zero")
	require.Nil(t, removed.Before.Gain)
	reversal.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	_, err = f.investmentService.ReverseSale(ctx, reversal)
	require.NoError(t, err)
	quantity, basis, knowledge := lotStateByID(t, f, unknownLotID)
	require.Equal(t, "2", quantity)
	require.Equal(t, db.InvestmentBasisUnknown, knowledge)
	require.False(t, basis.Valid)
	gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
	require.NoError(t, err)
	require.Empty(t, gains, "a reversed sale has no effective disposal")
	requireHealthyUnresolvedBook(t, f)
}
