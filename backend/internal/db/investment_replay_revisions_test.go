package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/exact"
)

func TestInvestmentReplayRevisionPreservesOriginalAndInstallsEffectiveState(t *testing.T) {
	ctx := context.Background()
	database := seedReplayTestBook(t)
	tx, err := database.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer rollbackTx(ctx, tx)
	intents, err := investmentReplayIntentsQuery(ctx, tx, 1, 15, 2, 1, "long")
	require.NoError(t, err)
	intents[0].AmountValue = exact.New(120000)
	projection, err := simulateInvestmentReplayTx(ctx, tx, 1, 15, 2, 1, intents)
	require.NoError(t, err)
	require.NoError(t, persistInvestmentReplayProjectionTx(ctx, tx, 1, 15, 2, 1,
		3, 31, 1, "2026-09-27T10:00:00Z", intents, projection))
	assertOriginalReplayRowsUnchanged(t, tx, 4)
	var revisionSeq, causeID, auditID, effectiveBasisScale int
	var effectiveBasis string
	require.NoError(t, tx.QueryRow(`SELECT revision_seq, caused_by_operation_id,
		disposed_basis_value, disposed_basis_scale, created_audit_event_id FROM investment_disposal_revisions
		WHERE decision_id = 1`).Scan(&revisionSeq, &causeID, &effectiveBasis, &effectiveBasisScale, &auditID))
	require.Equal(t, 2, revisionSeq)
	require.Equal(t, 3, causeID)
	basisValue, err := exact.Parse(effectiveBasis)
	require.NoError(t, err)
	require.Zero(t, exact.ScaledIntFromCoefficient(basisValue, effectiveBasisScale).Cmp(exact.ScaledIntFromInt64(1500, 0)))
	require.Equal(t, 31, auditID)
	var allocationCount int
	require.NoError(t, tx.QueryRow(`SELECT count(*) FROM investment_disposal_revision_allocations`).Scan(&allocationCount))
	require.Equal(t, 2, allocationCount)
	var remainingBasis int64
	var remainingBasisScale int
	require.NoError(t, tx.QueryRow(`SELECT remaining_cost_basis_value, remaining_cost_basis_scale
		FROM investment_lots WHERE id = 2`).Scan(&remainingBasis, &remainingBasisScale))
	require.Zero(t, exact.ScaledIntFromInt64(remainingBasis, remainingBasisScale).Cmp(exact.ScaledIntFromInt64(300, 0)))
	require.NoError(t, tx.Commit())

	_, err = database.Exec(`UPDATE investment_disposal_revisions SET disposed_basis_value = '0' WHERE decision_id = 1`)
	require.ErrorContains(t, err, "immutable")
	_, err = database.Exec(`DELETE FROM investment_disposal_revision_allocations`)
	require.ErrorContains(t, err, "immutable")
}

func TestInvestmentReplayRevisionRejectsIncompleteAllocationBeforeWriting(t *testing.T) {
	ctx := context.Background()
	database := seedReplayTestBook(t)
	tx, err := database.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer rollbackTx(ctx, tx)
	intents, err := investmentReplayIntentsQuery(ctx, tx, 1, 15, 2, 1, "long")
	require.NoError(t, err)
	projection, err := simulateInvestmentReplayTx(ctx, tx, 1, 15, 2, 1, intents)
	require.NoError(t, err)
	projection.Disposals[0].Allocations = projection.Disposals[0].Allocations[:1]
	err = persistInvestmentReplayProjectionTx(ctx, tx, 1, 15, 2, 1,
		3, 31, 1, "2026-09-27T10:00:00Z", intents, projection)
	require.ErrorContains(t, err, "conserve quantity and proceeds")
	var count int
	require.NoError(t, tx.QueryRow(`SELECT count(*) FROM investment_disposal_revisions`).Scan(&count))
	require.Zero(t, count)
	assertOriginalReplayRowsUnchanged(t, tx, 4)
}

func TestInvestmentReplayRevisionRejectsCrossPositionAllocation(t *testing.T) {
	ctx := context.Background()
	database := seedReplayTestBook(t)
	tx, err := database.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer rollbackTx(ctx, tx)
	intents, err := investmentReplayIntentsQuery(ctx, tx, 1, 15, 2, 1, "long")
	require.NoError(t, err)
	projection, err := simulateInvestmentReplayTx(ctx, tx, 1, 15, 2, 1, intents)
	require.NoError(t, err)
	projection.Disposals[0].Allocations[0].LotID = 999
	err = persistInvestmentReplayProjectionTx(ctx, tx, 1, 15, 2, 1,
		3, 31, 1, "2026-09-27T10:00:00Z", intents, projection)
	require.ErrorContains(t, err, "allocation is invalid")
	var count int
	require.NoError(t, tx.QueryRow(`SELECT count(*) FROM investment_disposal_revisions`).Scan(&count))
	require.Zero(t, count)
}

func TestRealizedGainsUseLatestEffectiveReplayRevision(t *testing.T) {
	ctx := context.Background()
	database := seedReplayTestBook(t)
	repo := NewInvestmentRepository(database)
	assertGain := func(basis, gain int64) {
		t.Helper()
		rows, err := repo.ListRealizedGains(ctx, 1, RealizedGainsParams{})
		require.NoError(t, err)
		require.Len(t, rows, 1, "a replayed sale must appear once")
		require.Zero(t, exact.ScaledIntFromInt64(rows[0].DisposedBasisValue, rows[0].DisposedBasisScale).Cmp(exact.ScaledIntFromInt64(-basis, 0)))
		require.Zero(t, exact.ScaledIntFromInt64(rows[0].RealizedGainValue, rows[0].RealizedGainScale).Cmp(exact.ScaledIntFromInt64(gain, 0)))
		require.Zero(t, exact.ScaledIntFromInt64(rows[0].ProceedsValue, rows[0].ProceedsScale).Cmp(exact.ScaledIntFromInt64(1500, 0)))
	}
	assertGain(1300, 200)
	for _, basis := range []struct {
		opening int64
		want    int64
		gain    int64
	}{
		{120000, 1500, 0},
		{110000, 1400, 100},
	} {
		tx, err := database.BeginTx(ctx, nil)
		require.NoError(t, err)
		intents, err := investmentReplayIntentsQuery(ctx, tx, 1, 15, 2, 1, "long")
		require.NoError(t, err)
		intents[0].AmountValue = exact.New(basis.opening)
		projection, err := simulateInvestmentReplayTx(ctx, tx, 1, 15, 2, 1, intents)
		require.NoError(t, err)
		require.NoError(t, persistInvestmentReplayProjectionTx(ctx, tx, 1, 15, 2, 1,
			3, 31, 1, "2026-09-27T10:00:00Z", intents, projection))
		require.NoError(t, tx.Commit())
		assertGain(basis.want, basis.gain)
	}
	var revisionSeq int
	var previousID int64
	require.NoError(t, database.QueryRow(`SELECT revision_seq, supersedes_revision_id
		FROM investment_disposal_revisions ORDER BY revision_seq DESC LIMIT 1`).Scan(&revisionSeq, &previousID))
	require.Equal(t, 3, revisionSeq)
	require.Positive(t, previousID)
	filtered, err := repo.ListRealizedGains(ctx, 1, RealizedGainsParams{From: "2026-04-03"})
	require.NoError(t, err)
	require.Empty(t, filtered)
	snapshot, err := database.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer rollbackTx(ctx, snapshot)
	exporter := NewExportRepository(database)
	revisions, err := exporter.ExportInvestmentFoundation(ctx, snapshot, 1, "disposal-revisions")
	require.NoError(t, err)
	require.Len(t, revisions, 2, "the bundle must preserve both effective revisions")
	require.Equal(t, "2", revisions[0][2])
	require.Equal(t, "3", revisions[1][2])
	require.Equal(t, revisions[0][0], revisions[1][4], "revision 3 supersedes revision 2")
	allocations, err := exporter.ExportInvestmentFoundation(ctx, snapshot, 1, "disposal-revision-allocations")
	require.NoError(t, err)
	require.Len(t, allocations, 4)
}

func TestSelfCheckLotEventsUseEffectiveDisposalAfterReplay(t *testing.T) {
	ctx := context.Background()
	database := seedReplayTestBook(t)
	tx, err := database.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer rollbackTx(ctx, tx)
	intents, err := investmentReplayIntentsQuery(ctx, tx, 1, 15, 2, 1, "long")
	require.NoError(t, err)
	intents[2].CostBasisMethod = "lifo"
	projection, err := simulateInvestmentReplayTx(ctx, tx, 1, 15, 2, 1, intents)
	require.NoError(t, err)
	require.NoError(t, persistInvestmentReplayProjectionTx(ctx, tx, 1, 15, 2, 1,
		3, 31, 1, "2026-09-27T10:00:00Z", intents, projection))
	checker := NewSelfCheckRepository(database, database)
	events, err := checker.SelfCheckLotEvents(ctx, tx, 1)
	require.NoError(t, err)
	lots, err := checker.SelfCheckLots(ctx, tx, 1)
	require.NoError(t, err)
	require.Len(t, events, 4, "two openings plus two effective disposal allocations")
	require.Len(t, lots, 2)
	quantities := map[int64]*exact.ScaledInt{}
	basisFromEvents, basisRemaining := exact.NewScaledInt(), exact.NewScaledInt()
	for _, event := range events {
		if quantities[event.LotID] == nil {
			quantities[event.LotID] = exact.NewScaledInt()
		}
		quantities[event.LotID].AddCoefficient(event.QuantityValue, event.QuantityScale)
		basisFromEvents.AddInt64(event.CostBasisValue, event.CostBasisScale)
	}
	for _, lot := range lots {
		require.Zero(t, quantities[lot.LotID].Cmp(exact.ScaledIntFromCoefficient(lot.RemainingQuantityValue, lot.RemainingQuantityScale)))
		basisRemaining.AddInt64(lot.RemainingCostBasisValue, lot.RemainingCostBasisScale)
	}
	require.Zero(t, basisFromEvents.Cmp(basisRemaining))
	require.Zero(t, basisRemaining.Cmp(exact.ScaledIntFromInt64(250, 0)), "LIFO leaves 2.5 shares of the older 1000 EUR lot")
}
