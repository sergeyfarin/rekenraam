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
