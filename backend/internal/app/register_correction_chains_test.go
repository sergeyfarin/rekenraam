package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// T-120 #135. The register keeps the original, reversal and replacement as
// separate posted rows — the running balance counts each posting once — and
// groups them by an annotation every row carries in full, so a member on
// another page is still named and the chain's net effect is counted once.

func cashRegister(t *testing.T, f *investmentsTestFixture, limit int, cursor string) AccountRegisterResult {
	t.Helper()
	result, err := f.transactionService.Register(context.Background(), f.cashAccountID, ListTransactionsInput{Limit: limit, Cursor: cursor})
	require.NoError(t, err)
	return result
}

func requireChainAmount(t *testing.T, expected int64, scale int, amount RegisterCorrectionAmount) {
	t.Helper()
	require.Zero(t, exact.ScaledIntFromInt64(expected, scale).Cmp(
		exact.ScaledIntFromCoefficient(amount.QuantityValue, amount.QuantityScale)),
		"got %s at scale %d", amount.QuantityValue, amount.QuantityScale)
}

func memberRoles(chain *RegisterCorrectionChain) []string {
	roles := []string{}
	for _, member := range chain.Members {
		roles = append(roles, member.Role)
	}
	return roles
}

func TestRegisterGroupsCorrectionChainAndCountsNetOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	unrelated := postEUR(t, f, "2026-01-05", 50)
	original := buyForNetting(t, f, "2026-01-01", 10, 100000)
	replaced, err := acknowledgedReplaceBuy(ctx, f.investmentService,
		buyReplacementForNetting(f, original.Transaction.ID, "2026-01-02", 10, 120000))
	require.NoError(t, err)

	register := cashRegister(t, f, 50, "")
	require.Len(t, register.Entries, 4)
	chained := 0
	for _, entry := range register.Entries {
		if entry.TransactionID == unrelated.ID {
			require.Nil(t, entry.CorrectionChain, "a transaction outside any chain is not grouped")
			continue
		}
		chained++
		chain := entry.CorrectionChain
		require.NotNil(t, chain)
		require.Equal(t, original.Transaction.ID, chain.RootTransactionID)
		require.Equal(t, []string{"original", "reversal", "replacement"}, memberRoles(chain))
		require.Equal(t, replaced.Replacement.Transaction.ID, *chain.EffectiveTransactionID)
		requireChainAmount(t, -120000, 2, chain.NetEffect)
		requireChainAmount(t, -100000, 2, *chain.Members[0].Amount)
		requireChainAmount(t, 100000, 2, *chain.Members[1].Amount)
		requireChainAmount(t, -120000, 2, *chain.Members[2].Amount)
		require.Equal(t, "broker corrected the fill", chain.Members[2].Reason)
		require.Equal(t, original.Transaction.ID, *chain.Members[1].CorrectionOfTransactionID)
		switch entry.TransactionID {
		case original.Transaction.ID:
			require.Equal(t, "original", chain.Role)
		case replaced.Inverse.ID:
			require.Equal(t, "reversal", chain.Role)
		case replaced.Replacement.Transaction.ID:
			require.Equal(t, "replacement", chain.Role)
		}
	}
	require.Equal(t, 3, chained)
	// The newest row's running balance is the ledger balance: the net effect
	// once (−1,200.00) plus the unrelated +50.
	requireChainAmount(t, -115000, 2, RegisterCorrectionAmount{
		QuantityValue: register.Entries[0].RunningBalance.QuantityValue,
		QuantityScale: register.Entries[0].RunningBalance.QuantityScale,
	})
}

func TestRegisterCorrectionChainSurvivesPagination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	original := buyForNetting(t, f, "2026-01-01", 10, 100000)
	_, err := acknowledgedReplaceBuy(ctx, f.investmentService,
		buyReplacementForNetting(f, original.Transaction.ID, "2026-03-01", 10, 120000))
	require.NoError(t, err)

	cursor, pages := "", 0
	for {
		page := cashRegister(t, f, 1, cursor)
		if len(page.Entries) == 0 {
			break
		}
		pages++
		require.Len(t, page.Entries[0].CorrectionChain.Members, 3, "page %d names the whole chain", pages)
		requireChainAmount(t, -120000, 2, page.Entries[0].CorrectionChain.NetEffect)
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	require.Equal(t, 3, pages)
}

func TestRegisterChainEndingInReversalHasNoEffectiveTransaction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	original := buyForNetting(t, f, "2026-01-01", 10, 100000)
	_, err := acknowledgedReverseBuy(ctx, f.investmentService, ReverseInvestmentBuyInput{
		OwnerUserID: f.ownerUserID, TransactionID: original.Transaction.ID, Reason: "never executed",
	})
	require.NoError(t, err)

	for _, entry := range cashRegister(t, f, 50, "").Entries {
		require.NotNil(t, entry.CorrectionChain)
		require.Equal(t, []string{"original", "reversal"}, memberRoles(entry.CorrectionChain))
		require.Nil(t, entry.CorrectionChain.EffectiveTransactionID)
		requireChainAmount(t, 0, 0, entry.CorrectionChain.NetEffect)
	}
}

func TestRegisterChainedCorrectionsShareOneChain(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	original := buyForNetting(t, f, "2026-01-01", 10, 100000)
	first, err := acknowledgedReplaceBuy(ctx, f.investmentService,
		buyReplacementForNetting(f, original.Transaction.ID, "2026-01-01", 10, 110000))
	require.NoError(t, err)
	second, err := acknowledgedReplaceBuy(ctx, f.investmentService,
		buyReplacementForNetting(f, first.Replacement.Transaction.ID, "2026-01-01", 10, 105000))
	require.NoError(t, err)

	entries := cashRegister(t, f, 50, "").Entries
	require.Len(t, entries, 5)
	for _, entry := range entries {
		chain := entry.CorrectionChain
		require.Equal(t, original.Transaction.ID, chain.RootTransactionID)
		require.Equal(t, []string{"original", "reversal", "replacement", "reversal", "replacement"}, memberRoles(chain))
		require.Equal(t, second.Replacement.Transaction.ID, *chain.EffectiveTransactionID)
		require.Equal(t, first.Replacement.Transaction.ID, *chain.Members[3].CorrectionOfTransactionID)
		requireChainAmount(t, -105000, 2, chain.NetEffect)
	}
}
