package db

import (
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
