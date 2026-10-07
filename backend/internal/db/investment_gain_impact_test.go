package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/exact"
)

func gainSnapshotEntry(decisionID int64, knowledge string, basis *exact.ScaledInt) investmentGainSnapshotEntry {
	state := InvestmentGainState{AccountID: 3, CommodityID: 4, CostCommodityID: 5,
		DisposalDate: "2026-03-01", CostBasisMethod: "fifo", Quantity: exact.ScaledIntFromInt64(5, 0),
		BasisKnowledge: knowledge, DisposedBasis: basis, Proceeds: exact.ScaledIntFromInt64(15000, 2)}
	if basis != nil {
		state.Gain = investmentGainValue(state.Proceeds, basis)
	}
	return investmentGainSnapshotEntry{operationID: 7, decisionID: decisionID, transactionID: 8, state: state}
}

func TestInvestmentGainComparisonDisclosesBasisKnowledgeTransitions(t *testing.T) {
	t.Parallel()
	identity := InvestmentGainIdentity{RootOperationID: 7, DecisionSeq: 1}
	known := gainSnapshotEntry(11, InvestmentBasisKnown, exact.ScaledIntFromInt64(10000, 2))
	unknown := gainSnapshotEntry(11, InvestmentBasisUnknown, nil)
	compare := func(before, after investmentGainSnapshotEntry) InvestmentGainImpact {
		return compareInvestmentGainSnapshots(map[InvestmentGainIdentity]investmentGainSnapshotEntry{identity: before},
			map[InvestmentGainIdentity]investmentGainSnapshotEntry{identity: after})
	}

	toUnknown := compare(known, unknown)
	require.Len(t, toUnknown.Changes, 1)
	require.Equal(t, GainImpactRevised, toUnknown.Changes[0].Kind)
	require.Nil(t, toUnknown.Changes[0].After.DisposedBasis, "unknown basis is never a fabricated zero")
	require.Nil(t, toUnknown.Changes[0].After.Gain)

	toKnown := compare(unknown, known)
	require.Len(t, toKnown.Changes, 1)
	require.Nil(t, toKnown.Changes[0].Before.Gain)
	require.Zero(t, toKnown.Changes[0].After.Gain.Cmp(exact.ScaledIntFromInt64(5000, 2)))
	require.NotEqual(t, toUnknown.Acknowledgement, toKnown.Acknowledgement)

	require.Empty(t, compare(unknown, unknown).Changes)
}

func TestInvestmentGainComparisonIsScaleInsensitiveAndBindsTheChangeSet(t *testing.T) {
	t.Parallel()
	identity := InvestmentGainIdentity{RootOperationID: 7, DecisionSeq: 1}
	before := gainSnapshotEntry(11, InvestmentBasisKnown, exact.ScaledIntFromInt64(10000, 2))
	sameValue := gainSnapshotEntry(11, InvestmentBasisKnown, exact.ScaledIntFromInt64(1000000, 4))
	compare := func(after investmentGainSnapshotEntry) InvestmentGainImpact {
		return compareInvestmentGainSnapshots(map[InvestmentGainIdentity]investmentGainSnapshotEntry{identity: before},
			map[InvestmentGainIdentity]investmentGainSnapshotEntry{identity: after})
	}
	require.Empty(t, compare(sameValue).Changes, "a restated scale is not a gain change")

	lower := compare(gainSnapshotEntry(11, InvestmentBasisKnown, exact.ScaledIntFromInt64(1000, 2)))
	lowerRestated := compare(gainSnapshotEntry(11, InvestmentBasisKnown, exact.ScaledIntFromInt64(10000, 3)))
	require.Equal(t, lower.Acknowledgement, lowerRestated.Acknowledgement, "the token is scale-normalized")
	other := compare(gainSnapshotEntry(11, InvestmentBasisKnown, exact.ScaledIntFromInt64(1001, 2)))
	require.NotEqual(t, lower.Acknowledgement, other.Acknowledgement)

	replaced := compare(gainSnapshotEntry(12, InvestmentBasisKnown, exact.ScaledIntFromInt64(1000, 2)))
	require.Equal(t, GainImpactReplaced, replaced.Changes[0].Kind)
	require.Equal(t, int64(11), replaced.Changes[0].DecisionID, "the disclosed ID is the committed decision")
	require.NotEqual(t, lower.Acknowledgement, replaced.Acknowledgement)

	require.NoError(t, requireInvestmentGainAcknowledgement(GainImpactPolicy{}, InvestmentGainImpact{}))
	require.NoError(t, requireInvestmentGainAcknowledgement(GainImpactPolicy{Acknowledgement: "old"}, InvestmentGainImpact{}))
	require.ErrorIs(t, requireInvestmentGainAcknowledgement(GainImpactPolicy{}, lower), ErrGainImpactAcknowledgementRequired)
	require.ErrorIs(t, requireInvestmentGainAcknowledgement(GainImpactPolicy{Acknowledgement: other.Acknowledgement}, lower), ErrGainImpactAcknowledgementStale)
	require.NoError(t, requireInvestmentGainAcknowledgement(GainImpactPolicy{Acknowledgement: lower.Acknowledgement}, lower))
}

func TestInvestmentGainSnapshotReadsRevisionKnowledgeAsOneTuple(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database := seedReplayTestBook(t)
	snapshot := func() investmentGainSnapshotEntry {
		tx, err := database.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer tx.Rollback()
		entries, err := investmentGainSnapshotTx(ctx, tx, 1)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		for _, entry := range entries {
			return entry
		}
		return investmentGainSnapshotEntry{}
	}
	revise := func(seq int, supersedes any, knowledge string, total any, allocations [][3]any) {
		var scale any
		if total != nil {
			scale = 2
		}
		result, err := database.ExecContext(ctx, `INSERT INTO investment_disposal_revisions (book_id, decision_id,
			revision_seq, caused_by_operation_id, supersedes_revision_id, disposed_basis_value, disposed_basis_scale,
			basis_knowledge, created_at, created_audit_event_id) VALUES (1, 1, ?, 3, ?, ?, ?, ?, '2026-10-07T00:00:00Z', 31)`,
			seq, supersedes, total, scale, knowledge)
		require.NoError(t, err)
		revisionID, err := result.LastInsertId()
		require.NoError(t, err)
		for index, allocation := range allocations {
			var basisScale any
			if allocation[1] != nil {
				basisScale = 2
			}
			_, err := database.ExecContext(ctx, `INSERT INTO investment_disposal_revision_allocations (book_id,
				revision_id, allocation_seq, lot_id, quantity_value, quantity_scale, cost_basis_value, cost_basis_scale,
				proceeds_value, proceeds_scale, basis_knowledge) VALUES (1, ?, ?, ?, ?, 2, ?, ?, ?, 2, ?)`,
				revisionID, index+1, allocation[0], []string{"1000", "250"}[index], allocation[1], basisScale,
				[]string{"120000", "30000"}[index], allocation[2])
			require.NoError(t, err)
		}
	}

	original := snapshot()
	require.Equal(t, InvestmentBasisKnown, original.state.BasisKnowledge)
	require.Zero(t, original.state.DisposedBasis.Cmp(exact.ScaledIntFromInt64(130000, 2)))

	revise(2, nil, InvestmentBasisUnknown, nil, [][3]any{{1, nil, "unknown"}, {2, "30000", "known"}})
	unresolved := snapshot()
	require.Equal(t, InvestmentBasisUnknown, unresolved.state.BasisKnowledge,
		"a revision's NULL total is never filled from the original snapshot")
	require.Nil(t, unresolved.state.DisposedBasis)
	require.Nil(t, unresolved.state.Gain)
	require.Zero(t, unresolved.state.Proceeds.Cmp(exact.ScaledIntFromInt64(150000, 2)), "proceeds stay known")

	identity := InvestmentGainIdentity{RootOperationID: 3, DecisionSeq: 1}
	toUnknown := compareInvestmentGainSnapshots(map[InvestmentGainIdentity]investmentGainSnapshotEntry{identity: original},
		map[InvestmentGainIdentity]investmentGainSnapshotEntry{identity: unresolved})
	require.Len(t, toUnknown.Changes, 1)
	require.Equal(t, GainImpactRevised, toUnknown.Changes[0].Kind)

	var latest int64
	require.NoError(t, database.QueryRowContext(ctx, `SELECT id FROM investment_disposal_revisions WHERE revision_seq = 2`).Scan(&latest))
	revise(3, latest, InvestmentBasisKnown, "0", [][3]any{{1, "0", "known"}, {2, "0", "known"}})
	knownZero := snapshot()
	require.Equal(t, InvestmentBasisKnown, knownZero.state.BasisKnowledge)
	require.Zero(t, knownZero.state.DisposedBasis.Sign())
	toZero := compareInvestmentGainSnapshots(map[InvestmentGainIdentity]investmentGainSnapshotEntry{identity: unresolved},
		map[InvestmentGainIdentity]investmentGainSnapshotEntry{identity: knownZero})
	require.Len(t, toZero.Changes, 1, "resolving to a known zero is a disclosed change, not equal to unknown")
	require.NotEqual(t, toUnknown.Acknowledgement, toZero.Acknowledgement)
}
